package telatest_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func TestOptionsReachTheTree(t *testing.T) {
	list := func() tela.Node {
		return Div(Class("h-[100px] w-[200px] overflow-y-auto"),
			Div(Class("h-[400px] bg-sky-500"), Text("Tall")))
	}
	width := func(opts ...telatest.Option) float32 {
		app := telatest.Mount(t, tela.C(list), append([]telatest.Option{telatest.Size(300, 200)}, opts...)...)
		_, _, w, _ := app.Bounds(telatest.ByText("Tall"))
		return w
	}
	if w := width(); w != 200 {
		t.Errorf("with overlay scrollbars the content is %v px wide, want 200", w)
	}
	if w := width(telatest.ClassicScrollbars()); w != 185 {
		t.Errorf("with ClassicScrollbars the content is %v px wide, want 185 (a 15 px gutter)", w)
	}

	// ReducedMotion makes the motion-reduce: variants apply.
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("size-10 bg-white motion-reduce:bg-black"))
	}), telatest.Size(40, 40), telatest.ReducedMotion())
	if p := app.Screenshot().RGBAAt(5, 5); p.R != 0 {
		t.Errorf("pixel %v: motion-reduce: did not apply", p)
	}

	// RootClass styles the root, which fills the window.
	app = telatest.Mount(t, tela.C(func() tela.Node { return Text("x") }),
		telatest.Size(40, 40), telatest.RootClass("bg-black"))
	if p := app.Screenshot().RGBAAt(30, 30); p.R != 0 || p.A != 255 {
		t.Errorf("pixel %v: RootClass did not paint the window", p)
	}

	// InstantScroll jumps; without it a key scrolls over 150 ms.
	scrollTime := func(opts ...telatest.Option) time.Duration {
		app := telatest.Mount(t, tela.C(list), append([]telatest.Option{telatest.Size(300, 200)}, opts...)...)
		before := app.Now()
		app.Press("End")
		return app.Now() - before
	}
	if d := scrollTime(); d < 100*time.Millisecond {
		t.Errorf("an animated scroll took %v of app time, want about 150 ms", d)
	}
	if d := scrollTime(telatest.InstantScroll()); d >= 100*time.Millisecond {
		t.Errorf("an instant scroll took %v of app time", d)
	}
}
