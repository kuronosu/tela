package raster

import "math"

// Scratch memory across frames.
//
// A Renderer keeps the memory it rendered with for the next Render, so that
// steady-state frames do not allocate: the free list of mask, layer and
// filter buffers (getBuf, putBuf), the blur planes and strips, the shadow
// tables, the scratch copy of a grown region (scratchImage), and the same
// for its worker Renderers, the Renderer that computes the filters of a
// phased render, the goroutines of its work pools and its sprite Renderer.
// Without more, all of it stays at the high-water mark of the largest frame
// ever rendered: a command palette's full-window backdrop blur and its
// layers, at scale 2, kept for as long as the window lives.
//
// EndFrame lets that memory follow what recent frames need. Every piece of
// it records whether a Render used it since the previous EndFrame (useAge);
// EndFrame releases what no Render used in the last maxIdleFrames frames,
// and, when the caller expects no frame soon (idle), what the frame that
// ends did not use. A Renderer whose owner never calls EndFrame keeps
// everything, as before. Releasing memory changes no pixel: every buffer
// is either zeroed when handed out (getBuf) or fully written before it is
// read, so a buffer allocated anew holds what a reused one would.

// maxIdleFrames is the number of frames that scratch memory no Render uses
// is kept for: about a quarter of a second at 60 Hz, longer than the gaps
// of an interaction (a buffer needed every few frames is not reallocated),
// short enough that what a dialog or a palette needed goes soon after it
// closes, before the window even settles.
const maxIdleFrames = 15

// useAge tracks the use of a piece of scratch memory across frames.
type useAge struct {
	used bool   // used since the last EndFrame
	idle uint16 // frames ended since its last use
}

// use records that a Render uses the memory.
func (a *useAge) use() { a.used = true }

// end ends a frame and reports whether the memory should be released: no
// Render used it in the last maxIdleFrames frames, or, when idle, in the
// frame that ends.
func (a *useAge) end(idle bool) bool {
	if a.used {
		a.used, a.idle = false, 0
		return false
	}
	if a.idle < math.MaxUint16 {
		a.idle++
	}
	return idle || a.idle > maxIdleFrames
}

// freeBuf is a released buffer of the free list. Every buffer a Render
// takes is released before the Render returns, so the free list holds all
// of them between Renders.
type freeBuf struct {
	b   []uint8
	age useAge
}

// fitsBuf reports whether a released buffer of capacity c may serve a
// request of n bytes: it must hold them, and waste at most as much as it
// uses, beyond a small slack. A buffer much larger than the request (a
// window-sized layer serving a 100x100 mask) would stay alive for it and
// keep its whole size in use; a new buffer of n bytes is allocated instead,
// and the large one is released when no frame needs it any more.
func fitsBuf(c, n int) bool {
	return c >= n && c-n <= max(n, bufSlack)
}

// bufSlack is the waste a released buffer may always have when it serves a
// request: small masks share buffers of any size up to it.
const bufSlack = 64 << 10

// EndFrame ends a frame: the Renders of r since the previous EndFrame. It
// releases the scratch memory of r and of the Renderers r owns (its
// workers, see Options.Workers) that no Render used in the last few frames,
// and with idle, when the caller expects no frame soon (nothing animates
// and nothing is pending), the memory the frame that ends did not use, so
// that what a large frame needed is not kept while the window rests. The
// next Render allocates again what it needs. The pixels do not depend on
// it.
//
// A frame's owner (core's RasterizeFrame) calls it once per frame, after
// the frame's Renders; a Renderer that never sees EndFrame keeps all its
// memory for reuse. It must not be called while r renders.
func (r *Renderer) EndFrame(idle bool) {
	kept := r.free[:0]
	for _, f := range r.free {
		if !f.age.end(idle) {
			kept = append(kept, f)
		}
	}
	clear(r.free[len(kept):])
	r.free = kept
	if r.scrAge.end(idle) {
		r.scratch = nil
	}
	if r.blurS.age.end(idle) {
		r.blurS = blurScratch{age: r.blurS.age} // par is nil between Renders
	}
	if r.shd.age.end(idle) {
		r.shd = shadowScratch{age: r.shd.age}
	}
	r.pool.endFrame(idle)
	if p := r.par; p != nil {
		for _, w := range p.workers {
			w.r.EndFrame(idle)
		}
		if p.comp != nil {
			p.comp.EndFrame(idle)
		}
		p.pool.endFrame(idle)
	}
	if r.sprite != nil {
		r.sprite.EndFrame(idle)
	}
}

// endFrame is EndFrame for the scratches of the pool's goroutines. A nil
// pool is ignored.
func (p *workPool) endFrame(idle bool) {
	if p == nil {
		return
	}
	for _, s := range p.scr {
		if s.age.end(idle) {
			*s = blurScratch{age: s.age}
		}
	}
}

// ScratchBytes returns the bytes of scratch memory r and the Renderers it
// owns keep between Renders: the buffers of the free lists, the blur
// planes, the shadow tables and the scratch copies, for tests and memory
// measurements. It must not be called while r renders.
func (r *Renderer) ScratchBytes() int {
	n := cap(r.scratch) + r.blurS.bytes() + r.shd.bytes()
	for _, f := range r.free {
		n += cap(f.b)
	}
	n += r.pool.bytes()
	if p := r.par; p != nil {
		for _, w := range p.workers {
			n += w.r.ScratchBytes()
		}
		if p.comp != nil {
			n += p.comp.ScratchBytes()
		}
		n += p.pool.bytes()
	}
	if r.sprite != nil {
		n += r.sprite.ScratchBytes()
	}
	return n
}

func (p *workPool) bytes() int {
	if p == nil {
		return 0
	}
	n := 0
	for _, s := range p.scr {
		n += s.bytes()
	}
	return n
}

// bytes returns the memory of the buffers of s.
func (s *blurScratch) bytes() int {
	return 2*(cap(s.a)+cap(s.b)+cap(s.c)+cap(s.pad)+cap(s.zeroRow)+cap(s.low)) +
		4*(cap(s.acc)+cap(s.dacc)+cap(s.ux)+cap(s.wx)) + 8*(cap(s.h0)+cap(s.h1))
}

// bytes returns the memory of the arenas and rows of sc (the evaluators'
// own tables are small).
func (sc *shadowScratch) bytes() int {
	return 8*(cap(sc.f)+cap(sc.vf)+cap(sc.kw)) + 4*cap(sc.i32) + cap(sc.want) + cap(sc.cov)
}
