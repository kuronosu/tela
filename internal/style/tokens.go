package style

import (
	"fmt"
	"maps"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// Semantic color tokens (ARCHITECTURE section 10, "Theme and tokens";
// ADR-089). A token is a color name with a light and a dark value, usable
// wherever a palette color is ("bg-primary", "text-muted-foreground/80",
// "hover:border-ring"). Unlike palette colors, which are resolved when a
// class is parsed (ADR-042), a token stays symbolic in the compiled class
// and is resolved by Compute for the color scheme of the element: the
// scheme of the nearest Scheme override (ComputedStyle.Scheme), else
// Context.Dark. The same compiled classes therefore serve both schemes
// and every subtree with its own scheme.

// SchemeColor is the value of a semantic color token in the light and the
// dark color scheme.
type SchemeColor struct {
	Light, Dark gfx.Color
}

// Scheme is a color scheme override for an element and its descendants
// (the Scheme prop): the dark: variants and the tokens follow it instead
// of Context.Dark.
type Scheme uint8

const (
	// SchemeInherit is the absence of an override: the element follows
	// its parent's scheme, or Context.Dark at the root.
	SchemeInherit Scheme = iota
	// SchemeLight forces the light scheme.
	SchemeLight
	// SchemeDark forces the dark scheme.
	SchemeDark
)

// String returns the name of the scheme.
func (s Scheme) String() string {
	switch s {
	case SchemeInherit:
		return "inherit"
	case SchemeLight:
		return "light"
	case SchemeDark:
		return "dark"
	}
	return fmt.Sprintf("Scheme(%d)", uint8(s))
}

// IsDark reports whether the element of style s uses the dark scheme in
// the context cx: its Scheme override, or cx.Dark without one.
func (s *ComputedStyle) IsDark(cx *Context) bool {
	switch s.Scheme {
	case SchemeLight:
		return false
	case SchemeDark:
		return true
	}
	return cx.Dark
}

// TokenNames returns the names of the theme's semantic color tokens,
// sorted.
func (t *Theme) TokenNames() []string { return sortedKeys(t.Tokens) }

// TokenColor returns the value of token name in the light or the dark
// scheme, and whether the theme has it.
func (t *Theme) TokenColor(name string, dark bool) (gfx.Color, bool) {
	v, ok := t.Tokens[name]
	if !ok {
		return gfx.Color{}, false
	}
	if dark {
		return v.Dark, true
	}
	return v.Light, true
}

// SuggestToken returns the token name closest to name, or "".
func (t *Theme) SuggestToken(name string) string {
	return closest(name, t.TokenNames())
}

// SetRadius makes the radius scale follow a base radius r in logical
// pixels, the way shadcn/ui's theme derives it from --radius: rounded-lg
// is r, rounded-sm 0.6 r, rounded-md 0.8 r, rounded-xl 1.4 r, rounded-2xl
// 1.8 r, rounded-3xl 2.2 r and rounded-4xl 2.6 r. rounded-xs, the bare
// rounded, rounded-none and rounded-full keep Tailwind's values. r <= 0
// leaves the scale unchanged.
func (t *Theme) SetRadius(r float32) {
	if !(r > 0) {
		return
	}
	if t.Radius == nil {
		t.Radius = map[string]Length{}
	} else {
		t.Radius = maps.Clone(t.Radius)
	}
	for k, f := range radiusFactors {
		t.Radius[k] = Px(float32(r * f))
	}
}

// radiusFactors are the multiples of --radius in shadcn/ui's v4 theme
// (apps/v4 globals.css and the theming docs: --radius-sm is
// calc(var(--radius) * 0.6) and so on).
var radiusFactors = map[string]float32{
	"sm": 0.6, "md": 0.8, "lg": 1, "xl": 1.4, "2xl": 1.8, "3xl": 2.2, "4xl": 2.6,
}

// CheckTokens reports token names that cannot be used in classes: empty
// names, characters other than lowercase letters, digits and inner
// dashes, and names that are already colors (a palette color of the
// theme, transparent, current, inherit).
func (t *Theme) CheckTokens() error {
	var bad []string
	for _, name := range t.TokenNames() {
		switch {
		case !isTokenName(name):
			bad = append(bad, fmt.Sprintf("%q is not a valid token name (use lowercase letters, digits and dashes, such as \"brand-muted\")", name))
		case name == "transparent" || name == "current" || name == "inherit":
			bad = append(bad, fmt.Sprintf("%q is a CSS color keyword, not a token name", name))
		default:
			if _, ok := t.Colors[name]; ok {
				bad = append(bad, fmt.Sprintf("%q is already a palette color; pick another token name", name))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("theme colors: %s", strings.Join(bad, "; "))
	}
	return nil
}

func isTokenName(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// defaultTokens returns shadcn/ui's v4 "neutral" theme (the default of
// its theming docs), converted from OKLCH to extended sRGB like the
// Tailwind palette (ADR-046). Its radius, 0.625rem, is not applied: the
// default theme keeps Tailwind's radius scale (SetRadius applies one).
func defaultTokens() map[string]SchemeColor {
	ok := func(l, c, h float32) gfx.Color { return gfx.OKLCH(l, c, h, 1) }
	gray := func(l float32) gfx.Color { return ok(l, 0, 0) }
	pair := func(light, dark gfx.Color) SchemeColor { return SchemeColor{Light: light, Dark: dark} }
	return map[string]SchemeColor{
		"background":           pair(gray(1), gray(0.145)),
		"foreground":           pair(gray(0.145), gray(0.985)),
		"card":                 pair(gray(1), gray(0.205)),
		"card-foreground":      pair(gray(0.145), gray(0.985)),
		"popover":              pair(gray(1), gray(0.205)),
		"popover-foreground":   pair(gray(0.145), gray(0.985)),
		"primary":              pair(gray(0.205), gray(0.922)),
		"primary-foreground":   pair(gray(0.985), gray(0.205)),
		"secondary":            pair(gray(0.97), gray(0.269)),
		"secondary-foreground": pair(gray(0.205), gray(0.985)),
		"muted":                pair(gray(0.97), gray(0.269)),
		"muted-foreground":     pair(gray(0.556), gray(0.708)),
		"accent":               pair(gray(0.97), gray(0.269)),
		"accent-foreground":    pair(gray(0.205), gray(0.985)),
		"destructive":          pair(ok(0.577, 0.245, 27.325), ok(0.704, 0.191, 22.216)),
		"border":               pair(gray(0.922), gfx.OKLCH(1, 0, 0, 0.1)),
		"input":                pair(gray(0.922), gfx.OKLCH(1, 0, 0, 0.15)),
		"ring":                 pair(gray(0.708), gray(0.556)),
		"chart-1":              pair(ok(0.646, 0.222, 41.116), ok(0.488, 0.243, 264.376)),
		"chart-2":              pair(ok(0.6, 0.118, 184.704), ok(0.696, 0.17, 162.48)),
		"chart-3":              pair(ok(0.398, 0.07, 227.392), ok(0.769, 0.188, 70.08)),
		"chart-4":              pair(ok(0.828, 0.189, 84.429), ok(0.627, 0.265, 303.9)),
		"chart-5":              pair(ok(0.769, 0.188, 70.08), ok(0.645, 0.246, 16.439)),
	}
}

// DefaultTokenNames lists the tokens of the default theme in shadcn/ui's
// order, for documentation and tests.
var DefaultTokenNames = []string{
	"background", "foreground", "card", "card-foreground", "popover", "popover-foreground",
	"primary", "primary-foreground", "secondary", "secondary-foreground", "muted", "muted-foreground",
	"accent", "accent-foreground", "destructive", "border", "input", "ring",
	"chart-1", "chart-2", "chart-3", "chart-4", "chart-5",
}

// cloneTokens copies a token map.
func cloneTokens(m map[string]SchemeColor) map[string]SchemeColor {
	if m == nil {
		return nil
	}
	return maps.Clone(m)
}
