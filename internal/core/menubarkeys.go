package core

import "github.com/kuronosu/tela/internal/platform"

// Menu bar keys (ADR-255): on Windows and Linux, F10 and a lone Alt (Alt
// pressed and released with no other key, button or pointer press in
// between) move the keyboard to the window's menu bar, as the native bars
// of Win32 and GTK do. A drawn bar (package aria's UseMenuBar) puts
// MenuBarKeys on its element; the engine tracks the lone Alt, which no
// element's handlers could see whole (the focus may move, a press in
// between is a pointer event), and calls the first live bar in tree order
// as the default action of those keys.

// MenuBarKeys returns a prop that makes the primitive a menu bar for the
// keys that reach one: f runs as the default action of an F10 keydown
// with no modifier and of the release of a lone Alt, when no handler
// prevented the event (F10's keydown, Alt's keyup), with that event. The
// first mounted element with the prop, in tree order, that is not inert
// (outside an active modal, in an exiting Presence child) gets them; the
// others get nothing. For package aria; it is not exposed by tela or h.
func MenuBarKeys(f func(KeyboardEvent)) Prop {
	return Prop{name: "MenuBarKeys", apply: func(d *desc) { d.menuKeys = f }}
}

// menuKeyDown tracks the lone Alt at a keydown that st's dispatch did or
// did not prevent, and runs the bar's F10. It reports whether a bar took
// the key.
func (t *Tree) menuKeyDown(ke KeyboardEvent, st *eventState) bool {
	alone := !ke.Ctrl && !ke.Meta && !ke.Shift
	if ke.Key == KeyValue(platform.KeyAlt) {
		if ke.Repeat {
			return false
		}
		t.loneAlt = alone && !st.defaultPrevented()
		return false
	}
	t.loneAlt = false
	if ke.Key != KeyValue(platform.KeyF10) || ke.Repeat || !alone || ke.Alt || st.defaultPrevented() {
		return false
	}
	return t.runMenuBarKeys(ke, st)
}

// menuKeyUp runs the bar's lone Alt at the release of the Alt that was
// pressed alone.
func (t *Tree) menuKeyUp(ke KeyboardEvent, st *eventState) {
	armed := t.loneAlt
	t.loneAlt = false
	if !armed || ke.Key != KeyValue(platform.KeyAlt) || st.defaultPrevented() {
		return
	}
	t.runMenuBarKeys(ke, st)
}

// runMenuBarKeys calls the MenuBarKeys of the first live bar with ke.
func (t *Tree) runMenuBarKeys(ke KeyboardEvent, st *eventState) bool {
	bar := t.menuBarElement()
	if bar == nil {
		return false
	}
	if st == nil {
		st = &eventState{}
	}
	(&ke).prepare(t, st, bar)
	t.handlerDepth++ // it runs as a handler (flushNested)
	func() {
		defer func() { t.handlerDepth-- }()
		bar.node.d.menuKeys(ke)
	}()
	t.flushDiscrete()
	return true
}

// menuBarElement returns the first live element with MenuBarKeys, in tree
// order, or nil. perf: a walk of the element tree, at an F10 or a lone
// Alt only.
func (t *Tree) menuBarElement() *Element {
	var found *Element
	var walk func(el *Element) bool
	walk = func(el *Element) bool {
		if !el.mounted || el.inert || el.outside == outsideYes {
			return true
		}
		if el.kind == elemPrimitive && el.node.d.menuKeys != nil && el.outside == outsideNo {
			found = el
			return false
		}
		for _, c := range el.children {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	if t.root != nil {
		walk(t.root)
	}
	return found
}
