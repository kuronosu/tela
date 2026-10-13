package core

import (
	"errors"
	"slices"

	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// layout0 is the CSS initial layout style.
var layout0 = layout.DefaultStyle()

// structuralStates are the states of the structural variants.
const structuralStates = style.StateFirst | style.StateLast | style.StateOnly | style.StateOdd | style.StateEven

// ---------------------------------------------------------------------
// Style phase.

// stylePhase computes the style of the queued primitives, shallowest
// first. A change of inherited properties restyles the children
// (ARCHITECTURE section 10: no cascade, only inheritance).
func (t *Tree) stylePhase() {
	for len(t.styleQ) > 0 {
		q := t.styleQ
		t.styleQ = nil
		sortByDepth(q)
		for _, el := range q {
			if el.mounted && el.needsStyle {
				t.restyle(el)
			}
		}
	}
}

// restyle computes the style of primitive el.
func (t *Tree) restyle(el *Element) {
	el.needsStyle = false
	bx := el.box
	t.stats.Styles++
	if bx.classesDirty {
		t.compileClasses(el)
	}
	sp := el.styleParent()
	var parent *style.ComputedStyle
	if sp != nil {
		parent = sp.box.cs
	}
	state := el.istate
	if bx.structural {
		state |= t.structuralState(el)
	}
	var group, peer style.StateFlags
	if bx.groupUse != 0 {
		group = groupState(el)
	}
	if bx.peerUse != 0 {
		peer = peerState(el)
	}
	cs := style.Compute(&t.cx, style.Input{
		Kind: bx.kind, Parent: parent, State: state, Group: group, Peer: peer,
		Classes: bx.classes, Props: el.node.d.props,
	})
	if el == t.root {
		// The root is the viewport (scroll.go).
		cs.OverflowX, cs.OverflowY = viewportOverflow(cs.OverflowX), viewportOverflow(cs.OverflowY)
	}
	old := bx.cs
	bx.cs = cs
	bx.deferLayout, bx.deferPaint = hasPhase(cs, style.PhaseLayout), hasPhase(cs, style.PhasePaint)
	t.updateScroller(bx)
	// Values read by deferred props the style no longer has stop
	// invalidating the box; the others are tracked again when the props
	// are evaluated (remap, paint).
	if !bx.deferLayout {
		dropObserver(bx.layoutObs)
	}
	if !bx.deferPaint {
		dropObserver(bx.paintObs)
	}
	if !bx.restyled {
		bx.restyled = true
		t.restyled = append(t.restyled, bx)
	}
	t.dirty()
	if old == nil {
		return // a new element: reconciliation marked its structure
	}
	inherited := !sameInherited(old, cs)
	if displayClass(old) != displayClass(cs) {
		t.markBoxes(el)
		t.markBoxes(sp)
	}
	if inherited || old.TextOverflow != cs.TextOverflow || old.LineClamp != cs.LineClamp {
		t.markBoxes(el) // its paragraph, or the one it is part of
	}
	if inherited {
		t.restyleChildren(el)
	}
}

// restyleChildren restyles the primitives whose style parent is el.
func (t *Tree) restyleChildren(el *Element) {
	for _, c := range el.children {
		switch c.kind {
		case elemPrimitive:
			if c.mounted {
				t.restyle(c)
			}
		case elemComponent:
			t.restyleChildren(c)
		}
	}
}

// compileClasses compiles the Class props of el with the tree's parser
// (interned), reporting class errors once per class string.
func (t *Tree) compileClasses(el *Element) {
	bx := el.box
	bx.classesDirty = false
	srcs := el.node.d.classes
	bx.classes = make([]*style.Classes, len(srcs))
	bx.structural = false
	bx.stateUse, bx.groupUse, bx.peerUse = 0, 0, 0
	bx.isGroup, bx.isPeer = false, false
	for i, s := range srcs {
		c := t.parser.Compile(s)
		bx.classes[i] = c
		if c.Err != nil {
			t.reportClasses(s, c.Err, el)
		}
		bx.isGroup = bx.isGroup || c.Group
		bx.isPeer = bx.isPeer || c.Peer
		for j := range c.Rules {
			cond := &c.Rules[j].Cond
			if cond.State&structuralStates != 0 {
				bx.structural = true
			}
			bx.stateUse |= cond.State &^ structuralStates
			if cond.Enabled {
				bx.stateUse |= style.StateDisabled
			}
			bx.groupUse |= cond.Group
			bx.peerUse |= cond.Peer
		}
	}
	if bx.groupUse != 0 {
		for a := el.parent; a != nil && !a.groupKids; a = a.parent {
			a.groupKids = true
		}
	}
	if bx.structural {
		if sp := el.styleParent(); sp != nil {
			sp.box.structuralKids = true
		}
	}
}

// reportClasses reports the errors of a class string to the dev log, once
// per distinct string (ARCHITECTURE section 3: errors that teach).
func (t *Tree) reportClasses(src string, err error, el *Element) {
	if t.reported[src] {
		return
	}
	t.reported[src] = true
	var errs style.ClassErrors
	if errors.As(err, &errs) && len(errs) == 1 {
		err = errs[0]
	}
	t.logf("tela: %v (in Class(%q) of a <%v>)", err, src, el.box.kind)
}

// structuralState returns the structural states of el (first:, last:,
// only:, odd:, even:) among the primitive children of its style parent.
func (t *Tree) structuralState(el *Element) style.StateFlags {
	sp := el.styleParent()
	if sp == nil {
		return style.StateFirst | style.StateLast | style.StateOnly | style.StateOdd
	}
	if sp.box.sibFrame != t.frame {
		n := int32(0)
		var walk func(p *Element)
		walk = func(p *Element) {
			for _, c := range p.children {
				switch c.kind {
				case elemPrimitive:
					c.box.sibIndex = n
					n++
				case elemComponent:
					walk(c)
				}
			}
		}
		walk(sp)
		sp.box.sibFrame, sp.box.sibCount = t.frame, n
	}
	i, n := el.box.sibIndex, sp.box.sibCount
	var s style.StateFlags
	if i == 0 {
		s |= style.StateFirst
	}
	if i == n-1 {
		s |= style.StateLast
	}
	if n == 1 {
		s |= style.StateOnly
	}
	if i%2 == 0 {
		s |= style.StateOdd // 1-based: the first child is odd
	} else {
		s |= style.StateEven
	}
	return s
}

// sameInherited reports whether two styles have the same inherited
// properties (ADR-042).
func sameInherited(a, b *style.ComputedStyle) bool {
	ta, tb := &a.Text, &b.Text
	return ta.Color == tb.Color && slices.Equal(ta.FontFamily, tb.FontFamily) &&
		ta.FontSize == tb.FontSize && ta.FontWeight == tb.FontWeight &&
		ta.FontStyle == tb.FontStyle && ta.FontStretch == tb.FontStretch &&
		ta.LineHeight == tb.LineHeight && ta.LetterSpacing == tb.LetterSpacing &&
		ta.WordSpacing == tb.WordSpacing && ta.TextAlign == tb.TextAlign &&
		ta.TextTransform == tb.TextTransform && ta.Decoration == tb.Decoration &&
		ta.WhiteSpace == tb.WhiteSpace && ta.WordBreak == tb.WordBreak &&
		ta.OverflowWrap == tb.OverflowWrap && ta.TabSize == tb.TabSize &&
		ta.Numeric == tb.Numeric && slices.Equal(ta.Shadow, tb.Shadow) &&
		a.Visibility == b.Visibility && a.Cursor == b.Cursor &&
		a.PointerEvents == b.PointerEvents && a.UserSelect == b.UserSelect
}

// displayClass groups display values by their effect on the box
// structure.
func displayClass(cs *style.ComputedStyle) uint8 {
	switch cs.Display {
	case style.DisplayContents:
		return 1
	case style.DisplayInline:
		return 2
	case style.DisplayFlex, style.DisplayInlineFlex:
		return 3
	case style.DisplayNone:
		return 4
	}
	return 0
}

// ---------------------------------------------------------------------
// ComputedStyle to layout.Style.

// remapPhase maps the layout styles of the boxes whose style or
// structure changed, and of their siblings and children (which depend on
// the parent's display, space-* and text-align).
func (t *Tree) remapPhase() {
	for _, b := range t.restyled {
		b.restyled = false
		b.queueRemap()
	}
	clear(t.restyled)
	t.restyled = t.restyled[:0]
	for i := 0; i < len(t.remapQ); i++ {
		b := t.remapQ[i]
		b.remapQueued = false
		t.remap(b)
	}
	clear(t.remapQ)
	t.remapQ = t.remapQ[:0]
	// Boxes whose layout-phase deferred props read a Value that changed
	// (a box remapped above is evaluated twice, with the same result).
	for _, b := range t.relayoutQ {
		b.relayoutQueued = false
		t.remapOne(b)
	}
	clear(t.relayoutQ)
	t.relayoutQ = t.relayoutQ[:0]
	if t.rootBox != nil && t.rootBox.cs != nil {
		ls := mapLayoutStyle(t.rootBox, nil, false)
		ls.Width, ls.Height = layout.Px(t.viewport.W), layout.Px(t.viewport.H)
		if ls != t.rootBox.ls {
			t.rootBox.ls = ls
			t.engine.MarkDirty(t.rootBox)
		}
	}
}

// remap maps the layout styles of p's children.
func (t *Tree) remap(p *Box) {
	last := -1
	for i := len(p.kids) - 1; i >= 0; i-- {
		if inFlow(p.kids[i].cs) {
			last = i
			break
		}
	}
	for i, k := range p.kids {
		ls := mapLayoutStyle(k, p, i < last && inFlow(k.cs))
		if ls != k.ls {
			k.ls = ls
			t.engine.MarkDirty(k)
		}
	}
}

func inFlow(cs *style.ComputedStyle) bool {
	return cs.Display != style.DisplayNone && cs.Position != style.PositionAbsolute && cs.Position != style.PositionFixed
}

// mapLayoutStyle maps the computed style of box b, a child of parent (nil
// for the root), onto layout's style. spaceAfter reports an in-flow child
// followed by another, which takes the parent's space-x/space-y margins.
//
// Approximations (until layout and the later phases support them):
//
//   - grid and inline-grid lay out as block (grid arrives in phase 6);
//   - fixed lays out as absolute against the nearest positioned ancestor,
//     sticky as relative;
//   - inline-block, inline-flex and inline-grid boxes in block flow are
//     block-level boxes with a fit-content width, each on its own line;
//     the text-align of the parent is emulated with auto margins when
//     theirs are zero.
func mapLayoutStyle(b *Box, parent *Box, spaceAfter bool) layout.Style {
	cs := b.cs
	if b.deferLayout {
		// The layout-phase deferred props: evaluated when the style or
		// the structure changes, and when a Value they read changes
		// (Tree.remapOne).
		cs = b.tree.resolveDeferred(b, style.PhaseLayout)
	}
	ls := layout0

	switch cs.Display {
	case style.DisplayFlex, style.DisplayInlineFlex:
		ls.Display = layout.DisplayFlex
	case style.DisplayNone:
		ls.Display = layout.DisplayNone
	default:
		ls.Display = layout.DisplayBlock
	}
	switch cs.Position {
	case style.PositionRelative, style.PositionSticky:
		ls.Position = layout.PositionRelative
	case style.PositionAbsolute, style.PositionFixed:
		ls.Position = layout.PositionAbsolute
	}
	ls.OverflowX, ls.OverflowY = overflowPair(cs.OverflowX, cs.OverflowY)
	if b.tree.classicBars(cs) && isScrollContainer(cs) {
		// Classic scrollbars take space (scrollbar.go): overflow: scroll
		// always, overflow: auto while its content overflows.
		ls.ScrollbarWidth = classicGutter
		if s := b.scr; s != nil {
			if ls.OverflowY == layout.OverflowAuto && s.gutterV {
				ls.OverflowY = layout.OverflowScroll
			}
			if ls.OverflowX == layout.OverflowAuto && s.gutterH {
				ls.OverflowX = layout.OverflowScroll
			}
		}
	}

	ls.Width, ls.Height = dimOf(cs.Width), dimOf(cs.Height)
	ls.MinWidth, ls.MinHeight = dimOf(cs.MinWidth), dimOf(cs.MinHeight)
	ls.MaxWidth, ls.MaxHeight = dimOf(cs.MaxWidth), dimOf(cs.MaxHeight)
	ls.AspectRatio = cs.AspectRatio
	if b.kind == style.KindImg {
		ls.Replaced = true
		if ls.AspectRatio == 0 && b.img != nil && b.img.Width() > 0 && b.img.Height() > 0 {
			ls.AspectRatio = float32(b.img.Width()) / float32(b.img.Height())
		}
	}
	ls.Inset = edgesOf(cs.Inset)
	ls.Margin = edgesOf(cs.Margin)
	ls.Padding = layout.Edges{Top: lpDim(cs.Padding.Top), Right: lpDim(cs.Padding.Right),
		Bottom: lpDim(cs.Padding.Bottom), Left: lpDim(cs.Padding.Left)}
	ls.Border = layout.Edges{Top: layout.Px(cs.Border.Top.Width), Right: layout.Px(cs.Border.Right.Width),
		Bottom: layout.Px(cs.Border.Bottom.Width), Left: layout.Px(cs.Border.Left.Width)}

	ls.FlexDirection = flexDirection(cs.FlexDirection)
	switch cs.FlexWrap {
	case style.FlexWrapOn:
		ls.FlexWrap = layout.Wrap
	case style.FlexWrapReverse:
		ls.FlexWrap = layout.WrapReverse
	}
	ls.RowGap, ls.ColumnGap = lpDim(cs.RowGap), lpDim(cs.ColumnGap)
	ls.AlignItems = alignOf(cs.AlignItems)
	ls.AlignContent = alignOf(cs.AlignContent)
	ls.JustifyContent = alignOf(cs.JustifyContent)
	ls.AlignSelf = alignOf(cs.AlignSelf)
	ls.FlexGrow, ls.FlexShrink = cs.FlexGrow, cs.FlexShrink
	ls.FlexBasis = dimOf(cs.FlexBasis)
	ls.Order = cs.Order

	if parent == nil || isFlexContainer(parent.cs) {
		return ls // the root, or a flex item: blockified
	}
	pcs := parent.cs
	// Block flow.
	switch cs.Display {
	case style.DisplayInlineBlock, style.DisplayInlineFlex, style.DisplayInlineGrid:
		if ls.Width.Unit == layout.UnitAuto {
			ls.Width = layout.FitContent
		}
		if inFlow(cs) && isZeroPx(ls.Margin.Left) && isZeroPx(ls.Margin.Right) {
			switch pcs.Text.TextAlign {
			case text.AlignCenter:
				ls.Margin.Left, ls.Margin.Right = layout.Auto, layout.Auto
			case text.AlignRight, text.AlignEnd:
				ls.Margin.Left = layout.Auto
			}
		}
	}
	// space-y-* and space-x-*: the gap as margins between in-flow
	// children (block layout has no gap; Tailwind uses margins too).
	if spaceAfter {
		if pcs.SpaceY {
			ls.Margin.Bottom = addLP(ls.Margin.Bottom, pcs.RowGap)
		}
		if pcs.SpaceX {
			ls.Margin.Right = addLP(ls.Margin.Right, pcs.ColumnGap)
		}
	}
	return ls
}

// overflowPair applies CSS's computed-value rule: when one axis is
// neither visible nor clip, visible becomes auto and clip becomes hidden.
func overflowPair(x, y style.Overflow) (layout.Overflow, layout.Overflow) {
	scrolls := func(o style.Overflow) bool { return o != style.OverflowVisible && o != style.OverflowClip }
	if scrolls(x) || scrolls(y) {
		fix := func(o style.Overflow) style.Overflow {
			switch o {
			case style.OverflowVisible:
				return style.OverflowAuto
			case style.OverflowClip:
				return style.OverflowHidden
			}
			return o
		}
		x, y = fix(x), fix(y)
	}
	return overflowOf(x), overflowOf(y)
}

func overflowOf(o style.Overflow) layout.Overflow {
	switch o {
	case style.OverflowHidden:
		return layout.OverflowHidden
	case style.OverflowClip:
		return layout.OverflowClip
	case style.OverflowScroll:
		return layout.OverflowScroll
	case style.OverflowAuto:
		return layout.OverflowAuto
	}
	return layout.OverflowVisible
}

// dimOf maps a size, inset, margin or flex-basis value.
func dimOf(d style.Dim) layout.Dim {
	switch d.Kind {
	case style.DimLength:
		return lpDim(d.LP)
	case style.DimMinContent:
		return layout.MinContent
	case style.DimMaxContent:
		return layout.MaxContent
	case style.DimFitContent:
		return layout.FitContent
	}
	return layout.Auto
}

// lpDim maps a length-percentage: a length, a percentage or, with both
// parts (calc(100% - 2rem)), a layout calc value.
func lpDim(lp style.LP) layout.Dim {
	return layout.Calc(lp.Px, lp.Pct/100)
}

func edgesOf(s style.Sides[style.Dim]) layout.Edges {
	return layout.Edges{Top: dimOf(s.Top), Right: dimOf(s.Right), Bottom: dimOf(s.Bottom), Left: dimOf(s.Left)}
}

// addLP adds a gap to a margin (space-x/space-y): lengths and percentages
// sum into a calc value; an auto margin stays auto.
func addLP(m layout.Dim, gap style.LP) layout.Dim {
	px, f := gap.Px, gap.Pct/100
	switch m.Unit {
	case layout.UnitInitial:
	case layout.UnitPx:
		px += m.Value
	case layout.UnitPercent:
		f += m.Value
	case layout.UnitCalc:
		px, f = px+m.Px, f+m.Value
	default:
		return m
	}
	return layout.Calc(px, f)
}

func isZeroPx(d layout.Dim) bool {
	return d.Unit == layout.UnitInitial || d.Unit == layout.UnitPx && d.Value == 0
}

func flexDirection(d style.FlexDirection) layout.FlexDirection {
	switch d {
	case style.FlexRowReverse:
		return layout.RowReverse
	case style.FlexColumn:
		return layout.Column
	case style.FlexColumnReverse:
		return layout.ColumnReverse
	}
	return layout.Row
}

func alignOf(a style.Align) layout.Align {
	switch a {
	case style.AlignStart:
		return layout.AlignStart
	case style.AlignEnd:
		return layout.AlignEnd
	case style.AlignFlexStart:
		return layout.AlignFlexStart
	case style.AlignFlexEnd:
		return layout.AlignFlexEnd
	case style.AlignCenter:
		return layout.AlignCenter
	case style.AlignStretch:
		return layout.AlignStretch
	case style.AlignBaseline:
		return layout.AlignBaseline
	case style.AlignSpaceBetween:
		return layout.AlignSpaceBetween
	case style.AlignSpaceAround:
		return layout.AlignSpaceAround
	case style.AlignSpaceEvenly:
		return layout.AlignSpaceEvenly
	}
	return layout.AlignNormal
}
