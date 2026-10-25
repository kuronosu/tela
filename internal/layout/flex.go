// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/flexbox.rs); see THIRD_PARTY.md.
// Not ported: flex-wrap: balance and flex-line-count (CSS Flexbox 2
// draft) and box-sizing: content-box. Added: the order property.

package layout

import (
	"math"
	"sort"
)

// flexItem holds the intermediate results of the flexbox algorithm for one
// item.
type flexItem struct {
	node Node
	st   *Style

	size      optSize // the base size
	sizeStyle dimSize
	// crossSize is the cross size style resolved. Unlike size's cross
	// size, it is never transferred from the main size through the aspect
	// ratio: that one follows the preferred main size, not the used one,
	// and is not a definite cross size of the item (ADR-157).
	crossSize opt
	minSize   optSize
	maxSize   optSize
	ratio     opt
	alignSelf Align

	flexShrink, flexGrow float32
	// basisDefinite reports whether the used flex basis is definite rather
	// than derived from the item's content.
	basisDefinite bool

	// resolvedMinMain is the minimum main size, including the automatic
	// minimum size from the content.
	resolvedMinMain float32

	inset      optRect
	margin     rect
	marginAuto boolRect
	padding    rect
	border     rect

	flexBasis      float32
	innerFlexBasis float32
	violation      float32
	frozen         bool

	// contentFlexFraction is the max- or min-content flex fraction
	// (https://www.w3.org/TR/css-flexbox-1/#intrinsic-main-sizes).
	contentFlexFraction float32

	hypoInner   size
	hypoOuter   size
	target      size
	outerTarget size

	baseline float32

	// offsetMain and offsetCross are the positions from alignment and
	// justification, margins, padding and borders not included.
	offsetMain, offsetCross float32
}

func (it *flexItem) isScrollContainer() bool { return it.st.isScrollContainer() }

// participatesInBaseline reports whether the item takes part in baseline
// alignment: align-self baseline and no auto cross margin.
func (it *flexItem) participatesInBaseline(dir FlexDirection) bool {
	return it.alignSelf == AlignBaseline && !it.marginAuto.crossStart(dir) && !it.marginAuto.crossEnd(dir)
}

// flexLine is a line of items.
type flexLine struct {
	items       []flexItem
	crossSize   float32
	offsetCross float32
}

// flexConsts are values used throughout the algorithm.
type flexConsts struct {
	dir                           FlexDirection
	layoutDir                     Direction
	isRow, isColumn               bool
	isWrap, isWrapReverse         bool
	minSize, maxSize              optSize
	margin, border                rect
	cbi                           rect  // content box inset: padding + border + scrollbar gutter
	gutter                        point // scrollbar gutter per axis
	scrolls                       bool
	gap                           size
	alignItems, alignContent      Align
	justifyContent                Align
	nodeOuter, nodeInner          optSize
	knownMainDefinite             bool
	hasDefiniteMain               bool
	hasDefiniteCross              bool
	crossAvailDefinite            bool
	containerSize, innerContainer size
}

// flex computes the layout of a flex container (Taffy's
// compute_flexbox_layout).
func (e *Engine) flex(n Node, st *Style, in input) output {
	known, parent := in.known, in.parent
	ratio := st.aspectRatio()
	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)
	pbSum := padding.sum().add(border.sum())

	minSize := st.minSize().resolve(parent).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(parent).applyAspectRatio(ratio)
	var clamped optSize
	if in.sizing == inherentSize {
		clamped = st.size().resolve(parent).applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
	}
	minMax := optSize{minMaxDefinite(minSize.w, maxSize.w), minMaxDefinite(minSize.h, maxSize.h)}
	styled := known.or(minMax.or(clamped).maybeMaxF(pbSum))

	if in.run == computeSize {
		if styled.bothSet() {
			return outerSize(size{styled.w.v, styled.h.v})
		}
		if in.axis == axisHorizontal && styled.w.ok {
			return outerSize(size{styled.w.v, 0})
		}
	}

	// Definiteness flags only apply to dimensions the parent passed in;
	// dimensions from the node's own style are definite.
	in.knownDefinite = boolSize{in.knownDefinite.w || !known.w.ok, in.knownDefinite.h || !known.h.ok}
	in.known = styled
	return e.flexPreliminary(n, st, in)
}

func (e *Engine) flexPreliminary(n Node, st *Style, in input) output {
	known := in.known
	c := flexConstants(st, known, in.knownDefinite, in.parent, in.avail)

	// 9.1. Initial setup: generate the flex items.
	items := e.flexItems(n, &c)

	// 9.2. Line length determination.
	avail := flexAvailableSpace(known, in.avail, &c)
	e.flexBaseSizes(n, &c, avail, items)

	// 9.3. Main size determination: collect items into lines.
	lines := collectFlexLines(&c, avail, items)

	if inner := c.nodeInner.main(c.dir); inner.ok {
		c.innerContainer.setMain(c.dir, inner.v)
		c.containerSize.setMain(c.dir, inner.v+c.cbi.mainSum(c.dir))
	} else {
		e.flexContainerMainSize(n, avail, lines, &c)
		c.nodeInner.setMain(c.dir, some(c.innerContainer.main(c.dir)))
		c.nodeOuter.setMain(c.dir, some(c.containerSize.main(c.dir)))
		// Percentage gaps resolve against the new main size.
		c.gap.setMain(c.dir, st.gap().main(c.dir).resolveSpacing(some(c.innerContainer.main(c.dir))).or0())
	}

	// 9.7. Resolve the flexible lengths.
	for i := range lines {
		resolveFlexibleLengths(&lines[i], &c)
	}

	// 9.4. Cross size determination.
	for i := range lines {
		e.hypotheticalCrossSizes(n, &lines[i], &c, avail)
	}
	e.flexBaselines(n, known, avail, lines, &c)
	flexLineCrossSizes(lines, known, &c)
	alignContentStretch(lines, known, &c)
	usedCrossSizes(lines, &c)

	// 9.5. Main-axis alignment.
	distributeFreeSpace(lines, &c)

	// 9.6. Cross-axis alignment.
	crossAutoMargins(lines, &c)
	totalLineCross := flexContainerCrossSize(lines, known, &c)

	if in.run == computeSize {
		return outerSize(c.containerSize)
	}

	alignFlexLines(lines, &c, totalLineCross)

	inflowOverflow, escapes := e.flexFinalPass(n, lines, &c)
	absOverflow, absEscapes := e.flexAbsolute(n, st, &c)
	escapes = escapes || absEscapes
	if escapes && st.absContainer(n.LayoutData().parent == nil) {
		area, areaOffset := c.absoluteArea()
		absOverflow = absOverflow.union(e.layoutEscaped(n, area, areaOffset, c.layoutDir, c.scrolls))
		escapes = false
	}

	e.hideChildren(n)

	// 8.5. The container's first baseline comes from its startmost line:
	// the last one in a wrap-reverse container.
	var baseline opt
	if len(lines) > 0 {
		line := &lines[0]
		if c.isWrapReverse {
			line = &lines[len(lines)-1]
		}
		if len(line.items) > 0 {
			if c.isColumn {
				it := &line.items[0]
				if c.dir.isReverse() {
					it = &line.items[len(line.items)-1]
				}
				baseline = some(it.baseline)
			} else {
				it := &line.items[0]
				for i := range line.items {
					if line.items[i].participatesInBaseline(c.dir) {
						it = &line.items[i]
						break
					}
				}
				baseline = some(it.baseline)
			}
		}
	}

	return output{size: c.containerSize, overflow: inflowOverflow.union(absOverflow), firstBaseline: baseline, lastBaseline: baseline, escapes: escapes}
}

// flexConstants computes the values used throughout the algorithm.
func flexConstants(st *Style, known optSize, knownDefinite boolSize, parent optSize, avail availSize) flexConsts {
	dir := st.FlexDirection
	ratio := st.aspectRatio()
	margin := st.Margin.resolveOrZero(parent.w)
	padding := st.Padding.resolveOrZero(parent.w)
	border := st.Border.resolveOrZero(parent.w)

	alignItems := st.AlignItems
	if alignItems == AlignNormal {
		alignItems = AlignStretch
	}
	alignContent := st.AlignContent
	if alignContent == AlignNormal {
		alignContent = AlignStretch
	}

	// A node that scrolls vertically reserves horizontal space for its
	// scrollbar, on the inline-end side.
	gutter := st.scrollbarGutter()
	cbi := padding.add(border)
	cbi.b += gutter.y
	if st.Direction == RTL {
		cbi.l += gutter.x
	} else {
		cbi.r += gutter.x
	}

	inner := known.maybeSubF(cbi.sum())
	knownMainDefinite := knownDefinite.w
	knownCrossDefinite := knownDefinite.h
	if !dir.isRow() {
		knownMainDefinite, knownCrossDefinite = knownCrossDefinite, knownMainDefinite
	}
	hasDefiniteCross := knownCrossDefinite && known.cross(dir).ok
	gapBasis := optSize{some(inner.w.or0()), some(inner.h.or0())}

	return flexConsts{
		dir:                dir,
		layoutDir:          st.Direction,
		isRow:              dir.isRow(),
		isColumn:           !dir.isRow(),
		isWrap:             st.FlexWrap != NoWrap,
		isWrapReverse:      st.FlexWrap == WrapReverse,
		minSize:            st.minSize().resolve(parent).applyAspectRatio(ratio),
		maxSize:            st.maxSize().resolve(parent).applyAspectRatio(ratio),
		margin:             margin,
		border:             border,
		cbi:                cbi,
		gutter:             gutter,
		scrolls:            st.isScrollContainer(),
		gap:                size{st.ColumnGap.resolveOrZero(gapBasis.w), st.RowGap.resolveOrZero(gapBasis.h)},
		alignItems:         alignItems,
		alignContent:       alignContent,
		justifyContent:     st.JustifyContent,
		nodeOuter:          known,
		nodeInner:          inner,
		knownMainDefinite:  knownMainDefinite,
		hasDefiniteMain:    knownMainDefinite && known.main(dir).ok,
		hasDefiniteCross:   hasDefiniteCross,
		crossAvailDefinite: hasDefiniteCross || avail.cross(dir).isDefinite(),
	}
}

// resolveSelfAlign resolves self-start and self-end against the item's own
// direction (they flip in the inline axis when the item's direction
// differs from the container's).
func resolveSelfAlign(a Align, itemDir, containerDir Direction, axisIsInline bool) Align {
	flip := axisIsInline && itemDir != containerDir
	safe := a & AlignSafe
	switch a.keyword() {
	case AlignSelfStart:
		if flip {
			return AlignEnd | safe
		}
		return AlignStart | safe
	case AlignSelfEnd:
		if flip {
			return AlignStart | safe
		}
		return AlignEnd | safe
	}
	return a
}

// flexItems generates the flex items: the children that are in flow and
// generate boxes, sorted by order.
func (e *Engine) flexItems(n Node, c *flexConsts) []flexItem {
	// Percentage sizes resolve against the inner size if it is definite.
	pctRes := c.nodeInner
	if !c.knownMainDefinite {
		pctRes.setMain(c.dir, none)
	}
	count := n.LayoutChildCount()
	items := make([]flexItem, 0, count) // perf: reuse a per-depth buffer
	sorted := true
	for i := 0; i < count; i++ {
		ch := n.LayoutChild(i)
		st := ch.LayoutStyle()
		ch.LayoutData().parent = n
		if st.Position == PositionAbsolute || st.Display == DisplayNone {
			continue
		}
		ratio := st.aspectRatio()
		alignSelf := st.AlignSelf
		if alignSelf == AlignNormal {
			alignSelf = c.alignItems
		}
		items = append(items, flexItem{
			node:       ch,
			st:         st,
			size:       st.size().resolve(pctRes).applyAspectRatio(ratio),
			sizeStyle:  st.size(),
			crossSize:  st.size().cross(c.dir).resolveSize(pctRes.cross(c.dir)),
			minSize:    st.minSize().resolve(pctRes),
			maxSize:    st.maxSize().resolve(pctRes),
			ratio:      ratio,
			inset:      st.Inset.resolveInsets(c.nodeInner),
			margin:     st.Margin.resolveOrZero(c.nodeInner.w),
			marginAuto: st.Margin.autoMargins(),
			padding:    st.Padding.resolveOrZero(c.nodeInner.w),
			border:     st.Border.resolveOrZero(c.nodeInner.w),
			alignSelf:  resolveSelfAlign(alignSelf, st.Direction, c.layoutDir, c.isColumn),
			flexGrow:   st.FlexGrow,
			flexShrink: st.FlexShrink,
		})
		if len(items) > 1 && items[len(items)-2].st.Order > st.Order {
			sorted = false
		}
	}
	if !sorted {
		sort.SliceStable(items, func(i, j int) bool { return items[i].st.Order < items[j].st.Order })
	}
	return items
}

// flexAvailableSpace determines the space available to the items: the
// content box if known, else the container's available space minus its
// margin, border and padding (which may be infinite).
func flexAvailableSpace(known optSize, outer availSize, c *flexConsts) availSize {
	var a availSize
	if known.w.ok {
		a.w = DefiniteSpace(known.w.v - c.cbi.hsum())
	} else {
		a.w = outer.w.subF(c.margin.hsum()).subF(c.cbi.hsum())
	}
	if known.h.ok {
		a.h = DefiniteSpace(known.h.v - c.cbi.vsum())
	} else {
		a.h = outer.h.subF(c.margin.vsum()).subF(c.cbi.vsum())
	}
	return a
}

// automaticMinSize returns the automatic minimum size from overflow: 0 for
// scroll containers.
func automaticMinSize(o Overflow) opt {
	if o.isScrollContainer() {
		return some(0)
	}
	return none
}

// measureCross returns the known cross size and the available cross space
// an item's main size is measured with (its flex base size, 9.2.3 E, its
// min-content size for the automatic minimum, 4.5, and its content
// contribution, 9.9.1), and whether that cross size is definite. avail is
// the available cross space, already clamped by the item's min and max
// cross sizes; tMin and tMax clamp a known cross size (min and max sizes
// transferred through the aspect ratio included).
//
//   - A cross size style that resolves is the known cross size. A cross
//     size the aspect ratio transfers from the item's own main size is
//     not: it follows the preferred main size, which is not the item's
//     size yet, and is not definite (css-sizing-4), so a row of w-full
//     aspect-square items shrinks to fit as in Chromium (Taffy, like Tela
//     before ADR-157, kept such items at their preferred size).
//   - A min-content, max-content or fit-content cross size gives the space
//     to measure under: the item is measured at its own cross size, never
//     stretched (a w-fit item in a column is measured at its fit-content
//     width, not at the column's).
//   - Otherwise a stretched item (align-self: stretch, no auto cross
//     margin) is measured at the available space minus its margins, as in
//     Taffy, definite when the container's cross size is
//     (https://www.w3.org/TR/css-flexbox-1/#definite-sizes). An item with
//     an aspect ratio is measured that way only when that size is
//     definite: an indefinite one would transfer into a main size that
//     Chromium does not give it (it measures the item fit-content into
//     the space).
func measureCross(c *flexConsts, it *flexItem, avail AvailableSpace, tMin, tMax opt) (known opt, space AvailableSpace, definite bool) {
	dir := c.dir
	if it.crossSize.ok {
		return it.crossSize.maybeClamp(tMin, tMax), avail, true
	}
	crossParent := c.nodeInner.cross(dir)
	margins := it.margin.crossSum(dir)
	cs := it.sizeStyle.cross(dir)
	if cs.isSizingKeyword() && !cs.isStretch() {
		if k, s, _ := resolveSizingKeyword(cs, crossParent.subF(margins), crossParent); k == keywordMeasure {
			return none, s, false
		}
		return none, avail, false
	}
	if it.alignSelf == AlignStretch && !it.marginAuto.crossStart(dir) && !it.marginAuto.crossEnd(dir) {
		definite := !c.isWrap && c.hasDefiniteCross && crossParent.ok
		if definite || !it.ratio.ok {
			return avail.opt().subF(margins), avail, definite
		}
	}
	return none, avail, false
}

// flexBaseSizes determines the flex base size and the hypothetical main
// size of each item (9.2.3).
func (e *Engine) flexBaseSizes(n Node, c *flexConsts, avail availSize, items []flexItem) {
	dir := c.dir
	for i := range items {
		it := &items[i]

		crossParent := c.nodeInner.cross(dir)
		childParent := sizeFromCross(dir, crossParent)

		// Min/max sizes transferred through the aspect ratio limit the
		// available space (csswg-drafts#10997).
		crossMarginSum := c.margin.crossSum(dir)
		transferredMin := it.minSize.applyAspectRatio(it.ratio)
		transferredMax := it.maxSize.applyAspectRatio(it.ratio)
		childMinCross := transferredMin.cross(dir).addF(crossMarginSum)
		childMaxCross := transferredMax.cross(dir).addF(crossMarginSum)

		var crossAvail AvailableSpace
		switch a := avail.cross(dir); a.Kind {
		case Definite:
			crossAvail = DefiniteSpace(fclampOpt(crossParent.unwrapOr(a.Value), childMinCross, childMaxCross))
		case MinContentSpace:
			crossAvail = MinContentConstraint
			if childMinCross.ok {
				crossAvail = DefiniteSpace(childMinCross.v)
			}
		default:
			crossAvail = MaxContentConstraint
			if childMaxCross.ok {
				crossAvail = DefiniteSpace(childMaxCross.v)
			}
		}

		// Known dimensions for sizing the item: the cross size, clamped,
		// or the stretched cross size (measureCross).
		var ckd optSize
		var crossKnown opt
		var crossDefinite bool
		crossKnown, crossAvail, crossDefinite = measureCross(c, it, crossAvail, transferredMin.cross(dir), transferredMax.cross(dir))
		ckd.setCross(dir, crossKnown)

		// Percentage flex bases resolve against a definite inner main size.
		var pctMain opt
		if c.knownMainDefinite {
			pctMain = c.nodeInner.main(dir)
		}
		basisStyle := it.st.FlexBasis
		flexBasis := basisStyle.resolveSize(pctMain)

		it.flexBasis = func() float32 {
			// A. A definite flex basis is the flex base size. B. A main size
			// from the cross size through the aspect ratio is too (it.size
			// already has the ratio applied).
			mainSize := it.size.main(dir)
			mainStretch := pctMain.subF(it.margin.mainSum(dir))

			// Sizing keywords in flex-basis replace the main size: stretch
			// is exact, the others give the space to measure under.
			var keywordSpace AvailableSpace
			hasKeywordSpace := false
			switch {
			case basisStyle.Unit == UnitContent:
			case basisStyle.isSizingKeyword():
				switch k, space, exact := resolveSizingKeyword(basisStyle, mainStretch, pctMain); k {
				case keywordExact:
					it.basisDefinite = true
					return exact
				case keywordMeasure:
					keywordSpace, hasKeywordSpace = space, true
				}
			default:
				if b := flexBasis.or(mainSize); b.ok {
					it.basisDefinite = true
					return b.v
				}
				switch k, space, exact := resolveSizingKeyword(it.sizeStyle.main(dir), mainStretch, pctMain); k {
				case keywordExact:
					it.basisDefinite = true
					return exact
				case keywordMeasure:
					keywordSpace, hasKeywordSpace = space, true
				}
			}

			// An aspect ratio and a definite cross size give the base size.
			if crossDefinite && it.ratio.ok {
				if cross := ckd.cross(dir); cross.ok {
					it.basisDefinite = true
					if dir.isRow() {
						return float32(cross.v * it.ratio.v)
					}
					return cross.v / it.ratio.v
				}
			}

			// C/E. Measure the item under the container's constraint, with
			// content as max-content.
			mainSpace := MaxContentConstraint
			if avail.main(dir).Kind == MinContentSpace {
				mainSpace = MinContentConstraint
			}
			if hasKeywordSpace {
				mainSpace = keywordSpace
			}
			childAvail := availSize{MaxContentConstraint, MaxContentConstraint}.withMain(dir, mainSpace).withCross(dir, crossAvail)
			return e.measureChildSize(n, it.node, ckd, childParent, childAvail, contentSize, dir.mainAxis())
		}()

		// The flex basis is floored by padding and border (the inner basis
		// at zero). The spec says not to, but Chrome and Firefox do.
		pbMain := it.padding.mainSum(dir) + it.border.mainSum(dir)
		it.flexBasis = fmax(it.flexBasis, pbMain)
		it.innerFlexBasis = it.flexBasis - pbMain

		pbAxes := it.padding.add(it.border).sum()

		// The minimum main size: the min-size style, else 0 for scroll
		// containers, else the automatic minimum size (4.5): the
		// min-content size, capped by the main size and the transferred
		// max size. Percentages in the main axis do not contribute
		// (https://drafts.csswg.org/css-sizing-3/#min-percentage-contribution),
		// so the parent size there is unset. A percentage min size of an
		// indefinite basis counts as 0 (its length part for a calc()), not
		// as auto (CSS 2.1 section 10.7; Chromium, ADR-157).
		mainMin := it.minSize.main(dir)
		if !mainMin.ok {
			mainMin = it.st.minSize().main(dir).resolveSize(some(0))
		}
		mainOverflow := it.st.OverflowX
		if !c.isRow {
			mainOverflow = it.st.OverflowY
		}
		styleMin := mainMin.or(automaticMinSize(mainOverflow))
		if styleMin.ok {
			it.resolvedMinMain = styleMin.v
		} else {
			childAvail := availSize{MinContentConstraint, MinContentConstraint}.withCross(dir, crossAvail)
			minContent := e.measureChildSize(n, it.node, ckd, childParent, childAvail, contentSize, dir.mainAxis())
			// With an aspect ratio and a definite cross size, the content
			// size suggestion is the size transferred through the ratio
			// (css-sizing-4; Taffy leaves this out: the fixture
			// xaspect_ratio_flex_row_stretch_fill_min_width). A cross size
			// transferred from the item's own main size is not definite
			// here (measureCross), so a row of w-full aspect-square items
			// shrinks like Chromium's.
			if cross := ckd.cross(dir); crossDefinite && it.ratio.ok && cross.ok {
				if dir.isRow() {
					minContent = fmax(minContent, float32(cross.v*it.ratio.v))
				} else {
					minContent = fmax(minContent, cross.v/it.ratio.v)
				}
			}
			// With an aspect ratio, the content size suggestion is clamped
			// by the min and max cross sizes converted through it (4.5).
			if it.ratio.ok {
				minContent = fclampOpt(minContent, transferredMin.main(dir), transferredMax.main(dir))
			}
			clamped := fminOpt(fminOpt(minContent, it.size.main(dir)), transferredMax.main(dir))
			it.resolvedMinMain = fmax(clamped, pbAxes.main(dir))
		}

		// Transferred sizes clamp the hypothetical main size only.
		hypoMin := fmax(fmaxOpt(it.resolvedMinMain, transferredMin.main(dir)), pbAxes.main(dir))
		hypoInner := fclampOpt(it.flexBasis, some(hypoMin), transferredMax.main(dir))
		it.hypoInner.setMain(dir, hypoInner)
		it.hypoOuter.setMain(dir, hypoInner+it.margin.mainSum(dir))
	}
}

// collectFlexLines collects the items into lines (9.3.5). Wrapping needs a
// definite main size: a container sized by its content has one line.
func collectFlexLines(c *flexConsts, avail availSize, items []flexItem) []flexLine {
	if !c.isWrap || !c.knownMainDefinite {
		return []flexLine{{items: items}}
	}
	mainAvail := avail.main(c.dir)
	if max := c.maxSize.main(c.dir); max.ok {
		// Without a definite main size the container is at most its max
		// size, which is then the limit items wrap against.
		a := avail.main(c.dir).opt().unwrapOr(max.v)
		if !c.hasDefiniteMain {
			a = fmin(a, max.v)
		}
		mainAvail = DefiniteSpace(fmaxOpt(a, c.minSize.main(c.dir)))
	}
	switch mainAvail.Kind {
	case MaxContentSpace:
		// Under a max-content constraint items never wrap.
		return []flexLine{{items: items}}
	case MinContentSpace:
		// Under a min-content constraint every item has its own line.
		lines := make([]flexLine, len(items))
		for i := range items {
			lines[i] = flexLine{items: items[i : i+1]}
		}
		return lines
	}
	space := mainAvail.Value
	gap := c.gap.main(c.dir)
	var lines []flexLine
	for len(items) > 0 {
		// The first item that does not fit starts the next line; a line
		// always takes at least one item.
		var length float32
		idx := len(items)
		for j := range items {
			if j != 0 {
				length += items[j].hypoOuter.main(c.dir) + gap
			} else {
				length += items[j].hypoOuter.main(c.dir)
			}
			if length > space && j != 0 {
				idx = j
				break
			}
		}
		lines = append(lines, flexLine{items: items[:idx]})
		items = items[idx:]
	}
	return lines
}

// itemDefiniteness says whether an item's known dimensions are definite
// for its own content: its post-flexing main size only when the container
// has a definite main size or the item's flex basis is definite; its cross
// size when it is stretched, has a definite cross size style, or (for
// columns) is fit-content sized into definite space
// (https://www.w3.org/TR/css-flexbox-1/#definite-sizes).
func itemDefiniteness(c *flexConsts, it *flexItem) boolSize {
	dir := c.dir
	mainDefinite := c.hasDefiniteMain || it.basisDefinite
	crossAuto := it.marginAuto.crossStart(dir) || it.marginAuto.crossEnd(dir)
	cs := it.sizeStyle.cross(dir)
	stretched := !crossAuto && (cs.isStretch() || (it.alignSelf == AlignStretch && cs.isAuto()))
	crossDefinite := stretched || it.size.cross(dir).ok || (!dir.isRow() && c.crossAvailDefinite)
	if dir.isRow() {
		return boolSize{mainDefinite, crossDefinite}
	}
	return boolSize{crossDefinite, mainDefinite}
}

// flexContainerMainSize determines the container's main size when it is
// not known (9.3.4), from the items' contributions.
func (e *Engine) flexContainerMainSize(n Node, avail availSize, lines []flexLine, c *flexConsts) {
	dir := c.dir
	mainCbi := c.cbi.mainSum(dir)

	itemMainLength := func(it *flexItem) float32 {
		pb := it.padding.mainSum(dir) + it.border.mainSum(dir)
		return fmax(fmaxOpt(it.flexBasis, it.minSize.main(dir))+it.margin.mainSum(dir), pb)
	}
	longestLine := func() float32 {
		longest := float32(math.Inf(-1))
		for i := range lines {
			var total float32
			for j := range lines[i].items {
				total += itemMainLength(&lines[i].items[j])
			}
			longest = fmax(longest, total+sumGaps(c.gap.main(dir), len(lines[i].items)))
		}
		if len(lines) == 0 {
			return 0
		}
		return longest
	}

	var outer float32
	if o := c.nodeOuter.main(dir); o.ok {
		outer = o.v
	} else {
		switch a := avail.main(dir); {
		case a.Kind == Definite && len(lines) > 1:
			// The items wrapped: the container fills the available space.
			outer = fmax(longestLine()+mainCbi, a.Value)
		case a.Kind == Definite:
			// fit-content: min(max-content, max(min-content, available)).
			// Taffy uses the sum of the flex bases here, which ignores the
			// items' max sizes and content; Chromium does fit-content.
			outer = e.flexIntrinsicMainSize(n, avail.withMain(dir, MaxContentConstraint), lines, c) + mainCbi
			if availOuter := a.Value + mainCbi; outer > availOuter {
				minContent := e.flexIntrinsicMainSize(n, avail.withMain(dir, MinContentConstraint), lines, c) + mainCbi
				outer = fmax(minContent, availOuter)
			}
		case a.Kind == MinContentSpace && c.isWrap:
			outer = longestLine() + mainCbi
		default:
			outer = e.flexIntrinsicMainSize(n, avail, lines, c) + mainCbi
		}
	}

	outer = fmax(fclampOpt(outer, c.minSize.main(dir), c.maxSize.main(dir)), mainCbi-c.gutter.main(dir))
	inner := fmax(outer-mainCbi, 0)
	c.containerSize.setMain(dir, outer)
	c.innerContainer.setMain(dir, inner)
	c.nodeInner.setMain(dir, some(inner))
}

// flexIntrinsicMainSize returns the container's min- or max-content main
// size: the largest sum, over the lines, of the items' content
// contributions (9.9.1).
func (e *Engine) flexIntrinsicMainSize(n Node, avail availSize, lines []flexLine, c *flexConsts) float32 {
	dir := c.dir
	var mainSize float32
	for li := range lines {
		line := &lines[li]
		for i := range line.items {
			it := &line.items[i]
			styleMin := it.minSize.main(dir)
			stylePref := it.size.main(dir)
			styleMax := it.maxSize.main(dir)
			marginMain := it.margin.mainSum(dir)

			// The flex base size clamps the contribution of items that do
			// not grow (max) or shrink (min) (csswg-drafts#1435). Taffy
			// takes the larger of the base size and the preferred size;
			// Chromium 152 uses the base size alone
			// (xpadding_border_overrides_size_flex_basis_0).
			clampingBasis := some(it.flexBasis)
			var basisMin, basisMax opt
			if it.flexShrink == 0 {
				basisMin = clampingBasis
			}
			if it.flexGrow == 0 {
				basisMax = clampingBasis
			}
			minMain := fmax(styleMin.maybeMax(basisMin).or(basisMin).unwrapOr(it.resolvedMinMain), it.resolvedMinMain)
			maxMain := styleMax.maybeMin(basisMax).or(basisMax).unwrapOr(inf)

			var contribution float32
			switch {
			case stylePref.ok && (maxMain <= minMain || maxMain <= stylePref.v):
				contribution = fmax(fmin(stylePref.v, maxMain), minMain) + marginMain
			case maxMain <= minMain:
				contribution = minMain + marginMain
			case it.isScrollContainer():
				contribution = it.flexBasis + marginMain
			case stylePref.ok:
				// A definite preferred size is the content contribution,
				// floored by padding and border.
				pb := it.padding.mainSum(dir) + it.border.mainSum(dir)
				content := fmax(stylePref.v, pb) + marginMain
				if c.isRow {
					contribution = fclampOpt(content, styleMin, styleMax)
				} else {
					contribution = fclampOpt(fmax(content, it.flexBasis), styleMin, styleMax)
				}
			default:
				crossParent := c.nodeInner.cross(dir)
				crossMarginSum := c.margin.crossSum(dir)
				childMinCross := it.minSize.cross(dir).addF(crossMarginSum)
				childMaxCross := it.maxSize.cross(dir).addF(crossMarginSum)
				crossAvail := avail.cross(dir)
				if crossAvail.Kind == Definite {
					crossAvail.Value = crossParent.unwrapOr(crossAvail.Value)
				}
				crossAvail = crossAvail.maybeClamp(childMinCross, childMaxCross)

				// The same cross size as the flex base size's
				// (measureCross): a w-fit item of a column contributes
				// its height at its fit-content width.
				var ckd optSize
				var crossKnown opt
				crossKnown, crossAvail, _ = measureCross(c, it, crossAvail, none, none)
				ckd.setCross(dir, crossKnown)
				childAvail := avail.withCross(dir, crossAvail)
				measured := e.measureChildSize(n, it.node, ckd, c.nodeInner, childAvail, contentSize, dir.mainAxis())

				// A known cross size transferred through the aspect ratio
				// floors the measured size.
				if it.ratio.ok {
					if cross := ckd.cross(dir); cross.ok {
						if c.isRow {
							measured = fmax(measured, float32(cross.v*it.ratio.v))
						} else {
							measured = fmax(measured, cross.v/it.ratio.v)
						}
					}
				}
				content := measured + marginMain
				// Asymmetric between rows and columns; this matches WebKit
				// and Firefox (the flex_basis_unconstraint_* fixtures).
				if c.isRow {
					contribution = fclampOpt(content, styleMin, styleMax)
				} else {
					contribution = fclampOpt(fmax(content, it.flexBasis), styleMin, styleMax)
				}
			}

			diff := contribution - it.flexBasis
			switch {
			case diff > 0:
				it.contentFlexFraction = diff / fmax(1, it.flexGrow)
			case diff < 0:
				it.contentFlexFraction = diff / float32(fmax(1, it.flexShrink)*it.innerFlexBasis)
			default:
				it.contentFlexFraction = 0
			}
		}

		// The spec scales by the line's largest flex fraction; Chrome and
		// Firefox use each item's own fraction, and so does Taffy.
		var sum float32
		for i := range line.items {
			it := &line.items[i]
			var contribution float32
			switch {
			case it.contentFlexFraction > 0:
				contribution = float32(fmax(1, it.flexGrow) * it.contentFlexFraction)
			case it.contentFlexFraction < 0:
				scaled := float32(fmax(1, it.flexShrink) * it.innerFlexBasis)
				if scaled != 0 {
					contribution = float32(scaled * it.contentFlexFraction)
				}
			}
			s := it.flexBasis + contribution
			it.outerTarget.setMain(dir, s)
			it.target.setMain(dir, s)
			sum += s
		}
		mainSize = fmax(mainSize, sum+sumGaps(c.gap.main(dir), len(line.items)))
	}
	return mainSize
}

// resolveFlexibleLengths resolves the flexible lengths of a line's items
// (9.7), setting their main target sizes.
func resolveFlexibleLengths(line *flexLine, c *flexConsts) {
	dir := c.dir
	totalGap := sumGaps(c.gap.main(dir), len(line.items))
	inner := c.nodeInner.main(dir)

	// 1. Determine the used flex factor.
	var hypoSum float32
	for i := range line.items {
		hypoSum += line.items[i].hypoOuter.main(dir)
	}
	usedFlexFactor := totalGap + hypoSum
	growing := usedFlexFactor < inner.or0()
	shrinking := usedFlexFactor > inner.or0()
	exact := !growing && !shrinking

	// 2. Size inflexible items.
	for i := range line.items {
		it := &line.items[i]
		innerTarget := it.hypoInner.main(dir)
		it.target.setMain(dir, innerTarget)
		if exact || (it.flexGrow == 0 && it.flexShrink == 0) ||
			(growing && it.flexBasis > it.hypoInner.main(dir)) ||
			(shrinking && it.flexBasis < it.hypoInner.main(dir)) {
			it.frozen = true
			it.outerTarget.setMain(dir, innerTarget+it.margin.mainSum(dir))
		}
	}
	if exact {
		return
	}

	usedSpace := func() float32 {
		s := totalGap
		for i := range line.items {
			it := &line.items[i]
			if it.frozen {
				s += it.outerTarget.main(dir)
			} else {
				s += it.flexBasis + it.margin.mainSum(dir)
			}
		}
		return s
	}

	// 3. Calculate the initial free space.
	initialFree := inner.subF(usedSpace()).or0()

	// 4. Loop.
	for {
		// a. Check for flexible items.
		allFrozen := true
		for i := range line.items {
			allFrozen = allFrozen && line.items[i].frozen
		}
		if allFrozen {
			break
		}

		// b. Calculate the remaining free space.
		used := usedSpace()
		var sumGrow, sumShrink float32
		for i := range line.items {
			if it := &line.items[i]; !it.frozen {
				sumGrow += it.flexGrow
				sumShrink += it.flexShrink
			}
		}
		var free float32
		switch {
		case growing && sumGrow < 1:
			free = fminOpt(float32(initialFree*sumGrow)-totalGap, inner.subF(used))
		case shrinking && sumShrink < 1:
			free = fmaxOpt(float32(initialFree*sumShrink)-totalGap, inner.subF(used))
		default:
			free = inner.subF(used).unwrapOr(usedFlexFactor - used)
		}

		// c. Distribute the free space proportionally to the flex factors.
		if isNormal(free) {
			if growing && sumGrow > 0 {
				for i := range line.items {
					if it := &line.items[i]; !it.frozen {
						it.target.setMain(dir, it.flexBasis+float32(free*(it.flexGrow/sumGrow)))
					}
				}
			} else if shrinking && sumShrink > 0 {
				var sumScaled float32
				for i := range line.items {
					if it := &line.items[i]; !it.frozen {
						sumScaled += float32(it.innerFlexBasis * it.flexShrink)
					}
				}
				if sumScaled > 0 {
					for i := range line.items {
						if it := &line.items[i]; !it.frozen {
							scaled := float32(it.innerFlexBasis * it.flexShrink)
							it.target.setMain(dir, it.flexBasis+float32(free*(scaled/sumScaled)))
						}
					}
				}
			}
		}

		// d. Fix min/max violations.
		var totalViolation float32
		for i := range line.items {
			it := &line.items[i]
			if it.frozen {
				continue
			}
			clamped := fmax(fclampOpt(it.target.main(dir), some(it.resolvedMinMain), it.maxSize.main(dir)), 0)
			it.violation = clamped - it.target.main(dir)
			it.target.setMain(dir, clamped)
			it.outerTarget.setMain(dir, clamped+it.margin.mainSum(dir))
			totalViolation += it.violation
		}

		// e. Freeze over-flexed items.
		for i := range line.items {
			it := &line.items[i]
			if it.frozen {
				continue
			}
			switch {
			case totalViolation > 0:
				it.frozen = it.violation > 0
			case totalViolation < 0:
				it.frozen = it.violation < 0
			default:
				it.frozen = true
			}
		}
	}
}

// hypotheticalCrossSizes determines the hypothetical cross size of each
// item of a line (9.4.7): layout with the used main size and the available
// space, auto as fit-content.
func (e *Engine) hypotheticalCrossSizes(n Node, line *flexLine, c *flexConsts, avail availSize) {
	dir := c.dir
	for i := range line.items {
		it := &line.items[i]
		pbCross := it.padding.crossSum(dir) + it.border.crossSum(dir)
		knownMain := DefiniteSpace(c.containerSize.main(dir))

		// Transferred sizes clamp the hypothetical cross size
		// (csswg-drafts#10997).
		tMin := it.minSize.applyAspectRatio(it.ratio).cross(dir)
		tMax := it.maxSize.applyAspectRatio(it.ratio).cross(dir)

		// Without a cross size style, an item with an aspect ratio takes
		// the cross size of its used main size (a flexed w-full
		// aspect-square item stays square); a size query of a leaf or
		// block in content-size mode would not apply the ratio.
		childCross := it.crossSize
		if !childCross.ok && it.ratio.ok {
			if c.isRow {
				childCross = some(it.target.w / it.ratio.v)
			} else {
				childCross = some(float32(it.target.h * it.ratio.v))
			}
		}
		childCross = childCross.maybeClamp(tMin, tMax).maxF(pbCross)
		availCross := avail.cross(dir).maybeClamp(tMin, tMax).maxF(pbCross)

		// A cross size keyword gives the space to measure under; stretch is
		// handled with the line's cross size.
		stretch := c.nodeInner.cross(dir).subF(it.margin.crossSum(dir))
		if k, space, _ := resolveSizingKeyword(it.sizeStyle.cross(dir), stretch, c.nodeInner.cross(dir)); k == keywordMeasure {
			availCross = space
		}

		var innerCross float32
		if childCross.ok {
			innerCross = childCross.v
		} else {
			var known optSize
			var childAvail availSize
			if c.isRow {
				known = optSize{some(it.target.w), childCross}
				childAvail = availSize{knownMain, availCross}
			} else {
				known = optSize{childCross, some(it.target.h)}
				childAvail = availSize{availCross, knownMain}
			}
			out := e.child(n, it.node, input{
				run: computeSize, sizing: contentSize, axis: axisRequest(dir.crossAxis()),
				known: known, knownDefinite: itemDefiniteness(c, it),
				parent: c.nodeInner, avail: childAvail,
			})
			innerCross = fmax(fclampOpt(out.size.get(dir.crossAxis()), tMin, tMax), pbCross)
		}
		it.hypoInner.setCross(dir, innerCross)
		it.hypoOuter.setCross(dir, innerCross+it.margin.crossSum(dir))
	}
}

// flexBaselines computes the baselines of the items that take part in
// baseline alignment, in rows (the only axis where Tela has baselines).
func (e *Engine) flexBaselines(n Node, nodeSize optSize, avail availSize, lines []flexLine, c *flexConsts) {
	if !c.isRow {
		return
	}
	for li := range lines {
		line := &lines[li]
		count := 0
		for i := range line.items {
			if line.items[i].participatesInBaseline(c.dir) {
				count++
			}
		}
		// With one participant, baseline alignment is a no-op.
		if count <= 1 {
			continue
		}
		for i := range line.items {
			it := &line.items[i]
			if !it.participatesInBaseline(c.dir) {
				continue
			}
			out := e.child(n, it.node, input{
				run: performLayout, sizing: contentSize, axis: axisBoth,
				known:         optSize{some(it.target.w), some(it.hypoInner.h)},
				knownDefinite: itemDefiniteness(c, it),
				parent:        c.nodeInner,
				avail:         availSize{DefiniteSpace(c.containerSize.w), avail.h.maybeSet(nodeSize.h)},
			})
			// A scroll container's baseline is clamped to its border box.
			h := out.size.h
			b := out.firstBaseline.unwrapOr(h)
			if it.st.OverflowY.isScrollContainer() {
				b = fmax(fmin(b, h), 0)
			}
			it.baseline = b + it.margin.t
		}
	}
}

// flexLineCrossSizes calculates the cross size of each line (9.4.8).
func flexLineCrossSizes(lines []flexLine, nodeSize optSize, c *flexConsts) {
	dir := c.dir
	cbiCross := c.cbi.crossSum(dir)
	minCross, maxCross := c.minSize.cross(dir), c.maxSize.cross(dir)
	// A single-line container with a definite cross size: the line is the
	// inner cross size.
	if !c.isWrap {
		if cs := nodeSize.cross(dir); cs.ok {
			lines[0].crossSize = cs.maybeClamp(minCross, maxCross).subF(cbiCross).maxF(0).or0()
			return
		}
	}
	// Else the largest outer hypothetical cross size, with baseline
	// participants contributing their distances above and below the
	// largest baseline.
	for li := range lines {
		line := &lines[li]
		var maxBaseline float32
		for i := range line.items {
			maxBaseline = fmax(maxBaseline, line.items[i].baseline)
		}
		var cross float32
		for i := range line.items {
			it := &line.items[i]
			v := it.hypoOuter.cross(dir)
			if it.participatesInBaseline(dir) {
				v = maxBaseline - it.baseline + it.hypoOuter.cross(dir)
			}
			cross = fmax(cross, v)
		}
		line.crossSize = cross
	}
	// A single line is clamped by the container's min and max cross sizes.
	if !c.isWrap {
		lines[0].crossSize = fclampOpt(lines[0].crossSize, minCross.subF(cbiCross), maxCross.subF(cbiCross))
	}
}

// alignContentStretch handles align-content: stretch (9.4.9): with a
// definite cross size, lines grow equally to fill the container.
func alignContentStretch(lines []flexLine, nodeSize optSize, c *flexConsts) {
	if c.alignContent != AlignStretch {
		return
	}
	dir := c.dir
	minCross, maxCross := c.minSize.cross(dir), c.maxSize.cross(dir)
	containerMinInner := nodeSize.cross(dir).or(minCross).maybeClamp(minCross, maxCross).subF(c.cbi.crossSum(dir)).maxF(0).or0()
	total := sumGaps(c.gap.cross(dir), len(lines))
	for i := range lines {
		total += lines[i].crossSize
	}
	if total < containerMinInner {
		add := (containerMinInner - total) / float32(len(lines))
		for i := range lines {
			lines[i].crossSize += add
		}
	}
}

// usedCrossSizes determines the used cross size of each item (9.4.11):
// stretched items take the line's cross size, clamped by their min and
// max cross sizes.
func usedCrossSizes(lines []flexLine, c *flexConsts) {
	dir := c.dir
	for li := range lines {
		line := &lines[li]
		for i := range line.items {
			it := &line.items[i]
			crossStretch := it.sizeStyle.cross(dir).isStretch()
			if !it.marginAuto.crossStart(dir) && !it.marginAuto.crossEnd(dir) &&
				(crossStretch || (it.alignSelf == AlignStretch && it.sizeStyle.cross(dir).isAuto())) {
				// The max size here ignores the aspect ratio; Chrome and
				// Firefox agree.
				maxIgnoringRatio := it.st.maxSize().resolve(c.nodeInner)
				it.target.setCross(dir, fclampOpt(line.crossSize-it.margin.crossSum(dir), it.minSize.cross(dir), maxIgnoringRatio.cross(dir)))
			} else {
				it.target.setCross(dir, it.hypoInner.cross(dir))
			}
			it.outerTarget.setCross(dir, it.target.cross(dir)+it.margin.crossSum(dir))
		}
	}
}

// distributeFreeSpace distributes the remaining free space of each line
// (9.5.12): auto margins first, then justify-content.
func distributeFreeSpace(lines []flexLine, c *flexConsts) {
	dir := c.dir
	for li := range lines {
		line := &lines[li]
		used := sumGaps(c.gap.main(dir), len(line.items))
		autoMargins := 0
		for i := range line.items {
			it := &line.items[i]
			used += it.outerTarget.main(dir)
			if it.marginAuto.mainStart(dir) {
				autoMargins++
			}
			if it.marginAuto.mainEnd(dir) {
				autoMargins++
			}
		}
		free := c.innerContainer.main(dir) - used

		if free > 0 && autoMargins > 0 {
			m := free / float32(autoMargins)
			for i := range line.items {
				it := &line.items[i]
				if it.marginAuto.mainStart(dir) {
					if c.isRow {
						it.margin.l = m
					} else {
						it.margin.t = m
					}
				}
				if it.marginAuto.mainEnd(dir) {
					if c.isRow {
						it.margin.r = m
					} else {
						it.margin.b = m
					}
				}
			}
			free = 0
		}

		justify := c.justifyContent
		if justify == AlignNormal {
			justify = AlignFlexStart
		}
		n := len(line.items)
		k := alignmentFallback(free, n, justify)
		reverse := dir.isReverse()
		for i := range line.items {
			idx := i
			if reverse {
				idx = n - 1 - i
			}
			line.items[idx].offsetMain = alignmentOffset(free, n, c.gap.main(dir), k, reverse, i == 0)
		}
	}
}

// crossAutoMargins resolves cross-axis auto margins (9.6.13) and aligns
// the other items per align-self (9.6.14).
func crossAutoMargins(lines []flexLine, c *flexConsts) {
	dir := c.dir
	for li := range lines {
		line := &lines[li]
		var maxBaseline, maxBelowBaseline float32
		for i := range line.items {
			it := &line.items[i]
			maxBaseline = fmax(maxBaseline, it.baseline)
			if it.participatesInBaseline(dir) {
				maxBelowBaseline = fmax(maxBelowBaseline, it.outerTarget.cross(dir)-it.baseline)
			}
		}
		for i := range line.items {
			it := &line.items[i]
			free := line.crossSize - it.outerTarget.cross(dir)
			start, end := it.marginAuto.crossStart(dir), it.marginAuto.crossEnd(dir)
			switch {
			case start && end:
				if c.isRow {
					it.margin.t, it.margin.b = free/2, free/2
				} else {
					it.margin.l, it.margin.r = free/2, free/2
				}
			case start:
				if c.isRow {
					it.margin.t = free
				} else {
					it.margin.l = free
				}
			case end:
				if c.isRow {
					it.margin.b = free
				} else {
					it.margin.r = free
				}
			default:
				it.offsetCross = alignCross(it, free, maxBaseline, maxBelowBaseline, c)
			}
		}
	}
}

// alignCross returns an item's cross offset in its line per align-self.
func alignCross(it *flexItem, free, maxBaseline, maxBelowBaseline float32, c *flexConsts) float32 {
	// Column containers in RTL lay out their cross axis right to left.
	reverse := c.isColumn && c.layoutDir == RTL

	// Safe alignment falls back to start when the item overflows.
	k := it.alignSelf.keyword()
	if it.alignSelf.isSafe() && free < 0 {
		k = AlignStart
	}
	switch k {
	case AlignStart:
		if reverse {
			return free
		}
		return 0
	case AlignEnd:
		if reverse {
			return 0
		}
		return free
	case AlignFlexEnd:
		if c.isWrapReverse != reverse {
			return 0
		}
		return free
	case AlignCenter:
		return free / 2
	case AlignBaseline:
		if c.isRow {
			if c.isWrapReverse {
				// The cross axis is flipped: baselines align to the bottom.
				lineCross := free + it.outerTarget.cross(c.dir)
				return lineCross - maxBelowBaseline - it.baseline
			}
			return maxBaseline - it.baseline
		}
		// Columns have no baselines: flex-start.
		if c.isWrapReverse != (reverse && !c.isWrap) {
			return free
		}
		return 0
	default: // flex-start, stretch
		if c.isWrapReverse != reverse {
			return free
		}
		return 0
	}
}

// flexContainerCrossSize determines the container's used cross size
// (9.6.15) and returns the sum of the lines' cross sizes.
func flexContainerCrossSize(lines []flexLine, nodeSize optSize, c *flexConsts) float32 {
	dir := c.dir
	totalGap := sumGaps(c.gap.cross(dir), len(lines))
	var total float32
	for i := range lines {
		total += lines[i].crossSize
	}
	cbiCross := c.cbi.crossSum(dir)
	outer := fmax(fclampOpt(nodeSize.cross(dir).unwrapOr(total+totalGap+cbiCross), c.minSize.cross(dir), c.maxSize.cross(dir)),
		cbiCross-c.gutter.cross(dir))
	c.containerSize.setCross(dir, outer)
	c.innerContainer.setCross(dir, fmax(outer-cbiCross, 0))
	return total
}

// alignFlexLines aligns the lines per align-content (9.6.16).
func alignFlexLines(lines []flexLine, c *flexConsts, totalCross float32) {
	n := len(lines)
	gap := c.gap.cross(c.dir)
	free := c.innerContainer.cross(c.dir) - totalCross - sumGaps(gap, n)
	k := alignmentFallback(free, n, c.alignContent)
	for i := range lines {
		idx := i
		if c.isWrapReverse {
			idx = n - 1 - i
		}
		lines[idx].offsetCross = alignmentOffset(free, n, gap, k, c.isWrapReverse, i == 0)
	}
}

// flexFinalPass lays out every item at its final size and position. It
// returns the in-flow scrollable overflow and whether any item hoists
// absolutely positioned boxes.
func (e *Engine) flexFinalPass(n Node, lines []flexLine, c *flexConsts) (rect, bool) {
	var crossOffset float32
	if c.isColumn && c.layoutDir == RTL {
		crossOffset = c.containerSize.w - c.cbi.crossEnd(c.dir)
	} else {
		crossOffset = c.cbi.crossStart(c.dir)
	}
	var of rect
	var escapes bool
	if c.isWrapReverse {
		for i := len(lines) - 1; i >= 0; i-- {
			escapes = e.flexLayoutLine(n, &lines[i], &crossOffset, &of, c) || escapes
		}
	} else {
		for i := range lines {
			escapes = e.flexLayoutLine(n, &lines[i], &crossOffset, &of, c) || escapes
		}
	}
	// A scroll container's end padding is part of its scrollable overflow.
	if c.scrolls {
		if c.layoutDir == RTL {
			of.r += c.cbi.l - c.border.l - c.gutter.x
		} else {
			of.r += c.cbi.r - c.border.r - c.gutter.x
		}
		of.b += c.cbi.b - c.border.b - c.gutter.y
	}
	return of, escapes
}

func (e *Engine) flexLayoutLine(n Node, line *flexLine, crossOffset *float32, of *rect, c *flexConsts) bool {
	dir := c.dir
	var mainOffset float32
	if c.layoutDir == RTL && c.isRow {
		mainOffset = c.containerSize.w - c.cbi.mainEnd(dir)
	} else {
		mainOffset = c.cbi.mainStart(dir)
	}
	rtlColumn := c.layoutDir == RTL && c.isColumn
	if rtlColumn {
		*crossOffset -= line.offsetCross + line.crossSize
	}
	escapes := false
	if dir.isReverse() {
		for i := len(line.items) - 1; i >= 0; i-- {
			escapes = e.flexLayoutItem(n, &line.items[i], &mainOffset, *crossOffset, line.offsetCross, of, c) || escapes
		}
	} else {
		for i := range line.items {
			escapes = e.flexLayoutItem(n, &line.items[i], &mainOffset, *crossOffset, line.offsetCross, of, c) || escapes
		}
	}
	if !rtlColumn {
		*crossOffset += line.offsetCross + line.crossSize
	}
	return escapes
}

// flexLayoutItem lays out one item at its final size and records its
// position.
func (e *Engine) flexLayoutItem(n Node, it *flexItem, mainOffset *float32, crossOffset, lineOffsetCross float32, of *rect, c *flexConsts) bool {
	dir := c.dir
	out := e.child(n, it.node, input{
		run: performLayout, sizing: contentSize, axis: axisBoth,
		known:         it.target.opts(),
		knownDefinite: itemDefiniteness(c, it),
		parent:        c.nodeInner,
		avail:         availSize{DefiniteSpace(c.containerSize.w), DefiniteSpace(c.containerSize.h)},
	})
	sz := out.size

	rtlRow := c.isRow && c.layoutDir == RTL
	rtlColumn := c.isColumn && c.layoutDir == RTL

	// Relative offsets (static items ignore their insets).
	var mainRel, crossRel float32
	if it.st.Position == PositionRelative {
		if rtlRow {
			if v := it.inset.mainEnd(dir); v.ok {
				mainRel = v.v
			} else if v := it.inset.mainStart(dir); v.ok {
				mainRel = -v.v
			}
		} else {
			if v := it.inset.mainStart(dir); v.ok {
				mainRel = v.v
			} else if v := it.inset.mainEnd(dir); v.ok {
				mainRel = -v.v
			}
		}
		if rtlColumn {
			if v := it.inset.crossEnd(dir); v.ok {
				crossRel = -v.v
			} else if v := it.inset.crossStart(dir); v.ok {
				crossRel = v.v
			}
		} else {
			if v := it.inset.crossStart(dir); v.ok {
				crossRel = v.v
			} else if v := it.inset.crossEnd(dir); v.ok {
				crossRel = -v.v
			}
		}
	}
	effLineOffset := lineOffsetCross
	if rtlColumn {
		effLineOffset = 0
	}

	var offMain float32
	if rtlRow {
		offMain = *mainOffset - it.offsetMain - it.margin.mainEnd(dir) - mainRel - sz.w
	} else {
		offMain = *mainOffset + it.offsetMain + it.margin.mainStart(dir) + mainRel
	}
	offCross := crossOffset + it.offsetCross + effLineOffset + it.margin.crossStart(dir) + crossRel

	if c.isRow {
		// A scroll container's baseline is clamped to its border box.
		b := out.firstBaseline.unwrapOr(sz.h)
		if it.st.OverflowY.isScrollContainer() {
			b = fmax(fmin(b, sz.h), 0)
		}
		it.baseline = crossOffset + it.offsetCross + effLineOffset + it.margin.crossStart(dir) + b
	} else {
		it.baseline = *mainOffset + it.offsetMain + it.margin.mainStart(dir) + out.firstBaseline.unwrapOr(sz.h)
	}

	loc := point{offMain, offCross}
	if !c.isRow {
		loc = point{offCross, offMain}
	}
	var sb point
	if it.st.OverflowY == OverflowScroll {
		sb.x = it.st.ScrollbarWidth
	}
	if it.st.OverflowX == OverflowScroll {
		sb.y = it.st.ScrollbarWidth
	}
	it.node.LayoutData().setLayout(loc, sz, it.border, it.padding, it.margin, sb, out.overflow, out.firstBaseline)

	if rtlRow {
		*mainOffset -= it.offsetMain + it.margin.mainSum(dir) + sz.main(dir)
	} else {
		*mainOffset += it.offsetMain + it.margin.mainSum(dir) + sz.main(dir)
	}

	contribLoc := point{loc.x - c.border.l, loc.y - c.border.t}
	if c.layoutDir == RTL {
		contribLoc.x = c.containerSize.w - (loc.x + sz.w) - c.border.r
	}
	*of = of.union(overflowContribution(contribLoc, sz, out.overflow, it.st, c.scrolls))
	return out.escapes && it.st.passesAbs()
}
