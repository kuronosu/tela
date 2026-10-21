package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestPointerCapture: an element that captures the mouse on pointerdown
// receives its moves and release outside it; without the capture it does
// not; a touch press can release its implicit capture.
func TestPointerCapture(t *testing.T) {
	var r recorder
	var ref ElementRef
	capture := true
	comp := func() Node {
		ref = UseElementRef()
		return div(Key("page"), Class("h-full"),
			div(Key("btn"), AttachRef(ref), Class("size-20"),
				On("pointerdown", func(e PointerEvent) {
					if capture {
						r.add(fmt.Sprint("capture:", SetPointerCapture(ref, e.PointerID)))
					}
				}),
				On("pointermove", func(e PointerEvent) { r.add(fmt.Sprintf("move:%g", e.X)) }),
				On("pointerup", func(e PointerEvent) { r.add(fmt.Sprintf("up:%g", e.X)) })))
	}
	x := newInput(t, Component(comp), 300, 300)
	x.send(move(10, 10), down(10, 10), moveHeld(150, 150), up(150, 150))
	if g := r.String(); g != "move:10 capture:true move:150 up:150" {
		t.Errorf("captured: %q", g)
	}
	if HasPointerCapture(ref, 1) {
		t.Error("the capture outlived the release")
	}
	r.reset()
	capture = false
	x.send(move(10, 10), down(10, 10), moveHeld(150, 150), up(150, 150))
	if g := r.String(); g != "move:10" {
		t.Errorf("not captured: %q", g)
	}
	// A pointer that is not pressed cannot be captured.
	if SetPointerCapture(ref, 1) {
		t.Error("captured a pointer without buttons")
	}
	// Touch: released implicit capture, the contact's moves go to what
	// is under it.
	r.reset()
	touch := func(a platform.PointerAction, px, py float32, held platform.Buttons) platform.PointerEvent {
		return platform.PointerEvent{Action: a, Kind: platform.PointerTouch, ID: 1, Pos: platform.Point{X: px, Y: py}, Button: platform.ButtonPrimary, Buttons: held}
	}
	x.send(touch(platform.PointerDown, 10, 10, platform.ButtonPrimary.Mask()))
	id := 0
	for k := range x.tree().in.ptrs {
		if k.kind == platform.PointerTouch {
			id = publicPointerID(k)
		}
	}
	if !HasPointerCapture(ref, id) {
		t.Fatal("a touch press is not captured by its target")
	}
	ReleasePointerCapture(ref, id)
	x.send(touch(platform.PointerMove, 150, 150, platform.ButtonPrimary.Mask()), touch(platform.PointerUp, 150, 150, 0))
	if g := r.String(); g != "" {
		t.Errorf("touch after releasing the capture: %q", g)
	}
}

// TestMergeProps: a merged prop applies its props in order, with the
// user's around it: handlers accumulate, other props take the last value.
func TestMergeProps(t *testing.T) {
	var r recorder
	hook := MergeProps(Role("menu"), r.mouse("click", "hook"), Prop{}, AriaLabel("Hook"))
	root := div(Key("box"), Class("size-20"), r.mouse("click", "before"), hook, r.mouse("click", "after"), AriaLabel("User"))
	x := newInput(t, root, 200, 200)
	x.click(10, 10)
	if g := r.String(); g != "click:before click:hook click:after" {
		t.Errorf("handlers: %q", g)
	}
	if snap := x.tree().AriaSnapshot(); snap != `- menu "User"` {
		t.Errorf("snapshot: %q", snap)
	}
	if msg := panicMessage(func() { Fragment(hook) }); msg == "" {
		t.Error("a merged prop on a Fragment does not panic")
	}
}

// TestFocusVisible: FocusVisible follows the focus-visible: heuristic.
func TestFocusVisible(t *testing.T) {
	var a, b ElementRef
	comp := func() Node {
		a, b = UseElementRef(), UseElementRef()
		return div(button(Key("a"), AttachRef(a), Class("block h-10 w-20"), Text("A")),
			button(Key("b"), AttachRef(b), Class("block h-10 w-20"), Text("B")))
	}
	x := newKeys(t, Component(comp), 200, 200)
	x.tab()
	if !FocusVisible(a) || FocusVisible(b) {
		t.Error("Tab: not focus-visible")
	}
	x.click(10, 50)
	if !b.Focused() || FocusVisible(b) {
		t.Error("click: focus-visible")
	}
	x.press("x", platform.CodeKeyX, 0)
	if !FocusVisible(b) {
		t.Error("a key after the click: not focus-visible")
	}
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	if FocusVisible(b) {
		t.Error("focus-visible in an inactive window")
	}
}
