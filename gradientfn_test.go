package tela_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// gradientBanner is a card whose gradient turns with a Value read in a
// BackgroundGradientFn (ADR-250): an animation of the angle repaints the
// card, and nothing builds or lays out.
type gradientBanner struct {
	builds *int
	angle  **tela.Value[float32]
}

func (b gradientBanner) Build() tela.Node {
	*b.builds++
	angle := tela.UseValue[float32](90)
	*b.angle = angle
	from, via, to := tela.Hex("#6366f1"), tela.Hex("#ec4899"), tela.Hex("#f59e0b")
	return Div(Class("flex h-full items-center justify-center bg-slate-100"),
		Div(TestID("banner"), Class("flex h-28 w-64 items-center justify-center rounded-2xl bg-slate-900 text-lg font-semibold text-white shadow-lg"),
			Style(tela.BackgroundGradientFn(func() tela.Gradient {
				return tela.LinearGradient(angle.Get(), tela.Stop(from), tela.StopAt(via, 50), tela.Stop(to))
			})),
			Text("Gradient")))
}

// TestBackgroundGradientFnFrames: the angle animates from 90 to 270
// degrees; each golden is the gradient at a known time, every frame is
// checked against a full rendering (damage), and the frames neither build
// nor lay out.
func TestBackgroundGradientFnFrames(t *testing.T) {
	builds := 0
	var angle *tela.Value[float32]
	app := telatest.Mount(t, gradientBanner{builds: &builds, angle: &angle},
		telatest.Size(320, 180), telatest.NoSettle(), telatest.CheckDamage())
	app.Golden("gradientfn_90.png")
	b0, s0 := builds, app.Stats()
	tela.Animate(angle, 270, tela.Duration(600*time.Millisecond), tela.Ease(tela.EaseLinear))
	app.Advance(300 * time.Millisecond)
	app.Golden("gradientfn_180.png")
	app.Advance(400 * time.Millisecond)
	app.Golden("gradientfn_270.png")
	s1 := app.Stats()
	if builds != b0 || s1.Builds != s0.Builds || s1.Styles != s0.Styles || s1.Layouts != s0.Layouts {
		t.Errorf("the animation: %d builds of the banner, %d builds, %d style computations, %d layouts; want none",
			builds-b0, s1.Builds-s0.Builds, s1.Styles-s0.Styles, s1.Layouts-s0.Layouts)
	}
	if s1.Paints-s0.Paints < 30 {
		t.Errorf("%d paints for 600 ms of animation", s1.Paints-s0.Paints)
	}
	// At 270 degrees the gradient runs right to left: the left end is
	// amber, the right end indigo.
	card := app.Bounds(telatest.ByTestID("banner"))
	left, right := app.PixelAt(card.X+8, card.Y+card.Height/2), app.PixelAt(card.Right()-8, card.Y+card.Height/2)
	if !(left.R > left.B && right.B > right.R) {
		t.Errorf("at 270 degrees the left end is %v and the right end %v, want amber then indigo", left, right)
	}
}
