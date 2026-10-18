// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/grid/placement.rs and
// src/compute/grid/types/cell_occupancy.rs); see THIRD_PARTY.md.

package layout

// cellState is the occupancy of a grid cell.
type cellState uint8

const (
	cellFree cellState = iota
	cellDefinite
	cellAuto
)

// interval is a run of occupied cells of one track: origin-zero lines
// [start, end) along the track.
type interval struct {
	start, end int
	state      cellState
}

// trackIntervals are the occupied cells of one track: sorted, disjoint
// intervals; touching intervals of the same state are merged.
type trackIntervals []interval

// paint sets the cells [start, end) to state, overwriting occupied cells.
func (t *trackIntervals) paint(start, end int, state cellState) {
	if start >= end {
		return
	}
	iv := *t
	// Fast path: after every interval (placement mostly advances).
	if n := len(iv); n == 0 || iv[n-1].end <= start {
		if n > 0 && iv[n-1].state == state && iv[n-1].end == start {
			iv[n-1].end = end
		} else {
			iv = append(iv, interval{start, end, state})
		}
		*t = iv
		return
	}
	out := make(trackIntervals, 0, len(iv)+2)
	for _, x := range iv {
		if x.end <= start {
			out = append(out, x)
		} else if x.start < start {
			out = append(out, interval{x.start, start, x.state})
		}
	}
	if n := len(out); n > 0 && out[n-1].state == state && out[n-1].end == start {
		out[n-1].end = end
	} else {
		out = append(out, interval{start, end, state})
	}
	for _, x := range iv {
		var tr interval
		switch {
		case x.start >= end:
			tr = x
		case x.end > end:
			tr = interval{end, x.end, x.state}
		default:
			continue
		}
		if n := len(out); out[n-1].state == tr.state && out[n-1].end == tr.start {
			out[n-1].end = tr.end
		} else {
			out = append(out, tr)
		}
	}
	*t = out
}

// collisionExtent returns the last occupied cell of the last interval
// overlapping [start, end): every search position before it collides too.
func (t trackIntervals) collisionExtent(start, end int) (int, bool) {
	for i := len(t) - 1; i >= 0; i-- {
		if t[i].start < end && t[i].end > start {
			return t[i].end - 1, true
		}
	}
	return 0, false
}

// lastOfState returns the last cell with the state.
func (t trackIntervals) lastOfState(s cellState) (int, bool) {
	for i := len(t) - 1; i >= 0; i-- {
		if t[i].state == s {
			return t[i].end - 1, true
		}
	}
	return 0, false
}

// occupancy tracks the occupied cells of the grid during placement, as
// interval lists per row and per column (memory proportional to the items,
// not the cells).
type occupancy struct {
	cols, rows   trackCounts
	rowIv, colIv []trackIntervals // per row (column lines), per column (row lines)
}

func newOccupancy(cols, rows trackCounts) *occupancy {
	return &occupancy{cols: cols, rows: rows, rowIv: make([]trackIntervals, rows.len()), colIv: make([]trackIntervals, cols.len())}
}

// counts returns the track counts of an axis (horizontal: columns).
func (o *occupancy) counts(a axis) trackCounts {
	if a == horizontal {
		return o.cols
	}
	return o.rows
}

// lists returns the interval lists of the tracks of an axis.
func (o *occupancy) lists(a axis) []trackIntervals {
	if a == horizontal {
		return o.colIv
	}
	return o.rowIv
}

func otherAxis(a axis) axis {
	if a == horizontal {
		return vertical
	}
	return horizontal
}

// expand grows the grid to hold the spans.
func (o *occupancy) expand(rowSpan, colSpan lineSpan) {
	negRows := max(-o.rows.neg-rowSpan.start, 0)
	posRows := max(rowSpan.end-o.rows.implicitEnd(), 0)
	negCols := max(-o.cols.neg-colSpan.start, 0)
	posCols := max(colSpan.end-o.cols.implicitEnd(), 0)
	if negRows > 0 {
		o.rowIv = append(make([]trackIntervals, negRows), o.rowIv...)
	}
	if posRows > 0 {
		o.rowIv = append(o.rowIv, make([]trackIntervals, posRows)...)
	}
	if negCols > 0 {
		o.colIv = append(make([]trackIntervals, negCols), o.colIv...)
	}
	if posCols > 0 {
		o.colIv = append(o.colIv, make([]trackIntervals, posCols)...)
	}
	o.rows.neg += negRows
	o.rows.pos += posRows
	o.cols.neg += negCols
	o.cols.pos += posCols
}

// mark marks an area as occupied, growing the grid as needed.
func (o *occupancy) mark(primary axis, pSpan, sSpan lineSpan, state cellState) {
	rowSpan, colSpan := sSpan, pSpan
	if primary == vertical {
		rowSpan, colSpan = pSpan, sSpan
	}
	o.expand(rowSpan, colSpan)
	for r := o.rows.ozToTrack(rowSpan.start); r < o.rows.ozToTrack(rowSpan.end); r++ {
		o.rowIv[r].paint(colSpan.start, colSpan.end, state)
	}
	for c := o.cols.ozToTrack(colSpan.start); c < o.cols.ozToTrack(colSpan.end); c++ {
		o.colIv[c].paint(rowSpan.start, rowSpan.end, state)
	}
}

// collisionJump checks an area for occupied cells. It returns false if the
// area is free, else the next search position along primary that does not
// surely collide with the cells found.
func (o *occupancy) collisionJump(primary axis, pSpan, sSpan lineSpan) (int, bool) {
	other := otherAxis(primary)
	lists := o.lists(other)
	sc := o.counts(other)
	// Cells outside the grid are free.
	s0 := max(sc.ozToTrack(sSpan.start), 0)
	s1 := min(sc.ozToTrack(sSpan.end), len(lists))
	found, extent := false, 0
	for i := s0; i < s1; i++ {
		if c, ok := lists[i].collisionExtent(pSpan.start, pSpan.end); ok {
			if !found || c > extent {
				extent = c
			}
			found = true
		}
	}
	return extent + 1, found
}

// occupiedTrackJump returns the position after the last non-empty track of
// a span of tracks of axis a, or false if they are all empty.
func (o *occupancy) occupiedTrackJump(a axis, span lineSpan) (int, bool) {
	c := o.counts(a)
	lists := o.lists(a)
	s0 := max(c.ozToTrack(span.start), 0)
	s1 := min(c.ozToTrack(span.end), len(lists))
	for i := s1 - 1; i >= s0; i-- {
		if len(lists[i]) != 0 {
			return c.trackToOZ(i) + 1, true
		}
	}
	return 0, false
}

func (o *occupancy) rowOccupied(i int) bool { return i < len(o.rowIv) && len(o.rowIv[i]) != 0 }
func (o *occupancy) colOccupied(i int) bool { return i < len(o.colIv) && len(o.colIv[i]) != 0 }

// lastOfType returns the last cell with state s in the track of the
// other axis that starts at line at.
func (o *occupancy) lastOfType(a axis, at int, s cellState) (int, bool) {
	other := otherAxis(a)
	i := o.counts(other).ozToTrack(at)
	lists := o.lists(other)
	if i < 0 || i >= len(lists) {
		return 0, false
	}
	return lists[i].lastOfState(s)
}

// resolveIndefiniteSpan returns the lines of a span starting at pos.
func resolveIndefiniteSpan(pos, span int) lineSpan { return lineSpan{pos, pos + span} }

// clampSpanToGrid clamps a placement into the limited grid, keeping a
// span of at least 1 (CSS Grid 1 section 7.9).
func clampSpanToGrid(s lineSpan) lineSpan {
	start := min(max(s.start, -maxGridTracks), maxGridTracks-1)
	end := min(max(s.end, start+1), maxGridTracks)
	return lineSpan{start, end}
}

// placeItems places the items in the grid (CSS Grid 1 section 8.5, Taffy's
// place_grid_items), growing the implicit grid. It returns the items in
// placement order.
func placeItems(occ *occupancy, children []gridChild, placements [][2]ozPair, flow GridAutoFlow, alignItems, justifyItems Align) []gridItem {
	items := make([]gridItem, 0, len(children))
	primary := flow.primaryAxis()
	secondary := otherAxis(primary)
	get := func(p [2]ozPair, a axis) ozPair {
		if a == horizontal {
			return p[0]
		}
		return p[1]
	}
	record := func(i int, pSpan, sSpan lineSpan, state cellState) {
		pSpan, sSpan = clampSpanToGrid(pSpan), clampSpanToGrid(sSpan)
		occ.mark(primary, pSpan, sSpan, state)
		col, row := pSpan, sSpan
		if primary == vertical {
			col, row = sSpan, pSpan
		}
		items = append(items, newGridItem(children[i], col, row, alignItems, justifyItems))
	}

	// 1. Items with definite positions in both axes.
	for i := range children {
		p := placements[i]
		if p[0].isDefinite() && p[1].isDefinite() {
			record(i, get(p, primary).resolveDefinite(), get(p, secondary).resolveDefinite(), cellDefinite)
		}
	}

	// 2. Items locked to a secondary-axis track (a row in row flow).
	for i := range children {
		p := placements[i]
		if !get(p, secondary).isDefinite() || get(p, primary).isDefinite() {
			continue
		}
		sSpan := get(p, secondary).resolveDefinite()
		pos := occ.counts(primary).implicitStart()
		if !flow.isDense() {
			if l, ok := occ.lastOfType(primary, sSpan.start, cellAuto); ok {
				pos = l
			}
		}
		span := get(p, primary).indefiniteSpan()
		for {
			pSpan := resolveIndefiniteSpan(pos, span)
			next, hit := occ.collisionJump(primary, pSpan, sSpan)
			if !hit {
				record(i, pSpan, sSpan, cellAuto)
				break
			}
			pos = next
		}
	}

	// 3. The implicit grid already holds every definite position (mark
	// grows it) and the largest span (the size estimate).

	// 4. The remaining items, from the auto-placement cursor.
	startPos := [2]int{occ.counts(primary).implicitStart(), occ.counts(secondary).implicitStart()}
	pos := startPos
	for i := range children {
		p := placements[i]
		if get(p, secondary).isDefinite() {
			continue
		}
		pSpan, sSpan := placeIndefinite(occ, get(p, primary), get(p, secondary), flow, pos)
		record(i, pSpan, sSpan, cellAuto)
		if flow.isDense() {
			pos = startPos
		} else {
			pos = [2]int{pSpan.end, sSpan.start}
		}
	}
	return items
}

// placeIndefinite places an item without a definite secondary-axis
// position, searching from the cursor pos (primary, secondary).
func placeIndefinite(occ *occupancy, pp, sp ozPair, flow GridAutoFlow, pos [2]int) (lineSpan, lineSpan) {
	primary := flow.primaryAxis()
	secondary := otherAxis(primary)
	sSpanLen := sp.indefiniteSpan()
	pStart := occ.counts(primary).implicitStart()
	pEnd := occ.counts(primary).implicitEnd()
	sStart := occ.counts(secondary).implicitStart()
	pIdx, sIdx := pos[0], pos[1]

	if pp.isDefinite() {
		// A fixed primary-axis position: move along the secondary axis.
		pSpan := pp.resolveDefinite()
		switch {
		case flow.isDense():
			sIdx = sStart
		case pSpan.start < pIdx:
			sIdx++
		}
		for {
			sSpan := resolveIndefiniteSpan(sIdx, sSpanLen)
			if next, hit := occ.collisionJump(secondary, sSpan, pSpan); hit {
				sIdx = next
				continue
			}
			return pSpan, sSpan
		}
	}

	pSpanLen := pp.indefiniteSpan()
	// An item spanning every primary track fits only in a stripe of empty
	// tracks.
	spansAll := pSpanLen >= occ.counts(primary).len()
	for {
		pSpan := resolveIndefiniteSpan(pIdx, pSpanLen)
		sSpan := resolveIndefiniteSpan(sIdx, sSpanLen)
		if pSpan.end > pEnd {
			// Out of bounds even at the start: it can never fit; the
			// placement is clamped into the limited grid.
			if pIdx == pStart {
				return pSpan, sSpan
			}
			sIdx++
			pIdx = pStart
			continue
		}
		if spansAll {
			if next, hit := occ.occupiedTrackJump(secondary, sSpan); hit {
				sIdx = next
				pIdx = pStart
				continue
			}
			return pSpan, sSpan
		}
		if next, hit := occ.collisionJump(primary, pSpan, sSpan); hit {
			pIdx = next
			continue
		}
		return pSpan, sSpan
	}
}
