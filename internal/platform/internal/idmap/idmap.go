// Package idmap is a persistent map from uint64 keys (accessibility node
// IDs) to values: an update makes a new map that shares all but the
// changed paths with the old one, which stays valid and unchanged. The
// accessibility bridges keep a window's tree and their views of it in
// such maps (ADR-277): the UI thread or a bridge's worker makes the next
// version in time proportional to what changed while assistive technology
// queries read the last one from other goroutines without locks.
//
// The map is a hash array mapped trie (Bagwell's HAMT, as Clojure's
// persistent maps): 32-way nodes indexed by 5 bits of the key's hash at a
// time, with a bitmap of the slots in use. The hash is a bijection of the
// key (splitmix64's finalizer), so two keys never collide: a trie is at
// most 13 levels deep, about 3 for 10,000 keys.
//
// A Builder applies many updates in a row and mutates the nodes it made
// itself in place (Clojure's transients), so that building a map of n
// keys from scratch costs O(n), not O(n log n) copies.
package idmap

import "math/bits"

// Map is a persistent map. The zero Map is empty and ready to use. A Map
// value must not be modified after it is published; Set and Delete return
// new maps. Reading a Map from several goroutines at once is safe.
type Map[V any] struct {
	root *node[V]
	n    int
}

// node is a trie node: the slots in use, in bit order.
type node[V any] struct {
	bitmap uint32
	slots  []slot[V]
	// owner is the Builder that made the node, which may change it in
	// place while it builds; nil once published.
	owner *token
}

// slot is an entry (sub nil) or a subtree.
type slot[V any] struct {
	sub *node[V]
	key uint64
	val V
}

// token identifies a Builder's edits.
type token struct{ _ byte }

const (
	bitsPerLevel = 5
	levelMask    = 1<<bitsPerLevel - 1
)

// hash mixes the key's bits (splitmix64's finalizer, a bijection).
func hash(k uint64) uint64 {
	k ^= k >> 30
	k *= 0xbf58476d1ce4e5b9
	k ^= k >> 27
	k *= 0x94d049bb133111eb
	k ^= k >> 31
	return k
}

// Len returns the number of keys.
func (m Map[V]) Len() int { return m.n }

// Get returns the value of key k and whether it is in the map.
func (m Map[V]) Get(k uint64) (V, bool) {
	h := hash(k)
	n := m.root
	for shift := uint(0); n != nil; shift += bitsPerLevel {
		bit := uint32(1) << ((h >> shift) & levelMask)
		if n.bitmap&bit == 0 {
			break
		}
		s := &n.slots[bits.OnesCount32(n.bitmap&(bit-1))]
		if s.sub != nil {
			n = s.sub
			continue
		}
		if s.key == k {
			return s.val, true
		}
		break
	}
	var zero V
	return zero, false
}

// Has reports whether key k is in the map.
func (m Map[V]) Has(k uint64) bool {
	_, ok := m.Get(k)
	return ok
}

// Set returns the map with key k set to v.
func (m Map[V]) Set(k uint64, v V) Map[V] {
	b := Builder[V]{m: m, tok: &token{}}
	b.Set(k, v)
	return b.Map()
}

// Delete returns the map without key k.
func (m Map[V]) Delete(k uint64) Map[V] {
	b := Builder[V]{m: m, tok: &token{}}
	b.Delete(k)
	return b.Map()
}

// Range calls f for every key and value, in no particular order, until f
// returns false.
func (m Map[V]) Range(f func(k uint64, v V) bool) {
	if m.root != nil {
		m.root.walk(f)
	}
}

func (n *node[V]) walk(f func(k uint64, v V) bool) bool {
	for i := range n.slots {
		s := &n.slots[i]
		if s.sub != nil {
			if !s.sub.walk(f) {
				return false
			}
		} else if !f(s.key, s.val) {
			return false
		}
	}
	return true
}

// Builder makes a new version of a Map through many updates: it copies a
// node of the original map once, the first time an update reaches it, and
// changes the copies in place afterwards. The zero Builder starts from the
// empty map. It must not be used from several goroutines at once.
type Builder[V any] struct {
	m   Map[V]
	tok *token
}

// NewBuilder returns a Builder that starts from m.
func NewBuilder[V any](m Map[V]) *Builder[V] { return &Builder[V]{m: m} }

// Len returns the number of keys of the map being built.
func (b *Builder[V]) Len() int { return b.m.n }

// Get returns the value of key k in the map being built.
func (b *Builder[V]) Get(k uint64) (V, bool) { return b.m.Get(k) }

// Map returns the map built so far. Later updates of the Builder do not
// change it.
func (b *Builder[V]) Map() Map[V] {
	b.tok = nil // the nodes made so far are published
	return b.m
}

// edit returns n, or a copy of it that the Builder owns.
func (b *Builder[V]) edit(n *node[V]) *node[V] {
	if n.owner == b.tok {
		return n
	}
	c := &node[V]{bitmap: n.bitmap, slots: make([]slot[V], len(n.slots), len(n.slots)+1), owner: b.tok}
	copy(c.slots, n.slots)
	return c
}

// Set sets key k to v.
func (b *Builder[V]) Set(k uint64, v V) {
	if b.tok == nil {
		b.tok = &token{}
	}
	var added bool
	b.m.root, added = b.set(b.m.root, hash(k), 0, k, v)
	if added {
		b.m.n++
	}
}

func (b *Builder[V]) set(n *node[V], h uint64, shift uint, k uint64, v V) (*node[V], bool) {
	bit := uint32(1) << ((h >> shift) & levelMask)
	if n == nil {
		return &node[V]{bitmap: bit, slots: []slot[V]{{key: k, val: v}}, owner: b.tok}, true
	}
	i := bits.OnesCount32(n.bitmap & (bit - 1))
	if n.bitmap&bit == 0 {
		n = b.edit(n)
		n.slots = append(n.slots, slot[V]{})
		copy(n.slots[i+1:], n.slots[i:])
		n.slots[i] = slot[V]{key: k, val: v}
		n.bitmap |= bit
		return n, true
	}
	s := n.slots[i]
	switch {
	case s.sub != nil:
		sub, added := b.set(s.sub, h, shift+bitsPerLevel, k, v)
		if sub != s.sub {
			n = b.edit(n)
			n.slots[i].sub = sub
		}
		return n, added
	case s.key == k:
		n = b.edit(n)
		n.slots[i].val = v
		return n, false
	}
	// Another key in this slot: both go one level down.
	sub, _ := b.set(nil, hash(s.key), shift+bitsPerLevel, s.key, s.val)
	sub, _ = b.set(sub, h, shift+bitsPerLevel, k, v)
	n = b.edit(n)
	n.slots[i] = slot[V]{sub: sub}
	return n, true
}

// Delete removes key k.
func (b *Builder[V]) Delete(k uint64) {
	if b.m.root == nil {
		return
	}
	if b.tok == nil {
		b.tok = &token{}
	}
	var removed bool
	b.m.root, removed = b.del(b.m.root, hash(k), 0, k)
	if removed {
		b.m.n--
	}
}

// del removes k below n and returns the node that replaces n (nil when it
// is empty).
func (b *Builder[V]) del(n *node[V], h uint64, shift uint, k uint64) (*node[V], bool) {
	bit := uint32(1) << ((h >> shift) & levelMask)
	if n.bitmap&bit == 0 {
		return n, false
	}
	i := bits.OnesCount32(n.bitmap & (bit - 1))
	s := n.slots[i]
	if s.sub != nil {
		sub, removed := b.del(s.sub, h, shift+bitsPerLevel, k)
		if !removed {
			return n, false
		}
		n = b.edit(n)
		switch {
		case sub == nil:
			n.removeSlot(i, bit)
		case len(sub.slots) == 1 && sub.slots[0].sub == nil:
			n.slots[i] = sub.slots[0] // a lone entry moves up
		default:
			n.slots[i].sub = sub
		}
		if len(n.slots) == 0 {
			return nil, true
		}
		return n, true
	}
	if s.key != k {
		return n, false
	}
	if len(n.slots) == 1 {
		return nil, true
	}
	n = b.edit(n)
	n.removeSlot(i, bit)
	return n, true
}

// removeSlot removes slot i, of bit, from n (owned by the Builder).
func (n *node[V]) removeSlot(i int, bit uint32) {
	copy(n.slots[i:], n.slots[i+1:])
	n.slots[len(n.slots)-1] = slot[V]{}
	n.slots = n.slots[:len(n.slots)-1]
	n.bitmap &^= bit
}
