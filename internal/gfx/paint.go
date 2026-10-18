package gfx

import "math"

// Paint describes how a shape is filled or stroked: a solid Color or one of
// the gradients. The interface is sealed: only this package defines Paints.
//
// Coordinates inside a Paint (gradient points, centers, radii) are in the
// local coordinate space of the command that uses it, so they follow the
// current transform like the geometry does.
type Paint interface {
	isPaint()
}

func (Color) isPaint()          {}
func (LinearGradient) isPaint() {}
func (RadialGradient) isPaint() {}
func (ConicGradient) isPaint()  {}
func (FocalGradient) isPaint()  {}

// ColorSpace is the color space in which gradient stops are interpolated.
// Compositing itself always happens in sRGB (ADR-007).
//
// Stop colors are extended sRGB (see Color) and are converted to the
// interpolation space without clipping: in Oklab through the sign-symmetric
// sRGB transfer function, in sRGB as they are. Each interpolated pixel is
// converted back to sRGB and clipped per channel to [0, 1]. Chromium 152
// does the same in both spaces (measured with out-of-gamut Tailwind colors:
// within 1 level, against up to 47 levels when the stops are clipped
// first).
type ColorSpace uint8

const (
	// InterpOklab interpolates in Oklab, like Tailwind v4's default
	// "in oklab" gradients (ADR-023). It is the zero value.
	InterpOklab ColorSpace = iota
	// InterpSRGB interpolates the sRGB-encoded components directly, like
	// classic CSS gradients ("in srgb").
	InterpSRGB
	// An Oklch variant with a hue interpolation method may be added after
	// InterpSRGB.
)

// String returns "oklab" or "srgb".
func (c ColorSpace) String() string {
	switch c {
	case InterpOklab:
		return "oklab"
	case InterpSRGB:
		return "srgb"
	}
	return "ColorSpace(?)"
}

// GradientStop is a color at an offset along the gradient. Offset 0 is the
// start and 1 the end of the gradient line (linear), the radius (radial) or
// the full turn (conic).
type GradientStop struct {
	Offset float32
	Color  Color
}

// AutoStop returns a stop whose offset is left to NormalizeStops to compute
// (CSS color stops without a position). Its Offset is NaN.
func AutoStop(c Color) GradientStop { return GradientStop{Offset: float32(math.NaN()), Color: c} }

// NormalizeStops returns a new slice with the CSS color-stop fix-up applied
// (CSS Images 3, section 3.5.3), so that the offsets are all in [0, 1] and
// non-decreasing:
//
//  1. a first stop with an automatic offset (NaN, see AutoStop) gets 0 and a
//     last one gets 1;
//  2. an offset that is lower than the largest offset before it is raised to
//     that value (so equal offsets form a hard stop);
//  3. runs of stops with automatic offsets are spread evenly between the
//     stops that surround them;
//  4. offsets are clamped to [0, 1]. (CSS allows stops outside that range
//     and clips the gradient; Tela clamps, which differs only when the
//     visible range would have shown an interpolated color at 0 or 1.)
//
// The input is not modified. Zero stops give nil: a gradient without stops
// paints nothing; a single stop paints that color. Colors are copied
// unchanged.
func NormalizeStops(stops []GradientStop) []GradientStop {
	if len(stops) == 0 {
		return nil
	}
	out := make([]GradientStop, len(stops))
	copy(out, stops)
	nan := func(v float32) bool { return v != v }
	if nan(out[0].Offset) {
		out[0].Offset = 0
	}
	if last := len(out) - 1; nan(out[last].Offset) {
		out[last].Offset = 1
	}
	// Step 4 first for explicit offsets, then 2: both are order-independent
	// combined with the auto fill below as long as we clamp before comparing.
	for i := range out {
		if !nan(out[i].Offset) {
			out[i].Offset = clamp01(out[i].Offset)
		}
	}
	// Rule 2, only over explicit offsets.
	maxSoFar := float32(0)
	for i := range out {
		if nan(out[i].Offset) {
			continue
		}
		if out[i].Offset < maxSoFar {
			out[i].Offset = maxSoFar
		}
		maxSoFar = out[i].Offset
	}
	// Rule 3: spread runs of auto offsets between their explicit neighbors.
	for i := 0; i < len(out); {
		if !nan(out[i].Offset) {
			i++
			continue
		}
		j := i
		for nan(out[j].Offset) {
			j++
		}
		// out[i-1] and out[j] are explicit (the ends were fixed above).
		lo, hi := out[i-1].Offset, out[j].Offset
		for k := i; k < j; k++ {
			out[k].Offset = lo + (hi-lo)*float32(k-i+1)/float32(j-i+1)
		}
		i = j
	}
	return out
}

// LinearGradient is a gradient along the line from Start to End: the color at
// a point is the one at the offset of its orthogonal projection on the line
// (CSS linear-gradient; the style layer converts angles and corner keywords
// into Start and End). Colors outside the line are padded with the first and
// last stop colors.
//
// Interpolation between stops is done on premultiplied colors (CSS Color 4,
// section 12.3), in the color space Interp. If Start == End the gradient
// paints the last stop's color.
type LinearGradient struct {
	Start, End Point
	Stops      []GradientStop
	Interp     ColorSpace
}

// RadialGradient is a gradient around Center. Radius holds the horizontal and
// vertical radii of the ending shape, an axis-aligned ellipse in local
// coordinates (equal for a circle); the color at a point is the one at the
// offset d, where d is the point's distance to the center divided by the
// radius of the ellipse in its direction (CSS radial-gradient). Points
// outside the ellipse are padded with the last stop color.
//
// Interpolation is premultiplied, in color space Interp. If either radius is
// not positive the gradient paints the last stop's color.
type RadialGradient struct {
	Center Point
	Radius Point
	Stops  []GradientStop
	Interp ColorSpace
}

// ConicGradient is a gradient sweeping around Center (CSS conic-gradient).
// StartAngle is in radians with the CSS convention: 0 points up (toward -Y)
// and positive angles turn clockwise on screen. Offset 0 is at StartAngle and
// offset 1 is one full turn later.
//
// Interpolation is premultiplied, in color space Interp.
type ConicGradient struct {
	Center     Point
	StartAngle float32
	Stops      []GradientStop
	Interp     ColorSpace
}

// FocalGradient is a two-point conical gradient: a gradient between the
// focal circle (Focal, FocalRadius), at offset 0, and the end circle
// (Center, Radius), at offset 1. It is SVG's radialGradient with fx, fy and
// fr, Canvas's createRadialGradient and COLRv1's PaintRadialGradient.
// Ellipses come from a transform.
//
// The circles C(t) = (Focal + t·(Center - Focal), FocalRadius +
// t·(Radius - FocalRadius)) interpolate the two circles for every real t.
// A point takes the color at offset t of the largest t whose circle C(t)
// passes through it with a radius of 0 or more; offsets beyond [0, 1]
// take the first or last stop color (pad). Points that no such circle
// passes through are transparent: when the focal circle lies inside the end
// circle (the usual focal gradient) every point has one, otherwise the
// circles sweep a cone and the plane outside it is not painted.
//
// Negative radii are treated as 0. If both circles are equal the gradient
// paints nothing. Interpolation is premultiplied, in color space Interp.
type FocalGradient struct {
	Focal       Point
	FocalRadius float32
	Center      Point
	Radius      float32
	Stops       []GradientStop
	Interp      ColorSpace
}

// sanitizePaint validates a Paint for the Builder and returns a copy that
// shares no mutable memory with the argument: solid colors are clamped to
// [0, 1], stop slices copied and normalized, with stop alphas clamped and
// stop colors kept in extended sRGB. ok is false if the paint is invalid or paints
// nothing (non-finite geometry or colors, a nil paint, a fully transparent
// solid color).
func sanitizePaint(p Paint) (Paint, bool) {
	switch p := p.(type) {
	case Color:
		if !p.IsFinite() {
			return nil, false
		}
		p = p.Clamp()
		return p, p.A > 0
	case LinearGradient:
		if !p.Start.IsFinite() || !p.End.IsFinite() {
			return nil, false
		}
		var ok bool
		p.Stops, ok = sanitizeStops(p.Stops)
		return p, ok
	case RadialGradient:
		if !p.Center.IsFinite() || !p.Radius.IsFinite() {
			return nil, false
		}
		var ok bool
		p.Stops, ok = sanitizeStops(p.Stops)
		return p, ok
	case ConicGradient:
		if !p.Center.IsFinite() || !finite(p.StartAngle) {
			return nil, false
		}
		var ok bool
		p.Stops, ok = sanitizeStops(p.Stops)
		return p, ok
	case FocalGradient:
		if !p.Focal.IsFinite() || !p.Center.IsFinite() || !finite(p.FocalRadius) || !finite(p.Radius) {
			return nil, false
		}
		p.FocalRadius, p.Radius = max(p.FocalRadius, 0), max(p.Radius, 0)
		if p.Focal == p.Center && p.FocalRadius == p.Radius {
			return nil, false // equal circles: nothing is painted
		}
		var ok bool
		p.Stops, ok = sanitizeStops(p.Stops)
		return p, ok
	}
	return nil, false
}

func sanitizeStops(stops []GradientStop) ([]GradientStop, bool) {
	out := NormalizeStops(stops) // copies
	if len(out) == 0 {
		return nil, false
	}
	visible := false
	for i := range out {
		if !out[i].Color.IsFinite() {
			return nil, false
		}
		// Stop colors keep their extended sRGB components: gradients
		// interpolate them unclipped and clip each pixel, like Chromium.
		out[i].Color.A = clamp01(out[i].Color.A)
		visible = visible || out[i].Color.A > 0
	}
	return out, visible
}
