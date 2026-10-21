package tela

import "github.com/kuronosu/tela/internal/core"

// Theme holds an app's design tokens: semantic colors with a light and a
// dark value, the base radius and the font families. Classes name the
// colors like palette colors, with every color utility, opacity modifier,
// variant and transition: bg-primary, text-muted-foreground,
// border-border, ring-ring/50, hover:bg-primary/90,
// from-chart-1. A token takes the value of the color scheme of the element
// that uses it ([App].ColorScheme, [SetColorScheme], or the Scheme prop of
// an ancestor, h.Scheme), so one class serves both schemes, without dark:
// pairs.
//
// The token names are shadcn/ui's (v4): background, foreground, card,
// card-foreground, popover, popover-foreground, primary,
// primary-foreground, secondary, secondary-foreground, muted,
// muted-foreground, accent, accent-foreground, destructive, border,
// input, ring and chart-1 to chart-5. [DefaultTheme] holds shadcn/ui's
// neutral values; start from it and change what you need:
//
//	theme := tela.DefaultTheme()
//	theme.Colors["primary"] = tela.SchemeColor{Light: tela.OKLCH(0.55, 0.2, 264), Dark: tela.OKLCH(0.7, 0.16, 264)}
//	theme.Colors["brand"] = tela.SchemeColor{Light: tela.Hex("#7c3aed"), Dark: tela.Hex("#a78bfa")} // bg-brand
//	theme.Radius = 10 // shadcn/ui's 0.625rem
//	tela.Main(tela.App{Title: "Notes", Root: tela.C(Notes), Theme: &theme, RootClass: "bg-background text-foreground"})
//
// A token missing from Colors keeps its value in DefaultTheme, so the
// shadcn/ui tokens always resolve. A token name is lowercase letters,
// digits and dashes, and cannot be a palette color ("red-500", "white")
// or transparent, current or inherit; [Run] panics otherwise. Tokens
// cannot be used inside arbitrary values (bg-[linear-gradient(...)]):
// use the utilities (from-primary, shadow-primary).
type Theme struct {
	// Colors maps token names to their values in the light and the dark
	// scheme.
	Colors map[string]SchemeColor
	// Radius is the base radius in logical pixels, shadcn/ui's --radius:
	// rounded-lg is Radius, rounded-sm 0.6 x, rounded-md 0.8 x,
	// rounded-xl 1.4 x, rounded-2xl 1.8 x, rounded-3xl 2.2 x and
	// rounded-4xl 2.6 x (shadcn/ui v4's theme). 0 keeps Tailwind's radius
	// scale, which DefaultTheme does so that existing pages do not change;
	// set 10 for shadcn/ui's look.
	Radius float32
	// FontSans and FontMono are font family names for font-sans (also
	// the default text font) and font-mono (code, kbd, pre, samp); the
	// embedded Inter and JetBrains Mono stay as fallbacks. "" keeps the
	// defaults.
	FontSans, FontMono string
}

// SchemeColor is the value of a theme color in the light and the dark
// color scheme.
//
//	tela.SchemeColor{Light: tela.Hex("#fafafa"), Dark: tela.Hex("#0a0a0a")}
type SchemeColor = core.SchemeColor

// DefaultTheme returns the default theme: shadcn/ui's neutral colors (v4,
// converted from OKLCH to extended sRGB like the Tailwind palette),
// Tailwind's radius scale and the embedded fonts. It is the theme of an
// [App] whose Theme is nil. Every call returns a new map.
func DefaultTheme() Theme { return Theme(core.DefaultTheme()) }

// SetTheme replaces the theme of the running app: classes are compiled
// again against it (with the app's ClassAliases), every element is styled
// again and the components that call [UseTheme] build again. Like
// [SetColorScheme] it is safe from any goroutine, applies on the UI
// thread before the next frame, goes to the app that receives posted
// functions, and called before [Run] applies when the window opens (it
// wins over [App].Theme). It panics for a token name classes cannot use.
func SetTheme(t Theme) { core.SetTheme(core.Theme(t)) }

// ThemeValues are the theme's values at a component's position, from
// [UseTheme]: the colors resolved for the color scheme there. Use them
// where classes cannot go: canvases, images tinted in Go, computed
// styles.
type ThemeValues struct{ v core.ThemeValues }

// UseTheme returns the theme's values for the color scheme at the
// component's position: the Scheme prop of the nearest element above it
// that has one (h.Scheme), else the app's scheme. The component builds
// again when that scheme or the theme ([SetTheme]) changes. It is a hook:
// call it at the top level of a component.
//
//	func Spark() tela.Node {
//	    th := tela.UseTheme()
//	    line := th.Color("chart-1")
//	    return Div(Style(tela.Bg(th.Color("muted"))), ...)
//	}
func UseTheme() ThemeValues { return ThemeValues{core.UseTheme()} }

// Color returns the value of a color token ("primary") or of a palette
// color ("red-500") in the scheme of the values. An unknown name is
// transparent; in dev mode it panics with the closest token name.
func (t ThemeValues) Color(name string) Color { return t.v.Color(name) }

// Lookup returns the value of a color token or palette color, and whether
// the theme has it.
func (t ThemeValues) Lookup(name string) (Color, bool) { return t.v.Lookup(name) }

// Scheme returns the color scheme of the values, [Light] or [Dark].
func (t ThemeValues) Scheme() ColorScheme {
	if t.v.Dark() {
		return Dark
	}
	return Light
}

// Radius returns the radius of rounded-lg in logical pixels: the theme's
// Radius, or Tailwind's 8 when the theme keeps Tailwind's scale.
func (t ThemeValues) Radius() float32 { return t.v.Radius() }

// FontSans returns the family of font-sans: the theme's FontSans, or the
// embedded "Inter".
func (t ThemeValues) FontSans() string { return t.v.FontSans() }

// FontMono returns the family of font-mono: the theme's FontMono, or the
// embedded "JetBrains Mono".
func (t ThemeValues) FontMono() string { return t.v.FontMono() }

// Tokens returns the names of the theme's color tokens, sorted.
func (t ThemeValues) Tokens() []string { return t.v.Tokens() }
