package tela_test

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleToggle() {
	t := &exampleT{}
	defer t.close()
	menu := func() tela.Node {
		open, setOpen := tela.UseState(false)
		return Div(
			Button(OnClick(tela.Toggle(setOpen)), Text("Menu")),
			If(open, P(Text("Settings"))))
	}
	app := telatest.Mount(t, tela.C(menu))
	app.Click(telatest.ByRole("button", "Menu"))
	fmt.Println(app.Count(telatest.ByText("Settings")))
	app.Click(telatest.ByRole("button", "Menu"))
	fmt.Println(app.Count(telatest.ByText("Settings")))
	// Output:
	// 1
	// 0
}

func ExampleMapValue() {
	t := &exampleT{}
	defer t.close()
	var open *tela.Value[float32]
	panel := func() tela.Node {
		open = tela.UseValue(float32(0))
		// 36 px closed, 156 px open: only the box is laid out again when
		// open changes, the component does not build.
		return Div(TestID("panel"), Class("w-40 overflow-hidden bg-slate-100"),
			Style(tela.HeightFn(tela.MapValue(open, func(t float32) float32 { return 36 + 120*t }))))
	}
	app := telatest.Mount(t, tela.C(panel))
	fmt.Println(app.Bounds(telatest.ByTestID("panel")).Height)
	tela.Animate(open, 1, tela.Duration(100*time.Millisecond))
	app.Advance(time.Second)
	fmt.Println(app.Bounds(telatest.ByTestID("panel")).Height)
	// Output:
	// 36
	// 156
}

func ExampleCubicBezier() {
	// Tailwind's ease-out utility, and a curve that overshoots.
	fmt.Println(tela.CubicBezier(0, 0, 0.2, 1))
	fmt.Println(tela.CubicBezier(0.34, 1.56, 0.64, 1))
	_ = tela.Ease(tela.CubicBezier(0, 0, 0.2, 1))
	// Output:
	// cubic-bezier(0, 0, 0.2, 1)
	// cubic-bezier(0.34, 1.56, 0.64, 1)
}

func ExampleEasing() {
	// The constants and string literals are both Easings; a variable
	// that holds a CSS easing converts with tela.Easing(s).
	opts := []tela.AnimationOption{
		tela.Ease(tela.EaseInOut),
		tela.Ease("ease-out"),
		tela.Ease(tela.Easing("steps(4, jump-end)")),
	}
	fmt.Println(len(opts), tela.EaseLinear, tela.EaseIn, tela.EaseOut, tela.EaseInOut)
	// Output: 3 linear ease-in ease-out ease-in-out
}

func ExampleUsePresenceTween() {
	t := &exampleT{}
	defer t.close()
	toast := func() tela.Node {
		shown := tela.UsePresenceTween(tela.Duration(200*time.Millisecond), tela.Ease(tela.EaseOut))
		return Div(Role("status"), Class("rounded-lg bg-slate-900 px-4 py-2 text-white"),
			Style(tela.TranslateFn(func() (float32, float32) { return 0, 24 * (1 - shown.Get()) }),
				tela.OpacityFn(shown.Get)),
			Text("Saved"))
	}
	page := func() tela.Node {
		saved, setSaved := tela.UseState(true)
		return Div(
			Button(OnClick(tela.Toggle(setSaved)), Text("Dismiss")),
			Presence(If(saved, tela.C(toast))))
	}
	app := telatest.Mount(t, tela.C(page))
	fmt.Println(app.Count(telatest.ByRole("status", "")))
	start := app.Now()
	app.Click(telatest.ByRole("button", "Dismiss")) // waits for the exit tween
	fmt.Println(app.Count(telatest.ByRole("status", "")), app.Now()-start >= 200*time.Millisecond)
	// Output:
	// 1
	// 0 true
}

func ExampleBox() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, Div(Class("relative h-40 w-60"),
		Div(TestID("marker"), Class("absolute rounded ring-2 ring-indigo-500"), tela.Box(20, 10, 80, 30))),
		telatest.Size(240, 160))
	fmt.Println(app.Bounds(telatest.ByTestID("marker")))
	// Output: {20 10 80 30}
}

func ExampleWH() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, Div(TestID("card"), Class("rounded-xl bg-white"), tela.WH(240, 160)),
		telatest.Size(300, 200))
	fmt.Println(app.Bounds(telatest.ByTestID("card")))
	// Output: {0 0 240 160}
}

func ExampleStyleProp() {
	t := &exampleT{}
	defer t.close()
	// A StyleProp is a child: no Style(...) around it, and If takes it.
	selected := true
	app := telatest.Mount(t, Div(TestID("swatch"), Class("size-8"),
		tela.Bg(tela.Hex("#336699")), If(selected, tela.Opacity(0.5))),
		telatest.Size(100, 100))
	fmt.Println(app.Bounds(telatest.ByTestID("swatch")).Width, app.PixelAt(16, 16))
	// Output: 32 {153 178 204 255}
}

type exampleRowProps struct {
	ID   int
	Name string
}

func exampleRow(p exampleRowProps) tela.Node {
	n, setN := tela.UseState(0)
	return Button(Textf("%s %d", p.Name, n), OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
}

func ExampleCP_key() {
	t := &exampleT{}
	defer t.close()
	var set tela.Setter[[]exampleRowProps]
	list := func() tela.Node {
		rows, s := tela.UseState([]exampleRowProps{{1, "Ada"}, {2, "Grace"}})
		set = s
		nodes := make([]tela.Node, len(rows))
		for i, r := range rows {
			// The key goes last: it is not a prop of the row.
			nodes[i] = tela.CP(exampleRow, r, Key(r.ID))
		}
		return Div(Nodes(nodes))
	}
	app := telatest.Mount(t, tela.C(list))
	app.Click(telatest.ByText("Grace 0")) // Grace has been clicked once
	set.Set([]exampleRowProps{{2, "Grace"}, {1, "Ada"}})
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Grace 1"
	// - button "Ada 0"
}
