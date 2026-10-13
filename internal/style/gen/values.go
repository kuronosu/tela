package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// Every value parser returns a Go expression for the value, valid inside
// package style, so the generated file is plain composite literals.

// floatLit formats v so that the Go literal converts back to exactly v.
func floatLit(v float32) string {
	return strconv.FormatFloat(float64(v), 'g', -1, 32)
}

func parseNumber(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return v, nil
}

// splitUnit splits "0.25rem" into 0.25 and "rem".
func splitUnit(s string) (float64, string, error) {
	i := len(s)
	for i > 0 && (s[i-1] >= 'a' && s[i-1] <= 'z' || s[i-1] == '%') {
		i--
	}
	v, err := parseNumber(s[:i])
	return v, s[i:], err
}

// lengthExpr parses a length in px, rem or em (a bare 0 is 0px).
func lengthExpr(s string) (string, error) {
	v, unit, err := splitUnit(s)
	if err != nil {
		return "", fmt.Errorf("length %q: %v", s, err)
	}
	switch unit {
	case "px", "":
		if unit == "" && v != 0 {
			return "", fmt.Errorf("length %q has no unit", s)
		}
		return fmt.Sprintf("Px(%s)", floatLit(float32(v))), nil
	case "rem":
		return fmt.Sprintf("Rem(%s)", floatLit(float32(v))), nil
	case "em":
		return fmt.Sprintf("Em(%s)", floatLit(float32(v))), nil
	}
	return "", fmt.Errorf("length %q: unit %q is not supported (px, rem and em are); extend Length and the generator first", s, unit)
}

// pxValue parses a length that must be in px and returns its value.
func pxValue(s string) (float32, error) {
	v, unit, err := splitUnit(s)
	if err != nil {
		return 0, fmt.Errorf("length %q: %v", s, err)
	}
	if unit != "px" && !(unit == "" && v == 0) {
		return 0, fmt.Errorf("length %q must be in px: shadow offsets are logical pixels", s)
	}
	return float32(v), nil
}

// ratio parses a number or "calc(a / b)" into a float32 (the division is
// done in float64 before rounding, like a browser's calc()).
func ratio(s string) (float32, error) {
	if inner, ok := strings.CutPrefix(s, "calc("); ok {
		inner, ok = strings.CutSuffix(inner, ")")
		if !ok {
			return 0, fmt.Errorf("unbalanced calc() in %q", s)
		}
		s = inner
	}
	if a, b, ok := strings.Cut(s, "/"); ok {
		x, err := parseNumber(strings.TrimSpace(a))
		if err != nil {
			return 0, err
		}
		y, err := parseNumber(strings.TrimSpace(b))
		if err != nil {
			return 0, err
		}
		if y == 0 {
			return 0, fmt.Errorf("division by zero in %q", s)
		}
		return float32(x / y), nil
	}
	v, err := parseNumber(s)
	return float32(v), err
}

func numberExpr(s string) (string, error) {
	v, err := ratio(s)
	return floatLit(v), err
}

// parseColor parses oklch(), rgb() and hex colors. OKLCH goes through
// gfx.OKLCH, which keeps out-of-gamut colors in extended sRGB (components
// outside [0, 1]); painting clips them like Chromium does.
func parseColor(s string) (gfx.Color, error) {
	switch {
	case strings.HasPrefix(s, "#"):
		return gfx.ParseHex(s)
	case strings.HasPrefix(s, "oklch("):
		args, err := colorArgs(s, "oklch(")
		if err != nil {
			return gfx.Color{}, err
		}
		if len(args.channels) != 3 {
			return gfx.Color{}, fmt.Errorf("color %q: oklch() takes 3 components", s)
		}
		l, err := component(args.channels[0], 1)
		if err != nil {
			return gfx.Color{}, fmt.Errorf("color %q: %v", s, err)
		}
		c, err := component(args.channels[1], 0.4)
		if err != nil {
			return gfx.Color{}, fmt.Errorf("color %q: %v", s, err)
		}
		h, err := hue(args.channels[2])
		if err != nil {
			return gfx.Color{}, fmt.Errorf("color %q: %v", s, err)
		}
		return gfx.OKLCH(float32(l), float32(c), float32(h), args.alpha), nil
	case strings.HasPrefix(s, "rgb("):
		args, err := colorArgs(s, "rgb(")
		if err != nil {
			return gfx.Color{}, err
		}
		if len(args.channels) != 3 {
			return gfx.Color{}, fmt.Errorf("color %q: rgb() takes 3 components", s)
		}
		var rgb [3]float32
		for i, ch := range args.channels {
			v, err := component(ch, 255)
			if err != nil {
				return gfx.Color{}, fmt.Errorf("color %q: %v", s, err)
			}
			rgb[i] = float32(v / 255)
		}
		return gfx.Color{R: rgb[0], G: rgb[1], B: rgb[2], A: args.alpha}, nil
	}
	return gfx.Color{}, fmt.Errorf("color %q: only oklch(), rgb() and #hex are supported", s)
}

type colorArguments struct {
	channels []string
	alpha    float32
}

// colorArgs splits "fn(a b c / alpha)" (also the comma form) into its
// channels and alpha.
func colorArgs(s, prefix string) (colorArguments, error) {
	inner, ok := strings.CutSuffix(strings.TrimPrefix(s, prefix), ")")
	if !ok {
		return colorArguments{}, fmt.Errorf("color %q: missing ')'", s)
	}
	args := colorArguments{alpha: 1}
	if body, alpha, ok := strings.Cut(inner, "/"); ok {
		inner = body
		a, err := component(strings.TrimSpace(alpha), 1)
		if err != nil {
			return args, fmt.Errorf("color %q: alpha: %v", s, err)
		}
		args.alpha = float32(a)
	}
	inner = strings.ReplaceAll(inner, ",", " ")
	args.channels = strings.Fields(inner)
	return args, nil
}

// component parses a number or a percentage of full, and "none" as 0.
func component(s string, full float64) (float64, error) {
	if s == "none" {
		return 0, nil
	}
	if p, ok := strings.CutSuffix(s, "%"); ok {
		v, err := parseNumber(p)
		return v / 100 * full, err
	}
	return parseNumber(s)
}

func hue(s string) (float64, error) {
	if s == "none" {
		return 0, nil
	}
	return parseNumber(strings.TrimSuffix(s, "deg"))
}

func colorExpr(s string) (string, error) {
	c, err := parseColor(s)
	if err != nil {
		return "", err
	}
	return colorLit(c), nil
}

func colorLit(c gfx.Color) string {
	return fmt.Sprintf("gfx.Color{R: %s, G: %s, B: %s, A: %s}",
		floatLit(c.R), floatLit(c.G), floatLit(c.B), floatLit(c.A))
}

// shadowExpr parses a comma-separated list of shadow layers:
// [inset] x y [blur [spread]] color, in any order of inset and color.
func shadowExpr(s string) (string, error) {
	var layers []string
	for _, layer := range splitTop(s, ',') {
		var (
			inset   bool
			hasCol  bool
			col     gfx.Color
			lengths []float32
		)
		for _, tok := range fieldsTop(layer) {
			switch {
			case tok == "inset":
				inset = true
			case strings.HasPrefix(tok, "rgb(") || strings.HasPrefix(tok, "oklch(") || strings.HasPrefix(tok, "#"):
				c, err := parseColor(tok)
				if err != nil {
					return "", err
				}
				col, hasCol = c, true
			default:
				v, err := pxValue(tok)
				if err != nil {
					return "", fmt.Errorf("shadow %q: %v", layer, err)
				}
				lengths = append(lengths, v)
			}
		}
		if !hasCol {
			return "", fmt.Errorf("shadow %q has no color; add one or teach the generator currentColor", layer)
		}
		if len(lengths) < 2 || len(lengths) > 4 {
			return "", fmt.Errorf("shadow %q has %d lengths, want 2 to 4", layer, len(lengths))
		}
		for len(lengths) < 4 {
			lengths = append(lengths, 0)
		}
		var f []string
		add := func(name string, v float32) {
			if v != 0 {
				f = append(f, name+": "+floatLit(v))
			}
		}
		add("OffsetX", lengths[0])
		add("OffsetY", lengths[1])
		add("Blur", lengths[2])
		add("Spread", lengths[3])
		f = append(f, "Color: "+colorLit(col))
		if inset {
			f = append(f, "Inset: true")
		}
		layers = append(layers, "{"+strings.Join(f, ", ")+"}")
	}
	return "{" + strings.Join(layers, ", ") + "}", nil
}

// cubicBezier parses cubic-bezier(x1, y1, x2, y2) into its four numbers.
func cubicBezier(s string) ([4]float32, error) {
	var out [4]float32
	inner, ok := strings.CutPrefix(s, "cubic-bezier(")
	if ok {
		inner, ok = strings.CutSuffix(inner, ")")
	}
	if !ok {
		return out, fmt.Errorf("easing %q: only cubic-bezier() is supported", s)
	}
	parts := splitTop(inner, ',')
	if len(parts) != 4 {
		return out, fmt.Errorf("easing %q: cubic-bezier() takes 4 numbers", s)
	}
	for i, p := range parts {
		v, err := parseNumber(p)
		if err != nil {
			return out, fmt.Errorf("easing %q: %v", s, err)
		}
		out[i] = float32(v)
	}
	return out, nil
}

func cubicBezierLit(b [4]float32) string {
	return fmt.Sprintf("CubicBezier{X1: %s, Y1: %s, X2: %s, Y2: %s}",
		floatLit(b[0]), floatLit(b[1]), floatLit(b[2]), floatLit(b[3]))
}

func easeExpr(s string) (string, error) {
	b, err := cubicBezier(s)
	if err != nil {
		return "", err
	}
	return cubicBezierLit(b), nil
}

// durationExpr parses a CSS time ("150ms", "1s") into a Go expression.
func durationExpr(s string) (string, error) {
	v, unit, err := splitUnit(s)
	if err != nil {
		return "", fmt.Errorf("duration %q: %v", s, err)
	}
	var ms float64
	switch unit {
	case "ms":
		ms = v
	case "s":
		ms = v * 1000
	default:
		return "", fmt.Errorf("duration %q: unit must be ms or s", s)
	}
	if ms != math.Trunc(ms) {
		return "", fmt.Errorf("duration %q is not a whole number of milliseconds", s)
	}
	return fmt.Sprintf("%d * time.Millisecond", int64(ms)), nil
}

func stringsLit(fams []string) string {
	q := make([]string, len(fams))
	for i, f := range fams {
		q[i] = strconv.Quote(f)
	}
	return "{" + strings.Join(q, ", ") + "}"
}
