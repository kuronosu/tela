package tela_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Frames of animations driven from the public API, captured with
// telatest.NoSettle: the clock moves only with Advance, so each golden is
// the picture at a known time. The numeric behavior of Animate, Velocity,
// OnDone and UsePresenceTween is checked in anim_example_test.go,
// ergonomics_test.go and internal/core; these tests check them end to end
// through real input and the pixels. Regenerate the goldens with
// TELA_UPDATE=1 and look at them.

// dragPage is a card that follows the pointer through a spring while it is
// dragged and springs back, with the pointer's release velocity, when it
// is released. The position is a Value read by a TranslateFn, so the drag
// never builds the component.
func dragPage() tela.Node {
	pos := tela.UseValue(tela.Point{})
	start := tela.UseRef(tela.Point{})
	dragPos = pos
	return Div(Class("flex h-full items-center justify-center bg-slate-100"),
		Div(Class("flex size-24 cursor-grab select-none items-center justify-center rounded-3xl",
			"bg-linear-to-br from-sky-400 to-indigo-500 text-sm font-medium text-white shadow-xl active:cursor-grabbing"),
			Style(tela.TranslateFn(func() (float32, float32) { p := pos.Get(); return p.X, p.Y })),
			OnDragStart(func() { start.Current = pos.Latest() }),
			OnDrag(func(e tela.DragEvent) {
				to := tela.Point{X: start.Current.X + e.OffsetX, Y: start.Current.Y + e.OffsetY}
				tela.Animate(pos, to, tela.Stiffness(700), tela.Damping(45))
			}),
			OnDragEnd(func(e tela.DragEvent) {
				tela.Animate(pos, tela.Point{}, tela.Spring(tela.Stiffness(250), tela.Damping(12)),
					tela.Velocity(tela.Point{X: e.VelocityX, Y: e.VelocityY}))
			}),
			Text("Drag me")))
}

// dragPos is the position Value of dragPage's last build.
var dragPos *tela.Value[tela.Point]

func TestFramesDragSpring(t *testing.T) {
	app := telatest.Mount(t, tela.C(dragPage), telatest.Size(360, 240), telatest.NoSettle())
	card := telatest.ByClass("cursor-grab")
	rest := app.Bounds(card)
	cx, cy := rest.X+rest.Width/2, rest.Y+rest.Height/2
	app.MouseMove(cx, cy)
	app.MouseDown()
	const dx, dy = 18, 6
	for i := 1; i <= 8; i++ {
		app.MouseMove(cx+dx*float32(i), cy+dy*float32(i))
		app.Advance(16 * time.Millisecond)
	}
	// The card trails the pointer on its spring: it moved, and it is
	// behind where the pointer is.
	if p := dragPos.Get(); p.X <= 0 || p.X >= 8*dx || p.Y <= 0 || p.Y >= 8*dy {
		t.Errorf("while dragging the card is at %v; want it between its place and the pointer (%v, %v)", p, 8*dx, 8*dy)
	}
	app.Golden("anim_drag_moving.png")
	app.MouseUp()
	var minX float32
	for i := 0; i < 120; i++ { // 2 s at 60 fps
		app.Advance(time.Second / 60)
		minX = min(minX, dragPos.Get().X)
		if i == 6 {
			app.Golden("anim_drag_release_120ms.png")
		}
		if i == 17 {
			app.Golden("anim_drag_release_300ms.png")
		}
	}
	// The bouncy spring overshoots the origin on its way back.
	if minX >= -1 {
		t.Errorf("the card never overshot its place after the release (minimum x %v)", minX)
	}
	app.ExpectText("Drag me")
	if r := app.Bounds(card); r != rest {
		t.Errorf("the card rests at %v; it started at %v", r, rest)
	}
}

// toastPage shows a toast that runs its own enter and exit: a tween from 0
// to 1 on mount, back to 0 while its Presence child exits, and the exit
// ends when the tween arrives (UsePresenceTween).
func toastPage() tela.Node {
	saved, setSaved := tela.UseState(false)
	return Div(Class("relative flex h-full flex-col items-center bg-slate-100 p-6"),
		Button(Class("rounded-lg bg-indigo-600 px-4 py-2 font-medium text-white"),
			OnClick(tela.Toggle(setSaved)), IfElse(saved, Text("Hide toast"), Text("Show toast"))),
		Presence(If(saved, tela.C(toast))))
}

func toast() tela.Node {
	shown := tela.UsePresenceTween(tela.Duration(250*time.Millisecond), tela.Ease(tela.EaseOut))
	return Div(Class("absolute inset-x-0 bottom-6 flex justify-center"),
		Div(Role("status"), Class("rounded-xl bg-slate-900 px-5 py-3 text-sm text-white shadow-lg"),
			Style(tela.TranslateFn(func() (float32, float32) { return 0, 24 * (1 - shown.Get()) }),
				tela.OpacityFn(shown.Get)),
			Text("Saved to your profile")))
}

func TestFramesToast(t *testing.T) {
	app := telatest.Mount(t, tela.C(toastPage), telatest.Size(360, 200), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Show toast"))
	app.Advance(100 * time.Millisecond)
	app.Golden("anim_toast_enter_100ms.png")
	app.Advance(time.Second)
	app.ExpectRole("status", "")
	app.Click(telatest.ByRole("button", "Hide toast"))
	app.Advance(100 * time.Millisecond)
	app.Golden("anim_toast_exit_100ms.png")
	app.Advance(time.Second) // the tween arrives and ends the exit
	app.ExpectCount(telatest.ByRole("status", ""), 0)
}
