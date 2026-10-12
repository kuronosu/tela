package text

import "math"

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
	for i := 1; i < len(cl); i++ {
		p := cl[i].r0
		if segs[p]&segMandatory != 0 {
			cl[i].flags |= cfMustBefore
			continue
		}
		if !wrap {
			continue
		}
		soft := segs[p]&segSoft != 0
		prev, cur := cl[i-1].flags, cl[i].flags
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
		if soft {
			cl[i].flags |= cfSoftBefore
		}
	}
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
// for each line (a cluster range and the reason it ends). Content that
// fits in maxW stays on the line; at the first cluster that does not fit,
// the line ends at the last soft wrap opportunity, or else at the last
// emergency opportunity (any cluster boundary, with overflow-wrap
// anywhere, or break-word unless minContent), or else the cluster
// overflows. Hanging white space always fits. A text that ends with a
// forced break gets a last, empty line. Tab advances depend on the
// position in the line; they are written to tabs[i] when tabs is not nil.
func (l *Layout) breakLines(maxW float32, minContent bool, tabs []float32, emit func(c0, c1 int32, reason BreakReason)) {
	cl := l.clusters
	n := int32(len(cl))
	if n == 0 {
		emit(0, 0, BreakNone)
		return
	}
	limit := maxW + fitEpsilon
	wrap := l.para.WhiteSpace.wraps()
	emergency := wrap && (l.para.OverflowWrap == OverflowWrapAnywhere || (l.para.OverflowWrap == OverflowWrapBreakWord && !minContent))
	for start := int32(0); start < n; {
		x := float32(0)
		opp, emg := int32(-1), int32(-1)
		end, reason := n, BreakNone
		for i := start; i < n; i++ {
			c := &cl[i]
			if i > start {
				if c.flags&cfMustBefore != 0 {
					end, reason = i, BreakForced
					break
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
			if c.flags&cfTab != 0 {
				a = l.tabAdvance(c, x)
				if tabs != nil {
					tabs[i] = a
				}
			}
			nx := x + a
			if nx <= limit || a <= 0 || l.hangable(c.flags) {
				x = nx
				continue
			}
			if opp > start {
				end, reason = opp, BreakSoft
				break
			}
			if emg > start {
				end, reason = emg, BreakEmergency
				break
			}
			x = nx
		}
		if end == n && cl[n-1].flags&cfNewline != 0 {
			emit(start, end, BreakForced)
			emit(n, n, BreakNone)
			return
		}
		emit(start, end, reason)
		start = end
	}
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
// the advance that hangs.
func (l *Layout) lineWidth(c0, c1 int32, reason BreakReason, maxW float32, adv func(i int32) float32) (width float32, trail int32, hang float32) {
	cl := l.clusters
	trail = c1
	for trail > c0 && l.hangable(cl[trail-1].flags) {
		trail--
	}
	for i := c0; i < trail; i++ {
		width += adv(i)
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
// width for maxW 0 with minContent set, the max-content width for +Inf.
func (l *Layout) measure(maxW float32, minContent bool) float32 {
	tabs := make([]float32, len(l.clusters))
	adv := func(i int32) float32 {
		if l.clusters[i].flags&cfTab != 0 {
			return tabs[i]
		}
		return l.clusters[i].adv
	}
	var w float32
	l.breakLines(maxW, minContent, tabs, func(c0, c1 int32, reason BreakReason) {
		lw, _, _ := l.lineWidth(c0, c1, reason, maxW, adv)
		w = max(w, lw)
	})
	return w
}
