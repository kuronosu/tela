package text

import (
	"math"
)

// This file holds hit-testing: the visual elements of a line (clusters,
// inline boxes and the ellipsis, left to right) and their caret stops.

// lineElemsBuf is the size of the stack buffers for the elements of one
// line: longer lines allocate.
const lineElemsBuf = 128

// velem is a visual element of a line: a visible cluster (text or inline
// box) or the ellipsis.
type velem struct {
	x0, x1 float32
	ci     int32 // cluster index, ciEllipsis for the ellipsis
	g      int32 // caret stops minus one: grapheme clusters, 1 for boxes and the ellipsis
	rtl    bool
	box    bool
}

// lineElems appends the visual elements of line li, left to right, to dst.
func (l *Layout) lineElems(li int, dst []velem) []velem {
	ln := &l.lines[li]
	for k := ln.i0; k < ln.i1; k++ {
		it := &l.items[k]
		if it.kind == itemEllipsis {
			x := ln.x + it.x
			dst = append(dst, velem{x0: x, x1: x + it.w, ci: ciEllipsis, g: 1, rtl: l.base == 1})
			continue
		}
		rtl := it.level%2 == 1
		add := func(c int32) {
			cl := &l.clusters[c]
			e := velem{x0: l.cx[c], x1: l.cx[c] + l.cadv[c], ci: c, g: int32(cl.graph), rtl: rtl, box: cl.flags&cfBox != 0}
			if e.box || e.g < 1 {
				e.g = 1
			}
			dst = append(dst, e)
		}
		if rtl {
			for c := it.c1 - 1; c >= it.c0; c-- {
				add(c)
			}
		} else {
			for c := it.c0; c < it.c1; c++ {
				add(c)
			}
		}
	}
	return dst
}

// stopX returns the x of caret stop s (0 at the left edge, e.g at the
// right edge) of element e.
func (e *velem) stopX(s int32) float32 {
	switch {
	case s <= 0:
		return e.x0
	case s >= e.g:
		return e.x1
	case e.rtl:
		return innerX(e.x0, e.x1-e.x0, e.g, e.g-s, true)
	}
	return innerX(e.x0, e.x1-e.x0, e.g, s, false)
}

// stopCursor returns the cursor that caret stop s of element e stands for
// (attached to e), and false when the stop has none: the start-side edge
// of the ellipsis, which belongs to the visible text next to it, and the
// logical end of white space removed at the end of a line.
func (l *Layout) stopCursor(li int, e *velem, s int32) (Cursor, bool) {
	if e.ci == ciEllipsis {
		if (s == 0) == e.rtl {
			return l.hiddenCursor(li)
		}
		return Cursor{}, false
	}
	if e.box {
		k := l.clusterText(e.ci)
		if (s == 0) != e.rtl {
			return l.canon(textPos{off: k}, Upstream), true
		}
		return l.canon(textPos{off: k}, Downstream), true
	}
	j := s
	if e.rtl {
		j = e.g - s
	}
	switch {
	case j <= 0:
		return l.slotCursor(e.ci, 0, Downstream), true
	case j >= e.g:
		if l.removedSpace(e.ci) {
			// No caret stop follows white space removed at the end of a
			// line (Chromium: the caret goes before it).
			return Cursor{}, false
		}
		return l.slotCursor(e.ci+1, 0, Upstream), true
	}
	return l.slotCursor(e.ci, j, Downstream), true
}

// hiddenCursor returns the cursor of the end-side edge of line li's
// ellipsis: the end of the line's content (before a final forced break),
// or the end of the text for the last line of a clamped paragraph, so that
// a selection that reaches the ellipsis holds the hidden text. When that
// position is not hidden (mixed bidi text truncated at its logical start),
// the start of the first hidden cluster of the line is used instead.
func (l *Layout) hiddenCursor(li int) (Cursor, bool) {
	ln := &l.lines[li]
	var cands [2]int
	if int32(li) == int32(len(l.lines)-1) && l.hiddenFrom < int32(len(l.clusters)) {
		cands[0] = len(l.src)
	} else {
		c := ln.c1
		for c > ln.c0 && l.clusters[c-1].flags&cfNewline != 0 {
			c--
		}
		cands[0] = l.clusterText(c)
	}
	cands[1] = -1
	for c := ln.c0; c < ln.c1; c++ {
		if l.cline[c] < 0 {
			cands[1] = l.clusterText(c)
			break
		}
	}
	for _, off := range cands {
		if off < 0 {
			continue
		}
		for _, a := range [...]Affinity{Downstream, Upstream} {
			p := textPos{off: off}
			if ci := l.caretFor(p, a); ci.ci == ciEllipsis && int(ci.line) == li {
				return l.canon(p, a), true
			}
		}
	}
	return Cursor{}, false
}

// stopAt reports whether cursor c shows its caret on line li at x (it is
// the canonical cursor of that caret stop).
func (l *Layout) stopAt(c Cursor, li int, x float32) bool {
	ci := l.caretFor(l.snap(c.Offset), c.Affinity)
	return int(ci.line) == li && float32(math.Abs(float64(ci.x-x))) <= stopEpsilon
}

// stopEpsilon is the tolerance when matching a caret with a caret stop:
// both come from the same cluster positions, up to the rounding of the
// even split of ligatures.
const stopEpsilon = 1.0 / 256

// HitTest returns the cursor for the point (x, y) in layout coordinates,
// for placing the caret on a click: the line at y (the first line for
// points above the text, the last one for points below), then the cluster
// under x in visual order (the first or last one for points beyond the
// line's ends), then the nearer of its edges (or of the grapheme
// boundaries inside a ligature), with the affinity that shows the caret at
// that edge. So the caret appears where the pointer is, including at bidi
// boundaries, where Chromium can show it at the other end of a run.
//
// A stop that no cursor shows on this line is replaced by the nearest one
// that does: past the end of a line that ends in a forced break or in
// collapsed white space, the caret goes before the break or the space (as
// in Chromium); next to an inline box at a line wrap, see Caret. Points on
// an ellipsis give the end of the hidden text on its end-side half
// (Hidden carets), the text before it on the other half.
func (l *Layout) HitTest(x, y float32) Cursor {
	return l.hitLine(l.lineAtY(y), x)
}

// hitLine hit-tests x on line li.
func (l *Layout) hitLine(li int, x float32) Cursor {
	if isNaN32(x) {
		x = 0
	}
	var buf [lineElemsBuf]velem
	elems := l.lineElems(li, buf[:0])
	if len(elems) == 0 {
		// An empty line: its only position.
		return l.canon(l.snap(l.clusterText(l.lines[li].c0)), Downstream)
	}
	e, s := l.nearestStop(elems, x)
	if c, ok := l.stopCursor(li, &elems[e], s); ok && l.stopAt(c, li, elems[e].stopX(s)) {
		return c
	}
	c, _ := l.representableNear(li, elems, e, x)
	return c
}

// nearestStop returns the element under x (the first one whose right edge
// is at or after x, the last one past the line's end) and its caret stop
// nearest to x (the left one on a tie).
func (l *Layout) nearestStop(elems []velem, x float32) (e int, s int32) {
	e = len(elems) - 1
	for i := range elems {
		if x <= elems[i].x1 {
			e = i
			break
		}
	}
	el := &elems[e]
	w := el.x1 - el.x0
	switch {
	case x <= el.x0:
		return e, 0
	case x >= el.x1 || !(w > 0):
		return e, el.g
	}
	t := float64(float32(float32((x-el.x0)/w) * float32(el.g)))
	s = int32(math.Ceil(t - 0.5))
	return e, min(max(s, 0), el.g)
}

// representableNear returns the cursor of the caret stop of line li
// nearest to x among those that a cursor shows on this line, preferring
// stops of element pref on ties. A line can have none: one that holds only
// inline boxes between two soft wraps at the same offset (a cursor shows
// the offset at the end of the previous line or at the start of the next
// one); then it returns the cursor of the nearest stop (before the boxes
// on their left half, after them on their right half) and false.
func (l *Layout) representableNear(li int, elems []velem, pref int, x float32) (Cursor, bool) {
	best, any := Cursor{}, Cursor{}
	bestD, anyD := float32(math.Inf(1)), float32(math.Inf(1))
	bestPref, found, foundAny := false, false, false
	for i := range elems {
		el := &elems[i]
		for s := int32(0); s <= el.g; s++ {
			sx := el.stopX(s)
			d := float32(math.Abs(float64(sx - x)))
			if d > bestD || (d == bestD && (bestPref || i != pref)) {
				continue
			}
			c, ok := l.stopCursor(li, el, s)
			if !ok {
				continue
			}
			if d < anyD {
				any, anyD, foundAny = c, d, true
			}
			if !l.stopAt(c, li, sx) {
				continue
			}
			best, bestD, bestPref, found = c, d, i == pref, true
		}
	}
	switch {
	case found:
		return best, true
	case foundAny:
		return any, false
	}
	return l.canon(l.snap(l.clusterText(l.lines[li].c0)), Downstream), false
}

// ClusterHit describes what is under a point (Layout.HitTestCluster).
type ClusterHit struct {
	// Cluster is the logical index of the cluster under the point, -1 for
	// the ellipsis of a truncated line.
	Cluster int
	// Line is the line under the point.
	Line int
	// Box is the index of the inline box (in Paragraph.Boxes) when the
	// cluster is one, else -1.
	Box int
	// Ellipsis reports that the point is on the ellipsis.
	Ellipsis bool
	// Leading reports that the point is on the leading half of the
	// cluster (the left half of left-to-right text, the right half of
	// right-to-left text).
	Leading bool
}

// HitTestCluster returns what is exactly under the point (x, y): the
// visible cluster, inline box or ellipsis whose box contains it, for
// pointer handling (hover, links, clicks on inline icons). A text cluster
// covers its advance and the height of its line box; an inline box covers
// its own rectangle (BoxRect). It returns false for points outside the
// text, between lines' content and the alignment edges, and on clusters
// without advance.
func (l *Layout) HitTestCluster(x, y float32) (ClusterHit, bool) {
	if !(y >= 0 && y < l.height) || isNaN32(x) {
		return ClusterHit{}, false
	}
	li := l.lineAtY(y)
	var buf [lineElemsBuf]velem
	for _, el := range l.lineElems(li, buf[:0]) {
		if !(x >= el.x0 && x < el.x1) {
			continue
		}
		hit := ClusterHit{Cluster: int(el.ci), Line: li, Box: -1}
		mid := el.x0 + float32((el.x1-el.x0)*0.5)
		hit.Leading = (x < mid) != el.rtl
		switch {
		case el.ci == ciEllipsis:
			hit.Cluster, hit.Ellipsis = -1, true
		case el.box:
			bi := int(l.runs[l.clusters[el.ci].run].box)
			r, ok := l.BoxRect(bi)
			if !ok || y < r.Y0 || y >= r.Y1 {
				return ClusterHit{}, false
			}
			hit.Box = bi
		}
		return hit, true
	}
	return ClusterHit{}, false
}
