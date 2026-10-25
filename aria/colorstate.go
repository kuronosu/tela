// Portions of this file follow react-stately's useColorPickerState and
// useColorAreaState and React Aria's color utilities
// (https://github.com/adobe/react-spectrum, packages/@react-stately/color
// and packages/@react-aria/color), Copyright 2020 Adobe, licensed under
// the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"math"
	"strconv"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// ColorChannel names one component of a color, for the sliders and the
// areas that change it.
type ColorChannel string

// The channels of the color hooks. Hue, saturation and brightness are
// those of [HSB], saturation and lightness (in a "hsl" space) those of
// [HSL], and alpha is the opacity.
const (
	// ColorHue is the hue, in degrees from 0 to 360.
	ColorHue ColorChannel = "hue"
	// ColorSaturation is the saturation, from 0 to 100.
	ColorSaturation ColorChannel = "saturation"
	// ColorBrightness is the brightness of the HSB model, from 0 to 100.
	ColorBrightness ColorChannel = "brightness"
	// ColorLightness is the lightness of the HSL model, from 0 to 100.
	ColorLightness ColorChannel = "lightness"
	// ColorAlpha is the opacity, from 0 to 1.
	ColorAlpha ColorChannel = "alpha"
)

// channelInfo is the range of a channel and how far the keys move it
// (React Aria's getChannelRange).
type channelInfo struct {
	min, max, step, page float64
	name                 string
}

func infoOf(ch ColorChannel) channelInfo {
	switch ch {
	case ColorHue:
		return channelInfo{0, 360, 1, 15, "Hue"}
	case ColorSaturation:
		return channelInfo{0, 100, 1, 10, "Saturation"}
	case ColorBrightness:
		return channelInfo{0, 100, 1, 10, "Brightness"}
	case ColorLightness:
		return channelInfo{0, 100, 1, 10, "Lightness"}
	default:
		return channelInfo{0, 1, 0.01, 0.1, "Alpha"}
	}
}

// channelText is the default text of a channel's value, as aria-valuetext
// reads it: "Hue 180°", "Saturation 50%", "Alpha 80%". Use a FormatValue
// option (and package i18n) for another language.
func channelText(ch ColorChannel, v float64) string {
	info := infoOf(ch)
	switch ch {
	case ColorHue:
		return info.name + " " + strconv.FormatFloat(math.Round(v), 'f', -1, 64) + "°"
	case ColorAlpha:
		v *= 100
	}
	return info.name + " " + strconv.FormatFloat(math.Round(v), 'f', -1, 64) + "%"
}

// ColorStateOptions configures [UseColorState].
//
// The color is uncontrolled when Value is nil (the state keeps it,
// starting at DefaultValue) and controlled when Value points to the
// caller's state: the hooks show it and only ask OnChange for the next
// one, so a change the caller does not store is refused. OnChange is also
// called for an uncontrolled state.
//
//	// A picker that keeps its color.
//	aria.ColorStateOptions{DefaultValue: tela.Hex("#6366f1")}
//	// A controlled one.
//	c, setC := tela.UseState(tela.Hex("#6366f1"))
//	aria.ColorStateOptions{Value: &c, OnChange: setC.Set}
type ColorStateOptions struct {
	// Value points to the color of a controlled state; nil leaves it to
	// the hook.
	Value *tela.Color
	// DefaultValue is the color of an uncontrolled state at the start.
	// The zero Color is transparent black: pass tela.Black, or a color,
	// to start with an opaque one.
	DefaultValue tela.Color
	// OnChange is called with the next color every time the user changes
	// it: while dragging, on each key, on a hex commit and when a swatch is
	// set. The color is in the sRGB gamut.
	OnChange func(tela.Color)
}

// ColorState is the color that the area, the sliders and the field of one
// picker share (react-stately's useColorPickerState), made by
// [UseColorState]. It keeps the color as an [HSB] and not as the
// [tela.Color] it reports, so that dragging through a gray or through
// black does not lose the hue (or the saturation), and a color the caller
// passes back is recognized as the one the state reported.
//
// The hooks that use a ColorState must be called in the component that
// made it: the state asks that component to build when a drag starts,
// when it ends and when the pointer has rested, as the slider does, and
// not on every move.
type ColorState struct {
	bump    tela.Setter[int]
	version *tela.Value[int]
	ver     int
	t       core.Timeout

	started, controlled bool
	onChange            func(tela.Color)

	// shown is the color as displayed (live: a drag of an uncontrolled
	// state moves it between builds), built the color of the last build,
	// and latest the last one asked for (the same as shown for an
	// uncontrolled state, and within an event for a controlled one).
	shown, latest, built HSB
	// hslS is the saturation of the HSL model, which a lightness of 0 or
	// 100 hides.
	hslS float64
}

// UseColorState holds the color of a picker and returns the state that
// [UseColorArea], [UseColorSlider] and [UseColorField] share.
func UseColorState(o ColorStateOptions) *ColorState {
	_, bump := tela.UseState(0)
	version := tela.UseValue(0)
	timeout := core.UseTimeout()
	s := tela.UseOnce(func() *ColorState { return &ColorState{} })
	s.bump, s.version, s.t = bump, version, timeout
	s.sync(o)
	return s
}

func (s *ColorState) invalidate() {
	s.bump.Update(func(n int) int { return n + 1 })
}

// repaint tells the paint-phase readers (the Live methods) that the color
// changed.
func (s *ColorState) repaint() {
	s.ver++
	core.PublishValue(s.version, s.ver)
}

// sync takes the options of a build.
func (s *ColorState) sync(o ColorStateOptions) {
	s.onChange = o.OnChange
	s.controlled = o.Value != nil
	prev, had := s.shown, s.started
	switch {
	case o.Value != nil:
		v := *o.Value
		switch {
		case !s.started:
			s.shown = ToHSB(v)
		case v == s.latest.Color():
			s.shown = s.latest // the caller stored what was asked
		case v == s.shown.Color():
			// refused, or the same color: keep the hue it was shown with
		default:
			s.shown = fromColorKeeping(v, s.shown)
		}
	case !s.started:
		s.shown = ToHSB(o.DefaultValue)
	}
	s.started = true
	s.noteHSL(s.shown)
	s.latest, s.built = s.shown, s.shown
	if had && prev != s.shown {
		s.repaint() // a controlled color moved: the paint closures read it now
	}
}

// fromColorKeeping converts a color that came from outside, keeping the
// hue of the color shown before when the new one is a gray (and the
// saturation when it is black).
func fromColorKeeping(c tela.Color, prev HSB) HSB {
	h := ToHSB(c)
	if h.S == 0 || h.B == 0 {
		h.H = prev.H
	}
	if h.B == 0 {
		h.S = prev.S
	}
	return h
}

// noteHSL remembers the HSL saturation of a color, unless its lightness
// hides it.
func (s *ColorState) noteHSL(h HSB) {
	if l := h.HSL(); l.L > 1e-9 && l.L < 100-1e-9 {
		s.hslS = l.S
	}
}

// hslOf returns h in the HSL model with the saturation the state kept
// when the lightness hides it.
func (s *ColorState) hslOf(h HSB) HSL {
	l := h.HSL()
	if l.L <= 1e-9 || l.L >= 100-1e-9 {
		l.S = s.hslS
	}
	return l
}

// Color returns the color as of the last build. While a drag moves an
// uncontrolled state the build waits for the pointer to rest or the drag
// to end; read [ColorState.LiveColor] in a paint-phase closure for the
// color that is shown now.
func (s *ColorState) Color() tela.Color { return s.built.Color() }

// HSB returns the color as of the last build, in the HSB model with the
// hue it keeps.
func (s *ColorState) HSB() HSB { return s.built }

// LiveColor returns the color as it is now. It subscribes what reads it:
// read it in a paint-phase closure (tela.BgFn, tela.TextColorFn) to follow
// a drag without building; read in the component's body it makes the
// component build on every move.
func (s *ColorState) LiveColor() tela.Color { return s.LiveHSB().Color() }

// LiveHSB is [ColorState.LiveColor] in the HSB model.
func (s *ColorState) LiveHSB() HSB {
	s.version.Get()
	return s.shown
}

// Set asks for a color, as a swatch or a hex field does. A color that is
// the one the state holds changes nothing; a gray keeps the hue.
func (s *ColorState) Set(c tela.Color) {
	if c == s.latest.Color() {
		return
	}
	s.apply(fromColorKeeping(c, s.latest), false)
}

// SetHSB asks for a color in the HSB model. The values are clamped to
// their ranges.
func (s *ColorState) SetHSB(h HSB) { s.apply(h, false) }

// apply asks for the next color and reports whether it changed anything.
// An uncontrolled state takes it and repaints; a controlled one only
// reports it. A drag builds when the pointer rests, any other change at
// once.
func (s *ColorState) apply(next HSB, dragging bool) bool {
	next = HSB{
		H: clampTo(next.H, 0, 360), S: clampTo(next.S, 0, 100),
		B: clampTo(next.B, 0, 100), A: clampTo(next.A, 0, 1),
	}
	if next == s.latest {
		return false
	}
	s.latest = next
	if !s.controlled {
		s.shown = next
		s.noteHSL(next)
		s.repaint()
		if dragging {
			s.t.Start(dragSettle, s.invalidate)
		} else {
			s.invalidate()
		}
	}
	if s.onChange != nil {
		s.onChange(next.Color())
	}
	return true
}

// settle ends a drag: the pending build of a resting pointer is no longer
// needed, since the drag's own end builds.
func (s *ColorState) settle() { s.t.Stop() }

// channelOf returns the value of a channel of h in a model ("hsb" or
// "hsl").
func (s *ColorState) channelOf(h HSB, space string, ch ColorChannel) float64 {
	switch ch {
	case ColorHue:
		return h.H
	case ColorAlpha:
		return h.A
	case ColorSaturation:
		if space == "hsl" {
			return s.hslOf(h).S
		}
		return h.S
	case ColorBrightness:
		return h.B
	}
	return s.hslOf(h).L // ColorLightness
}

// withChannel returns h with a channel replaced by v.
func (s *ColorState) withChannel(h HSB, space string, ch ColorChannel, v float64) HSB {
	switch ch {
	case ColorHue:
		h.H = v
	case ColorAlpha:
		h.A = v
	case ColorBrightness:
		h.B = v
	default:
		if space != "hsl" {
			h.S = v // saturation of HSB
			return h
		}
		l := s.hslOf(h)
		if ch == ColorSaturation {
			l.S = v
		} else {
			l.L = v
		}
		a, hue := h.A, h.H
		h = l.HSB()
		h.A = a
		h.H = hue // an HSL of lightness 0 or 100 has no hue to give back
	}
	return h
}

// stopsOf returns the colors of a channel's track, from its minimum to its
// maximum, with the other channels of h: the stops of the gradient behind
// a slider.
func (s *ColorState) stopsOf(h HSB, space string, ch ColorChannel) []tela.Color {
	var vs []float64
	switch ch {
	case ColorHue:
		vs = []float64{0, 60, 120, 180, 240, 300, 360}
	case ColorLightness:
		vs = []float64{0, 50, 100}
	default:
		info := infoOf(ch)
		vs = []float64{info.min, info.max}
	}
	out := make([]tela.Color, len(vs))
	for i, v := range vs {
		out[i] = s.withChannel(h, space, ch, v).Color()
	}
	return out
}

// checkSpace validates a color space option.
func checkSpace(hook, v string) string {
	return checkChoice(hook, "Space", v, "hsb", "hsb", "hsl")
}

// formatChannels is the text of an area's two channels.
func formatChannels(f func(ColorChannel, float64) string, x ColorChannel, xv float64, y ColorChannel, yv float64) string {
	return fmt.Sprintf("%s, %s", f(x, xv), f(y, yv))
}
