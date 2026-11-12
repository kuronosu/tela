package kit_test

import (
	"testing"
)

// TestTableFrameWork: a frame of a plain 2000-row table (12,000 cells in
// one grid) that is not a scroll does work for what changed, not for the
// rows (ADR-280): a hover restyles the two rows' cells and maps, lays out
// and places only them and the boxes above them; a new amount in a 140px
// column lays out the cell, not the grid (its measurements are the old
// ones, layout.Engine.MarkContent); a state outside the table lays out
// and places only its own box and the way to it. The counts are the
// deterministic side of BenchmarkTablePerf.
func TestTableFrameWork(t *testing.T) {
	cases := []struct {
		op                        string
		laidOut, remapped, placed int // at most
		wantLaidOut, wantRemapped bool
	}{
		{op: "hover", laidOut: 0, remapped: 30, placed: 60, wantRemapped: true},
		{op: "cellpx", laidOut: 25, remapped: 0, placed: 30, wantLaidOut: true},
		{op: "bump", laidOut: 10, remapped: 0, placed: 30, wantLaidOut: true},
	}
	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			r, _ := newPerfRig(t, perfPage{table: perfTable(2000, false)})
			for i := range 4 {
				before := r.tree.Stats()
				perfOps()[c.op](r, i)
				r.frame()
				w := statsOf(before, r.tree.Stats())
				if w.Paints != 1 {
					t.Fatalf("%s %d: %d paints, want a frame", c.op, i, w.Paints)
				}
				if w.LaidOut > c.laidOut || c.wantLaidOut && w.LaidOut == 0 {
					t.Errorf("%s %d: laid out %d boxes, want at most %d (the grid's 12,000 cells are not)", c.op, i, w.LaidOut, c.laidOut)
				}
				if w.Remapped > c.remapped || c.wantRemapped && w.Remapped == 0 {
					t.Errorf("%s %d: mapped %d layout styles, want at most %d", c.op, i, w.Remapped, c.remapped)
				}
				if w.Placed > c.placed || w.Placed == 0 {
					t.Errorf("%s %d: placed %d boxes, want 1 to %d (not the 24,000 of the table)", c.op, i, w.Placed, c.placed)
				}
			}
		})
	}
}
