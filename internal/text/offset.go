package text

import "github.com/kuronosu/tela/internal/gfx"

// Inline offsets (SetInlineOffset) are the relative positions of inlines:
// a CSS inline element with position: relative moves with its content,
// after the lines are laid out (CSS 2.1 section 9.4.3), as in Chromium,
// whose fragments of a relative span carry the offset: its fragments, its
// text with its decorations, the inline boxes and inlines inside it (their
// own offsets added) and the anchors in it. Line breaking, alignment and
// the other content do not move. What reports positions follows: painting,
// InlineFragments, BoxRect, AnchorPosition, Caret and CursorRect,
// hit-testing (horizontally: the line under a point is found without the
// offsets), selection rectangles and visual navigation. Cluster reports
// the layout position, without them.
//
// A positioned inline (SetInlinePositioned) is painted after the other
// content of the paragraph, as CSS paints positioned descendants after the
// in-flow content (CSS 2.1 appendix E, step 8), with its content: its text
// is in its own paint layer (PaintLineLayer), the innermost positioned
// inline around a cluster deciding (InlineLayer, BoxLayer).

// SetInlineOffset sets the offset of Paragraph.Inlines[i] to (dx, dy)
// (logical px; a non-finite value counts as 0) and reports whether it
// changed.
func (l *Layout) SetInlineOffset(i int, dx, dy float32) bool {
	if i < 0 || i >= len(l.inlines) {
		return false
	}
	dx, dy = finiteOr0(dx), finiteOr0(dy)
	if l.ioff == nil {
		if dx == 0 && dy == 0 {
			return false
		}
		l.ioff = make([]gfx.Point, len(l.inlines))
	}
	if l.ioff[i] == (gfx.Point{X: dx, Y: dy}) {
		return false
	}
	l.ioff[i] = gfx.Point{X: dx, Y: dy}
	l.coff = nil
	return true
}

// inlineOff returns the offset of inline k: its own and those of the
// inlines around it.
func (l *Layout) inlineOff(k int32) (dx, dy float32) {
	if l.ioff == nil {
		return 0, 0
	}
	for ; k >= 0; k = l.inlines[k].parent {
		dx += l.ioff[k].X
		dy += l.ioff[k].Y
	}
	return dx, dy
}

// clusterOff returns the offset of cluster c: that of the innermost
// inline that holds it.
func (l *Layout) clusterOff(c int32) (dx, dy float32) {
	if l.ioff == nil || l.cin == nil || c < 0 || int(c) >= len(l.cin) {
		return 0, 0
	}
	if l.coff == nil {
		l.coff = make([]gfx.Point, len(l.cin))
		for i, k := range l.cin {
			x, y := l.inlineOff(k)
			l.coff[i] = gfx.Point{X: x, Y: y}
		}
	}
	o := l.coff[c]
	return o.X, o.Y
}

// SetInlinePositioned marks Paragraph.Inlines[i] as positioned (CSS
// position: relative or sticky), or not, and reports whether that
// changed: its content is painted in a layer of its own (PaintLineLayer).
func (l *Layout) SetInlinePositioned(i int, on bool) bool {
	if i < 0 || i >= len(l.inlines) {
		return false
	}
	if l.ipos == nil {
		if !on {
			return false
		}
		l.ipos = make([]bool, len(l.inlines))
	}
	if l.ipos[i] == on {
		return false
	}
	l.ipos[i] = on
	l.clayer = nil
	return true
}

// InlineLayer returns the paint layer of inline k: the innermost
// positioned inline that holds it (itself included), or -1.
func (l *Layout) InlineLayer(k int) int {
	if l.ipos == nil || k < 0 || k >= len(l.inlines) {
		return -1
	}
	for i := int32(k); i >= 0; i = l.inlines[i].parent {
		if l.ipos[i] {
			return int(i)
		}
	}
	return -1
}

// BoxLayer returns the paint layer of Paragraph.Boxes[i] (InlineLayer of
// the innermost inline that holds it), or -1.
func (l *Layout) BoxLayer(i int) int {
	if i < 0 || i >= l.nboxes {
		return -1
	}
	return int(l.clusterLayer(l.boxClus[i]))
}

// clusterLayer returns the paint layer of cluster c.
func (l *Layout) clusterLayer(c int32) int32 {
	if l.ipos == nil || l.cin == nil || c < 0 || int(c) >= len(l.cin) {
		return -1
	}
	if l.clayer == nil {
		l.clayer = make([]int32, len(l.cin))
		for i, k := range l.cin {
			l.clayer[i] = int32(l.InlineLayer(int(k)))
		}
	}
	return l.clayer[c]
}

// PaintLineLayer is PaintLineRecolored for the clusters of line i in
// paint layer layer only: -1 for those outside every positioned inline,
// k for those whose innermost positioned inline is k (InlineLayer).
func (l *Layout) PaintLineLayer(b *gfx.Builder, origin gfx.Point, i int, scale float32, paints []gfx.Paint, layer int) {
	if i < 0 || i >= len(l.lines) {
		return
	}
	o := paintOver{styles: paints, layered: true, layer: int32(layer)}
	l.paintDecorations(b, origin, i, DecorationUnderline|DecorationOverline, scale, o)
	l.paintGlyphs(b, origin, i, nil, o)
	l.paintDecorations(b, origin, i, DecorationLineThrough, scale, o)
}

// PaintLineLayerWith is PaintLineLayer with every glyph run and
// decoration painted with paint, as in PaintWith (nil keeps the styles'
// paints): the selected text of a paint layer drawn again in the
// selection's color under a clip.
func (l *Layout) PaintLineLayerWith(b *gfx.Builder, origin gfx.Point, i int, scale float32, paint gfx.Paint, layer int) {
	if i < 0 || i >= len(l.lines) {
		return
	}
	o := paintOver{all: paint, layered: true, layer: int32(layer)}
	l.paintDecorations(b, origin, i, DecorationUnderline|DecorationOverline, scale, o)
	l.paintGlyphs(b, origin, i, nil, o)
	l.paintDecorations(b, origin, i, DecorationLineThrough, scale, o)
}

// skips reports whether painting with o leaves out cluster c (another
// layer's).
func (o paintOver) skips(l *Layout, c int32) bool {
	return o.layered && l.clusterLayer(c) != o.layer
}
