package gpu

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The atlases, on the Encoder's side: where every picture is, which pages
// exist, what was used when. The pixels themselves are only on the device;
// a frame uploads the pictures that are new (Frame.Data, Pass.Uploads), so
// a frame whose pictures are all there uploads nothing.
//
// A page is packed in shelves: rows as high as the pictures they hold,
// whose heights are rounded up to a few sizes so that pictures of about
// the same height share rows. A shelf fills from the left; the place of a
// picture that is evicted is reused by pictures that fit, a shelf left
// empty starts again, and the shelves at the end of a page are given back
// to it. Pictures are read texel by texel, never filtered, so they need no
// gap between them.
//
// A picture larger than a tile (half a page) is cut into tiles, each an
// entry of its own, drawn by an instance of its own: the atlas holds any
// size, and a huge picture costs only its tiles that are drawn.

// What an atlasKey names.
const (
	keyGlyph = iota + 1 // a tile of a glyph's mask or sprite
	keyCover            // a tile of the coverage of a path
	keyMask             // a tile of a clip mask
)

// atlasKey identifies a picture in an atlas: equal keys mean identical
// pixels.
type atlasKey struct {
	kind   uint8
	glyph  raster.GlyphKey // keyGlyph
	id     uint64          // keyCover: the shape; keyMask: the mask
	tx, ty int32           // the tile
}

// What an entry holds besides pixels.
const (
	tilePixels = iota // pixels in the atlas
	tileEmpty         // every pixel is 0: nothing is drawn
	tileFull          // every pixel is 255: no texture is needed
)

// atlasEntry is a picture in an atlas, or the knowledge that it is uniform
// (which takes no room).
type atlasEntry struct {
	key        atlasKey
	state      uint8
	page       int
	x, y, w, h int
	sh         *shelf
	used       uint64 // the frame that used it last
	idle       uint16 // frames ended since its last use (EndFrame)
}

// span is free room in a shelf.
type span struct{ x, w int }

// shelf is a row of a page.
type shelf struct {
	y, h   int
	cursor int    // the room right of it was never used
	free   []span // the places of evicted entries, by x
	live   int
}

type atlasPage struct {
	shelves []*shelf // by y; they cover the rows up to nextY
	nextY   int
	live    int
}

// atlas packs the pictures of one kind of pixel.
type atlas struct {
	bpp      int // bytes per pixel
	pageSize int
	tile     int // the largest width and height of an entry
	maxPages int
	pages    []*atlasPage
	entries  map[atlasKey]*atlasEntry
	// peak is the largest number of pages since the frame began: a frame
	// may draw with pages a flush has emptied since.
	peak int
}

func (a *atlas) init(bpp, pageSize, maxPages int) {
	*a = atlas{bpp: bpp, pageSize: pageSize, tile: pageSize / 2, maxPages: maxPages, entries: map[atlasKey]*atlasEntry{}}
}

// bucket rounds the height of an entry up to the height of a shelf: steps
// of an eighth, so a shelf wastes at most that much of its height.
func bucket(h int) int {
	q := 1
	for q*16 <= h {
		q *= 2
	}
	return (h + q - 1) / q * q
}

// take finds room for w pixels in the shelf.
func (s *shelf) take(w, pageSize int) (int, bool) {
	for i := range s.free {
		if f := &s.free[i]; f.w >= w {
			x := f.x
			f.x, f.w = f.x+w, f.w-w
			if f.w == 0 {
				s.free = append(s.free[:i], s.free[i+1:]...)
			}
			return x, true
		}
	}
	if s.cursor+w <= pageSize {
		x := s.cursor
		s.cursor += w
		return x, true
	}
	return 0, false
}

// give returns the room of an entry to the shelf.
func (s *shelf) give(x, w int) {
	i := 0
	for i < len(s.free) && s.free[i].x < x {
		i++
	}
	s.free = append(s.free, span{})
	copy(s.free[i+1:], s.free[i:])
	s.free[i] = span{x, w}
	// Merge with the neighbors, then with the room never used.
	if i+1 < len(s.free) && s.free[i].x+s.free[i].w == s.free[i+1].x {
		s.free[i].w += s.free[i+1].w
		s.free = append(s.free[:i+1], s.free[i+2:]...)
	}
	if i > 0 && s.free[i-1].x+s.free[i-1].w == s.free[i].x {
		s.free[i-1].w += s.free[i].w
		s.free = append(s.free[:i], s.free[i+1:]...)
	}
	if n := len(s.free); n > 0 && s.free[n-1].x+s.free[n-1].w == s.cursor {
		s.cursor = s.free[n-1].x
		s.free = s.free[:n-1]
	}
}

// alloc finds room for a picture of w x h pixels (at most a tile each
// way). ok is false when the atlas is full.
//
// perf: a linear search over the shelves, for new entries only.
func (a *atlas) alloc(w, h int) (page, x, y int, sh *shelf, ok bool) {
	bh := bucket(h)
	// A shelf of this height with room.
	for pi, pg := range a.pages {
		for _, s := range pg.shelves {
			if s.h != bh {
				continue
			}
			if x, ok := s.take(w, a.pageSize); ok {
				return pi, x, s.y, s, true
			}
		}
	}
	// A new shelf, or an empty one that is high enough (the lowest such).
	for pi, pg := range a.pages {
		if pg.nextY+bh <= a.pageSize {
			s := &shelf{y: pg.nextY, h: bh, cursor: w}
			pg.nextY += bh
			pg.shelves = append(pg.shelves, s)
			return pi, 0, s.y, s, true
		}
	}
	var best *shelf
	bestPage := 0
	for pi, pg := range a.pages {
		for _, s := range pg.shelves {
			if s.live == 0 && s.h >= bh && (best == nil || s.h < best.h) {
				best, bestPage = s, pi
			}
		}
	}
	if best != nil {
		best.cursor = w
		return bestPage, 0, best.y, best, true
	}
	if len(a.pages) < a.maxPages {
		s := &shelf{h: bh, cursor: w}
		a.pages = append(a.pages, &atlasPage{shelves: []*shelf{s}, nextY: bh})
		a.peak = max(a.peak, len(a.pages))
		return len(a.pages) - 1, 0, 0, s, true
	}
	return 0, 0, 0, nil, false
}

// add makes the entry of key: w x h pixels in the atlas, or with a state
// other than tilePixels, the knowledge that the picture is uniform. It
// returns nil when the atlas is full.
func (a *atlas) add(key atlasKey, state uint8, w, h int, frame uint64) *atlasEntry {
	e := &atlasEntry{key: key, state: state, used: frame}
	if state == tilePixels {
		page, x, y, sh, ok := a.alloc(w, h)
		if !ok {
			return nil
		}
		e.page, e.x, e.y, e.w, e.h, e.sh = page, x, y, w, h, sh
		sh.live++
		a.pages[page].live++
	}
	a.entries[key] = e
	return e
}

// get returns the entry of key, or nil, and marks it as used by the frame.
func (a *atlas) get(key *atlasKey, frame uint64) *atlasEntry {
	e := a.entries[*key]
	if e != nil {
		e.used = frame
	}
	return e
}

// remove evicts an entry.
func (a *atlas) remove(e *atlasEntry) {
	delete(a.entries, e.key)
	s := e.sh
	if s == nil {
		return
	}
	pg := a.pages[e.page]
	s.live--
	pg.live--
	if s.live > 0 {
		s.give(e.x, e.w)
		return
	}
	s.cursor, s.free = 0, nil
	for n := len(pg.shelves); n > 0 && pg.shelves[n-1].live == 0; n-- {
		pg.nextY = pg.shelves[n-1].y
		pg.shelves[n-1] = nil
		pg.shelves = pg.shelves[:n-1]
	}
}

// evict removes the entries no frame used for more than age frames before
// the given one (which never loses its own), and reports whether it removed
// any.
func (a *atlas) evict(frame, age uint64) bool {
	any := false
	for _, e := range a.entries {
		if e.used+age <= frame && e.used != frame {
			a.remove(e)
			any = true
		}
	}
	return any
}

// reset drops every entry; the pages stay.
func (a *atlas) reset() {
	clear(a.entries)
	for _, pg := range a.pages {
		*pg = atlasPage{}
	}
}

// endFrame ages the entries: those no frame used since mark, the frame of
// the previous EndFrame, are idle for one frame more, and those idle for
// more than idleFrames are evicted. Pages left empty at the end go.
func (a *atlas) endFrame(mark uint64) {
	for _, e := range a.entries {
		if e.used > mark {
			e.idle = 0
		} else if e.idle++; e.idle > idleFrames {
			a.remove(e)
		}
	}
	a.shrink()
}

// shrink drops the empty pages at the end.
func (a *atlas) shrink() {
	n := len(a.pages)
	for n > 0 && a.pages[n-1].live == 0 {
		n--
		a.pages[n] = nil
	}
	a.pages = a.pages[:n]
}

// rect returns the pixels of an entry in its page.
func (e *atlasEntry) rect() image.Rectangle {
	return image.Rect(e.x, e.y, e.x+e.w, e.y+e.h)
}
