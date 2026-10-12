package layout

// This file defines the layout style: the subset of CSS that the layout
// algorithms read (ADR-039). core maps its ComputedStyle onto it. Box sizing
// is always border-box (ARCHITECTURE section 9): Width, Height and their
// min/max bounds include padding and border.

// Unit is the unit or keyword of a Dim.
type Unit uint8

const (
	// UnitInitial is the zero value: the property's CSS initial value,
	// which is auto for sizes, insets and flex-basis, none for max sizes
	// and 0 for margins, padding, borders and gaps.
	UnitInitial Unit = iota
	// UnitPx is a length in logical pixels.
	UnitPx
	// UnitPercent is a percentage stored as a fraction (0.5 is 50%) of the
	// property's percentage basis.
	UnitPercent
	// UnitAuto is the auto keyword (none for max sizes). Padding, borders
	// and gaps cannot be auto; they treat it as 0.
	UnitAuto
	// UnitMinContent is the min-content keyword (sizes and flex-basis).
	UnitMinContent
	// UnitMaxContent is the max-content keyword (sizes and flex-basis).
	UnitMaxContent
	// UnitFitContent is the fit-content keyword (sizes and flex-basis):
	// the available space clamped between min-content and max-content.
	UnitFitContent
	// UnitFitContentPx is fit-content(Value px).
	UnitFitContentPx
	// UnitFitContentPercent is fit-content(Value), Value a fraction.
	UnitFitContentPercent
	// UnitStretch is the stretch keyword (sizes and flex-basis): the
	// available space minus margins.
	UnitStretch
	// UnitContent is the content keyword of flex-basis; elsewhere it is
	// auto.
	UnitContent
)

// Dim is the value of a sizing or spacing property: a length, a
// percentage or a keyword. The zero Dim is the property's initial value
// (see UnitInitial).
type Dim struct {
	Value float32
	Unit  Unit
}

// Px returns a length of v logical pixels.
func Px(v float32) Dim { return Dim{v, UnitPx} }

// Percent returns a percentage given as a fraction: Percent(0.5) is 50%.
func Percent(f float32) Dim { return Dim{f, UnitPercent} }

// FitContentPx returns fit-content(v px).
func FitContentPx(v float32) Dim { return Dim{v, UnitFitContentPx} }

// FitContentPercent returns fit-content(f), f a fraction.
func FitContentPercent(f float32) Dim { return Dim{f, UnitFitContentPercent} }

// Keyword values.
var (
	Auto       = Dim{Unit: UnitAuto}
	MinContent = Dim{Unit: UnitMinContent}
	MaxContent = Dim{Unit: UnitMaxContent}
	FitContent = Dim{Unit: UnitFitContent}
	Stretch    = Dim{Unit: UnitStretch}
	Content    = Dim{Unit: UnitContent}
)

// Edges holds one Dim per side of a box.
type Edges struct {
	Top, Right, Bottom, Left Dim
}

// EdgesAll returns Edges with d on every side.
func EdgesAll(d Dim) Edges { return Edges{d, d, d, d} }

// Display selects the layout algorithm of a node's children.
type Display uint8

const (
	// DisplayBlock stacks children vertically (block layout without margin
	// collapsing, ADR-006). It is the default, like an HTML div.
	DisplayBlock Display = iota
	// DisplayFlex lays children out with the flexbox algorithm.
	DisplayFlex
	// DisplayNone hides the node and its subtree: they get zero layouts.
	DisplayNone
	// Grid arrives in a later phase.
)

// Position is the CSS position property.
type Position uint8

const (
	// PositionStatic is the default: the node is in flow, its insets are
	// ignored and it is not the containing block of absolutely positioned
	// descendants.
	PositionStatic Position = iota
	// PositionRelative keeps the node in flow and offsets it by its insets
	// after layout; it is the containing block of absolutely positioned
	// descendants.
	PositionRelative
	// PositionAbsolute takes the node out of flow and places it against
	// the padding box of its nearest positioned ancestor (or the layout
	// root), using its insets or, where they are auto, its static
	// position.
	PositionAbsolute
	// fixed and sticky arrive in a later phase.
)

// Overflow is one axis of the CSS overflow property. Layout only uses its
// secondary effects: scroll containers (Hidden, Scroll, Auto) have a zero
// automatic minimum size as flex items and contain the scrollable overflow
// of their content, Scroll reserves ScrollbarWidth for a scrollbar, and
// Clip clips its own axis' overflow.
type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowClip
	OverflowHidden
	OverflowScroll
	// OverflowAuto is a scroll container that reserves no scrollbar space
	// (overlay scrollbars). Browsers with classic scrollbars reserve the
	// gutter only when the content overflows, which needs a second layout
	// pass; Tela does not do that.
	OverflowAuto
)

// isScrollContainer reports whether o makes a box a scroll container.
func (o Overflow) isScrollContainer() bool {
	return o == OverflowHidden || o == OverflowScroll || o == OverflowAuto
}

// Direction is the inline base direction (CSS direction). It is not
// inherited here: core resolves inheritance and sets it on every node.
type Direction uint8

const (
	LTR Direction = iota
	RTL
)

// FlexDirection is the CSS flex-direction property.
type FlexDirection uint8

const (
	Row FlexDirection = iota
	Column
	RowReverse
	ColumnReverse
)

func (d FlexDirection) isRow() bool     { return d == Row || d == RowReverse }
func (d FlexDirection) isReverse() bool { return d == RowReverse || d == ColumnReverse }

// FlexWrap is the CSS flex-wrap property.
type FlexWrap uint8

const (
	NoWrap FlexWrap = iota
	Wrap
	WrapReverse
)

// Align is a value of align-items, align-self, align-content or
// justify-content. AlignSafe may be or-ed with a positional value
// (AlignEnd|AlignSafe is "safe end"): if the item overflows its alignment
// container it is aligned to the start instead.
type Align uint8

const (
	// AlignNormal is the zero value: the property's initial value (normal
	// for align-items, align-content and justify-content; auto, which
	// means the parent's align-items, for align-self). For flex items
	// normal behaves as stretch; for justify-content as flex-start.
	AlignNormal Align = iota
	AlignStart
	AlignEnd
	AlignFlexStart
	AlignFlexEnd
	AlignSelfStart
	AlignSelfEnd
	AlignCenter
	AlignBaseline
	AlignStretch
	AlignSpaceBetween
	AlignSpaceAround
	AlignSpaceEvenly

	// AlignSafe is the safe overflow-position modifier.
	AlignSafe Align = 0x80
)

// keyword returns a without the safe modifier.
func (a Align) keyword() Align { return a &^ AlignSafe }

// isSafe reports whether a has the safe modifier.
func (a Align) isSafe() bool { return a&AlignSafe != 0 }

// Style is the layout style of a node. The zero Style is the CSS initial
// style except for FlexShrink (initial 1, zero here); use DefaultStyle for
// the CSS initial style.
type Style struct {
	Display   Display
	Position  Position
	Direction Direction

	// OverflowX and OverflowY are the two axes of overflow.
	OverflowX, OverflowY Overflow
	// ScrollbarWidth is the space a scrollbar takes in an OverflowScroll
	// axis (a vertical scrollbar takes width). Overlay scrollbars use 0.
	ScrollbarWidth float32

	// Width and Height are the preferred border-box size; MinWidth,
	// MinHeight, MaxWidth and MaxHeight bound it (UnitInitial/UnitAuto
	// mean no bound). Percentages resolve against the containing block.
	Width, Height       Dim
	MinWidth, MinHeight Dim
	MaxWidth, MaxHeight Dim
	// AspectRatio is width divided by height; 0 (or NaN) means none.
	AspectRatio float32

	// Inset holds top, right, bottom and left. They offset relative
	// nodes and place absolute ones; percentages resolve against the
	// containing block's width (left, right) or height (top, bottom).
	Inset Edges
	// Margin may be auto. Percentages of margins, padding and borders
	// resolve against the containing block's width, on all four sides.
	Margin  Edges
	Padding Edges
	Border  Edges

	// Flex container properties. AlignContent also applies to block
	// containers: it aligns their content as a whole (css-align-3).
	FlexDirection FlexDirection
	FlexWrap      FlexWrap
	// RowGap and ColumnGap are the gaps between flex lines and items;
	// percentages resolve against the container's content box.
	RowGap, ColumnGap Dim
	AlignItems        Align
	AlignContent      Align
	JustifyContent    Align

	// Flex item properties.
	AlignSelf  Align
	FlexGrow   float32
	FlexShrink float32
	FlexBasis  Dim
	// Order reorders flex items (stable: equal orders keep tree order).
	Order int32

	// Replaced marks a replaced element (an image): in block layout an
	// auto width takes the measured content size instead of stretching.
	Replaced bool
}

// DefaultStyle returns the CSS initial style: the zero Style with
// FlexShrink 1.
func DefaultStyle() Style { return Style{FlexShrink: 1} }

// aspectRatio returns the style's aspect ratio, if any.
func (s *Style) aspectRatio() opt {
	if r := s.AspectRatio; r > 0 && r < inf {
		return some(r)
	}
	return none
}

func (s *Style) isScrollContainer() bool {
	return s.OverflowX.isScrollContainer() || s.OverflowY.isScrollContainer()
}

// scrollbarGutter returns the space reserved for scrollbars in each axis:
// x is the width taken by a vertical scrollbar (overflow-y: scroll), y the
// height taken by a horizontal one.
func (s *Style) scrollbarGutter() point {
	var g point
	if s.OverflowY == OverflowScroll {
		g.x = s.ScrollbarWidth
	}
	if s.OverflowX == OverflowScroll {
		g.y = s.ScrollbarWidth
	}
	return g
}

func (s *Style) size() dimSize    { return dimSize{s.Width, s.Height} }
func (s *Style) minSize() dimSize { return dimSize{s.MinWidth, s.MinHeight} }
func (s *Style) maxSize() dimSize { return dimSize{s.MaxWidth, s.MaxHeight} }

// gap returns the gaps as a size: width is the column gap (between items
// of a row), height the row gap.
func (s *Style) gap() dimSize { return dimSize{s.ColumnGap, s.RowGap} }

// dimSize is a pair of Dims, one per axis.
type dimSize struct{ w, h Dim }

func (d dimSize) main(dir FlexDirection) Dim {
	if dir.isRow() {
		return d.w
	}
	return d.h
}

func (d dimSize) cross(dir FlexDirection) Dim {
	if dir.isRow() {
		return d.h
	}
	return d.w
}

// isAuto reports whether d is auto for a size property.
func (d Dim) isAuto() bool { return d.Unit == UnitInitial || d.Unit == UnitAuto }

// isStretch reports whether d is the stretch keyword.
func (d Dim) isStretch() bool { return d.Unit == UnitStretch }

// isSizingKeyword reports whether d is one of the intrinsic or stretch
// sizing keywords (not content).
func (d Dim) isSizingKeyword() bool {
	return d.Unit >= UnitMinContent && d.Unit <= UnitStretch
}

// resolveSize resolves a size, min/max size, inset or flex-basis value:
// lengths, and percentages of a known basis. Auto, keywords and
// percentages of an unknown basis give none.
func (d Dim) resolveSize(basis opt) opt {
	switch d.Unit {
	case UnitPx:
		return some(d.Value)
	case UnitPercent:
		if basis.ok {
			return some(float32(basis.v * d.Value))
		}
	}
	return none
}

// resolveSpacing resolves a margin, padding, border or gap value: the
// initial value is 0, auto gives none (only margins can be auto), and a
// percentage of an unknown basis gives none.
func (d Dim) resolveSpacing(basis opt) opt {
	switch d.Unit {
	case UnitInitial:
		return some(0)
	case UnitPx:
		return some(d.Value)
	case UnitPercent:
		if basis.ok {
			return some(float32(basis.v * d.Value))
		}
	}
	return none
}

// resolveOrZero resolves a margin, padding, border or gap value, with auto
// and unresolvable percentages as 0.
func (d Dim) resolveOrZero(basis opt) float32 { return d.resolveSpacing(basis).or0() }

// isAutoMargin reports whether d is an auto margin.
func (d Dim) isAutoMargin() bool { return d.Unit == UnitAuto }

// resolve resolves a dimSize against a basis per axis.
func (d dimSize) resolve(basis optSize) optSize {
	return optSize{d.w.resolveSize(basis.w), d.h.resolveSize(basis.h)}
}

// resolveOrZero resolves spacing edges (margin, padding, border) against
// one basis for all four sides, with auto as 0.
func (e Edges) resolveOrZero(basis opt) rect {
	return rect{
		l: e.Left.resolveOrZero(basis),
		r: e.Right.resolveOrZero(basis),
		t: e.Top.resolveOrZero(basis),
		b: e.Bottom.resolveOrZero(basis),
	}
}

// resolveMargins resolves margins against one basis, keeping auto as none.
func (e Edges) resolveMargins(basis opt) optRect {
	return optRect{
		l: e.Left.resolveSpacing(basis),
		r: e.Right.resolveSpacing(basis),
		t: e.Top.resolveSpacing(basis),
		b: e.Bottom.resolveSpacing(basis),
	}
}

// resolveInsets resolves insets against a basis per axis (auto is none).
func (e Edges) resolveInsets(basis optSize) optRect {
	return optRect{
		l: e.Left.resolveSize(basis.w),
		r: e.Right.resolveSize(basis.w),
		t: e.Top.resolveSize(basis.h),
		b: e.Bottom.resolveSize(basis.h),
	}
}

// autoMargins reports which margins are auto.
func (e Edges) autoMargins() boolRect {
	return boolRect{e.Left.isAutoMargin(), e.Right.isAutoMargin(), e.Top.isAutoMargin(), e.Bottom.isAutoMargin()}
}
