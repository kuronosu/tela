package telatest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/telatest"
)

// Clock counts clicks in a TextFn: the label changes, no component builds.
func Clock() tela.Node {
	n := tela.UseValue(0)
	return Div(Class("flex flex-col items-start gap-2 p-4"),
		Button(Class("rounded bg-slate-200 px-2"), OnClick(func(tela.MouseEvent) {
			n.Update(func(v int) int { return v + 1 })
		}), Text("Tick")),
		P(Class("tabular-nums"), Text("Count: "), TextFn(func() string { return fmt.Sprint(n.Get()) })),
	)
}

func TestTextFnDoesNotBuild(t *testing.T) {
	app := telatest.Mount(t, tela.C(Clock), telatest.Size(300, 200))
	app.ExpectText("Count: 0")
	before := app.Stats()
	for range 3 {
		app.Click(telatest.ByRole("button", "Tick"))
	}
	app.ExpectText("Count: 3")
	if d := app.Stats().Builds - before.Builds; d != 0 {
		t.Errorf("%d component builds for 3 clicks, want none", d)
	}
}

// Meter sizes its fill from the bounds of its track, read in a deferred
// width: no build when the track changes size.
func Meter() tela.Node {
	wide, setWide := tela.UseState(true)
	track := tela.UseElementRef()
	b := tela.UseBounds(track)
	class := "h-4 w-1/2 bg-slate-200"
	if wide {
		class = "h-4 w-full bg-slate-200"
	}
	return Div(Class("w-[400px] p-4"),
		Div(Ref(track), Class(class),
			Div(Role("progressbar"), AriaLabel("fill"), Class("h-4 bg-indigo-500"),
				Style(tela.WidthFn(func() float32 { return b.Get().Width * 0.25 })))),
		Button(OnClick(func(tela.MouseEvent) { setWide.Set(!wide) }), Text("Toggle")))
}

func TestUseBoundsSizesFromAnotherElement(t *testing.T) {
	app := telatest.Mount(t, tela.C(Meter), telatest.Size(500, 200))
	w := app.Bounds(telatest.ByRole("progressbar", "fill")).Width
	if w != 92 { // (400 - 32) / 4
		t.Errorf("fill width %g, want 92", w)
	}
	app.Click(telatest.ByRole("button", "Toggle"))
	w = app.Bounds(telatest.ByRole("progressbar", "fill")).Width
	if w != 46 {
		t.Errorf("fill width %g after the track halved, want 46", w)
	}
	// Settled: nothing keeps running.
	s0 := app.Stats()
	app.Advance(100e6)
	if s1 := app.Stats(); s1.Frames != s0.Frames {
		t.Errorf("%d frames while idle", s1.Frames-s0.Frames)
	}
}

func TestLayoutJSONBounds(t *testing.T) {
	root := Div(Class("p-4"),
		Div(Class("h-10 w-20 bg-sky-500")),
		Div(Class("h-10 w-20 scale-50 bg-sky-500")),
		Div(Class("h-10 w-20 bg-sky-500"), Style(tela.Translate(10, 5))),
	)
	js, err := telatest.LayoutJSON(root, telatest.Size(200, 160))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(js), `"bounds"`); n != 2 {
		t.Errorf("%d bounds objects, want 2 (the scaled and the moved box)", n)
	}
	golden.Text(t, "layout_bounds.json", string(js))
}
