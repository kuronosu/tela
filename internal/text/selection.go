package text

import (
	"unicode"

	"github.com/kuronosu/tela/internal/gfx"
)

// This file holds selections: ordering of cursors, selection rectangles,
// the selected text, and word, line and paragraph selections from a point
// (double and triple clicks) with drag extension by the same unit, after
// Parley's editing/selection.rs (no code was copied).

// selUnit is the unit a selection was made with, kept for drag extension.
type selUnit uint8

const (
	unitGrapheme selUnit = iota
	unitWord
	unitLine
	unitParagraph
)

// Selection is a range of text between two cursors: the anchor, where the
// selection started, and the focus, which moves (shift+arrow, drag). When
// they are equal the selection is collapsed to a caret. A Selection also
// remembers the horizontal position that vertical movement keeps (the
// "goal column", in px) and, for selections made by word, line or
// paragraph, that unit, for ExtendSelection.
//
// The zero value is a caret at the start of the text.
type Selection struct {
	Anchor, Focus Cursor

	goalX              float32
	hasGoal            bool
	unit               selUnit
	unitStart, unitEnd Cursor // the unit first selected, for unit drags
}

// NewSelection returns the selection from anchor to focus.
func NewSelection(anchor, focus Cursor) Selection {
	return Selection{Anchor: anchor, Focus: focus}
}

// CaretSelection returns the selection collapsed at c.
func CaretSelection(c Cursor) Selection { return Selection{Anchor: c, Focus: c} }

// IsCollapsed reports whether the selection is a caret (anchor and focus
// are the same cursor).
func (s Selection) IsCollapsed() bool { return s.Anchor == s.Focus }

// TextRange returns the selected bytes of the source text: from the
// smaller offset of anchor and focus to the larger one. Selections that
// Layout methods return hold snapped cursors; for others, SelectedText
// snaps first. The range can be empty for a selection that is not
// collapsed: one that holds only inline boxes (from before them to after
// them), or the two carets of a soft wrap or a bidi boundary.
func (s Selection) TextRange() (start, end int) {
	a, b := s.Anchor.Offset, s.Focus.Offset
	if a > b {
		a, b = b, a
	}
	return a, b
}

// GoalX returns the horizontal position that vertical movement keeps, and
// whether there is one (only after MoveUp, MoveDown and MovePage).
func (s Selection) GoalX() (float32, bool) { return s.goalX, s.hasGoal }

// before reports whether cursor a comes before b in logical order: by
// offset, then Upstream (before the inline boxes at the offset, or the end
// of a line) before Downstream.
func before(a, b Cursor) bool {
	if a.Offset != b.Offset {
		return a.Offset < b.Offset
	}
	return a.Affinity == Upstream && b.Affinity == Downstream
}

// ordered returns the snapped ends of s, the logically first one first.
func (l *Layout) ordered(s Selection) (start, end Cursor) {
	a, f := l.SnapCursor(s.Anchor), l.SnapCursor(s.Focus)
	if before(f, a) {
		return f, a
	}
	return a, f
}

// SelectedText returns the source text of the selection (TextRange of its
// snapped cursors).
func (l *Layout) SelectedText(s Selection) string {
	a, b := l.ordered(s)
	return l.src[a.Offset:b.Offset]
}

// SelectionRects appends the rectangles that highlight the selection to
// dst and returns it: one rectangle per run of visually adjacent selected
// content on a line, lines from top to bottom and rectangles left to right
// within a line, each as tall as its line box. Bidi text splits a logical
// range into several rectangles. Parts of ligatures are selected by
// grapheme cluster (the advance is split evenly). An inline box is
// selected when the selection covers it (see Affinity). The ellipsis is
// highlighted when the selection holds any of the text it hides.
//
// When the selection goes on past the end of a line (to a later line),
// the line's highlight is extended by the width of a space at its end edge
// (after the content and hanging white space: on the right in a
// left-to-right paragraph, on the left in a right-to-left one), for soft
// wraps and forced breaks alike. This is how Chromium paints selected line
// breaks; the last line of the paragraph is never extended. The rectangles
// do not overlap.
func (l *Layout) SelectionRects(s Selection, dst []gfx.Rect) []gfx.Rect {
	return l.selectionRects(s, dst, true)
}

func (l *Layout) selectionRects(s Selection, dst []gfx.Rect, lineEnds bool) []gfx.Rect {
	a, b := l.ordered(s)
	if a == b {
		return dst
	}
	pa := l.snap(a.Offset)
	pb := l.snap(b.Offset)
	la := l.caretFor(pa, a.Affinity).line
	lb := l.caretFor(pb, b.Affinity).line
	if la > lb {
		la, lb = lb, la
	}
	var buf [lineElemsBuf]velem
	var spans [16][2]float32
	for li := la; li <= lb; li++ {
		ln := &l.lines[li]
		elems := l.lineElems(int(li), buf[:0])
		iv := spans[:0]
		add := func(x0, x1 float32) {
			if !(x1 > x0) {
				return
			}
			if n := len(iv); n > 0 && x0 <= iv[n-1][1]+stopEpsilon {
				iv[n-1][1] = max(iv[n-1][1], x1)
				return
			}
			iv = append(iv, [2]float32{x0, x1})
		}
		// When the selection goes on past this line, its break is
		// highlighted at the end edge.
		brk := float32(0)
		if lineEnds && li < lb && a.Offset < b.Offset {
			brk = l.breakWidth(int(li))
		}
		if l.base == 1 {
			add(ln.x-brk, ln.x)
		}
		for i := range elems {
			el := &elems[i]
			switch {
			case el.ci == ciEllipsis:
				if l.selectsHidden(int(li), a, b) {
					add(el.x0, el.x1)
				}
			case el.box:
				k := l.clusterText(el.ci)
				if !before(Cursor{k, Upstream}, a) && !before(b, Cursor{k, Downstream}) {
					add(el.x0, el.x1)
				}
			default:
				j0, j1 := l.selectedGraphemes(el.ci, a.Offset, b.Offset)
				if j0 < j1 {
					if el.rtl {
						add(el.stopX(el.g-j1), el.stopX(el.g-j0))
					} else {
						add(el.stopX(j0), el.stopX(j1))
					}
				}
			}
		}
		if l.base == 0 {
			end := ln.x + ln.width + ln.hang
			add(end, end+brk)
		}
		for _, v := range iv {
			dst = append(dst, gfx.Rect{X0: v[0], Y0: ln.top, X1: v[1], Y1: ln.top + ln.height})
		}
	}
	return dst
}

// selectedGraphemes returns the range [j0, j1) of the grapheme clusters of
// text cluster ci (counted from its logical start) inside the bytes
// [a, b).
func (l *Layout) selectedGraphemes(ci int32, a, b int) (j0, j1 int32) {
	cs, ce := l.clusterRange(ci)
	if ce <= a || cs >= b {
		return 0, 0
	}
	g := int32(l.clusters[ci].graph)
	if g <= 1 || (a <= cs && ce <= b) {
		if a <= cs && ce <= b {
			return 0, max(g, 1)
		}
		return 0, 0
	}
	j0, j1 = g, 0
	lo := cs
	for j := int32(0); j < g; j++ {
		hi := ce
		if j+1 < g {
			hi = l.graphemeOffset(ci, j+1)
		}
		if a <= lo && hi <= b {
			j0, j1 = min(j0, j), max(j1, j+1)
		}
		lo = hi
	}
	return j0, j1
}

// selectsHidden reports whether the selection [a, b) holds text hidden on
// line li (by its ellipsis, and by line-clamp for the last line).
func (l *Layout) selectsHidden(li int, a, b Cursor) bool {
	ln := &l.lines[li]
	hit := func(c int32) bool {
		cs, ce := l.clusterRange(c)
		if l.clusters[c].flags&cfBox != 0 {
			return !before(Cursor{cs, Upstream}, a) && !before(b, Cursor{cs, Downstream})
		}
		return cs < b.Offset && ce > a.Offset
	}
	for c := ln.c0; c < ln.c1; c++ {
		if l.cline[c] < 0 && hit(c) {
			return true
		}
	}
	if li == len(l.lines)-1 {
		for c := l.hiddenFrom; c < int32(len(l.clusters)); c++ {
			if hit(c) {
				return true
			}
			if cs, _ := l.clusterRange(c); cs >= b.Offset {
				break
			}
		}
	}
	return false
}

// breakWidth returns the width of the highlight of line li's break: a
// space in the first available font of the style of the line's last
// cluster.
func (l *Layout) breakWidth(li int) float32 {
	ln := &l.lines[li]
	st := uint16(0)
	if ln.c1 > ln.c0 {
		st = l.clusters[ln.c1-1].style
	}
	si := &l.sinfo[st]
	return spaceAdvance(si.face, si.size)
}

// SelectAll returns the selection of the whole text.
func (l *Layout) SelectAll() Selection {
	return NewSelection(l.docStart(), l.docEnd())
}

// docStart and docEnd are the ends of the text: before inline boxes that
// begin it, after those that end it.
func (l *Layout) docStart() Cursor { return l.canon(l.snap(0), Upstream) }
func (l *Layout) docEnd() Cursor   { return l.canon(l.snap(len(l.src)), Downstream) }

// SelectWordAt returns the selection of the word under the point (x, y),
// for a double click: the UAX #29 word (letters, digits and what joins
// them, such as "don't" or "3.14") of the cluster nearest to the point on
// its line, or the run of white space, or of other characters
// (punctuation, symbols, emoji), between two words. On an inline box it
// selects the box. On an ellipsis it selects the hidden text.
func (l *Layout) SelectWordAt(x, y float32) Selection {
	start, end := l.wordAt(x, y)
	return Selection{Anchor: start, Focus: end, unit: unitWord, unitStart: start, unitEnd: end}
}

// wordAt returns the ends of the word under (x, y) (see SelectWordAt).
func (l *Layout) wordAt(x, y float32) (start, end Cursor) {
	li := l.lineAtY(y)
	var buf [lineElemsBuf]velem
	elems := l.lineElems(li, buf[:0])
	if len(elems) == 0 {
		c := l.hitLine(li, x)
		return c, c
	}
	e, _ := l.nearestStop(elems, x)
	// Prefer an element with an advance: past the end of a line, the
	// trailing break or collapsed space is not what the user points at.
	for e > 0 && !(elems[e].x1 > elems[e].x0) && x >= elems[e].x0 {
		e--
	}
	el := &elems[e]
	switch {
	case el.ci == ciEllipsis:
		// From the visible text next to the ellipsis to the end of the
		// hidden text.
		var twin Cursor
		ok := false
		if el.rtl && e+1 < len(elems) {
			twin, ok = l.stopCursor(li, &elems[e+1], 0)
		} else if !el.rtl && e > 0 {
			twin, ok = l.stopCursor(li, &elems[e-1], elems[e-1].g)
		}
		h, hok := l.hiddenCursor(li)
		if !ok || !hok {
			c := l.hitLine(li, x)
			return c, c
		}
		if before(h, twin) {
			return h, twin
		}
		return twin, h
	case el.box:
		k := l.clusterText(el.ci)
		return l.canon(textPos{off: k}, Upstream), l.canon(textPos{off: k}, Downstream)
	}
	r := l.clusters[el.ci].r0
	s, t := l.segmentAt(r)
	return l.runeCursor(s, Downstream), l.runeCursor(t, Upstream)
}

// segmentAt returns the processed runes [s, t) of the segment that holds
// rune r: the word (segWordStart to segWordEnd) that contains it, or the
// maximal run of white space, or of other characters, between words.
func (l *Layout) segmentAt(r int32) (s, t int32) {
	n := int32(len(l.text.runes))
	segs := l.segs
	// Rune r is in a word when the nearest word boundary at or before it
	// is a word start.
	for i := r; i >= 0; i-- {
		if segs[i]&segWordStart != 0 {
			t = i + 1
			for t < n && segs[t]&segWordEnd == 0 {
				t++
			}
			return i, t
		}
		if segs[i]&segWordEnd != 0 {
			break
		}
	}
	space := l.isSpaceRune(r)
	s, t = r, r+1
	for s > 0 && segs[s]&(segWordEnd|segWordStart) == 0 && l.isSpaceRune(s-1) == space {
		s--
	}
	for t < n && segs[t]&(segWordStart|segWordEnd) == 0 && l.isSpaceRune(t) == space {
		t++
	}
	return s, t
}

func (l *Layout) isSpaceRune(r int32) bool {
	return l.text.box[r] < 0 && unicode.IsSpace(l.text.runes[r])
}

// runeCursor returns the cursor at the boundary before processed rune r.
func (l *Layout) runeCursor(r int32, a Affinity) Cursor {
	off := len(l.src)
	if int(r) < len(l.text.runes) {
		off = int(l.text.src[r])
	}
	return l.canon(l.snap(off), a)
}

// SelectLineAt returns the selection of the visual line at y, for a
// triple click in editors that select lines: from the line's start to its
// end (MoveLineStart and MoveLineEnd, so a final forced break or collapsed
// space is not included).
func (l *Layout) SelectLineAt(x, y float32) Selection {
	_ = x
	li := l.lineAtY(y)
	start, end := l.lineStart(li), l.lineEnd(li)
	return Selection{Anchor: start, Focus: end, unit: unitLine, unitStart: start, unitEnd: end}
}

// SelectParagraphAt returns the selection of the paragraph under the point
// (x, y), for a triple click as browsers do it: the text between forced
// breaks that holds the hit position, including the forced break that
// ends it.
func (l *Layout) SelectParagraphAt(x, y float32) Selection {
	start, end := l.paragraphAt(l.HitTest(x, y))
	return Selection{Anchor: start, Focus: end, unit: unitParagraph, unitStart: start, unitEnd: end}
}

// paragraphAt returns the ends of the paragraph (text between forced
// breaks, the final break included) that holds c.
func (l *Layout) paragraphAt(c Cursor) (start, end Cursor) {
	n := int32(len(l.clusters))
	// The cluster that holds the position: paragraphs only change after
	// forced breaks, so the cluster after the offset (after its inline
	// boxes) is in the same paragraph as the position whatever the
	// affinity.
	p := l.snap(c.Offset)
	ci := p.ci
	if p.j == 0 {
		_, ci = l.gapClusters(p.off)
	}
	s := ci
	for s > 0 && l.clusters[s-1].flags&cfNewline == 0 {
		s--
	}
	e := ci
	for e < n && l.clusters[e].flags&cfNewline == 0 {
		e++
	}
	if e < n {
		e++ // the forced break that ends the paragraph
	}
	// Upstream puts the start before inline boxes that begin the
	// paragraph; at the end of the text, Downstream puts the end after
	// final boxes, and after a forced break Upstream keeps the boxes that
	// begin the next paragraph out.
	start = l.slotCursor(s, 0, Upstream)
	if e == n {
		end = l.slotCursor(e, 0, Downstream)
	} else {
		end = l.slotCursor(e, 0, Upstream)
	}
	return start, end
}

// ExtendSelection moves the focus of s to the point (x, y), for a drag:
// to the caret position under the point, or, for a selection made with
// SelectWordAt, SelectLineAt or SelectParagraphAt, to the edge of the
// word, line or paragraph under the point, keeping the unit first selected
// inside the selection.
func (l *Layout) ExtendSelection(s Selection, x, y float32) Selection {
	var ts, te Cursor
	switch s.unit {
	case unitWord:
		ts, te = l.wordAt(x, y)
	case unitLine:
		li := l.lineAtY(y)
		ts, te = l.lineStart(li), l.lineEnd(li)
	case unitParagraph:
		ts, te = l.paragraphAt(l.HitTest(x, y))
	default:
		return Selection{Anchor: s.Anchor, Focus: l.HitTest(x, y)}
	}
	us, ue := l.SnapCursor(s.unitStart), l.SnapCursor(s.unitEnd)
	out := s
	out.hasGoal = false
	if before(ts, us) {
		out.Anchor, out.Focus = ue, ts
	} else {
		out.Anchor, out.Focus = us, te
		if before(te, ue) {
			out.Focus = ue
		}
	}
	return out
}
