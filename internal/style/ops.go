package style

import (
	"image"
	"sync"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// Op is one style declaration: what a class or a typed Style prop does to
// a style being computed. Ops are immutable and shared; the class parser
// interns them.
//
// Ops come from three sources: class strings (Parser.Compile), the
// element defaults (KindDefaults) and the typed props below (Width, Bg,
// Padding...). An op that reads state known only in a later phase is
// built with the Fn constructors (TranslateFn) and carries a closure
// instead (ARCHITECTURE section 7).
type Op struct {
	apply func(*env)
	// font marks the ops that set the font size; they run before every
	// other op so that em resolves against the final font size.
	font bool
	// def is the closure of a phase-deferred op; phase says when it runs.
	def   func(*ComputedStyle)
	phase Phase
	// post, for a font op, runs in the second pass: the part of the op
	// that does not set the font size (the line height of text-sm/6).
	post func(*env)
}

// Phase returns the phase in which the op takes effect: PhaseStyle for
// ordinary ops, PhaseLayout or PhasePaint for deferred ones.
func (o Op) Phase() Phase {
	if o.def != nil {
		return o.phase
	}
	return PhaseStyle
}

// IsZero reports whether the op does nothing.
func (o Op) IsZero() bool { return o.apply == nil && o.def == nil }

func op(f func(*env)) Op { return Op{apply: f} }

// ---------------------------------------------------------------------
// Typed props. These are the building blocks of the public Style(...)
// props; they take already-typed values, so a dynamic value never goes
// through a class string.

// Prop returns an op that edits the computed style directly, an escape
// hatch for properties without a constructor. It runs after the classes
// and must only set plain fields (not currentColor-dependent ones).
func Prop(f func(*ComputedStyle)) Op {
	return op(func(e *env) { f(e.s) })
}

// Width sets the width in pixels.
func Width(px float32) Op { return WidthDim(DimPx(px)) }

// WidthDim sets the width to any dimension (auto, a percentage, fit-content...).
func WidthDim(d Dim) Op { return op(func(e *env) { e.s.Width = d }) }

// Height sets the height in pixels.
func Height(px float32) Op { return HeightDim(DimPx(px)) }

// HeightDim sets the height to any dimension.
func HeightDim(d Dim) Op { return op(func(e *env) { e.s.Height = d }) }

// MinWidth, MinHeight, MaxWidth and MaxHeight set the size limits in pixels.
func MinWidth(px float32) Op  { return op(func(e *env) { e.s.MinWidth = DimPx(px) }) }
func MinHeight(px float32) Op { return op(func(e *env) { e.s.MinHeight = DimPx(px) }) }
func MaxWidth(px float32) Op  { return op(func(e *env) { e.s.MaxWidth = DimPx(px) }) }
func MaxHeight(px float32) Op { return op(func(e *env) { e.s.MaxHeight = DimPx(px) }) }

// Padding sets the padding of all four sides in pixels.
func Padding(px float32) Op {
	return op(func(e *env) { e.s.Padding = AllSides(PxLP(px)) })
}

// PaddingXY sets the horizontal and vertical padding in pixels.
func PaddingXY(x, y float32) Op {
	return op(func(e *env) { e.s.Padding = Sides[LP]{PxLP(y), PxLP(x), PxLP(y), PxLP(x)} })
}

// Margin sets the margin of all four sides in pixels.
func Margin(px float32) Op {
	return op(func(e *env) { e.s.Margin = AllSides(DimPx(px)) })
}

// Gap sets the row and column gap in pixels.
func Gap(px float32) Op {
	return op(func(e *env) { e.s.RowGap, e.s.ColumnGap = PxLP(px), PxLP(px) })
}

// Left, Top, Right and Bottom set the inset of a positioned box in pixels.
func Left(px float32) Op   { return op(func(e *env) { e.s.Inset.Left = DimPx(px) }) }
func Top(px float32) Op    { return op(func(e *env) { e.s.Inset.Top = DimPx(px) }) }
func Right(px float32) Op  { return op(func(e *env) { e.s.Inset.Right = DimPx(px) }) }
func Bottom(px float32) Op { return op(func(e *env) { e.s.Inset.Bottom = DimPx(px) }) }

// Bg sets the background color.
func Bg(c gfx.Color) Op {
	return op(func(e *env) { e.sc.colors[slotBg] = solidSpec(c) })
}

// Color sets the text color.
func Color(c gfx.Color) Op {
	return op(func(e *env) { e.sc.colors[slotText] = solidSpec(c) })
}

// BorderColor sets the color of the four borders.
func BorderColor(c gfx.Color) Op {
	return op(func(e *env) {
		for slot := slotBorderTop; slot <= slotBorderLeft; slot++ {
			e.sc.colors[slot] = solidSpec(c)
		}
	})
}

// BorderWidth sets the width of the four borders in pixels.
func BorderWidth(px float32) Op {
	return op(func(e *env) {
		for _, b := range borderSides(e.s) {
			b.Width = px
		}
	})
}

// Radius sets the radius of the four corners in pixels.
func Radius(px float32) Op {
	return op(func(e *env) { e.s.Radius = Corners[LP]{PxLP(px), PxLP(px), PxLP(px), PxLP(px)} })
}

// Opacity sets the opacity, from 0 to 1.
func Opacity(v float32) Op {
	return op(func(e *env) { e.s.Opacity = clampf(v, 0, 1) })
}

// ZIndexOf sets an explicit z-index.
func ZIndexOf(z int32) Op {
	return op(func(e *env) { e.s.ZIndex = ZIndex{Explicit: true, Value: z} })
}

// Translate sets the translation in pixels.
func Translate(x, y float32) Op {
	return op(func(e *env) { e.s.Transform.TranslateX, e.s.Transform.TranslateY = PxLP(x), PxLP(y) })
}

// Scale sets the scale factors (1 is identity).
func Scale(x, y float32) Op {
	return op(func(e *env) { e.s.Transform.ScaleX, e.s.Transform.ScaleY = x, y })
}

// Rotate sets the rotation in degrees.
func Rotate(deg float32) Op {
	return op(func(e *env) { e.s.Transform.Rotate = deg })
}

// FontSize sets the font size in pixels.
func FontSize(px float32) Op {
	return Op{font: true, apply: func(e *env) { e.s.Text.FontSize = max(px, 0) }}
}

// FontWeight sets the font weight (100 to 900).
func FontWeight(w float32) Op {
	return op(func(e *env) { e.s.Text.FontWeight = w })
}

// BoxShadow sets the box shadows, replacing those of shadow-* classes.
func BoxShadow(layers ...Shadow) Op {
	cp := append([]Shadow(nil), layers...)
	return op(func(e *env) {
		e.sc.shadowUsed = true
		e.sc.shadow, e.sc.insetShadow = nil, nil
		e.sc.ringW, e.sc.ringOffW, e.sc.insetRingW = 0, 0, 0
		for _, l := range cp {
			if l.Inset {
				e.sc.insetShadow = append(e.sc.insetShadow, l)
			} else {
				e.sc.shadow = append(e.sc.shadow, l)
			}
		}
	})
}

// BackgroundGradient sets a gradient as the background image.
func BackgroundGradient(g Gradient) Op {
	return op(func(e *env) { e.sc.bgImage = bgImage{kind: 1, gradient: g} })
}

// BgImage sets a decoded image as the background image; the background
// size, position, repeat and origin classes apply to it. img must not be
// nil.
func BgImage(img image.Image) Op {
	return op(func(e *env) { e.sc.bgImage = bgImage{kind: 4, img: img} })
}

// ---------------------------------------------------------------------
// Phase-deferred props (ARCHITECTURE section 7). Compute records the
// closures in ComputedStyle.Deferred and Resolved applies them; core
// evaluates them when it maps the layout style (PhaseLayout) or paints the
// box (PhasePaint), tracking the Values they read, so that a change of
// those re-runs only that phase for that box.

// TranslateFn defers the translation to the paint phase: fn is called when
// the box is painted, so a change of its inputs invalidates paint only.
func TranslateFn(fn func() (x, y float32)) Op {
	return Op{phase: PhasePaint, def: func(s *ComputedStyle) {
		x, y := fn()
		s.Transform.TranslateX, s.Transform.TranslateY = PxLP(x), PxLP(y)
	}}
}

// OpacityFn defers the opacity to the paint phase.
func OpacityFn(fn func() float32) Op {
	return Op{phase: PhasePaint, def: func(s *ComputedStyle) { s.Opacity = clampf(fn(), 0, 1) }}
}

// WidthFn defers the width, in pixels, to the layout phase (a progress bar
// driven by a UseValue).
func WidthFn(fn func() float32) Op {
	return Op{phase: PhaseLayout, def: func(s *ComputedStyle) { s.Width = DimPx(fn()) }}
}

// HeightFn defers the height, in pixels, to the layout phase.
func HeightFn(fn func() float32) Op {
	return Op{phase: PhaseLayout, def: func(s *ComputedStyle) { s.Height = DimPx(fn()) }}
}

// ---------------------------------------------------------------------
// Element defaults (ADR-038).

// Kind is the kind of element a style is computed for; it selects the
// defaults of Tailwind's Preflight.
type Kind uint8

// The primitives of phase 3, the inline text elements of section 11 and
// the form elements of phase 5. The zero value is Div.
const (
	KindDiv Kind = iota
	KindSpan
	KindText
	KindH1
	KindH2
	KindH3
	KindH4
	KindH5
	KindH6
	KindP
	KindButton
	KindImg
	KindRoot
	KindStrong
	KindEm
	KindCode
	KindA
	KindInput
	KindTextarea
	KindForm
	KindLabel
	numKinds
)

var kindNames = [...]string{"div", "span", "text", "h1", "h2", "h3", "h4", "h5", "h6", "p", "button", "img", "root", "strong", "em", "code", "a",
	"input", "textarea", "form", "label"}

// IsTextField reports whether k is a text field (Input, Textarea).
func (k Kind) IsTextField() bool { return k == KindInput || k == KindTextarea }

func (k Kind) String() string { return enumName(kindNames[:], int(k)) }

var (
	kindOps  [numKinds][]Op
	kindOnce sync.Once
)

// KindDefaults returns the default ops of an element kind, applied before
// its classes. Each default is annotated with the rule it comes from.
//
// Tailwind's universal Preflight rules (margin: 0, padding: 0, border: 0
// solid currentColor, box-sizing: border-box) are the initial values of
// every ComputedStyle, and the root text style (Inter 16px, line-height
// 1.5, tab-size 4, black) is the style of an element without a parent, so
// they appear in no list.
func KindDefaults(k Kind) []Op { return kindDefaults(k) }

func kindDefaults(k Kind) []Op {
	kindOnce.Do(initKindDefaults)
	if k >= numKinds {
		return nil
	}
	return kindOps[k]
}

func display(d Display) Op { return op(func(e *env) { e.s.Display = d }) }

func initKindDefaults() {
	// div, p, h1-h6, root: display: block, the HTML default (section 9).
	// Preflight resets their margins, and the headings' font-size and
	// font-weight to inherit ("h1..h6 { font-size: inherit; font-weight:
	// inherit }"), which is what inheritance already does.
	for _, k := range []Kind{KindDiv, KindP, KindH1, KindH2, KindH3, KindH4, KindH5, KindH6, KindRoot} {
		kindOps[k] = []Op{display(DisplayBlock)}
	}
	// span, text, strong, em, code, a: inline elements live in a text
	// context (section 9).
	for _, k := range []Kind{KindSpan, KindText, KindStrong, KindEm, KindCode, KindA} {
		kindOps[k] = []Op{display(DisplayInline)}
	}
	// button (Preflight): "font: inherit; letter-spacing: inherit; color:
	// inherit; border-radius: 0; background-color: transparent; opacity:
	// 1" are inheritance and the initial values. A button is inline-block
	// in browsers, an atomic inline of its paragraph with its fit-content
	// width (ADR-038, ADR-066); section 10 gives it cursor-pointer; the
	// browser's own stylesheet centers its label.
	kindOps[KindButton] = []Op{
		display(DisplayInlineBlock),
		op(func(e *env) { e.s.Cursor = CursorPointer }),
		op(func(e *env) { e.s.Text.TextAlign = text.AlignCenter }),
	}
	// img (Preflight): "img { display: block; vertical-align: middle }"
	// and "img, video { max-width: 100%; height: auto }".
	kindOps[KindImg] = []Op{
		display(DisplayBlock),
		op(func(e *env) {
			e.s.MaxWidth = DimPct(100)
			e.s.Height = Auto
			e.s.VerticalAlign = text.BoxMiddle
		}),
	}
	// strong (Preflight): "b, strong { font-weight: bolder }".
	kindOps[KindStrong] = append(kindOps[KindStrong], op(func(e *env) {
		e.s.Text.FontWeight = bolder(e.s.Text.FontWeight)
	}))
	// em: the browser's stylesheet makes it italic.
	kindOps[KindEm] = append(kindOps[KindEm], op(func(e *env) { e.s.Text.FontStyle = text.StyleItalic }))
	// code (Preflight): "code, kbd, samp, pre { font-family: mono;
	// font-size: 1em }"; 1em is the size of the parent, which
	// inheritance gives.
	kindOps[KindCode] = append(kindOps[KindCode], op(func(e *env) {
		if f, ok := e.theme.Fonts[e.theme.DefaultMonoFont]; ok {
			e.s.Text.FontFamily = f
		}
	}))
	// a (Preflight): "a { color: inherit; text-decoration: inherit }" is
	// inheritance.

	// input, textarea (Preflight): "font: inherit; letter-spacing:
	// inherit; color: inherit; border-radius: 0; background-color:
	// transparent; opacity: 1" are inheritance and the initial values
	// (and the universal reset leaves them without padding or border).
	// Chromium's stylesheet: "display: inline-block; text-align: start;
	// text-transform: none; text-indent: 0; text-shadow: none;
	// word-spacing: normal" (letter-spacing and line-height are
	// Preflight's inherit), cursor: text (the I-beam over editable text);
	// an input clips its content ("overflow: clip !important") and a
	// textarea scrolls it ("overflow: auto") and wraps it ("white-space:
	// pre-wrap; overflow-wrap: break-word").
	field := func(e *env) {
		e.s.Display = DisplayInlineBlock
		e.s.Cursor = CursorText
		e.s.Text.TextAlign = text.AlignStart
		e.s.Text.TextTransform = text.TransformNone
		e.s.Text.Shadow = nil
		e.s.Text.WordSpacing = 0
	}
	kindOps[KindInput] = []Op{op(func(e *env) {
		field(e)
		e.s.OverflowX, e.s.OverflowY = OverflowClip, OverflowClip
	})}
	kindOps[KindTextarea] = []Op{op(func(e *env) {
		field(e)
		e.s.OverflowX, e.s.OverflowY = OverflowAuto, OverflowAuto
		e.s.Text.WhiteSpace = text.WhiteSpacePreWrap
		e.s.Text.OverflowWrap = text.OverflowWrapBreakWord
	})}
	// form: a block. label: inline, with Chromium's "cursor: default".
	kindOps[KindForm] = []Op{display(DisplayBlock)}
	kindOps[KindLabel] = []Op{display(DisplayInline), op(func(e *env) { e.s.Cursor = CursorDefault })}
}

// bolder is the CSS font-weight: bolder keyword relative to the inherited
// weight (CSS Fonts 4, table in section 2.2.1).
func bolder(w float32) float32 {
	switch {
	case w < 350:
		return 400
	case w < 550:
		return 700
	}
	return 900
}

var (
	sharedThemeOnce sync.Once
	sharedTheme     *Theme
)

// defaultThemeShared returns a process-wide default theme that must not be
// modified.
func defaultThemeShared() *Theme {
	sharedThemeOnce.Do(func() { sharedTheme = DefaultTheme() })
	return sharedTheme
}
