// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/leaf.rs); see THIRD_PARTY.md.

package layout

// leaf computes the layout of a node without children: its size comes from
// its size styles or, where they are auto, from Measure (Taffy's
// compute_leaf_layout).
func (e *Engine) leaf(n Node, st *Style, in input) output {
	known, parent, avail := in.known, in.parent, in.avail

	// Percentage padding and borders resolve against the containing
	// block's width on all four sides (CSS).
	margin := st.Margin.resolveOrZero(parent.w)
	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)
	pb := padding.add(border)
	pbSum := pb.sum()

	// In contentSize mode the node's size styles are ignored.
	var nodeSize, nodeMin, nodeMax optSize
	var ratio opt
	if in.sizing == contentSize {
		nodeSize = known
	} else {
		ratio = st.aspectRatio()
		nodeSize = known.or(st.size().resolve(parent).applyAspectRatio(ratio))
		nodeMin = st.minSize().resolve(parent).applyAspectRatio(ratio)
		nodeMax = st.maxSize().resolve(parent)
	}

	// A node that scrolls vertically reserves horizontal space for its
	// scrollbar, and the other way round.
	gutter := st.scrollbarGutter()
	cbi := pb
	cbi.r += gutter.x
	cbi.b += gutter.y

	// Return early if both dimensions are known.
	if in.run == computeSize && nodeSize.bothSet() {
		s := size{nodeSize.w.v, nodeSize.h.v}.maybeClamp(nodeMin, nodeMax).maybeMax(pbSum.opts())
		return outerSize(s)
	}

	// The space available to the content box.
	aw := avail.w.maybeSet(known.w).subF(margin.hsum()).maybeSet(known.w).maybeSet(nodeSize.w)
	if aw.Kind == Definite {
		aw.Value = fclampOpt(aw.Value, nodeMin.w, nodeMax.w) - cbi.hsum()
	}
	ah := avail.h.maybeSet(known.h).subF(margin.vsum()).maybeSet(known.h).maybeSet(nodeSize.h)
	if ah.Kind == Definite {
		ah.Value = fclampOpt(ah.Value, nodeMin.h, nodeMax.h) - cbi.vsum()
	}

	// Known dimensions are the border box; Measure gets the content box.
	// (Taffy passes the border box, so text with padding wrapped at the
	// wrong width in size queries.) In a full layout the node's size is
	// already known; the content is laid out into it (Measure gets it as
	// available space).
	var measureKnown optSize
	if in.run != performLayout {
		measureKnown = known.maybeSubF(cbi.sum()).maybeMaxF(size{})
	}
	m := e.measure(n, measureKnown, availSize{aw, ah})

	clamped := known.or(nodeSize).unwrapOr(size{m.w + cbi.hsum(), m.h + cbi.vsum()}).maybeClamp(nodeMin, nodeMax)
	sz := size{clamped.w, clamped.h}
	if ratio.ok {
		sz.h = fmax(sz.h, clamped.w/ratio.v)
	}
	sz = sz.maybeMax(pbSum.opts())

	// The scrollable overflow of a leaf is its content; a scroll
	// container's end padding is part of it.
	startPad, endPad := padding.l, padding.r
	if st.Direction == RTL {
		startPad, endPad = padding.r, padding.l
	}
	of := rect{r: startPad + m.w, b: padding.t + m.h}
	if st.isScrollContainer() {
		of.r += endPad
		of.b += padding.b
	}

	var baseline opt
	if m.hasBaseline {
		baseline = some(border.t + padding.t + m.base)
	}
	return output{size: sz, overflow: of, firstBaseline: baseline}
}

// measured is the content size reported by a Measurer.
type measured struct {
	w, h        float32
	base        float32
	hasBaseline bool
}

// measure asks a leaf for its content size. Known dimensions are passed
// through; negative definite space is passed as 0.
func (e *Engine) measure(n Node, known optSize, avail availSize) measured {
	if known.bothSet() {
		return measured{w: known.w.v, h: known.h.v}
	}
	m, ok := n.(Measurer)
	if !ok {
		return measured{w: known.w.or0(), h: known.h.or0()}
	}
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
	r := m.Measure(in)
	out := measured{w: r.Width, h: r.Height, base: r.Baseline, hasBaseline: r.HasBaseline}
	if known.w.ok {
		out.w = known.w.v
	}
	if known.h.ok {
		out.h = known.h.v
	}
	return out
}
