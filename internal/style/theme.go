package style

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// Unit is the unit of a Length.
type Unit uint8

// Units a theme can express. rem stays symbolic so the root font size is
// theme-configurable (Theme.RootFontSize); em is relative to the font size
// of the element that uses the value.
const (
	UnitPx Unit = iota
	UnitRem
	UnitEm
)

// String returns the CSS name of the unit.
func (u Unit) String() string {
	switch u {
	case UnitPx:
		return "px"
	case UnitRem:
		return "rem"
	case UnitEm:
		return "em"
	}
	return fmt.Sprintf("Unit(%d)", uint8(u))
}

// Length is a number with a unit, as written in theme.css.
type Length struct {
	Value float32
	Unit  Unit
}

// Px returns a length in logical pixels.
func Px(v float32) Length { return Length{v, UnitPx} }

// Rem returns a length in multiples of the root font size.
func Rem(v float32) Length { return Length{v, UnitRem} }

// Em returns a length in multiples of the element's font size.
func Em(v float32) Length { return Length{v, UnitEm} }

// Resolve returns the length in logical pixels, given the root font size
// (for rem) and the element's font size (for em).
func (l Length) Resolve(rootFontSize, fontSize float32) float32 {
	switch l.Unit {
	case UnitRem:
		return float32(l.Value * rootFontSize)
	case UnitEm:
		return float32(l.Value * fontSize)
	}
	return l.Value
}

// String formats the length as CSS, for example "0.25rem".
func (l Length) String() string { return fmt.Sprintf("%g%s", l.Value, l.Unit) }

// TextSize is one step of the type scale: a font size and its default line
// height, which Tailwind defines as a unitless multiple of the font size.
type TextSize struct {
	Size       Length
	LineHeight float32
}

// Shadow is one layer of a box, inset, drop or text shadow. Offsets, blur
// and spread are in logical pixels. Spread is zero for drop and text
// shadows, which have none.
type Shadow struct {
	OffsetX float32
	OffsetY float32
	Blur    float32
	Spread  float32
	Color   gfx.Color
	Inset   bool
}

// CubicBezier holds the control points of a CSS cubic-bezier() easing
// curve; the end points are (0, 0) and (1, 1).
type CubicBezier struct{ X1, Y1, X2, Y2 float32 }

// Transition is the default transition applied by the bare "transition"
// utilities.
type Transition struct {
	Duration time.Duration
	Timing   CubicBezier
}

// Theme is the set of design tokens that classes resolve against. Map keys
// are the Tailwind names without the namespace prefix: "fuchsia-600" for
// --color-fuchsia-600, "5xl" for --text-5xl, "2xl" for --radius-2xl.
//
// The key "DEFAULT" in Radius, Shadows, DropShadows and Blur is the bare
// utility ("rounded", "shadow", "drop-shadow", "blur"). Tailwind v4
// deprecates these but still honors them, and so does Tela.
//
// The colors "transparent" and "current" are not in Colors; the class
// parser handles them. The default theme also has shadcn/ui's semantic
// color tokens (Tokens), which classes name like palette colors.
type Theme struct {
	// RootFontSize is the pixel size of 1rem.
	RootFontSize float32

	// Colors are opaque sRGB colors keyed by name: "white", "black" and
	// the palette ("red-50" to "taupe-950").
	Colors map[string]gfx.Color

	// Spacing is the base unit of the spacing scale: p-4 is 4 * Spacing.
	Spacing Length

	// Breakpoints are the minimum window widths of the sm, md, lg, xl and
	// 2xl variants.
	Breakpoints map[string]Length

	// Containers are the named widths of max-w-*, w-* and container queries.
	Containers map[string]Length

	// Text is the type scale ("xs" to "9xl").
	Text map[string]TextSize

	// FontWeights maps names ("thin" to "black") to CSS font weights.
	FontWeights map[string]float32

	// Tracking is letter spacing, in em ("tight" is -0.025em).
	Tracking map[string]Length

	// Leading is line height as a unitless multiple of the font size.
	Leading map[string]float32

	// Radius is border radius.
	Radius map[string]Length

	// Shadows are box-shadow layers (shadow-*). A deprecated v3 alias,
	// "inner", is inset.
	Shadows map[string][]Shadow

	// InsetShadows are inset box-shadow layers (inset-shadow-*); every
	// layer has Inset set.
	InsetShadows map[string][]Shadow

	// DropShadows are the layers of the drop-shadow() filter function.
	DropShadows map[string][]Shadow

	// TextShadows are text-shadow layers (text-shadow-*).
	TextShadows map[string][]Shadow

	// Blur is the blur radius of the blur() and backdrop-blur() filters.
	Blur map[string]Length

	// Perspective is the distance of perspective-*.
	Perspective map[string]Length

	// Aspect maps names to width / height ("video" is 16 / 9).
	Aspect map[string]float32

	// Ease are the named easing curves "in", "out" and "in-out".
	Ease map[string]CubicBezier

	// DefaultTransition is the duration and easing of the bare
	// "transition" utilities.
	DefaultTransition Transition

	// Fonts maps the font-* families ("sans", "serif", "mono") to a
	// family list. Tela maps them to its embedded fonts instead of the
	// system stacks of Tailwind (ADR-008, ADR-038).
	Fonts map[string][]string

	// DefaultFont and DefaultMonoFont are the keys of Fonts used for
	// normal text and for monospace elements (code, pre, kbd, samp).
	DefaultFont     string
	DefaultMonoFont string

	// Tokens are the semantic color tokens ("primary",
	// "muted-foreground"), with a light and a dark value each. Classes
	// name them like palette colors; they resolve for the element's color
	// scheme when its style is computed (tokens.go).
	Tokens map[string]SchemeColor

	// Animate maps the animate-* names to their CSS animation shorthand
	// ("spin" is "spin 1s linear infinite"). The strings are kept raw;
	// animation support (phase 6) parses them.
	Animate map[string]string
}

// DefaultTheme returns the default Tailwind v4 theme. Every call returns an
// independent copy: callers may modify its maps and slices freely.
func DefaultTheme() *Theme {
	t := newDefaultTheme()
	t.Tokens = defaultTokens()
	return &t
}

// Clone returns a deep copy of t.
func (t *Theme) Clone() *Theme {
	c := *t
	c.Colors = maps.Clone(t.Colors)
	c.Breakpoints = maps.Clone(t.Breakpoints)
	c.Containers = maps.Clone(t.Containers)
	c.Text = maps.Clone(t.Text)
	c.FontWeights = maps.Clone(t.FontWeights)
	c.Tracking = maps.Clone(t.Tracking)
	c.Leading = maps.Clone(t.Leading)
	c.Radius = maps.Clone(t.Radius)
	c.Shadows = cloneSliceMap(t.Shadows)
	c.InsetShadows = cloneSliceMap(t.InsetShadows)
	c.DropShadows = cloneSliceMap(t.DropShadows)
	c.TextShadows = cloneSliceMap(t.TextShadows)
	c.Blur = maps.Clone(t.Blur)
	c.Perspective = maps.Clone(t.Perspective)
	c.Aspect = maps.Clone(t.Aspect)
	c.Ease = maps.Clone(t.Ease)
	c.Fonts = cloneSliceMap(t.Fonts)
	c.Animate = maps.Clone(t.Animate)
	c.Tokens = cloneTokens(t.Tokens)
	return &c
}

func cloneSliceMap[V any](m map[string][]V) map[string][]V {
	if m == nil {
		return nil
	}
	c := make(map[string][]V, len(m))
	for k, v := range m {
		c[k] = slices.Clone(v)
	}
	return c
}

// Namespace identifies one family of theme keys, named after the Tailwind
// theme variable prefix.
type Namespace uint8

// The theme namespaces. Spacing is not one: its keys are numbers (any
// multiple of Theme.Spacing), see SpacingSuggestions.
const (
	NamespaceColor Namespace = iota
	NamespaceBreakpoint
	NamespaceContainer
	NamespaceText
	NamespaceFontWeight
	NamespaceTracking
	NamespaceLeading
	NamespaceRadius
	NamespaceShadow
	NamespaceInsetShadow
	NamespaceDropShadow
	NamespaceTextShadow
	NamespaceBlur
	NamespacePerspective
	NamespaceAspect
	NamespaceEase
	NamespaceFont
	NamespaceAnimate
)

var namespaceNames = [...]string{
	NamespaceColor:       "color",
	NamespaceBreakpoint:  "breakpoint",
	NamespaceContainer:   "container",
	NamespaceText:        "text",
	NamespaceFontWeight:  "font-weight",
	NamespaceTracking:    "tracking",
	NamespaceLeading:     "leading",
	NamespaceRadius:      "radius",
	NamespaceShadow:      "shadow",
	NamespaceInsetShadow: "inset-shadow",
	NamespaceDropShadow:  "drop-shadow",
	NamespaceTextShadow:  "text-shadow",
	NamespaceBlur:        "blur",
	NamespacePerspective: "perspective",
	NamespaceAspect:      "aspect",
	NamespaceEase:        "ease",
	NamespaceFont:        "font",
	NamespaceAnimate:     "animate",
}

// String returns the Tailwind variable prefix of the namespace, such as
// "font-weight" for --font-weight-*.
func (n Namespace) String() string {
	if int(n) < len(namespaceNames) {
		return namespaceNames[n]
	}
	return fmt.Sprintf("Namespace(%d)", uint8(n))
}

// Namespaces returns every theme namespace in declaration order.
func Namespaces() []Namespace {
	ns := make([]Namespace, len(namespaceNames))
	for i := range ns {
		ns[i] = Namespace(i)
	}
	return ns
}

// Keys returns the keys of namespace ns in t, sorted. It is the theme half
// of the class vocabulary used for suggestions (ADR-037). A custom theme
// may add or remove keys, so the result always reflects t.
func (t *Theme) Keys(ns Namespace) []string {
	switch ns {
	case NamespaceColor:
		return sortedKeys(t.Colors)
	case NamespaceBreakpoint:
		return sortedKeys(t.Breakpoints)
	case NamespaceContainer:
		return sortedKeys(t.Containers)
	case NamespaceText:
		return sortedKeys(t.Text)
	case NamespaceFontWeight:
		return sortedKeys(t.FontWeights)
	case NamespaceTracking:
		return sortedKeys(t.Tracking)
	case NamespaceLeading:
		return sortedKeys(t.Leading)
	case NamespaceRadius:
		return sortedKeys(t.Radius)
	case NamespaceShadow:
		return sortedKeys(t.Shadows)
	case NamespaceInsetShadow:
		return sortedKeys(t.InsetShadows)
	case NamespaceDropShadow:
		return sortedKeys(t.DropShadows)
	case NamespaceTextShadow:
		return sortedKeys(t.TextShadows)
	case NamespaceBlur:
		return sortedKeys(t.Blur)
	case NamespacePerspective:
		return sortedKeys(t.Perspective)
	case NamespaceAspect:
		return sortedKeys(t.Aspect)
	case NamespaceEase:
		return sortedKeys(t.Ease)
	case NamespaceFont:
		return sortedKeys(t.Fonts)
	case NamespaceAnimate:
		return sortedKeys(t.Animate)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

// SpacingSuggestions lists the spacing keys worth suggesting for p-*, m-*,
// gap-*, w-* and the like: the classic Tailwind scale. Any non-negative
// multiple of 0.25 is also valid ("p-7", "p-13"), so the list is for
// suggestions only, not for validation.
var SpacingSuggestions = []string{
	"0", "px", "0.5", "1", "1.5", "2", "2.5", "3", "3.5", "4", "5", "6", "7", "8", "9",
	"10", "11", "12", "14", "16", "20", "24", "28", "32", "36", "40", "44", "48", "52",
	"56", "60", "64", "72", "80", "96",
}
