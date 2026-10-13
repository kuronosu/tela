// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/block.rs); see THIRD_PARTY.md.
// Tela's block layout has no margin collapsing (ADR-006) and no floats, so
// Taffy's margin-collapsing sets, block formatting contexts and float
// contexts are left out: vertical margins simply add up.

package layout

// blockItem is a child of a block container during layout.
type blockItem struct {
	node     Node
	st       *Style
	replaced bool

	sizeStyle              dimSize
	size, minSize, maxSize optSize

	padding, border rect
	pbSum           size

	// Results of the final layout, applied after align-content.
	inFlow bool
	loc    point
	out    output
	margin rect
	sbSize point
	static staticPos // absolutely positioned items
}

// block computes the layout of a block container (Taffy's
// compute_block_layout).
func (e *Engine) block(n Node, st *Style, in input) output {
	parent := in.parent
	ratio := st.aspectRatio()
	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)
	pbSize := padding.add(border).sum()

	minSize := st.minSize().resolve(parent).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(parent).applyAspectRatio(ratio)
	var clamped optSize
	if in.sizing == inherentSize {
		clamped = st.size().resolve(parent).applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
	}
	minMax := optSize{minMaxDefinite(minSize.w, maxSize.w), minMaxDefinite(minSize.h, maxSize.h)}
	styled := in.known.or(minMax).or(clamped).maybeMaxF(pbSize)

	// The size is all the caller wants and the styles determine it.
	if in.run == computeSize {
		if styled.bothSet() {
			return outerSize(size{styled.w.v, styled.h.v})
		}
		if in.axis == axisHorizontal && styled.w.ok {
			return outerSize(size{styled.w.v, 0})
		}
	}
	in.known = styled
	return e.blockInner(n, st, in)
}

func (e *Engine) blockInner(n Node, st *Style, in input) output {
	known, parent, avail := in.known, in.parent, in.avail
	ratio := st.aspectRatio()
	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)
	dir := st.Direction
	scrolls := st.isScrollContainer()

	// A vertical scrollbar takes space on the inline-end side.
	g := st.scrollbarGutter()
	gutter := rect{r: g.x, b: g.y}
	if dir == RTL {
		gutter = rect{l: g.x, b: g.y}
	}
	pb := padding.add(border)
	pbSize := pb.sum()
	cbi := pb.add(gutter)

	sz := st.size().resolve(parent).applyAspectRatio(ratio)
	minSize := st.minSize().resolve(parent).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(parent).applyAspectRatio(ratio)

	// A definite size in one axis transfers through aspect-ratio to the
	// other (css-sizing-4); an incoming known size is left as is.
	derived := known.applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
	known = optSize{known.w.or(derived.w), known.h.or(derived.h)}
	pctBasis := known
	if !in.knownDefinite.h {
		pctBasis.h = none
	}
	contentBox := pctBasis.maybeSubF(cbi.sum())

	// 1. Generate items.
	items := e.blockItems(n, contentBox)

	// 2. Compute the container width.
	var outerW float32
	if known.w.ok {
		outerW = known.w.v
	} else {
		aw := avail.w.subF(cbi.hsum())
		intrinsic := e.blockContentWidth(n, items, aw) + cbi.hsum()
		outerW = fmax(fclampOpt(intrinsic, minSize.w, maxSize.w), pbSize.w)
	}

	if in.run == computeSize && known.h.ok {
		return outerSize(size{outerW, known.h.v})
	}
	if in.run == computeSize && in.axis == axisHorizontal {
		return outerSize(size{outerW, 0})
	}

	pctHeight := pctBasis.h.or(sz.h.maybeMax(minSize.h))

	// 3. Lay out the in-flow children. Percentage padding and borders
	// resolve against the containing block's width; at the root of the
	// tree, against the box's own width.
	pctWidth := parent.w.unwrapOr(outerW)
	rPadding := st.Padding.resolveOrZero(some(pctWidth))
	rBorder := st.Border.resolveOrZero(some(pctWidth))
	rCbi := rPadding.add(rBorder).add(gutter)
	inflowOverflow, contentH, firstBaseline, lastBaseline, escapes := e.blockInFlow(n, in.run, items, outerW, pctHeight, rCbi, rBorder, dir, scrolls)

	outerH := fmax(known.h.unwrapOr(fclampOpt(contentH, minSize.h, maxSize.h)), pbSize.h)
	final := size{outerW, outerH}

	// align-content moves the in-flow children as one alignment subject
	// (css-align-3, block containers).
	if st.AlignContent != AlignNormal {
		innerH := outerH - rCbi.vsum()
		free := innerH - (contentH - rCbi.vsum())
		anyInFlow := false
		for i := range items {
			anyInFlow = anyInFlow || items[i].inFlow
		}
		if anyInFlow {
			k := alignmentFallback(free, 1, st.AlignContent)
			off := alignmentOffset(free, 1, 0, k, false, true)
			firstBaseline, lastBaseline = firstBaseline.addF(off), lastBaseline.addF(off)
			inflowOverflow = rect{}
			for i := range items {
				it := &items[i]
				if !it.inFlow {
					continue
				}
				it.loc.y += off
				inflowOverflow = inflowOverflow.union(overflowContribution(
					blockOverflowLoc(it.loc, it.out.size, outerW, rBorder, dir), it.out.size, it.out.overflow, it.st, scrolls))
			}
		}
	}

	if in.run == computeSize {
		return output{size: final, firstBaseline: firstBaseline, lastBaseline: lastBaseline}
	}

	// Commit the children's layouts.
	for i := range items {
		it := &items[i]
		if it.inFlow {
			it.node.LayoutData().setLayout(it.loc, it.out.size, it.border, it.padding, it.margin, it.sbSize, it.out.overflow, it.out.firstBaseline)
		}
	}

	// 4. Absolutely positioned children: laid out here if this box is
	// their containing block, else recorded for the nearest positioned
	// ancestor.
	absInset := rBorder.add(gutter)
	area := final.sub(absInset.sum())
	areaOffset := point{absInset.l, absInset.t}
	cb := st.Position != PositionStatic || n.LayoutData().parent == nil
	var absOverflow rect
	for i := range items {
		it := &items[i]
		if it.inFlow || it.st.Position != PositionAbsolute {
			continue
		}
		it.node.LayoutData().static = it.static
		if cb {
			absOverflow = absOverflow.union(e.layoutAbsolute(n, it.node, it.st, area, areaOffset, it.static, dir, scrolls, point{}))
		} else {
			escapes = true
		}
	}
	if cb && escapes {
		absOverflow = absOverflow.union(e.layoutEscaped(n, area, areaOffset, dir, scrolls))
		escapes = false
	}

	// A scroll container's end padding is part of its scrollable
	// overflow.
	if scrolls {
		if dir == RTL {
			inflowOverflow.r += rPadding.l
		} else {
			inflowOverflow.r += rPadding.r
		}
		inflowOverflow.b += rPadding.b
	}

	// 5. Hidden children.
	e.hideChildren(n)

	return output{size: final, overflow: inflowOverflow.union(absOverflow), firstBaseline: firstBaseline, lastBaseline: lastBaseline, escapes: escapes}
}

// blockItems collects the children of a block container that generate
// boxes. Percentage padding and borders resolve against the container's
// inner width.
func (e *Engine) blockItems(n Node, inner optSize) []blockItem {
	count := n.LayoutChildCount()
	items := make([]blockItem, 0, count) // perf: reuse a per-depth buffer
	for i := 0; i < count; i++ {
		c := n.LayoutChild(i)
		cst := c.LayoutStyle()
		c.LayoutData().parent = n
		if cst.Display == DisplayNone {
			continue
		}
		ratio := cst.aspectRatio()
		padding := cst.Padding.resolveOrZero(inner.w)
		border := cst.Border.resolveOrZero(inner.w)
		items = append(items, blockItem{
			node:      c,
			st:        cst,
			replaced:  cst.Replaced,
			sizeStyle: cst.size(),
			size:      cst.size().resolve(inner).applyAspectRatio(ratio),
			minSize:   cst.minSize().resolve(inner).applyAspectRatio(ratio),
			maxSize:   cst.maxSize().resolve(inner).applyAspectRatio(ratio),
			padding:   padding,
			border:    border,
			pbSum:     padding.add(border).sum(),
		})
	}
	return items
}

// blockContentWidth computes the content-based width of a block container
// whose width is not known: the widest in-flow child's margin box.
func (e *Engine) blockContentWidth(n Node, items []blockItem, availW AvailableSpace) float32 {
	var maxW float32
	for i := range items {
		it := &items[i]
		if it.st.Position == PositionAbsolute {
			continue
		}
		known := it.size.maybeClamp(it.minSize, it.maxSize)
		// Percentage margins count as zero in intrinsic contributions
		// (css-sizing-3 5.2.1; Taffy resolves them against the available
		// width).
		xMargin := it.st.Margin.resolveOrZero(none).hsum()
		var w float32
		if known.w.ok {
			w = known.w.v
		} else {
			var iw AvailableSpace
			switch k, space, exact := resolveSizingKeyword(it.sizeStyle.w, none, none); k {
			case keywordMeasure:
				iw = space
			case keywordExact:
				iw = DefiniteSpace(exact)
			default:
				iw = availW.subF(xMargin)
			}
			w = e.measureChildSize(n, it.node, known, optSize{}, availSize{iw, MinContentConstraint}, inherentSize, horizontal)
		}
		maxW = fmax(maxW, fmax(w, it.pbSum.w)+xMargin)
	}
	return maxW
}

// blockOverflowLoc returns a child's position from the container's scroll
// origin, for its overflow contribution.
func blockOverflowLoc(loc point, sz size, outerW float32, border rect, dir Direction) point {
	if dir == RTL {
		return point{outerW - (loc.x + sz.w) - border.r, loc.y - border.t}
	}
	return point{loc.x - border.l, loc.y - border.t}
}

// blockInFlow lays out the in-flow children of a block container top to
// bottom and records the static positions of the absolutely positioned
// ones. It returns the in-flow scrollable overflow, the content height
// (border box), the first baseline and whether any child hoists
// absolutely positioned boxes.
func (e *Engine) blockInFlow(n Node, run runMode, items []blockItem, outerW float32, pctHeight opt, rCbi, rBorder rect, dir Direction, scrolls bool) (rect, float32, opt, opt, bool) {
	innerW := outerW - rCbi.hsum()
	pctH := pctHeight.subF(rCbi.vsum())
	parentSize := optSize{some(innerW), pctH}

	var overflow rect
	var firstBaseline, lastBaseline opt
	var escapes bool
	y := rCbi.t
	for i := range items {
		it := &items[i]
		if it.st.Position == PositionAbsolute {
			// The static position: where the box would start if it were
			// in flow.
			sp := staticPos{y0: y, y1: y, modeY: alignStartMode}
			if dir == RTL {
				sp.x0, sp.x1, sp.modeX = outerW-rCbi.r, outerW-rCbi.r, alignEndMode
			} else {
				sp.x0, sp.x1, sp.modeX = rCbi.l, rCbi.l, alignStartMode
			}
			it.static = sp
			continue
		}

		margin := it.st.Margin.resolveMargins(some(innerW))
		nonAuto := margin.or0()
		stretchW := innerW - nonAuto.hsum()

		var sb point
		if it.st.OverflowY == OverflowScroll {
			sb.x = it.st.ScrollbarWidth
		}
		if it.st.OverflowX == OverflowScroll {
			sb.y = it.st.ScrollbarWidth
		}

		// Replaced elements are not stretched: they keep their measured
		// size.
		var known optSize
		if !it.replaced {
			// Sizing keywords resolve directly or by measuring the child.
			var keywordW opt
			switch k, space, exact := resolveSizingKeyword(it.sizeStyle.w, some(stretchW), some(innerW)); k {
			case keywordExact:
				keywordW = some(exact)
			case keywordMeasure:
				keywordW = some(e.measureChildSize(n, it.node, optSize{}, parentSize,
					availSize{space, MaxContentConstraint}, inherentSize, horizontal))
			}
			var keywordH opt
			if k, _, exact := resolveSizingKeyword(it.sizeStyle.h, pctH.subF(nonAuto.vsum()), pctH); k == keywordExact {
				keywordH = some(exact)
			}
			known = optSize{
				w: some(fclampOpt(it.size.w.or(keywordW).unwrapOr(stretchW), it.minSize.w, it.maxSize.w)),
				h: it.size.h.or(keywordH),
			}.maybeClamp(it.minSize, it.maxSize)
		}

		out := e.child(n, it.node, input{
			run: run, sizing: inherentSize, axis: axisBoth,
			known: known, knownDefinite: boolSize{true, true},
			parent: parentSize,
			avail:  availSize{DefiniteSpace(stretchW), MaxContentConstraint},
		})
		final := out.size
		escapes = escapes || (out.escapes && it.st.Position == PositionStatic)

		// Auto margins share the free horizontal space; vertical auto
		// margins are 0.
		freeX := fmax(0, stretchW-final.w)
		var autoCount float32
		if !margin.l.ok {
			autoCount++
		}
		if !margin.r.ok {
			autoCount++
		}
		var autoSize float32
		if autoCount > 0 {
			autoSize = freeX / autoCount
		}
		resolved := rect{l: margin.l.unwrapOr(autoSize), r: margin.r.unwrapOr(autoSize), t: nonAuto.t, b: nonAuto.b}

		off := relativeOffset(it.st, optSize{some(innerW), pctH}, dir)

		loc := point{y: y + resolved.t + off.y}
		if dir == RTL {
			loc.x = outerW - rCbi.r - final.w - resolved.r + off.x
		} else {
			loc.x = rCbi.l + off.x + resolved.l
		}

		// The first baseline is the first in-flow child's that has one. A
		// scroll container's baseline is clamped to its border box, and
		// synthesized from its bottom edge if it has none.
		if !firstBaseline.ok {
			b := out.firstBaseline
			if it.st.OverflowY.isScrollContainer() {
				b = some(fmax(fmin(b.unwrapOr(final.h), final.h), 0))
			}
			firstBaseline = b.addF(loc.y)
		}
		// The last baseline is the last in-flow child's that has one
		// (CSS Align 3, section 9.1; a scroll container has none for an
		// inline-block's baseline, which the caller decides).
		if b := out.lastBaseline.or(out.firstBaseline); b.ok {
			lastBaseline = b.addF(loc.y)
		}

		it.inFlow, it.loc, it.out, it.margin, it.sbSize = true, loc, out, resolved, sb
		overflow = overflow.union(overflowContribution(blockOverflowLoc(loc, final, outerW, rBorder, dir), final, out.overflow, it.st, scrolls))

		y = loc.y - off.y + final.h + resolved.b
	}
	y += rCbi.b
	return overflow, fmax(0, y), firstBaseline, lastBaseline, escapes
}

// relativeOffset returns the offset of a relatively positioned box from
// its insets (left wins over right in LTR, right in RTL; top over bottom).
// Static boxes ignore their insets.
func relativeOffset(st *Style, basis optSize, dir Direction) point {
	if st.Position != PositionRelative {
		return point{}
	}
	in := st.Inset.resolveInsets(basis)
	var off point
	if dir == RTL {
		if in.r.ok {
			off.x = -in.r.v
		} else if in.l.ok {
			off.x = in.l.v
		}
	} else {
		if in.l.ok {
			off.x = in.l.v
		} else if in.r.ok {
			off.x = -in.r.v
		}
	}
	if in.t.ok {
		off.y = in.t.v
	} else if in.b.ok {
		off.y = -in.b.v
	}
	return off
}
