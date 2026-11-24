package canvas

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Fill fills the path with FillStyle, its open subpaths as if they were
// closed.
func (c *Canvas) Fill() {
	r := c.use("Fill")
	if r == nil || r.path.IsEmpty() {
		return
	}
	if p, ok := r.paint(paintFill, c.FillStyle, c.GlobalAlpha); ok {
		r.drawn = true
		r.b.FillPath(r.path, c.FillRule, p)
	}
}

// Stroke draws the lines of the path with StrokeStyle. Their shape (width,
// ends, corners) follows the transform current now, and their place the
// transforms that were current when their points were added.
func (c *Canvas) Stroke() {
	if r := c.use("Stroke"); r != nil {
		c.strokePath(r, &r.path)
	}
}

// Clip cuts what is drawn from now on to the inside of the path, besides the
// clips made before, until the Restore that brings back the state before it.
// An empty path hides everything.
func (c *Canvas) Clip() {
	if r := c.use("Clip"); r != nil {
		r.b.PushClipPath(r.path, c.FillRule)
		r.clips++
	}
}

// FillRect fills a rectangle with FillStyle. The path stays as it is.
func (c *Canvas) FillRect(x, y, width, height float64) {
	r := c.use("FillRect")
	if r == nil || !finite(x, y, width, height) || width == 0 || height == 0 {
		return
	}
	p, ok := r.paint(paintFill, c.FillStyle, c.GlobalAlpha)
	if !ok {
		return
	}
	r.drawn = true
	if r.m.axisAligned() {
		a, b := r.m.point(x, y), r.m.point(x+width, y+height)
		r.b.FillRect(gfx.Rect{X0: a.X, Y0: a.Y, X1: b.X, Y1: b.Y}, p) // made upright
		return
	}
	r.setRect(x, y, width, height)
	r.b.FillPath(r.rect, gfx.NonZero, p)
}

// StrokeRect draws the outline of a rectangle with StrokeStyle. The path
// stays as it is.
func (c *Canvas) StrokeRect(x, y, width, height float64) {
	r := c.use("StrokeRect")
	if r == nil || !finite(x, y, width, height) || width == 0 && height == 0 {
		return
	}
	if width == 0 || height == 0 {
		r.rect.Reset()
		r.rect.MoveTo(r.m.point(x, y))
		r.rect.LineTo(r.m.point(x+width, y+height))
	} else {
		r.setRect(x, y, width, height)
	}
	c.strokePath(r, &r.rect)
}

// ClearRect makes a rectangle of the canvas transparent again. The path
// stays as it is.
//
// A canvas starts every draw empty: only what the draw itself painted needs
// clearing. A rectangle that covers the whole canvas, with no clip, forgets
// everything drawn so far. For now any other rectangle clears nothing.
func (c *Canvas) ClearRect(x, y, width, height float64) {
	r := c.use("ClearRect")
	if r == nil || !finite(x, y, width, height) || !r.drawn {
		return
	}
	if r.clips > 0 || !r.covers(x, y, width, height) {
		// TODO(ADR-319, phase 31 T4): clear a part of the canvas, with the
		// Porter-Duff clear of a layer.
		return
	}
	r.b.PopClip()
	r.b.Finish()
	r.b.PushClipRect(r.size)
	r.drawn = false
}

// setRect makes r.rect the path of a rectangle of the transform's
// coordinates, in the canvas's.
func (r *Recorder) setRect(x, y, width, height float64) {
	r.rect.Reset()
	r.rect.MoveTo(r.m.point(x, y))
	r.rect.LineTo(r.m.point(x+width, y))
	r.rect.LineTo(r.m.point(x+width, y+height))
	r.rect.LineTo(r.m.point(x, y+height))
	r.rect.Close()
}

// covers reports whether a rectangle of the transform's coordinates covers
// the whole canvas.
func (r *Recorder) covers(x, y, width, height float64) bool {
	inv, ok := r.m.invert()
	if !ok {
		return false
	}
	x0, x1 := min(x, x+width), max(x, x+width)
	y0, y1 := min(y, y+height), max(y, y+height)
	// A hair of rounding: the corners of the canvas go through the inverse.
	eps := 1e-6 * (1 + math.Abs(x1-x0) + math.Abs(y1-y0))
	for _, p := range [4]gfx.Point{{}, {X: r.size.X1}, {Y: r.size.Y1}, {X: r.size.X1, Y: r.size.Y1}} {
		px, py := inv.at(float64(p.X), float64(p.Y))
		if !(px >= x0-eps && px <= x1+eps && py >= y0-eps && py <= y1+eps) {
			return false
		}
	}
	return true
}

// strokePath draws the lines of path, whose points are in the canvas's
// coordinates, with the line and the transform of now.
func (c *Canvas) strokePath(r *Recorder, path *gfx.Path) {
	if path.IsEmpty() || !(c.LineWidth > 0) || math.IsInf(c.LineWidth, 0) {
		return
	}
	p, ok := r.paint(paintStroke, c.StrokeStyle, c.GlobalAlpha)
	if !ok {
		return
	}
	// perf: the bounds of a JoinMiter stroke are outset by MiterLimit times
	// half its width, whatever its corners: a path of single lines, or of
	// curves, has no miters.
	style := gfx.StrokeStyle{MiterLimit: DefaultMiterLimit, Join: c.LineJoin, Cap: c.LineCap}
	if c.MiterLimit > 0 && !math.IsInf(c.MiterLimit, 0) {
		style.MiterLimit = float32(max(c.MiterLimit, 1))
	}
	if k, ok := r.m.similar(); ok {
		style.Width = float32(c.LineWidth * k)
		r.drawn = true
		r.b.StrokePath(*path, style, p)
		return
	}
	// The transform distorts the line: it is drawn under it, over the path
	// taken back to the transform's coordinates.
	inv, ok := r.m.invert()
	if !ok {
		return // everything is on a line
	}
	r.user.Reset()
	for el := range path.Elements() {
		for i := range el.Pts[:el.Verb.NumPoints()] {
			el.Pts[i] = inv.point(float64(el.Pts[i].X), float64(el.Pts[i].Y))
		}
		switch el.Verb {
		case gfx.MoveTo:
			r.user.MoveTo(el.Pts[0])
		case gfx.LineTo:
			r.user.LineTo(el.Pts[0])
		case gfx.QuadTo:
			r.user.QuadTo(el.Pts[0], el.Pts[1])
		case gfx.CubicTo:
			r.user.CubicTo(el.Pts[0], el.Pts[1], el.Pts[2])
		case gfx.Close:
			r.user.Close()
		}
	}
	style.Width = float32(c.LineWidth)
	r.drawn = true
	r.b.PushTransform(r.m.affine())
	r.b.StrokePath(r.user, style, p)
	r.b.PopTransform()
}
