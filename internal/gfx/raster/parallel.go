package raster

import (
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx"
)

// Parallel rendering.
//
// A parallel Render splits its device region into full-width horizontal
// bands and renders each band, through the region machinery, with a
// Renderer of its own: the calling Renderer and up to Workers-1 worker
// Renderers, each driven by one goroutine. A band's pixels are exactly the
// pixels a full render gives (see "Regions and tiling" in the package
// documentation), so the frame does not depend on the split or on the
// number of workers. The goroutines claim bands from an atomic counter, top
// to bottom, so that a worker that finishes early takes more bands. See
// "Concurrency" in the package documentation for the ownership argument.

// Defaults of the band split, chosen by benchmark (BenchmarkBands,
// BenchmarkBandOverhead and BenchmarkSmallRegion); see bandRowsFor.
const (
	// defaultMinPixels is the smallest region, in device pixels, that is
	// split. Below it, starting the goroutines costs more than they save.
	defaultMinPixels = 128 * 128
	// minBandRows is the smallest automatic band height: below it the
	// per-band overhead (every band visits the list, flattens the paths and
	// sets up the shadows that cross it) exceeds a third of the work.
	minBandRows = 16
	// bandsPerWorker is the number of bands per worker the automatic band
	// height aims at, for load balance.
	bandsPerWorker = 4
	// reachRowsFactor scales the filter weight of a list (see bandRowsFor)
	// into a band height.
	reachRowsFactor = 8.0 / 3
)

// parallel holds the worker Renderers of a Renderer and the job of the
// parallel Render in progress.
type parallel struct {
	workers []*bandWorker

	// The job. The calling goroutine writes these fields before it starts
	// the workers (the go statement orders the writes before the workers'
	// reads) and clears them after wg.Wait; in between they are read-only.
	dst    *image.RGBA
	dl     *gfx.DisplayList
	scale  float32
	glyphs *GlyphCache
	area   image.Rectangle // the pixels to render
	rows   int             // band height
	nbands int32

	next   atomic.Int32 // the next band to claim
	failed atomic.Bool  // a band panicked: stop claiming bands
	wg     sync.WaitGroup
	ph     *phaser // a phased render: one band per goroutine, with barriers

	// For phased renders: the Renderer that computes the barrier filters
	// and the pool it uses, kept for later frames.
	comp *Renderer
	pool *workPool

	mu   sync.Mutex
	pval any // the first panic value, under mu
	pset bool
}

// bandWorker is a worker Renderer. work is its method value, created once so
// that starting the goroutine (go w.work()) does not allocate.
type bandWorker struct {
	r    Renderer
	p    *parallel
	work func()
	band int // the band of a phased render
}

func (w *bandWorker) run() {
	defer w.p.wg.Done()
	if w.p.ph != nil {
		w.p.phaseBand(&w.r, w.band)
		return
	}
	w.p.bands(&w.r)
}

// renderParallel renders dl into the device pixels clip of dst on up to
// workers goroutines (0 means GOMAXPROCS), the calling one included. It
// reports false, having rendered nothing, if the region is too small to be
// split or a single goroutine would do all the work; the caller then renders
// it sequentially.
func (r *Renderer) renderParallel(dst *image.RGBA, dl *gfx.DisplayList, s float32, clip image.Rectangle, workers int) bool {
	if workers == 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers <= 1 {
		return false
	}
	// Only the pixels the list can paint need work: every command, and so
	// the whole list, paints inside its bounds (pixelBounds adds a margin
	// of one pixel for rounding). Pixels outside are left untouched by a
	// sequential render too.
	tb := dl.Bounds()
	sc := float64(s)
	area := clip.Intersect(pixelBounds(float64(tb.X0)*sc, float64(tb.Y0)*sc, float64(tb.X1)*sc, float64(tb.Y1)*sc))
	// Lists with backdrop filters that reach beyond a pixel never get
	// here: Render draws them on one goroutine (backdrop.go).
	if area.Empty() {
		return false // nothing to paint; the sequential path is as cheap
	}
	minPix := r.tune.minPixels
	if minPix == 0 {
		minPix = defaultMinPixels
	}
	if area.Dx()*area.Dy() < minPix {
		return false
	}
	rows := r.tune.bandRows
	if rows <= 0 {
		rows = bandRowsFor(dl, sc, area, workers)
	}
	nbands := (area.Dy() + rows - 1) / rows
	workers = min(workers, nbands)
	if workers <= 1 {
		return false
	}

	p := r.par
	if p == nil {
		p = new(parallel)
		r.par = p
	}
	for len(p.workers) < workers-1 {
		w := &bandWorker{p: p}
		w.work = w.run
		p.workers = append(p.workers, w)
	}
	p.dst, p.dl, p.scale, p.area, p.rows, p.nbands = dst, dl, s, area, rows, int32(nbands)
	p.glyphs = r.glyphs
	p.next.Store(0)
	p.failed.Store(false)
	p.pval, p.pset = nil, false
	p.wg.Add(workers - 1)
	for _, w := range p.workers[:workers-1] {
		go w.work()
	}
	p.bands(r)
	p.wg.Wait()

	pval, pset := p.pval, p.pset
	// Do not retain dst, the list or the panic value.
	p.dst, p.dl, p.pval, p.pset, p.glyphs = nil, nil, nil, false, nil
	if pset {
		panic(pval)
	}
	return true
}

// bandRowsFor returns the automatic band height for rendering dl over area
// with the given number of workers. The height does not change the pixels,
// only the speed, so this is a heuristic (and its float code need not be
// fusion-safe).
//
// The base height gives about bandsPerWorker bands per worker, so that a
// worker that meets cheap bands takes more of them, and at least
// minBandRows rows. Filter layers make thin bands expensive: every band
// renders the part of a filter layer it crosses grown by the filters' reach
// R above and below, so a layer costs about 1 + 2R/rows times its area. The
// filter weight of the list, the mean device reach of its filter layers
// weighted by the fraction of area they cover, times reachRowsFactor, is
// the height that keeps that overhead near 75% for a layer covering the
// whole area; the height grows to it, but never beyond one band per worker,
// which loses more to idle workers than it saves (measured with full-frame
// blurs of standard deviation 8 and 32).
func bandRowsFor(dl *gfx.DisplayList, scale float64, area image.Rectangle, workers int) int {
	h := area.Dy()
	n := workers * bandsPerWorker
	rows := max((h+n-1)/n, minBandRows)
	var weight float64 // sum of device reach times covered area
	for i := range dl.Len() {
		reach := dl.LayerReach(i)
		if reach <= 0 {
			continue
		}
		bb := dl.BoundsAt(i)
		lb := pixelBounds(float64(bb.X0)*scale, float64(bb.Y0)*scale, float64(bb.X1)*scale, float64(bb.Y1)*scale).Intersect(area)
		weight += float64(reach) * scale * float64(lb.Dx()*lb.Dy())
	}
	if weight > 0 {
		reachRows := int(math.Ceil(reachRowsFactor * weight / float64(area.Dx()*h)))
		rows = max(rows, min(reachRows, (h+workers-1)/workers))
	}
	return rows
}

// bands renders bands of the job with r until none is left. It recovers a
// panic, records it and makes the other goroutines stop claiming bands.
func (p *parallel) bands(r *Renderer) {
	defer func() {
		if v := recover(); v != nil {
			p.fail(v)
		}
	}()
	area := p.area
	r.glyphs = p.glyphs
	root := gfx.Scale(p.scale, p.scale)
	for !p.failed.Load() {
		b := p.next.Add(1) - 1
		if b >= p.nbands {
			return
		}
		y0 := area.Min.Y + int(b)*p.rows
		band := image.Rect(area.Min.X, y0, area.Max.X, min(y0+p.rows, area.Max.Y))
		r.render(p.dst, p.dl, root, band)
	}
}

// fail records the panic of a band and stops the others: no more bands
// are claimed, and the bands waiting at a barrier are released.
func (p *parallel) fail(v any) {
	p.failed.Store(true)
	p.mu.Lock()
	if !p.pset || p.pval == errPhaseAborted {
		p.pval, p.pset = v, true
	}
	p.mu.Unlock()
	if p.ph != nil {
		p.ph.abort()
	}
}

// renderPhased renders dl, whose backdrop filters that spread pixels are
// bfs (all outside layers, see phaseBarriers), into the device pixels clip
// of dst on up to workers goroutines (0 means GOMAXPROCS) in phases (see
// phase.go): one band per goroutine. It reports false, having rendered
// nothing, when a single band would do all the work.
func (r *Renderer) renderPhased(dst *image.RGBA, dl *gfx.DisplayList, s float32, clip image.Rectangle, workers int, bfs []int) bool {
	if workers == 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	tb := dl.Bounds()
	sc := float64(s)
	area := clip.Intersect(pixelBounds(float64(tb.X0)*sc, float64(tb.Y0)*sc, float64(tb.X1)*sc, float64(tb.Y1)*sc))
	minPix := r.tune.minPixels
	if minPix == 0 {
		minPix = defaultMinPixels
	}
	if area.Empty() || area.Dx()*area.Dy() < minPix {
		return false
	}
	rows := max(r.tune.bandRows, minBandRows)
	workers = min(workers, (area.Dy()+rows-1)/rows)
	if workers <= 1 {
		return false
	}
	p := r.par
	if p == nil {
		p = new(parallel)
		r.par = p
	}
	for len(p.workers) < workers-1 {
		w := &bandWorker{p: p}
		w.work = w.run
		p.workers = append(p.workers, w)
	}
	if p.comp == nil {
		p.comp, p.pool = NewRenderer(), new(workPool)
	}
	p.pool.n = workers
	p.dst, p.dl, p.scale, p.area, p.nbands = dst, dl, s, area, int32(workers)
	p.glyphs = r.glyphs
	p.failed.Store(false)
	p.pval, p.pset = nil, false
	p.ph = newPhaser(bfs, workers, p.comp, p.pool, dst, area)
	p.wg.Add(workers - 1)
	for k, w := range p.workers[:workers-1] {
		w.band = k + 1
		go w.work()
	}
	p.phaseBand(r, 0)
	p.wg.Wait()

	pval, pset := p.pval, p.pset
	p.ph.release()
	for _, w := range p.workers[:workers-1] {
		w.r.ph = nil
	}
	r.ph = nil
	p.dst, p.dl, p.pval, p.pset, p.glyphs, p.ph = nil, nil, nil, false, nil, nil
	if pset {
		panic(pval)
	}
	return true
}

// phaseBand renders band k of a phased render with r.
func (p *parallel) phaseBand(r *Renderer, k int) {
	defer func() {
		r.ph = nil
		if v := recover(); v != nil {
			p.fail(v)
		}
	}()
	area, n := p.area, int(p.nbands)
	y0 := area.Min.Y + k*area.Dy()/n
	y1 := area.Min.Y + (k+1)*area.Dy()/n
	r.glyphs = p.glyphs
	r.ph = p.ph
	r.render(p.dst, p.dl, gfx.Scale(p.scale, p.scale), image.Rect(area.Min.X, y0, area.Max.X, y1))
}
