package raster

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
)

// Images.
//
// DrawImage maps Src (image pixels) onto Dst (local space) and the result
// through the current transform. The covered area is Dst (after clipping
// Src to the image and Dst with it), filled with the rect fast path when the
// transform keeps it axis-aligned and as a path otherwise. The image shader
// maps each device pixel center back to image coordinates (u, v) with an
// affine function computed once per command, from the absolute pixel
// coordinates (region-independent), and samples:
//
//   - SampleNearest: the pixel containing (u, v);
//   - SampleLinear: bilinear interpolation of the four pixels around
//     (u, v), with weights quantized to 1/256 and computed in integers on
//     the premultiplied values, rounded once.
//
// Sampling is clamped to the Src rectangle (its pixel centers, for
// bilinear), so pixels outside Src never bleed into the edges, like Skia's
// strict source-rectangle constraint. An identity mapping (Dst the size of
// Src at a whole-pixel offset) reproduces the source pixels exactly.

// imageShader samples a gfx.Image; see above.
type imageShader struct {
	img        *gfx.Image
	pix        []uint8
	stride     int
	nearest    bool
	isOpaque   bool
	ua, ub, uc float64 // u = ua*X + ub*Y + uc
	va, vb, vc float64 // v = va*X + vb*Y + vc
	// Bilinear: the clamp range of u-0.5 and v-0.5 (pixel centers).
	ulo, uhi, vlo, vhi float64
	// Nearest: the clamp range of the pixel indices.
	ilo, ihi, jlo, jhi int
	// Bilinear with an axis-aligned mapping (ub == 0 and va == 0: the
	// image is only scaled and translated): u depends on the column alone
	// and v on the row alone, so the taps of a column are computed once
	// (taps, from device column tapX0) and shared by every row. The
	// results are those of the general loop.
	axis  bool
	tapX0 int
	taps  []imageTap
	// hrow caches two source rows blended along x (see shadeAxis), for the
	// columns of taps; ok is false while a slot holds nothing valid.
	hrow [2]imageHRow
}

// imageHRow is a source row blended along x.
type imageHRow struct {
	iy int
	ok bool
	v  []uint64
}

// imageTap is the horizontal part of a bilinear sample: the byte offset of
// the left pixel, the offset to the right one (0 when its weight is 0, so
// it never leaves the image) and the weight of the right one in 1/256.
type imageTap struct {
	off, dx int32
	wx      uint32
}

func (s *imageShader) opaque() bool { return s.isOpaque }

func (s *imageShader) shadeSpan(x, y int, dst []uint8) {
	if s.axis && !s.nearest {
		s.shadeAxis(x, y, dst)
		return
	}
	s.shadeGeneral(x, y, dst)
}

// shadeGeneral samples any affine mapping, one pixel at a time.
func (s *imageShader) shadeGeneral(x, y int, dst []uint8) {
	Y := float64(y) + 0.5
	u0 := float64(s.ub*Y) + s.uc
	v0 := float64(s.vb*Y) + s.vc
	n := len(dst) / 4
	pix, stride := s.pix, s.stride
	if s.nearest {
		for i := 0; i < n; i++ {
			X := float64(x+i) + 0.5
			u := math.Floor(float64(s.ua*X) + u0)
			v := math.Floor(float64(s.va*X) + v0)
			ix := int(min(max(u, float64(s.ilo)), float64(s.ihi)))
			iy := int(min(max(v, float64(s.jlo)), float64(s.jhi)))
			o := iy*stride + 4*ix
			copy(dst[4*i:4*i+4], pix[o:o+4])
		}
		return
	}
	for i := 0; i < n; i++ {
		X := float64(x+i) + 0.5
		u := min(max(float64(s.ua*X)+u0-0.5, s.ulo), s.uhi)
		v := min(max(float64(s.va*X)+v0-0.5, s.vlo), s.vhi)
		// 8 fractional bits; the products by 256 are exact.
		fu, fv := int64(math.Floor(u*256)), int64(math.Floor(v*256))
		ix, iy := int(fu>>8), int(fv>>8)
		wx, wy := uint32(fu&255), uint32(fv&255)
		// The second tap has weight 0 when it would leave the image.
		dx, dy := 4, stride
		if wx == 0 {
			dx = 0
		}
		if wy == 0 {
			dy = 0
		}
		w00, w10 := (256-wx)*(256-wy), wx*(256-wy)
		w01, w11 := (256-wx)*wy, wx*wy
		o := iy*stride + 4*ix
		p00, p10 := pix[o:o+4:o+4], pix[o+dx:o+dx+4:o+dx+4]
		p01, p11 := pix[o+dy:o+dy+4:o+dy+4], pix[o+dy+dx:o+dy+dx+4:o+dy+dx+4]
		d := dst[4*i : 4*i+4 : 4*i+4]
		for c := 0; c < 4; c++ {
			sum := uint32(p00[c])*w00 + uint32(p10[c])*w10 + uint32(p01[c])*w01 + uint32(p11[c])*w11
			d[c] = uint8((sum + 1<<15) >> 16)
		}
	}
}

// shadeAxis is shadeGeneral for a bilinear, axis-aligned mapping: the
// expressions are the same, with va*X = 0 and ub*Y = 0, so every pixel has
// the same value, computed in a different order. A pixel's sum is
//
//	p00*(256-wx)*(256-wy) + p10*wx*(256-wy) + p01*(256-wx)*wy + p11*wx*wy
//	  = H(iy)*(256-wy) + H(iy+1)*wy,  H(r) = p(r)*(256-wx) + p(r+1)*wx,
//
// all in integers, so the blend of a source row along x (H, four 16-bit
// lanes per pixel, at most 255*256 each) is computed once per source row
// and shared by every output row that uses it, and a row is then a blend of
// two of them; the weights sum to 65536, so a channel's final sum stays
// below 2^24 and two 32-bit lanes per 64-bit operation never carry into
// each other.
func (s *imageShader) shadeAxis(x, y int, dst []uint8) {
	n := len(dst) / 4
	if n == 0 {
		return
	}
	if len(s.taps) == 0 || x < s.tapX0 || x+n > s.tapX0+len(s.taps) {
		s.buildTaps(x, n)
	}

	Y := float64(y) + 0.5
	v0 := float64(s.vb*Y) + s.vc
	v := min(max(float64(s.va*0.5)+v0-0.5, s.vlo), s.vhi)
	fv := int64(math.Floor(v * 256))
	iy, wy := int(fv>>8), uint64(fv&255)
	h0, h1 := s.hrows(iy, wy != 0)

	i0 := x - s.tapX0
	h0, h1 = h0[i0:i0+n], h1[i0:i0+n]
	w0, w1 := 256-wy, wy
	const round = 1<<15 | 1<<15<<32
	const lanes = 0x000000ff000000ff
	const even = 0x0000ffff0000ffff
	for i := range n {
		e0, e1 := h0[i], h1[i]
		// Red and blue, then green and alpha.
		rb := ((e0&even)*w0 + (e1&even)*w1 + round) >> 16 & lanes
		ga := ((e0>>16&even)*w0 + (e1>>16&even)*w1 + round) >> 16 & lanes
		binary.LittleEndian.PutUint32(dst[4*i:4*i+4], uint32(rb)|uint32(ga)<<8|uint32(rb>>32)<<16|uint32(ga>>32)<<24)
	}
}

// hrows returns the horizontally blended source rows iy and, when two is
// set, iy+1 (the same slice as iy otherwise), for the columns of the tap
// table, computing the ones the two slots do not hold.
func (s *imageShader) hrows(iy int, two bool) (h0, h1 []uint64) {
	a, b := -1, -1
	for k := range s.hrow {
		if r := &s.hrow[k]; r.ok {
			if r.iy == iy {
				a = k
			} else if two && r.iy == iy+1 {
				b = k
			}
		}
	}
	if a < 0 {
		a = 0
		if b == 0 {
			a = 1
		}
		s.blendRow(a, iy)
	}
	if !two {
		return s.hrow[a].v, s.hrow[a].v
	}
	if b < 0 {
		b = 1 - a
		s.blendRow(b, iy+1)
	}
	return s.hrow[a].v, s.hrow[b].v
}

// blendRow computes H of source row iy into slot k, for every column of the
// tap table.
func (s *imageShader) blendRow(k, iy int) {
	r := &s.hrow[k]
	r.v = slices.Grow(r.v[:0], len(s.taps))[:len(s.taps)]
	r.iy, r.ok = iy, true
	pix := s.pix
	row := iy * s.stride
	for i, t := range s.taps {
		a, b := row+int(t.off), row+int(t.off)+int(t.dx)
		r.v[i] = spread16(binary.LittleEndian.Uint32(pix[a:a+4]))*uint64(256-t.wx) +
			spread16(binary.LittleEndian.Uint32(pix[b:b+4]))*uint64(t.wx)
	}
}

// spread16 puts the four bytes of p in the four 16-bit lanes of a uint64.
func spread16(p uint32) uint64 {
	x := uint64(p)
	x = (x | x<<16) & 0x0000ffff0000ffff
	return (x | x<<8) & 0x00ff00ff00ff00ff
}

// buildTaps computes the column taps of the device columns x to x+n-1,
// together with those already in the table (it grows to the union).
func (s *imageShader) buildTaps(x, n int) {
	x0, x1 := x, x+n
	if len(s.taps) > 0 {
		x0, x1 = min(x0, s.tapX0), max(x1, s.tapX0+len(s.taps))
	}
	s.hrow[0].ok, s.hrow[1].ok = false, false // the columns change
	s.taps = s.taps[:0]
	if cap(s.taps) < x1-x0 {
		s.taps = make([]imageTap, 0, x1-x0)
	}
	// u = ua*X + ub*Y + uc with ub*Y = 0 (the general loop adds it as 0).
	u0 := float64(s.ub*0.5) + s.uc
	for i := x0; i < x1; i++ {
		X := float64(i) + 0.5
		u := min(max(float64(s.ua*X)+u0-0.5, s.ulo), s.uhi)
		fu := int64(math.Floor(u * 256))
		t := imageTap{off: int32(fu>>8) * 4, dx: 4, wx: uint32(fu & 255)}
		if t.wx == 0 {
			t.dx = 0
		}
		s.taps = append(s.taps, t)
	}
	s.tapX0 = x0
}

// drawImage draws a DrawImage command.
func (r *Renderer) drawImage(c gfx.DrawImage) {
	img := c.Image
	src := c.Src.Intersect(img.Bounds())
	if src.Empty() {
		return
	}
	inv, ok := invert(r.ctm)
	if !ok {
		return
	}
	// Src -> Dst scale factors, and the part of Dst that shows src.
	ku := float64(c.Src.Width()) / float64(c.Dst.Width())
	kv := float64(c.Src.Height()) / float64(c.Dst.Height())
	dst := gfx.Rect{
		X0: float32(float64(c.Dst.X0) + (float64(src.X0)-float64(c.Src.X0))/ku),
		Y0: float32(float64(c.Dst.Y0) + (float64(src.Y0)-float64(c.Src.Y0))/kv),
		X1: float32(float64(c.Dst.X0) + (float64(src.X1)-float64(c.Src.X0))/ku),
		Y1: float32(float64(c.Dst.Y0) + (float64(src.Y1)-float64(c.Src.Y0))/kv),
	}
	if c.Src == src {
		dst = c.Dst
	}
	if dst.Empty() || !(ku > 0 && kv > 0) || math.IsInf(ku, 0) || math.IsInf(kv, 0) {
		return
	}

	s := &r.img
	// The tables' storage lives as long as the Renderer.
	taps, hrow := s.taps[:0], s.hrow
	hrow[0].ok, hrow[1].ok = false, false
	*s = imageShader{img: img, pix: img.Pix(), stride: img.Stride(), nearest: c.Sampling == gfx.SampleNearest, taps: taps, hrow: hrow}
	s.isOpaque = r.opq.opaque(img)
	// u = Src.X0 + (x - Dst.X0) * ku with x the local point.
	s.ua, s.ub, s.uc = inv.compose(ku, 0, float64(c.Src.X0)-float64(float64(c.Dst.X0)*ku))
	s.va, s.vb, s.vc = inv.compose(0, kv, float64(c.Src.Y0)-float64(float64(c.Dst.Y0)*kv))
	w, h := float64(img.Width()), float64(img.Height())
	sx0, sy0, sx1, sy1 := float64(src.X0), float64(src.Y0), float64(src.X1), float64(src.Y1)
	centers := func(lo, hi, size float64) (float64, float64) {
		a, b := lo, hi-1
		if b < a { // narrower than a pixel: sample its middle
			a = (lo+hi)/2 - 0.5
			b = a
		}
		return min(max(a, 0), size-1), min(max(b, 0), size-1)
	}
	s.ulo, s.uhi = centers(sx0, sx1, w)
	s.vlo, s.vhi = centers(sy0, sy1, h)
	s.ilo, s.ihi = int(math.Floor(sx0)), min(int(math.Ceil(sx1))-1, img.Width()-1)
	s.jlo, s.jhi = int(math.Floor(sy0)), min(int(math.Ceil(sy1))-1, img.Height()-1)
	s.ilo, s.jlo = max(min(s.ilo, s.ihi), 0), max(min(s.jlo, s.jhi), 0)
	s.axis = s.ub == 0 && s.va == 0

	r.blit.setShader(s)
	r.coverRect(dst)
}

// imageOpacityCache remembers whether the last images drawn are opaque, so
// that the scan of their alpha channel is not repeated every frame.
type imageOpacityCache struct {
	ids      [16]uint64
	isOpaque [16]bool
	next     int
}

func (c *imageOpacityCache) opaque(img *gfx.Image) bool {
	id := img.ID()
	for i, v := range c.ids {
		if v == id {
			return c.isOpaque[i]
		}
	}
	o := true
	pix := img.Pix()
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 255 {
			o = false
			break
		}
	}
	c.ids[c.next], c.isOpaque[c.next] = id, o
	c.next = (c.next + 1) % len(c.ids)
	return o
}
