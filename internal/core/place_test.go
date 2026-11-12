package core

import (
	"testing"
)

// TestPartialPlaceWork: a frame that builds, restyles or lays out places
// the boxes whose place can have changed (place.go, ADR-280): a new label
// of one row of a 600-row list in a scroll container places that row's
// boxes and the boxes on the way to them, not the list; a label that
// wraps to a second line moves the rows below, which are placed again.
// core's tests check every partial place against a full one
// (TELA_CHECK_LAYOUT).
func TestPartialPlaceWork(t *testing.T) {
	setters := make([]func(string), 600)
	tr := newTestTree(t, 600, 800, div(Class("h-screen overflow-y-auto"), bigList(len(setters), setters)), nil)
	tr.Frame()
	work := func(f func()) Stats {
		s0 := tr.Stats()
		f()
		tr.Frame()
		s1 := tr.Stats()
		return Stats{LaidOut: s1.LaidOut - s0.LaidOut, Remapped: s1.Remapped - s0.Remapped, Placed: s1.Placed - s0.Placed}
	}
	// A new label of one line: the row's paragraph, checked up to its
	// row (layout.Engine.MarkContent), not the 600 rows.
	w := work(func() { setters[300]("Item number 301") })
	if w.Placed == 0 || w.Placed > 12 {
		t.Errorf("a new label placed %d boxes, want the row's and the way to it (at most 12, of 2,400)", w.Placed)
	}
	if w.LaidOut > 30 {
		t.Errorf("a new label laid out %d boxes, want its paragraph's and row's requests", w.LaidOut)
	}
	// Two lines: the row grows, the rows below it (4 boxes each) move.
	w = work(func() { setters[300]("A label long enough to wrap to a second line in its row, and a bit more than that") })
	if w.Placed < 299*4 {
		t.Errorf("a taller row placed %d boxes, want the rows below it too (about %d)", w.Placed, 300*4)
	}
	if w.LaidOut > 40 {
		t.Errorf("a taller row laid out %d boxes, want its ancestors, not the other rows", w.LaidOut)
	}
	// Another row: its boxes and the way to it again.
	w = work(func() { setters[10]("Item number 11") })
	if w.Placed == 0 || w.Placed > 12 {
		t.Errorf("placed %d boxes, want at most 12", w.Placed)
	}
}
