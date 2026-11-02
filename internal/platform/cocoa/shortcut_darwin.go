//go:build darwin

package cocoa

import (
	"fmt"
	"sync"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Global shortcuts with Carbon hot keys (ADR-224), the API that
// Electron's globalShortcut, MASShortcut and the HotKey package use:
// RegisterEventHotKey needs no permission (an event tap or an NSEvent
// global monitor needs the accessibility one) and reports releases.
//
//   - The accelerator's modifiers become Carbon's (shortcut.go) and its
//     key a virtual key code: fixed for the named keys and the space bar,
//     looked up in the current keyboard layout for characters
//     (UCKeyTranslate over every key code, cached per input source and
//     read again when the layout changed). The code is registered when
//     RegisterShortcut is called: a layout chosen later does not move a
//     registered shortcut (Win32's RegisterHotKey behaves the same).
//   - Registration uses kEventHotKeyExclusive, as the other apps that
//     register hot keys do, so a combination another app holds is
//     eventHotKeyExistsErr, ShortcutTaken. The same combination twice in
//     this process is that error too.
//   - One Carbon event handler, installed once per process on the
//     application event target, receives kEventHotKeyPressed and
//     kEventHotKeyReleased from the event dispatch of [NSApp run] on the
//     main thread. The hot key's EventHotKeyID ('TELA' and a number
//     unique in the process) finds the shortcut. A held hot key is
//     reported once until it is released.
//   - Carbon answers at once; the ShortcutStatusEvent is queued, so it
//     reaches the app from a later pass, as the contract wants.
//
// Every hot key is unregistered when Run returns.

// shortcutState is the backend's hot keys, on the main thread.
type shortcutState struct {
	byID   map[string]*hotkey
	layout *keyLayout // the cached layout, nil until read
}

// hotkey is a registered hot key.
type hotkey struct {
	b       *Backend
	id      string // the shortcut's ID
	num     uint32 // EventHotKeyID.ID
	ref     uintptr
	pressed bool
}

// hotkeys maps the EventHotKeyID numbers of the process to their hot
// keys, for the handler.
var hotkeys = struct {
	sync.Mutex
	last  uint32
	byNum map[uint32]*hotkey
}{byNum: map[uint32]*hotkey{}}

func (d desktop) RegisterShortcut(s platform.Shortcut) error {
	platform.CheckShortcut(s)
	b := d.b
	if b.closed {
		return errClosed
	}
	if err := darwin.LoadCarbon(); err != nil {
		return fmt.Errorf("cocoa: RegisterShortcut: Carbon hot keys are missing: %w (%w)", platform.ErrUnsupported, err)
	}
	if err := darwin.InstallHotKeyHandler(onHotKey); err != nil {
		return fmt.Errorf("cocoa: RegisterShortcut: %w (%w)", platform.ErrUnsupported, err)
	}
	b.registerShortcut(s)
	return nil
}

func (d desktop) UnregisterShortcut(id string) error {
	platform.CheckShortcutID("UnregisterShortcut", id)
	d.b.unregisterShortcut(id)
	return nil
}

// registerShortcut registers s and queues the ShortcutStatusEvent with
// the outcome. Registering an ID again replaces its hot key: the old one
// goes first, so asking for the same combination again works.
func (b *Backend) registerShortcut(s platform.Shortcut) {
	b.unregisterShortcut(s.ID)
	report := func(status platform.ShortcutStatus, reason string) {
		ev := platform.ShortcutStatusEvent{ID: s.ID, Status: status, Reason: reason}
		if status == platform.ShortcutBound {
			ev.Trigger = macTrigger(s.Accelerator)
		}
		b.enqueue(nil, ev)
		b.notify()
	}
	code, err := hotkeyKeyCode(s.Accelerator.Key, b.keyLayout())
	if err != nil {
		report(platform.ShortcutFailed, err.Error())
		return
	}
	hotkeys.Lock()
	hotkeys.last++
	num := hotkeys.last
	hotkeys.Unlock()
	ref, st := darwin.RegisterEventHotKey(uint32(code), hotkeyModifiers(s.Accelerator.Mods),
		darwin.EventHotKeyID{Signature: hotkeySignature, ID: num}, darwin.KEventHotKeyExclusive)
	switch {
	case st == darwin.EventHotKeyExistsErr:
		report(platform.ShortcutTaken, fmt.Sprintf("%s is registered already, by another application or by this one (eventHotKeyExistsErr)", macTrigger(s.Accelerator)))
		return
	case st != 0 || ref == 0:
		report(platform.ShortcutFailed, fmt.Sprintf("RegisterEventHotKey(key code %#x, modifiers %#x) returned OSStatus %d", code, hotkeyModifiers(s.Accelerator.Mods), st))
		return
	}
	h := &hotkey{b: b, id: s.ID, num: num, ref: ref}
	hotkeys.Lock()
	hotkeys.byNum[num] = h
	hotkeys.Unlock()
	if b.desk.keys.byID == nil {
		b.desk.keys.byID = map[string]*hotkey{}
	}
	b.desk.keys.byID[s.ID] = h
	report(platform.ShortcutBound, "")
}

// unregisterShortcut removes the hot key of a shortcut; an unknown ID is
// ignored.
func (b *Backend) unregisterShortcut(id string) {
	h := b.desk.keys.byID[id]
	if h == nil {
		return
	}
	delete(b.desk.keys.byID, id)
	hotkeys.Lock()
	delete(hotkeys.byNum, h.num)
	hotkeys.Unlock()
	darwin.UnregisterEventHotKey(h.ref)
}

// stopShortcuts unregisters every hot key (Run returns).
func (b *Backend) stopShortcuts() {
	for id := range b.desk.keys.byID {
		b.unregisterShortcut(id)
	}
}

// keyLayout returns the characters of the current keyboard layout, read
// again when the input source changed since the last call; nil when it
// cannot be read (hotkeyKeyCode then uses the ANSI layout's keys).
func (b *Backend) keyLayout() *keyLayout {
	k := &b.desk.keys
	id := darwin.KeyLayoutID()
	if k.layout != nil && k.layout.id == id {
		return k.layout
	}
	lid, plain, shifted, err := darwin.KeyLayoutCharacters()
	if err != nil {
		k.layout = nil
		return nil
	}
	k.layout = &keyLayout{id: lid, plain: plain, shifted: shifted}
	return k.layout
}

// onHotKey is the Carbon handler's function: a press or release of a hot
// key with Tela's signature becomes a ShortcutEvent of its backend.
// Repeated presses while the keys are held are dropped.
func onHotKey(kind uint32, id darwin.EventHotKeyID) bool {
	if id.Signature != hotkeySignature {
		return false
	}
	hotkeys.Lock()
	h := hotkeys.byNum[id.ID]
	hotkeys.Unlock()
	if h == nil {
		return true // unregistered meanwhile
	}
	b := h.b
	b.guard(func() {
		action := platform.ShortcutPressed
		switch kind {
		case darwin.KEventHotKeyPressed:
			if h.pressed {
				return
			}
			h.pressed = true
		case darwin.KEventHotKeyReleased:
			if !h.pressed {
				return
			}
			h.pressed = false
			action = platform.ShortcutReleased
		default:
			return
		}
		b.appEvent(platform.ShortcutEvent{ID: h.id, Action: action, Time: b.Now()})
	})
	return true
}
