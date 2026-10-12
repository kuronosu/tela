package gfx

import "iter"

// DisplayList is an immutable, flat sequence of drawing commands with
// precomputed bounds. It is produced by Builder.Finish and is safe to read
// from any number of goroutines at once. Nothing in it changes after
// Finish: the Builder copies every mutable input (paths, gradient stops,
// dashes, filters, glyphs), so callers may reuse their buffers. Fonts are
// immutable by contract (see Font).
//
// Consumers must treat the slices inside the commands they get from At, All
// or Visit (path points, stops, dashes, filters) as read-only.
//
// Per-command bounds are conservative rectangles in ROOT space (display list
// coordinates after all transforms, before display scale): the command can
// never paint outside its bounds, but may paint less. Draw commands have the
// geometry, grown by stroke width, shadow blur and so on, clipped by the
// active clip. Push commands (and their matching pop) have the union of
// everything inside the block; for a PushLayer with filters, that union grown
// by the filters' reach and clipped by the clip active at PushLayer, while
// its content is clipped by that clip grown by the reach (filters pull
// content from outside the clip). A rasterizer rounds the bounds out to whole
// pixels (Rect.RoundOut) and skips commands, or whole push/pop blocks, that
// miss the tile being rendered; see Visit.
type DisplayList struct {
	cmds   []Command
	bounds []Rect
	match  []int32   // push -> index of its pop, pop -> index of its push, else -1
	reach  []float32 // PushLayer only: root-space outset of the filters; else 0
	total  Rect
	// backdropReach is the largest root-space distance beyond a BackdropFilter
	// shape from which its filters read pixels.
	backdropReach float32
}

// Len returns the number of commands.
func (d *DisplayList) Len() int { return len(d.cmds) }

// At returns command i.
func (d *DisplayList) At(i int) Command { return d.cmds[i] }

// BoundsAt returns the root-space bounds of command i. For a pop it equals
// the bounds of its push.
func (d *DisplayList) BoundsAt(i int) Rect { return d.bounds[i] }

// Bounds returns the union of the bounds of the top-level commands: the
// region of the root space that the list can paint on. It is empty for an
// empty list. Every command lies inside it, except the content of layers with
// filters, which may extend beyond the layer's bounds by up to the layer's
// reach (see LayerReach): the filters pull it into the layer.
func (d *DisplayList) Bounds() Rect { return d.total }

// Match returns, for a push command, the index of its matching pop, and for a
// pop, the index of its push. It returns -1 for draw commands. A rasterizer
// that culls a push uses it to skip the whole block: continue at Match(i)+1.
func (d *DisplayList) Match(i int) int { return int(d.match[i]) }

// LayerReach returns, for a PushLayer at index i, how far in root space the
// filters of the layer spread content: children that lie up to that
// distance outside a region can still affect it. It is 0 for other
// commands and for layers without filters.
func (d *DisplayList) LayerReach(i int) float32 { return d.reach[i] }

// BackdropReach returns the largest root-space distance beyond the shape of
// any BackdropFilter from which the list reads already-painted pixels. A
// rasterizer that renders a sub-region must make sure content up to this
// distance around the BackdropFilter shapes inside the region has been
// painted, for example by rendering a region enlarged by BackdropReach.
func (d *DisplayList) BackdropReach() float32 { return d.backdropReach }

// All iterates over (index, command) in order.
func (d *DisplayList) All() iter.Seq2[int, Command] {
	return func(yield func(int, Command) bool) {
		for i, c := range d.cmds {
			if !yield(i, c) {
				return
			}
		}
	}
}

// Visit calls fn, in order, for the commands needed to render region (a
// rectangle in root space, for example a tile). It skips draw commands whose
// bounds miss region and whole push/pop blocks whose bounds miss it, and it
// always calls fn for the pop that matches a push it visited, so the calls
// form properly nested push/pop pairs.
//
// Inside a PushLayer with filters, region is widened by the layer's reach for
// the children, because blur and drop-shadow move content into the region
// from outside it. Visit does NOT widen the region for BackdropFilter reads;
// see BackdropReach.
func (d *DisplayList) Visit(region Rect, fn func(i int, c Command)) {
	type saved struct{ r Rect }
	// A fixed backing array keeps typical nesting depths off the heap, so
	// that a rasterizer visiting a list every frame does not allocate.
	var buf [32]saved
	stack := buf[:0]
	r := region
	for i := 0; i < len(d.cmds); i++ {
		c := d.cmds[i]
		if _, ok := pushKind(c); ok {
			if !d.bounds[i].Intersects(r) {
				i = int(d.match[i]) // the loop increment moves past the pop
				continue
			}
			fn(i, c)
			stack = append(stack, saved{r})
			if d.reach[i] > 0 {
				r = r.Outset(d.reach[i])
			}
			continue
		}
		if _, ok := popKind(c); ok {
			fn(i, c)
			r = stack[len(stack)-1].r
			stack = stack[:len(stack)-1]
			continue
		}
		if d.bounds[i].Intersects(r) {
			fn(i, c)
		}
	}
}
