package text

import (
	"math"

	"github.com/go-text/typesetting/language"
)

// BreakLines breaks the paragraph into lines that fit maxWidth (logical
// px; +Inf for no wrapping, NaN is treated as +Inf and negative values as
// 0), balanced or prettified as ParagraphStyle.WrapStyle asks, applies
// line-clamp and text-overflow, computes the line boxes and aligns the
// lines in a box of width maxWidth, or of the widest line when maxWidth is
// infinite (see AlignTo). It never shapes again.
func (l *Layout) BreakLines(maxWidth float32) {
	switch {
	case maxWidth != maxWidth:
		maxWidth = float32(math.Inf(1))
	case maxWidth < 0:
		maxWidth = 0
	}
	l.maxWidth = maxWidth
	n := len(l.clusters)
	l.intrinsic()
	if cap(l.cadv) < n {
		l.cadv = make([]float32, n)
		l.cx = make([]float32, n)
		l.cline = make([]int32, n)
	}
	l.cadv, l.cx, l.cline = l.cadv[:n], l.cx[:n], l.cline[:n]
	tabs := l.cadv // tab advances land in cadv, copied to the clusters below
	l.lines, l.items, l.ellipses = l.lines[:0], l.items[:0], l.ellipses[:0]
	l.breakLines(maxWidth, false, boxOwn, tabs, func(c0, c1 int32, reason BreakReason) {
		l.lines = append(l.lines, lineData{c0: c0, c1: c1, reason: reason, ellipsis: -1})
	})
	if l.para.WrapStyle != TextWrapAuto {
		l.applyWrapStyle(maxWidth, tabs) // text-wrap: balance, pretty (wrapstyle.go)
	}
	for i := range l.clusters {
		if l.clusters[i].flags&cfTab != 0 {
			l.clusters[i].adv = tabs[i]
		}
	}

	clamped := false
	l.hiddenFrom = int32(n)
	if k := l.para.LineClamp; k > 0 && len(l.lines) > k {
		clamped = true
		l.hiddenFrom = l.lines[k].c0
		l.lines = l.lines[:k]
	}
	for i := range l.cline {
		l.cline[i] = -1
	}
	for li := range l.lines {
		// Lines are built in order, so the items of line li are the last
		// ones when truncate rewrites them.
		l.buildLine(li)
		ln := &l.lines[li]
		last := li == len(l.lines)-1
		if (clamped && last) || (l.para.TextOverflow == TextOverflowEllipsis && ln.width > maxWidth+fitEpsilon) {
			l.truncate(li, maxWidth)
		}
	}
	l.buildFrags()
	l.width = 0
	for li := range l.lines {
		l.width = max(l.width, l.lines[li].width)
	}
	l.placeLines()
	aw := maxWidth
	if math.IsInf(float64(aw), 1) {
		aw = l.width
	}
	l.AlignTo(aw)
}

// clusterAdvance is the advance of cluster i on line ln before
// justification.
func (l *Layout) clusterAdvance(ln *lineData, i int32) float32 {
	c := &l.clusters[i]
	if i >= ln.trail && l.removed(c.flags) {
		return 0
	}
	return c.adv
}

// level returns the bidi level of cluster i on line ln: rule L1 resets
// trailing white space to the paragraph level.
func (l *Layout) level(i, bidiTrail int32) uint8 {
	if i >= bidiTrail {
		return l.base
	}
	return l.runs[l.clusters[i].run].level
}

// buildLine computes the trailing white space, width and items of line li.
func (l *Layout) buildLine(li int) {
	ln := &l.lines[li]
	adv := func(i int32) float32 { return l.clusters[i].adv }
	var trail int32
	ln.width, trail, ln.hang = l.lineWidth(ln.c0, ln.c1, ln.reason, l.maxWidth, adv)
	ln.trail = trail
	for i := ln.c0; i < ln.c1; i++ {
		l.cadv[i] = l.clusterAdvance(ln, i)
		l.cline[i] = int32(li)
	}
	l.buildItems(li, ln.c0, ln.c1)
}

// buildItems groups the clusters [c0, c1) of line li into items of one run
// and one level and reorders them visually (UAX #9 L1 and L2).
func (l *Layout) buildItems(li int, c0, c1 int32) {
	ln := &l.lines[li]
	cl := l.clusters
	bidiTrail := ln.c1
	for bidiTrail > ln.c0 && cl[bidiTrail-1].flags&cfBidiWS != 0 {
		bidiTrail--
	}
	i0 := int32(len(l.items))
	for c := c0; c < c1; {
		lvl := l.level(c, bidiTrail)
		r := cl[c].run
		e := c + 1
		for e < c1 && cl[e].run == r && l.level(e, bidiTrail) == lvl {
			e++
		}
		kind := itemText
		if l.runs[r].kind == runBox {
			kind = itemBox
		}
		l.items = append(l.items, itemData{kind: kind, level: lvl, run: r, c0: c, c1: e})
		c = e
	}
	ln.i0, ln.i1 = i0, int32(len(l.items))
	items := l.items[i0:]
	// reorder reverses nothing on a line without an odd level, as most
	// lines are (left to right throughout).
	odd := false
	for i := range items {
		if items[i].level%2 == 1 {
			odd = true
			break
		}
	}
	if !odd {
		return
	}
	l.levels = l.levels[:0]
	for i := range items {
		l.levels = append(l.levels, items[i].level)
	}
	reorder(l.levels, items)
}

// truncate hides content at the end edge of line li (the right edge in a
// left-to-right paragraph, the left edge in a right-to-left one, whatever
// the bidi levels of the content there) until the remaining content and an
// ellipsis fit in maxW, and places the ellipsis at that edge (CSS Overflow
// 3, text-overflow). Hanging white space is dropped first, whole clusters
// and inline boxes are hidden, and the first remaining one is never hidden.
// The ellipsis is styled by the block: the paragraph's Default style, as
// CSS specifies and Chromium does.
func (l *Layout) truncate(li int, maxW float32) {
	ln := &l.lines[li]
	// Rebuild the items without the trailing white space.
	end := ln.trail
	for i := end; i < ln.c1; i++ {
		l.cline[i] = -1
	}
	l.items = l.items[:ln.i0]
	l.buildItems(li, ln.c0, end)
	ln.trail, ln.hang = end, 0

	e := l.ellipsisFor(0)
	content := float32(0)
	count := 0
	for i := ln.c0; i < end; i++ {
		content += l.cadv[i] + l.extAt(i)
		count++
	}
	rtl := l.base == 1
	items := l.items[ln.i0:ln.i1]
	// Pop clusters from the end edge.
	for content+e.width > maxW+fitEpsilon && count > 1 {
		k := len(items) - 1
		if rtl {
			k = 0
		}
		it := &items[k]
		var ci int32
		if (it.level%2 == 0) != rtl {
			// The end edge of this item is its logical end.
			ci = it.c1 - 1
			it.c1--
		} else {
			ci = it.c0
			it.c0++
		}
		// A hidden cluster takes the inline edges attached to it: an
		// inline whose first or last cluster is hidden has no start or
		// end edge on the line.
		content -= l.cadv[ci] + l.extAt(ci)
		count--
		l.cline[ci] = -1
		if it.c0 == it.c1 {
			if rtl {
				items = items[1:]
				ln.i0++
			} else {
				items = items[:len(items)-1]
				ln.i1--
			}
		}
	}
	// Append the ellipsis item at the end edge.
	ei := int32(len(l.ellipses))
	l.ellipses = append(l.ellipses, *e)
	ell := itemData{kind: itemEllipsis, level: l.base, run: ei, c0: -1, c1: -1, w: e.width}
	vis := append([]itemData(nil), l.items[ln.i0:ln.i1]...)
	if rtl {
		vis = append([]itemData{ell}, vis...)
	} else {
		vis = append(vis, ell)
	}
	l.items = append(l.items[:ln.i0], vis...)
	ln.i1 = ln.i0 + int32(len(vis))
	ln.ellipsis = ei
	ln.width = content + e.width
}

// ellipsisFor returns the ellipsis in style s: U+2026 in the first font of
// the style's fallback list that has it, else three periods (CSS allows
// "..." when the ellipsis character is unavailable) in the style's first
// available font.
func (l *Layout) ellipsisFor(s uint16) *ellipsisData {
	si := &l.sinfo[s]
	if si.ellipsis != nil {
		return si.ellipsis
	}
	st := &l.styles[s]
	e := &ellipsisData{style: s, size: si.size}
	ell := []rune{'…'}
	runs := l.coll.Itemize(ell, 0, 1, st.query(), si.size, language.Common, nil)
	var f *Face
	if len(runs) > 0 {
		f = runs[0].Face
	}
	add := func(f *Face, r rune, n int) {
		g, _ := f.GlyphIndex(r)
		adv := float32(f.Advance(g) * si.size)
		for range n {
			e.glyphs = append(e.glyphs, glyphData{id: g, adv: adv})
			e.width += adv + st.LetterSpacing
		}
	}
	switch {
	case f != nil && f.HasRune('…'):
		e.face = f
		add(f, '…', 1)
	case si.face != nil:
		e.face = si.face
		add(si.face, '.', 3)
	}
	si.ellipsis = e
	return e
}

// placeLines computes the line boxes and stacks them: their heights,
// tops and baselines, and the paragraph's height.
func (l *Layout) placeLines() {
	var top float32
	for li := range l.lines {
		ln := &l.lines[li]
		l.lineMetrics(li)
		ln.top = top
		ln.baseline = top + ln.ascent
		top += ln.height
	}
	l.height = top
}

// lineMetrics computes the line box of line li (CSS 2.1, section 10.8):
// the strut (the paragraph's Default style), the span box of every style
// on the line, the boxes of the fonts used when the style's line-height is
// normal (CSS Inline 3, section 4.1), and the inline boxes.
func (l *Layout) lineMetrics(li int) {
	ln := &l.lines[li]
	strut := &l.sinfo[0]
	over, under := strut.over, strut.under
	add := func(o, u float32) {
		over, under = max(over, o), max(under, u)
	}
	var lineRel []int32 // top- and bottom-aligned boxes
	lastStyle, lastRun := int32(-1), int32(-1)
	for _, it := range l.items[ln.i0:ln.i1] {
		switch it.kind {
		case itemEllipsis:
			e := &l.ellipses[it.run]
			si := &l.sinfo[e.style]
			add(si.over, si.under)
			if si.normal && e.face != nil && e.face != si.face {
				add(l.faceBox(e.face, e.size))
			}
		case itemBox:
			b := &l.boxes[l.runs[it.run].box]
			ps := &l.sinfo[l.clusters[it.c0].style]
			switch b.Align {
			case BoxTop, BoxBottom:
				lineRel = append(lineRel, l.runs[it.run].box)
			default:
				add(boxExtents(b, ps))
			}
		default:
			for c := it.c0; c < it.c1; c++ {
				cc := &l.clusters[c]
				if int32(cc.style) != lastStyle {
					lastStyle = int32(cc.style)
					si := &l.sinfo[cc.style]
					add(si.over, si.under)
					lastRun = -1
				}
				if si := &l.sinfo[cc.style]; si.normal && cc.run != lastRun {
					lastRun = cc.run
					if r := &l.runs[cc.run]; r.kind == runText && r.face != si.face {
						add(l.halfLeading(r.ascent, r.descent, l.lineSum(r.ascent, r.descent, r.gap)))
					}
				}
			}
		}
	}
	// Every inline on the line contributes its own strut (CSS 2.1,
	// section 10.8: even an empty inline box has a line height).
	for f := ln.f0; f < ln.f1; f++ {
		si := &l.sinfo[l.inlines[l.frags[f].inline].style]
		add(si.over, si.under)
	}
	for _, bi := range lineRel {
		b := &l.boxes[bi]
		if h := b.Height; h > over+under {
			if b.Align == BoxTop {
				under = h - over
			} else {
				over = h - under
			}
		}
	}
	ln.ascent, ln.descent = over, under
	ln.height = max(0, over+under)
}

// faceBox returns the box above and below the baseline of a font used
// with line-height: normal.
func (l *Layout) faceBox(f *Face, size float32) (over, under float32) {
	m := f.Metrics()
	a, d, g := l.qround(m.Ascent, size), l.qround(m.Descent, size), l.qround(m.LineGap, size)
	return l.halfLeading(a, d, l.lineSum(a, d, g))
}

// boxExtents returns how far a baseline-relative inline box extends above
// and below the baseline; ps is the style of its span.
func boxExtents(b *InlineBox, ps *styleInfo) (over, under float32) {
	h := max(0, b.Height)
	switch b.Align {
	case BoxMiddle:
		return h/2 + ps.xHeight/2, h/2 - ps.xHeight/2
	case BoxTextTop:
		return ps.ascent, h - ps.ascent
	case BoxTextBottom:
		return h - ps.descent, ps.descent
	}
	return h - b.Descent, b.Descent
}

// AlignTo aligns the lines in a box of the given width (logical px) and
// positions every visible cluster, without breaking lines again: for a
// box that ends up wider than the width BreakLines used (stretched flex
// items, for example). Lines that overflow the box are start-aligned
// (CSS Text 3, section 7.1). Justified lines stretch their word
// separators, and the gaps around CJK characters, by equal amounts.
func (l *Layout) AlignTo(width float32) {
	if !(width >= 0) || math.IsInf(float64(width), 0) {
		width = l.width
	}
	l.alignWidth = width
	rtl := l.base == 1
	for li := range l.lines {
		ln := &l.lines[li]
		ln.justify = 0
		for i := ln.c0; i < ln.c1; i++ {
			if l.cline[i] == int32(li) {
				l.cadv[i] = l.clusterAdvance(ln, i)
			}
		}
		free := width - ln.width
		var off float32 // offset of the content without hanging white space
		align := l.para.Align
		last := ln.reason == BreakNone || ln.reason == BreakForced || li == len(l.lines)-1
		if align == AlignJustify && (last || ln.ellipsis >= 0) {
			align = AlignStart
		}
		switch {
		case free <= 0:
			if rtl {
				off = free
			}
		case align == AlignLeft:
		case align == AlignRight:
			off = free
		case align == AlignCenter:
			off = free / 2
		case align == AlignStart:
			if rtl {
				off = free
			}
		case align == AlignEnd:
			if !rtl {
				off = free
			}
		case align == AlignJustify:
			if n := l.justify(li); n > 0 {
				ln.justify = free / float32(n)
				for i := ln.c0; i < ln.trail; i++ {
					if u := l.justUnits[i]; u > 0 && l.cline[i] == int32(li) {
						l.cadv[i] += float32(float32(u) * ln.justify)
					}
				}
			} else if rtl {
				off = free
			}
		}
		ln.x = off
		if rtl {
			ln.x -= ln.hang
		}
		if ln.f1 > ln.f0 {
			l.positionInlines(li)
			continue
		}
		// Positions.
		x := ln.x
		for k := ln.i0; k < ln.i1; k++ {
			it := &l.items[k]
			it.x = x - ln.x
			start := x
			switch it.kind {
			case itemEllipsis:
				x += it.w
			default:
				if it.level%2 == 0 {
					for c := it.c0; c < it.c1; c++ {
						l.cx[c] = x
						x += l.cadv[c]
					}
				} else {
					for c := it.c1 - 1; c >= it.c0; c-- {
						l.cx[c] = x
						x += l.cadv[c]
					}
				}
			}
			it.w = x - start
		}
	}
}

// justify sets l.justUnits for the content of line li and returns their
// sum: one opportunity after every word separator, and one on each side of
// every CJK character, shared between neighbors (Blink's expansion
// opportunities), none at the end of the line.
func (l *Layout) justify(li int) int {
	ln := &l.lines[li]
	if len(l.justUnits) < len(l.clusters) {
		l.justUnits = make([]uint8, len(l.clusters))
	}
	n := 0
	after := true // no opportunity at the start of the line
	last := int32(-1)
	for i := ln.c0; i < ln.trail; i++ {
		l.justUnits[i] = 0
		if l.cline[i] != int32(li) {
			continue
		}
		f := l.clusters[i].flags
		switch {
		case f&cfWordSep != 0:
			l.justUnits[i]++
			n++
			after = true
		case f&cfCJK != 0:
			if !after && last >= 0 {
				l.justUnits[last]++
				n++
			}
			l.justUnits[i]++
			n++
			after = true
		default:
			after = false
		}
		last = i
	}
	if last >= 0 && l.justUnits[last] > 0 {
		l.justUnits[last]--
		n--
	}
	return n
}
