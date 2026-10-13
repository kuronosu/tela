package text

import (
	"math"
	"slices"
	"sort"
)

// This file holds inlines (Paragraph.Inlines): CSS inline boxes whose
// edges take space on the line and whose fragments the caller decorates,
// following Blink's inline layout (LayoutNG's open and close tags and
// InlineLayoutStateStack):
//
//   - Membership: every cluster belongs to the innermost inline whose text
//     (or, for an inline box, whose boxes) holds it. An inline left without
//     content by white-space processing (or empty) gets a placeholder: a
//     zero-width inline box that is transparent to line breaking (no break
//     opportunity before it; the opportunity between its neighbors comes
//     after it, so it stays with the content before it, as Blink keeps an
//     empty inline's tags on the line), except forced breaks, which it
//     follows.
//   - Breaking: the start edge of an inline (Left in a left-to-right
//     paragraph, Right in a right-to-left one) is attached to its first
//     cluster and the end edge to its last one (Layout.ext), so an edge
//     goes to the line of the content next to it and counts when fitting
//     it; an edge attached to white space that hangs or is removed at the
//     end of a line still counts.
//   - Fragments: after bidi reordering, an inline has one fragment per
//     maximal run of visually adjacent clusters it holds on a line. Like
//     Blink (InlineLayoutStateStack::UpdateFragmentedBoxDataEdges), the
//     leftmost fragment on a line takes the inline's left edge and the
//     rightmost its right edge, when the inline has that edge on the line:
//     in a left-to-right paragraph the left edge is the start edge (on the
//     line of its first cluster), in a right-to-left one the end edge.
//     Positions add the edges' widths where the fragments open and close
//     (ComputeInlinePositions), so the clusters inside move and the
//     fragment's margin box spans its content and its edges.
//   - Truncation: hidden clusters take the edges attached to them, so an
//     inline whose last cluster is hidden by an ellipsis has no end edge
//     on that line. The ellipsis is inside the inlines whose content on
//     its line the truncation cut, and outside the others (when
//     line-clamp only hides the lines after it, it is outside every
//     inline), as Chromium paints it.
//   - Height: each inline contributes its own strut to the lines it has
//     fragments on; its fragments' content area is its first available
//     font's quantized ascent and descent around the baseline (vertical
//     alignment of inlines other than baseline is not supported).

// inlineData is the resolved form of a Paragraph.Inlines entry.
type inlineData struct {
	parent      int32  // enclosing inline, -1 for none
	style       uint16 // index into Layout.styles
	left, right float32
	sIn, eIn    float32 // start and end edges: left and right swapped in RTL
	c0, c1      int32   // first cluster and last cluster + 1; c0 is -1 without clusters
}

// fragData is a fragment of an inline on one line: the visual positions
// [k0, k1) of the line's elements (clusters and the ellipsis, left to
// right), whether it has the inline's left and right edges, and its margin
// box (x0, x1), set by AlignTo.
type fragData struct {
	inline      int32
	k0, k1      int32
	left, right bool
	x0, x1      float32
}

// boxSizes selects the width of inline boxes for line breaking.
type boxSizes uint8

const (
	boxOwn boxSizes = iota // InlineBox.Width (SetBox)
	boxMin                 // the min-content width given to SetBox
	boxMax                 // the max-content width given to SetBox
)

// boxWidth returns the width of inline box cluster c for sizes.
func (l *Layout) boxWidth(c *cluster, sizes boxSizes) float32 {
	if sizes == boxOwn || l.boxMin == nil {
		return c.adv
	}
	bi := l.runs[c.run].box
	if bi < 0 || int(bi) >= len(l.boxMin) {
		return c.adv
	}
	if sizes == boxMin {
		return l.boxMin[bi]
	}
	return l.boxMax[bi]
}

// extAt returns the inline edges attached to cluster i.
func (l *Layout) extAt(i int32) float32 {
	if l.ext == nil {
		return 0
	}
	return l.ext[i]
}

// addPlaceholders inserts the placeholder boxes of the inlines that have
// no character left after white-space processing, no box and no inline
// inside, where their content would be: after what comes before them in
// the text and the boxes, before what comes after them. It returns the
// runes and the inline of each placeholder; the placeholder of tags[k] is
// box len(p.Boxes)+k. istyle is the style index of Inlines[0].
func addPlaceholders(kept []rawRune, p *Paragraph, istyle int) ([]rawRune, []int32) {
	if len(p.Inlines) == 0 {
		return kept, nil
	}
	hasChild := make([]bool, len(p.Inlines))
	var stack []int
	for i, in := range p.Inlines {
		stack = stack[:in.Depth]
		if in.Depth > 0 {
			hasChild[stack[in.Depth-1]] = true
		}
		stack = append(stack, i)
	}
	nb := int32(len(p.Boxes))
	var tags []int32
	for i := range p.Inlines {
		in := &p.Inlines[i]
		if hasChild[i] || in.BoxStart < in.BoxEnd {
			continue
		}
		j := sort.Search(len(kept), func(j int) bool { return kept[j].src >= int32(in.Start) })
		empty := true
		for k := j; k < len(kept) && kept[k].src < int32(in.End); k++ {
			if kept[k].box < 0 {
				empty = false
				break
			}
		}
		if !empty {
			continue
		}
		// After the boxes before the inline and the placeholders of
		// earlier inlines at the same offset.
		for j < len(kept) && kept[j].box >= 0 && (kept[j].box >= nb || int(kept[j].box) < in.BoxStart) {
			j++
		}
		// perf: one insertion per empty inline.
		kept = slices.Insert(kept, j, rawRune{objectReplacement, int32(in.End), uint16(istyle + i), nb + int32(len(tags))})
		tags = append(tags, int32(i))
	}
	return kept, tags
}

// shapingSplits returns the runes before which shaping breaks: the first
// character at or after each boundary of an inline that has a left or
// right edge (CSS Text 3, section 7.3: shaping is broken across inline box
// boundaries with margins, borders or padding). Nil when there is none.
func shapingSplits(kept []rawRune, p *Paragraph) []bool {
	var split []bool
	for i := range p.Inlines {
		in := &p.Inlines[i]
		if in.Left == 0 && in.Right == 0 {
			continue
		}
		for _, off := range [2]int{in.Start, in.End} {
			j := sort.Search(len(kept), func(j int) bool { return kept[j].src >= int32(off) })
			for j < len(kept) && kept[j].box >= 0 {
				j++
			}
			if j < len(kept) {
				if split == nil {
					split = make([]bool, len(kept)+1)
				}
				split[j] = true
			}
		}
	}
	return split
}

// tagBreaks recomputes the line break opportunities of segs, the
// boundaries of the processed text, as if the placeholders of empty
// inlines were not there: the opportunity between the characters around
// a run of placeholders goes after the run (soft wraps) or before it
// (forced breaks).
func (l *Layout) tagBreaks(segs []uint8) {
	pt := &l.text
	n := len(pt.runes)
	tag := func(i int) bool { return pt.box[i] >= int32(l.nboxes) }
	real := make([]rune, 0, n)
	idx := make([]int32, n+1)
	for i := range n {
		idx[i] = int32(len(real))
		if !tag(i) {
			real = append(real, pt.runes[i])
		}
	}
	idx[n] = int32(len(real))
	fs := segmentText(real)
	const bits = segSoft | segMandatory
	for i := 0; i <= n; i++ {
		f := fs[idx[i]] & bits
		segs[i] &^= bits
		here := i < n && tag(i)
		after := i > 0 && tag(i-1)
		switch {
		case here && !after:
			segs[i] |= f & segMandatory
		case here:
		case after:
			segs[i] |= f & segSoft
		default:
			segs[i] |= f
		}
	}
}

// setupInlines resolves the inlines of p after shaping: their parents,
// edges, clusters and the edges attached to each cluster. tags are the
// inlines of the placeholders (addPlaceholders).
func (l *Layout) setupInlines(p *Paragraph, tags []int32) {
	if len(p.Inlines) == 0 {
		return
	}
	l.inlines = make([]inlineData, len(p.Inlines))
	rtl := l.base == 1
	var stack []int32
	edges := false
	for i, in := range p.Inlines {
		d := &l.inlines[i]
		stack = stack[:in.Depth]
		d.parent = -1
		if in.Depth > 0 {
			d.parent = stack[in.Depth-1]
		}
		stack = append(stack, int32(i))
		d.style = uint16(l.istyle + i)
		d.left, d.right = finiteOr0(in.Left), finiteOr0(in.Right)
		d.sIn, d.eIn = d.left, d.right
		if rtl {
			d.sIn, d.eIn = d.right, d.left
		}
		d.c0, d.c1 = -1, -1
		edges = edges || d.left != 0 || d.right != 0
	}

	n := len(l.clusters)
	l.cin = make([]int32, n)
	for i := range l.cin {
		l.cin[i] = -1
	}
	// The source offset where cluster c's own characters start (the
	// characters removed before the first one belong to it, but not to
	// the inlines it is in).
	start := func(c int) int32 {
		if r := l.clusters[c].r0; r > 0 {
			return l.text.src[r]
		}
		return l.text.first
	}
	for i, in := range p.Inlines {
		c := sort.Search(n, func(c int) bool { return start(c) >= int32(in.Start) })
		for ; c < n && start(c) < int32(in.End); c++ {
			if l.clusters[c].flags&cfBox == 0 {
				l.cin[c] = int32(i)
			}
		}
		for b := in.BoxStart; b < in.BoxEnd; b++ {
			if bc := l.boxClus[b]; bc >= 0 {
				l.cin[bc] = int32(i)
			}
		}
	}
	for k, e := range tags {
		if bc := l.boxClus[l.nboxes+k]; bc >= 0 {
			l.cin[bc] = e
		}
	}
	for c, e := range l.cin {
		for ; e >= 0; e = l.inlines[e].parent {
			d := &l.inlines[e]
			if d.c0 < 0 || int32(c) < d.c0 {
				d.c0 = int32(c)
			}
			d.c1 = max(d.c1, int32(c)+1)
		}
	}
	if edges {
		l.ext = make([]float32, n)
		for i := range l.inlines {
			if d := &l.inlines[i]; d.c0 >= 0 {
				l.ext[d.c0] += d.sIn
				l.ext[d.c1-1] += d.eIn
			}
		}
	}
}

func finiteOr0(v float32) float32 {
	if !finite32(v) {
		return 0
	}
	return v
}

// buildFrags computes the fragments of every line from the visual order
// of its clusters (after truncation), and which of them have the left and
// right edges of their inline.
func (l *Layout) buildFrags() {
	l.frags = l.frags[:0]
	if len(l.inlines) == 0 {
		for li := range l.lines {
			l.lines[li].f0, l.lines[li].f1 = 0, 0
		}
		return
	}
	var stackBuf, chainBuf [16]int32
	stack, chain := stackBuf[:0], chainBuf[:0]
	// Per inline, a mark of the line where it was last seen (fragments,
	// edges, truncation).
	if cap(l.fseen) < len(l.inlines) {
		l.fseen = make([]int32, len(l.inlines))
	}
	seen := l.fseen[:len(l.inlines)]
	clear(seen)
	rtl := l.base == 1
	for li := range l.lines {
		ln := &l.lines[li]
		ln.f0 = int32(len(l.frags))
		v := int32(0)
		visit := func(c int32) {
			chain = chain[:0]
			if c >= 0 {
				for e := l.cin[c]; e >= 0; e = l.inlines[e].parent {
					chain = append(chain, e)
				}
				slices.Reverse(chain)
			}
			if c == ciEllipsis {
				// The ellipsis is inside the inlines whose content on the
				// line the truncation cut (Chromium paints their
				// backgrounds under it), among those of the content next
				// to it; a line-clamp that hides only later lines leaves
				// it outside.
				if a := l.besideEllipsis(ln); a >= 0 {
					cut := l.cutInlines(li, seen)
					for e := l.cin[a]; e >= 0; e = l.inlines[e].parent {
						if seen[e] == cut {
							chain = append(chain, e)
						} else {
							chain = chain[:0] // not cut, and neither are the inlines inside it
						}
					}
					slices.Reverse(chain)
				}
			}
			k := 0
			for k < len(stack) && k < len(chain) && l.frags[stack[k]].inline == chain[k] {
				k++
			}
			for len(stack) > k {
				l.frags[stack[len(stack)-1]].k1 = v
				stack = stack[:len(stack)-1]
			}
			for _, e := range chain[k:] {
				l.frags = append(l.frags, fragData{inline: e, k0: v})
				stack = append(stack, int32(len(l.frags)-1))
			}
			v++
		}
		for k := ln.i0; k < ln.i1; k++ {
			it := &l.items[k]
			switch {
			case it.kind == itemEllipsis:
				visit(ciEllipsis)
			case it.level%2 == 0:
				for c := it.c0; c < it.c1; c++ {
					visit(c)
				}
			default:
				for c := it.c1 - 1; c >= it.c0; c-- {
					visit(c)
				}
			}
		}
		for len(stack) > 0 {
			l.frags[stack[len(stack)-1]].k1 = v
			stack = stack[:len(stack)-1]
		}
		ln.f1 = int32(len(l.frags))
		// The leftmost fragment of an inline takes its left edge, the
		// rightmost its right edge, when the inline has them on the line.
		mark := int32(2*li + 1)
		for f := ln.f0; f < ln.f1; f++ {
			fr := &l.frags[f]
			if seen[fr.inline] != mark {
				seen[fr.inline] = mark
				s, e := l.inlineEdges(fr.inline, li)
				fr.left = s
				if rtl {
					fr.left = e
				}
			}
		}
		mark++
		for f := ln.f1 - 1; f >= ln.f0; f-- {
			fr := &l.frags[f]
			if seen[fr.inline] != mark {
				seen[fr.inline] = mark
				s, e := l.inlineEdges(fr.inline, li)
				fr.right = e
				if rtl {
					fr.right = s
				}
			}
		}
	}
}

// cutInlines marks in seen, with the value it returns, the inlines that
// hold clusters of line li that the truncation hid (not the trailing
// white space it dropped).
func (l *Layout) cutInlines(li int, seen []int32) int32 {
	ln := &l.lines[li]
	mark := -int32(li + 1)
	for c := ln.c0; c < ln.trail; c++ {
		if l.cline[c] >= 0 {
			continue
		}
		for e := l.cin[c]; e >= 0 && seen[e] != mark; e = l.inlines[e].parent {
			seen[e] = mark
		}
	}
	return mark
}

// besideEllipsis returns the visible cluster next to the ellipsis of line
// ln on its start side (before it in a left-to-right paragraph, after it in
// a right-to-left one), or -1.
func (l *Layout) besideEllipsis(ln *lineData) int32 {
	if l.base == 1 {
		if ln.i1-ln.i0 < 2 {
			return -1
		}
		it := &l.items[ln.i0+1]
		if it.kind == itemEllipsis {
			return -1
		}
		if it.level%2 == 0 {
			return it.c0
		}
		return it.c1 - 1
	}
	if ln.i1-ln.i0 < 2 {
		return -1
	}
	it := &l.items[ln.i1-2]
	if it.kind == itemEllipsis {
		return -1
	}
	if it.level%2 == 0 {
		return it.c1 - 1
	}
	return it.c0
}

// inlineEdges reports whether inline e has its start and end edges on line
// li: its first and last clusters are visible there.
func (l *Layout) inlineEdges(e int32, li int) (start, end bool) {
	d := &l.inlines[e]
	if d.c0 < 0 {
		return false, false
	}
	return l.cline[d.c0] == int32(li), l.cline[d.c1-1] == int32(li)
}

// positionInlines positions the items and clusters of line li from ln.x
// (AlignTo), adding the widths of the inline edges where their fragments
// open and close, and sets the fragments' margin boxes.
func (l *Layout) positionInlines(li int) {
	ln := &l.lines[li]
	x := ln.x
	fi := ln.f0
	var stackBuf [16]int32
	stack := stackBuf[:0]
	v := int32(0)
	open := func() {
		for fi < ln.f1 && l.frags[fi].k0 == v {
			f := &l.frags[fi]
			f.x0 = x
			if f.left {
				x += l.inlines[f.inline].left
			}
			stack = append(stack, fi)
			fi++
		}
	}
	shut := func() {
		v++
		for len(stack) > 0 && l.frags[stack[len(stack)-1]].k1 == v {
			f := &l.frags[stack[len(stack)-1]]
			if f.right {
				x += l.inlines[f.inline].right
			}
			f.x1 = x
			stack = stack[:len(stack)-1]
		}
	}
	for k := ln.i0; k < ln.i1; k++ {
		it := &l.items[k]
		if it.kind == itemEllipsis {
			open()
			it.x = x - ln.x
			x += it.w
			shut()
			continue
		}
		var start, end float32
		first := true
		place := func(c int32) {
			open()
			if first {
				it.x, start, first = x-ln.x, x, false
			}
			l.cx[c] = x
			x += l.cadv[c]
			end = x
			shut()
		}
		if it.level%2 == 0 {
			for c := it.c0; c < it.c1; c++ {
				place(c)
			}
		} else {
			for c := it.c1 - 1; c >= it.c0; c-- {
				place(c)
			}
		}
		it.w = end - start
	}
}

// SetBoxSize changes the size of inline box i (an index into
// Paragraph.Boxes) without shaping again: its width, height and descent
// (InlineBox). BreakLines must follow before anything else is read from
// the layout. CSS atomic inlines (inline-blocks), whose size depends on
// the available width, are laid out this way; SetBoxIntrinsic gives their
// intrinsic widths.
func (l *Layout) SetBoxSize(i int, width, height, descent float32) {
	if i < 0 || i >= l.nboxes {
		return
	}
	width, height, descent = max(finiteOr0(width), 0), max(finiteOr0(height), 0), finiteOr0(descent)
	b := &l.boxes[i]
	if b.Width != width && l.boxMin == nil {
		// The intrinsic widths used the box's width.
		l.minContent, l.maxContent = float32(math.NaN()), float32(math.NaN())
	}
	b.Width, b.Height, b.Descent = width, height, descent
	if c := l.boxClus[i]; c >= 0 {
		l.clusters[c].adv = width
	}
}

// SetBoxIntrinsic sets the min-content and max-content widths of inline
// box i, which MinContentWidth and MaxContentWidth use instead of its
// width from then on.
func (l *Layout) SetBoxIntrinsic(i int, minWidth, maxWidth float32) {
	if i < 0 || i >= l.nboxes {
		return
	}
	minWidth, maxWidth = max(finiteOr0(minWidth), 0), max(finiteOr0(maxWidth), 0)
	if l.boxMin == nil {
		l.boxMin = make([]float32, l.nboxes)
		l.boxMax = make([]float32, l.nboxes)
		for k := range l.boxMin {
			l.boxMin[k], l.boxMax[k] = l.boxes[k].Width, l.boxes[k].Width
		}
	} else if l.boxMin[i] == minWidth && l.boxMax[i] == maxWidth {
		return
	}
	l.boxMin[i], l.boxMax[i] = minWidth, maxWidth
	l.minContent, l.maxContent = float32(math.NaN()), float32(math.NaN())
}

// intrinsic computes the min- and max-content widths when they are stale.
func (l *Layout) intrinsic() {
	if math.IsNaN(float64(l.minContent)) || math.IsNaN(float64(l.maxContent)) {
		l.minContent = l.measure(0, true)
		l.maxContent = l.measure(float32(math.Inf(1)), false)
	}
}

// InlineFragment is the part of an inline (Paragraph.Inlines) on one
// line, or one of its parts when bidi reordering separates its content.
type InlineFragment struct {
	// Inline is the index in Paragraph.Inlines.
	Inline int
	// Line is the line it is on.
	Line int
	// X0 and X1 are the left and right edges of its margin box, in layout
	// coordinates: its content and the edges it has.
	X0, X1 float32
	// Left and Right report that it has the inline's left or right edge:
	// the inline's Left or Right width is inside [X0, X1) on that side.
	// A fragment without an edge is cut there (box-decoration-break:
	// slice): no margin, border, padding or rounded corners on that side.
	Left, Right bool
	// Baseline is the y of the line's baseline; Ascent and Descent are
	// the content area above and below it: the quantized ascent and
	// descent of the inline's first available font.
	Baseline, Ascent, Descent float32
}

// NumInlines returns the number of inlines (Paragraph.Inlines).
func (l *Layout) NumInlines() int { return len(l.inlines) }

// InlineFragments appends the fragments of the visible lines to dst: by
// line, then in tree order of the inlines (an inline before the ones it
// contains, which is the painting order of CSS inline boxes), then left to
// right.
func (l *Layout) InlineFragments(dst []InlineFragment) []InlineFragment {
	for li := range l.lines {
		ln := &l.lines[li]
		n := len(dst)
		for f := ln.f0; f < ln.f1; f++ {
			fr := &l.frags[f]
			si := &l.sinfo[l.inlines[fr.inline].style]
			dst = append(dst, InlineFragment{
				Inline: int(fr.inline), Line: li, X0: fr.x0, X1: fr.x1, Left: fr.left, Right: fr.right,
				Baseline: ln.baseline, Ascent: si.ascent, Descent: si.descent,
			})
		}
		slices.SortStableFunc(dst[n:], func(a, b InlineFragment) int {
			if a.Inline != b.Inline {
				return a.Inline - b.Inline
			}
			switch {
			case a.X0 < b.X0:
				return -1
			case a.X0 > b.X0:
				return 1
			}
			return 0
		})
	}
	return dst
}

// InlineAt returns the innermost inline (an index into Paragraph.Inlines)
// that holds cluster i, or -1.
func (l *Layout) InlineAt(cluster int) int {
	if cluster < 0 || cluster >= len(l.cin) {
		return -1
	}
	return int(l.cin[cluster])
}
