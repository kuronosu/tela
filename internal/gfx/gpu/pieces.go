package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// The pieces of a shape's coverage.
//
// The coverage of a path is 0 or 255 wherever no edge of the path comes
// near: the border of a panel, a ring of two rounded rectangles, covers
// millions of pixels of which a few thousand are not one or the other. A
// shape larger than a few cells is therefore cut along a grid of cells of
// coverCell pixels, on which the edges of the path are marked (a line by
// the boxes of its halves, a curve by the boxes of its control points,
// halved until they are small; a pixel of margin for the snapping to the
// rasterizer's grid). The cells an edge may touch are gathered into
// rectangles, the pieces that hold pixels: each is rasterized, uploaded
// and drawn on its own, as a tile was. The other cells are gathered into
// rectangles too, and one pixel of each tells whether it is inside the
// shape (a piece drawn as a plain rectangle, without the atlas) or outside
// (nothing): no edge crosses such a rectangle, so its winding is that of
// any of its pixels.
//
// A piece whose cells hold nothing but vertical edges that cross it from
// top to bottom (the sides of a border) has the same coverage on every
// row, since no edge starts, ends or crosses a row inside it: one row is
// rasterized and repeated, where the rasterizer would walk its edges for
// every row of a pane's height.
//
// The pieces depend on the shape only, never on the region rendered, and
// the pixels of a piece are the rasterizer's for those device pixels
// (raster.Masker gives the same values whatever the rectangle asked for),
// so a shape drawn by pieces has the pixels it had when it was drawn by
// tiles.

// coverCell is the size of the cells a shape is cut along.
const coverCell = 16

// maxPieces bounds the pieces of a shape: one cut into more (a hatching)
// is drawn by tiles, whole.
const maxPieces = 512

// maxPieceCells bounds the cells of the grid of a shape: one of more (a
// path far larger than a screen) is drawn by tiles.
const maxPieceCells = 1 << 18

// coverPiece is a part of a shape's coverage: the pixels rel, relative to
// the whole pixels of the shape's translation. full: every pixel is
// covered entirely, and nothing is in the atlas.
type coverPiece struct {
	rel  image.Rectangle
	full bool
	// rows: every row of the piece has the coverage of its first.
	rows bool
}

// What a cell of a pieceGrid holds.
const (
	cellEmpty = 0 // no edge comes near
	// cellEdge: an edge may touch the cell.
	cellEdge = 1
	// cellUneven: an edge that is not a vertical line through the whole
	// height of the cell may touch it, so its rows differ.
	cellUneven = 2
)

// pieceGrid marks the cells of a shape that an edge may touch.
type pieceGrid struct {
	org    image.Point // the first pixel of the first cell
	w, h   int         // in cells
	marked []uint8
}

// mark marks the cells the box of two points touches, with a pixel of
// margin, as holding an edge that makes their rows differ; or, with
// vertical, a vertical line from y0 to y1, which does so only in the cells
// where it ends.
func (g *pieceGrid) mark(x0, y0, x1, y1 float64, vertical bool) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	cx0 := max((int(math.Floor(x0))-1-g.org.X)/coverCell, 0)
	cy0 := max((int(math.Floor(y0))-1-g.org.Y)/coverCell, 0)
	cx1 := min((int(math.Ceil(x1))+1-g.org.X)/coverCell, g.w-1)
	cy1 := min((int(math.Ceil(y1))+1-g.org.Y)/coverCell, g.h-1)
	// The rows of pixels the line crosses whole, away from its ends by a
	// pixel (the snapping to the rasterizer's grid).
	top, bottom := int(math.Ceil(y0))+1, int(math.Floor(y1))-1
	for cy := cy0; cy <= cy1; cy++ {
		v := uint8(cellEdge | cellUneven)
		if t := g.org.Y + cy*coverCell; vertical && t >= top && t+coverCell <= bottom {
			v = cellEdge
		}
		row := g.marked[cy*g.w : (cy+1)*g.w]
		for cx := cx0; cx <= cx1; cx++ {
			row[cx] |= v
		}
	}
}

// small reports whether a box is small enough to be marked whole: no
// larger than two cells one way.
func smallBox(x0, y0, x1, y1 float64) bool {
	return math.Abs(x1-x0) <= 2*coverCell || math.Abs(y1-y0) <= 2*coverCell
}

// line marks the cells along a segment: by the boxes of its halves, until
// they are small.
func (g *pieceGrid) line(x0, y0, x1, y1 float64, depth int) {
	if x0 == x1 {
		g.mark(x0, y0, x1, y1, true)
		return
	}
	if depth == 0 || smallBox(x0, y0, x1, y1) {
		g.mark(x0, y0, x1, y1, false)
		return
	}
	mx, my := (x0+x1)/2, (y0+y1)/2
	g.line(x0, y0, mx, my, depth-1)
	g.line(mx, my, x1, y1, depth-1)
}

// cubic marks the cells along a cubic curve, which lies inside the box of
// its control points: by the boxes of its halves, until they are small.
func (g *pieceGrid) cubic(p [8]float64, depth int) {
	x0, y0, x1, y1 := p[0], p[1], p[0], p[1]
	for i := 2; i < 8; i += 2 {
		x0, x1 = min(x0, p[i]), max(x1, p[i])
		y0, y1 = min(y0, p[i+1]), max(y1, p[i+1])
	}
	if depth == 0 || smallBox(x0, y0, x1, y1) {
		g.mark(x0, y0, x1, y1, false)
		return
	}
	// De Casteljau at one half.
	var a, b [8]float64
	for k := range 2 {
		p0, p1, p2, p3 := p[k], p[2+k], p[4+k], p[6+k]
		q0, q1, q2 := (p0+p1)/2, (p1+p2)/2, (p2+p3)/2
		r0, r1 := (q0+q1)/2, (q1+q2)/2
		s := (r0 + r1) / 2
		a[k], a[2+k], a[4+k], a[6+k] = p0, q0, r0, s
		b[k], b[2+k], b[4+k], b[6+k] = s, r1, q2, p3
	}
	g.cubic(a, depth-1)
	g.cubic(b, depth-1)
}

// pieceDepth bounds the halvings of a segment.
const pieceDepth = 12

// edges marks the cells the edges of path p under m may touch. Every
// subpath counts as closed, as a fill closes it. ok is false when a
// coordinate is not finite.
func (g *pieceGrid) edges(m *gfx.Affine, p gfx.Path) bool {
	ok := true
	pt := func(q gfx.Point) (float64, float64) {
		x, y := apply(m, q.X, q.Y)
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			ok = false
		}
		return x, y
	}
	var sx, sy, cx, cy float64 // the subpath's start, the current point
	open := false
	closeSub := func() {
		if open && (cx != sx || cy != sy) {
			g.line(cx, cy, sx, sy, pieceDepth)
		}
		cx, cy, open = sx, sy, false
	}
	for el := range p.Elements() {
		switch el.Verb {
		case gfx.MoveTo:
			closeSub()
			sx, sy = pt(el.Pts[0])
			cx, cy = sx, sy
		case gfx.LineTo:
			x, y := pt(el.Pts[0])
			g.line(cx, cy, x, y, pieceDepth)
			cx, cy, open = x, y, true
		case gfx.QuadTo:
			// The same curve as a cubic (its degree raised), whose halves
			// are halves of the quadratic.
			x1, y1 := pt(el.Pts[0])
			x2, y2 := pt(el.Pts[1])
			g.cubic([8]float64{cx, cy, cx + (x1-cx)*2/3, cy + (y1-cy)*2/3, x2 + (x1-x2)*2/3, y2 + (y1-y2)*2/3, x2, y2}, pieceDepth)
			cx, cy, open = x2, y2, true
		case gfx.CubicTo:
			x1, y1 := pt(el.Pts[0])
			x2, y2 := pt(el.Pts[1])
			x3, y3 := pt(el.Pts[2])
			g.cubic([8]float64{cx, cy, x1, y1, x2, y2, x3, y3}, pieceDepth)
			cx, cy, open = x3, y3, true
		case gfx.Close:
			closeSub()
		}
		if !ok {
			return false
		}
	}
	closeSub()
	return ok
}

// cellClass is what rects gathers: the cells no edge comes near, those
// whose rows are all the same, and the others.
func cellClass(v uint8) uint8 {
	switch {
	case v == cellEmpty:
		return 0
	case v&cellUneven == 0:
		return 1
	}
	return 2
}

// rects gathers the cells of the class want into rectangles of pixels
// inside bounds, at most limit pixels wide and high (0: any size): the
// runs of each row of cells, joined with the equal run of the row above.
func (g *pieceGrid) rects(want uint8, bounds image.Rectangle, limit int, out []image.Rectangle) []image.Rectangle {
	maxCells := g.w + g.h
	if limit > 0 {
		maxCells = max(limit/coverCell, 1)
	}
	type run struct{ x0, x1, at int } // cells; at: the rectangle it extends
	var prev, cur []run
	for cy := 0; cy < g.h; cy++ {
		cur = cur[:0]
		row := g.marked[cy*g.w : (cy+1)*g.w]
		for cx := 0; cx < g.w; {
			if cellClass(row[cx]) != want {
				cx++
				continue
			}
			x0 := cx
			for cx < g.w && cellClass(row[cx]) == want && cx-x0 < maxCells {
				cx++
			}
			at := -1
			for _, p := range prev {
				if p.x0 == x0 && p.x1 == cx {
					if r := out[p.at]; (r.Dy()+coverCell-1)/coverCell < maxCells {
						at = p.at
					}
					break
				}
			}
			y0, y1 := g.org.Y+cy*coverCell, g.org.Y+(cy+1)*coverCell
			if at >= 0 {
				out[at].Max.Y = min(y1, bounds.Max.Y)
			} else {
				at = len(out)
				out = append(out, image.Rect(g.org.X+x0*coverCell, y0, g.org.X+cx*coverCell, y1).Intersect(bounds))
			}
			cur = append(cur, run{x0, cx, at})
		}
		prev, cur = cur, prev
	}
	return out
}

// buildPieces cuts the coverage of sh into its pieces (see the top of the
// file). tile is the largest width and height of a picture in the atlas.
func (r *resources) buildPieces(sh *coverShape, tile int) {
	rel := sh.rel
	sh.pieces = sh.pieces[:0]
	whole := func() {
		// By tiles, each of pixels: what the atlas finds uniform takes no
		// room in it.
		sh.pieces = sh.pieces[:0]
		for y := rel.Min.Y; y < rel.Max.Y; y += tile {
			for x := rel.Min.X; x < rel.Max.X; x += tile {
				sh.pieces = append(sh.pieces, coverPiece{rel: image.Rect(x, y, x+tile, y+tile).Intersect(rel)})
			}
		}
	}
	if (rel.Dx() <= 4*coverCell && rel.Dy() <= 4*coverCell) || tile < coverCell {
		whole() // an icon, a glyph: one picture; or an atlas of tiny pages
		return
	}
	g := pieceGrid{org: rel.Min, w: (rel.Dx() + coverCell - 1) / coverCell, h: (rel.Dy() + coverCell - 1) / coverCell}
	if g.w > maxPieceCells || g.h > maxPieceCells || g.w*g.h > maxPieceCells {
		whole() // larger than any target: only what is seen of it is worth a look
		return
	}
	if n := g.w * g.h; cap(r.cells) < n {
		r.cells = make([]uint8, n)
	} else {
		r.cells = r.cells[:n]
		clear(r.cells)
	}
	g.marked = r.cells
	if !g.edges(&sh.m, sh.path) {
		whole()
		return
	}
	// The rectangles no edge touches: inside or outside, by one pixel.
	// One that is neither (which the rasterizer's arithmetic does not
	// produce) is drawn from its pixels like the rest.
	r.rectBuf = g.rects(0, rel, 0, r.rectBuf[:0])
	for _, u := range r.rectBuf {
		var px [1]uint8
		r.masker.Fill(px[:], image.Rect(u.Min.X, u.Min.Y, u.Min.X+1, u.Min.Y+1), sh.m, sh.path, sh.rule)
		switch px[0] {
		case 0:
		case 255:
			sh.pieces = append(sh.pieces, coverPiece{rel: u, full: true})
		default:
			for cy := (u.Min.Y - g.org.Y) / coverCell; g.org.Y+cy*coverCell < u.Max.Y; cy++ {
				for cx := (u.Min.X - g.org.X) / coverCell; g.org.X+cx*coverCell < u.Max.X; cx++ {
					g.marked[cy*g.w+cx] = cellEdge | cellUneven
				}
			}
		}
	}
	r.rectBuf = g.rects(1, rel, tile, r.rectBuf[:0])
	for _, u := range r.rectBuf {
		sh.pieces = append(sh.pieces, coverPiece{rel: u, rows: u.Dy() > 1})
	}
	r.rectBuf = g.rects(2, rel, tile, r.rectBuf[:0])
	for _, u := range r.rectBuf {
		sh.pieces = append(sh.pieces, coverPiece{rel: u})
	}
	if len(sh.pieces) > maxPieces {
		whole()
	}
}
