package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// ---------------------------------------------------------------------
// OnTouchLongPress: a long press for touch and pens only.

func kindEv(a platform.PointerAction, kind platform.PointerKind, px, py float32) platform.PointerEvent {
	e := platform.PointerEvent{Action: a, Kind: kind, ID: 1, Pos: platform.Point{X: px, Y: py}}
	switch a {
	case platform.PointerDown:
		e.Button, e.Buttons = platform.ButtonPrimary, platform.ButtonPrimary.Mask()
	case platform.PointerUp:
		e.Button = platform.ButtonPrimary
	}
	return e
}

// hold advances the clock by d, letting a long press that is due fire.
func (x *inputHarness) hold(d time.Duration) {
	x.b.Advance(d)
	x.b.Wake()
	x.b.Drain(x.h)
}

func TestTouchLongPress(t *testing.T) {
	var r recorder
	tlp := OnTouchLongPress(func(e PointerEvent) { r.add("tlp:" + e.Kind.String()) })
	root := div(Class("p-0"),
		div(Key("area"), Class("h-[100px] w-[200px]"), tlp,
			button(Key("btn"), Class("block h-10 w-20"), r.mouse("click", "btn"), Text("b"))),
		div(Key("both"), Class("h-[100px] w-[200px]"), tlp, r.pointer("longpress", "both"), r.mouse("click", "both")),
	)
	x := newInput(t, root, 400, 300)

	// A mouse held on the button for longer than a long press clicks it:
	// the area's touch-only long press does not join the arena (no timer).
	x.send(move(20, 20), down(20, 20))
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("a mouse press armed the long-press timer of a touch-only long press")
	}
	x.hold(800 * time.Millisecond)
	x.send(up(20, 20))
	if got := r.String(); got != "click:btn" {
		t.Errorf("slow mouse click: %q", got)
	}

	// A touch and a pen that stay down are long presses, without a click.
	for _, kind := range []platform.PointerKind{platform.PointerTouch, platform.PointerPen} {
		r.reset()
		x.hold(time.Second)
		x.send(kindEv(platform.PointerDown, kind, 20, 20))
		x.hold(600 * time.Millisecond)
		x.send(kindEv(platform.PointerUp, kind, 20, 20))
		if got := r.String(); got != "tlp:"+PointerKind(kind).String() {
			t.Errorf("%v long press: %q", kind, got)
		}
		if kind == platform.PointerPen {
			x.send(kindEv(platform.PointerLeave, kind, 20, 20))
		}
	}

	// With a long-press handler for every pointer too, a mouse press is a
	// long press, and the touch-only handler does not run for it.
	r.reset()
	x.hold(time.Second)
	x.send(move(20, 150), down(20, 150))
	x.hold(600 * time.Millisecond)
	x.send(up(20, 150))
	if got := r.String(); got != "longpress:both" {
		t.Errorf("mouse on both: %q", got)
	}
	r.reset()
	x.hold(time.Second)
	x.send(kindEv(platform.PointerDown, platform.PointerTouch, 20, 150))
	x.hold(600 * time.Millisecond)
	x.send(kindEv(platform.PointerUp, platform.PointerTouch, 20, 150))
	if got := r.String(); got != "tlp:touch longpress:both" {
		t.Errorf("touch on both: %q", got)
	}
}
