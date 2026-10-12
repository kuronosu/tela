package raster

import (
	"image"
	"sync"

	"github.com/kuronosu/tela/internal/gfx"
)

// DefaultGlyphCacheBytes is the budget of the glyph cache that the package
// shares between all Renders that do not set Options.GlyphCache: 32 MiB,
// over 100,000 masks of 14 px glyphs (about 300 bytes each, overhead
// included) or about 4,000 color glyphs of 48 px.
const DefaultGlyphCacheBytes = 32 << 20

var defaultGlyphCache = NewGlyphCache(DefaultGlyphCacheBytes)

// glyphShards is the number of independently locked parts of a GlyphCache,
// so that the goroutines of a parallel Render rarely wait for each other.
const glyphShards = 16

// glyphEntryOverhead approximates the memory of an entry besides its pixels
// (the entry, its key and its map slot), counted against the budget.
const glyphEntryOverhead = 160

// GlyphCache holds rendered glyphs: 8-bit coverage masks of outline glyphs
// and premultiplied RGBA sprites of color glyphs, at a device size and a
// quantized subpixel offset (see "Text" in the package documentation). It
// is safe for concurrent use: the band workers of a parallel Render and
// any number of Renderers may share one.
//
// The cache is bounded by bytes (pixels plus a fixed overhead per entry)
// and evicts the least recently used entries of each of its shards. Its
// content never changes the pixels: a glyph found in the cache is exactly
// the glyph that would be rendered on a miss.
type GlyphCache struct {
	shards [glyphShards]glyphShard
}

type glyphShard struct {
	mu    sync.Mutex
	m     map[glyphKey]*glyphEntry
	lru   glyphEntry // sentinel: lru.next is the most recently used entry
	bytes int64
	limit int64
	_     [64]byte // keeps the hot fields of neighboring shards apart
}

// glyphKey identifies a rendered glyph. Every input of the rendering is part
// of it, so equal keys mean identical pixels.
type glyphKey struct {
	font   uint64      // gfx.Font.FontKey
	gid    gfx.GlyphID //
	sx, sy uint32      // float32 bits of the device em size along x and y
	tol    uint32      // float32 bits of FlattenTolerance
	sub    uint8       // x offset of the pen in quarters of a device pixel
	kind   uint8       // keyGlyph or keySprite
	// keySprite: the nesting depth of the sprite's color glyph, which
	// decides how deep the color glyphs inside it are drawn (see
	// maxGlyphNesting), and the float32 bits of the foreground color.
	depth uint8
	fg    [4]uint32
}

// Key kinds: a glyph's mask (or the knowledge that it is empty or a color
// glyph), and a color glyph's sprite for one foreground color.
const (
	keyGlyph = iota
	keySprite
)

// Entry kinds.
const (
	entryEmpty  = iota // the glyph paints nothing at this size
	entryMask          // data is an 8-bit coverage mask
	entryColor         // a color glyph: look up its keySprite entry
	entrySprite        // data is premultiplied RGBA
	entryDirect        // too large for a sprite: the glyph is drawn directly
)

// glyphEntry is a rendered glyph. kind, rect and data never change after
// the entry is created, so they may be read without the shard's lock by
// any goroutine that obtained the entry; prev and next belong to the lock.
type glyphEntry struct {
	key        glyphKey
	prev, next *glyphEntry
	kind       uint8
	// rect holds the device pixels of data, relative to the snapped pen
	// position (whole pixels); data is row-major with no padding.
	rect image.Rectangle
	data []uint8
	size int64
}

// NewGlyphCache returns an empty cache that holds about maxBytes bytes. The
// budget is split evenly between the shards; a glyph larger than its shard's
// share is rendered but not kept. A budget of 0 or less caches nothing.
func NewGlyphCache(maxBytes int64) *GlyphCache {
	c := new(GlyphCache)
	per := max(maxBytes, 0) / glyphShards
	for i := range c.shards {
		s := &c.shards[i]
		s.limit = per
		s.m = make(map[glyphKey]*glyphEntry)
		s.lru.next, s.lru.prev = &s.lru, &s.lru
	}
	return c
}

// Len returns the number of cached entries.
func (c *GlyphCache) Len() int {
	n := 0
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		n += len(s.m)
		s.mu.Unlock()
	}
	return n
}

// Bytes returns the memory charged to the budget by the cached entries.
func (c *GlyphCache) Bytes() int64 {
	var n int64
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		n += s.bytes
		s.mu.Unlock()
	}
	return n
}

// Reset removes every entry.
func (c *GlyphCache) Reset() {
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		clear(s.m)
		s.lru.next, s.lru.prev = &s.lru, &s.lru
		s.bytes = 0
		s.mu.Unlock()
	}
}

func (c *GlyphCache) shard(k *glyphKey) *glyphShard {
	h := k.font*0x9e3779b97f4a7c15 ^ uint64(k.gid)*0xc2b2ae3d27d4eb4f ^
		(uint64(k.sx)<<32|uint64(k.sy))*0x165667b19e3779f9 ^ uint64(k.sub)<<56 ^
		(uint64(k.fg[0])<<32|uint64(k.fg[1]))*0x27d4eb2f165667c5 ^ uint64(k.fg[2])<<17 ^ uint64(k.fg[3]) ^
		uint64(k.kind)<<61 ^ uint64(k.depth)<<48
	h ^= h >> 31
	h *= 0x94d049bb133111eb
	h ^= h >> 29
	return &c.shards[h%glyphShards]
}

// get returns the entry for k, or nil, and marks it as recently used.
func (c *GlyphCache) get(k *glyphKey) *glyphEntry {
	s := c.shard(k)
	s.mu.Lock()
	e := s.m[*k]
	if e != nil {
		s.unlink(e)
		s.pushFront(e)
	}
	s.mu.Unlock()
	return e
}

// put adds e (whose key is set) and returns the cached entry for its key:
// e, or an equal entry that another goroutine added first. Least recently
// used entries are evicted to stay within the budget; an entry larger than
// the shard's budget is returned without being kept.
func (c *GlyphCache) put(e *glyphEntry) *glyphEntry {
	e.size = int64(len(e.data)) + glyphEntryOverhead
	s := c.shard(&e.key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.m[e.key]; old != nil {
		s.unlink(old)
		s.pushFront(old)
		return old
	}
	if e.size > s.limit {
		return e
	}
	for s.bytes+e.size > s.limit {
		last := s.lru.prev
		s.unlink(last)
		delete(s.m, last.key)
		s.bytes -= last.size
	}
	s.m[e.key] = e
	s.pushFront(e)
	s.bytes += e.size
	return e
}

func (s *glyphShard) unlink(e *glyphEntry) {
	e.prev.next, e.next.prev = e.next, e.prev
	e.prev, e.next = nil, nil
}

func (s *glyphShard) pushFront(e *glyphEntry) {
	e.prev, e.next = &s.lru, s.lru.next
	s.lru.next.prev = e
	s.lru.next = e
}
