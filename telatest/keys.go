package telatest

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// keyDef is a key of the US layout, which tests type on: its physical
// code and what it produces without and with Shift.
type keyDef struct {
	code      platform.KeyCode
	key       platform.Key // the logical key
	shiftKey  platform.Key
	text      string // the text of a press; "" for keys that produce none
	shiftText string
}

// modDef is a modifier key.
type modDef struct {
	code platform.KeyCode
	key  platform.Key
	mod  platform.Modifiers
}

var (
	// keyByCode maps W3C code names ("KeyA", "Digit1", "Enter") to keys.
	keyByCode = map[string]*keyDef{}
	// keyByChar maps the character a key types to the key; shifted
	// reports the character is the Shift variant.
	keyByChar = map[string]charKey{}
	// modifierKeys maps Playwright's modifier names.
	modifierKeys map[string]modDef
)

type charKey struct {
	def     *keyDef
	shifted bool
}

func init() {
	code := func(name string) platform.KeyCode {
		c, ok := platform.ParseKeyCode(name)
		if !ok {
			panic("telatest: unknown key code " + name)
		}
		return c
	}
	add := func(codeName string, d keyDef) {
		d.code = code(codeName)
		keyByCode[codeName] = &d
		if d.text != "" {
			keyByChar[d.text] = charKey{&d, false}
			if d.shiftText != d.text {
				keyByChar[d.shiftText] = charKey{&d, true}
			}
		}
	}
	for c := 'a'; c <= 'z'; c++ {
		lo, up := string(c), strings.ToUpper(string(c))
		add("Key"+up, keyDef{key: platform.Key(lo), shiftKey: platform.Key(up), text: lo, shiftText: up})
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
		add(p.code, keyDef{key: platform.Key(p.lo), shiftKey: platform.Key(p.up), text: p.lo, shiftText: p.up})
	}
	named := []string{"Enter", "Tab", "Backspace", "Delete", "Escape", "Insert", "Home", "End", "PageUp", "PageDown",
		"ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "CapsLock", "ContextMenu"}
	for i := 1; i <= 12; i++ {
		named = append(named, fmt.Sprintf("F%d", i))
	}
	for _, n := range named {
		add(n, keyDef{key: platform.Key(n), shiftKey: platform.Key(n)})
	}
	modifierKeys = map[string]modDef{
		"Shift":   {code("ShiftLeft"), platform.KeyShift, platform.ModShift},
		"Control": {code("ControlLeft"), platform.KeyControl, platform.ModCtrl},
		"Alt":     {code("AltLeft"), platform.KeyAlt, platform.ModAlt},
		"Meta":    {code("MetaLeft"), platform.KeyMeta, platform.ModSuper},
	}
}

// keyNames returns every name Press accepts, for suggestions.
func keyNames() []string {
	var names []string
	for n := range keyByCode {
		names = append(names, n)
	}
	for n := range modifierKeys {
		names = append(names, n)
	}
	names = append(names, "ControlOrMeta")
	sort.Strings(names)
	return names
}

// splitCombo splits Playwright's key syntax "Control+Shift+a" into the
// modifier names and the key. A "+" at the end, or right after another
// "+", is the key itself ("Control++").
func splitCombo(s string) (mods []string, key string) {
	start := 0
	for i := 1; i < len(s)-1; i++ {
		if s[i] == '+' && s[i-1] != '+' {
			mods = append(mods, s[start:i])
			start = i + 1
		}
	}
	return mods, s[start:]
}

// modifierNamed resolves a modifier name, including ControlOrMeta.
func modifierNamed(name string) (modDef, bool) {
	if name == "ControlOrMeta" {
		if runtime.GOOS == "darwin" {
			name = "Meta"
		} else {
			name = "Control"
		}
	}
	m, ok := modifierKeys[name]
	return m, ok
}

// lookupKey resolves the key of a Press combination: a W3C code name
// ("KeyA", "Enter", "ArrowLeft"), a character the US layout types ("a", "A",
// "!", " ") or a modifier name. shifted reports a character that needs
// Shift.
func lookupKey(name string) (def *keyDef, shifted bool, ok bool) {
	if d, found := keyByCode[name]; found {
		return d, false, true
	}
	if utf8.RuneCountInString(name) == 1 {
		if c, found := keyByChar[name]; found {
			return c.def, c.shifted, true
		}
	}
	return nil, false, false
}

// unknownKey builds the message for a key name Press cannot resolve.
func unknownKey(name string) string {
	msg := fmt.Sprintf("unknown key %q", name)
	switch name {
	case "Ctrl", "Cmd", "Command", "Win", "Super", "Option":
		return msg + `; the modifiers are "Control", "Shift", "Alt", "Meta" and "ControlOrMeta"`
	case "Esc":
		return msg + `; did you mean "Escape"?`
	case "Return":
		return msg + `; did you mean "Enter"?`
	case "Up", "Down", "Left", "Right":
		return msg + fmt.Sprintf("; did you mean \"Arrow%s\"?", name)
	}
	if s := closest(name, keyNames()); len(s) > 0 {
		return msg + fmt.Sprintf("; did you mean %s?", quoteAll(s))
	}
	return msg + `; use a key name such as "Enter", "Tab", "ArrowDown", "F5" or "KeyA", or a character such as "a"`
}

// Press presses and releases a key, sending the events to the focused
// element (or, when nothing is focused, to the outermost element of the
// content). The syntax is Playwright's: a key name or character, with
// modifiers joined by "+":
//
//	app.Press("Enter")
//	app.Press("Shift+Tab")
//	app.Press("Control+s")      // ControlOrMeta+s on the platform's shortcut key
//	app.Press("ArrowDown")
//	app.Press("a")              // the character "a"
//
// Each key is a press and a release with the physical Code, the logical
// Key, the Text it produces (none for Enter, arrows...), its Location and
// the modifier state, in the US layout; modifiers are pressed first, as
// left keys, and released last.
func (a *App) Press(combo string) {
	a.tb.Helper()
	a.mustBeOpen()
	modNames, keyName := splitCombo(combo)
	if keyName == "" {
		a.fatalf("Press(%q): no key; give a key such as Press(\"Enter\") or Press(\"Control+s\")", combo)
	}
	var mods []modDef
	var held platform.Modifiers
	for _, n := range modNames {
		m, ok := modifierNamed(n)
		if !ok {
			a.fatalf("Press(%q): %q is not a modifier; only the last part is a key, the others are \"Control\", \"Shift\", \"Alt\", \"Meta\" or \"ControlOrMeta\"", combo, n)
		}
		mods = append(mods, m)
	}
	for _, m := range mods {
		held |= m.mod
		a.sendKey(platform.KeyEvent{Action: platform.KeyPress, Code: m.code, Key: m.key, Location: m.code.Location(), Mods: held})
	}
	if m, ok := modifierNamed(keyName); ok {
		// Press("Shift"): the modifier is the key.
		held |= m.mod
		a.sendKey(platform.KeyEvent{Action: platform.KeyPress, Code: m.code, Key: m.key, Location: m.code.Location(), Mods: held})
		a.sendKey(platform.KeyEvent{Action: platform.KeyRelease, Code: m.code, Key: m.key, Location: m.code.Location(), Mods: held &^ m.mod})
		held &^= m.mod
	} else {
		def, shifted, ok := lookupKey(keyName)
		if !ok {
			a.fatalf("Press(%q): %s", combo, unknownKey(keyName))
		}
		shifted = shifted || held&platform.ModShift != 0
		key, text := def.key, def.text
		if shifted {
			key, text = def.shiftKey, def.shiftText
		}
		a.sendKey(platform.KeyEvent{Action: platform.KeyPress, Code: def.code, Key: key, Text: text, Location: def.code.Location(), Mods: held})
		a.sendKey(platform.KeyEvent{Action: platform.KeyRelease, Code: def.code, Key: key, Location: def.code.Location(), Mods: held})
	}
	for i := len(mods) - 1; i >= 0; i-- {
		m := mods[i]
		held &^= m.mod
		a.sendKey(platform.KeyEvent{Action: platform.KeyRelease, Code: m.code, Key: m.key, Location: m.code.Location(), Mods: held})
	}
}

// Type types text one character at a time, as key presses and releases
// sent to the focused element: each has the character's physical Code
// (US layout, when it has one), Key and Text, and no modifiers (an upper
// case letter is not typed with Shift, as in Playwright's keyboard.type).
// A newline types Enter and a tab Tab. In a focused text field each key
// inserts its text through the real key path (OnKeyDown, then
// OnBeforeInput and OnChange), as a user typing; [App.Fill] replaces the
// value at once.
func (a *App) Type(text string) {
	a.tb.Helper()
	a.mustBeOpen()
	for _, r := range text {
		s := string(r)
		switch r {
		case '\n', '\r':
			a.pressKeyDef("Enter")
			continue
		case '\t':
			a.pressKeyDef("Tab")
			continue
		}
		ev := platform.KeyEvent{Code: platform.CodeUnidentified, Key: platform.Key(s), Text: s}
		if c, ok := keyByChar[s]; ok {
			ev.Code = c.def.code
			ev.Location = c.def.code.Location()
		}
		ev.Action = platform.KeyPress
		a.sendKey(ev)
		ev.Action, ev.Text = platform.KeyRelease, ""
		a.sendKey(ev)
	}
}

// pressKeyDef presses and releases a named key of the table.
func (a *App) pressKeyDef(name string) {
	a.tb.Helper()
	def := keyByCode[name]
	a.sendKey(platform.KeyEvent{Action: platform.KeyPress, Code: def.code, Key: def.key, Location: def.code.Location()})
	a.sendKey(platform.KeyEvent{Action: platform.KeyRelease, Code: def.code, Key: def.key, Location: def.code.Location()})
}

// sendKey injects a key event and settles the app, so the next event sees
// the rebuilt tree.
func (a *App) sendKey(e platform.KeyEvent) {
	a.tb.Helper()
	a.win.Inject(e)
	a.settleAfter()
}
