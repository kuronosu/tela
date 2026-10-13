package text

import (
	"sort"

	"github.com/kuronosu/tela/internal/gfx"
)

// This file holds the read-only view of a Layout: sizes, lines, the visual
// runs of a line, clusters (the unit of cursor positions, selection and
// hit-testing) and inline box positions. Coordinates are logical px with
// the origin at the top-left corner of the layout box (the point given to
// Paint), y down.

// Width returns the width of the widest line, without hanging white space.
func (l *Layout) Width() float32 { return l.width }

// Height returns the height of the visible lines.
func (l *Layout) Height() float32 { return l.height }

// MaxWidth returns the width the lines were broken at (+Inf for none).
func (l *Layout) MaxWidth() float32 { return l.maxWidth }

// AlignWidth returns the width of the box the lines are aligned in.
func (l *Layout) AlignWidth() float32 { return l.alignWidth }

// MinContentWidth returns the min-content width: the widest piece of text
// between soft wrap opportunities (and emergency opportunities with
// overflow-wrap: anywhere), without hanging white space. It does not
// depend on the current line breaking. After SetBoxIntrinsic it computes
// the widths again, so it then must not run concurrently with other
// methods (nor must MaxContentWidth).
func (l *Layout) MinContentWidth() float32 { l.intrinsic(); return l.minContent }

// MaxContentWidth returns the max-content width: the widest line when
// breaking only at forced breaks.
func (l *Layout) MaxContentWidth() float32 { l.intrinsic(); return l.maxContent }

// Text returns the source text.
func (l *Layout) Text() string { return l.src }

// RTL reports whether the paragraph's base direction is right to left.
func (l *Layout) RTL() bool { return l.base == 1 }

// ClampOffset returns the source offset where the text that line-clamp
// hides starts, or the length of the text when nothing is clamped.
func (l *Layout) ClampOffset() int { return l.clusterText(l.hiddenFrom) }

// Line describes one visible line.
type Line struct {
	// TextStart and TextEnd are the source bytes of the line's clusters,
	// trailing white space included (content hidden by an ellipsis too).
	TextStart, TextEnd int
	// ClusterStart and ClusterEnd are the line's clusters (logical order).
	ClusterStart, ClusterEnd int
	// Top and Height are the line box; Baseline is the y of the baseline,
	// Ascent and Descent the line box above and below it.
	Top, Height, Baseline, Ascent, Descent float32
	// X is the left edge of the line's visual content (hanging white space
	// included when it is on the left, in right-to-left paragraphs), Width
	// its advance without hanging white space, Hanging the advance of the
	// hanging white space.
	X, Width, Hanging float32
	Break             BreakReason
	// Ellipsis reports a truncated line (text-overflow or line-clamp).
	Ellipsis bool
}

// NumLines returns the number of visible lines (at least one).
func (l *Layout) NumLines() int { return len(l.lines) }

// Line returns line i.
func (l *Layout) Line(i int) Line {
	ln := &l.lines[i]
	return Line{
		TextStart: l.clusterText(ln.c0), TextEnd: l.clusterText(ln.c1),
		ClusterStart: int(ln.c0), ClusterEnd: int(ln.c1),
		Top: ln.top, Height: ln.height, Baseline: ln.baseline, Ascent: ln.ascent, Descent: ln.descent,
		X: ln.x, Width: ln.width, Hanging: ln.hang, Break: ln.reason, Ellipsis: ln.ellipsis >= 0,
	}
}

// clusterText returns the source offset where cluster i starts (or the end
// of the text for i == number of clusters).
func (l *Layout) clusterText(i int32) int {
	if int(i) >= len(l.clusters) {
		return len(l.src)
	}
	return int(l.text.src[l.clusters[i].r0])
}

// RunKind is the kind of a LineRun.
type RunKind uint8

const (
	// RunText is text (or a tab or forced break, which have no glyphs).
	RunText RunKind = iota
	// RunBox is an inline box.
	RunBox
	// RunEllipsis is the ellipsis of a truncated line.
	RunEllipsis
)

// LineRun is a visual piece of a line: clusters of one font at one bidi
// level, an inline box or an ellipsis.
type LineRun struct {
	Kind RunKind
	// Level is the bidi embedding level: odd levels are right to left.
	Level uint8
	// X and Width place the run on the line, in layout coordinates.
	X, Width float32
	// ClusterStart and ClusterEnd are its visible clusters, in logical
	// order (both -1 for an ellipsis). Right-to-left runs show them from
	// right to left.
	ClusterStart, ClusterEnd int
	Face                     *Face
	Size                     float32
	// Box is the index of the inline box in Paragraph.Boxes for RunBox, -1
	// for the zero-width placeholder of an empty inline.
	Box int
}

// LineRuns appends the runs of line i in visual order (left to right) to
// dst.
func (l *Layout) LineRuns(i int, dst []LineRun) []LineRun {
	ln := &l.lines[i]
	for k := ln.i0; k < ln.i1; k++ {
		it := &l.items[k]
		r := LineRun{Level: it.level, X: ln.x + it.x, Width: it.w, ClusterStart: int(it.c0), ClusterEnd: int(it.c1), Box: -1}
		switch it.kind {
		case itemEllipsis:
			e := &l.ellipses[it.run]
			r.Kind, r.Face, r.Size = RunEllipsis, e.face, e.size
		case itemBox:
			rn := &l.runs[it.run]
			r.Kind, r.Box = RunBox, int(rn.box)
			if r.Box >= l.nboxes {
				r.Box = -1 // the placeholder of an empty inline
			}
		default:
			rn := &l.runs[it.run]
			r.Kind, r.Face, r.Size = RunText, rn.face, rn.size
		}
		dst = append(dst, r)
	}
	return dst
}

// ClusterFlags describe a cluster.
type ClusterFlags uint16

const (
	// ClusterSpace is white space: a space, a tab or another space
	// separator.
	ClusterSpace ClusterFlags = 1 << iota
	// ClusterTab is a preserved tab.
	ClusterTab
	// ClusterNewline is a forced break character (a preserved newline).
	ClusterNewline
	// ClusterBox is an inline box.
	ClusterBox
	// ClusterWordSeparator is a word separator (CSS Text 3, section 7.1).
	ClusterWordSeparator
	// ClusterSoftBreak marks a soft wrap opportunity before the cluster.
	ClusterSoftBreak
	// ClusterHanging is trailing white space that hangs past the end of
	// its line or was removed (zero advance).
	ClusterHanging
)

// Cluster is the smallest unit of text that layout positions: one or more
// grapheme clusters that shaping joined (a ligature), never split across
// lines, by an ellipsis or by bidi reordering.
type Cluster struct {
	// TextStart and TextEnd are its source bytes. Characters removed by
	// white-space collapsing belong to the cluster before them; an inline
	// box has an empty range at its offset.
	TextStart, TextEnd int
	// GlyphStart and GlyphEnd index its glyphs (see Glyph), in visual
	// order.
	GlyphStart, GlyphEnd int
	// Line is the line it is on, -1 when it is hidden (truncated or
	// clamped).
	Line int
	// X is its left edge and Advance its width on the line (justification
	// included; removed white space has zero advance).
	X, Advance float32
	// Level is its bidi embedding level on the line (rule L1 included).
	Level uint8
	// Graphemes is the number of grapheme clusters it holds (more than one
	// for ligatures), for placing cursors inside it.
	Graphemes int
	Flags     ClusterFlags
}

// NumClusters returns the number of clusters, visible or not.
func (l *Layout) NumClusters() int { return len(l.clusters) }

// Cluster returns cluster i (logical order).
func (l *Layout) Cluster(i int) Cluster {
	c := &l.clusters[i]
	out := Cluster{
		TextStart: int(l.text.src[c.r0]), TextEnd: int(l.text.src[c.r1]),
		GlyphStart: int(c.g0), GlyphEnd: int(c.g1),
		Line: -1, Level: l.runs[c.run].level, Graphemes: int(c.graph),
	}
	if c.flags&cfBox != 0 {
		out.TextEnd = out.TextStart
	}
	if c.flags&(cfSpace|cfTab|cfOtherSpace) != 0 {
		out.Flags |= ClusterSpace
	}
	for _, m := range [...]struct {
		f clusterFlag
		o ClusterFlags
	}{{cfTab, ClusterTab}, {cfNewline, ClusterNewline}, {cfBox, ClusterBox}, {cfWordSep, ClusterWordSeparator}, {cfSoftBefore, ClusterSoftBreak}} {
		if c.flags&m.f != 0 {
			out.Flags |= m.o
		}
	}
	if li := l.cline[i]; li >= 0 {
		ln := &l.lines[li]
		out.Line = int(li)
		out.X, out.Advance = l.cx[i], l.cadv[i]
		bidiTrail := ln.c1
		for bidiTrail > ln.c0 && l.clusters[bidiTrail-1].flags&cfBidiWS != 0 {
			bidiTrail--
		}
		out.Level = l.level(int32(i), bidiTrail)
		if int32(i) >= ln.trail {
			out.Flags |= ClusterHanging
		}
	}
	return out
}

// ClusterAt returns the cluster whose source range contains the byte at
// offset (the first cluster for offsets before it, NumClusters() for the
// end of the text).
func (l *Layout) ClusterAt(offset int) int {
	n := len(l.clusters)
	if offset >= len(l.src) {
		return n
	}
	i := sort.Search(n, func(i int) bool { return int(l.text.src[l.clusters[i].r1]) > offset })
	return i
}

// Glyph is a shaped glyph: its id in the run's face, its advance and its
// offset from the pen position (px, y down).
type Glyph struct {
	ID               gfx.GlyphID
	Advance          float32
	XOffset, YOffset float32
}

// Glyph returns glyph i.
func (l *Layout) Glyph(i int) Glyph {
	g := &l.glyphs[i]
	return Glyph{g.id, g.adv, g.dx, g.dy}
}

// BoxLine returns the line that Paragraph.Boxes[i] is on, or -1 when it is
// hidden (truncated or clamped).
func (l *Layout) BoxLine(i int) int {
	if i < 0 || i >= l.nboxes {
		return -1
	}
	c := l.boxClus[i]
	if c < 0 {
		return -1
	}
	return int(l.cline[c])
}

// BoxRect returns the rectangle of Paragraph.Boxes[i], or false when the box
// is hidden (truncated or clamped).
func (l *Layout) BoxRect(i int) (gfx.Rect, bool) {
	if i < 0 || i >= l.nboxes {
		return gfx.Rect{}, false
	}
	c := l.boxClus[i]
	if c < 0 || l.cline[c] < 0 {
		return gfx.Rect{}, false
	}
	b := &l.boxes[i]
	ln := &l.lines[l.cline[c]]
	h := max(0, b.Height)
	var top float32
	switch b.Align {
	case BoxTop:
		top = ln.top
	case BoxBottom:
		top = ln.top + ln.height - h
	default:
		over, _ := boxExtents(b, &l.sinfo[l.clusters[c].style])
		top = ln.baseline - over
	}
	x := l.cx[c]
	return gfx.Rect{X0: x, Y0: top, X1: x + b.Width, Y1: top + h}, true
}
