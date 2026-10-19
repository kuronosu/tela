package raster

import (
	"encoding/binary"
	"image"
	"math"
	"math/bits"
)

// Separable Gaussian blur on premultiplied 8-bit planes.
//
// A blur is two 1D passes, horizontal then vertical, on 8.8 fixed-point
// values (uint16); pixels outside the plane count as transparent. Each box
// pass and each exact kernel rounds back to 8.8 (at most 1/256 of a level),
// and the result is rounded to 8 bits once at the end. Everything is integer
// arithmetic, so the result is the same on every architecture, and every
// channel goes through the same monotonic map, so premultiplied colors stay
// valid (R, G, B <= A).
//
// Kernels follow the SVG / Filter Effects recommendation for feGaussianBlur:
// for a device standard deviation s >= 2, three box blurs of size
// d = floor(s*3*sqrt(2*pi)/4 + 0.5) (for an even d, two boxes of size d
// centered on the pixel boundaries left and right of the output pixel and
// one of size d+1 centered on it), whose step response is within 1-2.5% of
// the Gaussian's (it is about 6% narrower). Below s = 2, exact weights: the
// Gaussian convolved with the pixels reconstructed linearly between their
// centers, (G * triangle)(t) = s*(I((t+1)/s) - 2I(t/s) + I((t-1)/s)) with I
// the integral of the normal CDF (gaussI), truncated at 3s and normalized;
// point samples of the Gaussian are worse below s = 1 (5% of a step at
// s = 0.5, against 3.4%; both are limited by the truncation at 3s that the
// reach requires).
//
// A kernel may also shift its output by an offset (drop-shadow). Its
// support never exceeds 3s + |offset|, the reach gfx.FilterOutset promises:
// the box kernel is used only when its support, with the offset rounded to
// whole pixels, fits; otherwise the exact kernel takes the fractional offset
// into its weights.
//
// Large standard deviations (from about 21 device pixels, where the first
// factor fits the reach) are blurred at a reduced resolution (ADR-078, see
// newDownKernel and blurDown): within one level of the full-resolution
// three-box blur (TestDownsampledBlur) at a fraction of its cost.
//
// Only the pixels that are needed are computed: those within the support
// of nonzero pixels, and of an output rectangle (blurPlaneOut); rows,
// column strips and block rows can be split among goroutines (pool.go)
// without changing a value.

// sigmaMin is the smallest device standard deviation that blurs at all;
// below it a Gaussian changes no pixel by more than about one level.
const sigmaMin = 1.0 / 64

// maxSigma caps device standard deviations and maxOffset device offsets
// (drop-shadow), which bounds the layer surfaces (they extend by the reach
// beyond the region) and the work of absurd filters. Tailwind's largest
// blur (blur-3xl, 64px) is 128 device pixels at scale 2.
const (
	maxSigma  = 256
	maxOffset = 512
)

// weightOne is the fixed-point unit of exact kernel weights.
const weightOne = 1 << 16

// gaussKernel is a 1D convolution kernel in device pixels.
type gaussKernel struct {
	box bool
	// Box kernel: three running sums with left and right extents l[k] and
	// r[k] (pass k sums in[i-l[k]] ... in[i+r[k]]); div is the product of
	// the three box sizes.
	l, r [3]int
	div  int64
	// Exact kernel: out[i] = sum over t of w[t] * in[i+lo+t], weights in
	// units of 1/weightOne summing to weightOne.
	lo int
	w  []int64
	// Fractional box kernel (frac): three passes of a box of 2fm+1
	// samples of weight fa plus one sample of weight fb at each end, in
	// units of 2^-boxFracBits (fracBox).
	frac   bool
	fm     int
	fa, fb uint64
	// Downsampled kernel (down > 1, see blurDown): the line is averaged
	// over blocks of down pixels aligned to device coordinates, the block
	// values are blurred with the fractional box kernel (fm, fa, fb),
	// interpolated back linearly at the pixel centers and shifted by sh
	// whole pixels.
	down, sh int
	// Support: the kernel reads in[i-left] ... in[i+right] (at full
	// resolution for a downsampled kernel).
	left, right int
}

// identity reports whether the kernel changes nothing.
func (k *gaussKernel) identity() bool {
	return !k.box && !k.frac && k.down <= 1 && k.lo == 0 && len(k.w) == 1
}

// support returns how far the kernel reaches on either side.
func (k *gaussKernel) support() int { return max(k.left, k.right) }

// factor returns the downsampling factor of the kernel (1: none).
func (k *gaussKernel) factor() int { return max(k.down, 1) }

// boxSize returns the box size d of the three-box kernel of a standard
// deviation s (SVG's recommendation, also Skia's).
func boxSize(s float64) int {
	return int(math.Floor(float64(s*(3*math.Sqrt(2*math.Pi)/4)) + 0.5))
}

// boxVariance returns the variance of the three-box kernel of size d: three
// boxes of d pixels, or for an even d two of d and one of d+1.
func boxVariance(d int) float64 {
	if d%2 == 1 {
		return float64(3*(d*d-1)) / 12
	}
	return float64(2*(d*d-1)+(d+1)*(d+1)-1) / 12
}

// newKernel returns the kernel of a Gaussian blur with device standard
// deviation s whose output is shifted by off device pixels. Its weights are
// appended to the arena w, which is returned; the kernel keeps a slice of it.
func newKernel(s, off float64, w []int64) (gaussKernel, []int64) {
	if !(s >= 0) {
		s = 0
	}
	s = min(s, maxSigma)
	off = min(max(off, -maxOffset), maxOffset)
	limit := float64(3*s) + math.Abs(off) // the reach the kernel must respect
	if k, ok := newDownKernel(s, off, limit); ok {
		return k, w
	}
	return newFullKernel(s, off, limit, w)
}

// newFullKernel returns the full-resolution kernel of newKernel's
// arguments (s and off clamped, limit their reach).
func newFullKernel(s, off, limit float64, w []int64) (gaussKernel, []int64) {
	if s >= 2 {
		d := boxSize(s)
		k := gaussKernel{box: true}
		if d%2 == 1 {
			for p := range 3 {
				k.l[p], k.r[p] = d/2, d/2
			}
			k.div = int64(d) * int64(d) * int64(d)
		} else {
			k.l = [3]int{d / 2, d/2 - 1, d / 2}
			k.r = [3]int{d/2 - 1, d / 2, d / 2}
			k.div = int64(d) * int64(d) * int64(d+1)
		}
		// Shift the output by the offset rounded to whole pixels:
		// out[i] = blur[i - sh].
		sh := int(math.Round(off))
		k.l[2] += sh
		k.r[2] -= sh
		k.left = k.l[0] + k.l[1] + k.l[2]
		k.right = k.r[0] + k.r[1] + k.r[2]
		if float64(k.left) <= limit && float64(k.right) <= limit {
			return k, w
		}
	}
	start := len(w)
	if s >= sigmaMin {
		// Taps t with |t + off| <= 3s; the weight of tap t is the Gaussian
		// centered on -off, convolved with the triangle, at t.
		t0 := int(math.Ceil(-off - float64(3*s)))
		t1 := int(math.Floor(-off + float64(3*s)))
		if w2, ok := appendTaps(w, s, off, t0, t1); ok {
			w = w2
			return gaussKernel{lo: t0, w: w[start:], left: max(-t0, 0), right: max(t1, 0)}, w
		}
	}
	// No blur: a shift by the offset truncated toward zero, which stays
	// within |off|.
	sh := int(off)
	w = append(w, weightOne)
	return gaussKernel{lo: -sh, w: w[start:], left: max(sh, 0), right: max(-sh, 0)}, w
}

// appendTaps appends to w the weights of the taps t0 ... t1 of a Gaussian
// of standard deviation s centered on -off, convolved with the triangle
// (the pixels reconstructed linearly between their centers), normalized to
// sum to weightOne exactly. ok is false when there is no weight.
func appendTaps(w []int64, s, off float64, t0, t1 int) ([]int64, bool) {
	if t0 > t1 {
		return w, false
	}
	inv := 1 / s
	tap := func(t int) float64 {
		c := float64(t) + off
		return gaussI(float64((c+1)*inv)) - 2*gaussI(float64(c*inv)) + gaussI(float64((c-1)*inv))
	}
	var sum float64
	for t := t0; t <= t1; t++ {
		sum += tap(t)
	}
	if !(sum > 0) {
		return w, false
	}
	start := len(w)
	var q, best int64
	bi := 0
	for t := t0; t <= t1; t++ {
		x := int64(math.Floor(float64(tap(t)/sum*weightOne) + 0.5))
		w = append(w, x)
		q += x
		if x > best {
			best, bi = x, t-t0
		}
	}
	w[start+bi] += weightOne - q // the weights sum to exactly one
	return w, true
}

// Large blurs are computed at a reduced resolution, as Skia and Chromium do
// on the GPU (blurDown): the plane is averaged over blocks of f x f pixels
// aligned to device coordinates, the block values are blurred with three
// passes of a fractional box (a box of 2m+1 blocks plus a weight alpha on
// the block at each end) and the result is interpolated linearly back to
// the pixel centers. The fractional box makes the variance exact: three
// passes have the variance of the full-resolution three-box kernel (what
// Chromium draws on the CPU) divided by f², minus the variance that the
// averaging and the interpolation add (downVar, in block units: 1/12 and
// 1/6), and a shape of the same family. f is the largest power of two (at
// most maxDown) whose full-resolution support, 3f(m+1) + 3f/2 - 1 plus the
// shift, fits the reach 3s + |offset| that gfx.FilterOutset promises, which
// happens from about s >= 10f: below s = 20 nothing fits and the blur is
// computed at full resolution.
const (
	maxDown = 64
	downVar = 0.25
)

// fracBox returns the fractional box of variance v (in samples²): weights 1
// on the 2m+1 samples around the center and alpha on the two samples next
// to them, normalized; a and b are the weights of a sample of the center and
// of an end in units of 2^-boxFracBits.
func fracBox(v float64) (m int, a, b uint64) {
	// A box of 2m+1 samples has variance m(m+1)/3.
	m = int(math.Floor((math.Sqrt(1+float64(12*v)) - 1) / 2))
	for m > 0 && float64(m*(m+1))/3 > v {
		m--
	}
	for float64((m+1)*(m+2))/3 <= v {
		m++
	}
	// Variance with ends alpha: (S + 2 alpha (m+1)²) / (2m+1 + 2 alpha),
	// S = m(m+1)(2m+1)/3; solve for alpha in [0, 1).
	sm := float64(m*(m+1)*(2*m+1)) / 3
	q := float64((m + 1) * (m + 1))
	alpha := (float64(v*float64(2*m+1)) - sm) / (2 * (q - v))
	alpha = min(max(alpha, 0), 1)
	width := float64(2*m+1) + 2*alpha
	a = uint64(math.Floor(boxFracOne/width + 0.5))
	b = uint64(math.Floor(float64(alpha*boxFracOne)/width + 0.5))
	return m, a, b
}

// boxFracBits is the precision of fractional box weights.
const (
	boxFracBits = 24
	boxFracOne  = 1 << boxFracBits
)

// newDownKernel returns the downsampled kernel of newKernel's arguments,
// or ok false when no factor fits the reach.
func newDownKernel(s, off, limit float64) (k gaussKernel, ok bool) {
	v := boxVariance(boxSize(s))
	sh := int(math.Round(off))
	f := maxDown
	for f > 1 && float64(f)*10 > s {
		f /= 2
	}
	for ; f >= 2; f /= 2 {
		ff := float64(f)
		vl := v/(ff*ff) - downVar
		if !(vl > 0) {
			continue
		}
		m, a, b := fracBox(vl / 3)
		ext := 3 * (m + 1) // the blocks the three passes read on each side
		left := f*ext + 3*f/2 - 1 + sh
		right := f*ext + 3*f/2 - 1 - sh
		if float64(left) > limit || float64(right) > limit {
			continue
		}
		return gaussKernel{down: f, sh: sh, fm: m, fa: a, fb: b, left: max(left, 0), right: max(right, 0)}, true
	}
	return k, false
}

// blurScratch holds the buffers of blurPlane, reused between calls.
type blurScratch struct {
	a       []uint16 // the plane, in 8.8 fixed point
	b, c    []uint16 // column strips of the vertical passes
	pad     []uint16 // a zero-padded line
	acc     []uint32
	acc64   []uint64
	zeroRow []uint16
	// Downsampled blurs: the block plane, its row accumulator, the
	// interpolation indices and weights of the output columns and two
	// interpolated block rows.
	low          []uint16
	dacc, ux, wx []uint32
	h0, h1       []uint64
	// par runs the stages of a blur on several goroutines (nil: on the
	// calling one).
	par *workPool
}

// stripWidth is the number of values (pixels times channels) per row that
// the vertical passes process at once, so that a strip stays in cache.
const stripWidth = 256

func grow16(s *[]uint16, n int) []uint16 {
	if cap(*s) < n {
		*s = make([]uint16, n)
	}
	return (*s)[:n]
}

// The plane blur works on 8.8 fixed-point values (uint16). A box pass sums
// with uint32 running sums (at most 2^25 for the largest box) and rounds the
// average back to 8.8 by multiplying with a 32-bit reciprocal; the error of
// a pass is at most 1/256 of a level, and the reference convolve (exact
// int64 arithmetic, used by the tests) agrees within one level. The passes
// over rows (vertical) update whole rows of running sums, which streams
// through memory.

// boxRecip returns the multiplier m and the rounding offset of a box of d
// samples: round(sum/d) is ((sum + d/2) * m) >> 32, up to one unit in rare
// near-ties (always the same result for the same input).
func boxRecip(d int) (uint64, uint32) {
	return (1<<32 + uint64(d) - 1) / uint64(d), uint32(d / 2)
}

// boxLine applies one box pass with extents l, r to a line of n samples with
// nch interleaved channels; src is zero padded by p samples on both sides
// (p > max(|l|, |r|)), dst is not padded.
func boxLine(src, dst []uint16, n, nch, p, l, r int) {
	m, half := boxRecip(l + r + 1)
	for c := 0; c < nch; c++ {
		var sum uint32
		for t := -l; t <= r; t++ {
			sum += uint32(src[(p+t)*nch+c])
		}
		add, sub := (p+r+1)*nch+c, (p-l)*nch+c
		for i := c; i < n*nch; i += nch {
			dst[i] = uint16((uint64(sum+half) * m) >> 32)
			sum += uint32(src[add]) - uint32(src[sub])
			add += nch
			sub += nch
		}
	}
}

// tapsLine applies an exact kernel to a padded line (see boxLine).
func (k *gaussKernel) tapsLine(src, dst []uint16, n, nch, p int) {
	// Two consecutive values per uint64 (32-bit lanes): a lane sums at
	// most weightOne * 65280 + weightOne/2 < 2^32, so it never carries.
	nv := n * nch
	base := (p + k.lo) * nch
	span := (len(k.w)-1)*nch + 2
	i := 0
	for ; i+1 < nv; i += 2 {
		seg := src[base+i : base+i+span]
		var sum uint64
		j := 0
		for _, w := range k.w {
			sum += uint64(w) * (uint64(seg[j]) | uint64(seg[j+1])<<32)
			j += nch
		}
		sum += pairHalf
		dst[i], dst[i+1] = uint16(sum>>16), uint16(sum>>48)
	}
	for ; i < nv; i++ {
		var sum uint64
		j := base + i
		for _, w := range k.w {
			sum += uint64(w) * uint64(src[j])
			j += nch
		}
		dst[i] = uint16((sum + weightOne/2) >> 16)
	}
}

// pairHalf rounds both lanes of a pair of tap sums.
const pairHalf = weightOne/2 | weightOne/2<<32

// fracLine applies one fractional box pass (see fracBox) to a padded line
// (see boxLine; p > m+1).
func fracLine(src, dst []uint16, n, nch, p, m int, a, b uint64) {
	if nch == 4 {
		fracLine4(src, dst, n, p, m, a, b)
		return
	}
	e := (m + 1) * nch
	for c := 0; c < nch; c++ {
		var sum uint64
		for t := -m; t <= m; t++ {
			sum += uint64(src[(p+t)*nch+c])
		}
		for i := c; i < n*nch; i += nch {
			j := p*nch + i
			ends := uint64(src[j-e]) + uint64(src[j+e])
			dst[i] = uint16((a*sum + b*ends + boxFracOne/2) >> boxFracBits)
			sum += uint64(src[j+e]) - uint64(src[j-e+nch])
		}
	}
}

// fracLine4 is fracLine for 4 channels, summed together.
func fracLine4(src, dst []uint16, n, p, m int, a, b uint64) {
	var s0, s1, s2, s3 uint64
	for t := -m; t <= m; t++ {
		q := src[4*(p+t) : 4*(p+t)+4 : 4*(p+t)+4]
		s0 += uint64(q[0])
		s1 += uint64(q[1])
		s2 += uint64(q[2])
		s3 += uint64(q[3])
	}
	e := 4 * (m + 1)
	for i := 0; i < n; i++ {
		j := 4 * (p + i)
		lo := src[j-e : j-e+8 : j-e+8] // the left end, then the first pixel leaving the window
		hi := src[j+e : j+e+4 : j+e+4] // the right end, entering the window
		d := dst[4*i : 4*i+4 : 4*i+4]
		d[0] = uint16((a*s0 + b*(uint64(lo[0])+uint64(hi[0])) + boxFracOne/2) >> boxFracBits)
		d[1] = uint16((a*s1 + b*(uint64(lo[1])+uint64(hi[1])) + boxFracOne/2) >> boxFracBits)
		d[2] = uint16((a*s2 + b*(uint64(lo[2])+uint64(hi[2])) + boxFracOne/2) >> boxFracBits)
		d[3] = uint16((a*s3 + b*(uint64(lo[3])+uint64(hi[3])) + boxFracOne/2) >> boxFracBits)
		s0 += uint64(hi[0]) - uint64(lo[4])
		s1 += uint64(hi[1]) - uint64(lo[5])
		s2 += uint64(hi[2]) - uint64(lo[6])
		s3 += uint64(hi[3]) - uint64(lo[7])
	}
}

// fracRows applies one fractional box pass of k over the h rows of n
// values of a strip, from src to dst.
func (s *blurScratch) fracRows(src, dst strip, n, h int, k *gaussKernel) {
	m, a, b := k.fm, k.fa, k.fb
	acc := s.acc[:n]
	clear(acc)
	for t := 0; t <= min(m, h-1); t++ {
		for x, v := range src.row(t, n) {
			acc[x] += uint32(v)
		}
	}
	for y := 0; y < h; y++ {
		out := dst.row(y, n)
		lo := s.rowAt(src, y-m-1, n, h)
		hi := s.rowAt(src, y+m+1, n, h)
		top := s.rowAt(src, y-m, n, h)
		for x := range out {
			sum := acc[x]
			ends := uint64(lo[x]) + uint64(hi[x])
			out[x] = uint16((a*uint64(sum) + b*ends + boxFracOne/2) >> boxFracBits)
			acc[x] = sum + uint32(hi[x]) - uint32(top[x])
		}
	}
}

// line blurs one line of n samples (nch channels) in place.
func (s *blurScratch) line(k *gaussKernel, line []uint16, n, nch int) {
	p := k.support() + 2
	pad := grow16(&s.pad, (n+2*p)*nch)
	clear(pad[:p*nch])
	clear(pad[(n+p)*nch:])
	body := pad[p*nch : (n+p)*nch]
	copy(body, line)
	if k.frac {
		fracLine(pad, line, n, nch, p, k.fm, k.fa, k.fb)
		copy(body, line)
		fracLine(pad, line, n, nch, p, k.fm, k.fa, k.fb)
		copy(body, line)
		fracLine(pad, line, n, nch, p, k.fm, k.fa, k.fb)
		return
	}
	if !k.box {
		k.tapsLine(pad, line, n, nch, p)
		return
	}
	boxLine(pad, line, n, nch, p, k.l[0], k.r[0])
	copy(body, line)
	boxLine(pad, line, n, nch, p, k.l[1], k.r[1])
	copy(body, line)
	boxLine(pad, line, n, nch, p, k.l[2], k.r[2])
}

// strip is a column strip of a plane: n values per row starting at off,
// rows stride values apart.
type strip struct {
	buf         []uint16
	off, stride int
}

func (st strip) row(y, n int) []uint16 {
	i := st.off + y*st.stride
	return st.buf[i : i+n : i+n]
}

// rowAt returns row j of a strip, or a row of zeros outside [0, h).
func (s *blurScratch) rowAt(src strip, j, n, h int) []uint16 {
	if j < 0 || j >= h {
		return s.zeroRow[:n]
	}
	return src.row(j, n)
}

// boxRows applies one box pass with extents l, r over the h rows of n
// values of a strip, from src to dst.
func (s *blurScratch) boxRows(src, dst strip, n, h, l, r int) {
	m, half := boxRecip(l + r + 1)
	acc := s.acc[:n]
	clear(acc)
	for t := max(-l, 0); t <= min(r, h-1); t++ {
		for x, v := range src.row(t, n) {
			acc[x] += uint32(v)
		}
	}
	for y := 0; y < h; y++ {
		out := dst.row(y, n)
		add := s.rowAt(src, y+r+1, n, h)
		sub := s.rowAt(src, y-l, n, h)
		for x := range out {
			a := acc[x]
			out[x] = uint16((uint64(a+half) * m) >> 32)
			acc[x] = a + uint32(add[x]) - uint32(sub[x])
		}
	}
}

// tapsRows applies an exact kernel over the rows of a strip.
func (s *blurScratch) tapsRows(k *gaussKernel, src, dst strip, n, h int) {
	buf := src.buf
	for y := 0; y < h; y++ {
		// The taps whose rows are inside the strip; the others read 0.
		lo := y + k.lo
		t0, t1 := max(0, -lo), min(len(k.w), h-lo)
		out := dst.row(y, n)
		if t0 >= t1 {
			clear(out)
			continue
		}
		ws := k.w[t0:t1]
		base := src.off + (lo+t0)*src.stride
		x := 0
		for ; x+1 < n; x += 2 {
			var sum uint64
			j := base + x
			for _, w := range ws {
				sum += uint64(w) * (uint64(buf[j]) | uint64(buf[j+1])<<32)
				j += src.stride
			}
			sum += pairHalf
			out[x], out[x+1] = uint16(sum>>16), uint16(sum>>48)
		}
		for ; x < n; x++ {
			var sum uint64
			j := base + x
			for _, w := range ws {
				sum += uint64(w) * uint64(buf[j])
				j += src.stride
			}
			out[x] = uint16((sum + weightOne/2) >> 16)
		}
	}
}

// blurPlane blurs a plane of w x h pixels with nch interleaved 8-bit
// channels (row y starts at pix[y*stride]) in place with kx horizontally
// and ky vertically. Pixel (0, 0) of the plane is device pixel (ox, oy):
// downsampled kernels average blocks aligned to device coordinates, so that
// the value of a pixel does not depend on where the plane starts. Pixels
// outside the plane count as 0.
func (s *blurScratch) blurPlane(pix []uint8, stride, w, h, nch, ox, oy int, kx, ky *gaussKernel) {
	s.blurPlaneOut(pix, stride, w, h, nch, ox, oy, image.Rect(0, 0, w, h), kx, ky)
}

// blurPlaneOut is blurPlane computing only the pixels of out (in plane
// coordinates); the other pixels of the plane are left with unspecified
// values. The pixels of out are those blurPlane gives.
func (s *blurScratch) blurPlaneOut(pix []uint8, stride, w, h, nch, ox, oy int, out image.Rectangle, kx, ky *gaussKernel) {
	out = out.Intersect(image.Rect(0, 0, w, h))
	if out.Empty() || (kx.identity() && ky.identity()) {
		return
	}
	nz := nonzeroBounds(pix, stride, w, h, nch)
	if nz.Empty() {
		return
	}
	// Only the pixels within the kernels' support of the nonzero ones can
	// change: blur that part of the plane, whose edges are either the
	// plane's or surrounded by zeros, so the result is the same. The margin
	// is the larger extent on both sides: the passes of a shifted box
	// kernel spread values on the side opposite to the shift before the
	// last pass moves them back.
	sx, sy := kx.support(), ky.support()
	work := image.Rect(nz.Min.X-sx, nz.Min.Y-sy, nz.Max.X+sx, nz.Max.Y+sy).Intersect(image.Rect(0, 0, w, h))
	o := out.Intersect(work)
	if o.Empty() {
		return
	}
	if kx.down > 1 || ky.down > 1 {
		s.blurDown(pix, stride, nch, ox, oy, nz, o, kx, ky)
		return
	}
	s.blurRect(pix, stride, nch, work, o, kx, ky)
}

// nonzeroBounds returns the bounds of the pixels of a plane with a nonzero
// channel.
func nonzeroBounds(pix []uint8, stride, w, h, nch int) image.Rectangle {
	x0, y0, x1, y1 := w, h, 0, 0
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w*nch]
		lo := firstNonzero(row)
		if lo < 0 {
			continue
		}
		hi := lastNonzero(row)
		x0, x1 = min(x0, lo/nch), max(x1, hi/nch+1)
		y0, y1 = min(y0, y), y+1
	}
	if x0 >= x1 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// firstNonzero returns the index of the first nonzero byte of b, or -1;
// it tests 8 bytes at a time.
func firstNonzero(b []uint8) int {
	i := 0
	for ; i+8 <= len(b); i += 8 {
		if binary.LittleEndian.Uint64(b[i:]) != 0 {
			break
		}
	}
	for ; i < len(b); i++ {
		if b[i] != 0 {
			return i
		}
	}
	return -1
}

// lastNonzero returns the index of the last nonzero byte of b, or -1.
func lastNonzero(b []uint8) int {
	i := len(b)
	for ; i >= 8; i -= 8 {
		if binary.LittleEndian.Uint64(b[i-8:]) != 0 {
			break
		}
	}
	for i--; i >= 0; i-- {
		if b[i] != 0 {
			return i
		}
	}
	return -1
}

// blurRect blurs the pixels o of the plane at full resolution; work
// contains o and every nonzero pixel within the kernels' support of it.
// The horizontal passes run over the rows that the vertical passes read,
// and the vertical passes over the columns of o: an output reads only
// values within the support, all computed from pixels of work.
func (s *blurScratch) blurRect(pix []uint8, stride, nch int, work, o image.Rectangle, kx, ky *gaussKernel) {
	ry0, ry1 := max(o.Min.Y-ky.support(), work.Min.Y), min(o.Max.Y+ky.support(), work.Max.Y)
	w, h := work.Dx(), ry1-ry0
	rowN := w * nch
	a := grow16(&s.a, rowN*h)
	s.par.run(s, h, rowGrain, func(_ *blurScratch, lo, hi int) {
		for y := lo; y < hi; y++ {
			dst := a[y*rowN : (y+1)*rowN]
			src := pix[(ry0+y)*stride+work.Min.X*nch:]
			for i := range dst {
				dst[i] = uint16(src[i]) << 8
			}
		}
	})
	s.passes(a, w, h, nch, kx, ky, (o.Min.X-work.Min.X)*nch, (o.Max.X-work.Min.X)*nch)
	n := o.Dx() * nch
	s.par.run(s, o.Dy(), rowGrain, func(_ *blurScratch, lo, hi int) {
		for y := o.Min.Y + lo; y < o.Min.Y+hi; y++ {
			src := a[(y-ry0)*rowN+(o.Min.X-work.Min.X)*nch:][:n]
			dst := pix[y*stride+o.Min.X*nch:][:n]
			for i, v := range src {
				dst[i] = uint8((uint32(v) + 128) >> 8)
			}
		}
	})
}

// rowGrain is the smallest number of rows a goroutine of a workPool takes.
const rowGrain = 16

// passes blurs the 8.8 plane a of w x h pixels (nch channels) in place: kx
// over every row, then ky over the values [cx0, cx1) of every row. Rows,
// then column strips, are split among the goroutines of s.par.
func (s *blurScratch) passes(a []uint16, w, h, nch int, kx, ky *gaussKernel, cx0, cx1 int) {
	rowN := w * nch
	if !kx.identity() {
		s.par.run(s, h, rowGrain, func(ws *blurScratch, lo, hi int) {
			for y := lo; y < hi; y++ {
				ws.line(kx, a[y*rowN:(y+1)*rowN], w, nch)
			}
		})
	}
	if ky.identity() {
		return
	}
	// Column strips of the plane: a -> b -> c -> a.
	sw := min(stripWidth, cx1-cx0)
	strips := (cx1 - cx0 + sw - 1) / sw
	s.par.run(s, strips, 1, func(ws *blurScratch, lo, hi int) {
		b, c := grow16(&ws.b, sw*h), grow16(&ws.c, sw*h)
		if cap(ws.acc) < sw {
			ws.acc, ws.acc64 = make([]uint32, sw), make([]uint64, sw)
		}
		ws.zeroRow = grow16(&ws.zeroRow, sw)
		clear(ws.zeroRow)
		for st := lo; st < hi; st++ {
			x0 := cx0 + st*sw
			n := min(sw, cx1-x0)
			sa := strip{a, x0, rowN}
			sb, sc := strip{b, 0, n}, strip{c, 0, n}
			switch {
			case ky.frac:
				ws.fracRows(sa, sb, n, h, ky)
				ws.fracRows(sb, sc, n, h, ky)
				ws.fracRows(sc, sa, n, h, ky)
			case ky.box:
				ws.boxRows(sa, sb, n, h, ky.l[0], ky.r[0])
				ws.boxRows(sb, sc, n, h, ky.l[1], ky.r[1])
				ws.boxRows(sc, sa, n, h, ky.l[2], ky.r[2])
			default:
				ws.tapsRows(ky, sa, sb, n, h)
				for y := 0; y < h; y++ {
					copy(sa.row(y, n), sb.row(y, n))
				}
			}
		}
	})
}

// lowKernel returns the kernel a downsampled blur applies to the blocks
// along one axis: the fractional box of a downsampled kernel, or the kernel
// itself for an axis that is not downsampled. l and r are its extents in
// blocks.
func (k *gaussKernel) lowKernel() (low gaussKernel, l, r int) {
	if k.down <= 1 {
		return *k, k.support(), k.support()
	}
	e := 3 * (k.fm + 1)
	return gaussKernel{frac: true, fm: k.fm, fa: k.fa, fb: k.fb, left: e, right: e}, e, e
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// blurDown blurs the pixels o of the plane with at least one downsampled
// kernel; nz bounds the nonzero pixels. With f the factor of an axis (1 if
// it is not downsampled), block I covers the device pixels [I*f, I*f+f)
// and holds their mean in 8.8 fixed point (rounded once); the blocks are
// blurred with the low-resolution kernels (lowKernel); and output pixel X
// samples the blurred blocks at the position of its center shifted by sh,
// u = (X - sh + 1/2)/f - 1/2: with 2(X-sh) + 1 - f = 2f*i + n, 0 <= n < 2f,
// it is blocks i and i+1 with weights (2f - n)/(2f) and n/(2f) (for f = 1,
// n = 0: the pixel itself). Both axes are interpolated with exact integer
// weights and the result rounded once to 8 bits. Every quantity is a
// function of device coordinates, so a pixel's value does not depend on
// the plane; it reads pixels within left ... right of it (newDownKernel).
// Block rows, the low-resolution passes and output rows are split among the
// goroutines of s.par.
func (s *blurScratch) blurDown(pix []uint8, stride, nch, ox, oy int, nz, o image.Rectangle, kx, ky *gaussKernel) {
	fx, fy := kx.factor(), ky.factor()
	lgx, lgy := bits.TrailingZeros(uint(fx)), bits.TrailingZeros(uint(fy))
	lowX, lxl, lxr := kx.lowKernel()
	lowY, lyl, lyr := ky.lowKernel()
	// The blurred blocks the outputs sample, [bx0, bx1) x [by0, by1), and
	// the blocks they read, which the low-resolution plane covers.
	bx0 := floorDiv(2*(o.Min.X+ox-kx.sh)+1-fx, 2*fx)
	bx1 := floorDiv(2*(o.Max.X-1+ox-kx.sh)+1-fx, 2*fx) + 2
	by0 := floorDiv(2*(o.Min.Y+oy-ky.sh)+1-fy, 2*fy)
	by1 := floorDiv(2*(o.Max.Y-1+oy-ky.sh)+1-fy, 2*fy) + 2
	lr := image.Rect(bx0-lxl, by0-lyl, bx1+lxr, by1+lyr)
	lw, lh := lr.Dx(), lr.Dy()
	rowL := lw * nch
	low := grow16(&s.low, rowL*lh)

	// Downsample the nonzero pixels whose blocks lie in the plane, one
	// block row at a time.
	shift, half := uint(lgx+lgy), uint32(1)<<uint(lgx+lgy)>>1
	ix0 := max(floorDiv(nz.Min.X+ox, fx), lr.Min.X)
	ix1 := min(floorDiv(nz.Max.X-1+ox, fx)+1, lr.Max.X)
	s.par.run(s, lh, rowGrain/2, func(ws *blurScratch, lo, hi int) {
		acc := grow32(&ws.dacc, rowL)
		for jr := lo; jr < hi; jr++ {
			out := low[jr*rowL:][:rowL]
			j := lr.Min.Y + jr
			y0, y1 := max(j*fy-oy, nz.Min.Y), min(j*fy+fy-oy, nz.Max.Y)
			if y0 >= y1 || ix0 >= ix1 {
				clear(out)
				continue
			}
			clear(acc)
			if nch == 4 {
				// Each pixel into its block; pixels of the blocks ix0 ...
				// ix1-1 only (the plane may extend beyond them).
				xs, xe := max(ix0*fx-ox, nz.Min.X), min(ix1*fx-ox, nz.Max.X)
				for y := y0; y < y1; y++ {
					row := pix[y*stride+4*xs : y*stride+4*xe]
					for x := 0; x < len(row)/4; x++ {
						bi := 4 * ((xs+x+ox)>>uint(lgx) - lr.Min.X)
						a := acc[bi : bi+4 : bi+4]
						p := row[4*x : 4*x+4 : 4*x+4]
						a[0] += uint32(p[0])
						a[1] += uint32(p[1])
						a[2] += uint32(p[2])
						a[3] += uint32(p[3])
					}
				}
				for i, v := range acc {
					out[i] = uint16((v<<8 + half) >> shift)
				}
				continue
			}
			for y := y0; y < y1; y++ {
				row := pix[y*stride:]
				for i := ix0; i < ix1; i++ {
					x0, x1 := max(i*fx-ox, nz.Min.X), min(i*fx+fx-ox, nz.Max.X)
					a := acc[(i-lr.Min.X)*nch:][:nch]
					src := row[x0*nch : x1*nch]
					if nch == 4 {
						var s0, s1, s2, s3 uint32
						for p := 0; p+3 < len(src); p += 4 {
							s0 += uint32(src[p])
							s1 += uint32(src[p+1])
							s2 += uint32(src[p+2])
							s3 += uint32(src[p+3])
						}
						a[0] += s0
						a[1] += s1
						a[2] += s2
						a[3] += s3
					} else {
						for p, v := range src {
							a[p%nch] += uint32(v)
						}
					}
				}
			}
			for i, v := range acc {
				out[i] = uint16((v<<8 + half) >> shift)
			}
		}
	})

	s.passes(low, lw, lh, nch, &lowX, &lowY, 0, rowL)

	// Upsample: the horizontal interpolation of a block row for every
	// output column, then the vertical one per output row.
	ow := o.Dx()
	ux := grow32(&s.ux, ow) // value index of block i in a low row
	wx := grow32(&s.wx, ow) // n
	for x := range ow {
		t := 2*(o.Min.X+x+ox-kx.sh) + 1 - fx
		i := floorDiv(t, 2*fx)
		ux[x] = uint32((i - lr.Min.X) * nch)
		wx[x] = uint32(t - 2*fx*i)
	}
	// Two values per uint64 (32-bit lanes): channels 0-1 and 2-3 of a
	// pixel, or two adjacent pixels of a single channel. A lane holds at
	// most 2f_x * 65280 < 2^23 after the horizontal step and less than
	// 2^31 after the vertical one, so lanes never carry into each other.
	nv := ow * nch
	np := (nv + 1) / 2
	f2 := uint32(2 * fx)
	interp := func(j int, dst []uint64) {
		row := low[(j-lr.Min.Y)*rowL:][:rowL]
		if nch == 4 {
			for x := range ow {
				i, n := int(ux[x]), wx[x]
				m := f2 - n
				p := row[i : i+8 : i+8]
				v0 := m*uint32(p[0]) + n*uint32(p[4])
				v1 := m*uint32(p[1]) + n*uint32(p[5])
				v2 := m*uint32(p[2]) + n*uint32(p[6])
				v3 := m*uint32(p[3]) + n*uint32(p[7])
				dst[2*x] = uint64(v0) | uint64(v1)<<32
				dst[2*x+1] = uint64(v2) | uint64(v3)<<32
			}
			return
		}
		clear(dst)
		for v := range nv {
			x, c := v/nch, v%nch
			i, n := int(ux[x])+c, wx[x]
			val := uint64((f2-n)*uint32(row[i]) + n*uint32(row[i+nch]))
			dst[v/2] |= val << (32 * uint(v%2))
		}
	}
	shOut := uint(lgx + 1 + lgy + 1 + 8)
	halfOut := uint64(1)<<(shOut-1) | uint64(1)<<(shOut-1+32)
	s.par.run(s, o.Dy(), rowGrain, func(ws *blurScratch, lo, hi int) {
		h0, h1 := grow64(&ws.h0, np), grow64(&ws.h1, np)
		j0 := lr.Min.Y - 2 // the block row in h0 (h1 holds j0+1)
		for y := o.Min.Y + lo; y < o.Min.Y+hi; y++ {
			t := 2*(y+oy-ky.sh) + 1 - fy
			j := floorDiv(t, 2*fy)
			ny := uint64(t - 2*fy*j)
			switch j {
			case j0:
			case j0 + 1:
				h0, h1 = h1, h0
				interp(j+1, h1)
			default:
				interp(j, h0)
				interp(j+1, h1)
			}
			j0 = j
			w0, w1 := uint64(2*fy)-ny, ny
			dst := pix[y*stride+o.Min.X*nch:][:nv]
			a, b := h0[:np], h1[:np]
			if nch == 4 {
				for x := range ow {
					lo := w0*a[2*x] + w1*b[2*x] + halfOut
					hi := w0*a[2*x+1] + w1*b[2*x+1] + halfOut
					v := uint32(uint8(lo>>shOut)) | uint32(uint8(lo>>(32+shOut)))<<8 |
						uint32(uint8(hi>>shOut))<<16 | uint32(uint8(hi>>(32+shOut)))<<24
					binary.LittleEndian.PutUint32(dst[4*x:], v)
				}
				continue
			}
			for v := range dst {
				p := w0*a[v/2] + w1*b[v/2] + halfOut
				dst[v] = uint8(p >> (shOut + 32*uint(v%2)))
			}
		}
		ws.h0, ws.h1 = h0, h1
	})
}

func grow64(s *[]uint64, n int) []uint64 {
	if cap(*s) < n {
		*s = make([]uint64, n)
	}
	return (*s)[:n]
}

func grow32(s *[]uint32, n int) []uint32 {
	if cap(*s) < n {
		*s = make([]uint32, n)
	}
	return (*s)[:n]
}
