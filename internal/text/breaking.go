package text

import (
	"math"
	"slices"
	"unicode"
)

// setBreaks sets the break flags of the clusters: forced breaks after
// forced break characters (UAX #14 mandatory breaks), and, when the
// white-space mode wraps, soft wrap opportunities from UAX #14 adjusted for
// word-break, break-spaces and inline boxes (always an opportunity on both
// sides, CSS Text 3, section 5.1). Opportunities inside a cluster (a
// ligature) are dropped.
func (l *Layout) setBreaks(segs []uint8) {
	ws := l.para.WhiteSpace
	wrap := ws.wraps()
	cl := l.clusters
	prevReal := int32(-1) // the last cluster that is not a placeholder (inline.go)
	if len(cl) > 0 && cl[0].flags&cfTag == 0 {
		prevReal = 0
	}
	for i := 1; i < len(cl); i++ {
		p := cl[i].r0
		cur := cl[i].flags
		if segs[p]&segMandatory != 0 {
			cl[i].flags |= cfMustBefore
			if cur&cfTag == 0 {
				prevReal = int32(i)
			}
			continue
		}
		if cur&cfTag != 0 {
			// The placeholder of an empty inline is transparent to line
			// breaking: the opportunities around it are those between
			// its neighbors, after it (tagBreaks).
			continue
		}
		if !wrap || prevReal < 0 {
			prevReal = int32(i)
			continue
		}
		soft := segs[p]&segSoft != 0
		prev := cl[prevReal].flags
		q := cl[prevReal].r1 // after the last rune of text before i (placeholders have none)
		prevReal = int32(i)
		switch l.para.WordBreak {
		case WordBreakBreakAll:
			if prev&cfLetter != 0 && cur&cfLetter != 0 {
				soft = true
			}
		case WordBreakKeepAll:
			if prev&cfLetter != 0 && cur&cfLetter != 0 {
				soft = false
			}
		}
		if ws == WhiteSpaceBreakSpaces && prev&(cfSpace|cfTab|cfOtherSpace) != 0 {
			soft = true
		}
		if prev&cfBox != 0 || cur&cfBox != 0 {
			soft = true
		}
		if soft && q > 0 && l.text.runes[q-1] == softHyphen && l.styles[l.text.style[q-1]].Hyphens == HyphensNone {
			// hyphens: none: a soft hyphen is not a break opportunity
			// (Blink's LazyLineBreakIterator with soft hyphens disabled).
			soft = false
		}
		if soft {
			cl[i].flags |= cfSoftBefore
		}
	}
}

// softHyphenBefore reports whether a line that ends before cluster i at a
// soft wrap opportunity ends with a hyphen: the opportunity follows a soft
// hyphen (whose hyphens is not none, or there would be none), and the
// placeholders after it, which have no text.
func (l *Layout) softHyphenBefore(i int32) bool {
	if i <= 0 || int(i) > len(l.clusters) {
		return false
	}
	for i > 1 && l.clusters[i-1].flags&cfTag != 0 {
		i--
	}
	p := l.clusters[i-1].r1
	return p > 0 && l.text.runes[p-1] == softHyphen
}

// hangable reports whether a cluster at the end of a line hangs (or is
// removed) instead of counting for the line's width: forced breaks, and
// spaces, tabs and other space separators unless white-space is pre or
// break-spaces.
func (l *Layout) hangable(f clusterFlag) bool {
	if f&cfNewline != 0 {
		return true
	}
	if f&(cfSpace|cfTab|cfOtherSpace) == 0 {
		return false
	}
	ws := l.para.WhiteSpace
	return ws != WhiteSpacePre && ws != WhiteSpaceBreakSpaces
}

// removed reports whether trailing white space at the end of a line is
// removed (zero advance) rather than hanging: collapsible spaces.
func (l *Layout) removed(f clusterFlag) bool {
	return f&cfSpace != 0 && l.para.WhiteSpace.collapses()
}

// fitEpsilon absorbs float rounding when a width computed elsewhere (a
// max-content width, rounded by a layout engine) is used as the limit.
const fitEpsilon = 1.0 / 1024

// breakLines runs the greedy line breaker over all clusters and calls emit
// for each line (a cluster range, the reason it ends and whether it ends
// with a hyphen). Content that fits in maxW stays on the line; at the
// first cluster that does not fit, the line ends at a hyphenation point of
// the word that does not fit (hyphens: auto, hyphenateBefore), else at
// the last soft wrap opportunity, or else at the last emergency
// opportunity (any cluster boundary, with overflow-wrap anywhere, or
// break-word unless minContent), or else the content overflows: up to the
// first hyphenation point after it, else to the next opportunity. A line
// that ends after a soft hyphen or at a hyphenation point shows a hyphen;
// when its content fits but not with the hyphen, the line is broken again
// in the width less the hyphen's (Blink's LineBreaker::BreakText). Hanging
// white space always fits, the inline edges attached to it (inline.go) do
// not. A text that ends with a forced break gets a last, empty line. Tab
// advances depend on the position in the line; they are written to
// tabs[i] when tabs is not nil. sizes selects the widths of the inline
// boxes: their own, or their min- or max-content widths (SetBox) for the
// intrinsic widths. With minContent, words are hyphenated at every point
// of their dictionary (Blink's fast min-content), not as what is left of
// them after a hyphen.
func (l *Layout) breakLines(maxW float32, minContent bool, sizes boxSizes, tabs []float32, emit func(c0, c1 int32, reason BreakReason, hy bool)) {
	cl := l.clusters
	n := int32(len(cl))
	if n == 0 {
		emit(0, 0, BreakNone, false)
		return
	}
	if cap(l.scanX) < int(n)+1 {
		l.scanX = make([]float32, n+1)
	}
	l.scanX = l.scanX[:n+1]
	limit := maxW + fitEpsilon
	for start := int32(0); start < n; {
		end, reason, hy, over := l.breakLine(start, limit, minContent, sizes, tabs)
		if hy && !over {
			w := l.scanX[end]
			if hw := l.hyphenFor(l.hyphenStyle(start, end)).width; w <= limit && w+hw > limit {
				// The hyphen does not fit: break again in the width
				// without it.
				end, reason, hy, _ = l.breakLine(start, limit-hw, minContent, sizes, tabs)
			}
		}
		if end == n && cl[n-1].flags&cfNewline != 0 {
			emit(start, end, BreakForced, false)
			emit(n, n, BreakNone, false)
			return
		}
		emit(start, end, reason, hy)
		start = end
	}
}

// breakLine returns where the line that starts at cluster start ends when
// its content must fit in limit (see breakLines), why, whether it ends
// with a hyphen and whether it overflows. l.scanX[i] is then the x of
// cluster i from the start of the line, inline edges included, for the
// clusters it scanned.
func (l *Layout) breakLine(start int32, limit float32, minContent bool, sizes boxSizes, tabs []float32) (end int32, reason BreakReason, hy, over bool) {
	cl := l.clusters
	n := int32(len(cl))
	wrap := l.para.WhiteSpace.wraps()
	emergency := wrap && (l.para.OverflowWrap == OverflowWrapAnywhere || (l.para.OverflowWrap == OverflowWrapBreakWord && !minContent))
	ext := l.ext
	x := float32(0)
	opp, emg := int32(-1), int32(-1)
	for i := start; i < n; i++ {
		c := &cl[i]
		l.scanX[i] = x
		if i > start {
			if c.flags&cfMustBefore != 0 {
				return i, BreakForced, false, over
			}
			if x > 0 {
				if c.flags&cfSoftBefore != 0 {
					opp = i
				} else if emergency {
					emg = i
				}
			}
		}
		a := c.adv
		switch {
		case c.flags&cfTab != 0:
			a = l.tabAdvance(c, x)
			if tabs != nil {
				tabs[i] = a
			}
		case sizes != boxOwn && c.flags&cfBox != 0:
			a = l.boxWidth(c, sizes)
		}
		var e float32
		if ext != nil {
			e = ext[i]
		}
		nx := x + a + e
		if nx <= limit || a+e <= 0 || (l.hangable(c.flags) && (x+e <= limit || e <= 0)) {
			x = nx
			continue
		}
		if !over && wrap {
			if k := l.hyphenateBefore(start, i, opp, minContent); k > start {
				return k, BreakSoft, true, false
			}
		}
		if opp > start {
			return opp, BreakSoft, l.softHyphenBefore(opp), over
		}
		if emg > start {
			return emg, BreakEmergency, false, over
		}
		if !over && wrap {
			over = true
			if k := l.hyphenateAfter(start, i, minContent); k > start {
				// Clusters up to k are scanned: their x is needed.
				for j := i; j < k; j++ {
					l.scanX[j] = x
					x += l.clusterWidth(j, x, sizes, tabs)
				}
				l.scanX[k] = x
				return k, BreakSoft, true, true
			}
		}
		over = true
		x = nx
	}
	l.scanX[n] = x
	return n, BreakNone, false, over
}

// clusterWidth returns the advance of cluster i at x on its line, with its
// inline edges, for the intrinsic sizes of sizes (see breakLines).
func (l *Layout) clusterWidth(i int32, x float32, sizes boxSizes, tabs []float32) float32 {
	c := &l.clusters[i]
	a := c.adv
	switch {
	case c.flags&cfTab != 0:
		a = l.tabAdvance(c, x)
		if tabs != nil {
			tabs[i] = a
		}
	case sizes != boxOwn && c.flags&cfBox != 0:
		a = l.boxWidth(c, sizes)
	}
	if l.ext != nil {
		a += l.ext[i]
	}
	return a
}

// hyphenWord returns the word around cluster i for hyphenation, as Blink's
// ShapingLineBreaker::Hyphenate finds it: from the last soft wrap
// opportunity at or before i (opp; the start of the line when there is
// none, which is how Blink hyphenates what a hyphen left of a word), or,
// with whole, from the last one in the paragraph, after the spaces there,
// to the next opportunity after i, before the spaces there. Placeholders
// at either end are not part of it either: a placeholder carries no
// opportunity, so the one after an anchor ending a word ("word <abs>
// next") is after the anchor, but Blink's out-of-flow and empty inline
// items have no text (hyphenPoints). ok is false when there is an
// opportunity before i itself or the style of i does not hyphenate.
func (l *Layout) hyphenWord(start, i, opp int32, whole bool) (ws, we int32, h *Hyphenator, ok bool) {
	cl := l.clusters
	n := int32(len(cl))
	h = l.sinfo[cl[i].style].hyph
	if h == nil || (i > start && cl[i].flags&cfSoftBefore != 0) {
		return 0, 0, nil, false
	}
	ws = start
	if whole {
		ws = i
		for ws > 0 && cl[ws].flags&(cfSoftBefore|cfMustBefore) == 0 {
			ws--
		}
	} else if opp > start {
		ws = opp
	}
	for ws < i && outsideWord(cl[ws].flags) {
		ws++
	}
	we = i + 1
	for we < n && cl[we].flags&(cfSoftBefore|cfMustBefore) == 0 {
		we++
	}
	for we > i+1 && outsideWord(cl[we-1].flags) {
		we--
	}
	return ws, we, h, true
}

// breakableSpace reports the white space Blink's line breaker skips around
// words: spaces, tabs, forced breaks and other space separators.
func breakableSpace(f clusterFlag) bool {
	return f&(cfSpace|cfTab|cfNewline|cfOtherSpace) != 0
}

// outsideWord reports the clusters left out at the ends of a word to
// hyphenate: breakable white space and placeholders.
func outsideWord(f clusterFlag) bool {
	return breakableSpace(f) || f&cfTag != 0
}

// hyphenPoints returns the clusters of [ws, we) that start a hyphenation
// point of h, in increasing order (a point inside a cluster, a
// ligature, is not one). The placeholders inside the word are left out of
// what Liang's algorithm sees, as Blink's out-of-flow and empty inline
// items have no text: "interna<abs>tionalization" is one word, and a
// point at the anchor breaks after it, like a soft wrap.
func (l *Layout) hyphenPoints(ws, we int32, h *Hyphenator) []int32 {
	cl := l.clusters
	r0 := cl[ws].r0
	r1 := cl[we-1].r1
	word := l.text.runes[r0:r1]
	var at []int32 // the rune of each rune of word, when placeholders are left out
	if l.hasPlaceholders() && slices.ContainsFunc(l.text.box[r0:r1], l.isPlaceholder) {
		l.hword, l.hat = l.hword[:0], l.hat[:0]
		for r := r0; r < r1; r++ {
			if !l.isPlaceholder(l.text.box[r]) {
				l.hword = append(l.hword, l.text.runes[r])
				l.hat = append(l.hat, r)
			}
		}
		word, at = l.hword, l.hat
	}
	l.hpts = h.points(word, l.hpts[:0])
	out := l.hpts[:0]
	k := ws
	for _, p := range l.hpts {
		r := r0 + p
		if at != nil {
			r = at[p]
		}
		for k < we && cl[k].r0 < r {
			k++
		}
		if k < we && cl[k].r0 == r {
			out = append(out, k)
		}
	}
	return out
}

// hyphenateBefore returns the cluster before which the line that starts at
// start ends with a hyphen when cluster i does not fit (hyphens: auto):
// the last hyphenation point of the word of i that is not after i, else
// 0 (Blink's ShapingLineBreaker::Hyphenate backwards). Blink does not
// hyphenate the last word of a paragraph, unless it starts the line or
// is the paragraph's only word.
func (l *Layout) hyphenateBefore(start, i, opp int32, minContent bool) int32 {
	ws, we, h, ok := l.hyphenWord(start, i, opp, minContent)
	if !ok || we-ws < hyphenMinWord {
		return 0
	}
	if !minContent && opp > start && l.lastWord(opp, we) {
		return 0
	}
	var best int32
	for _, k := range l.hyphenPoints(ws, we, h) {
		if k > i {
			break
		}
		if k > start {
			best = k
		}
	}
	return best
}

// hyphenateAfter returns the cluster before which an overflowing line that
// starts at start ends with a hyphen: the first hyphenation point of the
// word of i (the first cluster that does not fit) not before i, else 0
// (Blink's ShapingLineBreaker::Hyphenate forwards). With minContent, the
// first point of the whole word after start.
func (l *Layout) hyphenateAfter(start, i int32, minContent bool) int32 {
	ws, we, h, ok := l.hyphenWord(start, i, -1, minContent)
	if !ok || we-ws < hyphenMinWord {
		return 0
	}
	from := i
	if minContent {
		from = start + 1
	}
	for _, k := range l.hyphenPoints(ws, we, h) {
		if k >= from {
			return k
		}
	}
	return 0
}

// lastWord reports whether the word that starts after the opportunity at
// cluster opp and ends before we is the last of the paragraph (only white
// space follows) and not its only word (Blink's ShouldHyphenate).
func (l *Layout) lastWord(opp, we int32) bool {
	first, last := l.contentEnds()
	cl := l.clusters
	if int(we) < len(cl) && cl[we].r0 <= last {
		return false
	}
	return cl[opp].r0 > first
}

// contentEnds returns the first and the last rune of the processed text
// that is not white space Blink's line breaker skips (an inline box
// counts, a placeholder does not: Blink's out-of-flow and empty inline
// items have no text); -1 and -1 when there is none.
func (l *Layout) contentEnds() (first, last int32) {
	if l.cends[0] != 0 {
		return l.cends[0] - 1, l.cends[1] - 1
	}
	pt := &l.text
	first, last = -1, -1
	for i, r := range pt.runes {
		if l.isPlaceholder(pt.box[i]) {
			continue
		}
		switch {
		case r == ' ', r == '\t', r == '\n', unicode.Is(unicode.Zs, r), isForcedBreakChar(r):
			continue
		}
		if first < 0 {
			first = int32(i)
		}
		last = int32(i)
	}
	l.cends = [2]int32{first + 1, last + 1}
	return first, last
}

// tabAdvance returns the advance of a tab at x from the line start: up to
// the next tab stop, skipping a stop closer than half a space (CSS Text 3,
// section 4.2). Tab stops are multiples of tab-size spaces of the block
// container (the paragraph's Default style), letter- and word-spacing
// included, whatever the style of the tab itself, as CSS specifies and
// Chromium does.
func (l *Layout) tabAdvance(c *cluster, x float32) float32 {
	ts := l.para.tabSize()
	si := &l.sinfo[0]
	st := &l.styles[0]
	space := spaceAdvance(si.face, si.size) + st.LetterSpacing + st.WordSpacing
	if ts == 0 || !(space > 0) {
		return 0
	}
	w := float32(ts * space)
	next := float32(float32(math.Floor(float64(x/w))+1) * w)
	if next-x < space/2 {
		next += w
	}
	return next - x
}

// spaceAdvance returns the advance of U+0020 in f at size, in px.
func spaceAdvance(f *Face, size float32) float32 {
	if f == nil {
		return 0
	}
	g, ok := f.GlyphIndex(' ')
	if !ok {
		return float32(0.25 * size)
	}
	return float32(f.Advance(g) * size)
}

// lineWidth returns the width of the line [c0, c1) that counts for fitting
// and alignment, with adv giving each cluster's advance: trailing hanging
// or removed white space is excluded, except that preserved spaces before
// a forced break or the end of the text (white-space: pre-wrap) only hang
// when they do not fit (CSS Text 4, conditionally hanging white space).
// It also returns the first trailing cluster that hangs (c1 if none) and
// the advance that hangs. The inline edges attached to the clusters count,
// on hanging white space too.
func (l *Layout) lineWidth(c0, c1 int32, reason BreakReason, maxW float32, adv func(i int32) float32) (width float32, trail int32, hang float32) {
	cl := l.clusters
	trail = c1
	// The placeholders of empty inlines after trailing white space do
	// not keep it from hanging (Blink removes the space before them).
	for trail > c0 && (l.hangable(cl[trail-1].flags) || cl[trail-1].flags&cfTag != 0) {
		trail--
	}
	for trail < c1 && cl[trail].flags&cfTag != 0 {
		trail++ // trailing placeholders alone hang nothing
	}
	for i := c0; i < trail; i++ {
		width += adv(i)
	}
	if l.ext != nil {
		for i := c0; i < c1; i++ {
			width += l.ext[i]
		}
	}
	for i := trail; i < c1; i++ {
		if !l.removed(cl[i].flags) {
			hang += adv(i)
		}
	}
	if l.para.WhiteSpace == WhiteSpacePreWrap && (reason == BreakForced || reason == BreakNone) && hang > 0 && width+hang <= maxW+fitEpsilon {
		return width + hang, c1, 0
	}
	return width, trail, hang
}

// measure returns the widest line when breaking at maxW: the min-content
// width for maxW 0 with minContent set, the max-content width for +Inf;
// inline boxes take their own widths for each (SetBox).
func (l *Layout) measure(maxW float32, minContent bool) float32 {
	tabs := make([]float32, len(l.clusters))
	sizes := boxMax
	if minContent {
		sizes = boxMin
	}
	adv := func(i int32) float32 {
		c := &l.clusters[i]
		switch {
		case c.flags&cfTab != 0:
			return tabs[i]
		case c.flags&cfBox != 0:
			return l.boxWidth(c, sizes)
		}
		return c.adv
	}
	var w float32
	l.breakLines(maxW, minContent, sizes, tabs, func(c0, c1 int32, reason BreakReason, hy bool) {
		lw, _, _ := l.lineWidth(c0, c1, reason, maxW, adv)
		if hy {
			lw += l.hyphenFor(l.hyphenStyle(c0, c1)).width
		}
		w = max(w, lw)
	})
	return w
}
