package win32

import (
	"fmt"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure part of the global shortcuts (ADR-209): the mapping of an
// Accelerator to what RegisterHotKey takes, and the allocation of the
// hotkey IDs. hotkey_windows.go does the OS calls.

// Messages of the app-level services that nothing else in the package
// names.
const (
	wmHotKey                      = 0x0312
	wmCopyData                    = 0x004A
	wmSysColorChange              = 0x0015
	wmDwmColorizationColorChanged = 0x0320
)

// RegisterHotKey modifier flags (fsModifiers).
const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000 // Windows Vista and later: holding the keys sends one WM_HOTKEY
)

// errHotKeyAlreadyRegistered is ERROR_HOTKEY_ALREADY_REGISTERED: another
// application holds the combination.
const errHotKeyAlreadyRegistered = 1409

// hotkeyModifiers converts the accelerator's modifiers to the fsModifiers
// of RegisterHotKey, with MOD_NOREPEAT.
func hotkeyModifiers(m platform.Modifiers) uint32 {
	f := uint32(modNoRepeat)
	if m&platform.ModAlt != 0 {
		f |= modAlt
	}
	if m&platform.ModCtrl != 0 {
		f |= modControl
	}
	if m&platform.ModShift != 0 {
		f |= modShift
	}
	if m&platform.ModSuper != 0 {
		f |= modWin
	}
	return f
}

// vkByNamedKey is namedVK in reverse (and the function keys): the virtual
// key of a named logical key. Where several virtual keys share a name
// (Clear), the lowest wins.
var vkByNamedKey = sync.OnceValue(func() map[platform.Key]uint32 {
	m := map[platform.Key]uint32{}
	for vk, k := range namedVK {
		if k == "" {
			continue
		}
		if _, dup := m[k]; !dup {
			m[k] = uint32(vk)
		}
	}
	for n := 1; n <= 24; n++ {
		m[platform.Key("F"+strconv.Itoa(n))] = vkF1 + uint32(n-1)
	}
	return m
})

// hotkeyVK returns the virtual key that RegisterHotKey registers for an
// accelerator's key.
//
// A named key (KeyF1, KeyArrowUp, KeyEscape) has a fixed virtual key. A
// character is the one the key types without Shift, so it is looked up in
// the keyboard layout: scan is VkKeyScanExW, which finds the key that
// produces the character at any shift level. That is the key whose cap
// shows the character, which is what a user means by "Ctrl+1" on a layout
// that needs Shift for the digit, so the shift state scan reports is not
// added to the modifiers. When the layout has no such key (a Cyrillic
// layout asked for "k"), letters and digits fall back to their own virtual
// keys (VK_A to VK_Z, VK_0 to VK_9, which are the ASCII codes) and the
// punctuation of the US layout to its OEM keys. scan may be nil.
func hotkeyVK(key platform.Key, scan func(r rune) (vk uint32, ok bool)) (uint32, error) {
	if key.IsNamed() {
		if vk, ok := vkByNamedKey()[key]; ok {
			return vk, nil
		}
		return 0, fmt.Errorf("the key %q has no virtual key on Windows", key)
	}
	r, n := utf8.DecodeRuneInString(string(key))
	if r == utf8.RuneError || n != len(key) {
		return 0, fmt.Errorf("the key %q is not a single character", key)
	}
	if scan != nil {
		if vk, ok := scan(r); ok {
			return vk, nil
		}
	}
	if vk, ok := fallbackVK(r); ok {
		return vk, nil
	}
	return 0, fmt.Errorf("the key %q is not on the current keyboard layout", key)
}

// hotkeyRebind decides, after the keyboard layout changed, whether a
// shortcut must be registered again (ADR-234). A shortcut declared by
// character is bound to the virtual key that types the character in the
// layout of its registration, so a layout where another key types it moves
// it; a named key has a fixed virtual key. held is the virtual key the
// shortcut holds (0 for none: it failed for lack of a key, or was refused
// at its first registration and may find one now). It returns the key the
// new layout gives (vk, or err when it has none) and whether to register
// again: when the key changed, or when a shortcut without one finds one.
// A shortcut that has none and still has none stays as it is, and its
// status is not reported again.
func hotkeyRebind(key platform.Key, held uint32, scan func(r rune) (vk uint32, ok bool)) (vk uint32, again bool, err error) {
	if key.IsNamed() {
		return held, false, nil
	}
	vk, err = hotkeyVK(key, scan)
	if err != nil {
		return 0, held != 0, err
	}
	return vk, vk != held, nil
}

// hotkeyReg is a shortcut asked for with RegisterShortcut. It is kept
// whether or not RegisterHotKey took it, so that a change of the keyboard
// layout can bind it again (relayoutHotkeys).
type hotkeyReg struct {
	s          platform.Shortcut
	id         int32  // the RegisterHotKey ID (0: none was allocated)
	vk         uint32 // the virtual key it was bound to; 0: the layout had none
	registered bool   // RegisterHotKey succeeded
	// status is the last status reported for it, so that a layout change
	// reports only a status that changes.
	status platform.ShortcutStatus
}

// relayoutHotkeys moves the hot keys of regs, in order, after the keyboard
// layout changed (ADR-234): hotkeyRebind picks those that must be
// registered again. It works in two passes, as Cocoa's relayoutShortcuts
// does: every moving hot key is released first (unbind, for those that are
// registered), and only then are they registered under their new keys
// (bind), so two shortcuts that trade keys (Ctrl+A and Ctrl+Q from QWERTY
// to AZERTY) do not collide with the app's own old registrations, which
// RegisterHotKey refuses as it refuses another app's. A status that
// changes is reported (report, with the reason); a shortcut whose key only
// moved stays Bound with the same trigger and reports nothing.
func relayoutHotkeys(regs []*hotkeyReg, scan func(r rune) (vk uint32, ok bool),
	unbind func(*hotkeyReg), bind func(*hotkeyReg) (platform.ShortcutStatus, string),
	report func(r *hotkeyReg, status platform.ShortcutStatus, reason string)) {
	type move struct {
		reg *hotkeyReg
		vk  uint32
		err error
	}
	var moves []move
	for _, reg := range regs {
		vk, again, err := hotkeyRebind(reg.s.Accelerator.Key, reg.vk, scan)
		if !again {
			continue
		}
		moves = append(moves, move{reg, vk, err})
		if reg.registered {
			unbind(reg)
		}
	}
	for _, m := range moves {
		m.reg.vk = m.vk
		status, reason := platform.ShortcutFailed, ""
		if m.err != nil {
			reason = m.err.Error()
		} else {
			status, reason = bind(m.reg)
		}
		if status != m.reg.status {
			m.reg.status = status
			if status != platform.ShortcutBound {
				reason = "the keyboard layout changed: " + reason
			}
			report(m.reg, status, reason)
		}
	}
}

// fallbackVK is the virtual key of a character when the layout does not
// name one: letters, digits and the space bar everywhere, and the
// punctuation keys of the US layout.
func fallbackVK(r rune) (uint32, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return uint32(r - 'a' + 'A'), true
	case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return uint32(r), true
	}
	if vk, ok := usPunctuationVK[r]; ok {
		return vk, true
	}
	return 0, false
}

// usPunctuationVK are the OEM virtual keys of the US layout.
var usPunctuationVK = map[rune]uint32{
	' ':  vkSpace,
	';':  0xBA, // VK_OEM_1
	'=':  0xBB, // VK_OEM_PLUS
	',':  0xBC, // VK_OEM_COMMA
	'-':  0xBD, // VK_OEM_MINUS
	'.':  0xBE, // VK_OEM_PERIOD
	'/':  0xBF, // VK_OEM_2
	'`':  0xC0, // VK_OEM_3
	'[':  0xDB, // VK_OEM_4
	'\\': 0xDC, // VK_OEM_5
	']':  0xDD, // VK_OEM_6
	'\'': 0xDE, // VK_OEM_7
}

// maxHotKeyID is the largest ID RegisterHotKey takes for an application
// (0xC000 to 0xFFFF belong to shared libraries).
const maxHotKeyID = 0xBFFF

// hotkeyIDs hands out the integer IDs RegisterHotKey needs for the string
// IDs of the shortcuts: 1 to maxHotKeyID, each used once at a time, the
// next one after the last handed out so that the ID of a recent
// WM_HOTKEY is not reused at once.
type hotkeyIDs struct {
	last int32
	used map[int32]bool
}

// alloc returns an unused ID; ok is false when all 0xBFFF are in use.
func (h *hotkeyIDs) alloc() (id int32, ok bool) {
	if h.used == nil {
		h.used = map[int32]bool{}
	}
	if len(h.used) >= maxHotKeyID {
		return 0, false
	}
	for {
		h.last = h.last%maxHotKeyID + 1
		if !h.used[h.last] {
			h.used[h.last] = true
			return h.last, true
		}
	}
}

// free gives id back.
func (h *hotkeyIDs) free(id int32) { delete(h.used, id) }
