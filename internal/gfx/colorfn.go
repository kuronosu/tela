package gfx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ColorFuncProblem says what is wrong with the arguments of a CSS color
// function (see [ColorFuncError]).
type ColorFuncProblem uint8

const (
	// ColorFuncArity: the function does not have three components and an
	// optional alpha.
	ColorFuncArity ColorFuncProblem = iota + 1
	// ColorFuncSyntax: the separators are wrong: commas mixed with spaces
	// or with "/", an empty argument, or an alpha that is not alone after
	// the "/".
	ColorFuncSyntax
	// ColorFuncComponent: a component is not a number or a percentage
	// (or "none" in the space-separated syntax).
	ColorFuncComponent
	// ColorFuncHue: the hue is not a number or an angle.
	ColorFuncHue
	// ColorFuncAlpha: the alpha is not a number or a percentage.
	ColorFuncAlpha
	// ColorFuncPercent: the comma-separated syntax mixes numbers and
	// percentages in rgb(), or has plain numbers for the saturation or
	// lightness of hsl().
	ColorFuncPercent
)

// ColorFuncError is the error of [ParseRGBFunc] and [ParseHSLFunc]. It is
// structured so that a caller can word its own message.
type ColorFuncError struct {
	// Func is "rgb" or "hsl" (rgba() and hsla() are the same functions).
	Func string
	// Problem is what is wrong.
	Problem ColorFuncProblem
	// Arg is the offending argument, for ColorFuncComponent, ColorFuncHue
	// and ColorFuncAlpha.
	Arg string
}

func (e *ColorFuncError) Error() string {
	switch e.Problem {
	case ColorFuncArity:
		return fmt.Sprintf("gfx: %s() takes three components and an optional alpha", e.Func)
	case ColorFuncSyntax:
		return fmt.Sprintf("gfx: %s() separates its arguments with commas or with spaces and a \"/\" before the alpha, not both", e.Func)
	case ColorFuncComponent:
		return fmt.Sprintf("gfx: %s(): invalid color component %q; it takes a number or a percentage", e.Func, e.Arg)
	case ColorFuncHue:
		return fmt.Sprintf("gfx: %s(): invalid hue %q; it takes a number or an angle (deg, grad, rad, turn)", e.Func, e.Arg)
	case ColorFuncAlpha:
		return fmt.Sprintf("gfx: %s(): invalid alpha %q; it takes a number or a percentage", e.Func, e.Arg)
	case ColorFuncPercent:
		if e.Func == "hsl" {
			return "gfx: hsl() with commas takes percentages for the saturation and lightness (hsl(120, 100%, 25%)); without commas plain numbers also work (hsl(120 100 25))"
		}
		return "gfx: rgb() with commas takes three numbers or three percentages, not a mix"
	}
	return "gfx: invalid color function"
}

// ParseRGBFunc parses the arguments of rgb() or rgba(), the text between
// the parentheses, with the grammar of CSS Color 4: the legacy
// comma-separated form "r, g, b" or "r, g, b, a" (three numbers or three
// percentages) and the space-separated form "r g b" or "r g b / a", whose
// components are numbers, percentages or none. Channels are clamped to
// [0, 255] and the alpha to [0, 1].
func ParseRGBFunc(args string) (Color, error) {
	comps, alpha, legacy, err := splitColorFuncArgs("rgb", args)
	if err != nil {
		return Color{}, err
	}
	var ch [3]float32
	pcts := 0
	for i, c := range comps {
		v, pct, ok := colorFuncComponent(c, legacy)
		if !ok {
			return Color{}, &ColorFuncError{Func: "rgb", Problem: ColorFuncComponent, Arg: c}
		}
		if pct {
			pcts++
			v = v * 255 / 100
		}
		ch[i] = float32(min(max(v, 0), 255) / 255)
	}
	if legacy && pcts != 0 && pcts != 3 {
		return Color{}, &ColorFuncError{Func: "rgb", Problem: ColorFuncPercent}
	}
	a, err := colorFuncAlpha("rgb", alpha, legacy)
	if err != nil {
		return Color{}, err
	}
	return Color{R: ch[0], G: ch[1], B: ch[2], A: a}, nil
}

// ParseHSLFunc parses the arguments of hsl() or hsla(), the text between
// the parentheses, with the grammar of CSS Color 4: "h, s%, l%" or
// "h, s%, l%, a" (legacy, the saturation and lightness must be
// percentages) and "h s l" or "h s l / a" (the saturation and lightness
// are percentages or plain numbers of percent, any of them may be none).
// The hue is a number of degrees or has a deg, grad, rad or turn unit.
// Saturation and lightness are clamped to [0, 100]% and the alpha to
// [0, 1].
func ParseHSLFunc(args string) (Color, error) {
	comps, alpha, legacy, err := splitColorFuncArgs("hsl", args)
	if err != nil {
		return Color{}, err
	}
	h, ok := colorFuncHue(comps[0], legacy)
	if !ok {
		return Color{}, &ColorFuncError{Func: "hsl", Problem: ColorFuncHue, Arg: comps[0]}
	}
	var sl [2]float64
	for i, c := range comps[1:] {
		v, pct, ok := colorFuncComponent(c, legacy)
		if !ok {
			return Color{}, &ColorFuncError{Func: "hsl", Problem: ColorFuncComponent, Arg: c}
		}
		if legacy && !pct {
			return Color{}, &ColorFuncError{Func: "hsl", Problem: ColorFuncPercent}
		}
		sl[i] = min(max(v, 0), 100) / 100
	}
	a, err := colorFuncAlpha("hsl", alpha, legacy)
	if err != nil {
		return Color{}, err
	}
	r, g, b := hslToRGB(h, sl[0], sl[1])
	return Color{R: float32(r), G: float32(g), B: float32(b), A: a}, nil
}

// splitColorFuncArgs splits the arguments of a color function into its
// three components and the optional alpha. legacy reports the comma
// syntax.
func splitColorFuncArgs(fn, s string) (comps []string, alpha string, legacy bool, err error) {
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
			if parts[i] == "" || strings.ContainsAny(parts[i], " \t\n/") {
				return nil, "", false, &ColorFuncError{Func: fn, Problem: ColorFuncSyntax}
			}
		}
		switch len(parts) {
		case 3:
			return parts, "", true, nil
		case 4:
			return parts[:3], parts[3], true, nil
		}
		return nil, "", false, &ColorFuncError{Func: fn, Problem: ColorFuncArity}
	}
	main, a, slash := strings.Cut(s, "/")
	comps = strings.Fields(main)
	if len(comps) != 3 {
		return nil, "", false, &ColorFuncError{Func: fn, Problem: ColorFuncArity}
	}
	if slash {
		a = strings.TrimSpace(a)
		if a == "" || strings.ContainsAny(a, " \t\n/") {
			return nil, "", false, &ColorFuncError{Func: fn, Problem: ColorFuncSyntax}
		}
	}
	return comps, a, false, nil
}

// colorFuncComponent parses a number or percentage; pct reports which.
// "none" (the space-separated syntax only) is 0.
func colorFuncComponent(s string, legacy bool) (v float64, pct, ok bool) {
	if !legacy && strings.EqualFold(s, "none") {
		return 0, false, true
	}
	if num, isPct := strings.CutSuffix(s, "%"); isPct {
		v, ok = cssNumber(num)
		return v, true, ok
	}
	v, ok = cssNumber(s)
	return v, false, ok
}

func colorFuncAlpha(fn, s string, legacy bool) (float32, error) {
	if s == "" {
		return 1, nil
	}
	if !legacy && strings.EqualFold(s, "none") {
		return 0, nil
	}
	v, pct, ok := colorFuncComponent(s, true)
	if !ok {
		return 0, &ColorFuncError{Func: fn, Problem: ColorFuncAlpha, Arg: s}
	}
	if pct {
		v /= 100
	}
	return float32(min(max(v, 0), 1)), nil
}

// colorFuncHue parses a hue in degrees (unitless) or with deg, grad, rad
// or turn, and returns it in sextants of the color wheel, in [0, 6).
func colorFuncHue(s string, legacy bool) (float64, bool) {
	if !legacy && strings.EqualFold(s, "none") {
		return 0, true
	}
	ls := strings.ToLower(s)
	scale := 1.0
	for _, u := range []struct {
		suffix string
		deg    float64
	}{{"deg", 1}, {"grad", 0.9}, {"rad", 180 / math.Pi}, {"turn", 360}} {
		if strings.HasSuffix(ls, u.suffix) {
			ls, scale = ls[:len(ls)-len(u.suffix)], u.deg
			break
		}
	}
	v, ok := cssNumber(ls)
	if !ok {
		return 0, false
	}
	deg := math.Mod(float64(v*scale), 360)
	if deg < 0 {
		deg += 360
	}
	return deg / 60, true
}

// cssNumber parses a CSS <number>: an optional sign, digits with an
// optional fraction (or only a fraction) and an optional exponent. It
// rejects what strconv accepts besides that (inf, nan, hex, underscores)
// and values beyond ±1e9.
func cssNumber(s string) (float64, bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		exp := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			exp++
		}
		if exp == 0 {
			return 0, false
		}
	}
	if i != len(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return 0, false
		}
	}
	if math.IsNaN(v) || math.Abs(v) > 1e9 {
		return 0, false
	}
	return v, true
}

// hslToRGB converts HSL (hue in sextants, saturation and lightness in
// [0, 1]) to sRGB with the algorithm of CSS Color 4. Products are wrapped
// in conversions so that no architecture fuses them (ADR-027).
func hslToRGB(h, s, l float64) (r, g, b float64) {
	var t2 float64
	if l <= 0.5 {
		t2 = float64(l * (s + 1))
	} else {
		t2 = l + s - float64(l*s)
	}
	t1 := float64(l*2) - t2
	return hueToRGB(t1, t2, h+2), hueToRGB(t1, t2, h), hueToRGB(t1, t2, h-2)
}

func hueToRGB(t1, t2, h float64) float64 {
	if h < 0 {
		h += 6
	}
	if h >= 6 {
		h -= 6
	}
	switch {
	case h < 1:
		return float64((t2-t1)*h) + t1
	case h < 3:
		return t2
	case h < 4:
		return float64((t2-t1)*(4-h)) + t1
	}
	return t1
}
