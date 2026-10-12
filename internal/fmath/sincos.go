// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Ported from Go's src/math/sincos.go and sin.go (go1.27.2). The original C
// code and the constants were from http://netlib.sandia.gov/cephes/cmath/sin.c
// (Cephes Math Library Release 2.8: June, 2000, Copyright 1984, 1987, 1989,
// 1992, 2000 by Stephen L. Moshier). The readme file at
// http://netlib.sandia.gov/cephes/ says: "Some software in this archive may
// be from the book _Methods and Programs for Mathematical Functions_
// (Prentice-Hall or Simon & Schuster International, 1989) or from the Cephes
// Mathematical Library, a commercial product. In either event, it is
// copyrighted by the author. What you see here may be used freely but it
// comes with no support or guarantee."
//
// Changes: every product that feeds an addition or subtraction is wrapped in
// an explicit float64 conversion (see the package documentation); arguments
// at or above 2^29 are first reduced with math.Mod (exact) by the float64
// value of 2*Pi instead of Payne-Hanek reduction, which keeps the result
// deterministic but not accurate for such arguments. See THIRD_PARTY.md.

package fmath

import "math"

var sinCoef = [...]float64{
	1.58962301576546568060e-10, // 0x3de5d8fd1fd19ccd
	-2.50507477628578072866e-8, // 0xbe5ae5e5a9291f5d
	2.75573136213857245213e-6,  // 0x3ec71de3567d48a1
	-1.98412698295895385996e-4, // 0xbf2a01a019bfdf03
	8.33333333332211858878e-3,  // 0x3f8111111110f7d0
	-1.66666666666666307295e-1, // 0xbfc5555555555548
}

var cosCoef = [...]float64{
	-1.13585365213876817300e-11, // 0xbda8fa49a0861a9b
	2.08757008419747316778e-9,   // 0x3e21ee9d7b4e3f05
	-2.75573141792967388112e-7,  // 0xbe927e4f7eac4bc6
	2.48015872888517045348e-5,   // 0x3efa01a019c844f5
	-1.38888888888730564116e-3,  // 0xbf56c16c16c14f91
	4.16666666666665929218e-2,   // 0x3fa555555555554b
}

// Sincos returns Sin(x), Cos(x), like math.Sincos for |x| < 2^29.
func Sincos(x float64) (sin, cos float64) {
	const (
		PI4A = 7.85398125648498535156e-1  // 0x3fe921fb40000000, Pi/4 split into three parts
		PI4B = 3.77489470793079817668e-8  // 0x3e64442d00000000,
		PI4C = 2.69515142907905952645e-15 // 0x3ce8469898cc5170,

		reduceThreshold = 1 << 29
	)
	switch {
	case x == 0:
		return x, 1
	case x != x || math.IsInf(x, 0):
		return math.NaN(), math.NaN()
	}
	sinSign, cosSign := false, false
	if x < 0 {
		x = -x
		sinSign = true
	}
	if x >= reduceThreshold {
		x = math.Mod(x, 2*math.Pi)
	}
	j := uint64(x * (4 / math.Pi)) // integer part of x/(Pi/4)
	y := float64(j)
	if j&1 == 1 { // map zeros to origin
		j++
		y++
	}
	j &= 7                                                           // octant modulo 2Pi radians (360 degrees)
	z := ((x - float64(y*PI4A)) - float64(y*PI4B)) - float64(y*PI4C) // extended precision modular arithmetic
	if j > 3 {                                                       // reflect in x axis
		j -= 4
		sinSign, cosSign = !sinSign, !cosSign
	}
	if j > 1 {
		cosSign = !cosSign
	}

	zz := z * z
	pc, ps := float64(cosCoef[0]*zz), float64(sinCoef[0]*zz)
	for k := 1; k < 6; k++ {
		if k > 1 {
			pc, ps = float64(pc*zz), float64(ps*zz)
		}
		pc, ps = pc+cosCoef[k], ps+sinCoef[k]
	}
	cos = 1.0 - float64(0.5*zz) + float64(zz*zz*pc)
	sin = z + float64(z*zz*ps)
	if j == 1 || j == 2 {
		sin, cos = cos, sin
	}
	if cosSign {
		cos = -cos
	}
	if sinSign {
		sin = -sin
	}
	return sin, cos
}

// Tan returns Sin(x)/Cos(x), deterministic like Sincos (it may differ from
// math.Tan in the last bit).
func Tan(x float64) float64 {
	s, c := Sincos(x)
	return s / c
}
