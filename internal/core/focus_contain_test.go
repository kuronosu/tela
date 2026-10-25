package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// TestFocusScopeContainment: while a containing focus scope is active, no
// focus change takes the focus out of it (ADR-160, React Aria's
// useFocusContainment): a press on nothing focusable keeps the focus, a
// press or a script aiming outside is refused or brought back to the
// element focused last inside, Blur does nothing, and the handlers see no
// focus going out and coming back.
func TestFocusScopeContainment(t *testing.T) {
	var r recorder
	var setDis Setter[bool]
	var outRef, d2Ref ElementRef
	outClicks := 0
	app := func() Node {
		outRef, d2Ref = UseElementRef(), UseElementRef()
		dis, s := UseState(false)
		setDis = s
		return div(Key("page"), Class("h-full"),
			button(Key("out"), AttachRef(outRef), Class("block h-10 w-20"), r.focus("focus", "out"), r.focus("blur", "out"),
				On("click", func(MouseEvent) { outClicks++ }), Text("out")),
			div(Key("blank"), Class("h-10 w-20")),
			div(Key("scope"), FocusScope(true, false),
				button(Key("d1"), Class("block h-10 w-20"), r.focus("focus", "d1"), r.focus("blur", "d1"), Text("1")),
				div(Key("plain"), Class("h-10 w-20"), Text("p")),
				button(Key("d2"), AttachRef(d2Ref), Disabled(dis), Class("block h-10 w-20"),
					r.focus("focus", "d2"), r.focus("blur", "d2"), Text("2")),
				// Portal content rendered from inside the scope is inside it.
				Portal(button(Key("pop"), Class("fixed top-[0px] left-[200px] h-10 w-20"),
					r.focus("focus", "pop"), r.focus("blur", "pop"), Text("pop")))))
	}
	x := newKeys(t, Component(app), 400, 300)
	tr := x.tree()
	expect := func(step, focused, events string) {
		t.Helper()
		if g := focusedName(tr); g != focused {
			t.Errorf("%s: focused %s, want %s", step, g, focused)
		}
		if g := r.String(); g != events {
			t.Errorf("%s: events %q, want %q", step, g, events)
		}
		r.reset()
	}
	visible := func(k string) bool { return keyed(tr, k).istate&style.StateFocusVisible != 0 }

	// Nothing is focused: a press on a focusable element outside sends the
	// focus into the scope (its first tabbable element), and the click
	// still reaches the element pressed.
	x.click(x.center("out"))
	expect("press outside with nothing focused", "d1", "focus:d1")
	if outClicks != 1 {
		t.Errorf("the press outside clicked %d times, want 1", outClicks)
	}

	// The focus is inside: presses on nothing focusable, outside or inside,
	// and on a focusable element outside leave it there, with no events.
	x.click(x.center("blank"))
	expect("press on nothing focusable outside", "d1", "")
	x.click(x.center("plain"))
	expect("press on nothing focusable inside", "d1", "")
	x.click(x.center("out"))
	expect("press on a focusable element outside", "d1", "")
	if outClicks != 2 {
		t.Errorf("the second press outside clicked %d times in all, want 2", outClicks)
	}
	// Scripts: Focus of an element outside, the focus manager over an
	// outside subtree, Blur of the focused element.
	outRef.Focus()
	expect("Focus outside", "d1", "")
	if FocusFirst(outRef, FocusOptions{}) {
		t.Error("FocusFirst outside the scope reported success")
	}
	expect("FocusFirst outside", "d1", "")

	// Inside moves work, the Portal's content included.
	x.click(x.center("pop"))
	expect("press in a Portal of the scope", "pop", "blur:d1 focus:pop")
	d2Ref.Focus()
	expect("Focus inside", "d2", "blur:pop focus:d2")
	d2Ref.Blur()
	expect("Blur inside", "d2", "")

	// focus-visible: a press that keeps the focus leaves its state alone.
	x.tab()
	expect("Tab", "pop", "blur:d2 focus:pop")
	if !visible("pop") {
		t.Fatal("Tab: pop is not focus-visible")
	}
	x.click(x.center("blank"))
	expect("press on nothing after Tab", "pop", "")
	if !visible("pop") {
		t.Error("a press that kept the focus turned focus-visible off")
	}
	x.click(x.center("d1"))
	expect("press on d1", "d1", "blur:pop focus:d1")
	x.click(x.center("blank"))
	expect("press on nothing after a press", "d1", "")
	if visible("d1") {
		t.Error("a press that kept the focus turned focus-visible on")
	}

	// The window losing the focus is not leaving the scope.
	tr.SetWindowFocused(false)
	tr.SetWindowFocused(true)
	expect("window focus", "d1", "blur:d1 focus:d1")

	// A focused element that stops being focusable is still blurred; the
	// next focus change outside comes back to the scope: to the element
	// focused last while it can take the focus, else to the first one.
	d2Ref.Focus()
	setDis.Set(true)
	x.frame()
	expect("disable the focused d2", "<nil>", "blur:d1 focus:d2 blur:d2")
	x.click(x.center("blank"))
	expect("press on nothing with nothing focused", "<nil>", "")
	outRef.Focus()
	expect("Focus outside after the fix-up, last unfocusable", "d1", "focus:d1")
	setDis.Set(false)
	x.frame()
	d2Ref.Focus()
	setDis.Set(true)
	x.frame()
	setDis.Set(false)
	x.frame()
	expect("d2 disabled and enabled again", "<nil>", "blur:d1 focus:d2 blur:d2")
	x.click(x.center("out"))
	expect("press outside after the fix-up", "d2", "focus:d2")
	if outClicks != 3 {
		t.Errorf("presses outside clicked %d times in all, want 3", outClicks)
	}
}

// TestFocusScopeContainmentNested: only the active containing scope (the
// most recently mounted) holds the focus; when it goes, the outer one does
// again, and a scope that does not contain holds nothing.
func TestFocusScopeContainmentNested(t *testing.T) {
	var setInner, setContain Setter[bool]
	var outerRef ElementRef
	app := func() Node {
		outerRef = UseElementRef()
		inner, si := UseState(false)
		contain, sc := UseState(true)
		setInner, setContain = si, sc
		return div(Key("page"), Class("h-full"),
			button(Key("out"), Class("block h-10 w-20"), Text("out")),
			div(Key("outer"), FocusScope(contain, true),
				button(Key("o1"), AttachRef(outerRef), Class("block h-10 w-20"), Text("o1")),
				If(inner, div(Key("inner"), Scope(ScopeOptions{Contain: true, Restore: true, AutoFocus: true}),
					button(Key("i1"), Class("block h-10 w-20"), Text("i1"))))))
	}
	x := newKeys(t, Component(app), 400, 300)
	tr := x.tree()
	outerRef.Focus()
	setInner.Set(true)
	x.frame()
	if g := focusedName(tr); g != "i1" {
		t.Fatalf("inner AutoFocus: %s", g)
	}
	// The inner scope is active: the outer one's elements are outside it.
	outerRef.Focus()
	if g := focusedName(tr); g != "i1" {
		t.Errorf("Focus into the outer scope from the inner one: %s, want i1", g)
	}
	x.click(x.center("o1"))
	if g := focusedName(tr); g != "i1" {
		t.Errorf("press in the outer scope: %s, want i1", g)
	}
	// It goes: the focus is restored to o1, and the outer scope holds it.
	setInner.Set(false)
	x.frame()
	if g := focusedName(tr); g != "o1" {
		t.Fatalf("restore after the inner scope: %s, want o1", g)
	}
	x.click(x.center("out"))
	if g := focusedName(tr); g != "o1" {
		t.Errorf("press outside the outer scope: %s, want o1", g)
	}
	// A scope that does not contain keeps today's behavior.
	setContain.Set(false)
	x.frame()
	x.click(x.center("out"))
	if g := focusedName(tr); g != "out" {
		t.Errorf("press outside a scope that does not contain: %s, want out", g)
	}
	x.click(x.center("page"))
	if tr.Focused() != nil {
		t.Errorf("press on nothing without containment: %s still focused", focusedName(tr))
	}
}
