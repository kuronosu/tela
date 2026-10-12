package gfx

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/fmath"
)

// Color is a non-premultiplied, sRGB-encoded color with float32 components.
// The zero value is transparent black.
//
// R, G and B are extended sRGB (CSS Color 4): a color outside the sRGB
// gamut, such as most of the converted OKLCH colors of Tailwind's palette,
// keeps components below 0 or above 1 instead of being gamut-mapped. A is
// meant to be in [0, 1]. Consumers clamp where Chromium does: a solid color
// is painted with every component clipped to [0, 1] (Clamp; the Builder
// does it when it records a command), while gradients interpolate the
// unclipped stop colors and clip each resulting pixel (see LinearGradient).
//
// Blending and compositing happen on sRGB-encoded values (ADR-007), like
// browsers do, so Color values can be written exactly as in CSS.
type Color struct{ R, G, B, A float32 }

// Common colors.
var (
	Transparent = Color{0, 0, 0, 0}
	Black       = Color{0, 0, 0, 1}
	White       = Color{1, 1, 1, 1}
)

// RGBA8 returns the color with the given non-premultiplied 8-bit components.
func RGBA8(r, g, b, a uint8) Color {
	return Color{float32(r) / 255, float32(g) / 255, float32(b) / 255, float32(a) / 255}
}

// RGB8 returns the opaque color with the given 8-bit components.
func RGB8(r, g, b uint8) Color { return RGBA8(r, g, b, 255) }

// ParseHex parses a CSS hex color: "#rgb", "#rgba", "#rrggbb" or "#rrggbbaa"
// (case-insensitive). The leading '#' is required.
func ParseHex(s string) (Color, error) {
	if !strings.HasPrefix(s, "#") {
		return Color{}, fmt.Errorf("gfx: invalid hex color %q: it must start with '#' (for example \"#1e1e2e\" or \"#fff8\")", s)
	}
	digits := s[1:]
	switch len(digits) {
	case 3, 4, 6, 8:
	default:
		return Color{}, fmt.Errorf("gfx: invalid hex color %q: expected 3, 4, 6 or 8 hex digits after '#' (#rgb, #rgba, #rrggbb or #rrggbbaa), got %d", s, len(digits))
	}
	var v [8]uint8
	for i := 0; i < len(digits); i++ {
		d, ok := hexDigit(digits[i])
		if !ok {
			return Color{}, fmt.Errorf("gfx: invalid hex color %q: %q at position %d is not a hex digit (0-9, a-f)", s, digits[i], i+1)
		}
		v[i] = d
	}
	switch len(digits) {
	case 3, 4:
		a := uint8(255)
		if len(digits) == 4 {
			a = v[3] * 17
		}
		return RGBA8(v[0]*17, v[1]*17, v[2]*17, a), nil
	default:
		a := uint8(255)
		if len(digits) == 8 {
			a = v[6]<<4 | v[7]
		}
		return RGBA8(v[0]<<4|v[1], v[2]<<4|v[3], v[4]<<4|v[5], a), nil
	}
}

// MustParseHex is like ParseHex but panics on error. It is meant for
// constants written in source code.
func MustParseHex(s string) Color {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
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

// WithAlpha returns c with its alpha replaced by a.
func (c Color) WithAlpha(a float32) Color { c.A = a; return c }

// MulAlpha returns c with its alpha multiplied by f (Tailwind's "bg-white/15"
// is White.MulAlpha(0.15)).
func (c Color) MulAlpha(f float32) Color { c.A *= f; return c }

// IsOpaque reports whether A >= 1.
func (c Color) IsOpaque() bool { return c.A >= 1 }

// IsFinite reports whether all four components are finite.
func (c Color) IsFinite() bool { return finite(c.R) && finite(c.G) && finite(c.B) && finite(c.A) }

// Clamp returns c with every component clamped to [0, 1]. NaN becomes 0.
// For an extended sRGB color this per-channel clip is what browsers paint.
func (c Color) Clamp() Color {
	return Color{clamp01(c.R), clamp01(c.G), clamp01(c.B), clamp01(c.A)}
}

// Premultiplied returns the premultiplied components (R*A, G*A, B*A, A) as
// floats in [0, 1].
func (c Color) Premultiplied() (r, g, b, a float32) {
	c = c.Clamp()
	return c.R * c.A, c.G * c.A, c.B * c.A, c.A
}

// Premul8 returns the premultiplied 8-bit color, with every channel rounded
// to nearest (half up) from the exact float value, never by premultiplying
// already-rounded bytes. The result satisfies R,G,B <= A.
func (c Color) Premul8() color.RGBA {
	c = c.Clamp()
	return color.RGBA{
		R: round8(float64(c.R) * float64(c.A)),
		G: round8(float64(c.G) * float64(c.A)),
		B: round8(float64(c.B) * float64(c.A)),
		A: round8(float64(c.A)),
	}
}

// NRGBA8 returns the non-premultiplied 8-bit color, rounded to nearest.
func (c Color) NRGBA8() color.NRGBA {
	c = c.Clamp()
	return color.NRGBA{R: round8(float64(c.R)), G: round8(float64(c.G)), B: round8(float64(c.B)), A: round8(float64(c.A))}
}

// String returns "#rrggbb" for opaque colors and "#rrggbbaa" otherwise, with
// components rounded to 8 bits.
func (c Color) String() string {
	n := c.NRGBA8()
	if n.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", n.R, n.G, n.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", n.R, n.G, n.B, n.A)
}

func round8(v float64) uint8 {
	// The product is rounded explicitly so that it cannot be fused with the
	// addition into a multiply-add, which rounds differently on some
	// architectures.
	v = float64(v*255) + 0.5
	switch {
	case !(v > 0):
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v)
}

func clamp01(v float32) float32 {
	switch {
	case !(v > 0):
		return 0
	case v > 1:
		return 1
	}
	return v
}

// ---------------------------------------------------------------------------
// Oklab / OKLCH
// ---------------------------------------------------------------------------

// The conversions below are fusion-safe (ADR-027, ADR-041): every product
// that feeds an addition is rounded explicitly with a float64 conversion and
// the elementary functions come from internal/fmath, so a color converts to
// the same bits on every architecture. They neither clamp nor gamut-map:
// colors outside sRGB come out as extended sRGB.

// Matrices from CSS Color 4 (as published in the spec and in colorjs.io),
// recalculated for a consistent D65 reference white. The two combined
// matrices below are derived from them at init time.
var (
	// linear sRGB -> XYZ D65, and the inverse.
	srgbToXYZ = [3][3]float64{
		{0.41239079926595934, 0.357584339383878, 0.1804807884018343},
		{0.21263900587151027, 0.715168678767756, 0.07219231536073371},
		{0.01933081871559182, 0.11919477979462598, 0.9505321522496607},
	}
	xyzToSRGB = [3][3]float64{
		{3.2409699419045226, -1.537383177570094, -0.4986107602930034},
		{-0.9692436362808796, 1.8759675015077202, 0.04155505740717559},
		{0.05563007969699366, -0.20397695888897652, 1.0569715142428786},
	}
	// XYZ D65 -> LMS, and the inverse.
	xyzToLMS = [3][3]float64{
		{0.8190224379967030, 0.3619062600528904, -0.1288737815209879},
		{0.0329836539323885, 0.9292868615863434, 0.0361446663506424},
		{0.0481771893596242, 0.2642395317527308, 0.6335478284694309},
	}
	lmsToXYZ = [3][3]float64{
		{1.2268798758459243, -0.5578149944602171, 0.2813910456659647},
		{-0.0405757452148008, 1.1122868032803170, -0.0717110580655164},
		{-0.0763729366746601, -0.4214933324022432, 1.5869240198367816},
	}
	// LMS' (cube-rooted) -> Oklab, and the inverse.
	lmsToOklab = [3][3]float64{
		{0.2104542683093140, 0.7936177747023054, -0.0040720430116193},
		{1.9779985324311684, -2.4285922420485799, 0.4505937096174110},
		{0.0259040424655478, 0.7827717124575296, -0.8086757549230774},
	}
	oklabToLMS = [3][3]float64{
		{1.0000000000000000, 0.3963377773761749, 0.2158037573099136},
		{1.0000000000000000, -0.1055613458156586, -0.0638541728258133},
		{1.0000000000000000, -0.0894841775298119, -1.2914855480194092},
	}

	linToLMS = matMul(xyzToLMS, srgbToXYZ) // linear sRGB -> LMS
	lmsToLin = matMul(xyzToSRGB, lmsToXYZ) // LMS -> linear sRGB
)

func matMul(a, b [3][3]float64) (m [3][3]float64) {
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			m[i][j] = float64(a[i][0]*b[0][j]) + float64(a[i][1]*b[1][j]) + float64(a[i][2]*b[2][j])
		}
	}
	return m
}

func matVec(m *[3][3]float64, x, y, z float64) (float64, float64, float64) {
	return float64(m[0][0]*x) + float64(m[0][1]*y) + float64(m[0][2]*z),
		float64(m[1][0]*x) + float64(m[1][1]*y) + float64(m[1][2]*z),
		float64(m[2][0]*x) + float64(m[2][1]*y) + float64(m[2][2]*z)
}

// SRGBToLinear converts an sRGB-encoded component to linear light using the
// piecewise sRGB transfer function. Values outside [0, 1] (extended sRGB)
// are extended symmetrically around zero rather than clamped, as CSS Color 4
// does.
func SRGBToLinear(v float32) float32 { return float32(srgbToLinear(float64(v))) }

// LinearToSRGB converts a linear-light component to its sRGB encoding. It is
// the inverse of SRGBToLinear, also symmetric around zero.
func LinearToSRGB(v float32) float32 { return float32(linearToSRGB(float64(v))) }

func srgbToLinear(v float64) float64 {
	a := math.Abs(v)
	if !(a > 0.04045) { // also NaN
		return v / 12.92
	}
	return math.Copysign(fmath.Pow((a+0.055)/1.055, 2.4), v)
}

func linearToSRGB(v float64) float64 {
	a := math.Abs(v)
	if !(a > 0.0031308) { // also NaN
		return v * 12.92
	}
	return math.Copysign(float64(1.055*fmath.Pow(a, 1/2.4))-0.055, v)
}

func linToOklab(r, g, b float64) (l, a, bb float64) {
	lc, mc, sc := matVec(&linToLMS, r, g, b)
	return matVec(&lmsToOklab, fmath.Cbrt(lc), fmath.Cbrt(mc), fmath.Cbrt(sc))
}

func oklabToLin(l, a, b float64) (r, g, bb float64) {
	l_, m_, s_ := matVec(&oklabToLMS, l, a, b)
	return matVec(&lmsToLin, l_*l_*l_, m_*m_*m_, s_*s_*s_)
}

// LinearSRGBToOklab converts linear-light sRGB (not encoded) to Oklab, with
// the CSS Color 4 matrices. No clamping is applied.
func LinearSRGBToOklab(r, g, b float32) (l, a, bb float32) {
	l64, a64, b64 := linToOklab(float64(r), float64(g), float64(b))
	return float32(l64), float32(a64), float32(b64)
}

// OklabToLinearSRGB converts Oklab to linear-light sRGB without gamut mapping
// or clamping: the result may lie outside [0, 1].
func OklabToLinearSRGB(l, a, b float32) (r, g, bb float32) {
	r64, g64, b64 := oklabToLin(float64(l), float64(a), float64(b))
	return float32(r64), float32(g64), float32(b64)
}

// ToOklab returns the Oklab coordinates of c (alpha is ignored), extended
// sRGB components included. l is in [0, 1] for in-gamut colors; a and b are
// roughly in [-0.4, 0.4].
func (c Color) ToOklab() (l, a, b float32) {
	l64, a64, b64 := linToOklab(
		srgbToLinear(float64(c.R)), srgbToLinear(float64(c.G)), srgbToLinear(float64(c.B)))
	return float32(l64), float32(a64), float32(b64)
}

// ToOKLCH returns the OKLCH coordinates of c (alpha is ignored). The hue h is
// in degrees in [0, 360); it is 0 for achromatic colors.
func (c Color) ToOKLCH() (l, ch, h float32) {
	l64, a, b := linToOklab(
		srgbToLinear(float64(c.R)), srgbToLinear(float64(c.G)), srgbToLinear(float64(c.B)))
	c64, h64 := labToLCH(a, b)
	return float32(l64), float32(c64), float32(h64)
}

func labToLCH(a, b float64) (c, h float64) {
	c = math.Sqrt(float64(a*a) + float64(b*b))
	if c < 1e-6 { // achromatic: the hue is meaningless
		return c, 0
	}
	h = float64(fmath.Atan2(b, a)*180) / math.Pi
	if h < 0 {
		h += 360
	}
	return c, h
}

// Oklab returns the color for the Oklab coordinates (l, a, b) and the given
// alpha. l is lightness, clamped to [0, 1] like CSS does at parse time (CSS
// "100%" is 1); a and b are unbounded.
//
// The result is extended sRGB: a color outside the sRGB gamut keeps its
// components outside [0, 1], without gamut mapping, so that gradients
// interpolate the color itself, like browsers do; see OKLCH.
func Oklab(l, a, b, alpha float32) Color {
	return oklabColor(float64(l), float64(a), float64(b), alpha)
}

// OKLCH returns the color for the OKLCH coordinates: lightness l, clamped to
// [0, 1] (CSS "100%" is 1), chroma c (about 0 to 0.4; negative values are
// taken as 0) and hue h in degrees. This is the notation of the Tailwind v4
// palette, e.g. OKLCH(0.637, 0.237, 25.331, 1) is red-500.
//
// The result is extended sRGB: a color outside the sRGB gamut (89 of the 286
// colors of Tailwind's palette, e.g. fuchsia-600) has components below 0 or
// above 1. No gamut mapping is applied. Painting a solid color clips each
// component to [0, 1] (Color.Clamp), and a gradient interpolates the
// unclipped stops and clips every pixel: both are what Chromium draws.
func OKLCH(l, c, h, alpha float32) Color {
	if !(c > 0) { // also NaN
		c = 0
	}
	sin, cos := fmath.Sincos(float64(h) * (math.Pi / 180))
	return oklabColor(float64(l), float64(float64(c)*cos), float64(float64(c)*sin), alpha)
}

func oklabColor(l, a, b float64, alpha float32) Color {
	if l != l || a != a || b != b {
		return Color{0, 0, 0, alpha}
	}
	l = min(max(l, 0), 1)
	r, g, bb := oklabToLin(l, a, b)
	return Color{float32(linearToSRGB(r)), float32(linearToSRGB(g)), float32(linearToSRGB(bb)), alpha}
}
