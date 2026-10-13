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
// (percentages and auto margins count as 0).
func edgeWidths(cs *style.ComputedStyle) inlineEdgeWidths {
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
		border: layout.Insets{Top: cs.Border.Top.Width, Right: cs.Border.Right.Width, Bottom: cs.Border.Bottom.Width, Left: cs.Border.Left.Width},
		padding: layout.Insets{Top: lp(cs.Padding.Top), Right: lp(cs.Padding.Right),
			Bottom: lp(cs.Padding.Bottom), Left: lp(cs.Padding.Left)},
	}
}

// fragBorderBox returns the border box of fragment f of an inline element
// with style cs, in the coordinates of the paragraph's content box.
func fragBorderBox(cs *style.ComputedStyle, f *text.InlineFragment) gfx.Rect {
	e := edgeWidths(cs)
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
		w := fragBorderBox(cs, f).Width()
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
	fi := 0
	for li := range l.NumLines() {
		for ; fi < len(frags) && frags[fi].Line == li; fi++ {
			if visible {
				p.paintFragment(para, frags, fi, origin)
			}
		}
		if visible {
			l.PaintLine(p.b, origin, li, float32(p.scale))
		}
		for i, k := range para.atoms {
			if k.parent == bx && l.BoxLine(para.atomBox[i]) == li {
				p.paintFlow(k)
			}
		}
	}
	if !visible {
		return
	}
	// Outlines, after the content (CSS paints them last).
	for i := range frags {
		f := &frags[i]
		cs := para.inlineStyle(f.Inline)
		if cs == nil || cs.Visibility != style.VisibilityVisible || cs.Outline.Width <= 0 || cs.Outline.Color.A == 0 ||
			cs.Outline.Style == style.BorderNone || cs.Outline.Style == style.BorderHidden {
			continue
		}
		r := fragBorderBox(cs, f).Translate(origin.X, origin.Y)
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
	cs := para.inlineStyle(f.Inline)
	if cs == nil || cs.Visibility != style.VisibilityVisible || !hasDecorations(cs) {
		return
	}
	r := fragBorderBox(cs, f)
	if !(r.Width() > 0) || !(r.Height() > 0) {
		return
	}
	pad := edgeWidths(cs).padding
	if f.Left && f.Right {
		g := p.geomOf(cs, r.Translate(origin.X, origin.Y))
		p.paintDecor(cs, &g, pad, false)
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
	p.paintDecor(cs, &g, pad, false)
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
		r := fragBorderBox(cs, f)
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
			u = u.Union(fragBorderBox(cs, &frags[i]))
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
		if fragBorderBox(cs, f).Contains(pt) {
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
			dst = append(dst, fragBorderBox(cs, &f))
		}
	}
	return dst
}
