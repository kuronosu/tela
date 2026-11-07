package cocoa

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure part of the global shortcuts (ADR-224): what Carbon's
// RegisterEventHotKey takes for an Accelerator (the modifiers and the
// virtual key code), and how the trigger is shown. shortcut_darwin.go does
// the OS calls; this file is tested on every OS.

// Carbon modifier bits (HIToolbox/Events.h), the same as the darwin
// package's CmdKey, ShiftKey, OptionKey and ControlKey.
const (
	carbonCmdKey     = 1 << 8
	carbonShiftKey   = 1 << 9
	carbonOptionKey  = 1 << 11
	carbonControlKey = 1 << 12
)

// hotkeySignature is the signature of Tela's EventHotKeyIDs, 'TELA': the
// handler leaves hot keys with another signature (a library in the same
// process) to their owners.
const hotkeySignature = 0x54454C41

// hotkeyModifiers converts the accelerator's modifiers to Carbon's:
// Super is Command (an accelerator's CmdOrCtrl is Super already on a Mac,
// platform.ParseAccelerator), Alt is Option.
func hotkeyModifiers(m platform.Modifiers) uint32 {
	var f uint32
	if m&platform.ModSuper != 0 {
		f |= carbonCmdKey
	}
	if m&platform.ModShift != 0 {
		f |= carbonShiftKey
	}
	if m&platform.ModAlt != 0 {
		f |= carbonOptionKey
	}
	if m&platform.ModCtrl != 0 {
		f |= carbonControlKey
	}
	return f
}

// keyLayout is what the current keyboard layout types with each virtual
// key code below 128: without modifiers (plain) and with Shift (shifted),
// "" for keys that type nothing (UCKeyTranslate, darwin.KeyLayoutCharacters).
// id is the layout's input source identifier, which the backend's cache
// compares to the current one.
type keyLayout struct {
	id             string
	plain, shifted [128]string
}

// The virtual key codes (HIToolbox/Events.h kVK_*) of the named keys,
// fixed whatever the layout. The key at the Insert position of a PC
// keyboard is kVK_Help, so Insert and Help share it.
var namedKeyCodes = map[platform.Key]uint16{
	platform.KeyEnter:     0x24, // kVK_Return
	platform.KeyTab:       0x30, // kVK_Tab
	platform.KeyBackspace: 0x33, // kVK_Delete
	platform.KeyEscape:    0x35, // kVK_Escape
	platform.KeyDelete:    0x75, // kVK_ForwardDelete
	platform.KeyInsert:    0x72, // kVK_Help
	platform.KeyHelp:      0x72, // kVK_Help
	platform.KeyHome:      0x73, // kVK_Home
	platform.KeyEnd:       0x77, // kVK_End
	platform.KeyPageUp:    0x74, // kVK_PageUp
	platform.KeyPageDown:  0x79, // kVK_PageDown

	platform.KeyArrowLeft:  0x7B, // kVK_LeftArrow
	platform.KeyArrowRight: 0x7C, // kVK_RightArrow
	platform.KeyArrowDown:  0x7D, // kVK_DownArrow
	platform.KeyArrowUp:    0x7E, // kVK_UpArrow

	platform.KeyF1: 0x7A, platform.KeyF2: 0x78, platform.KeyF3: 0x63, platform.KeyF4: 0x76,
	platform.KeyF5: 0x60, platform.KeyF6: 0x61, platform.KeyF7: 0x62, platform.KeyF8: 0x64,
	platform.KeyF9: 0x65, platform.KeyF10: 0x6D, platform.KeyF11: 0x67, platform.KeyF12: 0x6F,
	platform.KeyF13: 0x69, platform.KeyF14: 0x6B, platform.KeyF15: 0x71, platform.KeyF16: 0x6A,
	platform.KeyF17: 0x40, platform.KeyF18: 0x4F, platform.KeyF19: 0x50, platform.KeyF20: 0x5A,

	platform.KeyClear:       0x47, // kVK_ANSI_KeypadClear
	platform.KeyContextMenu: 0x6E, // kVK_ContextualMenu
	platform.KeyEisu:        0x66, // kVK_JIS_Eisu
	platform.KeyKanaMode:    0x68, // kVK_JIS_Kana

	platform.KeyAudioVolumeUp:   0x48, // kVK_VolumeUp
	platform.KeyAudioVolumeDown: 0x49, // kVK_VolumeDown
	platform.KeyAudioVolumeMute: 0x4A, // kVK_Mute
}

// kvkSpace is kVK_Space, the space bar on every layout.
const kvkSpace = 0x31

// keypadCodes are the virtual key codes of the numeric keypad: a
// character is looked for on the main keys, never there (Cmd+1 is the
// digit row's 1, as on Windows).
var keypadCodes = map[uint16]bool{
	0x41: true, 0x43: true, 0x45: true, 0x47: true, 0x4B: true, 0x4C: true, 0x4E: true,
	0x51: true, 0x52: true, 0x53: true, 0x54: true, 0x55: true, 0x56: true, 0x57: true,
	0x58: true, 0x59: true, 0x5B: true, 0x5C: true, 0x5F: true,
}

// ansiKeyCodes are the characters of the ANSI (US) layout's keys, the
// kVK_ANSI_* codes: the fallback for a character the current layout does
// not type (a Cyrillic layout asked for "k").
var ansiKeyCodes = map[rune]uint16{
	'a': 0x00, 's': 0x01, 'd': 0x02, 'f': 0x03, 'h': 0x04, 'g': 0x05, 'z': 0x06, 'x': 0x07,
	'c': 0x08, 'v': 0x09, 'b': 0x0B, 'q': 0x0C, 'w': 0x0D, 'e': 0x0E, 'r': 0x0F, 'y': 0x10,
	't': 0x11, '1': 0x12, '2': 0x13, '3': 0x14, '4': 0x15, '6': 0x16, '5': 0x17, '=': 0x18,
	'9': 0x19, '7': 0x1A, '-': 0x1B, '8': 0x1C, '0': 0x1D, ']': 0x1E, 'o': 0x1F, 'u': 0x20,
	'[': 0x21, 'i': 0x22, 'p': 0x23, 'l': 0x25, 'j': 0x26, '\'': 0x27, 'k': 0x28, ';': 0x29,
	'\\': 0x2A, ',': 0x2B, '/': 0x2C, 'n': 0x2D, 'm': 0x2E, '.': 0x2F, '`': 0x32,
}

// hotkeyKeyCode returns the virtual key code RegisterEventHotKey registers
// for an accelerator's key.
//
// A named key (KeyF1, KeyArrowUp, KeyEscape) and the space bar have fixed
// codes. A character is the one the key types without Shift, looked up in
// the current layout (layout, nil when unknown): the first main key whose
// plain text is the character, else the first whose shifted text is
// (Ctrl++ on a US keyboard is the key of '=' and '+', and Shift is not
// added: the key whose cap shows the character, as Win32's VkKeyScanExW
// finds it). A character the layout does not type falls back to the ANSI
// layout's key, so Cmd+K stays on the K position under a Cyrillic layout.
func hotkeyKeyCode(key platform.Key, layout *keyLayout) (uint16, error) {
	if key.IsNamed() {
		if c, ok := namedKeyCodes[key]; ok {
			return c, nil
		}
		return 0, fmt.Errorf("the key %q has no key code on macOS", key)
	}
	r, n := utf8.DecodeRuneInString(string(key))
	if r == utf8.RuneError || n != len(key) {
		return 0, fmt.Errorf("the key %q is not a single character", key)
	}
	if r == ' ' {
		return kvkSpace, nil
	}
	if layout != nil {
		for _, table := range []*[128]string{&layout.plain, &layout.shifted} {
			for code := range uint16(len(table)) {
				if !keypadCodes[code] && layoutTypes(table[code], r) {
					return code, nil
				}
			}
		}
	}
	if c, ok := ansiKeyCodes[unicode.ToLower(r)]; ok {
		return c, nil
	}
	return 0, fmt.Errorf("the key %q is not on the current keyboard layout", key)
}

// layoutTypes reports whether text, what a key types, is the character r:
// one printable character, compared without regard to case (a layout
// whose plain letters are capitals, such as one with Caps Lock in its
// tables, still names the key).
func layoutTypes(text string, r rune) bool {
	t, n := utf8.DecodeRuneInString(text)
	if n == 0 || n != len(text) || !unicode.IsPrint(t) || t >= 0xF700 && t <= 0xF8FF {
		return false
	}
	return t == r || unicode.ToLower(t) == unicode.ToLower(r)
}

// macKeyGlyphs are the symbols macOS menus show for named keys.
var macKeyGlyphs = map[platform.Key]string{
	platform.KeyEnter:      "↩",
	platform.KeyTab:        "⇥",
	platform.KeyBackspace:  "⌫",
	platform.KeyDelete:     "⌦",
	platform.KeyEscape:     "⎋",
	platform.KeyHome:       "↖",
	platform.KeyEnd:        "↘",
	platform.KeyPageUp:     "⇞",
	platform.KeyPageDown:   "⇟",
	platform.KeyArrowLeft:  "←",
	platform.KeyArrowRight: "→",
	platform.KeyArrowUp:    "↑",
	platform.KeyArrowDown:  "↓",
	platform.KeyClear:      "⌧",
}

// macTrigger formats a as macOS shows a key combination in menus and the
// keyboard settings: the modifier symbols in the order Control, Option,
// Shift, Command (⌃⌥⇧⌘), then the key, letters upper-cased, named keys as
// their symbols (↩, ⎋, ←) or names (F5), the space bar as "Space":
// "⌃⌥⌘K", "⇧⌘F5". It is the Trigger of ShortcutBound.
func macTrigger(a platform.Accelerator) string {
	var b strings.Builder
	for _, m := range [...]struct {
		m platform.Modifiers
		s string
	}{{platform.ModCtrl, "⌃"}, {platform.ModAlt, "⌥"}, {platform.ModShift, "⇧"}, {platform.ModSuper, "⌘"}} {
		if a.Mods&m.m != 0 {
			b.WriteString(m.s)
		}
	}
	switch g, ok := macKeyGlyphs[a.Key]; {
	case ok:
		b.WriteString(g)
	case a.Key == " ":
		b.WriteString("Space")
	case a.Key.IsNamed():
		b.WriteString(string(a.Key))
	default:
		b.WriteString(strings.ToUpper(string(a.Key)))
	}
	return b.String()
}

// hotkeyTarget is where a shortcut's hot key goes in one keyboard layout:
// the virtual key code, or why its key has none (err, ShortcutFailed).
type hotkeyTarget struct {
	code uint16
	err  string
}

// targetOf returns the hot key target of key in layout (hotkeyKeyCode).
func targetOf(key platform.Key, layout *keyLayout) hotkeyTarget {
	code, err := hotkeyKeyCode(key, layout)
	if err != nil {
		return hotkeyTarget{err: err.Error()}
	}
	return hotkeyTarget{code: code}
}

// relayoutPlan returns the shortcuts whose hot key moves when the keyboard
// layout changes: the indexes i where now[i], the target under the new
// layout, differs from old[i], the one the hot key was registered (or
// tried) with, in order. A named key never moves; a character moves to
// the key that types it now, and one the old layout did not type (an
// error, or the ANSI fallback) may find its key. The backend unregisters
// every moving hot key first and then registers them again in this order,
// so two shortcuts whose keys trade places (AZERTY's A and Q) do not take
// each other's old combination.
func relayoutPlan(old, now []hotkeyTarget) []int {
	var out []int
	for i := range min(len(old), len(now)) {
		if old[i] != now[i] {
			out = append(out, i)
		}
	}
	return out
}
