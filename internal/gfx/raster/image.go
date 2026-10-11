package raster

import (
	"math"

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
}

func (s *imageShader) opaque() bool { return s.isOpaque }

func (s *imageShader) shadeSpan(x, y int, dst []uint8) {
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
	*s = imageShader{img: img, pix: img.Pix(), stride: img.Stride(), nearest: c.Sampling == gfx.SampleNearest}
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
