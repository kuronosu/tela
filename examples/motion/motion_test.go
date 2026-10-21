package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// The frames of the example's animations, captured with telatest.NoSettle
// (the clock moves only with Advance). Regenerate the goldens with
// TELA_UPDATE=1 go test ./examples/motion and look at them.

func mount(t *testing.T) *telatest.App {
	return telatest.Mount(t, tela.C(Motion), telatest.Size(720, 540), telatest.NoSettle())
}

func TestDialogFrames(t *testing.T) {
	app := mount(t)
	app.Golden("motion.png")
	app.Click(telatest.ByRole("button", "Open dialog"))
	app.Advance(80 * time.Millisecond)
	app.Golden("dialog_enter_80ms.png")
	app.Advance(220 * time.Millisecond)
	app.Golden("dialog_open.png")
	app.ExpectRole("dialog", "Changes saved")
	app.Click(telatest.ByRole("button", "Close"))
	app.Advance(80 * time.Millisecond)
	app.Golden("dialog_exit_80ms.png")
	app.ExpectNoText("Your profile is up to date") // settles: the exit ends
	if n := app.Count(telatest.ByRole("dialog", "")); n != 0 {
		t.Errorf("%d dialogs after the exit", n)
	}
}

func TestToastFrames(t *testing.T) {
	app := mount(t)
	app.Click(telatest.ByRole("button", "Show toast"))
	app.Advance(100 * time.Millisecond)
	app.Golden("toast_enter_100ms.png")
	app.Click(telatest.ByRole("button", "Hide toast"))
	app.Advance(100 * time.Millisecond)
	app.Golden("toast_exit_100ms.png")
	app.ExpectCount(telatest.ByRole("status", ""), 0)
}

func TestDragSpring(t *testing.T) {
	app := mount(t)
	card := telatest.ByClass("cursor-grab")
	x0Box := app.Bounds(card)
	x0, y0, w, h := x0Box.X, x0Box.Y, x0Box.Width, x0Box.Height
	cx, cy := x0+w/2, y0+h/2
	app.MouseMove(cx, cy)
	app.MouseDown()
	for i := 1; i <= 8; i++ {
		app.MouseMove(cx+18*float32(i), cy+6*float32(i))
		app.Advance(16 * time.Millisecond)
	}
	// The card trails the pointer on its spring (Bounds would settle the
	// app and let it catch up, so the golden shows it).
	app.Golden("drag_moving.png")
	app.MouseUp()
	app.Advance(120 * time.Millisecond)
	app.Golden("drag_release_120ms.png")
	app.Advance(180 * time.Millisecond)
	app.Golden("drag_release_300ms.png")
	app.ExpectText("Drag me") // settles: the spring comes to rest
	if r := app.Bounds(card); r.X != x0 || r.Y != y0 {
		t.Errorf("the card rests at %v, %v; it started at %v, %v", r.X, r.Y, x0, y0)
	}
}
