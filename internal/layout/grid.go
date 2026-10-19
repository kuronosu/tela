// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/grid/mod.rs); see THIRD_PARTY.md.
// Not ported: box-sizing: content-box, subgrid, masonry, layout
// containment and detailed grid information.
//
// Absolutely positioned children follow Tela's containing blocks
// (absolute.go): a positioned grid lays them out in their grid area (or
// its padding box for auto lines) with CSS 2's rules; a static grid
// records their static position (CSS Grid 1 section 9.2: the content box
// as a grid area, aligned by justify-self and align-self) for the nearest
// positioned ancestor. The container's first baseline is the first
// baseline of the first item of its first row (CSS Grid 1 section 10.8);
// Taffy uses that item's bottom edge unless it is baseline-aligned.

package layout

import "sort"

// emptyGrid is the template of a grid container without one.
var emptyGrid GridTemplate

// grid computes the layout of a grid container (Taffy's
// compute_grid_layout).
func (e *Engine) grid(n Node, st *Style, in input) output {
	known, parent, avail := in.known, in.parent, in.avail
	dir := st.Direction
	rtl := dir == RTL

	// 1. The available grid space.
	ratio := st.aspectRatio()
	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)
	pb := padding.add(border)
	pbSize := pb.sum()
	minSize := st.minSize().resolve(parent).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(parent).applyAspectRatio(ratio)
	var preferred optSize
	if in.sizing == inherentSize {
		preferred = st.size().resolve(parent).applyAspectRatio(ratio)
	}

	// A vertical scrollbar takes space on the inline-end side.
	gutter := st.scrollbarGutter()
	cbi := pb
	cbi.b += gutter.y
	if rtl {
		cbi.l += gutter.x
	} else {
		cbi.r += gutter.x
	}
	scrolls := st.isScrollContainer()

	alignContent := st.AlignContent
	if alignContent == AlignNormal {
		alignContent = AlignStretch
	}
	justifyContent := st.JustifyContent
	if justifyContent == AlignNormal {
		justifyContent = AlignStretch
	}

	styled := known.or(preferred)
	outer := styled.maybeClamp(minSize, maxSize).maybeMaxF(pbSize)

	// The size is all the caller wants and the styles determine it.
	if in.run == computeSize {
		if outer.bothSet() {
			return outerSize(size{outer.w.v, outer.h.v})
		}
		if in.axis == axisHorizontal && outer.w.ok {
			return outerSize(size{outer.w.v, 0})
		}
	}

	// An auto width under definite available space is fit-content:
	// min(max-content, max(min-content, available)), from the grid sized
	// under max-content and min-content constraints (CSS Grid 1 section
	// 12.1, CSS 2 10.3.7). Taffy sizes the tracks in the available space
	// directly.
	if !outer.w.ok && avail.w.Kind == Definite {
		space := avail.w.Value - st.Margin.resolveOrZero(parent.w).hsum()
		probe := func(s AvailableSpace) float32 {
			return e.layout(n, input{run: computeSize, sizing: in.sizing, axis: axisHorizontal,
				known: known, knownDefinite: in.knownDefinite, parent: parent, avail: availSize{s, avail.h}}).size.w
		}
		w := probe(MaxContentConstraint)
		if w > space {
			w = fmax(probe(MinContentConstraint), space)
		}
		w = fmax(fclampOpt(w, minSize.w, maxSize.w), pbSize.w)
		styled.w, outer.w = some(w), some(w)
		if in.run == computeSize {
			if outer.bothSet() {
				return outerSize(size{outer.w.v, outer.h.v})
			}
			if in.axis == axisHorizontal {
				return outerSize(size{w, 0})
			}
		}
	}

	// The available grid space (CSS Grid 1 section 11.1): the content box
	// where the size is known, else the sizing constraint; an auto height
	// is a max-content constraint (Taffy uses the available space).
	constrained := avail
	if styled.w.ok {
		constrained.w = DefiniteSpace(styled.w.v)
	}
	if styled.h.ok {
		constrained.h = DefiniteSpace(styled.h.v)
	} else if avail.h.Kind == Definite {
		constrained.h = MaxContentConstraint
	}
	constrained.w = constrained.w.maybeClamp(minSize.w, maxSize.w).maxF(pbSize.w)
	constrained.h = constrained.h.maybeClamp(minSize.h, maxSize.h).maxF(pbSize.h)
	availGrid := availSize{constrained.w.subF(cbi.hsum()), constrained.h.subF(cbi.vsum())}

	innerMin := minSize.maybeSubF(cbi.sum())
	innerMax := maxSize.maybeSubF(cbi.sum())
	inner := outer.maybeSubF(cbi.sum())

	// The in-flow children (absolutely positioned ones take no part in
	// placement).
	count := n.LayoutChildCount()
	children := make([]gridChild, 0, count) // perf: reuse a per-depth buffer
	for i := 0; i < count; i++ {
		ch := n.LayoutChild(i)
		cst := ch.LayoutStyle()
		ch.LayoutData().parent = n
		if cst.Display == DisplayNone || cst.Position == PositionAbsolute {
			continue
		}
		children = append(children, gridChild{node: ch, st: cst, index: i})
	}

	// 2. The explicit grid. The container's size, else its max size, sets
	// the most auto repetitions that fit; else its min size the fewest that
	// fill it.
	g := st.Grid
	if g == nil {
		g = &emptyGrid
	}
	fitSize := outer.or(maxSize).or(minSize).maybeClamp(minSize, maxSize).maybeMaxF(pbSize).maybeSubF(cbi.sum())
	colReps, colCount := explicitGridSize(&g.Columns, st.ColumnGap, fitSize.w, outer.w.or(maxSize.w).ok)
	rowReps, rowCount := explicitGridSize(&g.Rows, st.RowGap, fitSize.h, outer.h.or(maxSize.h).ok)
	names := newNamedResolver(g, colReps, rowReps)
	explicitCols := min(max(colCount, g.AreaColumns), maxGridTracks)
	explicitRows := min(max(rowCount, g.AreaRows), maxGridTracks)
	names.cols.explicit, names.rows.explicit = explicitCols, explicitRows

	// 3. Estimate the implicit grid; 4. place the items.
	estCols, estRows := gridSizeEstimate(explicitCols, explicitRows, children)
	occ := newOccupancy(estCols, estRows)
	placements := make([][2]ozPair, len(children))
	for i := range children {
		placements[i] = [2]ozPair{names.cols.resolve(children[i].st.GridColumn), names.rows.resolve(children[i].st.GridRow)}
	}
	alignItems, justifyItems := st.AlignItems, st.JustifyItems
	stretchIfNormal := func(a Align) Align {
		if a == AlignNormal {
			return AlignStretch
		}
		return a
	}
	items := placeItems(occ, children, placements, st.GridAutoFlow, stretchIfNormal(alignItems), stretchIfNormal(justifyItems))
	colCounts, rowCounts := occ.cols, occ.rows

	// 5. Initialize the tracks and gutters.
	columns := initTracks(nil, colCounts, &g.Columns, g.AutoColumns, st.ColumnGap, colReps, occ.colOccupied)
	rows := initTracks(nil, rowCounts, &g.Rows, g.AutoRows, st.RowGap, rowReps, occ.rowOccupied)

	// 6. Track sizing.
	hasBaseline := false
	for i := range items {
		it := &items[i]
		cs, _ := colCounts.vecIndex(it.col.start)
		ce, _ := colCounts.vecIndex(it.col.end)
		rs, _ := rowCounts.vecIndex(it.row.start)
		re, _ := rowCounts.vecIndex(it.row.end)
		it.colIdx, it.rowIdx = lineSpan{cs, ce}, lineSpan{rs, re}
		for _, t := range it.spanned(horizontal, columns) {
			it.crossesFlex[horizontal] = it.crossesFlex[horizontal] || t.isFlexible()
			it.crossesIntrinsic[horizontal] = it.crossesIntrinsic[horizontal] || t.hasIntrinsic()
		}
		for _, t := range it.spanned(vertical, rows) {
			it.crossesFlex[vertical] = it.crossesFlex[vertical] || t.isFlexible()
			it.crossesIntrinsic[vertical] = it.crossesIntrinsic[vertical] || t.hasIntrinsic()
		}
		hasBaseline = hasBaseline || it.participatesInBaseline()
	}

	// Items whose width depends on their height (an aspect ratio and an
	// auto width) are measured with an estimate of the rows: the rows
	// sized first with the columns at their fixed max sizes, as Chromium
	// does. Otherwise rows count as infinite unless their max size is
	// fixed (CSS Grid 1 section 12.1, Taffy).
	rowEstimate := estimateFromMax
	for i := range items {
		if dependsOnBlockSize(items[i].st) {
			e.sizeTracks(n, vertical, innerMin.h, innerMax.h, alignContent, justifyContent, availGrid, inner, rows, columns, items, estimateFromMax, false)
			clearItemCaches(items, vertical)
			rowEstimate = estimateFromBase
			break
		}
	}
	e.sizeTracks(n, horizontal, innerMin.w, innerMax.w, justifyContent, alignContent, availGrid, inner, columns, rows, items, rowEstimate, hasBaseline)
	colSum := sumBase(columns)
	inner.w = inner.w.or(some(colSum))
	for i := range items {
		items[i].hasArea = false
	}
	e.sizeTracks(n, vertical, innerMin.h, innerMax.h, alignContent, justifyContent, availGrid, inner, rows, columns, items, estimateFromBase, false)
	rowSum := sumBase(rows)
	inner.h = inner.h.or(some(rowSum))

	// The container's size.
	borderBox := size{
		w: fmax(fclampOpt(styled.w.unwrapOr(colSum+cbi.hsum()), minSize.w, maxSize.w), pbSize.w),
		h: fmax(fclampOpt(styled.h.unwrapOr(rowSum+cbi.vsum()), minSize.h, maxSize.h), pbSize.h),
	}
	content := size{fmax(0, borderBox.w-cbi.hsum()), fmax(0, borderBox.h-cbi.vsum())}

	// 7. Percentage tracks of a container sized by its content resolve
	// against the content box now.
	if !availGrid.w.isDefinite() {
		resolvePercentTracks(columns, content.w)
	}
	if !availGrid.h.isDefinite() {
		resolvePercentTracks(rows, content.h)
	}

	// Columns are sized again (once) when the container's width was
	// indefinite and a column uses a percentage, or when the min-content
	// contribution of an item crossing an intrinsic column changed; rows
	// likewise after them. Size queries do this too (Taffy returns before
	// it; Chromium's intrinsic sizes include it).
	hasPctCol, hasPctRow := usesPercentage(columns), usesPercentage(rows)
	rerunCols := !avail.w.isDefinite() && hasPctCol
	colChanged, rowChanged := false, false
	if !rerunCols {
		colChanged = e.intrinsicContributionChanged(n, horizontal, items, columns, rows, inner, func(it *gridItem) bool { return it.crossesIntrinsic[horizontal] })
		rerunCols = colChanged
	} else {
		clearItemCaches(items, horizontal)
	}
	if rerunCols {
		e.sizeTracks(n, horizontal, innerMin.w, innerMax.w, justifyContent, alignContent, availGrid, inner, columns, rows, items, estimateFromBase, hasBaseline)
		rerunRows := (!avail.h.isDefinite() || !styled.h.ok) && hasPctRow
		if !rerunRows {
			// Taffy checks the items crossing an intrinsic column here too.
			rowChanged = e.intrinsicContributionChanged(n, vertical, items, rows, columns, inner, func(it *gridItem) bool { return it.crossesIntrinsic[horizontal] })
			rerunRows = rowChanged
		} else {
			clearItemCaches(items, vertical)
		}
		if rerunRows {
			e.sizeTracks(n, vertical, innerMin.h, innerMax.h, alignContent, justifyContent, availGrid, inner, rows, columns, items, estimateFromBase, false)
		}
	}
	if colChanged && !hasPctCol {
		borderBox.w = fmax(fclampOpt(styled.w.unwrapOr(sumBase(columns)+cbi.hsum()), minSize.w, maxSize.w), pbSize.w)
		content.w = fmax(0, borderBox.w-cbi.hsum())
	}
	if rowChanged && !hasPctRow {
		borderBox.h = fmax(fclampOpt(styled.h.unwrapOr(sumBase(rows)+cbi.vsum()), minSize.h, maxSize.h), pbSize.h)
		content.h = fmax(0, borderBox.h-cbi.vsum())
	}
	if in.run == computeSize {
		return outerSize(borderBox)
	}

	// 8. Track alignment. In RTL the columns are laid out right to left
	// and the vertical scrollbar is on the left.
	colOrigin := padding.l
	if rtl {
		colOrigin += fmin(gutter.x, fmax(borderBox.w-pbSize.w, 0))
	}
	alignTracks(content.w, colOrigin, border.l, columns, justifyContent, rtl)
	alignTracks(content.h, padding.t, border.t, rows, alignContent, false)

	// 9. Size, align and position the items, in tree order.
	sort.Slice(items, func(i, j int) bool { return items[i].order < items[j].order })
	gs := &gridItemSizing{justifyItems: justifyItems, alignItems: alignItems, dir: dir, containerW: borderBox.w, border: border, scrolls: scrolls}
	var of rect
	escapes := false
	for i := range items {
		it := &items[i]
		area := rect{
			t: rows[it.rowIdx.start+1].offset,
			b: rows[it.rowIdx.end].offset,
		}
		if rtl {
			area.l, area.r = columns[it.colIdx.end-1].offset, columns[it.colIdx.start].offset
		} else {
			area.l, area.r = columns[it.colIdx.start+1].offset, columns[it.colIdx.end].offset
		}
		c, y, h, out := e.positionGridItem(n, it.node, it.st, area, it.baselineShim, gs)
		it.y, it.height = y, h
		it.baseline = out.firstBaseline
		if it.st.OverflowY.isScrollContainer() && it.baseline.ok {
			it.baseline.v = fmax(fmin(it.baseline.v, h), 0)
		}
		of = of.union(c)
		escapes = escapes || (out.escapes && it.st.passesAbs())
	}

	// Absolutely positioned children; fixed ones are laid out by their
	// fixed containing block (fixed.go).
	cb := st.absContainer(n.LayoutData().parent == nil)
	var absOf rect
	for i := 0; i < count; i++ {
		ch := n.LayoutChild(i)
		cst := ch.LayoutStyle()
		if cst.Display == DisplayNone || cst.Position != PositionAbsolute {
			continue
		}
		if !cb || cst.Fixed {
			// The static position: the content box as the grid area.
			box := rect{l: cbi.l, r: borderBox.w - cbi.r, t: cbi.t, b: borderBox.h - cbi.b}
			ch.LayoutData().static = gridStaticPos(box, cst, gs, false)
			escapes = escapes || !cst.Fixed
			continue
		}
		area := gridAbsoluteArea(names, colCounts, rowCounts, columns, rows, cst, borderBox, border, gutter, rtl)
		sp := gridStaticPos(area, cst, gs, true)
		loc, final, out := e.placeAbsolute(n, ch, cst, size{area.r - area.l, area.b - area.t}, point{area.l, area.t}, sp, dir, point{})
		cl := point{loc.x - border.l, loc.y - border.t}
		if rtl {
			cl.x = borderBox.w - (loc.x + final.w) - border.r
		}
		absOf = absOf.union(overflowContribution(cl, final, out.overflow, cst, scrolls))
	}
	if cb && escapes {
		in := border
		in.b += gutter.y
		if rtl {
			in.l += gutter.x
		} else {
			in.r += gutter.x
		}
		absOf = absOf.union(e.layoutEscaped(n, borderBox.sub(in.sum()), point{in.l, in.t}, dir, scrolls))
		escapes = false
	}

	e.hideChildren(n)

	// A scroll container's end padding is part of its scrollable overflow.
	if scrolls {
		if rtl {
			of.r += padding.l
		} else {
			of.r += padding.r
		}
		of.b += padding.b
	}
	of = of.union(absOf)

	baseline := gridBaseline(items)
	return output{size: borderBox, overflow: of, firstBaseline: baseline, lastBaseline: baseline, escapes: escapes}
}

// dependsOnBlockSize reports whether an item's width depends on its
// height: it has an aspect ratio and no width of its own.
func dependsOnBlockSize(st *Style) bool {
	if !st.aspectRatio().ok {
		return false
	}
	switch st.Width.Unit {
	case UnitPx, UnitPercent, UnitCalc:
		return false
	}
	return true
}

func sumBase(tracks []gridTrack) float32 {
	var s float32
	for i := range tracks {
		s += tracks[i].base
	}
	return s
}

func usesPercentage(tracks []gridTrack) bool {
	for i := range tracks {
		if tracks[i].usesPercentage() {
			return true
		}
	}
	return false
}

// resolvePercentTracks clamps the base sizes of percentage tracks, which
// were sized as auto, by their percentages of the content box.
func resolvePercentTracks(tracks []gridTrack, content float32) {
	for i := range tracks {
		t := &tracks[i]
		t.base = fclampOpt(t.base, t.min.percentOf(content), t.max.percentOf(content))
	}
}

// clearItemCaches drops the intrinsic size caches of axis a.
func clearItemCaches(items []gridItem, a axis) {
	for i := range items {
		it := &items[i]
		it.hasArea = false
		it.minContent[a], it.maxContent[a], it.minimum[a] = none, none, none
	}
}

// intrinsicContributionChanged measures again the min-content contribution
// in axis a of the selected items with the final sizes of the other axis,
// updating their caches; it stops at the first one that changed (Taffy's
// any()).
func (e *Engine) intrinsicContributionChanged(n Node, a axis, items []gridItem, tracks, other []gridTrack, inner optSize, sel func(*gridItem) bool) bool {
	for i := range items {
		it := &items[i]
		if !sel(it) {
			continue
		}
		area := it.gridAreaSize(a, tracks, other, inner, estimateFromBase)
		avail := area
		avail.set(a, none)
		v := e.measureGridItem(n, it, a, area, avail, MinContentConstraint)
		changed := !it.minContent[a].ok || it.minContent[a].v != v
		it.area, it.hasArea = area, true
		it.minContent[a] = some(v)
		it.maxContent[a], it.minimum[a] = none, none
		if changed {
			return true
		}
	}
	return false
}

// gridBaseline returns the container's first baseline: the shared
// baseline of the baseline-aligned items of the first row, else the first
// baseline of the first item of that row (its bottom edge if it has none).
func gridBaseline(items []gridItem) opt {
	if len(items) == 0 {
		return none
	}
	first := items[0].row.start
	for i := range items {
		first = min(first, items[i].row.start)
	}
	var pick *gridItem
	for i := range items {
		it := &items[i]
		if it.row.start != first {
			continue
		}
		if it.participatesInBaseline() {
			pick = it
			break
		}
		if pick == nil || it.col.start < pick.col.start {
			pick = it
		}
	}
	return some(pick.y + pick.baseline.unwrapOr(pick.height))
}

// gridStaticPos returns the static position of an absolutely positioned
// child in area (border-box coordinates): aligned by its justify-self and
// align-self (or the container's justify-items and align-items); normal,
// stretch and baseline are start. safe says whether safe alignment applies
// (Chromium applies it when the container is the containing block).
func gridStaticPos(area rect, st *Style, gs *gridItemSizing, safe bool) staticPos {
	rtl := gs.dir == RTL
	mode := func(own, container Align, inline bool) (alignMode, bool) {
		a := own
		if a == AlignNormal {
			a = container
		}
		a = resolveSelfAlign(a, st.Direction, gs.dir, inline)
		flip := inline && rtl
		switch a.keyword() {
		case AlignEnd, AlignFlexEnd:
			return startEnd(flip), a.isSafe()
		case AlignCenter:
			return alignCenterMode, a.isSafe()
		}
		return startEnd(!flip), a.isSafe()
	}
	mx, sx := mode(st.JustifySelf, gs.justifyItems, true)
	my, sy := mode(st.AlignSelf, gs.alignItems, false)
	return staticPos{
		x0: area.l, x1: area.r, y0: area.t, y1: area.b,
		modeX: mx, modeY: my,
		safeX: safe && sx, safeY: safe && sy,
		fallbackX: startEnd(!rtl), fallbackY: alignStartMode,
	}
}

// gridAbsoluteArea returns the grid area of an absolutely positioned
// child of a grid that is its containing block, in border-box coordinates:
// its lines resolve to the edges of the tracks next to them; auto lines
// and lines outside the grid to the padding box.
func gridAbsoluteArea(names namedResolver, colCounts, rowCounts trackCounts, columns, rows []gridTrack, st *Style, bb size, border rect, gutter point, rtl bool) rect {
	idx := func(l int, ok bool, c trackCounts) (int, bool) {
		if !ok {
			return 0, false
		}
		return c.vecIndex(l)
	}
	cs, ce, csOK, ceOK := names.cols.resolve(st.GridColumn).resolveAbsolute()
	rs, re, rsOK, reOK := names.rows.resolve(st.GridRow).resolveAbsolute()
	ci0, ci0OK := idx(cs, csOK, colCounts)
	ci1, ci1OK := idx(ce, ceOK, colCounts)
	ri0, ri0OK := idx(rs, rsOK, rowCounts)
	ri1, ri1OK := idx(re, reOK, rowCounts)

	var a rect
	if rtl {
		// Columns are offset right to left: a start line is the physical
		// right edge of the track after it, an end line the left edge of
		// the track before it.
		a.l, a.r = border.l+gutter.x, bb.w-border.r
		if ci1OK {
			a.l = rtlLineAsEnd(columns, ci1)
		}
		if ci0OK {
			a.r = rtlLineAsStart(columns, ci0)
		}
	} else {
		a.l, a.r = border.l, bb.w-border.r-gutter.x
		if ci0OK {
			a.l = lineAsStart(columns, ci0)
		}
		if ci1OK {
			a.r = lineAsEnd(columns, ci1)
		}
	}
	a.t, a.b = border.t, bb.h-border.b-gutter.y
	if ri0OK {
		a.t = lineAsStart(rows, ri0)
	}
	if ri1OK {
		a.b = lineAsEnd(rows, ri1)
	}
	return a
}

// lineAsStart resolves a line used as a start edge to the start of the
// track after it (past content alignment space in the gutter).
func lineAsStart(tracks []gridTrack, i int) float32 {
	if i+1 < len(tracks) {
		return tracks[i+1].offset
	}
	return tracks[i].offset
}

// lineAsEnd resolves a line used as an end edge to the end of the track
// before it.
func lineAsEnd(tracks []gridTrack, i int) float32 {
	if i == 0 {
		if len(tracks) > 1 {
			return tracks[1].offset
		}
		return tracks[0].offset
	}
	return tracks[i].offset
}

func rtlLineAsStart(tracks []gridTrack, i int) float32 {
	switch {
	case i+1 < len(tracks):
		return tracks[i].offset
	case i == 0:
		return tracks[0].offset
	}
	return tracks[i-1].offset
}

func rtlLineAsEnd(tracks []gridTrack, i int) float32 {
	if i == 0 {
		return tracks[0].offset
	}
	return tracks[i-1].offset
}
