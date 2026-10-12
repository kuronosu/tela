package style

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// colorSpec is a color as written in a class, before currentColor is known.
// For a current spec, c.A is the opacity applied to the text color.
type colorSpec struct {
	c       gfx.Color
	current bool
	set     bool
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
	if s.current {
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
	args := splitColorArgs(s[open+1 : len(s)-1])
	switch fn {
	case "rgb", "rgba":
		return parseRGBFunc(args)
	case "hsl", "hsla":
		return parseHSLFunc(args)
	case "oklch":
		return parseOklchFunc(args, true)
	case "oklab":
		return parseOklchFunc(args, false)
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

func parseRGBFunc(args []string) (colorSpec, error) {
	if len(args) < 3 || len(args) > 4 {
		return colorSpec{}, errors.New("rgb() takes three components and an optional alpha: rgb(30 30 46 / 0.5)")
	}
	var v [3]float32
	for i := 0; i < 3; i++ {
		x, err := colorNum(args[i], 255)
		if err != nil {
			return colorSpec{}, err
		}
		v[i] = clampf(x/255, 0, 1)
	}
	a, err := colorAlpha(args[3:])
	if err != nil {
		return colorSpec{}, err
	}
	return solidSpec(gfx.Color{R: v[0], G: v[1], B: v[2], A: a}), nil
}

func parseHSLFunc(args []string) (colorSpec, error) {
	if len(args) < 3 || len(args) > 4 {
		return colorSpec{}, errors.New("hsl() takes hue, saturation, lightness and an optional alpha: hsl(220 60% 50%)")
	}
	h, ok := parseAngle(args[0])
	if !ok {
		var hv float32
		hv, ok = parseFloat(args[0])
		h = hv
	}
	if !ok {
		return colorSpec{}, fmt.Errorf("invalid hue %q", args[0])
	}
	s, err := colorNum(args[1], 100)
	if err != nil {
		return colorSpec{}, err
	}
	l, err := colorNum(args[2], 100)
	if err != nil {
		return colorSpec{}, err
	}
	a, err := colorAlpha(args[3:])
	if err != nil {
		return colorSpec{}, err
	}
	r, g, b := hslToRGB(h, clampf(s/100, 0, 1), clampf(l/100, 0, 1))
	return solidSpec(gfx.Color{R: r, G: g, B: b, A: a}), nil
}

func hslToRGB(h, s, l float32) (r, g, b float32) {
	h = float32(math.Mod(float64(h), 360))
	if h < 0 {
		h += 360
	}
	c := float32((1 - float32(math.Abs(float64(2*l-1)))) * s)
	hp := h / 60
	x := float32(c * (1 - float32(math.Abs(math.Mod(float64(hp), 2)-1))))
	var r1, g1, b1 float32
	switch {
	case hp < 1:
		r1, g1, b1 = c, x, 0
	case hp < 2:
		r1, g1, b1 = x, c, 0
	case hp < 3:
		r1, g1, b1 = 0, c, x
	case hp < 4:
		r1, g1, b1 = 0, x, c
	case hp < 5:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}
	m := l - c/2
	return r1 + m, g1 + m, b1 + m
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
