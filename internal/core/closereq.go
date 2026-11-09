package core

import "slices"

// Close and quit requests inside the component tree (ADR-253): the hooks
// UseCloseRequest and UseQuitRequest let components that hold the state
// a close depends on (unsaved changes, a "keep running in the tray"
// setting) answer the request, as Tauri's onCloseRequested and Electron's
// close and before-quit events do, instead of Options.OnClose and OnQuit,
// which are plain functions outside the tree.

// CloseRequestEvent is the user asking to close a window (its close
// button, Alt+F4, a WindowClose control, WindowHandle.Close), given to the
// handlers of UseCloseRequest.
type CloseRequestEvent struct {
	// Window is the window the user asked to close.
	Window    WindowHandle
	prevented *bool
}

// PreventDefault keeps the window from doing what its close request does
// by default: for the main window Options.OnClose, else quitting; for a
// Window component its OnClose. The handler then decides: hide the
// window, ask about unsaved changes, quit later with Quit.
func (e CloseRequestEvent) PreventDefault() { *e.prevented = true }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e CloseRequestEvent) DefaultPrevented() bool { return *e.prevented }

// QuitRequestEvent is the user or the OS asking the app to quit (macOS's
// Cmd+Q, the app menu's and the Dock's Quit, a logout), given to the
// handlers of UseQuitRequest.
type QuitRequestEvent struct{ prevented *bool }

// PreventDefault keeps the app from doing what a quit request does by
// default: Options.OnQuit, else a close request to every window. The
// handler then decides, and quits with Quit.
func (e QuitRequestEvent) PreventDefault() { *e.prevented = true }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e QuitRequestEvent) DefaultPrevented() bool { return *e.prevented }

type requestListener[E any] struct {
	f       func(E)
	stopped bool
}

// addListener adds a listener calling cur's latest function to *list and
// returns its removal.
func addListener[E any](list *[]*requestListener[E], cur *Ref[func(E)]) func() {
	l := &requestListener[E]{f: func(e E) { cur.Current(e) }}
	*list = append(*list, l)
	return func() {
		l.stopped = true
		*list = slices.DeleteFunc(*list, func(x *requestListener[E]) bool { return x == l })
	}
}

// runListeners calls the listeners of list with e, in the order they were
// added, as handlers of tree t run (handlerDepth).
func runListeners[E any](t *Tree, list []*requestListener[E], e E) {
	t.handlerDepth++
	defer func() { t.handlerDepth-- }()
	for _, l := range slices.Clone(list) {
		if !l.stopped {
			l.f(e)
		}
	}
}

// UseCloseRequest calls f, on the UI thread, each time the user asks to
// close the window the component renders in (UseWindow): its close
// button, Alt+F4, a WindowClose control, WindowHandle.Close, and the close
// request every window gets from a quit request without a handler. Every
// mounted handler of the window runs, in the order they mounted; when one
// calls e.PreventDefault, the window's default does not run (for the main
// window Options.OnClose, else the app quits; for a Window component its
// OnClose). f is the latest build's. A tree without a window (tela
// snapshot) never calls it. While a modal window is open over the window,
// the request brings the modal window forward and f does not run.
func UseCloseRequest(f func(CloseRequestEvent)) {
	if f == nil {
		panic("tela.UseCloseRequest(nil): pass the function that answers the request, such as tela.UseCloseRequest(func(e tela.CloseRequestEvent) { if dirty { e.PreventDefault(); ask() } })")
	}
	w := UseWindow()
	cur := UseRef[func(CloseRequestEvent)](nil)
	cur.Current = f
	UseMount(func() func() {
		v := w.t.wv
		if v == nil {
			return nil
		}
		return addListener(&v.closers, cur)
	})
}

// UseQuitRequest calls f, on the UI thread, each time the user or the OS
// asks the app to quit (macOS's Cmd+Q, the app menu's and the Dock's
// Quit, a logout). Every mounted handler of the app runs, in the order
// they mounted; when one calls e.PreventDefault, the default does not run
// (Options.OnQuit, else a close request to every window). f is the latest
// build's.
func UseQuitRequest(f func(QuitRequestEvent)) {
	if f == nil {
		panic("tela.UseQuitRequest(nil): pass the function that answers the request, such as tela.UseQuitRequest(func(e tela.QuitRequestEvent) { e.PreventDefault(); askThenQuit() })")
	}
	w := UseWindow()
	cur := UseRef[func(QuitRequestEvent)](nil)
	cur.Current = f
	UseMount(func() func() {
		v := w.t.wv
		if v == nil {
			return nil
		}
		return addListener(&v.host.quitters, cur)
	})
}

// closePrevented runs the close request handlers of window v and reports
// whether one prevented the default.
func (v *winView) closePrevented() bool {
	if len(v.closers) == 0 {
		return false
	}
	prevented := false
	runListeners(v.tree, v.closers, CloseRequestEvent{Window: WindowHandle{v.tree}, prevented: &prevented})
	return prevented
}

// quitPrevented runs the quit request handlers of the app and reports
// whether one prevented the default.
func (h *Host) quitPrevented() bool {
	if len(h.quitters) == 0 {
		return false
	}
	prevented := false
	runListeners(h.tree, h.quitters, QuitRequestEvent{prevented: &prevented})
	return prevented
}
