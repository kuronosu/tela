package raster

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// refBlend is a direct float64 transcription of W3C Compositing and
// Blending Level 1 (sections 5.8, 9 and 10) on premultiplied values in
// [0, 1], used as the oracle of blendPx.
func refBlend(mode gfx.BlendMode, s, d [4]float64) [4]float64 {
	sa, da := s[3], d[3]
	if mode == gfx.PlusLighter {
		return [4]float64{min(s[0]+d[0], 1), min(s[1]+d[1], 1), min(s[2]+d[2], 1), min(sa+da, 1)}
	}
	var cs, cb [3]float64
	for i := range 3 {
		if sa > 0 {
			cs[i] = s[i] / sa
		}
		if da > 0 {
			cb[i] = d[i] / da
		}
	}
	sep := func(f func(b, s float64) float64) (out [3]float64) {
		for i := range 3 {
			out[i] = f(cb[i], cs[i])
		}
		return out
	}
	multiply := func(b, s float64) float64 { return b * s }
	screen := func(b, s float64) float64 { return b + s - b*s }
	hard := func(b, s float64) float64 {
		if s <= 0.5 {
			return multiply(b, 2*s)
		}
		return screen(b, 2*s-1)
	}
	var bl [3]float64
	switch mode {
	case gfx.SrcOver:
		bl = cs
	case gfx.Multiply:
		bl = sep(multiply)
	case gfx.Screen:
		bl = sep(screen)
	case gfx.Overlay:
		bl = sep(func(b, s float64) float64 { return hard(s, b) })
	case gfx.Darken:
		bl = sep(math.Min)
	case gfx.Lighten:
		bl = sep(math.Max)
	case gfx.ColorDodge:
		bl = sep(func(b, s float64) float64 {
			switch {
			case b == 0:
				return 0
			case s == 1:
				return 1
			}
			return math.Min(1, b/(1-s))
		})
	case gfx.ColorBurn:
		bl = sep(func(b, s float64) float64 {
			switch {
			case b == 1:
				return 1
			case s == 0:
				return 0
			}
			return 1 - math.Min(1, (1-b)/s)
		})
	case gfx.HardLight:
		bl = sep(hard)
	case gfx.SoftLight:
		bl = sep(func(b, s float64) float64 {
			if s <= 0.5 {
				return b - (1-2*s)*b*(1-b)
			}
			dd := math.Sqrt(b)
			if b <= 0.25 {
				dd = ((16*b-12)*b + 4) * b
			}
			return b + (2*s-1)*(dd-b)
		})
	case gfx.Difference:
		bl = sep(func(b, s float64) float64 { return math.Abs(b - s) })
	case gfx.Exclusion:
		bl = sep(func(b, s float64) float64 { return b + s - 2*b*s })
	case gfx.Hue:
		bl = setLum(setSat(cs, sat(cb)), lum(cb))
	case gfx.Saturation:
		bl = setLum(setSat(cb, sat(cs)), lum(cb))
	case gfx.ColorBlend:
		bl = setLum(cs, lum(cb))
	case gfx.Luminosity:
		bl = setLum(cb, lum(cs))
	}
	var o [4]float64
	for i := range 3 {
		o[i] = s[i]*(1-da) + d[i]*(1-sa) + sa*da*bl[i]
	}
	o[3] = sa + da - sa*da
	return o
}

func randPremul(rng *rand.Rand) [4]uint8 {
	a := uint8(rng.IntN(256))
	switch rng.IntN(5) {
	case 0:
		a = 255
	case 1:
		a = uint8(1 + rng.IntN(3))
	}
	var p [4]uint8
	for i := range 3 {
		p[i] = uint8(rng.IntN(int(a) + 1))
		if rng.IntN(6) == 0 {
			p[i] = []uint8{0, a}[rng.IntN(2)]
		}
	}
	p[3] = a
	return p
}

// TestBlendModes checks every blend mode against the float64 W3C formulas
// on random premultiplied pixels: within one level (rounding), the alpha
// exact, and colors never above alpha.
func TestBlendModes(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 74))
	for _, mode := range allBlendModes[1:] {
		worst := 0.0
		for k := 0; k < 20000; k++ {
			s, d := randPremul(rng), randPremul(rng)
			if s[3] == 0 {
				continue
			}
			got := blendPx(mode, s, d)
			var fs, fd [4]float64
			for i := range 4 {
				fs[i], fd[i] = float64(s[i])/255, float64(d[i])/255
			}
			want := refBlend(mode, fs, fd)
			for i := range 4 {
				diff := math.Abs(float64(got[i]) - want[i]*255)
				worst = max(worst, diff)
				if diff > 1 || (i < 3 && got[i] > got[3]) {
					t.Fatalf("%s: s %v d %v: got %v, want %.2f (channel %d)", mode, s, d, got, scale255(want), i)
				}
			}
		}
		t.Logf("%-12s worst %.3f levels", mode, worst)
	}
}

func scale255(v [4]float64) [4]float64 {
	return [4]float64{v[0] * 255, v[1] * 255, v[2] * 255, v[3] * 255}
}

// TestBlendIdentities checks the cases that bound a blend layer's effect:
// a transparent source leaves the backdrop, a transparent backdrop takes
// the source, and SrcOver through compositeLayer equals composite.
func TestBlendIdentities(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, mode := range allBlendModes[1:] {
		for k := 0; k < 200; k++ {
			s := randPremul(rng)
			if s[3] == 0 {
				continue
			}
			if got := blendPx(mode, s, [4]uint8{}); got != s {
				t.Fatalf("%s onto transparent: %v, want the source %v", mode, got, s)
			}
		}
	}
	// compositeLayer with SrcOver and no mask is composite.
	src := make([]uint8, 4*64)
	for i := 0; i < 64; i++ {
		p := randPremul(rng)
		copy(src[4*i:], p[:])
	}
	for _, op := range []uint8{255, 200, 3} {
		a, b := image.NewRGBA(image.Rect(0, 0, 64, 1)), image.NewRGBA(image.Rect(0, 0, 64, 1))
		background(a)
		copy(b.Pix, a.Pix)
		ba := blitter{pix: a.Pix, stride: a.Stride, clip: a.Rect}
		bb := blitter{pix: b.Pix, stride: b.Stride, clip: b.Rect}
		ba.composite(0, 0, src, op)
		bb.compositeLayer(0, 0, src, op, nil, gfx.SrcOver)
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				t.Fatalf("op %d: compositeLayer SrcOver differs from composite at byte %d: %d, %d", op, i, b.Pix[i], a.Pix[i])
			}
		}
	}
}
