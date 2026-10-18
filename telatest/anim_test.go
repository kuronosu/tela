package telatest_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Transitions and animations on the controllable clock (ARCHITECTURE
// sections 12 and 17): auto-wait settles finite ones and does not wait for
// infinite ones; Advance steps them.

func transitionButton() tela.Node {
	return Div(Class("p-4"),
		Button(Class("h-12 w-32 rounded-lg bg-[#ffffff] hover:bg-[#000000] transition-colors duration-300 ease-linear"), Text("Go")))
}

func TestAutoWaitSettlesTransitions(t *testing.T) {
	app := telatest.Mount(t, tela.C(transitionButton), telatest.Size(200, 100))
	before := app.Now()
	app.Hover(telatest.ByRole("button", "Go"))
	if d := app.Now() - before; d < 300*time.Millisecond {
		t.Errorf("Hover returned after %v of app time: the 300 ms transition was not waited for", d)
	}
	// The screenshot shows the settled state: the button is black.
	img := app.Screenshot()
	if px := img.RGBAAt(40, 40); px.R != 0 || px.G != 0 || px.B != 0 {
		t.Errorf("the hovered button is %v after the action returned", px)
	}
}

func TestAutoWaitDoesNotWaitForInfiniteAnimations(t *testing.T) {
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("flex items-center gap-4 p-4"),
			Div(Class("size-8 rounded-full border-4 border-gray-300 border-t-blue-600 animate-spin")),
			Button(Class("rounded bg-blue-500 px-3 py-1 text-white"), Text("Go")))
	}), telatest.Size(200, 100))
	before := app.Now()
	app.Click(telatest.ByRole("button", "Go"))
	app.Hover(telatest.ByRole("button", "Go"))
	if d := app.Now() - before; d > 500*time.Millisecond {
		t.Errorf("actions took %v of app time: an infinite animation was waited for", d)
	}
	// Advance steps the animation: the spinner is drawn at another angle.
	a := app.Screenshot()
	app.Advance(125 * time.Millisecond)
	b := app.Screenshot()
	if bytes.Equal(a.Pix, b.Pix) {
		t.Error("the spinner did not move in 125 ms of app time")
	}
	app.Advance(time.Second - 125*time.Millisecond) // a whole turn
	c := app.Screenshot()
	if d := app.Now() - before; d < time.Second {
		t.Fatalf("clock at %v", d)
	}
	_ = c
}

func TestReducedMotionOption(t *testing.T) {
	app := telatest.Mount(t, tela.C(transitionButton), telatest.Size(200, 100), telatest.ReducedMotion())
	before := app.Now()
	app.Hover(telatest.ByRole("button", "Go"))
	if d := app.Now() - before; d > 100*time.Millisecond {
		t.Errorf("with reduced motion Hover took %v: the transition was not instant", d)
	}
	if px := app.Screenshot().RGBAAt(40, 40); px.R != 0 {
		t.Errorf("button %v", px)
	}
}
