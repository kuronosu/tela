package kit_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// liveProgress is a Progress whose value a tela.Value holds (ADR-250);
// builds counts its builds and done is its Value.
type liveProgress struct {
	builds *int
	done   **tela.Value[float64]
	max    float64
	format func(float64) string
	dir    string
}

func (p liveProgress) Build() tela.Node {
	*p.builds++
	done := tela.UseValue(10.0)
	*p.done = done
	bar := kit.Progress{ValueFn: done.Get, Max: p.max, FormatValue: p.format, AriaLabel: "Download", Props: h.TestID("bar")}
	if p.dir != "" {
		return h.Div(h.Dir(p.dir), h.Class("w-[200px]"), bar)
	}
	return h.Div(h.Class("w-[200px]"), bar)
}

// shown returns how many pixels of the bar the indicator covers, from the
// start of the bar (its left, or its right under rtl).
func shown(app *telatest.App, rtl bool) float32 {
	bar := app.Bounds(telatest.ByTestID("bar"))
	ind := app.Bounds(telatest.ByClass("h-full", "flex-1"))
	if rtl {
		return bar.Right() - ind.X
	}
	return ind.Right() - bar.X
}

// TestProgressValueFn: a Progress with ValueFn moves when the Value
// changes, with no build and no layout of anything (a repaint), and
// its aria-valuenow and aria-valuetext follow the Value in the semantics
// tree, as assistive technologies read it.
func TestProgressValueFn(t *testing.T) {
	builds := 0
	var done *tela.Value[float64]
	app := telatest.Mount(t, liveProgress{builds: &builds, done: &done})
	bar := progressbar("Download")
	if got := shown(app, false); got < 19.9 || got > 20.1 {
		t.Errorf("at 10%% the indicator shows %g px of the bar, want 20", got)
	}
	app.Expect(bar, telatest.HasValue("10%"))
	s0, b0 := app.Stats(), builds
	for _, v := range []float64{25, 60, 75.4} {
		done.Set(v)
		app.Expect(bar, telatest.HasValue(fmt.Sprintf("%.0f%%", v)))
		if got, want := shown(app, false), float32(2*v); got < want-0.1 || got > want+0.1 {
			t.Errorf("at %g%% the indicator shows %g px of the bar, want %g", v, got, want)
		}
	}
	s1 := app.Stats()
	if builds != b0 || s1.Builds != s0.Builds || s1.Layouts != s0.Layouts || s1.Styles != s0.Styles {
		t.Errorf("three values: %d builds of the screen, %d builds, %d style computations, %d layouts; want none",
			builds-b0, s1.Builds-s0.Builds, s1.Styles-s0.Styles, s1.Layouts-s0.Layouts)
	}
	if s1.Paints == s0.Paints {
		t.Error("the bar never repainted")
	}
	// Out of range and NaN.
	done.Set(150)
	app.Expect(bar, telatest.HasValue("100%"))
	if got := shown(app, false); got < 199.9 {
		t.Errorf("past the end the indicator shows %g px, want the whole bar", got)
	}
	done.Set(-3)
	app.Expect(bar, telatest.HasValue("0%"))
}

// TestProgressValueFnFormatAndMax: FormatValue and Max apply to ValueFn,
// and under rtl the bar fills from the right.
func TestProgressValueFnFormatAndMax(t *testing.T) {
	builds := 0
	var done *tela.Value[float64]
	app := telatest.Mount(t, liveProgress{builds: &builds, done: &done, max: 40, dir: "rtl",
		format: func(v float64) string { return fmt.Sprintf("%.0f of 40 MB", v) }})
	bar := progressbar("Download")
	app.Expect(bar, telatest.HasValue("10 of 40 MB"))
	if got := shown(app, true); got < 49.9 || got > 50.1 {
		t.Errorf("rtl, 10 of 40: the indicator shows %g px from the right, want 50", got)
	}
	done.Set(30)
	app.Expect(bar, telatest.HasValue("30 of 40 MB"))
	if got := shown(app, true); got < 149.9 || got > 150.1 {
		t.Errorf("rtl, 30 of 40: the indicator shows %g px from the right, want 150", got)
	}
}

// TestProgressValueFnAnimated: a float32 Value that tela.Animate moves
// (what UseTween and UseSpring give) moves the bar frame by frame without
// building it.
func TestProgressValueFnAnimated(t *testing.T) {
	builds := 0
	var pct *tela.Value[float32]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		builds++
		pct = tela.UseValue[float32](10)
		return h.Div(h.Class("w-[200px]"),
			kit.Progress{ValueFn: func() float64 { return float64(pct.Get()) }, AriaLabel: "Upload", Props: h.TestID("bar")})
	}), telatest.NoSettle())
	b0 := builds
	s0 := app.Stats()
	tela.Animate(pct, 90, tela.Duration(400*time.Millisecond), tela.Ease(tela.EaseLinear))
	var last float32
	for i := range 4 {
		app.Advance(100 * time.Millisecond)
		got := shown(app, false)
		if got <= last {
			t.Errorf("step %d: the indicator shows %g px, not more than %g", i, got, last)
		}
		last = got
	}
	app.Advance(100 * time.Millisecond)
	if got := shown(app, false); got < 179.9 || got > 180.1 {
		t.Errorf("at the end the indicator shows %g px, want 180", got)
	}
	if s1 := app.Stats(); builds != b0 || s1.Builds != s0.Builds || s1.Layouts != s0.Layouts {
		t.Errorf("the animation built %d times and laid out %d times", s1.Builds-s0.Builds, s1.Layouts-s0.Layouts)
	}
	app.Expect(progressbar("Upload"), telatest.HasValue("90%"))
}

// A Progress may switch between Value and ValueFn (same hooks).
func TestProgressValueFnSwitch(t *testing.T) {
	var setLive tela.Setter[bool]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		live, set := tela.UseState(false)
		setLive = set
		p := kit.Progress{Value: 30, AriaLabel: "P"}
		if live {
			p.ValueFn = func() float64 { return 70 }
		}
		return h.Div(h.Class("w-[200px]"), p)
	}))
	app.Expect(progressbar("P"), telatest.HasValue("30%"))
	setLive.Set(true)
	app.Expect(progressbar("P"), telatest.HasValue("70%"))
	setLive.Set(false)
	app.Expect(progressbar("P"), telatest.HasValue("30%"))
}
