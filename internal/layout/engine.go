package layout

// Compute lays out the tree rooted at root into the given available space
// (for a window: DefiniteSpace of its logical size in both axes; for an
// intrinsic measurement: MinContentConstraint or MaxContentConstraint).
// Percentages on the root resolve against definite available space. The
// root is placed at (0, 0) (at the right edge in RTL) and is the
// containing block of absolutely positioned boxes without a positioned
// ancestor.
//
// Results are read from each node's Data (Layout). Compute reuses the
// results of nodes that were not marked dirty since the previous run, so a
// second run with nothing dirty and the same available space does no work.
func (e *Engine) Compute(root Node, width, height AvailableSpace) {
	avail := availSize{width, height}
	e.settleContent()
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
		if out.size != old.size || out.firstBaseline != old.firstBaseline || out.escapes || old.escapes ||
			out.escapesFixed || old.escapesFixed {
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

// MarkContent records that the content of n changed (its text, its
// image) and nothing else: neither its style nor its children. Its
// parent's algorithm reads only n's results, so the next Compute first
// lays n out again for every request its cache holds (its size queries
// and its last full layout); if each gives what it gave before, its
// ancestors keep their layouts. Otherwise its parent, whose style did not
// change either, is checked the same way with n's new results, and so on
// up: the first ancestor whose results hold stops it, and one that cannot
// be checked cheaply (many children, absolutely positioned descendants it
// does not contain, a paragraph with atomic inlines, a cache that forgot
// a result) is laid out again with its ancestors, as MarkDirty does. A
// node that cannot be checked itself (one with children, never laid out,
// hidden) is MarkDirty. A node whose parent reads more of it than its
// results (an InlinePlacer whose anchored boxes or containing blocks
// move) must use MarkDirty.
func (e *Engine) MarkContent(n Node) {
	d := n.LayoutData()
	if d.contentDirty {
		return
	}
	d.contentDirty = true
	e.content = append(e.content, n)
}

// Limits of the checks of MarkContent: a container with more children is
// laid out again rather than checked, and one settle checks at most about
// contentBudget children's results in all.
const (
	contentKids   = 32
	contentBudget = 1024
)

// settleContent checks the nodes passed to MarkContent and the ancestors
// their new results reach (see MarkContent).
func (e *Engine) settleContent() {
	for i := 0; i < len(e.content); i++ {
		n := e.content[i]
		d := n.LayoutData()
		if !d.contentDirty {
			continue
		}
		d.contentDirty = false
		e.settle(n)
	}
	clear(e.content)
	e.content = e.content[:0]
}

// settle checks leaf n, then its ancestors while their results change.
// A leaf whose cache cannot be checked (it forgot a result, or was never
// laid out) is taken as changed: its parent's check measures it again.
func (e *Engine) settle(n Node) {
	if n.LayoutChildCount() != 0 {
		e.MarkDirty(n)
		return
	}
	budget := contentBudget
	same, checked := false, false
	if e.checkable(n) {
		same, checked = e.sameResults(n, &budget)
	}
	for !same {
		if !checked {
			n.LayoutData().cache.clear() // its results are unknown
		}
		// n holds its new results (or none); its parent's were computed
		// from the old ones.
		p := n.LayoutData().parent
		if p == nil {
			return // the root: computeRoot takes its new layout
		}
		if p.LayoutChildCount() > contentKids || inlineContainer(p) != nil || !e.checkable(p) {
			e.markParentDirty(n)
			return
		}
		if same, checked = e.sameResults(p, &budget); !checked {
			e.MarkDirty(p) // over the budget
			return
		}
		n = p
	}
}

// checkable reports whether n's cache can be checked by sameResults: it
// holds its last full layout and every size result computed since it was
// cleared, and n's layout does not lay out absolutely positioned or fixed
// descendants that an ancestor contains (that ancestor reads them).
func (e *Engine) checkable(n Node) bool {
	d := n.LayoutData()
	c := &d.cache
	return d.hasLast && c.hasFinal && !c.evicted && c.finalKey == makeKey(&d.last) &&
		!c.final.escapes && !c.final.escapesFixed
}

// sameResults lays n, which is checkable, out again for every request its
// cache holds and reports whether every result is the one cached (checked
// is false, and nothing is done, when the budget, counted in children, is
// spent). The size queries go first and the last full layout last, so
// n's subtree ends laid out as its parent placed it. A size query is
// compared in the axes it was asked for.
func (e *Engine) sameResults(n Node, budget *int) (same, checked bool) {
	d := n.LayoutData()
	c := &d.cache
	runs := 1
	for i := range c.used {
		if c.used[i] {
			runs++
		}
	}
	if *budget -= runs * (1 + n.LayoutChildCount()); *budget < 0 {
		return false, false
	}
	old := *c
	last := d.last
	*c = cache{}
	same = true
	for i := range old.entries {
		if !old.used[i] {
			continue
		}
		en := &old.entries[i]
		out := e.layout(n, en.key.request(computeSize))
		switch en.key.axis {
		case axisHorizontal:
			same = same && out.size.w == en.size.w
		case axisVertical:
			same = same && out.size.h == en.size.h
		default:
			same = same && out.size == en.size
		}
		same = same && out.firstBaseline == en.base && out.lastBaseline == en.last
	}
	out := e.layout(n, last)
	return same && out == old.final, true
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
