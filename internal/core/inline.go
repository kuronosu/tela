package core

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Inline boxes and atomic inlines of paragraphs (ARCHITECTURE section 11;
// ADR-066). The box phase gives a paragraph's text.Layout one text.Inline
// per inline element (Span, Strong, Em, Code, A, anything with display:
// inline), whose edges (margin, border and padding in the inline axis)
// take space on its lines, and one inline box per atomic inline, whose box
// is a child of the paragraph box (structure.go, box.go). This file paints
// and hit-tests them.
//
// # Fragments
//
// The text layout splits each inline into fragments (text.InlineFragment:
// one per line, and per bidi run that separates its content). A
// fragment's border box spans its content and the edges it has, without
// its margins, horizontally, and the content area of the element's font
// (its quantized ascent and descent around the baseline) grown by the
// vertical padding and borders, vertically: vertical padding and borders
// are painted, overlap the lines around, and do not change the line
// heights (CSS 2.1 section 10.6.1). Percentages of padding and margins
// count as 0 on inline elements.
//
// # Painting
//
// Decorations follow box-decoration-break: slice (the CSS initial
// value): a fragment without its start or end edge is a slice of the box
// as if its fragments were laid side by side in the order of the text,
// left to right (right to left in a right-to-left paragraph). It is
// painted as that stitched box (shadows, backgrounds with their gradients
// positioned in it, borders, rounded corners at its real ends only),
// clipped to the fragment at the sides it lacks (Blink paints multi-line
// inline backgrounds the same way). Outlines are drawn around each
// fragment's border box.
//
// A paragraph paints line by line, as CSS 2.1 appendix E does: the
// decorations of the inline elements on the line (an element before the
// elements inside it), then the line's text, then the atomic inlines on
// it, each painted atomically like an in-flow block. A later line
// therefore covers what the decorations of an earlier one painted under
// it, and an inline's vertical padding covers the line above. Atomic
// inlines hidden by an ellipsis or line-clamp are not painted. Effects
// (opacity, transforms, filters) of inline elements are not painted.
//
// A relative (or sticky) inline element is positioned: its content (its
// decorations, text and atomic inlines, with the inline elements inside
// it) is painted after the rest of the paragraph, in tree order, as CSS
// paints positioned descendants after in-flow content (appendix E, step
// 8), and a relative one is moved by its insets with everything it holds
// (box.go: offsetInlines; text.Layout.SetInlineOffset, ADR-268).
//
// # Hit-testing
//
// Hit-testing mirrors the painting order: atomic inlines (as boxes, the
// paragraph box's children), then, on the text, the innermost element of
// the cluster under the point unless a fragment of a later line covers it,
// then the topmost fragment under the point (border box, corners not
// rounded), so an A with padding is clicked on its padding.

// fragments returns the inline fragments of the paragraph's current lines
// (prepare first).
func (p *paragraph) fragments() []text.InlineFragment {
	if p.lay == nil || len(p.inl) == 0 {
		return nil
	}
	key := [2]float32{p.brokeAt, p.alignedAt}
	if p.fragAt != key || p.frags == nil {
		p.frags = p.lay.InlineFragments(p.frags[:0])
		if p.frags == nil {
			p.frags = []text.InlineFragment{}
		}
		p.fragAt = key
	}
	return p.frags
}

// decorated reports whether the paragraph paints more than its text:
// inline elements or atomic inlines.
func (p *paragraph) decorated() bool { return len(p.inl) > 0 || len(p.atoms) > 0 }

// inlineEdgeWidths are the margins, borders and padding of an inline
// element's box, in px.
type inlineEdgeWidths struct {
	margin, border, padding layout.Insets
}

// edgeWidths returns the edges of an inline element with style cs
// (percentages and auto margins count as 0), its border widths snapped to
// the device pixels of display scale k (snapLineWidth).
func edgeWidths(cs *style.ComputedStyle, k float32) inlineEdgeWidths {
	m := func(d style.Dim) float32 {
		if d.Kind == style.DimLength && d.LP.Pct == 0 {
			return d.LP.Px
		}
		return 0
	}
	lp := func(v style.LP) float32 {
		if v.Pct == 0 {
			return v.Px
		}
		return 0
	}
	return inlineEdgeWidths{
		margin: layout.Insets{Top: m(cs.Margin.Top), Right: m(cs.Margin.Right), Bottom: m(cs.Margin.Bottom), Left: m(cs.Margin.Left)},
		border: layout.Insets{Top: snapLineWidth(cs.Border.Top.Width, float64(k)), Right: snapLineWidth(cs.Border.Right.Width, float64(k)),
			Bottom: snapLineWidth(cs.Border.Bottom.Width, float64(k)), Left: snapLineWidth(cs.Border.Left.Width, float64(k))},
		padding: layout.Insets{Top: lp(cs.Padding.Top), Right: lp(cs.Padding.Right),
			Bottom: lp(cs.Padding.Bottom), Left: lp(cs.Padding.Left)},
	}
}

// fragBorderBox returns the border box of fragment f of an inline element
// with style cs, in the coordinates of the paragraph's content box, at
// display scale k.
func fragBorderBox(cs *style.ComputedStyle, f *text.InlineFragment, k float32) gfx.Rect {
	e := edgeWidths(cs, k)
	x0, x1 := f.X0, f.X1
	if f.Left {
		x0 += e.margin.Left
	}
	if f.Right {
		x1 -= e.margin.Right
	}
	return gfx.Rect{
		X0: x0, Y0: f.Baseline - f.Ascent - e.padding.Top - e.border.Top,
		X1: max(x0, x1), Y1: f.Baseline + f.Descent + e.padding.Bottom + e.border.Bottom,
	}
}

// fragBox returns the border box of fragment f of an inline element of the
// paragraph with style cs (fragBorderBox), its borders snapped at the
// scale the paragraph's inline edges were assembled at.
func (p *paragraph) fragBox(cs *style.ComputedStyle, f *text.InlineFragment) gfx.Rect {
	return fragBorderBox(cs, f, p.edgeK)
}

// inlineStyle returns the style of inline k of the paragraph, or nil when
// its element is gone.
func (p *paragraph) inlineStyle(k int) *style.ComputedStyle {
	if k < 0 || k >= len(p.inl) {
		return nil
	}
	el := p.inl[k]
	if el == nil || !el.mounted || el.box.cs == nil {
		return nil
	}
	return el.box.cs
}

// stitched returns the box that the fragment frags[i] is a slice of: the
// fragments of its inline laid side by side in text order, as one box
// with the height of frags[i], in content box coordinates.
func (p *paragraph) stitched(frags []text.InlineFragment, i int, cs *style.ComputedStyle, r gfx.Rect) gfx.Rect {
	k := frags[i].Inline
	rtl := p.lay.RTL()
	var before, total float32
	first, last := -1, -1
	// Fragments are by line, then left to right: text order is left to
	// right within a line, or right to left in a right-to-left paragraph.
	for j := range frags {
		f := &frags[j]
		if f.Inline != k {
			continue
		}
		if first < 0 || f.Line < frags[first].Line || f.Line == frags[first].Line && (f.X0 < frags[first].X0) != rtl {
			first = j
		}
		if last < 0 || f.Line > frags[last].Line || f.Line == frags[last].Line && (f.X0 > frags[last].X0) != rtl {
			last = j
		}
		w := p.fragBox(cs, f).Width()
		total += w
		switch {
		case f.Line < frags[i].Line:
			before += w
		case f.Line == frags[i].Line && !rtl && f.X0 < frags[i].X0:
			before += w
		case f.Line == frags[i].Line && rtl && f.X0 > frags[i].X0:
			before += w
		}
	}
	// The ends of the box that are not on the lines (hidden by
	// line-clamp or text-overflow) are beyond the visible fragments: far
	// enough that no rounded corner reaches them.
	more := max(r.Height(), 1)
	if f := &frags[first]; (rtl && !f.Right) || (!rtl && !f.Left) {
		before += more
		total += more
	}
	if f := &frags[last]; (rtl && !f.Left) || (!rtl && !f.Right) {
		total += more
	}
	if rtl {
		x1 := r.X1 + before
		return gfx.Rect{X0: x1 - total, Y0: r.Y0, X1: x1, Y1: r.Y1}
	}
	x0 := r.X0 - before
	return gfx.Rect{X0: x0, Y0: r.Y0, X1: x0 + total, Y1: r.Y1}
}

// paintParagraph paints the text, the inline decorations and the atomic
// inlines of paragraph box bx whose content box is at origin, line by
// line (see the top of this file); text reports whether the paragraph's
// own text and decorations are visible.
func (p *painter) paintParagraph(bx *Box, origin gfx.Point, visible bool) {
	para := bx.para
	l := para.lay
	frags := para.fragments()
	paints := p.textPaints(bx)
	for _, k := range para.atoms {
		// Their sizes place the text on the lines, also where they are
		// culled (record.go).
		p.dep(k)
	}
	var rects []selRect
	var bg gfx.Color
	var fg gfx.Paint
	if visible {
		// The static selection (textsel.go), line by line: over the
		// decorations of the inline elements, under the text.
		rects, bg, fg = p.selHighlight(bx)
	}
	// The content of positioned inline elements (relative spans) is
	// painted after the rest, each in its layer, in tree order (CSS 2.1
	// appendix E, step 8; text.Layout.InlineLayer).
	layered := len(para.posInl) > 0
	paintLayer := func(layer int) {
		fi := 0
		for li := range l.NumLines() {
			for ; fi < len(frags) && frags[fi].Line == li; fi++ {
				if visible && (!layered || l.InlineLayer(frags[fi].Inline) == layer) {
					p.paintFragment(para, frags, fi, origin)
				}
			}
			if visible {
				if layer < 0 {
					p.paintSelBg(rects, origin, bg, li)
				}
				if layered {
					l.PaintLineLayer(p.b, origin, li, float32(p.scale), paints, layer)
				} else {
					l.PaintLineRecolored(p.b, origin, li, float32(p.scale), paints)
				}
				if layer < 0 {
					p.paintSelText(l, rects, origin, fg, li)
				}
			}
			for i, k := range para.atoms {
				if k.parent == bx && l.BoxLine(para.atomBox[i]) == li && (!layered || l.BoxLayer(para.atomBox[i]) == layer) {
					p.paintFlow(k)
				}
			}
		}
	}
	paintLayer(-1)
	for _, k := range para.posInl {
		paintLayer(k)
	}
	if !visible {
		return
	}
	// Outlines, after the content (CSS paints them last).
	for i := range frags {
		f := &frags[i]
		cs := p.inlinePaintStyle(para, f.Inline)
		if cs == nil || cs.Visibility != style.VisibilityVisible || cs.Outline.Width <= 0 || cs.Outline.Color.A == 0 ||
			cs.Outline.Style == style.BorderNone || cs.Outline.Style == style.BorderHidden {
			continue
		}
		r := para.fragBox(cs, f).Translate(origin.X, origin.Y)
		g := p.geomOf(cs, r)
		if !g.border.Rect.Empty() {
			p.paintOutlineOf(cs, &g)
		}
	}
}

// paintFragment paints the decorations of fragment frags[i] (shadows,
// background, border), a slice of its stitched box when it lacks an edge.
func (p *painter) paintFragment(para *paragraph, frags []text.InlineFragment, i int, origin gfx.Point) {
	f := &frags[i]
	cs := p.inlinePaintStyle(para, f.Inline)
	if cs == nil || cs.Visibility != style.VisibilityVisible || !hasDecorations(cs) {
		return
	}
	r := para.fragBox(cs, f)
	if !(r.Width() > 0) || !(r.Height() > 0) {
		return
	}
	pad := edgeWidths(cs, para.edgeK).padding
	var mask func() *gfx.DisplayList
	if cs.BackgroundClip == style.BackgroundText {
		// Its glyphs: those of its line (the background is clipped to the
		// fragment).
		mask = func() *gfx.DisplayList { return p.lineMask(para.lay, f.Line, origin) }
	}
	if f.Left && f.Right {
		g := p.geomOf(cs, r.Translate(origin.X, origin.Y))
		p.paintDecor(cs, &g, pad, false, mask)
		return
	}
	s := para.stitched(frags, i, cs, r).Translate(origin.X, origin.Y)
	r = r.Translate(origin.X, origin.Y)
	const far = 1 << 24
	clip := gfx.Rect{X0: -far, Y0: -far, X1: far, Y1: far}
	if !f.Left {
		clip.X0 = p.snap(r.X0)
	}
	if !f.Right {
		clip.X1 = p.snap(r.X1)
	}
	g := p.geomOf(cs, s)
	p.b.PushClipRect(clip)
	p.paintDecor(cs, &g, pad, false, mask)
	p.b.PopClip()
}

// hasDecorations reports whether a box with style cs paints decorations
// under its content.
func hasDecorations(cs *style.ComputedStyle) bool {
	if cs.BackgroundColor.A > 0 || len(cs.Backgrounds) > 0 || len(cs.BoxShadows) > 0 {
		return true
	}
	for _, b := range [4]*style.Border{&cs.Border.Top, &cs.Border.Right, &cs.Border.Bottom, &cs.Border.Left} {
		if b.Width > 0 && b.Color.A > 0 && b.Style != style.BorderNone && b.Style != style.BorderHidden {
			return true
		}
	}
	return false
}

// decorInk returns a rectangle that contains what the inline decorations
// of the paragraph paint, for a content box at c: the fragments' border
// boxes grown by their shadows and outlines.
func (p *paragraph) decorInk(c gfx.Point) gfx.Rect {
	var u gfx.Rect
	frags := p.fragments()
	for i := range frags {
		f := &frags[i]
		cs := p.inlineStyle(f.Inline)
		if cs == nil {
			continue
		}
		r := p.fragBox(cs, f)
		g := float32(1)
		for j := range cs.BoxShadows {
			if s := &cs.BoxShadows[j]; !s.Inset {
				g = max(g, float32(math.Abs(float64(s.OffsetX)))+float32(math.Abs(float64(s.OffsetY)))+max(s.Spread, 0)+1.5*max(s.Blur, 0)+2)
			}
		}
		if o := &cs.Outline; o.Width > 0 {
			g = max(g, max(o.Offset, 0)+o.Width+2)
		}
		// A slice paints its stitched box cut to its own width: the
		// shadows reach beyond its border box at the edges it has and
		// above and below it, never further.
		u = u.Union(r.Outset(g))
	}
	if u.Empty() {
		return u
	}
	return u.Translate(c.X, c.Y)
}

// fragmentsBox returns the union of the fragments' border boxes, for a
// content box at c (the hit bounds of the decorations).
func (p *paragraph) fragmentsBox(c gfx.Point) gfx.Rect {
	var u gfx.Rect
	frags := p.fragments()
	for i := range frags {
		if cs := p.inlineStyle(frags[i].Inline); cs != nil {
			u = u.Union(p.fragBox(cs, &frags[i]))
		}
	}
	if u.Empty() {
		return u
	}
	return u.Translate(c.X, c.Y)
}

// fragmentAt returns the topmost fragment whose border box holds the
// point (content box coordinates) and whose element takes pointer events,
// with its line, or -1.
func (p *paragraph) fragmentAt(pt gfx.Point) (k, line int) {
	frags := p.fragments()
	for i := len(frags) - 1; i >= 0; i-- {
		f := &frags[i]
		cs := p.inlineStyle(f.Inline)
		if cs == nil || cs.PointerEvents != style.PointerEventsAuto || cs.Visibility != style.VisibilityVisible {
			continue
		}
		if p.fragBox(cs, f).Contains(pt) {
			return f.Inline, f.Line
		}
	}
	return -1, -1
}

// inlineIndex returns the index of inline element el in the paragraph, or
// -1.
func (p *paragraph) inlineIndex(el *Element) int {
	for k, e := range p.inl {
		if e == el {
			return k
		}
	}
	return -1
}

// inlineRects appends the border boxes of the fragments of inline element
// el to dst, in the coordinates of the paragraph's content box.
func (p *paragraph) inlineRects(el *Element, dst []gfx.Rect) []gfx.Rect {
	k := p.inlineIndex(el)
	if k < 0 {
		return dst
	}
	cs := p.inlineStyle(k)
	if cs == nil {
		return dst
	}
	for _, f := range p.fragments() {
		if f.Inline == k {
			dst = append(dst, p.fragBox(cs, &f))
		}
	}
	return dst
}

// Out-of-flow boxes in a paragraph's inline content are children of the
// block that holds the paragraph (structure.go, ADR-176). The box records
// its paragraph and its place in the text (Box.anchor), and the paragraph
// box gives layout its static position and, when a positioned inline
// element holds it, its containing block (ADR-268).

// InlineParagraph implements layout.InlineAnchored: the paragraph box b's
// content was hoisted from, while it is b's sibling.
func (b *Box) InlineParagraph() layout.Node {
	if a := b.anchor; a != nil && a.para != nil && a.parent == b.parent {
		return a
	}
	return nil
}

// anchoredIn returns the paragraph of b that places box c (laid out at a
// border-box width w), its lines prepared for that width; nil if b does
// not place c.
func (b *Box) anchoredIn(c layout.Node, w float32) (*Box, *paragraph) {
	cb, _ := c.(*Box)
	p := b.para
	if cb == nil || cb.anchor != b || p == nil || p.lay == nil || p.empty {
		return nil, nil
	}
	// An anonymous paragraph box has no edges: its content box is its
	// border box.
	p.prepare(b.tree, w)
	return cb, p
}

// StaticPosition implements layout.InlinePlacer, as Blink places
// out-of-flow inline items (text/anchor.go): for a box that was
// inline-level, its anchor's place on its line and the line's top, with
// the box's left edge there when the anchor is left-to-right text and its
// right edge when it is right-to-left ("XX <span absolute>YY" in a
// right-to-left paragraph has the box's left edge after "XX "); for a
// block-level one, the start of its line in the paragraph's direction, or
// of the next line after content on its own.
func (b *Box) StaticPosition(c layout.Node, w float32) (x, y float32, right, ok bool) {
	cb, p := b.anchoredIn(c, w)
	if cb == nil {
		return 0, 0, false, false
	}
	l := p.lay
	ap, found := l.AnchorPosition(int(cb.anchorBox))
	if !found {
		// Hidden by text-overflow or line-clamp: at the ellipsis.
		ca := l.Caret(text.Cursor{Offset: int(cb.anchorOff), Affinity: text.Upstream})
		ap = text.AnchorPos{X: ca.X, Top: ca.Top, Height: ca.Height, Line: ca.Line, RTL: l.RTL(),
			AfterContent: int(cb.anchorOff) > l.Line(ca.Line).TextStart}
	}
	if cb.cs != nil && cb.cs.InlineOrigin {
		return ap.X, ap.Top, ap.RTL, true
	}
	y = ap.Top
	if ap.AfterContent {
		y += ap.Height
	}
	if l.RTL() {
		x = w
	}
	return x, y, l.RTL(), true
}

// ContainingBlock implements layout.InlinePlacer: for box c held by a
// relative or sticky inline element of the paragraph, the padding boxes of
// the element's fragments on its first and last lines, from the left edge
// of the first to the right edge of the last (empty when the last ends
// before the first starts) and from the top of the first to the bottom of
// the last, as Chromium does across lines (CSS 2.1 section 10.1 leaves it
// undefined).
func (b *Box) ContainingBlock(c layout.Node, w float32) (gfx.Rect, bool) {
	cb, p := b.anchoredIn(c, w)
	if cb == nil || cb.anchorCB < 0 {
		return gfx.Rect{}, false
	}
	k := int(cb.anchorCB)
	cs := p.inlineStyle(k)
	if cs == nil {
		return gfx.Rect{}, false
	}
	e := edgeWidths(cs, p.edgeK)
	first, last := -1, -1
	var r gfx.Rect
	var right float32
	for _, f := range p.fragments() {
		if f.Inline != k {
			continue
		}
		pad := fragBorderBox(cs, &f, p.edgeK)
		pad.Y0 += e.border.Top
		pad.Y1 -= e.border.Bottom
		if f.Left {
			pad.X0 += e.border.Left
		}
		if f.Right {
			pad.X1 -= e.border.Right
		}
		switch {
		case first < 0 || f.Line < first:
			first, r = f.Line, pad
		case f.Line == first:
			r.X0, r.Y0 = min(r.X0, pad.X0), min(r.Y0, pad.Y0)
		}
		switch {
		case f.Line > last:
			last, right, r.Y1 = f.Line, pad.X1, pad.Y1
		case f.Line == last:
			right, r.Y1 = max(right, pad.X1), max(r.Y1, pad.Y1)
		}
	}
	if first < 0 {
		return gfx.Rect{}, false
	}
	r.X1 = max(right, r.X0)
	return r, true
}
