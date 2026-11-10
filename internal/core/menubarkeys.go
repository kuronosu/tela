package core

import (
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

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
// prevented the event (F10's keydown, Alt's keyup), with that event, and
// reports whether it took the key (it moved the focus). The first mounted
// element with the prop, in tree order, that is displayed (it and its
// ancestors not display:none) and not inert (outside an active modal, in
// an exiting Presence child) gets them; the others get nothing. For
// package aria; it is not exposed by tela or h.
func MenuBarKeys(f func(KeyboardEvent) bool) Prop {
	return Prop{name: "MenuBarKeys", apply: func(d *desc) { d.menuKeys = f }}
}

// MenuBarAccessOptions are the access keys of a menu bar (ADR-260), on
// the element that has MenuBarKeys.
type MenuBarAccessOptions struct {
	// Mnemonic runs as the default action of a keydown of a character
	// with Alt (Shift allowed; not Ctrl, Meta or a repeat) that no handler
	// prevented, and reports whether the bar took it (it opened a menu):
	// the key then has no other default action.
	Mnemonic func(KeyboardEvent) bool
	// AltHeld is called when Alt starts or stops being held alone (no
	// Ctrl or Meta): the keyboard cues of Windows and GTK, which underline
	// the access keys while it is. It stops at Alt's release, a keydown
	// without Alt (a release the window never got) and a change of the
	// window's focus.
	AltHeld func(held bool)
}

// MenuBarAccess returns a prop that gives the menu bar of MenuBarKeys
// (the same element) its access keys: see [MenuBarAccessOptions]. For
// package aria; it is not exposed by tela or h.
func MenuBarAccess(o MenuBarAccessOptions) Prop {
	return Prop{name: "MenuBarAccess", apply: func(d *desc) { d.menuAccess = &o }}
}

// menuKeyDown tracks the lone Alt at a keydown that st's dispatch did or
// did not prevent, and runs the bar's F10 and access keys. It reports
// whether the bar took the key.
func (t *Tree) menuKeyDown(ke KeyboardEvent, st *eventState) bool {
	alone := !ke.Ctrl && !ke.Meta && !ke.Shift
	if ke.Key == KeyValue(platform.KeyAlt) {
		if ke.Repeat {
			return false
		}
		t.loneAlt = alone && !st.defaultPrevented()
		t.setAltHeld(!ke.Ctrl && !ke.Meta)
		return false
	}
	t.loneAlt = false
	if !ke.Alt || ke.Ctrl || ke.Meta {
		t.setAltHeld(false)
	}
	if ke.Repeat || st.defaultPrevented() {
		return false
	}
	if ke.Alt && !ke.Ctrl && !ke.Meta && isCharKey(ke.Key) {
		return t.runMenuBarMnemonic(ke, st)
	}
	if ke.Key != KeyValue(platform.KeyF10) || !alone || ke.Alt {
		return false
	}
	return t.runMenuBarKeys(ke, st)
}

// isCharKey reports whether k is the logical key of a character (one
// printable rune), not a named key.
func isCharKey(k KeyValue) bool {
	r, n := utf8.DecodeRuneInString(string(k))
	return n > 0 && n == len(k) && r != utf8.RuneError && unicode.IsPrint(r) && r != ' '
}

// menuKeyUp runs the bar's lone Alt at the release of the Alt that was
// pressed alone.
func (t *Tree) menuKeyUp(ke KeyboardEvent, st *eventState) {
	armed := t.loneAlt
	t.loneAlt = false
	if ke.Key == KeyValue(platform.KeyAlt) {
		t.setAltHeld(false)
	}
	if !armed || ke.Key != KeyValue(platform.KeyAlt) || st.defaultPrevented() {
		return
	}
	t.runMenuBarKeys(ke, st)
}

// setAltHeld records whether Alt is held alone and tells a bar's AltHeld
// when it changes: the first live bar that Alt is held, and at its end
// the bar that was told (altBar), even when it went inert or another bar
// became the live one meanwhile (Alt+Enter opened a modal dialog), so
// that its cues do not stay.
func (t *Tree) setAltHeld(held bool) {
	if t.altHeld == held {
		return
	}
	t.altHeld = held
	var bar *Element
	if held {
		bar = t.menuBarElement()
		t.altBar = bar
	} else {
		bar, t.altBar = t.altBar, nil
	}
	if bar == nil || !bar.mounted {
		return
	}
	a := bar.node.d.menuAccess
	if a == nil || a.AltHeld == nil {
		return
	}
	t.handlerDepth++
	func() {
		defer func() { t.handlerDepth-- }()
		a.AltHeld(held)
	}()
	t.flushDiscrete()
}

// runMenuBarMnemonic calls the Mnemonic of the first live bar with ke and
// reports whether it took the key.
func (t *Tree) runMenuBarMnemonic(ke KeyboardEvent, st *eventState) bool {
	bar := t.menuBarElement()
	if bar == nil {
		return false
	}
	a := bar.node.d.menuAccess
	if a == nil || a.Mnemonic == nil {
		return false
	}
	if st == nil {
		st = &eventState{}
	}
	(&ke).prepare(t, st, bar)
	t.handlerDepth++
	took := func() bool {
		defer func() { t.handlerDepth-- }()
		return a.Mnemonic(ke)
	}()
	t.flushDiscrete()
	return took
}

// runMenuBarKeys calls the MenuBarKeys of the first live bar with ke and
// reports whether it took the key.
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
	took := func() bool {
		defer func() { t.handlerDepth-- }()
		return bar.node.d.menuKeys(ke)
	}()
	t.flushDiscrete()
	return took
}

// updateMenuBars records whether primitive el, whose node was old (nil
// at mount), has MenuBarKeys: the elements that have it are kept in
// t.menuBars, so that finding the live bar walks no tree.
func (t *Tree) updateMenuBars(el *Element, old *desc) {
	was, is := old != nil && old.menuKeys != nil, el.node.d.menuKeys != nil
	switch {
	case is && !was:
		t.menuBars = append(t.menuBars, el)
	case was && !is:
		t.menuBars = removeElement(t.menuBars, el)
	}
}

// unmountMenuBar forgets primitive el, unmounted, as a bar.
func (t *Tree) unmountMenuBar(el *Element) {
	t.menuBars = removeElement(t.menuBars, el)
	if t.altBar == el {
		t.altBar = nil
	}
}

// menuBarElement returns the first live element with MenuBarKeys, in tree
// order, or nil: mounted, displayed (it and its ancestors not
// display:none) and not inert. perf: it looks at the bars only (usually
// one), each with a walk up its ancestors, at F10, Alt's edges and an
// Alt+character.
func (t *Tree) menuBarElement() *Element {
	var found *Element
	for _, el := range t.menuBars {
		if !liveBar(el) {
			continue
		}
		if found == nil || treeBefore(el, found) {
			found = el
		}
	}
	return found
}

// liveBar reports whether el, an element with MenuBarKeys, can take the
// keys: mounted, not inert, inside the active modal's subtrees, and it
// and its ancestors rendered and not display:none.
func liveBar(el *Element) bool {
	if !el.mounted || el.inert || el.outside != outsideNo {
		return false
	}
	for a := el; a != nil; a = a.parent {
		if a.kind != elemPrimitive {
			continue
		}
		if !a.mounted || a.inert || a.outside == outsideYes || a.box == nil || a.box.cs == nil || a.box.cs.Display == style.DisplayNone {
			return false
		}
	}
	return true
}
