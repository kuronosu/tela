// Portions of this file follow React Aria's useSlider and useSliderThumb
// and react-stately's useSliderState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/slider
// and packages/@react-stately/slider), Copyright 2020 Adobe, licensed
// under the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// SliderOptions configures [UseSlider]: one thumb, or several (a range
// slider has two) on a track from Min to Max.
//
// The values are uncontrolled when Values is nil (the hook keeps them,
// starting at DefaultValues) and controlled when Values points to the
// caller's state: the slider shows it and only asks OnValuesChange for the
// next one, so a change the caller does not store is refused.
// OnValuesChange is also called for an uncontrolled slider. The number of
// thumbs is the length of the values (one at Min when DefaultValues is
// empty), and the values are sorted by the thumbs: a thumb stops at its
// neighbor.
//
//	// A range slider that keeps its values.
//	aria.SliderOptions{DefaultValues: []float64{20, 80}, Max: 100, Step: 5}
//	// A controlled single slider.
//	vol, setVol := tela.UseState([]float64{30})
//	aria.SliderOptions{Values: &vol, OnValuesChange: setVol.Set}
//
// A slider with one thumb is a slice of one value, as in Radix and
// shadcn/ui; [SliderResult.Values] has it at index 0.
type SliderOptions struct {
	// Values points to the values of a controlled slider, one per thumb;
	// nil leaves them to the hook.
	Values *[]float64
	// LiveValues, for a controlled slider whose values change between
	// builds without building it (a seek bar a clock moves), returns the
	// values as they are now. The slider reads it at the start of a press
	// on the track or on a thumb and of a key, in place of the Values of
	// its last build, so a change it asks for is compared with what the
	// caller really has. It is called on the UI thread, in handlers, and
	// not from paint-phase closures: they place the thumbs from what the
	// caller reads itself (see [SliderResult.ThumbPercent]). Nil reads
	// only the Values of each build.
	LiveValues func() []float64
	// DefaultValues are the values of an uncontrolled slider at the start.
	DefaultValues []float64
	// OnValuesChange is called with all the values (a copy) every time the
	// user moves a thumb to a new value: while dragging, and on each key.
	OnValuesChange func([]float64)
	// OnChangeEnd is called with the values (a copy) when the user stops
	// changing them: the pointer is released, or a key changed a value.
	OnChangeEnd func([]float64)
	// Min and Max are the ends of the track. Min is 0 by default and Max
	// 100 when both are 0.
	Min, Max float64
	// Step is the distance between the values the thumbs can take, from
	// Min; 1 by default. Values are rounded to it.
	Step float64
	// PageSize is how far PageUp, PageDown and Shift with an arrow move a
	// thumb, rounded to Step; a tenth of the range by default.
	PageSize float64
	// MinStepsBetweenThumbs is the least number of Steps between two
	// thumbs; 0 lets them touch.
	MinStepsBetweenThumbs int
	// Orientation is "horizontal" (the default; Min is at the left, or at
	// the right with a "rtl" Direction) or "vertical" (Min at the bottom).
	Orientation string
	// Direction is "ltr" (the default) or "rtl", which mirrors a
	// horizontal slider and its arrow keys.
	Direction string
	// Disabled makes the thumbs inert and disabled, and the track ignores
	// presses.
	Disabled bool
	// FormatValue returns a value as people read it, for aria-valuetext
	// and [SliderResult.ValueText]; the default is the number as it is
	// written (3, 0.25). Use package i18n for a locale's digits and
	// units.
	FormatValue func(float64) string
	// AriaLabel names the slider; else it is named by the element with
	// LabelProps, or by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the slider, when it is not
	// the one with LabelProps.
	AriaLabelledBy tela.ElementRef
	// ThumbLabels name each thumb (aria-label), for a range slider whose
	// thumbs are "Minimum price" and "Maximum price"; a thumb without one
	// takes the slider's name.
	ThumbLabels []string
}

// SliderResult is what [UseSlider] returns. It is the result of one build:
// call its methods in the build or in handlers; ThumbPercent is also made
// to be called in the paint-phase closures of the thumbs.
type SliderResult struct {
	// Props go to the slider's root element: the "group" role and its
	// name.
	Props tela.Prop
	// TrackProps go to the track: a press on it moves the nearest thumb to
	// the point and starts dragging it, with a mouse, a finger or a pen.
	TrackProps tela.Prop
	// LabelProps go to the element that labels the slider (a span or a
	// Label): its text names the slider and its thumbs, unless AriaLabel
	// or ThumbLabels do.
	LabelProps tela.Prop
	// Values are the values of the thumbs as of this build. While a thumb
	// is dragged in an uncontrolled slider they are brought up to date at
	// the end of the drag; read ThumbPercent and ValueText for the live
	// ones.
	Values []float64
	// Dragging is whether a thumb is being dragged, as of this build.
	Dragging bool
	// TrackRef is the ref that TrackProps attaches to the track.
	TrackRef tela.ElementRef

	s *sliderState
}

// ThumbProps returns the props of thumb i (React Aria's useSliderThumb):
// the "slider" role, aria-valuenow, aria-valuemin and aria-valuemax (the
// limits of the thumb: its neighbors), aria-valuetext, aria-orientation,
// its name, the ref, a tab stop, and the handlers: a press on the thumb
// drags it (the pointer keeps it while it is down, wherever it goes), the
// arrow keys move it by one Step (by a page with Shift), PageUp and
// PageDown by a page (a tenth of the range, rounded to Step), Home and End
// to its limits. Right-to-left Direction mirrors the horizontal arrows.
func (r SliderResult) ThumbProps(i int) tela.Prop { return r.s.thumbProps(i) }

// ThumbRef returns the ref that ThumbProps(i) attaches to thumb i.
func (r SliderResult) ThumbRef(i int) tela.ElementRef { return r.s.thumbRef(i) }

// ThumbPercent returns the position of thumb i along the track, from 0
// (Min) to 100 (Max), as it is now. It subscribes what reads it: read it
// in a paint-phase closure (tela.TranslateFn, tela.WidthFn) to place the
// thumb while it is dragged without building the component, as
//
//	Style(tela.TranslateFn(func() (x, y float32) {
//	    return float32(s.ThumbPercent(i)) / 100 * track.Get().Width, 0
//	}))
//
// where track is tela.UseSize(s.TrackRef). Read in the component's body it
// makes the component build on every move.
func (r SliderResult) ThumbPercent(i int) float64 { return r.s.thumbPercent(i) }

// ValueText returns the value of thumb i as people read it: FormatValue of
// the live value.
func (r SliderResult) ValueText(i int) string { return r.s.valueText(i) }

// IsThumbDragging reports whether thumb i is being dragged now.
func (r SliderResult) IsThumbDragging(i int) bool { return r.s.drag.active && r.s.drag.thumb == i }

// UseSlider holds the state of a slider and returns the props of its
// root, track and thumbs (React Aria's useSlider, useSliderThumb and
// react-stately's useSliderState). The caller renders the track, the
// thumbs and the fill between them with whatever markup and classes it
// wants:
//
//	s := aria.UseSlider(aria.SliderOptions{DefaultValues: []float64{40}, AriaLabel: "Volume"})
//	track := tela.UseSize(s.TrackRef)
//	return Div(s.Props, Class("flex h-5 w-64 items-center"),
//	    Div(s.TrackProps, Class("relative h-2 flex-1 rounded-full bg-muted"),
//	        Div(s.ThumbProps(0), Class("absolute -top-1.5 -ml-2.5 size-5 rounded-full bg-primary"),
//	            Style(tela.TranslateFn(func() (x, y float32) {
//	                return float32(s.ThumbPercent(0)) / 100 * track.Get().Width, 0
//	            })))))
//
// Dragging moves a thumb by repainting only: the thumb's position is read
// from ThumbPercent in a paint-phase closure, and an uncontrolled slider
// builds when the drag starts and ends, not on every move (a controlled
// one builds as often as the caller's state changes). A press on the track
// moves the nearest thumb to the point and goes on dragging it. A thumb
// never passes its neighbor (nor comes closer than MinStepsBetweenThumbs).
func UseSlider(o SliderOptions) SliderResult {
	const hook = "UseSlider: SliderOptions."
	orient := checkChoice(hook, "Orientation", o.Orientation, "horizontal", "horizontal", "vertical")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	if o.Min == 0 && o.Max == 0 {
		o.Max = 100
	}
	if o.Max <= o.Min && core.DevNow() {
		panic(fmt.Sprintf("aria: UseSlider: SliderOptions.Max (%g) must be greater than Min (%g)", o.Max, o.Min))
	}
	if o.Step <= 0 {
		if o.Step < 0 && core.DevNow() {
			panic(fmt.Sprintf("aria: UseSlider: SliderOptions.Step %g: a step is positive (0 means the default, 1)", o.Step))
		}
		o.Step = 1
	}
	_, bump := tela.UseState(0)
	version := tela.UseValue(0)
	timeout := core.UseTimeout()
	s := tela.UseOnce(func() *sliderState {
		return &sliderState{track: core.NewElementRef(), label: core.NewElementRef()}
	})
	s.bump, s.version, s.t = bump, version, timeout
	s.sync(o, orient == "vertical", dir == "rtl")

	name := h.AriaLabelledBy(s.label)
	switch {
	case o.AriaLabel != "":
		name = h.AriaLabel(o.AriaLabel)
	case o.AriaLabelledBy != (tela.ElementRef{}):
		name = h.AriaLabelledBy(o.AriaLabelledBy)
	}
	s.name = name
	props := []tela.Prop{h.Role("group"), name}
	if o.Disabled {
		props = append(props, h.AriaDisabled(true))
	}
	return SliderResult{
		Props:      core.MergeProps(props...),
		TrackProps: core.MergeProps(h.Ref(s.track), h.OnPointerDown(s.onTrackDown), h.OnPointerMove(s.onMove), h.OnPointerUp(s.onUp), h.OnPointerCancel(s.onCancel)),
		LabelProps: h.Ref(s.label),
		Values:     slices.Clone(s.values),
		Dragging:   s.drag.active,
		TrackRef:   s.track,
		s:          s,
	}
}

// dragSettle is how long a pointer must rest, while dragging, for the
// slider to build again with the values it has reached.
const dragSettle = 120 * time.Millisecond

// sliderState is the state of one slider hook, on the UI thread. The
// handlers read and write it directly (a second event before the next
// build sees the first one's effect, as boolState does); bump asks for a
// build and version, a Value that paint-phase closures read through
// ThumbPercent, for a repaint.
type sliderState struct {
	bump    tela.Setter[int]
	version *tela.Value[int]
	ver     int
	t       core.Timeout

	min, max, step, gap float64
	page                float64
	vertical, rtl       bool
	disabled            bool
	controlled          bool
	started             bool
	onChange, onEnd     func([]float64)
	live                func() []float64
	format              func(float64) string
	labels              []string
	name                tela.Prop

	// values are the values shown; latest the values last asked for (the
	// same for an uncontrolled slider, and within an event for a
	// controlled one), which the thumbs' limits and the callbacks use.
	values, latest []float64

	track  tela.ElementRef
	label  tela.ElementRef
	thumbs []tela.ElementRef

	drag struct {
		active  bool
		thumb   int
		pointer int
		grab    float64 // the pointer's fraction minus the thumb's, at the press
		holder  tela.ElementRef
	}
}

func (s *sliderState) invalidate() {
	s.bump.Update(func(n int) int { return n + 1 })
}

// repaint tells the paint-phase readers of ThumbPercent that a value
// changed.
func (s *sliderState) repaint() {
	s.ver++
	core.PublishValue(s.version, s.ver)
}

// sync takes the options of a build.
func (s *sliderState) sync(o SliderOptions, vertical, rtl bool) {
	s.min, s.max, s.step, s.gap = o.Min, o.Max, o.Step, float64(max(o.MinStepsBetweenThumbs, 0))*o.Step
	s.page = o.PageSize
	s.vertical, s.rtl, s.disabled = vertical, rtl, o.Disabled
	s.onChange, s.onEnd, s.labels = o.OnValuesChange, o.OnChangeEnd, o.ThumbLabels
	s.live = o.LiveValues
	s.format = o.FormatValue
	if s.format == nil {
		s.format = formatNumber
	}
	s.controlled = o.Values != nil
	prev := s.values
	switch {
	case s.controlled:
		s.values = slices.Clone(*o.Values)
	case !s.started:
		s.values = slices.Clone(o.DefaultValues)
		if len(s.values) == 0 {
			s.values = []float64{o.Min}
		}
	}
	s.started = true
	for i, v := range s.values {
		s.values[i] = min(max(v, s.min), s.max)
	}
	s.latest = slices.Clone(s.values)
	if s.disabled {
		s.drag.active = false
	}
	if prev != nil && !slices.Equal(prev, s.values) {
		s.repaint() // a controlled value moved: the thumbs' closures read it now
	}
}

// ---------------------------------------------------------------------
// Values.

// formatNumber is the default FormatValue: the number as it is written.
func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// roundToStepPrecision rounds v to the decimals of step, which removes the
// noise of adding steps in floating point (React Aria's
// roundToStepPrecision).
func roundToStepPrecision(v, step float64) float64 {
	str := strconv.FormatFloat(step, 'f', -1, 64)
	i := strings.IndexByte(str, '.')
	if i < 0 {
		return v
	}
	p := math.Pow(10, float64(len(str)-i))
	return math.Floor(v*p+0.5) / p
}

// snapValueToStep rounds v to the nearest multiple of step from min, and
// keeps it in [min, max] (React Aria's snapValueToStep).
func snapValueToStep(v, lo, hi, step float64) float64 {
	rem := math.Mod(v-lo, step)
	var snapped float64
	if math.Abs(rem)*2 >= step {
		sign := 1.0
		if rem < 0 {
			sign = -1
		}
		snapped = roundToStepPrecision(v+sign*(step-math.Abs(rem)), step)
	} else {
		snapped = roundToStepPrecision(v-rem, step)
	}
	if snapped < lo {
		snapped = lo
	} else if snapped > hi {
		snapped = lo + math.Floor(roundToStepPrecision((hi-lo)/step, step))*step
	}
	return roundToStepPrecision(snapped, step)
}

// pageSize is how far PageUp, PageDown and Shift+arrow move a thumb: the
// PageSize option, else a tenth of the range, rounded to a step, at least
// one step.
func (s *sliderState) pageSize() float64 {
	if s.page > 0 {
		return max(s.step, math.Floor(s.page/s.step+0.5)*s.step)
	}
	return max(s.step, math.Floor((s.max-s.min)/10/s.step+0.5)*s.step)
}

// thumbMin and thumbMax are the limits of thumb i: the ends of the track,
// or its neighbors (with the gap between thumbs).
func (s *sliderState) thumbMin(i int) float64 {
	if i <= 0 {
		return s.min
	}
	return s.latest[i-1] + s.gap
}

func (s *sliderState) thumbMax(i int) float64 {
	if i >= len(s.latest)-1 {
		return s.max
	}
	return s.latest[i+1] - s.gap
}

func (s *sliderState) fractionOf(v float64) float64 { return (v - s.min) / (s.max - s.min) }

func (s *sliderState) valueAt(f float64) float64 { return s.min + f*(s.max-s.min) }

// setThumb asks for value v on thumb i, rounded to the step and kept
// between its neighbors, and reports whether the value changed. An
// uncontrolled slider takes it and repaints; a controlled one only
// reports it.
func (s *sliderState) setThumb(i int, v float64) bool {
	if s.disabled || i < 0 || i >= len(s.latest) {
		return false
	}
	nv := snapValueToStep(v, s.thumbMin(i), s.thumbMax(i), s.step)
	if nv == s.latest[i] {
		return false
	}
	s.latest = slices.Clone(s.latest)
	s.latest[i] = nv
	if !s.controlled {
		s.values = s.latest
		s.repaint()
		if !s.drag.active {
			s.invalidate()
		} else {
			// A drag builds when it ends, and when the pointer rests, so
			// that the semantics (aria-valuenow) follow it without a build
			// per move.
			s.t.Start(dragSettle, s.invalidate)
		}
	}
	if s.onChange != nil {
		s.onChange(slices.Clone(s.latest))
	}
	return true
}

// refresh takes the values the caller has now (LiveValues), when it gave a
// function for them and the number of thumbs agrees.
func (s *sliderState) refresh() {
	if s.live == nil || !s.controlled {
		return
	}
	v := s.live()
	if len(v) != len(s.values) {
		return
	}
	v = slices.Clone(v)
	for i := range v {
		v[i] = min(max(v[i], s.min), s.max)
	}
	s.values, s.latest = v, slices.Clone(v)
}

func (s *sliderState) changeEnd() {
	if s.onEnd != nil {
		s.onEnd(slices.Clone(s.latest))
	}
}

func (s *sliderState) thumbPercent(i int) float64 {
	s.version.Get() // subscribe the closure that reads it
	if i < 0 || i >= len(s.values) {
		return 0
	}
	return s.fractionOf(s.values[i]) * 100
}

func (s *sliderState) valueText(i int) string {
	s.version.Get()
	if i < 0 || i >= len(s.values) {
		return ""
	}
	return s.format(s.values[i])
}

func (s *sliderState) thumbRef(i int) tela.ElementRef {
	for len(s.thumbs) <= i {
		s.thumbs = append(s.thumbs, core.NewElementRef())
	}
	return s.thumbs[i]
}

// ---------------------------------------------------------------------
// Props.

func (s *sliderState) thumbProps(i int) tela.Prop {
	if i < 0 || i >= len(s.values) {
		if core.DevNow() {
			panic(fmt.Sprintf("aria: SliderResult.ThumbProps(%d): the slider has %d thumbs, from 0 to %d; the number of thumbs is the length of the values (Values or DefaultValues)",
				i, len(s.values), len(s.values)-1))
		}
		return tela.Prop{}
	}
	orient := "horizontal"
	if s.vertical {
		orient = "vertical"
	}
	name := s.name
	if i < len(s.labels) && s.labels[i] != "" {
		name = h.AriaLabel(s.labels[i])
	}
	v := s.values[i]
	ps := []tela.Prop{
		h.Ref(s.thumbRef(i)), h.Role("slider"), name,
		h.AriaValueNow(v), h.AriaValueMin(s.thumbMin(i)), h.AriaValueMax(s.thumbMax(i)),
		h.AriaValueText(s.format(v)), h.AriaOrientation(orient),
	}
	if s.disabled {
		return core.MergeProps(append(ps, h.Disabled(true), h.AriaDisabled(true))...)
	}
	ps = append(ps,
		h.TabIndex(0),
		h.OnPointerDown(func(e tela.PointerEvent) { s.onThumbDown(i, e) }),
		h.OnPointerMove(s.onMove), h.OnPointerUp(s.onUp), h.OnPointerCancel(s.onCancel),
		h.OnKeyDown(func(e tela.KeyboardEvent) { s.onKeyDown(i, e) }),
	)
	return core.MergeProps(ps...)
}

// ---------------------------------------------------------------------
// Pointer.

// fraction returns how far along the track the pointer is, from 0 at Min
// to 1 at Max, and false while the track has no size.
func (s *sliderState) fraction(e tela.PointerEvent) (float64, bool) {
	b := s.track.Bounds()
	size, pos := b.Width, e.X-b.X
	if s.vertical {
		size, pos = b.Height, e.Y-b.Y
	}
	if size <= 0 {
		return 0, false
	}
	f := float64(pos / size)
	if s.vertical || s.rtl {
		f = 1 - f
	}
	return min(max(f, 0), 1), true
}

// startDrag makes thumb i follow the pointer, which a mouse keeps (a
// touch or a pen is captured by what it pressed).
func (s *sliderState) startDrag(i int, e tela.PointerEvent, grab float64, holder tela.ElementRef) {
	s.drag.active, s.drag.thumb, s.drag.pointer, s.drag.grab, s.drag.holder = true, i, e.PointerID, grab, holder
	if e.Kind == tela.PointerMouse {
		core.SetPointerCapture(holder, e.PointerID)
	}
	s.thumbRef(i).Focus()
	s.invalidate() // Dragging
}

func (s *sliderState) endDrag() {
	if !s.drag.active {
		return
	}
	s.t.Stop()
	s.drag.active = false
	core.ReleasePointerCapture(s.drag.holder, s.drag.pointer)
	s.invalidate() // Dragging, and the values the drag left unbuilt
	s.changeEnd()
}

func (s *sliderState) onThumbDown(i int, e tela.PointerEvent) {
	if e.Button != tela.MouseButtonLeft || s.disabled || s.drag.active {
		return
	}
	s.refresh()
	f, ok := s.fraction(e)
	if !ok || i >= len(s.latest) {
		return
	}
	s.startDrag(i, e, f-s.fractionOf(s.latest[i]), s.thumbRef(i))
	e.StopPropagation()
}

func (s *sliderState) onTrackDown(e tela.PointerEvent) {
	if e.Button != tela.MouseButtonLeft || s.disabled || s.drag.active || len(s.latest) == 0 {
		return
	}
	s.refresh()
	f, ok := s.fraction(e)
	if !ok {
		return
	}
	i := s.closestThumb(snapValueToStep(s.valueAt(f), s.min, s.max, s.step))
	e.PreventDefault() // the focus goes to the thumb, not away
	e.StopPropagation()
	s.startDrag(i, e, 0, s.track)
	s.setThumb(i, s.valueAt(f))
}

// closestThumb is React Aria's rule for a press on the track: the thumb
// nearest to the value, the lower one on a tie, the first or the last past
// the ends.
func (s *sliderState) closestThumb(v float64) int {
	split := -1
	for i, tv := range s.latest {
		if v-tv < 0 {
			split = i
			break
		}
	}
	switch {
	case split == 0:
		return 0
	case split == -1:
		return len(s.latest) - 1
	}
	if math.Abs(s.latest[split-1]-v) < math.Abs(s.latest[split]-v) {
		return split - 1
	}
	return split
}

func (s *sliderState) onMove(e tela.PointerEvent) {
	if !s.drag.active || e.PointerID != s.drag.pointer {
		return
	}
	e.StopPropagation()
	if f, ok := s.fraction(e); ok {
		s.setThumb(s.drag.thumb, s.valueAt(f-s.drag.grab))
	}
}

func (s *sliderState) onUp(e tela.PointerEvent) {
	if !s.drag.active || e.PointerID != s.drag.pointer {
		return
	}
	e.StopPropagation()
	s.endDrag()
}

func (s *sliderState) onCancel(e tela.PointerEvent) { s.onUp(e) }

// ---------------------------------------------------------------------
// Keyboard.

func (s *sliderState) onKeyDown(i int, e tela.KeyboardEvent) {
	if s.disabled || i >= len(s.latest) {
		return
	}
	s.refresh()
	cur := s.latest[i]
	var next float64
	switch e.Key {
	case tela.KeyArrowRight, tela.KeyArrowUp, tela.KeyArrowLeft, tela.KeyArrowDown:
		by := s.step
		if e.Shift {
			by = s.pageSize()
		}
		// ArrowRight and ArrowUp increase, except that a right-to-left
		// slider mirrors the horizontal arrows (React Aria's reverseX).
		up := e.Key == tela.KeyArrowRight || e.Key == tela.KeyArrowUp
		if s.rtl && (e.Key == tela.KeyArrowRight || e.Key == tela.KeyArrowLeft) {
			up = !up
		}
		if !up {
			by = -by
		}
		next = snapValueToStep(cur+by, s.min, s.max, s.step)
	case tela.KeyPageUp:
		next = snapValueToStep(cur+s.pageSize(), s.min, s.max, s.step)
	case tela.KeyPageDown:
		next = snapValueToStep(cur-s.pageSize(), s.min, s.max, s.step)
	case tela.KeyHome:
		next = s.thumbMin(i)
	case tela.KeyEnd:
		next = s.thumbMax(i)
	default:
		return
	}
	e.PreventDefault()
	if s.setThumb(i, next) {
		s.changeEnd()
	}
}
