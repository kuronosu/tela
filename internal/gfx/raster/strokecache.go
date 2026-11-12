package raster

import (
	"math"
	"slices"
	"sync"
	"unsafe"

	"github.com/kuronosu/tela/internal/gfx"
)

// Stroke outlines across bands and frames.
//
// A StrokePath is filled as the outline gfx.Path.Stroke computes from the
// path, the stroke style and the flattening tolerance (which follows the
// transform's largest scale). Computing it flattens the path, cuts the
// dashes and offsets every piece, and allocates all of it: an icon set of
// strokes or a dashed border did that again in every band of every frame
// that rasterized it. The outline depends on nothing else, so a Renderer
// keeps the outlines it computed, keyed by that content, for its worker
// Renderers and for the next frames: the pixels are those of an outline
// computed again, bit for bit (the stroker is fusion-safe, ADR-041).
//
// The cache is shared by the bands of a parallel Render (the owner's
// Renderer and its workers), so it has a mutex; an outline is computed
// outside the lock, and two bands that miss at once compute the same one.
// Outlines are read-only once stored. An entry no Render used in the last
// maxIdleFrames frames goes at EndFrame, and Trim empties the cache; a
// Renderer that sees neither (the pooled ones of the package's Render)
// keeps at most strokeCacheBytes of outlines, dropping what the frame does
// not use first.

// strokeCacheBytes bounds the outlines one cache keeps, in bytes of verbs
// and points: far more than a page of icons and dashed borders needs (the
// dashboard's keep about 0.1 MiB), small next to the raster's buffers.
const strokeCacheBytes = 4 << 20

// strokeCache maps stroke inputs to their outlines.
type strokeCache struct {
	mu      sync.Mutex
	entries map[uint64]*strokeEntry // by hash; collisions are chained
	bytes   int                     // verbs and points of the outlines kept
}

// strokeEntry is a stroke's inputs and its outline.
type strokeEntry struct {
	next    *strokeEntry // the next entry with the same hash
	path    gfx.Path     // shares the list's storage (lists are immutable)
	style   gfx.StrokeStyle
	tol     float32
	outline gfx.Path
	size    int
	age     useAge
}

// stroke returns the outline of path stroked with style at tolerance tol,
// from the cache when it has it. A nil cache computes it.
func (c *strokeCache) stroke(path gfx.Path, style gfx.StrokeStyle, tol float32) gfx.Path {
	if c == nil {
		return path.Stroke(style, tol)
	}
	h := strokeHash(path, style, tol)
	c.mu.Lock()
	for e := c.entries[h]; e != nil; e = e.next {
		if e.tol == tol && sameStyle(e.style, style) && samePath(e.path, path) {
			e.age.use()
			out := e.outline
			c.mu.Unlock()
			return out
		}
	}
	c.mu.Unlock()

	out := path.Stroke(style, tol)
	size := len(out.Verbs()) + int(unsafe.Sizeof(gfx.Point{}))*len(out.Points())
	if size > strokeCacheBytes/4 {
		return out // one outline that would crowd out the others
	}
	e := &strokeEntry{path: path, style: style, tol: tol, outline: out, size: size}
	e.age.use()
	c.mu.Lock()
	defer c.mu.Unlock()
	for o := c.entries[h]; o != nil; o = o.next {
		if o.tol == tol && sameStyle(o.style, style) && samePath(o.path, path) {
			return out // another band stored it meanwhile
		}
	}
	if c.bytes+size > strokeCacheBytes {
		c.evict(false)
		if c.bytes+size > strokeCacheBytes {
			c.evict(true)
		}
	}
	if c.entries == nil {
		c.entries = make(map[uint64]*strokeEntry)
	}
	e.next = c.entries[h]
	c.entries[h] = e
	c.bytes += size
	return out
}

// evict drops the entries no Render used since the last EndFrame, or with
// all every entry. The caller holds mu.
func (c *strokeCache) evict(all bool) {
	for h, e := range c.entries {
		var kept *strokeEntry
		for e != nil {
			next := e.next
			if all || !e.age.used {
				c.bytes -= e.size
			} else {
				e.next, kept = kept, e
			}
			e = next
		}
		if kept == nil {
			delete(c.entries, h)
		} else {
			c.entries[h] = kept
		}
	}
}

// endFrame ages the entries (see useAge) and drops those no Render used in
// the last maxIdleFrames frames; with all (Trim), every entry. It must not
// run while a Render uses the cache.
func (c *strokeCache) endFrame(all bool) {
	if len(c.entries) == 0 {
		return
	}
	if all {
		c.entries, c.bytes = nil, 0
		return
	}
	for h, e := range c.entries {
		var kept *strokeEntry
		for e != nil {
			next := e.next
			if e.age.end(false) {
				c.bytes -= e.size
			} else {
				e.next, kept = kept, e
			}
			e = next
		}
		if kept == nil {
			delete(c.entries, h)
		} else {
			c.entries[h] = kept
		}
	}
}

// size returns the bytes the cache keeps, for ScratchBytes.
func (c *strokeCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// strokeHash hashes the inputs of a stroke (FNV-1a over 32-bit words). It
// only picks the chain to search: entries are compared in full.
func strokeHash(p gfx.Path, s gfx.StrokeStyle, tol float32) uint64 {
	const prime = 1099511628211
	h := uint64(14695981039346656037)
	mix := func(v uint32) { h = (h ^ uint64(v)) * prime }
	for _, v := range p.Verbs() {
		mix(uint32(v))
	}
	for _, pt := range p.Points() {
		mix(math.Float32bits(pt.X))
		mix(math.Float32bits(pt.Y))
	}
	mix(math.Float32bits(s.Width))
	mix(math.Float32bits(s.MiterLimit))
	mix(uint32(s.Join))
	mix(uint32(s.Cap))
	for _, d := range s.Dashes {
		mix(math.Float32bits(d))
	}
	mix(math.Float32bits(s.DashOffset))
	mix(math.Float32bits(tol))
	return h
}

// samePath reports whether two paths have the same verbs and the same
// points, bit for bit (-0 and +0 differ, as they may for the stroker).
// Paths that share their storage are equal without a look at it.
func samePath(a, b gfx.Path) bool {
	av, bv := a.Verbs(), b.Verbs()
	ap, bp := a.Points(), b.Points()
	if len(av) != len(bv) || len(ap) != len(bp) {
		return false
	}
	if len(av) == 0 {
		return true
	}
	if unsafe.SliceData(av) == unsafe.SliceData(bv) && unsafe.SliceData(ap) == unsafe.SliceData(bp) {
		return true
	}
	if !slices.Equal(av, bv) {
		return false
	}
	for i := range ap {
		if math.Float32bits(ap[i].X) != math.Float32bits(bp[i].X) || math.Float32bits(ap[i].Y) != math.Float32bits(bp[i].Y) {
			return false
		}
	}
	return true
}

// sameStyle reports whether two stroke styles are equal, bit for bit.
func sameStyle(a, b gfx.StrokeStyle) bool {
	f := math.Float32bits
	if f(a.Width) != f(b.Width) || f(a.MiterLimit) != f(b.MiterLimit) || a.Join != b.Join || a.Cap != b.Cap ||
		f(a.DashOffset) != f(b.DashOffset) || len(a.Dashes) != len(b.Dashes) {
		return false
	}
	for i := range a.Dashes {
		if f(a.Dashes[i]) != f(b.Dashes[i]) {
			return false
		}
	}
	return true
}
