// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/grid/types/grid_item.rs); see
// THIRD_PARTY.md.
//
// The automatic minimum size of an item (CSS Grid 1 section 6.6) looks at
// the tracks the item spans, as the spec says; Taffy looks at every track
// of the axis.

package layout

// gridChild is an in-flow child of a grid container.
type gridChild struct {
	node  Node
	st    *Style
	index int // among all children: the placement order of equal items
}

// gridItem is an item during grid layout. Axes are absolute: horizontal is
// the inline axis (columns), vertical the block axis (rows).
type gridItem struct {
	node  Node
	st    *Style
	order int

	// col and row are the item's lines (origin-zero); colIdx and rowIdx
	// the same lines as track vector indexes.
	col, row       lineSpan
	colIdx, rowIdx lineSpan

	// alignSelf and justifySelf are the item's alignment, the container's
	// align-items and justify-items when auto, stretch when both are
	// normal.
	alignSelf, justifySelf Align

	// baseline is the first baseline (from the top of the margin box) of
	// an item taking part in baseline alignment; baselineShim the extra
	// top margin that aligns it.
	baseline     opt
	baselineShim float32

	crossesFlex, crossesIntrinsic [2]bool // per axis

	// Caches of one run of the track sizing algorithm, per axis.
	area                            optSize
	hasArea                         bool
	minContent, minimum, maxContent [2]opt

	// Final position and height, for the container's baseline.
	y, height float32
}

func newGridItem(c gridChild, col, row lineSpan, alignItems, justifyItems Align) gridItem {
	a := c.st.AlignSelf
	if a == AlignNormal {
		a = alignItems
	}
	j := c.st.JustifySelf
	if j == AlignNormal {
		j = justifyItems
	}
	return gridItem{node: c.node, st: c.st, order: c.index, col: col, row: row, alignSelf: a, justifySelf: j}
}

func (it *gridItem) lines(a axis) lineSpan {
	if a == horizontal {
		return it.col
	}
	return it.row
}

func (it *gridItem) indexes(a axis) lineSpan {
	if a == horizontal {
		return it.colIdx
	}
	return it.rowIdx
}

// trackRange returns the track vector range of the tracks (and inner
// gutters) the item spans in axis a.
func (it *gridItem) trackRange(a axis) (int, int) {
	ix := it.indexes(a)
	return ix.start + 1, ix.end
}

func (it *gridItem) spanned(a axis, tracks []gridTrack) []gridTrack {
	s, e := it.trackRange(a)
	return tracks[s:e]
}

func (it *gridItem) span(a axis) int { return it.lines(a).span() }

func overflowIn(st *Style, a axis) Overflow {
	if a == horizontal {
		return st.OverflowX
	}
	return st.OverflowY
}

func (it *gridItem) scrollsIn(a axis) bool { return overflowIn(it.st, a).isScrollContainer() }

// participatesInBaseline reports whether the item takes part in baseline
// alignment: align-self baseline, no auto block margin, and a block size
// that does not depend on its rows (CSS Grid 1 section 11.5, CSS Align 3
// section 9.3).
func (it *gridItem) participatesInBaseline() bool {
	cyclic := it.st.Height.Unit == UnitPercent || it.st.Height.Unit == UnitCalc
	cyclic = cyclic && (it.crossesIntrinsic[vertical] || it.crossesFlex[vertical])
	return it.alignSelf.keyword() == AlignBaseline && !it.st.Margin.Top.isAutoMargin() && !it.st.Margin.Bottom.isAutoMargin() && !cyclic
}

// spannedTrackLimit returns the sum of the fixed max sizing functions
// (fit-content arguments included) of the spanned tracks, if all have one:
// the limit of an item's limited contributions.
func (it *gridItem) spannedTrackLimit(a axis, tracks []gridTrack, basis opt) opt {
	var sum float32
	sp := it.spanned(a, tracks)
	for i := range sp {
		v := sp[i].max.definiteLimit(basis)
		if !v.ok {
			return none
		}
		sum += v.v
	}
	return some(sum)
}

// spannedFixedTrackLimit is spannedTrackLimit without fit-content
// arguments: the clamp of the automatic minimum contribution.
func (it *gridItem) spannedFixedTrackLimit(a axis, tracks []gridTrack, basis opt) opt {
	var sum float32
	sp := it.spanned(a, tracks)
	for i := range sp {
		v := sp[i].max.definite(basis)
		if !v.ok {
			return none
		}
		sum += v.v
	}
	return some(sum)
}

// margins returns the item's margin sums for size contributions: auto
// margins are 0, horizontal percentages are 0 (they would be cyclic),
// vertical ones resolve against width; the baseline shim adds to the top.
func (it *gridItem) margins(width opt) size {
	m := it.st.Margin
	return size{
		w: m.Left.resolveOrZero(some(0)) + m.Right.resolveOrZero(some(0)),
		h: m.Top.resolveOrZero(width) + it.baselineShim + m.Bottom.resolveOrZero(width),
	}
}

// knownDimensions returns the known dimensions passed when measuring the
// item: its size styles, the stretched size of the grid area where it
// stretches, clamped by its min and max sizes.
func (it *gridItem) knownDimensions(area optSize) optSize {
	st := it.st
	margins := it.margins(area.w)
	ratio := st.aspectRatio()
	inherent := st.size().resolve(area).applyAspectRatio(ratio)
	minSize := st.minSize().resolve(area).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(area).applyAspectRatio(ratio)
	minusMargins := area.maybeSubF(margins)

	w := inherent.w
	if !w.ok {
		switch {
		case st.Width.isSizingKeyword():
			if k, _, exact := resolveSizingKeyword(st.Width, minusMargins.w, area.w); k == keywordExact {
				w = some(exact)
			}
		case !st.Margin.Left.isAutoMargin() && !st.Margin.Right.isAutoMargin() && it.justifySelf.keyword() == AlignStretch:
			w = minusMargins.w
		}
	}
	k := optSize{w, inherent.h}.applyAspectRatio(ratio)
	if !k.h.ok {
		switch {
		case st.Height.isSizingKeyword():
			if kind, _, exact := resolveSizingKeyword(st.Height, minusMargins.h, area.h); kind == keywordExact {
				k.h = some(exact)
			}
		case !st.Margin.Top.isAutoMargin() && !st.Margin.Bottom.isAutoMargin() && it.alignSelf.keyword() == AlignStretch:
			k.h = minusMargins.h
		}
	}
	return k.applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
}

// gridAreaSize returns the size of the item's grid area as far as it is
// known while sizing axis a: in a, the sum of the spanned tracks if they
// all have a fixed size (min = max); in the other axis, the estimated
// sizes of the spanned tracks plus their content alignment adjustment.
func (it *gridItem) gridAreaSize(a axis, tracks, other []gridTrack, inner optSize, estimate func(*gridTrack, opt) opt) optSize {
	sum := func(ts []gridTrack, f func(*gridTrack) opt) opt {
		var s float32
		for i := range ts {
			v := f(&ts[i])
			if !v.ok {
				return none
			}
			s += v.v
		}
		return some(s)
	}
	basis := inner.get(a)
	inAxis := sum(it.spanned(a, tracks), func(t *gridTrack) opt {
		mn, mx := t.min.definite(basis), t.max.definite(basis)
		if mn.ok && mx.ok && mn.v == mx.v {
			return some(t.base)
		}
		return none
	})
	ob := inner.get(otherAxis(a))
	inOther := sum(it.spanned(otherAxis(a), other), func(t *gridTrack) opt {
		return estimate(t, ob).addF(t.alignAdj)
	})
	var s optSize
	s.set(a, inAxis)
	s.set(otherAxis(a), inOther)
	return s
}

// measure measures the item's size in axis a under the given available
// space (none: the min-content or max-content constraint of space).
func (e *Engine) measureGridItem(n Node, it *gridItem, a axis, area, avail optSize, space AvailableSpace) float32 {
	as := availSize{space, space}
	if avail.w.ok {
		as.w = DefiniteSpace(avail.w.v)
	}
	if avail.h.ok {
		as.h = DefiniteSpace(avail.h.v)
	}
	as = it.keywordAdjustedAvail(area, as)
	return e.child(n, it.node, input{
		run: computeSize, sizing: inherentSize, axis: axisRequest(a),
		known: it.knownDimensions(area), knownDefinite: boolSize{true, true},
		parent: area, avail: as,
	}).size.get(a)
}

// keywordAdjustedAvail replaces the available space of an axis whose size
// is a sizing keyword measured under a constraint.
func (it *gridItem) keywordAdjustedAvail(area optSize, as availSize) availSize {
	st := it.st
	if !st.Width.isSizingKeyword() && !st.Height.isSizingKeyword() {
		return as
	}
	margins := it.margins(area.w)
	if st.Width.isSizingKeyword() {
		if k, space, _ := resolveSizingKeyword(st.Width, area.w.subF(margins.w), area.w); k == keywordMeasure {
			as.w = space
		}
	}
	if st.Height.isSizingKeyword() {
		if k, space, _ := resolveSizingKeyword(st.Height, area.h.subF(margins.h), area.h); k == keywordMeasure {
			as.h = space
		}
	}
	return as
}

// minimumContribution returns the item's minimum contribution in axis a
// (CSS Grid 1 section 6.6): its preferred size, else its min size, else 0
// for a scroll container, else its automatic minimum size.
func (e *Engine) minimumContribution(n Node, it *gridItem, a axis, tracks []gridTrack, area, inner optSize) float32 {
	st := it.st
	ratio := st.aspectRatio()
	if v := st.size().resolve(area).applyAspectRatio(ratio).get(a); v.ok {
		return v.v
	}
	// The min size of the other axis does not transfer through the aspect
	// ratio here: an auto min size is the automatic minimum (Chromium;
	// Taffy transfers it).
	if v := st.minSize().resolve(area).get(a); v.ok {
		return v.v
	}
	if it.scrollsIn(a) {
		return 0
	}
	// The content-based minimum applies when the item spans a track with
	// an auto min sizing function and, spanning several tracks, none is
	// flexible.
	sp := it.spanned(a, tracks)
	autoMin, flexible := false, false
	for i := range sp {
		autoMin = autoMin || sp[i].min.kind == tfAuto
		flexible = flexible || sp[i].isFlexible()
	}
	if !autoMin || (len(sp) > 1 && flexible) {
		return 0
	}
	c := e.minContentContribution(n, it, a, area, area)
	// The content-based minimum is clamped by the fixed max sizing
	// functions of the spanned tracks.
	return fminOpt(c, it.spannedFixedTrackLimit(a, tracks, inner.get(a)))
}

// minContentContribution returns the item's min-content size in axis a
// (cached for the current track sizing run).
func (e *Engine) minContentContribution(n Node, it *gridItem, a axis, area, avail optSize) float32 {
	if v := it.minContent[a]; v.ok {
		return v.v
	}
	v := e.measureGridItem(n, it, a, area, avail, MinContentConstraint)
	it.minContent[a] = some(v)
	return v
}

// maxContentContribution returns the item's max-content size in axis a
// (cached for the current track sizing run).
func (e *Engine) maxContentContribution(n Node, it *gridItem, a axis, area, avail optSize) float32 {
	if v := it.maxContent[a]; v.ok {
		return v.v
	}
	v := e.measureGridItem(n, it, a, area, avail, MaxContentConstraint)
	it.maxContent[a] = some(v)
	return v
}

// optSize helpers by axis.

func (s optSize) get(a axis) opt {
	if a == horizontal {
		return s.w
	}
	return s.h
}

func (s *optSize) set(a axis, v opt) {
	if a == horizontal {
		s.w = v
	} else {
		s.h = v
	}
}

func (s availSize) get(a axis) AvailableSpace {
	if a == horizontal {
		return s.w
	}
	return s.h
}
