package core

import (
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// Phase 15 (ADR-224, ADR-226): what package desktop reaches through core
// for the app's menu bar and the standard menu roles: the open windows and
// the key window, a notice when the set of windows changes, and a key
// press delivered to a window as its backend would deliver it.

// Windows returns the handles of the open windows: the main window, then
// the windows of Window components in the order they opened.
func (d Desktop) Windows() []WindowHandle {
	out := make([]WindowHandle, 0, len(d.h.views))
	for _, v := range d.h.views {
		if v.alive() {
			out = append(out, WindowHandle{v.tree})
		}
	}
	return out
}

// KeyWindow returns the window the keyboard focus is in (macOS's key
// window): the focused window, else the main window.
func (d Desktop) KeyWindow() WindowHandle {
	for _, v := range d.h.views {
		if v.focused && v.alive() {
			return WindowHandle{v.tree}
		}
	}
	return WindowHandle{d.h.main.tree}
}

// windowWatch is a listener of WatchWindows.
type windowWatch struct {
	f       func()
	stopped bool
}

// WatchWindows calls f on the UI thread after a window of the app opened
// or closed, until stop is called (package desktop gives each new window
// the app's menu bar). A window opens or closes while the app runs, never
// during f.
func (d Desktop) WatchWindows(f func()) (stop func()) {
	if f == nil {
		panic("core: Desktop.WatchWindows(nil)")
	}
	dk := d.h.desktop()
	l := &windowWatch{f: f}
	dk.winWatchers = append(dk.winWatchers, l)
	return func() {
		if l.stopped {
			return
		}
		l.stopped = true
		dk.winWatchers = slices.DeleteFunc(dk.winWatchers, func(x *windowWatch) bool { return x == l })
	}
}

// windowsChanged tells the watchers of WatchWindows that a window opened
// or closed.
func (h *Host) windowsChanged() {
	if h.desk == nil || h.ended {
		return
	}
	for _, l := range slices.Clone(h.desk.winWatchers) {
		if !l.stopped {
			l.f()
		}
	}
}

// PrimaryModifier returns the modifier of shortcuts in w's window: Super
// (Command) under the Mac flavor, Ctrl under the others (ADR-221), the
// same one KeyboardEvent.CmdOrCtrl tests.
func (d Desktop) PrimaryModifier(w WindowHandle) platform.Modifiers {
	if w.t != nil && w.t.flavor == platform.FlavorMac {
		return platform.ModSuper
	}
	return platform.ModCtrl
}

// PressKey delivers a press and a release of the key to w's window as its
// backend would (a KeyEvent through the window's event path: the modal
// check, the focused element, the text field's key bindings), and
// reports whether the window is open. It is how package desktop carries
// out a menu role the OS did not (Copy acts on the focused field as the
// shortcut does). Call it on the UI thread, in an event's handling (a
// menu choice), not in a build.
func (d Desktop) PressKey(w WindowHandle, code platform.KeyCode, key platform.Key, mods platform.Modifiers) bool {
	if w.t == nil || w.t.wv == nil || w.t.wv.host != d.h || !w.t.wv.alive() {
		return false
	}
	v, h := w.t.wv, d.h
	for _, a := range [...]platform.KeyAction{platform.KeyPress, platform.KeyRelease} {
		h.handle(v.win, platform.KeyEvent{
			Action: a, Code: code, Key: key, Location: code.Location(), Mods: mods,
			Time: h.backend.Now(),
		})
	}
	return true
}
