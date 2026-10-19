package raster

import (
	"fmt"
	"image"
	"math"
	"slices"
	"testing"
)

// gaussLine returns the Gaussian (standard deviation s) sampled at the
// offsets i - c - off, truncated at 3s and normalized: the response of the
// exact kernel to a unit impulse at c.
func gaussLine(n, c int, s, off float64) []float64 {
	out := make([]float64, n)
	var sum float64
	for i := range out {
		d := float64(i-c) - off
		if math.Abs(d) <= 3*s {
			out[i] = refI((d+1)/s) - 2*refI(d/s) + refI((d-1)/s)
			sum += out[i]
		}
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// gaussStep returns the response of an ideal Gaussian (standard deviation
// s, shifted by off) to a step at c: the samples of Phi.
func gaussStep(n, c int, s, off float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = refPhi((float64(i-c) + 0.5 - off) / s)
	}
	return out
}

// TestKernelSupport checks that no kernel reads farther than the reach that
// gfx.FilterOutset promises: 3 sigma plus the length of the offset.
func TestKernelSupport(t *testing.T) {
	for s := 0.0; s < 40; s += 0.0625 {
		for _, off := range []float64{0, 0.3, -0.5, 1, 2.5, -3.7, 7.25, 20} {
			k, _ := newKernel(s, off, nil)
			limit := 3*s + math.Abs(off)
			if float64(k.left) > limit+1e-9 || float64(k.right) > limit+1e-9 {
				t.Fatalf("s=%v off=%v: support %d/%d beyond %v (box %v)", s, off, k.left, k.right, limit, k.box)
			}
			if k.down > 1 {
				continue // TestDownsampledBlur
			}
			// The kernel's real extent matches its declared support.
			n := 2*(k.support()+2) + 1
			in := make([]int64, n)
			c := n / 2
			in[c] = 1 << 20
			out, a, b := make([]int64, n), make([]int64, n), make([]int64, n)
			k.convolve(in, out, a, b, n, 1, make([]int64, 1))
			for i, v := range out {
				if v != 0 && (i-c > k.left || c-i > k.right) {
					t.Fatalf("s=%v off=%v: output at %d beyond the support %d/%d", s, off, i-c, k.left, k.right)
				}
			}
		}
	}
}

// TestKernelAccuracy compares the exact kernels' impulse responses with the
// sampled Gaussian, and every full-resolution kernel's step response with
// the ideal Gaussian's: the three-box approximation (the SVG
// recommendation, about 6% narrower than the Gaussian) stays within 3% of
// a step. (TestDownsampledBlur compares the downsampled kernels with these.)
func TestKernelAccuracy(t *testing.T) {
	for _, s := range []float64{0.3, 0.5, 1, 1.9, 2, 3, 4, 8, 12, 30, 100} {
		for _, off := range []float64{0, 0.5, 3} {
			k, _ := newFullKernel(s, off, 3*s+math.Abs(off), nil)
			n := 2*(k.support()+int(3*s)+4) + 1
			c := n / 2
			conv := func(in []int64) []float64 {
				out, a, b := make([]int64, n), make([]int64, n), make([]int64, n)
				k.convolve(in, out, a, b, n, 1, make([]int64, 1))
				f := make([]float64, n)
				for i, v := range out {
					f[i] = float64(v) / float64(k.divisor())
				}
				return f
			}
			if !k.box {
				in := make([]int64, n)
				in[c] = 1
				got, want := conv(in), gaussLine(n, c, s, off)
				for i := range got {
					if math.Abs(got[i]-want[i]) > 1e-4 {
						t.Errorf("s=%v off=%v: impulse response %v at %d, want %v", s, off, got[i], i-c, want[i])
						break
					}
				}
			}
			in := make([]int64, n)
			for i := c; i < n; i++ {
				in[i] = 1
			}
			got, want := conv(in), gaussStep(n, c, s, off)
			var worst float64
			for i := c - n/4; i < c+n/4; i++ {
				worst = math.Max(worst, math.Abs(got[i]-want[i]))
			}
			t.Logf("s=%v off=%v box=%v: step error %.4f", s, off, k.box, worst)
			lim := 0.03
			if s < 1 {
				lim = 0.1 // sampling a narrow Gaussian at whole pixels
			}
			if k.box && off != math.Round(off) {
				lim += 0.5 / s // the offset is rounded to whole pixels
			}
			if worst > lim {
				t.Errorf("s=%v off=%v box=%v: step error %.4f", s, off, k.box, worst)
			}
		}
	}
}

// TestBlurPlane checks the two-pass blur of a plane: an impulse becomes the
// outer product of the 1D responses, for 1 and 4 channels and planes wider
// than a strip.
func TestBlurPlane(t *testing.T) {
	for _, nch := range []int{1, 4} {
		for _, s := range []float64{0.7, 5} {
			t.Run(fmt.Sprintf("nch%d_s%v", nch, s), func(t *testing.T) {
				w, h := 41, 37
				pix := make([]uint8, w*h*nch)
				cx, cy := 23, 17
				for ch := 0; ch < nch; ch++ {
					pix[(cy*w+cx)*nch+ch] = 255
				}
				k, _ := newKernel(s, 0, nil)
				var sc blurScratch
				sc.blurPlane(pix, w*nch, w, h, nch, 0, 0, &k, &k)
				gx, gy := gaussLine(w, cx, s, 0), gaussLine(h, cy, s, 0)
				lim := 1.5
				if k.box {
					lim = 0.12 * 255 / (2 * math.Pi * s * s)
				}
				var worst float64
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						for ch := 0; ch < nch; ch++ {
							got := float64(pix[(y*w+x)*nch+ch])
							worst = math.Max(worst, math.Abs(got-255*gx[x]*gy[y]))
						}
					}
				}
				if worst > lim+1 {
					t.Errorf("max error %.2f levels", worst)
				}
			})
		}
	}
}

// TestGaussTables checks the interpolated Phi and I against the exact
// functions.
func TestGaussTables(t *testing.T) {
	var worstPhi, worstI float64
	for x := -10.0; x <= 10; x += 0.000731 {
		worstPhi = math.Max(worstPhi, math.Abs(normCDF(x)-refPhi(x)))
		worstI = math.Max(worstI, math.Abs(gaussI(x)-refI(x)))
	}
	t.Logf("max error: Phi %.3g, I %.3g", worstPhi, worstI)
	if worstPhi > 1e-9 || worstI > 1e-9 {
		t.Errorf("max error: Phi %.3g, I %.3g", worstPhi, worstI)
	}
}

// convolve and boxPass are the exact reference of the kernels: int64
// arithmetic without rounding between the passes.

// convolve applies k to a line of n samples with nch interleaved channels,
// src to dst (both n*nch long; a and b are scratch of the same length). The
// results are not normalized: divide by k.divisor().
func (k *gaussKernel) convolve(src, dst, a, b []int64, n, nch int, sums []int64) {
	if k.box {
		boxPass(src, a, n, nch, k.l[0], k.r[0], sums)
		boxPass(a, b, n, nch, k.l[1], k.r[1], sums)
		boxPass(b, dst, n, nch, k.l[2], k.r[2], sums)
		return
	}
	clear(dst[:n*nch])
	for t, wt := range k.w {
		sh := k.lo + t // dst[i] += wt * src[i+sh]
		i0, i1 := max(0, -sh), min(n, n-sh)
		if i0 >= i1 {
			continue
		}
		s := src[(i0+sh)*nch : (i1+sh)*nch]
		d := dst[i0*nch : i1*nch]
		for j := range d {
			d[j] += wt * s[j]
		}
	}
}

// divisor returns the sum of the kernel's weights.
func (k *gaussKernel) divisor() int64 {
	if k.box {
		return k.div
	}
	return weightOne
}

// boxPass writes dst[i] = src[i-l] + ... + src[i+r] (samples outside
// [0, n) count as 0), per channel. l + r + 1 > 0; either may be negative.
func boxPass(src, dst []int64, n, nch, l, r int, sums []int64) {
	sums = sums[:nch]
	clear(sums)
	for t := max(0, -l); t <= min(n-1, r); t++ {
		row := src[t*nch : t*nch+nch]
		for c := range sums {
			sums[c] += row[c]
		}
	}
	for i := 0; i < n; i++ {
		copy(dst[i*nch:i*nch+nch], sums)
		if j := i + r + 1; j >= 0 && j < n {
			row := src[j*nch : j*nch+nch]
			for c := range sums {
				sums[c] += row[c]
			}
		}
		if j := i - l; j >= 0 && j < n {
			row := src[j*nch : j*nch+nch]
			for c := range sums {
				sums[c] -= row[c]
			}
		}
	}
}

// downPlane returns a w x h plane (nch channels) with an opaque disc and a
// translucent bar: hard edges, the worst case for a blur's shape.
func downPlane(w, h, nch int, s float64) []uint8 {
	pix := make([]uint8, w*h*nch)
	cx, cy, r := float64(w)/2-0.3*s, float64(h)/2, 0.8*s+3
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			var v uint8
			if dx*dx+dy*dy <= r*r {
				v = 255
			} else if x >= w/2 && x < w/2+int(s)+5 && y >= h/2-int(s) && y < h/2+3 {
				v = 140
			}
			for c := 0; c < nch; c++ {
				pix[(y*w+x)*nch+c] = v >> uint(c)
			}
		}
	}
	return pix
}

// TestDownsampledBlur compares the downsampled blur of large standard
// deviations with the full-resolution three-box blur (Chromium's), on hard
// edges, with offsets and planes at odd device origins.
func TestDownsampledBlur(t *testing.T) {
	sigmas := []float64{21.3, 24, 31.9, 32, 40, 48, 64, 100, 128, 200, 256}
	if testing.Short() {
		sigmas = []float64{24, 48, 128}
	}
	var worst int
	for _, s := range sigmas {
		for _, off := range []float64{0, 9.4} {
			kd, _ := newKernel(s, off, nil)
			if kd.down < 2 {
				t.Errorf("s=%v: not downsampled", s)
				continue
			}
			limit := 3*s + math.Abs(off)
			kf, _ := newFullKernel(s, off, limit, nil)
			if float64(kd.left) > limit || float64(kd.right) > limit {
				t.Errorf("s=%v off=%v: support %d/%d beyond %v", s, off, kd.left, kd.right, limit)
			}
			ky, _ := newKernel(s, 0, nil)
			kyf, _ := newFullKernel(s, 0, 3*s, nil)
			n := int(2*s) + 40
			w, h := n+2*kd.support()+2, n+2*ky.support()+2
			nch := 1
			if s == 24 {
				nch = 4
			}
			src := downPlane(w, h, nch, s)
			got, want := slices.Clone(src), slices.Clone(src)
			var sc blurScratch
			sc.blurPlane(got, w*nch, w, h, nch, -37, 1003, &kd, &ky)
			sc.blurPlane(want, w*nch, w, h, nch, 0, 0, &kf, &kyf)
			m := 0
			for i := range got {
				m = max(m, iabs(int(got[i])-int(want[i])))
			}
			worst = max(worst, m)
			t.Logf("s=%v off=%v: factor %d, box m=%d, support %d/%d (limit %.1f): max difference %d", s, off, kd.down, kd.fm, kd.left, kd.right, limit, m)
			if m > 2 {
				t.Errorf("s=%v off=%v: max difference %d from the full-resolution blur", s, off, m)
			}
		}
	}
	t.Logf("worst %d", worst)
}

// TestBlurPlanePool checks that splitting the stages of a blur among the
// goroutines of a workPool does not change a pixel: full-resolution box
// and exact kernels, offsets and downsampled kernels, 1 and 4 channels,
// whole planes and windows.
func TestBlurPlanePool(t *testing.T) {
	for _, nch := range []int{1, 4} {
		for _, k := range []struct{ sx, sy, off float64 }{{1.3, 0.7, 0}, {5, 9, 2.6}, {24, 3, 0}, {40, 64, -7.2}} {
			kx, _ := newKernel(k.sx, k.off, nil)
			ky, _ := newKernel(k.sy, 0, nil)
			w, h := 300, 260
			src := downPlane(w, h, nch, 30)
			for _, out := range []image.Rectangle{image.Rect(0, 0, w, h), image.Rect(37, 51, 211, 130)} {
				want := slices.Clone(src)
				var sc blurScratch
				sc.blurPlaneOut(want, w*nch, w, h, nch, 13, -9, out, &kx, &ky)
				for _, n := range []int{2, 3, 7} {
					got := slices.Clone(src)
					sc := blurScratch{par: &workPool{n: n}}
					sc.blurPlaneOut(got, w*nch, w, h, nch, 13, -9, out, &kx, &ky)
					for y := out.Min.Y; y < out.Max.Y; y++ {
						i, j := (y*w+out.Min.X)*nch, (y*w+out.Max.X)*nch
						if !slices.Equal(got[i:j], want[i:j]) {
							t.Fatalf("nch=%d %+v out=%v, %d goroutines: row %d differs", nch, k, out, n, y)
						}
					}
				}
			}
		}
	}
}
