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
type Progress struct {
	// Value is how far the task is, from 0 to Max.
	Value float64
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
	pb := aria.UseProgressBar(aria.ProgressBarOptions{Value: p.Value, Max: p.Max, Indeterminate: p.Indeterminate,
		FormatValue: p.FormatValue, AriaLabel: p.AriaLabel})
	// The bar is as wide as the track and is moved left by what is not done.
	left := 100 - pb.Percent
	if p.Indeterminate {
		left = 0
	}
	left = math.Round(left*100) / 100
	return h.Div(cls(s.Root, p.Class), pb.Props, p.Props,
		h.Div(cls(s.Indicator, when(p.Indeterminate, s.Indeterminate), fmt.Sprintf("-translate-x-[%g%%] rtl:translate-x-[%g%%]", left, left), p.IndicatorClass)))
}
