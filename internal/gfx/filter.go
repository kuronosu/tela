package gfx

import "math"

// Filter is an image filter applied to a layer (PushLayer) or to the backdrop
// (BackdropFilter). The interface is sealed: Blur, DropShadow and
// ColorMatrix are the only implementations.
//
// Filter parameters are in the local coordinate space of the command that
// carries them, so the current transform scales them.
type Filter interface {
	isFilter()
}

func (Blur) isFilter()        {}
func (DropShadow) isFilter()  {}
func (ColorMatrix) isFilter() {}

// Blur is a Gaussian blur. StdDev is the standard deviation in logical
// pixels, exactly as CSS filter: blur(r), whose argument r is the standard
// deviation (not a diameter). The blurred edge extends about 3*StdDev beyond
// the shape. A StdDev of 0 or less is the identity.
type Blur struct{ StdDev float32 }

// DropShadow paints a blurred, offset, tinted copy of the layer's alpha
// behind the layer content (CSS filter: drop-shadow()).
//
// StdDev is the standard deviation of the Gaussian. The CSS function takes a
// blur radius instead: the CSS drop-shadow(offset-x offset-y blur-radius
// color) blur radius B corresponds to StdDev = B/2. That conversion belongs
// to the style layer.
type DropShadow struct {
	Offset Point
	StdDev float32
	Color  Color
}

// ColorMatrix is a 4x5 color matrix as in SVG's feColorMatrix, in row-major
// order, applied to non-premultiplied, sRGB-encoded components in [0, 1]
// (what browsers do for the CSS filter shorthand functions). With
// (r, g, b, a) the input:
//
//	r' = M[0]*r  + M[1]*g  + M[2]*b  + M[3]*a  + M[4]
//	g' = M[5]*r  + M[6]*g  + M[7]*b  + M[8]*a  + M[9]
//	b' = M[10]*r + M[11]*g + M[12]*b + M[13]*a + M[14]
//	a' = M[15]*r + M[16]*g + M[17]*b + M[18]*a + M[19]
//
// Results are clamped to [0, 1]. A rasterizer un-premultiplies the layer,
// applies the matrix and premultiplies again.
type ColorMatrix struct{ M [20]float32 }

// IdentityMatrix is the ColorMatrix that changes nothing.
var IdentityMatrix = ColorMatrix{M: [20]float32{
	1, 0, 0, 0, 0,
	0, 1, 0, 0, 0,
	0, 0, 1, 0, 0,
	0, 0, 0, 1, 0,
}}

// IsIdentity reports whether m is exactly IdentityMatrix.
func (m ColorMatrix) IsIdentity() bool { return m == IdentityMatrix }

// Mul returns the matrix that applies m first and then n:
// the composition of the two color transforms.
func (m ColorMatrix) Mul(n ColorMatrix) ColorMatrix {
	var out ColorMatrix
	for row := 0; row < 4; row++ {
		for col := 0; col < 5; col++ {
			var s float64
			for k := 0; k < 4; k++ {
				s += f64(n.M[row*5+k]) * f64(m.M[k*5+col])
			}
			if col == 4 {
				s += f64(n.M[row*5+4])
			}
			out.M[row*5+col] = float32(s)
		}
	}
	return out
}

func matrixFromRGB(r, g, b [3]float64) ColorMatrix {
	return ColorMatrix{M: [20]float32{
		float32(r[0]), float32(r[1]), float32(r[2]), 0, 0,
		float32(g[0]), float32(g[1]), float32(g[2]), 0, 0,
		float32(b[0]), float32(b[1]), float32(b[2]), 0, 0,
		0, 0, 0, 1, 0,
	}}
}

func clampAmount(v float32, hi float64) float64 {
	x := f64(v)
	if math.IsNaN(x) || x < 0 {
		return 0
	}
	return math.Min(x, hi)
}

// The constructors below are the CSS filter functions of Filter Effects
// Module Level 1, section 13.1. Amounts follow CSS: 1 means 100%.

// Brightness is filter: brightness(amount). Amounts below 0 count as 0;
// values above 1 brighten.
func Brightness(amount float32) ColorMatrix {
	a := float32(clampAmount(amount, math.Inf(1)))
	m := IdentityMatrix
	m.M[0], m.M[6], m.M[12] = a, a, a
	return m
}

// Contrast is filter: contrast(amount). Amounts below 0 count as 0.
func Contrast(amount float32) ColorMatrix {
	a := clampAmount(amount, math.Inf(1))
	m := IdentityMatrix
	m.M[0], m.M[6], m.M[12] = float32(a), float32(a), float32(a)
	i := float32(-0.5*a + 0.5)
	m.M[4], m.M[9], m.M[14] = i, i, i
	return m
}

// Grayscale is filter: grayscale(amount). The amount is clamped to [0, 1].
func Grayscale(amount float32) ColorMatrix {
	a := 1 - clampAmount(amount, 1)
	return matrixFromRGB(
		[3]float64{0.2126 + 0.7874*a, 0.7152 - 0.7152*a, 0.0722 - 0.0722*a},
		[3]float64{0.2126 - 0.2126*a, 0.7152 + 0.2848*a, 0.0722 - 0.0722*a},
		[3]float64{0.2126 - 0.2126*a, 0.7152 - 0.7152*a, 0.0722 + 0.9278*a},
	)
}

// Saturate is filter: saturate(amount). Amounts below 0 count as 0; values
// above 1 oversaturate.
func Saturate(amount float32) ColorMatrix {
	s := clampAmount(amount, math.Inf(1))
	return matrixFromRGB(
		[3]float64{0.213 + 0.787*s, 0.715 - 0.715*s, 0.072 - 0.072*s},
		[3]float64{0.213 - 0.213*s, 0.715 + 0.285*s, 0.072 - 0.072*s},
		[3]float64{0.213 - 0.213*s, 0.715 - 0.715*s, 0.072 + 0.928*s},
	)
}

// Sepia is filter: sepia(amount). The amount is clamped to [0, 1].
func Sepia(amount float32) ColorMatrix {
	a := 1 - clampAmount(amount, 1)
	return matrixFromRGB(
		[3]float64{0.393 + 0.607*a, 0.769 - 0.769*a, 0.189 - 0.189*a},
		[3]float64{0.349 - 0.349*a, 0.686 + 0.314*a, 0.168 - 0.168*a},
		[3]float64{0.272 - 0.272*a, 0.534 - 0.534*a, 0.131 + 0.869*a},
	)
}

// Invert is filter: invert(amount). The amount is clamped to [0, 1].
func Invert(amount float32) ColorMatrix {
	a := clampAmount(amount, 1)
	m := IdentityMatrix
	s := float32(1 - 2*a)
	m.M[0], m.M[6], m.M[12] = s, s, s
	m.M[4], m.M[9], m.M[14] = float32(a), float32(a), float32(a)
	return m
}

// HueRotate is filter: hue-rotate(angle), with the angle in radians (CSS
// degrees times pi/180).
func HueRotate(angle float32) ColorMatrix {
	sin, cos := math.Sincos(f64(angle))
	return matrixFromRGB(
		[3]float64{0.213 + cos*0.787 - sin*0.213, 0.715 - cos*0.715 - sin*0.715, 0.072 - cos*0.072 + sin*0.928},
		[3]float64{0.213 - cos*0.213 + sin*0.143, 0.715 + cos*0.285 + sin*0.140, 0.072 - cos*0.072 - sin*0.283},
		[3]float64{0.213 - cos*0.213 - sin*0.787, 0.715 - cos*0.715 + sin*0.715, 0.072 + cos*0.928 + sin*0.072},
	)
}

// gaussianExtent is how many standard deviations a Gaussian blur is
// considered to extend. Beyond 3 sigma the kernel weight is under 0.14% of
// the peak, below one 8-bit level for any opaque source.
const gaussianExtent = 3

// FilterOutset returns how far, in the local coordinate space, the filter can
// paint outside the bounds of its input: 3*StdDev for Blur, the length of the
// offset plus 3*StdDev for DropShadow, and 0 for ColorMatrix. The value is a
// conservative scalar, applied on all four sides; it bounds the reach in
// every direction, so it stays conservative under rotation when it is
// converted to root space with Affine.MaxScale.
func FilterOutset(f Filter) float32 {
	switch f := f.(type) {
	case Blur:
		return gaussianExtent * max(f.StdDev, 0)
	case DropShadow:
		// The length of the offset, not its larger component: rotated by
		// 45 degrees, an offset (d, d) moves content by sqrt(2)*d along one
		// device axis. The squares of float32 values are exact in float64,
		// so the result is the same on every architecture.
		x, y := f64(f.Offset.X), f64(f.Offset.Y)
		o := float32(math.Sqrt(x*x + y*y))
		return o + gaussianExtent*max(f.StdDev, 0)
	}
	return 0
}

// FiltersOutset returns the total outset of a filter chain applied in order:
// the sum of the individual outsets.
func FiltersOutset(fs []Filter) float32 {
	var s float32
	for _, f := range fs {
		s += FilterOutset(f)
	}
	return s
}

// sanitizeFilters returns a copy of fs that the Builder can store: parameters
// clamped, identity blurs removed. ok is false for non-finite parameters.
func sanitizeFilters(fs []Filter) (out []Filter, ok bool) {
	for _, f := range fs {
		switch f := f.(type) {
		case Blur:
			if !finite(f.StdDev) {
				return nil, false
			}
			if f.StdDev <= 0 {
				continue
			}
			out = append(out, f)
		case DropShadow:
			if !f.Offset.IsFinite() || !finite(f.StdDev) || !f.Color.IsFinite() {
				return nil, false
			}
			f.StdDev = max(f.StdDev, 0)
			f.Color = f.Color.Clamp()
			out = append(out, f)
		case ColorMatrix:
			for _, v := range f.M {
				if !finite(v) {
					return nil, false
				}
			}
			out = append(out, f)
		default:
			return nil, false
		}
	}
	return out, true
}
