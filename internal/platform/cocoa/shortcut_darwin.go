//go:build darwin

package cocoa

import (
	"cmp"
	"fmt"
	"slices"
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
//     read again when the layout changed).
//   - The layout is followed (ADR-237): an observer of the distributed
//     notification kTISNotifySelectedKeyboardInputSourceChanged
//     (NSDistributedNotificationCenter, delivered immediately even while
//     the app is in the background, since a global shortcut works there)
//     reads the new layout and moves every hot key whose character is
//     typed by another key now (relayoutPlan): the moving hot keys are
//     unregistered first, then registered again, so two that trade keys
//     do not collide. A shortcut declared by character stays on the key
//     that types it, as macOS menus' key equivalents do. A status that
//     changes (a key the new layout lacks is ShortcutFailed, another
//     app's combination ShortcutTaken, and back to ShortcutBound when a
//     later layout gives it a key again) is a new ShortcutStatusEvent;
//     a shortcut whose key only moved reports nothing (its trigger is the
//     same). A hot key held while it moves gets its release. Shortcuts
//     that failed when registered are kept and tried again at each
//     change of their target. Win32's RegisterHotKey still keeps the
//     virtual key of the layout at registration.
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
	// byID holds every shortcut registered and not unregistered, bound or
	// not (a failed one is tried again when the layout moves its key).
	byID   map[string]*hotkey
	layout *keyLayout // the cached layout, nil until read
	// observer is the TelaLayoutObserver of the input source changes
	// (owned), or 0 before the first shortcut.
	observer darwin.ID
	// layoutChanges counts the input source notifications (tests).
	layoutChanges int
}

// hotkey is a shortcut and its Carbon hot key.
type hotkey struct {
	b       *Backend
	id      string // the shortcut's ID
	accel   platform.Accelerator
	num     uint32 // EventHotKeyID.ID
	ref     uintptr
	target  hotkeyTarget            // the key code it is registered (or was tried) on
	status  platform.ShortcutStatus // the last status reported
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
	b.observeLayout()
	hotkeys.Lock()
	hotkeys.last++
	h := &hotkey{b: b, id: s.ID, accel: s.Accelerator, num: hotkeys.last}
	hotkeys.byNum[h.num] = h
	hotkeys.Unlock()
	if b.desk.keys.byID == nil {
		b.desk.keys.byID = map[string]*hotkey{}
	}
	b.desk.keys.byID[s.ID] = h
	h.target = targetOf(h.accel.Key, b.keyLayout())
	status, reason := h.bind()
	h.status = status
	h.report(status, reason)
}

// bind registers the hot key on h.target and returns the status.
func (h *hotkey) bind() (platform.ShortcutStatus, string) {
	if h.target.err != "" {
		return platform.ShortcutFailed, h.target.err
	}
	mods := hotkeyModifiers(h.accel.Mods)
	ref, st := darwin.RegisterEventHotKey(uint32(h.target.code), mods,
		darwin.EventHotKeyID{Signature: hotkeySignature, ID: h.num}, darwin.KEventHotKeyExclusive)
	switch {
	case st == darwin.EventHotKeyExistsErr:
		return platform.ShortcutTaken, fmt.Sprintf("%s is registered already, by another application or by this one (eventHotKeyExistsErr)", macTrigger(h.accel))
	case st != 0 || ref == 0:
		return platform.ShortcutFailed, fmt.Sprintf("RegisterEventHotKey(key code %#x, modifiers %#x) returned OSStatus %d", h.target.code, mods, st)
	}
	h.ref = ref
	return platform.ShortcutBound, ""
}

// unbind unregisters the hot key, if registered; a press that waits for
// its release gets it now.
func (h *hotkey) unbind() {
	if h.ref != 0 {
		darwin.UnregisterEventHotKey(h.ref)
		h.ref = 0
	}
	if h.pressed {
		h.pressed = false
		h.b.appEvent(platform.ShortcutEvent{ID: h.id, Action: platform.ShortcutReleased, Time: h.b.Now()})
	}
}

// report queues a ShortcutStatusEvent of h.
func (h *hotkey) report(status platform.ShortcutStatus, reason string) {
	ev := platform.ShortcutStatusEvent{ID: h.id, Status: status, Reason: reason}
	if status == platform.ShortcutBound {
		ev.Trigger = macTrigger(h.accel)
	}
	h.b.enqueue(nil, ev)
	h.b.notify()
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
	if h.ref != 0 {
		darwin.UnregisterEventHotKey(h.ref)
		h.ref = 0
	}
}

// stopShortcuts unregisters every hot key and stops observing the layout
// (Run returns).
func (b *Backend) stopShortcuts() {
	for id := range b.desk.keys.byID {
		b.unregisterShortcut(id)
	}
	k := &b.desk.keys
	if k.observer != 0 {
		darwin.Send(distributedCenter(), setSel.removeObserver, k.observer)
		layoutObservers.Lock()
		delete(layoutObservers.byID, k.observer)
		layoutObservers.Unlock()
		b.releaseLater(k.observer)
		k.observer = 0
	}
}

// relayoutShortcuts moves the hot keys to the keys that type their
// characters in the current layout (see the top of the file).
func (b *Backend) relayoutShortcuts() {
	k := &b.desk.keys
	if len(k.byID) == 0 {
		return
	}
	layout := b.keyLayout()
	list := make([]*hotkey, 0, len(k.byID))
	for _, h := range k.byID {
		list = append(list, h)
	}
	slices.SortFunc(list, func(x, y *hotkey) int { return cmp.Compare(x.num, y.num) })
	old := make([]hotkeyTarget, len(list))
	now := make([]hotkeyTarget, len(list))
	for i, h := range list {
		old[i], now[i] = h.target, targetOf(h.accel.Key, layout)
	}
	moves := relayoutPlan(old, now)
	for _, i := range moves {
		list[i].unbind()
	}
	for _, i := range moves {
		h := list[i]
		h.target = now[i]
		status, reason := h.bind()
		if status != h.status {
			h.status = status
			h.report(status, "the keyboard layout changed: "+reasonOr(reason, "the shortcut is bound again"))
		}
	}
}

func reasonOr(reason, def string) string {
	if reason == "" {
		return def
	}
	return reason
}

// --- following the keyboard layout ---

// layoutObservers maps the TelaLayoutObserver objects to their backends.
var layoutObservers = struct {
	sync.Mutex
	once  sync.Once
	class darwin.Class
	err   error
	byID  map[darwin.ID]*Backend
}{byID: map[darwin.ID]*Backend{}}

var selLayoutChanged = darwin.Sel("layoutChanged:")

func distributedCenter() darwin.ID {
	return darwin.SendClass(darwin.GetClass("NSDistributedNotificationCenter"), setSel.defaultCenter)
}

// nsNotificationSuspensionBehaviorDeliverImmediately delivers a
// distributed notification even while the app is inactive (NSApplication
// suspends the others then).
const nsNotificationSuspensionBehaviorDeliverImmediately = 4

// observeLayout starts observing the input source changes, once per run.
// Without the observer the hot keys stay on the keys of the layout they
// were registered in.
func (b *Backend) observeLayout() {
	k := &b.desk.keys
	if k.observer != 0 {
		return
	}
	lo := &layoutObservers
	lo.once.Do(func() {
		lo.class, lo.err = darwin.RegisterClass("TelaLayoutObserver", darwin.GetClass("NSObject"), nil, []darwin.Method{
			{Sel: selLayoutChanged, Fn: layoutChanged},
		})
	})
	if lo.err != nil {
		return
	}
	center := distributedCenter()
	if center == 0 {
		return
	}
	k.observer = darwin.Send(darwin.SendClass(lo.class, sel.alloc), sel.init)
	lo.Lock()
	lo.byID[k.observer] = b
	lo.Unlock()
	darwin.Send(center, shortcutSel.addObserverSuspension, k.observer, selLayoutChanged,
		darwin.AutoString(darwin.SelectedInputSourceChangedName()), darwin.ID(0),
		uint(nsNotificationSuspensionBehaviorDeliverImmediately))
}

var shortcutSel = struct{ addObserverSuspension darwin.SEL }{
	addObserverSuspension: darwin.Sel("addObserver:selector:name:object:suspensionBehavior:"),
}

// layoutChanged is -[TelaLayoutObserver layoutChanged:], on the main
// thread.
func layoutChanged(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
	layoutObservers.Lock()
	b := layoutObservers.byID[self]
	layoutObservers.Unlock()
	if b == nil || b.closed {
		return
	}
	b.guard(func() {
		b.desk.keys.layoutChanges++
		b.relayoutShortcuts()
	})
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
