package raster

import "github.com/kuronosu/tela/internal/gfx"

// What the GPU renderer (internal/gfx/gpu, ADR-282) shares with the
// rasterizer: the pieces whose values must be the same on both backends
// and that are not worth computing twice. The geometry of fills, gradients
// and shadows is evaluated per pixel by the GPU's own shaders; the colors
// of a gradient and the shape of a shadow come from here.

// RampSize is the number of colors in the lookup table of one segment of a
// gradient (see Ramp).
const RampSize = rampSize

// ShadowCutoff is where the Gaussian of an analytic shadow is cut, in
// standard deviations: beyond it a blurred edge is exactly 0 or 1.
const ShadowCutoff = shadowK

// MinSigma is the smallest device standard deviation that blurs at all: a
// shadow below it is the shape's coverage, unblurred.
const MinSigma = sigmaMin

// Ramp is the color lookup of a gradient's stops, the one the rasterizer
// paints with (see the comment at the top of gradient.go): one table of
// RampSize premultiplied RGBA8 colors per segment, a segment being two
// consecutive stops at different offsets. It is immutable.
//
// The color at the gradient parameter t, already folded by the spread
// mode, is: First when !(t >= Lo); Last when t >= Hi; otherwise the
// entry int(t*Scale + Bias), clamped to the table, of the last segment
// whose Offset is at most t.
type Ramp struct {
	// Lo and Hi are the offsets of the first and the last stop.
	Lo, Hi float64
	// First and Last are the colors before Lo and from Hi on. They are not
	// always the ends of the tables: stops at equal offsets (hard stops)
	// make no segment.
	First, Last [4]uint8
	// Segments are in increasing order of Offset; there are none when
	// every stop has the same offset.
	Segments []RampSegment
}

// RampSegment is the part of a Ramp between two stops.
type RampSegment struct {
	// Offset is the offset of the segment's first stop.
	Offset float64
	// Scale and Bias map t to an entry: int(t*Scale + Bias), which is
	// round((t - Offset) * (RampSize-1) / (length of the segment)).
	Scale, Bias float64
	// Colors holds the premultiplied RGBA8 colors; the first and the last
	// are the stop colors exactly.
	Colors *[RampSize][4]uint8
}

// GradientRamp bakes the ramp of normalized stops (gfx.NormalizeStops; at
// least two, as the Builder stores them) interpolated in the color space
// interp. The result does not share memory with stops.
func GradientRamp(stops []gfx.GradientStop, interp gfx.ColorSpace) Ramp {
	rp := newRamp(stops, interp)
	out := Ramp{Lo: rp.lo, Hi: rp.hi, First: rp.first, Last: rp.last, Segments: make([]RampSegment, len(rp.segs))}
	for i := range rp.segs {
		s := &rp.segs[i]
		out.Segments[i] = RampSegment{Offset: s.o0, Scale: s.scale, Bias: s.bias, Colors: &s.lut}
	}
	return out
}

// ShadowShape returns the shape a DrawShadow blurs, in the command's local
// space: the box translated by the offset and grown by the spread (outer)
// or shrunk by it (inset), with the CSS spread rule for the radii (see
// shadow.go). ok is false when the shape is empty: an outer shadow then
// paints nothing, and an inset one fills the whole box.
func ShadowShape(s gfx.DrawShadow) (shape gfx.RRect, ok bool) { return shadowShape(s) }
