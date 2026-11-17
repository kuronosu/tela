package raster

import (
	"math"
	"math/rand"
	"testing"
)

// applyBlurKernel computes, in float64, what a BlurKernel says the
// rasterizer computes along a line of device pixels starting at ox:
// pixels outside the line are 0.
func applyBlurKernel(k *BlurKernel, in []float64, ox int) []float64 {
	at := func(i int) float64 {
		if i < 0 || i >= len(in) {
			return 0
		}
		return in[i]
	}
	out := make([]float64, len(in))
	if k.Down <= 1 {
		for i := range out {
			for t, w := range k.W {
				out[i] += w * at(i+k.Lo+t)
			}
		}
		return out
	}
	f := k.Down
	block := func(b int) float64 { // the mean of the device pixels [b*f, b*f+f)
		var s float64
		for x := b * f; x < b*f+f; x++ {
			s += at(x - ox)
		}
		return s / float64(f)
	}
	low := func(b int) float64 {
		var s float64
		for t, w := range k.W {
			s += w * block(b+k.Lo+t)
		}
		return s
	}
	for x := range out {
		t := 2*(x+ox-k.Shift) + 1 - f
		i := floorDiv(t, 2*f)
		n := float64(t - 2*f*i)
		out[x] = ((float64(2*f)-n)*low(i) + n*low(i+1)) / float64(2*f)
	}
	return out
}

// TestBlurKernel checks that a BlurKernel describes what blurPlane
// computes: its weights applied in floating point give the blurred line
// within the rounding of the fixed-point passes.
func TestBlurKernel(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(7))
	const n = 3000
	line := make([]uint8, n)
	for i := range line {
		switch {
		case i%97 < 40:
			line[i] = 255
		case i%13 == 0:
			line[i] = uint8(rng.Intn(256))
		}
	}
	in := make([]float64, n)
	for i, v := range line {
		in[i] = float64(v)
	}
	ident, _ := newKernel(0, 0, nil)
	for _, s := range []float64{0, 0.3, 0.7, 1.5, 1.99, 2, 3.3, 5, 8, 12, 20.5, 21, 24, 33, 48, 64, 100, 128, 256, 300} {
		for _, off := range []float64{0, 0.4, 3.3, -7, 40, -600} {
			for _, ox := range []int{0, 5, -37} {
				k := NewBlurKernel(s, off)
				kx, _ := newKernel(s, off, nil)
				if k.Left != kx.left || k.Right != kx.right || k.Down != kx.factor() {
					t.Fatalf("s %g, off %g: kernel %+v, rasterizer's %+v", s, off, k, kx)
				}
				if k.Identity() != kx.identity() {
					t.Errorf("s %g, off %g: Identity %v, rasterizer's %v", s, off, k.Identity(), kx.identity())
				}
				var sum float64
				for _, w := range k.W {
					sum += w
				}
				if math.Abs(sum-1) > 1e-6 {
					t.Errorf("s %g, off %g: the weights add up to %v", s, off, sum)
				}
				got := append([]uint8(nil), line...)
				var bs blurScratch
				bs.blurPlane(got, n, n, 1, 1, ox, 0, &kx, &ident)
				want := applyBlurKernel(&k, in, ox)
				// Each box pass drops what it spreads beyond the line, so
				// only the pixels farther than the support from its ends
				// are the kernel's (those a layer composites).
				worst, sup := 0.0, max(k.Left, k.Right)
				for i := sup; i < n-sup; i++ {
					worst = max(worst, math.Abs(float64(got[i])-want[i]))
				}
				// Half a level of final rounding, and the passes' own.
				if worst > 0.75 {
					t.Errorf("s %g, off %g, origin %d: the kernel differs from the blur by %.3f levels", s, off, ox, worst)
				}
			}
		}
	}
	// The fractional boxes of a variance.
	for _, v := range []float64{1, 4.6, 13.75, 30, 88.15, 400} {
		w := FracBoxKernel(v)
		var sum, m2 float64
		c := len(w) / 2
		for i, x := range w {
			sum += x
			m2 += x * float64((i-c)*(i-c))
		}
		if len(w)%2 != 1 || math.Abs(sum-1) > 1e-9 || math.Abs(m2-v) > 1e-3*v {
			t.Errorf("FracBoxKernel(%g): %d weights, sum %v, variance %v", v, len(w), sum, m2)
		}
	}
}
