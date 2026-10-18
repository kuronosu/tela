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
	if !bx.kind.IsTextField() {
		state |= style.StateReadOnly // CSS :read-only: every element that cannot be edited
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
	// Transitions and keyframe animations: cs is the target; the style the
	// box uses now may differ from it (anim.go).
	cs = t.animRestyle(el, bx, old, cs)
	bx.cs = cs
	if bx.fld != nil {
		t.fieldRestyled(bx.fld, old, cs)
	}
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
	if displayClass(old) != displayClass(cs) || atomicStyle(old) != atomicStyle(cs) {
		t.markBoxes(el)
		t.markBoxes(sp)
	}
	if inherited || old.TextOverflow != cs.TextOverflow || old.LineClamp != cs.LineClamp {
		t.markBoxes(el) // its paragraph, or the one it is part of
	}
	if bx.inPara != nil && inlineBoxChanged(old, cs) {
		t.markBoxes(el) // the edges of an inline element are laid out
	}
	if old.VerticalAlign != cs.VerticalAlign {
		t.markBoxes(sp) // the inline box of an atomic inline
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
				// The children follow the inherited properties of el,
				// including the ones it animates (anim.go).
				was := t.anim.inherited
				t.anim.inherited = true
				t.restyle(c)
				t.anim.inherited = was
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
	case style.DisplayFlex, style.DisplayGrid:
		return 3 // children are items
	case style.DisplayNone:
		return 4
	case style.DisplayInlineBlock:
		return 5 // atomic inline-level
	case style.DisplayInlineFlex, style.DisplayInlineGrid:
		return 6 // atomic inline-level, children are items
	}
	return 0
}

// atomicStyle reports whether a box with style cs can be an atomic inline
// (atomicInline), as far as its style goes: inline-level and in flow.
func atomicStyle(cs *style.ComputedStyle) bool {
	switch cs.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex, style.DisplayInlineGrid:
		return cs.Position != style.PositionAbsolute && cs.Position != style.PositionFixed
	}
	return false
}

// inlineBoxChanged reports whether the layout of an inline element's box
// changed: the widths of its edges (inlineEdges).
func inlineBoxChanged(a, b *style.ComputedStyle) bool {
	al, ar := inlineEdges(a)
	bl, br := inlineEdges(b)
	return al != bl || ar != br
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
		keepGrid(&ls, &t.rootBox.ls)
		if ls != t.rootBox.ls {
			t.rootBox.ls = ls
			t.engine.MarkDirty(t.rootBox)
		}
	}
}

// remap maps the layout styles of p's children.
func (t *Tree) remap(p *Box) {
	last := spaceLast(p)
	for _, k := range p.kids {
		ls := mapLayoutStyle(k, p, hasSpaceAfter(k, last))
		keepGrid(&ls, &k.ls)
		if ls != k.ls {
			k.ls = ls
			t.engine.MarkDirty(k)
		}
	}
}

func inFlow(cs *style.ComputedStyle) bool {
	return cs.Display != style.DisplayNone && cs.Position != style.PositionAbsolute && cs.Position != style.PositionFixed
}

// spaceLast returns the element that is the last child of container p in
// the DOM's sense, when p has space-x-* or space-y-* (nil otherwise):
// Tailwind v4 gives the space to every child but the last
// (:where(& > :not(:last-child))), also when the last child is
// display: none or absolutely positioned and the one before it is not.
// Components and display: contents elements have no node of their own:
// the last element below them counts.
func spaceLast(p *Box) *Element {
	if p.el == nil || p.cs == nil || p.cs.SpaceX == (style.LP{}) && p.cs.SpaceY == (style.LP{}) {
		return nil
	}
	return lastChildElement(p.el)
}

func lastChildElement(el *Element) *Element {
	for i := len(el.children) - 1; i >= 0; i-- {
		c := el.children[i]
		switch c.kind {
		case elemComponent:
			if l := lastChildElement(c); l != nil {
				return l
			}
		case elemPrimitive:
			if cs := c.box.cs; cs != nil && cs.Display == style.DisplayContents {
				if l := lastChildElement(c); l != nil {
					return l
				}
				continue
			}
			return c
		}
	}
	return nil
}

// hasSpaceAfter reports whether box k of a container whose last child is
// last takes the container's space-x-* and space-y-* margins. Anonymous
// boxes (runs of text) are not elements and take none.
func hasSpaceAfter(k *Box, last *Element) bool {
	return last != nil && k.el != nil && k.el != last
}

// applySpace gives ls the space-x-* and space-y-* of a container whose
// style is pcs as margins on the inline-end and block-end sides. They come
// from a rule without specificity in Tailwind v4, so a margin of the child
// on that side wins; an explicit zero margin cannot be told from none.
func applySpace(ls *layout.Style, pcs *style.ComputedStyle) {
	if pcs.SpaceY != (style.LP{}) && isZeroPx(ls.Margin.Bottom) {
		ls.Margin.Bottom = lpDim(pcs.SpaceY)
	}
	if pcs.SpaceX != (style.LP{}) && isZeroPx(ls.Margin.Right) {
		ls.Margin.Right = lpDim(pcs.SpaceX)
	}
}

// mapLayoutStyle maps the computed style of box b, a child of parent (nil
// for the root), onto layout's style. spaceAfter reports a child that
// takes the parent's space-x/space-y margins (hasSpaceAfter).
//
// Approximations (until layout and the later phases support them):
//
//   - fixed lays out as absolute against the nearest positioned ancestor,
//     sticky as relative;
//   - in flow, inline-block, inline-flex and inline-grid boxes are the
//     atomic inlines of a paragraph (ADR-066), with a fit-content width;
//     the other inline-level boxes that a block lays out (absolutely
//     positioned ones) are fit-content too, and the text-align of the
//     parent is emulated with auto margins when theirs are zero.
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
	case style.DisplayGrid, style.DisplayInlineGrid:
		ls.Display = layout.DisplayGrid
		ls.Grid = gridTemplate(cs)
		ls.GridAutoFlow = layout.GridAutoFlow(cs.GridAutoFlow)
		ls.JustifyItems = alignOf(cs.JustifyItems)
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
	if b.kind == style.KindInput {
		// An Input clips its line itself and keeps its text's baseline
		// (field.go): for layout its overflow is visible.
		ls.OverflowX, ls.OverflowY = layout.OverflowVisible, layout.OverflowVisible
	}
	if b.tree.classicBars(cs) && isScrollContainer(cs) && b.kind != style.KindInput {
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
	ls.JustifySelf = alignOf(cs.JustifySelf)
	ls.GridColumn, ls.GridRow = gridPlacement(cs.GridColumn), gridPlacement(cs.GridRow)
	if v := b.virtMark(); v != nil {
		v.layoutStyle(&ls) // the boxes of a VirtualList (virtual.go)
	}

	if parent == nil {
		return ls
	}
	if itemContainer(parent.cs) {
		if spaceAfter {
			applySpace(&ls, parent.cs)
		}
		return ls // a flex or grid item: blockified
	}
	if parent.mode == modeParagraph {
		// An atomic inline of a paragraph: its line places it (text-align
		// and space-x-* included); an auto width is fit-content.
		if ls.Width.Unit == layout.UnitAuto && b.kind != style.KindImg {
			ls.Width = layout.FitContent
		}
		return ls
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
	if spaceAfter {
		applySpace(&ls, pcs)
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

// keepGrid gives ls the grid template pointer of old when the templates
// are equal: layout compares templates by pointer (layout.Style.Grid), and
// mapLayoutStyle builds a new one each time.
func keepGrid(ls, old *layout.Style) {
	if ls.Grid != nil && old.Grid != nil && ls.Grid != old.Grid && ls.Grid.Equal(old.Grid) {
		ls.Grid = old.Grid
	}
}

// gridTemplate maps the track lists of a grid container (nil when it has
// none: implicit auto tracks only).
func gridTemplate(cs *style.ComputedStyle) *layout.GridTemplate {
	if cs.GridTemplateColumns.IsNone() && cs.GridTemplateRows.IsNone() && len(cs.GridAutoColumns) == 0 && len(cs.GridAutoRows) == 0 {
		return nil
	}
	return &layout.GridTemplate{
		Columns:     trackList(cs.GridTemplateColumns),
		Rows:        trackList(cs.GridTemplateRows),
		AutoColumns: trackSizes(cs.GridAutoColumns),
		AutoRows:    trackSizes(cs.GridAutoRows),
	}
}

func trackList(l style.TrackList) layout.TrackList {
	if l.IsNone() {
		return layout.TrackList{}
	}
	out := layout.TrackList{Entries: make([]layout.TrackListEntry, len(l.Entries)), LineNames: l.LineNames}
	for i, e := range l.Entries {
		le := layout.TrackListEntry{Tracks: trackSizes(e.Tracks), LineNames: e.LineNames}
		if e.Repeat {
			switch e.Kind {
			case style.RepeatAutoFill:
				le.Repeat = layout.RepeatAutoFill
			case style.RepeatAutoFit:
				le.Repeat = layout.RepeatAutoFit
			default:
				le.Repeat, le.Count = layout.RepeatCount, e.Count
			}
		}
		out.Entries[i] = le
	}
	return out
}

func trackSizes(ts []style.TrackSize) []layout.TrackSize {
	if len(ts) == 0 {
		return nil
	}
	out := make([]layout.TrackSize, len(ts))
	for i, t := range ts {
		out[i] = layout.TrackSize{Min: breadth(t.Min), Max: breadth(t.Max)}
	}
	return out
}

// breadth maps a track breadth onto a layout Dim.
func breadth(b style.TrackBreadth) layout.Dim {
	switch b.Kind {
	case style.BreadthLength:
		return lpDim(b.LP)
	case style.BreadthFr:
		return layout.Fr(b.Fr)
	case style.BreadthMinContent:
		return layout.MinContent
	case style.BreadthMaxContent:
		return layout.MaxContent
	case style.BreadthFitContent:
		if b.LP.Pct == 0 {
			return layout.FitContentPx(b.LP.Px)
		}
		return layout.Dim{Unit: layout.UnitFitContentPercent, Value: b.LP.Pct / 100, Px: b.LP.Px}
	}
	return layout.Auto
}

func gridPlacement(p style.GridPlacement) layout.GridPlacement {
	return layout.GridPlacement{Start: gridLine(p.Start), End: gridLine(p.End)}
}

func gridLine(l style.GridLine) layout.GridLine {
	switch l.Kind {
	case style.GridLineIndex:
		return layout.GridLine{Kind: layout.GridLineNumber, N: l.N}
	case style.GridLineSpan:
		return layout.GridLine{Kind: layout.GridSpan, N: l.N}
	case style.GridLineNamed:
		return layout.GridLine{Kind: layout.GridNamedLine, N: l.N, Name: l.Name}
	case style.GridLineNamedSpan:
		return layout.GridLine{Kind: layout.GridNamedSpan, N: l.N, Name: l.Name}
	}
	return layout.GridLine{}
}
