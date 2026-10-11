package raster

import (
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/kuronosu/tela/internal/gfx"
)

// Options configures one Render call.
type Options struct {
	// Scale is the display scale: device pixels per logical pixel. The zero
	// value means 1. It must otherwise be positive and finite.
	Scale float32
	// Region limits drawing to these device pixels, in dst's coordinate
	// space. Pixels outside it are not touched. The empty rectangle (the
	// zero value) means all of dst. The pixels of a region are identical to
	// the same pixels of a full render.
	Region image.Rectangle
}

// Renderer renders display lists into RGBA images. It owns the scratch
// buffers of the rasterizer and reuses them between calls, so a long-lived
// Renderer does not allocate in the steady state.
//
// A Renderer must not be used by several goroutines at once; give each
// raster worker its own. The zero value is ready to use.
type Renderer struct {
	fill  pathFiller
	blit  blitter
	solid solidShader

	ctm      gfx.Affine // current transform: local -> device
	skipping bool       // inside a transform block that shows nothing
	stack    []savedState

	// Path flattening state; emitFn is the method value of emit, created
	// once so that flattening does not allocate.
	flatM     gfx.Affine
	flatCurve bool     // flattening one curve of flattenCulled
	curve     gfx.Path // scratch path for flattenCulled
	emitFn    func(gfx.Verb, gfx.Point)
	visitFn   func(int, gfx.Command)

	rowMask []uint8   // rrect rows
	rowSub  []float64 // rrect corner areas
}

type savedState struct {
	ctm      gfx.Affine
	skipping bool
}

// NewRenderer returns a new Renderer. It is equivalent to new(Renderer).
func NewRenderer() *Renderer { return new(Renderer) }

var pool = sync.Pool{New: func() any { return NewRenderer() }}

// Render renders dl onto dst (compositing over its current content) with a
// Renderer taken from a pool. It is safe for concurrent use; see
// Renderer.Render.
func Render(dst *image.RGBA, dl *gfx.DisplayList, opts Options) {
	r := pool.Get().(*Renderer)
	defer pool.Put(r)
	r.Render(dst, dl, opts)
}

// Render renders dl onto dst, compositing over its current content (use
// Clear first for a fresh frame). The root space of dl is scaled by
// opts.Scale, and only the device pixels in opts.Region are written.
//
// Render panics if dst is nil, if opts.Scale is negative, NaN or infinite,
// and (for now) if the visited part of the list holds a command or paint
// that is not implemented yet; see the package documentation. A nil dl
// renders nothing.
func (r *Renderer) Render(dst *image.RGBA, dl *gfx.DisplayList, opts Options) {
	if dst == nil {
		panic("raster: Render called with a nil destination image")
	}
	s := opts.Scale
	if s == 0 {
		s = 1
	}
	if !(s > 0) || math.IsInf(float64(s), 0) {
		panic(fmt.Sprintf("raster: Options.Scale must be a positive finite number (0 means 1), got %v", opts.Scale))
	}
	if dl == nil || dl.Len() == 0 {
		return
	}
	clip := dst.Rect
	if !opts.Region.Empty() {
		clip = clip.Intersect(opts.Region)
	}
	clip = clip.Intersect(image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit))
	if clip.Empty() {
		return
	}
	if r.emitFn == nil {
		r.emitFn = r.emit
		r.visitFn = r.visit
	}
	r.blit.pix, r.blit.stride, r.blit.origin, r.blit.clip = dst.Pix, dst.Stride, dst.Rect.Min, clip
	r.ctm = gfx.Scale(s, s)
	r.skipping = false
	r.stack = r.stack[:0]

	// The clip in root space, grown by one device pixel so that float32
	// rounding in the list's bounds can never cull a command that touches
	// a pixel of the region.
	sc := float64(s)
	root := gfx.Rect{
		X0: float32(float64(clip.Min.X-1) / sc), Y0: float32(float64(clip.Min.Y-1) / sc),
		X1: float32(float64(clip.Max.X+1) / sc), Y1: float32(float64(clip.Max.Y+1) / sc),
	}
	defer func() {
		r.blit.pix, r.blit.sh, r.blit.solid = nil, nil, nil // do not retain dst
	}()
	dl.Visit(root, r.visitFn)
}

// visit executes one command.
func (r *Renderer) visit(_ int, c gfx.Command) {
	switch c := c.(type) {
	case gfx.FillRect:
		if !r.skipping {
			r.fillRect(c.Rect, c.Paint)
		}
	case gfx.FillRRect:
		if !r.skipping {
			r.fillRRect(c.RRect, c.Paint)
		}
	case gfx.FillPath:
		if !r.skipping {
			r.blit.setShader(r.shaderFor(c.Paint, r.ctm))
			r.fillPath(c.Path, c.Rule)
		}
	case gfx.PushTransform:
		r.stack = append(r.stack, savedState{r.ctm, r.skipping})
		if !r.skipping {
			next := r.ctm.Mul(c.Affine)
			if next.IsFinite() && next.Det() != 0 {
				r.ctm = next
			} else {
				// The display scale pushed the transform out of float32
				// range, or it became singular: nothing inside can show.
				r.skipping = true
			}
		}
	case gfx.PopTransform:
		st := r.stack[len(r.stack)-1]
		r.stack = r.stack[:len(r.stack)-1]
		r.ctm, r.skipping = st.ctm, st.skipping
	default:
		panic(fmt.Sprintf("raster: %s is not implemented yet", c.Name()))
	}
}

// apply transforms a local point to device space in float64. The products
// of two float32 values are exact in float64, so this result cannot change
// if the compiler fuses them into multiply-adds.
func apply(m *gfx.Affine, p gfx.Point) (float64, float64) {
	x, y := float64(p.X), float64(p.Y)
	return float64(m[0])*x + float64(m[2])*y + float64(m[4]),
		float64(m[1])*x + float64(m[3])*y + float64(m[5])
}

// fillRect fills an axis-aligned rectangle in local space.
func (r *Renderer) fillRect(rect gfx.Rect, paint gfx.Paint) {
	r.blit.setShader(r.shaderFor(paint, r.ctm))
	m := &r.ctm
	if m.IsAxisAligned() {
		// Opposite corners map to opposite corners.
		x0, y0 := apply(m, gfx.Pt(rect.X0, rect.Y0))
		x1, y1 := apply(m, gfx.Pt(rect.X1, rect.Y1))
		r.fillRectDevice(min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1))
		return
	}
	f := &r.fill
	f.reset()
	f.moveTo(apply(m, gfx.Pt(rect.X0, rect.Y0)))
	f.lineTo(apply(m, gfx.Pt(rect.X1, rect.Y0)))
	f.lineTo(apply(m, gfx.Pt(rect.X1, rect.Y1)))
	f.lineTo(apply(m, gfx.Pt(rect.X0, rect.Y1)))
	f.closePath()
	f.rasterize(gfx.NonZero, &r.blit)
}

// fillRectDevice fills a device-space rectangle (x0 <= x1, y0 <= y1). The
// parts beyond ±coordLimit are outside every region, so clamping is exact.
func (r *Renderer) fillRectDevice(x0, y0, x1, y1 float64) {
	r.fillRectFixed(fix(clampLim(x0)), fix(clampLim(y0)), fix(clampLim(x1)), fix(clampLim(y1)))
}

// fillRectFixed fills the fixed-point rectangle [X0, X1) x [Y0, Y1). The
// coverage of a pixel is the product of its horizontal and vertical
// overlaps, which is exactly what the path rasterizer computes for the same
// rectangle.
func (r *Renderer) fillRectFixed(X0, Y0, X1, Y1 int64) {
	b := &r.blit
	if X0 >= X1 || Y0 >= Y1 {
		return
	}
	px0 := max(int(X0>>fracBits), b.clip.Min.X)
	px1 := min(int((X1+one-1)>>fracBits), b.clip.Max.X)
	py0 := max(int(Y0>>fracBits), b.clip.Min.Y)
	py1 := min(int((Y1+one-1)>>fracBits), b.clip.Max.Y)
	if px0 >= px1 || py0 >= py1 {
		return
	}
	for y := py0; y < py1; y++ {
		top := int64(y) << fracBits
		covy := min(Y1, top+one) - max(Y0, top)
		b.rectRow(y, X0, X1, covy, px0, px1)
	}
}

// rectRow composites one row of the rectangle [X0, X1) whose vertical
// coverage in this row is covy (fixed point), restricted to the columns
// [px0, px1).
func (b *blitter) rectRow(y int, X0, X1, covy int64, px0, px1 int) {
	full0 := (X0 + one - 1) >> fracBits // first fully covered column
	full1 := X1 >> fracBits             // end of the fully covered columns
	in := func(x int64) bool { return x >= int64(px0) && x < int64(px1) }
	if full0 > full1 { // both edges in one column
		if x := X0 >> fracBits; in(x) {
			b.run(y, int(x), 1, cov8((X1-X0)*covy))
		}
		return
	}
	if X0&(one-1) != 0 {
		if x := full0 - 1; in(x) {
			b.run(y, int(x), 1, cov8((full0<<fracBits-X0)*covy))
		}
	}
	if lo, hi := max(int(full0), px0), min(int(full1), px1); lo < hi {
		b.run(y, lo, hi-lo, cov8(one*covy))
	}
	if X1&(one-1) != 0 {
		if x := full1; in(x) {
			b.run(y, int(x), 1, cov8((X1-full1<<fracBits)*covy))
		}
	}
}

// rectCovX returns the horizontal overlap (fixed point) of column x with
// [X0, X1).
func rectCovX(x int, X0, X1 int64) int64 {
	l := int64(x) << fracBits
	return max(min(X1, l+one)-max(X0, l), 0)
}

// fillPath fills a local-space path under the current transform.
func (r *Renderer) fillPath(p gfx.Path, rule gfx.FillRule) {
	m := r.ctm
	// The Frobenius norm bounds the largest stretch of m (and is at most
	// sqrt(2) times it), so the local tolerance below keeps the device-space
	// error within FlattenTolerance. The squares are exact products.
	a, b, c, d := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	norm := math.Sqrt(a*a + b*b + c*c + d*d)
	if !(norm > 0) || math.IsInf(norm, 0) {
		return
	}
	tol := float32(float64(FlattenTolerance) / norm)
	f := &r.fill
	f.reset()
	r.flatM = m
	r.flatCurve = false
	if r.pathNear(p) {
		p.Flatten(tol, r.emitFn)
	} else {
		r.flattenCulled(p, tol)
	}
	f.closePath()
	f.rasterize(rule, &r.blit)
}

// cullMargin is how far, in device pixels, outside the clip a curve must lie
// to be replaced by its chord (see flattenCulled).
const cullMargin = 2

// nearMargin is how far, in device pixels, a path may extend beyond the clip
// before its curves are culled. Either way gives the same pixels; tests
// set it to 0 to cull every path.
var nearMargin = 512

// pathNear reports whether the device-space control bounds of p lie within
// nearMargin pixels of the clip, so that flattening the whole path is cheap.
func (r *Renderer) pathNear(p gfx.Path) bool {
	near := r.blit.clip.Inset(-nearMargin)
	x0, y0 := float64(near.Min.X), float64(near.Min.Y)
	x1, y1 := float64(near.Max.X), float64(near.Max.Y)
	for _, pt := range p.Points() {
		x, y := apply(&r.flatM, pt)
		if !(x >= x0 && x <= x1 && y >= y0 && y <= y1) {
			return false
		}
	}
	return true
}

// flattenCulled flattens p segment by segment, replacing every curve whose
// device-space control box lies entirely outside the clip (grown by
// cullMargin pixels) by its chord. Such a curve and its chord give exactly
// the same pixels inside the clip: above, below or right of the clip neither
// contributes; left of it, each row only sees the net signed height that the
// polyline spans in that row, which telescopes to the chord's (the margin
// keeps the snapped points in the cells that are lumped). This bounds the
// work for huge paths, of which only a small part is visible, without
// changing a single pixel, whatever the region.
func (r *Renderer) flattenCulled(p gfx.Path, tol float32) {
	cull := r.blit.clip.Inset(-cullMargin)
	cx0, cy0 := float64(cull.Min.X), float64(cull.Min.Y)
	cx1, cy1 := float64(cull.Max.X), float64(cull.Max.Y)
	f := &r.fill
	for s := range p.Segments() {
		switch s.Verb {
		case gfx.MoveTo:
			f.moveTo(apply(&r.flatM, s.P[0]))
		case gfx.LineTo:
			f.lineTo(apply(&r.flatM, s.P[1]))
		case gfx.Close:
			f.closePath()
		case gfx.QuadTo, gfx.CubicTo:
			n := 3
			if s.Verb == gfx.CubicTo {
				n = 4
			}
			bx0, by0 := math.Inf(1), math.Inf(1)
			bx1, by1 := math.Inf(-1), math.Inf(-1)
			for _, pt := range s.P[:n] {
				x, y := apply(&r.flatM, pt)
				bx0, by0, bx1, by1 = min(bx0, x), min(by0, y), max(bx1, x), max(by1, y)
			}
			if bx1 < cx0 || bx0 > cx1 || by1 < cy0 || by0 > cy1 {
				f.lineTo(apply(&r.flatM, s.P[n-1]))
				continue
			}
			q := &r.curve
			q.Reset()
			q.MoveTo(s.P[0])
			if s.Verb == gfx.QuadTo {
				q.QuadTo(s.P[1], s.P[2])
			} else {
				q.CubicTo(s.P[1], s.P[2], s.P[3])
			}
			r.flatCurve = true
			q.Flatten(tol, r.emitFn)
			r.flatCurve = false
		}
	}
}

// emit receives the flattened path in local space. While a single curve is
// flattened (flatCurve), its leading MoveTo is the current point and is
// skipped.
func (r *Renderer) emit(v gfx.Verb, pt gfx.Point) {
	switch v {
	case gfx.MoveTo:
		if !r.flatCurve {
			r.fill.moveTo(apply(&r.flatM, pt))
		}
	case gfx.LineTo:
		r.fill.lineTo(apply(&r.flatM, pt))
	case gfx.Close:
		r.fill.closePath()
	}
}
