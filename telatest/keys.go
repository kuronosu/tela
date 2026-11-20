package telatest

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/keycombo"
)

// keyNames returns every name Press accepts, for suggestions.
func keyNames() []string { return keycombo.Names() }

// modifierNamed resolves a modifier name, including ControlOrMeta: Meta
// under the Mac flavor fl, Control under the others ([PlatformKeys]).
func modifierNamed(name string, fl platform.Flavor) (keycombo.Modifier, bool) {
	return keycombo.ModifierNamed(name, fl)
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
//	app.Press("Control+s")
//	app.Press("ControlOrMeta+s") // Meta+s under the Mac flavor, Control+s elsewhere
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
	evs, err := keycombo.Press(combo, a.cfg.flavor)
	if err != nil {
		var ke *keycombo.Error
		errors.As(err, &ke)
		switch ke.Kind {
		case keycombo.NoKey:
			a.fatalf("Press(%q): no key; give a key such as Press(\"Enter\") or Press(\"Control+s\")", combo)
		case keycombo.NotModifier:
			a.fatalf("Press(%q): %q is not a modifier; only the last part is a key, the others are \"Control\", \"Shift\", \"Alt\", \"Meta\" or \"ControlOrMeta\"", combo, ke.Name)
		default:
			a.fatalf("Press(%q): %s", combo, unknownKey(ke.Name))
		}
	}
	for _, e := range evs {
		a.sendKey(e)
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
	for _, e := range keycombo.Type(text) {
		a.sendKey(e)
	}
}

// sendKey injects a key event and settles the app, so the next event sees
// the rebuilt tree.
func (a *App) sendKey(e platform.KeyEvent) {
	a.tb.Helper()
	a.activate()
	if !a.focused() {
		a.fatalf("cannot send keys to the window %q: it is hidden, so it cannot have the keyboard focus; show it first", a.win.Title())
	}
	a.win.Inject(e)
	a.settleAfter()
}
