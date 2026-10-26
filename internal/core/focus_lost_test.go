package core

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// lostProbe is a page with a containing focus scope (a dialog: its root
// has TabIndex(-1), as aria.UseDialog gives it) holding a list of
// buttons, each removing itself when clicked, and a trigger outside.
type lostProbe struct {
	r                 recorder
	setItems          Setter[[]string]
	setOpen           Setter[bool]
	setAuto           Setter[bool]
	setRootFocusable  Setter[bool]
	setContain        Setter[bool]
	trigRef           ElementRef
	presence, restore bool
}

func (p *lostProbe) root() Node {
	return Component(func() Node {
		items, setItems := UseState([]string{"a", "b", "c"})
		open, setOpen := UseState(true)
		auto, setAuto := UseState(false)
		rootFocusable, setRF := UseState(true)
		contain, setContain := UseState(true)
		p.setItems, p.setOpen, p.setAuto, p.setRootFocusable, p.setContain = setItems, setOpen, setAuto, setRF, setContain
		p.trigRef = UseElementRef()
		dialogRef := UseElementRef()
		kids := make([]Child, 0, len(items)+1)
		if auto {
			kids = append(kids, button(Key("auto"), AutoFocus(true), Class("block h-8 w-20"),
				p.r.focus("focus", "auto"), Text("auto")))
		}
		cls := "block h-8 w-20"
		if p.presence {
			cls += " transition duration-200 opacity-100" // an exit that lasts
		}
		for _, k := range items {
			kids = append(kids, button(Key(k), Class(cls), Exit("opacity-0"),
				p.r.focus("focus", k), p.r.focus("blur", k),
				On("click", func(MouseEvent) {
					setItems.Update(func(s []string) []string {
						return slices.DeleteFunc(slices.Clone(s), func(x string) bool { return x == k })
					})
				}), Text(k)))
		}
		var list Child = Fragment(kids...)
		if p.presence {
			list = Presence(kids...)
		}
		dialogProps := []Child{Key("dialog"), AttachRef(dialogRef), Class("h-40 w-40"),
			Scope(ScopeOptions{Contain: contain, Restore: p.restore}),
			On("keydown", func(e KeyboardEvent) { p.r.add("keydown:dialog:" + string(e.Key)) }),
			// Only the dialog's own focus, not its children's bubbling.
			On("focus", func(FocusEvent) {
				if dialogRef.Focused() {
					p.r.add("focus:dialog")
				}
			})}
		if rootFocusable {
			dialogProps = append(dialogProps, TabIndex(-1))
		}
		return div(Key("page"), Class("h-full"),
			button(Key("trigger"), AttachRef(p.trigRef), Class("block h-8 w-20"), p.r.focus("focus", "trigger"), Text("t")),
			If(open, div(append(dialogProps, list)...)))
	})
}

// TestFocusLostInContainingScope: when the focused element inside a
// containing focus scope unmounts, the focus goes to the scope's root
// (Radix's FocusScope trapped, which focuses its container), without
// focus-visible after a pointer press and with it after a key; keys then
// reach the scope; Tab goes on from the root.
func TestFocusLostInContainingScope(t *testing.T) {
	p := &lostProbe{}
	x := newKeys(t, p.root(), 400, 300)
	x.frame()
	tr := x.tree()
	expect := func(step, focused, events string) {
		t.Helper()
		if g := focusedName(tr); g != focused {
			t.Errorf("%s: focused %s, want %s", step, g, focused)
		}
		if g := p.r.String(); g != events {
			t.Errorf("%s: events %q, want %q", step, g, events)
		}
		p.r.reset()
	}
	visible := func(k string) bool { return keyed(tr, k).istate&style.StateFocusVisible != 0 }

	// A press on b, which removes itself: the dialog takes the focus, with
	// no ring (the last interaction was the pointer).
	x.click(x.center("b"))
	expect("press on b", "dialog", "focus:b focus:dialog")
	if visible("dialog") {
		t.Error("the dialog shows focus-visible after a pointer press")
	}
	if keyed(tr, "dialog").istate&style.StateFocus == 0 {
		t.Error("the dialog does not match focus:")
	}
	// Keys go to the dialog now, not to the page.
	x.press("x", platform.CodeKeyX, 0)
	expect("a key after the unmount", "dialog", "keydown:dialog:x")
	// Tab goes on from the dialog's root: to its first tabbable element.
	x.tab()
	expect("Tab from the dialog", "a", "keydown:dialog:Tab focus:a")
	// With the keyboard (Enter clicks a): the dialog takes the focus and
	// shows focus-visible, as any script focus after a key does.
	x.enter()
	expect("Enter on a", "dialog", "keydown:dialog:Enter focus:dialog")
	if !visible("dialog") {
		t.Error("the dialog does not show focus-visible after a key")
	}
	// Shift+Tab from the root wraps to the scope's last element.
	x.shiftTab()
	expect("Shift+Tab from the dialog", "c", "keydown:dialog:Tab focus:c")
}

// TestFocusLostFallbacks: a scope whose root cannot take the focus gives
// it to its first tabbable element; a scope that does not contain keeps
// today's outcome (nothing focused); an AutoFocus element mounted in the
// same commit wins; the unmount in a commit that changes the rest of the
// scope too still keeps the focus inside.
func TestFocusLostFallbacks(t *testing.T) {
	p := &lostProbe{}
	x := newKeys(t, p.root(), 400, 300)
	x.frame()
	tr := x.tree()

	// No focusable root: the first tabbable element of the scope.
	p.setRootFocusable.Set(false)
	x.frame()
	x.click(x.center("b"))
	if g := focusedName(tr); g != "a" {
		t.Errorf("unmount in a scope without a focusable root: focused %s, want a", g)
	}

	// The unmount in a commit that also mounts and reorders the scope's
	// children (from a state update, in a frame).
	p.setRootFocusable.Set(true)
	x.frame()
	x.tab() // c
	if g := focusedName(tr); g != "c" {
		t.Fatalf("setup: focused %s", g)
	}
	p.setItems.Set([]string{"n2", "a", "n1"})
	x.frame()
	if g := focusedName(tr); g != "dialog" {
		t.Errorf("unmount with other changes in the commit: focused %s, want dialog", g)
	}

	// An AutoFocus element of the same commit takes the focus.
	x.tab() // n2
	p.r.reset()
	p.setItems.Set([]string{"a", "n1"})
	p.setAuto.Set(true)
	x.frame()
	if g := focusedName(tr); g != "auto" {
		t.Errorf("unmount with an AutoFocus element: focused %s, want auto", g)
	}
	if g := p.r.String(); g != "focus:auto" {
		t.Errorf("unmount with an AutoFocus element: events %q", g)
	}

	// A scope that does not contain: nothing focused, as in the browser.
	p.setAuto.Set(false)
	p.setContain.Set(false)
	x.frame()
	x.click(x.center("a"))
	if tr.Focused() != nil {
		t.Errorf("unmount in a scope that does not contain: %s focused", focusedName(tr))
	}
}

// TestFocusLostScopeCloses: when the whole scope unmounts (a dialog
// closing) its own restoration runs, and the root is never focused.
func TestFocusLostScopeCloses(t *testing.T) {
	p := &lostProbe{restore: true}
	x := newKeys(t, p.root(), 400, 300)
	x.frame()
	tr := x.tree()
	p.setOpen.Set(false)
	x.frame()
	p.trigRef.Focus()
	p.setOpen.Set(true)
	x.frame()
	x.tab() // into the scope: a
	if g := focusedName(tr); g != "a" {
		t.Fatalf("setup: focused %s", g)
	}
	p.r.reset()
	p.setOpen.Set(false)
	x.frame()
	if g := focusedName(tr); g != "trigger" {
		t.Errorf("the scope closed: focused %s, want trigger (restored)", g)
	}
	if g := p.r.String(); g != "focus:trigger" {
		t.Errorf("the scope closed: events %q, want only the restoration", g)
	}
	// The same after the root took the focus of an element that unmounted.
	p.setOpen.Set(true)
	x.frame()
	x.tab()
	x.click(x.center("b")) // removes itself: the root has the focus
	if g := focusedName(tr); g != "dialog" {
		t.Fatalf("setup: focused %s", g)
	}
	p.r.reset()
	p.setOpen.Set(false)
	x.frame()
	if g := focusedName(tr); g != "trigger" {
		t.Errorf("the scope closed with its root focused: focused %s, want trigger", g)
	}
	if g := p.r.String(); g != "focus:trigger" {
		t.Errorf("the scope closed with its root focused: events %q", g)
	}
}

// TestFocusLostExit: a focused element that starts an exit (Presence) is
// lost as on unmount, and the containing scope takes the focus back.
func TestFocusLostExit(t *testing.T) {
	p := &lostProbe{presence: true}
	x := newKeys(t, p.root(), 400, 300)
	x.frame()
	tr := x.tree()
	x.click(x.center("b"))
	if keyed(tr, "b") == nil {
		t.Fatal("b has no exit")
	}
	if g := focusedName(tr); g != "dialog" {
		t.Errorf("exit of the focused element: focused %s, want dialog", g)
	}
}
