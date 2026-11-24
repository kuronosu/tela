package gpu

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Occlusion.
//
// A command that paints opaque pixels with source-over sets them whatever
// lies under them. What a frame's list paints before such a cover, inside
// the pixels the cover paints fully, is not seen, and unless something
// reads it in between (a backdrop filter that is computed, a layer with a
// blend mode, which read the surface they are painted on), it changes no
// pixel of the frame: it is not encoded. The root's background under a
// wallpaper is one case. The one that pays is a glass pane whose filtered
// backdrop comes from the cache (backdropcache.go): the backdrop replaces
// the pixels under the pane and reads none of them, so a render of a
// region inside the pane starts at the pane.
//
// Before a frame's list is encoded it is walked once, with the transforms
// and the clips of the encoding, for its covers: a FillRect or FillRRect of
// an opaque color, a DrawImage of an opaque image, and a BackdropFilter at
// full opacity drawn from an opaque cache entry; drawn through pixel
// aligned clips only, outside every layer, each over the pixels its shape
// covers entirely. And for its readers, each with the pixels it reads.
//
// A command is not encoded when a cover that follows it holds the pixels
// it can touch in this render (its bounds inside the scissor) and no
// reader between the two reads them. Else its instances are cut around the
// covers that hide a part of them: the wallpaper of a page of glass panes
// is the strips between the panes. The parts are those of the list, the
// same in every render (the covers and the readers do not depend on the
// region), and a render draws the parts its scissor shows.
//
// What is hidden depends on the region rendered; the pixels do not: where
// a cover is opaque, the device's source-over leaves the cover's own
// value, whatever was under it.

// minCover is the fewest pixels a cover must paint fully to be considered.
const minCover = 64 * 64

// maxCovers bounds the covers of a list.
const maxCovers = 64

// minCut is the fewest pixels a part of an instance must hold to be cut
// around the covers it lies under in part: a smaller one is drawn whole,
// or not at all.
const minCut = 48 * 48

// maxCuts bounds the parts an instance is cut into around covers.
const maxCuts = 24

// cover is an opaque command of the frame's list: its index and device
// pixels it paints fully.
type cover struct {
	idx  int
	rect image.Rectangle
}

// reader is a command of the frame's list that reads the surface it is
// painted on: its index and the device pixels it reads.
type reader struct {
	idx  int
	rect image.Rectangle
}

// occluderKey is what the covers and the readers of a list depend on.
type occluderKey struct {
	dl    *gfx.DisplayList
	size  image.Point
	scale float32
	bdc   *BackdropCache
	epoch uint64
	res   *resources
}

// findCovers walks the frame's list for its covers and its readers, as
// visit walks it for its instances, but whole and without the region: they
// are the list's, whatever is rendered of it, so that the parts an instance
// is cut into around them are the same in every render. The walk is done
// once for the renders of a frame (the list, the scale, the target's size
// and what the cache of filtered backdrops holds make its result).
func (e *Encoder) findCovers(dl *gfx.DisplayList, scale float32) {
	key := occluderKey{dl: dl, size: e.target.Size(), scale: scale, bdc: e.bdc, res: e.res}
	if e.bdc != nil {
		key.epoch = e.bdc.epoch
	}
	if key == e.coverKey {
		// The same list as the render before: its images are still in use.
		for _, id := range e.listImages {
			if im := e.res.images[id]; im != nil {
				im.used = e.res.frame
			}
		}
		return
	}
	e.listImages = e.listImages[:0]
	e.coverKey = key
	e.covers, e.readers = e.covers[:0], e.readers[:0]
	e.scan = true
	e.stack = append(e.stack[:0], state{ctm: e.root, scissor: e.target, bound: e.target, clip: -1, mask: -1, surf: e.target, layer: -1})
	for i := range dl.Len() {
		e.scanVisit(i, dl.At(i))
	}
	e.scan = false
	// The clips of the walk are pushed again by the encoding.
	e.stack = e.stack[:0]
	e.clips = e.clips[:0]
	clear(e.masks)
	e.masks = e.masks[:0]
}

// scanVisit is visit for findCovers: the same state, and covers and
// readers in the place of instances.
func (e *Encoder) scanVisit(i int, c gfx.Command) {
	switch c := c.(type) {
	case gfx.FillRect:
		e.scanFill(i, gfx.RRect{Rect: c.Rect}, c.Paint)
	case gfx.FillRRect:
		e.scanFill(i, c.RRect, c.Paint)
	case gfx.DrawImage:
		img := c.Image
		if img == nil {
			return
		}
		// An image of the frame's list is in use, drawn or not: hidden under
		// a pane, or outside the regions of the frames that follow, it
		// keeps its texture for the frame that shows it again.
		if im := e.res.images[img.ID()]; im != nil {
			im.used = e.res.frame
		}
		e.listImages = append(e.listImages, img.ID())
		if !c.Src.Empty() && c.Src == c.Src.Intersect(img.Bounds()) && !c.Dst.Empty() && e.coverable() && e.res.opaqueImage(img) {
			e.addCovers(i, gfx.RRect{Rect: c.Dst}, image.Rectangle{})
		}
	case gfx.PushTransform:
		st := e.push()
		if !st.skip {
			next := raster.GridMul(st.ctm, c.Affine)
			if next.IsFinite() && next.Det() != 0 {
				st.ctm = next
			} else {
				st.skip = true
			}
		}
	case gfx.PushClipRect:
		if st := e.push(); !st.skip {
			e.clipRRect(st, gfx.RRect{Rect: c.Rect})
		}
	case gfx.PushClipRRect:
		if st := e.push(); !st.skip {
			e.clipRRect(st, c.RRect)
		}
	case gfx.PushClipPath:
		if st := e.push(); !st.skip {
			e.clipPath(st, c.Path, c.Rule)
		}
	case gfx.PushLayer:
		// A layer is drawn apart: nothing in it covers or reads the
		// surface, but a blend mode reads what is under it when it is
		// composited.
		st := e.push()
		if !st.skip && c.Blend != gfx.SrcOver {
			if r := e.devBounds(e.dl.BoundsAt(i)).Intersect(st.bound); !r.Empty() {
				e.readers = append(e.readers, reader{idx: e.dl.Match(i), rect: r})
			}
		}
		st.skip = true
	case gfx.BackdropFilter:
		st := e.top()
		if st.skip {
			return
		}
		if e.bdc != nil {
			if en, k := e.bdc.entry(i); en != nil && e.bdc.hit(k) {
				// Drawn from the cache: it reads nothing, and where it is
				// opaque it covers.
				if en.opaque && raster.Unit8(float64(c.Opacity)) == 255 && e.coverable() {
					e.addCovers(i, c.Shape, en.in)
				}
				return
			}
		}
		// Computed: it reads the device bounds of its shape.
		r := c.Shape.Rect
		if in := touchBounds(&st.ctm, [4]float64{float64(r.X0), float64(r.Y0), float64(r.X1), float64(r.Y1)}).Intersect(e.target); !in.Empty() {
			e.readers = append(e.readers, reader{idx: i, rect: in})
		}
	case gfx.PopLayer, gfx.PopTransform, gfx.PopClip:
		e.stack = e.stack[:len(e.stack)-1]
	}
}

// coverable reports whether what is drawn in the current state of the
// walk can cover: through pixel aligned clips only, under a transform that
// keeps rectangles axis-aligned.
func (e *Encoder) coverable() bool {
	st := e.top()
	return !st.skip && st.clip < 0 && st.mask < 0 && st.ctm.IsAxisAligned() && len(e.covers) < maxCovers
}

// scanFill notes the cover of a rectangle or rounded rectangle filled with
// an opaque color.
func (e *Encoder) scanFill(i int, rr gfx.RRect, p gfx.Paint) {
	if col, ok := p.(gfx.Color); ok && e.coverable() && premul8(col)[3] == 255 {
		e.addCovers(i, rr, image.Rectangle{})
	}
}

// addCovers adds the covers of the shape rr drawn by command i in the
// current state of the walk, which is coverable: the pixels it covers
// entirely, inside within when that is not empty.
func (e *Encoder) addCovers(i int, rr gfx.RRect, within image.Rectangle) {
	st := e.top()
	sh, _, ok := deviceShape(&st.ctm, rr)
	if !ok {
		return
	}
	f1, f2 := deviceFull(&sh)
	for n, r := range [2]image.Rectangle{f1, f2} {
		if n == 1 && f2 == f1 {
			break // square corners: one rectangle
		}
		r = r.Intersect(st.bound)
		if !within.Empty() {
			r = r.Intersect(within)
		}
		if r.Dx()*r.Dy() >= minCover {
			e.covers = append(e.covers, cover{idx: i, rect: r})
		}
	}
}

// read reports whether a command between the commands i and j of the
// frame's list reads pixels of r.
func (e *Encoder) read(i, j int, r image.Rectangle) bool {
	for k := range e.readers {
		if rd := &e.readers[k]; i < rd.idx && rd.idx < j && rd.rect.Overlaps(r) {
			return true
		}
	}
	return false
}

// culls reports whether what the command being encoded draws can be
// hidden by the covers: it is a command of the frame's list painted on the
// frame's surface.
func (e *Encoder) culls() bool {
	return len(e.covers) != 0 && e.dl == e.frameDL && len(e.layers) == 0
}

// hidden reports whether command i of the list being encoded shows
// nothing in this render: a cover that follows it, with nothing between
// the two that reads what it paints there, covers the pixels it can touch.
// It also notes whether a cover hides a part of it (Encoder.cut).
func (e *Encoder) hidden(i int) bool {
	e.cuts = false
	if !e.culls() {
		return false
	}
	st := e.top()
	all := e.devBounds(e.dl.BoundsAt(i)).Intersect(st.bound)
	vis := all.Intersect(st.scissor)
	for k := range e.covers {
		c := &e.covers[k]
		if c.idx <= i {
			continue
		}
		if vis.In(c.rect) && !e.read(i, c.idx, vis) {
			e.culled++
			return true
		}
		// Whether its instances are cut does not depend on the region.
		e.cuts = e.cuts || all.Overlaps(c.rect)
	}
	return false
}

// cut appends to out the parts of r, pixels of an instance of command i of
// the frame's list, that no cover hides: r without the pixels of the
// covers that follow the command, where nothing in between reads them. A
// small r is whole or nothing; a large one is cut into rectangles around
// the covers, at most maxCuts. The covers are the list's and the parts do
// not depend on the region, so an instance has the same parts in every
// render.
func (e *Encoder) cut(i int, r image.Rectangle, out []image.Rectangle) []image.Rectangle {
	first := len(out)
	out = append(out, r)
	small := r.Dx()*r.Dy() < minCut
	for k := range e.covers {
		c := &e.covers[k]
		if c.idx <= i || !r.Overlaps(c.rect) {
			continue
		}
		for j := first; j < len(out); j++ {
			p := out[j]
			x := p.Intersect(c.rect)
			if x.Empty() || (small && x != p) || e.read(i, c.idx, x) {
				continue
			}
			// p without x: above and below over its whole width, left and
			// right beside x.
			parts := [4]image.Rectangle{
				rectNoSwap(p.Min.X, p.Min.Y, p.Max.X, x.Min.Y),
				rectNoSwap(p.Min.X, x.Min.Y, x.Min.X, x.Max.Y),
				rectNoSwap(x.Max.X, x.Min.Y, p.Max.X, x.Max.Y),
				rectNoSwap(p.Min.X, x.Max.Y, p.Max.X, p.Max.Y),
			}
			n := 0
			for _, q := range parts {
				if !q.Empty() {
					n++
				}
			}
			if len(out)-first-1+n > maxCuts {
				return out // enough rectangles: the rest is drawn under its covers
			}
			out[j] = out[len(out)-1]
			out = out[:len(out)-1]
			for _, q := range parts {
				if !q.Empty() {
					out = append(out, q)
				}
			}
			j-- // the part moved here, or the first new one
		}
	}
	return out
}

// opaqueImage reports whether every pixel of img is opaque, remembered by
// the image's ID.
func (r *resources) opaqueImage(img *gfx.Image) bool {
	if o, known := img.KnownOpaque(); known {
		return o
	}
	id := img.ID()
	if o, ok := r.opaque[id]; ok {
		return o
	}
	if r.opaque == nil || len(r.opaque) > 256 {
		r.opaque = map[uint64]bool{}
	}
	o := true
	pix := img.Pix()
	for k := 3; k < len(pix); k += 4 {
		if pix[k] != 255 {
			o = false
			break
		}
	}
	r.opaque[id] = o
	return o
}
