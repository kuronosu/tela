package raster

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// What the GPU renderer (internal/gfx/gpu, ADR-282) shares with the
// rasterizer: the pieces whose values must be the same on both backends
// and that are not worth computing twice. The geometry of rectangle fills,
// gradients and shadows is evaluated per pixel by the GPU's own shaders;
// the colors of a gradient, the mip level and the mapping of an image
// pattern, the shape of a shadow, the coverage of paths, strokes, clips and
// glyphs (Masker) and the kernels of blurs (BlurKernel) come from here.

// RampSize is the number of colors in the lookup table of one segment of a
// gradient (see Ramp).
const RampSize = rampSize

// ShadowCutoff is where the Gaussian of an analytic shadow is cut, in
// standard deviations: beyond it a blurred edge is exactly 0 or 1.
const ShadowCutoff = shadowK

// MinSigma is the smallest device standard deviation that blurs at all: a
// shadow below it is the shape's coverage, unblurred.
const MinSigma = sigmaMin

// Ramp is the color lookup of a gradient's stops, the one the rasterizer
// paints with (see the comment at the top of gradient.go): one table of
// RampSize premultiplied RGBA8 colors per segment, a segment being two
// consecutive stops at different offsets. It is immutable.
//
// The color at the gradient parameter t, already folded by the spread
// mode, is: First when !(t >= Lo); Last when t >= Hi; otherwise the
// entry int(t*Scale + Bias), clamped to the table, of the last segment
// whose Offset is at most t.
type Ramp struct {
	// Lo and Hi are the offsets of the first and the last stop.
	Lo, Hi float64
	// First and Last are the colors before Lo and from Hi on. They are not
	// always the ends of the tables: stops at equal offsets (hard stops)
	// make no segment.
	First, Last [4]uint8
	// Segments are in increasing order of Offset; there are none when
	// every stop has the same offset.
	Segments []RampSegment
}

// RampSegment is the part of a Ramp between two stops.
type RampSegment struct {
	// Offset is the offset of the segment's first stop.
	Offset float64
	// Scale and Bias map t to an entry: int(t*Scale + Bias), which is
	// round((t - Offset) * (RampSize-1) / (length of the segment)).
	Scale, Bias float64
	// Colors holds the premultiplied RGBA8 colors; the first and the last
	// are the stop colors exactly.
	Colors *[RampSize][4]uint8
}

// GradientRamp bakes the ramp of normalized stops (gfx.NormalizeStops; at
// least two, as the Builder stores them) interpolated in the color space
// interp. The result does not share memory with stops.
func GradientRamp(stops []gfx.GradientStop, interp gfx.ColorSpace) Ramp {
	rp := newRamp(stops, interp)
	out := Ramp{Lo: rp.lo, Hi: rp.hi, First: rp.first, Last: rp.last, Segments: make([]RampSegment, len(rp.segs))}
	for i := range rp.segs {
		s := &rp.segs[i]
		out.Segments[i] = RampSegment{Offset: s.o0, Scale: s.scale, Bias: s.bias, Colors: &s.lut}
	}
	return out
}

// Pattern is how an image pattern is sampled (see the comment at the top of
// pattern.go).
type Pattern struct {
	// Image is the mip level of the pattern's image that is sampled.
	Image *gfx.Image
	// The pixel center (X, Y) reads the pattern at u = UA*X + UB*Y + UC,
	// v = VA*X + VB*Y + VC, in pixels of Image.
	UA, UB, UC float64
	VA, VB, VC float64
}

// MapPattern returns how the rasterizer samples p under the transform m of
// the device: patternFor, without a look at the pixels (TestMapPattern
// checks that the two agree). ok is false when the pattern shows nothing (a
// transform without an inverse).
func MapPattern(p gfx.ImagePattern, m gfx.Affine) (s Pattern, ok bool) {
	img := p.Image
	inv, ok := invert(m)
	pinv, pok := invert(p.Transform)
	if !ok || !pok || img.Width() == 0 || img.Height() == 0 {
		return Pattern{}, false
	}
	if s.Image = img; p.Sampling != gfx.SampleNearest {
		s.Image = img.Level(patternLevel(&m, &p.Transform))
	}
	w, h := s.Image.Width(), s.Image.Height()
	kx, ky := float64(w)/float64(img.Width()), float64(h)/float64(img.Height())
	s.UA, s.UB, s.UC = inv.compose(float64(pinv.a*kx), float64(pinv.c*kx), float64(pinv.e*kx))
	s.VA, s.VB, s.VC = inv.compose(float64(pinv.b*ky), float64(pinv.d*ky), float64(pinv.f*ky))
	if s.UB == 0 && s.VA == 0 {
		s.UA, s.UC = unitMapping(s.UA, s.UC, 0, unitSpan(w, p.Repeat.X()))
		s.VB, s.VC = unitMapping(s.VB, s.VC, 0, unitSpan(h, p.Repeat.Y()))
	}
	return s, true
}

// ShadowShape returns the shape a DrawShadow blurs, in the command's local
// space: the box translated by the offset and grown by the spread (outer)
// or shrunk by it (inset), with the CSS spread rule for the radii (see
// shadow.go). ok is false when the shape is empty: an outer shadow then
// paints nothing, and an inset one fills the whole box.
func ShadowShape(s gfx.DrawShadow) (shape gfx.RRect, ok bool) { return shadowShape(s) }

// A Masker computes for another backend the 8-bit coverage the rasterizer
// draws with: of filled paths, of strokes, of the shapes of clips, and of
// the glyphs of a run, with the code and the caches a Render uses, so the
// values are the Renderer's own, bit for bit. It owns the scratch memory
// of that code and a cache of stroke outlines (strokecache.go). The zero
// value is ready to use; it is not safe for concurrent use.
//
// Coverage is written into a buffer the caller owns: dst holds the device
// pixels of rect row by row, rect.Dx() bytes per row, and must be zeroed
// (only the pixels a shape covers are written). As for a Render, the
// values depend on the geometry and the absolute pixel coordinates only,
// not on rect: any rectangle gives the same pixels of the same picture.
// rect must lie within ±2^20 device pixels.
type Masker struct {
	r   Renderer
	run glyphRun
	c   gfx.DrawGlyphRun
	ctm gfx.Affine // of the run
}

// begin points the Renderer's coverage producers at dst under the
// transform m. It reports false when there is nothing to write.
func (k *Masker) begin(dst []uint8, rect image.Rectangle, m gfx.Affine) bool {
	rect = rect.Intersect(image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit))
	if rect.Empty() || len(dst) < rect.Dx()*rect.Dy() {
		return false
	}
	r := &k.r
	if r.emitFn == nil {
		r.emitFn = r.emit
		r.visitFn = r.visit
	}
	r.ctm = m
	b := &r.blit
	b.toMask, b.a8, b.a8Stride, b.a8Origin = true, dst, rect.Dx(), rect.Min
	b.clip, b.cm = rect, clipMask{}
	return true
}

func (k *Masker) end() {
	b := &k.r.blit
	b.toMask, b.a8 = false, nil
}

// Fill writes the coverage of FillPath: p filled with rule under the
// transform m (local to device), with the Renderer's flattening tolerance
// and fixed-point grid.
func (k *Masker) Fill(dst []uint8, rect image.Rectangle, m gfx.Affine, p gfx.Path, rule gfx.FillRule) {
	if k.begin(dst, rect, m) {
		k.r.fillPath(p, rule)
		k.end()
	}
}

// StrokeOutline returns the outline whose NonZero fill under m is the
// coverage of StrokePath: gfx.Path.Stroke at the tolerance the Renderer
// uses for m, from the Masker's cache of outlines. The outline depends on
// the linear part of m only. It is shared with the cache and must not be
// modified; it is empty when nothing is stroked.
func (k *Masker) StrokeOutline(p gfx.Path, style gfx.StrokeStyle, m gfx.Affine) gfx.Path {
	s := m.MaxScale()
	if !(s > 0) {
		return gfx.Path{}
	}
	return k.r.strokes.stroke(p, style, FlattenTolerance/s)
}

// Rect writes the coverage of a rectangle clip (PushClipRect, and the
// area of DrawImage and FillRect): exact on the fixed-point grid when m
// keeps it axis-aligned, a filled parallelogram otherwise.
func (k *Masker) Rect(dst []uint8, rect image.Rectangle, m gfx.Affine, r gfx.Rect) {
	if k.begin(dst, rect, m) {
		k.r.coverRect(r)
		k.end()
	}
}

// RRect writes the coverage of a rounded rectangle (PushClipRRect,
// FillRRect): the closed form under a scale and a translation, a filled
// path otherwise.
func (k *Masker) RRect(dst []uint8, rect image.Rectangle, m gfx.Affine, rr gfx.RRect) {
	if k.begin(dst, rect, m) {
		k.r.coverRRect(rr)
		k.end()
	}
}

// EndFrame and Trim age and release the Masker's scratch memory and its
// stroke outlines, like Renderer.EndFrame and Renderer.Trim.
func (k *Masker) EndFrame() { k.r.endFrame(false) }

// Trim releases all the scratch memory of the Masker.
func (k *Masker) Trim() { k.r.endFrame(true) }

// GlyphKey identifies the pixels of a cached glyph (a mask or a sprite):
// every input of its rendering is part of it (font, glyph, device em size,
// quarter-pixel offset, flattening tolerance, and for the sprites of color
// glyphs the nesting depth and the foreground color), so equal keys mean
// identical pixels. It is comparable.
type GlyphKey struct{ k glyphKey }

// GlyphKind says how a glyph of a run is drawn (see "Text" in the package
// documentation).
type GlyphKind uint8

const (
	// GlyphNone: nothing is drawn (no outline, outside the visible
	// rectangle, nested too deep).
	GlyphNone GlyphKind = iota
	// GlyphMask: Pix is the 8-bit coverage of the device pixels Rect,
	// composited with the run's paint.
	GlyphMask
	// GlyphSprite: Pix is the premultiplied RGBA picture of a color glyph
	// over the device pixels Rect, composited with source-over at the
	// run's alpha.
	GlyphSprite
	// GlyphOutline: Outline is filled with NonZero under Transform and the
	// run's paint (rotations, skews, large sizes).
	GlyphOutline
	// GlyphList: the commands of List, a color glyph's display list, are
	// drawn under Transform, one level deeper in the nesting of color
	// glyphs.
	GlyphList
)

// Glyph is one glyph of a run as the rasterizer draws it.
type Glyph struct {
	Kind GlyphKind
	// GlyphMask and GlyphSprite: the cached pixels, which are immutable and
	// shared with the GlyphCache, the device pixels they cover (Rect.Dx()
	// bytes per row for a mask, four times that for a sprite) and their
	// key.
	Key  GlyphKey
	Rect image.Rectangle
	Pix  []uint8
	// GlyphOutline and GlyphList: the glyph's transform (em space to
	// device) and what is drawn under it.
	Transform gfx.Affine
	Outline   gfx.Path
	List      *gfx.DisplayList
}

// BeginGlyphRun starts the run c drawn under the transform ctm, with the
// glyph cache a Render would use (nil selects the package's) and at the
// given nesting depth of color glyphs (0 for a run of the frame's list,
// one more inside each GlyphList). It makes the decisions of DrawGlyphRun
// for the whole run. ok is false when the run draws nothing (a transparent
// solid paint); alpha is what color glyph sprites are faded by.
//
// c must stay unchanged until the last Glyph call for the run. The other
// methods of the Masker may be called between the glyphs of a run, but
// not BeginGlyphRun.
func (k *Masker) BeginGlyphRun(c *gfx.DrawGlyphRun, ctm gfx.Affine, cache *GlyphCache, depth int) (alpha uint8, ok bool) {
	r := &k.r
	if r.emitFn == nil {
		r.emitFn = r.emit
		r.visitFn = r.visit
	}
	k.c, k.ctm = *c, ctm
	r.glyphs, r.strk, r.glyphDepth = cache, &r.strokes, depth
	k.run, ok = newGlyphRun(&k.c, &k.ctm, false)
	return k.run.alpha, ok
}

// Glyph resolves glyph g of the run started by BeginGlyphRun into out: its
// cached mask or sprite at the snapped pen position, rendered on a miss
// exactly as a Render does and kept in the glyph cache, or its outline or
// list for the glyphs the rasterizer draws directly. visible is the device
// rectangle being drawn: a glyph whose bounds miss it is GlyphNone, as a
// Render skips it.
func (k *Masker) Glyph(g gfx.Glyph, visible image.Rectangle, out *Glyph) {
	r := &k.r
	r.ctm = k.ctm // a coverage rasterized since the last glyph changed it
	run := &k.run
	c := run.c
	*out = Glyph{}
	gb := c.Font.GlyphBounds(g.ID)
	if gb.Empty() { // also NaN
		return
	}
	lx, ly := c.Origin.X+g.X, c.Origin.Y+g.Y
	if run.fast {
		e, ix, iy, res := r.cachedGlyph(run, g.ID, gb, lx, ly, visible)
		switch res {
		case glyphFound:
			out.Kind = GlyphMask
			if e.kind == entrySprite {
				out.Kind = GlyphSprite
			}
			out.Key, out.Rect, out.Pix = GlyphKey{e.key}, e.rect.Add(image.Pt(ix, iy)), e.data
			return
		case glyphNothing:
			return
		}
	}
	gm, cl, p, ok := directGlyph(run, &r.ctm, g.ID, gb, lx, ly, visible)
	switch {
	case !ok:
	case cl != nil:
		if r.glyphDepth < maxGlyphNesting {
			out.Kind, out.Transform, out.List = GlyphList, gm, cl
		}
	default:
		out.Kind, out.Transform, out.Outline = GlyphOutline, gm, p
	}
}

// BlurKernel is the kernel a Blur or a DropShadow applies along one device
// axis, as the rasterizer computes it (blur.go): what the GPU renderer
// reproduces, or approximates within its tolerance.
type BlurKernel struct {
	// Sigma and Offset are the device standard deviation and the shift of
	// the output, clamped as the rasterizer clamps them.
	Sigma, Offset float64
	// Down is the size of the blocks of a kernel applied at a reduced
	// resolution (1: none), and Shift the whole pixels its output is
	// shifted by.
	Down, Shift int
	// Lo and W are the weights: out[i] is the sum of W[t]*in[i+Lo+t], on
	// pixels, or on the means of blocks when Down > 1. The weights add up
	// to 1; three box passes are their convolution. A kernel that changes
	// nothing has Lo 0 and a single weight.
	Lo int
	W  []float64
	// Left and Right are how far the kernel reads before and after a
	// pixel, in pixels.
	Left, Right int
	// Variance is the variance, in pixels², of the three box passes the
	// kernel is made of, or of those a kernel at a reduced resolution
	// stands for. It is 0 for the exact kernels of small deviations.
	Variance float64
	// Limit is the reach the kernel may not exceed: 3*Sigma + |Offset|
	// (gfx.FilterOutset), which is what keeps regions exact.
	Limit float64
}

// Identity reports whether the kernel changes nothing.
func (k *BlurKernel) Identity() bool { return k.Down <= 1 && k.Lo == 0 && len(k.W) == 1 }

// NewBlurKernel returns the kernel of a Gaussian of device standard
// deviation sigma whose output is shifted by offset device pixels.
func NewBlurKernel(sigma, offset float64) BlurKernel {
	if !(sigma >= 0) {
		sigma = 0
	}
	sigma = min(sigma, maxSigma)
	offset = min(max(offset, -maxOffset), maxOffset)
	k, _ := newKernel(sigma, offset, nil)
	out := BlurKernel{Sigma: sigma, Offset: offset, Down: k.factor(), Shift: k.sh, Left: k.left, Right: k.right,
		Limit: float64(3*sigma) + math.Abs(offset)}
	switch {
	case k.down > 1:
		out.Variance = boxVariance(boxSize(sigma))
		out.W = fracBoxWeights(k.fm, k.fa, k.fb)
		out.Lo = -(len(out.W) / 2)
	case k.box:
		out.Variance = boxVariance(boxSize(sigma))
		w := []float64{1}
		for p := range 3 {
			box := make([]float64, k.l[p]+k.r[p]+1)
			for i := range box {
				box[i] = 1 / float64(len(box))
			}
			w = convolve(w, box)
		}
		out.W, out.Lo = w, -(k.l[0] + k.l[1] + k.l[2])
	default:
		out.Lo = k.lo
		out.W = make([]float64, len(k.w))
		for i, v := range k.w {
			out.W[i] = float64(v) / weightOne
		}
	}
	return out
}

// FracBoxKernel returns the weights of three passes of the fractional box
// (fracBox) whose variances add up to v, in samples²: the kernel a blur at
// a reduced resolution applies to its blocks. The weights are centered on
// the middle one and add up to 1.
func FracBoxKernel(v float64) []float64 {
	m, a, b := fracBox(v / 3)
	return fracBoxWeights(m, a, b)
}

// fracBoxWeights returns the convolution of three fractional boxes of
// 2m+1 samples of weight a and an end of weight b on each side, in units
// of 2^-boxFracBits, normalized.
func fracBoxWeights(m int, a, b uint64) []float64 {
	box := make([]float64, 2*m+3)
	var sum float64
	for i := range box {
		box[i] = float64(a)
		if i == 0 || i == len(box)-1 {
			box[i] = float64(b)
		}
		sum += box[i]
	}
	for i := range box {
		box[i] /= sum
	}
	return convolve(convolve(box, box), box)
}

// convolve returns the convolution of two kernels.
func convolve(a, b []float64) []float64 {
	out := make([]float64, len(a)+len(b)-1)
	for i, x := range a {
		for j, y := range b {
			out[i+j] += float64(x * y)
		}
	}
	return out
}

// DeviceSigmas returns the standard deviations along device x and y of a
// local Gaussian of standard deviation s under m, as the filters of a
// layer take them (filter.go).
func DeviceSigmas(m gfx.Affine, s float64) (sx, sy float64) { return deviceSigmas(&m, s) }

// Unit8 converts a value in [0, 1] to 8 bits as the rasterizer does for
// opacities: rounded to nearest, clamped.
func Unit8(v float64) uint8 { return unit8(v) }
