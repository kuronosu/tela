// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/grid/track_sizing.rs); see
// THIRD_PARTY.md.

package layout

import (
	"math"
	"sort"
)

// trackEstimate estimates the size of a track of the other axis while
// items are measured: the max sizing function's fixed value in the first
// column pass, the base size afterwards.
type trackEstimate func(t *gridTrack, basis opt) opt

func estimateFromMax(t *gridTrack, basis opt) opt { return t.max.definite(basis) }
func estimateFromBase(t *gridTrack, _ opt) opt    { return some(t.base) }

// trackSizer holds one run of the track sizing algorithm (CSS Grid 1
// section 11.3) in axis a.
type trackSizer struct {
	e        *Engine
	n        Node
	a        axis
	tracks   []gridTrack // the axis being sized
	other    []gridTrack
	items    []gridItem
	inner    optSize // the container's inner size, where known
	estimate trackEstimate
}

// sizeTracks runs the track sizing algorithm in axis a (Taffy's
// track_sizing_algorithm). minSize and maxSize are the container's inner
// min and max sizes in a; align and otherAlign the content alignment of a
// and of the other axis (normal resolved to stretch).
func (e *Engine) sizeTracks(n Node, a axis, minSize, maxSize opt, align, otherAlign Align, avail availSize, inner optSize,
	tracks, other []gridTrack, items []gridItem, estimate trackEstimate, hasBaseline bool) {
	ts := &trackSizer{e: e, n: n, a: a, tracks: tracks, other: other, items: items, inner: inner, estimate: estimate}

	// 11.4 Initialize track sizes.
	basis := inner.get(a).or(minSize)
	for i := range tracks {
		t := &tracks[i]
		t.base = t.min.definite(basis).or0()
		t.limit = t.max.definite(basis).unwrapOr(inf)
		if t.limit < t.base {
			t.limit = t.base
		}
	}

	// 11.5.1 Shim baseline-aligned items.
	if hasBaseline {
		ts.resolveBaselines()
	}

	// Fixed tracks are final already (an intrinsic min can still grow
	// beyond a fixed growth limit).
	final := true
	for i := range tracks {
		if tracks[i].base != tracks[i].limit || !tracks[i].min.definite(basis).ok {
			final = false
			break
		}
	}
	if final {
		return
	}

	// The content alignment of the other axis spaces its tracks, which
	// items measured in this axis see as extra size.
	adj := gutterAlignmentAdjustment(otherAlign, inner.get(otherAxis(a)), estimate, other)
	if len(other) > 3 {
		for i := 2; i < len(other); i += 2 {
			other[i].alignAdj = adj
		}
	}

	// 11.5 Resolve intrinsic track sizes.
	ts.resolveIntrinsic(avail.get(a))

	// 11.6 Maximize tracks.
	maximizeTracks(tracks, inner.get(a), avail.get(a))

	// The last two steps only expand into space from the container's own
	// size, not into any available space.
	var expansion AvailableSpace
	switch {
	case inner.get(a).ok:
		expansion = DefiniteSpace(inner.get(a).v)
	case avail.get(a).Kind == MinContentSpace:
		expansion = MinContentConstraint
	default:
		expansion = MaxContentConstraint
	}

	// 11.7 Expand flexible tracks.
	ts.expandFlexible(minSize, maxSize, expansion)

	// 11.8 Stretch auto tracks.
	if align.keyword() == AlignStretch {
		stretchAutoTracks(tracks, minSize, expansion)
	}
}

// gutterAlignmentAdjustment returns the space that the content alignment
// of an axis adds to each inner gutter, when the container's size and the
// tracks' sizes are known (Taffy's compute_alignment_gutter_adjustment).
func gutterAlignmentAdjustment(align Align, size opt, estimate trackEstimate, tracks []gridTrack) float32 {
	if len(tracks) <= 1 {
		return 0
	}
	var outer, innerW int
	switch align.keyword() {
	case AlignSpaceBetween:
		innerW = 1
	case AlignSpaceAround:
		outer, innerW = 1, 2
	case AlignSpaceEvenly:
		outer, innerW = 1, 1
	case AlignStretch:
	default:
		outer = 1
	}
	if innerW == 0 || !size.ok {
		return 0
	}
	var sum float32
	for i := range tracks {
		v := estimate(&tracks[i], size)
		if !v.ok {
			return 0
		}
		sum += v.v
	}
	free := fmax(0, size.v-sum)
	weighted := (len(tracks)-3)/2*innerW + 2*outer
	return float32(float32(free/float32(weighted)) * float32(innerW))
}

// resolveBaselines computes the baselines of the items taking part in
// baseline alignment in each row and their shims (only rows have
// baselines in Tela).
func (ts *trackSizer) resolveBaselines() {
	items := ts.items
	other := otherAxis(ts.a)
	sort.SliceStable(items, func(i, j int) bool { return items[i].lines(other).start < items[j].lines(other).start })
	for i := 0; i < len(items); {
		j := i + 1
		for j < len(items) && items[j].lines(other).start == items[i].lines(other).start {
			j++
		}
		row := items[i:j]
		i = j
		count := 0
		for k := range row {
			if row[k].participatesInBaseline() {
				count++
			}
		}
		if count <= 1 {
			continue
		}
		maxBase := float32(math.Inf(-1))
		for k := range row {
			it := &row[k]
			if !it.participatesInBaseline() {
				continue
			}
			out := ts.e.child(ts.n, it.node, input{
				run: performLayout, sizing: inherentSize, axis: axisBoth,
				knownDefinite: boolSize{true, true}, parent: ts.inner,
				avail: availSize{MinContentConstraint, MinContentConstraint},
			})
			h := out.size.h
			b := out.firstBaseline.unwrapOr(h)
			if it.st.OverflowY.isScrollContainer() {
				b = fmax(fmin(b, h), 0)
			}
			it.baseline = some(b + it.st.Margin.Top.resolveOrZero(ts.inner.w))
			maxBase = fmax(maxBase, it.baseline.v)
		}
		for k := range row {
			if it := &row[k]; it.participatesInBaseline() {
				it.baselineShim = maxBase - it.baseline.or0()
			}
		}
	}
}

// Contributions with margins (Taffy's IntrinsicSizeMeasurer).

func (ts *trackSizer) area(it *gridItem) optSize {
	if !it.hasArea {
		it.area = it.gridAreaSize(ts.a, ts.tracks, ts.other, ts.inner, ts.estimate)
		it.hasArea = true
	}
	return it.area
}

func (ts *trackSizer) minContent(it *gridItem) float32 {
	area := ts.area(it)
	avail := area
	avail.set(ts.a, none)
	return ts.e.minContentContribution(ts.n, it, ts.a, area, avail) + it.margins(avail.w).get(ts.a)
}

func (ts *trackSizer) maxContent(it *gridItem) float32 {
	area := ts.area(it)
	avail := area
	avail.set(ts.a, none)
	return ts.e.maxContentContribution(ts.n, it, ts.a, area, avail) + it.margins(avail.w).get(ts.a)
}

func (ts *trackSizer) minimum(it *gridItem) float32 {
	area := ts.area(it)
	avail := area
	avail.set(ts.a, none)
	v := it.minimum[ts.a]
	if !v.ok {
		v = some(ts.e.minimumContribution(ts.n, it, ts.a, ts.tracks, area, ts.inner))
		it.minimum[ts.a] = v
	}
	return v.v + it.margins(avail.w).get(ts.a)
}

// distribution says whether minimum or maximum contributions are being
// distributed (for distributing space beyond limits).
type distribution uint8

const (
	distMinimum distribution = iota
	distMaximum
)

// resolveIntrinsic resolves the intrinsic track sizes (11.5).
func (ts *trackSizer) resolveIntrinsic(avail AvailableSpace) {
	a := ts.a
	tracks := ts.tracks
	items := ts.items
	basis := ts.inner.get(a)

	// Items are processed by increasing span, the ones crossing flexible
	// tracks last, all together.
	sort.SliceStable(items, func(i, j int) bool {
		fi, fj := items[i].crossesFlex[a], items[j].crossesFlex[a]
		if fi != fj {
			return !fi
		}
		si, sj := items[i].span(a), items[j].span(a)
		if si != sj {
			return si < sj
		}
		return items[i].lines(a).start < items[j].lines(a).start
	})

	constrained := avail.Kind != Definite
	for off := 0; off < len(items); {
		span := items[off].span(a)
		isFlex := items[off].crossesFlex[a]
		end := len(items)
		if !isFlex {
			for k := off; k < len(items); k++ {
				if items[k].crossesFlex[a] || items[k].span(a) > span {
					end = k
					break
				}
			}
		}
		batch := items[off:end]
		off = end

		if !isFlex && span == 1 {
			ts.sizeSingleSpan(batch, constrained)
			continue
		}

		// 1. Intrinsic minimums.
		for k := range batch {
			it := &batch[k]
			if !it.crossesIntrinsic[a] {
				continue
			}
			var space float32
			if constrained && !it.scrollsIn(a) {
				minimum := ts.minimum(it)
				limited := fmax(fminOpt(ts.minContent(it), it.spannedTrackLimit(a, tracks, basis)), minimum)
				if isFlex {
					// Items crossing flexible tracks give them the content
					// beyond the inflexible tracks in proportion to the
					// crossed flex factor sum, clamped at one (CSS Grid 2,
					// 12.5); the minimum contribution is a floor.
					inflexible, scale := flexShare(it.spanned(a, tracks))
					space = fmax(minimum, inflexible+float32(fmax(limited-inflexible, 0)*scale))
				} else {
					space = limited
				}
			} else {
				space = ts.minimum(it)
			}
			if space > 0 {
				affected := func(t *gridTrack) bool { return !t.min.definite(basis).ok }
				distributeToBase(isFlex, space, it.spanned(a, tracks), affected, ts.itemLimit(it), distMinimum, basis)
			}
		}
		flushBasePlanned(tracks)

		// 2. Content-based minimums.
		minOrMax := func(t *gridTrack) bool { return t.min.isMinOrMaxContent() }
		for k := range batch {
			it := &batch[k]
			if space := ts.minContent(it); space > 0 {
				distributeToBase(isFlex, space, it.spanned(a, tracks), minOrMax, ts.itemLimit(it), distMinimum, basis)
			}
		}
		flushBasePlanned(tracks)

		// 3. Max-content minimums, under a max-content constraint. Tracks
		// with a max-content min take the space before auto ones (as in
		// Chrome and Firefox).
		maxContentMin := func(t *gridTrack) bool { return t.min.kind == tfMaxContent }
		if avail.Kind == MaxContentSpace {
			autoMin := func(t *gridTrack) bool { return t.min.kind == tfAuto && t.max.kind != tfMinContent }
			for k := range batch {
				it := &batch[k]
				space := fminOpt(ts.maxContent(it), it.spannedTrackLimit(a, tracks, basis))
				sp := it.spanned(a, tracks)
				if isFlex {
					inflexible, scale := flexShare(sp)
					space = inflexible + float32(fmax(space-inflexible, 0)*scale)
				}
				if space <= 0 {
					continue
				}
				anyMax := false
				for i := range sp {
					anyMax = anyMax || maxContentMin(&sp[i])
				}
				if anyMax {
					distributeToBase(isFlex, space, sp, maxContentMin, func(*gridTrack) float32 { return inf }, distMaximum, basis)
				} else {
					distributeToBase(isFlex, space, sp, autoMin, func(t *gridTrack) float32 { return t.fitContentLimitedGrowthLimit(basis) }, distMaximum, basis)
				}
			}
			flushBasePlanned(tracks)
		}

		// In all cases, max-content mins take max-content contributions.
		for k := range batch {
			it := &batch[k]
			if space := ts.maxContent(it); space > 0 {
				distributeToBase(isFlex, space, it.spanned(a, tracks), maxContentMin, func(t *gridTrack) float32 { return t.limit }, distMaximum, basis)
			}
		}
		flushBasePlanned(tracks)

		// 4. Growth limits are at least the base sizes.
		for i := range tracks {
			if tracks[i].limit < tracks[i].base {
				tracks[i].limit = tracks[i].base
			}
		}

		// Flexible tracks have no intrinsic max: steps 5 and 6 do not
		// apply.
		if isFlex {
			continue
		}

		// 5. Intrinsic maximums.
		intrinsicMax := func(t *gridTrack) bool { return !t.max.hasDefinite(basis) }
		for k := range batch {
			it := &batch[k]
			if space := ts.minContent(it); space > 0 {
				distributeToLimit(space, it.spanned(a, tracks), intrinsicMax, basis)
			}
		}
		flushLimitPlanned(tracks, true)

		// 6. Max-content maximums.
		maxContentMax := func(t *gridTrack) bool {
			return t.max.isMaxContentAlike() || (t.max.usesPercentage() && !basis.ok)
		}
		for k := range batch {
			it := &batch[k]
			if space := ts.maxContent(it); space > 0 {
				distributeToLimit(space, it.spanned(a, tracks), maxContentMax, basis)
			}
		}
		flushLimitPlanned(tracks, false)
	}

	// Tracks still without a growth limit (no items, or flexible) take
	// their base size, so maximizing does not grow them.
	for i := range tracks {
		if tracks[i].limit == inf {
			tracks[i].limit = tracks[i].base
		}
	}
}

// itemLimit returns the limit of base size increases from an item: a
// scroll container's is the fit-content limited growth limit.
func (ts *trackSizer) itemLimit(it *gridItem) func(*gridTrack) float32 {
	basis := ts.inner.get(ts.a)
	if it.scrollsIn(ts.a) {
		return func(t *gridTrack) float32 { return t.fitContentLimitedGrowthLimit(basis) }
	}
	return func(t *gridTrack) float32 { return t.limit }
}

// flexShare returns the base sizes of the inflexible tracks of a span and
// the crossed flex factor sum clamped at one.
func flexShare(sp []gridTrack) (float32, float32) {
	var inflexible, ff float32
	for i := range sp {
		if sp[i].isFlexible() {
			ff += sp[i].flexFactor()
		} else {
			inflexible += sp[i].base
		}
	}
	return inflexible, fmin(ff, 1)
}

// sizeSingleSpan sizes the tracks of items spanning one track and no
// flexible track (11.5 step 2).
func (ts *trackSizer) sizeSingleSpan(batch []gridItem, constrained bool) {
	a := ts.a
	tracks := ts.tracks
	basis := ts.inner.get(a)
	for k := range batch {
		it := &batch[k]
		ti := it.indexes(a).start + 1
		t := &tracks[ti]

		base := t.base
		switch t.min.kind {
		case tfMinContent:
			base = fmax(base, ts.minContent(it))
		case tfMaxContent:
			base = fmax(base, ts.maxContent(it))
		case tfFixed:
			// A percentage of an indefinite container is min-content (as
			// in Chrome).
			if t.min.hasPct && !basis.ok {
				base = fmax(base, ts.minContent(it))
			}
		case tfAuto:
			// Under a min- or max-content constraint, the limited
			// min-content contribution, except for scroll containers,
			// whose automatic minimum (zero) wins (as in browsers).
			var space float32
			if constrained && !it.scrollsIn(a) {
				minimum := ts.minimum(it)
				space = fmax(fminOpt(ts.minContent(it), t.max.definiteLimit(basis)), minimum)
			} else {
				space = ts.minimum(it)
			}
			base = fmax(base, space)
		}
		var minContentLimit opt
		if !it.scrollsIn(a) {
			minContentLimit = some(ts.minContent(it))
		}
		maxContent := ts.maxContent(it)
		minContent := ts.minContent(it)
		t = &tracks[ti]
		t.base = base

		switch {
		case t.max.kind == tfFitContent:
			if minContentLimit.ok {
				t.limitPlanned = fmax(t.limitPlanned, minContentLimit.v)
			}
			t.limitPlanned = fmax(t.limitPlanned, fmin(maxContent, t.fitContentLimit(basis)))
			t.limitSet = true
		case t.max.isMaxContentAlike() || (t.max.usesPercentage() && !basis.ok):
			// A percentage of an indefinite container is auto (as in
			// Chrome).
			t.limitPlanned = fmax(t.limitPlanned, maxContent)
			t.limitSet = true
		case t.max.isIntrinsic():
			t.limitPlanned = fmax(t.limitPlanned, minContent)
			t.limitSet = true
		}
	}
	for i := range tracks {
		t := &tracks[i]
		// A track with items gets a finite growth limit, even when their
		// contributions are zero (as in Chromium; Taffy keeps it
		// infinite, which lets later spanning items grow it).
		if t.limitSet {
			t.limitSet = false
			if t.limit == inf {
				t.limit = t.limitPlanned
			} else {
				t.limit = fmax(t.limit, t.limitPlanned)
			}
		}
		t.infGrowable = false
		t.limitPlanned = 0
		if t.limit < t.base {
			t.limit = t.base
		}
	}
}

func flushBasePlanned(tracks []gridTrack) {
	for i := range tracks {
		tracks[i].base += tracks[i].basePlanned
		tracks[i].basePlanned = 0
	}
}

func flushLimitPlanned(tracks []gridTrack, setInfGrowable bool) {
	for i := range tracks {
		t := &tracks[i]
		if t.limitPlanned > 0 {
			if t.limit == inf {
				t.limit = t.base + t.limitPlanned
			} else {
				t.limit += t.limitPlanned
			}
			t.infGrowable = setInfGrowable
		} else {
			t.infGrowable = false
		}
		t.limitPlanned = 0
	}
}

// distributeToBase distributes an item's contribution over the base sizes
// of the tracks it spans (11.5.1). Items crossing flexible tracks give it
// to the flexible affected tracks, by flex factor when their sum is not
// zero (Chrome sums the affected tracks only and uses pure ratios).
func distributeToBase(isFlex bool, space float32, sp []gridTrack, affected func(*gridTrack) bool, limit func(*gridTrack) float32, kind distribution, basis opt) {
	if isFlex {
		flexAffected := func(t *gridTrack) bool { return t.isFlexible() && affected(t) }
		var sum float32
		for i := range sp {
			if flexAffected(&sp[i]) {
				sum += sp[i].flexFactor()
			}
		}
		if sum > 0 {
			distributeToBaseInner(space, sp, flexAffected, (*gridTrack).flexFactor, limit, kind, basis)
		} else {
			distributeToBaseInner(space, sp, flexAffected, func(*gridTrack) float32 { return 1 }, limit, kind, basis)
		}
		return
	}
	distributeToBaseInner(space, sp, affected, func(*gridTrack) float32 { return 1 }, limit, kind, basis)
}

func distributeToBaseInner(space float32, sp []gridTrack, affected func(*gridTrack) bool, proportion, limit func(*gridTrack) float32, kind distribution, basis opt) {
	any := false
	for i := range sp {
		any = any || affected(&sp[i])
	}
	if space == 0 || !any {
		return
	}
	base := func(t *gridTrack) float32 { return t.base }

	// 1. The space to distribute.
	var sizes float32
	for i := range sp {
		sizes += sp[i].base
	}
	extra := fmax(0, space-sizes)

	// 2. Distribute up to the limits.
	extra = distributeUpToLimits(extra, sp, affected, proportion, base, limit)

	// 3. Distribute beyond the limits: to the affected tracks with an
	// intrinsic max (minimums) or a max-content max (maximums; auto and
	// fit-content() count as max-content), else to all affected tracks.
	// fit-content() arguments cap the growth from max-content
	// contributions only. (Chromium; Taffy leaves auto out and caps
	// both.)
	if extra > 0.000001 {
		filter := func(t *gridTrack) bool { return t.max.isIntrinsic() }
		capFn := func(*gridTrack) float32 { return inf }
		if kind == distMaximum {
			filter = func(t *gridTrack) bool { return t.max.isMaxContentAlike() }
			capFn = func(t *gridTrack) float32 { return t.fitContentLimit(basis) }
		}
		n := 0
		for i := range sp {
			if affected(&sp[i]) && filter(&sp[i]) {
				n++
			}
		}
		if n == 0 {
			filter = func(*gridTrack) bool { return true }
		}
		distributeUpToLimits(extra, sp, func(t *gridTrack) bool { return affected(t) && filter(t) }, proportion, base, capFn)
	}

	// 4. The planned increase is the largest item-incurred increase.
	for i := range sp {
		t := &sp[i]
		if t.incurred > t.basePlanned {
			t.basePlanned = t.incurred
		}
		t.incurred = 0
	}
}

// distributeToLimit distributes an item's contribution over the growth
// limits of the tracks it spans (11.5.1, simplified for growth limits).
func distributeToLimit(space float32, sp []gridTrack, affected func(*gridTrack) bool, basis opt) {
	n := 0
	for i := range sp {
		if affected(&sp[i]) {
			n++
		}
	}
	if space == 0 || n == 0 {
		return
	}
	limitOrBase := func(t *gridTrack) float32 {
		if t.limit == inf {
			return t.base
		}
		return t.limit
	}
	var sizes float32
	for i := range sp {
		sizes += limitOrBase(&sp[i])
	}
	extra := fmax(0, space-sizes)

	// Growth limits are infinite or their own limit: all the space goes to
	// the growable tracks, if any.
	growable := func(t *gridTrack) bool {
		return affected(t) && (t.infGrowable || t.fitContentLimitedGrowthLimit(basis) == inf)
	}
	g := 0
	for i := range sp {
		if growable(&sp[i]) {
			g++
		}
	}
	if g > 0 {
		inc := extra / float32(g)
		for i := range sp {
			if growable(&sp[i]) {
				sp[i].incurred = inc
			}
		}
	} else {
		distributeUpToLimits(extra, sp, affected, func(*gridTrack) float32 { return 1 }, limitOrBase,
			func(t *gridTrack) float32 { return t.fitContentLimit(basis) })
	}
	for i := range sp {
		t := &sp[i]
		if t.incurred > t.limitPlanned {
			t.limitPlanned = t.incurred
		}
		t.incurred = 0
	}
}

// distributeUpToLimits distributes space to the affected tracks in
// proportion, each up to its limit, into their item-incurred increases.
// It returns the space left.
func distributeUpToLimits(space float32, tracks []gridTrack, affected func(*gridTrack) bool, proportion, property, limit func(*gridTrack) float32) float32 {
	// Stop below a small threshold (rounding errors), and after at most
	// one iteration per track (each freezes one at its limit; non-finite
	// inputs could otherwise loop).
	const threshold = 0.01
	for range len(tracks) + 1 {
		if space <= threshold {
			break
		}
		growing := func(t *gridTrack) bool { return affected(t) && property(t)+t.incurred < limit(t) }
		var psum float32
		minInc := inf
		found := false
		for i := range tracks {
			t := &tracks[i]
			if !growing(t) {
				continue
			}
			p := proportion(t)
			psum += p
			v := (limit(t) - property(t) - t.incurred) / p
			if !found || v < minInc || minInc != minInc {
				minInc = v
				found = true
			}
		}
		if psum == 0 {
			break
		}
		inc := fmin(minInc, space/psum)
		for i := range tracks {
			t := &tracks[i]
			if !affected(t) {
				continue
			}
			d := float32(inc * proportion(t))
			if d > 0 && property(t)+t.incurred+d <= limit(t)+threshold {
				t.incurred += d
				space -= d
			}
		}
	}
	return space
}

// computeFreeSpace returns the free space of an axis given its used space:
// infinite under a max-content constraint, 0 under a min-content one.
func computeFreeSpace(a AvailableSpace, used float32) float32 {
	switch a.Kind {
	case MaxContentSpace:
		return inf
	case MinContentSpace:
		return 0
	}
	return a.Value - used
}

// maximizeTracks distributes free space to the tracks up to their growth
// limits (11.6).
func maximizeTracks(tracks []gridTrack, basis opt, avail AvailableSpace) {
	var used float32
	for i := range tracks {
		used += tracks[i].base
	}
	free := computeFreeSpace(avail, used)
	switch {
	case free == inf:
		for i := range tracks {
			tracks[i].base = tracks[i].limit
		}
	case free > 0:
		distributeUpToLimits(free, tracks, func(*gridTrack) bool { return true }, func(*gridTrack) float32 { return 1 },
			func(t *gridTrack) float32 { return t.base }, func(t *gridTrack) float32 { return t.fitContentLimitedGrowthLimit(basis) })
		for i := range tracks {
			tracks[i].base += tracks[i].incurred
			tracks[i].incurred = 0
		}
	}
}

// expandFlexible sizes the flexible tracks with the largest fr that does
// not exceed the space (11.7).
func (ts *trackSizer) expandFlexible(minSize, maxSize opt, space AvailableSpace) {
	tracks := ts.tracks
	var frac float32
	switch space.Kind {
	case Definite:
		var used float32
		for i := range tracks {
			used += tracks[i].base
		}
		if space.Value-used > 0 {
			frac = findFrSize(tracks, space.Value)
		}
	case MinContentSpace:
		frac = 0
	default:
		// The largest of each flexible track's base size per fr and each
		// item's max-content contribution per fr of its tracks.
		var a float32
		for i := range tracks {
			t := &tracks[i]
			if !t.isFlexible() {
				continue
			}
			v := t.base
			if ff := t.flexFactor(); ff > 1 {
				v = t.base / ff
			}
			a = fmax(a, v)
		}
		var b float32
		for k := range ts.items {
			it := &ts.items[k]
			if !it.crossesFlex[ts.a] {
				continue
			}
			mc := ts.e.maxContentContribution(ts.n, it, ts.a, optSize{}, optSize{})
			b = fmax(b, findFrSize(it.spanned(ts.a, tracks), mc))
		}
		frac = fmax(a, b)

		// Too small for the min size or too large for the max size: redo
		// with that size as definite space (min wins).
		var hypothetical float32
		for i := range tracks {
			t := &tracks[i]
			if t.isFlexible() {
				hypothetical += fmax(t.base, float32(t.max.fr*frac))
			} else {
				hypothetical += t.base
			}
		}
		switch {
		case hypothetical < minSize.or0():
			frac = findFrSize(tracks, minSize.or0())
		case hypothetical > maxSize.unwrapOr(inf):
			frac = findFrSize(tracks, maxSize.v)
		}
	}
	for i := range tracks {
		t := &tracks[i]
		if t.isFlexible() {
			t.base = fmax(t.base, float32(t.max.fr*frac))
		}
	}
}

// findFrSize finds the largest size of an fr that does not exceed the
// space to fill (11.7.1).
func findFrSize(tracks []gridTrack, fill float32) float32 {
	if fill == 0 {
		return 0
	}
	// Each restart treats at least one more flexible track as inflexible;
	// the bound protects against non-finite inputs.
	hyp := inf
	for range len(tracks) + 1 {
		var used, ffSum float32
		for i := range tracks {
			t := &tracks[i]
			if t.isFlexible() && float32(t.max.fr*hyp) >= t.base {
				ffSum += t.max.fr
			} else {
				used += t.base
			}
		}
		leftover := fill - used
		prev := hyp
		hyp = leftover / fmax(ffSum, 1)
		valid := true
		for i := range tracks {
			t := &tracks[i]
			if t.isFlexible() && !(float32(t.max.fr*hyp) >= t.base || float32(t.max.fr*prev) < t.base) {
				valid = false
				break
			}
		}
		if valid {
			break
		}
	}
	return hyp
}

// stretchAutoTracks shares the remaining definite free space equally
// among the tracks with an auto max sizing function (11.8).
func stretchAutoTracks(tracks []gridTrack, minSize opt, space AvailableSpace) {
	n := 0
	var used float32
	for i := range tracks {
		if tracks[i].max.kind == tfAuto {
			n++
		}
		used += tracks[i].base
	}
	if n == 0 {
		return
	}
	var free float32
	switch {
	case space.Kind == Definite:
		free = computeFreeSpace(space, used)
	case minSize.ok:
		free = minSize.v - used
	}
	if free > 0 {
		per := free / float32(n)
		for i := range tracks {
			if tracks[i].max.kind == tfAuto {
				tracks[i].base += per
			}
		}
	}
}

// floor64 and ceil64 are math.Floor and math.Ceil on float32 values.
func floor64(v float32) float64 { return math.Floor(float64(v)) }
func ceil64(v float32) float64  { return math.Ceil(float64(v)) }
