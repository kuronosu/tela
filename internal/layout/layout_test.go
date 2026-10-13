package layout

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
)

// Unit tests for what the fixtures do not cover well: the measure
// contract, caching and dirty marking, relayout boundaries, rounding,
// display: none, order and Tela's containing blocks.

func box(st Style, children ...*testNode) *testNode {
	return &testNode{style: st, children: children}
}

func styleOf(f func(*Style)) Style {
	st := DefaultStyle()
	f(&st)
	return st
}

// text returns a leaf with Ahem-like text: 10px glyphs, breaks at "​".
func text(s string) *testNode {
	return &testNode{style: DefaultStyle(), measure: ahemMeasure(s)}
}

func layoutOf(n *testNode) Layout { return *n.data.Layout() }

func rectOf(n *testNode) [4]float32 {
	l := n.data.Layout()
	return [4]float32{l.Location.X, l.Location.Y, l.Size.W, l.Size.H}
}

func wantRect(t *testing.T, name string, n *testNode, want [4]float32) {
	t.Helper()
	if got := rectOf(n); got != want {
		t.Errorf("%s: got x=%g y=%g w=%g h=%g, want x=%g y=%g w=%g h=%g", name, got[0], got[1], got[2], got[3], want[0], want[1], want[2], want[3])
	}
}

func compute(e *Engine, root *testNode, w, h float32) {
	e.Compute(root, DefiniteSpace(w), DefiniteSpace(h))
}

// TestMeasureContract checks what a text-like leaf is asked: known
// dimensions are passed through, available space is the content box's,
// and the first baseline propagates to the containers.
func TestMeasureContract(t *testing.T) {
	var calls []MeasureInput
	leaf := &testNode{style: styleOf(func(s *Style) {
		s.Padding = EdgesAll(Px(5))
		s.Border = EdgesAll(Px(1))
	})}
	m := ahemMeasure("HHHH\u200bHHHH\u200bHH")
	leaf.measure = func(in MeasureInput) Measurement {
		calls = append(calls, in)
		return m(in)
	}
	// A block container 70px wide: the leaf's content box is 70-12 = 58px
	// wide, which fits one 40px word per line ("HHHH" + "HH" is 60px):
	// three lines.
	root := box(styleOf(func(s *Style) { s.Width = Px(70) }), leaf)
	var e Engine
	compute(&e, root, 500, 500)

	wantRect(t, "leaf", leaf, [4]float32{0, 0, 70, 3*10 + 12})
	if len(calls) == 0 {
		t.Fatal("Measure was not called")
	}
	for _, in := range calls {
		if in.AvailableWidth.Kind == Definite && in.AvailableWidth.Value != 58 {
			t.Errorf("available width %g, want the content box width 58", in.AvailableWidth.Value)
		}
		if in.HasWidth && in.Width != 58 {
			t.Errorf("known width %g, want 58", in.Width)
		}
	}
	// The first baseline: border + padding + the text's 8px.
	if l := layoutOf(leaf); !l.HasBaseline || l.FirstBaseline != 14 {
		t.Errorf("leaf baseline %v %g, want 14", l.HasBaseline, l.FirstBaseline)
	}
	if l := layoutOf(root); !l.HasBaseline || l.FirstBaseline != 14 {
		t.Errorf("root baseline %v %g, want 14 (from its first child)", l.HasBaseline, l.FirstBaseline)
	}
	// A second run with nothing dirty does not measure again.
	n := len(calls)
	compute(&e, root, 500, 500)
	if len(calls) != n {
		t.Errorf("clean relayout measured %d more times", len(calls)-n)
	}
}

// TestFlexTextWrapping lays out text in a flex row: the min-content width
// limits shrinking and the text wraps to the flexed width.
func TestFlexTextWrapping(t *testing.T) {
	a := text("HH\u200bHH\u200bHH\u200bHH") // min 20, max 80
	b := text("HHHHHH")                     // min 60, max 60
	root := box(styleOf(func(s *Style) {
		s.Display = DisplayFlex
		s.Width = Px(100)
	}), a, b)
	var e Engine
	compute(&e, root, 500, 500)
	// 80 + 60 = 140 > 100: both shrink in proportion to their bases (80
	// and 60) by 40: a to 80-40*80/140, b to 60-40*60/140 = 42.86, which
	// is below its 60px min-content: b freezes at 60, a gets 40.
	wantRect(t, "a", a, [4]float32{0, 0, 40, 20})
	wantRect(t, "b", b, [4]float32{40, 0, 60, 20})
	wantRect(t, "root", root, [4]float32{0, 0, 100, 20})
}

// TestDisplayNone checks that a hidden subtree gets zero layouts and comes
// back when shown again.
func TestDisplayNone(t *testing.T) {
	inner := text("HHH")
	hidden := box(styleOf(func(s *Style) {
		s.Display = DisplayNone
		s.Width, s.Height = Px(50), Px(50)
	}), inner)
	after := box(styleOf(func(s *Style) { s.Height = Px(10) }))
	root := box(DefaultStyle(), hidden, after)
	var e Engine
	compute(&e, root, 200, 200)
	wantRect(t, "hidden", hidden, [4]float32{0, 0, 0, 0})
	wantRect(t, "inner", inner, [4]float32{0, 0, 0, 0})
	wantRect(t, "after", after, [4]float32{0, 0, 200, 10})

	hidden.style.Display = DisplayBlock
	e.MarkDirty(hidden)
	compute(&e, root, 200, 200)
	wantRect(t, "shown", hidden, [4]float32{0, 0, 50, 50})
	wantRect(t, "inner shown", inner, [4]float32{0, 0, 50, 10})
	wantRect(t, "after shown", after, [4]float32{0, 50, 200, 10})

	hidden.style.Display = DisplayNone
	e.MarkDirty(hidden)
	compute(&e, root, 200, 200)
	wantRect(t, "hidden again", hidden, [4]float32{0, 0, 0, 0})
	wantRect(t, "inner hidden again", inner, [4]float32{0, 0, 0, 0})
	wantRect(t, "after again", after, [4]float32{0, 0, 200, 10})
}

// TestOrder checks the order property: flex items are placed by order,
// ties in tree order.
func TestOrder(t *testing.T) {
	item := func(order int32) *testNode {
		return box(styleOf(func(s *Style) {
			s.Width, s.Height = Px(10), Px(10)
			s.Order = order
		}))
	}
	a, b, c, d := item(2), item(-1), item(0), item(2)
	root := box(styleOf(func(s *Style) { s.Display = DisplayFlex }), a, b, c, d)
	var e Engine
	compute(&e, root, 100, 100)
	// Order: b (-1), c (0), a (2), d (2).
	for name, w := range map[string]struct {
		n *testNode
		x float32
	}{"a": {a, 20}, "b": {b, 0}, "c": {c, 10}, "d": {d, 30}} {
		if got := w.n.data.Layout().Location.X; got != w.x {
			t.Errorf("%s at x=%g, want %g", name, got, w.x)
		}
	}
}

// TestContainingBlock checks that absolutely positioned boxes are placed
// against their nearest positioned ancestor, through static ancestors, and
// that static boxes ignore their insets.
func TestContainingBlock(t *testing.T) {
	abs := box(styleOf(func(s *Style) {
		s.Position = PositionAbsolute
		s.Inset = Edges{Top: Px(5), Right: Px(7), Bottom: Auto, Left: Auto}
		s.Width, s.Height = Px(20), Px(10)
	}))
	// A static abs box with auto insets stays at its static position.
	absStatic := box(styleOf(func(s *Style) {
		s.Position = PositionAbsolute
		s.Width, s.Height = Px(4), Px(4)
	}))
	inner := box(styleOf(func(s *Style) {
		s.Margin = Edges{Top: Px(30), Left: Px(40)}
		s.Width, s.Height = Px(50), Px(50)
		s.Inset = Edges{Top: Px(1000), Left: Px(1000)} // ignored: static
	}), abs, absStatic)
	middle := box(styleOf(func(s *Style) {
		s.Padding = EdgesAll(Px(3))
	}), inner)
	cb := box(styleOf(func(s *Style) {
		s.Position = PositionRelative
		s.Width, s.Height = Px(300), Px(200)
		s.Border = EdgesAll(Px(2))
	}), middle)
	root := box(DefaultStyle(), cb)
	var e Engine
	compute(&e, root, 1000, 1000)

	wantRect(t, "inner", inner, [4]float32{43, 33, 50, 50})
	// abs: right 7 from cb's padding box (2px border): its right edge at
	// x = 300-2-7 = 291 in cb, so x = 271 in cb; in inner's coordinates
	// (inner is at 2+3+40 = 45, 2+3+30 = 35 in cb) that is 226, -30.
	wantRect(t, "abs", abs, [4]float32{271 - 45, 2 + 5 - 35, 20, 10})
	// absStatic: inner's content box start.
	wantRect(t, "absStatic", absStatic, [4]float32{0, 0, 4, 4})

	// The positioned ancestor's scrollable overflow includes abs.
	if of := cb.data.Layout().ScrollableOverflow; of.X1 < 289 {
		t.Errorf("cb overflow %v does not include abs", of)
	}
}

// TestStaticFlexParentStaticPosition checks the static position of an
// absolutely positioned child of a static flex container: aligned as the
// only item of the container.
func TestStaticFlexParentStaticPosition(t *testing.T) {
	abs := box(styleOf(func(s *Style) {
		s.Position = PositionAbsolute
		s.Width, s.Height = Px(10), Px(20)
	}))
	flex := box(styleOf(func(s *Style) {
		s.Display = DisplayFlex
		s.JustifyContent = AlignCenter
		s.AlignItems = AlignFlexEnd
		s.Width, s.Height = Px(100), Px(60)
		s.Margin.Left = Px(7)
	}), abs)
	cb := box(styleOf(func(s *Style) {
		s.Position = PositionRelative
		s.Padding = EdgesAll(Px(10))
	}), flex)
	var e Engine
	compute(&e, cb, 500, 500)
	wantRect(t, "abs", abs, [4]float32{45, 40, 10, 20})
}

// TestCacheReuse checks that a change to one leaf only measures that leaf
// again and only lays out its ancestors.
func TestCacheReuse(t *testing.T) {
	var leaves []*testNode
	var rows []*testNode
	for r := 0; r < 20; r++ {
		var row []*testNode
		for c := 0; c < 10; c++ {
			l := text("HH\u200bHHH")
			leaves = append(leaves, l)
			row = append(row, l)
		}
		rows = append(rows, box(styleOf(func(s *Style) {
			s.Display = DisplayFlex
			s.ColumnGap = Px(4)
		}), row...))
	}
	root := box(DefaultStyle(), rows...)
	var e Engine
	compute(&e, root, 800, 600)
	runs := e.runs

	// Nothing dirty: no work.
	compute(&e, root, 800, 600)
	if e.runs != runs {
		t.Errorf("clean relayout ran %d algorithms", e.runs-runs)
	}

	// One leaf changes: it is measured again, the others are not.
	for _, l := range leaves {
		l.measures = 0
	}
	changed := leaves[37]
	changed.measure = ahemMeasure("HHHHHHHH")
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
	// The root, the changed row and the leaf run their algorithms (a few
	// times each for the leaf); nothing else does.
	if n := e.runs - runs; n > 12 {
		t.Errorf("%d layout runs after one leaf changed", n)
	}
	checkIncremental(t, root)
}

// TestRelayoutBoundary checks that a change inside a fixed-size box with
// overflow hidden does not lay out its ancestors, unless its baseline
// changes.
func TestRelayoutBoundary(t *testing.T) {
	leaf := text("HHH")
	boundary := box(styleOf(func(s *Style) {
		s.Width, s.Height = Px(100), Px(50)
		s.OverflowX, s.OverflowY = OverflowHidden, OverflowHidden
		s.Padding.Top = Px(5)
	}), leaf)
	sibling := text("HHHH")
	row := box(styleOf(func(s *Style) {
		s.Display = DisplayFlex
	}), boundary, sibling)
	root := box(DefaultStyle(), row)
	var e Engine
	compute(&e, root, 800, 600)

	leaf.measure = ahemMeasure("HHHHHH\u200bHHHHHH")
	e.MarkDirty(leaf)
	if !root.data.cache.hasFinal || !row.data.cache.hasFinal {
		t.Error("MarkDirty invalidated ancestors above the relayout boundary")
	}
	if len(e.pending) != 1 || e.pending[0] != Node(boundary) {
		t.Errorf("pending boundaries %v, want the boundary", e.pending)
	}
	runs := e.runs
	compute(&e, root, 800, 600)
	if n := e.runs - runs; n > 2 {
		t.Errorf("%d layout runs, want the boundary and the leaf only", n)
	}
	wantRect(t, "leaf", leaf, [4]float32{0, 5, 100, 20})
	checkIncremental(t, root)

	// In a baseline-aligned row the boundary's baseline matters: a change
	// that moves it lays out the row again.
	row.style.AlignItems = AlignBaseline
	e.MarkDirty(row)
	compute(&e, root, 800, 600)
	boundary.style.Padding.Top = Px(15)
	leaf.measure = ahemMeasure("HH")
	e.MarkDirty(leaf)
	// The boundary's padding changed too: its own style.
	e.MarkDirty(boundary)
	compute(&e, root, 800, 600)
	checkIncremental(t, root)

	leaf.measure = ahemMeasure("HH\u200bHH")
	e.MarkDirty(leaf)
	compute(&e, root, 800, 600)
	checkIncremental(t, root)
}

// TestBoundaryBaselineEscalation changes the content of a relayout
// boundary so that its baseline moves, and checks that its baseline-aligned
// sibling moves too.
func TestBoundaryBaselineEscalation(t *testing.T) {
	spacer := box(styleOf(func(s *Style) { s.Height = Px(0) }))
	leaf := text("HH")
	inner := box(DefaultStyle(), spacer, leaf)
	boundary := box(styleOf(func(s *Style) {
		s.Width, s.Height = Px(100), Px(80)
		s.OverflowX, s.OverflowY = OverflowHidden, OverflowHidden
	}), inner)
	sibling := text("HHHH")
	row := box(styleOf(func(s *Style) {
		s.Display = DisplayFlex
		s.AlignItems = AlignBaseline
	}), boundary, sibling)
	root := box(DefaultStyle(), row)
	var e Engine
	compute(&e, root, 800, 600)
	before := rectOf(sibling)

	spacer.style.Height = Px(30)
	e.MarkDirty(spacer)
	compute(&e, root, 800, 600)
	if rectOf(sibling) == before {
		t.Error("the sibling did not follow the boundary's baseline")
	}
	checkIncremental(t, root)
}

// TestHoistedAbsoluteInsideBoundary changes a static relayout boundary
// that contains an absolutely positioned box whose containing block is
// above the boundary.
func TestHoistedAbsoluteInsideBoundary(t *testing.T) {
	abs := box(styleOf(func(s *Style) {
		s.Position = PositionAbsolute
		s.Width, s.Height = Px(10), Px(10)
	}))
	leaf := text("HH")
	boundary := box(styleOf(func(s *Style) {
		s.Width, s.Height = Px(100), Px(80)
		s.OverflowX, s.OverflowY = OverflowHidden, OverflowHidden
	}), leaf, abs)
	root := box(styleOf(func(s *Style) { s.Position = PositionRelative }), boundary)
	var e Engine
	compute(&e, root, 800, 600)
	wantRect(t, "abs", abs, [4]float32{0, 10, 10, 10})

	leaf.measure = ahemMeasure("HH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH\u200bHH")
	e.MarkDirty(leaf)
	compute(&e, root, 800, 600)
	// 12 words of 20px in 100px: 3 lines.
	wantRect(t, "abs after", abs, [4]float32{0, 30, 10, 10})
	checkIncremental(t, root)
}

// TestRound checks the rounding pass: rounded edges stay adjacent and the
// rounded sizes add up.
func TestRound(t *testing.T) {
	var items []*testNode
	for i := 0; i < 3; i++ {
		items = append(items, box(styleOf(func(s *Style) {
			s.FlexGrow = 1
			s.Height = Px(10.4)
		})))
	}
	row := box(styleOf(func(s *Style) {
		s.Display = DisplayFlex
		s.Width = Px(100)
		s.Margin.Left = Px(0.3)
		s.Padding.Left = Px(0.4)
	}), items...)
	root := box(DefaultStyle(), row)
	var e Engine
	compute(&e, root, 800, 600)
	Round(root)

	// The row is at x = 0.3 with 0.4 of padding; its items are 99.6/3 =
	// 33.2 wide, so their cumulative edges are 0.7, 33.9, 67.1 and 100.3,
	// rounded 1, 34, 67 and 100: widths 33, 33 and 33 that touch, and the
	// last one ends where the row (0.3 to 100.3, rounded 0 to 100) ends.
	rl := row.data.RoundedLayout()
	if rl.Location.X != 0 || rl.Size.W != 100 || rl.Padding.Left != 1 {
		t.Errorf("row: x=%g w=%g padding-left=%g, want 0, 100, 1", rl.Location.X, rl.Size.W, rl.Padding.Left)
	}
	wantX := []float32{1, 34, 67}
	wantW := []float32{33, 33, 33}
	for i, it := range items {
		r := it.data.RoundedLayout()
		if r.Location.X != wantX[i] || r.Size.W != wantW[i] || r.Size.H != 10 {
			t.Errorf("item %d: x=%g w=%g h=%g, want x=%g w=%g h=10", i, r.Location.X, r.Size.W, r.Size.H, wantX[i], wantW[i])
		}
	}
	// The unrounded layout is kept.
	if u := row.data.Layout(); u.Location.X != 0.3 || math.Abs(float64(items[0].data.Layout().Size.W)-33.2) > 1e-4 {
		t.Errorf("the unrounded layout changed: %+v", *u)
	}
}

// checkIncremental lays out a copy of the tree from scratch and compares
// every layout with the incremental result.
func checkIncremental(t *testing.T, root *testNode) {
	t.Helper()
	fresh := cloneTree(root)
	var e Engine
	e.Compute(fresh, root.data.last.avail.w, root.data.last.avail.h)
	if msg := compareTrees(root, fresh, "root"); msg != "" {
		t.Errorf("incremental layout differs from a fresh one: %s", msg)
	}
}

func cloneTree(n *testNode) *testNode {
	c := &testNode{name: n.name, style: n.style, measure: n.measure}
	for _, ch := range n.children {
		c.children = append(c.children, cloneTree(ch))
	}
	return c
}

func compareTrees(a, b *testNode, path string) string {
	if la, lb := a.data.Layout(), b.data.Layout(); *la != *lb {
		return fmt.Sprintf("%s: %+v, fresh %+v", path, *la, *lb)
	}
	for i := range a.children {
		if m := compareTrees(a.children[i], b.children[i], fmt.Sprintf("%s/%d", path, i)); m != "" {
			return m
		}
	}
	return ""
}

// randomTree builds a random tree of flex and block containers and text
// leaves.
func randomTree(rng *rand.Rand, depth int) *testNode {
	st := DefaultStyle()
	if depth == 0 || rng.IntN(4) == 0 {
		words := 1 + rng.IntN(6)
		s := ""
		for i := 0; i < words; i++ {
			if i > 0 {
				s += "\u200b"
			}
			for j := 0; j <= rng.IntN(5); j++ {
				s += "H"
			}
		}
		n := text(s)
		randomizeStyle(rng, &n.style, true)
		return n
	}
	randomizeStyle(rng, &st, false)
	n := box(st)
	for i, c := 0, 1+rng.IntN(4); i < c; i++ {
		n.children = append(n.children, randomTree(rng, depth-1))
	}
	return n
}

func randomizeStyle(rng *rand.Rand, st *Style, leaf bool) {
	if !leaf && rng.IntN(2) == 0 {
		st.Display = DisplayFlex
		st.FlexDirection = FlexDirection(rng.IntN(4))
		st.FlexWrap = FlexWrap(rng.IntN(3))
		st.AlignItems = []Align{AlignNormal, AlignStart, AlignCenter, AlignBaseline, AlignFlexEnd}[rng.IntN(5)]
		st.JustifyContent = []Align{AlignNormal, AlignCenter, AlignSpaceBetween, AlignFlexEnd}[rng.IntN(4)]
		st.ColumnGap = Px(float32(rng.IntN(3)) * 3)
	}
	dims := []Dim{Auto, Auto, Auto, Px(40), Px(73.5), Percent(0.5), MinContent, FitContent}
	st.Width = dims[rng.IntN(len(dims))]
	if rng.IntN(3) == 0 {
		st.Height = []Dim{Px(30), Percent(0.4), Auto}[rng.IntN(3)]
	}
	if rng.IntN(4) == 0 {
		st.Padding = EdgesAll(Px(float32(rng.IntN(4))))
	}
	if rng.IntN(4) == 0 {
		st.Margin = Edges{Top: Px(1), Left: []Dim{Px(2), Auto, Percent(0.1)}[rng.IntN(3)]}
	}
	st.FlexGrow = float32(rng.IntN(2))
	st.FlexShrink = float32(rng.IntN(2))
	if rng.IntN(6) == 0 {
		st.Position = PositionAbsolute
		st.Inset.Top = Px(3)
	} else if rng.IntN(5) == 0 {
		st.Position = PositionRelative
	}
	if rng.IntN(6) == 0 {
		st.OverflowX, st.OverflowY = OverflowHidden, OverflowHidden
		if rng.IntN(2) == 0 {
			st.OverflowY = OverflowScroll
			st.ScrollbarWidth = 7
		}
	}
	if rng.IntN(15) == 0 {
		st.Display = DisplayNone
	}
	if rng.IntN(8) == 0 {
		st.Direction = RTL
	}
	if rng.IntN(8) == 0 {
		st.AspectRatio = 1.5
	}
	if rng.IntN(8) == 0 {
		st.MinWidth = Px(25)
		st.MaxHeight = Px(60)
	}
	if rng.IntN(8) == 0 {
		st.AlignSelf = AlignBaseline
	}
	if rng.IntN(10) == 0 {
		st.Order = int32(rng.IntN(3)) - 1
	}
	if !leaf && rng.IntN(8) == 0 {
		st.AlignContent = AlignCenter
	}
}

// nodes returns all nodes of a tree.
func nodes(n *testNode) []*testNode {
	out := []*testNode{n}
	for _, c := range n.children {
		out = append(out, nodes(c)...)
	}
	return out
}

// TestIncrementalRandom mutates random trees and checks that incremental
// layout always equals layout from scratch.
func TestIncrementalRandom(t *testing.T) {
	seed := uint64(1)
	if s := os.Getenv("TELA_LAYOUT_SEED"); s != "" {
		seed, _ = strconv.ParseUint(s, 10, 64)
	}
	rng := rand.New(rand.NewPCG(seed, 2))
	iters := 300
	if testing.Short() {
		iters = 60
	}
	if s := os.Getenv("TELA_LAYOUT_STRESS"); s != "" {
		iters, _ = strconv.Atoi(s)
	}
	for iter := 0; iter < iters; iter++ {
		root := randomTree(rng, 4)
		root.style.Position = PositionRelative
		var e Engine
		w, h := float32(200+rng.IntN(400)), float32(300)
		compute(&e, root, w, h)
		checkIncremental(t, root)
		all := nodes(root)
		for step := 0; step < 8; step++ {
			n := all[rng.IntN(len(all))]
			switch rng.IntN(3) {
			case 0:
				if n.measure != nil {
					n.measure = ahemMeasure("HHH\u200bH")
				}
			case 1:
				randomizeStyle(rng, &n.style, len(n.children) == 0)
			case 2:
				n.style.Width = Px(float32(rng.IntN(100)))
				n.style.Height = Px(float32(rng.IntN(100)))
				n.style.OverflowX, n.style.OverflowY = OverflowHidden, OverflowHidden
			}
			e.MarkDirty(n)
			compute(&e, root, w, h)
			if t.Failed() {
				return
			}
			checkIncremental(t, root)
			if t.Failed() {
				t.Logf("iteration %d, step %d, changed %s\n%s", iter, step, pathOf(root, n, "root"), dumpStyles(root, ""))
				return
			}
		}
	}
}

// TestNaNAndInfinite checks that odd inputs do not crash or produce NaN.
func TestNaNAndInfinite(t *testing.T) {
	nan := float32(math.NaN())
	leaf := text("HH")
	root := box(styleOf(func(s *Style) {
		s.Display = DisplayFlex
		s.AspectRatio = nan
		s.FlexGrow = 1
	}), leaf, box(styleOf(func(s *Style) {
		s.Width = Px(-10)
		s.MaxWidth = Px(-5)
		s.FlexBasis = Percent(2)
	})))
	var e Engine
	e.Compute(root, MaxContentConstraint, MaxContentConstraint)
	e.Compute(root, MinContentConstraint, DefiniteSpace(0))
	for _, n := range nodes(root) {
		l := n.data.Layout()
		for _, v := range []float32{l.Location.X, l.Location.Y, l.Size.W, l.Size.H} {
			if v != v {
				t.Fatalf("NaN in layout %+v", *l)
			}
		}
	}
}

func pathOf(root, n *testNode, p string) string {
	if root == n {
		return p
	}
	for i, c := range root.children {
		if q := pathOf(c, n, fmt.Sprintf("%s/%d", p, i)); q != "" {
			return q
		}
	}
	return ""
}

func dumpStyles(n *testNode, indent string) string {
	st := n.style
	s := fmt.Sprintf("%sdisp=%d pos=%d dir=%d wrap=%d ai=%d jc=%d w=%v h=%v pad=%v mar=%v grow=%g shrink=%g ovf=%d text=%v loc=%v size=%v\n", indent,
		st.Display, st.Position, st.FlexDirection, st.FlexWrap, st.AlignItems, st.JustifyContent, st.Width, st.Height, st.Padding.Top, st.Margin, st.FlexGrow, st.FlexShrink, st.OverflowX, n.measure != nil, n.data.Layout().Location, n.data.Layout().Size)
	for _, c := range n.children {
		s += dumpStyles(c, indent+"  ")
	}
	return s
}
