package platform

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Accelerator is a key combination: modifiers plus one key, as a global
// shortcut's trigger ([Shortcut]) or a menu item's shown shortcut
// (String). Key is a logical key (a W3C "key" value, like KeyEvent.Key):
// a named key (KeyF1, KeyArrowUp, KeyEscape) or the character the key
// types without Shift ("k", "1", "/"), so {ModCtrl | ModShift, "k"} is
// Ctrl+Shift+K. Accelerator values are comparable.
type Accelerator struct {
	Mods Modifiers
	Key  Key
}

// String formats a as menus show it: the modifiers in the order Ctrl,
// Alt, Shift, Super, then the key, joined by '+', with letters upper-cased
// and " " as "Space" ("Ctrl+Shift+K", "Super+F1").
func (a Accelerator) String() string {
	var parts []string
	for _, m := range [...]struct {
		m Modifiers
		n string
	}{{ModCtrl, "Ctrl"}, {ModAlt, "Alt"}, {ModShift, "Shift"}, {ModSuper, "Super"}} {
		if a.Mods&m.m != 0 {
			parts = append(parts, m.n)
		}
	}
	k := string(a.Key)
	switch {
	case a.Key == " ":
		k = "Space"
	case !a.Key.IsNamed():
		k = strings.ToUpper(k)
	}
	return strings.Join(append(parts, k), "+")
}

// CheckAccelerator panics with an actionable message if a has unknown
// modifier bits or a key that cannot trigger a shortcut: empty, a
// modifier or lock key (Shift, Control, CapsLock…), Dead, Process,
// Unidentified, a control character, more than one character, or an
// upper-case letter (write {ModShift, "k"}). what names the value.
func CheckAccelerator(what string, a Accelerator) {
	if a.Mods&^(ModShift|ModCtrl|ModAlt|ModSuper) != 0 {
		panicf("platform: %s: invalid modifiers %d; use ModShift, ModCtrl, ModAlt and ModSuper", what, uint8(a.Mods))
	}
	k := a.Key
	switch {
	case k == "":
		panicf("platform: %s has no Key; give a key such as \"k\" or platform.KeyF1", what)
	case k.IsNamed():
		switch k {
		case KeyAlt, KeyAltGraph, KeyCapsLock, KeyControl, KeyFn, KeyFnLock, KeyMeta, KeyNumLock,
			KeyScrollLock, KeyShift, KeySymbol, KeySymbolLock, KeyHyper, KeySuper,
			KeyDead, KeyProcess, KeyUnidentified:
			panicf("platform: %s: %q cannot be a shortcut's key; modifiers go in Mods, and the key is the one pressed with them", what, k)
		}
	case utf8.RuneCountInString(string(k)) != 1:
		panicf("platform: %s: Key %q is neither a named key nor one character", what, k)
	default:
		r, _ := utf8.DecodeRuneInString(string(k))
		switch {
		case r == utf8.RuneError || unicode.IsControl(r):
			panicf("platform: %s: Key %q is a control character; use the named key (KeyEnter, KeyTab, KeyEscape)", what, k)
		case unicode.IsUpper(r) && unicode.ToLower(r) != r:
			panicf("platform: %s: Key %q is upper case; write the key's own character with ModShift: {ModShift, %q}", what, k, string(unicode.ToLower(r)))
		}
	}
}

// Shortcut is a global (system-wide) keyboard shortcut: it fires while
// the app has no focus. See "Global shortcuts" in the package
// documentation.
type Shortcut struct {
	// ID identifies the shortcut to the app (ShortcutEvent) and to the OS,
	// which keeps the user's changes to it across runs (the portal): keep
	// it stable. Non-empty, printable, at most 255 bytes.
	ID string
	// Accelerator is the preferred trigger. The OS may bind another one
	// (the portal lets the user choose); ShortcutStatusEvent reports the
	// trigger bound.
	Accelerator Accelerator
	// Description says what the shortcut does, in the user's language
	// ("Show the notes window"); required, the portal shows it.
	Description string
}

// CheckShortcut panics with an actionable message if s has an invalid ID,
// accelerator or description.
func CheckShortcut(s Shortcut) {
	CheckShortcutID("RegisterShortcut", s.ID)
	CheckAccelerator(fmt.Sprintf("Shortcut %q Accelerator", s.ID), s.Accelerator)
	if strings.TrimSpace(s.Description) == "" {
		panicf("platform: Shortcut %q has no Description; the system's shortcut settings show it to the user", s.ID)
	}
}

// CheckShortcutID panics with an actionable message if id is not a valid
// Shortcut.ID. what names the call.
func CheckShortcutID(what, id string) {
	if id == "" || len(id) > 255 || strings.IndexFunc(id, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		panicf("platform: %s: invalid shortcut ID %q; IDs are non-empty printable text of at most 255 bytes, stable across runs", what, id)
	}
}

// ShortcutAction is what happened to a global shortcut in a
// [ShortcutEvent].
type ShortcutAction uint8

const (
	// ShortcutPressed: the user pressed the shortcut's trigger.
	ShortcutPressed ShortcutAction = iota
	// ShortcutReleased: the user released it, where the OS reports
	// releases (the portal's Deactivated, X11 key releases; not Win32).
	ShortcutReleased
)

var shortcutActionNames = [...]string{"pressed", "released"}

func (a ShortcutAction) String() string {
	if int(a) < len(shortcutActionNames) {
		return shortcutActionNames[a]
	}
	return "ShortcutAction(invalid)"
}

// ShortcutEvent is delivered, with a nil Window, when the user presses or
// releases a bound global shortcut ([Desktop.RegisterShortcut]).
type ShortcutEvent struct {
	ID     string
	Action ShortcutAction
	// Time is when the event happened, in the time base of [Backend.Now].
	Time time.Duration
}

// ShortcutStatus is the binding state of a global shortcut, reported by
// [ShortcutStatusEvent].
type ShortcutStatus uint8

const (
	// ShortcutBound: the shortcut is registered; Trigger is what the user
	// presses. Reported again when the user changes the trigger.
	ShortcutBound ShortcutStatus = iota
	// ShortcutTaken: another application holds the combination (Win32
	// RegisterHotKey, X11 XGrabKey with BadAccess).
	ShortcutTaken
	// ShortcutRefused: the user or the compositor declined to bind it
	// (the portal's dialog).
	ShortcutRefused
	// ShortcutRevoked: a bound shortcut was removed by the OS or the user
	// (the portal's ShortcutsChanged without it).
	ShortcutRevoked
	// ShortcutFailed: the request failed (Reason says why).
	ShortcutFailed
)

var shortcutStatusNames = [...]string{"bound", "taken", "refused", "revoked", "failed"}

func (s ShortcutStatus) String() string {
	if int(s) < len(shortcutStatusNames) {
		return shortcutStatusNames[s]
	}
	return "ShortcutStatus(invalid)"
}

// ShortcutStatusEvent is delivered, with a nil Window, with the outcome of
// a RegisterShortcut that returned nil (exactly one, through the loop,
// even where the OS answers at once), and later whenever the binding
// changes (ShortcutBound with a new trigger, ShortcutRevoked).
type ShortcutStatusEvent struct {
	ID     string
	Status ShortcutStatus
	// Trigger describes the bound trigger as the OS shows it ("Ctrl+Alt+K",
	// the portal's trigger_description), for ShortcutBound; "" when the OS
	// does not say.
	Trigger string
	// Reason explains a status other than ShortcutBound, in English, for
	// logs ("" when the OS gives none).
	Reason string
}
