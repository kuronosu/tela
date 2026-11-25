package raster

import (
	"encoding/binary"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Image patterns.
//
// A pattern (gfx.ImagePattern) is a tiled image: the shader maps each
// device pixel center back to the pattern's space, in image pixels, with
// an affine function computed once per command from the absolute pixel
// coordinates (region-independent), and samples the image there, nearest or
// bilinear with the weights of image.go (1/256, integers on the
// premultiplied values, rounded once). Pixel indices beyond the image wrap
// around along an axis that repeats and are transparent along one that does
// not, for each of the four taps of a bilinear sample: the edge of an image
// that does not repeat fades over one image pixel, as with Skia's kDecal.
//
// A pattern drawn smaller than its image with bilinear sampling reads the
// mip level (gfx.Image.Level) whose size is the smallest that is not smaller
// than the size drawn, the larger scale of the two axes of the pattern
// deciding: the choice the callers of DrawImage make (ADR-079), made here
// because a paint does not know the transform it is drawn under.
//
// A mapping by whole pixels (to a thousandth of a pixel over the image, or
// over the tiles a surface can show: unitMapping) copies the image's pixels.

// maxMipLevel bounds the mip levels chosen: more than an image can have.
const maxMipLevel = 32

// patternShader samples a gfx.ImagePattern; see above.
type patternShader struct {
	pix        []uint8 // of the level sampled
	w, h       int     // its size
	repX, repY bool
	nearest    bool
	isOpaque   bool
	// unit: ua = vb = 1, ub = va = 0 and uc, vc are whole numbers.
	unit       bool
	ua, ub, uc float64 // u = ua*X + ub*Y + uc, in pixels of the level
	va, vb, vc float64 // v = va*X + vb*Y + vc
}

func (s *patternShader) opaque() bool { return s.isOpaque }

// coordMax bounds sampled coordinates, so that they convert to integers
// alike on every architecture: beyond it nothing sensible is left of the
// mapping's precision.
const coordMax = 1 << 40

// floorCoord returns floor(v), clamped (a NaN gives the lower bound). It is
// math.Floor for values that fit an int64, in a form that is inlined.
func floorCoord(v float64) int64 {
	if !(v > -coordMax) {
		v = -coordMax
	} else if v > coordMax {
		v = coordMax
	}
	i := int64(v) // toward zero
	if float64(i) > v {
		i--
	}
	return i
}

// tile returns the pixel of an axis of n pixels that coordinate i reads:
// wrapped when the axis repeats, else i, or -1 outside.
func tile(i int64, n int, repeat bool) int {
	if uint64(i) < uint64(n) {
		return int(i)
	}
	if !repeat {
		return -1
	}
	m := i % int64(n)
	if m < 0 {
		m += int64(n)
	}
	return int(m)
}

// texel returns the pixel (x, y) of the level, transparent for the -1 of
// tile.
func (s *patternShader) texel(x, y int) uint32 {
	if x|y < 0 {
		return 0
	}
	o := 4 * (y*s.w + x)
	return binary.LittleEndian.Uint32(s.pix[o : o+4])
}

func (s *patternShader) shadeSpan(x, y int, dst []uint8) {
	if s.unit {
		s.shadeUnit(x, y, dst)
		return
	}
	Y := float64(y) + 0.5
	u0 := float64(s.ub*Y) + s.uc
	v0 := float64(s.vb*Y) + s.vc
	n := len(dst) / 4
	// Nearest sampling is bilinear at the pixel's center: with no weight on
	// the other taps. A center exactly on the boundary of two pixels of the
	// image (pixel art at 4.5 times its size) takes the one after it,
	// whichever side the rounding of the coordinate leaves it on.
	half, weights := 0.5, int64(255)
	if s.nearest {
		half, weights = -1.0/(1<<20), 0
	}
	const round = 1<<15 | 1<<15<<32
	const lanes = 0x000000ff000000ff
	const even = 0x0000ffff0000ffff
	pix, stride := s.pix, 4*s.w
	w, h := int64(s.w), int64(s.h)
	var tx, ty int64
	for i := range n {
		X := float64(x+i) + 0.5
		// 8 fractional bits; the products by 256 are exact.
		fu := floorCoord((float64(s.ua*X) + u0 - half) * 256)
		fv := floorCoord((float64(s.va*X) + v0 - half) * 256)
		wx, wy := uint64(fu&weights), uint64(fv&weights)
		// The pixel's place in its tile, along an axis that repeats:
		// neighbors mostly lie in one tile, whose origin is (tx, ty).
		ix, iy := fu>>8-tx, fv>>8-ty
		if s.repX && uint64(ix) >= uint64(w) {
			tx += ix - int64(tile(ix, s.w, true))
			ix = fu>>8 - tx
		}
		if s.repY && uint64(iy) >= uint64(h) {
			ty += iy - int64(tile(iy, s.h, true))
			iy = fv>>8 - ty
		}
		var p00, p10, p01, p11 uint32
		if uint64(ix) < uint64(w-1) && uint64(iy) < uint64(h-1) {
			o := int(iy)*stride + 4*int(ix)
			r0, r1 := pix[o:o+8:o+8], pix[o+stride:o+stride+8:o+stride+8]
			p00, p10 = binary.LittleEndian.Uint32(r0), binary.LittleEndian.Uint32(r0[4:])
			p01, p11 = binary.LittleEndian.Uint32(r1), binary.LittleEndian.Uint32(r1[4:])
		} else {
			x0, y0 := tile(ix, s.w, s.repX), tile(iy, s.h, s.repY)
			x1, y1 := tile(ix+1, s.w, s.repX), tile(iy+1, s.h, s.repY)
			p00, p10, p01, p11 = s.texel(x0, y0), s.texel(x1, y0), s.texel(x0, y1), s.texel(x1, y1)
		}
		// As in shadeAxis: rows blended along x in four 16-bit lanes, then
		// along y in pairs of 32-bit lanes.
		e0 := spread16(p00)*(256-wx) + spread16(p10)*wx
		e1 := spread16(p01)*(256-wx) + spread16(p11)*wx
		rb := ((e0&even)*(256-wy) + (e1&even)*wy + round) >> 16 & lanes
		ga := ((e0>>16&even)*(256-wy) + (e1>>16&even)*wy + round) >> 16 & lanes
		binary.LittleEndian.PutUint32(dst[4*i:4*i+4], uint32(rb)|uint32(ga)<<8|uint32(rb>>32)<<16|uint32(ga>>32)<<24)
	}
}

// shadeUnit is shadeSpan for a whole-pixel translation: it copies runs of
// the image's row.
func (s *patternShader) shadeUnit(x, y int, dst []uint8) {
	iy := tile(int64(y)+floorCoord(s.vc), s.h, s.repY)
	u := int64(x) + floorCoord(s.uc)
	if iy < 0 {
		clear(dst)
		return
	}
	row := s.pix[4*iy*s.w : 4*(iy+1)*s.w]
	for len(dst) > 0 {
		k := tile(u, s.w, s.repX)
		if k >= 0 {
			n := copy(dst, row[4*k:])
			dst, u = dst[n:], u+int64(n/4)
			continue
		}
		// Outside an axis that does not repeat: transparent up to the
		// image, or to the end.
		n := len(dst)
		if u < 0 && -u < int64(n/4) {
			n = 4 * int(-u)
		}
		clear(dst[:n])
		dst, u = dst[n:], u+int64(n/4)
	}
}

// patternFor returns the shader of an image pattern under the current
// transform.
func (r *Renderer) patternFor(p gfx.ImagePattern) shader {
	img := p.Image
	inv, ok := invert(r.ctm)
	pinv, pok := invert(p.Transform)
	if !ok || !pok || img.Width() == 0 || img.Height() == 0 {
		return r.solidFor(gfx.Transparent)
	}
	w, h := img.Width(), img.Height()
	isOpaque := p.Repeat == gfx.RepeatBoth && r.opq.opaque(img)
	nearest := p.Sampling == gfx.SampleNearest
	if !nearest {
		img = img.Level(patternLevel(&r.ctm, &p.Transform))
	}
	r.pat = patternShader{pix: img.Pix(), w: img.Width(), h: img.Height(),
		repX: p.Repeat.X(), repY: p.Repeat.Y(), nearest: nearest, isOpaque: isOpaque}
	s := &r.pat
	// u = (pinv.a*x + pinv.c*y + pinv.e) * kx, with (x, y) the local point
	// and kx the size of the level relative to the image's.
	kx, ky := float64(s.w)/float64(w), float64(s.h)/float64(h)
	s.ua, s.ub, s.uc = inv.compose(float64(pinv.a*kx), float64(pinv.c*kx), float64(pinv.e*kx))
	s.va, s.vb, s.vc = inv.compose(float64(pinv.b*ky), float64(pinv.d*ky), float64(pinv.f*ky))
	if s.ub == 0 && s.va == 0 {
		s.ua, s.uc = unitMapping(s.ua, s.uc, 0, unitSpan(s.w, s.repX))
		s.vb, s.vc = unitMapping(s.vb, s.vc, 0, unitSpan(s.h, s.repY))
		s.unit = s.ua == 1 && s.vb == 1 && s.uc == math.Trunc(s.uc) && s.vc == math.Trunc(s.vc) &&
			math.Abs(s.uc) < coordMax && math.Abs(s.vc) < coordMax
	}
	return s
}

// unitSpan returns the source interval over which a pattern's mapping must
// be whole pixels to be taken for one: the image's n pixels, or when it
// repeats, many tiles, over which a scale that is not quite 1 adds up.
func unitSpan(n int, repeat bool) float64 {
	if repeat {
		n = max(n, 1024)
	}
	return float64(n)
}

// patternLevel returns the mip level that a pattern with transform p drawn
// under m is sampled from: the last whose pixels are not smaller than a
// device pixel along the axis of the pattern that is drawn larger.
func patternLevel(m, p *gfx.Affine) int {
	// The images of the pattern's unit vectors in device space. Sums of two
	// products of float32 values: exact products, rounded once.
	col := func(x, y float32) float64 {
		dx := float64(m[0])*float64(x) + float64(m[2])*float64(y)
		dy := float64(m[1])*float64(x) + float64(m[3])*float64(y)
		return float64(dx*dx) + float64(dy*dy)
	}
	k2 := max(col(p[0], p[1]), col(p[2], p[3])) // the scale, squared
	n := 0
	for n < maxMipLevel && k2 > 0 && k2*4 <= 1 { // k2*4 is exact
		k2 *= 4
		n++
	}
	return n
}
