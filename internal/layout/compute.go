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
	// growH reports that known.h is a block's height from its width
	// through its aspect ratio, which taller content exceeds: the children
	// are laid out against it and the block grows to them (ratioHeight).
	// Only ratioHeight sets it, from the request the cache keys, so it is
	// not part of the key.
	growH bool
	// pctH is the height the items of a flex container whose height is not
	// known resolve their percentages against: its height from the aspect
	// ratio while ratioHeight measures its content. Only ratioHeight sets
	// it, so it is not part of the cache key either.
	pctH opt
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
	// escapesFixed reports that the subtree contains fixed boxes whose
	// containing block is above the node (fixed.go).
	escapesFixed bool
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
	content []Node // nodes passed to MarkContent since the last Compute

	// gridIdx and gridTmp are the scratch of sortItems.
	gridIdx []int32
	gridTmp []gridItem

	// measuring counts the computeSize runs on the call stack; clobbers
	// counts full layouts done inside them (see layout).
	measuring, clobbers int

	// Statistics for tests: Measure calls and uncached layout runs.
	measureCalls int
	runs         int
}

// Runs returns the number of layout algorithm runs since the Engine was
// created: the requests (full layouts and size queries) that missed the
// caches, one per node and request. A Compute with nothing dirty adds none.
func (e *Engine) Runs() int { return e.runs }

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
		out = e.ratioHeight(n, st, in, (*Engine).flex, false)
	case st.Display == DisplayGrid:
		out = e.ratioHeight(n, st, in, (*Engine).grid, false)
	default:
		out = e.ratioHeight(n, st, in, (*Engine).block, true)
	}
	if in.run == computeSize {
		e.measuring--
		if e.clobbers != clobbers {
			d.cache.hasFinal = false
		}
	}
	if in.run == performLayout && st.Display != DisplayNone {
		out = e.fixedPass(n, st, in, out)
	}
	d.cache.store(&in, out)
	if in.run == performLayout {
		d.last, d.lastOut, d.hasLast = in, out, true
		d.boundary = isBoundary(st)
		d.needLayout = false
	}
	return out
}

// ratioHeight runs a container algorithm (block, flex or grid) for a box
// with an aspect ratio whose height is not known but whose width is (from
// the parent or, when its own sizes apply, from its width style): the
// ratio gives the height, clamped by the min and max heights
// (css-sizing-4 section 5). While the box's min-height is auto and it is
// neither a scroll container nor replaced (Style.ratioGrowsHeight), that
// height is a minimum: taller content makes the box taller, up to its max
// height, as Chromium applies the automatic minimum size of a box with a
// preferred aspect ratio. A flex aspect-square box in a 30 px wide block
// is 30x30 (its content height before), an aspect-square block with 60 px
// of content 30x60 (30x30 before; ADR-268). The algorithms transfer only
// heights into widths themselves.
//
// A block (selfGrows) lays its children out against the ratio height and
// grows to them (input.growH), so their percentages resolve against it as
// in Chromium. A flex or grid container measures its content height first
// with an indefinite height (Chromium's intrinsic block size), then is laid
// out at the larger height; in that measurement a flex container's items
// resolve percentages against the ratio height (input.pctH) and a grid's
// count them as auto, as Chromium does.
func (e *Engine) ratioHeight(n Node, st *Style, in input, algo func(*Engine, Node, *Style, input) output, selfGrows bool) output {
	ratio := st.aspectRatio()
	if !ratio.ok || in.known.h.ok {
		return algo(e, n, st, in)
	}
	own := in.sizing == inherentSize
	w, wDefinite := in.known.w, in.knownDefinite.w
	var minH, maxH opt
	if own {
		// A height style of its own (or a sizing keyword) is the
		// algorithm's.
		if h := st.Height; !h.isAuto() && (h.resolveSize(in.parent.h).ok || h.isSizingKeyword()) {
			return algo(e, n, st, in)
		}
		if !w.ok {
			w = st.Width.resolveSize(in.parent.w).maybeClamp(st.MinWidth.resolveSize(in.parent.w), st.MaxWidth.resolveSize(in.parent.w))
			wDefinite = true
		}
		minH, maxH = st.MinHeight.resolveSize(in.parent.h), st.MaxHeight.resolveSize(in.parent.h)
	}
	if !w.ok {
		return algo(e, n, st, in)
	}
	h := fclampOpt(w.v/ratio.v, minH, maxH)
	grows := st.ratioGrowsHeight()
	if grows && selfGrows {
		in.growH = true
	} else if grows && !(in.run == computeSize && in.axis == axisHorizontal) {
		// perf: an extra size query of the box, uncached.
		probe := in
		probe.run, probe.axis, probe.pctH = computeSize, axisVertical, some(h)
		if st.Display == DisplayFlex && st.FlexDirection.isRow() {
			// A row's content height is measured with its items at their
			// max-content widths, as Chromium finds it: aspect-[2] text
			// in a flex row of 30 px is 30x15, not its 40 px of wrapped
			// lines (a column's or a grid's is 30x40).
			probe.sizing, probe.known.w, probe.avail.w = contentSize, none, MaxContentConstraint
		}
		content := algo(e, n, st, probe).size.h
		h = fmax(h, fminOpt(content, maxH))
	}
	in.known.h = some(h)
	in.knownDefinite.h = wDefinite
	return algo(e, n, st, in)
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
	if d.setHidden() {
		layoutChanged(n)
	}
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
			if ch.LayoutData().setHidden() {
				layoutChanged(ch)
			}
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
		minSize := st.minSize().resolve(parent).applyAspectRatioToWidth(ratio)
		maxSize := st.maxSize().resolve(parent).applyAspectRatioToWidth(ratio)
		clamped := st.size().resolve(parent).applyAspectRatioToWidth(ratio).maybeClamp(minSize, maxSize)
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
	setLayoutOf(root, loc, out.size, border, padding, margin, st.scrollbarGutter(), out.overflow, out.firstBaseline)
}

// minMaxDefinite returns min when min and max are both set and max <= min:
// the bounds then determine the size.
func minMaxDefinite(min, max opt) opt {
	if min.ok && max.ok && max.v <= min.v {
		return min
	}
	return none
}
