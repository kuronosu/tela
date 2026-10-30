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
	// ColorFuncSyntax: the separators are wrong in a function that has a
	// comma form (rgb(), hsl()): commas mixed with spaces or with "/", or
	// an empty argument.
	ColorFuncSyntax
	// ColorFuncCommas: the function has only the space-separated form
	// (oklch(), oklab()) and its arguments have commas.
	ColorFuncCommas
	// ColorFuncSlash: the "/" is not followed by exactly one alpha: the
	// alpha is missing or there is more than one argument after it.
	ColorFuncSlash
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

// ColorFuncError is the error of [ParseRGBFunc], [ParseHSLFunc],
// [ParseOKLCHFunc] and [ParseOklabFunc]. It is structured so that a caller
// can word its own message.
type ColorFuncError struct {
	// Func is "rgb", "hsl", "oklch" or "oklab" (rgba() and hsla() are the
	// same functions as rgb() and hsl()).
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
	case ColorFuncCommas:
		return fmt.Sprintf("gfx: %s() separates its arguments with spaces and a \"/\" before the alpha, not with commas", e.Func)
	case ColorFuncSlash:
		return fmt.Sprintf("gfx: %s() takes exactly one alpha after the \"/\"", e.Func)
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
//
// It rejects mixed separators ("1, 2 3", "1 2 3, 0.5", "1,2,3 / 0.5": a
// [ColorFuncSyntax] error), an empty argument ("1,,3"), a "/" without
// exactly one alpha after it ([ColorFuncSlash]), none in the comma form,
// and numbers mixed with percentages in the comma form
// ([ColorFuncPercent]).
func ParseRGBFunc(args string) (Color, error) {
	comps, alpha, legacy, err := splitColorFuncArgs("rgb", true, args)
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
//
// It rejects the same separators as [ParseRGBFunc], and plain numbers for
// the saturation or lightness in the comma form ("120,100,25": a
// [ColorFuncPercent] error; "120 100 25" is valid).
func ParseHSLFunc(args string) (Color, error) {
	comps, alpha, legacy, err := splitColorFuncArgs("hsl", true, args)
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

// ParseOKLCHFunc parses the arguments of oklch(), the text between the
// parentheses, with the grammar of CSS Color 4: "l c h" or "l c h / a",
// separated by spaces (oklch() has no comma form). The lightness is a
// number in [0, 1] or a percentage (100% is 1), the chroma a number or a
// percentage (100% is 0.4), the hue a number of degrees or has a deg,
// grad, rad or turn unit, and any of them may be none. The alpha is
// clamped to [0, 1]. The hue is wrapped into [0, 360) in float64.
//
// It rejects commas ([ColorFuncCommas]), a missing "/" before the alpha
// ("0.5 0.1 200 0.5": [ColorFuncArity]) and an empty or doubled alpha
// ("0.5 0.1 200 /", "0.5 0.1 200 / 1 2": [ColorFuncSlash]).
func ParseOKLCHFunc(args string) (Color, error) {
	comps, alpha, _, err := splitColorFuncArgs("oklch", false, args)
	if err != nil {
		return Color{}, err
	}
	l, err := oklabComponent("oklch", comps[0], 1)
	if err != nil {
		return Color{}, err
	}
	c, err := oklabComponent("oklch", comps[1], 0.4)
	if err != nil {
		return Color{}, err
	}
	h, ok := colorFuncDegrees(comps[2], false)
	if !ok {
		return Color{}, &ColorFuncError{Func: "oklch", Problem: ColorFuncHue, Arg: comps[2]}
	}
	a, err := colorFuncAlpha("oklch", alpha, false)
	if err != nil {
		return Color{}, err
	}
	return OKLCH(l, c, float32(normalizeDegrees(h)), a), nil
}

// ParseOklabFunc parses the arguments of oklab(), the text between the
// parentheses, with the grammar of CSS Color 4: "l a b" or "l a b / a",
// separated by spaces (oklab() has no comma form). The lightness is a
// number in [0, 1] or a percentage (100% is 1), the other two components
// numbers or percentages (100% is 0.4), and any of them may be none. The
// alpha is clamped to [0, 1].
//
// It rejects the same forms as [ParseOKLCHFunc], and an angle in a or b.
func ParseOklabFunc(args string) (Color, error) {
	comps, alpha, _, err := splitColorFuncArgs("oklab", false, args)
	if err != nil {
		return Color{}, err
	}
	l, err := oklabComponent("oklab", comps[0], 1)
	if err != nil {
		return Color{}, err
	}
	a, err := oklabComponent("oklab", comps[1], 0.4)
	if err != nil {
		return Color{}, err
	}
	b, err := oklabComponent("oklab", comps[2], 0.4)
	if err != nil {
		return Color{}, err
	}
	alp, err := colorFuncAlpha("oklab", alpha, false)
	if err != nil {
		return Color{}, err
	}
	return Oklab(l, a, b, alp), nil
}

// oklabComponent parses a number or a percentage of an oklch() or oklab()
// component; pct is the value of 100%.
func oklabComponent(fn, s string, pct float64) (float32, error) {
	v, isPct, ok := colorFuncComponent(s, false)
	if !ok {
		return 0, &ColorFuncError{Func: fn, Problem: ColorFuncComponent, Arg: s}
	}
	if isPct {
		v = float64(v*pct) / 100
	}
	return float32(v), nil
}

// splitColorFuncArgs splits the arguments of a color function into its
// three components and the optional alpha. allowCommas says whether the
// function has the comma syntax (rgb(), hsl()); legacy reports that the
// arguments used it.
func splitColorFuncArgs(fn string, allowCommas bool, s string) (comps []string, alpha string, legacy bool, err error) {
	if strings.Contains(s, ",") {
		if !allowCommas {
			return nil, "", false, &ColorFuncError{Func: fn, Problem: ColorFuncCommas}
		}
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
			return nil, "", false, &ColorFuncError{Func: fn, Problem: ColorFuncSlash}
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
	deg, ok := colorFuncDegrees(s, legacy)
	if !ok {
		return 0, false
	}
	return normalizeDegrees(deg) / 60, true
}

// normalizeDegrees wraps an angle into [0, 360) in float64, before any
// narrowing to float32 loses the fraction of a huge angle.
func normalizeDegrees(deg float64) float64 {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}
	if deg >= 360 { // -1e-17 + 360 rounds to 360
		deg = 0
	}
	return deg
}

// colorFuncDegrees parses a hue in degrees (unitless) or with deg, grad,
// rad or turn, and returns it in degrees, not normalized. "none" (the
// space-separated syntax only) is 0.
func colorFuncDegrees(s string, legacy bool) (float64, bool) {
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
	return float64(v * scale), true
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
