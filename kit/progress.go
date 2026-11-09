// Portions of this file follow shadcn/ui, progress.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"fmt"
	"math"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// ProgressStyle is the recipe of [Progress] (shadcn/ui's Progress).
type ProgressStyle struct {
	// Root is the track, which clips the indicator.
	Root string
	// Indicator is the bar; it is as wide as the track and moves toward the
	// start (left, or right under a right-to-left direction, with the
	// rtl: class the component adds) by the part that is not done.
	Indicator string
	// Indeterminate is added to the indicator while the amount of progress
	// is unknown (a pulsing, full bar; shadcn draws an empty one).
	Indeterminate string
}

func init() {
	defaultStyle.Progress = ProgressStyle{
		Root:          "relative h-2 w-full overflow-hidden rounded-full bg-primary/20",
		Indicator:     "h-full w-full flex-1 bg-primary transition-all",
		Indeterminate: "animate-pulse",
	}
}

// Progress is a bar that shows how far a task is, shadcn/ui's Progress, on
// aria.UseProgressBar: it has the progressbar role, its range and
// aria-valuenow, and the bar moves with a transition when Value changes.
//
//	kit.Progress{Value: uploaded, AriaLabel: "Upload"} // 0 to 100
//
//	kit.Progress{Value: 512, Max: 2048, FormatValue: func(v float64) string { return fmt.Sprintf("%.0f MB", v) }}
//
// Use Indeterminate while the amount is unknown. For a measure that is not
// the progress of a task, such as disk usage, use aria.UseMeter.
//
// A value that changes many times a second (a download, a tween) goes in
// ValueFn instead, which is read when the bar is painted and when the
// semantics tree is built: the bar moves and aria-valuenow follows without
// building the component (ADR-250).
//
//	done := tela.UseValue(0.0) // the download's goroutine calls done.Set(percent)
//	kit.Progress{ValueFn: done.Get, AriaLabel: "Download"}
//
//	pct := tela.UseTween(target) // a float32 that eases toward target
//	kit.Progress{ValueFn: func() float64 { return float64(pct.Get()) }, AriaLabel: "Upload"}
type Progress struct {
	// Value is how far the task is, from 0 to Max.
	Value float64
	// ValueFn, when not nil, is read in place of Value: by a paint-phase
	// closure that slides the bar (tela.TranslateFn), and when the
	// semantics tree is built (h.AriaValueNowFn, h.AriaValueTextFn, with
	// FormatValue). A change of a tela.Value it reads repaints the bar and
	// makes the semantics stale, and builds nothing. The bar moves at once,
	// without the transition of the recipe: read an animated float32 Value
	// (tela.UseTween, tela.Animate) for a smooth bar. NaN is 0, and the
	// value is clamped to the range as Value is.
	ValueFn func() float64
	// Max is the value of a finished task (100 by default).
	Max float64
	// Indeterminate says that the amount of progress is unknown: the bar
	// has no value and pulses.
	Indeterminate bool
	// FormatValue returns Value as people read it (aria-valuetext); the
	// default is the percentage, "40%".
	FormatValue func(float64) string
	// AriaLabel names the bar.
	AriaLabel string
	// Class is added after the recipe's classes; IndicatorClass does the
	// same for the bar.
	Class          string
	IndicatorClass string
	// Props are more props for the track element.
	Props tela.Child
}

// Build returns the progress element.
func (p Progress) Build() tela.Node {
	s := &UseStyle().Progress
	live := p.ValueFn != nil && !p.Indeterminate
	opts := aria.ProgressBarOptions{Value: p.Value, Max: p.Max, Indeterminate: p.Indeterminate,
		FormatValue: p.FormatValue, AriaLabel: p.AriaLabel}
	if live {
		opts.ValueFn = p.ValueFn
	}
	pb := aria.UseProgressBar(opts)
	// The same hooks with and without ValueFn, so a bar may switch.
	bar := tela.UseElementRef()
	size := tela.UseSize(bar)
	dir := tela.UseDirection()
	if live {
		return p.live(s, pb, bar, size, dir)
	}
	// The bar is as wide as the track and is moved left by what is not done.
	left := 100 - pb.Percent
	if p.Indeterminate {
		left = 0
	}
	left = math.Round(left*100) / 100
	return h.Div(cls(s.Root, p.Class), pb.Props, p.Props,
		h.Div(cls(s.Indicator, when(p.Indeterminate, s.Indeterminate), fmt.Sprintf("-translate-x-[%g%%] rtl:translate-x-[%g%%]", left, left), p.IndicatorClass)))
}

// live is the bar of a Progress with ValueFn: the indicator is slid by a
// tela.TranslateFn of its own width (what a percentage of translate-x is
// of), and the value reaches the semantics through the hook's
// h.AriaValueNowFn and h.AriaValueTextFn, so a change of the Values ValueFn
// reads repaints and builds nothing.
func (p Progress) live(s *ProgressStyle, pb aria.ProgressBarResult, bar tela.ElementRef, size *tela.Value[tela.Size], dir string) tela.Node {
	sign := float32(-1) // toward the start: left, or right under rtl
	if dir == "rtl" {
		sign = 1
	}
	return h.Div(cls(s.Root, p.Class), pb.Props, p.Props,
		h.Div(h.Ref(bar), cls(s.Indicator, p.IndicatorClass), h.Style(tela.TranslateFn(func() (x, y float32) {
			return sign * float32((100-pb.PercentFn())/100) * size.Get().Width, 0
		}))))
}
