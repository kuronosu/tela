package text

import (
	"slices"
	"sort"
)

// Anchors (InlineBox.Anchor) are the places of out-of-flow boxes in a
// paragraph's content, modeled on Blink's out-of-flow inline items, which
// it measures to give an absolutely positioned box with auto insets its
// static position on its line (Chromium 155):
//
//   - Text: an anchor is a U+FFFC in the processed text, inserted after
//     white-space processing (so the spaces around it collapse as if it
//     were not there: "a <abs> b" has one space), before the text at its
//     offset and after the boxes at that offset that come before it. As
//     a bidi neutral (class ON) it takes the level of the text around it
//     (UAX #9 rules N1 and N2): between "12" and "34" in a right-to-left
//     paragraph it is right to left and splits the number, as in Blink.
//     It breaks shaping like any box (Chromium loses kerning across it,
//     "A<abs>V" is wider than "AV"), but the text around it is shaped in
//     the context of the text on its other side, so Arabic letters still
//     join across it, as in Chromium (anchorContext).
//   - Breaking: like the placeholder of an empty inline it is transparent
//     to line breaking (no opportunity around it; the one between its
//     neighbors goes after it, so it stays at the end of the line before
//     a soft wrap), except that one that opens an inline whose content
//     goes on past it goes to the next line with that content, as Blink
//     breaks before an open tag (glueAnchors).
//   - Geometry: it has no size and takes no space; AnchorPosition reports
//     its x on its line (the inline edges before it included) and its
//     direction, which decides which edge of the out-of-flow box goes
//     there.

// isPlaceholder reports whether box b (an index into l.boxes) is a
// zero-size placeholder that is transparent to line breaking: the
// placeholder of an empty inline or an anchor.
func (l *Layout) isPlaceholder(b int32) bool {
	return b >= int32(l.nboxes) || b >= 0 && l.boxes[b].Anchor
}

// hasPlaceholders reports whether the layout has placeholders
// (isPlaceholder).
func (l *Layout) hasPlaceholders() bool { return len(l.boxes) > l.nboxes || l.anchors }

// anchorContext sets l.ctxRunes to the processed text without its
// anchors and l.ctxIdx to the index there of every rune: the shaping
// context of the runs (shapeRun).
func (l *Layout) anchorContext() {
	pt := &l.text
	l.ctxRunes = make([]rune, 0, len(pt.runes))
	l.ctxIdx = make([]int32, len(pt.runes)+1)
	for i, r := range pt.runes {
		l.ctxIdx[i] = int32(len(l.ctxRunes))
		if b := pt.box[i]; b < 0 || int(b) >= l.nboxes || !l.boxes[b].Anchor {
			l.ctxRunes = append(l.ctxRunes, r)
		}
	}
	l.ctxIdx[len(pt.runes)] = int32(len(l.ctxRunes))
}

// insertAnchors inserts the U+FFFC of every anchor of p into kept, the
// text after white-space processing (see the top of this file). order is
// the boxes sorted by offset.
func insertAnchors(kept []rawRune, p *Paragraph, order []int) []rawRune {
	span := 0
	for _, bi := range order {
		b := &p.Boxes[bi]
		if !b.Anchor {
			continue
		}
		at := int32(b.At)
		// perf: one insertion per anchor.
		j := sort.Search(len(kept), func(j int) bool {
			e := &kept[j]
			return e.src > at || e.src == at && (e.box < 0 || e.box > int32(bi))
		})
		kept = slices.Insert(kept, j, rawRune{objectReplacement, at, styleAt(p, &span, b.At), int32(bi)})
	}
	return kept
}

// glueAnchors moves the line break opportunity after a run of
// placeholders before the first anchor of the run that opens an inline
// whose content goes on after the run: Blink breaks before an open tag
// (the open tag has no break opportunity after it, and an out-of-flow
// item takes the one of the item before it), so an out-of-flow box at the
// start of an inline that wraps goes to the next line with the inline's
// start edge. When an inline ends within the run after that anchor, the
// opportunity stays after the run (Blink breaks after a close tag).
func (l *Layout) glueAnchors() {
	if !l.anchors || len(l.inlines) == 0 {
		return
	}
	cl := l.clusters
	n := int32(len(cl))
	tag := func(c int32) bool { return cl[c].flags&cfTag != 0 }
	for r1 := int32(1); r1 < n; r1++ {
		if cl[r1].flags&cfSoftBefore == 0 || tag(r1) || !tag(r1-1) {
			continue
		}
		r0 := r1 - 1
		for r0 > 0 && tag(r0-1) {
			r0--
		}
		g := int32(-1)
		for i := range l.inlines {
			d := &l.inlines[i]
			if d.c0 < r0 || d.c0 >= r1 || d.c1 <= r1 || !l.isAnchorCluster(d.c0) {
				continue
			}
			if g < 0 || d.c0 < g {
				g = d.c0
			}
		}
		if g < 0 {
			continue
		}
		closes := false
		for i := range l.inlines {
			if d := &l.inlines[i]; d.c0 >= 0 && d.c1-1 >= g && d.c1-1 < r1 {
				closes = true
				break
			}
		}
		if closes {
			continue
		}
		cl[r1].flags &^= cfSoftBefore
		cl[g].flags |= cfSoftBefore
	}
}

// isAnchorCluster reports whether cluster c is an anchor's.
func (l *Layout) isAnchorCluster(c int32) bool {
	if l.clusters[c].flags&cfBox == 0 {
		return false
	}
	b := l.runs[l.clusters[c].run].box
	return b >= 0 && int(b) < l.nboxes && l.boxes[b].Anchor
}

// AnchorPos is where an anchor is on its line (AnchorPosition).
type AnchorPos struct {
	// X is the x of its place on its line (layout coordinates; the inline
	// edges before it included); Top and Height are its line box. X and
	// Top include the offsets of the inlines that hold it
	// (SetInlineOffset).
	X, Top, Height float32
	Line           int
	// RTL reports that it is right-to-left text (an odd bidi level):
	// Blink puts the left edge of an inline-level out-of-flow box at X
	// when it is left-to-right, its right edge when it is right-to-left.
	RTL bool
	// AfterContent reports that text or an inline box comes before it on
	// its line, in text order (Blink's preceding inline-level content,
	// which moves a block-level out-of-flow box to the next line).
	AfterContent bool
}

// AnchorPosition returns where Paragraph.Boxes[i], an anchor, is once the
// lines are aligned; false when i is not an anchor or it is hidden
// (truncated or clamped).
func (l *Layout) AnchorPosition(i int) (AnchorPos, bool) {
	if i < 0 || i >= l.nboxes || !l.boxes[i].Anchor {
		return AnchorPos{}, false
	}
	c := l.boxClus[i]
	if c < 0 || l.cline[c] < 0 {
		return AnchorPos{}, false
	}
	ln := &l.lines[l.cline[c]]
	dx, dy := l.clusterOff(c)
	p := AnchorPos{X: l.cx[c] + dx, Top: ln.top + dy, Height: ln.height, Line: int(l.cline[c]), RTL: l.clusterLevel(c)%2 == 1}
	for k := ln.c0; k < c; k++ {
		if l.clusters[k].flags&cfTag == 0 {
			p.AfterContent = true
			break
		}
	}
	return p, true
}
