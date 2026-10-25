package aria

import (
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/gfx"
)

// HSB is a color in the hue, saturation and brightness model (HSV, what a
// color picker's area shows). H is in degrees, from 0 to 360; S and B are
// percentages, from 0 to 100; A is the opacity, from 0 to 1.
//
// The model keeps what RGB forgets: the hue of a gray (S is 0) and the hue
// and saturation of black (B is 0). That is why the color hooks keep an
// HSB in their [ColorState] and not the [tela.Color] they report: a drag
// that passes through the gray axis does not lose the hue.
type HSB struct{ H, S, B, A float64 }

// HSL is a color in the hue, saturation and lightness model. H is in
// degrees, from 0 to 360; S and L are percentages, from 0 to 100; A is the
// opacity, from 0 to 1. Its saturation is not [HSB]'s.
type HSL struct{ H, S, L, A float64 }

// Color returns c as a [tela.Color] in the sRGB gamut. A hue of 360 is the
// hue of 0. Values out of range are clamped.
func (c HSB) Color() tela.Color {
	s, v := clampTo(c.S, 0, 100)/100, clampTo(c.B, 0, 100)/100
	return hsvToColor(c.H, s, v, c.A)
}

// Color returns c as a [tela.Color] in the sRGB gamut. A hue of 360 is the
// hue of 0. Values out of range are clamped.
func (c HSL) Color() tela.Color {
	s, l := clampTo(c.S, 0, 100)/100, clampTo(c.L, 0, 100)/100
	v := l + s*math.Min(l, 1-l)
	sv := 0.0
	if v > 0 {
		sv = 2 * (1 - l/v)
	}
	return hsvToColor(c.H, sv, v, c.A)
}

// HSB returns the same color in the HSB model.
func (c HSL) HSB() HSB {
	s, l := clampTo(c.S, 0, 100)/100, clampTo(c.L, 0, 100)/100
	v := l + s*math.Min(l, 1-l)
	sv := 0.0
	if v > 0 {
		sv = 2 * (1 - l/v)
	}
	return HSB{H: normalizeHue(c.H, 360), S: sv * 100, B: v * 100, A: clampTo(c.A, 0, 1)}
}

// HSL returns the same color in the HSL model.
func (c HSB) HSL() HSL {
	s, v := clampTo(c.S, 0, 100)/100, clampTo(c.B, 0, 100)/100
	l := v * (1 - s/2)
	sl := 0.0
	if l > 0 && l < 1 {
		sl = (v - l) / math.Min(l, 1-l)
	}
	return HSL{H: normalizeHue(c.H, 360), S: sl * 100, L: l * 100, A: clampTo(c.A, 0, 1)}
}

// ToHSB returns c in the HSB model. The color is clamped to the sRGB gamut
// first. The hue of a gray is 0 (RGB does not have one): a caller that
// holds a hue keeps it itself, as [ColorState] does.
func ToHSB(c tela.Color) HSB {
	r, g, b := channel01(c.R), channel01(c.G), channel01(c.B)
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := hi - lo
	out := HSB{B: hi * 100, A: channel01(c.A)}
	if hi > 0 {
		out.S = d / hi * 100
	}
	if d == 0 {
		return out
	}
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	out.H = normalizeHue(h*60, 360)
	return out
}

// ToHSL returns c in the HSL model, with the clamping and the hue of
// [ToHSB].
func ToHSL(c tela.Color) HSL { return ToHSB(c).HSL() }

// hsvToColor is the HSV to RGB conversion of the CSS Color specification:
// h in degrees, s and v from 0 to 1.
func hsvToColor(h, s, v, a float64) tela.Color {
	h = normalizeHue(h, 360) / 60
	i := math.Floor(h)
	f := h - i
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return tela.Color{R: float32(r), G: float32(g), B: float32(b), A: float32(clampTo(a, 0, 1))}
}

// normalizeHue brings a hue into [0, 360): a full turn is the hue of 0.
// NaN is 0.
func normalizeHue(h, turn float64) float64 {
	if math.IsNaN(h) || math.IsInf(h, 0) {
		return 0
	}
	h = math.Mod(h, turn)
	if h < 0 {
		h += turn
	}
	return h
}

func clampTo(v, lo, hi float64) float64 {
	switch {
	case math.IsNaN(v):
		return lo
	case v < lo:
		return lo
	case v > hi:
		return hi
	}
	return v
}

func channel01(v float32) float64 { return clampTo(float64(v), 0, 1) }

// ParseHexColor parses a hex color as people type it: "#rgb", "#rgba",
// "#rrggbb" or "#rrggbbaa", in any case, with or without the leading "#"
// and with surrounding spaces ignored. The error says what is wrong and
// is meant to be shown (or the input reverted).
//
//	c, err := aria.ParseHexColor("#1e1e2e")
func ParseHexColor(s string) (tela.Color, error) {
	in := strings.TrimSpace(s)
	digits := strings.TrimPrefix(in, "#")
	switch len(digits) {
	case 3, 4, 6, 8:
	case 0:
		return tela.Color{}, fmt.Errorf("aria: invalid hex color %q: it is empty; write #rgb, #rrggbb or #rrggbbaa", s)
	default:
		return tela.Color{}, fmt.Errorf("aria: invalid hex color %q: expected 3, 4, 6 or 8 hex digits (#rgb, #rgba, #rrggbb or #rrggbbaa), got %d", s, len(digits))
	}
	for i := 0; i < len(digits); i++ {
		if _, ok := hexDigit(digits[i]); !ok {
			return tela.Color{}, fmt.Errorf("aria: invalid hex color %q: %q is not a hex digit (0-9, a-f)", s, digits[i])
		}
	}
	// The input is valid by now, whatever gfx's own messages would say.
	return gfx.ParseHex("#" + digits)
}

func hexDigit(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// FormatHexColor returns c as "#rrggbb", in lower case with the components
// rounded to 8 bits and clamped to the sRGB gamut; with alpha it is
// "#rrggbbaa". [ParseHexColor] reads it back.
func FormatHexColor(c tela.Color, alpha bool) string {
	if alpha {
		str := c.String()
		if len(str) == len("#rrggbb") {
			str += "ff"
		}
		return str
	}
	c.A = 1
	return c.String()
}
