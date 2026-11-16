package gpu

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Text: DrawGlyphRun, with the decisions and the pixels of the rasterizer
// (see "Text" in package raster's documentation). raster.Masker resolves
// every glyph of a run the way a Render does, from the glyph cache the two
// backends share:
//
//   - a cached mask is 8-bit coverage placed at whole pixels: it goes into
//     the coverage atlas under the rasterizer's own key and is drawn as a
//     quad with the run's paint, so the text has the rasterizer's coverage;
//   - a cached sprite of a color glyph is premultiplied RGBA: it goes into
//     the color atlas and is composited with the run's alpha;
//   - an outline that the rasterizer fills directly (rotations, skews,
//     large sizes) is a path like any other, under the glyph's transform;
//   - a color glyph too large or too transformed for a sprite is its
//     display list, encoded in place under the glyph's transform, its
//     layers included.

// glyphRun encodes a DrawGlyphRun.
func (e *Encoder) glyphRun(c gfx.DrawGlyphRun) {
	st := e.top()
	r := e.res
	alpha, ok := r.masker.BeginGlyphRun(&c, st.ctm, e.GlyphCache, e.glyphDepth)
	if !ok {
		return
	}
	// The paint of the outline glyphs, in the run's space. A run whose
	// paint shows nothing still draws its color glyphs.
	pe, painted := e.paint(c.Paint, &st.ctm)
	gl := &e.glyph
	for _, g := range c.Glyphs {
		r.masker.Glyph(g, st.scissor, gl)
		switch gl.Kind {
		case raster.GlyphMask:
			if painted {
				e.picture(atlasKey{kind: keyGlyph, glyph: gl.Key}, gl.Rect, gl.Pix, st, &pe, 0)
			}
		case raster.GlyphSprite:
			if alpha != 0 {
				e.picture(atlasKey{kind: keyGlyph, glyph: gl.Key}, gl.Rect, gl.Pix, st, nil, alpha)
			}
		case raster.GlyphOutline:
			if painted {
				e.cover(&gl.Transform, gl.Outline, gfx.NonZero, nil, &pe, st)
			}
		case raster.GlyphList:
			list, gm := gl.List, gl.Transform
			e.inline(list, gm)
			// The list may hold runs of its own: this one goes on.
			st = e.top()
			r.masker.BeginGlyphRun(&c, st.ctm, e.GlyphCache, e.glyphDepth)
		}
	}
	*gl = raster.Glyph{} // keep no pixels or lists alive
}

// inline encodes the commands of a color glyph's list under the glyph's
// transform, as if they were part of the list being encoded inside a
// PushTransform (raster's drawColorInline): one by one, not as a group.
func (e *Encoder) inline(cl *gfx.DisplayList, gm gfx.Affine) {
	depth := len(e.stack)
	e.push().ctm = gm
	e.glyphDepth++
	// The root space of the glyph's list is the em square: its layers take
	// their bounds from it.
	dl, root := e.dl, e.root
	e.dl, e.root = cl, gm
	for i := range cl.Len() {
		e.visit(i, cl.At(i))
	}
	e.dl, e.root = dl, root
	e.glyphDepth--
	e.stack = e.stack[:depth]
}
