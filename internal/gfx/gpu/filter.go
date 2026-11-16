package gpu

import (
	"image"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Filters: Blur, DropShadow and ColorMatrix, on the layers that carry them
// and on the backdrop of a BackdropFilter, with the meaning of the
// rasterizer's (raster/filter.go, blur.go, backdrop.go).
//
// A filter is a few passes of PipelineFilter (shaders/filter.frag), each
// one instance that draws a rectangle of a target of the pool from another
// one: a picture is a plane, a target and the pixels of it that hold the
// picture, beyond which it is transparent, as the rasterizer's surfaces
// are. Each step computes only the pixels the steps after it read. The
// steps read texels by their whole coordinates and sum them with weights
// that are data (Frame.Taps), so a pixel's value is a function of the
// picture and of the pixel's device coordinates only, whatever the target
// and the region: a render of a region gives the pixels of a full render,
// bit for bit.
//
// # Blur
//
// The rasterizer blurs with three box passes per axis (an exact kernel
// below a standard deviation of 2), and from about 21 pixels at a reduced
// resolution (ADR-078). Its pixels are the reference, so the kernels here
// are its own (raster.BlurKernel): three boxes are one kernel, their
// convolution, applied in one pass per axis, the horizontal one into a
// half-float target, the vertical one rounded once to 8 bits.
//
// A pass costs a texel per tap and pixel, where the rasterizer's running
// sums cost the same for any size, so the wide kernels are applied at a
// reduced resolution, from a smaller deviation than the rasterizer's (6
// device pixels: lowSigma): the picture is averaged into blocks of g x g
// pixels aligned to device coordinates, g the largest power of two that
// leaves the blocks a deviation of lowSigma, the blocks are blurred with
// the rasterizer's kernel for blocks (three fractional boxes,
// raster.FracBoxKernel) and interpolated linearly back to the pixel
// centers, as the rasterizer does. Two things differ from its reduced
// blur. The blocks are weighted by a tent of 2g pixels instead of a box of
// g, whose response is zero where frequencies alias, so that fine
// patterns (text, hairlines) do not beat with the block grid at the
// smaller deviations; the kernel's variance is what is left of the three
// boxes' after the tent's and the interpolation's. And the kernel is cut
// where its reach would exceed the 3 sigma (plus the offset) the display
// list promises, which the regions depend on: at most maxTail of its
// weight, renormalized; a block size whose kernel would lose more is not
// used.
//
// Measured without a device on lines of hard edges, single pixels,
// stripes of one to four pixels and noise (TestBlurKernels), against the
// rasterizer's kernels: nothing below a deviation of 6, where the kernels
// are the same; at most 0.6 level from 6 to 21, where the rasterizer blurs
// at full resolution (0.56 at 6, 0.34 at 8, 0.42 at 16); at most 1.4
// level from 21 on, where the rasterizer reduces too and its mean of
// blocks lets the finest stripes alias (the tent does not: against three
// boxes at full resolution the passes here are the closer of the two).
// Rendered, half of the blurred pixels differ by a level, the rounding of
// two different sums, and a color matrix after a blur scales that
// (saturate(1.6): two levels on about a tenth of them); TestCompare's
// scenes of blurs measure it on every device.
//
// The color matrices that follow a blur or a drop shadow are applied by
// its last step, and a backdrop filter's first step reads the surface
// itself, mirrored by the shader: a glass panel is the blocks along x,
// the small passes on blocks, and one pass at full resolution, then its
// paint.
//
// perf: the first step of a reduced blur reads 2g texels per block along
// x over every row (two texels per pixel of the picture); bilinear
// fetches would halve it, but their weights are the device's, not data,
// and the pixels of a region would no longer be those of a full render
// for certain. The last step of a filter and the paint that composites
// its result are two passes at full resolution; one shader could do both.

const (
	// lowSigma is the smallest standard deviation, in blocks, a blur at a
	// reduced resolution leaves: the linear interpolation between blocks
	// then errs by about half a level on a hard edge.
	lowSigma = 3
	// maxBlock is the largest block of a blur at a reduced resolution.
	maxBlock = 64
	// maxTail is the largest share of a kernel's weight that may be cut
	// to keep it within its reach.
	maxTail = 1e-3
)

// plane is a picture in a texture. Its coordinates are those of the
// passes that drew it; outside rect it is transparent.
type plane struct {
	// from is the texture: an offscreen target's, or the destination's.
	// It is 0 for no picture (a step that did not fit a texture).
	from Source
	// t is the pooled target the picture is in when it is the filters'
	// own to release, else nil.
	t *poolTarget
	// org is the coordinates of the texture's first pixel, and rect holds
	// the picture.
	org  image.Point
	rect image.Rectangle
	// mirror, when not empty, says the picture is the texture's pixels
	// inside mirror, mirrored at its edges up to rect (the edge pixel
	// repeated): a backdrop.
	mirror image.Rectangle
}

// targetPlane returns the picture a pooled target holds over rect, its
// first pixel being rect's.
func targetPlane(t *poolTarget, rect image.Rectangle) plane {
	return plane{from: Source(t.id), t: t, org: rect.Min, rect: rect}
}

// tapRange is a kernel in Frame.Taps.
type tapRange struct{ first, n int }

// blurAxis is the kernel of a blur along one device axis, as the passes
// apply it.
type blurAxis struct {
	// ident: nothing changes along the axis.
	ident bool
	// g is the size of the blocks the axis is reduced to (1: none) and sh
	// the whole pixels the output is shifted by when it is brought back.
	g, sh int
	// pre is the kernel that makes a block (g > 1): offsets from the
	// block's first pixel, plo to phi.
	pre      tapRange
	plo, phi int
	// taps is the kernel, on pixels or on blocks: offsets lo to hi.
	taps   tapRange
	lo, hi int
	// left and right are how far the axis reads before and after a pixel,
	// in device pixels.
	left, right int
}

func (a *blurAxis) support() int { return max(a.left, a.right) }

// frameKernel is a kernel the frame being encoded holds.
type frameKernel struct {
	sigma, off uint64 // float64 bits
	axis       blurAxis
}

// addTaps appends a kernel to the frame: weight w[i] at offset lo+i.
func (e *Encoder) addTaps(lo int, w []float64) tapRange {
	f := &e.frame
	r := tapRange{first: len(f.Taps) / 2, n: len(w)}
	for i, v := range w {
		f.Taps = append(f.Taps, float32(lo+i), float32(v))
	}
	return r
}

// kernel returns the kernel of a Gaussian of device standard deviation
// sigma whose output is shifted by off pixels, with its taps in the frame.
func (e *Encoder) kernel(sigma, off float64) blurAxis {
	sb, ob := math.Float64bits(sigma), math.Float64bits(off)
	for i := range e.kernels {
		if k := &e.kernels[i]; k.sigma == sb && k.off == ob {
			return k.axis
		}
	}
	// perf: the rasterizer's kernel is computed again in every frame that
	// blurs (a convolution of three boxes).
	k := raster.NewBlurKernel(sigma, off)
	a := blurAxis{g: 1}
	switch {
	case k.Identity():
		a.ident = true
	case e.reduced(&a, &k):
	case k.Down > 1:
		// The rasterizer's own blocks: their mean, its kernel.
		g := k.Down
		box := make([]float64, g)
		for i := range box {
			box[i] = 1 / float64(g)
		}
		a.g, a.sh = g, k.Shift
		a.pre, a.plo, a.phi = e.addTaps(0, box), 0, g-1
		a.taps, a.lo, a.hi = e.addTaps(k.Lo, k.W), k.Lo, k.Lo+len(k.W)-1
		a.left, a.right = k.Left, k.Right
	default:
		a.taps, a.lo, a.hi = e.addTaps(k.Lo, k.W), k.Lo, k.Lo+len(k.W)-1
		a.left, a.right = k.Left, k.Right
	}
	e.kernels = append(e.kernels, frameKernel{sigma: sb, off: ob, axis: a})
	return a
}

// reduced makes a the kernel k applied at a reduced resolution, when k is
// three box passes wide enough for one (see the comment at the top of the
// file), and reports whether it did.
func (e *Encoder) reduced(a *blurAxis, k *raster.BlurKernel) bool {
	if !(k.Variance > 0) {
		return false
	}
	g := 1
	for 2*g <= maxBlock && k.Sigma >= lowSigma*float64(2*g) {
		g *= 2
	}
	sh := int(math.Round(k.Offset))
	for ; g >= 2; g /= 2 {
		// In blocks²: what the tent (2g pixels: two boxes of g and one of
		// 2) and the linear interpolation add is taken from the kernel.
		gg := float64(g * g)
		vl := k.Variance/gg - (1.0/3 + 1/(12*gg))
		if !(vl > 0) {
			continue
		}
		w := raster.FracBoxKernel(vl)
		ext := len(w) / 2
		// Output pixel X reads the pixels X - sh - (ext+2)g + 1 to
		// X - sh + (ext+2)g - 1: the two blocks around its center, the
		// kernel's blocks beyond them, the tent's half block beyond those.
		fit := (int(math.Floor(k.Limit))+1-absInt(sh))/g - 2
		if fit < ext {
			if fit < 1 {
				continue
			}
			var tail float64
			for i := 0; i < ext-fit; i++ {
				tail += w[i] + w[len(w)-1-i]
			}
			if tail > maxTail {
				continue
			}
			w = w[ext-fit : ext+fit+1]
			for i := range w {
				w[i] /= 1 - tail
			}
			ext = fit
		}
		tent := make([]float64, 2*g)
		for i := range tent {
			// The distance of the pixel's center to the block's.
			d := math.Abs(float64(i-g/2) + 0.5 - float64(g)/2)
			tent[i] = (float64(g) - d) / gg
		}
		a.g, a.sh = g, sh
		a.pre, a.plo, a.phi = e.addTaps(-g/2, tent), -g/2, 3*g/2-1
		a.taps, a.lo, a.hi = e.addTaps(-ext, w), -ext, ext
		a.left, a.right = max((ext+2)*g-1+sh, 0), max((ext+2)*g-1-sh, 0)
		return true
	}
	return false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// The kinds of devFilter.
const (
	filterBlur = iota
	filterDrop
	filterMatrix
)

// devFilter is a filter prepared for the device space of what it filters.
type devFilter struct {
	kind   uint8
	kx, ky blurAxis
	color  [4]uint8    // a drop shadow's, premultiplied
	m      [20]float32 // a color matrix, row by row
}

// reach returns how far the filter reads along x and y, in device pixels.
func (f *devFilter) reach() (int, int) {
	if f.kind == filterMatrix {
		return 0, 0
	}
	return f.kx.support(), f.ky.support()
}

// filtersReach returns the sum of the reaches of fs.
func filtersReach(fs []devFilter) (rx, ry int) {
	for i := range fs {
		x, y := fs[i].reach()
		rx, ry = rx+x, ry+y
	}
	return rx, ry
}

// prepareFilters appends the device filters of fs under the transform m to
// Encoder.dfs (raster's prepareFilters).
func (e *Encoder) prepareFilters(fs []gfx.Filter, m *gfx.Affine) {
	for _, f := range fs {
		var df devFilter
		switch f := f.(type) {
		case gfx.Blur:
			sx, sy := raster.DeviceSigmas(*m, float64(f.StdDev))
			df.kind = filterBlur
			df.kx, df.ky = e.kernel(sx, 0), e.kernel(sy, 0)
		case gfx.DropShadow:
			sx, sy := raster.DeviceSigmas(*m, float64(f.StdDev))
			ox, oy := float64(f.Offset.X), float64(f.Offset.Y)
			// Products of float32 values are exact in float64.
			dx := float64(m[0])*ox + float64(m[2])*oy
			dy := float64(m[1])*ox + float64(m[3])*oy
			df.kind = filterDrop
			df.kx, df.ky = e.kernel(sx, dx), e.kernel(sy, dy)
			df.color = premul8(f.Color)
		case gfx.ColorMatrix:
			df.kind = filterMatrix
			df.m = f.M
		}
		e.dfs = append(e.dfs, df)
	}
}

// filterStep is an instance of PipelineFilter being written.
type filterStep struct {
	step, flags uint32
	taps        tapRange
	// The source: the plane read, the stride it is read at and what the
	// taps step by.
	src    plane
	stride image.Point
	axis   [4]int
	// aux is the content a drop shadow goes under, and color its color.
	aux   plane
	color [4]uint8
	// post is the color matrices applied to the result: where they start
	// in Frame.Taps, and how many.
	post tapRange
}

// filter draws the pixels bounds of the target of the pass being encoded
// with a step of PipelineFilter.
func (e *Encoder) filter(bounds image.Rectangle, fs *filterStep) {
	var in instance
	i32 := func(v int) uint32 { return uint32(int32(v)) }
	rect := func(r image.Rectangle) [4]uint32 {
		return [4]uint32{i32(r.Min.X), i32(r.Min.Y), i32(r.Max.X), i32(r.Max.Y)}
	}
	stride := fs.stride
	if stride == (image.Point{}) {
		stride = image.Pt(1, 1)
	}
	flags := fs.flags
	batch := Batch{Pipeline: PipelineFilter, Scissor: bounds, First: len(e.frame.Instances) / InstanceSize, Count: 1, Sources: [3]Source{fs.src.from}}
	mirror := fs.src.mirror
	if !mirror.Empty() {
		flags |= FilterFlagMirror
	}
	if flags&FilterFlagDrop != 0 {
		in[FilterSlotAux] = [4]uint32{i32(-fs.aux.org.X), i32(-fs.aux.org.Y)}
		in[FilterSlotAuxRect] = rect(fs.aux.rect)
		in[FilterSlotColor] = [4]uint32{uint32(fs.color[0]), uint32(fs.color[1]), uint32(fs.color[2]), uint32(fs.color[3])}
		batch.Sources[1] = fs.aux.from
		if !fs.aux.mirror.Empty() {
			// A mirrored source is the picture the filter started from, and
			// so is the content: one rectangle serves both.
			flags |= FilterFlagMirrorAux
			mirror = fs.aux.mirror
		}
	}
	in.floats(SlotBounds, float64(bounds.Min.X), float64(bounds.Min.Y), float64(bounds.Max.X), float64(bounds.Max.Y))
	in[FilterSlotHead] = [4]uint32{fs.step, flags, uint32(fs.taps.first), uint32(fs.taps.n)}
	in[FilterSlotSrc] = [4]uint32{i32(-fs.src.org.X), i32(-fs.src.org.Y), i32(stride.X), i32(stride.Y)}
	in[FilterSlotSrcRect] = rect(fs.src.rect)
	in[FilterSlotAxis] = [4]uint32{i32(fs.axis[0]), i32(fs.axis[1]), i32(fs.axis[2]), i32(fs.axis[3])}
	in[FilterSlotMirror] = [4]uint32{i32(mirror.Min.X), i32(mirror.Min.Y), i32(mirror.Dx()), i32(mirror.Dy())}
	in[FilterSlotPost] = [4]uint32{uint32(fs.post.first), uint32(fs.post.n)}
	e.frame.Instances = in.appendTo(e.frame.Instances)
	e.batches = append(e.batches, batch)
}

// copyPixels draws the pixels bounds of the target of the pass being
// encoded with the pixels at the same coordinates of another target, whose
// pixels rect are valid and whose first pixel is at org.
func (e *Encoder) copyPixels(bounds image.Rectangle, from Source, rect image.Rectangle, org image.Point) {
	e.filter(bounds, &filterStep{step: FilterCopy, flags: FilterFlagRound, src: plane{from: from, org: org, rect: rect}})
}

// newPlane starts a pass into a target of the pool that holds the pixels
// rect and returns it; the plane has no texture, with the reason reported,
// when rect does not fit a texture of the device.
func (e *Encoder) newPlane(rect image.Rectangle, format TargetFormat) plane {
	t, ok := e.acquireTarget(rect.Size(), format)
	if !ok {
		e.unsupported("a filter (a step larger than the device's textures)")
		return plane{}
	}
	e.setTarget(t.id, rect.Min, nil)
	return targetPlane(t, rect)
}

// matrices appends the color matrices of fs (all of that kind) to the
// frame and returns where they are.
func (e *Encoder) matrices(fs []devFilter) tapRange {
	f := &e.frame
	r := tapRange{first: len(f.Taps) / 2, n: len(fs)}
	for i := range fs {
		f.Taps = append(f.Taps, fs[i].m[:]...)
	}
	return r
}

// runFilters applies the filters fs, in order, to the picture src and
// returns the plane that holds the pixels out of the result: src itself
// when nothing changes, else a target the caller releases. Each filter
// computes the pixels the filters after it read (raster's
// applyFiltersOut), and the color matrices that follow a blur or a drop
// shadow are applied by its last step, each on the 8-bit result of what is
// before it, as the rasterizer applies them. The plane has no texture when
// a step did not fit one.
func (e *Encoder) runFilters(src plane, fs []devFilter, out image.Rectangle) plane {
	cur := src
	for k := 0; k < len(fs); {
		// Filter k, if it is not a color matrix, and the matrices after it.
		j := k
		if fs[k].kind != filterMatrix {
			j++
		}
		m := j
		for m < len(fs) && fs[m].kind == filterMatrix {
			m++
		}
		var post tapRange
		if m > j {
			post = e.matrices(fs[j:m])
		}
		need := out
		for i := m; i < len(fs); i++ {
			x, y := fs[i].reach()
			need = growXY(need, x, y)
		}
		need = need.Intersect(src.rect)
		var drop *[4]uint8
		kx, ky := &blurAxis{ident: true, g: 1}, &blurAxis{ident: true, g: 1}
		if j > k {
			kx, ky = &fs[k].kx, &fs[k].ky
			if fs[k].kind == filterDrop {
				drop = &fs[k].color
			}
		}
		next := e.blur(cur, need, kx, ky, drop, post)
		if next.from != cur.from && cur.from != src.from {
			// (src stays the caller's: while it is in use no step can be
			// given its target, so a result in another texture is another
			// picture.)
			e.releaseTarget(cur.t)
		}
		if cur = next; cur.from == 0 {
			break
		}
		k = m
	}
	return cur
}

// blur applies a blur's kernels along x and y to the picture src, then the
// color matrices post, and returns the plane that holds the pixels need of
// the result: src when nothing changes. With drop, the blur's result is a
// drop shadow's: the blurred alpha, in that color, under the picture.
func (e *Encoder) blur(src plane, need image.Rectangle, kx, ky *blurAxis, drop *[4]uint8, post tapRange) plane {
	if kx.ident && ky.ident && drop == nil && post.n == 0 {
		return src
	}
	// last finishes the step that writes the result.
	last := func(fs *filterStep) {
		fs.flags |= FilterFlagRound
		fs.post = post
		if drop != nil {
			fs.flags |= FilterFlagDrop
			fs.aux, fs.color = src, *drop
		}
	}
	cur := src
	// step draws rect of a new plane with fs, which reads cur, and makes
	// it cur.
	step := func(rect image.Rectangle, format TargetFormat, fs *filterStep) bool {
		next := e.newPlane(rect, format)
		if next.from != 0 {
			fs.src = cur
			e.filter(rect, fs)
		}
		if cur.from != src.from {
			e.releaseTarget(cur.t)
		}
		cur = next
		return next.from != 0
	}
	rows := func(r image.Rectangle, y0, y1 int) image.Rectangle {
		return image.Rect(r.Min.X, max(y0, src.rect.Min.Y), r.Max.X, min(y1, src.rect.Max.Y))
	}
	if kx.g == 1 && ky.g == 1 {
		// At full resolution: the kernel along x into a half-float target,
		// over the rows the kernel along y reads, then the kernel along y.
		if !kx.ident && !ky.ident {
			hr := rows(need, need.Min.Y+ky.lo, need.Max.Y+ky.hi)
			if !step(hr, FormatRGBA16F, &filterStep{step: FilterTaps, taps: kx.taps, axis: [4]int{1, 0}}) {
				return cur
			}
		}
		fs := filterStep{step: FilterTaps, taps: ky.taps, axis: [4]int{0, 1}}
		switch {
		case ky.ident && !kx.ident:
			fs.taps, fs.axis = kx.taps, [4]int{1, 0}
		case ky.ident:
			fs.step = FilterCopy // a drop shadow without a blur, or only matrices
		}
		last(&fs)
		step(need, FormatRGBA8, &fs)
		return cur
	}

	// At a reduced resolution along one axis or both (an axis that is not
	// reduced has blocks of one pixel). The blocks the output samples,
	// then those the kernels read.
	bx0, bx1 := sampled(need.Min.X, need.Max.X, kx)
	by0, by1 := sampled(need.Min.Y, need.Max.Y, ky)
	low := image.Rect(bx0+kx.lo, by0+ky.lo, bx1+kx.hi, by1+ky.hi)
	if kx.g > 1 {
		// Blocks along x, over the pixel rows the blocks along y read.
		y0, y1 := low.Min.Y, low.Max.Y
		if ky.g > 1 {
			y0, y1 = low.Min.Y*ky.g+ky.plo, (low.Max.Y-1)*ky.g+ky.phi+1
		}
		r := rows(low, y0, y1)
		if r.Empty() {
			r = image.Rect(low.Min.X, y0, low.Max.X, y0+1) // nothing to read: a transparent row
		}
		if !step(r, FormatRGBA16F, &filterStep{step: FilterTaps, taps: kx.pre, stride: image.Pt(kx.g, 1), axis: [4]int{1, 0}}) {
			return cur
		}
	}
	if ky.g > 1 {
		if !step(low, FormatRGBA16F, &filterStep{step: FilterTaps, taps: ky.pre, stride: image.Pt(1, ky.g), axis: [4]int{0, 1}}) {
			return cur
		}
	}
	if !kx.ident {
		r := image.Rect(bx0, max(low.Min.Y, cur.rect.Min.Y), bx1, min(low.Max.Y, cur.rect.Max.Y))
		if r.Empty() {
			r = image.Rect(bx0, low.Min.Y, bx1, low.Min.Y+1)
		}
		if !step(r, FormatRGBA16F, &filterStep{step: FilterTaps, taps: kx.taps, axis: [4]int{1, 0}}) {
			return cur
		}
	}
	if !ky.ident {
		if !step(image.Rect(bx0, by0, bx1, by1), FormatRGBA16F, &filterStep{step: FilterTaps, taps: ky.taps, axis: [4]int{0, 1}}) {
			return cur
		}
	}
	fs := filterStep{step: FilterUp, axis: [4]int{kx.g, ky.g, kx.sh, ky.sh}}
	if kx.g == 1 {
		fs.axis[2] = 0 // its shift is in its taps
	}
	if ky.g == 1 {
		fs.axis[3] = 0
	}
	last(&fs)
	step(need, FormatRGBA8, &fs)
	return cur
}

// sampled returns the blocks that the output pixels x0 to x1-1 of an axis
// sample when they are interpolated (raster's blurDown): pixel X, with
// t = 2(X - sh) + 1 - g, takes the blocks floor(t / 2g) and the next.
func sampled(x0, x1 int, k *blurAxis) (int, int) {
	if k.g == 1 {
		return x0, x1
	}
	g := k.g
	return floorDiv(2*(x0-k.sh)+1-g, 2*g), floorDiv(2*(x1-1-k.sh)+1-g, 2*g) + 2
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// backdropNeeds returns, for a render of region grown to grown for the
// backdrop filters of the frame's list (raster.BackdropRegion), the pixels
// of each filter's output that the render needs, by the filter's order in
// the list: the region, grown by what the filters after it read. The
// rasterizer computes each over all of the grown region; the others are
// pixels nothing exact reads. It returns nil when the outputs cannot be
// told apart (a filter inside a layer with filters, which makes the
// rasterizer render the whole frame).
func backdropNeeds(dl *gfx.DisplayList, scale float32, region, grown, bounds image.Rectangle, needs []image.Rectangle) []image.Rectangle {
	n := dl.NumBackdrops()
	needs = slices.Grow(needs[:0], n)[:n]
	reg := region
	for k := n - 1; k >= 0; k-- {
		needs[k] = reg
		_, _, reach, inFilter := dl.Backdrop(k)
		if !(reach > 0) {
			continue
		}
		rd := raster.BackdropReadRegion(dl, scale, k, reg).Intersect(bounds)
		if rd.Empty() {
			continue
		}
		if inFilter {
			return nil
		}
		reg = reg.Union(rd)
	}
	if reg != grown {
		return nil // not the rasterizer's region: take no shortcut
	}
	return needs
}

// backdropFilter encodes the BackdropFilter at index i of the list being
// encoded (raster's backdropFilter): the pixels of the surface being drawn
// into inside the device bounds of the shape, mirrored beyond them as far
// as the filters reach, are filtered into a target and painted through the
// shape over the surface, whose pass ends for the filters, which read it,
// and goes on for the paint.
func (e *Encoder) backdropFilter(i int, c gfx.BackdropFilter) {
	st := e.top()
	op := raster.Unit8(float64(c.Opacity))
	r := c.Shape.Rect
	in := touchBounds(&st.ctm, [4]float64{float64(r.X0), float64(r.Y0), float64(r.X1), float64(r.Y1)}).Intersect(st.surf)
	if op == 0 || in.Empty() || !in.Overlaps(st.scissor) {
		return
	}
	sh, bounds, ok := shapeFor(&st.ctm, c.Shape)
	if !ok {
		return
	}
	first := len(e.dfs)
	e.prepareFilters(c.Filters, &st.ctm)
	fs := e.dfs[first:]
	defer func() {
		clear(fs)
		e.dfs = e.dfs[:first]
	}()
	rx, ry := filtersReach(fs)
	if rx == 0 && ry == 0 {
		// Filters of one pixel: only the pixels drawn are read.
		in = in.Intersect(st.scissor)
	}
	// Only the pixels drawn are computed, each from the pixels within the
	// filters' reach of it: a window of the backdrop, mirrored at the edges
	// of in. When the render grew its region for the backdrop filters, the
	// pixels of this one's output beyond what the render needs are left
	// out (backdropNeeds).
	out := in.Intersect(st.scissor)
	if e.dl == e.needList && e.needs != nil {
		if k, ok := slices.BinarySearchFunc(e.needIdx, i, func(a, b int) int { return a - b }); ok {
			out = out.Intersect(e.needs[k])
		}
	}
	if out.Empty() || !bounds.Overlaps(out) {
		return
	}
	win := image.Rect(out.Min.X-rx, out.Min.Y-ry, out.Max.X+rx, out.Max.Y+ry).
		Intersect(image.Rect(in.Min.X-rx, in.Min.Y-ry, in.Max.X+rx, in.Max.Y+ry))
	if tl := e.res.limits.MaxTextureSize; win.Dx() > tl || win.Dy() > tl {
		e.unsupported("BackdropFilter (a backdrop larger than the device's textures)")
		return
	}
	below := e.pass
	below.hasClear = false // done by the pass that ends here
	// The backdrop: the surface's pixels inside in, mirrored up to win. The
	// first step of the filters reads it there; when nothing filters it, it
	// is copied, since the paint cannot read the surface it draws into.
	back := plane{from: e.source(), org: below.origin, rect: win, mirror: in}
	res := e.runFilters(back, fs, out)
	if res.from == back.from {
		if res = e.newPlane(out, FormatRGBA8); res.from != 0 {
			e.filter(out, &filterStep{step: FilterCopy, flags: FilterFlagRound, src: back})
		}
	}
	e.setTarget(below.target, below.origin, nil)
	if res.from == 0 {
		return
	}
	// The filtered backdrop, through the shape and the clips, at the
	// command's opacity.
	var inst instance
	quad := e.begin(&inst, bounds)
	var flags uint32
	if sh.local {
		flags |= FlagShapeLocal
	}
	sh.put(&inst, SlotShape, float64(quad.Min.X), float64(quad.Min.Y))
	cs := *st
	cs.scissor = st.scissor.Intersect(out)
	e.emitLayer(&inst, flags, &cs, quad, &layerPaint{src: res, opacity: op, fade: true})
	e.releaseTarget(res.t)
}
