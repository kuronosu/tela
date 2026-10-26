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

// wizardProbe is a dialog-like containing scope (its root has
// TabIndex(-1)) whose content is a step: step 1 is a Next button that
// moves to step 2; step 2 is an input, which a layout effect of the
// commit focuses when focus is set (a wizard focusing its next step's
// field), and a Back button.
type wizardProbe struct {
	r                recorder
	setStep          Setter[int]
	setRootFocusable Setter[bool]
	focus            bool
}

func (p *wizardProbe) root() Node {
	return Component(func() Node {
		step, setStep := UseState(1)
		rootFocusable, setRF := UseState(true)
		p.setStep, p.setRootFocusable = setStep, setRF
		dialogRef, inputRef := UseElementRef(), UseElementRef()
		UseLayoutEffect(func() func() {
			if step == 2 && p.focus {
				inputRef.Focus()
			}
			return nil
		}, step)
		var content Child
		if step == 1 {
			content = button(Key("next"), Class("block h-8 w-20"), p.r.focus("focus", "next"),
				On("click", func(MouseEvent) { setStep.Set(2) }), Text("next"))
		} else {
			content = Fragment(
				input(Key("name"), AttachRef(inputRef), AriaLabel("name"), Class("block h-8 w-20"),
					p.r.focus("focus", "name")),
				button(Key("back"), Class("block h-8 w-20"), p.r.focus("focus", "back"),
					On("click", func(MouseEvent) { setStep.Set(1) }), Text("back")))
		}
		dialog := []Child{Key("dialog"), AttachRef(dialogRef), Class("h-40 w-40"),
			Scope(ScopeOptions{Contain: true}),
			On("keydown", func(e KeyboardEvent) { p.r.add("keydown:dialog:" + string(e.Key)) }),
			// The dialog's own focus only; every blur inside bubbles to it.
			On("focus", func(FocusEvent) {
				if dialogRef.Focused() {
					p.r.add("focus:dialog")
				}
			}),
			p.r.focus("blur", "dialog"),
			content}
		if rootFocusable {
			dialog = append(dialog, TabIndex(-1))
		}
		return div(Key("page"), Class("h-full"), div(dialog...))
	})
}

// TestFocusLostLayoutEffectFocuses: when the commit that unmounts the
// focused element of a containing scope has a layout effect that focuses
// another element of it (a wizard step replacing its content), that is
// the only focus change: the scope's root never takes the focus in
// between (Radix refocuses its container from a MutationObserver, after
// React's layout effects). A layout effect that does not focus leaves
// the recovery to the frame, styled before paint.
func TestFocusLostLayoutEffectFocuses(t *testing.T) {
	p := &wizardProbe{focus: true}
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

	// By keyboard: Enter on Next.
	x.tab()
	expect("Tab", "next", "focus:next")
	gen := tr.foc.gen
	x.enter()
	x.frame()
	expect("Enter on next", "name", "keydown:dialog:Enter focus:name")
	if d := tr.foc.gen - gen; d != 1 {
		t.Errorf("Enter on next: %d focus changes, want 1", d)
	}

	// By pointer. Back unmounts and the layout effect of step 1 does not
	// focus: the dialog takes the focus in the frame, styled before paint.
	x.click(x.center("back"))
	x.frame()
	expect("press on back", "dialog", "blur:dialog focus:back focus:dialog")
	if keyed(tr, "dialog").istate&style.StateFocus == 0 {
		t.Error("press on back: the dialog does not match focus: after the frame")
	}
	x.click(x.center("next"))
	gen = tr.foc.gen
	x.frame()
	expect("press on next", "name", "blur:dialog focus:next focus:name")
	if d := tr.foc.gen - gen; d != 1 {
		t.Errorf("press on next: %d focus changes in the frame, want 1", d)
	}
}

// TestFocusLostKeyBeforeFrame: a key pressed after the unmount, before the
// frame that runs the commit's layout effects, finds the focus back in
// the scope, so it reaches the scope (Escape closes a dialog).
func TestFocusLostKeyBeforeFrame(t *testing.T) {
	p := &wizardProbe{}
	x := newKeys(t, p.root(), 400, 300)
	x.frame()
	tr := x.tree()
	x.tab()
	x.enter() // Next unmounts; the layout effect of step 2 waits for the frame
	p.r.reset()
	if tr.Focused() != nil {
		t.Fatalf("setup: %s focused before the frame", focusedName(tr))
	}
	x.press("x", platform.CodeKeyX, 0)
	if g := focusedName(tr); g != "dialog" {
		t.Errorf("a key before the frame: focused %s, want dialog", g)
	}
	if g := p.r.String(); g != "focus:dialog keydown:dialog:x" {
		t.Errorf("a key before the frame: events %q", g)
	}
	p.r.reset()
	x.frame()
	if g := focusedName(tr); g != "dialog" || p.r.String() != "" {
		t.Errorf("the frame after the key: focused %s, events %q", g, p.r.String())
	}
}

// TestFocusLostPressWhilePending: a press on nothing focusable while the
// recovery waits for the frame does not cancel it: the containing scope
// holds the focus against such a press (ADR-160), as it does once the
// focus is back, so the frame gives the focus to the scope as usual. A
// focus change meanwhile does cancel it: when the element focused since
// is blurred by the fix-up (it stops being focusable), nothing is
// focused (ADR-160), and the stale recovery does not move the focus.
func TestFocusLostPressWhilePending(t *testing.T) {
	p := &wizardProbe{}
	x := newKeys(t, p.root(), 400, 300)
	x.frame()
	tr := x.tree()
	x.tab()
	x.enter() // Next unmounts; the recovery waits for the frame
	p.r.reset()
	if tr.Focused() != nil {
		t.Fatalf("setup: %s focused before the frame", focusedName(tr))
	}
	x.click(300, 250) // the page, outside the dialog: nothing focusable
	if tr.foc.lost == nil {
		t.Error("a press on nothing focusable cancelled the pending recovery")
	}
	x.frame()
	if g := focusedName(tr); g != "dialog" {
		t.Errorf("a press on nothing focusable before the frame: focused %s, want dialog", g)
	}
	if g := p.r.String(); g != "focus:dialog" {
		t.Errorf("a press on nothing focusable before the frame: events %q", g)
	}

	// Back to step 1 and Next by Enter again; then a press on the dialog
	// focuses it before the frame, and that frame's fix-up blurs it.
	p.setStep.Set(1)
	x.frame()
	x.tab()
	if g := focusedName(tr); g != "next" {
		t.Fatalf("setup: focused %s, want next", g)
	}
	x.enter()
	x.click(x.center("dialog"))
	if g := focusedName(tr); g != "dialog" {
		t.Fatalf("setup: the press focused %s, want dialog", g)
	}
	if tr.foc.lost != nil {
		t.Error("a focus change left the recovery pending")
	}
	p.setRootFocusable.Set(false)
	p.r.reset()
	x.frame()
	if tr.Focused() != nil {
		t.Errorf("fix-up blur after a focus change: %s focused, want none (no stale recovery)", focusedName(tr))
	}
	if g := p.r.String(); g != "blur:dialog" {
		t.Errorf("fix-up blur after a focus change: events %q", g)
	}
}

// TestFocusLostFallbackKeepsScroll: when the scope's root cannot take the
// focus, its first tabbable element takes it without being scrolled into
// view (Radix focuses with preventScroll), so a scrolled list whose
// focused row is deleted does not jump to the top; after a pointer press
// it shows no ring.
func TestFocusLostFallbackKeepsScroll(t *testing.T) {
	var list ElementRef
	app := func() Node {
		items, setItems := UseState([]string{"r0", "r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8", "r9"})
		list = UseElementRef()
		rows := make([]Child, 0, len(items)+3)
		rows = append(rows, Key("list"), AttachRef(list), Class("h-20 w-40 overflow-y-auto"))
		for _, k := range items {
			rows = append(rows, button(Key(k), Class("block h-8 w-40"), On("click", func(MouseEvent) {
				setItems.Update(func(s []string) []string {
					return slices.DeleteFunc(slices.Clone(s), func(x string) bool { return x == k })
				})
			}), Text(k)))
		}
		return div(Key("scope"), Class("w-40"), Scope(ScopeOptions{Contain: true}), div(rows...))
	}
	x := newKeys(t, Component(app), 400, 300)
	x.frame()
	tr := x.tree()
	list.ScrollTo(0, 96)
	x.frame()
	x.click(x.center("r4"))
	x.frame()
	if g := focusedName(tr); g != "r0" {
		t.Errorf("deleted row: focused %s, want r0 (the scope's first tabbable element)", g)
	}
	if _, y := list.ScrollOffset(); y != 96 {
		t.Errorf("deleted row: the list scrolled to %v, want 96 (no reveal)", y)
	}
	if keyed(tr, "r0").istate&style.StateFocusVisible != 0 {
		t.Error("deleted row: r0 shows focus-visible after a pointer press")
	}
}
