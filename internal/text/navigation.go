package text

import (
	"sort"
	"unicode"

	"github.com/go-text/typesetting/segmenter"
)

// This file holds keyboard navigation: logical and visual movement by
// grapheme cluster, word movement, line start and end, vertical movement
// with a goal column, pages and the ends of the text. The visual movement
// follows Parley's editing/cursor.rs (each step crosses one cluster, and
// the cursor attaches to the cluster it crossed); no code was copied.

// Movement is a caret movement for Layout.Move.
type Movement uint8

const (
	// MoveLeft and MoveRight move one grapheme cluster to the left or
	// right on screen (visual movement): in bidi text, the caret crosses
	// the cluster next to it on that side, whatever its direction, and
	// attaches to it; at a line's end it goes to the adjacent line in
	// reading order. With a selection and no extension, they collapse it
	// to its start or end in the paragraph's direction (Chromium's rule).
	MoveLeft Movement = iota
	MoveRight
	// MoveBackward and MoveForward move one grapheme cluster in logical
	// order (what Chromium does for the arrow keys, mapped by the
	// paragraph's direction). An inline box is a step of its own.
	MoveBackward
	MoveForward
	// MoveWordBackward moves to the start of the word before the caret
	// (or of the word it is in); MoveWordForward to the end of the word
	// after the caret (or of the word it is in). Words are UAX #29 word
	// segments that start with a letter or a digit; white space,
	// punctuation, symbols and emoji between words are skipped, as macOS
	// and GTK do (Chromium on Linux also stops after punctuation).
	MoveWordBackward
	MoveWordForward
	// MoveNextWordStart moves to the start of the next word (Windows'
	// ctrl+right).
	MoveNextWordStart
	// MoveWordLeft and MoveWordRight are MoveWordBackward and
	// MoveWordForward mapped by the paragraph's direction (swapped in a
	// right-to-left paragraph), like Chromium.
	MoveWordLeft
	MoveWordRight
	// MoveLineStart and MoveLineEnd move to the logical start and end of
	// the visual line (Home and End). The end is before a final forced
	// break and before white space collapsed at a soft wrap, after
	// preserved white space that hangs (Chromium's rule).
	MoveLineStart
	MoveLineEnd
	// MoveUp and MoveDown move to the previous or next line at the goal
	// column: the caret's x when vertical movement started, kept by the
	// selection across lines of any content. From the first line MoveUp
	// goes to the start of the text, from the last line MoveDown to the
	// end.
	MoveUp
	MoveDown
	// MoveDocumentStart and MoveDocumentEnd move to the ends of the text.
	MoveDocumentStart
	MoveDocumentEnd
)

// Move returns s with its focus moved by m. With extend (shift), the
// anchor stays and the selection grows or shrinks; without it, the result
// is collapsed at the new focus. Vertical movement keeps the goal column
// of s; every other movement drops it.
func (l *Layout) Move(s Selection, m Movement, extend bool) Selection {
	anchor, focus := l.SnapCursor(s.Anchor), l.SnapCursor(s.Focus)
	var nf Cursor
	switch m {
	case MoveLeft, MoveRight, MoveBackward, MoveForward:
		if anchor != focus && !extend {
			start, end := l.ordered(s)
			var forward bool
			switch m {
			case MoveForward:
				forward = true
			case MoveRight:
				forward = l.base == 0
			case MoveLeft:
				forward = l.base == 1
			}
			if forward {
				return CaretSelection(end)
			}
			return CaretSelection(start)
		}
		switch m {
		case MoveLeft:
			nf = l.moveVisual(focus, false)
		case MoveRight:
			nf = l.moveVisual(focus, true)
		case MoveBackward:
			nf = l.moveLogical(focus, false)
		default:
			nf = l.moveLogical(focus, true)
		}
	case MoveWordBackward:
		nf = l.prevWordStart(focus)
	case MoveWordForward:
		nf = l.nextWordEnd(focus)
	case MoveNextWordStart:
		nf = l.nextWordStart(focus)
	case MoveWordLeft, MoveWordRight:
		if (m == MoveWordRight) == (l.base == 0) {
			nf = l.nextWordEnd(focus)
		} else {
			nf = l.prevWordStart(focus)
		}
	case MoveLineStart:
		nf = l.lineStart(int(l.caretFor(l.snap(focus.Offset), focus.Affinity).line))
	case MoveLineEnd:
		nf = l.lineEnd(int(l.caretFor(l.snap(focus.Offset), focus.Affinity).line))
	case MoveUp, MoveDown:
		goal, ok := s.goalX, s.hasGoal
		if !ok {
			goal = l.Caret(focus).X
		}
		li := int(l.caretFor(l.snap(focus.Offset), focus.Affinity).line)
		step := 1
		if m == MoveUp {
			step = -1
		}
		if nf, ok = l.verticalTarget(li, step, goal); !ok {
			if m == MoveUp {
				nf = l.docStart()
			} else {
				nf = l.docEnd()
			}
		}
		out := l.withFocus(anchor, nf, extend)
		out.goalX, out.hasGoal = goal, true
		return out
	case MoveDocumentStart:
		nf = l.docStart()
	case MoveDocumentEnd:
		nf = l.docEnd()
	default:
		return s
	}
	return l.withFocus(anchor, nf, extend)
}

// MovePage moves the focus of s by a page of the given height (logical
// px; negative for page up): to the line at the middle of the caret's line
// plus height (at least one line away), at the goal column. With less than
// a page left it goes to the first or last line, and from there to the
// ends of the text. The goal column is kept, as for MoveUp and MoveDown.
func (l *Layout) MovePage(s Selection, height float32, extend bool) Selection {
	anchor, focus := l.SnapCursor(s.Anchor), l.SnapCursor(s.Focus)
	if isNaN32(height) || height == 0 {
		return s
	}
	goal, ok := s.goalX, s.hasGoal
	if !ok {
		goal = l.Caret(focus).X
	}
	li := int(l.caretFor(l.snap(focus.Offset), focus.Affinity).line)
	ln := &l.lines[li]
	step := 1
	if height < 0 {
		step = -1
	}
	target := l.lineAtY(ln.top + ln.height/2 + height)
	if target == li {
		target += step
	}
	nf, ok := l.verticalTarget(target-step, step, goal)
	if !ok {
		if step < 0 {
			nf = l.docStart()
		} else {
			nf = l.docEnd()
		}
	}
	out := l.withFocus(anchor, nf, extend)
	out.goalX, out.hasGoal = goal, true
	return out
}

func (l *Layout) withFocus(anchor, focus Cursor, extend bool) Selection {
	if extend {
		return Selection{Anchor: anchor, Focus: focus}
	}
	return CaretSelection(focus)
}

// slot is a logical caret position: before cluster ci (j == 0) or at its
// j-th inner grapheme boundary.
type slot struct{ ci, j int32 }

// slotOf returns the logical position of cursor c: before the inline
// boxes at its offset for Upstream, after them for Downstream.
func (l *Layout) slotOf(c Cursor) slot {
	p := l.snap(c.Offset)
	if p.j > 0 {
		return slot{p.ci, p.j}
	}
	gF, gL := l.gapClusters(p.off)
	if c.Affinity == Upstream {
		return slot{gF, 0}
	}
	return slot{gL, 0}
}

// hiddenSlot reports whether position s is inside hidden text (with hidden
// clusters on both sides), where logical movement does not stop.
func (l *Layout) hiddenSlot(s slot) bool {
	n := int32(len(l.clusters))
	if s.j > 0 {
		return l.cline[s.ci] < 0
	}
	return s.ci > 0 && s.ci < n && l.cline[s.ci-1] < 0 && l.cline[s.ci] < 0
}

// moveLogical moves c one grapheme cluster (or inline box) forward or
// backward in logical order. The cursor attaches to what it crossed:
// moving forward over text gives Upstream (so the caret stays at the end
// of a line at a soft wrap after preserved spaces, as in Chromium), moving
// backward gives Downstream. Positions between two inline boxes at the
// same offset, which no cursor can hold, and positions inside hidden text
// are skipped.
func (l *Layout) moveLogical(c Cursor, forward bool) Cursor {
	n := int32(len(l.clusters))
	s := l.slotOf(c)
	from := l.caretFor(l.snap(c.Offset), c.Affinity)
	for {
		var aff Affinity
		var crossed int32
		if forward {
			if s.ci >= n {
				return l.SnapCursor(c)
			}
			crossed = s.ci
			if g := int32(l.clusters[s.ci].graph); s.j+1 < g {
				s.j++
			} else {
				s = slot{s.ci + 1, 0}
			}
			aff = Upstream
			if s.j == 0 && l.clusters[crossed].flags&cfBox != 0 {
				aff = Downstream // after the box
			}
		} else {
			switch {
			case s.j > 0:
				s.j--
			case s.ci == 0:
				return l.SnapCursor(c)
			default:
				s.ci--
				if g := int32(l.clusters[s.ci].graph); g > 1 {
					s.j = g - 1
				}
			}
			crossed = s.ci
			aff = Downstream
			if s.j == 0 && l.clusters[s.ci].flags&cfBox != 0 {
				aff = Upstream // before the box
			}
		}
		if s.j == 0 && s.ci > 0 && s.ci < n && l.clusters[s.ci].flags&cfBox != 0 && l.clusters[s.ci-1].flags&cfBox != 0 &&
			l.clusterText(s.ci) == l.clusterText(s.ci-1) {
			continue // between two boxes at one offset
		}
		if l.hiddenSlot(s) {
			continue
		}
		cur := l.slotCursor(s.ci, s.j, aff)
		if s.j == 0 && l.Caret(cur).Hidden {
			// At the edge of hidden text, attach to the visible side.
			if alt := l.slotCursor(s.ci, 0, 1-aff); !l.Caret(alt).Hidden {
				cur = alt
			}
		}
		if l.removedSpace(crossed) && l.sameCaret(cur, from) {
			continue // collapsed white space is not a stop of its own
		}
		return cur
	}
}

// removedSpace reports whether visible cluster ci is white space removed at
// the end of its line (zero advance).
func (l *Layout) removedSpace(ci int32) bool {
	if ci < 0 || ci >= int32(len(l.clusters)) || l.cline[ci] < 0 {
		return false
	}
	return ci >= l.lines[l.cline[ci]].trail && l.removed(l.clusters[ci].flags)
}

// sameCaret reports whether cursor c shows its caret where info is.
func (l *Layout) sameCaret(c Cursor, info caretInfo) bool {
	ci := l.caretFor(l.snap(c.Offset), c.Affinity)
	return ci.line == info.line && ci.x == info.x
}

// moveVisual moves c one grapheme cluster to the right or left on screen
// (see MoveRight).
func (l *Layout) moveVisual(c Cursor, right bool) Cursor {
	c = l.SnapCursor(c)
	info := l.caretFor(l.snap(c.Offset), c.Affinity)
	li := int(info.line)
	var buf [lineElemsBuf]velem
	elems := l.lineElems(li, buf[:0])
	e, s := l.locate(elems, info)
	for len(elems) > 0 {
		if right {
			if s == elems[e].g {
				if e+1 == len(elems) {
					break
				}
				e, s = e+1, 0
			}
			s++
		} else {
			if s == 0 {
				if e == 0 {
					break
				}
				e, s = e-1, elems[e-1].g
			}
			s--
		}
		// The cursor attached to the crossed element at this stop, or
		// the one of the element on the other side of the same boundary.
		// Collapsed white space is not a stop of its own.
		x := elems[e].stopX(s)
		if elems[e].ci >= 0 && l.removedSpace(elems[e].ci) && x == info.x {
			continue
		}
		if cur, ok := l.stopCursor(li, &elems[e], s); ok && l.stopAt(cur, li, x) {
			return cur
		}
		var te int
		var ts int32
		switch {
		case right && s == elems[e].g && e+1 < len(elems):
			te, ts = e+1, 0
		case !right && s == 0 && e > 0:
			te, ts = e-1, elems[e-1].g
		default:
			continue
		}
		if cur, ok := l.stopCursor(li, &elems[te], ts); ok && l.stopAt(cur, li, x) {
			return cur
		}
	}
	// Past the end of the line: the adjacent line in reading order (the
	// next one at the right end of a left-to-right paragraph's line, the
	// previous one in a right-to-left paragraph), at its near edge. Lines
	// that no cursor can show (inline boxes alone between two wraps) are
	// skipped.
	step := 1
	if right == (l.base == 1) {
		step = -1
	}
	for next := li + step; next >= 0 && next < len(l.lines); next += step {
		if cur, ok := l.edgeStop(next, right); ok {
			return cur
		}
	}
	return c
}

// edgeStop returns the cursor of the first caret stop of line li, from its
// left edge (or from its right edge when fromLeft is false), that a cursor
// shows on that line.
func (l *Layout) edgeStop(li int, fromLeft bool) (Cursor, bool) {
	var buf [lineElemsBuf]velem
	elems := l.lineElems(li, buf[:0])
	if len(elems) == 0 {
		return l.hitLine(li, 0), true
	}
	if fromLeft {
		for e := range elems {
			for s := int32(0); s <= elems[e].g; s++ {
				if cur, ok := l.stopCursor(li, &elems[e], s); ok && l.stopAt(cur, li, elems[e].stopX(s)) {
					return cur, true
				}
			}
		}
		return Cursor{}, false
	}
	for e := len(elems) - 1; e >= 0; e-- {
		for s := elems[e].g; s >= 0; s-- {
			if cur, ok := l.stopCursor(li, &elems[e], s); ok && l.stopAt(cur, li, elems[e].stopX(s)) {
				return cur, true
			}
		}
	}
	return Cursor{}, false
}

// verticalTarget returns the cursor at x on the first line from li in
// direction step (±1) that a cursor can show, or false past the text.
func (l *Layout) verticalTarget(li, step int, x float32) (Cursor, bool) {
	for t := li + step; t >= 0 && t < len(l.lines); t += step {
		c := l.hitLine(t, x)
		if int(l.caretFor(l.snap(c.Offset), c.Affinity).line) == t {
			return c, true
		}
	}
	return Cursor{}, false
}

// locate returns the element and caret stop of a caret on its line.
func (l *Layout) locate(elems []velem, info caretInfo) (int, int32) {
	for i := range elems {
		el := &elems[i]
		if el.ci != info.ci {
			continue
		}
		if el.ci == ciEllipsis {
			if el.rtl {
				return i, 0
			}
			return i, 1
		}
		var j int32 // stop from the leading edge
		switch {
		case info.edge == 0:
			j = 0
		case info.edge < 0:
			j = el.g
		default:
			j = info.edge
		}
		if el.rtl {
			return i, el.g - j
		}
		return i, j
	}
	// Not attached to an element (an empty line): the nearest stop.
	if len(elems) == 0 {
		return 0, 0
	}
	return l.nearestStop(elems, info.x)
}

// lineStart returns the cursor at the logical start of line li (Home).
func (l *Layout) lineStart(li int) Cursor {
	ln := &l.lines[li]
	k := l.clusterText(ln.c0)
	// Upstream is before inline boxes that begin the line, when it is on
	// this line (after a forced break or at the start of the text).
	for _, a := range [...]Affinity{Upstream, Downstream} {
		p := l.snap(k)
		if int(l.caretFor(p, a).line) == li {
			return l.canon(p, a)
		}
	}
	return l.canon(l.snap(k), Downstream)
}

// lineEnd returns the cursor at the logical end of line li (End): before
// a final forced break and white space removed at its end, after hanging
// preserved white space; on a truncated line, the end of the hidden text.
func (l *Layout) lineEnd(li int) Cursor {
	ln := &l.lines[li]
	if ln.ellipsis >= 0 {
		if c, ok := l.hiddenCursor(li); ok {
			return c
		}
	}
	c := ln.c1
	for c > ln.c0 && (l.clusters[c-1].flags&cfNewline != 0 || (c-1 >= ln.trail && l.removed(l.clusters[c-1].flags))) {
		c--
	}
	k := l.clusterText(c)
	if c == int32(len(l.clusters)) {
		k = len(l.src)
	}
	prefer := [...]Affinity{Upstream, Downstream}
	if c > ln.c0 && l.clusters[c-1].flags&cfBox != 0 {
		prefer = [...]Affinity{Downstream, Upstream} // after a final inline box
	}
	for _, a := range prefer {
		p := l.snap(k)
		if int(l.caretFor(p, a).line) == li {
			return l.canon(p, a)
		}
	}
	return l.canon(l.snap(k), Upstream)
}

// runeAt returns the index of the first processed rune whose source offset
// is at or after off (the number of runes at the end of the text).
func (l *Layout) runeAt(off int) int32 {
	src := l.text.src
	n := len(l.text.runes)
	return int32(sort.Search(n, func(i int) bool { return int(src[i]) >= off }))
}

// nextWordEnd returns the cursor at the end of the first word that ends
// after c, or at the end of the text.
func (l *Layout) nextWordEnd(c Cursor) Cursor {
	off := l.snap(c.Offset).off
	n := int32(len(l.text.runes))
	for i := l.runeAt(off) + 1; i <= n; i++ {
		if l.segs[i]&segWordEnd != 0 && l.runeOffset(i) > off {
			return l.runeCursor(i, Upstream)
		}
	}
	return l.docEnd()
}

// prevWordStart returns the cursor at the start of the last word that
// starts before c, or at the start of the text.
func (l *Layout) prevWordStart(c Cursor) Cursor {
	off := l.snap(c.Offset).off
	for i := min(l.runeAt(off), int32(len(l.text.runes))-1); i >= 0; i-- {
		if l.segs[i]&segWordStart != 0 && l.runeOffset(i) < off {
			return l.runeCursor(i, Downstream)
		}
	}
	return l.docStart()
}

// nextWordStart returns the cursor at the start of the first word that
// starts after c, or at the end of the text.
func (l *Layout) nextWordStart(c Cursor) Cursor {
	off := l.snap(c.Offset).off
	n := int32(len(l.text.runes))
	for i := l.runeAt(off) + 1; i < n; i++ {
		if l.segs[i]&segWordStart != 0 && l.runeOffset(i) > off {
			return l.runeCursor(i, Downstream)
		}
	}
	return l.docEnd()
}

// markWords sets segWordStart and segWordEnd in out (one entry per rune
// boundary) for the UAX #29 words of runes (segments that start with a
// letter or a digit), from seg, a Segmenter initialized with runes.
//
// go-text's WordIterator (at e49e2bd) never reports a word that starts
// where the previous reported word ends: after returning a word it moves
// to the next boundary without checking whether the segment it skips is a
// word, so in "日本語" (one word per ideograph) it reports 日 and 語 but not
// 本. Every word it skips that way directly follows a reported one, so
// each is recovered by segmenting a window of the text from the reported
// word's end.
func markWords(seg *segmenter.Segmenter, runes []rune, out []uint8) {
	mark := func(s, e int) {
		out[s] |= segWordStart
		out[e] |= segWordEnd
	}
	wit := seg.WordIterator()
	for wit.Next() {
		w := wit.Word()
		e := w.Offset + len(w.Text)
		mark(w.Offset, e)
		if e < len(runes) && (unicode.IsLetter(runes[e]) || unicode.IsNumber(runes[e])) {
			if e2 := firstWordEnd(runes, e); e2 > e {
				mark(e, e2)
			}
		}
	}
}

// firstWordEnd returns the end of the word that starts at rune b (b if no
// word starts there), segmenting a window that grows until the end is far
// enough from its edge for the UAX #29 rules (which look two characters
// ahead) to be unaffected by it.
func firstWordEnd(runes []rune, b int) int {
	var seg segmenter.Segmenter
	for win := 32; ; win *= 2 {
		end := min(b+win, len(runes))
		seg.Init(runes[b:end])
		it := seg.WordIterator()
		if !it.Next() || it.Word().Offset != 0 {
			return b
		}
		e := b + len(it.Word().Text)
		if end == len(runes) || e < end-2 {
			return e
		}
	}
}

// runeOffset returns the source offset of the boundary before processed
// rune i (the end of the text for the number of runes).
func (l *Layout) runeOffset(i int32) int {
	if int(i) >= len(l.text.runes) {
		return len(l.src)
	}
	return int(l.text.src[i])
}
