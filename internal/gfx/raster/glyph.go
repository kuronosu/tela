package raster

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Text: DrawGlyphRun. See "Text" in the package documentation.

const (
	// glyphSubpixels is the number of horizontal pen positions per device
	// pixel that the cache distinguishes: pen x is rounded to a quarter of a
	// device pixel.
	glyphSubpixels = 4
	// maxCachedEm is the largest device em size, in device pixels, whose
	// glyphs are cached. Larger text is rare, its masks are large and it
	// gains little from caching, so it is filled as paths.
	maxCachedEm = 256
	// maxGlyphSide bounds the width and height, in device pixels, of a
	// cached glyph (from its em bounds); a glyph whose bounds reach further
	// (a font with huge glyphs) is filled as a path.
	maxGlyphSide = 1024
	// glyphPosLimit bounds the device pen position of a cached glyph, so
	// that the integer arithmetic of the placement cannot overflow and the
	// glyph stays inside the ±coordLimit box; pens further away are filled
	// as paths, which clip their geometry to the box.
	glyphPosLimit = coordLimit / 2
	// maxGlyphNesting bounds color glyphs drawn inside color glyphs (a
	// DisplayList of a color glyph may contain glyph runs). Deeper color
	// glyphs are not drawn, which stops a font whose color glyphs refer to
	// themselves.
	maxGlyphNesting = 3
)

// glyphCache returns the cache of the current Render.
func (r *Renderer) glyphCache() *GlyphCache {
	if r.glyphs != nil {
		return r.glyphs
	}
	return defaultGlyphCache
}

// glyphRun is the state of the DrawGlyphRun being drawn.
type glyphRun struct {
	c      *gfx.DrawGlyphRun
	fg     gfx.Color // foreground of color glyphs
	paint  bool      // the paint of outline glyphs can show
	shaded bool      // the blitter's shader is the run's paint
	fast   bool      // the transform allows cached glyphs
	sx, sy float32   // device em size along x and y (fast)
	key    glyphKey  // the key of the glyph being drawn, gid and sub aside
}

// drawGlyphRun draws a run: through the glyph cache under a transform that
// is a positive scale and a translation, by filling outlines (and drawing
// color glyphs inline) otherwise.
func (r *Renderer) drawGlyphRun(c gfx.DrawGlyphRun) {
	if r.blit.clip.Empty() {
		return
	}
	run := glyphRun{c: &c, paint: true}
	if col, ok := c.Paint.(gfx.Color); ok {
		run.fg, run.paint = col, col.A > 0
	}
	m := &r.ctm
	if m[1] == 0 && m[2] == 0 && !r.tune.glyphPaths {
		// The device em size is what ctm.Mul(Scale(Size)) gives: the
		// product rounded once to float32.
		run.sx = float32(float64(m[0]) * float64(c.Size))
		run.sy = float32(float64(m[3]) * float64(c.Size))
		run.fast = run.sx > 0 && run.sy > 0 && run.sx <= maxCachedEm && run.sy <= maxCachedEm
	}
	if run.fast {
		run.key = glyphKey{
			font: c.Font.FontKey(),
			sx:   math.Float32bits(run.sx), sy: math.Float32bits(run.sy),
			tol: math.Float32bits(FlattenTolerance),
		}
	}
	for _, g := range c.Glyphs {
		gb := c.Font.GlyphBounds(g.ID)
		if gb.Empty() { // also NaN
			continue
		}
		lx, ly := c.Origin.X+g.X, c.Origin.Y+g.Y
		if run.fast && r.glyphCached(&run, g.ID, gb, lx, ly) {
			continue
		}
		r.glyphDirect(&run, g.ID, gb, lx, ly)
	}
}

// useRunPaint makes the run's paint the blitter's shader, under the run's
// transform (r.ctm).
func (r *Renderer) useRunPaint(run *glyphRun) {
	if !run.shaded {
		r.blit.setShader(r.shaderFor(run.c.Paint))
		run.shaded = true
	}
}

// glyphCached draws glyph id with its pen at local (lx, ly) from the glyph
// cache. It reports false, having drawn nothing, if the glyph is too far
// away or too large to be cached; the caller then draws it directly.
//
// The device pen position is rounded to a quarter pixel in x and to a whole
// pixel in y. The glyph is rendered (on a miss) with its pen at
// (sub/4, 0), sub in 0..3, and placed at whole pixels: a mask holds exactly
// the coverage that fillPath gives for the outline under the transform
// Scale(sx, sy) followed by Translate(sub/4, 0), with the same flattening
// tolerance, moved by whole pixels, which do not change coverage.
func (r *Renderer) glyphCached(run *glyphRun, id gfx.GlyphID, gb gfx.Rect, lx, ly float32) bool {
	X, Y := apply(&r.ctm, gfx.Pt(lx, ly)) // exact products
	if !(math.Abs(X) <= glyphPosLimit && math.Abs(Y) <= glyphPosLimit) {
		return false
	}
	// X*glyphSubpixels and the division are exact (powers of two).
	q := math.Floor(X*glyphSubpixels + 0.5)
	ix := int(math.Floor(q / glyphSubpixels))
	sub := int(q) - glyphSubpixels*ix
	iy := int(math.Floor(Y + 0.5))
	ox := float64(sub) / glyphSubpixels

	// The device box of the glyph's em bounds, relative to (ix, iy). The
	// products of float32 values are exact in float64.
	sx, sy := float64(run.sx), float64(run.sy)
	bx0, bx1 := float64(gb.X0)*sx+ox, float64(gb.X1)*sx+ox
	by0, by1 := float64(gb.Y0)*sy, float64(gb.Y1)*sy
	if !(bx1-bx0 <= maxGlyphSide && by1-by0 <= maxGlyphSide) {
		return false
	}
	box := pixelBounds(bx0, by0, bx1, by1).Add(image.Pt(ix, iy))
	if !box.Overlaps(r.blit.clip) {
		return true // nothing to draw in this region
	}

	cache := r.glyphCache()
	k := run.key
	k.gid, k.sub = id, uint8(sub)
	e := cache.get(&k)
	var cl *gfx.DisplayList
	if e == nil {
		font := run.c.Font
		cl = font.ColorGlyph(id, run.fg)
		e = &glyphEntry{key: k, kind: entryEmpty}
		switch {
		case cl != nil:
			e.kind = entryColor
		case !run.paint:
			// The outline would not show: draw nothing and cache nothing.
			return true
		default:
			if p, ok := font.GlyphOutline(id); ok && !p.IsEmpty() {
				e.rect, e.data = r.glyphMask(p, run.sx, run.sy, ox)
				if e.data != nil {
					e.kind = entryMask
				}
			}
		}
		e = cache.put(e)
	}
	switch e.kind {
	case entryMask:
		if run.paint {
			r.useRunPaint(run)
			r.blitMask(e, ix, iy)
		}
	case entryColor:
		if r.glyphDepth >= maxGlyphNesting {
			return true
		}
		ks := k
		ks.kind, ks.depth = keySprite, uint8(r.glyphDepth)
		ks.fg = [4]uint32{math.Float32bits(run.fg.R), math.Float32bits(run.fg.G), math.Float32bits(run.fg.B), math.Float32bits(run.fg.A)}
		s := cache.get(&ks)
		if s == nil {
			if cl == nil {
				cl = run.c.Font.ColorGlyph(id, run.fg)
			}
			s = &glyphEntry{key: ks, kind: entryEmpty}
			if cl != nil {
				var ok bool
				s.rect, s.data, ok = r.glyphSprite(cl, run.sx, run.sy, ox)
				if !ok {
					return false // too large for a sprite: draw it inline
				}
				if s.data != nil {
					s.kind = entrySprite
				}
			}
			s = cache.put(s)
		}
		if s.kind == entrySprite {
			r.blitSprite(s, ix, iy)
		}
	}
	return true
}

// glyphMask rasterizes an em-space outline under Scale(sx, sy) followed by
// Translate(ox, 0) into a new 8-bit coverage mask. It returns the mask's
// pixels and a nil mask if nothing is covered.
func (r *Renderer) glyphMask(p gfx.Path, sx, sy float32, ox float64) (image.Rectangle, []uint8) {
	m := gfx.Affine{sx, 0, 0, sy, float32(ox), 0} // ox is a multiple of 1/4: exact
	tol, ok := flattenTolerance(&m)
	if !ok {
		return image.Rectangle{}, nil
	}
	if r.emitFn == nil {
		r.emitFn = r.emit
	}
	f := &r.fill
	f.reset()
	r.flatM = m
	r.flatCurve = false
	r.flat.Flatten(p, tol, r.emitFn)
	f.closePath()
	if len(f.edges) == 0 {
		return image.Rectangle{}, nil
	}
	minX, minY := int64(math.MaxInt64), int64(math.MaxInt64)
	maxX, maxY := int64(math.MinInt64), int64(math.MinInt64)
	for i := range f.edges {
		e := &f.edges[i]
		minX, maxX = min(minX, e.x0, e.x1), max(maxX, e.x0, e.x1)
		minY, maxY = min(minY, e.y0), max(maxY, e.y1)
	}
	rect := image.Rect(int(minX>>fracBits), int(minY>>fracBits), int((maxX+one-1)>>fracBits), int((maxY+one-1)>>fracBits))
	if rect.Empty() {
		return image.Rectangle{}, nil
	}
	data := make([]uint8, rect.Dx()*rect.Dy())
	b := &r.maskBlit
	*b = blitter{toMask: true, a8: data, a8Stride: rect.Dx(), a8Origin: rect.Min, clip: rect}
	f.rasterize(gfx.NonZero, b)
	b.a8 = nil
	return rect, data
}

// glyphSprite renders a color glyph's em-space DisplayList under
// Scale(sx, sy) followed by Translate(ox, 0) onto a new transparent
// premultiplied RGBA sprite, trimmed to its painted pixels. ok is false if
// the list's bounds are too large for a sprite. A nil sprite means that
// nothing is painted.
func (r *Renderer) glyphSprite(cl *gfx.DisplayList, sx, sy float32, ox float64) (rect image.Rectangle, pix []uint8, ok bool) {
	if r.sprite == nil {
		r.sprite = new(Renderer)
	}
	sp := r.sprite
	sp.glyphs, sp.spriteDepth, sp.tune = r.glyphs, r.glyphDepth+1, r.tune
	root := gfx.Affine{sx, 0, 0, sy, float32(ox), 0}
	sp.root = root
	rect = sp.devBounds(cl.Bounds())
	if rect.Dx() > maxGlyphSide+4 || rect.Dy() > maxGlyphSide+4 {
		return image.Rectangle{}, nil, false
	}
	if rect.Empty() {
		return image.Rectangle{}, nil, true
	}
	img := image.NewRGBA(rect)
	sp.render(img, cl, root, rect)

	// Trim the transparent border.
	tight := image.Rectangle{}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		row := img.Pix[img.PixOffset(rect.Min.X, y):][:4*rect.Dx()]
		x0, x1 := -1, -1
		for i := 3; i < len(row); i += 4 {
			if row[i] != 0 {
				if x0 < 0 {
					x0 = i / 4
				}
				x1 = i/4 + 1
			}
		}
		if x0 >= 0 {
			tight = bbox(tight, image.Rect(rect.Min.X+x0, y, rect.Min.X+x1, y+1))
		}
	}
	if tight.Empty() {
		return image.Rectangle{}, nil, true
	}
	pix = make([]uint8, 4*tight.Dx()*tight.Dy())
	for y := tight.Min.Y; y < tight.Max.Y; y++ {
		i := img.PixOffset(tight.Min.X, y)
		copy(pix[4*(y-tight.Min.Y)*tight.Dx():], img.Pix[i:i+4*tight.Dx()])
	}
	return tight, pix, true
}

// blitMask composites a cached mask placed at whole pixels (ix, iy) with
// the current shader, through the clip.
func (r *Renderer) blitMask(e *glyphEntry, ix, iy int) {
	b := &r.blit
	dr := e.rect.Add(image.Pt(ix, iy)).Intersect(b.clip)
	if dr.Empty() {
		return
	}
	w := e.rect.Dx()
	mx := dr.Min.X - ix - e.rect.Min.X
	for y := dr.Min.Y; y < dr.Max.Y; y++ {
		i := (y-iy-e.rect.Min.Y)*w + mx
		row := e.data[i : i+dr.Dx() : i+dr.Dx()]
		lo, hi := 0, len(row)
		for lo < hi && row[lo] == 0 {
			lo++
		}
		for hi > lo && row[hi-1] == 0 {
			hi--
		}
		if lo < hi {
			b.mask(y, dr.Min.X+lo, row[lo:hi])
		}
	}
}

// blitSprite composites a cached sprite placed at whole pixels (ix, iy)
// with source-over, through the clip.
func (r *Renderer) blitSprite(e *glyphEntry, ix, iy int) {
	b := &r.blit
	dr := e.rect.Add(image.Pt(ix, iy)).Intersect(b.clip)
	if dr.Empty() {
		return
	}
	w := e.rect.Dx()
	mx := dr.Min.X - ix - e.rect.Min.X
	for y := dr.Min.Y; y < dr.Max.Y; y++ {
		i := 4 * ((y-iy-e.rect.Min.Y)*w + mx)
		b.composite(y, dr.Min.X, e.data[i:i+4*dr.Dx():i+4*dr.Dx()], 255)
	}
}

// glyphDirect draws glyph id with its pen at local (lx, ly) without the
// cache: the outline is filled under the glyph's full transform, and a
// color glyph's list is drawn inline under it.
func (r *Renderer) glyphDirect(run *glyphRun, id gfx.GlyphID, gb gfx.Rect, lx, ly float32) {
	c := run.c
	gm := r.ctm.Mul(gfx.Translate(lx, ly)).Mul(gfx.Scale(c.Size, c.Size))
	if !gm.IsFinite() || gm.Det() == 0 {
		return
	}
	if !affineBounds(&gm, gb).Overlaps(r.blit.clip) {
		return
	}
	if cl := c.Font.ColorGlyph(id, run.fg); cl != nil {
		r.drawColorInline(cl, gm)
		run.shaded = false // the list changed the shader
		return
	}
	if !run.paint {
		return
	}
	p, ok := c.Font.GlyphOutline(id)
	if !ok || p.IsEmpty() {
		return
	}
	r.useRunPaint(run)
	saved := r.ctm
	r.ctm = gm
	r.fillPath(p, gfx.NonZero)
	r.ctm = saved
}

// drawColorInline draws the em-space list of a color glyph under the
// glyph's transform gm, as if its commands were part of the list being
// rendered inside a PushTransform. Unlike a sprite, the commands are not
// grouped: they composite one by one onto the surface (which differs from
// a sprite only by rounding, as long as the list composites with
// source-over).
func (r *Renderer) drawColorInline(cl *gfx.DisplayList, gm gfx.Affine) {
	if r.glyphDepth >= maxGlyphNesting {
		return
	}
	dl, root := r.dl, r.root
	r.save()
	r.ctm, r.dl, r.root = gm, cl, gm
	r.glyphDepth++
	for i := range cl.Len() {
		r.visit(i, cl.At(i))
	}
	r.glyphDepth--
	r.dl, r.root = dl, root
	r.restore()
}
