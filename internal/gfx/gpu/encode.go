package gpu

import (
	"fmt"
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Options configures one render, with the meaning of raster.Options.
type Options struct {
	// Scale is the display scale: device pixels per logical pixel. The zero
	// value means 1. It must otherwise be positive and finite.
	Scale float32
	// Region limits drawing to these device pixels of the target. Pixels
	// outside it are not touched. The empty rectangle (the zero value)
	// means the whole target. The pixels of a region are those of a full
	// render.
	Region image.Rectangle
	// Clear, when not nil, is the color the region is replaced by before
	// the list is drawn. Without it the list is drawn over the pixels the
	// target holds.
	Clear *gfx.Color
}

// coordLimit bounds device coordinates, as in the rasterizer.
const coordLimit = 1 << 20

// An Encoder turns display lists into Frames. It keeps its buffers and
// the ramps of the gradients it has seen between calls; the zero value is
// ready to use. It is not safe for concurrent use.
type Encoder struct {
	frame   Frame
	pass    [1]Pass
	clear   Clear
	batches []Batch
	stack   []state
	clips   []softClip
	target  image.Rectangle

	ramps  []*bakedRamp
	serial uint64 // of the frame being encoded
	tick   uint64

	visitFn func(int, gfx.Command)
}

// state is what a push saves and its pop restores.
type state struct {
	ctm gfx.Affine // local -> device
	// skip: nothing up to the matching pop shows, or can be drawn.
	skip bool
	// scissor is the device rectangle that may be written: the region,
	// inside every clip. bound is the same without the region: what
	// decides how a clip is drawn looks at it, so that a render of a
	// region makes the decisions, and the pixels, of a full render.
	scissor, bound image.Rectangle
	// clip is the index in Encoder.clips of the anti-aliased clip whose
	// coverage multiplies what is drawn, or -1.
	clip int
}

// softClip is a clip whose edges do not fall on pixel boundaries.
type softClip struct {
	sh shape
	// full are the pixels the clip covers entirely (maybe none): where the
	// scissor lies inside them, the clip changes nothing.
	full image.Rectangle
}

// Encode returns the frame that renders dl into a target of the given size
// in device pixels. The frame is valid until the next call. A nil or empty
// list gives a frame that only clears (or does nothing).
//
// Encode panics if opts.Scale is negative, NaN or infinite, like
// raster.Render.
func (e *Encoder) Encode(dl *gfx.DisplayList, size image.Point, opts Options) *Frame {
	s := opts.Scale
	if s == 0 {
		s = 1
	}
	if !(s > 0) || math.IsInf(float64(s), 0) {
		panic(fmt.Sprintf("gpu: Options.Scale must be a positive finite number (0 means 1), got %v", opts.Scale))
	}
	if e.visitFn == nil {
		e.visitFn = e.visit
	}
	e.serial++
	e.batches = e.batches[:0]
	e.clips = e.clips[:0]
	e.stack = e.stack[:0]
	f := &e.frame
	*f = Frame{
		Size:         size,
		Instances:    f.Instances[:0],
		Ramps:        f.Ramps[:0],
		RampSegments: f.RampSegments[:0],
		Unsupported:  f.Unsupported[:0],
	}
	e.target = image.Rectangle{Max: size}
	region := e.target
	if !opts.Region.Empty() {
		region = region.Intersect(opts.Region)
	}
	region = region.Intersect(image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit))
	if region.Empty() {
		return f
	}
	e.pass[0] = Pass{}
	if opts.Clear != nil {
		e.clear = Clear{Rect: region, Color: premul8(*opts.Clear)}
		e.pass[0].Clear = &e.clear
	}
	if dl != nil && dl.Len() > 0 {
		e.stack = append(e.stack, state{ctm: gfx.Scale(s, s), scissor: region, bound: e.target, clip: -1})
		// The region in root space, grown by one device pixel so that
		// float32 rounding in the list's bounds never culls a command that
		// touches a pixel of the region (as raster does).
		fs := float64(s)
		dl.Visit(gfx.Rect{
			X0: float32(float64(region.Min.X-1) / fs), Y0: float32(float64(region.Min.Y-1) / fs),
			X1: float32(float64(region.Max.X+1) / fs), Y1: float32(float64(region.Max.Y+1) / fs),
		}, e.visitFn)
	}
	e.pass[0].Batches = e.batches
	if e.pass[0].Clear != nil || len(e.batches) > 0 {
		f.Passes = e.pass[:]
	}
	return f
}

// Unsupported returns the names of what dl holds that the renderer does
// not draw yet (see Frame.Unsupported), wherever it is in the list, or nil
// when it draws all of it. The cases that depend on the transform (nested
// anti-aliased clips) are only known to Encode.
func Unsupported(dl *gfx.DisplayList) []string {
	var out []string
	if dl == nil {
		return nil
	}
	for _, c := range dl.All() {
		if name, bad := unsupported(c); bad {
			out = appendOnce(out, name)
		}
	}
	return out
}

// unsupported reports whether the renderer does not draw c yet, and the
// name it is reported under.
func unsupported(c gfx.Command) (string, bool) {
	switch c := c.(type) {
	case gfx.FillRect, gfx.FillRRect, gfx.DrawShadow, gfx.PushClipRect, gfx.PopClip,
		gfx.PushTransform, gfx.PopTransform, gfx.PopLayer, gfx.Marker:
		return "", false
	case gfx.PushClipRRect:
		return c.Name(), !c.RRect.IsRect()
	}
	return c.Name(), true
}

func appendOnce(list []string, name string) []string {
	for _, s := range list {
		if s == name {
			return list
		}
	}
	return append(list, name)
}

func (e *Encoder) top() *state { return &e.stack[len(e.stack)-1] }

// push saves the current state; the matching pop restores it.
func (e *Encoder) push() *state {
	e.stack = append(e.stack, *e.top())
	return e.top()
}

// visit encodes one command.
func (e *Encoder) visit(_ int, c gfx.Command) {
	if name, bad := unsupported(c); bad {
		st := e.top()
		if !st.skip {
			// Inside a block that shows nothing the command would not be
			// drawn anyway.
			e.frame.Unsupported = appendOnce(e.frame.Unsupported, name)
		}
		switch c.(type) {
		case gfx.PushClipRRect, gfx.PushClipPath, gfx.PushLayer:
			e.push().skip = true
		}
		return
	}
	switch c := c.(type) {
	case gfx.FillRect:
		if !e.top().skip {
			e.fill(gfx.RRect{Rect: c.Rect}, c.Paint)
		}
	case gfx.FillRRect:
		if !e.top().skip {
			e.fill(c.RRect, c.Paint)
		}
	case gfx.DrawShadow:
		if !e.top().skip {
			e.shadow(c)
		}
	case gfx.PushTransform:
		st := e.push()
		if !st.skip {
			next := raster.GridMul(st.ctm, c.Affine) // whole-pixel translations stay on the grid
			if next.IsFinite() && next.Det() != 0 {
				st.ctm = next
			} else {
				st.skip = true
			}
		}
	case gfx.PushClipRect:
		if st := e.push(); !st.skip {
			e.clipRect(st, c.Rect)
		}
	case gfx.PushClipRRect: // with square corners
		if st := e.push(); !st.skip {
			e.clipRect(st, c.RRect.Rect)
		}
	case gfx.PopTransform, gfx.PopClip, gfx.PopLayer:
		e.stack = e.stack[:len(e.stack)-1]
	case gfx.Marker:
		// A placeholder of a composed list: it paints nothing.
	}
}

// clipRect intersects the clip of st, a state just pushed, with a local
// rectangle (raster's clipRect). A rectangle whose device edges fall on
// pixel boundaries only narrows the scissor. Any other becomes the
// anti-aliased clip of the instances inside it, unless the pixels left by
// the clips lie within those it covers entirely. One instance carries one
// such clip, so a second one that still matters is reported as
// unsupported.
func (e *Encoder) clipRect(st *state, rect gfx.Rect) {
	var sc softClip
	var outer image.Rectangle
	if st.ctm.IsAxisAligned() {
		sh, bounds, ok := deviceShape(&st.ctm, gfx.RRect{Rect: rect})
		if !ok {
			st.skip = true // snapped to nothing
			return
		}
		outer = bounds
		sc = softClip{sh: sh, full: rectNoSwap(
			int(math.Ceil(sh.rect[0])), int(math.Ceil(sh.rect[1])),
			int(math.Floor(sh.rect[2])), int(math.Floor(sh.rect[3])))}
	} else {
		sh, bounds, ok := localShape(&st.ctm, gfx.RRect{Rect: rect})
		if !ok {
			st.skip = true
			return
		}
		outer, sc = bounds, softClip{sh: sh}
	}
	st.scissor, st.bound = st.scissor.Intersect(outer), st.bound.Intersect(outer)
	if st.scissor.Empty() {
		st.skip = true
		return
	}
	if st.clip >= 0 && st.bound.In(e.clips[st.clip].full) {
		st.clip = -1 // the enclosing clip covers every pixel left
	}
	if st.bound.In(sc.full) {
		return // pixel aligned, or nothing left at its edges
	}
	if st.clip >= 0 {
		e.frame.Unsupported = appendOnce(e.frame.Unsupported, "PushClipRect (nested anti-aliased)")
		st.skip = true
		return
	}
	e.clips = append(e.clips, sc)
	st.clip = len(e.clips) - 1
}

// rectNoSwap is image.Rect without the swapping of inverted coordinates:
// an inverted rectangle is empty.
func rectNoSwap(x0, y0, x1, y1 int) image.Rectangle {
	if x0 >= x1 || y0 >= y1 {
		return image.Rectangle{}
	}
	return image.Rectangle{Min: image.Pt(x0, y0), Max: image.Pt(x1, y1)}
}

// fill encodes a rectangle or rounded rectangle filled with a paint.
func (e *Encoder) fill(rr gfx.RRect, p gfx.Paint) {
	st := e.top()
	sh, bounds, ok := shapeFor(&st.ctm, rr)
	if !ok || !bounds.Overlaps(st.scissor) {
		return
	}
	pe, ok := e.paint(p, &st.ctm)
	if !ok {
		return
	}
	var in instance
	quad := e.begin(&in, bounds)
	ox, oy := float64(quad.Min.X), float64(quad.Min.Y)
	var flags uint32
	if sh.local {
		flags |= FlagShapeLocal
	}
	sh.put(&in, SlotShape, ox, oy)
	flags |= pe.put(&in, ox, oy)
	e.emit(&in, flags, st, ox, oy)
}

// begin starts an instance whose pixels lie in bounds, and returns its
// quad: the bounds inside the target. The quad does not depend on the
// region, so that an instance has the same bytes, and its pixels the same
// values, in a render of a region and in a full one.
func (e *Encoder) begin(in *instance, bounds image.Rectangle) image.Rectangle {
	quad := bounds.Intersect(e.target)
	in.floats(SlotBounds, float64(quad.Min.X), float64(quad.Min.Y), float64(quad.Max.X), float64(quad.Max.Y))
	return quad
}

// emit finishes an instance with its clip and flags and appends it to the
// frame, in the batch of the current scissor.
func (e *Encoder) emit(in *instance, flags uint32, st *state, ox, oy float64) {
	if st.clip >= 0 {
		sc := &e.clips[st.clip]
		if sc.sh.local {
			flags |= ClipLocal << FlagClipShift
		} else {
			flags |= ClipDevice << FlagClipShift
		}
		sc.sh.put(in, SlotClip, ox, oy)
	}
	in[SlotHead][0] = flags
	// A coordinate beyond float32 (a transform that scales out of range)
	// would reach the shader as an infinity: nothing sensible is drawn.
	for slot := range in {
		if slot == SlotHead {
			continue
		}
		for _, v := range in[slot] {
			if v&0x7f800000 == 0x7f800000 {
				return
			}
		}
	}
	f := &e.frame
	n := len(f.Instances) / InstanceSize
	f.Instances = in.appendTo(f.Instances)
	// perf: instances of different scissors could share a draw if the
	// scissor moved into the instance (its quad clipped by it).
	if k := len(e.batches) - 1; k >= 0 && e.batches[k].Pipeline == PipelineQuad && e.batches[k].Scissor == st.scissor {
		e.batches[k].Count++
		return
	}
	e.batches = append(e.batches, Batch{Pipeline: PipelineQuad, Scissor: st.scissor, First: n, Count: 1})
}

func premul8(c gfx.Color) [4]uint8 {
	p := c.Premul8()
	return [4]uint8{p.R, p.G, p.B, p.A}
}

// ---------------------------------------------------------------------------
// Shapes
// ---------------------------------------------------------------------------

// shape is a rounded rectangle as an instance carries it (see ShapeSlots),
// in float64 and absolute device coordinates until it is written relative
// to an instance's origin.
type shape struct {
	local      bool
	rect       [4]float64 // x0, y0, x1, y1
	radX, radY [4]float64 // top-left, top-right, bottom-right, bottom-left
	inv        inverse    // local shapes: device -> local
}

// put writes the shape into the slots of in from slot on, for an instance
// whose origin is (ox, oy).
func (s *shape) put(in *instance, slot int, ox, oy float64) {
	if s.local {
		in.floats(slot, s.rect[0], s.rect[1], s.rect[2], s.rect[3])
		s.inv.put(in, slot+3, ox, oy)
	} else {
		// Whole numbers subtracted from multiples of 1/256: exact.
		in.floats(slot, s.rect[0]-ox, s.rect[1]-oy, s.rect[2]-ox, s.rect[3]-oy)
	}
	in.floats(slot+1, s.radX[0], s.radX[1], s.radX[2], s.radX[3])
	in.floats(slot+2, s.radY[0], s.radY[1], s.radY[2], s.radY[3])
}

// inverse is the inverse of a device transform, in float64: a local point
// is (a*X + c*Y + e, b*X + d*Y + f).
type inverse struct{ a, b, c, d, e, f float64 }

// invert returns the inverse of m. The products of its float32 entries are
// exact in float64, so the result does not depend on fused multiply-adds
// (raster's invert).
func invert(m *gfx.Affine) (inverse, bool) {
	m0, m1, m2, m3, m4, m5 := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3]), float64(m[4]), float64(m[5])
	det := m0*m3 - m1*m2
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return inverse{}, false
	}
	inv := inverse{
		a: m3 / det, b: -m1 / det, c: -m2 / det, d: m0 / det,
		e: (m2*m5 - m3*m4) / det, f: (m1*m4 - m0*m5) / det,
	}
	for _, v := range [6]float64{inv.a, inv.b, inv.c, inv.d, inv.e, inv.f} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return inverse{}, false
		}
	}
	return inv, true
}

// compose returns the coefficients (a, b, c) of the device-space affine
// function a*X + b*Y + c equal to the local function kx*x + ky*y + k0.
func (inv *inverse) compose(kx, ky, k0 float64) (a, b, c float64) {
	a = float64(kx*inv.a) + float64(ky*inv.b)
	b = float64(kx*inv.c) + float64(ky*inv.d)
	c = float64(kx*inv.e) + float64(ky*inv.f) + k0
	return a, b, c
}

// put writes the map as two slots, for pixel centers relative to (ox, oy).
func (inv *inverse) put(in *instance, slot int, ox, oy float64) {
	in.floats(slot, inv.a, inv.b, inv.c, inv.d)
	in.floats(slot+1, inv.e+float64(inv.a*ox)+float64(inv.c*oy), inv.f+float64(inv.b*ox)+float64(inv.d*oy), 0, 0)
}

// apply transforms a local point to device space in float64. The products
// of two float32 values are exact in float64.
func apply(m *gfx.Affine, x, y float32) (float64, float64) {
	fx, fy := float64(x), float64(y)
	return float64(m[0])*fx + float64(m[2])*fy + float64(m[4]),
		float64(m[1])*fx + float64(m[3])*fy + float64(m[5])
}

func clampLim(x float64) float64 { return min(max(x, -coordLimit), coordLimit) }

// snap rounds a device coordinate to the 1/256 pixel grid of the
// rasterizer (raster's fix: round half up).
func snap(v float64) float64 { return math.Floor(v*256+0.5) / 256 }

// shapeFor returns the shape of rr under m and the device pixels it can
// touch: a device shape when m keeps rectangles axis-aligned, where the
// rasterizer has its exact fast paths, else a local one. ok is false when
// nothing is covered.
func shapeFor(m *gfx.Affine, rr gfx.RRect) (shape, image.Rectangle, bool) {
	if m.IsAxisAligned() {
		return deviceShape(m, rr)
	}
	return localShape(m, rr)
}

// deviceShape maps rr through the axis-aligned transform m: a scale and a
// translation, maybe with a quarter turn, flips included. The rectangle
// is snapped to 1/256 pixel and the radii go to the device corners their
// corners land on, then shrink if snapping left a side shorter than their
// sum, like raster's fillRRectFast. (For a quarter turn the rasterizer
// fills a rounded rectangle as a path; this is the exact area it
// approximates.)
func deviceShape(m *gfx.Affine, rr gfx.RRect) (s shape, bounds image.Rectangle, ok bool) {
	ax, ay := apply(m, rr.Rect.X0, rr.Rect.Y0)
	bx, by := apply(m, rr.Rect.X1, rr.Rect.Y1)
	// Opposite corners map to opposite corners.
	x0, y0, x1, y1 := min(ax, bx), min(ay, by), max(ax, bx), max(ay, by)
	if !(x0 <= x1 && y0 <= y1) { // NaN
		return s, bounds, false
	}
	if rr.IsRect() {
		// The parts beyond ±coordLimit are outside every target, so
		// clamping is exact. (Rounded corners would move with the edges:
		// a rounded rectangle that large keeps its coordinates, and the
		// shader's float32 gives its far side up.)
		x0, y0, x1, y1 = clampLim(x0), clampLim(y0), clampLim(x1), clampLim(y1)
	}
	x0, y0, x1, y1 = snap(x0), snap(y0), snap(x1), snap(y1)
	if x0 >= x1 || y0 >= y1 || math.IsInf(x0, 0) || math.IsInf(y0, 0) || math.IsInf(x1, 0) || math.IsInf(y1, 0) {
		return s, bounds, false
	}
	s.rect = [4]float64{x0, y0, x1, y1}
	bounds = image.Rect(
		int(math.Floor(clampLim(x0))), int(math.Floor(clampLim(y0))),
		int(math.Ceil(clampLim(x1))), int(math.Ceil(clampLim(y1))))
	if rr.IsRect() {
		return s, bounds, true
	}
	a, b, c, d := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	turn := a == 0 && d == 0
	for i, p := range rr.Radii {
		if !(p.X > 0 && p.Y > 0) {
			continue
		}
		right, bottom := i == 1 || i == 2, i >= 2
		var rx, ry float64 // exact products of float32 values
		if turn {
			rx, ry = math.Abs(c)*float64(p.Y), math.Abs(b)*float64(p.X)
			right, bottom = bottom != (c < 0), right != (b < 0)
		} else {
			rx, ry = math.Abs(a)*float64(p.X), math.Abs(d)*float64(p.Y)
			right, bottom = right != (a < 0), bottom != (d < 0)
		}
		j := 0
		switch {
		case right && !bottom:
			j = 1
		case right && bottom:
			j = 2
		case bottom:
			j = 3
		}
		if rx > 0 && ry > 0 {
			s.radX[j], s.radY[j] = rx, ry
		}
	}
	// Snapping can shrink a side below the sum of its radii by up to
	// 1/256 pixel: scale the radii down again like gfx.RRect.Normalize.
	w, h := x1-x0, y1-y0
	f := 1.0
	for _, side := range [4][2]float64{
		{w, s.radX[0] + s.radX[1]}, {h, s.radY[1] + s.radY[2]},
		{w, s.radX[3] + s.radX[2]}, {h, s.radY[0] + s.radY[3]},
	} {
		if side[1] > side[0] {
			f = min(f, side[0]/side[1])
		}
	}
	if f < 1 {
		for i := range s.radX {
			s.radX[i], s.radY[i] = float64(s.radX[i]*f), float64(s.radY[i]*f)
		}
	}
	return s, bounds, true
}

// localShape returns rr as a local shape under a transform that rotates or
// skews, and the device pixels it can touch (with a margin of one pixel,
// like raster's affineBounds).
func localShape(m *gfx.Affine, rr gfx.RRect) (s shape, bounds image.Rectangle, ok bool) {
	inv, ok := invert(m)
	if !ok {
		return s, bounds, false
	}
	r := rr.Rect.Abs()
	if r.Empty() {
		return s, bounds, false
	}
	s = shape{local: true, rect: [4]float64{float64(r.X0), float64(r.Y0), float64(r.X1), float64(r.Y1)}, inv: inv}
	for i, p := range rr.Radii {
		if p.X > 0 && p.Y > 0 {
			s.radX[i], s.radY[i] = float64(p.X), float64(p.Y)
		}
	}
	bounds = touchBounds(m, s.rect).Inset(-1)
	return s, bounds, !bounds.Empty()
}

// touchBounds returns the device pixels whose square touches the
// transformed local rectangle r, clamped to ±coordLimit (raster's
// touchBounds).
func touchBounds(m *gfx.Affine, r [4]float64) image.Rectangle {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4][2]float64{{r[0], r[1]}, {r[2], r[1]}, {r[2], r[3]}, {r[0], r[3]}} {
		x := float64(float64(m[0])*p[0]) + float64(float64(m[2])*p[1]) + float64(m[4])
		y := float64(float64(m[1])*p[0]) + float64(float64(m[3])*p[1]) + float64(m[5])
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	if !(x0 <= x1 && y0 <= y1) {
		return image.Rectangle{}
	}
	return image.Rect(
		int(math.Floor(clampLim(x0))), int(math.Floor(clampLim(y0))),
		int(math.Ceil(clampLim(x1))), int(math.Ceil(clampLim(y1))),
	)
}
