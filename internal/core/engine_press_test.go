package core

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// A touch pan that wins the arena cancels the pointer for the element the
// finger pressed (pointercancel, as a browser does when it takes a touch
// over for scrolling); a touch that only taps gets none.
func TestTouchPanCancelsThePressedElement(t *testing.T) {
	var r recorder
	root := div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"),
		div(Key("row"), Class("h-[60px]"), r.pointer("pointerdown", "row"), r.pointer("pointercancel", "row"), r.pointer("pointerup", "row")),
		div(Class("h-[1000px]")))
	x := newScrollHost(t, root, 300, 300, Config{})
	touch := func(a platform.PointerAction, py float32) platform.PointerEvent {
		e := ptr(a, 100, py, platform.ButtonNone, 0)
		e.Kind = platform.PointerTouch
		switch a {
		case platform.PointerDown:
			e.Button, e.Buttons = platform.ButtonPrimary, platform.ButtonPrimary.Mask()
		case platform.PointerMove:
			e.Buttons = platform.ButtonPrimary.Mask()
		case platform.PointerUp:
			e.Button = platform.ButtonPrimary
		}
		return e
	}
	x.send(touch(platform.PointerDown, 30), touch(platform.PointerUp, 30))
	if got := r.String(); got != "pointerdown:row pointerup:row" {
		t.Errorf("a tap: %q", got)
	}
	r.reset()
	// A pan that starts on the row cancels it once.
	x.send(touch(platform.PointerDown, 50))
	y := float32(50)
	for range 8 {
		x.b.Advance(10 * time.Millisecond)
		y -= 5
		x.send(touch(platform.PointerMove, y))
	}
	if got := r.String(); got != "pointerdown:row pointercancel:row" {
		t.Errorf("a pan from the row: %q", got)
	}
}

// TestTouchPanEndsThePress: once a touch pan took the touch over
// (pointercancel), the pointer's later events are not dispatched, as in
// a browser: no pointermove, pointerup or mouseup reaches the element the
// finger pressed, and a popover whose outside the touch started in is not
// told that the press ended outside it (it would close although the user
// only scrolled past it). The next touch is a press again.
func TestTouchPanEndsThePress(t *testing.T) {
	var r recorder
	row := []Child{Key("row"), Class("h-[60px]"),
		r.pointer("pointerdown", "row"), r.pointer("pointermove", "row"), r.pointer("pointercancel", "row"),
		r.pointer("pointerup", "row"), r.mouse("mouseup", "row")}
	root := div(Key("page"),
		div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"), div(row...), div(Class("h-[1000px]"))),
		Portal(div(Key("pop"), Class("fixed top-[250px] left-[250px] size-10"),
			InteractOutside(OutsideOptions{
				OnStart: func(PointerEvent) { r.add("start") },
				OnEnd:   func(PointerEvent) { r.add("end") },
			}),
			Overlay(OverlayOptions{
				OnInteractOutsideStart: func(PointerEvent) { r.add("overlay-start") },
				OnInteractOutside:      func(PointerEvent) { r.add("overlay-outside") },
			}))))
	x := newScrollHost(t, root, 300, 300, Config{})
	touch := func(a platform.PointerAction, py float32) platform.PointerEvent {
		e := ptr(a, 100, py, platform.ButtonNone, 0)
		e.Kind = platform.PointerTouch
		switch a {
		case platform.PointerDown:
			e.Button, e.Buttons = platform.ButtonPrimary, platform.ButtonPrimary.Mask()
		case platform.PointerMove:
			e.Buttons = platform.ButtonPrimary.Mask()
		case platform.PointerUp:
			e.Button = platform.ButtonPrimary
		}
		return e
	}
	const tap = "start overlay-start pointerdown:row end overlay-outside pointerup:row mouseup:row"
	x.send(touch(platform.PointerDown, 30), touch(platform.PointerUp, 30))
	if got := r.String(); got != tap {
		t.Errorf("a tap: %q", got)
	}
	r.reset()
	x.send(touch(platform.PointerDown, 50))
	y := float32(50)
	for range 8 {
		x.b.Advance(10 * time.Millisecond)
		y -= 5
		x.send(touch(platform.PointerMove, y))
	}
	x.send(touch(platform.PointerUp, y))
	got := r.String()
	if !strings.HasPrefix(got, "start overlay-start pointerdown:row pointermove:row") || !strings.HasSuffix(got, "pointermove:row pointercancel:row") {
		t.Errorf("a pan from the row, released: %q", got)
	}
	r.reset()
	x.frame()
	x.send(touch(platform.PointerDown, 30), touch(platform.PointerUp, 30))
	if got := r.String(); got != tap {
		t.Errorf("a tap after the pan: %q", got)
	}
}

// FocusVisibleWithin covers the focused element and its ancestors, and
// follows the focus-visible heuristic.
func TestFocusVisibleWithin(t *testing.T) {
	var box, other ElementRef
	comp := func() Node {
		box, other = UseElementRef(), UseElementRef()
		return div(div(Key("box"), AttachRef(box), button(Key("a"), Class("block h-10 w-20"), Text("A"))),
			div(AttachRef(other), button(Key("b"), Class("block h-10 w-20"), Text("B"))))
	}
	x := newKeys(t, Component(comp), 200, 200)
	if FocusVisibleWithin(box) || ContainsFocus(box) {
		t.Fatal("nothing is focused yet")
	}
	x.tab()
	if !FocusVisibleWithin(box) || FocusVisibleWithin(other) {
		t.Error("Tab: box within, other not")
	}
	x.click(10, 50) // B: the focus moves by the mouse
	if ContainsFocus(box) || !ContainsFocus(other) || FocusVisibleWithin(box) || FocusVisibleWithin(other) {
		t.Error("click on B: other is focused but not focus-visible")
	}
	x.click(10, 10)
	if !ContainsFocus(box) || FocusVisibleWithin(box) {
		t.Error("click: focused but not focus-visible")
	}
	x.press("x", platform.CodeKeyX, 0)
	if !FocusVisibleWithin(box) {
		t.Error("a key after the click: focus-visible")
	}
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	if FocusVisibleWithin(box) {
		t.Error("focus-visible in an inactive window")
	}
}
