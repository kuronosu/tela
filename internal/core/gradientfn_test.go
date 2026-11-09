package core

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Tests of BackgroundGradientFn (ADR-250): a background gradient made in
// the paint phase.

// angleGradient returns a linear gradient from red to blue at angle deg.
func angleGradient(deg float32) style.Gradient {
	return style.Gradient{Kind: style.GradientLinear, Angle: deg, CenterX: style.PctLP(50), CenterY: style.PctLP(50),
		Stops: []style.ColorStop{{Color: gfx.RGB8(255, 0, 0)}, {Color: gfx.RGB8(0, 0, 255)}}}
}

// linearFills returns the linear gradients the fill commands of a display
// list paint, with the rectangles they fill.
func linearFills(dl *gfx.DisplayList) (gs []gfx.LinearGradient, rs []gfx.Rect) {
	for _, c := range dl.All() {
		var p gfx.Paint
		var r gfx.Rect
		switch f := c.(type) {
		case gfx.FillRect:
			p, r = f.Paint, f.Rect
		case gfx.FillRRect:
			p, r = f.Paint, f.RRect.Rect
		default:
			continue
		}
		if g, ok := p.(gfx.LinearGradient); ok {
			gs, rs = append(gs, g), append(rs, r)
		}
	}
	return gs, rs
}

// TestBackgroundGradientFnPaintsOnly: the Fn's gradient replaces the
// class's, over the background color, and a change of the Value it reads
// repaints without building, computing styles or laying out.
func TestBackgroundGradientFnPaintsOnly(t *testing.T) {
	var angle *Value[float32]
	builds := 0
	comp := func() Node {
		builds++
		angle = UseValue[float32](90)
		return div(Class("p-4"),
			div(Key("g"), Class("size-20 rounded-lg bg-white bg-linear-to-r from-green-500 to-green-700"),
				Style(style.BackgroundGradientFn(func() style.Gradient { return angleGradient(angle.Get()) }))),
			p(Class("w-40"), Text("a paragraph")))
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	dl, _ := tr.Frame()
	gs, _ := linearFills(dl)
	if len(gs) != 1 || gs[0].Stops[0].Color != gfx.RGB8(255, 0, 0) {
		t.Fatalf("gradients %v, want the Fn's red to blue only", gs)
	}
	if !hasFillPaint(dl, gfx.White) {
		t.Error("the background color is not painted under the gradient")
	}
	right := gs[0]
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
	if builds != 1 || len(gs) != 1 {
		t.Fatalf("builds %d, gradients %v", builds, gs)
	}
	// 90 degrees runs left to right, 180 top to bottom.
	if right.Start.Y != right.End.Y || gs[0].Start.X != gs[0].End.X || gs[0].Start.Y >= gs[0].End.Y {
		t.Errorf("90 degrees %v..%v, 180 degrees %v..%v", right.Start, right.End, gs[0].Start, gs[0].End)
	}
	// The same angle again changes nothing.
	angle.Set(180)
	if tr.ApplyUpdates() && tr.Dirty() {
		s0 = tr.Stats()
		tr.Frame()
		if d := work(s0, tr.Stats()); d.Paints != 0 {
			t.Errorf("an equal value repainted: %+v", d)
		}
	}
}

// hasFillPaint reports whether a fill command of dl paints color c.
func hasFillPaint(dl *gfx.DisplayList, c gfx.Color) bool {
	for _, cmd := range dl.All() {
		switch f := cmd.(type) {
		case gfx.FillRect:
			if f.Paint == gfx.Paint(c) {
				return true
			}
		case gfx.FillRRect:
			if f.Paint == gfx.Paint(c) {
				return true
			}
		}
	}
	return false
}

// TestBackgroundGradientFnLayerClasses: the background size, position and
// repeat classes apply to the Fn's gradient, a gradient without stops
// paints no image, and the stops are copied.
func TestBackgroundGradientFnLayerClasses(t *testing.T) {
	stops := []style.ColorStop{{Color: gfx.RGB8(255, 0, 0)}, {Color: gfx.RGB8(0, 0, 255)}}
	var on *Value[bool]
	comp := func() Node {
		on = UseValue(true)
		return div(Key("g"), Class("h-20 w-40 bg-no-repeat bg-size-[50%_100%] bg-right"),
			Style(style.BackgroundGradientFn(func() style.Gradient {
				if !on.Get() {
					return style.Gradient{}
				}
				return style.Gradient{Kind: style.GradientLinear, Angle: 90, Stops: stops}
			})))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	dl, _ := tr.Frame()
	gs, _ := linearFills(dl)
	if len(gs) != 1 {
		t.Fatalf("gradients %v", gs)
	}
	// One tile, half the width, at the right: the gradient line runs
	// across it.
	if g := gs[0]; g.Start.X != 80 || g.End.X != 160 {
		t.Errorf("the gradient runs from %v to %v, want x 80 to 160 (the right half)", g.Start, g.End)
	}
	stops[0].Color = gfx.RGB8(0, 255, 0) // the caller reuses its slice
	if bx := findBox(tr, "g"); bx.pcs.Backgrounds[0].Gradient.Stops[0].Color != gfx.RGB8(255, 0, 0) {
		t.Errorf("the painted gradient shares the caller's stops")
	}
	on.Set(false)
	tr.ApplyUpdates()
	dl, _ = tr.Frame()
	if gs, _ := linearFills(dl); len(gs) != 0 {
		t.Errorf("a gradient without stops painted %v", gs)
	}
}

// TestBackgroundGradientFnRecordsItsLayer: on a page of positioned cards
// (each painted in a chunk of its own), a change of the gradient of one
// card records that card's chunk only; a build of another card places
// every box again, and the gradient card, whose Fn makes an equal
// gradient (compared by content), is not recorded again. Every frame
// shows what a full rendering shows (with TELA_CHECK_LAYERS=1 the partial
// recordings compose what a full one does).
func TestBackgroundGradientFnRecordsItsLayer(t *testing.T) {
	st := &sceneState{values: make([]*Value[float32], 1), setters: make([]Setter[int], 1)}
	card := func(i int) Node {
		n, set := UseState(0)
		c := []Child{Key(i), Class("relative h-24 rounded-xl border border-slate-200 bg-white p-3"), Textf("Card %d (%d)", i, n)}
		switch i {
		case 2:
			st.setters[0] = set
		case 5:
			a := st.values[0]
			c = append(c, Style(style.BackgroundGradientFn(func() style.Gradient { return angleGradient(a.Get()) })))
		}
		return div(c...)
	}
	scene := func() Node {
		st.values[0] = UseValue[float32](90)
		items := make([]int, 40)
		for i := range items {
			items[i] = i
		}
		return div(Class("h-screen overflow-y-auto bg-slate-100"),
			div(Class("grid grid-cols-4 gap-3 p-3"),
				Map(items, func(i int) Node { return WithKey(ComponentProps(card, i), i) })))
	}
	x := newDamageHarness(t, Component(scene), 600, 400, 1, Config{}, 7)
	tr := x.tree()
	want := func(what string, leads ...string) {
		t.Helper()
		if got := recordedLeads(tr); fmt.Sprint(got) != fmt.Sprint(leads) {
			t.Errorf("%s recorded %v, want %v", what, got, leads)
		}
	}
	for i := range 6 {
		s0 := tr.Stats()
		st.values[0].Set(float32(90 + 15*(i+1)))
		x.frame(fmt.Sprintf("angle %d", i))
		if d := work(s0, tr.Stats()); d != paintOnly {
			t.Errorf("angle %d: work %+v, want a paint only", i, d)
		}
		want(fmt.Sprintf("angle %d", i), "scroll 5")
	}
	// Card 2's text changes: it builds and is laid out again, and every
	// box is placed again.
	s0 := tr.Stats()
	st.setters[0].Set(1)
	x.frame("text")
	if d := work(s0, tr.Stats()); d.Builds != 1 || d.Layouts == 0 {
		t.Errorf("the text: work %+v, want one build and a layout", d)
	}
	want("the text", "scroll 2")
}

// TestSameGradientCoversEveryField fails when style.Gradient gains a field
// that sameGradient does not compare.
func TestSameGradientCoversEveryField(t *testing.T) {
	want := []string{"Kind", "Angle", "Corner", "Shape", "Size", "Radius", "CenterX", "CenterY", "Interp", "Stops"}
	ty := reflect.TypeFor[style.Gradient]()
	var got []string
	for i := range ty.NumField() {
		got = append(got, ty.Field(i).Name)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("style.Gradient has the fields %v; sameGradient (recordstyle.go) compares %v: compare the new ones", got, want)
	}
	a, b := angleGradient(10), angleGradient(10)
	if !sameGradient(&a, &b) {
		t.Error("equal gradients differ")
	}
	b.Stops = append([]style.ColorStop(nil), b.Stops...)
	b.Stops[1].Color = gfx.RGB8(0, 255, 0)
	if sameGradient(&a, &b) || sameGradient(&a, nil) || !sameGradient(nil, nil) {
		t.Error("different gradients are the same")
	}
}
