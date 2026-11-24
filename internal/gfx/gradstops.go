package gfx

import "math"

// UnpremultipliedStops returns stops with intermediate ones that make the
// gradient, which interpolates premultiplied colors, look as one that
// interpolates unpremultiplied colors in sRGB (InterpSRGB), as browsers
// paint the gradients of SVG and Canvas: a segment whose two stops differ in
// both alpha and color is cut so that the premultiplied piecewise-linear
// ramp stays within half an 8-bit level of the other. The offsets of stops
// must not decrease.
func UnpremultipliedStops(stops []GradientStop) []GradientStop {
	out := make([]GradientStop, 0, len(stops))
	for i, s := range stops {
		c0 := s.Color
		out = append(out, s)
		if i+1 == len(stops) {
			break
		}
		n := stops[i+1]
		c1 := n.Color
		da := math.Abs(float64(c1.A - c0.A))
		dc := max(math.Abs(float64(c1.R-c0.R)), math.Abs(float64(c1.G-c0.G)), math.Abs(float64(c1.B-c0.B)))
		if !(n.Offset > s.Offset) || da == 0 || dc == 0 || c0.A <= 0 && c1.A <= 0 {
			continue
		}
		// The premultiplied value a(u)*c(u) is quadratic in u; k linear
		// pieces stay within da*dc/(4k²) of it: half a level for
		// k² >= 127.5*da*dc.
		k := int(math.Ceil(math.Sqrt(127.5 * float64(da*dc))))
		k = min(max(k, 1), 16)
		for j := 1; j < k; j++ {
			u := float32(j) / float32(k)
			out = append(out, GradientStop{
				Offset: s.Offset + float32((n.Offset-s.Offset)*u),
				Color: Color{
					R: c0.R + float32((c1.R-c0.R)*u),
					G: c0.G + float32((c1.G-c0.G)*u),
					B: c0.B + float32((c1.B-c0.B)*u),
					A: c0.A + float32((c1.A-c0.A)*u),
				},
			})
		}
	}
	return out
}
