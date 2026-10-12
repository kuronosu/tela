// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Ported from Go's src/math/erf.go (go1.27.2). The original C code and the
// constants are from FreeBSD's /usr/src/lib/msun/src/s_erf.c and came with
// this notice:
//
// ====================================================
// Copyright (C) 1993 by Sun Microsystems, Inc. All rights reserved.
//
// Developed at SunPro, a Sun Microsystems, Inc. business.
// Permission to use, copy, modify, and distribute this
// software is freely granted, provided that this notice
// is preserved.
// ====================================================
//
// Changes: every product that feeds an addition or subtraction is wrapped in
// an explicit float64 conversion (see the package documentation), Exp is
// this package's port, and special cases are reduced to what the callers
// need. See THIRD_PARTY.md for the method (rational approximations on
// [0, 0.84375], [0.84375, 1.25], [1.25, 1/0.35] and [1/0.35, 28]).

package fmath

import "math"

const (
	erx = 8.45062911510467529297e-01 // 0x3FEB0AC160000000
	// Coefficients for approximation to  erf in [0, 0.84375]
	efx  = 1.28379167095512586316e-01  // 0x3FC06EBA8214DB69
	efx8 = 1.02703333676410069053e+00  // 0x3FF06EBA8214DB69
	pp0  = 1.28379167095512558561e-01  // 0x3FC06EBA8214DB68
	pp1  = -3.25042107247001499370e-01 // 0xBFD4CD7D691CB913
	pp2  = -2.84817495755985104766e-02 // 0xBF9D2A51DBD7194F
	pp3  = -5.77027029648944159157e-03 // 0xBF77A291236668E4
	pp4  = -2.37630166566501626084e-05 // 0xBEF8EAD6120016AC
	qq1  = 3.97917223959155352819e-01  // 0x3FD97779CDDADC09
	qq2  = 6.50222499887672944485e-02  // 0x3FB0A54C5536CEBA
	qq3  = 5.08130628187576562776e-03  // 0x3F74D022C4D36B0F
	qq4  = 1.32494738004321644526e-04  // 0x3F215DC9221C1A10
	qq5  = -3.96022827877536812320e-06 // 0xBED09C4342A26120
	// Coefficients for approximation to  erf  in [0.84375, 1.25]
	pa0 = -2.36211856075265944077e-03 // 0xBF6359B8BEF77538
	pa1 = 4.14856118683748331666e-01  // 0x3FDA8D00AD92B34D
	pa2 = -3.72207876035701323847e-01 // 0xBFD7D240FBB8C3F1
	pa3 = 3.18346619901161753674e-01  // 0x3FD45FCA805120E4
	pa4 = -1.10894694282396677476e-01 // 0xBFBC63983D3E28EC
	pa5 = 3.54783043256182359371e-02  // 0x3FA22A36599795EB
	pa6 = -2.16637559486879084300e-03 // 0xBF61BF380A96073F
	qa1 = 1.06420880400844228286e-01  // 0x3FBB3E6618EEE323
	qa2 = 5.40397917702171048937e-01  // 0x3FE14AF092EB6F33
	qa3 = 7.18286544141962662868e-02  // 0x3FB2635CD99FE9A7
	qa4 = 1.26171219808761642112e-01  // 0x3FC02660E763351F
	qa5 = 1.36370839120290507362e-02  // 0x3F8BEDC26B51DD1C
	qa6 = 1.19844998467991074170e-02  // 0x3F888B545735151D
	// Coefficients for approximation to  erfc in [1.25, 1/0.35]
	ra0 = -9.86494403484714822705e-03 // 0xBF843412600D6435
	ra1 = -6.93858572707181764372e-01 // 0xBFE63416E4BA7360
	ra2 = -1.05586262253232909814e+01 // 0xC0251E0441B0E726
	ra3 = -6.23753324503260060396e+01 // 0xC04F300AE4CBA38D
	ra4 = -1.62396669462573470355e+02 // 0xC0644CB184282266
	ra5 = -1.84605092906711035994e+02 // 0xC067135CEBCCABB2
	ra6 = -8.12874355063065934246e+01 // 0xC054526557E4D2F2
	ra7 = -9.81432934416914548592e+00 // 0xC023A0EFC69AC25C
	sa1 = 1.96512716674392571292e+01  // 0x4033A6B9BD707687
	sa2 = 1.37657754143519042600e+02  // 0x4061350C526AE721
	sa3 = 4.34565877475229228821e+02  // 0x407B290DD58A1A71
	sa4 = 6.45387271733267880336e+02  // 0x40842B1921EC2868
	sa5 = 4.29008140027567833386e+02  // 0x407AD02157700314
	sa6 = 1.08635005541779435134e+02  // 0x405B28A3EE48AE2C
	sa7 = 6.57024977031928170135e+00  // 0x401A47EF8E484A93
	sa8 = -6.04244152148580987438e-02 // 0xBFAEEFF2EE749A62
	// Coefficients for approximation to  erfc in [1/.35, 28]
	rb0 = -9.86494292470009928597e-03 // 0xBF84341239E86F4A
	rb1 = -7.99283237680523006574e-01 // 0xBFE993BA70C285DE
	rb2 = -1.77579549177547519889e+01 // 0xC031C209555F995A
	rb3 = -1.60636384855821916062e+02 // 0xC064145D43C5ED98
	rb4 = -6.37566443368389627722e+02 // 0xC083EC881375F228
	rb5 = -1.02509513161107724954e+03 // 0xC09004616A2E5992
	rb6 = -4.83519191608651397019e+02 // 0xC07E384E9BDC383F
	sb1 = 3.03380607434824582924e+01  // 0x403E568B261D5190
	sb2 = 3.25792512996573918826e+02  // 0x40745CAE221B9F0A
	sb3 = 1.53672958608443695994e+03  // 0x409802EB189D5118
	sb4 = 3.19985821950859553908e+03  // 0x40A8FFB7688C246A
	sb5 = 2.55305040643316442583e+03  // 0x40A3F219CEDF3BE6
	sb6 = 4.74528541206955367215e+02  // 0x407DA874E79FE763
	sb7 = -2.24409524465858183362e+01 // 0xC03670E242712D62
)

// erfSmall returns erf(x)/x - 1 for 0 <= x < 0.84375: R(x²) = P/Q.
func erfSmall(x float64) float64 {
	z := float64(x * x)
	r := pp0 + float64(z*(pp1+float64(z*(pp2+float64(z*(pp3+float64(z*pp4)))))))
	s := 1 + float64(z*(qq1+float64(z*(qq2+float64(z*(qq3+float64(z*(qq4+float64(z*qq5)))))))))
	return r / s
}

// erfMid returns P/Q for 0.84375 <= x < 1.25: erf(x) = erx + P/Q.
func erfMid(x float64) float64 {
	s := x - 1
	P := pa0 + float64(s*(pa1+float64(s*(pa2+float64(s*(pa3+float64(s*(pa4+float64(s*(pa5+float64(s*pa6)))))))))))
	Q := 1 + float64(s*(qa1+float64(s*(qa2+float64(s*(qa3+float64(s*(qa4+float64(s*(qa5+float64(s*qa6)))))))))))
	return P / Q
}

// erfcTail returns erfc(x) for 1.25 <= x < 28.
func erfcTail(x float64) float64 {
	s := 1 / float64(x*x)
	var R, S float64
	if x < 1/0.35 { // |x| < 1 / 0.35  ~ 2.857143
		R = ra0 + float64(s*(ra1+float64(s*(ra2+float64(s*(ra3+float64(s*(ra4+float64(s*(ra5+float64(s*(ra6+float64(s*ra7)))))))))))))
		S = 1 + float64(s*(sa1+float64(s*(sa2+float64(s*(sa3+float64(s*(sa4+float64(s*(sa5+float64(s*(sa6+float64(s*(sa7+float64(s*sa8)))))))))))))))
	} else { // |x| >= 1 / 0.35  ~ 2.857143
		R = rb0 + float64(s*(rb1+float64(s*(rb2+float64(s*(rb3+float64(s*(rb4+float64(s*(rb5+float64(s*rb6)))))))))))
		S = 1 + float64(s*(sb1+float64(s*(sb2+float64(s*(sb3+float64(s*(sb4+float64(s*(sb5+float64(s*(sb6+float64(s*sb7)))))))))))))
	}
	z := math.Float64frombits(math.Float64bits(x) & 0xffffffff00000000) // pseudo-single (20-bit) precision x
	r := Exp(-float64(z*z)-0.5625) * Exp(float64((z-x)*(z+x))+R/S)
	return r / x
}

// Erf returns the error function of x. NaN gives NaN; infinities give ±1.
func Erf(x float64) float64 {
	const (
		VeryTiny = 2.848094538889218e-306 // 0x0080000000000000
		Small    = 1.0 / (1 << 28)        // 2**-28
	)
	if x != x {
		return x
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var v float64
	switch {
	case x < Small:
		if x < VeryTiny {
			v = 0.125 * (8.0*x + float64(efx8*x)) // 8*x is exact
		} else {
			v = x + float64(efx*x)
		}
	case x < 0.84375:
		v = x + float64(x*erfSmall(x))
	case x < 1.25:
		v = erx + erfMid(x)
	case x >= 6:
		v = 1
	default:
		v = 1 - erfcTail(x)
	}
	if sign {
		return -v
	}
	return v
}

// Erfc returns the complementary error function 1 - erf(x), accurate in
// relative terms for large positive x. NaN gives NaN; +Inf gives 0 and -Inf
// gives 2.
func Erfc(x float64) float64 {
	const Tiny = 1.0 / (1 << 56) // 2**-56
	if x != x {
		return x
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	switch {
	case x < 0.84375:
		var temp float64
		switch {
		case x < Tiny:
			temp = x
		case x < 0.25:
			temp = x + float64(x*erfSmall(x))
		default:
			temp = 0.5 + (float64(x*erfSmall(x)) + (x - 0.5))
		}
		if sign {
			return 1 + temp
		}
		return 1 - temp
	case x < 1.25:
		pq := erfMid(x)
		if sign {
			return 1 + erx + pq
		}
		return 1 - erx - pq
	case x < 28:
		if sign && x > 6 {
			return 2
		}
		r := erfcTail(x)
		if sign {
			return 2 - r
		}
		return r
	}
	if sign {
		return 2
	}
	return 0
}
