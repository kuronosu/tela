package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// A single-layer class keeps its gradient under the Fn's.
func TestBackgroundGradientFnOverASingleLayerClass(t *testing.T) {
	tr := newTestTree(t, 200, 100, Component(func() Node {
		return div(Key("g"), Class("h-20 w-40 bg-linear-to-b from-[#00ff00] to-[#00ffff]"),
			Style(style.BackgroundGradientFn(func() style.Gradient { return angleGradient(90) })))
	}), nil)
	dl, _ := tr.Frame()
	gs, _ := linearFills(dl)
	if len(gs) != 2 || gs[0].Start.X != gs[0].End.X || gs[1].Start.Y != gs[1].End.Y {
		t.Fatalf("gradients %v, want the class's top to bottom under the Fn's left to right", gs)
	}
	// bg-none leaves the Fn's layer alone.
	tr = newTestTree(t, 200, 100, Component(func() Node {
		return div(Key("g"), Class("h-20 w-40 bg-linear-to-b from-[#00ff00] to-[#00ffff] bg-none"),
			Style(style.BackgroundGradientFn(func() style.Gradient { return angleGradient(90) })))
	}), nil)
	dl, _ = tr.Frame()
	if gs, _ := linearFills(dl); len(gs) != 1 {
		t.Errorf("with bg-none: gradients %v, want the Fn's only", gs)
	}
}

// BackgroundGradientFn is a layer over the layers a class gives (ADR-263).
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
	if len(gs) != 3 {
		t.Fatalf("gradients %v, want the class's two layers and the Fn's", gs)
	}
	// Painted bottom to top: the class's vertical magenta-to-yellow, its
	// green-to-cyan, then the Fn's 90 degrees.
	if g := gs[0]; g.Start.X != g.End.X || g.Start.Y >= g.End.Y {
		t.Errorf("the lowest layer runs %v..%v, want the class's top to bottom", g.Start, g.End)
	}
	if g := gs[1]; g.Start.Y != g.End.Y || g.Stops[0].Color != gfx.RGB8(0, 255, 0) {
		t.Errorf("the class's first layer is %v..%v %v", g.Start, g.End, g.Stops)
	}
	if g := gs[2]; g.Start.Y != g.End.Y || g.Start.X >= g.End.X || g.Stops[0].Color != gfx.RGB8(255, 0, 0) {
		t.Errorf("the topmost layer runs %v..%v, want the Fn's left to right", g.Start, g.End)
	}
	if bx := findBox(tr, "g"); len(bx.pcs.Backgrounds) != 3 {
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
	if len(gs) != 3 || gs[2].Start.X != gs[2].End.X || gs[0].Start.X != gs[0].End.X || gs[1].Start.Y != gs[1].End.Y {
		t.Fatalf("after 180 degrees: %v", gs)
	}
	if builds != 1 {
		t.Errorf("%d builds", builds)
	}
}
