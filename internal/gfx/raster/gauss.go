package raster

import (
	"math"

	"github.com/kuronosu/tela/internal/fmath"
)

// The standard normal CDF Phi and its integral I(t) = t*Phi(t) + phi(t),
// which every shadow and kernel evaluates many times per pixel, come from
// tables with cubic Hermite interpolation: both functions and their
// derivatives (phi for Phi; Phi for I) are tabulated on a grid of step
// 1/gaussGrid over [-gaussLim, gaussLim] at package initialization, from the
// deterministic erfc and exp of fmath. The interpolation error is below
// 1e-9 (TestGaussTables), and every product is rounded explicitly, so the
// values are the same on every architecture. Beyond the range, Phi is 0 or
// 1 and I is 0 or t (the neglected tails are below 1e-15).
const (
	gaussLim  = 8
	gaussGrid = 64
	gaussN    = 2*gaussLim*gaussGrid + 1
)

// gaussTab holds, for t = -gaussLim + i/gaussGrid: Phi, phi and I.
var gaussTab = func() (tab [gaussN][3]float64) {
	for i := range tab {
		t := float64(i-gaussLim*gaussGrid) / gaussGrid
		// Every product is rounded explicitly: Go may fuse a product with
		// an addition even across statements.
		cdf := 0.5 * fmath.Erfc(float64(-t*(1/math.Sqrt2)))
		pdf := float64(invSqrt2Pi * fmath.Exp(-0.5*float64(t*t)))
		tab[i] = [3]float64{cdf, pdf, float64(t*cdf) + pdf}
	}
	return tab
}()

// hermite interpolates on [0, 1] between values f0, f1 with derivatives
// (scaled by the step) d0, d1, at u.
func hermite(f0, d0, f1, d1, u float64) float64 {
	u2 := float64(u * u)
	u3 := float64(u2 * u)
	h00 := float64(2*u3) - float64(3*u2) + 1
	h10 := u3 - float64(2*u2) + u
	h01 := float64(-2*u3) + float64(3*u2)
	h11 := u3 - u2
	return float64(h00*f0) + float64(h10*d0) + float64(h01*f1) + float64(h11*d1)
}

// gaussIndex splits t into a table index and a fraction; ok is false
// outside the table.
func gaussIndex(t float64) (int, float64, bool) {
	if !(t > -gaussLim && t < gaussLim) {
		return 0, 0, false
	}
	x := float64((t + gaussLim) * gaussGrid)
	i := int(x)
	return i, x - float64(i), true
}

// normCDF returns the standard normal cumulative distribution function
// Phi(t), deterministically.
func normCDF(t float64) float64 {
	i, u, ok := gaussIndex(t)
	if !ok {
		if t > 0 {
			return 1
		}
		return 0 // also NaN
	}
	a, b := &gaussTab[i], &gaussTab[i+1]
	const h = 1.0 / gaussGrid
	return hermite(a[0], float64(a[1]*h), b[0], float64(b[1]*h), u)
}

// gaussI returns the integral of the standard normal CDF up to t:
// t*Phi(t) + phi(t).
func gaussI(t float64) float64 {
	i, u, ok := gaussIndex(t)
	if !ok {
		if t > 0 {
			return t
		}
		return 0 // also NaN
	}
	a, b := &gaussTab[i], &gaussTab[i+1]
	const h = 1.0 / gaussGrid
	return hermite(a[2], float64(a[0]*h), b[2], float64(b[0]*h), u)
}
