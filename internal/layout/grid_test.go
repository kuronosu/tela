package layout

import (
	"fmt"
	"math"
	"testing"
)

// Unit tests of the grid algorithm for what the fixtures do not cover:
// caching and relayout boundaries inside grids, the template equality used
// by core, and limits.

func gridOf(cols, rows TrackList, children ...*testNode) *testNode {
	return box(styleOf(func(s *Style) {
		s.Display = DisplayGrid
		s.Grid = &GridTemplate{Columns: cols, Rows: rows}
	}), children...)
}

func repeatTracks(n int, t TrackSize) TrackList {
	return TrackList{Entries: []TrackListEntry{Repeat(n, t)}}
}

// TestGridCacheReuse changes one item of a grid with fixed tracks: only
// that item is measured again.
func TestGridCacheReuse(t *testing.T) {
	var leaves []*testNode
	for i := 0; i < 60; i++ {
		leaves = append(leaves, text("HH\u200bHHH"))
	}
	grid := gridOf(repeatTracks(6, Track(Px(50))), TrackList{}, leaves...)
	grid.style.Grid.AutoRows = []TrackSize{Track(Px(20))}
	root := box(DefaultStyle(), grid)
	var e Engine
	compute(&e, root, 800, 600)
	wantRect(t, "item 7", leaves[7], [4]float32{50, 20, 50, 20})
	runs := e.runs

	compute(&e, root, 800, 600)
	if e.runs != runs {
		t.Errorf("clean relayout ran %d algorithms", e.runs-runs)
	}

	for _, l := range leaves {
		l.measures = 0
	}
	changed := leaves[23]
	changed.measure = ahemMeasure("HHHHHHHHHH")
	e.MarkDirty(changed)
	runs = e.runs
	compute(&e, root, 800, 600)
	for i, l := range leaves {
		if l != changed && l.measures != 0 {
			t.Errorf("leaf %d measured %d times", i, l.measures)
		}
	}
	if changed.measures == 0 {
		t.Error("the changed leaf was not measured")
	}
	if n := e.runs - runs; n > 8 {
		t.Errorf("%d layout runs after one leaf changed", n)
	}
	checkIncremental(t, root)
}

// TestGridIntrinsicRelayout changes an item in an auto track: the track
// and the items that share it follow.
func TestGridIntrinsicRelayout(t *testing.T) {
	a, b, c := text("HH"), text("HHHH"), text("H")
	grid := gridOf(Tracks(Track(Auto), Track(Fr(1))), TrackList{}, a, b, c)
	root := box(styleOf(func(s *Style) { s.Width = Px(200) }), grid)
	var e Engine
	compute(&e, root, 800, 600)
	wantRect(t, "b", b, [4]float32{20, 0, 180, 10})
	wantRect(t, "c", c, [4]float32{0, 10, 20, 10})

	a.measure = ahemMeasure("HHHHHH")
	e.MarkDirty(a)
	compute(&e, root, 800, 600)
	wantRect(t, "b after", b, [4]float32{60, 0, 140, 10})
	wantRect(t, "c after", c, [4]float32{0, 10, 60, 10})
	checkIncremental(t, root)
}

// TestGridRelayoutBoundary changes the content of a grid item that is a
// relayout boundary: its ancestors are not laid out again.
func TestGridRelayoutBoundary(t *testing.T) {
	leaf := text("HHH")
	boundary := box(styleOf(func(s *Style) {
		s.Width, s.Height = Px(100), Px(50)
		s.OverflowX, s.OverflowY = OverflowHidden, OverflowHidden
	}), leaf)
	grid := gridOf(Tracks(Track(Auto), Track(Auto)), TrackList{}, boundary, text("HHHH"))
	root := box(DefaultStyle(), grid)
	var e Engine
	compute(&e, root, 800, 600)

	leaf.measure = ahemMeasure("HHHHHH\u200bHHHHHH")
	e.MarkDirty(leaf)
	if !root.data.cache.hasFinal || !grid.data.cache.hasFinal {
		t.Error("MarkDirty invalidated ancestors above the relayout boundary")
	}
	runs := e.runs
	compute(&e, root, 800, 600)
	if n := e.runs - runs; n > 2 {
		t.Errorf("%d layout runs, want the boundary and the leaf only", n)
	}
	checkIncremental(t, root)
}

// TestGridAbsoluteHoisted checks an absolutely positioned child of a
// static grid, laid out by the positioned root at its static position, and
// incremental layout after the grid changes.
func TestGridAbsoluteHoisted(t *testing.T) {
	abs := box(styleOf(func(s *Style) {
		s.Position = PositionAbsolute
		s.Width, s.Height = Px(10), Px(10)
		s.JustifySelf = AlignEnd
	}))
	grid := gridOf(Tracks(Track(Px(40))), TrackList{}, text("HH"), abs)
	grid.style.Width = Px(100)
	grid.style.Padding = EdgesAll(Px(5))
	root := box(styleOf(func(s *Style) {
		s.Position = PositionRelative
		s.Padding.Top = Px(7)
	}), grid)
	var e Engine
	compute(&e, root, 800, 600)
	// The grid's content box: x 5 to 95, y from 5 (the location is
	// relative to the parent, the grid).
	wantRect(t, "abs", abs, [4]float32{85, 5, 10, 10})

	grid.style.Padding.Left = Px(15)
	e.MarkDirty(grid)
	compute(&e, root, 800, 600)
	wantRect(t, "abs after", abs, [4]float32{85, 5, 10, 10})
	grid.style.Width = Px(120)
	e.MarkDirty(grid)
	compute(&e, root, 800, 600)
	wantRect(t, "abs wider", abs, [4]float32{105, 5, 10, 10})
	checkIncremental(t, root)
}

// TestGridTemplateEqual checks the equality core uses to keep template
// pointers stable.
func TestGridTemplateEqual(t *testing.T) {
	mk := func() *GridTemplate {
		return &GridTemplate{
			Columns: TrackList{Entries: []TrackListEntry{Single(Track(Px(10))), {Repeat: RepeatAutoFill, Tracks: []TrackSize{Minmax(Px(5), Fr(1))},
				LineNames: [][]string{{"a"}, {"b"}}}}, LineNames: [][]string{{"x"}, nil, {"y"}}},
			AutoRows: []TrackSize{Track(Auto)},
			Areas:    []GridArea{{Name: "h", RowStart: 1, RowEnd: 2, ColumnStart: 1, ColumnEnd: 3}},
			AreaRows: 1, AreaColumns: 2,
		}
	}
	a, b := mk(), mk()
	if !a.Equal(b) {
		t.Error("equal templates differ")
	}
	b.Columns.Entries[1].LineNames[1][0] = "c"
	if a.Equal(b) {
		t.Error("different line names are equal")
	}
	c := mk()
	c.Areas[0].ColumnEnd = 2
	if a.Equal(c) {
		t.Error("different areas are equal")
	}
	if !(*GridTemplate)(nil).Equal(nil) || a.Equal(nil) {
		t.Error("nil equality")
	}
}

// TestGridLimits lays out grids at the track limit and with extreme lines
// quickly and without panics.
func TestGridLimits(t *testing.T) {
	items := []*testNode{
		box(styleOf(func(s *Style) { s.GridColumn = GridPlacement{GridLineAt(1 << 29), GridSpanOf(1 << 29)} })),
		box(styleOf(func(s *Style) { s.GridRow = GridPlacement{Start: GridLineAt(-(1 << 29))} })),
		box(styleOf(func(s *Style) {
			s.GridColumn = GridPlacement{GridSpanOf(-5), GridLine{Kind: GridNamedSpan, Name: "x", N: 1 << 29}}
		})),
		box(styleOf(func(s *Style) {
			s.GridColumn = GridPlacement{GridLine{Kind: GridNamedLine, Name: "x", N: -(1 << 29)}, GridLine{Kind: GridNamedLine, Name: "x", N: 1 << 29}}
		})),
		text("HH"),
	}
	grid := gridOf(TrackList{Entries: []TrackListEntry{Repeat(1<<20, Track(Px(0)))}}, TrackList{}, items...)
	var e Engine
	compute(&e, box(DefaultStyle(), grid), 800, 600)
	for i, n := range nodes(grid) {
		l := n.data.Layout()
		for _, v := range []float32{l.Location.X, l.Location.Y, l.Size.W, l.Size.H} {
			if v != v {
				t.Fatalf("node %d: NaN in %+v", i, *l)
			}
		}
	}
}

// TestGridNonFinite lays out grids with NaN and infinite track sizes and
// gaps: no hang, no panic.
func TestGridNonFinite(t *testing.T) {
	nan, posInf := float32(math.NaN()), float32(math.Inf(1))
	for _, ts := range []TrackSize{Track(Fr(nan)), Track(Fr(posInf)), Track(Px(nan)), Minmax(Px(posInf), Fr(1)),
		Track(FitContentPx(nan)), Minmax(Percent(nan), Auto)} {
		grid := gridOf(TrackList{Entries: []TrackListEntry{Single(ts), Single(Track(Fr(1))),
			{Repeat: RepeatAutoFill, Tracks: []TrackSize{Track(Px(nan))}}}}, Tracks(ts), text("HH\u200bH"), text("H"), text("HHH"))
		grid.style.ColumnGap = Px(nan)
		var e Engine
		compute(&e, box(DefaultStyle(), grid), 300, 300)
		e.Compute(grid, MaxContentConstraint, MinContentConstraint)
	}
}

// TestGridPlacementOccupancy checks the interval occupancy lists against
// a dense matrix on random paints.
func TestGridPlacementOccupancy(t *testing.T) {
	for seed := 0; seed < 200; seed++ {
		var iv trackIntervals
		dense := map[int]cellState{}
		r := seed*7919 + 1
		next := func(n int) int {
			r = (r*1103515245 + 12345) & 0x7fffffff
			return r % n
		}
		for k := 0; k < 12; k++ {
			s := next(30) - 10
			e := s + next(6)
			st := cellState(1 + next(2))
			iv.paint(s, e, st)
			for c := s; c < e; c++ {
				dense[c] = st
			}
		}
		for c := -12; c < 26; c++ {
			got := cellFree
			for _, x := range iv {
				if x.start <= c && c < x.end {
					got = x.state
				}
			}
			if got != dense[c] {
				t.Fatalf("seed %d: cell %d is %d, want %d (%v)", seed, c, got, dense[c], iv)
			}
		}
		for i := 1; i < len(iv); i++ {
			if iv[i-1].end > iv[i].start || iv[i-1].end == iv[i].start && iv[i-1].state == iv[i].state {
				t.Fatalf("seed %d: intervals not sorted, disjoint and merged: %v", seed, iv)
			}
		}
	}
}

// BenchmarkGrid1000 lays out a dashboard-like grid of 1,000 text cards in
// repeat(auto-fill, minmax(120px, 1fr)) columns, cold.
func BenchmarkGrid1000(b *testing.B) {
	root := benchGrid(1000)
	var e Engine
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		resetData(root)
		b.StartTimer()
		e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	}
}

// BenchmarkGrid1000Auto is BenchmarkGrid1000 with intrinsic (auto)
// columns, which measures every item's contributions.
func BenchmarkGrid1000Auto(b *testing.B) {
	root := benchGrid(1000)
	root.children[0].style.Grid.Columns = repeatTracks(8, Track(Auto))
	var e Engine
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		resetData(root)
		b.StartTimer()
		e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	}
}

// BenchmarkGrid1000Incremental changes one card of a laid out 1,000-card
// grid and lays it out again.
func BenchmarkGrid1000Incremental(b *testing.B) {
	root := benchGrid(1000)
	var e Engine
	e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	card := root.children[0].children[500]
	para := card.children[1]
	texts := []func(MeasureInput) Measurement{
		ahemMeasure("HHHHHH\u200bHHH\u200bHHHHH\u200bHH"),
		ahemMeasure("HHH\u200bHHHHHHHHH\u200bHH\u200bHHHH\u200bHHHHHH\u200bHHHHHHHHH"),
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		para.measure = texts[i%2]
		i++
		e.MarkDirty(para)
		e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	}
}

// benchGrid builds a block root holding a grid of n cards (a flex column
// with a title and a wrapping paragraph each).
func benchGrid(n int) *testNode {
	var cards []*testNode
	for i := 0; i < n; i++ {
		cards = append(cards, box(styleOf(func(s *Style) {
			s.Display = DisplayFlex
			s.FlexDirection = Column
			s.Padding = EdgesAll(Px(6))
			s.RowGap = Px(4)
		}), text(fmt.Sprintf("HHH\u200bH%d", i%10)), text("HHHHHH\u200bHHH\u200bHHHHH\u200bHH\u200bHHHHHHH\u200bHHHH")))
	}
	grid := box(styleOf(func(s *Style) {
		s.Display = DisplayGrid
		s.Grid = &GridTemplate{Columns: TrackList{Entries: []TrackListEntry{{Repeat: RepeatAutoFill,
			Tracks: []TrackSize{Minmax(Px(120), Fr(1))}}}}}
		s.ColumnGap, s.RowGap = Px(8), Px(8)
		s.Padding = EdgesAll(Px(16))
	}), cards...)
	return box(DefaultStyle(), grid)
}
