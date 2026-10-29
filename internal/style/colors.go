package style

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// colorSpec is a color as written in a class, before currentColor and the
// color scheme are known. For a current spec, c.A is the opacity applied
// to the text color; for a token spec, the opacity applied to the token's
// value (bind resolves it).
type colorSpec struct {
	c       gfx.Color
	current bool
	set     bool
	token   string // a semantic token of the theme ("primary")
}

func tokenSpec(name string) colorSpec {
	return colorSpec{c: gfx.Color{A: 1}, token: name, set: true}
}

// bind resolves a token spec for the color scheme (ADR-089): its value in
// th, faded by the opacity modifier. Other specs are returned unchanged. A
// token th does not have (a class compiled against another theme) is
// transparent.
func (s colorSpec) bind(th *Theme, dark bool) colorSpec {
	if s.token == "" {
		return s
	}
	c, _ := th.TokenColor(s.token, dark)
	return colorSpec{c: c.MulAlpha(s.c.A), set: true}
}

func solidSpec(c gfx.Color) colorSpec { return colorSpec{c: c, set: true} }

func currentSpec(alpha float32) colorSpec {
	return colorSpec{c: gfx.Color{A: alpha}, current: true, set: true}
}

// resolve returns the color given the element's text color.
func (s colorSpec) resolve(text gfx.Color) gfx.Color {
	if s.current {
		return text.MulAlpha(s.c.A)
	}
	return s.c
}

func (s colorSpec) withAlpha(a float32) colorSpec {
	if s.current || s.token != "" {
		s.c.A = float32(s.c.A * a)
		return s
	}
	s.c = s.c.MulAlpha(a)
	return s
}

// themeColor looks up a color class value such as "red-500", "white",
// "transparent" or "current" (and any custom semantic key of the theme).
func themeColor(th *Theme, name string) (colorSpec, bool) {
	switch name {
	case "transparent":
		return solidSpec(gfx.Transparent), true
	case "current":
		return currentSpec(1), true
	}
	if c, ok := th.Colors[name]; ok {
		return solidSpec(c), true
	}
	if _, ok := th.Tokens[name]; ok {
		return tokenSpec(name), true // resolved by Compute (tokens.go)
	}
	return colorSpec{}, false
}

// parseColor parses an arbitrary CSS color: #hex, rgb(), rgba(), hsl(),
// hsla(), oklch(), oklab(), transparent and currentColor.
func parseColor(s string) (colorSpec, error) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	switch low {
	case "transparent":
		return solidSpec(gfx.Transparent), nil
	case "currentcolor", "current":
		return currentSpec(1), nil
	}
	if strings.HasPrefix(s, "#") {
		c, err := gfx.ParseHex(s)
		if err != nil {
			return colorSpec{}, err
		}
		return solidSpec(c), nil
	}
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.HasSuffix(s, ")") {
		if isIdent(low) {
			return colorSpec{}, fmt.Errorf("the color name %q is not supported; use a theme color (bg-red-500) or a hex value (bg-[#ef4444])", s)
		}
		return colorSpec{}, errNotColor
	}
	fn := strings.TrimSpace(low[:open])
	inner := s[open+1 : len(s)-1]
	switch fn {
	case "rgb", "rgba":
		c, err := gfx.ParseRGBFunc(inner)
		if err != nil {
			return colorSpec{}, colorFuncError(err)
		}
		return solidSpec(c), nil
	case "hsl", "hsla":
		c, err := gfx.ParseHSLFunc(inner)
		if err != nil {
			return colorSpec{}, colorFuncError(err)
		}
		return solidSpec(c), nil
	case "oklch":
		return parseOklchFunc(splitColorArgs(inner), true)
	case "oklab":
		return parseOklchFunc(splitColorArgs(inner), false)
	case "var":
		return colorSpec{}, errors.New("CSS variables are not supported; use a theme token or a literal color")
	case "color-mix", "color", "lab", "lch", "hwb", "light-dark":
		return colorSpec{}, fmt.Errorf("the color function %s() is not supported; use rgb(), hsl(), oklch(), oklab() or a hex value", fn)
	}
	return colorSpec{}, errNotColor
}

var errNotColor = errors.New("not a color")

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c == '-' && i > 0) {
			return false
		}
	}
	return true
}

// looksLikeColor reports whether an arbitrary value is a color, for type
// inference.
func looksLikeColor(s string) bool {
	if strings.HasPrefix(s, "#") {
		return true
	}
	for _, p := range []string{"rgb(", "rgba(", "hsl(", "hsla(", "oklch(", "oklab(", "lab(", "lch(", "hwb(", "color(", "color-mix("} {
		if strings.HasPrefix(strings.ToLower(s), p) {
			return true
		}
	}
	switch strings.ToLower(s) {
	case "transparent", "currentcolor", "current":
		return true
	}
	return false
}

// splitColorArgs splits "0 0 0 / 50%" or "0, 0, 0, 0.5" into components.
func splitColorArgs(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '/' || r == '\t' })
	return f
}

// colorNum parses a number or a percentage; pct is the value of 100%.
func colorNum(s string, pct float32) (float32, error) {
	if strings.HasSuffix(s, "%") {
		v, ok := parseFloat(s[:len(s)-1])
		if !ok {
			return 0, fmt.Errorf("invalid color component %q", s)
		}
		return float32(v * pct / 100), nil
	}
	v, ok := parseFloat(s)
	if !ok {
		return 0, fmt.Errorf("invalid color component %q", s)
	}
	return v, nil
}

func colorAlpha(args []string) (float32, error) {
	if len(args) == 0 {
		return 1, nil
	}
	a, err := colorNum(args[0], 1)
	if err != nil {
		return 0, err
	}
	return clampf(a, 0, 1), nil
}

func clampf(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

// colorFuncError words the error of gfx.ParseRGBFunc and gfx.ParseHSLFunc
// (CSS Color 4 syntax, ADR-193) for a class.
func colorFuncError(err error) error {
	var ce *gfx.ColorFuncError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Problem {
	case gfx.ColorFuncArity:
		if ce.Func == "hsl" {
			return errors.New("hsl() takes hue, saturation, lightness and an optional alpha: hsl(220 60% 50%)")
		}
		return errors.New("rgb() takes three components and an optional alpha: rgb(30 30 46 / 0.5)")
	case gfx.ColorFuncSyntax:
		return fmt.Errorf("%s() separates its arguments with commas (%s(30, 30, 46, 0.5)) or with spaces and a \"/\" before the alpha (%s(30 30 46 / 0.5)), not both", ce.Func, ce.Func, ce.Func)
	case gfx.ColorFuncHue:
		return fmt.Errorf("invalid hue %q", ce.Arg)
	case gfx.ColorFuncComponent, gfx.ColorFuncAlpha:
		return fmt.Errorf("invalid color component %q", ce.Arg)
	case gfx.ColorFuncPercent:
		if ce.Func == "hsl" {
			return errors.New("hsl() with commas takes percentages for the saturation and lightness: hsl(120, 100%, 25%); without commas plain numbers also work: hsl(120 100 25)")
		}
		return errors.New("rgb() with commas takes three numbers or three percentages, not a mix: rgb(255, 128, 0) or rgb(100%, 50%, 0%)")
	}
	return err
}

func parseOklchFunc(args []string, lch bool) (colorSpec, error) {
	name := "oklab"
	if lch {
		name = "oklch"
	}
	if len(args) < 3 || len(args) > 4 {
		return colorSpec{}, fmt.Errorf("%s() takes three components and an optional alpha, for example %s(0.7 0.15 250 / 0.5)", name, name)
	}
	l, err := colorNum(args[0], 1)
	if err != nil {
		return colorSpec{}, err
	}
	a, err := colorAlpha(args[3:])
	if err != nil {
		return colorSpec{}, err
	}
	if lch {
		c, err := colorNum(args[1], 0.4)
		if err != nil {
			return colorSpec{}, err
		}
		h, ok := parseAngle(args[2])
		if !ok {
			h, ok = parseFloat(args[2])
		}
		if !ok {
			return colorSpec{}, fmt.Errorf("invalid hue %q", args[2])
		}
		return solidSpec(gfx.OKLCH(l, c, h, a)), nil
	}
	x, err := colorNum(args[1], 0.4)
	if err != nil {
		return colorSpec{}, err
	}
	y, err := colorNum(args[2], 0.4)
	if err != nil {
		return colorSpec{}, err
	}
	return solidSpec(gfx.Oklab(l, x, y, a)), nil
}
