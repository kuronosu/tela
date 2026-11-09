package aria_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// ValueFn (ADR-256): UseProgressBar and UseMeter read the value when the
// bar is painted and when the semantics tree is built, never in the build.

func TestProgressBarValueFn(t *testing.T) {
	builds := 0
	var done *tela.Value[float64]
	var p aria.ProgressBarResult
	page := func() tela.Node {
		builds++
		done = tela.UseValue(40.0)
		p = aria.UseProgressBar(aria.ProgressBarOptions{ValueFn: done.Get, Min: 20, Max: 70, AriaLabel: "Upload",
			FormatValue: func(v float64) string { return fmt.Sprintf("%g of 70", v) }})
		return Div(p.Props, Class("h-2 w-[200px] bg-muted"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	bar := telatest.ByRole("progressbar", "Upload")
	app.Expect(bar, telatest.HasValue("40 of 70"))
	if p.PercentFn == nil || p.ValueTextFn == nil {
		t.Fatal("no PercentFn or ValueTextFn with a ValueFn")
	}
	if p.Percent != 0 || p.ValueText != "" {
		t.Errorf("Percent %g and ValueText %q are build-time values: want none with a ValueFn", p.Percent, p.ValueText)
	}
	if got := p.PercentFn(); got != 40 {
		t.Errorf("PercentFn %g at 40 of 20..70, want 40", got)
	}
	b0 := builds
	done.Set(45)
	app.Expect(bar, telatest.HasValue("45 of 70"))
	done.Set(500) // clamped
	app.Expect(bar, telatest.HasValue("70 of 70"))
	done.Set(math.NaN()) // Min
	app.Expect(bar, telatest.HasValue("20 of 70"))
	if p.PercentFn() != 0 {
		t.Errorf("PercentFn %g for NaN, want 0", p.PercentFn())
	}
	if builds != b0 {
		t.Errorf("the value changed %d times and the page built %d more times, want none", 3, builds-b0)
	}
}

func TestProgressBarValueFnDefaultText(t *testing.T) {
	var done *tela.Value[float64]
	page := func() tela.Node {
		done = tela.UseValue(0.0)
		p := aria.UseProgressBar(aria.ProgressBarOptions{ValueFn: done.Get, AriaLabel: "x"})
		return Div(p.Props)
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	done.Set(66.6)
	app.Expect(telatest.ByRole("progressbar", "x"), telatest.HasValue("67%"))
}

// Indeterminate wins over a ValueFn: no value at all.
func TestProgressBarValueFnIndeterminate(t *testing.T) {
	var p aria.ProgressBarResult
	page := func() tela.Node {
		p = aria.UseProgressBar(aria.ProgressBarOptions{ValueFn: func() float64 { return 40 }, Indeterminate: true, AriaLabel: "x"})
		return Div(p.Props)
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	app.Expect(telatest.ByRole("progressbar", "x"), telatest.HasValue(""))
	if p.PercentFn != nil || p.ValueTextFn != nil {
		t.Error("an indeterminate bar has a PercentFn")
	}
}

func TestMeterValueFn(t *testing.T) {
	builds := 0
	var done *tela.Value[float64]
	var m aria.ProgressBarResult
	page := func() tela.Node {
		builds++
		done = tela.UseValue(128.0)
		m = aria.UseMeter(aria.MeterOptions{ValueFn: done.Get, Max: 512, AriaLabel: "Disk usage"})
		return Div(m.Props)
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	meter := telatest.ByRole("meter", "Disk usage")
	app.Expect(meter, telatest.HasValue("25%"))
	b0 := builds
	done.Set(256)
	app.Expect(meter, telatest.HasValue("50%"))
	if builds != b0 || m.PercentFn() != 50 {
		t.Errorf("%d builds after a change, PercentFn %g; want none and 50", builds-b0, m.PercentFn())
	}
}
