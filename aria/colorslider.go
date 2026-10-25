// Portions of this file follow React Aria's useColorSlider
// (https://github.com/adobe/react-spectrum, packages/@react-aria/color),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// ColorSliderOptions configures [UseColorSlider]: a slider for one channel
// of the shared color.
type ColorSliderOptions struct {
	// State is the color the slider changes, from [UseColorState]. It is
	// required, and must come from the same component.
	State *ColorState
	// Channel is the channel the slider changes. It is required:
	// [ColorHue] (0 to 360), [ColorAlpha] (0 to 1), [ColorSaturation] and
	// [ColorBrightness] (0 to 100), or [ColorLightness] with a "hsl"
	// Space.
	Channel ColorChannel
	// Space is the color model of the saturation and the lightness:
	// "hsb" (the default; it has a brightness) or "hsl" (it has a
	// lightness, and its saturation is not the HSB's). Hue and alpha are
	// the same in both.
	Space string
	// Orientation is "horizontal" (the default) or "vertical" (the minimum
	// at the bottom).
	Orientation string
	// Direction is "ltr" (the default) or "rtl", which mirrors a
	// horizontal slider and its arrow keys.
	Direction string
	// Disabled makes the thumb inert and disabled.
	Disabled bool
	// OnChangeEnd is called with the color when the user stops changing it.
	OnChangeEnd func(tela.Color)
	// FormatValue returns the text of the value for aria-valuetext and
	// [ColorSliderResult.ValueText]; the default is English ("Hue 180°",
	// "Alpha 80%"). Use package i18n for another language.
	FormatValue func(ColorChannel, float64) string
	// AriaLabel names the slider; the default is the channel's name
	// ("Hue"). A visible label is the slider's name with AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the slider.
	AriaLabelledBy tela.ElementRef
}

// ColorSliderResult is what [UseColorSlider] returns.
type ColorSliderResult struct {
	// Props go to the slider's root element: the "group" role and its
	// name.
	Props tela.Prop
	// TrackProps go to the track, which is the gradient: a press on it
	// moves the thumb to the point and starts dragging it.
	TrackProps tela.Prop
	// ThumbProps go to the thumb: the "slider" role, aria-valuenow,
	// aria-valuetext (the channel's name and value), a tab stop and the
	// handlers of [UseSlider] (arrows by one step, Shift and the page keys
	// by a page, Home and End to the ends).
	ThumbProps tela.Prop
	// TrackRef and ThumbRef are the refs that the props attach.
	TrackRef, ThumbRef tela.ElementRef
	// Channel is the channel of the slider, and Min, Max and Step its
	// range: 0 to 360 for the hue, 0 to 1 by 0.01 for the alpha and 0 to
	// 100 for the others.
	Channel        ColorChannel
	Min, Max, Step float64
	// Dragging is whether the thumb is being dragged, as of this build.
	Dragging bool

	state  *ColorState
	space  string
	format func(ColorChannel, float64) string
}

// ThumbPercent returns the position of the thumb along the track as it is
// now, from 0 (Min) to 100 (Max), exactly as [SliderResult.ThumbPercent]:
// it subscribes what reads it, so read it in a paint-phase closure to place
// the thumb while it is dragged without building the component.
func (r ColorSliderResult) ThumbPercent() float64 {
	info := infoOf(r.Channel)
	v := r.state.channelOf(r.state.LiveHSB(), r.space, r.Channel)
	return (v - info.min) / (info.max - info.min) * 100
}

// Value returns the value of the channel as it is now, in the channel's
// range. Like ThumbPercent it subscribes what reads it.
func (r ColorSliderResult) Value() float64 {
	return r.state.channelOf(r.state.LiveHSB(), r.space, r.Channel)
}

// ValueText returns the text of the value as it is now.
func (r ColorSliderResult) ValueText() string { return r.format(r.Channel, r.Value()) }

// TrackColors returns the colors of the track from the minimum of the
// channel to its maximum, with the other channels of the color as they are
// now: the stops of the gradient behind the thumb. The hue has seven (every
// 60 degrees), the lightness three (black, the color, white) and the other
// channels two. For the alpha the first is transparent and the last
// opaque, to show over a checkerboard. It subscribes what reads it, so a
// track whose gradient follows the color is drawn from a paint-phase
// closure.
func (r ColorSliderResult) TrackColors() []tela.Color {
	return r.state.stopsOf(r.state.LiveHSB(), r.space, r.Channel)
}

// UseColorSlider holds the interaction of a slider for one channel of a
// color (React Aria's useColorSlider). It is built on [UseSlider], so it
// drags, rounds and answers keys like it, with the channel's range, step
// and page. The caller renders the track (a gradient of TrackColors) and
// the thumb:
//
//	st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: tela.Hex("#6366f1")})
//	hue := aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorHue})
//	track := tela.UseSize(hue.TrackRef)
//	return Div(hue.Props, Class("w-64"),
//	    Div(hue.TrackProps, Class("relative h-3 rounded-full"),
//	        Div(hue.ThumbProps, Class("absolute -top-1 -ml-2.5 size-5 rounded-full border-2 border-white"),
//	            Style(tela.TranslateFn(func() (x, y float32) {
//	                return float32(hue.ThumbPercent()) / 100 * track.Get().Width, 0
//	            })))))
//
// Dragging repaints and does not build, as the slider does.
func UseColorSlider(o ColorSliderOptions) ColorSliderResult {
	const hook = "UseColorSlider: ColorSliderOptions."
	if o.State == nil {
		panic("aria: UseColorSlider: ColorSliderOptions.State is required: make it in the same component with aria.UseColorState(aria.ColorStateOptions{...})")
	}
	space := checkSpace(hook, o.Space)
	ch := ColorChannel(checkChoice(hook, "Channel", string(o.Channel), "",
		string(ColorHue), string(ColorSaturation), string(ColorBrightness), string(ColorLightness), string(ColorAlpha)))
	if ch == "" {
		panic(`aria: UseColorSlider: ColorSliderOptions.Channel is required: one of "hue", "saturation", "brightness", "lightness" or "alpha" (aria.ColorHue, ...)`)
	}
	if (ch == ColorBrightness && space == "hsl") || (ch == ColorLightness && space == "hsb") {
		want := map[string]string{"hsb": "ColorLightness is the HSL model's: set Space to \"hsl\", or use ColorBrightness",
			"hsl": "ColorBrightness is the HSB model's: set Space to \"hsb\", or use ColorLightness"}[space]
		if ch == ColorBrightness {
			want = want + " (the default Space is \"hsb\")"
		}
		if core.DevNow() {
			panic(fmt.Sprintf("aria: UseColorSlider: ColorSliderOptions.Channel %q does not exist in Space %q: %s", ch, space, want))
		}
		space = map[ColorChannel]string{ColorBrightness: "hsb", ColorLightness: "hsl"}[ch]
	}
	info := infoOf(ch)
	st := o.State
	format := o.FormatValue
	if format == nil {
		format = channelText
	}

	label := o.AriaLabel
	if label == "" && o.AriaLabelledBy == (tela.ElementRef{}) {
		label = info.name
	}
	var res SliderResult
	cur := []float64{st.channelOf(st.built, space, ch)}
	res = UseSlider(SliderOptions{
		Values: &cur,
		OnValuesChange: func(v []float64) {
			st.apply(st.withChannel(st.latest, space, ch, v[0]), res.IsThumbDragging(0))
		},
		OnChangeEnd: func([]float64) {
			st.settle()
			if o.OnChangeEnd != nil {
				o.OnChangeEnd(st.latest.Color())
			}
		},
		Min: info.min, Max: info.max, Step: info.step, PageSize: info.page,
		Orientation: o.Orientation, Direction: o.Direction, Disabled: o.Disabled,
		FormatValue: func(v float64) string { return format(ch, v) },
		AriaLabel:   label, AriaLabelledBy: o.AriaLabelledBy,
	})
	return ColorSliderResult{
		Props: res.Props, TrackProps: res.TrackProps, ThumbProps: res.ThumbProps(0),
		TrackRef: res.TrackRef, ThumbRef: res.ThumbRef(0),
		Channel: ch, Min: info.min, Max: info.max, Step: info.step,
		Dragging: res.Dragging,
		state:    st, space: space, format: format,
	}
}
