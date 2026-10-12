// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/tree/cache.rs); see THIRD_PARTY.md.

package layout

import "math"

// cacheSize is the number of size measurements cached per node.
const cacheSize = 9

// cacheKey identifies a layout request. Unlike Taffy, the key holds the
// whole input (both parent size axes and the sizing mode): a cached result
// is only reused for an identical request.
type cacheKey struct {
	known         [2]uint32 // float bits of known dimensions or available space
	kind          [2]uint8  // what known holds per axis: see mixedKind
	parent        [2]uint32 // float bits of the parent size (+Inf bits: unknown)
	axis          requestedAxis
	sizing        sizingMode
	knownDefinite [2]bool // normalized: an unknown dimension is definite
}

var infBits = math.Float32bits(float32(math.Inf(1)))

// optBits encodes an optional float, unset as +Inf.
func optBits(o opt) uint32 {
	if o.ok {
		return math.Float32bits(o.v)
	}
	return infBits
}

// mixedBits encodes a known dimension, or the available space when the
// dimension is unknown: its value when definite, nothing (zero) for
// min-content and max-content. mixedKind tells them apart: definite
// available space can be negative (a narrow box with large padding), so
// the value alone cannot.
func mixedBits(known opt, avail AvailableSpace) uint32 {
	if known.ok {
		return math.Float32bits(known.v)
	}
	if avail.Kind == Definite {
		return math.Float32bits(avail.Value)
	}
	return 0
}

// mixedKind is 0 for a known dimension and 1 + the kind of the available
// space otherwise.
func mixedKind(known opt, avail AvailableSpace) uint8 {
	if known.ok {
		return 0
	}
	return 1 + uint8(avail.Kind)
}

func makeKey(in *input) cacheKey {
	return cacheKey{
		known:         [2]uint32{mixedBits(in.known.w, in.avail.w), mixedBits(in.known.h, in.avail.h)},
		kind:          [2]uint8{mixedKind(in.known.w, in.avail.w), mixedKind(in.known.h, in.avail.h)},
		parent:        [2]uint32{optBits(in.parent.w), optBits(in.parent.h)},
		axis:          in.axis,
		sizing:        in.sizing,
		knownDefinite: [2]bool{in.knownDefinite.w || !in.known.w.ok, in.knownDefinite.h || !in.known.h.ok},
	}
}

// sizeValidFor reports whether a measurement computed for k is usable for
// a request with key o: sizes computed for one axis may hold garbage in the
// other axis.
func (k *cacheKey) sizeValidFor(o *cacheKey) bool {
	return k.axis == axisBoth || k.axis == o.axis
}

// sameRequest reports whether two keys match except for the requested
// axis.
func (k *cacheKey) sameRequest(o *cacheKey) bool {
	return k.known == o.known && k.kind == o.kind && k.parent == o.parent && k.sizing == o.sizing && k.knownDefinite == o.knownDefinite
}

type measureEntry struct {
	key  cacheKey
	size size
	base opt // first baseline
}

// cache holds a node's last full layout result and a few size
// measurements, each keyed by its request.
type cache struct {
	final    output
	finalKey cacheKey
	hasFinal bool
	entries  [cacheSize]measureEntry
	used     [cacheSize]bool
	recent   uint16 // entries used since the eviction cursor last passed them
	next     uint8  // the next entry to consider replacing
	nonEmpty bool
}

// get returns a cached result for in.
func (c *cache) get(in *input) (output, bool) {
	key := makeKey(in)
	switch in.run {
	case performLayout:
		if c.hasFinal && c.finalKey == key {
			return c.final, true
		}
	case computeSize:
		for i := range c.entries {
			e := &c.entries[i]
			if c.used[i] && e.key.sameRequest(&key) && e.key.sizeValidFor(&key) {
				c.recent |= 1 << i
				return output{size: e.size, firstBaseline: e.base}, true
			}
		}
	}
	return output{}, false
}

// store records the result of in.
func (c *cache) store(in *input, out output) {
	key := makeKey(in)
	switch in.run {
	case performLayout:
		c.nonEmpty = true
		c.final, c.finalKey, c.hasFinal = out, key, true
	case computeSize:
		c.nonEmpty = true
		for i := range c.entries {
			if c.used[i] && c.entries[i].key == key {
				c.entries[i].size, c.entries[i].base = out.size, out.firstBaseline
				c.recent |= 1 << i
				return
			}
		}
		// Second-chance eviction: skip entries used since the cursor last
		// passed them.
		for c.recent&(1<<c.next) != 0 {
			c.recent &^= 1 << c.next
			c.next++
			if c.next == cacheSize {
				c.next = 0
			}
		}
		i := c.next
		c.entries[i] = measureEntry{key: key, size: out.size, base: out.firstBaseline}
		c.used[i] = true
		c.recent |= 1 << i
		c.next++
		if c.next == cacheSize {
			c.next = 0
		}
	}
}

// clear empties the cache and reports whether it held anything.
func (c *cache) clear() bool {
	if !c.nonEmpty {
		return false
	}
	*c = cache{}
	return true
}
