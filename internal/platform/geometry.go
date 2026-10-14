package platform

import (
	"fmt"
	"image"
	"math"
)

// Point is a position in logical pixels, relative to the top-left corner of a
// window's content area (the area Tela draws; OS decorations excluded).
type Point struct {
	X, Y float32
}

// Size is a size in logical pixels. It may be fractional: backends whose
// native unit is the physical pixel (Win32, X11) derive it as
// physical / scale (see [LogicalSize]).
type Size struct {
	W, H float32
}

// Rect is a rectangle in logical pixels: its top-left corner (X, Y),
// relative to the content area like [Point], and its size (W, H).
type Rect struct {
	X, Y, W, H float32
}

func (r Rect) String() string { return fmt.Sprintf("(%g,%g %gx%g)", r.X, r.Y, r.W, r.H) }

// Empty reports whether s has no area.
func (s Size) Empty() bool { return !(s.W > 0 && s.H > 0) }

func (s Size) String() string { return fmt.Sprintf("%gx%g", s.W, s.H) }

// Metrics is the geometry of a window: its logical size and its display
// scale. The framebuffer size follows from it via [Metrics.Physical], which
// is the single sizing rule every backend uses.
type Metrics struct {
	// Size is the logical size of the content area.
	Size Size
	// Scale is the number of physical pixels per logical pixel (1, 2, or
	// fractional such as 1.25). Always > 0 once delivered.
	Scale float64
}

// Physical returns the framebuffer size in physical pixels:
// PhysicalSize(m.Size, m.Scale).
func (m Metrics) Physical() image.Point { return PhysicalSize(m.Size, m.Scale) }

func (m Metrics) String() string {
	p := m.Physical()
	return fmt.Sprintf("%v@%gx (%dx%d px)", m.Size, m.Scale, p.X, p.Y)
}

// halfTolerance is how close to an exact .5 a product must be to be treated
// as one. It absorbs float error (scales like 13/12 are not exactly
// representable) while staying far below 1/240, the smallest non-zero
// distance from .5 that an integer size times an n/120 Wayland fractional
// scale can have.
const halfTolerance = 1e-6

// PhysicalSize is the one sizing rule of the platform layer: the physical
// size of a window whose logical size is logical, at the given scale.
//
// Each axis is logical × scale, computed in float64 and rounded to the
// nearest integer, halfway cases away from zero. This is the rule
// wp_fractional_scale_v1 mandates for toplevel buffers, so on Wayland the
// result matches what the compositor expects for both integer
// (wl_surface.set_buffer_scale) and fractional (wp_viewporter) scaling.
// Products within 1e-6 of a half count as a half, so a scale that is not
// exactly representable (for example 130/120) still rounds like the exact
// rational would. Wayland backends must compute the scale as
// float64(n)/120 from the protocol's integer.
//
// Negative and NaN inputs yield 0. scale must be > 0; PhysicalSize panics
// otherwise, because a non-positive scale is always a bug in the caller.
func PhysicalSize(logical Size, scale float64) image.Point {
	checkScale(scale)
	return image.Point{X: physicalAxis(logical.W, scale), Y: physicalAxis(logical.H, scale)}
}

func physicalAxis(l float32, scale float64) int {
	if !(l > 0) { // also catches NaN
		return 0
	}
	v := float64(l) * scale
	if math.IsInf(v, 0) || v > math.MaxInt32 {
		return math.MaxInt32
	}
	f := math.Floor(v)
	if math.Abs(v-f-0.5) <= halfTolerance {
		return int(f) + 1
	}
	return int(math.Round(v))
}

// LogicalSize returns physical / scale per axis. Backends whose native unit
// is the physical pixel (Win32 client rects, X11 ConfigureNotify) use it to
// report the logical size; PhysicalSize(LogicalSize(p, s), s) == p for every
// p whose axes are below 2^22 and every s in [1/16, 16].
func LogicalSize(physical image.Point, scale float64) Size {
	checkScale(scale)
	return Size{W: logicalAxis(physical.X, scale), H: logicalAxis(physical.Y, scale)}
}

func logicalAxis(p int, scale float64) float32 {
	if p <= 0 {
		return 0
	}
	return float32(float64(p) / scale)
}

func checkScale(scale float64) {
	if !(scale > 0) || math.IsInf(scale, 0) {
		panic(fmt.Sprintf("platform: invalid display scale %v; a scale must be a finite number > 0 (1 for standard displays, 2 for HiDPI)", scale))
	}
}
