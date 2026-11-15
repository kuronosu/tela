package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Clips.
//
// A clip narrows the scissor to the pixels its shape can touch. A rectangle
// whose device edges fall on pixel boundaries is nothing more. Any other
// shape also multiplies the coverage of what is drawn inside it, in one of
// two ways:
//
//   - the first rectangle or rounded rectangle that matters is carried by
//     the instances (SlotClip), and the shader computes its coverage as it
//     does for a fill;
//   - every other clip (a path, or a second shape while the first still
//     matters) goes into a clip mask: 8-bit coverage rasterized on the CPU
//     by the rasterizer's own code and kept in the coverage atlas, which
//     the instances read (FlagMask). A mask inside a mask is the product
//     of the two, as the rasterizer's nested masks are, so any stack of
//     clips is one shape in the instance and one mask.
//
// A clip stops mattering when the pixels the clips leave lie where it
// covers entirely (the inside of a rectangle, the middle band of a rounded
// one): what is drawn there does not carry it.
//
// A mask is cut into tiles on a grid fixed to the device pixels, each
// rasterized when an instance first draws through it and found again, by
// the clips that make it, in the frames that follow.

// maskNode is a clip mask: a shape, times the mask it is nested in.
type maskNode struct {
	id     uint64
	parent int // in Encoder.masks, or -1
	path   gfx.Path
	rule   gfx.FillRule
	isPath bool
	rr     gfx.RRect
	ctm    gfx.Affine
	// rect holds the pixels where the mask may be other than 0: the bounds
	// of the clips at the push, inside the target. full are pixels where it
	// is 255.
	rect, full image.Rectangle
}

// maskDef is what makes the pixels of a mask: equal values mean identical
// pixels.
type maskDef struct {
	parent uint64 // the mask it is nested in, or 0
	path   uint64 // the path's number, or 0 for a rounded rectangle
	rule   gfx.FillRule
	rr     gfx.RRect
	ctm    gfx.Affine
	rect   image.Rectangle
	tol    float32
}

type maskID struct {
	id   uint64
	used uint64
	idle uint16
}

// narrow intersects the clip of st, a state just pushed, with the pixels a
// clip shape can touch, and drops the clips that no longer matter in what
// is left. It reports false when nothing is left.
func (e *Encoder) narrow(st *state, outer image.Rectangle) bool {
	st.scissor, st.bound = st.scissor.Intersect(outer), st.bound.Intersect(outer)
	if st.scissor.Empty() {
		st.skip = true
		return false
	}
	if st.clip >= 0 && e.clips[st.clip].covers(st.bound) {
		st.clip = -1
	}
	if st.mask >= 0 && st.bound.In(e.masks[st.mask].full) {
		st.mask = -1
	}
	return true
}

// clipRRect intersects the clip of st, a state just pushed, with a local
// rectangle or rounded rectangle (raster's pushClip).
func (e *Encoder) clipRRect(st *state, rr gfx.RRect) {
	var sc softClip
	var outer image.Rectangle
	if st.ctm.IsAxisAligned() {
		sh, bounds, ok := deviceShape(&st.ctm, rr)
		if !ok {
			st.skip = true // snapped to nothing
			return
		}
		outer, sc = bounds, softClip{sh: sh}
		sc.full, sc.full2 = deviceFull(&sh)
	} else {
		sh, bounds, ok := localShape(&st.ctm, rr)
		if !ok {
			st.skip = true
			return
		}
		outer, sc = bounds, softClip{sh: sh}
	}
	if !e.narrow(st, outer) || sc.covers(st.bound) {
		return // nothing left; or pixel aligned, or nothing left at its edges
	}
	if st.clip < 0 {
		e.clips = append(e.clips, sc)
		st.clip = len(e.clips) - 1
		return
	}
	e.pushMask(st, maskNode{rr: rr, full: sc.full})
}

// clipPath intersects the clip of st, a state just pushed, with a path.
func (e *Encoder) clipPath(st *state, p gfx.Path, rule gfx.FillRule) {
	if e.narrow(st, pathPixels(&st.ctm, p)) {
		e.pushMask(st, maskNode{path: p, rule: rule, isPath: true})
	}
}

// pushMask makes n, with its shape set, the clip mask of st, inside the
// mask st has.
func (e *Encoder) pushMask(st *state, n maskNode) {
	r := e.res
	n.parent, n.ctm, n.rect = st.mask, st.ctm, st.bound
	def := maskDef{rule: n.rule, rr: n.rr, ctm: n.ctm, rect: n.rect, tol: raster.FlattenTolerance}
	if n.isPath {
		def.path = r.pathID(n.path)
	}
	if n.parent >= 0 {
		def.parent = e.masks[n.parent].id
		n.full = n.full.Intersect(e.masks[n.parent].full)
	}
	m := r.maskIDs[def]
	if m == nil {
		m = &maskID{id: r.newID()}
		r.maskIDs[def] = m
	}
	m.used = r.frame
	n.id = m.id
	e.masks = append(e.masks, n)
	st.mask = len(e.masks) - 1
}

// maskTile returns the atlas entry of tile (i, j) of mask mi and the device
// pixels it holds. The entry is nil when the atlas has no room for it.
func (e *Encoder) maskTile(mi, i, j int) (*atlasEntry, image.Rectangle) {
	r := e.res
	n := &e.masks[mi]
	a := &r.atlas[AtlasCoverage]
	rect := image.Rect(i*a.tile, j*a.tile, (i+1)*a.tile, (j+1)*a.tile).Intersect(n.rect)
	key := atlasKey{kind: keyMask, id: n.id, tx: int32(i), ty: int32(j)}
	if ent := a.get(&key, r.frame); ent != nil {
		return ent, rect
	}
	if rect.Empty() {
		return a.add(key, tileEmpty, 0, 0, r.frame), rect
	}
	return e.store(AtlasCoverage, key, rect.Dx(), rect.Dy(), e.maskPixels(mi, rect), rect.Dx()), rect
}

// maskPixels rasterizes the pixels rect of mask mi: the coverage of its
// shape, multiplied by that of the masks it is nested in, from the
// outermost in, with the rounding of the rasterizer's nested masks.
func (e *Encoder) maskPixels(mi int, rect image.Rectangle) []uint8 {
	r := e.res
	n := rect.Dx() * rect.Dy()
	var chain [16]int
	nodes := chain[:0]
	for k := mi; k >= 0; k = e.masks[k].parent {
		nodes = append(nodes, k)
	}
	acc := scratch(&r.tmp, n)
	for k := len(nodes) - 1; k >= 0; k-- {
		node := &e.masks[nodes[k]]
		dst := acc
		if k != len(nodes)-1 {
			dst = scratch(&r.tmp2, n)
		}
		if node.isPath {
			r.masker.Fill(dst, rect, node.ctm, node.path, node.rule)
		} else {
			r.masker.RRect(dst, rect, node.ctm, node.rr)
		}
		if k != len(nodes)-1 {
			for i, c := range dst {
				acc[i] = mul8(c, acc[i])
			}
		}
	}
	return acc
}

// mul8 returns round(a*b/255) exactly (raster's mul8).
func mul8(a, b uint8) uint8 {
	t := uint32(a)*uint32(b) + 128
	return uint8((t + t>>8) >> 8)
}

// deviceFull returns pixels a device shape covers entirely: the band
// between its top and bottom corners and the band between its left and
// right ones, the larger first; the whole rectangle, twice, when its
// corners are square.
func deviceFull(s *shape) (image.Rectangle, image.Rectangle) {
	x0, y0, x1, y1 := s.rect[0], s.rect[1], s.rect[2], s.rect[3]
	top, bottom := max(s.radY[0], s.radY[1]), max(s.radY[2], s.radY[3])
	left, right := max(s.radX[0], s.radX[3]), max(s.radX[1], s.radX[2])
	if math.IsInf(x0, 0) || math.IsInf(y0, 0) || math.IsInf(x1, 0) || math.IsInf(y1, 0) {
		return image.Rectangle{}, image.Rectangle{}
	}
	px := func(v float64, up bool) int {
		v = clampLim(v)
		if up {
			return int(math.Ceil(v))
		}
		return int(math.Floor(v))
	}
	h := rectNoSwap(px(x0, true), px(y0+top, true), px(x1, false), px(y1-bottom, false))
	v := rectNoSwap(px(x0+left, true), px(y0, true), px(x1-right, false), px(y1, false))
	if v.Dx()*v.Dy() > h.Dx()*h.Dy() {
		return v, h
	}
	return h, v
}
