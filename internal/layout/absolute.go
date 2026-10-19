// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (perform_absolute_layout_on_absolute_children
// in src/compute/block.rs and src/compute/flexbox.rs); see THIRD_PARTY.md.
//
// Taffy places an absolutely positioned box against its parent. Tela, like
// CSS, places it against its nearest positioned ancestor (ARCHITECTURE
// section 9): a static parent records the box's static position and
// reports it upwards (output.escapes); the containing block lays it out
// after its own children (layoutEscaped).

package layout

// alignMode is how a static position aligns a box in one axis.
type alignMode uint8

const (
	alignStartMode alignMode = iota
	alignEndMode
	alignCenterMode
)

// staticPos is the static position of an absolutely positioned box: where
// it would be if it were in flow, as an alignment of its margin box inside
// [x0, x1] × [y0, y1] (coordinates from its parent's border box). With
// safeX or safeY, a box that overflows the range takes the fallback mode
// instead (safe alignment). Chromium 152 applies safe alignment to the
// static position only when the box's own align-self has it and its flex
// parent is its containing block.
type staticPos struct {
	x0, x1, y0, y1       float32
	modeX, modeY         alignMode
	safeX, safeY         bool
	fallbackX, fallbackY alignMode
}

// translate moves the static position by off.
func (s staticPos) translate(off point) staticPos {
	s.x0 += off.x
	s.x1 += off.x
	s.y0 += off.y
	s.y1 += off.y
	return s
}

// placeAxis aligns a box of size sz with margins m0 and m1 in [a0, a1].
func placeAxis(mode alignMode, a0, a1, sz, m0, m1 float32) float32 {
	switch mode {
	case alignEndMode:
		return a1 - sz - m1
	case alignCenterMode:
		return (a0 + a1 - sz + m0 - m1) / 2
	}
	return a0 + m0
}

// place returns the border-box position of a box of size sz and margins m
// at the static position.
func (s staticPos) place(sz size, m rect) point {
	mx, my := s.modeX, s.modeY
	if s.safeX && sz.w+m.hsum() > s.x1-s.x0 {
		mx = s.fallbackX
	}
	if s.safeY && sz.h+m.vsum() > s.y1-s.y0 {
		my = s.fallbackY
	}
	return point{placeAxis(mx, s.x0, s.x1, sz.w, m.l, m.r), placeAxis(my, s.y0, s.y1, sz.h, m.t, m.b)}
}

// layoutAbsolute lays out an absolutely positioned box c (child of p)
// against a containing block whose padding box is area, at areaOffset from
// the containing block's border box (ported from Taffy's block version,
// for every kind of parent). sp is the static position and parentOff the
// position of p's border box, both from the containing block's border box.
// It returns the box's contribution to the containing block's scrollable
// overflow.
func (e *Engine) layoutAbsolute(p, c Node, st *Style, area size, areaOffset point, sp staticPos, dir Direction, scrolls bool, parentOff point) rect {
	loc, final, out := e.placeAbsolute(p, c, st, area, areaOffset, sp, dir, parentOff)
	// The overflow contribution is measured from the scroll origin (the
	// inline-start edge: the right side in RTL).
	rel := point{loc.x - areaOffset.x, loc.y - areaOffset.y}
	if dir == RTL {
		rel.x = area.w - rel.x - final.w
	}
	return overflowContribution(rel, final, out.overflow, st, scrolls)
}

// placeAbsolute is layoutAbsolute without the overflow contribution: it
// returns the box's border-box position from the containing block's
// border box, its size and the output of its layout.
func (e *Engine) placeAbsolute(p, c Node, st *Style, area size, areaOffset point, sp staticPos, dir Direction, parentOff point) (point, size, output) {
	ratio := st.aspectRatio()
	aw := some(area.w)
	margin := st.Margin.resolveMargins(aw)
	padding := st.Padding.resolveOrZero(aw)
	border := st.Border.resolveOrZero(aw)
	pbSum := padding.add(border).sum()

	left := st.Inset.Left.resolveSize(aw)
	right := st.Inset.Right.resolveSize(aw)
	top := st.Inset.Top.resolveSize(some(area.h))
	bottom := st.Inset.Bottom.resolveSize(some(area.h))

	sizeStyle := st.size()
	areaOpts := area.opts()
	minSize := st.minSize().resolve(areaOpts).applyAspectRatio(ratio).or(pbSum.opts()).maybeMaxF(pbSum)
	maxSize := st.maxSize().resolve(areaOpts).applyAspectRatio(ratio)
	known := sizeStyle.resolve(areaOpts).applyAspectRatio(ratio).maybeClamp(minSize, maxSize)

	// Sizing keywords; an explicit size wins over the insets below.
	if sizeStyle.w.isSizingKeyword() || sizeStyle.h.isSizingKeyword() {
		e.resolveAbsoluteSizingKeywords(p, c, &known, sizeStyle, area, optRect{left, right, top, bottom}, margin, contentSize)
		known = known.applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
	}
	// Both insets in an axis give the size, then the aspect ratio applies.
	if !known.w.ok && left.ok && right.ok {
		known.w = some(fmax(fsubOpt(fsubOpt(area.w, margin.l), margin.r)-left.v-right.v, 0))
		known = known.applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
	}
	if !known.h.ok && top.ok && bottom.ok {
		known.h = some(fmax(fsubOpt(fsubOpt(area.h, margin.t), margin.b)-top.v-bottom.v, 0))
		known = known.applyAspectRatio(ratio).maybeClamp(minSize, maxSize)
	}

	// The space available to a shrink-to-fit box is the containing block
	// minus the insets that are set or, without any, minus the static
	// position (CSS 2 10.3.7; Taffy uses the whole containing block).
	avail := availSize{
		DefiniteSpace(fclampOpt(absAvailable(area.w, left, right, sp.x0-areaOffset.x, sp.x1-areaOffset.x, sp.modeX), minSize.w, maxSize.w)),
		DefiniteSpace(fclampOpt(absAvailable(area.h, top, bottom, sp.y0-areaOffset.y, sp.y1-areaOffset.y, sp.modeY), minSize.h, maxSize.h)),
	}
	// A height from the content (auto) is not definite for the box's own
	// content (percentages inside it do not resolve against it, a column
	// wraps at its max height): the box sizes itself again in that axis.
	// A fit-content width is definite (css-sizing-3).
	definite := boolSize{true, known.h.ok}
	var final size
	if known.bothSet() {
		final = size{known.w.v, known.h.v}
	} else {
		final = known.unwrapOr(e.measureChildSizeBoth(p, c, known, areaOpts, avail, contentSize))
	}
	final = final.maybeClamp(minSize, maxSize)
	out := e.performChildLayout(p, c, contentKnown(final, definite), areaOpts, avail, contentSize)

	// Auto margins share the free space only in an axis with an inset
	// that constrains them (CSS 2 10.3.7 and 10.6.4).
	var nonAuto rect
	if left.ok {
		nonAuto.l = margin.l.or0()
	}
	if right.ok {
		nonAuto.r = margin.r.or0()
	}
	if top.ok {
		nonAuto.t = margin.t.or0()
	}
	if bottom.ok {
		nonAuto.b = margin.b.or0()
	}
	spaceX, spaceY := final.w, final.h
	if right.ok {
		spaceX = area.w - right.v - left.or0()
	}
	if bottom.ok {
		spaceY = area.h - bottom.v - top.or0()
	}
	autoW := autoMarginShare(margin.l, margin.r, spaceX-final.w-nonAuto.hsum())
	autoH := autoMarginShare(margin.t, margin.b, spaceY-final.h-nonAuto.vsum())
	resolved := rect{margin.l.unwrapOr(autoW), margin.r.unwrapOr(autoW), margin.t.unwrapOr(autoH), margin.b.unwrapOr(autoH)}

	var loc point
	staticLoc := sp.place(final, resolved)
	switch {
	case left.ok && right.ok:
		// Over-constrained: right is ignored in LTR, left in RTL.
		if dir == RTL {
			loc.x = area.w - final.w - right.v - resolved.r + areaOffset.x
		} else {
			loc.x = left.v + resolved.l + areaOffset.x
		}
	case left.ok:
		loc.x = left.v + resolved.l + areaOffset.x
	case right.ok:
		loc.x = area.w - final.w - right.v - resolved.r + areaOffset.x
	default:
		loc.x = staticLoc.x
	}
	switch {
	case top.ok:
		loc.y = top.v + resolved.t + areaOffset.y
	case bottom.ok:
		loc.y = area.h - final.h - bottom.v - resolved.b + areaOffset.y
	default:
		loc.y = staticLoc.y
	}

	var sb point
	if st.OverflowY == OverflowScroll {
		sb.x = st.ScrollbarWidth
	}
	if st.OverflowX == OverflowScroll {
		sb.y = st.ScrollbarWidth
	}
	c.LayoutData().setLayout(point{loc.x - parentOff.x, loc.y - parentOff.y}, final, border, padding, resolved, sb, out.overflow, out.firstBaseline)
	return loc, final, out
}

// absAvailable returns the space available to an absolutely positioned box
// in one axis of a containing block of size area: area minus its insets
// that are set, or, when both are auto, minus the space before the static
// position (s0 and s1 are the static range from the area's start, mode the
// static alignment).
func absAvailable(area float32, start, end opt, s0, s1 float32, mode alignMode) float32 {
	var a float32
	switch {
	case start.ok || end.ok:
		a = area - start.or0() - end.or0()
	case mode == alignStartMode:
		a = area - s0
	case mode == alignEndMode:
		a = s1
	default:
		a = area
	}
	return fmax(a, 0)
}

// autoMarginShare returns the size of each auto margin of a pair: the free
// space divided between them, or 0 when both are auto and there is no free
// space.
func autoMarginShare(m0, m1 opt, free float32) float32 {
	var count float32
	if !m0.ok {
		count++
	}
	if !m1.ok {
		count++
	}
	switch {
	case count == 2 && free <= 0:
		return 0
	case count > 0:
		return free / count
	}
	return 0
}

// layoutEscaped lays out the absolutely positioned descendants of cb whose
// containing block is cb but whose parent is not: the boxes hoisted by
// cb's static descendants. area and areaOffset are cb's padding box.
func (e *Engine) layoutEscaped(cb Node, area size, areaOffset point, dir Direction, scrolls bool) rect {
	var of rect
	e.walkEscaped(cb, point{}, true, area, areaOffset, dir, scrolls, &of)
	return of
}

func (e *Engine) walkEscaped(x Node, off point, top bool, area size, areaOffset point, dir Direction, scrolls bool, of *rect) {
	for i, count := 0, x.LayoutChildCount(); i < count; i++ {
		ch := x.LayoutChild(i)
		st := ch.LayoutStyle()
		if st.Display == DisplayNone {
			continue
		}
		d := ch.LayoutData()
		switch {
		case st.Position == PositionAbsolute:
			// cb's own absolutely positioned children are already laid
			// out; fixed ones belong to their fixed containing block.
			if !top && !st.Fixed {
				*of = of.union(e.layoutAbsolute(x, ch, st, area, areaOffset, d.static.translate(off), dir, scrolls, off))
			}
		case st.passesAbs() && d.hasLast && d.lastOut.escapes:
			l := d.unrounded.Location
			e.walkEscaped(ch, point{off.x + l.X, off.y + l.Y}, false, area, areaOffset, dir, scrolls, of)
		}
	}
}

// flexAbsolute lays out the absolutely positioned children of a flex
// container if it is their containing block, or records their static
// positions for their containing block. It returns their overflow
// contribution and whether any was hoisted.
//
// Unlike Taffy, which has a flex variant of absolute layout, the children
// are laid out like those of a block container (CSS 2 10.3.7: available
// space and auto margins from the insets, percentages against the padding
// box) with the flex static position (CSS Flexbox 4.1); Chromium agrees.
func (e *Engine) flexAbsolute(n Node, st *Style, c *flexConsts) (rect, bool) {
	cb := st.absContainer(n.LayoutData().parent == nil)
	area, areaOffset := c.absoluteArea()
	mainIsRTL := c.isRow && c.layoutDir == RTL
	crossIsRTL := !c.isRow && c.layoutDir == RTL
	mainReversed := c.dir.isReverse() != mainIsRTL
	crossReversed := c.isWrapReverse != crossIsRTL
	justify := c.justifyContent
	if justify == AlignNormal {
		justify = AlignFlexStart
	}

	var of rect
	escapes := false
	for i, count := 0, n.LayoutChildCount(); i < count; i++ {
		ch := n.LayoutChild(i)
		cst := ch.LayoutStyle()
		if cst.Display == DisplayNone || cst.Position != PositionAbsolute {
			continue
		}
		// Chromium applies safe alignment to an absolutely positioned item
		// only when its own align-self says so, not from align-items, and
		// only when this container is its containing block.
		alignSelf := cst.AlignSelf
		if alignSelf == AlignNormal {
			alignSelf = c.alignItems &^ AlignSafe
		} else if !cb || cst.Fixed {
			alignSelf &^= AlignSafe
		}
		alignSelf = resolveSelfAlign(alignSelf, cst.Direction, c.layoutDir, c.isColumn)
		sp := flexStaticPos(c, justify, alignSelf, mainIsRTL, crossIsRTL, mainReversed, crossReversed)
		if cst.Fixed {
			// Laid out by its fixed containing block (fixed.go).
			ch.LayoutData().static = sp
			continue
		}
		if !cb {
			ch.LayoutData().static = sp
			escapes = true
			continue
		}
		of = of.union(e.layoutAbsolute(n, ch, cst, area, areaOffset, sp, c.layoutDir, c.scrolls, point{}))
	}
	return of, escapes
}

// absoluteArea returns the padding box of a flex container, the containing
// block of its absolutely positioned descendants, and its offset from the
// border box. A vertical scrollbar is on the inline-end side.
func (c *flexConsts) absoluteArea() (size, point) {
	in := c.border
	in.b += c.gutter.y
	if c.layoutDir == RTL {
		in.l += c.gutter.x
	} else {
		in.r += c.gutter.x
	}
	return c.containerSize.sub(in.sum()), point{in.l, in.t}
}

// flexStaticPos returns the static position of an absolutely positioned
// child of a flex container: aligned in the content box as if it were the
// only item, by justify-content and align-self (start and end follow the
// writing mode, flex-start and flex-end the flex directions; Chromium does
// not apply safe to justify-content here). These are the rules of Taffy's
// flex variant of absolute layout for auto insets.
func flexStaticPos(c *flexConsts, justify, alignSelf Align, mainIsRTL, crossIsRTL, mainReversed, crossReversed bool) staticPos {
	dir := c.dir
	mainStart := c.cbi.mainStart(dir)
	mainEnd := c.containerSize.main(dir) - c.cbi.mainEnd(dir)
	crossStart := c.cbi.crossStart(dir)
	crossEnd := c.containerSize.cross(dir) - c.cbi.crossEnd(dir)

	var mainMode alignMode
	switch k := justify.keyword(); {
	case k == AlignStart:
		mainMode = startEnd(!mainIsRTL)
	case k == AlignEnd:
		mainMode = startEnd(mainIsRTL)
	case k == AlignSpaceEvenly || k == AlignSpaceAround || k == AlignCenter:
		mainMode = alignCenterMode
	case k == AlignFlexEnd:
		mainMode = startEnd(mainReversed)
	default: // flex-start, space-between, stretch
		mainMode = startEnd(!mainReversed)
	}

	crossMode := crossModeFor(alignSelf.keyword(), crossIsRTL, crossReversed)
	safe := alignSelf.isSafe()
	fallback := crossModeFor(AlignStart, crossIsRTL, crossReversed)
	if !c.isRow {
		return staticPos{x0: crossStart, x1: crossEnd, y0: mainStart, y1: mainEnd, modeX: crossMode, modeY: mainMode,
			safeX: safe, fallbackX: fallback}
	}
	return staticPos{x0: mainStart, x1: mainEnd, y0: crossStart, y1: crossEnd, modeX: mainMode, modeY: crossMode,
		safeY: safe, fallbackY: fallback}
}

// startEnd returns the start mode if start, else the end mode.
func startEnd(start bool) alignMode {
	if start {
		return alignStartMode
	}
	return alignEndMode
}

// crossModeFor returns the static cross-axis mode of an align-self value.
func crossModeFor(k Align, crossIsRTL, crossReversed bool) alignMode {
	switch k {
	case AlignStart, AlignBaseline:
		return startEnd(!crossIsRTL)
	case AlignEnd:
		return startEnd(crossIsRTL)
	case AlignCenter:
		return alignCenterMode
	case AlignFlexEnd:
		return startEnd(crossReversed)
	default: // flex-start, stretch
		return startEnd(!crossReversed)
	}
}

// contentKnown returns the known dimensions of the final layout of an
// absolutely positioned box: the axes whose size did not come from the
// content.
func contentKnown(final size, definite boolSize) optSize {
	var k optSize
	if definite.w {
		k.w = some(final.w)
	}
	if definite.h {
		k.h = some(final.h)
	}
	return k
}
