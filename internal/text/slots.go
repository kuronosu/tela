package text

import "sync/atomic"

// slotPageBits sets the size of a page of glyphSlots: 128 slots, 1 KB.
const slotPageBits = 7

const slotPage = 1 << slotPageBits

type slotPageT[T any] [slotPage]atomic.Pointer[T]

// glyphSlots is a lock-free table of one lazily loaded value per glyph of
// a face. Its slots are allocated in pages on first use, so a face pays for
// the glyph ranges it draws rather than for every glyph of the font: a face
// of Noto Color Emoji (about 41,000 glyphs) used for a few emoji holds a
// few pages instead of two 330 KB arrays, and every Collection makes one.
//
// Readers and writers may run concurrently (raster workers load outlines
// in parallel); a page is published with a compare-and-swap, and the loser
// of a race uses the winner's page.
type glyphSlots[T any] struct {
	n     int
	pages []atomic.Pointer[slotPageT[T]]
}

func newGlyphSlots[T any](n int) glyphSlots[T] {
	return glyphSlots[T]{n: n, pages: make([]atomic.Pointer[slotPageT[T]], (n+slotPage-1)>>slotPageBits)}
}

// slot returns the slot of glyph g, or nil when g is out of range.
func (s *glyphSlots[T]) slot(g int) *atomic.Pointer[T] {
	if g < 0 || g >= s.n {
		return nil
	}
	pp := &s.pages[g>>slotPageBits]
	p := pp.Load()
	if p == nil {
		p = new(slotPageT[T])
		if !pp.CompareAndSwap(nil, p) {
			p = pp.Load()
		}
	}
	return &p[g&(slotPage-1)]
}
