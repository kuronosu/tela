package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

func TestProgressBar(t *testing.T) {
	var p aria.ProgressBarResult
	var set tela.Setter[float64]
	page := func() tela.Node {
		v, s := tela.UseState(40.0)
		set = s
		p = aria.UseProgressBar(aria.ProgressBarOptions{Value: v, AriaLabel: "Upload"})
		return Div(p.Props, Class("h-2 w-[200px] bg-muted"), Div(Class("h-full bg-primary"), Style(tela.WidthFn(func() float32 { return float32(p.Percent) * 2 }))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	bar := telatest.ByRole("progressbar", "Upload")
	app.Expect(bar, telatest.HasValue("40%"))
	app.ExpectAria(`- progressbar "Upload"`)
	if p.Percent != 40 || p.ValueText != "40%" {
		t.Errorf("Percent %g, ValueText %q", p.Percent, p.ValueText)
	}
	set.Set(66.6)
	app.Expect(bar, telatest.HasValue("67%"))
	set.Set(250) // clamped
	app.Expect(bar, telatest.HasValue("100%"))
}

func TestProgressBarRangeAndFormat(t *testing.T) {
	var p aria.ProgressBarResult
	page := func() tela.Node {
		p = aria.UseProgressBar(aria.ProgressBarOptions{
			Value: 30, Min: 20, Max: 70, AriaLabel: "Download",
			FormatValue: func(v float64) string { return fmt.Sprintf("%g of 70 MB", v) },
		})
		return Div(p.Props)
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	app.Expect(telatest.ByRole("progressbar", "Download"), telatest.HasValue("30 of 70 MB"))
	if p.Percent != 20 {
		t.Errorf("Percent %g, want 20", p.Percent)
	}
}

// An indeterminate bar has no value.
func TestProgressBarIndeterminate(t *testing.T) {
	var p aria.ProgressBarResult
	page := func() tela.Node {
		p = aria.UseProgressBar(aria.ProgressBarOptions{Value: 40, Indeterminate: true, AriaLabel: "Loading"})
		return Div(p.Props)
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	app.Expect(telatest.ByRole("progressbar", "Loading"), telatest.HasValue(""))
	if p.Percent != 0 || p.ValueText != "" {
		t.Errorf("Percent %g, ValueText %q, want none", p.Percent, p.ValueText)
	}
	// No aria-valuenow, but the range is there.
	a := mountRaw(t, tela.C(page), 100, 100)
	n := rawwin.FindNode(a.tree.Semantics(), "progressbar", "Loading")
	if n == nil || n.Range.HasNow || !n.Range.HasMin || !n.Range.HasMax {
		t.Errorf("range of an indeterminate bar: %+v", n)
	}
}

func TestProgressBarDeterminateRange(t *testing.T) {
	a := mountRaw(t, tela.C(func() tela.Node {
		return Div(aria.UseProgressBar(aria.ProgressBarOptions{Value: 30, Min: 20, Max: 70, AriaLabel: "Download"}).Props)
	}), 100, 100)
	n := rawwin.FindNode(a.tree.Semantics(), "progressbar", "Download")
	if n == nil || !n.Range.HasNow || n.Range.Now != 30 || n.Range.Min != 20 || n.Range.Max != 70 {
		t.Errorf("range of a bar: %+v", n)
	}
}

func TestMeter(t *testing.T) {
	var m aria.ProgressBarResult
	page := func() tela.Node {
		m = aria.UseMeter(aria.MeterOptions{Value: 128, Max: 512, AriaLabel: "Disk usage"})
		return Div(m.Props)
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	app.ExpectAria(`- meter "Disk usage"`)
	app.Expect(telatest.ByRole("meter", "Disk usage"), telatest.HasValue("25%"))
	if m.Percent != 25 {
		t.Errorf("Percent %g, want 25", m.Percent)
	}
}

// The label element names the bar.
func TestProgressBarLabel(t *testing.T) {
	page := func() tela.Node {
		p := aria.UseProgressBar(aria.ProgressBarOptions{Value: 10})
		return Div(Span(p.LabelProps, Text("Syncing")), Div(p.Props))
	}
	app := telatest.MountFunc(t, page, telatest.Size(100, 100))
	app.ExpectRole("progressbar", "Syncing")
}

func TestProgressBarOptionErrors(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "Max (5) must be greater than Min (10)") {
			t.Errorf("message %q", msg)
		}
	}()
	telatest.MountFunc(t, func() tela.Node {
		return Div(aria.UseMeter(aria.MeterOptions{Min: 10, Max: 5}).Props)
	}, telatest.Size(100, 100))
}
