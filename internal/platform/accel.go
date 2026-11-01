package platform

import (
	"fmt"
	"runtime"
	"strings"
	"unicode/utf8"
)

// ParseAccelerator parses a key combination as people write it and menus
// show it ([Accelerator.String]): modifiers and one key joined by '+',
// such as "Ctrl+Alt+N", "Shift+F5", "Super+Space" or "Ctrl++". Names are
// matched without regard to case and spaces around them are ignored.
//
//   - Modifiers: Ctrl (Control), Alt (Option), Shift, Super (Meta, Win,
//     Cmd, Command), and CmdOrCtrl (CommandOrControl, Electron's name):
//     Super on macOS, Ctrl elsewhere.
//   - The key, last: one character (a letter names the key and is
//     lower-cased, so "N" is "n"; Shift is written as a modifier), a named
//     [Key] ("F1", "Escape", "ArrowUp", "PageDown", "MediaPlayPause"), or
//     one of the short names Esc, Space, Return, Up, Down, Left, Right,
//     Del, Ins, PgUp, PgDn, Plus and Minus.
//
// The result passes CheckAccelerator; any other input is an error that
// says what is wrong.
func ParseAccelerator(s string) (Accelerator, error) {
	return parseAccelerator(s, runtime.GOOS == "darwin")
}

// parseAccelerator is ParseAccelerator with CmdOrCtrl's meaning chosen by
// mac, so the tests cover both on every OS.
func parseAccelerator(s string, mac bool) (Accelerator, error) {
	fail := func(format string, args ...any) (Accelerator, error) {
		return Accelerator{}, fmt.Errorf("key combination %q: "+format, append([]any{s}, args...)...)
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return fail("empty; write it such as \"Ctrl+Alt+N\"")
	}
	var parts []string
	switch {
	case t == "+":
		parts = []string{"+"}
	case strings.HasSuffix(t, "++"):
		// The key is '+' itself: "Ctrl++".
		parts = append(strings.Split(strings.TrimSuffix(t, "++"), "+"), "+")
	default:
		parts = strings.Split(t, "+")
	}
	mods := modifierNames(mac)
	var a Accelerator
	for i, p := range parts {
		if p != "+" {
			p = strings.TrimSpace(p)
		}
		if p == "" {
			return fail("an empty part between '+' signs (write the key '+' as \"Plus\")")
		}
		m, isMod := mods[strings.ToLower(p)]
		if i < len(parts)-1 {
			if !isMod {
				return fail("%q is not a modifier (Ctrl, Alt, Shift, Super, CmdOrCtrl); the key goes last", p)
			}
			if a.Mods&m != 0 {
				return fail("the modifier %q appears twice", p)
			}
			a.Mods |= m
			continue
		}
		if isMod {
			return fail("it ends with the modifier %q; add the key pressed with it, such as %q", p, p+"+K")
		}
		k, ok := keyByName(p)
		if !ok {
			return fail("%q is neither one character nor a key name such as F1, Escape, Space or ArrowUp", p)
		}
		a.Key = k
	}
	if err := acceleratorError(fmt.Sprintf("key combination %q", s), a); err != nil {
		return Accelerator{}, err
	}
	return a, nil
}

// acceleratorError returns CheckAccelerator's message for a as an error,
// or nil.
func acceleratorError(what string, a Accelerator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	CheckAccelerator(what, a)
	return nil
}

// modifierNames maps the lower-cased modifier names ParseAccelerator
// accepts to their bits.
func modifierNames(mac bool) map[string]Modifiers {
	cmdOrCtrl := ModCtrl
	if mac {
		cmdOrCtrl = ModSuper
	}
	return map[string]Modifiers{
		"ctrl": ModCtrl, "control": ModCtrl,
		"alt": ModAlt, "option": ModAlt,
		"shift": ModShift,
		"super": ModSuper, "meta": ModSuper, "win": ModSuper, "cmd": ModSuper, "command": ModSuper,
		"cmdorctrl": cmdOrCtrl, "commandorcontrol": cmdOrCtrl,
	}
}

// keyShortNames are the short key names ParseAccelerator accepts besides
// the named keys, lower-cased.
var keyShortNames = map[string]Key{
	"esc": KeyEscape, "space": " ", "spacebar": " ", "return": KeyEnter,
	"up": KeyArrowUp, "down": KeyArrowDown, "left": KeyArrowLeft, "right": KeyArrowRight,
	"del": KeyDelete, "ins": KeyInsert, "pgup": KeyPageUp, "pgdn": KeyPageDown,
	"plus": "+", "minus": "-",
}

// keyByName returns the Key that name stands for in ParseAccelerator.
func keyByName(name string) (Key, bool) {
	if utf8.RuneCountInString(name) == 1 {
		return Key(strings.ToLower(name)), true
	}
	low := strings.ToLower(name)
	if k, ok := keyShortNames[low]; ok {
		return k, true
	}
	for _, k := range allNamedKeys {
		if strings.ToLower(string(k)) == low {
			return k, true
		}
	}
	return "", false
}
