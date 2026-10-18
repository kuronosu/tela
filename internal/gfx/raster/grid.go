package raster

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Translations onto the device pixel grid.
//
// A translation by whole device pixels is a whole number of logical pixels
// times 1/scale. At integer scales that is exact in float32, but at a
// fractional scale it usually is not: a scroll offset of 80 device pixels
// at 1.5 is the float32 53.333332, which maps to 79.999998 device pixels.
// Content moved by it would land a hair off the grid, so rasterizing it
// again after a scroll would not give the pixels of the previous frame
// shifted by 80, and an incremental frame could not shift them (core's
// damage). So a pure translation pushed while the current transform is an
// axis-aligned scale plus a whole-pixel translation moves by whole device
// pixels when its device displacement is within GridTolerance of them: the
// displacement is rounded, and the current transform stays on the grid.
// The decision depends only on the scale and the translation, not on where
// the content already is, so the same push is placed the same way in every
// frame. At integer scales whole-pixel translations are exact and nothing
// changes; a sub-pixel offset this small never moved a vertex by more than
// the 1/256 px fixed-point grid of the coverage computation anyway.

// GridTolerance returns how far from whole device pixels a translation of
// v device pixels may be and still be placed on them: 1/1024 px, growing
// with |v| to absorb the relative error of float32 logical offsets
// (|v|/2^20: 0.1 px at 100,000 px) and capped at 1/8 px.
func GridTolerance(v float64) float64 {
	return min(max(1.0/1024, math.Abs(v)/(1<<20)), 1.0/8)
}

// GridStep returns the device displacement of a translation by t logical
// pixels at scale s rounded to whole device pixels, and whether it is
// within GridTolerance of them (if not, the rasterizer moves the content by
// the exact product).
func GridStep(s, t float32) (float32, bool) {
	v := float64(s) * float64(t) // exact: a product of two float32 values
	r := math.Round(v)
	return float32(r), math.Abs(v-r) <= GridTolerance(v)
}

// GridMul returns ctm.Mul(m) for a pushed transform m, placed on the device
// pixel grid as described above: when m is a pure translation, ctm an
// axis-aligned scale plus a whole-pixel translation, and an axis's device
// displacement within GridTolerance of whole pixels, that axis moves by the
// rounded displacement. Core's damage computes the translations of scrolled
// content with it, so it predicts the rasterizer exactly.
func GridMul(ctm, m gfx.Affine) gfx.Affine {
	next := ctm.Mul(m)
	if m[0] != 1 || m[1] != 0 || m[2] != 0 || m[3] != 1 || ctm[1] != 0 || ctm[2] != 0 {
		return next
	}
	if e := ctm[4]; e == float32(math.Round(float64(e))) {
		if d, ok := GridStep(ctm[0], m[4]); ok {
			next[4] = e + d // whole numbers: exact
		}
	}
	if e := ctm[5]; e == float32(math.Round(float64(e))) {
		if d, ok := GridStep(ctm[3], m[5]); ok {
			next[5] = e + d
		}
	}
	return next
}
