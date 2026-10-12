// Copyright 2021 The go-text authors. Licensed under the BSD 3-Clause
// license (github.com/go-text/typesetting, SPDX: Unlicense OR BSD-3-Clause).
// Ported from font/glyphs.go (getPointsForGlyphRec, transformPoints),
// font/variations.go (gvar parsing and applyDeltasToPoints, itself adapted
// from HarfBuzz's hb-ot-var-gvar-table.hh) and font/renderer.go
// (buildSegments, adapted from sfnt/truetype.go), with fusion-safe float64
// arithmetic; see THIRD_PARTY.md.

package text

import (
	"encoding/binary"
	"errors"

	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/gfx"
)

// This file loads TrueType ('glyf') outlines with their 'gvar' variations.
//
// go-text can do it (font.Face.GlyphDataOutline), but its delta arithmetic
// is float32 code of the form x += float32(d) * scalar, which Go may fuse
// into multiply-adds on arm64 and on amd64 with GOAMD64=v3: the outlines of
// a variable font at non-default coordinates then differ in their last bits
// between architectures (measured on Inter at wght 650, opsz 20), and so do
// the pixels. The port below computes in float64 and rounds every product
// that feeds an addition explicitly (ADR-027), so the outlines are
// bit-identical everywhere.

// Phantom points appended after the contour points of every glyph.
const (
	phantomLeft = iota
	phantomRight
	phantomTop
	phantomBottom
	phantomCount
)

const maxCompositeNesting = 20 // protects against malicious fonts

// glyfPoint is a contour point in font units (y up).
type glyfPoint struct {
	x, y     float64
	onCurve  bool
	endPoint bool // last point of its contour
	explicit bool // gvar: the tuple gives a delta for this point
}

// glyfTables holds the tables needed to load TrueType outlines.
type glyfTables struct {
	glyf tables.Glyf
	hmtx tables.Hmtx
	upem int
	// gvar, nil if the font has no glyph variations.
	sharedTuples [][]tables.Coord
	variations   []tables.GlyphVariationData
}

// points returns the contour points of glyph gid at coords followed by its
// four phantom points, shifted so that the left phantom point is at x = 0
// (the "undocumented rasterizer behavior" of HarfBuzz and FreeType).
func (g *glyfTables) points(gid tables.GlyphID, coords []tables.Coord) []glyfPoint {
	var out []glyfPoint
	g.pointsRec(gid, coords, 0, map[tables.GlyphID]bool{}, &out)
	return out
}

func (g *glyfTables) pointsRec(gid tables.GlyphID, coords []tables.Coord, depth int, seen map[tables.GlyphID]bool, out *[]glyfPoint) {
	if depth > maxCompositeNesting || int(gid) >= len(g.glyf) {
		*out = append(*out, make([]glyfPoint, phantomCount)...)
		return
	}
	gl := g.glyf[gid]

	var pts []glyfPoint
	switch d := gl.Data.(type) {
	case tables.SimpleGlyph:
		pts = make([]glyfPoint, len(d.Points), len(d.Points)+phantomCount)
		for _, e := range d.EndPtsOfContours {
			if int(e) < len(pts) {
				pts[e].endPoint = true
			}
		}
		for i, p := range d.Points {
			pts[i].x, pts[i].y = float64(p.X), float64(p.Y)
			pts[i].onCurve = p.Flag&1 != 0
		}
	case tables.CompositeGlyph:
		// One pseudo point per component: gvar varies the component offsets.
		pts = make([]glyfPoint, len(d.Glyphs), len(d.Glyphs)+phantomCount)
	}
	pts = append(pts, make([]glyfPoint, phantomCount)...)
	ph := pts[len(pts)-phantomCount:]

	adv := g.upem / 2 // HarfBuzz's default when hmtx is missing
	if !g.hmtx.IsEmpty() {
		adv = int(g.hmtx.Advance(gid))
	}
	hDelta := float64(int(gl.XMin) - int(g.hmtx.SideBearing(gid)))
	ph[phantomLeft].x = hDelta
	ph[phantomRight].x = float64(adv) + hDelta

	if len(coords) != 0 && g.variations != nil {
		g.applyDeltas(gid, coords, pts)
	}

	switch d := gl.Data.(type) {
	case tables.SimpleGlyph:
		*out = append(*out, pts...)
	case tables.CompositeGlyph:
		for ci, item := range d.Glyphs {
			if seen[item.GlyphIndex] {
				continue
			}
			seen[item.GlyphIndex] = true

			var comp []glyfPoint
			g.pointsRec(item.GlyphIndex, coords, depth+1, seen, &comp)
			n := len(comp) // at least phantomCount

			if item.HasUseMyMetrics() {
				copy(ph, comp[n-phantomCount:])
			}
			transformComponent(&item, comp)
			// gvar deltas of the component offset.
			translatePoints(comp, pts[ci].x, pts[ci].y)
			if item.IsAnchored() {
				p1, p2 := item.ArgsAsIndices()
				if p1 < len(*out) && p2 < n {
					translatePoints(comp, (*out)[p1].x-comp[p2].x, (*out)[p1].y-comp[p2].y)
				}
			}
			*out = append(*out, comp[:n-phantomCount]...)
			delete(seen, item.GlyphIndex)
		}
		*out = append(*out, ph...)
	default: // empty glyph
		*out = append(*out, ph...)
	}

	if depth == 0 {
		translatePoints(*out, -ph[phantomLeft].x, 0)
	}
}

func translatePoints(pts []glyfPoint, dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	for i := range pts {
		pts[i].x += dx
		pts[i].y += dy
	}
}

// transformComponent applies a component's offset and 2x2 matrix.
func transformComponent(c *tables.CompositeGlyphPart, pts []glyfPoint) {
	var tx, ty float64
	if !c.IsAnchored() {
		a1, a2 := c.ArgsAsTranslation()
		tx, ty = float64(a1), float64(a2)
	}
	m := c.Scale // exact F2Dot14 values
	if tx == 0 && ty == 0 && m == [4]float32{1, 0, 0, 1} {
		return
	}
	m0, m1, m2, m3 := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	apply := func(p *glyfPoint) {
		x, y := p.x, p.y
		p.x = float64(x*m0) + float64(y*m2)
		p.y = float64(x*m1) + float64(y*m3)
	}
	if c.IsScaledOffsets() {
		for i := range pts {
			pts[i].x += tx
			pts[i].y += ty
			apply(&pts[i])
		}
	} else {
		for i := range pts {
			apply(&pts[i])
			pts[i].x += tx
			pts[i].y += ty
		}
	}
}

// tupleVar is one parsed tuple variation of a glyph.
type tupleVar struct {
	peak, start, end []tables.Coord
	points           []uint16 // nil: every point
	deltas           []int16  // x deltas then y deltas
}

// applyDeltas adds the gvar deltas of glyph gid at coords to pts (contour
// points and phantom points), inferring the deltas of unreferenced points
// (IUP).
func (g *glyfTables) applyDeltas(gid tables.GlyphID, coords []tables.Coord, pts []glyfPoint) {
	if int(gid) >= len(g.variations) {
		return
	}
	vd := g.variations[gid]
	if len(vd.TupleVariationHeaders) == 0 {
		return
	}
	tuples, err := g.parseTuples(&vd, len(pts))
	if err != nil {
		return
	}
	orig := append([]glyfPoint(nil), pts...)
	deltas := make([]glyfPoint, len(pts))
	for _, t := range tuples {
		scalar := tupleScalar(coords, t.peak, t.start, t.end)
		if scalar == 0 {
			continue
		}
		clear(deltas)
		half := len(t.deltas) / 2
		xd, yd := t.deltas[:half], t.deltas[half:]
		for i := range xd {
			idx := i
			if t.points != nil {
				idx = int(t.points[i])
			}
			if idx >= len(deltas) {
				continue
			}
			deltas[idx].explicit = true
			deltas[idx].x += float64(float64(xd[i]) * scalar)
			deltas[idx].y += float64(float64(yd[i]) * scalar)
		}
		if t.points != nil {
			inferDeltas(orig, deltas)
		}
		for i, d := range deltas {
			pts[i].x += d.x
			pts[i].y += d.y
		}
	}
}

// inferDeltas interpolates the deltas of the points that a tuple does not
// reference (gvar "Inferred deltas for un-referenced point numbers").
// Contour ends come from orig; phantom points are never inferred.
func inferDeltas(orig, deltas []glyfPoint) {
	start := 0
	for start < len(orig) {
		end := start
		for end < len(orig) && !orig[end].endPoint {
			end++
		}
		if end == len(orig) {
			return
		}
		unref := 0
		for _, d := range deltas[start : end+1] {
			if !d.explicit {
				unref++
			}
		}
		if unref != 0 && unref <= end-start {
			j := start
			for unref > 0 {
				// The next gap of unreferenced points between two referenced
				// points prev and next; gaps may wrap around the contour.
				var i int
				for {
					i = j
					j = nextIndex(i, start, end)
					if deltas[i].explicit && !deltas[j].explicit {
						break
					}
				}
				prev := i
				j = i
				for {
					i = j
					j = nextIndex(i, start, end)
					if !deltas[i].explicit && deltas[j].explicit {
						break
					}
				}
				next := j
				for i = nextIndex(prev, start, end); i != next; i = nextIndex(i, start, end) {
					deltas[i].x = inferDelta(orig[i].x, orig[prev].x, orig[next].x, deltas[prev].x, deltas[next].x)
					deltas[i].y = inferDelta(orig[i].y, orig[prev].y, orig[next].y, deltas[prev].y, deltas[next].y)
					unref--
				}
			}
		}
		start = end + 1
	}
}

func nextIndex(i, start, end int) int {
	if i >= end {
		return start
	}
	return i + 1
}

func inferDelta(target, prev, next, prevDelta, nextDelta float64) float64 {
	switch {
	case prev == next:
		if prevDelta == nextDelta {
			return prevDelta
		}
		return 0
	case target <= min(prev, next):
		if prev < next {
			return prevDelta
		}
		return nextDelta
	case target >= max(prev, next):
		if prev > next {
			return prevDelta
		}
		return nextDelta
	}
	r := (target - prev) / (next - prev)
	return prevDelta + float64(r*(nextDelta-prevDelta))
}

var errGvarData = errors.New("text: invalid gvar data")

// parseTuples parses the serialized tuple variation data of one glyph;
// numPoints counts its points including the phantom points.
func (g *glyfTables) parseTuples(vd *tables.GlyphVariationData, numPoints int) ([]tupleVar, error) {
	data := vd.SerializedData
	var shared []uint16
	var err error
	if vd.HasSharedPointNumbers() {
		if shared, data, err = parsePointNumbers(data); err != nil {
			return nil, err
		}
	}
	out := make([]tupleVar, len(vd.TupleVariationHeaders))
	for i := range vd.TupleVariationHeaders {
		h := &vd.TupleVariationHeaders[i]
		if len(data) < int(h.VariationDataSize) {
			return nil, errGvarData
		}
		next := data[h.VariationDataSize:]
		t := &out[i]
		t.peak = h.PeakTuple.Values
		if t.peak == nil {
			idx := int(h.Index())
			if idx >= len(g.sharedTuples) {
				return nil, errGvarData
			}
			t.peak = g.sharedTuples[idx]
		}
		if h.IntermediateTuples[0].Values != nil {
			t.start, t.end = h.IntermediateTuples[0].Values, h.IntermediateTuples[1].Values
		}
		t.points = shared
		if h.HasPrivatePointNumbers() {
			if t.points, data, err = parsePointNumbers(data); err != nil {
				return nil, err
			}
		}
		count := numPoints
		if t.points != nil {
			count = len(t.points)
		}
		if t.deltas, err = unpackDeltas(data, 2*count); err != nil {
			return nil, err
		}
		data = next
	}
	return out, nil
}

// parsePointNumbers reads packed point numbers. The slice is nil when the
// data means "all points".
func parsePointNumbers(data []byte) ([]uint16, []byte, error) {
	if len(data) < 1 {
		return nil, nil, errGvarData
	}
	var count int
	switch {
	case data[0] == 0:
		return nil, data[1:], nil
	case data[0]&0x80 == 0:
		count = int(data[0])
		data = data[1:]
	default:
		if len(data) < 2 {
			return nil, nil, errGvarData
		}
		count = int(data[0]&0x7F)<<8 | int(data[1])
		data = data[2:]
	}
	points := make([]uint16, 0, count)
	var last uint16
	for len(points) < count {
		if len(data) == 0 {
			return nil, nil, errGvarData
		}
		control := data[0]
		run := int(control&0x7F) + 1
		data = data[1:]
		if control&0x80 != 0 {
			if len(data) < 2*run {
				return nil, nil, errGvarData
			}
			for k := 0; k < run; k++ {
				last += binary.BigEndian.Uint16(data[2*k:])
				points = append(points, last)
			}
			data = data[2*run:]
		} else {
			if len(data) < run {
				return nil, nil, errGvarData
			}
			for k := 0; k < run; k++ {
				last += uint16(data[k])
				points = append(points, last)
			}
			data = data[run:]
		}
	}
	return points, data, nil
}

// unpackDeltas reads n packed deltas.
func unpackDeltas(data []byte, n int) ([]int16, error) {
	out := make([]int16, n)
	read := 0
	for read < n {
		if len(data) == 0 {
			return nil, errGvarData
		}
		control := data[0]
		count := int(control&0x3F) + 1
		data = data[1:]
		if read+count > n {
			return nil, errGvarData
		}
		switch {
		case control&0x80 != 0: // zeros
		case control&0x40 != 0: // words
			if len(data) < 2*count {
				return nil, errGvarData
			}
			for k := 0; k < count; k++ {
				out[read+k] = int16(binary.BigEndian.Uint16(data[2*k:]))
			}
			data = data[2*count:]
		default: // bytes
			if len(data) < count {
				return nil, errGvarData
			}
			for k := 0; k < count; k++ {
				out[read+k] = int16(int8(data[k]))
			}
			data = data[count:]
		}
		read += count
	}
	return out, nil
}

// glyfPath converts contour points (without the phantom points) to an em
// space path: x / upem, -y / upem. An on-curve point is implied between two
// consecutive off-curve points. Every contour is closed.
func glyfPath(pts []glyfPoint, upem float64) gfx.Path {
	var p gfx.Path
	em := func(q glyfPoint) gfx.Point { return gfx.Point{X: float32(q.x / upem), Y: float32(-q.y / upem)} }
	mid := func(a, b glyfPoint) glyfPoint {
		return glyfPoint{x: (a.x + b.x) / 2, y: (a.y + b.y) / 2, onCurve: true}
	}

	for start := 0; start < len(pts); {
		end := start
		for end < len(pts)-1 && !pts[end].endPoint {
			end++
		}
		c := pts[start : end+1]
		start = end + 1
		n := len(c)

		// Start at the first on-curve point; a contour made of off-curve
		// points only starts at the implied point between its last and
		// first points.
		first := -1
		for i := range c {
			if c[i].onCurve {
				first = i
				break
			}
		}
		var s glyfPoint
		k0 := 0
		if first >= 0 {
			s, k0 = c[first], first+1
		} else {
			s, first = mid(c[n-1], c[0]), 0
		}
		p.MoveTo(em(s))
		var ctrl glyfPoint
		haveCtrl := false
		for k := k0; k < first+n; k++ {
			q := c[k%n]
			if q.onCurve {
				if haveCtrl {
					p.QuadTo(em(ctrl), em(q))
				} else {
					p.LineTo(em(q))
				}
				haveCtrl = false
				continue
			}
			if haveCtrl {
				p.QuadTo(em(ctrl), em(mid(ctrl, q)))
			}
			ctrl, haveCtrl = q, true
		}
		if haveCtrl {
			p.QuadTo(em(ctrl), em(s))
		}
		p.Close()
	}
	return p
}
