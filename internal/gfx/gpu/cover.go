package gpu

import (
	"bytes"
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Coverage from the atlas: filled paths and strokes (and the glyphs the
// rasterizer fills as paths).
//
// The coverage of a path is the rasterizer's own (raster.Masker): the same
// flattening, the same 1/256 pixel grid, the same rules, and for a stroke
// the same outline. It is rasterized under the path's transform without
// the whole pixels of its translation, which only move a picture: a shape
// is found again, by its content, wherever it is scrolled to, and its
// picture is placed at whole pixels like a cached glyph. The pixels the
// shape can touch are cut into tiles (one for a small shape), and only the
// tiles a frame draws are rasterized, each whole: a huge path costs what
// is seen of it, and nothing of it depends on the region rendered. A tile
// that turns out empty or entirely covered takes no room in the atlas: the
// first draws nothing, the second a plain rectangle.

// originLimit bounds the whole pixels taken out of a translation, so that
// the target stays well inside the rasterizer's ±2^20 box in the shape's
// coordinates. Beyond it a shape is rasterized where it is.
const originLimit = 1 << 19

// coverKey is what makes the coverage of a shape: equal keys mean
// identical pixels.
type coverKey struct {
	path, style uint64    // the numbers of the path and of the stroke style (0: a fill)
	m           [6]uint32 // the transform, as float32 bits, without the whole pixels of its translation
	tol         uint32    // raster.FlattenTolerance
	rule        gfx.FillRule
}

// coverShape is a shape whose coverage is in the atlas, tile by tile.
type coverShape struct {
	id   uint64
	m    gfx.Affine
	path gfx.Path // the path filled: for a stroke, its outline
	rule gfx.FillRule
	// rel holds the pixels the shape can touch, relative to the whole
	// pixels of its translation.
	rel image.Rectangle
	// pieces are the parts of rel that hold coverage (pieces.go), made
	// when the shape is first drawn.
	pieces []coverPiece
	used   uint64
	idle   uint16
}

// coverSrc says where the pixels of an instance's own atlas tile come
// from.
type coverSrc struct {
	atlas int
	key   atlasKey
	rect  image.Rectangle // the device pixels of the tile
	// The pixels: the coverage of shape over rel, rasterized on a miss (or
	// with full, known to be covered entirely: nothing is in the atlas)...
	// With rows, every row of the picture is its first one.
	shape *coverShape
	rel   image.Rectangle
	full  bool
	rows  bool
	// ...or a part of a picture in memory, whose first pixel is the device
	// pixel org, with stride bytes per row.
	pix    []uint8
	org    image.Point
	stride int
}

// pathPixels returns the device pixels the path can touch under m: those
// its control points span. (Snapping to the rasterizer's grid moves a
// point by less than a pixel and never past a pixel boundary it is not
// on.)
func pathPixels(m *gfx.Affine, p gfx.Path) image.Rectangle {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, pt := range p.Points() {
		x, y := apply(m, pt.X, pt.Y)
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	if !(x0 <= x1 && y0 <= y1) { // no points, or NaN
		return image.Rectangle{}
	}
	return rectNoSwap(
		int(math.Floor(clampLim(x0))), int(math.Floor(clampLim(y0))),
		int(math.Ceil(clampLim(x1))), int(math.Ceil(clampLim(y1))))
}

// cover encodes a path filled with rule under m, or stroked when style is
// not nil, painted with pe (a paint of the current state, whose transform m
// is unless the path is a glyph's).
func (e *Encoder) cover(m *gfx.Affine, p gfx.Path, rule gfx.FillRule, style *gfx.StrokeStyle, pe *paintEnc, st *state) {
	if p.IsEmpty() || !m.IsFinite() {
		return
	}
	r := e.res
	tx, ty := float64(m[4]), float64(m[5])
	ix, iy := math.Floor(tx), math.Floor(ty)
	if !(math.Abs(ix) <= originLimit && math.Abs(iy) <= originLimit) {
		ix, iy = 0, 0
	}
	// A float32 minus its whole part is a float32.
	fm := gfx.Affine{m[0], m[1], m[2], m[3], float32(tx - ix), float32(ty - iy)}
	key := coverKey{path: r.pathID(p), rule: rule, tol: math.Float32bits(raster.FlattenTolerance)}
	for i, v := range fm {
		key.m[i] = math.Float32bits(v)
	}
	if style != nil {
		key.style, key.rule = r.styleID(style), gfx.NonZero
	}
	sh := r.shapes[key]
	if sh == nil {
		sh = &coverShape{id: r.newID(), m: fm, path: p, rule: key.rule}
		if style != nil {
			sh.path = r.masker.StrokeOutline(p, *style, fm)
		}
		sh.rel = pathPixels(&fm, sh.path)
		r.shapes[key] = sh
	}
	sh.used = r.frame
	org := image.Pt(int(ix), int(iy))
	vis := st.scissor.Sub(org).Intersect(sh.rel)
	if vis.Empty() {
		return
	}
	if sh.pieces == nil {
		r.buildPieces(sh, r.atlas[AtlasCoverage].tile)
	}
	for k := range sh.pieces {
		pc := &sh.pieces[k]
		if !pc.rel.Overlaps(vis) {
			continue
		}
		dev := pc.rel.Add(org)
		var in instance
		quad := e.begin(&in, dev)
		flags := FlagCoverAtlas | pe.put(&in, float64(quad.Min.X), float64(quad.Min.Y))
		e.src = coverSrc{
			atlas: AtlasCoverage, key: atlasKey{kind: keyCover, id: sh.id, tx: int32(k)},
			rect: dev, shape: sh, rel: pc.rel, full: pc.full, rows: pc.rows,
		}
		e.emit(&in, flags, st, quad, &e.src)
	}
}

// fullTile is the entry of a piece known to be covered entirely.
var fullTile = atlasEntry{state: tileFull}

// picture encodes a picture in memory: a glyph's mask (pix holds the
// coverage of the device pixels rect, painted with pe) or, when pe is nil,
// a color glyph's sprite (premultiplied RGBA, faded by alpha). It is cut
// into tiles like any other entry: one, but for the largest glyphs.
func (e *Encoder) picture(key atlasKey, rect image.Rectangle, pix []uint8, st *state, pe *paintEnc, alpha uint8) {
	ai := AtlasCoverage
	if pe == nil {
		ai = AtlasColor
	}
	a := &e.res.atlas[ai]
	vis := st.scissor.Intersect(rect)
	if vis.Empty() {
		return
	}
	t := a.tile
	x0, y0 := rect.Min.X, rect.Min.Y
	for j := (vis.Min.Y - y0) / t; y0+j*t < vis.Max.Y; j++ {
		for i := (vis.Min.X - x0) / t; x0+i*t < vis.Max.X; i++ {
			tile := image.Rect(x0+i*t, y0+j*t, x0+(i+1)*t, y0+(j+1)*t).Intersect(rect)
			var in instance
			quad := e.begin(&in, tile)
			ox, oy := float64(quad.Min.X), float64(quad.Min.Y)
			var flags uint32
			if pe != nil {
				flags = FlagCoverAtlas | pe.put(&in, ox, oy)
			} else {
				// The sprite's rectangle, covered entirely.
				sh := shape{rect: [4]float64{float64(tile.Min.X), float64(tile.Min.Y), float64(tile.Max.X), float64(tile.Max.Y)}}
				sh.put(&in, SlotShape, ox, oy)
				in[SlotHead][2] = uint32(alpha) << 24
				flags = PaintSprite << FlagPaintShift
			}
			key.tx, key.ty = int32(i), int32(j)
			e.src = coverSrc{atlas: ai, key: key, rect: tile, pix: pix, org: rect.Min, stride: rect.Dx() * a.bpp}
			e.emit(&in, flags, st, quad, &e.src)
		}
	}
}

// acquire returns the atlas entry of an instance's own tile, rasterizing
// or copying its pixels into the frame's uploads when the atlas does not
// have it. It returns nil when the atlas has no room for it.
func (e *Encoder) acquire(s *coverSrc) *atlasEntry {
	r := e.res
	a := &r.atlas[s.atlas]
	if s.full {
		return &fullTile
	}
	if ent := a.get(&s.key, r.frame); ent != nil {
		return ent
	}
	w, h := s.rect.Dx(), s.rect.Dy()
	if s.shape != nil {
		buf := scratch(&r.tmp, w*h)
		if s.rows {
			// One row, repeated (pieces.go), when the last row is the first:
			// the edges are monotonic, so a row between them is too. (An
			// edge vertical in this package's arithmetic and a hair off it in
			// the rasterizer's, a fused product apart, is rasterized whole.)
			first, last := s.rel, s.rel
			first.Max.Y, last.Min.Y = first.Min.Y+1, last.Max.Y-1
			top, bottom := buf[:w], buf[(h-1)*w:h*w]
			r.masker.Fill(top, first, s.shape.m, s.shape.path, s.shape.rule)
			r.masker.Fill(bottom, last, s.shape.m, s.shape.path, s.shape.rule)
			if bytes.Equal(top, bottom) {
				for y := 1; y < h-1; y++ {
					copy(buf[y*w:(y+1)*w], top)
				}
			} else {
				r.masker.Fill(buf, s.rel, s.shape.m, s.shape.path, s.shape.rule)
			}
		} else {
			r.masker.Fill(buf, s.rel, s.shape.m, s.shape.path, s.shape.rule)
		}
		return e.store(s.atlas, s.key, w, h, buf, w)
	}
	off := (s.rect.Min.Y-s.org.Y)*s.stride + (s.rect.Min.X-s.org.X)*a.bpp
	return e.store(s.atlas, s.key, w, h, s.pix[off:], s.stride)
}

// store adds a picture of w x h pixels to the atlas ai, from pix with
// stride bytes per row, and its pixels to the frame's uploads. A picture
// whose pixels are all 0, or all 255 in the coverage atlas, is only noted.
// When the atlas is full it evicts what the frame does not use, the oldest
// first, and when the frame uses all of it, flushes. It returns nil when
// even the emptied atlas cannot hold the picture.
func (e *Encoder) store(ai int, key atlasKey, w, h int, pix []uint8, stride int) *atlasEntry {
	r := e.res
	a := &r.atlas[ai]
	n := w * a.bpp
	first, uniform := pix[0], true
	for y := 0; y < h && uniform; y++ {
		for _, v := range pix[y*stride : y*stride+n] {
			if v != first {
				uniform = false
				break
			}
		}
	}
	if uniform && first == 0 {
		return a.add(key, tileEmpty, 0, 0, r.frame)
	}
	if uniform && first == 255 && a.bpp == 1 {
		return a.add(key, tileFull, 0, 0, r.frame)
	}
	ent := a.add(key, tilePixels, w, h, r.frame)
	for _, age := range [...]uint64{64, 8, 1} {
		if ent != nil {
			break
		}
		if a.evict(r.frame, age) {
			ent = a.add(key, tilePixels, w, h, r.frame)
		}
	}
	if ent == nil {
		e.flush(ai)
		if ent = a.add(key, tilePixels, w, h, r.frame); ent == nil {
			return nil
		}
	}
	f := &e.frame
	for len(f.Data)%4 != 0 {
		f.Data = append(f.Data, 0)
	}
	off := len(f.Data)
	for y := 0; y < h; y++ {
		f.Data = append(f.Data, pix[y*stride:y*stride+n]...)
	}
	e.uploads = append(e.uploads, Upload{Atlas: ai, Page: ent.page, Rect: ent.rect(), Offset: off})
	r.dirty = true
	return ent
}

// flush empties an atlas the frame has filled: the pass being encoded
// ends with the instances drawn so far, which read the atlas as it is, and
// a new pass goes on in the emptied atlas, uploading again what it draws
// with.
func (e *Encoder) flush(ai int) {
	e.endPass()
	e.res.atlas[ai].reset()
	e.flushes++
}

// bound keeps the tables that find pictures again from growing without
// limit in an Encoder that never sees EndFrame: beyond a size, what the
// previous frame did not use goes.
func (r *resources) bound() {
	const limit = 1 << 14
	if len(r.shapes) > limit {
		for k, s := range r.shapes {
			if s.used+1 < r.frame {
				delete(r.shapes, k)
			}
		}
	}
	if len(r.maskIDs) > limit {
		for k, m := range r.maskIDs {
			if m.used+1 < r.frame {
				delete(r.maskIDs, k)
			}
		}
	}
	if len(r.paths.byHash) > limit {
		for h, list := range r.paths.byHash {
			kept := list[:0]
			for _, p := range list {
				if p.used+1 >= r.frame {
					kept = append(kept, p)
				}
			}
			clear(list[len(kept):])
			if len(kept) == 0 {
				delete(r.paths.byHash, h)
			} else {
				r.paths.byHash[h] = kept
			}
		}
	}
	if len(r.styles) > limit {
		clear(r.styles) // styles are small: they are numbered again
	}
}
