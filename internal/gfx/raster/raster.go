package raster

import (
	"fmt"
	"image"
	"math"
	"runtime"
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
	// Workers is the largest number of goroutines that render the frame,
	// the calling goroutine included. The zero value means
	// runtime.GOMAXPROCS(0); 1 renders on the calling goroutine only. It
	// must not be negative. The pixels do not depend on it: a parallel
	// render is bit-identical to a sequential one. Small regions are
	// rendered sequentially whatever its value (see the package
	// documentation).
	Workers int
	// GlyphCache caches rendered glyphs across frames and Renderers. nil
	// selects a cache shared by the whole package, of
	// DefaultGlyphCacheBytes. The pixels do not depend on the cache.
	GlyphCache *GlyphCache
	// Backdrops, when not nil, keeps the filtered backdrops of the
	// backdrop filters of the list between Renders into the same
	// destination (see BackdropCache): a filter whose output is in it is
	// drawn from there and does not grow the rendered region. The pixels
	// do not depend on it, as long as its owner carries only the filters
	// whose input did not change (BackdropCache.Retain).
	Backdrops *BackdropCache
	// From, when positive, renders only the commands from index From on:
	// the pixels the render starts from are those of the destination (or
	// of Base) as the commands before From left them, which the caller
	// vouches for, and the commands before From are not drawn (the
	// transforms and clips open at From still apply). No layer may be open
	// at From. The backdrop filters before From do not grow the region;
	// the pixels the render reads are those of the region grown for the
	// filters from From on (BackdropCache.RegionFrom), which the base must
	// hold. The pixels of the region are then those of a full render.
	From int
	// Base, when not nil, holds the pixels the render starts from (see
	// From) over the region grown for the backdrop filters from From on;
	// the destination's pixels there are not read, and only the region is
	// written. It must not share pixels with the destination. When nil,
	// the render starts from the destination's pixels.
	Base *image.RGBA
	// Snap, when not nil, receives the pixels of the destination as the
	// commands before Snap.At leave them, over the region grown for the
	// backdrop filters from Snap.At on, the pixels a later render with
	// From = Snap.At over the region reads from its base (see Snapshot).
	Snap *Snapshot
}

// Snapshot holds the pixels of a render just before one of its commands
// (Options.Snap): a list's prefix rendered over a rectangle, from which a
// later render of the same pixels draws only the commands that follow
// (Options.From and Base), when its caller knows the commands before them
// paint the same pixels there.
type Snapshot struct {
	// At is the index of the command before which the pixels are taken.
	At int
	// Pix holds the pixels (premultiplied RGBA, rows of 4*Rect.Dx()
	// bytes) after the Render; its buffer is reused when large enough.
	Pix []uint8
	// Rect is set by the Render: the device pixels held, those of the
	// region grown for the backdrop filters from At on, which are exact
	// there (the pixels of a full render). It is empty when the Render
	// took none: At lies inside a layer, before Options.From or after a
	// region with nothing to draw.
	Rect image.Rectangle
}

// Image returns the pixels of s as an image (sharing Pix), nil when it
// holds none.
func (s *Snapshot) Image() *image.RGBA {
	if s == nil || s.Rect.Empty() {
		return nil
	}
	return &image.RGBA{Pix: s.Pix[:4*s.Rect.Dx()*s.Rect.Dy()], Stride: 4 * s.Rect.Dx(), Rect: s.Rect}
}

// Renderer renders display lists into RGBA images. It owns the scratch
// buffers of the rasterizer (edges, clip masks, layer surfaces, gradient
// ramps) and reuses them between calls, so a long-lived Renderer does not
// allocate in the steady state (StrokePath excepted: gfx.Path.Stroke
// allocates its outline; and glyphs missing from the glyph cache, whose
// masks and sprites are new memory owned by the cache). The owner of a
// Renderer that draws frames calls EndFrame after each one, so that the
// buffers follow what recent frames need instead of the largest frame
// ever drawn (trim.go).
//
// A parallel Render (Options.Workers) uses worker Renderers that this
// Renderer creates on first use and keeps for later frames.
//
// A Renderer is not safe for concurrent use: it must not be used by several
// goroutines at once. Different Renderers may render at the same time, even
// the same DisplayList into disjoint regions of the same image. The zero
// value is ready to use.
type Renderer struct {
	fill  pathFiller
	blit  blitter
	solid solidShader
	grad  gradShader
	img   imageShader
	ramps rampCache
	opq   imageOpacityCache

	dl *gfx.DisplayList // the list being rendered
	// root maps the root space of dl to device space: the display scale,
	// or for a color glyph, the glyph's transform (see glyph.go).
	root gfx.Affine

	ctm      gfx.Affine // current transform: local -> device
	skipping bool       // inside a block that shows nothing
	stack    []savedState
	free     []freeBuf // released mask, layer and filter buffers (trim.go)

	// region is the device rectangle whose pixels must be exact at the
	// current level: the rendered region, grown by the reach of the
	// filters of the enclosing layers. bound is the device bounds of the
	// active clips and layers, without the region. The blitter's clip is
	// their intersection (filter layers also intersect it with their
	// content's bounds, which they keep in bound).
	region, bound image.Rectangle
	// surfBounds is the device rectangle of the current surface (the
	// destination or the innermost layer's buffer), which backdrop
	// filters read.
	surfBounds image.Rectangle

	dfs   []devFilter // device filters of the open filter layers
	kw    []int64     // kernel weights of dfs
	blurS blurScratch
	shd   shadowScratch

	// Path flattening state; emitFn is the method value of emit, created
	// once so that flattening does not allocate.
	flat      gfx.Flattener
	flatM     gfx.Affine
	flatCurve bool     // flattening one curve of flattenCulled
	curve     gfx.Path // scratch path for flattenCulled
	shape     gfx.Path // scratch path for rrects drawn as paths
	emitFn    func(gfx.Verb, gfx.Point)
	visitFn   func(int, gfx.Command)

	maskBlit blitter // writes glyph masks (glyph.go)

	rowMask []uint8   // rrect rows
	rowSub  []float64 // rrect corner areas

	// Text (glyph.go): the glyph cache of the current Render (nil means
	// the package default), the Renderer that draws color glyph sprites,
	// the nesting depth of color glyphs, and the depth every render
	// starts at (0, or for a sprite Renderer, its glyph's depth plus 1).
	glyphs *GlyphCache
	sprite *Renderer

	// strokes is the cache of stroke outlines of the Renderer
	// (strokecache.go) and strk the one the current Render uses: its own,
	// or for a worker, sprite or filter Renderer, its owner's.
	strokes     strokeCache
	strk        *strokeCache
	glyphDepth  int
	spriteDepth int

	// bdc is the cache of filtered backdrops of the current Render
	// (Options.Backdrops) while it renders.
	bdc *BackdropCache
	// from is Options.From of the current Render, and snap its
	// Options.Snap when it takes pixels (nil otherwise); snapped reports
	// that the current render (a band) copied its part of them. target
	// and renderClip are the destination and region of the current render.
	from       int
	snap       *Snapshot
	snapped    bool
	target     *image.RGBA
	renderClip image.Rectangle
	par        *parallel // worker Renderers and the job of a parallel Render
	scratch    []uint8   // the pixels of scratchImage
	scrAge     useAge    // scratch's age in frames (trim.go)
	pool       *workPool // blurPool
	// A band of a phased render (phase.go): the barriers, and the next one
	// this band has to pass.
	ph     *phaser
	phNext int
	// tune overrides defaults for tests: the band split (zero values mean
	// the defaults, see parallel.go), and glyphPaths draws every glyph
	// without the glyph cache, as under rotations.
	tune struct {
		bandRows, minPixels int
		glyphPaths          bool
	}
}

// savedState is the state that a push saves and its pop restores.
type savedState struct {
	ctm      gfx.Affine
	skipping bool
	clip     image.Rectangle // blitter clip
	cm       clipMask        // active clip mask

	ownMask []uint8 // mask buffer created by this push, released at its pop

	region, bound image.Rectangle
	surfBounds    image.Rectangle

	// Layers: the surface below, and this layer's buffer and opacity.
	layer   bool
	pix     []uint8
	stride  int
	origin  image.Point
	surf    []uint8
	opacity uint8
	blend   gfx.BlendMode
	mask    []uint8 // coverage of the layer's mask over comp, or nil
	// comp is the rectangle composited at PopLayer; surfRect the pixels
	// of surf. They are equal for layers without filters. nf is the
	// number of device filters the layer pushed onto Renderer.dfs.
	comp, surfRect image.Rectangle
	nf             int
}

// NewRenderer returns a new Renderer. It is equivalent to new(Renderer).
func NewRenderer() *Renderer { return new(Renderer) }

var pool = sync.Pool{New: func() any { return NewRenderer() }}

// Render renders dl onto dst (compositing over its current content) with a
// Renderer taken from a pool, on up to opts.Workers goroutines. It is safe
// for concurrent use; see Renderer.Render. The pooled Renderers keep their
// worker Renderers, so steady-state frames do not allocate.
func Render(dst *image.RGBA, dl *gfx.DisplayList, opts Options) {
	r := pool.Get().(*Renderer)
	defer pool.Put(r)
	r.Render(dst, dl, opts)
}

// Render renders dl onto dst, compositing over its current content (use
// Clear first for a fresh frame). The root space of dl is scaled by
// opts.Scale, and only the device pixels in opts.Region are written.
//
// With opts.Workers other than 1, Render may split the region into bands
// rendered by several goroutines; it returns when all of them are done.
//
// Render panics if dst is nil, if opts.Scale is negative, NaN or infinite,
// and if opts.Workers is negative. A panic in a worker goroutine (from a
// font, for example) is raised again, with the same value, on the calling
// goroutine once every worker has stopped. A nil dl renders nothing.
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
	if opts.Workers < 0 {
		panic(fmt.Sprintf("raster: Options.Workers must not be negative (0 means GOMAXPROCS), got %d", opts.Workers))
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
	from := max(opts.From, 0)
	if from > 0 && layerOpenAt(dl, from) {
		panic(fmt.Sprintf("raster: Options.From (%d) lies inside a layer", from))
	}
	r.glyphs = opts.GlyphCache
	r.strk = &r.strokes
	r.from = from
	defer func() { r.from, r.snap = 0, nil }()
	src := dst // the pixels the render starts from
	if opts.Base != nil {
		src = opts.Base
	}
	// prepare sets target up to hold the starting pixels over rect (the
	// region, grown for the backdrop filters), and the snapshot to hold the
	// starting pixels over its rectangle, which every band of the render
	// updates when it reaches the snapshot's command (takeSnap).
	prepare := func(target *image.RGBA, rect image.Rectangle, snapRect func() image.Rectangle) {
		if src != target {
			if !rect.In(src.Rect) {
				panic(fmt.Sprintf("raster: Options.Base %v does not hold the pixels the render reads, %v", src.Rect, rect))
			}
			copyRect(target, src, rect)
		}
		if sn := opts.Snap; sn != nil {
			sn.Rect = image.Rectangle{}
			if sn.At < from || layerOpenAt(dl, sn.At) {
				return
			}
			sr := snapRect().Intersect(rect)
			n := 4 * sr.Dx() * sr.Dy()
			if cap(sn.Pix) < n {
				sn.Pix = make([]uint8, n)
			}
			sn.Pix, sn.Rect = sn.Pix[:n], sr
			if !sr.Empty() {
				copyRect(sn.Image(), target, sr)
				r.snap = sn
			}
		}
	}
	if dl.BackdropReach() > 0 {
		// A backdrop filter that reaches beyond a pixel reads pixels
		// painted by earlier commands around its output: render on this
		// goroutine (see backdrop.go) a region grown to contain what the
		// backdrop filters that touch it read (BackdropRegion). When
		// nothing grows, the region is rendered in place; otherwise the
		// grown region is rendered on a scratch copy and the region copied
		// back, so that its pixels are those of a full render and nothing
		// else is touched (the grown region's other pixels need not be).
		//
		// When the filters that spread pixels lie outside layers, the
		// region is rendered in parallel bands that wait for each other at
		// those filters (phase.go); otherwise on this goroutine, with the
		// stages of large blurs split among several (pool.go).
		full := dst.Rect.Intersect(image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit))
		bc := opts.Backdrops
		if bc != nil {
			bc.prepare(dl, s, full)
		}
		grown := backdropRegion(dl, s, clip, full, bc, bc != nil, from)
		target := dst
		if grown != clip {
			target = r.scratchImage(grown)
		}
		prepare(target, grown, func() image.Rectangle {
			// The pixels exact before the snapshot's command: the region
			// grown for the filters from it on (backdropRegion), with the
			// cache's decisions of the grown region above.
			return backdropRegion(dl, s, clip, full, bc, false, opts.Snap.At)
		})
		rendered := false
		if opts.Workers != 1 {
			if bfs, ok := phaseBarriers(dl, from); ok {
				rendered = r.renderPhased(target, dl, s, grown, opts.Workers, bfs, bc)
			}
		}
		if !rendered {
			r.blurS.par = r.blurPool(opts.Workers)
			r.bdc = bc
			r.render(target, dl, gfx.Scale(s, s), grown)
			r.blurS.par, r.bdc = nil, nil
		}
		if target != dst {
			copyRect(dst, target, clip)
		}
		return
	}
	prepare(dst, clip, func() image.Rectangle { return clip })
	if opts.Workers != 1 && r.renderParallel(dst, dl, s, clip, opts.Workers) {
		return
	}
	r.render(dst, dl, gfx.Scale(s, s), clip)
}

// layerOpenAt reports whether a layer of dl is open before its command at
// (a PushLayer before it whose PopLayer is at or after it).
func layerOpenAt(dl *gfx.DisplayList, at int) bool {
	for i := 0; i < at && i < dl.Len(); i++ {
		m := dl.Match(i)
		if m <= i {
			continue // a draw or a pop
		}
		if m < at {
			i = m // closed before at
			continue
		}
		if _, ok := dl.At(i).(gfx.PushLayer); ok {
			return true
		}
	}
	return false
}

// takeSnap copies the pixels of the snapshot inside the region of the
// current render (a band of a parallel render: its rows) from the
// destination of the render, which no layer replaces at the snapshot's
// command (layerOpenAt).
func (r *Renderer) takeSnap() {
	r.snapped = true
	sn := r.snap
	rect := sn.Rect.Intersect(r.renderClip)
	if rect.Empty() {
		return
	}
	t := r.target
	n := 4 * rect.Dx()
	w := 4 * sn.Rect.Dx()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		o := (y-sn.Rect.Min.Y)*w + 4*(rect.Min.X-sn.Rect.Min.X)
		copy(sn.Pix[o:o+n], t.Pix[t.PixOffset(rect.Min.X, y):][:n])
	}
}

// blurPool returns the pool that splits the stages of large blurs among
// workers goroutines (0 means GOMAXPROCS) when this Renderer renders alone,
// or nil for one goroutine.
func (r *Renderer) blurPool(workers int) *workPool {
	if workers == 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers <= 1 {
		return nil
	}
	if r.pool == nil {
		r.pool = new(workPool)
	}
	r.pool.n = workers
	return r.pool
}

// scratchImage returns an image of bounds rect whose pixels are kept by
// the Renderer between calls (with unspecified values).
func (r *Renderer) scratchImage(rect image.Rectangle) *image.RGBA {
	n := 4 * rect.Dx() * rect.Dy()
	r.scrAge.use()
	if cap(r.scratch) < n {
		r.scratch = make([]uint8, n)
	}
	return &image.RGBA{Pix: r.scratch[:n], Stride: 4 * rect.Dx(), Rect: rect}
}

// render renders dl onto the device pixels clip of dst (clip lies inside
// dst.Rect and the ±coordLimit box and is not empty) on the calling
// goroutine. root maps the root space of dl to device space: the display
// scale, or a positive scale and a translation for color glyph sprites.
func (r *Renderer) render(dst *image.RGBA, dl *gfx.DisplayList, root gfx.Affine, clip image.Rectangle) {
	if r.emitFn == nil {
		r.emitFn = r.emit
		r.visitFn = r.visit
	}
	r.blit.pix, r.blit.stride, r.blit.origin, r.blit.clip = dst.Pix, dst.Stride, dst.Rect.Min, clip
	r.surfBounds = dst.Rect
	r.target, r.renderClip = dst, clip
	r.snapped = r.snap == nil
	r.blit.cm = clipMask{}
	r.blit.toMask = false
	r.dl = dl
	r.root = root
	r.ctm = root
	r.glyphDepth = r.spriteDepth
	r.skipping = false
	r.stack = r.stack[:0]
	r.region, r.bound = clip, image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit)
	r.dfs, r.kw = r.dfs[:0], r.kw[:0]

	// The clip in root space, grown by one device pixel so that float32
	// rounding in the list's bounds can never cull a command that touches
	// a pixel of the region.
	sx, sy, tx, ty := float64(root[0]), float64(root[3]), float64(root[4]), float64(root[5])
	rootClip := gfx.Rect{
		X0: float32((float64(clip.Min.X-1) - tx) / sx), Y0: float32((float64(clip.Min.Y-1) - ty) / sy),
		X1: float32((float64(clip.Max.X+1) - tx) / sx), Y1: float32((float64(clip.Max.Y+1) - ty) / sy),
	}
	defer func() {
		// Do not retain dst, the list or the paints.
		r.blit.pix, r.blit.sh, r.blit.solid, r.blit.cm, r.blit.a8 = nil, nil, nil, clipMask{}, nil
		r.dl, r.target = nil, nil
		r.grad.ramp, r.img.img = nil, nil
		for i := range r.stack {
			r.stack[i] = savedState{}
		}
		r.stack = r.stack[:0]
		clear(r.dfs)
		r.dfs = r.dfs[:0]
	}()
	r.phNext = 0
	dl.Visit(rootClip, r.visitFn)
	if !r.snapped {
		r.takeSnap() // nothing at or after the snapshot's command was visited
	}
	if r.ph != nil {
		r.ph.pass(r, dl.Len(), nil) // the barriers of filters this band culled
	}
}

// visit executes one command.
func (r *Renderer) visit(i int, c gfx.Command) {
	if !r.snapped && i >= r.snap.At {
		r.takeSnap()
	}
	if i < r.from {
		// Before Options.From: only the transforms and clips open at From
		// apply; the blocks closed before it are skipped.
		switch c.(type) {
		case gfx.PushTransform, gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushLayer:
			if r.dl.Match(i) < r.from {
				r.save()
				r.skipping = true
				return
			}
		case gfx.PopTransform, gfx.PopClip, gfx.PopLayer:
		default:
			return
		}
	}
	if ph := r.ph; ph != nil && r.phNext < len(ph.bfs) && ph.bfs[r.phNext] <= i {
		// A band of a phased render reached a backdrop filter barrier.
		if res, barrier := ph.pass(r, i, c); barrier {
			if res != nil {
				r.drawBackdrop(res.cmd.Shape, res.buf, res.win, res.out, unit8(float64(res.cmd.Opacity)))
			}
			return
		}
	}
	switch c := c.(type) {
	case gfx.FillRect:
		if !r.skipping {
			r.blit.setShader(r.shaderFor(c.Paint))
			r.coverRect(c.Rect)
		}
	case gfx.FillRRect:
		if !r.skipping {
			r.blit.setShader(r.shaderFor(c.Paint))
			r.coverRRect(c.RRect)
		}
	case gfx.FillPath:
		if !r.skipping {
			r.blit.setShader(r.shaderFor(c.Paint))
			r.fillPath(c.Path, c.Rule)
		}
	case gfx.StrokePath:
		if !r.skipping {
			r.strokePath(c)
		}
	case gfx.DrawImage:
		if !r.skipping {
			r.drawImage(c)
		}
	case gfx.DrawGlyphRun:
		if !r.skipping {
			r.drawGlyphRun(c)
		}
	case gfx.PushTransform:
		r.save()
		if !r.skipping {
			next := GridMul(r.ctm, c.Affine) // whole-pixel translations stay on the grid (grid.go)
			if next.IsFinite() && next.Det() != 0 {
				r.ctm = next
			} else {
				// The display scale pushed the transform out of float32
				// range, or it became singular: nothing inside can show.
				r.skipping = true
			}
		}
	case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath:
		r.save()
		if !r.skipping {
			r.pushClip(c)
		}
	case gfx.PushLayer:
		r.save()
		if !r.skipping {
			r.pushLayer(i, c)
		}
	case gfx.PopTransform, gfx.PopClip, gfx.PopLayer:
		r.restore()
	case gfx.DrawShadow:
		if !r.skipping {
			r.drawShadow(c)
		}
	case gfx.BackdropFilter:
		if !r.skipping {
			r.backdropFilter(i, c)
		}
	case gfx.Marker:
		// A placeholder of a composed list: it paints nothing.
	default:
		panic(fmt.Sprintf("raster: %s is not implemented yet", c.Name()))
	}
}

// save pushes the current state; the matching pop restores it.
func (r *Renderer) save() {
	b := &r.blit
	r.stack = append(r.stack, savedState{ctm: r.ctm, skipping: r.skipping, clip: b.clip, cm: b.cm,
		region: r.region, bound: r.bound, surfBounds: r.surfBounds})
}

// restore pops the state saved by the matching push: it composites a layer,
// releases the buffers the push created and restores the transform and the
// clip.
func (r *Renderer) restore() {
	st := &r.stack[len(r.stack)-1]
	b := &r.blit
	if st.layer {
		surf, sr, comp, stride := st.surf, st.surfRect, st.comp, 4*st.surfRect.Dx()
		if st.nf > 0 {
			fs := r.dfs[len(r.dfs)-st.nf:]
			r.applyFilters(surf, sr, fs)
			clear(fs)
			r.dfs = r.dfs[:len(r.dfs)-st.nf]
		}
		b.pix, b.stride, b.origin = st.pix, st.stride, st.origin
		b.clip, b.cm = st.clip, st.cm
		if st.blend == gfx.SrcOver && st.mask == nil {
			for y := comp.Min.Y; y < comp.Max.Y; y++ {
				i := (y-sr.Min.Y)*stride + 4*(comp.Min.X-sr.Min.X)
				b.composite(y, comp.Min.X, surf[i:i+4*comp.Dx()], st.opacity)
			}
		} else {
			w := comp.Dx()
			for y := comp.Min.Y; y < comp.Max.Y; y++ {
				i := (y-sr.Min.Y)*stride + 4*(comp.Min.X-sr.Min.X)
				var m []uint8
				if st.mask != nil {
					m = st.mask[(y-comp.Min.Y)*w : (y-comp.Min.Y+1)*w]
				}
				b.compositeLayer(y, comp.Min.X, surf[i:i+4*w], st.opacity, m, st.blend)
			}
		}
		r.putBuf(st.surf)
		if st.mask != nil {
			r.putBuf(st.mask)
		}
	}
	if st.ownMask != nil {
		r.putBuf(st.ownMask)
	}
	r.ctm, r.skipping = st.ctm, st.skipping
	b.clip, b.cm = st.clip, st.cm
	r.region, r.bound, r.surfBounds = st.region, st.bound, st.surfBounds
	*st = savedState{} // drop references
	r.stack = r.stack[:len(r.stack)-1]
}

// getBuf returns a zeroed buffer of n bytes, reusing a released one when
// possible (the smallest that fits).
func (r *Renderer) getBuf(n int) []uint8 {
	best := -1
	for i := range r.free {
		if c := cap(r.free[i].b); c >= n && (best < 0 || c < cap(r.free[best].b)) {
			best = i
		}
	}
	if best < 0 {
		return make([]uint8, n)
	}
	b := r.free[best].b[:n]
	last := len(r.free) - 1
	r.free[best], r.free[last] = r.free[last], freeBuf{}
	r.free = r.free[:last]
	clear(b)
	return b
}

// putBuf releases a buffer from getBuf. Its age starts again: it was used
// in the current frame.
func (r *Renderer) putBuf(b []uint8) {
	r.free = append(r.free, freeBuf{b: b, age: useAge{used: true}})
}

// strokePath fills the outline of a stroke. The outline is computed in local
// space with a tolerance that keeps the device-space error within
// FlattenTolerance, and filled with NonZero (gfx.Path.Stroke's contract).
func (r *Renderer) strokePath(c gfx.StrokePath) {
	s := r.ctm.MaxScale()
	if !(s > 0) {
		return
	}
	outline := r.strk.stroke(c.Path, c.Style, FlattenTolerance/s)
	if outline.IsEmpty() {
		return
	}
	r.blit.setShader(r.shaderFor(c.Paint))
	r.fillPath(outline, gfx.NonZero)
}

// apply transforms a local point to device space in float64. The products
// of two float32 values are exact in float64, so this result cannot change
// if the compiler fuses them into multiply-adds.
func apply(m *gfx.Affine, p gfx.Point) (float64, float64) {
	x, y := float64(p.X), float64(p.Y)
	return float64(m[0])*x + float64(m[2])*y + float64(m[4]),
		float64(m[1])*x + float64(m[3])*y + float64(m[5])
}

// coverRect composites the coverage of an axis-aligned rectangle in local
// space with the current shader (or writes it to the clip mask being
// built).
func (r *Renderer) coverRect(rect gfx.Rect) {
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
	tol, ok := flattenTolerance(&m)
	if !ok {
		return
	}
	f := &r.fill
	f.reset()
	r.flatM = m
	r.flatCurve = false
	if r.pathNear(p) {
		r.flat.Flatten(p, tol, r.emitFn)
	} else {
		r.flattenCulled(p, tol)
	}
	f.closePath()
	f.rasterize(rule, &r.blit)
}

// flattenTolerance returns the local-space flattening tolerance that keeps
// the device-space error of a path under m within FlattenTolerance. ok is
// false for a degenerate or non-finite m.
func flattenTolerance(m *gfx.Affine) (tol float32, ok bool) {
	// The Frobenius norm bounds the largest stretch of m (and is at most
	// sqrt(2) times it). The squares are exact products.
	a, b, c, d := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	norm := math.Sqrt(a*a + b*b + c*c + d*d)
	if !(norm > 0) || math.IsInf(norm, 0) {
		return 0, false
	}
	return float32(float64(FlattenTolerance) / norm), true
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
			r.flat.Flatten(*q, tol, r.emitFn)
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
