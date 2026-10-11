package raster

import (
	"fmt"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/internal/fmath"
)

// Gradients.
//
// A gradient shader maps the center of each device pixel back to the local
// space of the command (the inverse of the current transform), computes the
// gradient parameter t there and looks the color up in a ramp:
//
//   - linear: t is the projection of the point on Start→End, divided by the
//     squared length (CSS linear-gradient);
//   - radial: t = |(p - Center) / Radius|, the distance to the center
//     relative to an axis-aligned ellipse (CSS radial-gradient);
//   - conic: t = the angle of p - Center, measured from "up" (-Y) and
//     clockwise on screen, minus StartAngle, in turns, wrapped to [0, 1)
//     (CSS conic-gradient).
//
// t below the first stop offset gives the first color, at or above the last
// offset the last color (pad). The map from device pixels to t is affine (an
// affine function feeding a distance or an angle for radial and conic), its
// coefficients are computed once per command in float64 with explicit
// rounding, and t is evaluated from the absolute pixel coordinates, never
// accumulated along a span, so region renders are bit-identical.
//
// # Ramps
//
// Between two consecutive stops with different offsets (a segment), colors
// are interpolated on premultiplied values in the gradient's color space
// (CSS Color 4, section 12.3; Oklab by default, ADR-023) and stored in a
// lookup table of 256 premultiplied RGBA8 entries, entry i holding the color
// at i/255 of the way through the segment. A pixel takes the entry nearest
// to its position, so the ramp error is at most half an entry: a 0-255
// channel change spread over a segment moves by about one 8-bit level per
// entry, which is the resolution of the output anyway; no dithering is
// applied. Stops at equal offsets form no segment, and the segment of a
// pixel is chosen by comparing t with the exact stop offsets, so hard stops
// stay perfectly sharp wherever they fall. The two ends of every segment
// are the stop colors exactly.
//
// Ramps are computed with deterministic color arithmetic (fmath) and cached
// per Renderer (rampCache), keyed by the stops and the color space, so a
// warm Renderer does not rebuild or allocate them frame after frame.

// rampSize is the number of entries of a segment's lookup table.
const rampSize = 256

type gradKind uint8

const (
	gradLinear gradKind = iota
	gradRadial
	gradConic
)

// gradShader shades linear, radial and conic gradients. With (X, Y) the
// device pixel center, p = a*X + b*Y + c and q likewise with the second set
// of coefficients:
//
//	linear: t = p
//	radial: t = sqrt(p² + q²)
//	conic:  t = atan2(p, -q) / 2π - start, wrapped to [0, 1)
type gradShader struct {
	kind       gradKind
	ramp       *ramp
	pa, pb, pc float64
	qa, qb, qc float64
	start      float64
}

func (g *gradShader) opaque() bool { return g.ramp.opaque }

func (g *gradShader) shadeSpan(x, y int, dst []uint8) {
	Y := float64(y) + 0.5
	p0 := float64(g.pb*Y) + g.pc
	q0 := float64(g.qb*Y) + g.qc
	rp := g.ramp
	n := len(dst) / 4
	switch g.kind {
	case gradLinear:
		for i := 0; i < n; i++ {
			X := float64(x+i) + 0.5
			c := rp.at(float64(g.pa*X) + p0)
			copy(dst[4*i:4*i+4], c[:])
		}
	case gradRadial:
		for i := 0; i < n; i++ {
			X := float64(x+i) + 0.5
			u := float64(g.pa*X) + p0
			v := float64(g.qa*X) + q0
			c := rp.at(math.Sqrt(float64(u*u) + float64(v*v)))
			copy(dst[4*i:4*i+4], c[:])
		}
	case gradConic:
		for i := 0; i < n; i++ {
			X := float64(x+i) + 0.5
			u := float64(g.pa*X) + p0
			v := float64(g.qa*X) + q0
			// The angle from up (-Y), clockwise on screen (y down).
			t := float64(fmath.Atan2(u, -v)*(1/(2*math.Pi))) - g.start
			t -= math.Floor(t)
			c := rp.at(t)
			copy(dst[4*i:4*i+4], c[:])
		}
	}
}

// inverse is the inverse of a device transform, in float64: a local point
// is (a*X + c*Y + e, b*X + d*Y + f).
type inverse struct{ a, b, c, d, e, f float64 }

// invert returns the inverse of m. The products of its float32 entries are
// exact in float64, so the result does not depend on fused multiply-adds.
func invert(m gfx.Affine) (inverse, bool) {
	m0, m1, m2, m3, m4, m5 := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3]), float64(m[4]), float64(m[5])
	det := m0*m3 - m1*m2
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return inverse{}, false
	}
	inv := inverse{
		a: m3 / det, b: -m1 / det, c: -m2 / det, d: m0 / det,
		e: (m2*m5 - m3*m4) / det, f: (m1*m4 - m0*m5) / det,
	}
	for _, v := range [6]float64{inv.a, inv.b, inv.c, inv.d, inv.e, inv.f} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return inverse{}, false
		}
	}
	return inv, true
}

// compose returns the coefficients (a, b, c) of the device-space affine
// function a*X + b*Y + c equal to the local function kx*x + ky*y + k0.
func (inv *inverse) compose(kx, ky, k0 float64) (a, b, c float64) {
	a = float64(kx*inv.a) + float64(ky*inv.b)
	b = float64(kx*inv.c) + float64(ky*inv.d)
	c = float64(kx*inv.e) + float64(ky*inv.f) + k0
	return a, b, c
}

// shaderFor returns the shader for paint under the current transform. The
// shaders are held by the Renderer, so selecting one does not allocate.
func (r *Renderer) shaderFor(p gfx.Paint) shader {
	switch p := p.(type) {
	case gfx.Color:
		return r.solidFor(p)
	case gfx.LinearGradient:
		inv, sh := r.gradientStart(p.Stops)
		if sh != nil {
			return sh
		}
		sx, sy := float64(p.Start.X), float64(p.Start.Y)
		dx, dy := float64(p.End.X)-sx, float64(p.End.Y)-sy
		l2 := float64(dx*dx) + float64(dy*dy)
		if !(l2 > 0) || math.IsInf(l2, 0) { // Start == End
			return r.solidFor(p.Stops[len(p.Stops)-1].Color)
		}
		g := r.gradient(gradLinear, p.Stops, p.Interp)
		kx, ky := dx/l2, dy/l2
		g.pa, g.pb, g.pc = inv.compose(kx, ky, -(float64(kx*sx) + float64(ky*sy)))
		return g
	case gfx.RadialGradient:
		inv, sh := r.gradientStart(p.Stops)
		if sh != nil {
			return sh
		}
		rx, ry := float64(p.Radius.X), float64(p.Radius.Y)
		if !(rx > 0 && ry > 0) {
			return r.solidFor(p.Stops[len(p.Stops)-1].Color)
		}
		g := r.gradient(gradRadial, p.Stops, p.Interp)
		g.pa, g.pb, g.pc = inv.compose(1/rx, 0, -float64(p.Center.X)/rx)
		g.qa, g.qb, g.qc = inv.compose(0, 1/ry, -float64(p.Center.Y)/ry)
		return g
	case gfx.ConicGradient:
		inv, sh := r.gradientStart(p.Stops)
		if sh != nil {
			return sh
		}
		g := r.gradient(gradConic, p.Stops, p.Interp)
		g.pa, g.pb, g.pc = inv.compose(1, 0, -float64(p.Center.X))
		g.qa, g.qb, g.qc = inv.compose(0, 1, -float64(p.Center.Y))
		g.start = float64(p.StartAngle) * (1 / (2 * math.Pi))
		return g
	case nil:
		panic("raster: nil paint in a display list")
	}
	panic(fmt.Sprintf("raster: %T paint is not implemented", p))
}

func (r *Renderer) solidFor(c gfx.Color) shader {
	r.solid = solidShader{premul8(c)}
	return &r.solid
}

// gradientStart handles the gradients that paint a single color: no stops
// (never stored by the Builder), one stop, or a transform that cannot be
// inverted (never stored either). Otherwise it returns the inverse of the
// current transform and a nil shader.
func (r *Renderer) gradientStart(stops []gfx.GradientStop) (inverse, shader) {
	switch len(stops) {
	case 0:
		return inverse{}, r.solidFor(gfx.Transparent)
	case 1:
		return inverse{}, r.solidFor(stops[0].Color)
	}
	inv, ok := invert(r.ctm)
	if !ok {
		return inv, r.solidFor(stops[len(stops)-1].Color)
	}
	return inv, nil
}

// gradient resets the Renderer's gradient shader with the ramp of stops.
func (r *Renderer) gradient(kind gradKind, stops []gfx.GradientStop, interp gfx.ColorSpace) *gradShader {
	g := &r.grad
	*g = gradShader{kind: kind, ramp: r.ramps.get(stops, interp)}
	return g
}

// ramp is the color lookup of one gradient; see the package comment above.
type ramp struct {
	stops  []gfx.GradientStop // cache key (a private copy)
	interp gfx.ColorSpace

	lo, hi      float64  // first and last stop offsets
	first, last [4]uint8 // pad colors
	segs        []rampSeg
	opaque      bool
}

// rampSeg is the part of a ramp between two stops with offsets o0 < o1. The
// entry for t is round((t - o0) * 255 / (o1 - o0)), computed as
// t*scale + bias.
type rampSeg struct {
	o0          float64
	scale, bias float64
	lut         [rampSize][4]uint8
}

// at returns the premultiplied color at t.
func (rp *ramp) at(t float64) [4]uint8 {
	if !(t >= rp.lo) { // also NaN
		return rp.first
	}
	if t >= rp.hi {
		return rp.last
	}
	segs := rp.segs
	k := 0
	for k+1 < len(segs) && t >= segs[k+1].o0 {
		k++
	}
	s := &segs[k]
	i := int(float64(t*s.scale) + s.bias)
	return s.lut[min(max(i, 0), rampSize-1)]
}

func (rp *ramp) matches(stops []gfx.GradientStop, interp gfx.ColorSpace) bool {
	if rp.interp != interp || len(rp.stops) != len(stops) {
		return false
	}
	for i := range stops {
		if rp.stops[i] != stops[i] {
			return false
		}
	}
	return true
}

// newRamp builds the ramp of normalized stops (gfx.NormalizeStops; at least
// two).
func newRamp(stops []gfx.GradientStop, interp gfx.ColorSpace) *ramp {
	n := len(stops)
	rp := &ramp{
		stops:  append([]gfx.GradientStop(nil), stops...),
		interp: interp,
		lo:     float64(stops[0].Offset),
		hi:     float64(stops[n-1].Offset),
		first:  premul8(stops[0].Color),
		last:   premul8(stops[n-1].Color),
		opaque: true,
	}
	for _, s := range stops {
		rp.opaque = rp.opaque && s.Color.Clamp().A >= 1
	}
	nseg := 0
	for k := 0; k+1 < n; k++ {
		if stops[k].Offset < stops[k+1].Offset {
			nseg++
		}
	}
	rp.segs = make([]rampSeg, 0, nseg)
	for k := 0; k+1 < n; k++ {
		s0, s1 := stops[k], stops[k+1]
		if !(s0.Offset < s1.Offset) {
			continue
		}
		o0, o1 := float64(s0.Offset), float64(s1.Offset)
		scale := (rampSize - 1) / (o1 - o0)
		rp.segs = append(rp.segs, rampSeg{o0: o0, scale: scale, bias: 0.5 - float64(o0*scale)})
		seg := &rp.segs[len(rp.segs)-1]
		a, b := newInterpColor(s0.Color, interp), newInterpColor(s1.Color, interp)
		seg.lut[0] = premul8(s0.Color)
		seg.lut[rampSize-1] = premul8(s1.Color)
		for i := 1; i < rampSize-1; i++ {
			seg.lut[i] = mixColor(&a, &b, float64(i)/(rampSize-1), interp)
		}
	}
	return rp
}

// rampCache keeps the ramps of the last gradients a Renderer has drawn.
type rampCache struct {
	entries [32]*ramp
	next    int
}

func (c *rampCache) get(stops []gfx.GradientStop, interp gfx.ColorSpace) *ramp {
	for _, e := range c.entries {
		if e != nil && e.matches(stops, interp) {
			return e
		}
	}
	e := newRamp(stops, interp)
	c.entries[c.next] = e
	c.next = (c.next + 1) % len(c.entries)
	return e
}

// interpColor is a stop color in the interpolation space, premultiplied:
// v holds the three color components times alpha.
type interpColor struct {
	v [3]float64
	a float64
}

func newInterpColor(c gfx.Color, space gfx.ColorSpace) interpColor {
	c = c.Clamp()
	r, g, b, a := float64(c.R), float64(c.G), float64(c.B), float64(c.A)
	if space == gfx.InterpOklab {
		r, g, b = linearToOklab(srgbToLinear(r), srgbToLinear(g), srgbToLinear(b))
	}
	return interpColor{v: [3]float64{float64(r * a), float64(g * a), float64(b * a)}, a: a}
}

// mixColor returns the premultiplied RGBA8 color at u in [0, 1] between a
// and b, interpolated on premultiplied components (CSS Color 4, section
// 12.3) and converted back to sRGB, clamped to [0, 1].
func mixColor(a, b *interpColor, u float64, space gfx.ColorSpace) [4]uint8 {
	lerp := func(x, y float64) float64 { return x + float64((y-x)*u) }
	alpha := lerp(a.a, b.a)
	if !(alpha > 0) {
		return [4]uint8{}
	}
	v0, v1, v2 := lerp(a.v[0], b.v[0]), lerp(a.v[1], b.v[1]), lerp(a.v[2], b.v[2])
	a8 := unit8(alpha)
	var r, g, bl float64
	if space == gfx.InterpOklab {
		lr, lg, lb := oklabToLinear(v0/alpha, v1/alpha, v2/alpha)
		r, g, bl = linearToSRGB(lr), linearToSRGB(lg), linearToSRGB(lb)
		r, g, bl = float64(clamp01(r)*alpha), float64(clamp01(g)*alpha), float64(clamp01(bl)*alpha)
	} else {
		r, g, bl = min(v0, alpha), min(v1, alpha), min(v2, alpha)
	}
	// Premultiplied channels never exceed alpha, even after rounding.
	return [4]uint8{min(unit8(r), a8), min(unit8(g), a8), min(unit8(bl), a8), a8}
}

// Color space conversions with the CSS Color 4 matrices (the ones of
// gfx/color.go), evaluated with explicit rounding and fmath so that they are
// the same on every architecture.
var (
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
	linToLMS = matMul(xyzToLMS, srgbToXYZ)
	lmsToLin = matMul(xyzToSRGB, lmsToXYZ)
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

func linearToOklab(r, g, b float64) (float64, float64, float64) {
	l, m, s := matVec(&linToLMS, r, g, b)
	return matVec(&lmsToOklab, fmath.Cbrt(l), fmath.Cbrt(m), fmath.Cbrt(s))
}

func oklabToLinear(l, a, b float64) (float64, float64, float64) {
	l_, m_, s_ := matVec(&oklabToLMS, l, a, b)
	return matVec(&lmsToLin, l_*l_*l_, m_*m_*m_, s_*s_*s_)
}

// srgbToLinear decodes an sRGB component in [0, 1].
func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return fmath.Pow((v+0.055)/1.055, 2.4)
}

// linearToSRGB encodes a linear component; values outside [0, 1] give
// values outside [0, 1] (clamped by the caller).
func linearToSRGB(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return float64(1.055*fmath.Pow(v, 1/2.4)) - 0.055
}
