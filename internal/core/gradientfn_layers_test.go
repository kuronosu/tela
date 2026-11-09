package core

import (
	"github.com/kuronosu/tela/internal/style"
	"testing"
)

// BackgroundGradientFn is the topmost image layer and keeps the layers a
// class gives below it (ADR-256).
func TestBackgroundGradientFnKeepsTheLayersBelow(t *testing.T) {
	var angle *Value[float32]
	builds := 0
	comp := func() Node {
		builds++
		angle = UseValue[float32](90)
		return div(Key("g"), Class("h-20 w-40 bg-[linear-gradient(to_right,#00ff00,#00ffff),linear-gradient(to_bottom,#ff00ff,#ffff00)]"),
			Style(style.BackgroundGradientFn(func() style.Gradient { return angleGradient(angle.Get()) })))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	dl, _ := tr.Frame()
	gs, _ := linearFills(dl)
	if len(gs) != 2 {
		t.Fatalf("gradients %v, want the class's second layer and the Fn's", gs)
	}
	// Painted bottom to top: the class's vertical magenta-to-yellow, then
	// the Fn's 90 degrees (the class's first layer, left to right, is
	// replaced and does not show).
	if g := gs[0]; g.Start.X != g.End.X || g.Start.Y >= g.End.Y {
		t.Errorf("the lower layer runs %v..%v, want the class's top to bottom", g.Start, g.End)
	}
	if g := gs[1]; g.Start.Y != g.End.Y || g.Start.X >= g.End.X {
		t.Errorf("the topmost layer runs %v..%v, want the Fn's left to right", g.Start, g.End)
	}
	if bx := findBox(tr, "g"); len(bx.pcs.Backgrounds) != 2 {
		t.Errorf("%d layers", len(bx.pcs.Backgrounds))
	}
	// The Fn's layer follows its Value with a paint only; the layer below
	// stays.
	s0 := tr.Stats()
	angle.Set(180)
	if !tr.ApplyUpdates() {
		t.Fatal("a Value read by BackgroundGradientFn invalidated nothing")
	}
	dl, _ = tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	gs, _ = linearFills(dl)
	if len(gs) != 2 || gs[1].Start.X != gs[1].End.X || gs[0].Start.X != gs[0].End.X {
		t.Fatalf("after 180 degrees: %v", gs)
	}
	if builds != 1 {
		t.Errorf("%d builds", builds)
	}
}
