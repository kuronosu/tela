package raster

import (
	"image"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
)

// Cached filtered backdrops (ADR-200).
//
// A backdrop filter that spreads pixels (a blur) computes each output
// pixel from the pixels within its reach, so a region render must first
// render the region grown by that reach (BackdropRegion) and then blur
// it. When what lies behind the filtered element did not change from one
// frame to the next (a fixed glass pane over a static wallpaper, whose
// content scrolls or moves above the filter), the output is the same
// pixels as before. A BackdropCache keeps that output between Renders:
// a filter whose output inside the region is in the cache draws it from
// there, and the region does not grow for it.
//
// The rasterizer cannot tell whether the pixels a filter reads changed
// without rendering them, which is the cost the cache saves. The owner of
// the destination image tells it, from the damage of two frames: Retain
// carries the entries of the filters whose input is the same pixels
// (core's damage, ADR-061, knows which ones). Everything else is checked
// here: the entry must belong to the same list position, scale and
// destination bounds, the filter must be drawn under the same transform
// over the same device rectangle, and the output pixels it needs must be
// held. The output of a filter does not depend on the region it was
// computed for (ADR-078), so the pixels drawn from the cache are those of
// an uncached render, bit for bit.
//
// Only the filters that spread pixels and lie outside every layer are
// cached: they read the destination itself (their input does not depend
// on the region), and they are the ones a phased render computes at its
// barriers.

// BackdropCache keeps the filtered backdrops of the backdrop filters of
// one display list between Renders into one destination image (see
// Options.Backdrops). The zero value is an empty cache, ready to use; a
// nil *BackdropCache caches nothing. It is not safe for concurrent use:
// Render writes it, from one goroutine at a time.
//
// Its memory is bounded: the entries and their spare buffers hold at most
// two destination images' worth of pixels.
type BackdropCache struct {
	dl    *gfx.DisplayList // the list of the entries; nil: none
	scale float32
	full  image.Rectangle // the destination bounds the entries belong to
	ents  []bdEntry       // one per backdrop filter of dl, in list order
	old   []bdEntry       // Retain's scratch
	idx   []int           // the command index of each backdrop filter of dl
	top   []bool          // whether it is cached: it spreads, outside layers
	// ctms is the device transform each backdrop filter of dl is drawn
	// under at scale, as the Renderer computes it (zero: it is never
	// drawn), so that BackdropRegion checks the whole key of an entry.
	ctms  []gfx.Affine
	open  []idxState // index's stack
	bytes int        // the pixels held by ents and spare
	spare [][]uint8  // buffers of dropped entries, for reuse
	// exact is, for the current Render, the device region inside which
	// each backdrop filter's output is computed from exact pixels (see
	// backdropRegion); its output elsewhere is not kept.
	exact []image.Rectangle
	// plan is, for the current Render, the device pixels of the output of
	// each backdrop filter that backdropRegion found in the cache (empty:
	// none): the render did not make exact what it reads, so it is drawn
	// from the cache, which holds these pixels. Its other pixels in the
	// grown region are never presented nor read by a filter's exact output.
	plan []image.Rectangle
	// hits counts the filters drawn from the cache (tests).
	hits int
}

// bdEntry is the cached output of one backdrop filter.
type bdEntry struct {
	// ok: pix holds the filter's output over out for the list's input.
	// stable: the entry was carried from the previous list, whose input
	// was the same pixels (Retain): a filter whose input does not change
	// from frame to frame, so filling all of its output pays off.
	ok, stable bool
	ctm        gfx.Affine      // the transform the filter was drawn under
	in         image.Rectangle // the device bounds of its shape in full
	filters    gfx.BackdropFilter
	bounds     gfx.Rect // the filter's root-space bounds in the list
	// cover holds the device pixels of the filter's bounds whose output
	// is in pix: out is in ∩ cover, whatever the clip it was drawn
	// through, so every pixel it draws inside cover lies in out.
	cover, out image.Rectangle
	pix        []uint8 // premultiplied RGBA over out
}

// idxState is the state index keeps per open block.
type idxState struct {
	layer bool
	ctm   gfx.Affine
	skip  bool
}

// maxSpare bounds the number of spare buffers.
const maxSpare = 4

// Reset empties the cache. A nil cache is ignored.
func (c *BackdropCache) Reset() {
	if c != nil {
		c.bind(nil, 0)
	}
}

// Retain moves the cache to dl, the next frame's list rendered at scale
// into the same destination: prev has one element per backdrop filter of
// dl (in list order), the index among the backdrop filters of the list
// the cache holds of a filter whose input is the same pixels (the same
// pixels painted under its shape before it), or -1. Their entries are
// kept; the others are dropped. A nil prev, or one of another length,
// empties the cache. A nil cache is ignored.
//
// The caller vouches for the input: a filter carried whose input changed
// draws its old output.
func (c *BackdropCache) Retain(dl *gfx.DisplayList, scale float32, prev []int) {
	if c == nil {
		return
	}
	if scale == 0 {
		scale = 1
	}
	if dl == nil || c.dl == nil || prev == nil || len(prev) != dl.NumBackdrops() || scale != c.scale {
		c.bind(dl, scale)
		return
	}
	c.old, c.ents = c.ents, c.old[:0]
	full := c.full
	c.index(dl)
	c.full, c.scale = full, scale
	c.ents = slices.Grow(c.ents[:0], len(prev))[:len(prev)]
	for k, p := range prev {
		c.ents[k] = bdEntry{}
		if p >= 0 && p < len(c.old) && c.old[p].ok && c.top[k] {
			c.ents[k] = c.old[p]
			c.ents[k].stable = true
			c.old[p] = bdEntry{}
		}
	}
	for k := range c.old {
		c.drop(&c.old[k])
	}
	clear(c.old)
	c.old = c.old[:0]
}

// bind empties the cache and makes it the cache of dl at scale.
func (c *BackdropCache) bind(dl *gfx.DisplayList, scale float32) {
	for k := range c.ents {
		c.drop(&c.ents[k])
	}
	clear(c.ents)
	c.ents = c.ents[:0]
	c.full = image.Rectangle{}
	c.scale = scale
	c.index(dl)
	if dl != nil {
		c.ents = slices.Grow(c.ents, len(c.idx))[:len(c.idx)]
	}
}

// index sets dl and computes idx, top and ctms for it at c.scale.
func (c *BackdropCache) index(dl *gfx.DisplayList) {
	c.dl = dl
	c.idx, c.top, c.ctms = c.idx[:0], c.top[:0], c.ctms[:0]
	if dl == nil {
		return
	}
	n := dl.NumBackdrops()
	for k := range n {
		i, _, reach, _ := dl.Backdrop(k)
		c.idx = append(c.idx, i)
		c.top = append(c.top, reach > 0)
	}
	c.ctms = slices.Grow(c.ctms, n)[:n]
	// The filters inside layers are not cached. The transform is the one
	// render computes (visit's PushTransform, from the root scale): a
	// filter it draws lies inside no skipped block, so every transform
	// around it was applied, in this order, with GridMul.
	k, layers := 0, 0
	ctm, skip := gfx.Scale(c.scale, c.scale), false
	open := c.open[:0]
	defer func() { c.open = open[:0] }()
	for i := 0; i < dl.Len() && k < n; i++ {
		if i == c.idx[k] {
			c.top[k] = c.top[k] && layers == 0
			c.ctms[k] = gfx.Affine{}
			if !skip {
				c.ctms[k] = ctm
			}
			k++
			continue
		}
		switch t := dl.At(i).(type) {
		case gfx.PushLayer:
			open = append(open, idxState{layer: true, ctm: ctm, skip: skip})
			layers++
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath:
			open = append(open, idxState{ctm: ctm, skip: skip})
		case gfx.PushTransform:
			open = append(open, idxState{ctm: ctm, skip: skip})
			if !skip {
				if next := GridMul(ctm, t.Affine); next.IsFinite() && next.Det() != 0 {
					ctm = next
				} else {
					skip = true
				}
			}
		case gfx.PopLayer, gfx.PopClip, gfx.PopTransform:
			if m := len(open); m > 0 {
				if open[m-1].layer {
					layers--
				}
				ctm, skip = open[m-1].ctm, open[m-1].skip
				open = open[:m-1]
			}
		}
	}
}

// prepare makes the cache the cache of a Render of dl at scale into a
// destination of bounds full, emptying it when it holds another list,
// scale or destination.
func (c *BackdropCache) prepare(dl *gfx.DisplayList, scale float32, full image.Rectangle) {
	if c.dl != dl || c.scale != scale || (c.full != full && !c.full.Empty()) {
		c.bind(dl, scale)
	}
	c.full = full
}

// entry returns the entry of the backdrop filter at command index i of
// the cache's list, and its index among the list's backdrop filters, or
// nil when the filter is not cached.
func (c *BackdropCache) entry(i int) (*bdEntry, int) {
	k, found := slices.BinarySearch(c.idx, i)
	if !found || !c.top[k] {
		return nil, -1
	}
	return &c.ents[k], k
}

// Region is BackdropRegion for a render of region with the cache: the
// filters whose output inside the grown region is in the cache do not grow
// it, and those whose input did not change since the previous frame but
// whose cached output does not cover it grow it by all they read, so that
// their whole output is computed once and cached. A nil cache, or one of
// another list or scale, is BackdropRegion.
func (c *BackdropCache) Region(dl *gfx.DisplayList, scale float32, region, bounds image.Rectangle) image.Rectangle {
	if scale == 0 {
		scale = 1
	}
	if c == nil || c.dl != dl || c.scale != scale || (c.full != bounds && !c.full.Empty()) {
		return BackdropRegion(dl, scale, region, bounds)
	}
	return backdropRegion(dl, scale, region, bounds, c, false)
}

// hit reports whether the k-th backdrop filter of the cache's list, at
// command index i, finds its output in the device pixels need in the
// cache, for a render into a destination of bounds full. It checks what
// lookup checks when the filter is drawn (the transform and the device
// bounds come from index), so that a filter found here is drawn from the
// cache (see BackdropCache.plan).
func (c *BackdropCache) hit(k, i int, need, full image.Rectangle) bool {
	e := &c.ents[k]
	if !c.top[k] || !e.ok || e.bounds != c.dl.BoundsAt(i) || !need.In(e.cover) {
		return false
	}
	f, ok := c.dl.At(i).(gfx.BackdropFilter)
	if !ok {
		return false
	}
	m := c.ctms[k]
	return m != (gfx.Affine{}) && e.matches(f, e.bounds, m, shapeBounds(&m, f.Shape.Rect).Intersect(full))
}

// need returns the device pixels of out, the output a render draws of the
// k-th backdrop filter (-1: not cached), that must be exact: those
// BackdropRegion found in the cache when it did not grow the region for
// the filter (the pixels it reads were not rendered), all of out
// otherwise.
func (c *BackdropCache) need(k int, out image.Rectangle) image.Rectangle {
	if k >= 0 && k < len(c.plan) && !c.plan[k].Empty() {
		return out.Intersect(c.plan[k])
	}
	return out
}

// keep returns the device pixels of in, the device bounds of the shape of
// the k-th backdrop filter (-1: not cached), whose output the render
// computes from exact pixels (exact), which the cache keeps: whatever the
// clip the filter is drawn through, so that a later render that draws
// more of it through another clip finds them.
func (c *BackdropCache) keep(k int, in image.Rectangle) image.Rectangle {
	if k < 0 || k >= len(c.exact) {
		return image.Rectangle{}
	}
	return in.Intersect(devRootRect(c.dl.BoundsAt(c.idx[k]), c.scale)).Intersect(c.exact[k])
}

// matches reports whether e holds the output of the filter c of
// root-space bounds b drawn under ctm over the device bounds in (in the
// destination).
func (e *bdEntry) matches(c gfx.BackdropFilter, b gfx.Rect, ctm gfx.Affine, in image.Rectangle) bool {
	return e != nil && e.ok && e.bounds == b && e.ctm == ctm && e.in == in && sameFilters(e.filters, c)
}

// lookup returns the cached output of the filter c of root-space bounds b
// (the filter of entry e, which may be nil) drawn under ctm over the
// device bounds in (in the destination) when it holds the pixels need: the
// buffer and the rectangle it covers.
func (e *bdEntry) lookup(c gfx.BackdropFilter, b gfx.Rect, ctm gfx.Affine, in, need image.Rectangle) ([]uint8, image.Rectangle, bool) {
	if !e.matches(c, b, ctm, in) || !need.In(e.out) {
		return nil, image.Rectangle{}, false
	}
	return e.pix, e.out, true
}

// sameFilters reports whether two backdrop filters have equal filters.
func sameFilters(a, b gfx.BackdropFilter) bool {
	return gfx.Equal(gfx.BackdropFilter{Filters: a.Filters}, gfx.BackdropFilter{Filters: b.Filters})
}

// store makes the output of the k-th backdrop filter f, drawn under ctm
// over the device bounds in, the content of entry e: the pixels keep (see
// keep), which buf over win holds. With keep empty (the render computed
// none of its output from exact pixels), an entry for the same filter is
// left alone: its pixels are still its output. It keeps nothing when the
// cache would exceed its budget.
func (c *BackdropCache) store(e *bdEntry, k int, f gfx.BackdropFilter, ctm gfx.Affine, in, keep image.Rectangle, buf []uint8, win image.Rectangle) {
	b := c.dl.BoundsAt(c.idx[k])
	if keep.Empty() {
		if !e.matches(f, b, ctm, in) {
			c.drop(e)
		}
		return
	}
	cover := devRootRect(b, c.scale).Intersect(c.exact[k])
	out := keep
	n := 4 * out.Dx() * out.Dy()
	old := e.pix
	*e = bdEntry{}
	var pix []uint8
	switch {
	case cap(old) >= n:
		pix, old = old[:n], nil
	default:
		if old != nil {
			c.give(old)
			old = nil
		}
		if pix = c.take(n); pix == nil {
			budget := 2 * 4 * c.full.Dx() * c.full.Dy()
			if c.bytes+n > budget {
				c.trim(budget - n)
			}
			if c.bytes+n <= budget {
				pix = make([]uint8, n)
				c.bytes += cap(pix)
			}
		}
	}
	if old != nil {
		c.give(old)
	}
	if pix == nil {
		return
	}
	w := 4 * out.Dx()
	ww := 4 * win.Dx()
	for y := out.Min.Y; y < out.Max.Y; y++ {
		src := buf[(y-win.Min.Y)*ww+4*(out.Min.X-win.Min.X):]
		copy(pix[(y-out.Min.Y)*w:(y-out.Min.Y+1)*w], src[:w])
	}
	*e = bdEntry{ok: true, ctm: ctm, in: in, filters: gfx.BackdropFilter{Filters: f.Filters}, bounds: b, cover: cover, out: out, pix: pix}
}

// drop empties entry e, keeping its buffer as a spare.
func (c *BackdropCache) drop(e *bdEntry) {
	if e.pix != nil {
		c.give(e.pix)
	}
	*e = bdEntry{}
}

// give keeps a buffer of the cache as a spare (whose bytes stay counted),
// or frees it.
func (c *BackdropCache) give(b []uint8) {
	c.spare = append(c.spare, b)
	if len(c.spare) > maxSpare {
		// Free the smallest.
		m := 0
		for k := range c.spare {
			if cap(c.spare[k]) < cap(c.spare[m]) {
				m = k
			}
		}
		c.bytes -= cap(c.spare[m])
		c.spare = slices.Delete(c.spare, m, m+1)
	}
}

// take returns a spare buffer of length n, or nil.
func (c *BackdropCache) take(n int) []uint8 {
	best := -1
	for k, b := range c.spare {
		if cap(b) >= n && (best < 0 || cap(b) < cap(c.spare[best])) {
			best = k
		}
	}
	if best < 0 {
		return nil
	}
	b := c.spare[best][:n]
	c.spare = slices.Delete(c.spare, best, best+1)
	return b
}

// trim frees spare buffers until the cache holds at most limit bytes.
func (c *BackdropCache) trim(limit int) {
	for len(c.spare) > 0 && c.bytes > limit {
		last := len(c.spare) - 1
		c.bytes -= cap(c.spare[last])
		c.spare[last] = nil
		c.spare = c.spare[:last]
	}
}
