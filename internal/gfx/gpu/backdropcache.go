package gpu

import (
	"image"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Cached filtered backdrops: the GPU's counterpart of raster.BackdropCache
// (ADR-200), for the same reason. A backdrop filter that spreads pixels
// (the blur of a glass pane) computes each output pixel from the pixels
// within its reach, so a render of a region must first draw the region
// grown by that reach, from the start of the list, and then filter it:
// a change of a pixel inside a pane costs the pane's backdrop around it.
// When what lies behind the pane did not change, the output is the pixels
// it was before.
//
// A BackdropCache keeps the output of each backdrop filter of a target's
// frames, whole (over the device bounds of its shape inside the target),
// in an offscreen target of the Encoder's that leaves the pool for as long
// (poolTarget.kept). The owner of the target tells it what changed, from
// the damage of two frames (Retain, as core's damage tells the
// rasterizer's cache):
//
//   - a filter whose input is the same pixels keeps its entry, and is
//     drawn from it: no filter runs, the region does not grow for it, and
//     where its output is opaque and covers what a render draws, nothing
//     painted before it is drawn at all (cull.go);
//   - a filter whose input changed inside the frame's damage keeps its
//     entry too: its output changed only within its reach of that damage,
//     which is damage as well, so every render of the frame computes the
//     output it draws, as without a cache, and writes it into the entry,
//     which is whole again when the frame is drawn;
//   - any other filter has no entry. One is made when a render computes
//     the whole output anyway (a full frame, a pane that appears), and
//     when the input of a filter without one did not change for a frame,
//     by computing all of it once (the rasterizer's rule).
//
// The output of a filter is a function of the picture under it and of
// device coordinates only, whatever the targets its steps run in and the
// region rendered (filter.go), so the pixels drawn from an entry are those
// a render without a cache computes, bit for bit. The caller vouches for
// the input; the rest of what makes the output is checked here: the list,
// the scale and the size of the target, the transform the filter is drawn
// under, the device bounds of its shape, its filters, and the resources
// the entry's target belongs to (an Encoder that started over holds none
// of them).
//
// Only the filters that spread pixels and lie outside every layer are
// cached: they read the target itself.

// BackdropCache keeps the filtered backdrops of the backdrop filters of
// the frames of one target between Renders (Options.Backdrops). The zero
// value is an empty cache, ready to use; a nil *BackdropCache caches
// nothing. It belongs to one Encoder at a time, and is not safe for
// concurrent use.
//
// Its memory is bounded: its entries hold at most two targets' worth of
// pixels, and a filter whose entry would exceed that is computed as
// without a cache.
type BackdropCache struct {
	dl    *gfx.DisplayList // the list of the entries; nil: none
	scale float32
	size  image.Point // of the target
	// res is the Encoder's resources the entries' targets belong to.
	res  *resources
	ents []glassEntry // one per backdrop filter of dl, in list order
	old  []glassEntry // Retain's scratch
	idx  []int        // the command index of each backdrop filter of dl
	// top: the filter is cached (it spreads pixels, outside every layer);
	// ctms and ins: the transform it is drawn under and the device bounds
	// of its shape inside the target (a zero transform: never drawn).
	top  []bool
	ctms []gfx.Affine
	ins  []image.Rectangle
	open []glassState // index's stack
	// epoch counts the changes of what the entries hold: what an Encoder
	// derived from it (the covers of a list, cull.go) is as old.
	epoch uint64

	stats BackdropStats
}

// BackdropStats counts what the Renders of a BackdropCache did with the
// backdrop filters it caches, since it was made.
type BackdropStats struct {
	// Hits counts the filters drawn from their entry without computing
	// anything, Patched those computed over what a render drew of them and
	// written into their entry, Stored the entries made, and Computed the
	// filters computed without an entry to show for it.
	Hits, Patched, Stored, Computed int
}

// The states of an entry for the frame being drawn.
const (
	// glassNone: the entry holds nothing that can be drawn.
	glassNone = iota
	// glassValid: the entry holds the filter's output.
	glassValid
	// glassPatch: the entry holds the filter's output except inside the
	// damage of the frame being drawn, where every render writes it.
	glassPatch
)

// glassEntry is the cached output of one backdrop filter.
type glassEntry struct {
	state uint8
	// stable: the filter's input did not change since the previous frame
	// (Retain), so filling all of its output pays off.
	stable bool
	// opaque: every pixel of the output has an alpha of 255.
	opaque bool
	// tried: a render computed the filter for an entry and could not make
	// one (the filter is drawn through a clip that cuts it, or the entry
	// does not fit the budget): with the same input it is not tried again.
	tried bool
	// t holds the output over in, its first pixel being in's; nil: none.
	t       *poolTarget
	ctm     gfx.Affine
	in      image.Rectangle
	filters []gfx.Filter
	bounds  gfx.Rect
}

// glassState is what index keeps per open block.
type glassState struct {
	layer bool
	ctm   gfx.Affine
	skip  bool
}

// drop gives the entry's target back to the pool and empties the entry.
func (en *glassEntry) drop() {
	if en.t != nil {
		en.t.kept = false
	}
	*en = glassEntry{}
}

// Reset empties the cache. A nil cache is ignored.
func (c *BackdropCache) Reset() {
	if c != nil {
		c.bind(nil, 0)
	}
}

// bind empties the cache and makes it the cache of dl at scale.
func (c *BackdropCache) bind(dl *gfx.DisplayList, scale float32) {
	c.epoch++
	for k := range c.ents {
		c.ents[k].drop()
	}
	c.ents = c.ents[:0]
	c.scale = scale
	c.index(dl)
	if dl != nil {
		c.ents = slices.Grow(c.ents, len(c.idx))[:len(c.idx)]
		clear(c.ents)
	}
}

// index sets dl and computes idx, top, ctms and ins for it at c.scale in a
// target of c.size.
func (c *BackdropCache) index(dl *gfx.DisplayList) {
	c.dl = dl
	c.idx, c.top, c.ctms, c.ins = c.idx[:0], c.top[:0], c.ctms[:0], c.ins[:0]
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
	c.ins = slices.Grow(c.ins, n)[:n]
	// The transform is the one the Encoder computes (visit's
	// PushTransform, from the root scale): a filter it draws lies inside no
	// skipped block, so every transform around it was applied, in this
	// order, with GridMul.
	target := image.Rectangle{Max: c.size}
	k, layers := 0, 0
	ctm, skip := gfx.Scale(c.scale, c.scale), false
	open := c.open[:0]
	defer func() { c.open = open[:0] }()
	for i := 0; i < dl.Len() && k < n; i++ {
		if i == c.idx[k] {
			c.top[k] = c.top[k] && layers == 0
			c.ctms[k], c.ins[k] = gfx.Affine{}, image.Rectangle{}
			if f, ok := dl.At(i).(gfx.BackdropFilter); ok && !skip {
				r := f.Shape.Rect
				c.ctms[k] = ctm
				c.ins[k] = touchBounds(&ctm, [4]float64{float64(r.X0), float64(r.Y0), float64(r.X1), float64(r.Y1)}).Intersect(target)
			}
			k++
			continue
		}
		switch t := dl.At(i).(type) {
		case gfx.PushLayer:
			open = append(open, glassState{layer: true, ctm: ctm, skip: skip})
			layers++
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath:
			open = append(open, glassState{ctm: ctm, skip: skip})
		case gfx.PushTransform:
			open = append(open, glassState{ctm: ctm, skip: skip})
			if !skip {
				if next := raster.GridMul(ctm, t.Affine); next.IsFinite() && next.Det() != 0 {
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

// Retain moves the cache to dl, the next frame's list rendered at scale
// into the same target. prev and patch have one element per backdrop
// filter of dl (in list order): the index, among the backdrop filters of
// the list the cache holds, of the same filter (an equal command with
// equal bounds, drawn the same way) when
//
//   - prev: its input is the same pixels (the same pixels painted under
//     its shape before it), or -1;
//   - patch: its input changed, and its output with it, only inside the
//     pixels the frame's renders draw (the frame's damage, which holds the
//     filter's output within its reach of every change under it), or -1.
//
// Their entries are kept; the others are dropped. A nil prev, or one of
// another length, empties the cache; a nil patch names no filter. A nil
// cache is ignored.
//
// The caller vouches for the input: a filter carried whose input changed
// draws its old output. With patch it also vouches for the renders: every
// rectangle of the frame's damage is rendered, with this cache, before the
// frame after it is retained (or the cache is Reset).
func (c *BackdropCache) Retain(dl *gfx.DisplayList, scale float32, prev, patch []int) {
	if c == nil {
		return
	}
	if scale == 0 {
		scale = 1
	}
	if dl == nil || c.dl == nil || prev == nil || len(prev) != dl.NumBackdrops() || scale != c.scale || (patch != nil && len(patch) != len(prev)) {
		c.bind(dl, scale)
		return
	}
	c.epoch++
	c.old, c.ents = c.ents, c.old[:0]
	c.index(dl)
	c.ents = slices.Grow(c.ents[:0], len(prev))[:len(prev)]
	for k, p := range prev {
		c.ents[k] = glassEntry{}
		q := -1
		if patch != nil {
			q = patch[k]
		}
		switch {
		case !c.top[k]:
		case p >= 0 && p < len(c.old):
			// The same input: the same output, if there is one.
			c.ents[k] = c.old[p]
			c.old[p] = glassEntry{}
			c.ents[k].stable = true
			if c.ents[k].state == glassPatch {
				c.ents[k].state = glassValid // the frame before wrote it
			}
		case q >= 0 && q < len(c.old) && c.old[q].t != nil && c.old[q].state != glassNone:
			c.ents[k] = c.old[q]
			c.old[q] = glassEntry{}
			c.ents[k].state, c.ents[k].stable = glassPatch, false
		}
	}
	for k := range c.old {
		c.old[k].drop()
	}
	c.old = c.old[:0]
}

// prepare makes the cache the cache of a Render of dl at scale into a
// target of size with the resources res, emptying it when it holds another
// list, scale or size, or targets of other resources.
func (c *BackdropCache) prepare(res *resources, dl *gfx.DisplayList, scale float32, size image.Point) {
	if c.res != res {
		// The targets of the entries are not this Encoder's (it started
		// over): they are gone.
		for k := range c.ents {
			c.ents[k] = glassEntry{stable: c.ents[k].stable}
		}
		c.old = c.old[:0]
		c.epoch++
		c.res = res
	}
	switch {
	case c.dl != dl || c.scale != scale:
		c.size = size
		c.bind(dl, scale)
	case c.size != size:
		c.size = size
		c.bind(dl, scale)
	case len(c.ctms) != len(c.idx) || len(c.ents) != len(c.idx):
		c.bind(dl, scale)
	}
}

// entry returns the entry of the backdrop filter at command index i of the
// cache's list and its index among the list's backdrop filters, or nil
// when the filter is not cached.
func (c *BackdropCache) entry(i int) (*glassEntry, int) {
	k, found := slices.BinarySearch(c.idx, i)
	if !found || !c.top[k] || c.ctms[k] == (gfx.Affine{}) {
		return nil, -1
	}
	return &c.ents[k], k
}

// holds reports whether the entry of the k-th backdrop filter holds pixels
// of that filter as the list draws it: the transform, the device bounds,
// the filters and the list bounds are the entry's.
func (c *BackdropCache) holds(k int) bool {
	en := &c.ents[k]
	if !c.top[k] || en.t == nil || en.state == glassNone || c.ctms[k] == (gfx.Affine{}) {
		return false
	}
	i := c.idx[k]
	f, ok := c.dl.At(i).(gfx.BackdropFilter)
	return ok && en.ctm == c.ctms[k] && en.in == c.ins[k] && en.bounds == c.dl.BoundsAt(i) &&
		gfx.Equal(gfx.BackdropFilter{Filters: en.filters}, gfx.BackdropFilter{Filters: f.Filters})
}

// hit reports whether the k-th backdrop filter is drawn from its entry
// without computing anything.
func (c *BackdropCache) hit(k int) bool {
	return c.ents[k].state == glassValid && c.holds(k)
}

// whole reports whether the k-th backdrop filter, which is not a hit,
// computes all of its output for an entry: its input did not change since
// the previous frame, and it has none.
func (c *BackdropCache) whole(k int) bool {
	return c.top[k] && c.ents[k].stable && !c.ents[k].tried && !c.holds(k) && c.ctms[k] != (gfx.Affine{}) && c.fits(k)
}

// bytes returns the memory of the entries' targets.
func (c *BackdropCache) bytes() int64 {
	var n int64
	for k := range c.ents {
		if t := c.ents[k].t; t != nil {
			n += t.bytes()
		}
	}
	return n
}

// fits reports whether an entry for the k-th filter fits the budget: two
// targets' worth of pixels for all of them.
func (c *BackdropCache) fits(k int) bool {
	in := c.ins[k]
	// The pixels the entries hold count, not their targets' (which the
	// pool rounds up).
	var have int64
	for j := range c.ents {
		if en := &c.ents[j]; en.t != nil && j != k {
			have += int64(en.in.Dx()) * int64(en.in.Dy())
		}
	}
	return !in.Empty() && have+int64(in.Dx())*int64(in.Dy()) <= 2*int64(c.size.X)*int64(c.size.Y)
}

// Bytes returns the bytes of pixels the cache's entries hold on the
// device, for tests and memory measurements. A nil cache holds none.
func (c *BackdropCache) Bytes() int64 {
	if c == nil {
		return 0
	}
	return c.bytes()
}

// Stats returns what the Renders did with the filters the cache caches. A
// nil cache has none.
func (c *BackdropCache) Stats() BackdropStats {
	if c == nil {
		return BackdropStats{}
	}
	return c.stats
}

// Region returns the device pixels a render of region with the cache
// covers (the Encoder renders them apart and copies the region): region
// grown by what the backdrop filters that are computed read, as
// raster.BackdropRegion grows it for all of them. The filters drawn from
// the cache do not grow it, and one that computes all of its output for
// the cache grows it by all it reads. A nil cache, or one of another list
// or scale, is raster.BackdropRegion.
func (c *BackdropCache) Region(dl *gfx.DisplayList, scale float32, region, bounds image.Rectangle) image.Rectangle {
	if scale == 0 {
		scale = 1
	}
	if c != nil && (c.dl != dl || c.scale != scale || c.size != bounds.Size() || len(c.ents) != dl.NumBackdrops()) {
		c = nil
	}
	grown, _ := planBackdrops(dl, scale, region, bounds, c, nil)
	return grown
}

// planBackdrops returns the pixels a render of region covers, inside
// bounds, the target's pixels (see BackdropCache.Region), and in needs,
// for each backdrop filter of dl in list order, the pixels of its output
// that the render needs: the region, grown by what the filters after it
// read (the rasterizer computes each over all of the grown region; the
// others are pixels nothing exact reads). ok is false when the outputs
// cannot be told apart (a filter inside a layer with filters): the render
// covers bounds, and every filter computes what it draws. c is the cache
// of dl at scale, or nil.
func planBackdrops(dl *gfx.DisplayList, scale float32, region, bounds image.Rectangle, c *BackdropCache, needs []image.Rectangle) (grown image.Rectangle, out []image.Rectangle) {
	region = region.Intersect(bounds)
	if dl == nil || dl.BackdropReach() <= 0 || region.Empty() {
		return region, nil
	}
	n := dl.NumBackdrops()
	needs = slices.Grow(needs[:0], n)[:n]
	reg := region
	for k := n - 1; k >= 0; k-- {
		needs[k] = reg
		_, _, reach, inFilter := dl.Backdrop(k)
		if !(reach > 0) {
			continue
		}
		need := reg
		if c != nil && c.top[k] && !raster.BackdropReadRegion(dl, scale, k, reg).Empty() {
			if c.hit(k) {
				continue // drawn from the cache: it reads nothing
			}
			if c.whole(k) {
				need = bounds
				needs[k] = bounds
			}
		}
		rd := raster.BackdropReadRegion(dl, scale, k, need).Intersect(bounds)
		if rd.Empty() {
			continue
		}
		if inFilter {
			return bounds, nil
		}
		reg = reg.Union(rd)
	}
	return reg, needs
}
