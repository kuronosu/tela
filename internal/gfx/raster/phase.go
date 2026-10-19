package raster

import (
	"errors"
	"image"
	"sync"

	"github.com/kuronosu/tela/internal/gfx"
)

// Phased rendering of lists with backdrop filters (ADR-078).
//
// A backdrop filter that spreads pixels (a blur, a drop shadow) reads the
// pixels painted around its output, which other bands paint. When every
// such filter of a list lies outside layers, so that the surface it reads
// is the destination, the list is rendered in parallel bands, one per
// goroutine, that wait for each other at each of these filters (a
// barrier): when every band has rendered the commands before it, the last
// band to arrive computes the filter's output over the whole render region
// (with a workPool, while the other bands wait and nothing writes the
// destination), and then every band draws its part of the output and goes
// on. The filter's output is computed exactly as a sequential render
// computes it (the same window of the same pixels, the same filters), so
// the pixels are those of a sequential render.
//
// A band that culls a filter (its block lies outside the band) passes the
// barrier when it visits a later command or finishes the list. The state
// the filter's output depends on (the transform, the bounds of the clips,
// the surface) does not depend on the band, so the first band that visits
// it records it for the computation; when no band visits it, it draws
// nothing and nothing is computed.

// phaser is the barriers of a phased render. It is shared by the bands;
// its fields are written under mu, except bfs, set before the bands start.
type phaser struct {
	bfs []int // command indices of the barrier filters, in list order

	mu      sync.Mutex
	cond    sync.Cond
	n       int // bands
	arrived []int
	done    []bool
	aborted bool
	res     []phaseResult

	// The computation, run by the last band to arrive with comp (a
	// Renderer no band uses) over dst, the destination, in area.
	comp *Renderer
	pool *workPool
	dst  *image.RGBA
	area image.Rectangle
}

// phaseResult is a barrier filter: the state a band recorded when it
// visited it, and the computed output (buf covers win; out is the output
// rectangle).
type phaseResult struct {
	seen        bool
	cmd         gfx.BackdropFilter
	ctm         gfx.Affine
	bound, surf image.Rectangle

	buf      []uint8
	win, out image.Rectangle
}

// errPhaseAborted is the panic value of a band waiting at a barrier when
// another band panicked; the panic of that band is the one Render raises.
var errPhaseAborted = errors.New("raster: phased render aborted")

// phaseBarriers returns the command indices of the backdrop filters of dl
// that spread pixels, in list order, and whether all of them lie outside
// layers (only then can the list be rendered in phases).
func phaseBarriers(dl *gfx.DisplayList) ([]int, bool) {
	var bfs []int
	k, nb := 0, dl.NumBackdrops()
	next := -1 // the command index of backdrop k
	if nb > 0 {
		next, _, _, _ = dl.Backdrop(0)
	}
	layers := 0 // open layers
	var open []bool
	for i := range dl.Len() {
		if i == next {
			_, _, reach, _ := dl.Backdrop(k)
			if reach > 0 {
				if layers > 0 {
					return nil, false
				}
				bfs = append(bfs, i)
			}
			if k++; k < nb {
				next, _, _, _ = dl.Backdrop(k)
			} else {
				next = -1
			}
			continue
		}
		switch dl.At(i).(type) {
		case gfx.PushLayer:
			open = append(open, true)
			layers++
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushTransform:
			open = append(open, false)
		case gfx.PopLayer, gfx.PopClip, gfx.PopTransform:
			if n := len(open); n > 0 {
				if open[n-1] {
					layers--
				}
				open = open[:n-1]
			}
		}
		if next < 0 && layers == 0 {
			break
		}
	}
	return bfs, true
}

func newPhaser(bfs []int, bands int, comp *Renderer, pool *workPool, dst *image.RGBA, area image.Rectangle) *phaser {
	ph := &phaser{bfs: bfs, n: bands, comp: comp, pool: pool, dst: dst, area: area,
		arrived: make([]int, len(bfs)), done: make([]bool, len(bfs)), res: make([]phaseResult, len(bfs))}
	ph.cond.L = &ph.mu
	return ph
}

// pass makes r, a band, pass the barriers of the filters up to command i
// (all of them when i is the length of the list). When command i is a
// barrier filter, barrier is true and res is its output to draw, nil when
// r does not draw it (skipped) or there is none.
func (ph *phaser) pass(r *Renderer, i int, c gfx.Command) (res *phaseResult, barrier bool) {
	for r.phNext < len(ph.bfs) && ph.bfs[r.phNext] <= i {
		j := r.phNext
		r.phNext++
		var bf *gfx.BackdropFilter
		if ph.bfs[j] == i {
			barrier = true
			if c, ok := c.(gfx.BackdropFilter); ok && !r.skipping {
				bf = &c
			}
		}
		if ph.wait(r, j, bf) && bf != nil {
			res = &ph.res[j]
		}
	}
	return res, barrier
}

// wait records r's state for barrier j (when r draws its filter bf), waits
// until every band has arrived and the filter is computed, and reports
// whether there is an output to draw.
func (ph *phaser) wait(r *Renderer, j int, bf *gfx.BackdropFilter) bool {
	ph.mu.Lock()
	res := &ph.res[j]
	if bf != nil && !res.seen {
		res.seen, res.cmd, res.ctm, res.bound, res.surf = true, *bf, r.ctm, r.bound, r.surfBounds
	}
	ph.arrived[j]++
	if ph.arrived[j] == ph.n {
		ph.mu.Unlock()
		// Every band waits at this barrier: compute alone, with the pool.
		ok := false
		defer func() {
			ph.mu.Lock()
			if ok {
				ph.done[j] = true
			} else {
				ph.aborted = true
			}
			ph.cond.Broadcast()
			ph.mu.Unlock()
		}()
		if res.seen {
			ph.compute(res)
		}
		ok = true
		return res.buf != nil
	}
	for !ph.done[j] && !ph.aborted {
		ph.cond.Wait()
	}
	aborted := ph.aborted
	ph.mu.Unlock()
	if aborted {
		panic(errPhaseAborted)
	}
	return res.buf != nil
}

// abort releases the bands waiting at a barrier: a band panicked.
func (ph *phaser) abort() {
	ph.mu.Lock()
	ph.aborted = true
	ph.cond.Broadcast()
	ph.mu.Unlock()
}

// compute computes the output of a barrier filter over the render area,
// as backdropFilter does for a band.
func (ph *phaser) compute(res *phaseResult) {
	r := ph.comp
	r.ctm = res.ctm
	r.dfs, r.kw = r.dfs[:0], r.kw[:0]
	r.prepareFilters(res.cmd.Filters)
	fs := r.dfs
	defer func() {
		clear(r.dfs)
		r.dfs = r.dfs[:0]
	}()
	in := r.backdropBounds(res.cmd.Shape.Rect).Intersect(res.surf)
	out := in.Intersect(res.bound).Intersect(ph.area)
	if out.Empty() {
		return
	}
	r.blurS.par = ph.pool
	defer func() { r.blurS.par = nil }()
	b := blitter{pix: ph.dst.Pix, stride: ph.dst.Stride, origin: ph.dst.Rect.Min}
	res.buf, res.win, res.out = r.filterBackdrop(&b, in, out, fs)
}

// release returns the buffers of the results to comp.
func (ph *phaser) release() {
	for k := range ph.res {
		if ph.res[k].buf != nil {
			ph.comp.putBuf(ph.res[k].buf)
		}
		ph.res[k] = phaseResult{}
	}
}
