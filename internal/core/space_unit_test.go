package core

import "testing"

func TestNegativeSpaceOverlaps(t *testing.T) {
	tr := newTestTree(t, 400, 400, div(
		div(Key("row"), Class("flex -space-x-2"),
			div(Key("a0"), Class("size-10 shrink-0")), div(Key("a1"), Class("size-10 shrink-0")), div(Key("a2"), Class("size-10 shrink-0"))),
		div(Key("col"), Class("flex flex-col -space-y-3"),
			div(Key("c0"), Class("h-10 w-10")), div(Key("c1"), Class("h-10 w-10")), div(Key("c2"), Class("h-10 w-10"))),
		div(Key("blk"), Class("-space-y-4"),
			div(Key("b0"), Class("h-10")), div(Key("b1"), Class("h-10")), div(Key("b2"), Class("h-10"))),
		div(Key("both"), Class("flex gap-2 -space-x-2"), div(Key("g0"), Class("size-10")), div(Key("g1"), Class("size-10"))),
	), nil)
	tr.Frame()
	// Each avatar starts 32 px after the previous one: 40 less the 8 of
	// the negative margin.
	wantRect(t, mustFind(t, tr, "a0"), 0, 0, 40, 40)
	wantRect(t, mustFind(t, tr, "a1"), 32, 0, 40, 40)
	wantRect(t, mustFind(t, tr, "a2"), 64, 0, 40, 40)
	wantRect(t, mustFind(t, tr, "row"), 0, 0, 400, 40)
	// Vertically (flex column and block flow): 40 less 12, and less 16.
	wantRect(t, mustFind(t, tr, "c1"), 0, 40+28, 40, 40)
	wantRect(t, mustFind(t, tr, "c2"), 0, 40+56, 40, 40)
	wantRect(t, mustFind(t, tr, "col"), 0, 40, 400, 3*40-2*12)
	wantRect(t, mustFind(t, tr, "b1"), 0, 40+96+24, 400, 40)
	wantRect(t, mustFind(t, tr, "blk"), 0, 40+96, 400, 3*40-2*16)
	// The gap and the space are independent: 8 px of gap, -8 of margin.
	wantRect(t, mustFind(t, tr, "g1"), 40, 40+96+88, 40, 40)
}

func TestSpaceCountsTheLastChildInTheDOMSense(t *testing.T) {
	tr := newTestTree(t, 400, 200, div(
		div(Key("hid"), Class("flex space-x-4"),
			div(Key("h0"), Class("size-10")), div(Key("h1"), Class("size-10")), div(Key("hx"), Class("hidden size-10"))),
		div(Key("own"), Class("flex space-x-4"),
			div(Key("o0"), Class("size-10 mr-1")), div(Key("o1"), Class("size-10"))),
	), nil)
	tr.Frame()
	// The hidden last child is still the last child: h1 keeps its margin,
	// which a flex container as wide as the window does not show; the
	// width of a fit-content one does (the gallery compares it with
	// Chromium).
	wantRect(t, mustFind(t, tr, "h1"), 56, 0, 40, 40)
	// A margin of the child on that side wins over the space.
	wantRect(t, mustFind(t, tr, "o1"), 44, 40, 40, 40)
}

func TestSpaceFollowsTheContainerStyle(t *testing.T) {
	// A restyle of the container (a class change) maps its children again.
	var set func(string)
	comp := func() Node {
		c, st := UseStateSet("flex space-x-4")
		set = st
		return div(Class(c), div(Key("x0"), Class("size-10")), div(Key("x1"), Class("size-10")))
	}
	tr := newTestTree(t, 400, 200, Component(comp), nil)
	tr.Frame()
	wantRect(t, mustFind(t, tr, "x1"), 56, 0, 40, 40)
	set("flex -space-x-2")
	tr.Frame()
	wantRect(t, mustFind(t, tr, "x1"), 32, 0, 40, 40)
	set("flex")
	tr.Frame()
	wantRect(t, mustFind(t, tr, "x1"), 40, 0, 40, 40)
}
