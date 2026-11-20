// Package keycombo turns Playwright's key syntax ("Control+Shift+a",
// "Enter", "ArrowDown") into the key events a real keyboard with the US
// layout produces, and text into the presses that type it. telatest's
// Press and Type use it, and so does tela dev's input injection (ADR-299),
// so both speak one syntax.
//
// Names are W3C UI Events code names ("KeyA", "Digit1", "Enter", "F5"),
// characters the US layout types ("a", "A", "!", " ") and the modifiers
// "Shift", "Control", "Alt", "Meta" and "ControlOrMeta" (Meta under the
// Mac flavor, Control under the others). Events carry no Time: the caller
// stamps them with its clock.
package keycombo

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// Key is a key of the US layout: its physical code and what it produces
// without and with Shift.
type Key struct {
	Code      platform.KeyCode
	Key       platform.Key // the logical key
	ShiftKey  platform.Key
	Text      string // the text of a press; "" for keys that produce none
	ShiftText string
}

// Modifier is a modifier key, pressed as its left key.
type Modifier struct {
	Code platform.KeyCode
	Key  platform.Key
	Mod  platform.Modifiers
}

var (
	// byCode maps W3C code names ("KeyA", "Digit1", "Enter") to keys.
	byCode = map[string]*Key{}
	// byChar maps the character a key types to the key; shifted
	// reports the character is the Shift variant.
	byChar = map[string]charKey{}
	// modifiers maps Playwright's modifier names.
	modifiers map[string]Modifier
)

type charKey struct {
	k       *Key
	shifted bool
}

func init() {
	code := func(name string) platform.KeyCode {
		c, ok := platform.ParseKeyCode(name)
		if !ok {
			panic("keycombo: unknown key code " + name)
		}
		return c
	}
	add := func(codeName string, k Key) {
		k.Code = code(codeName)
		byCode[codeName] = &k
		if k.Text != "" {
			byChar[k.Text] = charKey{&k, false}
			if k.ShiftText != k.Text {
				byChar[k.ShiftText] = charKey{&k, true}
			}
		}
	}
	for c := 'a'; c <= 'z'; c++ {
		lo, up := string(c), strings.ToUpper(string(c))
		add("Key"+up, Key{Key: platform.Key(lo), ShiftKey: platform.Key(up), Text: lo, ShiftText: up})
	}
	for _, p := range [...]struct{ code, lo, up string }{
		{"Digit1", "1", "!"}, {"Digit2", "2", "@"}, {"Digit3", "3", "#"}, {"Digit4", "4", "$"},
		{"Digit5", "5", "%"}, {"Digit6", "6", "^"}, {"Digit7", "7", "&"}, {"Digit8", "8", "*"},
		{"Digit9", "9", "("}, {"Digit0", "0", ")"},
		{"Backquote", "`", "~"}, {"Minus", "-", "_"}, {"Equal", "=", "+"},
		{"BracketLeft", "[", "{"}, {"BracketRight", "]", "}"}, {"Backslash", "\\", "|"},
		{"Semicolon", ";", ":"}, {"Quote", "'", "\""}, {"Comma", ",", "<"}, {"Period", ".", ">"},
		{"Slash", "/", "?"}, {"Space", " ", " "},
	} {
		add(p.code, Key{Key: platform.Key(p.lo), ShiftKey: platform.Key(p.up), Text: p.lo, ShiftText: p.up})
	}
	named := []string{"Enter", "Tab", "Backspace", "Delete", "Escape", "Insert", "Home", "End", "PageUp", "PageDown",
		"ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "CapsLock", "ContextMenu"}
	for i := 1; i <= 12; i++ {
		named = append(named, fmt.Sprintf("F%d", i))
	}
	for _, n := range named {
		add(n, Key{Key: platform.Key(n), ShiftKey: platform.Key(n)})
	}
	modifiers = map[string]Modifier{
		"Shift":   {code("ShiftLeft"), platform.KeyShift, platform.ModShift},
		"Control": {code("ControlLeft"), platform.KeyControl, platform.ModCtrl},
		"Alt":     {code("AltLeft"), platform.KeyAlt, platform.ModAlt},
		"Meta":    {code("MetaLeft"), platform.KeyMeta, platform.ModSuper},
	}
}

// Names returns every name Press accepts but the single characters,
// sorted, for suggestions.
func Names() []string {
	var names []string
	for n := range byCode {
		names = append(names, n)
	}
	for n := range modifiers {
		names = append(names, n)
	}
	names = append(names, "ControlOrMeta")
	sort.Strings(names)
	return names
}

// Split splits "Control+Shift+a" into the modifier names and the key. A
// "+" at the end, or right after another "+", is the key itself
// ("Control++").
func Split(s string) (mods []string, key string) {
	start := 0
	for i := 1; i < len(s)-1; i++ {
		if s[i] == '+' && s[i-1] != '+' {
			mods = append(mods, s[start:i])
			start = i + 1
		}
	}
	return mods, s[start:]
}

// ModifierNamed resolves a modifier name, including ControlOrMeta: Meta
// under the Mac flavor fl, Control under the others.
func ModifierNamed(name string, fl platform.Flavor) (Modifier, bool) {
	if name == "ControlOrMeta" {
		if fl.Resolve() == platform.FlavorMac {
			name = "Meta"
		} else {
			name = "Control"
		}
	}
	m, ok := modifiers[name]
	return m, ok
}

// Lookup resolves the key of a combination: a W3C code name ("KeyA",
// "Enter", "ArrowLeft") or a character the US layout types ("a", "A", "!",
// " "). shifted reports a character that needs Shift.
func Lookup(name string) (k *Key, shifted bool, ok bool) {
	if d, found := byCode[name]; found {
		return d, false, true
	}
	if utf8.RuneCountInString(name) == 1 {
		if c, found := byChar[name]; found {
			return c.k, c.shifted, true
		}
	}
	return nil, false, false
}

// ErrorKind is what is wrong with a combination.
type ErrorKind uint8

const (
	// NoKey: the combination is empty or ends with modifiers only.
	NoKey ErrorKind = iota + 1
	// NotModifier: a part before the key is not a modifier.
	NotModifier
	// UnknownKey: the key is not a name or a character of the table.
	UnknownKey
)

// Error is a combination Press cannot press. Name is the part at fault.
type Error struct {
	Kind ErrorKind
	Name string
}

func (e *Error) Error() string {
	switch e.Kind {
	case NoKey:
		return `no key; give a key such as "Enter" or "Control+s"`
	case NotModifier:
		return fmt.Sprintf(`%q is not a modifier; only the last part is a key, the others are "Control", "Shift", "Alt", "Meta" or "ControlOrMeta"`, e.Name)
	}
	return fmt.Sprintf(`unknown key %q; use a key name such as "Enter", "Tab", "ArrowDown", "F5" or "KeyA", or a character such as "a"`, e.Name)
}

// Press returns the events that press and release combo: modifiers
// pressed first (as left keys), then the key's press and release with its
// physical Code, logical Key, the Text it produces (none for Enter,
// arrows...), its Location and the modifier state, then the modifiers
// released last. A combination that is a modifier alone presses and
// releases it. The error is an *Error.
func Press(combo string, fl platform.Flavor) ([]platform.KeyEvent, error) {
	down, up, err := split(combo, fl)
	if err != nil {
		return nil, err
	}
	return append(down, up...), nil
}

// Down returns the presses of combo: its modifiers, then its key (Press
// without the releases). Up returns the releases Press ends with.
func Down(combo string, fl platform.Flavor) ([]platform.KeyEvent, error) {
	down, _, err := split(combo, fl)
	return down, err
}

// Up returns the releases of combo: its key, then its modifiers in
// reverse order, as Press ends.
func Up(combo string, fl platform.Flavor) ([]platform.KeyEvent, error) {
	_, up, err := split(combo, fl)
	return up, err
}

// split returns the presses and the releases of combo.
func split(combo string, fl platform.Flavor) (down, up []platform.KeyEvent, err error) {
	modNames, keyName := Split(combo)
	if keyName == "" {
		return nil, nil, &Error{Kind: NoKey}
	}
	var mods []Modifier
	var held platform.Modifiers
	for _, n := range modNames {
		m, ok := ModifierNamed(n, fl)
		if !ok {
			return nil, nil, &Error{Kind: NotModifier, Name: n}
		}
		mods = append(mods, m)
	}
	for _, m := range mods {
		held |= m.Mod
		down = append(down, platform.KeyEvent{Action: platform.KeyPress, Code: m.Code, Key: m.Key, Location: m.Code.Location(), Mods: held})
	}
	if m, ok := ModifierNamed(keyName, fl); ok {
		// "Shift": the modifier is the key.
		held |= m.Mod
		down = append(down, platform.KeyEvent{Action: platform.KeyPress, Code: m.Code, Key: m.Key, Location: m.Code.Location(), Mods: held})
		held &^= m.Mod
		up = append(up, platform.KeyEvent{Action: platform.KeyRelease, Code: m.Code, Key: m.Key, Location: m.Code.Location(), Mods: held})
	} else {
		k, shifted, ok := Lookup(keyName)
		if !ok {
			return nil, nil, &Error{Kind: UnknownKey, Name: keyName}
		}
		shifted = shifted || held&platform.ModShift != 0
		key, text := k.Key, k.Text
		if shifted {
			key, text = k.ShiftKey, k.ShiftText
		}
		down = append(down, platform.KeyEvent{Action: platform.KeyPress, Code: k.Code, Key: key, Text: text, Location: k.Code.Location(), Mods: held})
		up = append(up, platform.KeyEvent{Action: platform.KeyRelease, Code: k.Code, Key: key, Location: k.Code.Location(), Mods: held})
	}
	for i := len(mods) - 1; i >= 0; i-- {
		m := mods[i]
		held &^= m.Mod
		up = append(up, platform.KeyEvent{Action: platform.KeyRelease, Code: m.Code, Key: m.Key, Location: m.Code.Location(), Mods: held})
	}
	return down, up, nil
}

// Type returns the presses and releases that type text one character at a
// time: each has the character's physical Code (US layout, when it has
// one; CodeUnidentified otherwise), Key and Text, and no modifiers (an
// upper case letter is not typed with Shift, as in Playwright's
// keyboard.type). A newline types Enter and a tab Tab.
func Type(text string) []platform.KeyEvent {
	var out []platform.KeyEvent
	for _, r := range text {
		s := string(r)
		switch r {
		case '\n', '\r':
			out = append(out, named("Enter")...)
			continue
		case '\t':
			out = append(out, named("Tab")...)
			continue
		}
		ev := platform.KeyEvent{Code: platform.CodeUnidentified, Key: platform.Key(s), Text: s}
		if c, ok := byChar[s]; ok {
			ev.Code = c.k.Code
			ev.Location = c.k.Code.Location()
		}
		ev.Action = platform.KeyPress
		out = append(out, ev)
		ev.Action, ev.Text = platform.KeyRelease, ""
		out = append(out, ev)
	}
	return out
}

// named returns the press and release of a named key of the table.
func named(name string) []platform.KeyEvent {
	k := byCode[name]
	return []platform.KeyEvent{
		{Action: platform.KeyPress, Code: k.Code, Key: k.Key, Location: k.Code.Location()},
		{Action: platform.KeyRelease, Code: k.Code, Key: k.Key, Location: k.Code.Location()},
	}
}
