package layout

import "testing"

// TestFixed: a fixed node is placed against the root, through positioned
// and flex ancestors, at its static position where its insets are auto,
// against an ancestor with ContainingBlock, and adds nothing to the
// root's scrollable overflow.
func TestFixed(t *testing.T) {
	fixed := func(f func(*Style)) *testNode {
		return box(styleOf(func(s *Style) {
			s.Position, s.Fixed = PositionAbsolute, true
			s.Width, s.Height = Px(10), Px(10)
			f(s)
		}))
	}
	full := fixed(func(s *Style) {
		s.Width, s.Height = Auto, Auto
		s.Inset = EdgesAll(Px(0))
	})
	corner := fixed(func(s *Style) { s.Inset.Right, s.Inset.Bottom = Px(5), Px(-500) })
	static := fixed(func(s *Style) {})
	inCB := fixed(func(s *Style) { s.Inset.Left, s.Inset.Top = Px(1), Px(2) })
	abs := box(styleOf(func(s *Style) {
		s.Position = PositionAbsolute
		s.Inset.Right, s.Inset.Top = Px(0), Px(0)
		s.Width, s.Height = Px(4), Px(4)
	}))
	cb := box(styleOf(func(s *Style) {
		s.ContainingBlock = true
		s.Margin.Left = Px(7)
		s.Width, s.Height = Px(50), Px(40)
	}), inCB, abs)
	rel := box(styleOf(func(s *Style) {
		s.Position = PositionRelative
		s.Inset.Left = Px(3)
		s.Padding = EdgesAll(Px(10))
		s.Display = DisplayFlex
	}), box(styleOf(func(s *Style) { s.Width, s.Height = Px(20), Px(20) })), static, full, corner)
	root := box(styleOf(func(s *Style) {
		s.OverflowX, s.OverflowY = OverflowAuto, OverflowAuto
		s.Width, s.Height = Px(200), Px(100)
	}), rel, cb)
	var e Engine
	compute(&e, root, 200, 100)
	// rel is at (3, 0); its children are relative to it.
	wantRect(t, "full", full, [4]float32{-3, 0, 200, 100})
	wantRect(t, "corner", corner, [4]float32{200 - 5 - 10 - 3, 100 + 500 - 10, 10, 10})
	// Static position: the flex static position, the content box's start
	// (as if it were the only item, CSS Flexbox 4.1).
	wantRect(t, "static", static, [4]float32{10, 10, 10, 10})
	wantRect(t, "in containing block", inCB, [4]float32{1, 2, 10, 10})
	wantRect(t, "absolute in containing block", abs, [4]float32{46, 0, 4, 4})
	if b := root.data.Layout().ScrollableOverflow.Y1; b > 100 {
		t.Errorf("root overflow reaches %g: fixed nodes must not add to it", b)
	}

	// A change inside the fixed node's relayout boundary lays it out
	// again against the root.
	corner.style.Width = Percent(0.5)
	e.MarkDirty(corner)
	compute(&e, root, 200, 100)
	wantRect(t, "corner 50%", corner, [4]float32{200 - 5 - 100 - 3, 100 + 500 - 10, 100, 10})
}

// TestFixedAndAbsoluteInGrids: a positioned grid lays out its absolute
// children but not its fixed ones (the root does, from the grid's content
// box as their static position); a transformed (ContainingBlock) static
// grid contains both.
func TestFixedAndAbsoluteInGrids(t *testing.T) {
	child := func(fixed bool, f func(*Style)) *testNode {
		return box(styleOf(func(s *Style) {
			s.Position, s.Fixed = PositionAbsolute, fixed
			s.Width, s.Height = Px(10), Px(10)
			f(s)
		}))
	}
	grid := func(f func(*Style), kids ...*testNode) *testNode {
		g := gridOf(repeatTracks(2, Track(Px(40))), repeatTracks(1, Track(Px(30))), kids...)
		g.style.Margin.Left, g.style.Margin.Top = Px(20), Px(10)
		g.style.Padding = EdgesAll(Px(5))
		f(&g.style)
		return g
	}
	// Positioned grid.
	absP := child(false, func(s *Style) { s.Inset.Right, s.Inset.Bottom = Px(0), Px(0) })
	fixP := child(true, func(s *Style) { s.Inset.Right, s.Inset.Bottom = Px(0), Px(0) })
	fixStatic := child(true, func(*Style) {})
	pos := grid(func(s *Style) { s.Position = PositionRelative }, absP, fixP, fixStatic)
	// Transformed static grid.
	absT := child(false, func(s *Style) { s.Inset.Left, s.Inset.Top = Px(1), Px(2) })
	fixT := child(true, func(s *Style) { s.Inset.Left, s.Inset.Top = Px(3), Px(4) })
	tr := grid(func(s *Style) { s.ContainingBlock = true }, absT, fixT)
	root := box(styleOf(func(s *Style) { s.Width, s.Height = Px(200), Px(150) }), pos, tr)
	var e Engine
	compute(&e, root, 200, 150)
	// The positioned grid stretches: its border box is 180x40 at (20, 10).
	wantRect(t, "absolute in a positioned grid", absP, [4]float32{170, 30, 10, 10})
	wantRect(t, "fixed in a positioned grid", fixP, [4]float32{200 - 10 - 20, 150 - 10 - 10, 10, 10})
	wantRect(t, "fixed at its static position", fixStatic, [4]float32{5, 5, 10, 10})
	wantRect(t, "absolute in a transformed grid", absT, [4]float32{1, 2, 10, 10})
	wantRect(t, "fixed in a transformed grid", fixT, [4]float32{3, 4, 10, 10})
}
