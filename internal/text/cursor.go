package text

import (
	"math"
	"sort"

	"github.com/kuronosu/tela/internal/gfx"
)

// This file holds the cursor model of a Layout: positions (byte offset and
// affinity), how offsets snap to valid positions, which affinity is
// canonical, and caret geometry. See the "Cursors, selection and
// hit-testing" section of the package documentation. The design follows
// Parley's editing/cursor.rs (a cursor is a byte index and an affinity
// that attaches it to the cluster before or after it); no code was copied.

// Affinity tells which side of its offset a cursor is attached to. It only
// matters where one offset has two visual positions: at a soft line wrap
// (the end of a line or the start of the next one), at a bidi level change
// (the edge of the text before the offset or of the text after it) and next
// to inline boxes (before or after them).
type Affinity uint8

const (
	// Downstream attaches the cursor to what logically follows its offset:
	// the start of the next line at a soft wrap, the leading edge of the
	// following text at a bidi boundary, after the inline boxes at the
	// offset. It is the zero value and the canonical affinity wherever
	// both affinities look the same.
	Downstream Affinity = iota
	// Upstream attaches the cursor to what logically precedes its offset:
	// the end of the line at a soft wrap, the trailing edge of the
	// preceding text at a bidi boundary, before the inline boxes at the
	// offset.
	Upstream
)

// Cursor is a caret position in a Layout: a byte offset in the source text
// and an affinity. Methods that take a cursor snap it first (see
// SnapCursor), so a cursor kept from an older Layout of the same text, or
// computed by an editor after an edit, is always usable.
type Cursor struct {
	Offset   int
	Affinity Affinity
}

// textPos is a snapped position: a source offset that is a cluster
// boundary (j == 0) or a grapheme boundary inside cluster ci (a ligature,
// 0 < j < graphemes).
type textPos struct {
	off int
	ci  int32
	j   int32
}

// SnapCursor returns the valid, canonical cursor for c:
//
//   - The offset is clamped to the text and moved to a caret position: a
//     boundary between clusters, or between the grapheme clusters inside a
//     ligature (Cluster.Graphemes). An offset inside a grapheme cluster
//     (inside a UTF-8 sequence, or between a base and its combining mark)
//     moves back to the start of that grapheme cluster, except that an
//     offset in white space removed by white-space collapsing moves forward
//     to the end of it, where the visible text resumes (like Chromium,
//     which shows a caret inside collapsed spaces after the visible space).
//     Offsets before the first cluster (removed leading white space before
//     an inline box) move to its start.
//   - The affinity is Downstream unless Upstream puts the caret somewhere
//     else (see Affinity), so two cursors that show the same caret are
//     equal.
func (l *Layout) SnapCursor(c Cursor) Cursor {
	return l.canon(l.snap(c.Offset), c.Affinity)
}

// snap moves off to a caret position (see SnapCursor).
func (l *Layout) snap(off int) textPos {
	n := len(l.clusters)
	if n == 0 {
		return textPos{off: len(l.src)}
	}
	if first := l.clusterText(0); off <= first {
		return textPos{off: first}
	}
	if off >= len(l.src) {
		return textPos{off: len(l.src)}
	}
	ci := int32(l.ClusterAt(off))
	cs, ce := l.clusterRange(ci)
	if off <= cs {
		return textPos{off: cs}
	}
	// Inside the cluster: find the grapheme cluster that contains off.
	g := int32(l.clusters[ci].graph)
	lo := cs
	for j := int32(1); j <= g; j++ {
		hi := ce
		if j < g {
			hi = l.graphemeOffset(ci, j)
		}
		if off < hi {
			if collapsibleSpaces(l.src[off:hi]) {
				// Removed white space: the caret goes after it.
				if j == g {
					return textPos{off: ce}
				}
				return textPos{off: hi, ci: ci, j: j}
			}
			if j == 1 {
				return textPos{off: lo}
			}
			return textPos{off: lo, ci: ci, j: j - 1}
		}
		if off == hi {
			if j == g {
				return textPos{off: ce}
			}
			return textPos{off: hi, ci: ci, j: j}
		}
		lo = hi
	}
	return textPos{off: ce}
}

// collapsibleSpaces reports whether s is made of the characters that
// white-space collapsing removes (spaces, tabs, line feeds and carriage
// returns), and is not empty.
func collapsibleSpaces(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return true
}

// clusterRange returns the source byte range of cluster i (empty for an
// inline box).
func (l *Layout) clusterRange(i int32) (start, end int) {
	c := &l.clusters[i]
	start = int(l.text.src[c.r0])
	if c.flags&cfBox != 0 {
		return start, start
	}
	return start, int(l.text.src[c.r1])
}

// graphemeOffset returns the source offset of the j-th grapheme boundary
// inside cluster ci (0 < j < graphemes): the j-th processed rune boundary
// that starts a grapheme cluster and a new source character, the rule that
// shaping used to count Cluster.Graphemes.
func (l *Layout) graphemeOffset(ci, j int32) int {
	c := &l.clusters[ci]
	pt := &l.text
	k := int32(0)
	for p := c.r0 + 1; p < c.r1; p++ {
		if l.segs[p]&segGrapheme != 0 && pt.src[p] > pt.src[p-1] {
			k++
			if k == j {
				return int(pt.src[p])
			}
		}
	}
	_, end := l.clusterRange(ci)
	return end
}

// gapClusters returns, for a cluster boundary at offset k, the first
// cluster that starts at k (gF) and the first one after the inline boxes
// at k (gL; gF when there are none). Either is the number of clusters at
// the end of the text.
func (l *Layout) gapClusters(k int) (gF, gL int32) {
	n := len(l.clusters)
	g := sort.Search(n, func(i int) bool { return l.clusterText(int32(i)) >= k })
	gl := g
	for gl < n && l.clusters[gl].flags&cfBox != 0 && l.clusterText(int32(gl)) == k {
		gl++
	}
	return int32(g), int32(gl)
}

// caretInfo is the geometry of a caret and what it is attached to.
type caretInfo struct {
	line int32
	x    float32
	// ci is the cluster the caret is attached to; -1 when it is on an
	// empty line, ciEllipsis when it is in hidden text (at the ellipsis).
	ci int32
	// edge is 0 for the leading edge of ci, -1 for its trailing edge and j
	// for its j-th inner grapheme boundary counted from the leading edge.
	edge int32
	rtl  bool
}

const ciEllipsis = -2

// canon returns the canonical cursor of position p with affinity a:
// Downstream unless Upstream shows a different caret.
func (l *Layout) canon(p textPos, a Affinity) Cursor {
	if a != Upstream || p.j > 0 {
		return Cursor{p.off, Downstream}
	}
	up := l.caretFor(p, Upstream)
	down := l.caretFor(p, Downstream)
	if up.line == down.line && up.x == down.x {
		return Cursor{p.off, Downstream}
	}
	return Cursor{p.off, Upstream}
}

// caretFor computes the caret of position p with affinity a.
//
// Between clusters, an Upstream caret is at the trailing edge of the
// cluster before the offset and a Downstream caret at the leading edge of
// the cluster after it (after the inline boxes at the offset). There are
// two exceptions: nothing is attached after a forced break or after white
// space removed at the end of a line, so an Upstream caret there is at the
// start of the next line (Chromium does the same: no caret position
// follows the trailing space of a wrapped line); and at the ends of the
// text, the missing side is replaced by the other one.
func (l *Layout) caretFor(p textPos, a Affinity) caretInfo {
	if p.j > 0 {
		return l.attach(p.ci, p.j)
	}
	n := int32(len(l.clusters))
	gF, gL := l.gapClusters(p.off)
	if a == Upstream {
		switch {
		case gF > 0 && !l.breaksAfter(gF-1):
			return l.attach(gF-1, -1)
		case gF < n:
			return l.attach(gF, 0)
		}
		return l.endCaret()
	}
	switch {
	case gL < n:
		return l.attach(gL, 0)
	case gL > gF:
		return l.attach(gL-1, -1)
	}
	return l.endCaret()
}

// breaksAfter reports whether the position after cluster c belongs to the
// next line whatever its affinity: c is a forced break, or white space
// removed at the end of a line that another line follows.
func (l *Layout) breaksAfter(c int32) bool {
	f := l.clusters[c].flags
	if f&cfNewline != 0 {
		return true
	}
	if !l.removed(f) {
		return false
	}
	li := l.cline[c]
	if li < 0 || int(li)+1 >= len(l.lines) {
		return false
	}
	ln := &l.lines[li]
	return c >= ln.trail && c == ln.c1-1
}

// endCaret is the caret at the end of the text.
func (l *Layout) endCaret() caretInfo {
	n := int32(len(l.clusters))
	if n > 0 {
		if l.cline[n-1] < 0 {
			return l.hiddenCaret(n - 1)
		}
		if l.clusters[n-1].flags&cfNewline == 0 {
			return l.attach(n-1, -1)
		}
	}
	// Empty text, or a final forced break: the empty last line.
	li := int32(len(l.lines) - 1)
	return caretInfo{line: li, x: l.lines[li].x, ci: -1, rtl: l.base == 1}
}

// attach returns the caret at an edge of cluster ci (see caretInfo.edge).
func (l *Layout) attach(ci, edge int32) caretInfo {
	li := l.cline[ci]
	if li < 0 {
		return l.hiddenCaret(ci)
	}
	rtl := l.clusterLevel(ci)%2 == 1
	x0, w := l.cx[ci], l.cadv[ci]
	var x float32
	switch {
	case edge == 0:
		x = x0
		if rtl {
			x = x0 + w
		}
	case edge < 0:
		x = x0 + w
		if rtl {
			x = x0
		}
	default:
		x = innerX(x0, w, int32(l.clusters[ci].graph), edge, rtl)
	}
	return caretInfo{line: li, x: x, ci: ci, edge: edge, rtl: rtl}
}

// innerX returns the x of the j-th grapheme boundary (from the leading
// edge) of a cluster at x0 with advance w and g grapheme clusters: the
// advance is split evenly, like Parley and Chromium do for ligatures.
func innerX(x0, w float32, g, j int32, rtl bool) float32 {
	d := float32(float32(w*float32(j)) / float32(g))
	if rtl {
		return x0 + w - d
	}
	return x0 + d
}

// clusterLevel returns the bidi level of visible cluster ci on its line
// (rule L1 included), or its run's level when it is hidden.
func (l *Layout) clusterLevel(ci int32) uint8 {
	li := l.cline[ci]
	if li < 0 {
		return l.runs[l.clusters[ci].run].level
	}
	ln := &l.lines[li]
	bidiTrail := ln.c1
	for bidiTrail > ln.c0 && l.clusters[bidiTrail-1].flags&cfBidiWS != 0 {
		bidiTrail--
	}
	return l.level(ci, bidiTrail)
}

// hiddenLine returns the line that hidden cluster ci was cut from: the
// line truncated by text-overflow, or the last line when line-clamp hid it.
func (l *Layout) hiddenLine(ci int32) int32 {
	if ci >= l.hiddenFrom {
		return int32(len(l.lines) - 1)
	}
	i := sort.Search(len(l.lines), func(i int) bool { return l.lines[i].c1 > ci })
	return int32(min(i, len(l.lines)-1))
}

// hiddenCaret is the caret of a position in hidden text: the end-side
// edge of the ellipsis of the line it was cut from (the right edge in a
// left-to-right paragraph), as if the ellipsis held the hidden text.
func (l *Layout) hiddenCaret(ci int32) caretInfo {
	li := l.hiddenLine(ci)
	return caretInfo{line: li, x: l.ellipsisEnd(li), ci: ciEllipsis, rtl: l.base == 1}
}

// ellipsisEnd returns the x of the end-side edge of line li's ellipsis, or
// of the line's content when it has none.
func (l *Layout) ellipsisEnd(li int32) float32 {
	ln := &l.lines[li]
	for k := ln.i0; k < ln.i1; k++ {
		if it := &l.items[k]; it.kind == itemEllipsis {
			if l.base == 1 {
				return ln.x + it.x
			}
			return ln.x + it.x + it.w
		}
	}
	if l.base == 1 {
		return ln.x
	}
	return ln.x + ln.width + ln.hang
}

// Caret is the geometry of a cursor (Layout.Caret).
type Caret struct {
	// X is the caret's position on the line (layout coordinates).
	X float32
	// Line is the line the caret is on.
	Line int
	// Top and Height are the line box (what Chromium's caret and
	// selection cover); Baseline is the y of the line's baseline.
	Top, Height, Baseline float32
	// Ascent and Descent are those of the font of the text the caret is
	// attached to (rounded to whole px like line boxes), for a caret sized
	// to the text instead of the line.
	Ascent, Descent float32
	// RTL reports that the caret is attached to right-to-left text (for a
	// direction marker, as some platforms draw at bidi boundaries).
	RTL bool
	// Hidden reports a position in text hidden by text-overflow or
	// line-clamp: the caret is at the end-side edge of the ellipsis.
	Hidden bool
}

// Caret returns the geometry of cursor c (snapped first, see SnapCursor).
//
// Between two clusters, the caret is at the trailing edge of the cluster
// before the offset when the affinity is Upstream, and at the leading edge
// of the cluster after it when it is Downstream (the left edge of
// left-to-right text, the right edge of right-to-left text). This is
// Parley's rule: at a bidi boundary, the affinity picks which of the two
// visual positions shows the caret, and hit-testing (HitTest) returns the
// affinity that puts the caret where the pointer is. Inside a ligature,
// the caret splits the cluster's advance evenly among its grapheme
// clusters. Inline boxes sit between the two affinities: Upstream is
// before the boxes at an offset, Downstream after them. A position after
// a forced break, or after white space removed at a soft wrap, is at the
// start of the next line whatever its affinity; the end of a text that
// ends with a forced break is on the empty last line. Positions in text
// hidden by text-overflow or line-clamp are at the end-side edge of the
// ellipsis (Hidden).
func (l *Layout) Caret(c Cursor) Caret {
	p := l.snap(c.Offset)
	return l.caretOut(l.caretFor(p, c.Affinity))
}

func (l *Layout) caretOut(ci caretInfo) Caret {
	ln := &l.lines[ci.line]
	out := Caret{
		X: ci.x, Line: int(ci.line), Top: ln.top, Height: ln.height, Baseline: ln.baseline,
		RTL: ci.rtl, Hidden: ci.ci == ciEllipsis,
	}
	switch {
	case ci.ci >= 0:
		r := &l.runs[l.clusters[ci.ci].run]
		out.Ascent, out.Descent = r.ascent, r.descent
		if r.kind == runBox {
			si := &l.sinfo[l.clusters[ci.ci].style]
			out.Ascent, out.Descent = si.ascent, si.descent
		}
	case ci.ci == ciEllipsis && ln.ellipsis >= 0:
		si := &l.sinfo[l.ellipses[ln.ellipsis].style]
		out.Ascent, out.Descent = si.ascent, si.descent
	default:
		out.Ascent, out.Descent = l.sinfo[0].ascent, l.sinfo[0].descent
	}
	return out
}

// CursorRect returns the caret of cursor c as a rectangle of the given
// width covering the line box: from X to X+width for a caret attached to
// left-to-right text, from X-width to X for right-to-left text, so that
// the caret lies over the text it belongs to.
func (l *Layout) CursorRect(c Cursor, width float32) gfx.Rect {
	ca := l.Caret(c)
	x0 := ca.X
	if ca.RTL {
		x0 -= width
	}
	return gfx.Rect{X0: x0, Y0: ca.Top, X1: x0 + width, Y1: ca.Top + ca.Height}
}

// lineAtY returns the line at y: the first line above it, the last line
// below it.
func (l *Layout) lineAtY(y float32) int {
	if !(y >= 0) { // also NaN
		return 0
	}
	i := sort.Search(len(l.lines), func(i int) bool { return l.lines[i].top+l.lines[i].height > y })
	return min(i, len(l.lines)-1)
}

// slotCursor returns the canonical cursor for a logical position given as
// a cluster boundary (ci, j): before cluster ci (j == 0, with the given
// affinity; the end of the text for ci == number of clusters) or at its
// j-th inner grapheme boundary.
func (l *Layout) slotCursor(ci, j int32, a Affinity) Cursor {
	if ci >= int32(len(l.clusters)) {
		return l.canon(textPos{off: len(l.src)}, a)
	}
	start, _ := l.clusterRange(ci)
	if j == 0 {
		return l.canon(textPos{off: start}, a)
	}
	return Cursor{l.graphemeOffset(ci, j), Downstream}
}

// isNaN32 reports whether v is NaN.
func isNaN32(v float32) bool { return math.IsNaN(float64(v)) }
