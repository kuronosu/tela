package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
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
	// A pan that starts on the row cancels it once, and ends its active:
	// state when the scroll starts, as Chromium does (not when the finger
	// lifts).
	x.send(touch(platform.PointerDown, 50))
	row := keyed(x.tree(), "row")
	if row.istate&style.StateActive == 0 {
		t.Fatal("the pressed row is not active")
	}
	y := float32(50)
	for range 8 {
		x.b.Advance(10 * time.Millisecond)
		y -= 5
		x.send(touch(platform.PointerMove, y))
	}
	if got := r.String(); got != "pointerdown:row pointercancel:row" {
		t.Errorf("a pan from the row: %q", got)
	}
	if row.istate&style.StateActive != 0 {
		t.Error("the row is still active while the touch scrolls")
	}
	x.send(touch(platform.PointerUp, y))
	if row.istate&style.StateActive != 0 {
		t.Error("the row is active after the release")
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
	const tap = "overlay-start pointerdown:row overlay-outside pointerup:row mouseup:row"
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
	if !strings.HasPrefix(got, "overlay-start pointerdown:row pointermove:row") || !strings.HasSuffix(got, "pointermove:row pointercancel:row") {
		t.Errorf("a pan from the row, released: %q", got)
	}
	r.reset()
	x.frame()
	x.send(touch(platform.PointerDown, 30), touch(platform.PointerUp, 30))
	if got := r.String(); got != tap {
		t.Errorf("a tap after the pan: %q", got)
	}
}

// touchEv is a touch event of the primary contact at (px, py), with the
// buttons a pressed contact holds.
func touchEv(a platform.PointerAction, px, py float32) platform.PointerEvent {
	e := ptr(a, px, py, platform.ButtonNone, 0)
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

// TestTouchPanResetsClicks: a press that a touch pan took over did not
// click, so it does not start a click sequence: a quick tap after the
// scroll counts 1 (2 would select a word in a text field or fire
// OnDoubleClick), even where no tap recognizer was rejected (the row has
// no click handler).
func TestTouchPanResetsClicks(t *testing.T) {
	var counts []int
	root := div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"),
		div(Key("row"), Class("h-[60px]"), On("mousedown", func(e MouseEvent) { counts = append(counts, e.ClickCount) })),
		div(Class("h-[1000px]")))
	x := newScrollHost(t, root, 300, 300, Config{})
	x.send(touchEv(platform.PointerDown, 100, 50))
	y := float32(50)
	for range 8 {
		x.b.Advance(10 * time.Millisecond)
		y -= 5
		x.send(touchEv(platform.PointerMove, 100, y))
	}
	x.send(touchEv(platform.PointerUp, 100, y))
	x.frame()
	x.send(touchEv(platform.PointerDown, 100, 20), touchEv(platform.PointerUp, 100, 20))
	if want := []int{1, 1}; !slices.Equal(counts, want) {
		t.Errorf("a tap after a pan: click counts %v, want %v", counts, want)
	}
	// A second tap right away is a double tap.
	x.send(touchEv(platform.PointerDown, 100, 20), touchEv(platform.PointerUp, 100, 20))
	if want := []int{1, 1, 2}; !slices.Equal(counts, want) {
		t.Errorf("a double tap: click counts %v, want %v", counts, want)
	}
}

// TestTouchPanNoBoundaryEvents: once a pan cancelled the pointer, the
// elements the finger passes over get no enter or leave events: the
// pressed element's chain is left right after the pointercancel (Pointer
// Events: pointerout and pointerleave follow pointercancel), and nothing
// more of the touch is dispatched until it lifts, even with the implicit
// capture released (here by the row's pointerdown handler) or lost.
func TestTouchPanNoBoundaryEvents(t *testing.T) {
	var r recorder
	var refs [4]ElementRef
	comp := func() Node {
		var rows []Child
		for i := range refs {
			refs[i] = UseElementRef()
			id := fmt.Sprintf("r%d", i)
			ref := refs[i]
			rows = append(rows, div(Key(id), AttachRef(ref), Class("h-[40px]"),
				On("pointerdown", func(e PointerEvent) {
					r.add("pointerdown:" + id)
					ReleasePointerCapture(ref, e.PointerID)
				}),
				r.pointer("pointerenter", id), r.pointer("pointerleave", id), r.pointer("pointercancel", id),
				r.pointer("pointerup", id), r.mouse("mouseenter", id), r.mouse("mouseleave", id)))
		}
		return div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"), div(rows...), div(Class("h-[1000px]")))
	}
	x := newScrollHost(t, Component(comp), 300, 300, Config{})
	// The list is at its top: a pan down does not scroll it, so the
	// finger moves over r1, r2 and r3.
	x.send(touchEv(platform.PointerDown, 100, 5))
	const pressed = "pointerenter:r0 mouseenter:r0 pointerdown:r0"
	if got := r.String(); got != pressed {
		t.Fatalf("the press: %q, want %q", got, pressed)
	}
	r.reset()
	for y := float32(10); y <= 150; y += 5 {
		x.b.Advance(10 * time.Millisecond)
		x.send(touchEv(platform.PointerMove, 100, y))
	}
	const cancelled = "pointercancel:r0 pointerleave:r0 mouseleave:r0"
	if got := r.String(); got != cancelled {
		t.Errorf("the pan: %q, want %q", got, cancelled)
	}
	r.reset()
	x.send(touchEv(platform.PointerUp, 100, 150))
	if got := r.String(); got != "" {
		t.Errorf("the release after the pan: %q, want nothing", got)
	}
	r.reset()
	x.frame()
	x.send(touchEv(platform.PointerDown, 100, 60), touchEv(platform.PointerUp, 100, 60))
	const tap = "pointerenter:r1 mouseenter:r1 pointerdown:r1 pointerup:r1 pointerleave:r1 mouseleave:r1"
	if got := r.String(); got != tap {
		t.Errorf("a tap after the pan: %q, want %q", got, tap)
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
