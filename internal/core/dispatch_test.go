package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

func TestPreventDefaultOnPointerDown(t *testing.T) {
	var r recorder
	var prevented []bool
	root := div(Class("p-10"),
		div(Key("b"), Class("size-20"),
			On("pointerdown", func(e PointerEvent) { e.PreventDefault(); r.add("pointerdown") }),
			r.mouse("mousedown", "b"), r.mouse("mousemove", "b"), r.mouse("mouseup", "b"), r.mouse("click", "b")),
		div(Key("p"), Class("size-20"),
			On("mousedown", func(e MouseEvent) { prevented = append(prevented, e.DefaultPrevented()); e.PreventDefault() }),
			On("mousedown", func(e MouseEvent) { prevented = append(prevented, e.DefaultPrevented()) })),
	)
	x := newInput(t, root, 400, 300)
	x.send(move(50, 50))
	r.reset()
	x.send(down(50, 50), moveHeld(51, 50), up(51, 50), move(52, 50))
	if got := r.String(); got != "pointerdown click:b mousemove:b" {
		t.Errorf("got %q: a cancelled pointerdown suppresses the press's mouse events, not the click", got)
	}
	x.b.Advance(time.Second)
	x.send(down(50, 130), up(50, 130))
	if fmt.Sprint(prevented) != "[false true]" {
		t.Errorf("DefaultPrevented %v, want [false true]", prevented)
	}
}

func TestPointerCancel(t *testing.T) {
	var r recorder
	var end DragEvent
	root := div(Class("p-10"),
		div(Key("d"), Class("size-20 active:bg-black"), r.pointer("pointercancel", "d"), r.mouse("click", "d"),
			On("dragstart", func(DragEvent) { r.add("dragstart") }),
			On("dragend", func(e DragEvent) { r.add("dragend"); end = e })))
	x := newInput(t, root, 400, 300)
	x.send(down(50, 50), moveHeld(70, 50), moveHeld(90, 50), ptr(platform.PointerCancel, 90, 50, 0, 0))
	if got := r.String(); got != "dragstart pointercancel:d dragend" {
		t.Errorf("got %q", got)
	}
	if end.VelocityX != 0 || end.OffsetX != 40 {
		t.Errorf("cancelled drag end %+v", end)
	}
	if d := keyed(x.tree(), "d"); d.istate&style.StateActive != 0 {
		t.Error("still active after the cancel")
	}
	r.reset()
	x.send(down(50, 50), ptr(platform.PointerCancel, 50, 50, 0, 0), up(50, 50))
	if got := r.String(); got != "pointercancel:d" {
		t.Errorf("a cancelled press clicked: %q", got)
	}
}

func TestHandlerTypeChecked(t *testing.T) {
	for _, c := range []struct {
		event string
		h     any
	}{
		{"click", func(PointerEvent) {}},
		{"pointerdown", func(MouseEvent) {}},
		{"drag", func(MouseEvent) {}},
		{"longpress", (func(PointerEvent))(nil)},
		{"wheel", func(MouseEvent) {}},
	} {
		if msg := panicMessage(func() { On(c.event, c.h) }); msg == "" {
			t.Errorf("On(%q, %T) did not panic", c.event, c.h)
		}
	}
}
