package core

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// CSS effects of the paint phase (ADR-079): clip-path, the isolation that
// mix-blend-mode needs, the glyph masks of background-clip: text, and CSS
// masks (mask-image and the mask-* properties, ADR-098; at the end).
//
// # clip-path
//
// A box with a clip path is a stacking context (CSS Masking 1). Its shape,
// resolved against the unsnapped border box (style.ClipPath.Resolve; the
// default reference box), is pushed inside its transform and before its
// backdrop filter and its layer (pushEffects), so that it clips
// everything the stacking context paints, shadows, outline, filter
// results and descendants included, as in CSS (filters, then the clip,
// then opacity and blending). inset() with or without radii, circle() and
// ellipse() are rounded-rectangle clips; polygon() is a path clip with its
// fill rule. Hit-testing applies the same shape (hit.go: stacked), in the
// box's coordinates after its transform is undone.
//
// # mix-blend-mode
//
// A box with a blend mode is a stacking context painted in a layer that
// composites with that mode (gfx.PushLayer's Blend). CSS blends it with
// the content of its parent stacking context only, which is an isolated
// group: a stacking context that has blending descendants in its own
// scope (Box.blendDesc, computed by place) pushes a layer even when it has
// no opacity or filter of its own, so that what is painted before it, out
// of the group, is not the backdrop. The root's group is the canvas: a
// blend at the top level blends with the window's background.
//
// # background-clip: text
//
// The backgrounds of a box with bg-clip-text are painted inside a mask
// layer (gfx.PushMaskLayer, ADR-074) whose mask is the glyph runs of the
// box's text and of its in-flow descendants' (the foreground text of CSS),
// painted opaque, so the background shows where the glyphs are: color
// glyphs (emoji) mask with their own alpha, and text decorations are part
// of the mask, like Chromium's text clip. Text that is in another scroll
// space (the content of a scroll container that moves) is not part of the
// mask. An inline element's fragment is masked by the glyphs of its line
// (the background is already clipped to the fragment). The text itself is
// painted afterwards as usual, so bg-clip-text is used with
// text-transparent.

// pushClipPath pushes the clip path of bx, when it has one, and reports
// whether it did.
func (p *painter) pushClipPath(bx *Box) bool {
	cp := &bx.pcs.ClipPath
	if cp.IsNone() {
		return false
	}
	g := cp.Resolve(p.rect(bx))
	switch {
	case g.IsPoly:
		p.b.PushClipPath(g.Path(), g.Rule)
	case g.RRect.IsRect():
		p.b.PushClipRect(g.RRect.Rect)
	default:
		p.b.PushClipRRect(g.RRect)
	}
	return true
}

// inClipPath reports whether point pt (in the coordinates bx is painted
// in, its transform undone) is inside the clip path of bx.
func inClipPath(bx *Box, pt gfx.Point) bool {
	g := bx.pcs.ClipPath.Resolve(bx.u)
	if !g.IsPoly {
		return rrectContains(g.RRect, pt)
	}
	return polygonContains(g.Poly, g.Rule, pt)
}

// polygonContains reports whether pt is inside the polygon under rule
// (half-open crossings, like the rasterizer's sampling at pixel centers).
func polygonContains(poly []gfx.Point, rule gfx.FillRule, pt gfx.Point) bool {
	wind := 0
	n := len(poly)
	for i := range n {
		a, b := poly[i], poly[(i+1)%n]
		if (a.Y <= pt.Y) == (b.Y <= pt.Y) {
			continue
		}
		// The x where the edge crosses the horizontal line through pt.
		t := float64(pt.Y-a.Y) / float64(b.Y-a.Y)
		x := float64(a.X) + float64(float64(b.X-a.X)*t)
		if x > float64(pt.X) {
			if b.Y > a.Y {
				wind++
			} else {
				wind--
			}
		}
	}
	if rule == gfx.EvenOdd {
		return wind%2 != 0
	}
	return wind != 0
}

// blendsIn reports whether box k, a child of a box in the same stacking
// context scope, blends with that stacking context: it has a blend mode,
// or it is not a stacking context and has such a descendant.
func blendsIn(k *Box) bool {
	return k.pcs.MixBlendMode != gfx.SrcOver || (k.blendDesc && !stacking(k))
}

// textMask returns the mask of background-clip: text for box bx: the
// glyphs of its text and of its in-flow descendants, painted opaque, in
// the coordinates bx is painted in.
func (p *painter) textMask(bx *Box) *gfx.DisplayList {
	L := &p.t.lay
	b := p.b
	p.b = L.builder()
	p.maskText(bx)
	dl := p.b.Finish()
	L.free = append(L.free, p.b)
	p.b = b
	return dl
}

// maskText paints the glyphs of bx and its in-flow descendants (inside
// their overflow clips) into the mask being recorded.
func (p *painter) maskText(bx *Box) {
	if bx.scr != nil && bx.scr.chunked {
		return // in another scroll space
	}
	clipped := clipsOverflow(bx.pcs) && p.pushOverflowClip(bx)
	if bx.mode == modeParagraph && bx.para != nil && bx.para.lay != nil && bx.pcs.Visibility == style.VisibilityVisible {
		c := contentRect(p.rect(bx), bx.ld.Layout())
		bx.para.prepare(p.t, max(c.Width(), 0))
		bx.para.lay.PaintWith(p.b, c.Origin(), float32(p.scale), gfx.Black)
	}
	for _, k := range bx.kids {
		if k.ls.Display == layout.DisplayNone || k.hiddenAtom || k.pcs == nil || participant(k) {
			continue
		}
		p.maskText(k)
	}
	if clipped {
		p.b.PopClip()
	}
}

// lineMask returns the mask of background-clip: text for a fragment of
// an inline element on line li of layout l, whose content box is at
// origin: the glyphs of the line, painted opaque.
func (p *painter) lineMask(l *text.Layout, li int, origin gfx.Point) *gfx.DisplayList {
	L := &p.t.lay
	mb := L.builder()
	l.PaintLineWith(mb, origin, li, float32(p.scale), gfx.Black)
	dl := mb.Finish()
	L.free = append(L.free, mb)
	return dl
}

// ---------------------------------------------------------------------
// mask-image (ADR-098).

// cssMask returns the mask of box bx as an alpha mask list, in the
// coordinates bx is painted in: its layers painted like background
// layers (sized, positioned and tiled in their origin box, clipped to the
// mask clip box with its radii, or unclipped), turned into coverage by
// the mask mode (a luminance layer becomes white covered by its
// luminance) and combined from the bottom layer up by mask-composite:
// add paints a layer over the result, intersect paints it masked by the
// result, subtract masked by its inverse, exclude adds both differences.
// An opaque gradient layer (Tailwind's unset mask variables) is skipped
// under intersect, where it changes nothing. ok is false when the mask
// changes nothing (every layer was skipped).
func (p *painter) cssMask(bx *Box) (list *gfx.DisplayList, ok bool) {
	m := &bx.pcs.Mask
	g := p.geom(bx)
	box := func(b style.BackgroundBox) gfx.RRect {
		switch b {
		case style.BackgroundPaddingBox:
			return g.padding
		case style.BackgroundContentBox:
			return p.contentRRect(bx, &g)
		}
		return g.border
	}
	clip := box(m.Clip)
	if m.NoClip {
		clip = gfx.RRect{Rect: bx.ink.Union(g.border.Rect)}
	}
	var acc *gfx.DisplayList
	have := false
	for i := len(m.Layers) - 1; i >= 0; i-- {
		l := &m.Layers[i]
		if m.Composite == style.MaskIntersect && opaqueLayer(l) {
			continue
		}
		lay := p.maskLayer(l, box(l.Origin).Rect, clip, m.Mode)
		if !have {
			acc, have = lay, true
			continue
		}
		acc = p.composeMask(lay, acc, m.Composite, clip.Rect)
	}
	if !have {
		// Only opaque layers: the mask clips to its box.
		if clip.Rect.ContainsRect(bx.ink) {
			return nil, false
		}
		b := p.t.lay.builder()
		b.FillRRect(clip, gfx.White)
		return p.finishMask(b), true
	}
	return acc, true
}

// opaqueLayer reports whether a mask layer is an opaque gradient that
// tiles its whole area (Tailwind's linear-gradient(#fff, #fff)).
func opaqueLayer(l *style.BackgroundLayer) bool {
	if l.Gradient == nil || len(l.Gradient.Stops) == 0 || l.Repeat != style.RepeatBoth {
		return false
	}
	for _, s := range l.Gradient.Stops {
		if s.Color != gfx.White {
			return false
		}
	}
	return true
}

// maskLayer paints one mask layer into a list of alpha coverage.
func (p *painter) maskLayer(l *style.BackgroundLayer, area gfx.Rect, clip gfx.RRect, mode style.MaskMode) *gfx.DisplayList {
	saved := p.b
	p.b = p.t.lay.builder()
	p.paintBgLayer(l, area, clip, "mask")
	list := p.finishMask(p.b)
	p.b = saved
	if mode != style.MaskModeLuminance || list == nil {
		return list
	}
	b := p.t.lay.builder()
	b.PushMaskLayer(gfx.Mask{List: list, Mode: gfx.MaskLuminance}, 1, gfx.SrcOver)
	b.FillRRect(clip, gfx.White)
	b.PopLayer()
	return p.finishMask(b)
}

// composeMask combines mask layer top with the result of the layers below
// it.
func (p *painter) composeMask(top, below *gfx.DisplayList, op style.MaskComposite, area gfx.Rect) *gfx.DisplayList {
	b := p.t.lay.builder()
	switch op {
	case style.MaskAdd:
		appendMask(b, below)
		appendMask(b, top)
	case style.MaskIntersect:
		b.PushMaskLayer(gfx.Mask{List: below, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
		appendMask(b, top)
		b.PopLayer()
	case style.MaskSubtract:
		p.subtractMask(b, top, below, area)
	case style.MaskExclude:
		p.subtractMask(b, top, below, area)
		b.PushLayer(1, gfx.PlusLighter)
		p.subtractMask(b, below, top, area)
		b.PopLayer()
	}
	return p.finishMask(b)
}

// subtractMask paints a × (1 − b's alpha) into mb: a masked by the
// luminance of white with b painted black over it.
func (p *painter) subtractMask(mb *gfx.Builder, a, b *gfx.DisplayList, area gfx.Rect) {
	inv := p.t.lay.builder()
	inv.FillRect(area, gfx.White)
	if b != nil {
		inv.PushLayer(1, gfx.SrcOver, gfx.Brightness(0))
		inv.AppendList(b, nil)
		inv.PopLayer()
	}
	mb.PushMaskLayer(gfx.Mask{List: p.finishMask(inv), Mode: gfx.MaskLuminance}, 1, gfx.SrcOver)
	appendMask(mb, a)
	mb.PopLayer()
}

func appendMask(b *gfx.Builder, l *gfx.DisplayList) {
	if l != nil {
		b.AppendList(l, nil)
	}
}

// finishMask finishes a mask builder from the pool and returns it; an
// empty list is nil (a transparent layer).
func (p *painter) finishMask(b *gfx.Builder) *gfx.DisplayList {
	list := b.Finish()
	p.t.lay.free = append(p.t.lay.free, b)
	if list.Len() == 0 {
		return nil
	}
	return list
}
