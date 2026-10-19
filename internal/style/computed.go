package style

import (
	"math"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// Display is the CSS display property, reduced to the values Tela lays out.
// The zero value is block, the default of a Div (ARCHITECTURE section 9).
type Display uint8

const (
	DisplayBlock Display = iota
	DisplayFlex
	DisplayGrid
	// DisplayInline is the display of Span and Text: they only exist in a
	// text context (ARCHITECTURE section 9).
	DisplayInline
	// DisplayInlineBlock is the display of Button: in a block container
	// the box is an atomic inline of its paragraph, with its fit-content
	// width; as a flex or grid item it is blockified like any item.
	DisplayInlineBlock
	DisplayInlineFlex
	DisplayInlineGrid
	// DisplayContents removes the box and lifts its children into the
	// parent. Layout may not support it yet.
	DisplayContents
	DisplayNone
)

var displayNames = [...]string{"block", "flex", "grid", "inline", "inline-block", "inline-flex", "inline-grid", "contents", "none"}

func (d Display) String() string { return enumName(displayNames[:], int(d)) }

// Position is the CSS position property.
type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

var positionNames = [...]string{"static", "relative", "absolute", "fixed", "sticky"}

func (p Position) String() string { return enumName(positionNames[:], int(p)) }

// Overflow is the CSS overflow property of one axis.
type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowHidden
	OverflowClip
	OverflowScroll
	OverflowAuto
)

var overflowNames = [...]string{"visible", "hidden", "clip", "scroll", "auto"}

func (o Overflow) String() string { return enumName(overflowNames[:], int(o)) }

// Clips reports whether content outside the padding box is clipped.
func (o Overflow) Clips() bool { return o != OverflowVisible }

// Visibility is the CSS visibility property (inherited).
type Visibility uint8

const (
	VisibilityVisible Visibility = iota
	VisibilityHidden
	VisibilityCollapse
)

// FlexDirection is the CSS flex-direction property.
type FlexDirection uint8

const (
	FlexRow FlexDirection = iota
	FlexRowReverse
	FlexColumn
	FlexColumnReverse
)

var flexDirNames = [...]string{"row", "row-reverse", "column", "column-reverse"}

func (d FlexDirection) String() string { return enumName(flexDirNames[:], int(d)) }

// IsColumn reports whether the main axis is vertical.
func (d FlexDirection) IsColumn() bool { return d == FlexColumn || d == FlexColumnReverse }

// FlexWrap is the CSS flex-wrap property.
type FlexWrap uint8

const (
	FlexNoWrap FlexWrap = iota
	FlexWrapOn
	FlexWrapReverse
)

// Align is the value of align-items, align-self, align-content,
// justify-content, justify-items and justify-self. The zero value is the
// CSS keyword normal (auto for the -self properties), which layout treats
// as stretch for items and as start for content.
type Align uint8

const (
	AlignAuto Align = iota
	AlignStart
	AlignEnd
	AlignFlexStart
	AlignFlexEnd
	AlignCenter
	AlignStretch
	AlignBaseline
	AlignSpaceBetween
	AlignSpaceAround
	AlignSpaceEvenly
)

var alignNames = [...]string{"auto", "start", "end", "flex-start", "flex-end", "center", "stretch", "baseline", "space-between", "space-around", "space-evenly"}

func (a Align) String() string { return enumName(alignNames[:], int(a)) }

// BorderStyle is the CSS border-style and outline-style property.
type BorderStyle uint8

const (
	BorderNone BorderStyle = iota
	BorderSolid
	BorderDashed
	BorderDotted
	BorderDouble
	BorderHidden
)

var borderStyleNames = [...]string{"none", "solid", "dashed", "dotted", "double", "hidden"}

func (b BorderStyle) String() string { return enumName(borderStyleNames[:], int(b)) }

// Border is one side of the border. Width is the used width: zero when the
// style is none or hidden.
type Border struct {
	Width float32
	Style BorderStyle
	Color gfx.Color
}

// Outline is the CSS outline (drawn outside the border box, not part of
// layout).
type Outline struct {
	Width  float32
	Offset float32
	Style  BorderStyle
	Color  gfx.Color
}

// Cursor is the CSS cursor property (inherited). The names are the
// Tailwind cursor-* values.
type Cursor uint8

const (
	CursorAuto Cursor = iota
	CursorDefault
	CursorPointer
	CursorWait
	CursorText
	CursorMove
	CursorHelp
	CursorNotAllowed
	CursorNone
	CursorContextMenu
	CursorProgress
	CursorCell
	CursorCrosshair
	CursorVerticalText
	CursorAlias
	CursorCopy
	CursorNoDrop
	CursorGrab
	CursorGrabbing
	CursorAllScroll
	CursorColResize
	CursorRowResize
	CursorNResize
	CursorEResize
	CursorSResize
	CursorWResize
	CursorNEResize
	CursorNWResize
	CursorSEResize
	CursorSWResize
	CursorEWResize
	CursorNSResize
	CursorNESWResize
	CursorNWSEResize
	CursorZoomIn
	CursorZoomOut
)

var cursorNames = [...]string{
	"auto", "default", "pointer", "wait", "text", "move", "help", "not-allowed", "none",
	"context-menu", "progress", "cell", "crosshair", "vertical-text", "alias", "copy",
	"no-drop", "grab", "grabbing", "all-scroll", "col-resize", "row-resize", "n-resize",
	"e-resize", "s-resize", "w-resize", "ne-resize", "nw-resize", "se-resize", "sw-resize",
	"ew-resize", "ns-resize", "nesw-resize", "nwse-resize", "zoom-in", "zoom-out",
}

func (c Cursor) String() string { return enumName(cursorNames[:], int(c)) }

// PointerEvents is the CSS pointer-events property (inherited).
type PointerEvents uint8

const (
	PointerEventsAuto PointerEvents = iota
	PointerEventsNone
)

// UserSelect is the CSS user-select property (inherited here, as it
// effectively is in browsers).
type UserSelect uint8

const (
	UserSelectAuto UserSelect = iota
	UserSelectNone
	UserSelectText
	UserSelectAll
)

// ScrollBehavior is how a scroll container animates (the CSS
// scroll-behavior property, plus Tela's instant). Not inherited.
type ScrollBehavior uint8

const (
	// ScrollBehaviorAuto (scroll-auto, the default): programmatic scrolls
	// jump; user scrolling (wheel notches, keys) animates unless the app
	// set InstantScroll.
	ScrollBehaviorAuto ScrollBehavior = iota
	// ScrollBehaviorSmooth (scroll-smooth): programmatic scrolls animate,
	// and user scrolling animates even when the app set InstantScroll.
	ScrollBehaviorSmooth
	// ScrollBehaviorInstant (scroll-instant, Tela-specific): nothing
	// animates and touchpad or touch scrolling never flings.
	ScrollBehaviorInstant
)

var scrollBehaviorNames = [...]string{"auto", "smooth", "instant"}

func (b ScrollBehavior) String() string { return enumName(scrollBehaviorNames[:], int(b)) }

// ScrollbarMode is the look of a scroll container's scrollbars
// (Tela-specific; CSS leaves it to the platform). Not inherited.
type ScrollbarMode uint8

const (
	// ScrollbarDefault follows the app (tela.App.ClassicScrollbars).
	ScrollbarDefault ScrollbarMode = iota
	// ScrollbarClassic (scrollbar-classic): always-visible bars in a
	// reserved gutter.
	ScrollbarClassic
	// ScrollbarOverlay (scrollbar-overlay): thin bars over the content,
	// shown while scrolling.
	ScrollbarOverlay
)

var scrollbarModeNames = [...]string{"default", "classic", "overlay"}

func (m ScrollbarMode) String() string { return enumName(scrollbarModeNames[:], int(m)) }

// Overscroll is one axis of the CSS overscroll-behavior property: whether
// a scroll that reaches the edge of this scroll container chains to its
// ancestors. Not inherited.
type Overscroll uint8

const (
	// OverscrollAuto chains to the ancestors.
	OverscrollAuto Overscroll = iota
	// OverscrollContain stops the chain at this container.
	OverscrollContain
	// OverscrollNone stops the chain too (Tela draws no overscroll
	// effect, so it is contain).
	OverscrollNone
)

var overscrollNames = [...]string{"auto", "contain", "none"}

func (o Overscroll) String() string { return enumName(overscrollNames[:], int(o)) }

// ZIndex is the CSS z-index. The zero value is auto.
type ZIndex struct {
	Explicit bool
	Value    int32
}

// FlexGrow and FlexShrink are plain float32 fields on ComputedStyle.

// DecorationStyle is the CSS text-decoration-style property.
type DecorationStyle uint8

const (
	DecorationSolid DecorationStyle = iota
	DecorationDouble
	DecorationDotted
	DecorationDashed
	DecorationWavy
)

// Decoration is the text decoration. Color is the resolved decoration
// color (the color of the element that declared the decoration, unless
// decoration-* set one).
type Decoration struct {
	Lines text.Decoration
	Style DecorationStyle
	Color gfx.Color
	// Thickness is the line thickness in pixels; zero is auto (the
	// font's own).
	Thickness float32
	// Offset is the distance of an underline from its default position
	// in pixels; it applies when OffsetSet is true.
	Offset    float32
	OffsetSet bool
}

// TextProps holds the inherited text properties (ARCHITECTURE sections 10
// and 11). A child starts with a copy of its parent's.
type TextProps struct {
	// Color is the text color; every currentColor resolves to it.
	Color gfx.Color
	// FontFamily is the family list: names and CSS generics.
	FontFamily []string
	// FontSize in logical pixels.
	FontSize   float32
	FontWeight float32
	FontStyle  text.Style
	// FontStretch is a percentage; 100 is normal.
	FontStretch float32
	// LineHeight is a multiple of the font size or a pixel value.
	LineHeight    text.LineHeight
	LetterSpacing float32 // pixels
	WordSpacing   float32 // pixels
	TextAlign     text.TextAlign
	TextTransform text.TextTransform
	Decoration    Decoration
	WhiteSpace    text.WhiteSpace
	WordBreak     text.WordBreak
	OverflowWrap  text.OverflowWrap
	// TabSize is a multiple of a space's advance.
	TabSize float32
	// Numeric is font-variant-numeric (tabular-nums and friends).
	Numeric text.NumericVariant
	// Shadow is the text-shadow list; its colors are resolved.
	Shadow []Shadow
}

// CaretColor is the CSS caret-color: auto (the zero value: the text
// color) or a color.
type CaretColor struct {
	Set   bool
	Color gfx.Color
}

// FieldSizing is the CSS field-sizing property of text fields.
type FieldSizing uint8

const (
	// FieldSizingFixed sizes a field by its size or rows (the default).
	FieldSizingFixed FieldSizing = iota
	// FieldSizingContent sizes a field to its content.
	FieldSizingContent
)

// SelectionColors are the ::selection colors set by selection:* classes;
// a color not set keeps the platform's.
type SelectionColors struct {
	Background, Color       gfx.Color
	HasBackground, HasColor bool
}

// Transform is the 2D transform of a box, kept as the CSS properties that
// compose it. The composition order is CSS's: translate, rotate, scale and
// then the transform list (skew), around the transform origin.
type Transform struct {
	TranslateX, TranslateY LP // percentages refer to the box's own size
	Rotate                 float32
	ScaleX, ScaleY         float32 // 1 is identity
	SkewX, SkewY           float32 // degrees
	OriginX, OriginY       LP      // from the top-left of the border box; the default is 50% 50%
}

// IsIdentity reports whether the transform does nothing, whatever the
// size of the box.
func (t *Transform) IsIdentity() bool {
	return t.TranslateX == LP{} && t.TranslateY == LP{} && t.Rotate == 0 &&
		t.ScaleX == 1 && t.ScaleY == 1 && t.SkewX == 0 && t.SkewY == 0
}

// Matrix returns the transform for a box with the given border-box
// rectangle, in the coordinate space of the rectangle (no change for an
// identity transform). Percentages in translate and origin refer to the
// box's size.
func (t *Transform) Matrix(box gfx.Rect) gfx.Affine {
	if t.IsIdentity() {
		return gfx.Identity
	}
	w, h := box.Width(), box.Height()
	o := gfx.Point{X: box.X0 + t.OriginX.Resolve(w), Y: box.Y0 + t.OriginY.Resolve(h)}
	m := gfx.Translate(o.X, o.Y)
	if tx, ty := t.TranslateX.Resolve(w), t.TranslateY.Resolve(h); tx != 0 || ty != 0 {
		m = m.Mul(gfx.Translate(tx, ty))
	}
	if t.Rotate != 0 {
		m = m.Mul(gfx.Rotate(degToRad(t.Rotate)))
	}
	if t.ScaleX != 1 || t.ScaleY != 1 {
		m = m.Mul(gfx.Scale(t.ScaleX, t.ScaleY))
	}
	if t.SkewX != 0 || t.SkewY != 0 {
		m = m.Mul(gfx.Skew(degToRad(t.SkewX), degToRad(t.SkewY)))
	}
	return m.Mul(gfx.Translate(-o.X, -o.Y))
}

func degToRad(d float32) float32 { return float32(float64(d) * math.Pi / 180) }

// FilterMask records which filter functions of a FilterSet are present.
type FilterMask uint16

const (
	FilterBlur FilterMask = 1 << iota
	FilterBrightness
	FilterContrast
	FilterGrayscale
	FilterHueRotate
	FilterInvert
	FilterSaturate
	FilterSepia
)

// FilterSet is the filter or backdrop-filter of a box: the functions of
// Tailwind's filter utilities, applied in Tailwind's fixed order (blur,
// brightness, contrast, grayscale, hue-rotate, invert, saturate, sepia,
// drop-shadow).
type FilterSet struct {
	Set FilterMask
	// Blur is the standard deviation in pixels (CSS blur(r)).
	Blur       float32
	Brightness float32 // 1 is identity
	Contrast   float32 // 1 is identity
	Grayscale  float32 // 0 is identity
	HueRotate  float32 // degrees
	Invert     float32 // 0 is identity
	Saturate   float32 // 1 is identity
	Sepia      float32 // 0 is identity
	// DropShadow are the drop-shadow() layers (Blur is the CSS blur
	// radius, twice the standard deviation).
	DropShadow []Shadow
}

// IsEmpty reports whether the set has no filter function.
func (f *FilterSet) IsEmpty() bool { return f.Set == 0 && len(f.DropShadow) == 0 }

// List returns the filters in order, ready for gfx.PushLayer or
// gfx.BackdropFilter.
func (f *FilterSet) List() []gfx.Filter {
	if f.IsEmpty() {
		return nil
	}
	var out []gfx.Filter
	if f.Set&FilterBlur != 0 && f.Blur > 0 {
		out = append(out, gfx.Blur{StdDev: f.Blur})
	}
	if f.Set&FilterBrightness != 0 {
		out = append(out, gfx.Brightness(f.Brightness))
	}
	if f.Set&FilterContrast != 0 {
		out = append(out, gfx.Contrast(f.Contrast))
	}
	if f.Set&FilterGrayscale != 0 {
		out = append(out, gfx.Grayscale(f.Grayscale))
	}
	if f.Set&FilterHueRotate != 0 {
		out = append(out, gfx.HueRotate(degToRad(f.HueRotate)))
	}
	if f.Set&FilterInvert != 0 {
		out = append(out, gfx.Invert(f.Invert))
	}
	if f.Set&FilterSaturate != 0 {
		out = append(out, gfx.Saturate(f.Saturate))
	}
	if f.Set&FilterSepia != 0 {
		out = append(out, gfx.Sepia(f.Sepia))
	}
	for _, s := range f.DropShadow {
		out = append(out, gfx.DropShadow{
			Offset: gfx.Point{X: s.OffsetX, Y: s.OffsetY},
			StdDev: s.Blur / 2,
			Color:  s.Color,
		})
	}
	return out
}

// TransitionProps is the set of properties a transition applies to.
type TransitionProps uint32

const (
	TransitionColor TransitionProps = 1 << iota
	TransitionBackgroundColor
	TransitionBorderColor
	TransitionOutlineColor
	TransitionDecorationColor
	TransitionOpacity
	TransitionShadow
	TransitionTransform
	TransitionFilter
	TransitionBackdropFilter
	TransitionWidth
	TransitionHeight
	TransitionSpacing // padding, margin and gap
	TransitionInset
	TransitionBorderRadius
	TransitionBorderWidth
	TransitionFontSize
	TransitionClipPath

	// TransitionNone transitions nothing. It is the zero value.
	TransitionNone TransitionProps = 0
	// TransitionAll transitions every animatable property.
	TransitionAll TransitionProps = math.MaxUint32
	// TransitionColors is the set of Tailwind's transition-colors.
	TransitionColors = TransitionColor | TransitionBackgroundColor | TransitionBorderColor |
		TransitionOutlineColor | TransitionDecorationColor
	// TransitionDefault is the set of Tailwind's bare "transition".
	TransitionDefault = TransitionColors | TransitionOpacity | TransitionShadow |
		TransitionTransform | TransitionFilter | TransitionBackdropFilter
)

// ComputedTransition is the transition of a box: which properties animate
// when their computed value changes, and how.
type ComputedTransition struct {
	Properties TransitionProps
	Duration   time.Duration
	Timing     CubicBezier
	Delay      time.Duration
}

// Phase says when a deferred prop value is evaluated (ARCHITECTURE
// section 7).
type Phase uint8

const (
	// PhaseStyle is the normal phase: values are fixed when the style is
	// computed.
	PhaseStyle Phase = iota
	// PhaseLayout values are evaluated in the layout phase, when the
	// box's layout style is mapped.
	PhaseLayout
	// PhasePaint values are evaluated in the paint phase, when the box
	// is painted.
	PhasePaint
)

// Deferred is a closure-carrying prop evaluated in a later phase; see
// ComputedStyle.Resolved.
type Deferred struct {
	Phase Phase
	Apply func(*ComputedStyle)
}

// ComputedStyle is the fully resolved style of a box: typed values, no
// strings (ARCHITECTURE section 4). Compute produces it; layout, paint and
// the bridges to text read it.
//
// Slice fields are shared and must be treated as read-only. Lengths that
// depend on layout stay as LP or Dim; rem, em, vw, vh and ch are already
// pixels.
type ComputedStyle struct {
	// Layout and box.
	Display  Display
	Position Position
	// Inset are the top, right, bottom and left offsets (auto by default).
	Inset      Sides[Dim]
	ZIndex     ZIndex
	Isolate    bool
	OverflowX  Overflow
	OverflowY  Overflow
	Visibility Visibility // inherited
	// Scrolling (scroll containers only): scroll-auto/smooth/instant,
	// scrollbar-classic/overlay, overscroll-* per axis.
	ScrollBehavior           ScrollBehavior
	Scrollbar                ScrollbarMode
	OverscrollX, OverscrollY Overscroll

	// Sizing. Box sizing is always border-box. Max sizes treat auto as none.
	Width, Height       Dim
	MinWidth, MinHeight Dim
	MaxWidth, MaxHeight Dim
	// AspectRatio is width / height; zero is auto.
	AspectRatio float32

	// Spacing.
	Margin  Sides[Dim]
	Padding Sides[LP]
	// RowGap and ColumnGap are the gap between rows and between columns.
	RowGap, ColumnGap LP
	// SpaceX and SpaceY are the space-x-* and space-y-* lengths, which may
	// be negative (-space-x-2). Like Tailwind v4 they are not gaps: the
	// container's children but the last get them as a margin on the
	// inline-end (SpaceX) or block-end (SpaceY) side, in block and flex
	// containers alike, unless their own margin on that side is set
	// (ADR-070). Zero is unset.
	SpaceX, SpaceY LP

	// Flex.
	FlexDirection FlexDirection
	FlexWrap      FlexWrap
	FlexGrow      float32
	FlexShrink    float32 // 1 by default
	FlexBasis     Dim
	Order         int32

	// Alignment.
	AlignItems     Align
	AlignSelf      Align
	AlignContent   Align
	JustifyContent Align
	JustifyItems   Align
	JustifySelf    Align

	// Grid.
	GridTemplateColumns TrackList
	GridTemplateRows    TrackList
	GridAutoColumns     []TrackSize
	GridAutoRows        []TrackSize
	GridAutoFlow        GridAutoFlow
	GridColumn          GridPlacement
	GridRow             GridPlacement

	// Background. Backgrounds are the image layers, topmost first; the
	// color is painted below them.
	BackgroundColor gfx.Color
	Backgrounds     []BackgroundLayer
	BackgroundClip  BackgroundBox
	// BackgroundBlend is the background-blend-mode of every image layer
	// (bg-blend-*): each layer blends with the layers and the color below
	// it, inside the element's background.
	BackgroundBlend gfx.BlendMode

	// Borders and outline.
	Border  Sides[Border]
	Radius  Corners[LP] // percentages refer to the border box
	Outline Outline

	// Effects.
	// BoxShadows are the box-shadow layers, first on top. Rings are
	// shadows, like in Tailwind.
	BoxShadows     []Shadow
	Opacity        float32
	MixBlendMode   gfx.BlendMode
	Filter         FilterSet
	BackdropFilter FilterSet
	// ClipPath is the clip-path ([clip-path:...]); it creates a stacking
	// context and clips everything the box paints and its hit-testing.
	ClipPath ClipPath

	// Motion.
	Transform  Transform
	Transition ComputedTransition
	// Animation is the raw animation shorthand of an animate-* class
	// ("spin 1s linear infinite"); core parses it (anim.ParseSpec).
	Animation string

	// Interaction (cursor, pointer-events and user-select are inherited).
	Cursor        Cursor
	PointerEvents PointerEvents
	UserSelect    UserSelect

	// Text. Text is inherited; TextOverflow and LineClamp are not.
	Text         TextProps
	TextOverflow text.TextOverflow
	LineClamp    int
	// VerticalAlign is the CSS vertical-align of an atomic inline-level
	// box (not inherited); inline elements are on the baseline.
	VerticalAlign text.BoxAlign

	// Text fields (Input, Textarea). Caret is caret-color (inherited);
	// FieldSizing is field-sizing (not inherited). Placeholder is the
	// text style of the placeholder (placeholder:*), nil when no class
	// sets one: the default is the field's text at half opacity
	// (Preflight). Selection holds the colors of selected text
	// (selection:*, inherited like Tailwind's "& *::selection"), nil for
	// the platform's.
	Caret       CaretColor
	FieldSizing FieldSizing
	Placeholder *TextProps
	Selection   *SelectionColors

	// Deferred are the phase-deferred props (ARCHITECTURE section 7),
	// applied by Resolved.
	Deferred []Deferred
}

// Resolved returns the style with the deferred props of phase applied. It
// returns s itself when there is nothing to apply, otherwise a copy.
func (s *ComputedStyle) Resolved(phase Phase) *ComputedStyle {
	has := false
	for i := range s.Deferred {
		if s.Deferred[i].Phase == phase {
			has = true
			break
		}
	}
	if !has {
		return s
	}
	c := *s
	for _, d := range s.Deferred {
		if d.Phase == phase {
			d.Apply(&c)
		}
	}
	return &c
}

func enumName(names []string, i int) string {
	if i >= 0 && i < len(names) {
		return names[i]
	}
	return "?"
}
