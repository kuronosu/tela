package raster

import (
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
)

// Fixed-point geometry. Device coordinates are snapped to a grid of 1/256
// pixel (24.8 fixed point in int64) before coverage is computed, so that the
// accumulation is exact integer arithmetic.
const (
	fracBits = 8
	one      = 1 << fracBits // one device pixel in fixed point
	// coordLimit bounds device coordinates (in pixels): geometry is clipped
	// to [-coordLimit, coordLimit] before it is snapped. With 8 fractional
	// bits the fixed-point coordinates stay below 2^28 and every product in
	// the walk below fits in int64 (at most 2^29 * 2^29).
	coordLimit = 1 << 20
	// fullCov is the accumulated value of a fully covered pixel: the area
	// unit is 1/(2*one*one) square pixels.
	fullCov = 2 * one * one
)

// FlattenTolerance is the maximum distance, in device pixels, between a
// curve and the polyline that replaces it before rasterization.
var FlattenTolerance float32 = 0.1

// fix snaps a device coordinate, already clamped to ±coordLimit, to the
// fixed-point grid (round half up). v*one is exact.
func fix(v float64) int64 { return int64(math.Floor(v*one + 0.5)) }

// cov8 converts an area in units of 1/65536 square pixels (0..65536) to an
// 8-bit coverage, rounding to nearest.
func cov8(a int64) uint8 { return uint8((a*255 + 1<<15) >> 16) }

// edge is a line of the path being filled, in fixed point, oriented
// downward: y0 < y1. dir is +1 if the original line went down, -1 if it went
// up.
type edge struct {
	x0, y0, x1, y1 int64
	dir            int64
}

// xAt returns the x coordinate of the edge at height y (y0 <= y <= y1),
// computed from the endpoints alone so that it does not depend on where the
// walk started. Division truncates toward zero, which keeps it monotonic.
func (e *edge) xAt(y int64) int64 {
	switch y {
	case e.y0:
		return e.x0
	case e.y1:
		return e.x1
	}
	return e.x0 + (y-e.y0)*(e.x1-e.x0)/(e.y1-e.y0)
}

// pathFiller collects the edges of one fill, then rasterizes them.
type pathFiller struct {
	edges  []edge
	active []int32
	acc    []int64 // accumulation row: acc[i] belongs to column cx0+i
	mask   []uint8

	// Subpath state while adding a flattened path, in device space.
	start, last [2]float64
	open        bool
}

func (f *pathFiller) reset() {
	f.edges = f.edges[:0]
	f.open = false
}

// moveTo starts a new subpath, closing the previous one.
func (f *pathFiller) moveTo(x, y float64) {
	f.closePath()
	f.start = [2]float64{x, y}
	f.last = f.start
	f.open = true
}

func (f *pathFiller) lineTo(x, y float64) {
	f.addLine(f.last[0], f.last[1], x, y)
	f.last = [2]float64{x, y}
}

// closePath closes the current subpath with a line to its start (paths are
// filled as if every subpath were closed).
func (f *pathFiller) closePath() {
	if f.open && f.last != f.start {
		f.addLine(f.last[0], f.last[1], f.start[0], f.start[1])
	}
	f.last = f.start
}

// addLine clips the device-space line to the fixed box ±coordLimit and
// stores it. The clip does not depend on the region being rendered. Parts
// above or below the box are dropped (no row of the box sees them); parts
// left or right of it are replaced by vertical lines on the box boundary,
// which keeps the winding of everything inside.
func (f *pathFiller) addLine(x0, y0, x1, y1 float64) {
	const lim = coordLimit
	if x0 != x0 || y0 != y0 || x1 != x1 || y1 != y1 { // NaN: cannot happen with finite input
		return
	}
	if y0 == y1 { // horizontal: contributes nothing
		return
	}
	if (y0 <= -lim && y1 <= -lim) || (y0 >= lim && y1 >= lim) {
		return
	}
	if y0 < -lim || y1 < -lim || y0 > lim || y1 > lim {
		// Clip with the downward orientation; clipY restores the original.
		if y0 < y1 {
			f.clipY(x0, y0, x1, y1, false)
		} else {
			f.clipY(x1, y1, x0, y0, true)
		}
		return
	}
	f.splitX(x0, y0, x1, y1)
}

// clipY clips a downward line (y0 < y1) to the box vertically. reversed says
// the original line went up.
func (f *pathFiller) clipY(x0, y0, x1, y1 float64, reversed bool) {
	const lim = coordLimit
	dx, dy := x1-x0, y1-y0
	nx0, ny0, nx1, ny1 := x0, y0, x1, y1
	if y0 < -lim {
		t := (-lim - y0) / dy
		nx0, ny0 = x0+float64(t*dx), -lim
	}
	if y1 > lim {
		t := (lim - y0) / dy
		nx1, ny1 = x0+float64(t*dx), lim
	}
	if reversed {
		f.splitX(nx1, ny1, nx0, ny0)
	} else {
		f.splitX(nx0, ny0, nx1, ny1)
	}
}

// splitX splits the line at x = ±coordLimit and replaces the outside parts
// by vertical lines on the boundary.
func (f *pathFiller) splitX(x0, y0, x1, y1 float64) {
	const lim = coordLimit
	for _, b := range [2]float64{-lim, lim} {
		if (x0 < b) != (x1 < b) && x0 != b && x1 != b {
			t := (b - x0) / (x1 - x0)
			ym := y0 + float64(t*(y1-y0))
			f.splitX(x0, y0, b, ym)
			f.splitX(b, ym, x1, y1)
			return
		}
	}
	f.addFixed(clampLim(x0), y0, clampLim(x1), y1)
}

func clampLim(x float64) float64 { return min(max(x, -coordLimit), coordLimit) }

// addFixed snaps a line inside the box and stores it as an edge.
func (f *pathFiller) addFixed(x0, y0, x1, y1 float64) {
	e := edge{x0: fix(x0), y0: fix(y0), x1: fix(x1), y1: fix(y1), dir: 1}
	if e.y0 == e.y1 {
		return
	}
	if e.y0 > e.y1 {
		e.x0, e.y0, e.x1, e.y1 = e.x1, e.y1, e.x0, e.y0
		e.dir = -1
	}
	f.edges = append(f.edges, e)
}

// rasterize fills the collected edges under rule, restricted to the device
// pixels of b.clip, and composites through b.
func (f *pathFiller) rasterize(rule gfx.FillRule, b *blitter) {
	if len(f.edges) == 0 {
		return
	}
	minX, minY := int64(math.MaxInt64), int64(math.MaxInt64)
	maxX, maxY := int64(math.MinInt64), int64(math.MinInt64)
	for i := range f.edges {
		e := &f.edges[i]
		minX = min(minX, e.x0, e.x1)
		maxX = max(maxX, e.x0, e.x1)
		minY = min(minY, e.y0)
		maxY = max(maxY, e.y1)
	}
	// Pixel bounds of the path, intersected with the clip. Left of minX and
	// right of maxX the winding is 0 (every subpath is closed).
	cx0 := max(int(minX>>fracBits), b.clip.Min.X)
	cx1 := min(int((maxX+one-1)>>fracBits), b.clip.Max.X)
	ry0 := max(int(minY>>fracBits), b.clip.Min.Y)
	ry1 := min(int((maxY+one-1)>>fracBits), b.clip.Max.Y)
	if cx0 >= cx1 || ry0 >= ry1 {
		return
	}
	w := cx1 - cx0
	if cap(f.acc) < w+1 {
		f.acc = make([]int64, w+1)
	}
	acc := f.acc[:w+1]
	clear(acc)
	if cap(f.mask) < w {
		f.mask = make([]uint8, w)
	}
	mask := f.mask[:w]

	slices.SortFunc(f.edges, func(a, b edge) int {
		switch {
		case a.y0 < b.y0:
			return -1
		case a.y0 > b.y0:
			return 1
		}
		return 0
	})
	active := f.active[:0]
	next := 0
	for y := ry0; y < ry1; y++ {
		top := int64(y) << fracBits
		bot := top + one
		for next < len(f.edges) && f.edges[next].y0 < bot {
			if f.edges[next].y1 > top {
				active = append(active, int32(next))
			}
			next++
		}
		n := 0
		for _, ei := range active {
			e := &f.edges[ei]
			accumulateRow(acc, e, top, int64(cx0), w)
			if e.y1 > bot {
				active[n] = ei
				n++
			}
		}
		active = active[:n]

		// Prefix sum -> coverage, clearing the accumulator as we go.
		var s int64
		lo, hi := w, 0
		for i := 0; i < w; i++ {
			s += acc[i]
			acc[i] = 0
			c := coverage(s, rule)
			mask[i] = c
			if c != 0 {
				lo = min(lo, i)
				hi = i + 1
			}
		}
		acc[w] = 0
		if lo < hi {
			b.mask(y, cx0+lo, mask[lo:hi])
		}
	}
	f.active = active[:0]
}

// coverage converts an accumulated signed area (units of 1/fullCov square
// pixels) to an 8-bit coverage under the fill rule.
func coverage(s int64, rule gfx.FillRule) uint8 {
	if s < 0 {
		s = -s
	}
	if rule == gfx.EvenOdd {
		s &= 2*fullCov - 1
		if s > fullCov {
			s = 2*fullCov - s
		}
	} else if s >= fullCov {
		return 255
	}
	return uint8((s*255 + fullCov/2) >> (2*fracBits + 1))
}

// accumulateRow adds the contribution of edge e within the pixel row whose
// top is top (fixed point) to acc, whose element i belongs to pixel column
// cx0+i; w columns are kept and acc has w+1 elements.
//
// For a piece of the edge inside one cell, from (xs, ys) to (xe, ye) with x
// relative to the cell's left side, the cell gets dy*(2*one - xs - xe) (twice
// the area to the right of the piece, times one) and the next cell gets the
// rest, dy*(xs + xe), so that every cell to the right sees the full
// dy*2*one. Pieces left of cx0 add their whole dy*2*one to acc[0]; pieces
// right of the kept columns are dropped.
func accumulateRow(acc []int64, e *edge, top, cx0 int64, w int) {
	ya := max(e.y0, top)
	yb := min(e.y1, top+one)
	if ya >= yb {
		return
	}
	xa, xb := e.xAt(ya), e.xAt(yb)
	sg := e.dir
	if xa == xb {
		c := xa >> fracBits
		i := c - cx0
		dy := (yb - ya) * sg
		switch {
		case i < 0:
			acc[0] += dy * 2 * one
		case i < int64(w):
			xs := 2 * (xa - c<<fracBits)
			acc[i] += dy * (2*one - xs)
			acc[i+1] += dy * xs
		}
		return
	}
	// Walk left to right; if the edge goes right to left, swap the ends and
	// negate the sign (every piece's dy changes sign).
	xl, yl, xr, yr := xa, ya, xb, yb
	if xl > xr {
		xl, yl, xr, yr = xr, yr, xl, yl
		sg = -sg
	}
	dx, dyl := xr-xl, yr-yl
	// yAt returns y at the vertical cell boundary X (xl < X < xr), in closed
	// form, so that the split at cx0 matches a full walk exactly.
	yAt := func(X int64) int64 { return yl + (X-xl)*dyl/dx }

	cl := xl >> fracBits
	cr := (xr - 1) >> fracBits // last cell with a piece of positive width
	end := cx0 + int64(w)      // first column not kept
	c := cl
	ys := yl
	if cl < cx0 {
		if cr < cx0 {
			acc[0] += dyl * sg * 2 * one
			return
		}
		ys = yAt(cx0 << fracBits)
		acc[0] += (ys - yl) * sg * 2 * one
		c = cx0
	}
	for ; c <= cr && c < end; c++ {
		X0 := c << fracBits
		xs := max(xl, X0) - X0
		xe := min(xr, X0+one) - X0
		var ye int64
		if c == cr {
			ye = yr
		} else {
			ye = yAt(X0 + one)
		}
		dy := (ye - ys) * sg
		i := c - cx0
		sum := xs + xe
		acc[i] += dy * (2*one - sum)
		acc[i+1] += dy * sum
		ys = ye
	}
}
