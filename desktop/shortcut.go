package desktop

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// GlobalShortcut is a component that registers a system-wide keyboard
// shortcut while it is mounted: OnPress runs, on the UI thread, when the
// user presses Keys anywhere, even while another application has the
// focus. It follows its fields (registering again only when ID, Keys or
// Description change) and unregisters when it unmounts. It renders
// nothing where it is declared; several, with different IDs, may be
// mounted at once.
//
//	desktop.GlobalShortcut{
//	    ID:          "new-note",
//	    Keys:        "Ctrl+Alt+N",
//	    Description: "Create a note",
//	    OnPress:     func() { win.Show(); newNote() },
//	    OnStatus: func(e desktop.ShortcutStatusEvent) {
//	        if e.Status != desktop.ShortcutBound {
//	            setHint.Set(e.Err.Error()) // "Ctrl+Alt+N is taken by another application"
//	        }
//	    },
//	}
//
// The OS decides: Windows refuses a combination another application
// holds (ShortcutTaken), and on Wayland the desktop asks the user first
// (through the GlobalShortcuts portal) and may bind other keys, which
// ShortcutStatusEvent.Trigger reports. Where the desktop has no global
// shortcuts (Wayland without the portal), OnStatus receives ShortcutFailed
// with an error wrapping [ErrUnsupported]. Tests press it with telatest's
// App.PressShortcut.
type GlobalShortcut struct {
	// ID identifies the shortcut to the OS, which keeps the user's
	// changes to it across runs (the portal): keep it stable
	// ("new-note"). Required; unique among the mounted shortcuts.
	ID string
	// Keys is the preferred key combination, such as "Ctrl+Alt+N" or
	// "Super+F5" (see [ParseKeys] for the names). Required; a combination
	// that cannot be parsed panics in dev mode and is reported to
	// OnStatus as ShortcutFailed otherwise (keys a user typed: check them
	// with ParseKeys first).
	Keys string
	// Description says what the shortcut does, in the user's language
	// ("Create a note"); the desktop's shortcut settings show it.
	// Required.
	Description string
	// OnPress runs when the user presses the shortcut.
	OnPress func()
	// OnRelease runs when the user releases it, where the OS reports
	// releases (Linux; not Windows).
	OnRelease func()
	// OnStatus receives the outcome of the registration and its later
	// changes (the user bound other keys, or removed the shortcut). Nil
	// reports a status other than ShortcutBound as a dev warning.
	OnStatus func(ShortcutStatusEvent)
}

// ShortcutStatus is the state of a [GlobalShortcut]'s registration.
type ShortcutStatus uint8

const (
	// ShortcutBound: the shortcut works; Trigger is what the user presses.
	ShortcutBound ShortcutStatus = iota
	// ShortcutTaken: another application holds the combination.
	ShortcutTaken
	// ShortcutRefused: the user or the desktop declined to bind it.
	ShortcutRefused
	// ShortcutRevoked: the user or the desktop removed it after it was
	// bound.
	ShortcutRevoked
	// ShortcutFailed: it could not be registered (no global shortcuts on
	// this desktop: ErrUnsupported; invalid Keys; an OS error).
	ShortcutFailed
)

var shortcutStatusNames = [...]string{"bound", "taken", "refused", "revoked", "failed"}

func (s ShortcutStatus) String() string {
	if int(s) < len(shortcutStatusNames) {
		return shortcutStatusNames[s]
	}
	return fmt.Sprintf("ShortcutStatus(%d)", uint8(s))
}

// ShortcutStatusEvent is what [GlobalShortcut]'s OnStatus receives.
type ShortcutStatusEvent struct {
	// ID is the shortcut's.
	ID string
	// Status is the registration's state.
	Status ShortcutStatus
	// Trigger is the combination the user presses, as the desktop shows
	// it ("Ctrl+Alt+N"), for ShortcutBound: the desktop may bind other
	// keys than Keys. "" when it does not say.
	Trigger string
	// Err says why, for every status but ShortcutBound, in a sentence an
	// app can show ("desktop: the global shortcut Ctrl+Alt+N is taken by
	// another application").
	Err error
}

// ParseKeys checks a key combination as [GlobalShortcut].Keys takes it
// and returns it written the usual way, as menus show it (the text for
// [MenuItem].Accelerator): modifiers then the key, joined by '+'.
//
//	desktop.ParseKeys("ctrl+alt+n")   // "Ctrl+Alt+N", nil
//	desktop.ParseKeys("Ctrl+Hyper+N") // error: "Hyper" is not a modifier
//
// Names ignore case. Modifiers: Ctrl (Control), Alt (Option), Shift,
// Super (Meta, Win, Cmd), and CmdOrCtrl (Super on macOS, Ctrl elsewhere).
// The key comes last: one character ("N", "1", "/"; Shift is written as a
// modifier), a key name ("F1"…"F24", "Escape", "Enter", "Tab", "Space",
// "Backspace", "Delete", "Insert", "Home", "End", "PageUp", "PageDown",
// "ArrowUp" or "Up", "PrintScreen", "MediaPlayPause"…), or "Plus" for '+'.
func ParseKeys(keys string) (string, error) {
	a, err := platform.ParseAccelerator(keys)
	if err != nil {
		return "", fmt.Errorf("desktop: %w", err)
	}
	return a.String(), nil
}

// Build registers the shortcut while the component is mounted (see
// [GlobalShortcut]).
func (g GlobalShortcut) Build() tela.Node {
	if g.ID == "" {
		panic("desktop.GlobalShortcut has no ID: give it a stable name, such as desktop.GlobalShortcut{ID: \"new-note\", Keys: \"Ctrl+Alt+N\", Description: \"Create a note\", OnPress: newNote}")
	}
	if strings.TrimSpace(g.Description) == "" {
		panic(fmt.Sprintf("desktop.GlobalShortcut %q has no Description: say what it does (\"Create a note\"); the desktop's shortcut settings show it", g.ID))
	}
	accel, perr := platform.ParseAccelerator(g.Keys)
	if perr != nil && core.DevNow() {
		panic(fmt.Sprintf("desktop.GlobalShortcut %q: Keys: %v; write it such as \"Ctrl+Alt+N\" (desktop.ParseKeys checks a combination)", g.ID, perr))
	}
	want := platform.Shortcut{ID: g.ID, Accelerator: accel, Description: g.Description}
	if strings.IndexFunc(g.ID, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 || len(g.ID) > 255 {
		panic(fmt.Sprintf("desktop.GlobalShortcut: invalid ID %q; IDs are printable text of at most 255 bytes, stable across runs", g.ID))
	}
	w := tela.UseWindow()
	s := tela.UseOnce(func() *shortcutState { return &shortcutState{} })
	s.props = g
	tela.UseMount(func() func() {
		d, ok := core.DesktopOf(w)
		if !ok {
			return nil // tela snapshot, or a tree without windows
		}
		s.d = d
		s.owners = d.Local(shortcutsKey{}, func() any { return map[string]*shortcutState{} }).(map[string]*shortcutState)
		s.stop = d.Listen(s.event)
		return s.remove
	})
	tela.UseEffectDo(func() {
		var err error
		if perr != nil {
			err = fmt.Errorf("desktop: the global shortcut %q: %w", g.ID, perr)
		}
		s.sync(want, err)
	})
	return nil
}

type shortcutsKey struct{}

// shortcutState is a mounted GlobalShortcut's link to the backend.
type shortcutState struct {
	d     core.Desktop
	stop  func()
	props GlobalShortcut
	// owners are the app's registered shortcuts by ID: only the owner of
	// an ID gets its events and unregisters it.
	owners map[string]*shortcutState
	// reg is the shortcut asked for (its ID is "" before the first
	// request); failed: the request failed, not retried until it changes.
	reg    platform.Shortcut
	failed bool
}

// sync registers want unless it is what was asked for already. parseErr
// is set when Keys did not parse (outside dev mode).
func (s *shortcutState) sync(want platform.Shortcut, parseErr error) {
	if s.d.IsZero() || s.d.Ended() {
		return
	}
	if s.reg == want && (s.failed || s.owners[want.ID] == s) {
		return
	}
	if s.reg.ID != "" && s.reg.ID != want.ID {
		s.unregister()
	}
	s.reg, s.failed = want, false
	if parseErr != nil {
		s.failed = true
		s.report(ShortcutStatusEvent{ID: want.ID, Status: ShortcutFailed, Err: parseErr})
		return
	}
	if o := s.owners[want.ID]; o != nil && o != s {
		s.d.Warn("desktop-shortcut-twice:"+want.ID, func() string {
			return fmt.Sprintf("desktop: two desktop.GlobalShortcut components with the ID %q are mounted; the later one replaces the earlier one (give each shortcut its own ID)", want.ID)
		})
	}
	if err := s.d.Services().RegisterShortcut(want); err != nil {
		s.failed = true
		if s.owners[want.ID] == s {
			delete(s.owners, want.ID)
		}
		s.report(ShortcutStatusEvent{ID: want.ID, Status: ShortcutFailed,
			Err: wrap(fmt.Sprintf("GlobalShortcut %q (%v)", want.ID, want.Accelerator), err)})
		return
	}
	s.owners[want.ID] = s
}

// unregister removes the shortcut s registered, if s still owns its ID.
func (s *shortcutState) unregister() {
	id := s.reg.ID
	if id == "" || s.owners[id] != s {
		return
	}
	delete(s.owners, id)
	if !s.d.Ended() {
		_ = s.d.Services().UnregisterShortcut(id)
	}
}

func (s *shortcutState) remove() {
	if s.stop != nil {
		s.stop()
	}
	s.unregister()
	s.reg = platform.Shortcut{}
}

// event runs the callbacks of the shortcut's presses and statuses.
func (s *shortcutState) event(e platform.Event) {
	switch e := e.(type) {
	case platform.ShortcutEvent:
		if e.ID != s.reg.ID || s.owners[e.ID] != s {
			return
		}
		f := s.props.OnPress
		if e.Action == platform.ShortcutReleased {
			f = s.props.OnRelease
		}
		if f != nil {
			f()
		}
	case platform.ShortcutStatusEvent:
		if e.ID != s.reg.ID || s.owners[e.ID] != s {
			return
		}
		s.report(statusEvent(e, s.reg.Accelerator))
	}
}

// statusEvent converts a platform status of the shortcut whose preferred
// keys are accel.
func statusEvent(e platform.ShortcutStatusEvent, accel platform.Accelerator) ShortcutStatusEvent {
	out := ShortcutStatusEvent{ID: e.ID, Status: ShortcutStatus(e.Status), Trigger: e.Trigger}
	keys := accel.String()
	reason := ""
	if e.Reason != "" {
		reason = " (" + e.Reason + ")"
	}
	switch e.Status {
	case platform.ShortcutBound:
	case platform.ShortcutTaken:
		out.Err = fmt.Errorf("desktop: the global shortcut %s is taken by another application%s", keys, reason)
	case platform.ShortcutRefused:
		out.Err = fmt.Errorf("desktop: the global shortcut %s was declined%s", keys, reason)
	case platform.ShortcutRevoked:
		out.Err = fmt.Errorf("desktop: the global shortcut %s was removed%s", keys, reason)
	default:
		out.Status = ShortcutFailed
		out.Err = fmt.Errorf("desktop: the global shortcut %s could not be registered%s", keys, reason)
	}
	return out
}

func (s *shortcutState) report(e ShortcutStatusEvent) {
	if f := s.props.OnStatus; f != nil {
		f(e)
		return
	}
	if e.Status == ShortcutBound {
		return
	}
	err := e.Err
	if err == nil {
		err = errors.New(e.Status.String())
	}
	s.d.Warn("desktop-shortcut:"+e.ID+":"+err.Error(), func() string {
		return fmt.Sprintf("%v; set desktop.GlobalShortcut's OnStatus to tell the user", err)
	})
}
