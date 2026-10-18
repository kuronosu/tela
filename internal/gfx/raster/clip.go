package raster

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Clips and layers.
//
// The clip is the blitter's clip rectangle (device pixels that may be
// written: the region, intersected with the bounds of every active clip)
// plus an optional 8-bit coverage mask (blitter.cm). A clip push:
//
//   - for a rectangle whose device edges fall on whole pixels (under a
//     transform that keeps it axis-aligned), only shrinks the clip
//     rectangle: no mask;
//   - otherwise rasterizes the shape's coverage, with the same coverage
//     producers as fills (rect, rrect and path fast paths included), into a
//     new mask that covers only the clip rectangle ∩ the shape's device
//     bounds, multiplied by the enclosing mask (nested clips intersect). The
//     mask values depend only on the geometry and the absolute pixel
//     coordinates, so region renders stay bit-identical. A mask records a
//     rectangle where it is known to be 255 (the interior of a rectangle or
//     the central band of a rounded rectangle), where drawing skips the
//     multiplication.
//
// A layer (PushLayer) renders its content into an offscreen premultiplied
// buffer that covers the clip rectangle ∩ the layer's device bounds,
// starting transparent and with no clip mask: the clip that is active at
// PushLayer is applied once, at PopLayer, when the buffer is composited
// with the layer's opacity, mask and blend mode through the clip mask (see
// blend.go). Clipping the group as a whole, rather than each child, is what
// CSS does for opacity groups and keeps antialiased clip edges free of
// seams.
//
// A layer's mask (gfx.Mask) is rendered at PushLayer, before the content:
// its list is drawn, under the transform current at PushLayer, onto a
// transparent surface over the pixels the layer composites (the whole
// device bounds of the mask when its list holds backdrop filters that
// spread, which read their surface), with no clip, and converted to an
// 8-bit coverage (alpha, or luminance times alpha). The mask is not clipped
// by the region or by the clips around the layer, only cut to the pixels
// that are composited, so its values depend on the geometry and the
// absolute pixel coordinates only.

// pushClip intersects the clip with the shape of a PushClip* command. The
// state has already been saved.
func (r *Renderer) pushClip(c gfx.Command) {
	switch c := c.(type) {
	case gfx.PushClipRect:
		r.clipRect(c.Rect)
	case gfx.PushClipRRect:
		rr := c.RRect
		if rr.IsRect() {
			r.clipRect(rr.Rect)
			return
		}
		bounds, inner := r.rectBounds(rr.Rect), image.Rectangle{}
		if r.ctm.IsScaleTranslate() {
			inner = r.rrectInner(rr)
		}
		if r.beginMask(bounds) {
			r.coverRRect(rr)
			r.endMask(inner)
		}
	case gfx.PushClipPath:
		if r.beginMask(r.pathBounds(c.Path)) {
			r.fillPath(c.Path, c.Rule)
			r.endMask(image.Rectangle{})
		}
	}
}

// clipRect intersects the clip with a local-space rectangle.
func (r *Renderer) clipRect(rect gfx.Rect) {
	m := &r.ctm
	if !m.IsAxisAligned() {
		if r.beginMask(r.rectBounds(rect)) {
			r.coverRect(rect)
			r.endMask(image.Rectangle{})
		}
		return
	}
	x0, y0 := apply(m, gfx.Pt(rect.X0, rect.Y0))
	x1, y1 := apply(m, gfx.Pt(rect.X1, rect.Y1))
	X0, Y0 := fix(clampLim(min(x0, x1))), fix(clampLim(min(y0, y1)))
	X1, Y1 := fix(clampLim(max(x0, x1))), fix(clampLim(max(y0, y1)))
	// The pixels that the rectangle touches, and those it covers fully.
	outer := image.Rect(int(X0>>fracBits), int(Y0>>fracBits), int((X1+one-1)>>fracBits), int((Y1+one-1)>>fracBits))
	full := rectNoSwap(int((X0+one-1)>>fracBits), int((Y0+one-1)>>fracBits), int(X1>>fracBits), int(Y1>>fracBits))
	if X0 >= X1 || Y0 >= Y1 {
		outer = image.Rectangle{}
	}
	if outer == full { // pixel aligned: no mask needed
		r.narrow(outer)
		return
	}
	if r.beginMask(outer) {
		r.fillRectFixed(X0, Y0, X1, Y1)
		r.endMask(full)
	}
}

// narrow intersects the clip rectangle with rect; an empty result makes
// everything up to the matching pop invisible.
func (r *Renderer) narrow(rect image.Rectangle) {
	b := &r.blit
	r.bound = r.bound.Intersect(rect)
	b.clip = b.clip.Intersect(rect)
	if b.clip.Empty() {
		b.clip = image.Rectangle{}
		r.skipping = true
	}
}

// beginMask narrows the clip rectangle to bounds (the device pixels the
// clip shape may cover) and prepares a new mask over it; the caller then
// rasterizes the shape's coverage and calls endMask. It returns false, with
// everything clipped away, if nothing is left.
func (r *Renderer) beginMask(bounds image.Rectangle) bool {
	r.narrow(bounds)
	if r.skipping {
		return false
	}
	b := &r.blit
	buf := r.getBuf(b.clip.Dx() * b.clip.Dy())
	r.stack[len(r.stack)-1].ownMask = buf
	b.toMask = true
	b.a8, b.a8Stride, b.a8Origin = buf, b.clip.Dx(), b.clip.Min
	return true
}

// endMask makes the mask rasterized since beginMask the active clip mask.
// inner is a rectangle where the shape covers every pixel fully.
func (r *Renderer) endMask(inner image.Rectangle) {
	b := &r.blit
	if b.cm.data != nil {
		inner = inner.Intersect(b.cm.inner)
	}
	b.cm = clipMask{data: b.a8, stride: b.a8Stride, rect: b.clip, inner: inner.Intersect(b.clip)}
	b.toMask, b.a8 = false, nil
}

// rectNoSwap is image.Rect without the swapping of inverted coordinates: an
// inverted rectangle is empty (the zero Rectangle).
func rectNoSwap(x0, y0, x1, y1 int) image.Rectangle {
	if x0 >= x1 || y0 >= y1 {
		return image.Rectangle{}
	}
	return image.Rectangle{Min: image.Pt(x0, y0), Max: image.Pt(x1, y1)}
}

// pixelBounds returns the device pixels that a device-space box can touch,
// with a margin of one pixel for rounding, clamped to ±coordLimit.
func pixelBounds(x0, y0, x1, y1 float64) image.Rectangle {
	if !(x0 <= x1 && y0 <= y1) { // also NaN
		return image.Rectangle{}
	}
	return image.Rect(
		int(math.Floor(clampLim(x0)))-1, int(math.Floor(clampLim(y0)))-1,
		int(math.Ceil(clampLim(x1)))+1, int(math.Ceil(clampLim(y1)))+1,
	)
}

// affineBounds returns the device pixels that a rectangle can touch under
// m.
func affineBounds(m *gfx.Affine, rect gfx.Rect) image.Rectangle {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4]gfx.Point{{X: rect.X0, Y: rect.Y0}, {X: rect.X1, Y: rect.Y0}, {X: rect.X1, Y: rect.Y1}, {X: rect.X0, Y: rect.Y1}} {
		x, y := apply(m, p)
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	return pixelBounds(x0, y0, x1, y1)
}

// rectBounds returns the device pixels that a local-space rectangle can
// touch under the current transform.
func (r *Renderer) rectBounds(rect gfx.Rect) image.Rectangle {
	return affineBounds(&r.ctm, rect)
}

// pathBounds returns the device pixels that a local-space path can touch
// under the current transform (from its control points).
func (r *Renderer) pathBounds(p gfx.Path) image.Rectangle {
	m := &r.ctm
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, pt := range p.Points() {
		x, y := apply(m, pt)
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	return pixelBounds(x0, y0, x1, y1)
}

// rrectInner returns pixels that a rounded rectangle covers fully under the
// current scale/translate transform: the larger of its horizontal band
// (full width, between the corners' vertical extents) and its vertical band,
// shrunk by one pixel for rounding. It may be empty.
func (r *Renderer) rrectInner(rr gfx.RRect) image.Rectangle {
	m := &r.ctm
	x0, y0 := apply(m, gfx.Pt(rr.Rect.X0, rr.Rect.Y0))
	x1, y1 := apply(m, gfx.Pt(rr.Rect.X1, rr.Rect.Y1))
	x0, x1 = min(x0, x1), max(x0, x1)
	y0, y1 = min(y0, y1), max(y0, y1)
	var rx, ry float64
	for _, p := range rr.Radii {
		rx = max(rx, float64(p.X)*math.Abs(float64(m[0])))
		ry = max(ry, float64(p.Y)*math.Abs(float64(m[3])))
	}
	in := func(x0, y0, x1, y1 float64) image.Rectangle {
		if !(x0 < x1 && y0 < y1) || x0 < -coordLimit || y0 < -coordLimit || x1 > coordLimit || y1 > coordLimit {
			return image.Rectangle{}
		}
		return rectNoSwap(int(math.Ceil(x0))+1, int(math.Ceil(y0))+1, int(math.Floor(x1))-1, int(math.Floor(y1))-1)
	}
	h := in(x0, y0+ry, x1, y1-ry)
	v := in(x0+rx, y0, x1-rx, y1)
	if h.Empty() || (!v.Empty() && v.Dx()*v.Dy() > h.Dx()*h.Dy()) {
		return v
	}
	return h
}

// pushLayer starts an offscreen layer for the PushLayer at index i of the
// list. The state has already been saved.
func (r *Renderer) pushLayer(i int, c gfx.PushLayer) {
	op := unit8(float64(c.Opacity))
	if op == 0 {
		r.narrow(image.Rectangle{})
		return
	}
	if len(c.Filters) > 0 {
		r.pushFilterLayer(i, c, op)
		return
	}
	// The layer's root-space bounds (its content, clipped by the clip at
	// PushLayer) in device pixels.
	r.narrow(r.devBounds(r.dl.BoundsAt(i)))
	if r.skipping {
		return
	}
	b := &r.blit
	rect := b.clip
	mask := r.renderMask(c.Mask, rect)
	buf := r.getBuf(4 * rect.Dx() * rect.Dy())
	st := &r.stack[len(r.stack)-1]
	st.layer, st.pix, st.stride, st.origin, st.surf, st.opacity = true, b.pix, b.stride, b.origin, buf, op
	st.comp, st.surfRect, st.blend, st.mask = rect, rect, c.Blend, mask
	// The saved clip is the one before narrow: the layer is composited
	// within the narrowed rectangle, which lies inside it.
	b.pix, b.stride, b.origin, b.cm = buf, 4*rect.Dx(), rect.Min, clipMask{}
	r.surfBounds = rect
}

// renderMask renders the mask m of a layer that composites the device
// pixels comp (not empty) and returns its 8-bit coverage over comp (stride
// comp.Dx()), or nil when the layer has no mask. The blitter still targets
// the surface below the layer, which is left untouched.
func (r *Renderer) renderMask(m gfx.Mask, comp image.Rectangle) []uint8 {
	if m.List == nil {
		return nil
	}
	lim := image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit)
	rect := comp
	if m.List.BackdropReach() > 0 {
		// Its backdrop filters read their whole shapes on the mask's
		// surface: render all of it, so that they never see the region.
		saved := r.root
		r.root = r.ctm
		rect = r.devBounds(m.List.Bounds()).Intersect(lim).Union(comp)
		r.root = saved
	}
	b := &r.blit
	pix := r.getBuf(4 * rect.Dx() * rect.Dy())
	surf, stride, origin := b.pix, b.stride, b.origin
	dl, root := r.dl, r.root
	r.save()
	b.pix, b.stride, b.origin = pix, 4*rect.Dx(), rect.Min
	b.clip, b.cm = rect, clipMask{}
	r.region, r.bound, r.surfBounds = rect, lim, rect
	r.skipping = false
	// The root space of the mask list is the current local space.
	r.dl, r.root = m.List, r.ctm
	for i := range m.List.Len() {
		r.visit(i, m.List.At(i))
	}
	r.dl, r.root = dl, root
	r.restore()
	b.pix, b.stride, b.origin = surf, stride, origin

	cov := r.getBuf(comp.Dx() * comp.Dy())
	w := comp.Dx()
	for y := comp.Min.Y; y < comp.Max.Y; y++ {
		i := (y-rect.Min.Y)*4*rect.Dx() + 4*(comp.Min.X-rect.Min.X)
		maskCoverage(cov[(y-comp.Min.Y)*w:(y-comp.Min.Y+1)*w], pix[i:i+4*w], w, m.Mode)
	}
	r.putBuf(pix)
	return cov
}
