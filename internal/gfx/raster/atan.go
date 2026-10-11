// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Ported from Go's src/math/atan.go (go1.27.2), which is itself based on the
// Cephes Math Library by Stephen L. Moshier ("Some software in this archive
// may be from the book _Methods and Programs for Mathematical Functions_
// (Prentice-Hall or Simon & Schuster International, 1989) or from the Cephes
// Mathematical Library, a commercial product. In either event, it is
// copyrighted by the author. What you see here may be used freely but it
// comes with no support or guarantee."). See THIRD_PARTY.md.
//
// Changes: every product that feeds an addition is wrapped in an explicit
// float64 conversion so that the compiler cannot fuse it into a
// multiply-add; the result is then the same on every architecture (see the
// package documentation, "Determinism"). asin01 is new.

package raster

import "math"

// xatan evaluates a series valid in the range [0, 0.66].
func xatan(x float64) float64 {
	const (
		P0 = -8.750608600031904122785e-01
		P1 = -1.615753718733365076637e+01
		P2 = -7.500855792314704667340e+01
		P3 = -1.228866684490136173410e+02
		P4 = -6.485021904942025371773e+01
		Q0 = +2.485846490142306297962e+01
		Q1 = +1.650270098316988542046e+02
		Q2 = +4.328810604912902668951e+02
		Q3 = +4.853903996359136964868e+02
		Q4 = +1.945506571482613964425e+02
	)
	z := float64(x * x)
	p := float64(P0*z) + P1
	p = float64(p*z) + P2
	p = float64(p*z) + P3
	p = float64(p*z) + P4
	q := z + Q0
	q = float64(q*z) + Q1
	q = float64(q*z) + Q2
	q = float64(q*z) + Q3
	q = float64(q*z) + Q4
	z = float64(z*p) / q
	return float64(x*z) + x
}

// satan reduces its argument (known to be positive) to the range [0, 0.66]
// and calls xatan.
func satan(x float64) float64 {
	const (
		Morebits = 6.123233995736765886130e-17 // pi/2 = PIO2 + Morebits
		Tan3pio8 = 2.41421356237309504880      // tan(3*pi/8)
	)
	if x <= 0.66 {
		return xatan(x)
	}
	if x > Tan3pio8 {
		return math.Pi/2 - xatan(1/x) + Morebits
	}
	return math.Pi/4 + xatan((x-1)/(x+1)) + 0.5*Morebits
}

// asin01 returns asin(x) for x in [0, 1] (arguments outside are clamped),
// with the same result on every architecture.
func asin01(x float64) float64 {
	switch {
	case !(x > 0):
		return 0
	case x >= 1:
		return math.Pi / 2
	}
	// sqrt(1 - x*x) without cancellation near 1. math.Sqrt is correctly
	// rounded on every architecture.
	c := math.Sqrt(float64((1 - x) * (1 + x)))
	return satan(x / c)
}
