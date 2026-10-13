package core

import (
	"fmt"
	"testing"
)

// TestLayerTree checks the layers of a page: the root, the content of the
// viewport and of the inner scroll containers, a deferred layer, the
// scrollbars; and that frames that only repaint record nothing but the
// scrollbars.
func TestLayerTree(t *testing.T) {
	st := &sceneState{refs: make([]ElementRef, 4), setters: make([]Setter[int], 4), values: make([]*Value[float32], 4)}
	x := newDamageHarness(t, plainPageScene(st), 360, 280, 1, Config{InstantScroll: true}, 1)
	kinds := map[string]int{}
	for _, l := range x.tree().Layers() {
		kinds[l.Kind]++
		if !l.Recorded {
			t.Errorf("%s layer not recorded by the first paint", l.Kind)
		}
	}
	if kinds["root"] != 1 || kinds["scroll"] < 3 || kinds["scrollbars"] < 3 {
		t.Errorf("layers %v: want the root, the scroll content of the viewport, the inner list and the strip, and their bars", kinds)
	}
	// A deferred layer appears when the box becomes a stacking context.
	st.values[0].Set(20)
	x.frame("translate")
	if n := countLayers(x.tree(), "deferred"); n != 1 {
		t.Errorf("%d deferred layers after a translation, want 1", n)
	}
	// Moving it again, fading it and scrolling record nothing else.
	for i, op := range []func(){
		func() { st.values[0].Set(35) },
		func() { st.values[1].Set(0.5) },
		func() { st.refs[0].ScrollBy(0, 30) },
		func() { st.refs[1].ScrollBy(40, 0) },
	} {
		s0 := x.tree().Stats()
		op()
		x.frame(fmt.Sprintf("op %d", i))
		if d := work(s0, x.tree().Stats()); d.Builds != 0 || d.Layouts != 0 || d.Styles != 0 {
			t.Errorf("op %d: work %+v, want a paint only", i, d)
		}
		for _, l := range x.tree().Layers() {
			if l.Recorded && l.Kind != "scrollbars" {
				t.Errorf("op %d recorded a %s layer", i, l.Kind)
			}
		}
	}
}

func countLayers(tr *Tree, kind string) int {
	n := 0
	for _, l := range tr.Layers() {
		if l.Kind == kind {
			n++
		}
	}
	return n
}

// TestHoverDamage checks that a hover in a long list rasterizes the two
// rows that change, not the window.
func TestHoverDamage(t *testing.T) {
	x := newDamageHarness(t, hoverList(300), 400, 300, 2, Config{}, 1)
	x.move(100, 40)
	x.frame("hover")
	x.move(100, 140)
	x.frame("hover")
	d := x.tree().LastDamage()
	area := 0
	for _, r := range d.Rects {
		area += r.Dx() * r.Dy()
	}
	if d.Full || area == 0 || area > 2*800*80 {
		t.Errorf("hover damage %+v (%d px), want the two rows", d, area)
	}
}
