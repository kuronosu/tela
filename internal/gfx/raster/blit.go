package raster

import (
	"fmt"
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// shader produces the premultiplied source colors of a paint, in device
// space. It is the extension point for gradients and images (T4b): a new
// paint is a new shader returned by Renderer.shaderFor; the blitter and the
// coverage producers stay unchanged.
//
// Shaders are created per command, never shared between goroutines, and may
// keep mutable scratch state.
type shader interface {
	// shadeSpan writes the premultiplied sRGB RGBA8 colors of the device
	// pixels (x, y), (x+1, y), ..., one per 4 bytes of dst (len(dst)/4
	// pixels), each sampled at its center (x+i+0.5, y+0.5). The result must
	// depend only on the pixel coordinates, never on the span boundaries,
	// so that region rendering stays bit-identical.
	shadeSpan(x, y int, dst []uint8)
	// opaque reports whether every color the shader produces has alpha 255,
	// which lets the blitter copy instead of blend at full coverage.
	opaque() bool
}

// solidShader paints one color. The blitter recognizes it and skips
// shadeSpan.
type solidShader struct {
	c [4]uint8 // premultiplied RGBA
}

func (s *solidShader) shadeSpan(x, y int, dst []uint8) {
	for i := 0; i+3 < len(dst); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = s.c[0], s.c[1], s.c[2], s.c[3]
	}
}

func (s *solidShader) opaque() bool { return s.c[3] == 255 }

// shaderFor returns the shader for paint under the device transform ctm.
// Solid colors reuse a shader held by the Renderer, so they do not allocate.
func (r *Renderer) shaderFor(p gfx.Paint, ctm gfx.Affine) shader {
	_ = ctm // gradients and images will map device pixels back with ctm's inverse
	switch p := p.(type) {
	case gfx.Color:
		r.solid = solidShader{premul8(p)}
		return &r.solid
	case nil:
		panic("raster: nil paint in a display list")
	default:
		panic(fmt.Sprintf("raster: %T paint is not implemented yet", p))
	}
}

// premul8 returns the premultiplied 8-bit color, each channel rounded to
// nearest from the exact float value. It equals gfx.Color.Premul8 computed
// without fused multiply-adds: Premul8 evaluates v*255 + 0.5, which the
// compiler may fuse on some architectures and then rounds differently.
func premul8(c gfx.Color) [4]uint8 {
	c = c.Clamp()
	a := float64(c.A)
	// The products of two float32 values are exact in float64; the product
	// by 255 is not, so it is rounded explicitly before adding 0.5.
	ch := func(v float64) uint8 {
		v = math.Floor(float64(v*255) + 0.5)
		return uint8(min(max(v, 0), 255))
	}
	return [4]uint8{ch(float64(c.R) * a), ch(float64(c.G) * a), ch(float64(c.B) * a), ch(a)}
}

// blitter composites shaded, coverage-weighted spans onto a surface with
// source-over. Coverage producers (rect, rrect and path fills) call run and
// mask with spans that they have already clipped to clip.
//
// T4b will add a clip mask (multiplied into the coverage here) and
// offscreen layer surfaces (another pix/stride/origin).
type blitter struct {
	pix    []uint8
	stride int
	origin image.Point     // device coordinates of pix[0]
	clip   image.Rectangle // device pixels that may be written

	sh      shader
	solid   *solidShader // sh, when it is a solid color
	scratch []uint8      // shadeSpan output for non-solid shaders

	// Cached lookup tables of blendLUT.
	lut      [4][256]uint8
	lutColor [4]uint8
	lutCov   uint8
	lutValid bool
}

func (b *blitter) offset(x, y int) int {
	return (y-b.origin.Y)*b.stride + (x-b.origin.X)*4
}

// setShader selects the paint for the following spans.
func (b *blitter) setShader(sh shader) {
	b.sh = sh
	b.solid, _ = sh.(*solidShader)
}

// div65025 returns round(v / 65025) for the blend numerators below. 65025
// is odd, so v/65025 is never exactly halfway between two integers.
func div65025(v uint32) uint32 { return (v + 32512) / 65025 }

// blendPixel composites the premultiplied color s at coverage cov (0-255)
// over d: d = s*cov/255 + d*(1 - s.a*cov/255²), each channel computed
// exactly over the common denominator 255² and rounded once.
func blendPixel(d []uint8, s0, s1, s2, s3 uint32, cov uint32) {
	k := 65025 - s3*cov
	c := cov * 255
	d[0] = uint8(div65025(s0*c + uint32(d[0])*k))
	d[1] = uint8(div65025(s1*c + uint32(d[1])*k))
	d[2] = uint8(div65025(s2*c + uint32(d[2])*k))
	d[3] = uint8(div65025(s3*c + uint32(d[3])*k))
}

// run composites n pixels of row y starting at x, all with coverage cov.
func (b *blitter) run(y, x, n int, cov uint8) {
	if cov == 0 || n <= 0 {
		return
	}
	row := b.pix[b.offset(x, y):]
	row = row[:4*n]
	if s := b.solid; s != nil {
		if cov == 255 && s.c[3] == 255 {
			fillColor(row, s.c)
			return
		}
		if n >= lutMinRun {
			lut := b.blendLUT(s.c, cov)
			for i := 0; i < len(row); i += 4 {
				d := row[i : i+4 : i+4]
				d[0], d[1], d[2], d[3] = lut[0][d[0]], lut[1][d[1]], lut[2][d[2]], lut[3][d[3]]
			}
			return
		}
		// Constant source and coverage: hoist the source terms.
		c := uint32(cov) * 255
		k := 65025 - uint32(s.c[3])*uint32(cov)
		t0, t1, t2, t3 := uint32(s.c[0])*c+32512, uint32(s.c[1])*c+32512, uint32(s.c[2])*c+32512, uint32(s.c[3])*c+32512
		for i := 0; i < len(row); i += 4 {
			d := row[i : i+4 : i+4]
			d[0] = uint8((t0 + uint32(d[0])*k) / 65025)
			d[1] = uint8((t1 + uint32(d[1])*k) / 65025)
			d[2] = uint8((t2 + uint32(d[2])*k) / 65025)
			d[3] = uint8((t3 + uint32(d[3])*k) / 65025)
		}
		return
	}
	src := b.shade(x, y, n)
	full := cov == 255 && b.sh.opaque()
	for i := 0; i < len(row); i += 4 {
		if full {
			copy(row[i:i+4], src[i:i+4])
			continue
		}
		blendPixel(row[i:i+4:i+4], uint32(src[i]), uint32(src[i+1]), uint32(src[i+2]), uint32(src[i+3]), uint32(cov))
	}
}

// fillColor sets every pixel of row to c, doubling the filled prefix with
// copy (a memmove) instead of storing pixel by pixel.
func fillColor(row []uint8, c [4]uint8) {
	if len(row) == 0 {
		return
	}
	row[0], row[1], row[2], row[3] = c[0], c[1], c[2], c[3]
	for n := 4; n < len(row); n *= 2 {
		copy(row[n:], row[:n])
	}
}

// lutMinRun is the run length from which a translucent solid run is blended
// through lookup tables: building them costs about 1024 divisions, which
// pays off when they are reused (the rows of a rectangle share them).
const lutMinRun = 64

// blendLUT returns, for each channel, the table d -> the result of
// blendPixel for source color c at coverage cov over a destination channel
// value d. The last tables are cached, so consecutive rows of one shape
// build them once.
func (b *blitter) blendLUT(c [4]uint8, cov uint8) *[4][256]uint8 {
	if b.lutValid && b.lutColor == c && b.lutCov == cov {
		return &b.lut
	}
	cc := uint32(cov) * 255
	k := 65025 - uint32(c[3])*uint32(cov)
	for ch := 0; ch < 4; ch++ {
		t := uint32(c[ch])*cc + 32512
		tab := &b.lut[ch]
		for d := range tab {
			tab[d] = uint8((t + uint32(d)*k) / 65025)
		}
	}
	b.lutValid, b.lutColor, b.lutCov = true, c, cov
	return &b.lut
}

// mask composites len(mask) pixels of row y starting at x, pixel i with
// coverage mask[i].
func (b *blitter) mask(y, x int, mask []uint8) {
	if len(mask) == 0 {
		return
	}
	row := b.pix[b.offset(x, y):]
	row = row[:4*len(mask)]
	if s := b.solid; s != nil {
		s0, s1, s2, s3 := uint32(s.c[0]), uint32(s.c[1]), uint32(s.c[2]), uint32(s.c[3])
		opaque := s3 == 255
		for i, m := range mask {
			switch {
			case m == 0:
				continue
			case m == 255 && opaque:
				d := row[4*i : 4*i+4 : 4*i+4]
				d[0], d[1], d[2], d[3] = s.c[0], s.c[1], s.c[2], s.c[3]
			default:
				blendPixel(row[4*i:4*i+4:4*i+4], s0, s1, s2, s3, uint32(m))
			}
		}
		return
	}
	src := b.shade(x, y, len(mask))
	opaque := b.sh.opaque()
	for i, m := range mask {
		switch {
		case m == 0:
			continue
		case m == 255 && opaque:
			copy(row[4*i:4*i+4], src[4*i:4*i+4])
		default:
			blendPixel(row[4*i:4*i+4:4*i+4], uint32(src[4*i]), uint32(src[4*i+1]), uint32(src[4*i+2]), uint32(src[4*i+3]), uint32(m))
		}
	}
}

// shade runs the shader for n pixels into the scratch buffer.
func (b *blitter) shade(x, y, n int) []uint8 {
	if cap(b.scratch) < 4*n {
		b.scratch = make([]uint8, 4*n)
	}
	s := b.scratch[:4*n]
	b.sh.shadeSpan(x, y, s)
	return s
}

// Clear sets every pixel of dst inside r to the color c (premultiplied),
// replacing what was there. An empty r means all of dst.
func Clear(dst *image.RGBA, r image.Rectangle, c gfx.Color) {
	if r.Empty() {
		r = dst.Rect
	}
	r = r.Intersect(dst.Rect)
	if r.Empty() {
		return
	}
	p := premul8(c)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i := dst.PixOffset(r.Min.X, y)
		row := dst.Pix[i : i+4*r.Dx()]
		for j := 0; j < len(row); j += 4 {
			row[j], row[j+1], row[j+2], row[j+3] = p[0], p[1], p[2], p[3]
		}
	}
}
