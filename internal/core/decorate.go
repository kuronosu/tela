package core

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Box decorations: shadows, backgrounds, borders and outlines (see the
// comment at the top of paint.go for the order).

// paintSelf paints the decorations of bx under its content: outer
// shadows, background, inset shadows and border.
func (p *painter) paintSelf(bx *Box) {
	cs := bx.pcs
	if cs.Visibility != style.VisibilityVisible {
		return
	}
	g := p.geom(bx)
	if g.border.Rect.Empty() {
		return
	}
	// Blink clips an outer shadow out of the box shrunk by one pixel when
	// the background is opaque and covers the border box, so that the
	// shadow reaches under the anti-aliased edge: the same shadow shape
	// with that smaller clip is the shrunk box with one more pixel of
	// spread.
	shadowBox, grow := g.border, float32(0)
	if cs.BackgroundColor.A >= 1 && cs.BackgroundClip == style.BackgroundBorderBox {
		grow = float32(1 / p.scale)
		shadowBox = insetRRect(g.border, grow, grow, grow, grow)
	}
	for i := len(cs.BoxShadows) - 1; i >= 0; i-- {
		if s := &cs.BoxShadows[i]; !s.Inset {
			p.b.DrawShadow(gfx.DrawShadow{RRect: shadowBox, Offset: gfx.Point{X: s.OffsetX, Y: s.OffsetY},
				Blur: s.Blur, Spread: s.Spread + grow, Color: s.Color})
		}
	}
	p.paintBackground(bx, cs, &g)
	if !g.padding.Rect.Empty() {
		for i := len(cs.BoxShadows) - 1; i >= 0; i-- {
			if s := &cs.BoxShadows[i]; s.Inset {
				p.b.DrawShadow(gfx.DrawShadow{RRect: g.padding, Offset: gfx.Point{X: s.OffsetX, Y: s.OffsetY},
					Blur: s.Blur, Spread: s.Spread, Color: s.Color, Inset: true})
			}
		}
	}
	p.paintBorder(cs, &g)
}

// fillRRect fills rr, as a rectangle when its corners are square.
func (p *painter) fillRRect(rr gfx.RRect, paint gfx.Paint) {
	if rr.IsRect() {
		p.b.FillRect(rr.Rect, paint)
		return
	}
	p.b.FillRRect(rr, paint)
}

// ---------------------------------------------------------------------
// Backgrounds.

// bgBox returns the box of bx that background-clip or -origin names.
func (p *painter) bgBox(bx *Box, g *boxGeom, which style.BackgroundBox) gfx.RRect {
	switch which {
	case style.BackgroundPaddingBox:
		return g.padding
	case style.BackgroundContentBox:
		return p.contentRRect(bx, g)
	}
	return g.border
}

// paintBackground paints the background color and image layers of bx in
// its background painting area. The root's color is the canvas, already
// painted.
func (p *painter) paintBackground(bx *Box, cs *style.ComputedStyle, g *boxGeom) {
	if cs.BackgroundColor.A == 0 && len(cs.Backgrounds) == 0 {
		return
	}
	if cs.BackgroundClip == style.BackgroundText {
		p.t.reportOnce("bg-clip-text", "tela: background-clip: text is not painted yet; the background is skipped")
		return
	}
	clip := p.bgBox(bx, g, cs.BackgroundClip)
	if cs.BackgroundClip == style.BackgroundBorderBox && !clip.IsRect() && borderObscuresEdge(cs, g) {
		// Blink's bleed avoidance: under an opaque border the background
		// shrinks by a device pixel, so that its anti-aliased edge does
		// not show through the border's.
		d := float32(1 / p.scale)
		clip = insetRRect(clip, d, d, d, d)
	}
	if clip.Rect.Empty() {
		return
	}
	if bx != p.t.rootBox && cs.BackgroundColor.A > 0 {
		p.fillRRect(clip, cs.BackgroundColor)
	}
	for i := len(cs.Backgrounds) - 1; i >= 0; i-- {
		l := &cs.Backgrounds[i]
		p.paintBgLayer(l, p.bgBox(bx, g, l.Origin).Rect, clip)
	}
}

// borderObscuresEdge reports whether every side of the border is present,
// opaque and solid or double, so that it covers the edge of the border
// box.
func borderObscuresEdge(cs *style.ComputedStyle, g *boxGeom) bool {
	for i, b := range [4]*style.Border{&cs.Border.Top, &cs.Border.Right, &cs.Border.Bottom, &cs.Border.Left} {
		if !(g.bw[i] > 0) || b.Color.A < 1 || (b.Style != style.BorderSolid && b.Style != style.BorderDouble) {
			return false
		}
	}
	return true
}

// maxBgTiles bounds the tiles of one background layer.
const maxBgTiles = 1024

// paintBgLayer paints one background image layer whose positioning area
// is area, clipped to clip. Gradients have no natural size: an auto
// size, cover and contain all make one tile the size of the area (CSS
// Backgrounds 3 section 3.9). repeat: space is painted as repeat.
func (p *painter) paintBgLayer(l *style.BackgroundLayer, area gfx.Rect, clip gfx.RRect) {
	if l.Gradient == nil {
		if l.URL != "" {
			p.t.reportOnce("bg-url", "tela: background images (url()) are not painted yet")
		}
		return
	}
	aw, ah := area.Width(), area.Height()
	if !(aw > 0 && ah > 0) {
		return
	}
	tw, th := aw, ah
	if l.Size.Kind == style.SizeLength {
		if v, ok := l.Size.W.Resolve(aw); ok {
			tw = v
		}
		if v, ok := l.Size.H.Resolve(ah); ok {
			th = v
		}
	}
	if !(tw > 0 && th > 0) {
		return
	}
	var repX, repY bool
	switch l.Repeat {
	case style.RepeatBoth, style.RepeatRound, style.RepeatSpace:
		repX, repY = true, true
	case style.RepeatX:
		repX = true
	case style.RepeatY:
		repY = true
	}
	if l.Repeat == style.RepeatRound {
		tw = aw / float32(max(1, math.Round(float64(aw/tw))))
		th = ah / float32(max(1, math.Round(float64(ah/th))))
	}
	x := area.X0 + l.PosX.Resolve(aw-tw)
	y := area.Y0 + l.PosY.Resolve(ah-th)
	cb := clip.Rect
	i0, i1, j0, j1 := 0, 1, 0, 1
	if repX {
		i0, i1 = int(math.Floor(float64((cb.X0-x)/tw))), int(math.Ceil(float64((cb.X1-x)/tw)))
	}
	if repY {
		j0, j1 = int(math.Floor(float64((cb.Y0-y)/th))), int(math.Ceil(float64((cb.Y1-y)/th)))
	}
	if (i1-i0)*(j1-j0) > maxBgTiles {
		p.t.reportOnce("bg-tiles", "tela: a background layer has too many tiles; only the first ones are painted")
	}
	tiles := 0
	for j := j0; j < j1; j++ {
		for i := i0; i < i1 && tiles < maxBgTiles; i++ {
			tx, ty := x+float32(float32(i)*tw), y+float32(float32(j)*th)
			tile := gfx.Rect{X0: tx, Y0: ty, X1: tx + tw, Y1: ty + th}
			if !tile.Intersects(cb) {
				continue
			}
			tiles++
			paint := l.Gradient.Paint(tile)
			if tile.ContainsRect(cb) {
				p.fillRRect(clip, paint)
				continue
			}
			p.b.PushClipRect(tile)
			p.fillRRect(clip, paint)
			p.b.PopClip()
		}
	}
}

// ---------------------------------------------------------------------
// Borders and outlines.

// borderSide is one side of a border or outline, with its snapped width.
type borderSide struct {
	w     float32
	style style.BorderStyle
	color gfx.Color
}

func (s *borderSide) visible() bool {
	return s.w > 0 && s.color.A > 0 && s.style != style.BorderNone && s.style != style.BorderHidden
}

// paintBorder paints the border of a box.
func (p *painter) paintBorder(cs *style.ComputedStyle, g *boxGeom) {
	b := [4]*style.Border{&cs.Border.Top, &cs.Border.Right, &cs.Border.Bottom, &cs.Border.Left}
	var sd [4]borderSide
	for i := range sd {
		sd[i] = borderSide{g.bw[i], b[i].Style, b[i].Color}
	}
	p.paintEdges(g.border, g.padding, g.bw, sd)
}

// paintOutline paints the outline of bx outside its border box, offset by
// outline-offset. Like Chromium, a rounded border box gives a rounded
// outline: the inner radius of a rounded corner is the border radius plus
// the offset, the outer one that plus the width.
func (p *painter) paintOutline(bx *Box) {
	cs := bx.pcs
	o := &cs.Outline
	if cs.Visibility != style.VisibilityVisible || o.Width <= 0 || o.Color.A == 0 ||
		o.Style == style.BorderNone || o.Style == style.BorderHidden {
		return
	}
	g := p.geom(bx)
	if g.border.Rect.Empty() {
		return
	}
	w := p.snapWidth(o.Width)
	off := float32(math.Floor(float64(float64(o.Offset)*p.scale)+0.5) / p.scale)
	in := gfx.RRect{Rect: g.border.Rect.Outset(off)}
	out := gfx.RRect{Rect: in.Rect.Outset(w)}
	for i, r := range g.border.Radii {
		if r.X > 0 && r.Y > 0 {
			in.Radii[i] = gfx.Point{X: max(r.X+off, 0), Y: max(r.Y+off, 0)}
			out.Radii[i] = gfx.Point{X: max(r.X+off+w, 0), Y: max(r.Y+off+w, 0)}
		}
	}
	if in.Rect.Empty() {
		in = gfx.RRect{}
	}
	in, out = in.Normalize(), out.Normalize()
	s := borderSide{w, o.Style, o.Color}
	p.paintEdges(out, in, [4]float32{w, w, w, w}, [4]borderSide{s, s, s, s})
}

// paintEdges paints the band between outer and inner (a border or an
// outline) whose sides have widths bw and styles sd (top, right, bottom,
// left).
//
// Sides that differ split the corners along the line from the outer
// corner to the corner of the inner rectangle, like browsers: each side
// is painted through a clip polygon. (Painting the sides over each other
// to hide the anti-aliased seam along the diagonals was tried: the
// earlier side then shows through the anti-aliased outer edge of the
// later one, which is further from Chromium than the seam.)
//
// Styles: solid fills the band; double paints two stripes of about a
// third of the width each (Blink's integer split in device pixels; solid
// under 3 device pixels); dashed and dotted stroke each side's center line
// from corner diagonal to corner diagonal, following the corner curves,
// with Blink's dash lengths (dashes of 3 and gaps of 2 widths under 3
// device pixels, 2 and 1 widths from 3 on; dots one width wide, round
// from 3 device pixels, with centers two widths apart) and the gap
// adjusted so that a dash or dot sits at both ends of the side.
func (p *painter) paintEdges(outer, inner gfx.RRect, bw [4]float32, sd [4]borderSide) {
	n := 0
	for i := range sd {
		if sd[i].visible() {
			n++
		}
	}
	if n == 0 || outer.Rect.Empty() {
		return
	}
	// Visible sides of one style and color, the others of zero width:
	// the whole band at once (the widths may differ).
	var first *borderSide
	same := true
	for i := range sd {
		switch {
		case !sd[i].visible():
			same = same && sd[i].w == 0
		case first == nil:
			first = &sd[i]
		default:
			same = same && sd[i].style == first.style && sd[i].color == first.color
		}
	}
	if same && (first.style == style.BorderSolid || first.style == style.BorderDouble) {
		p.paintBand(outer, inner, bw, first.style, first.color, -1)
		return
	}
	if same && n == 4 && sd[1].w == sd[0].w && sd[2].w == sd[0].w && sd[3].w == sd[0].w && !outer.IsRect() {
		// One closed dashed or dotted path for the whole band.
		p.strokeSide(outer, inner, bw, 0, sd[0].style, sd[0].color)
		return
	}
	for i := range sd {
		s := &sd[i]
		if !s.visible() {
			continue
		}
		p.b.PushClipPath(sidePolygon(outer.Rect, bw, i), gfx.NonZero)
		p.paintBand(outer, inner, bw, s.style, s.color, i)
		p.b.PopClip()
	}
}

// ringPath returns the band between outer and inner, to fill with the
// nonzero rule: inner runs counterclockwise, so that where the two edges
// coincide (a side of zero width) their coverage cancels exactly, which
// the even-odd rule does not do inside anti-aliased pixels.
func ringPath(outer, inner gfx.RRect) gfx.Path {
	var path gfx.Path
	path.AddRRect(outer)
	if !inner.Rect.Empty() {
		// The mirror image of inner with its left and right corners
		// swapped, reflected about inner's vertical axis: the same shape,
		// counterclockwise.
		r := inner.Rect
		sw := gfx.RRect{Rect: r, Radii: [4]gfx.Point{inner.Radii[1], inner.Radii[0], inner.Radii[3], inner.Radii[2]}}
		var rev gfx.Path
		rev.AddRRect(sw)
		rev = rev.Transform(gfx.Affine{-1, 0, 0, 1, r.X0 + r.X1, 0})
		for el := range rev.Elements() {
			switch el.Verb {
			case gfx.MoveTo:
				path.MoveTo(el.Pts[0])
			case gfx.LineTo:
				path.LineTo(el.Pts[0])
			case gfx.QuadTo:
				path.QuadTo(el.Pts[0], el.Pts[1])
			case gfx.CubicTo:
				path.CubicTo(el.Pts[0], el.Pts[1], el.Pts[2])
			case gfx.Close:
				path.Close()
			}
		}
	}
	return path
}

// paintBand paints the band between outer and inner in one style and
// color; side is the side being painted (0-3, top first) for dashed and
// dotted styles, or -1 for all of them.
func (p *painter) paintBand(outer, inner gfx.RRect, bw [4]float32, st style.BorderStyle, c gfx.Color, side int) {
	switch st {
	case style.BorderDouble:
		// Blink's stripes, in whole device pixels: the outer stripe is
		// w/3 (+1 when w%3 == 2), the inner one starts at 2w/3 (+1 when
		// w%3 == 1).
		var o, s [4]float32
		for i, w := range bw {
			d := int(math.Round(float64(float64(w) * p.scale)))
			if d < 3 {
				o[i], s[i] = w, w
				continue
			}
			od, sdv := d/3, 2*d/3
			if d%3 == 2 {
				od++
			}
			if d%3 == 1 {
				sdv++
			}
			o[i], s[i] = float32(float64(od)/p.scale), float32(float64(sdv)/p.scale)
		}
		mid1 := insetRRect(outer, o[0], o[1], o[2], o[3])
		mid2 := insetRRect(outer, s[0], s[1], s[2], s[3])
		p.b.FillPath(ringPath(outer, mid1), gfx.NonZero, c)
		if s != bw {
			p.b.FillPath(ringPath(mid2, inner), gfx.NonZero, c)
		}
	case style.BorderDashed, style.BorderDotted:
		if side < 0 {
			for i := range 4 {
				p.strokeSide(outer, inner, bw, i, st, c)
			}
			return
		}
		p.strokeSide(outer, inner, bw, side, st, c)
	default:
		p.b.FillPath(ringPath(outer, inner), gfx.NonZero, c)
	}
}

// strokeSide strokes the dashes or dots of side i, clipped to the band.
// It follows Blink: a band with a rounded corner strokes its whole closed
// center line (the outer edge inset by half the side's width) through
// the side's clip, starting at the left end of the top edge and going
// clockwise, with the gap fitted to the perimeter; a band with square
// corners strokes the side's own center line from corner to corner, with
// a dash or dot at both ends.
func (p *painter) strokeSide(outer, inner gfx.RRect, bw [4]float32, i int, st style.BorderStyle, c gfx.Color) {
	w := bw[i]
	if !(w > 0) {
		return
	}
	// The thresholds are in device pixels (Blink paints at the device
	// scale).
	dev := float32(float64(w) * p.scale)
	round := st == style.BorderDotted && dev > 3
	dash, gap := w, w // thin dotted borders are short dashes
	if st == style.BorderDashed {
		if dev >= 3 {
			dash, gap = 2*w, w
		} else {
			dash, gap = 3*w, 2*w
		}
	}
	closed := !outer.IsRect()
	var path gfx.Path
	var length float32
	if closed {
		path, length = centerPath(outer, w)
	} else {
		path, length = sidePath(outer, bw, i, !round)
	}
	ss := gfx.NewStrokeStyle(w)
	switch {
	case round:
		// Round dots one width wide (Blink: an interval of the fitted gap
		// plus the width, less an epsilon that keeps the last dot).
		ss.Cap = gfx.CapRound
		if length >= 2*w {
			ss.Dashes = []float32{0, selectDashGap(length, w, w, closed) + w - 0.01}
		} else {
			ss.Dashes = []float32{0, 2 * w}
		}
	case length > 2*dash:
		ss.Dashes = []float32{dash, selectDashGap(length, dash, gap, closed)}
	}
	if closed && !round {
		// Blink strokes 10% wider and lets the band's clip cut the edges.
		ss.Width = float32(w * 1.1)
	}
	p.b.PushClipPath(ringPath(outer, inner), gfx.NonZero)
	p.b.StrokePath(path, ss, c)
	p.b.PopClip()
}

// selectDashGap returns the gap between dashes of the given length that
// fits a whole number of them on a path of the given length, closest to
// gap (Blink's SelectBestDashGap). An open path has a dash at both ends.
func selectDashGap(length, dash, gap float32, closed bool) float32 {
	l, d, g := float64(length), float64(dash), float64(gap)
	nmin := math.Floor((l + g) / (d + g))
	nmax := nmin + 1
	gmin, gmax := nmin, nmax
	if !closed {
		gmin, gmax = nmin-1, nmax-1
	}
	minGap := (l - float64(nmin*d)) / gmin
	maxGap := (l - float64(nmax*d)) / gmax
	if maxGap <= 0 || math.Abs(minGap-g) < math.Abs(maxGap-g) {
		if math.IsInf(minGap, 0) || math.IsNaN(minGap) {
			return gap
		}
		return float32(minGap)
	}
	return float32(maxGap)
}

// centerPath returns the closed center line of a rounded band of width w
// whose outer edge is outer (Blink's rounded rectangle path: from the
// left end of the top edge, clockwise), and its length.
func centerPath(outer gfx.RRect, w float32) (gfx.Path, float32) {
	h := w / 2
	c := outer.Rect.Inset(h)
	var rx, ry [4]float32
	for k, r := range outer.Radii {
		rx[k], ry[k] = max(r.X-h, 0), max(r.Y-h, 0)
		if rx[k] == 0 || ry[k] == 0 {
			rx[k], ry[k] = 0, 0
		}
	}
	const arcLen = math.Pi / 4 // a quarter ellipse, per unit of rx + ry
	var path gfx.Path
	var length float64
	corner := func(k int, cen gfx.Point) {
		if rx[k] == 0 {
			path.LineTo(cen)
			return
		}
		sx, sy := float32(1), float32(1)
		if k == 1 || k == 2 {
			sx = -1
		}
		if k >= 2 {
			sy = -1
		}
		o := gfx.Point{X: cen.X + sx*rx[k], Y: cen.Y + sy*ry[k]}
		b := [4]int{4, 6, 0, 2}[k]
		path.LineTo(ellipsePoint(o, rx[k], ry[k], b))
		addArc45(&path, o, rx[k], ry[k], b)
		addArc45(&path, o, rx[k], ry[k], b+1)
		length += float64(arcLen * float64(rx[k]+ry[k]))
	}
	path.MoveTo(gfx.Point{X: c.X0 + rx[0], Y: c.Y0})
	corner(1, gfx.Point{X: c.X1, Y: c.Y0})
	corner(2, gfx.Point{X: c.X1, Y: c.Y1})
	corner(3, gfx.Point{X: c.X0, Y: c.Y1})
	corner(0, gfx.Point{X: c.X0, Y: c.Y0})
	path.Close()
	sides := float64(c.Width()-rx[0]-rx[1]) + float64(c.Height()-ry[1]-ry[2]) +
		float64(c.Width()-rx[2]-rx[3]) + float64(c.Height()-ry[3]-ry[0])
	return path, float32(length + max(sides, 0))
}

// Unit circle at multiples of 45 degrees (y down: 90 degrees points down).
var (
	cos45 = [8]float32{1, math.Sqrt2 / 2, 0, -math.Sqrt2 / 2, -1, -math.Sqrt2 / 2, 0, math.Sqrt2 / 2}
	sin45 = [8]float32{0, math.Sqrt2 / 2, 1, math.Sqrt2 / 2, 0, -math.Sqrt2 / 2, -1, -math.Sqrt2 / 2}
)

// kappa45 is the cubic Bezier handle length of a 45 degree arc of the
// unit circle: 4/3 tan(45°/4).
const kappa45 = 0.26521648983954

// ellipsePoint returns the point at k × 45 degrees on the ellipse.
func ellipsePoint(c gfx.Point, rx, ry float32, k int) gfx.Point {
	return gfx.Point{X: c.X + float32(rx*cos45[k&7]), Y: c.Y + float32(ry*sin45[k&7])}
}

// addArc45 adds the 45 degree arc of the ellipse from k × 45 degrees,
// clockwise on screen, as one cubic; the current point is its start.
func addArc45(path *gfx.Path, c gfx.Point, rx, ry float32, k int) {
	p0, p3 := ellipsePoint(c, rx, ry, k), ellipsePoint(c, rx, ry, k+1)
	// Tangents (-rx sin, ry cos) scaled by kappa.
	t0 := gfx.Point{X: float32(kappa45 * float32(-rx*sin45[k&7])), Y: float32(kappa45 * float32(ry*cos45[k&7]))}
	t1 := gfx.Point{X: float32(kappa45 * float32(-rx*sin45[(k+1)&7])), Y: float32(kappa45 * float32(ry*cos45[(k+1)&7]))}
	path.CubicTo(p0.Add(t0), p3.Sub(t1), p3)
}

// sidePath returns the center line of side i of the band whose outer
// edge is outer (side widths bw), from the diagonal of its first corner
// to the diagonal of its second, clockwise, and its length. At a square
// corner the line ends at the corner of the center rectangle, or at the
// outer edge when extend is set.
func sidePath(outer gfx.RRect, bw [4]float32, i int, extend bool) (gfx.Path, float32) {
	o := outer.Rect
	c := gfx.Rect{X0: o.X0 + bw[3]/2, Y0: o.Y0 + bw[0]/2, X1: o.X1 - bw[1]/2, Y1: o.Y1 - bw[2]/2}
	// Widths of the vertical and horizontal sides at each corner (TL, TR,
	// BR, BL), the corner points of the center rectangle, the angle where
	// each corner's arc starts (in 45 degree units) and the direction of
	// each side.
	vx := [4]float32{bw[3], bw[1], bw[1], bw[3]}
	hy := [4]float32{bw[0], bw[0], bw[2], bw[2]}
	corner := [4]gfx.Point{{X: c.X0, Y: c.Y0}, {X: c.X1, Y: c.Y0}, {X: c.X1, Y: c.Y1}, {X: c.X0, Y: c.Y1}}
	base := [4]int{4, 6, 0, 2}
	dir := [4]gfx.Point{{X: 1}, {Y: 1}, {X: -1}, {Y: -1}}
	radii := func(k int) (float32, float32) {
		r := outer.Radii[k]
		return max(r.X-vx[k]/2, 0), max(r.Y-hy[k]/2, 0)
	}
	center := func(k int, rx, ry float32) gfx.Point {
		sx, sy := float32(1), float32(1)
		if k == 1 || k == 2 {
			sx = -1
		}
		if k >= 2 {
			sy = -1
		}
		return gfx.Point{X: corner[k].X + sx*rx, Y: corner[k].Y + sy*ry}
	}
	const arcLen = math.Pi / 8 // a 45 degree arc of an ellipse, per unit of rx + ry
	var path gfx.Path
	var length float64
	d := dir[i]
	ka, kb := i, (i+1)%4
	var s0, s1 gfx.Point // the straight part
	if rx, ry := radii(ka); rx > 0 && ry > 0 {
		cen := center(ka, rx, ry)
		path.MoveTo(ellipsePoint(cen, rx, ry, base[ka]+1))
		addArc45(&path, cen, rx, ry, base[ka]+1)
		s0 = ellipsePoint(cen, rx, ry, base[ka]+2)
		length += float64(arcLen * float64(rx+ry))
	} else {
		s0 = corner[ka]
		if extend {
			e := bw[(i+3)%4] / 2
			s0 = gfx.Point{X: s0.X - d.X*e, Y: s0.Y - d.Y*e}
		}
		path.MoveTo(s0)
	}
	if rx, ry := radii(kb); rx > 0 && ry > 0 {
		cen := center(kb, rx, ry)
		s1 = ellipsePoint(cen, rx, ry, base[kb])
		path.LineTo(s1)
		addArc45(&path, cen, rx, ry, base[kb])
		length += float64(arcLen * float64(rx+ry))
	} else {
		s1 = corner[kb]
		if extend {
			e := bw[(i+1)%4] / 2
			s1 = gfx.Point{X: s1.X + d.X*e, Y: s1.Y + d.Y*e}
		}
		path.LineTo(s1)
	}
	length += math.Abs(float64(s1.X-s0.X)) + math.Abs(float64(s1.Y-s0.Y))
	return path, float32(length)
}

// sidePolygon returns the region of side i of a band whose outer edge is
// o: o (grown by one pixel) cut, at both corners of the side, by the line
// from the corner of o to the matching corner of o inset by the side
// widths.
func sidePolygon(o gfx.Rect, bw [4]float32, i int) gfx.Path {
	g := o.Outset(1)
	poly := []gfx.Point{{X: g.X0, Y: g.Y0}, {X: g.X1, Y: g.Y0}, {X: g.X1, Y: g.Y1}, {X: g.X0, Y: g.Y1}}
	corners := [4]gfx.Point{{X: o.X0, Y: o.Y0}, {X: o.X1, Y: o.Y0}, {X: o.X1, Y: o.Y1}, {X: o.X0, Y: o.Y1}}
	diag := [4]gfx.Point{{X: bw[3], Y: bw[0]}, {X: -bw[1], Y: bw[0]}, {X: -bw[1], Y: -bw[2]}, {X: bw[3], Y: -bw[2]}}
	mx, my := (o.X0+o.X1)/2, (o.Y0+o.Y1)/2
	ref := [4]gfx.Point{{X: mx, Y: o.Y0}, {X: o.X1, Y: my}, {X: mx, Y: o.Y1}, {X: o.X0, Y: my}}[i]
	cut := func(k int) {
		org, dv := corners[k], diag[k]
		side := dv.Cross(ref.Sub(org))
		if side == 0 {
			return
		}
		f := func(q gfx.Point) float32 {
			v := dv.Cross(q.Sub(org))
			if side < 0 {
				v = -v
			}
			return v
		}
		var out []gfx.Point
		for j := range poly {
			a, b := poly[j], poly[(j+1)%len(poly)]
			fa, fb := f(a), f(b)
			if fa >= 0 {
				out = append(out, a)
			}
			if (fa >= 0) != (fb >= 0) {
				t := fa / (fa - fb)
				out = append(out, gfx.Point{X: a.X + float32((b.X-a.X)*t), Y: a.Y + float32((b.Y-a.Y)*t)})
			}
		}
		poly = out
	}
	cut(i)
	cut((i + 1) % 4)
	var path gfx.Path
	for j, q := range poly {
		if j == 0 {
			path.MoveTo(q)
		} else {
			path.LineTo(q)
		}
	}
	if len(poly) > 0 {
		path.Close()
	}
	return path
}
