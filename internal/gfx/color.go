package gfx

import (
	"fmt"
	"image/color"
	"math"
	"strings"
)

// Color is a non-premultiplied, sRGB-encoded color with float32 components
// in [0, 1]. The zero value is transparent black.
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
			m[i][j] = a[i][0]*b[0][j] + a[i][1]*b[1][j] + a[i][2]*b[2][j]
		}
	}
	return m
}

// SRGBToLinear converts an sRGB-encoded component to linear light using the
// piecewise sRGB transfer function. Values outside [0, 1] are extended
// symmetrically around zero rather than clamped.
func SRGBToLinear(v float32) float32 { return float32(srgbToLinear(float64(v))) }

// LinearToSRGB converts a linear-light component to its sRGB encoding. It is
// the inverse of SRGBToLinear.
func LinearToSRGB(v float32) float32 { return float32(linearToSRGB(float64(v))) }

func srgbToLinear(v float64) float64 {
	a := math.Abs(v)
	if a <= 0.04045 {
		return v / 12.92
	}
	return math.Copysign(math.Pow((a+0.055)/1.055, 2.4), v)
}

func linearToSRGB(v float64) float64 {
	a := math.Abs(v)
	if a <= 0.0031308 {
		return v * 12.92
	}
	return math.Copysign(1.055*math.Pow(a, 1/2.4)-0.055, v)
}

func linToOklab(r, g, b float64) (l, a, bb float64) {
	m := &linToLMS
	l_ := math.Cbrt(m[0][0]*r + m[0][1]*g + m[0][2]*b)
	m_ := math.Cbrt(m[1][0]*r + m[1][1]*g + m[1][2]*b)
	s_ := math.Cbrt(m[2][0]*r + m[2][1]*g + m[2][2]*b)
	o := &lmsToOklab
	return o[0][0]*l_ + o[0][1]*m_ + o[0][2]*s_,
		o[1][0]*l_ + o[1][1]*m_ + o[1][2]*s_,
		o[2][0]*l_ + o[2][1]*m_ + o[2][2]*s_
}

func oklabToLin(l, a, b float64) (r, g, bb float64) {
	o := &oklabToLMS
	l_ := o[0][0]*l + o[0][1]*a + o[0][2]*b
	m_ := o[1][0]*l + o[1][1]*a + o[1][2]*b
	s_ := o[2][0]*l + o[2][1]*a + o[2][2]*b
	l3, m3, s3 := l_*l_*l_, m_*m_*m_, s_*s_*s_
	m := &lmsToLin
	return m[0][0]*l3 + m[0][1]*m3 + m[0][2]*s3,
		m[1][0]*l3 + m[1][1]*m3 + m[1][2]*s3,
		m[2][0]*l3 + m[2][1]*m3 + m[2][2]*s3
}

// LinearSRGBToOklab converts linear-light sRGB (not encoded) to Oklab, with
// the CSS Color 4 matrices. No clamping is applied. Gradient rasterizers use
// it, together with OklabToLinearSRGB, to interpolate in Oklab.
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

// ToOklab returns the Oklab coordinates of c (alpha is ignored). l is in
// [0, 1] for in-gamut colors; a and b are roughly in [-0.4, 0.4].
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
	c = math.Hypot(a, b)
	if c < 1e-6 { // achromatic: the hue is meaningless
		return c, 0
	}
	h = math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return c, h
}

// Oklab returns the sRGB color for the Oklab coordinates (l, a, b) and the
// given alpha. l is lightness in [0, 1] (CSS "100%" is 1).
//
// If the color is outside the sRGB gamut it is mapped into it with the CSS
// Color 4 gamut-mapping algorithm; see OKLCH.
func Oklab(l, a, b, alpha float32) Color {
	c, h := labToLCH(float64(a), float64(b))
	return oklchColor(float64(l), c, h, alpha)
}

// OKLCH returns the sRGB color for the OKLCH coordinates: lightness l in
// [0, 1] (CSS "100%" is 1), chroma c (about 0 to 0.4) and hue h in degrees.
// This is the notation of the Tailwind v4 palette, e.g.
// OKLCH(0.637, 0.237, 25.331, 1) is red-500.
//
// Colors outside the sRGB gamut are mapped with the CSS Color 4 gamut-mapping
// algorithm (section 13, "CSS gamut mapping"): lightness at or above 1 gives
// white and at or below 0 gives black; otherwise, if clipping the color to
// sRGB moves it by less than the just noticeable difference (deltaEOK < 0.02)
// the clipped color is used; otherwise chroma is reduced by binary search
// (epsilon 0.0001), keeping lightness and hue, until the clipped result is
// within the JND of the reduced-chroma color. Hue and lightness are
// preserved, which keeps gradients and palettes visually coherent.
func OKLCH(l, c, h, alpha float32) Color {
	return oklchColor(float64(l), float64(c), float64(h), alpha)
}

func oklchColor(l, c, h float64, alpha float32) Color {
	r, g, b := gamutMapOKLCH(l, c, h)
	return Color{float32(linearToSRGB(r)), float32(linearToSRGB(g)), float32(linearToSRGB(b)), alpha}
}

// gamutMapOKLCH implements the CSS Color 4 gamut mapping algorithm and
// returns linear sRGB components inside [0, 1].
func gamutMapOKLCH(l, c, h float64) (float64, float64, float64) {
	const (
		jnd = 0.02
		eps = 0.0001
	)
	if math.IsNaN(l) || math.IsNaN(c) || math.IsNaN(h) {
		return 0, 0, 0
	}
	if l >= 1 {
		return 1, 1, 1
	}
	if l <= 0 {
		return 0, 0, 0
	}
	if c < 0 {
		c = 0
	}
	hr := h * math.Pi / 180
	sin, cos := math.Sincos(hr)
	lin := func(chroma float64) (r, g, b float64) { return oklabToLin(l, chroma*cos, chroma*sin) }
	inGamut := func(r, g, b float64) bool {
		return r >= 0 && r <= 1 && g >= 0 && g <= 1 && b >= 0 && b <= 1
	}
	clip := func(r, g, b float64) (float64, float64, float64) {
		return math.Min(1, math.Max(0, r)), math.Min(1, math.Max(0, g)), math.Min(1, math.Max(0, b))
	}
	deltaE := func(r, g, b float64, chroma float64) float64 {
		cl, ca, cb := linToOklab(r, g, b)
		return math.Sqrt((cl-l)*(cl-l) + (ca-chroma*cos)*(ca-chroma*cos) + (cb-chroma*sin)*(cb-chroma*sin))
	}

	r, g, b := lin(c)
	if inGamut(r, g, b) {
		return r, g, b
	}
	lo, hi := 0.0, c
	loInGamut := true
	cr, cg, cb := clip(r, g, b)
	if deltaE(cr, cg, cb, c) < jnd { // the "clip shortcut"
		return cr, cg, cb
	}
	for hi-lo > eps {
		chroma := (lo + hi) / 2
		r, g, b = lin(chroma)
		if loInGamut && inGamut(r, g, b) {
			lo = chroma
			continue
		}
		cr, cg, cb = clip(r, g, b)
		e := deltaE(cr, cg, cb, chroma)
		if e < jnd {
			if jnd-e < eps {
				break
			}
			loInGamut = false
			lo = chroma
		} else {
			hi = chroma
		}
	}
	return cr, cg, cb
}
