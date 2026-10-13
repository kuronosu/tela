package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// checkScrollSets checks the invariants of the scroller sets (scroll.go):
// every index matches its slot, the list is in creation order, and each
// set holds every scroller in its state (setEvent and setHover exactly).
func checkScrollSets(t testing.TB, tr *Tree) {
	t.Helper()
	st := &tr.scr
	holes := 0
	var last uint64
	for i, s := range st.list {
		if s == nil {
			holes++
			continue
		}
		if int(s.at[setAll]) != i+1 {
			t.Fatalf("list[%d] has index %d", i, s.at[setAll]-1)
		}
		if s.seq <= last {
			t.Fatalf("list[%d] out of creation order (seq %d after %d)", i, s.seq, last)
		}
		last = s.seq
		if s.box.scr != s {
			t.Fatalf("list[%d] is not its box's scroller", i)
		}
		in := func(k scrollSet) bool { return s.at[k] != 0 }
		if s.animating() && !in(setMoving) {
			t.Fatalf("list[%d] animates outside setMoving", i)
		}
		if barsLive(s) && !in(setBars) {
			t.Fatalf("list[%d] has active overlay bars outside setBars", i)
		}
		if s.eventPending != in(setEvent) {
			t.Fatalf("list[%d]: eventPending %v, in setEvent %v", i, s.eventPending, in(setEvent))
		}
		b := &s.bars
		if hov := b.near != [2]bool{} || b.over != [2]bool{} || b.thumb != [2]bool{}; hov != in(setHover) {
			t.Fatalf("list[%d]: hovered bars %v, in setHover %v", i, hov, in(setHover))
		}
	}
	if holes != st.holes {
		t.Fatalf("%d holes, counted %d", holes, st.holes)
	}
	if n := len(st.list); n > 0 && st.list[n-1] == nil {
		t.Fatalf("the list ends with a hole")
	}
	for k := setAll + 1; k < numScrollSets; k++ {
		for i, s := range st.sets[k] {
			if int(s.at[k]) != i+1 {
				t.Fatalf("set %d [%d] has index %d", k, i, s.at[k]-1)
			}
			if s.at[setAll] == 0 {
				t.Fatalf("set %d [%d] holds a dropped scroller", k, i)
			}
		}
	}
}

// TestScrollerListRemoval drops scroll containers from the middle, the end
// and everywhere: the list keeps its order and compacts, and the dropped
// scrollers leave every set.
func TestScrollerListRemoval(t *testing.T) {
	var setN func(int)
	var setHidden func(func(int) bool)
	var setScrolls func(bool)
	page := func() Node {
		n, set := UseStateSet(300)
		hide, setH := UseStateSet[func(int) bool](nil)
		scrolls, setS := UseStateSet(true)
		setN, setHidden, setScrolls = set, setH, setS
		rows := make([]Node, 0, n)
		for i := range n {
			if hide != nil && hide(i) {
				continue
			}
			rows = append(rows, WithKey(div(Class("h-6 overflow-hidden"), Textf("row %d", i)), i))
		}
		cls := "h-screen"
		if scrolls {
			cls += " overflow-y-auto"
		}
		return div(Class(cls), List(rows))
	}
	tr := newTestTree(t, 300, 300, Component(page), nil)
	tr.Frame()
	checkScrollSets(t, tr)
	count := func() int { return tr.scr.scrollers() }
	if got := count(); got != 302 { // 300 rows, the list, the viewport
		t.Fatalf("%d scrollers, want 302", got)
	}
	// A few from the middle: holes, no compaction.
	setHidden(func(i int) bool { return i >= 100 && i < 110 })
	tr.Frame()
	checkScrollSets(t, tr)
	if got, holes := count(), tr.scr.holes; got != 292 || holes != 10 {
		t.Fatalf("%d scrollers, %d holes; want 292, 10", got, holes)
	}
	// The end: cut, with the holes before it kept.
	setN(250)
	tr.Frame()
	checkScrollSets(t, tr)
	if got := count(); got != 242 {
		t.Fatalf("%d scrollers, want 242", got)
	}
	// Every other row: compaction.
	setHidden(func(i int) bool { return i%2 == 1 || i >= 100 && i < 110 })
	tr.Frame()
	checkScrollSets(t, tr)
	if got, holes := count(), tr.scr.holes; got != 122 || holes >= got {
		t.Fatalf("%d scrollers, %d holes; want 122 and fewer holes", got, holes)
	}
	// Scroll, animate and drop a scrolled container while it animates.
	s := tr.root.box.scr
	for _, x := range tr.scr.list {
		if x != nil && x.max.Y > 0 && x != s {
			s = x
		}
	}
	tr.scrollUser(s, gfx.Point{Y: 500})
	tr.Animate(tr.scr.now + frameStep)
	tr.Frame()
	checkScrollSets(t, tr)
	if s.at[setMoving] == 0 || s.at[setBars] == 0 {
		t.Fatalf("the scrolling list is not in setMoving and setBars: %v", s.at)
	}
	// The list stops being a scroll container while it animates.
	setScrolls(false)
	tr.Frame()
	checkScrollSets(t, tr)
	if s.at != [numScrollSets]int32{} {
		t.Fatalf("the dropped scroller is still in sets %v", s.at)
	}
	tr.Animate(tr.scr.now + frameStep)
	if tr.scrollAnimating() {
		t.Fatal("a dropped scroller still animates the tree")
	}
	setN(0)
	tr.Frame()
	checkScrollSets(t, tr)
	if got := count(); got != 1 {
		t.Fatalf("%d scrollers, want the viewport", got)
	}
}
