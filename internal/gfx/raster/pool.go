package raster

import "sync"

// workPool runs the stages of a large blur (rows, column strips, block
// rows) on several goroutines. It serves the goroutine that renders alone:
// a list with backdrop filters computed between the phases of a phased
// render (phase.go), or a sequential Render with Options.Workers other
// than 1. Band workers never use one: their bands already run in parallel.
//
// Each stage writes disjoint parts of shared buffers (rows, strips) and
// reads only data that no goroutine writes during the stage, and every
// value is computed with the same integer arithmetic whatever the split, so
// the pixels do not depend on the number of goroutines.
type workPool struct {
	n   int            // goroutines, the caller included
	scr []*blurScratch // the scratch of the goroutines other than the caller

	wg   sync.WaitGroup
	mu   sync.Mutex
	pval any // the first panic of a helper goroutine, under mu
	pset bool
}

// run calls fn over [0, n) split into at most p.n chunks of at least grain
// indices: the first chunk on the calling goroutine with its scratch s, the
// others on new goroutines with scratches of their own (whose pool is nil).
// It returns when every chunk is done; a panic in a chunk is raised again
// on the calling goroutine. A nil pool runs fn(s, 0, n).
func (p *workPool) run(s *blurScratch, n, grain int, fn func(s *blurScratch, lo, hi int)) {
	k := 1
	if p != nil {
		k = min(p.n, n/max(grain, 1))
	}
	if k <= 1 {
		fn(s, 0, n)
		return
	}
	for len(p.scr) < k-1 {
		p.scr = append(p.scr, new(blurScratch))
	}
	p.wg.Add(k - 1)
	for c := 1; c < k; c++ {
		go p.chunk(p.scr[c-1], c*n/k, (c+1)*n/k, fn)
	}
	fn(s, 0, n/k)
	p.wg.Wait()
	if p.pset {
		v := p.pval
		p.pval, p.pset = nil, false
		panic(v)
	}
}

func (p *workPool) chunk(s *blurScratch, lo, hi int, fn func(s *blurScratch, lo, hi int)) {
	defer p.wg.Done()
	defer func() {
		if v := recover(); v != nil {
			p.mu.Lock()
			if !p.pset {
				p.pval, p.pset = v, true
			}
			p.mu.Unlock()
		}
	}()
	fn(s, lo, hi)
}
