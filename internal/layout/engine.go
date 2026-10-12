package layout

// Compute lays out the tree rooted at root into the given available space
// (for a window: DefiniteSpace of its logical size in both axes; for an
// intrinsic measurement: MinContentConstraint or MaxContentConstraint).
// Percentages on the root resolve against definite available space. The
// root is placed at (0, 0) (at the right edge in RTL) and is the
// containing block of absolutely positioned boxes without a positioned
// ancestor.
//
// Results are read from each node's Data (Layout; RoundedLayout after
// Round). Compute reuses the results of nodes that were not marked dirty
// since the previous run, so a second run with nothing dirty and the same
// available space does no work.
func (e *Engine) Compute(root Node, width, height AvailableSpace) {
	avail := availSize{width, height}
	for {
		e.computeRoot(root, avail)
		if !e.relayoutBoundaries() {
			return
		}
	}
}

// relayoutBoundaries lays out again the dirty relayout boundaries that the
// root pass did not reach, from their previous input. It returns true if a
// boundary's result changed in a way that its ancestors see (its size, its
// baseline or hoisted absolutely positioned descendants); its parent is
// then marked dirty and the root pass must run again.
func (e *Engine) relayoutBoundaries() bool {
	again := false
	for len(e.pending) > 0 {
		n := e.pending[len(e.pending)-1]
		e.pending = e.pending[:len(e.pending)-1]
		d := n.LayoutData()
		if !d.needLayout {
			continue
		}
		d.needLayout = false
		if !d.hasLast {
			e.markParentDirty(n)
			again = true
			continue
		}
		if _, ok := d.cache.get(&d.last); ok {
			continue // laid out by an ancestor in the root pass
		}
		old := d.lastOut
		out := e.layout(n, d.last)
		// The parent placed n with its old output; n's own layout keeps
		// its location and size and takes the new overflow.
		d.unrounded.ScrollableOverflow.X0 = out.overflow.l
		d.unrounded.ScrollableOverflow.X1 = out.overflow.r
		d.unrounded.ScrollableOverflow.Y0 = out.overflow.t
		d.unrounded.ScrollableOverflow.Y1 = out.overflow.b
		if out.size != old.size || out.firstBaseline != old.firstBaseline || out.escapes || old.escapes {
			e.markParentDirty(n)
			again = true
		}
	}
	return again
}

// MarkDirty records that n changed in a way that affects layout: its style,
// its measured content, or its list of children. The next Compute lays out
// n again, and its ancestors up to the nearest relayout boundary.
//
// A relayout boundary is a node whose size cannot depend on its content: a
// fixed width and height (px) and overflow other than visible or clip in
// both axes (ARCHITECTURE section 9). A change inside a boundary only lays
// out the boundary's subtree, unless the boundary's baseline changes or it
// hoists absolutely positioned descendants to an ancestor, in which case
// Compute continues upwards.
func (e *Engine) MarkDirty(n Node) {
	n.LayoutData().cache.clear()
	e.markParentDirty(n)
}

// markParentDirty invalidates the ancestors of n up to and including the
// nearest relayout boundary, which is queued for Compute.
func (e *Engine) markParentDirty(n Node) {
	p := n.LayoutData().parent
	for p != nil {
		d := p.LayoutData()
		cleared := d.cache.clear()
		if d.boundary && d.parent != nil {
			if !d.needLayout {
				d.needLayout = true
				e.pending = append(e.pending, p)
			}
			return
		}
		// An ancestor that was already dirty has dirty ancestors too.
		if !cleared {
			return
		}
		p = d.parent
	}
}
