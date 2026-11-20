package core

import "slices"

// Window-level defaults for input nobody handled (ADR-298): the browser
// navigation of a window, such as the mouse's back and forward buttons and
// Alt+Left, which Chromium runs when the page did not handle the event. The
// hooks are for packages above core (router); apps handle the events with
// the usual handlers, which win by stopping or preventing them.

// unhandledListener is a listener of the window's unhandled input: f
// reports whether it used the input, which ends the dispatch.
type unhandledListener[E any] struct {
	f       func(E) bool
	stopped bool
}

// unhandled is the listeners of a Tree for one kind of input, in mount
// order (the effects of children run before their parents', so the
// innermost component listens first).
type unhandled[E any] struct{ list []*unhandledListener[E] }

// add adds a listener calling cur's latest function and returns its removal.
func (u *unhandled[E]) add(cur *Ref[func(E) bool]) func() {
	l := &unhandledListener[E]{f: func(e E) bool { return cur.Current(e) }}
	u.list = append(u.list, l)
	return func() {
		l.stopped = true
		u.list = slices.DeleteFunc(u.list, func(x *unhandledListener[E]) bool { return x == l })
	}
}

// run calls the listeners with e until one reports it used it, as handlers
// of tree t run (handlerDepth).
func (u *unhandled[E]) run(t *Tree, e E) {
	if len(u.list) == 0 {
		return
	}
	t.handlerDepth++
	defer func() { t.handlerDepth-- }()
	for _, l := range slices.Clone(u.list) {
		if !l.stopped && l.f(e) {
			return
		}
	}
}

// UseUnhandledKeyDown calls f, on the UI thread, with each keydown of the
// window the component renders in (UseWindow) that nothing handled: no
// handler called StopPropagation or PreventDefault, and neither a focused
// text field nor the static selection took the key (a field takes the
// keys its bindings do, as in Chromium, where Alt+Left is none of them on
// Windows and Linux and Command+[ none on a Mac). It runs after the
// default actions of the key, so f decides what a key means to the
// window, as the browser's accelerators do. f reports whether it used the
// key; the first that does ends the dispatch (the innermost component's
// first). f is the latest build's. Key repeats are given too: f ignores
// them when it should (e.Repeat).
func UseUnhandledKeyDown(f func(KeyboardEvent) bool) {
	if f == nil {
		panic("core.UseUnhandledKeyDown(nil): pass the function that handles the key and reports whether it used it")
	}
	w := UseWindow()
	cur := UseRef[func(KeyboardEvent) bool](nil)
	cur.Current = f
	UseMount(func() func() { return w.t.unhandledKeys.add(cur) })
}

// UseUnhandledMouseButton calls f, on the UI thread, with each release of
// the mouse's back or forward side button in the window the component
// renders in (UseWindow) that nothing handled: no handler of its
// pointerdown, mousedown, pointerup or mouseup called StopPropagation or
// PreventDefault. f reports whether it used the button; the first that does
// ends the dispatch (the innermost component's first). f is the latest
// build's.
func UseUnhandledMouseButton(f func(MouseButton) bool) {
	if f == nil {
		panic("core.UseUnhandledMouseButton(nil): pass the function that handles the button and reports whether it used it")
	}
	w := UseWindow()
	cur := UseRef[func(MouseButton) bool](nil)
	cur.Current = f
	UseMount(func() func() { return w.t.unhandledButtons.add(cur) })
}

// unhandledKey gives keydown e to the window's unhandled-key listeners.
func (t *Tree) unhandledKey(e KeyboardEvent) { t.unhandledKeys.run(t, e) }

// isSideButton reports whether b is the mouse's back or forward button.
func isSideButton(b MouseButton) bool { return b == MouseButtonBack || b == MouseButtonForward }
