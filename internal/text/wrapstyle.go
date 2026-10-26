// Portions of this file are ported from Chromium's Blink
// (third_party/blink/renderer/core/layout/inline/score_line_breaker.cc,
// paragraph_line_breaker.cc and line_break_candidate.cc):
//
// Copyright 2023 The Chromium Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file (see THIRD_PARTY.md).

package text

import (
	"math"
	"unicode/utf16"
)

// This file holds text-wrap-style (ParagraphStyle.WrapStyle): balanced and
// pretty lines, as Blink chooses them. Both start from the greedy lines
// that BreakLines computed and only move line breaks, reusing the shaping,
// the clusters and the break opportunities (nothing is shaped again); the
// lines are then built, truncated and aligned in the full width like
// greedy ones. The intrinsic widths never go through here.
//
// Balance (Blink's LineBreakStrategy::Balance) tries two algorithms:
//
//  1. The score line breaker (ScoreLineBreaker::BalanceBreakPoints), a
//     Knuth-Plass-style search after Android's Minikin: over the break
//     opportunities of the lines up to the first forced break (or the
//     first LineClamp lines), it minimizes the sum of the squared free
//     space of every line, the last one included, plus a line penalty
//     (four times the width times the font size, none when justified) and
//     an orphan penalty of 10000 on the last opportunity. It gives up
//     with more than 6 lines (kMaxLinesForBalance), fewer than 3 break
//     opportunities, preserved tabs, negative inline edges, or a result
//     with a different number of lines than greedy breaking. Only the
//     first part of a paragraph with forced breaks is balanced: Blink
//     starts balancing only at the start of the block.
//  2. Else, bisection of the available width
//     (ParagraphLineBreaker::AttemptParagraphBalancing), unless the
//     paragraph has a forced break: greedy breaking at the available width
//     must give 2 to 6 lines (or LineClamp lines, which then bound what is
//     balanced), and a rough estimate of the number of lines (characters
//     over the advance of a space) at most 12; the narrowest width, to
//     1 px in LayoutUnits (1/64 px), between 80% of the mean line width and
//     the available width at which greedy breaking still gives that many
//     lines breaks every line.
//
// Neither applies when a line overflows or ends in an emergency break
// (overflow-wrap): Blink's line breaker disables both on an overflowing
// line and on the second pass that breaks inside words.
//
// Pretty (ScoreLineBreaker::OptimalBreakPoints) uses the same search on the
// last 4 lines of every part of the paragraph (the lines up to a forced
// break or the end), when the last line has a single word and less than a
// third of the width; its last line has no free-space score.
//
// Cost: nothing for other paragraphs (BreakLines checks WrapStyle). For a
// balanced or pretty one, one pass over the clusters of the lines it
// chooses again, a search over their break opportunities that the line
// width bounds, and, when bisecting, about 8 greedy breakings of the
// paragraph (which bisection limits to 6 lines). perf: the trials could
// stop at the line limit instead of breaking the whole paragraph.
//
// Blink lays out in device pixels with the zoom set to the display scale
// and divides its scores by the zoom, so its choices do not depend on the
// scale; this file works in logical px at zoom 1 and is the same at every
// scale. Fusion safety (ADR-041): every product is rounded explicitly.

const (
	// balanceMaxLines is Blink's kMaxLinesForBalance: longer paragraphs
	// wrap greedily.
	balanceMaxLines = 6
	// prettyMaxLines is Blink's kMaxLinesForOptimal: text-wrap: pretty
	// chooses the breaks of the last lines of a paragraph only.
	prettyMaxLines = 4
	// scoreOverfull is the score of a line that does not fit (Blink's
	// kScoreOverfull).
	scoreOverfull = float32(1e12)
	// orphanPenalty is added to the last break opportunity, to keep a
	// word alone on the last line (Blink's kOrphansPenalty at zoom 1).
	orphanPenalty = float32(10000)
	// lastLinePenalty multiplies the penalty of the break before the last
	// line when the last line has no width score (Blink's
	// kLastLinePenaltyMultiplier).
	lastLinePenalty = float32(4)
)

// wrapCand is a break candidate (Blink's LineBreakCandidate): a line can
// end before cluster at. Positions are from the start of the lines being
// broken: noBreak is where the next line starts (after the white space
// that hangs or is removed at the break), ifBreak where this line's
// content ends if it breaks here (before that white space). penalty is
// added to the score of a line that ends here.
type wrapCand struct {
	at               int32
	noBreak, ifBreak float32
	penalty          float32
}

// applyWrapStyle chooses the line breaks of text-wrap: balance and pretty
// over the greedy lines in l.lines, broken at maxW; tabs holds the tab
// advances of those lines (see breakLines).
func (l *Layout) applyWrapStyle(maxW float32, tabs []float32) {
	if !l.para.WhiteSpace.wraps() || math.IsInf(float64(maxW), 1) || len(l.lines) < 2 {
		return
	}
	switch l.para.WrapStyle {
	case TextWrapBalance:
		l.balance(maxW, tabs)
	case TextWrapPretty:
		l.pretty(maxW)
	}
}

// advFunc returns the advance of each cluster for line widths: tab
// advances from tabs, the clusters' own otherwise.
func (l *Layout) advFunc(tabs []float32) func(i int32) float32 {
	return func(i int32) float32 {
		if tabs != nil && l.clusters[i].flags&cfTab != 0 {
			return tabs[i]
		}
		return l.clusters[i].adv
	}
}

// overflows reports whether one of the lines [la, lb) of l.lines is
// wider than w (content without a break opportunity that does not fit) or
// ends in an emergency break (overflow-wrap). Blink's paragraph-level
// breakers handle neither: an overflowing line, and the second pass of
// LineBreaker::RetryAfterOverflow that breaks inside words, disable both.
func (l *Layout) overflows(la, lb int, w float32, adv func(int32) float32) bool {
	for li := la; li < lb; li++ {
		ln := &l.lines[li]
		if l.overflowing(ln.c0, ln.c1, ln.reason, w, adv) {
			return true
		}
	}
	return false
}

// overflowing reports whether the line [c0, c1) broken at width w ends in
// an emergency break or is wider than w (see overflows).
func (l *Layout) overflowing(c0, c1 int32, reason BreakReason, w float32, adv func(int32) float32) bool {
	if reason == BreakEmergency {
		return true
	}
	lw, _, _ := l.lineWidth(c0, c1, reason, w, adv)
	return lw > w+fitEpsilon
}

// scoreDisabled reports what keeps Blink's score line breaker away from a
// whole paragraph: preserved tabs (InlineItemsBuilder disables it) and
// inline edges of negative width (break positions must not decrease).
func (l *Layout) scoreDisabled() bool {
	for i := range l.clusters {
		if l.clusters[i].flags&cfTab != 0 {
			return true
		}
	}
	for _, e := range l.ext {
		if e < 0 {
			return true
		}
	}
	return false
}

// hasForcedBreak reports a forced break anywhere in the paragraph, which
// keeps Blink from bisecting its width.
func (l *Layout) hasForcedBreak() bool {
	for i := range l.clusters {
		if l.clusters[i].flags&(cfMustBefore|cfNewline) != 0 {
			return true
		}
	}
	return false
}

// balance implements text-wrap: balance (see the top of the file).
func (l *Layout) balance(maxW float32, tabs []float32) {
	adv := l.advFunc(tabs)
	clamp := l.para.LineClamp
	// The lines the score line breaker balances: up to the first forced
	// break or the clamp, whichever comes first.
	m := 0
	for m < len(l.lines) {
		m++
		if l.lines[m-1].reason == BreakForced || (clamp > 0 && m == clamp) {
			break
		}
	}
	if m > 1 && m <= balanceMaxLines && !l.scoreDisabled() && !l.overflows(0, m, maxW, adv) &&
		l.scoreBreaks(0, m, maxW, true) {
		return
	}

	// Bisection of the available width.
	if l.hasForcedBreak() || !(maxW > 0) {
		return
	}
	num, stopAt := 0, int32(-1)
	if clamp > 0 && clamp <= balanceMaxLines {
		if clamp == 1 {
			return
		}
		num = min(clamp, len(l.lines))
		if len(l.lines) > clamp {
			stopAt = l.lines[clamp].c0
		}
	} else {
		// Because this is an estimate, it may be twice the limit.
		if l.estimateLines(maxW) > 2*balanceMaxLines || len(l.lines) > balanceMaxLines {
			return
		}
		num = len(l.lines)
	}
	if num <= 1 || l.overflows(0, num, maxW, adv) {
		return
	}
	w, ok := l.bisectWidth(maxW, num, stopAt, adv)
	if !ok {
		return
	}
	l.lines = l.lines[:0]
	l.breakLines(w, false, boxOwn, tabs, func(c0, c1 int32, reason BreakReason) {
		l.lines = append(l.lines, lineData{c0: c0, c1: c1, reason: reason, ellipsis: -1})
	})
}

// pretty implements text-wrap: pretty (see the top of the file).
func (l *Layout) pretty(maxW float32) {
	if l.scoreDisabled() {
		return
	}
	adv := l.advFunc(nil) // no tabs: scoreDisabled
	clamp := l.para.LineClamp
	for s := 0; s < len(l.lines); {
		e := s + 1 // the part [s, e): up to a forced break or the end
		for e < len(l.lines) && l.lines[e-1].reason != BreakForced {
			e++
		}
		end := e
		if clamp > 0 && end > clamp {
			// Blink ends the part at the clamp; the lines after it are
			// hidden.
			end = clamp
		}
		if start := max(s, end-prettyMaxLines); end-start > 1 && !l.overflows(s, end, maxW, adv) && l.prettyLastLine(end-1, maxW) {
			l.scoreBreaks(start, end, maxW, false)
		}
		if end < e {
			return
		}
		s = e
	}
}

// prettyLastLine reports whether line li is worth prettifying (Blink's
// ShouldOptimize): it holds less than a third of the width and has no
// break opportunity inside.
func (l *Layout) prettyLastLine(li int, maxW float32) bool {
	ln := &l.lines[li]
	w, trail, hang := l.lineWidth(ln.c0, ln.c1, ln.reason, maxW, l.advFunc(nil))
	if layoutUnitsCeil(float32(w+hang)) >= layoutUnitsFloor(maxW)/3 {
		return false
	}
	for i := ln.c0 + 1; i < trail; i++ {
		if l.clusters[i].flags&cfSoftBefore != 0 {
			return false
		}
	}
	return true
}

// layoutUnitsFloor and layoutUnitsCeil convert px to Blink's LayoutUnits
// (1/64 px), rounding down and up.
func layoutUnitsFloor(v float32) int64 { return int64(math.Floor(float64(v) * 64)) }
func layoutUnitsCeil(v float32) int64  { return int64(math.Ceil(float64(v) * 64)) }

// scoreBreaks chooses the breaks of the lines [la, lb) of l.lines by
// scores (Blink's ScoreLineBreaker::Optimize) and replaces them, when the
// result has as many lines. balanced selects text-wrap: balance (the last
// line is scored like the others) over pretty.
func (l *Layout) scoreBreaks(la, lb int, maxW float32, balanced bool) bool {
	cands := l.wrapCandidates(la, lb)
	// The sentinels at both ends and at least 3 break opportunities.
	if len(cands) < 5 {
		return false
	}
	cands[len(cands)-2].penalty += orphanPenalty
	ends := l.scoreLines(cands, maxW, balanced)
	if len(ends) != lb-la {
		return false
	}
	lastReason := l.lines[lb-1].reason
	start := l.lines[la].c0
	for k, ci := range ends {
		end := cands[ci].at
		reason := BreakSoft
		if k == len(ends)-1 {
			reason = lastReason
		}
		l.lines[la+k] = lineData{c0: start, c1: end, reason: reason, ellipsis: -1}
		start = end
	}
	return true
}

// wrapCandidates returns the break candidates of the lines [la, lb) of
// l.lines (Blink's LineBreakCandidateContext): a sentinel at their start,
// every soft wrap opportunity inside them, and their end. Like Blink's, an
// opportunity before white space that hangs at a break (after an inline
// box) is the one after that white space. Line widths follow lineWidth:
// the advances without the white space that hangs at the break, plus every
// inline edge on the line. The greedy lines end at soft opportunities:
// emergency breaks keep balancing away (overflows).
func (l *Layout) wrapCandidates(la, lb int) []wrapCand {
	c0, c1 := l.lines[la].c0, l.lines[lb-1].c1
	cl := l.clusters
	// Prefix sums of the advances and of the inline edges from c0.
	n := c1 - c0
	adv := make([]float32, n+1)
	ext := make([]float32, n+1)
	for i := c0; i < c1; i++ {
		adv[i-c0+1] = adv[i-c0] + cl[i].adv
		ext[i-c0+1] = ext[i-c0] + l.extAt(i)
	}
	cand := func(at, prev int32) wrapCand {
		t := at
		for t > prev && (l.hangable(cl[t-1].flags) || cl[t-1].flags&cfTag != 0) {
			t--
		}
		for t < at && cl[t].flags&cfTag != 0 {
			t++ // trailing placeholders alone hang nothing (lineWidth)
		}
		e := ext[at-c0]
		return wrapCand{at: at, noBreak: adv[at-c0] + e, ifBreak: adv[t-c0] + e}
	}
	cands := []wrapCand{{at: c0}}
	prev := c0
	for i := c0 + 1; i < c1; i++ {
		if cl[i].flags&cfSoftBefore == 0 {
			continue
		}
		at := i
		for at < c1 && l.hangable(cl[at].flags) {
			at++ // Blink's AppendTrailingSpaces
		}
		if at == c1 {
			break // the end of the lines is the last candidate
		}
		cands = append(cands, cand(at, prev))
		prev, i = at, at // an opportunity at the end of the white space is this one
	}
	return append(cands, cand(c1, prev))
}

// scoreLines is Blink's ScoreLineBreaker::ComputeScores and
// ComputeBreakPoints at zoom 1, for lines of width maxW (no text-indent,
// no exclusions): it returns the indices of the candidates that end the
// lines of the best score. Float operations are those of Blink, in
// float32, including its pruning (best_hope, active), which decides ties.
func (l *Layout) scoreLines(c []wrapCand, maxW float32, balanced bool) []int {
	justified := l.para.Align == AlignJustify
	// Minikin's computePenalties: the hyphen penalty is twice the width
	// times the font size (half when justified) and the line penalty
	// twice that (none when justified).
	var linePenalty float32
	if !justified {
		wfs := float32(maxW * l.sinfo[0].size)
		linePenalty = float32(float32(wfs*2) * 2)
	}
	fit := maxW + fitEpsilon // Blink's AvailableWidthToFit
	type lineScore struct {
		score      float32
		prev, line int
	}
	scores := make([]lineScore, 1, len(c))
	active := 0
	for end := 1; end < len(c); end++ {
		ec := &c[end]
		last := end == len(c)-1
		best, bestPrev := float32(math.MaxFloat32), 0
		startEdge := ec.ifBreak - fit
		var hope float32
		for start := active; start < end; start++ {
			ss := scores[start].score
			if ss+hope >= best {
				continue
			}
			delta := c[start].noBreak - startEdge
			var width, extra float32
			switch {
			case (last || !justified) && delta < 0:
				width = scoreOverfull
			case last && !balanced:
				extra = float32(lastLinePenalty * c[start].penalty)
			case delta < 0:
				width = scoreOverfull
			default:
				width = float32(delta * delta)
			}
			if delta < 0 {
				active = start + 1
			} else {
				hope = width
			}
			if s := ss + width + extra; s <= best {
				best, bestPrev = s, start
			}
		}
		scores = append(scores, lineScore{best + ec.penalty + linePenalty, bestPrev, scores[bestPrev].line + 1})
	}
	n := scores[len(scores)-1].line
	ends := make([]int, n)
	for i, k := len(c)-1, n-1; i > 0 && k >= 0; i, k = scores[i].prev, k-1 {
		ends[k] = i
	}
	return ends
}

// estimateLines is Blink's EstimateNumLines: the number of lines the text
// would take with every character as wide as a space of the block's font
// (0 when that font has no space).
func (l *Layout) estimateLines(maxW float32) int {
	si := &l.sinfo[0]
	space := spaceAdvance(si.face, si.size)
	if !(space > 0) {
		return 0
	}
	perLine := int(float32(float32(layoutUnitsFloor(maxW))/64) / space)
	if perLine <= 0 {
		return math.MaxInt
	}
	// The length of Blink's text content, in UTF-16 code units: the text
	// after white-space processing, an object replacement character per
	// atomic inline and nothing for empty inlines.
	chars := 0
	for i, r := range l.text.runes {
		if b := l.text.box[i]; b >= int32(l.nboxes) {
			continue
		}
		chars += max(1, utf16.RuneLen(r))
	}
	return (chars + perLine - 1) / perLine
}

// bisectWidth is Blink's BisectAvailableWidth: the narrowest width,
// between 80% of the mean width of the first num greedy lines and maxW,
// at which greedy breaking still ends the paragraph (or reaches the
// cluster stopAt, when it is not negative) within num lines and without
// overflow, to 1 px, in LayoutUnits. It reports false when that is maxW
// itself: the greedy lines stay.
func (l *Layout) bisectWidth(maxW float32, num int, stopAt int32, adv func(int32) float32) (float32, bool) {
	var sum int64
	for li := range num {
		ln := &l.lines[li]
		w, _, hang := l.lineWidth(ln.c0, ln.c1, ln.reason, maxW, adv)
		sum += layoutUnitsCeil(float32(w + hang)) // LineInfo::Width, hanging space included
	}
	avg := sum / int64(num)
	lower := int64(math.Round(float64(float32(float32(float32(avg)/64)*0.8)) * 64))
	upper := layoutUnitsFloor(maxW)
	top := upper
	const epsilon = 64 // 1 px: narrower steps are worthless
	tabs := make([]float32, len(l.clusters))
	tadv := l.advFunc(tabs)
	n := int32(len(l.clusters))
	for lower+epsilon < upper {
		mid := (upper + lower) / 2
		w := float32(mid) / 64
		count, done, ok := 0, false, true
		l.breakLines(w, false, boxOwn, tabs, func(c0, c1 int32, reason BreakReason) {
			if done || !ok {
				return
			}
			count++
			if l.overflowing(c0, c1, reason, w, tadv) {
				ok = false
				return
			}
			if c1 >= n || (stopAt >= 0 && c1 >= stopAt) {
				done = true
			} else if count == num {
				ok = false
			}
		})
		if ok && done {
			upper = mid
		} else {
			lower = mid
		}
	}
	if upper == top {
		return 0, false
	}
	return float32(upper) / 64, true
}
