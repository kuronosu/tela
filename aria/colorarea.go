// Portions of this file follow React Aria's useColorArea
// (https://github.com/adobe/react-spectrum, packages/@react-aria/color),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// ColorAreaOptions configures [UseColorArea]: a two-dimensional slider
// that changes two channels of the color at once.
type ColorAreaOptions struct {
	// State is the color the area changes, from [UseColorState]. It is
	// required, and must come from the same component.
	State *ColorState
	// XChannel and YChannel are the channels along the two axes: any two
	// of [ColorHue], [ColorSaturation] and [ColorBrightness] (the HSB
	// model), the third one stays as it is. The defaults are the picker's:
	// saturation along X and brightness along Y.
	XChannel, YChannel ColorChannel
	// Direction is "ltr" (the default) or "rtl", which mirrors the X axis
	// and the left and right arrow keys, as React Aria's area does.
	Direction string
	// Disabled makes the thumb inert and disabled, and the area ignores
	// presses.
	Disabled bool
	// OnChangeEnd is called with the color when the user stops changing
	// it: the pointer is released, or a key changed it.
	OnChangeEnd func(tela.Color)
	// FormatValue returns the text of a channel's value for aria-valuetext
	// and [ColorAreaResult.ValueText]; the default is English ("Saturation
	// 50%"). Use package i18n for another language.
	FormatValue func(ColorChannel, float64) string
	// AriaLabel names the area ("Color area" by default); else it is named
	// by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the area.
	AriaLabelledBy tela.ElementRef
}

// ColorAreaResult is what [UseColorArea] returns.
type ColorAreaResult struct {
	// AreaProps go to the area element: the "group" role and its name,
	// and the handlers that make a press on it move the thumb to the
	// point and start dragging it, with a mouse, a finger or a pen.
	AreaProps tela.Prop
	// ThumbProps go to the thumb: the "slider" role (one for both
	// channels, as the semantics tree has no two-dimensional slider) with
	// the first channel's aria-valuenow and a value text that has both, a
	// tab stop, and the handlers: a press drags it (it keeps the grab, with
	// no jump), the arrow keys move it by one step (by a page with Shift,
	// mirrored in a "rtl" Direction), PageUp and PageDown change the Y
	// channel by a page, Home and End the X channel.
	ThumbProps tela.Prop
	// AreaRef and ThumbRef are the refs that the props attach.
	AreaRef, ThumbRef tela.ElementRef
	// Dragging is whether the thumb is being dragged, as of this build.
	Dragging bool

	a *colorAreaState
}

// ThumbPercent returns the position of the thumb in the area as it is
// now, in percent from the left edge (x) and from the top edge (y), 0 to
// 100: the X axis runs from the minimum of the X channel at the left to its
// maximum at the right (reversed in a "rtl" Direction) and the Y axis from
// the maximum at the top. It subscribes what reads it: read it in a
// paint-phase closure to place the thumb while it is dragged without
// building the component, as
//
//	Style(tela.TranslateFn(func() (x, y float32) {
//	    px, py := area.ThumbPercent()
//	    sz := size.Get()
//	    return float32(px) / 100 * sz.Width, float32(py) / 100 * sz.Height
//	}))
//
// where size is tela.UseSize(area.AreaRef). Read in the component's body it
// makes the component build on every move.
func (r ColorAreaResult) ThumbPercent() (x, y float64) { return r.a.thumbPercent() }

// ValueText returns the text of both channels as it is now ("Saturation
// 50%, Brightness 70%").
func (r ColorAreaResult) ValueText() string { return r.a.valueText() }

// UseColorArea holds the interaction of a color area (React Aria's
// useColorArea): a thumb in a rectangle, dragged with a pointer or moved
// with the keys, that changes two channels of the shared [ColorState]. The
// caller renders the area (its background is the gradients of the hue, as
// the kit's does) and the thumb:
//
//	st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(tela.Hex("#6366f1"))})
//	area := aria.UseColorArea(aria.ColorAreaOptions{State: st, AriaLabel: "Color"})
//	size := tela.UseSize(area.AreaRef)
//	return Div(area.AreaProps, Class("relative size-48 rounded-md"),
//	    Style(tela.BgFn(func() tela.Color { return aria.HSB{H: st.LiveHSB().H, S: 100, B: 100, A: 1}.Color() })),
//	    Div(area.ThumbProps, Class("absolute -ml-2.5 -mt-2.5 size-5 rounded-full border-2 border-white"),
//	        Style(tela.TranslateFn(func() (x, y float32) {
//	            px, py := area.ThumbPercent()
//	            return float32(px) / 100 * size.Get().Width, float32(py) / 100 * size.Get().Height
//	        }))))
//
// Dragging moves the thumb by repainting only: like [UseSlider], an
// uncontrolled color builds when the drag starts and ends and when the
// pointer rests, not on every move.
//
// Unlike a slider's, the Home and End keys step the X channel by a page
// and PageUp and PageDown the Y channel, as React Aria's area does. The
// third channel (the hue, in the default area) is the state's: it is kept
// while the area moves the other two.
func UseColorArea(o ColorAreaOptions) ColorAreaResult {
	const hook = "UseColorArea: ColorAreaOptions."
	if o.State == nil {
		panic("aria: UseColorArea: ColorAreaOptions.State is required: make it in the same component with aria.UseColorState(aria.ColorStateOptions{...})")
	}
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	xch := ColorChannel(checkChoice(hook, "XChannel", string(o.XChannel), string(ColorSaturation), string(ColorHue), string(ColorSaturation), string(ColorBrightness)))
	ych := ColorChannel(checkChoice(hook, "YChannel", string(o.YChannel), string(ColorBrightness), string(ColorHue), string(ColorSaturation), string(ColorBrightness)))
	if xch == ych {
		if core.DevNow() {
			panic(fmt.Sprintf("aria: UseColorArea: ColorAreaOptions.XChannel and YChannel are both %q: an area changes two different channels (any two of hue, saturation and brightness)", xch))
		}
		xch, ych = ColorSaturation, ColorBrightness
	}
	a := tela.UseOnce(func() *colorAreaState {
		return &colorAreaState{area: core.NewElementRef(), thumb: core.NewElementRef()}
	})
	a.st, a.x, a.y, a.rtl, a.disabled = o.State, xch, ych, dir == "rtl", o.Disabled
	a.onEnd, a.format = o.OnChangeEnd, o.FormatValue
	if a.format == nil {
		a.format = channelText
	}
	if o.Disabled {
		a.drag.active = false
	}

	name := h.AriaLabel(cmpOr(o.AriaLabel, "Color area"))
	if o.AriaLabel == "" && o.AriaLabelledBy != (tela.ElementRef{}) {
		name = h.AriaLabelledBy(o.AriaLabelledBy)
	}
	a.name = name
	group := []tela.Prop{h.Role("group"), name, h.Ref(a.area)}
	if !o.Disabled {
		group = append(group, h.OnPointerDown(a.onAreaDown), h.OnPointerMove(a.onMove), h.OnPointerUp(a.onUp), h.OnPointerCancel(a.onCancel))
	} else {
		group = append(group, h.AriaDisabled(true))
	}
	return ColorAreaResult{
		AreaProps:  core.MergeProps(group...),
		ThumbProps: a.thumbProps(),
		AreaRef:    a.area,
		ThumbRef:   a.thumb,
		Dragging:   a.drag.active,
		a:          a,
	}
}

func cmpOr(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// colorAreaState is the interaction state of one area hook, on the UI
// thread; the color itself is the ColorState's.
type colorAreaState struct {
	st       *ColorState
	x, y     ColorChannel
	rtl      bool
	disabled bool
	onEnd    func(tela.Color)
	format   func(ColorChannel, float64) string
	name     tela.Prop

	area, thumb tela.ElementRef

	drag struct {
		active       bool
		pointer      int
		grabX, grabY float64 // the pointer's fractions minus the thumb's, at the press
		holder       tela.ElementRef
	}
}

func (a *colorAreaState) xi() channelInfo { return infoOf(a.x) }
func (a *colorAreaState) yi() channelInfo { return infoOf(a.y) }

// fractions returns the X and Y fractions (0 to 1) of the thumb's value:
// how far its channels are from their minimums, in the live color.
func (a *colorAreaState) fractions(c HSB) (fx, fy float64) {
	xi, yi := a.xi(), a.yi()
	fx = (a.st.channelOf(c, "hsb", a.x) - xi.min) / (xi.max - xi.min)
	fy = (a.st.channelOf(c, "hsb", a.y) - yi.min) / (yi.max - yi.min)
	return fx, fy
}

func (a *colorAreaState) thumbPercent() (x, y float64) {
	fx, fy := a.fractions(a.st.LiveHSB())
	if a.rtl {
		fx = 1 - fx
	}
	return fx * 100, (1 - fy) * 100
}

func (a *colorAreaState) valueText() string {
	c := a.st.LiveHSB()
	return formatChannels(a.format, a.x, a.st.channelOf(c, "hsb", a.x), a.y, a.st.channelOf(c, "hsb", a.y))
}

func (a *colorAreaState) thumbProps() tela.Prop {
	c := a.st.built
	xv := a.st.channelOf(c, "hsb", a.x)
	xi := a.xi()
	ps := []tela.Prop{
		h.Ref(a.thumb), h.Role("slider"), a.name,
		h.AriaValueNow(xv), h.AriaValueMin(xi.min), h.AriaValueMax(xi.max),
		h.AriaValueText(formatChannels(a.format, a.x, xv, a.y, a.st.channelOf(c, "hsb", a.y))),
	}
	if a.disabled {
		return core.MergeProps(append(ps, h.Disabled(true), h.AriaDisabled(true))...)
	}
	return core.MergeProps(append(ps,
		h.TabIndex(0),
		h.OnPointerDown(a.onThumbDown), h.OnPointerMove(a.onMove), h.OnPointerUp(a.onUp), h.OnPointerCancel(a.onCancel),
		h.OnKeyDown(a.onKeyDown),
	)...)
}

// ---------------------------------------------------------------------
// Pointer.

// point returns how far along each axis the pointer is in the area, as
// fractions of the channels' ranges (Y from the bottom), and false while
// the area has no size.
func (a *colorAreaState) point(e tela.PointerEvent) (fx, fy float64, ok bool) {
	b := a.area.Bounds()
	if b.Width <= 0 || b.Height <= 0 {
		return 0, 0, false
	}
	fx = float64((e.X - b.X) / b.Width)
	if a.rtl {
		fx = 1 - fx
	}
	fy = 1 - float64((e.Y-b.Y)/b.Height)
	return clampTo(fx, 0, 1), clampTo(fy, 0, 1), true
}

func (a *colorAreaState) startDrag(e tela.PointerEvent, gx, gy float64, holder tela.ElementRef) {
	a.drag.active, a.drag.pointer, a.drag.grabX, a.drag.grabY, a.drag.holder = true, e.PointerID, gx, gy, holder
	if e.Kind == tela.PointerMouse {
		core.SetPointerCapture(holder, e.PointerID)
	}
	a.thumb.Focus()
	a.st.invalidate() // Dragging
}

func (a *colorAreaState) endDrag() {
	if !a.drag.active {
		return
	}
	a.st.settle()
	a.drag.active = false
	core.ReleasePointerCapture(a.drag.holder, a.drag.pointer)
	a.st.invalidate() // Dragging, and the color the drag left unbuilt
	a.changeEnd()
}

func (a *colorAreaState) changeEnd() {
	if a.onEnd != nil {
		a.onEnd(a.st.latest.Color())
	}
}

// setFractions moves the thumb to the fractions of the two axes, rounding
// each channel to its step.
func (a *colorAreaState) setFractions(fx, fy float64, dragging bool) bool {
	xi, yi := a.xi(), a.yi()
	xv := snapValueToStep(xi.min+clampTo(fx, 0, 1)*(xi.max-xi.min), xi.min, xi.max, xi.step)
	yv := snapValueToStep(yi.min+clampTo(fy, 0, 1)*(yi.max-yi.min), yi.min, yi.max, yi.step)
	return a.setValues(xv, yv, dragging)
}

func (a *colorAreaState) setValues(xv, yv float64, dragging bool) bool {
	if a.disabled {
		return false
	}
	next := a.st.withChannel(a.st.latest, "hsb", a.x, xv)
	next = a.st.withChannel(next, "hsb", a.y, yv)
	return a.st.apply(next, dragging)
}

func (a *colorAreaState) onThumbDown(e tela.PointerEvent) {
	if e.Button != tela.MouseButtonLeft || a.disabled || a.drag.active {
		return
	}
	fx, fy, ok := a.point(e)
	if !ok {
		return
	}
	tx, ty := a.fractions(a.st.latest)
	a.startDrag(e, fx-tx, fy-ty, a.thumb)
	e.StopPropagation()
}

func (a *colorAreaState) onAreaDown(e tela.PointerEvent) {
	if e.Button != tela.MouseButtonLeft || a.disabled || a.drag.active {
		return
	}
	fx, fy, ok := a.point(e)
	if !ok {
		return
	}
	e.PreventDefault() // the focus goes to the thumb, not away
	e.StopPropagation()
	a.startDrag(e, 0, 0, a.area)
	a.setFractions(fx, fy, true)
}

func (a *colorAreaState) onMove(e tela.PointerEvent) {
	if !a.drag.active || e.PointerID != a.drag.pointer {
		return
	}
	e.StopPropagation()
	if fx, fy, ok := a.point(e); ok {
		a.setFractions(fx-a.drag.grabX, fy-a.drag.grabY, true)
	}
}

func (a *colorAreaState) onUp(e tela.PointerEvent) {
	if !a.drag.active || e.PointerID != a.drag.pointer {
		return
	}
	e.StopPropagation()
	a.endDrag()
}

func (a *colorAreaState) onCancel(e tela.PointerEvent) { a.onUp(e) }

// ---------------------------------------------------------------------
// Keyboard.

func (a *colorAreaState) onKeyDown(e tela.KeyboardEvent) {
	if a.disabled {
		return
	}
	xi, yi := a.xi(), a.yi()
	step := func(i channelInfo) float64 {
		if e.Shift {
			return i.page
		}
		return i.step
	}
	var dx, dy float64
	switch e.Key {
	case tela.KeyArrowRight, tela.KeyArrowLeft:
		dx = step(xi)
		if (e.Key == tela.KeyArrowLeft) != a.rtl { // left decreases, right increases; "rtl" mirrors
			dx = -dx
		}
	case tela.KeyArrowUp:
		dy = step(yi)
	case tela.KeyArrowDown:
		dy = -step(yi)
	case tela.KeyPageUp:
		dy = yi.page
	case tela.KeyPageDown:
		dy = -yi.page
	case tela.KeyHome, tela.KeyEnd:
		dx = xi.page
		if (e.Key == tela.KeyHome) != a.rtl { // Home goes toward the left edge
			dx = -dx
		}
	default:
		return
	}
	e.PreventDefault()
	cur := a.st.latest
	xv := snapValueToStep(a.st.channelOf(cur, "hsb", a.x)+dx, xi.min, xi.max, xi.step)
	yv := snapValueToStep(a.st.channelOf(cur, "hsb", a.y)+dy, yi.min, yi.max, yi.step)
	if a.setValues(xv, yv, false) {
		a.changeEnd()
	}
}
