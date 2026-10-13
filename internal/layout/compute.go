// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/tree/layout.rs, src/compute/mod.rs and the
// dispatch of src/tree/taffy_tree.rs); see THIRD_PARTY.md.

package layout

// runMode says whether a request needs a full layout or only a size.
type runMode uint8

const (
	// performLayout computes the node's size and lays out its subtree.
	performLayout runMode = iota
	// computeSize only computes the node's size (an intrinsic size query).
	computeSize
	// performHiddenLayout gives the node and its subtree zero layouts
	// (an ancestor is display: none).
	performHiddenLayout
)

// sizingMode says whether the node's own size styles apply.
type sizingMode uint8

const (
	// inherentSize applies the node's size, min and max styles.
	inherentSize sizingMode = iota
	// contentSize ignores them: only the content contributes.
	contentSize
)

// requestedAxis says which axes of the result the caller uses.
type requestedAxis uint8

const (
	axisBoth requestedAxis = iota
	axisHorizontal
	axisVertical
)

func axisRequest(a axis) requestedAxis {
	if a == horizontal {
		return axisHorizontal
	}
	return axisVertical
}

// boolSize is a pair of flags, one per axis.
type boolSize struct{ w, h bool }

// input is a layout request: Taffy's LayoutInput. Constraints go down as
// known dimensions (sizes the node must take), available space (the soft
// limit text wraps to, or a min/max-content constraint) and the parent size
// (the basis of percentages).
type input struct {
	run    runMode
	sizing sizingMode
	axis   requestedAxis
	// known holds dimensions the node must take ("what is your height at
	// this width?"). knownDefinite says whether they are definite for the
	// node's own content (a flex item's post-flexing main size is not
	// definite when the container's main size is not).
	known         optSize
	knownDefinite boolSize
	parent        optSize
	avail         availSize
}

// output is a layout result: Taffy's LayoutOutput. Sizes go up.
type output struct {
	size          size
	overflow      rect // the scrollable overflow rectangle (see Layout)
	firstBaseline opt
	// lastBaseline is the baseline of the last line box (CSS 2.1 section
	// 10.8.1, the baseline of an inline-block), measured like
	// firstBaseline; flex containers report their first baseline.
	lastBaseline opt
	// escapes reports that the subtree contains absolutely positioned
	// boxes whose containing block is above the node: the nearest
	// positioned ancestor lays them out.
	escapes bool
}

func outerSize(s size) output { return output{size: s} }

// Engine lays out trees of Nodes and keeps the relayout boundaries marked
// dirty since the last Compute. The zero Engine is ready to use. An Engine
// is not safe for concurrent use; it may lay out several trees.
//
// perf: the container algorithms allocate their item lists on every run;
// per-depth buffers kept here would remove most allocations of a cold
// layout.
type Engine struct {
	pending []Node // dirty relayout boundaries

	// measuring counts the computeSize runs on the call stack; clobbers
	// counts full layouts done inside them (see layout).
	measuring, clobbers int

	// Statistics for tests: Measure calls and uncached layout runs.
	measureCalls int
	runs         int
}

// layout computes the layout of node n for request in, from the cache if
// possible (Taffy's compute_child_layout).
func (e *Engine) layout(n Node, in input) output {
	if in.run == performHiddenLayout {
		return e.hidden(n)
	}
	d := n.LayoutData()
	if out, ok := d.cache.get(&in); ok {
		return out
	}
	e.runs++
	// A size query can lay out descendants fully (flex baselines). That
	// replaces their layouts, which the cached full layouts of the nodes
	// being measured relied on: those entries are dropped, so the next
	// full layout of each lays its subtree out again.
	clobbers := e.clobbers
	switch in.run {
	case computeSize:
		e.measuring++
	case performLayout:
		if e.measuring > 0 {
			e.clobbers++
		}
	}
	st := n.LayoutStyle()
	var out output
	switch {
	case st.Display == DisplayNone:
		out = e.hidden(n)
	case n.LayoutChildCount() == 0:
		out = e.leaf(n, st, in)
	case inlineContainer(n) != nil:
		out = e.inline(n, inlineContainer(n), st, in)
	case st.Display == DisplayFlex:
		out = e.flex(n, st, in)
	default:
		out = e.block(n, st, in)
	}
	if in.run == computeSize {
		e.measuring--
		if e.clobbers != clobbers {
			d.cache.hasFinal = false
		}
	}
	d.cache.store(&in, out)
	if in.run == performLayout {
		d.last, d.lastOut, d.hasLast = in, out, true
		d.boundary = isBoundary(st)
		d.needLayout = false
	}
	return out
}

// child lays out child c of parent p, recording p as its parent.
func (e *Engine) child(p, c Node, in input) output {
	c.LayoutData().parent = p
	return e.layout(c, in)
}

// measureChildSize computes one axis of a child's size.
func (e *Engine) measureChildSize(p, c Node, known, parent optSize, avail availSize, sizing sizingMode, a axis) float32 {
	return e.child(p, c, input{
		run: computeSize, sizing: sizing, axis: axisRequest(a),
		known: known, knownDefinite: boolSize{true, true}, parent: parent, avail: avail,
	}).size.get(a)
}

// measureChildSizeBoth computes a child's size in both axes.
func (e *Engine) measureChildSizeBoth(p, c Node, known, parent optSize, avail availSize, sizing sizingMode) size {
	return e.child(p, c, input{
		run: computeSize, sizing: sizing, axis: axisBoth,
		known: known, knownDefinite: boolSize{true, true}, parent: parent, avail: avail,
	}).size
}

// performChildLayout lays out a child fully.
func (e *Engine) performChildLayout(p, c Node, known, parent optSize, avail availSize, sizing sizingMode) output {
	return e.child(p, c, input{
		run: performLayout, sizing: sizing, axis: axisBoth,
		known: known, knownDefinite: boolSize{true, true}, parent: parent, avail: avail,
	})
}

// hidden gives n and its subtree zero layouts (display: none).
func (e *Engine) hidden(n Node) output {
	d := n.LayoutData()
	d.cache.clear()
	d.setHidden()
	// A hidden node is not a relayout boundary: a change inside it must
	// not lay it out on its own.
	d.boundary, d.hasLast, d.needLayout = false, false, false
	for i, c := 0, n.LayoutChildCount(); i < c; i++ {
		ch := n.LayoutChild(i)
		ch.LayoutData().parent = n
		e.layout(ch, input{run: performHiddenLayout})
	}
	return output{}
}

// hideChildren gives the display: none children of n zero layouts; the
// container algorithms call it after laying out the other children.
func (e *Engine) hideChildren(n Node) {
	for i, c := 0, n.LayoutChildCount(); i < c; i++ {
		ch := n.LayoutChild(i)
		if ch.LayoutStyle().Display == DisplayNone {
			ch.LayoutData().setHidden()
			e.child(n, ch, input{run: performLayout, sizing: inherentSize, axis: axisBoth,
				knownDefinite: boolSize{true, true}, avail: availSize{MaxContentConstraint, MaxContentConstraint}})
		}
	}
}

// isBoundary reports whether a node with style st is a relayout boundary:
// its size cannot depend on its descendants, so a change inside it does not
// require its parent to lay out again (ARCHITECTURE section 9: an explicit
// size and overflow other than visible). Its baseline and its absolutely
// positioned descendants still can; Compute checks those after laying the
// boundary out again.
func isBoundary(st *Style) bool {
	return st.Display != DisplayNone &&
		st.Width.Unit == UnitPx && st.Height.Unit == UnitPx &&
		st.OverflowX != OverflowVisible && st.OverflowY != OverflowVisible &&
		st.OverflowX != OverflowClip && st.OverflowY != OverflowClip
}

// computeRoot lays out the root of a tree (Taffy's compute_root_layout).
func (e *Engine) computeRoot(root Node, avail availSize) {
	st := root.LayoutStyle()
	parent := avail.opts()
	var known optSize
	if st.Display == DisplayBlock {
		// A block root stretches to the available width, like any block.
		ratio := st.aspectRatio()
		margin := st.Margin.resolveOrZero(parent.w)
		pb := st.Padding.resolveOrZero(parent.w).add(st.Border.resolveOrZero(parent.w)).sum()
		minSize := st.minSize().resolve(parent).applyAspectRatio(ratio)
		maxSize := st.maxSize().resolve(parent).applyAspectRatio(ratio)
		clamped := st.size().resolve(parent).applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
		minMax := optSize{minMaxDefinite(minSize.w, maxSize.w), minMaxDefinite(minSize.h, maxSize.h)}
		stretch := optSize{w: avail.w.opt().subF(margin.hsum())}
		known = known.or(minMax).or(clamped).or(stretch).maybeMaxF(pb)
	}
	out := e.performChildLayout(nil, root, known, parent, avail, inherentSize)
	root.LayoutData().parent = nil

	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)
	margin := st.Margin.resolveOrZero(parent.w)
	var loc point
	if st.Direction == RTL && avail.w.isDefinite() {
		loc.x = avail.w.Value - out.size.w
	}
	root.LayoutData().setLayout(loc, out.size, border, padding, margin, st.scrollbarGutter(), out.overflow, out.firstBaseline)
}

// minMaxDefinite returns min when min and max are both set and max <= min:
// the bounds then determine the size.
func minMaxDefinite(min, max opt) opt {
	if min.ok && max.ok && max.v <= min.v {
		return min
	}
	return none
}
