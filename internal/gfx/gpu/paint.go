package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// paintEnc is a paint as an instance carries it: the geometry of
// raster/gradient.go, in float64 and absolute device coordinates until it
// is written relative to an instance's origin.
type paintEnc struct {
	kind   uint32 // Paint constants
	spread gfx.SpreadMode
	// first is the solid color, or the color before the first stop; last
	// the color from the last stop on.
	first, last [4]uint8
	row, rows   int     // the ramp rows
	lo, hi      float64 // the first and last stop offsets

	// With (X, Y) the pixel center: p = pa*X + pb*Y + pc, q likewise.
	pa, pb, pc float64
	qa, qb, qc float64
	start      float64 // conic: the start angle, in turns
	// focal: d = Center - Focal, dr, r0 = FocalRadius and the quadratic's a.
	dx, dy, dr, r0, a float64

	// A pattern: base is its image and img the mip level sampled (nil for
	// any other paint), whose coordinates are p and q; pattern holds
	// FlagPattern and the axes that repeat.
	img, base *gfx.Image
	pattern   uint32
}

// put writes the paint into in, whose quad is given, and returns its flags.
func (p *paintEnc) put(in *instance, quad image.Rectangle) uint32 {
	ox, oy := float64(quad.Min.X), float64(quad.Min.Y)
	if p.img != nil {
		// The image coordinates of the pixel center, for centers relative to
		// the origin.
		u, v := p.pc+float64(p.pa*ox)+float64(p.pb*oy), p.qc+float64(p.qa*ox)+float64(p.qb*oy)
		if p.pattern&FlagRepeatX != 0 {
			u = tileStart(u, p.pa, p.pb, quad, p.img.Width())
		}
		if p.pattern&FlagRepeatY != 0 {
			v = tileStart(v, p.qa, p.qb, quad, p.img.Height())
		}
		// What the rasterizer's sampling adds to them.
		half := -0.5
		if p.kind == PaintImageNearest {
			half = 1.0 / (1 << 20)
		}
		ua, ub, uc, ra, rb, rc := splitCoord(p.pa, p.pb, u+half, quad)
		va, vb, vc, sa, sb, sc := splitCoord(p.qa, p.qb, v+half, quad)
		in.floats(SlotPaint, ua, ub, uc, ra)
		in.floats(SlotPaint+1, va, vb, vc, sa)
		in.floats(SlotPaint+2, rb, rc, sb, sc)
		return p.kind<<FlagPaintShift | p.pattern
	}
	in[SlotHead][1] = uint32(p.row)
	in[SlotHead][2] = packRGBA8(p.first)
	in[SlotHead][3] = packRGBA8(p.last)
	if p.kind == PaintSolid {
		return PaintSolid << FlagPaintShift
	}
	// The functions of the pixel center, for centers relative to the
	// origin.
	in.floats(SlotPaint, p.pa, p.pb, p.pc+float64(p.pa*ox)+float64(p.pb*oy), p.start)
	in.floats(SlotPaint+1, p.qa, p.qb, p.qc+float64(p.qa*ox)+float64(p.qb*oy), 0)
	in.floats(SlotPaint+2, p.dx, p.dy, p.dr, p.r0)
	in.floats(SlotPaint+3, p.a, p.lo, p.hi, 0)
	return p.kind<<FlagPaintShift | uint32(p.spread)<<FlagSpreadShift | uint32(p.rows)<<FlagSegmentsShift
}

func solid(c gfx.Color) (paintEnc, bool) {
	p := paintEnc{kind: PaintSolid, first: premul8(c)}
	return p, p.first != [4]uint8{} // a transparent color paints nothing
}

// tileStart moves u, the coordinate of a quad's origin along an axis of n
// pixels that repeats, by whole tiles, so that the coordinates u + a*x + b*y
// of the quad's pixels are 0 or more, and no larger than that takes.
func tileStart(u, a, b float64, quad image.Rectangle, n int) float64 {
	lo := u + min(float64(a*float64(quad.Dx())), 0) + min(float64(b*float64(quad.Dy())), 0)
	return u - float64(math.Floor(lo/float64(n))*float64(n))
}

// splitCoord splits the coordinate a*x + b*y + c of the pixel centers (x, y)
// of a quad, relative to its origin, in two parts for the shader, which
// computes in float32: with a1, b1 and c1, multiples of a power of two, the
// products and their sum are not rounded, and a2, b2 and c2 are what is left
// of them, a few thousandths of a pixel or less. A pattern sampled by its
// nearest pixel then takes the rasterizer's pixel but within a ten millionth
// of a boundary, where one sum in float32 is wrong within a ten thousandth: a
// pixel of some turned pattern of every window.
func splitCoord(a, b, c float64, quad image.Rectangle) (a1, b1, c1, a2, b2, c2 float64) {
	_, exp := math.Frexp(math.Abs(float64(a*float64(quad.Dx()))) + math.Abs(float64(b*float64(quad.Dy()))) + math.Abs(c))
	// The centers are multiples of 1/2, so the sum is one of grid/2 = 2^(exp-23),
	// and less than 2^(exp+1): 24 bits.
	grid := math.Ldexp(1, exp-22)
	part := func(v float64) (float64, float64) {
		w := float64(math.Round(v/grid) * grid) // exact but for the rounding asked for
		return w, v - w
	}
	a1, a2 = part(a)
	b1, b2 = part(b)
	c1, c2 = part(c)
	return
}

// paint returns the paint p under the transform m, with the cases of
// raster's shaderFor: gradients that paint a single color are solid. ok is
// false when nothing is painted.
func (e *Encoder) paint(p gfx.Paint, m *gfx.Affine) (paintEnc, bool) {
	var stops []gfx.GradientStop
	var interp gfx.ColorSpace
	switch p := p.(type) {
	case gfx.Color:
		return solid(p)
	case gfx.LinearGradient:
		stops, interp = p.Stops, p.Interp
	case gfx.RadialGradient:
		stops, interp = p.Stops, p.Interp
	case gfx.ConicGradient:
		stops, interp = p.Stops, p.Interp
	case gfx.FocalGradient:
		stops, interp = p.Stops, p.Interp
	case gfx.ImagePattern:
		return patternPaint(p, m)
	default:
		return paintEnc{}, false // nil: the Builder stores none
	}
	switch len(stops) {
	case 0:
		return paintEnc{}, false
	case 1:
		return solid(stops[0].Color)
	}
	last := stops[len(stops)-1].Color
	inv, ok := invert(m)
	if !ok {
		return solid(last)
	}
	var out paintEnc
	switch p := p.(type) {
	case gfx.LinearGradient:
		sx, sy := float64(p.Start.X), float64(p.Start.Y)
		dx, dy := float64(p.End.X)-sx, float64(p.End.Y)-sy
		l2 := float64(dx*dx) + float64(dy*dy)
		if !(l2 > 0) || math.IsInf(l2, 0) { // Start == End
			return solid(last)
		}
		kx, ky := dx/l2, dy/l2
		out.kind, out.spread = PaintLinear, p.Spread
		out.pa, out.pb, out.pc = inv.compose(kx, ky, -(float64(kx*sx) + float64(ky*sy)))
	case gfx.RadialGradient:
		rx, ry := float64(p.Radius.X), float64(p.Radius.Y)
		if !(rx > 0 && ry > 0) {
			return solid(last)
		}
		out.kind, out.spread = PaintRadial, p.Spread
		out.pa, out.pb, out.pc = inv.compose(1/rx, 0, -float64(p.Center.X)/rx)
		out.qa, out.qb, out.qc = inv.compose(0, 1/ry, -float64(p.Center.Y)/ry)
	case gfx.ConicGradient:
		out.kind = PaintConic
		out.pa, out.pb, out.pc = inv.compose(1, 0, -float64(p.Center.X))
		out.qa, out.qb, out.qc = inv.compose(0, 1, -float64(p.Center.Y))
		out.start = float64(p.StartAngle) * (1 / (2 * math.Pi))
	case gfx.FocalGradient:
		fx, fy := float64(p.Focal.X), float64(p.Focal.Y)
		out.kind, out.spread = PaintFocal, p.Spread
		out.pa, out.pb, out.pc = inv.compose(1, 0, -fx)
		out.qa, out.qb, out.qc = inv.compose(0, 1, -fy)
		// Differences and squares of float32 values are exact in float64.
		out.r0 = max(float64(p.FocalRadius), 0)
		out.dx, out.dy = float64(p.Center.X)-fx, float64(p.Center.Y)-fy
		out.dr = max(float64(p.Radius), 0) - out.r0
		out.a = float64(out.dx*out.dx) + float64(out.dy*out.dy) - float64(out.dr*out.dr)
	}
	rp := e.ramp(stops, interp)
	if rp.rows > maxSegments {
		e.frame.Unsupported = appendOnce(e.frame.Unsupported, "gradient (more than 65535 segments)")
		return paintEnc{}, false
	}
	out.first, out.last = rp.ramp.First, rp.ramp.Last
	out.lo, out.hi = rp.ramp.Lo, rp.ramp.Hi
	out.row, out.rows = rp.row, rp.rows
	return out, true
}

// patternPaint returns the pattern p under the transform m, with the mip
// level and the mapping of the rasterizer (raster.MapPattern). ok is false
// when nothing is painted; a mapping near the end of float32's range, whose
// pixels have no meaning, paints nothing either.
func patternPaint(p gfx.ImagePattern, m *gfx.Affine) (out paintEnc, ok bool) {
	mp, ok := raster.MapPattern(p, *m)
	if !ok {
		return out, false
	}
	const limit = 1e30
	for _, v := range [6]float64{mp.UA, mp.UB, mp.UC, mp.VA, mp.VB, mp.VC} {
		if !(math.Abs(v) < limit) {
			return out, false
		}
	}
	out.kind, out.img, out.base, out.pattern = PaintImageLinear, mp.Image, p.Image, FlagPattern
	if p.Sampling == gfx.SampleNearest {
		out.kind = PaintImageNearest
	}
	if p.Repeat.X() {
		out.pattern |= FlagRepeatX
	}
	if p.Repeat.Y() {
		out.pattern |= FlagRepeatY
	}
	out.pa, out.pb, out.pc = mp.UA, mp.UB, mp.UC
	out.qa, out.qb, out.qc = mp.VA, mp.VB, mp.VC
	return out, true
}

// usePaint makes the image of a pattern the texture of the instances
// emitted until e.image is cleared. It reports false when the image does not
// fit a texture of the device.
func (e *Encoder) usePaint(p *paintEnc) bool {
	if p.img == nil {
		return true
	}
	if lim := e.res.limits.MaxTextureSize; p.img.Width() > lim || p.img.Height() > lim {
		e.unsupported("image pattern (an image larger than the device's textures)")
		return false
	}
	e.image = e.useImage(p.img, p.base)
	return true
}

// bakedRamp is a gradient's ramp, baked by the rasterizer's code, and
// where the frame being encoded holds it.
type bakedRamp struct {
	stops  []gfx.GradientStop // the key (a private copy)
	interp gfx.ColorSpace
	ramp   raster.Ramp
	rows   int

	serial uint64 // the frame that holds it from row on
	row    int
	used   uint64 // Encoder.tick of its last use
}

// maxRamps bounds the ramps an Encoder keeps between frames.
const maxRamps = 256

// ramp returns the ramp of the stops, placed in the frame's ramp rows. A
// gradient used several times in a frame has one copy, and one drawn frame
// after frame is baked once.
func (e *Encoder) ramp(stops []gfx.GradientStop, interp gfx.ColorSpace) *bakedRamp {
	e.tick++
	var rp *bakedRamp
	// perf: a linear search; frames have few distinct gradients.
search:
	for _, c := range e.ramps {
		if c.interp != interp || len(c.stops) != len(stops) {
			continue
		}
		for i := range stops {
			if c.stops[i] != stops[i] {
				continue search
			}
		}
		rp = c
		break
	}
	if rp == nil {
		rp = &bakedRamp{stops: append([]gfx.GradientStop(nil), stops...), interp: interp, ramp: raster.GradientRamp(stops, interp)}
		rp.rows = len(rp.ramp.Segments)
		if len(e.ramps) < maxRamps {
			e.ramps = append(e.ramps, rp)
		} else {
			// Replace the least recently used ramp that this frame does
			// not hold; if it holds them all, this one is not kept.
			old := -1
			for i, c := range e.ramps {
				if c.serial != e.serial && (old < 0 || c.used < e.ramps[old].used) {
					old = i
				}
			}
			if old >= 0 {
				e.ramps[old] = rp
			}
		}
	}
	rp.used = e.tick
	if rp.serial != e.serial {
		f := &e.frame
		rp.serial, rp.row = e.serial, len(f.RampSegments)/RampSegmentSize
		for i := range rp.ramp.Segments {
			s := &rp.ramp.Segments[i]
			for _, c := range s.Colors {
				f.Ramps = append(f.Ramps, c[0], c[1], c[2], c[3])
			}
			f.RampSegments = append(f.RampSegments, float32(s.Offset), float32(s.Scale), 0, 0)
		}
	}
	return rp
}
