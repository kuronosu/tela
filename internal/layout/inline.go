package layout

// Inline formatting contexts with atomic inlines (CSS 2.1 sections 9.4.2
// and 10.8; Blitz lays out Parley's inline boxes the same way). A block
// container whose content is a paragraph lays the paragraph out itself
// (a leaf that measures, core's text): its children are the atomic
// inline-level boxes of the paragraph (inline-block, inline-flex,
// inline-grid and inline replaced elements), which the paragraph places on
// its lines. The node implements InlineContainer; layout sizes and lays
// out the children for it through Atoms, so they are cached and
// invalidated like any child.

// InlineContainer is implemented by nodes that lay out inline content
// with atomic inline-level children. A node whose InlineContent reports
// true is laid out like a leaf (its size styles, padding and border; its
// content box from MeasureInline instead of Measure), whatever its
// children.
type InlineContainer interface {
	// InlineContent reports whether the node's children are the atomic
	// inlines of its own content.
	InlineContent() bool
	// MeasureInline is Measurer.Measure for the content box, with access
	// to the children: Atoms sizes them and, in a full layout, places
	// them (every child should be placed). It must be a pure function of
	// its input, the node's content and the children's results.
	// Measurement.LastBaseline is the baseline of the last line, which an
	// inline-block's baseline comes from.
	MeasureInline(in MeasureInput, atoms *Atoms) Measurement
}

// Atoms gives MeasureInline access to the children of an inline container.
type Atoms struct {
	e    *Engine
	n    Node
	full bool // a full layout: Size lays the children out, Place positions them
	// off is the content box's offset in the container's border box.
	off   point
	items []atomItem
}

type atomItem struct {
	out                     output
	margin, border, padding rect
	sb                      point
	rel                     point // relative positioning offset
	loc                     point
	placed, sized           bool
}

// AtomSize is the margin box of an atomic inline and its baseline.
type AtomSize struct {
	Width, Height float32
	// Baseline is the distance from the top of the margin box to the
	// baseline, when HasBaseline is set; otherwise the bottom margin edge
	// sits on the line's baseline (CSS 2.1 section 10.8.1: replaced
	// boxes, inline-blocks without in-flow line boxes or with an overflow
	// other than visible).
	Baseline    float32
	HasBaseline bool
}

// Len returns the number of children.
func (a *Atoms) Len() int { return len(a.items) }

// Full reports a full layout: the last MeasureInline of a layout pass,
// whose sizes and placements are the children's layouts.
func (a *Atoms) Full() bool { return a.full }

// Intrinsic returns the min-content and max-content contributions of
// child i: its margin box widths (percentage margins count as zero).
func (a *Atoms) Intrinsic(i int) (minW, maxW float32) {
	c := a.n.LayoutChild(i)
	st := c.LayoutStyle()
	margin := st.Margin.resolveOrZero(none).hsum()
	ratio := st.aspectRatio()
	known := st.size().resolve(optSize{}).applyAspectRatio(ratio).maybeClamp(
		st.minSize().resolve(optSize{}).applyAspectRatio(ratio), st.maxSize().resolve(optSize{}).applyAspectRatio(ratio))
	if known.w.ok {
		return known.w.v + margin, known.w.v + margin
	}
	known.h = none
	minW = a.e.measureChildSize(a.n, c, known, optSize{}, availSize{MinContentConstraint, MinContentConstraint}, inherentSize, horizontal)
	maxW = a.e.measureChildSize(a.n, c, known, optSize{}, availSize{MaxContentConstraint, MaxContentConstraint}, inherentSize, horizontal)
	return minW + margin, maxW + margin
}

// Size returns the margin box and baseline of child i laid out in an
// inline formatting context whose content box is avail wide (CSS 2.1
// section 10.3.9: an auto width is fit-content within it). In a full
// layout the child is laid out completely; Place positions it.
func (a *Atoms) Size(i int, avail float32) AtomSize {
	if !(avail >= 0) || avail > inf {
		avail = 0
	}
	e, n := a.e, a.n
	c := n.LayoutChild(i)
	st := c.LayoutStyle()
	it := &a.items[i]
	basis := some(avail)
	parent := optSize{w: basis}
	ratio := st.aspectRatio()
	// Auto margins of inline-level boxes are 0 (CSS 2.1 10.3.9, 10.6.2).
	margin := st.Margin.resolveMargins(basis).or0()
	padding := st.Padding.resolveOrZero(basis)
	border := st.Border.resolveOrZero(basis)
	stretchW := fmax(0, avail-margin.hsum())
	sz := st.size().resolve(parent).applyAspectRatio(ratio)
	minSize := st.minSize().resolve(parent).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(parent).applyAspectRatio(ratio)
	var known optSize
	if !st.Replaced {
		w := sz.w
		if !w.ok {
			measure := func(space AvailableSpace) opt {
				return some(e.measureChildSize(n, c, optSize{}, parent, availSize{space, MaxContentConstraint}, inherentSize, horizontal))
			}
			switch k, s, exact := resolveSizingKeyword(st.Width, some(stretchW), basis); k {
			case keywordExact:
				w = some(exact)
			case keywordMeasure:
				w = measure(s)
			default:
				w = measure(DefiniteSpace(stretchW)) // auto: fit-content
			}
		}
		known = optSize{w: some(fclampOpt(w.v, minSize.w, maxSize.w)), h: sz.h}.maybeClamp(minSize, maxSize)
	}
	run := computeSize
	if a.full || st.Display == DisplayFlex || st.Display == DisplayGrid {
		// A flex or grid container's baseline needs its items' positions
		// (a size query that lays children out drops the cached layouts
		// that relied on them; see Engine.layout).
		run = performLayout
	}
	out := e.child(n, c, input{
		run: run, sizing: inherentSize, axis: axisBoth,
		known: known, knownDefinite: boolSize{true, true},
		parent: parent, avail: availSize{DefiniteSpace(stretchW), MaxContentConstraint},
	})
	var sb point
	if st.OverflowY == OverflowScroll {
		sb.x = st.ScrollbarWidth
	}
	if st.OverflowX == OverflowScroll {
		sb.y = st.ScrollbarWidth
	}
	it.out, it.margin, it.border, it.padding, it.sb, it.sized = out, margin, border, padding, sb, true
	it.rel = relativeOffset(st, parent, LTR)

	s := AtomSize{Width: out.size.w + margin.hsum(), Height: out.size.h + margin.vsum()}
	var b opt
	switch {
	case st.Replaced, st.OverflowX != OverflowVisible, st.OverflowY != OverflowVisible:
	case st.Display == DisplayFlex, st.Display == DisplayGrid:
		// inline-flex and inline-grid align by their first baseline.
		b = out.firstBaseline
	default:
		b = out.lastBaseline.or(out.firstBaseline)
	}
	if b.ok {
		s.Baseline, s.HasBaseline = margin.t+b.v, true
	}
	return s
}

// Place puts the margin box of child i at (x, y) in the container's
// content box. Only a full layout records it.
func (a *Atoms) Place(i int, x, y float32) {
	it := &a.items[i]
	it.loc = point{a.off.x + x + it.margin.l + it.rel.x, a.off.y + y + it.margin.t + it.rel.y}
	it.placed = true
}

// inline lays out an inline container: like a leaf, with MeasureInline as
// its measure, then, in a full layout, MeasureInline again in the final
// content box with the children laid out and placed.
func (e *Engine) inline(n Node, ic InlineContainer, st *Style, in input) output {
	a := &Atoms{e: e, n: n, items: make([]atomItem, n.LayoutChildCount())} // perf: reuse
	for i := range a.items {
		n.LayoutChild(i).LayoutData().parent = n
	}
	out := e.leafMeasure(n, st, in, func(known optSize, avail availSize) measured {
		return e.measureInline(ic, a, known, avail)
	})
	if in.run != performLayout {
		return out
	}
	parentW := in.parent.w
	padding := st.Padding.resolveOrZero(parentW)
	border := st.Border.resolveOrZero(parentW)
	g := st.scrollbarGutter()
	cw := fmax(0, out.size.w-padding.hsum()-border.hsum()-g.x)
	ch := fmax(0, out.size.h-padding.vsum()-border.vsum()-g.y)
	a.full = true
	a.off = point{border.l + padding.l, border.t + padding.t}
	ic.MeasureInline(MeasureInput{Width: cw, HasWidth: true, Height: ch, HasHeight: true,
		AvailableWidth: DefiniteSpace(cw), AvailableHeight: DefiniteSpace(ch)}, a)
	escapes := false
	for i := range a.items {
		it := &a.items[i]
		c := n.LayoutChild(i)
		if !it.sized {
			// Not measured by MeasureInline: give it a layout anyway.
			a.Size(i, cw)
		}
		if !it.placed {
			it.loc = a.off
		}
		cst := c.LayoutStyle()
		c.LayoutData().setLayout(it.loc, it.out.size, it.border, it.padding, it.margin, it.sb, it.out.overflow, it.out.firstBaseline)
		escapes = escapes || (it.out.escapes && cst.passesAbs())
		ol := point{it.loc.x - border.l, it.loc.y - border.t}
		out.overflow = out.overflow.union(overflowContribution(ol, it.out.size, it.out.overflow, cst, st.isScrollContainer()))
	}
	e.hideChildren(n)
	out.escapes = escapes
	return out
}

// measureInline calls MeasureInline like measure calls Measure.
func (e *Engine) measureInline(ic InlineContainer, a *Atoms, known optSize, avail availSize) measured {
	in := MeasureInput{
		Width: known.w.v, HasWidth: known.w.ok,
		Height: known.h.v, HasHeight: known.h.ok,
		AvailableWidth: avail.w, AvailableHeight: avail.h,
	}
	if in.AvailableWidth.Kind == Definite && !(in.AvailableWidth.Value > 0) {
		in.AvailableWidth.Value = 0
	}
	if in.AvailableHeight.Kind == Definite && !(in.AvailableHeight.Value > 0) {
		in.AvailableHeight.Value = 0
	}
	e.measureCalls++
	r := ic.MeasureInline(in, a)
	out := measured{w: r.Width, h: r.Height, base: r.Baseline, hasBaseline: r.HasBaseline, last: r.LastBaseline, hasLast: r.HasLastBaseline}
	if known.w.ok {
		out.w = known.w.v
	}
	if known.h.ok {
		out.h = known.h.v
	}
	return out
}

// inlineContainer returns n as an InlineContainer laying out its children
// as atomic inlines, or nil.
func inlineContainer(n Node) InlineContainer {
	if ic, ok := n.(InlineContainer); ok && ic.InlineContent() {
		return ic
	}
	return nil
}
