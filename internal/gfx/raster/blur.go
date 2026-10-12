package raster

import "math"

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
	// Support: the kernel reads in[i-left] ... in[i+right].
	left, right int
}

// identity reports whether the kernel changes nothing.
func (k *gaussKernel) identity() bool {
	return !k.box && k.lo == 0 && len(k.w) == 1
}

// support returns how far the kernel reaches on either side.
func (k *gaussKernel) support() int { return max(k.left, k.right) }

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
	if s >= 2 {
		d := int(math.Floor(float64(s*(3*math.Sqrt(2*math.Pi)/4)) + 0.5))
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
		if t0 <= t1 {
			inv := 1 / s
			tap := func(t int) float64 {
				c := float64(t) + off
				return gaussI(float64((c+1)*inv)) - 2*gaussI(float64(c*inv)) + gaussI(float64((c-1)*inv))
			}
			var sum float64
			for t := t0; t <= t1; t++ {
				sum += tap(t)
			}
			if sum > 0 {
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
				return gaussKernel{lo: t0, w: w[start:], left: max(-t0, 0), right: max(t1, 0)}, w
			}
		}
	}
	// No blur: a shift by the offset truncated toward zero, which stays
	// within |off|.
	sh := int(off)
	w = append(w, weightOne)
	return gaussKernel{lo: -sh, w: w[start:], left: max(sh, 0), right: max(-sh, 0)}, w
}

// blurScratch holds the buffers of blurPlane, reused between calls.
type blurScratch struct {
	a       []uint16 // the plane, in 8.8 fixed point
	b, c    []uint16 // column strips of the vertical passes
	pad     []uint16 // a zero-padded line
	acc     []uint32
	acc64   []uint64
	zeroRow []uint16
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
	for i := 0; i < n*nch; i++ {
		var sum uint64
		j := (p+k.lo)*nch + i
		for _, w := range k.w {
			sum += uint64(w) * uint64(src[j])
			j += nch
		}
		dst[i] = uint16((sum + weightOne/2) >> 16)
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
	acc := s.acc64[:n]
	for y := 0; y < h; y++ {
		clear(acc)
		for t, w := range k.w {
			j := y + k.lo + t
			if j < 0 || j >= h {
				continue
			}
			wt := uint64(w)
			for x, v := range src.row(j, n) {
				acc[x] += wt * uint64(v)
			}
		}
		out := dst.row(y, n)
		for x, a := range acc {
			out[x] = uint16((a + weightOne/2) >> 16)
		}
	}
}

// blurPlane blurs a plane of w x h pixels with nch interleaved 8-bit
// channels (row y starts at pix[y*stride]) in place with kx horizontally
// and ky vertically. Pixels outside the plane count as 0.
func (s *blurScratch) blurPlane(pix []uint8, stride, w, h, nch int, kx, ky *gaussKernel) {
	if w <= 0 || h <= 0 || (kx.identity() && ky.identity()) {
		return
	}
	// Only the pixels within the kernels' support of the nonzero ones can
	// change: blur that part of the plane (its edges are either the plane's
	// or surrounded by zeros, so the result is the same).
	x0, y0, x1, y1 := w, h, 0, 0
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w*nch]
		lo, hi := -1, -1
		for i, v := range row {
			if v != 0 {
				lo = i
				break
			}
		}
		if lo < 0 {
			continue
		}
		for i := len(row) - 1; i >= 0; i-- {
			if row[i] != 0 {
				hi = i
				break
			}
		}
		x0, x1 = min(x0, lo/nch), max(x1, hi/nch+1)
		y0, y1 = min(y0, y), y+1
	}
	if x0 >= x1 {
		return
	}
	x0, x1 = max(x0-kx.support(), 0), min(x1+kx.support(), w)
	y0, y1 = max(y0-ky.support(), 0), min(y1+ky.support(), h)
	s.blurRect(pix[y0*stride+x0*nch:], stride, x1-x0, y1-y0, nch, kx, ky)
}

// blurRect is blurPlane without the bounds search.
func (s *blurScratch) blurRect(pix []uint8, stride, w, h, nch int, kx, ky *gaussKernel) {
	rowN := w * nch
	a := grow16(&s.a, rowN*h)
	for y := 0; y < h; y++ {
		dst := a[y*rowN : (y+1)*rowN]
		for i, v := range pix[y*stride : y*stride+rowN] {
			dst[i] = uint16(v) << 8
		}
	}
	if !kx.identity() {
		for y := 0; y < h; y++ {
			s.line(kx, a[y*rowN:(y+1)*rowN], w, nch)
		}
	}
	if !ky.identity() {
		// Column strips of the plane: a -> b -> c -> a.
		sw := min(stripWidth, rowN)
		b, c := grow16(&s.b, sw*h), grow16(&s.c, sw*h)
		if cap(s.acc) < sw {
			s.acc, s.acc64 = make([]uint32, sw), make([]uint64, sw)
		}
		s.zeroRow = grow16(&s.zeroRow, sw)
		clear(s.zeroRow)
		for x0 := 0; x0 < rowN; x0 += sw {
			n := min(sw, rowN-x0)
			sa := strip{a, x0, rowN}
			sb, sc := strip{b, 0, n}, strip{c, 0, n}
			if ky.box {
				s.boxRows(sa, sb, n, h, ky.l[0], ky.r[0])
				s.boxRows(sb, sc, n, h, ky.l[1], ky.r[1])
				s.boxRows(sc, sa, n, h, ky.l[2], ky.r[2])
			} else {
				s.tapsRows(ky, sa, sb, n, h)
				for y := 0; y < h; y++ {
					copy(sa.row(y, n), sb.row(y, n))
				}
			}
		}
	}
	for y := 0; y < h; y++ {
		src := a[y*rowN : (y+1)*rowN]
		for i, v := range src {
			pix[y*stride+i] = uint8((uint32(v) + 128) >> 8)
		}
	}
}
