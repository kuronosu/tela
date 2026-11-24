package canvas

import (
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/svg"
)

// The drawing model ("drawing model" of the HTML Standard). What a method
// draws, the source, is composited with its shadow first, when it has one,
// and then itself, each with the current operation:
//
//   - source-over, the usual one, records the source as it is;
//   - any other operation records it in a layer with the blend mode, so that
//     what the operation does outside a source (the unbounded ones clear the
//     whole clip) is done;
//   - a shadow is the source again: in the color of the shadow, for a source
//     of one color; else in a layer that a color matrix paints in the color
//     of the shadow, with the source's alpha. A blur is the filter of a
//     layer, or the closed form of gfx.DrawShadow for a rectangle or a
//     rounded rectangle.
//
// The opacity is the paint's when it can be, or the one of a layer (images,
// patterns, color glyphs).

// Fill fills the path with FillStyle, its open subpaths as if they were
// closed.
func (c *Canvas) Fill() {
	if r := c.use("Fill"); r != nil {
		r.fill(c, &r.path, nil)
	}
}

// FillPath fills path as Fill does the current one, with the transform
// current now. The current path stays as it is.
func (c *Canvas) FillPath(path *Path2D) {
	if r := c.use("FillPath"); r != nil {
		if p := c.usePath("FillPath", path); p != nil {
			r.fill(c, p, &r.m)
		}
	}
}

// Stroke draws the lines of the path with StrokeStyle. Their shape (width,
// ends, corners, dashes) follows the transform current now, and their place
// the transforms that were current when their points were added.
func (c *Canvas) Stroke() {
	if r := c.use("Stroke"); r != nil {
		r.strokeLines(c, &r.path)
	}
}

// StrokePath draws the lines of path as Stroke does the ones of the current
// path, with the transform current now. The current path stays as it is.
func (c *Canvas) StrokePath(path *Path2D) {
	if r := c.use("StrokePath"); r != nil {
		if p := c.usePath("StrokePath", path); p != nil {
			p.in(&r.kept, &r.m)
			r.strokeLines(c, &r.kept)
		}
	}
}

// Clip cuts what is drawn from now on to the inside of the path, besides the
// clips made before, until the Restore that brings back the state before it.
// An empty path hides everything.
func (c *Canvas) Clip() {
	if r := c.use("Clip"); r != nil {
		r.clip(c, &r.path, nil)
	}
}

// ClipPath cuts what is drawn from now on to the inside of path, as Clip does
// to the one of the current path, with the transform current now.
func (c *Canvas) ClipPath(path *Path2D) {
	if r := c.use("ClipPath"); r != nil {
		if p := c.usePath("ClipPath", path); p != nil {
			r.clip(c, p, &r.m)
		}
	}
}

// FillRect fills a rectangle with FillStyle. The path stays as it is.
func (c *Canvas) FillRect(x, y, width, height float64) {
	r := c.use("FillRect")
	if r == nil || !finite(x, y, width, height) {
		return
	}
	if width == 0 || height == 0 {
		if _, ok := r.m.invert(); ok {
			r.draw(c, &drawing{}, ink{})
		}
		return
	}
	if r.m.axisAligned() && c.plain() {
		// The usual case: one color.
		if k, _ := r.ink(inkFill, c.FillStyle, c.alpha(), &r.m, c.ImageSmoothing); k.solid {
			r.drawn = true
			a, b := r.m.point(x, y), r.m.point(x+width, y+height)
			r.b.FillRect(gfx.Rect{X0: a.X, Y0: a.Y, X1: b.X, Y1: b.Y}, k.paint) // made upright
			return
		}
	}
	r.rect.reset()
	r.rect.rect(&r.m, x, y, width, height)
	r.fill(c, &r.rect, nil)
}

// StrokeRect draws the outline of a rectangle with StrokeStyle. The path
// stays as it is.
func (c *Canvas) StrokeRect(x, y, width, height float64) {
	r := c.use("StrokeRect")
	if r == nil || !finite(x, y, width, height) || width == 0 && height == 0 {
		return
	}
	r.rect.reset()
	if width == 0 || height == 0 {
		// A line there and back, as in Chromium: it has corners, not ends.
		r.rect.moveTo(r.m.vec(x, y))
		r.rect.lineTo(r.m.vec(x+width, y+height))
		r.rect.close()
	} else {
		r.rect.rect(&r.m, x, y, width, height)
	}
	r.strokeLines(c, &r.rect)
}

// ClearRect makes a rectangle of the canvas transparent again, inside the
// clip. The path stays as it is.
//
// A canvas starts every draw empty: only what the draw itself painted needs
// clearing. A rectangle that covers the whole canvas, with no clip, forgets
// everything drawn so far, and costs nothing; another one makes the canvas a
// layer of its own. Upright, it clears whole pixels of the screen.
func (c *Canvas) ClearRect(x, y, width, height float64) {
	r := c.use("ClearRect")
	if r == nil || !finite(x, y, width, height) || !r.drawn || width == 0 || height == 0 {
		return
	}
	if r.clips == 0 && r.covers(x, y, width, height) {
		r.forget()
		return
	}
	if inv, ok := r.m.invert(); ok && r.m.axisAligned() {
		// Whole pixels, as in Chromium, which does not antialias it: what is
		// cleared leaves no traces of its edges.
		at := [4]float64{min(x, x+width), min(y, y+height), max(x, x+width), max(y, y+height)}
		at, _ = r.snap(&inv, at)
		x, y, width, height = at[0], at[1], at[2]-at[0], at[3]-at[1]
	}
	r.rect.reset()
	r.rect.rect(&r.m, x, y, width, height)
	r.isolate = true
	r.b.PushLayer(1, gfx.DstOut)
	if r.rect.hasBox {
		r.b.FillRect(r.rect.box.rrect().Rect, gfx.Black)
	} else {
		r.rect.gfx(&r.geom, nil)
		r.b.FillPath(r.geom, gfx.NonZero, gfx.Black)
	}
	r.b.PopLayer()
}

// forget drops everything drawn so far; no clip is open.
func (r *Recorder) forget() {
	r.b.PopClip()
	r.b.Finish()
	r.b.PushClipRect(r.size)
	r.drawn, r.isolate = false, false
}

// covers reports whether a rectangle of the transform's coordinates covers
// the whole canvas.
func (r *Recorder) covers(x, y, width, height float64) bool {
	inv, ok := r.m.invert()
	if !ok {
		return false
	}
	x0, y0 := min(x, x+width), min(y, y+height)
	x1, y1 := max(x, x+width), max(y, y+height)
	// A hair of rounding, since the corners of the canvas go through the
	// inverse: a ten thousandth of a pixel of the canvas.
	ex := 1e-4 * (math.Abs(inv.a) + math.Abs(inv.c))
	ey := 1e-4 * (math.Abs(inv.b) + math.Abs(inv.d))
	for _, p := range [4]gfx.Point{{}, {X: r.size.X1}, {Y: r.size.Y1}, {X: r.size.X1, Y: r.size.Y1}} {
		px, py := inv.at(float64(p.X), float64(p.Y))
		if !(px >= x0-ex && px <= x1+ex && py >= y0-ey && py <= y1+ey) {
			return false
		}
	}
	return true
}

// clip opens a clip to path, transformed by m (nil for none).
func (r *Recorder) clip(c *Canvas, p *path, m *matrix) {
	r.clips++
	if p.hasBox && !p.oval && (m == nil || m.axisAligned()) {
		b := p.box
		if m != nil {
			b = b.in(m)
		}
		if rr := b.rrect(); rr.IsRect() {
			r.b.PushClipRect(rr.Rect)
		} else {
			r.b.PushClipRRect(rr)
		}
		return
	}
	p.gfx(&r.geom, m)
	r.b.PushClipPath(r.geom, c.FillRule)
}

// ---------------------------------------------------------------------
// Drawings.

// drawing is what one method draws, the same for itself and its shadow.
type drawing struct {
	kind drawingKind
	// under: the drawing is in the coordinates that m maps to the canvas's,
	// and recorded under that transform; else it is in the canvas's.
	under bool
	m     matrix
	path  *gfx.Path    // drawPath, drawStroke
	parts []gfx.Path   // drawStroke: the parts of path, stroked one by one
	rule  gfx.FillRule // drawPath
	box   gfx.RRect    // drawBox, and when boxed the shape of the shadow
	boxed bool
	line  gfx.StrokeStyle // drawStroke
	text  *shaped         // drawText
	at    gfx.Point       // drawText: where its left end is, on the baseline
	// mask is the shape of a drawMask, which is painted under its transform
	// over area.
	mask *gfx.DisplayList
	area gfx.Rect
	// What drawImage draws: src of img, with sampling, in each of the nx
	// columns and ny rows, cut to them when cut.
	img      *gfx.Image
	src      gfx.Rect
	xs, ys   [3]imagePiece
	nx, ny   int
	cut      bool
	sampling gfx.Sampling
	// What drawIcon does: list, of doc, in dst.
	doc  *svg.Doc
	list *gfx.DisplayList
	dst  gfx.Rect
}

type drawingKind uint8

const (
	drawPath drawingKind = iota
	drawBox
	drawStroke
	drawText
	drawMask
	drawImage
	drawIcon
)

// alpha returns GlobalAlpha as it applies.
func (c *Canvas) alpha() float32 {
	if c.GlobalAlpha >= 1 {
		return 1
	}
	if a := float32(c.GlobalAlpha); a > 0 {
		return a
	}
	return 0
}

// plain reports whether what is drawn is recorded as it is: source-over,
// without a shadow.
func (c *Canvas) plain() bool {
	return !(c.ShadowColor.A > 0) && c.GlobalCompositeOperation.blend() == gfx.SrcOver
}

// nothing reports that a path is not drawn at all, so that not even an
// unbounded operation clears what is outside it: as in Chromium, an empty
// one, a lone line or point (which its fill, and its stroke, skip), and any
// path under a transform that has no inverse. (The filled point and other
// paths of no area are drawn, and paint nothing.)
func (r *Recorder) nothing(p *path, stroke bool) bool {
	switch n := len(p.verbs); {
	case n == 0, stroke && n == 1, !stroke && n == 2 && p.verbs[1] == gfx.LineTo:
		return true
	}
	_, ok := r.m.invert()
	return !ok
}

// fill fills path, transformed by m (nil for none).
func (r *Recorder) fill(c *Canvas, p *path, m *matrix) {
	if r.nothing(p, false) {
		return
	}
	k, distorted := r.ink(inkFill, c.FillStyle, c.alpha(), &r.m, c.ImageSmoothing)
	if k.solid && (!p.hasBox || p.oval) && c.plain() { // the usual case
		r.drawn = true
		p.gfx(&r.geom, m)
		r.b.FillPath(r.geom, c.FillRule, k.paint)
		return
	}
	d := drawing{kind: drawPath, rule: c.FillRule}
	switch {
	case distorted:
		// In the coordinates of the gradient. (The transform has an inverse,
		// or there would be no paint.)
		d.under, d.m = true, r.m
		k, _ = r.ink(inkFill, c.FillStyle, c.alpha(), &identity, c.ImageSmoothing)
		if m != nil {
			p.gfx(&r.geom, nil)
		} else {
			inv, _ := r.m.invert()
			p.gfx(&r.geom, &inv)
		}
		d.path = &r.geom
	case p.hasBox && (m == nil || m.axisAligned()):
		b := p.box
		if m != nil {
			b = b.in(m)
		}
		d.kind, d.box, d.boxed = drawBox, b.rrect(), true
		if p.oval {
			d.kind, d.boxed = drawPath, math.Abs(b.r[0].x-b.r[0].y) <= 1e-6*b.r[0].x
			p.gfx(&r.geom, m)
			d.path = &r.geom
		}
	default:
		p.gfx(&r.geom, m)
		d.path = &r.geom
	}
	r.draw(c, &d, k)
}

// strokeLines strokes path.
func (r *Recorder) strokeLines(c *Canvas, p *path) {
	if r.nothing(p, true) {
		return
	}
	if scale, ok := r.m.similar(); ok && c.LineWidth > 0 && c.plain() && !math.IsInf(c.LineWidth, 0) {
		if k, _ := r.ink(inkStroke, c.StrokeStyle, c.alpha(), &r.m, c.ImageSmoothing); k.solid { // the usual case
			r.drawn = true
			p.gfx(&r.geom, nil)
			r.b.StrokePath(r.geom, r.line(c, scale, &r.geom), k.paint)
			return
		}
	}
	d := drawing{kind: drawStroke}
	if !r.stroke(c, &d, p) {
		r.draw(c, &d, ink{})
		return
	}
	g := &r.m
	if d.under {
		g = &identity
	}
	k, distorted := r.ink(inkStroke, c.StrokeStyle, c.alpha(), g, c.ImageSmoothing)
	if distorted {
		if !r.strokeUnder(c, &d, p) {
			return
		}
		k, _ = r.ink(inkStroke, c.StrokeStyle, c.alpha(), &identity, c.ImageSmoothing)
	}
	r.draw(c, &d, k)
}

// stroke makes d the stroke of path with the line of now, and reports whether
// there is anything to draw.
func (r *Recorder) stroke(c *Canvas, d *drawing, p *path) bool {
	if len(p.verbs) == 0 || !(c.LineWidth > 0) || math.IsInf(c.LineWidth, 0) {
		return false
	}
	k, ok := r.m.similar()
	if !ok {
		return r.strokeUnder(c, d, p)
	}
	p.gfx(&r.geom, nil)
	d.path = &r.geom
	d.line = r.line(c, k, d.path)
	return true
}

// strokeUnder is stroke for a transform that distorts the line, which is
// drawn under it, over the path taken back to its coordinates.
func (r *Recorder) strokeUnder(c *Canvas, d *drawing, p *path) bool {
	inv, ok := r.m.invert()
	if !ok {
		return false // everything is on a line
	}
	p.gfx(&r.geom, &inv)
	d.path, d.under, d.m = &r.geom, true, r.m
	d.line = r.line(c, 1, d.path)
	return true
}

// line returns the line style of now, scaled by k, for path.
func (r *Recorder) line(c *Canvas, k float64, p *gfx.Path) gfx.StrokeStyle {
	s := gfx.StrokeStyle{Width: float32(c.LineWidth * k), MiterLimit: DefaultMiterLimit, Join: c.LineJoin, Cap: c.LineCap}
	if c.MiterLimit > 0 && !math.IsInf(c.MiterLimit, 0) {
		s.MiterLimit = float32(max(c.MiterLimit, 1))
		if float64(s.MiterLimit) < c.MiterLimit {
			// Corners at the limit, such as right ones at the square root of
			// 2, have a miter.
			s.MiterLimit = math.Nextafter32(s.MiterLimit, math.MaxFloat32)
		}
	}
	if s.Join == gfx.JoinMiter {
		s.MiterLimit = miterReach(p, s.MiterLimit)
	}
	if len(c.dash) > 0 {
		r.dash = r.dash[:0]
		for _, v := range c.dash {
			r.dash = append(r.dash, float32(v*k))
		}
		s.Dashes = r.dash // which the Builder copies
		if o := float64(c.LineDashOffset * k); finite(o) {
			s.DashOffset = float32(o)
		}
	}
	return s
}

// miterReach returns a miter limit, at most limit, with which the corners of
// p are drawn as with limit: the longest miter among them that limit allows.
// The Builder takes a stroke's bounds for those of the longest miter there
// may be, and a canvas that strokes lines would damage several times what it
// paints.
func miterReach(p *gfx.Path, limit float32) float32 {
	if limit > 1000 {
		return limit // corners that come back on themselves are beyond the arithmetic below
	}
	// Corners are compared by the square of their miter, 2/(1+cos), with a
	// margin for the rounding of the tangents, here and in the stroker.
	const margin = 1.002
	limit2 := float64(float64(limit) * float64(limit) * margin)
	worst := 1.0
	var start, cur gfx.Point
	var in, first vec // directions: of the last segment, of the subpath's first
	have := false     // in and first are
	pts := p.Points()
	for _, verb := range p.Verbs() {
		n := verb.NumPoints()
		end := start
		switch verb {
		case gfx.MoveTo:
			start, cur, have = pts[0], pts[0], false
			pts = pts[1:]
			continue
		case gfx.Close:
			if cur != start {
				d := toward(cur, start)
				if tiny(d, cur) {
					return limit
				}
				if have {
					worst = miter2(in, d, worst, limit2)
				}
				in = d
			}
			if have {
				worst = miter2(in, first, worst, limit2)
			}
			cur, have = start, false
			continue
		default:
			end = pts[n-1]
		}
		// The stroker has its own idea of the tangent of a curve whose
		// control points are (almost) its ends.
		d0 := toward(cur, pts[0])
		d1 := d0
		if n > 1 {
			d1 = toward(pts[n-2], end)
		}
		if tiny(d0, cur) || tiny(d1, end) {
			return limit
		}
		if have {
			worst = miter2(in, d0, worst, limit2)
		} else {
			first, have = d0, true
		}
		in, cur, pts = d1, end, pts[n:]
	}
	return min(limit, float32(math.Sqrt(float64(worst*margin))))
}

// toward returns the vector from a to b.
func toward(a, b gfx.Point) vec {
	return vec{float64(b.X) - float64(a.X), float64(b.Y) - float64(a.Y)}
}

// tiny reports whether d, a vector between points near at, has no direction
// to speak of.
func tiny(d vec, at gfx.Point) bool {
	return math.Abs(d.x)+math.Abs(d.y) <= 1e-6*(1+math.Abs(float64(at.X))+math.Abs(float64(at.Y)))
}

// miter2 returns the larger of worst and the square of the miter between two
// directions, unless it is over limit2.
func miter2(a, b vec, worst, limit2 float64) float64 {
	dot := float64(a.x*b.x) + float64(a.y*b.y)
	n := math.Sqrt(float64((float64(a.x*a.x) + float64(a.y*a.y)) * (float64(b.x*b.x) + float64(b.y*b.y))))
	if m := 2 * n / max(n+dot, 1e-9*n); m <= limit2 {
		return max(worst, m)
	}
	return worst
}

// ---------------------------------------------------------------------
// Compositing.

// draw draws d with ink k and its shadow. An ink of nothing still clears the
// clip with an unbounded operation.
func (r *Recorder) draw(c *Canvas, d *drawing, k ink) {
	op := c.GlobalCompositeOperation.blend()
	visible := k.paint != nil || k.alpha > 0 && (d.kind == drawImage || d.kind == drawIcon)
	if !visible {
		if op.Unbounded() && r.drawn {
			r.isolate = true
			r.b.PushLayer(0, op)
			r.b.PopLayer()
		}
		return
	}
	r.drawn = true
	if c.ShadowColor.A > 0 && c.ShadowColor.IsFinite() {
		if s := c.shadow(); s.blur > 0 || s.dx != 0 || s.dy != 0 {
			r.shadow(d, k, &s, op)
		}
	}
	if op == gfx.SrcOver {
		r.emit(d, k)
		return
	}
	r.isolate = true
	r.b.PushLayer(k.alpha, op)
	k.alpha = 1
	r.emit(d, k)
	r.b.PopLayer()
}

// shadow is the shadow of a canvas.
type shadow struct {
	color  gfx.Color
	blur   float32 // ShadowBlur: twice the standard deviation
	dx, dy float32
}

func (c *Canvas) shadow() shadow {
	s := shadow{color: c.ShadowColor.Clamp()}
	if b := float32(c.ShadowBlur); b > 0 && !math.IsInf(float64(b), 0) {
		s.blur = b
	}
	if x, y := float64(float32(c.ShadowOffsetX)), float64(float32(c.ShadowOffsetY)); finite(x, y) {
		s.dx, s.dy = float32(x), float32(y)
	}
	return s
}

// shadow draws the shadow s of d, painted with k.
func (r *Recorder) shadow(d *drawing, k ink, s *shadow, op gfx.BlendMode) {
	b := r.b
	moved := func(k ink) {
		if s.dx != 0 || s.dy != 0 {
			b.PushTransform(gfx.Translate(s.dx, s.dy))
			defer b.PopTransform()
		}
		r.emit(d, k)
	}
	if !k.solid || d.kind == drawText && d.text.color {
		// The color matrix keeps the alpha, times the shadow's.
		filters := [2]gfx.Filter{
			gfx.ColorMatrix{M: [20]float32{4: s.color.R, 9: s.color.G, 14: s.color.B, 18: s.color.A}},
			gfx.Blur{StdDev: s.blur / 2},
		}
		n := 2
		if s.blur == 0 {
			n = 1
		}
		r.isolate = r.isolate || op != gfx.SrcOver
		b.PushLayer(k.alpha, op, filters[:n]...)
		k.alpha = 1
		moved(k)
		b.PopLayer()
		return
	}
	color := s.color
	color.A = float32(color.A * float32(k.color.A*k.alpha))
	sk := ink{paint: color, alpha: 1, color: color, solid: true}
	closed := s.blur > 0 && d.boxed && closedShadow(d.box)
	layer := op != gfx.SrcOver || s.blur > 0 && !closed
	if layer {
		r.isolate = r.isolate || op != gfx.SrcOver
		if s.blur > 0 && !closed {
			b.PushLayer(1, op, gfx.Blur{StdDev: s.blur / 2})
		} else {
			b.PushLayer(1, op)
		}
	}
	if closed {
		// DrawShadow leaves out the box itself, as CSS does: the box it is
		// given is far from here, and so is its shadow from it. The distance
		// is a power of two, so that little of the precision is lost.
		far := float32(1024)
		for reach := d.box.Rect.Width() + 4*s.blur + float32(math.Abs(float64(s.dx))); far < 4*reach; {
			far *= 2
		}
		rr := d.box
		rr.Rect = rr.Rect.Translate(-far, 0)
		b.DrawShadow(gfx.DrawShadow{RRect: rr, Offset: gfx.Point{X: far + s.dx, Y: s.dy}, Blur: s.blur, Color: color})
	} else {
		moved(sk)
	}
	if layer {
		b.PopLayer()
	}
}

// closedShadow reports whether the closed form of a blurred shadow is right
// for rr: its size and place are within what the precision of float32 leaves.
func closedShadow(rr gfx.RRect) bool {
	const limit = 1 << 14
	r := rr.Rect
	return !r.Empty() && r.X0 > -limit && r.Y0 > -limit && r.X1 < limit && r.Y1 < limit
}

// emit records d with ink k.
func (r *Recorder) emit(d *drawing, k ink) {
	b := r.b
	if k.alpha != 1 {
		b.PushLayer(k.alpha, gfx.SrcOver)
	}
	if d.under {
		b.PushTransform(d.m.affine())
	}
	switch d.kind {
	case drawPath:
		b.FillPath(*d.path, d.rule, k.paint)
	case drawBox:
		if d.box.IsRect() {
			b.FillRect(d.box.Rect, k.paint)
		} else {
			b.FillRRect(d.box, k.paint)
		}
	case drawStroke:
		for i := range d.parts {
			b.StrokePath(d.parts[i], d.line, k.paint)
		}
		if d.parts == nil {
			b.StrokePath(*d.path, d.line, k.paint)
		}
	case drawText:
		for _, run := range d.text.runs {
			run.Origin.X += d.at.X
			run.Origin.Y = d.at.Y
			run.Paint = k.paint
			b.DrawGlyphRun(run)
		}
	case drawMask:
		b.PushMaskLayer(gfx.Mask{List: d.mask, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
		b.PushTransform(r.m.affine())
		b.FillRect(d.area, k.paint)
		b.PopTransform()
		b.PopLayer()
	case drawImage:
		for _, y := range d.ys[:d.ny] {
			for _, x := range d.xs[:d.nx] {
				if d.cut {
					b.PushClipRect(gfx.Rect{X0: float32(x.lo), Y0: float32(y.lo), X1: float32(x.hi), Y1: float32(y.hi)})
				}
				b.DrawImage(d.img, d.src, gfx.Rect{X0: float32(x.p), Y0: float32(y.p), X1: float32(x.q), Y1: float32(y.q)}, d.sampling)
				if d.cut {
					b.PopClip()
				}
			}
		}
	case drawIcon:
		if k.paint != nil {
			// The shape of the icon, painted: see DrawIcon.
			b.PushMaskLayer(gfx.Mask{List: d.mask, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
			b.FillRect(d.dst, k.paint)
			b.PopLayer()
		} else {
			d.doc.DrawRecorded(b, d.dst, d.list)
		}
	}
	if d.under {
		b.PopTransform()
	}
	if k.alpha != 1 {
		b.PopLayer()
	}
}

// ---------------------------------------------------------------------
// Line dashes.

// SetLineDash makes the lines dashed: the lengths of a dash, a gap, a dash...
// in turns, which repeat. An odd number of them is repeated once to be even.
// Without any, or with a sum of 0, lines are whole. A call with a negative,
// infinite or NaN length is ignored.
func (c *Canvas) SetLineDash(segments ...float64) {
	if c.use("SetLineDash") == nil {
		return
	}
	for _, s := range segments {
		if !finite(s) || s < 0 {
			return
		}
	}
	if len(segments) == 0 {
		c.dash = nil
		return
	}
	// A new slice: the saved states keep theirs.
	d := make([]float64, 0, 2*len(segments))
	d = append(d, segments...)
	if len(segments)%2 != 0 {
		d = append(d, segments...)
	}
	c.dash = d
}

// LineDash returns the lengths given to SetLineDash, an even number of them,
// and none when lines are whole.
func (c *Canvas) LineDash() []float64 {
	if c.use("LineDash") == nil {
		return nil
	}
	return slices.Clone(c.dash)
}
