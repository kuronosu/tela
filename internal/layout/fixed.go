package layout

// Fixed positioning (CSS 2 section 9.6.1, ARCHITECTURE section 9). A fixed
// node is out of flow like an absolute one, but its containing block is
// the viewport (the layout root) unless an ancestor has ContainingBlock
// (a transform, filter or backdrop filter in CSS); positioned ancestors do
// not contain it.
//
// The container algorithms treat a fixed child like an absolute one for
// flow (it takes no space, its parent records its static position) but do
// not lay it out. Instead, after any node's full layout, fixedPass looks
// at its children: a fixed child, or a child whose subtree reports fixed
// nodes that escape it (output.escapesFixed), means the node's subtree
// holds fixed nodes. A node that is their containing block lays them out
// against its padding box, walking down the children that report them;
// any other node reports them upwards. A container algorithm only records
// a fixed child's static position and skips it (block, flex and grid do);
// the rest is here, the same for every kind of container.

// fixedContainer reports whether a node with style st is the containing
// block of its fixed descendants; root reports the layout root.
func fixedContainer(st *Style, root bool) bool { return st.ContainingBlock || root }

// fixedPass lays out the fixed nodes of n's subtree if n is their
// containing block, or reports them in out.escapesFixed.
func (e *Engine) fixedPass(n Node, st *Style, in input, out output) output {
	found := false
	for i, count := 0, n.LayoutChildCount(); i < count && !found; i++ {
		ch := n.LayoutChild(i)
		cst := ch.LayoutStyle()
		if cst.Display == DisplayNone {
			continue
		}
		if cst.isFixed() {
			found = true
		} else if d := ch.LayoutData(); d.hasLast && d.lastOut.escapesFixed && !cst.ContainingBlock {
			found = true
		}
	}
	if !found {
		return out
	}
	root := n.LayoutData().parent == nil
	if !fixedContainer(st, root) {
		out.escapesFixed = true
		return out
	}
	// The containing block is n's padding box (without the scrollbar
	// gutters), as for absolutely positioned children.
	pw := in.parent.w.unwrapOr(out.size.w)
	inset := st.Border.resolveOrZero(some(pw))
	g := st.scrollbarGutter()
	inset.b += g.y
	if st.Direction == RTL {
		inset.l += g.x
	} else {
		inset.r += g.x
	}
	area := out.size.sub(inset.sum())
	areaOffset := point{inset.l, inset.t}
	var of rect
	e.walkFixed(n, point{}, area, areaOffset, st.Direction, st.isScrollContainer(), &of)
	if !root {
		// A fixed node does not scroll with the viewport, so it adds
		// nothing to its scrollable overflow; a transformed containing
		// block contains it like an absolute one.
		out.overflow = out.overflow.union(of)
	}
	return out
}

// walkFixed lays out the fixed nodes below x whose containing block is the
// node the walk started from; off is x's position from that node's border
// box.
func (e *Engine) walkFixed(x Node, off point, area size, areaOffset point, dir Direction, scrolls bool, of *rect) {
	for i, count := 0, x.LayoutChildCount(); i < count; i++ {
		ch := x.LayoutChild(i)
		st := ch.LayoutStyle()
		if st.Display == DisplayNone {
			continue
		}
		d := ch.LayoutData()
		if st.isFixed() {
			*of = of.union(e.layoutAbsolute(x, ch, st, area, areaOffset, d.static.translate(off), dir, scrolls, off))
		}
		// Its own subtree (laid out above when it is fixed itself).
		if d.hasLast && d.lastOut.escapesFixed && !st.ContainingBlock {
			l := d.unrounded.Location
			e.walkFixed(ch, point{off.x + l.X, off.y + l.Y}, area, areaOffset, dir, scrolls, of)
		}
	}
}
