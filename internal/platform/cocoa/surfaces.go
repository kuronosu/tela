package cocoa

import "image"

// The pure half of presenting through IOSurfaces (present_darwin.go,
// ADR-278), tested on every OS: which surface of a window's swap chain
// takes the next frame, and which parts of the retained frame each
// surface still lacks.
//
// Every surface must hold the whole retained frame when it is shown,
// since CoreAnimation shows it whole. A Present changes the rectangles it
// damages in the retained frame, so every surface misses them (stale);
// the surface written next gets its own stale rectangles copied (this
// frame's damage plus what it missed while other surfaces were shown)
// and is clean when shown. A new surface (the first ones, or after a
// resize) misses the whole frame.

// maxSurfaces is how many surfaces a window keeps at most: two swap, a
// third is added only while the window server still uses the one that
// would be written next (IOSurfaceIsInUse), as Firefox's SurfacePoolCA
// adds one when none is free.
const maxSurfaces = 3

// maxStale is how many stale rectangles a surface keeps before they are
// replaced by their bounds.
const maxStale = 16

// chainSurface is one surface of a swap chain.
type chainSurface struct {
	ref   uintptr           // the IOSurfaceRef (owned: +1)
	stale []image.Rectangle // parts of the retained frame it lacks
	shown uint64            // surfaceChain.seq when it was last shown, 0 never
}

// surfaceChain is a window's swap chain of surfaces, all of size.
type surfaceChain struct {
	size  image.Point
	surfs []chainSurface
	front int    // index of the surface the layer shows, -1 for none
	seq   uint64 // the number of surfaces shown so far

	// For tests and logs: the bytes copied into surfaces by the last
	// present and by all of them, and the surfaces created.
	lastCopied, totalCopied int
	created                 int
}

// resize empties the chain for surfaces of a new size and returns the
// surfaces it held, for the caller to release.
func (c *surfaceChain) resize(size image.Point) []uintptr {
	refs := c.refs()
	c.size, c.surfs, c.front = size, nil, -1
	return refs
}

// refs returns every surface of the chain.
func (c *surfaceChain) refs() []uintptr {
	var out []uintptr
	for _, s := range c.surfs {
		out = append(out, s.ref)
	}
	return out
}

// damage records rects as changed in the retained frame: every surface
// lacks them now.
func (c *surfaceChain) damage(rects []image.Rectangle) {
	for i := range c.surfs {
		c.surfs[i].stale = addStale(c.surfs[i].stale, rects)
	}
}

// next returns the surface to write the next frame into, or -1 when a new
// one must be added (add): never the one shown; the most recently shown
// of those the window server no longer uses (busy reports it), which
// lacks the least; while all of them are busy, a new one, or when the
// chain is full the one shown longest ago.
func (c *surfaceChain) next(busy func(ref uintptr) bool) int {
	if len(c.surfs) < 2 {
		return -1
	}
	best, oldest := -1, -1
	for i, s := range c.surfs {
		if i == c.front {
			continue
		}
		if oldest < 0 || s.shown < c.surfs[oldest].shown {
			oldest = i
		}
		if busy(s.ref) {
			continue
		}
		if best < 0 || s.shown > c.surfs[best].shown {
			best = i
		}
	}
	switch {
	case best >= 0:
		return best
	case len(c.surfs) < maxSurfaces:
		return -1
	default:
		return oldest
	}
}

// add adds the new surface ref, which lacks the whole frame, and returns
// its index.
func (c *surfaceChain) add(ref uintptr) int {
	c.surfs = append(c.surfs, chainSurface{ref: ref, stale: []image.Rectangle{{Max: c.size}}})
	c.created++
	return len(c.surfs) - 1
}

// stale returns the rectangles surface i lacks.
func (c *surfaceChain) stale(i int) []image.Rectangle { return c.surfs[i].stale }

// shownAt records that surface i, its stale rectangles copied (copied
// bytes), is now the one shown.
func (c *surfaceChain) shownAt(i, copied int) {
	c.seq++
	s := &c.surfs[i]
	s.stale = s.stale[:0]
	s.shown = c.seq
	c.front = i
	c.lastCopied = copied
	c.totalCopied += copied
}

// addStale adds rects to the stale list of a surface: a rectangle inside
// one already there adds nothing, one that covers others replaces them,
// and past maxStale the list becomes its bounds.
func addStale(list, rects []image.Rectangle) []image.Rectangle {
next:
	for _, r := range rects {
		if r.Empty() {
			continue
		}
		for _, s := range list {
			if r.In(s) {
				continue next
			}
		}
		kept := list[:0]
		for _, s := range list {
			if !s.In(r) {
				kept = append(kept, s)
			}
		}
		list = append(kept, r)
	}
	if len(list) > maxStale {
		var u image.Rectangle
		for _, s := range list {
			u = u.Union(s)
		}
		list = append(list[:0], u)
	}
	return list
}
