// Portions of this file follow React Aria's useProgressBar and useMeter
// (https://github.com/adobe/react-spectrum, packages/@react-aria/progress
// and packages/@react-aria/meter), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"math"
	"strconv"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// ProgressBarOptions configures [UseProgressBar].
type ProgressBarOptions struct {
	// Value is the progress, from Min to Max. It is ignored while
	// Indeterminate.
	Value float64
	// Min and Max are the ends of the scale. Min is 0 by default and Max
	// 100 when both are 0.
	Min, Max float64
	// Indeterminate says that the amount of progress is unknown: the bar
	// has no aria-valuenow, and Percent is 0.
	Indeterminate bool
	// FormatValue returns Value as people read it, for aria-valuetext; the
	// default is the percentage, rounded ("40%"). Use package i18n for a
	// locale's formatting.
	FormatValue func(float64) string
	// AriaLabel names the bar; else it is named by the element with
	// LabelProps, or by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the bar, when it is not
	// the one with LabelProps.
	AriaLabelledBy tela.ElementRef
}

// MeterOptions configures [UseMeter]: a measure within a known range, such
// as disk usage, as opposed to the progress of a task. The fields are those
// of [ProgressBarOptions] without Indeterminate.
type MeterOptions struct {
	// Value is the measure, from Min to Max.
	Value float64
	// Min and Max are the ends of the scale. Min is 0 by default and Max
	// 100 when both are 0.
	Min, Max float64
	// FormatValue returns Value as people read it, for aria-valuetext; the
	// default is the percentage, rounded ("40%").
	FormatValue func(float64) string
	// AriaLabel names the meter; else it is named by the element with
	// LabelProps, or by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the meter, when it is not
	// the one with LabelProps.
	AriaLabelledBy tela.ElementRef
}

// ProgressBarResult is what [UseProgressBar] and [UseMeter] return.
type ProgressBarResult struct {
	// Props go to the bar's element: its role ("progressbar" or "meter"),
	// aria-valuenow, aria-valuemin, aria-valuemax, aria-valuetext and its
	// name.
	Props tela.Prop
	// LabelProps go to the element that labels the bar.
	LabelProps tela.Prop
	// Percent is the value as a percentage of the range, from 0 to 100
	// (React Aria's percentage is a fraction from 0 to 1): the width of the
	// fill, as Class(fmt.Sprintf("w-[%g%%]", p.Percent)) or a tela.WidthFn
	// of the bar's size. It is 0 for an indeterminate bar.
	Percent float64
	// ValueText is the value as people read it (aria-valuetext); empty
	// for an indeterminate bar.
	ValueText string
}

// UseProgressBar returns the props of a progress bar (React Aria's
// useProgressBar): the "progressbar" role with its value, range and text,
// and its name. A bar with Indeterminate has no value. The caller renders
// the bar and its fill:
//
//	p := aria.UseProgressBar(aria.ProgressBarOptions{Value: done, AriaLabel: "Upload"})
//	return Div(p.Props, Class("h-2 w-64 overflow-hidden rounded-full bg-muted"),
//	    Div(Class("h-full bg-primary transition-all"), Class(fmt.Sprintf("w-[%g%%]", p.Percent))))
func UseProgressBar(o ProgressBarOptions) ProgressBarResult {
	return useRange("UseProgressBar", "progressbar", o.Value, o.Min, o.Max, o.Indeterminate, o.FormatValue, o.AriaLabel, o.AriaLabelledBy)
}

// UseMeter returns the props of a meter (React Aria's useMeter): the
// "meter" role with its value, range and text, and its name.
//
//	m := aria.UseMeter(aria.MeterOptions{Value: used, Max: 512, AriaLabel: "Disk usage"})
//	return Div(m.Props, Class("h-2 w-64 rounded-full bg-muted"),
//	    Div(Class("h-full bg-primary"), Class(fmt.Sprintf("w-[%g%%]", m.Percent))))
func UseMeter(o MeterOptions) ProgressBarResult {
	return useRange("UseMeter", "meter", o.Value, o.Min, o.Max, false, o.FormatValue, o.AriaLabel, o.AriaLabelledBy)
}

func useRange(hook, role string, value, lo, hi float64, indeterminate bool, format func(float64) string, label string, labelledBy tela.ElementRef) ProgressBarResult {
	labelRef := tela.UseElementRef()
	if lo == 0 && hi == 0 {
		hi = 100
	}
	if hi <= lo && core.DevNow() {
		panic(fmt.Sprintf("aria: %s: Max (%g) must be greater than Min (%g)", hook, hi, lo))
	}
	value = min(max(value, lo), hi)
	name := h.AriaLabelledBy(labelRef)
	switch {
	case label != "":
		name = h.AriaLabel(label)
	case labelledBy != (tela.ElementRef{}):
		name = h.AriaLabelledBy(labelledBy)
	}
	ps := []tela.Prop{h.Role(role), name, h.AriaValueMin(lo), h.AriaValueMax(hi)}
	r := ProgressBarResult{LabelProps: h.Ref(labelRef)}
	if !indeterminate {
		r.Percent = (value - lo) / (hi - lo) * 100
		r.ValueText = percentText(r.Percent)
		if format != nil {
			r.ValueText = format(value)
		}
		ps = append(ps, h.AriaValueNow(value), h.AriaValueText(r.ValueText))
	}
	r.Props = core.MergeProps(ps...)
	return r
}

// percentText is a percentage rounded to a whole number: "40%".
func percentText(p float64) string {
	return strconv.FormatFloat(math.Floor(p+0.5), 'f', -1, 64) + "%"
}
