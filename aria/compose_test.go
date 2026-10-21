package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Several hooks on one element, sharing a ref, all work, and the user's own
// handlers run too.
func TestHooksComposeOnOneElement(t *testing.T) {
	var ev events
	page := func() tela.Node {
		ref := tela.UseElementRef()
		btn := aria.UseButton(aria.ButtonOptions{Ref: ref, OnPress: func(aria.PressEvent) { ev.add("press") }})
		ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
		hover := aria.UseHover(aria.HoverOptions{})
		return Button(btn.Props, ring.Props, hover.Props, TestID("b"),
			OnPointerDown(func() { ev.add("user:pointerdown") }),
			Class("h-10 w-[100px]"),
			If(btn.Pressed, Class("h-[50px]")), If(ring.Focused, Class("w-[120px]")), If(hover.Hovered, Class("bg-red-500")),
			Text("Go"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.MouseMove(20, 20)
	app.MouseDown()
	if b := app.Bounds(telatest.ByTestID("b")); b.Height != 50 || b.Width != 120 {
		t.Errorf("pressed and focused: %g x %g, want 120 x 50", b.Width, b.Height)
	}
	app.MouseUp()
	if got := ev.take(); got != "user:pointerdown press" {
		t.Errorf("events %q", got)
	}
	// The press measures the element it was given: moving off ends it.
	app.MouseDown()
	app.MouseMove(250, 50)
	if b := app.Bounds(telatest.ByTestID("b")); b.Height != 40 {
		t.Errorf("after leaving: height %g, want 40", b.Height)
	}
	app.MouseUp()
	if got := ev.take(); got != "user:pointerdown" {
		t.Errorf("events after leaving: %q", got)
	}
}
