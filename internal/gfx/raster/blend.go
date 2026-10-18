package raster

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Blend modes and layer masks.
//
// A layer is composited at PopLayer. Its premultiplied pixel s is first
// scaled by the layer's opacity times its mask's coverage (their 8-bit
// product, applied to each channel with rounding), then combined with the
// backdrop pixel d by the
// layer's blend mode, giving r; the coverage m of the clip active at
// PushLayer then interpolates between the backdrop and that result:
// out = d + (r - d)·m. This is what Skia does for blend modes that cannot
// fold coverage into the source (and what CSS asks for: opacity and masks
// belong to the element, the clip to its ancestors). For SrcOver the
// interpolation equals scaling the source by m, so an unmasked SrcOver
// layer keeps the single-rounding composite of blitter.composite.
//
// The blend modes follow W3C Compositing and Blending Level 1 on
// premultiplied sRGB-encoded values (ADR-007), with the general formula
//
//	co = cs·(1 - αb) + cb·(1 - αs) + αs·αb·B(Cb, Cs)
//	αo = αs + αb·(1 - αs)
//
// where cs, cb are premultiplied and Cs, Cb their unpremultiplied values.
// The separable modes whose B is a polynomial (multiply, screen, overlay,
// darken, lighten, hard-light, difference, exclusion) are expanded into
// premultiplied integer expressions over the denominator 255² and rounded
// once, exactly; color-dodge, color-burn, soft-light and the non-separable
// modes (hue, saturation, color, luminosity; Lum with 0.3, 0.59, 0.11)
// are evaluated in float64 on unpremultiplied values, with every product
// rounded explicitly (ADR-027). plus-lighter is min(1, s + d) on every
// channel, alpha included (CSS Compositing 2). Every mode leaves the
// backdrop unchanged where the source is transparent, so a layer only
// touches the pixels inside its bounds, and every mode returns the source
// where the backdrop is transparent.

// compositeLayer composites a layer row onto row y of the surface starting
// at x: src holds n premultiplied pixels, op is the layer's opacity, mask
// (nil for none) the coverage of its mask for these pixels, and mode its
// blend mode. The active clip mask interpolates the result.
func (b *blitter) compositeLayer(y, x int, src []uint8, op uint8, mask []uint8, mode gfx.BlendMode) {
	n := len(src) / 4
	row := b.pix[b.offset(x, y):]
	row = row[: 4*n : 4*n]
	var cm []uint8
	if b.cm.data != nil && !b.cm.innerSpan(y, x, n) {
		cm = b.cm.row(y, x, n)
	}
	if mode == gfx.SrcOver {
		// A masked source-over layer: the coverage is opacity × mask ×
		// clip, rounded once per pixel by blendPixel.
		for i := 0; i < n; i++ {
			s := src[4*i : 4*i+4 : 4*i+4]
			if s[3] == 0 { // premultiplied: R, G, B are 0 too
				continue
			}
			cov := op
			if mask != nil {
				cov = mul8(cov, mask[i])
			}
			if cm != nil {
				cov = mul8(cov, cm[i])
			}
			d := row[4*i : 4*i+4 : 4*i+4]
			switch {
			case cov == 0:
			case cov == 255 && s[3] == 255:
				d[0], d[1], d[2], d[3] = s[0], s[1], s[2], s[3]
			default:
				blendPixel(d, uint32(s[0]), uint32(s[1]), uint32(s[2]), uint32(s[3]), uint32(cov))
			}
		}
		return
	}
	// Scale the source by the opacity and the mask.
	s := src
	if op != 255 || mask != nil {
		s = grow(&b.scratch, 4*n)
		for i := 0; i < n; i++ {
			k := op
			if mask != nil {
				k = mul8(k, mask[i])
			}
			p, q := src[4*i:4*i+4:4*i+4], s[4*i:4*i+4:4*i+4]
			if k == 255 {
				q[0], q[1], q[2], q[3] = p[0], p[1], p[2], p[3]
				continue
			}
			q[0], q[1], q[2], q[3] = mul8(p[0], k), mul8(p[1], k), mul8(p[2], k), mul8(p[3], k)
		}
	}
	if cm == nil {
		blendRow(mode, row, row, s)
		return
	}
	out := grow(&b.blendBuf, 4*n)
	blendRow(mode, out, row, s)
	for i, m := range cm {
		d, r := row[4*i:4*i+4:4*i+4], out[4*i:4*i+4:4*i+4]
		switch m {
		case 0:
		case 255:
			d[0], d[1], d[2], d[3] = r[0], r[1], r[2], r[3]
		default:
			im := 255 - uint32(m)
			for c := range 4 {
				d[c] = div255(uint32(r[c])*uint32(m) + uint32(d[c])*im)
			}
		}
	}
}

// grow returns the first n bytes of *buf, growing it if needed.
func grow(buf *[]uint8, n int) []uint8 {
	if cap(*buf) < n {
		*buf = make([]uint8, n)
	}
	return (*buf)[:n]
}

// blendRow writes to out the result of compositing the premultiplied
// pixels s onto d with mode (not SrcOver); out may be d. The separable
// integer modes have a loop each, so that the per-channel expression is
// inlined; the others go through blendPx.
func blendRow(mode gfx.BlendMode, out, d, s []uint8) {
	switch mode {
	case gfx.Multiply:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(sc*(255-da)+dc*(255-sa)+sc*dc, a)
			}
			o[3] = a
		}
	case gfx.Screen:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(255*(sc+dc)-sc*dc, a)
			}
			o[3] = a
		}
	case gfx.Overlay:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(sc*(255-da)+dc*(255-sa)+hardLight(dc, da, sc, sa), a)
			}
			o[3] = a
		}
	case gfx.HardLight:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(sc*(255-da)+dc*(255-sa)+hardLight(sc, sa, dc, da), a)
			}
			o[3] = a
		}
	case gfx.Darken:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(255*(sc+dc)-max(sc*da, dc*sa), a)
			}
			o[3] = a
		}
	case gfx.Lighten:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(255*(sc+dc)-min(sc*da, dc*sa), a)
			}
			o[3] = a
		}
	case gfx.Difference:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(255*(sc+dc)-2*min(sc*da, dc*sa), a)
			}
			o[3] = a
		}
	case gfx.Exclusion:
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			sa, da := int32(sp[3]), int32(dp[3])
			if sa == 0 || da == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			a := alphaOver(sa, da)
			for c := 0; c < 3; c++ {
				sc, dc := int32(sp[c]), int32(dp[c])
				o[c] = chan8(255*(sc+dc)-2*sc*dc, a)
			}
			o[3] = a
		}
	case gfx.PlusLighter:
		for i := range s {
			out[i] = uint8(min(uint32(s[i])+uint32(d[i]), 255))
		}
	default:
		// The float modes are slow: neighbors often repeat the same pair
		// of pixels (gradients, flat colors), so the last result is kept.
		var lastS, lastD [4]uint8
		var last [4]uint8
		valid := false
		for i := 0; i+3 < len(s); i += 4 {
			sp, dp, o := s[i:i+4:i+4], d[i:i+4:i+4], out[i:i+4:i+4]
			if sp[3] == 0 || dp[3] == 0 {
				srcOrDst(o, sp, dp)
				continue
			}
			ps, pd := [4]uint8{sp[0], sp[1], sp[2], sp[3]}, [4]uint8{dp[0], dp[1], dp[2], dp[3]}
			if !valid || ps != lastS || pd != lastD {
				last, lastS, lastD, valid = blendPx(mode, ps, pd), ps, pd, true
			}
			o[0], o[1], o[2], o[3] = last[0], last[1], last[2], last[3]
		}
	}
}

// srcOrDst sets o to the result of every blend mode where the source or
// the backdrop is transparent: the backdrop if the source is, else the
// source.
func srcOrDst(o, s, d []uint8) {
	if s[3] == 0 {
		o[0], o[1], o[2], o[3] = d[0], d[1], d[2], d[3]
		return
	}
	o[0], o[1], o[2], o[3] = s[0], s[1], s[2], s[3]
}

// alphaOver returns the 8-bit αs + αb - αs·αb.
func alphaOver(sa, da int32) uint8 { return uint8(rdiv255(uint32(255*(sa+da) - sa*da))) }

// chan8 rounds a color numerator over 255² to 8 bits, at most alpha a.
func chan8(v int32, a uint8) uint8 { return min(uint8(rdiv255(uint32(max(v, 0)))), a) }

// rdiv255 returns round(v / 255) exactly (255 is odd: no ties).
func rdiv255(v uint32) uint32 { return (v + 127) / 255 }

// div255 returns round(v / 255) exactly, for v <= 255·255.
func div255(v uint32) uint8 {
	v += 128
	return uint8((v + v>>8) >> 8)
}

// blendPx returns the premultiplied result of compositing the source s
// onto the backdrop d with mode (any mode but SrcOver), each channel
// rounded to nearest. s is not transparent. blendRow is the fast form of
// the same arithmetic.
func blendPx(mode gfx.BlendMode, s, d [4]uint8) [4]uint8 {
	var o [4]uint8
	switch mode {
	case gfx.ColorDodge, gfx.ColorBurn, gfx.SoftLight, gfx.Hue, gfx.Saturation, gfx.ColorBlend, gfx.Luminosity:
		if d[3] == 0 {
			return s
		}
		// αo = αs + αb - αs·αb; the numerators of the integer modes are
		// 255² times the result, so dividing by 255 gives 8 bits.
		o[3] = alphaOver(int32(s[3]), int32(d[3]))
		blendFloat(mode, &s, &d, &o)
		return o
	}
	blendRow(mode, o[:], d[:], s[:])
	return o
}

// hardLight returns αs·αb·HardLight(Cb, Cs) over 255² for premultiplied
// s, d: Multiply(Cb, 2·Cs) for Cs <= 1/2, else Screen(Cb, 2·Cs - 1).
// Overlay is hard-light with the roles swapped.
func hardLight(s, sa, d, da int32) int32 {
	if 2*s <= sa {
		return 2 * s * d
	}
	return sa*da - 2*(da-d)*(sa-s)
}

// blendFloat computes the color channels of the modes that need divisions,
// square roots or the non-separable functions, in float64 (fusion-safe:
// every product that feeds an addition is rounded explicitly). s and d are
// not transparent; o[3] holds the result's alpha.
func blendFloat(mode gfx.BlendMode, s, d *[4]uint8, o *[4]uint8) {
	// Divisions are slow: multiply by reciprocals, every product rounded
	// explicitly (it is stored, and later added).
	const inv255 = 1.0 / 255
	sa, da := float64(float64(s[3])*inv255), float64(float64(d[3])*inv255)
	is, id := 1/float64(s[3]), 1/float64(d[3])
	var cs, cb, ps, pd [3]float64
	for c := range 3 {
		ps[c], pd[c] = float64(float64(s[c])*inv255), float64(float64(d[c])*inv255)
		// Unpremultiplied values; 1 exactly when the channel equals alpha
		// (color-dodge and color-burn test for 1).
		cs[c], cb[c] = 1, 1
		if s[c] < s[3] {
			cs[c] = float64(float64(s[c]) * is)
		}
		if d[c] < d[3] {
			cb[c] = float64(float64(d[c]) * id)
		}
	}
	var bl [3]float64
	switch mode {
	case gfx.ColorDodge:
		for c := range 3 {
			switch {
			case cb[c] == 0:
				bl[c] = 0
			case cs[c] >= 1:
				bl[c] = 1
			default:
				bl[c] = min(1, cb[c]/(1-cs[c]))
			}
		}
	case gfx.ColorBurn:
		for c := range 3 {
			switch {
			case cb[c] >= 1:
				bl[c] = 1
			case cs[c] <= 0:
				bl[c] = 0
			default:
				bl[c] = 1 - min(1, (1-cb[c])/cs[c])
			}
		}
	case gfx.SoftLight:
		for c := range 3 {
			b, f := cb[c], cs[c]
			if f <= 0.5 {
				bl[c] = b - float64(float64(float64(1-2*f)*b)*(1-b))
				continue
			}
			var dd float64
			if b <= 0.25 {
				// ((16·b - 12)·b + 4)·b; 16·b is exact.
				t := float64((16*b-12)*b) + 4
				dd = float64(t * b)
			} else {
				dd = math.Sqrt(b)
			}
			bl[c] = b + float64(float64(2*f-1)*(dd-b))
		}
	case gfx.Hue:
		bl = setLum(setSat(cs, sat(cb)), lum(cb))
	case gfx.Saturation:
		bl = setLum(setSat(cb, sat(cs)), lum(cb))
	case gfx.ColorBlend:
		bl = setLum(cs, lum(cb))
	case gfx.Luminosity:
		bl = setLum(cb, lum(cs))
	}
	k := float64(sa * da)
	for c := range 3 {
		v := float64(ps[c]*(1-da)) + float64(pd[c]*(1-sa)) + float64(k*bl[c])
		o[c] = min(unit8(v), o[3])
	}
}

// lum, setLum, sat and setSat are the non-separable helpers of W3C
// Compositing and Blending Level 1, section 10.
func lum(c [3]float64) float64 {
	return float64(0.3*c[0]) + float64(0.59*c[1]) + float64(0.11*c[2])
}

func setLum(c [3]float64, l float64) [3]float64 {
	d := l - lum(c)
	c = [3]float64{c[0] + d, c[1] + d, c[2] + d}
	// ClipColor.
	l = lum(c)
	n, x := min(c[0], c[1], c[2]), max(c[0], c[1], c[2])
	for i := range 3 {
		if n < 0 && l-n != 0 {
			c[i] = l + float64(float64((c[i]-l)*l)/(l-n))
		}
		if x > 1 && x-l != 0 {
			c[i] = l + float64(float64((c[i]-l)*(1-l))/(x-l))
		}
	}
	return c
}

func sat(c [3]float64) float64 { return max(c[0], c[1], c[2]) - min(c[0], c[1], c[2]) }

// setSat gives c the saturation s: its smallest component becomes 0, its
// largest s and the middle one is scaled in proportion.
func setSat(c [3]float64, s float64) [3]float64 {
	mn, mx := min(c[0], c[1], c[2]), max(c[0], c[1], c[2])
	var out [3]float64
	if mx > mn {
		for i := range 3 {
			out[i] = float64((c[i]-mn)*s) / (mx - mn)
		}
	}
	return out
}

// maskCoverage converts the premultiplied pixels of a rendered mask
// (stride 4·w over rect) into 8-bit coverage over comp (stride comp.Dx()).
// Luminance is 0.2126 R + 0.7152 G + 0.0722 B of the premultiplied values,
// which is the luminance of the color times its alpha, in 16-bit fixed
// point (the weights add up to 65536).
func maskCoverage(dst []uint8, pix []uint8, w int, mode gfx.MaskMode) {
	if mode == gfx.MaskLuminance {
		for i := range dst {
			p := pix[4*i : 4*i+3 : 4*i+3]
			dst[i] = uint8((13933*uint32(p[0]) + 46871*uint32(p[1]) + 4732*uint32(p[2]) + 32768) >> 16)
		}
		return
	}
	for i := range dst {
		dst[i] = pix[4*i+3]
	}
}
