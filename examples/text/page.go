package main

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// A page is the whole showcase: a header and a set of cards laid out in
// columns by simple manual geometry (the layout engine arrives in phase 3).
//
// The paragraphs are shaped once, when the page is built (text.NewLayout);
// arrange only re-breaks them (Layout.BreakLines) when the width changes,
// which is what makes resizing cheap.
type page struct {
	coll   *text.Collection
	head   []block // eyebrow, title and subtitle, drawn on the page itself
	cards  []*card
	paras  []*para // every paragraph, in reading order
	width  float32 // the width arrange was last called with (0: never)
	height float32 // content height at that width
	breaks int     // number of BreakLines calls so far (for tests)
}

// A card is a rounded box with a stack of blocks.
type card struct {
	dark       bool
	blocks     []block
	x, y, w, h float32
}

// A block is a row of paragraphs placed side by side (usually one),
// with mt logical pixels of space above it.
type block struct {
	mt  float32
	row []*para
}

// A para is one text layout with its place on the page. Selection is per
// para.
type para struct {
	lay   *text.Layout
	boxes int     // number of inline boxes in the layout
	fr    float32 // share of the row width, among the row's paragraphs (0 means 1)
	dark  bool    // draws on a dark card: selection and caret colors
	// clip hides what overflows the paragraph's box on the right.
	clip bool
	// icon paints inline box i of the layout in r (page coordinates).
	icon func(b *gfx.Builder, r gfx.Rect, i int)

	// Set by arrange, in page coordinates.
	x, y, w float32
}

// Page geometry, in logical pixels.
const (
	minColumn  = 340 // narrowest card column
	maxColumns = 3
	gap        = 16 // between cards
	cardPadX   = 20
	cardPadY   = 18
	cardRadius = 16
)

// newPage shapes every paragraph of the showcase with the fonts of c.
func newPage(c *text.Collection) *page {
	pg := &page{coll: c}
	pg.head = headerBlocks(c)
	pg.cards = []*card{
		latinCard(c),
		arabicCard(c),
		cjkCard(c),
		codeCard(c),
		emojiCard(c),
		clampCard(c),
		detailsCard(c),
	}
	for _, b := range pg.head {
		pg.paras = append(pg.paras, b.row...)
	}
	for _, cd := range pg.cards {
		for _, b := range cd.blocks {
			for _, p := range b.row {
				p.dark = cd.dark
				pg.paras = append(pg.paras, p)
			}
		}
	}
	return pg
}

// columnsFor returns how many card columns fit in width w and their
// geometry: the outer margin and the width of a column.
func columnsFor(w float32) (cols int, margin, colW float32) {
	margin = 32
	if w < 560 {
		margin = 20
	}
	inner := w - 2*margin
	cols = int((inner + gap) / (minColumn + gap))
	cols = max(1, min(cols, maxColumns))
	colW = float32(math.Floor(float64((inner - float32(cols-1)*gap) / float32(cols))))
	return cols, margin, max(colW, 1)
}

// arrange places the cards for a window w logical pixels wide and breaks
// every paragraph at the width of its box. Calling it again with the same
// width does nothing; with another width it only re-breaks lines, no
// paragraph is shaped again.
func (pg *page) arrange(w float32) {
	if w == pg.width {
		return
	}
	pg.width = w
	cols, margin, colW := columnsFor(w)

	// Header: as wide as the content, readable lines at most.
	contentW := w - 2*margin
	y := margin
	for _, b := range pg.head {
		y += b.mt
		y += pg.placeRow(b, margin, y, min(contentW, 780))
	}
	y += 22

	// Cards go to the shortest column, in order (masonry).
	tops := make([]float32, cols)
	for i := range tops {
		tops[i] = y
	}
	for _, cd := range pg.cards {
		col := 0
		for i, t := range tops {
			if t < tops[col] {
				col = i
			}
		}
		cd.x = margin + float32(col)*(colW+gap)
		cd.y = tops[col]
		cd.w = colW
		inner := colW - 2*cardPadX
		cy := cd.y + cardPadY
		for _, b := range cd.blocks {
			cy += b.mt
			cy += pg.placeRow(b, cd.x+cardPadX, cy, inner)
		}
		cd.h = cy + cardPadY - cd.y
		tops[col] = cd.y + cd.h + gap
	}
	tallest := float32(0)
	for _, t := range tops {
		tallest = max(tallest, t-gap)
	}
	pg.height = tallest + margin
}

// placeRow breaks the paragraphs of b to fit a row of the given width at
// (x, y) and returns the row's height.
func (pg *page) placeRow(b block, x, y, width float32) float32 {
	var total float32
	for _, p := range b.row {
		total += max(p.fr, 0)
		if p.fr <= 0 {
			total++
		}
	}
	const rowGap = 16
	avail := width - rowGap*float32(len(b.row)-1)
	var h float32
	for _, p := range b.row {
		share := p.fr
		if share <= 0 {
			share = 1
		}
		p.x, p.y = x, y
		p.w = float32(math.Floor(float64(avail * share / total)))
		p.lay.BreakLines(p.w)
		pg.breaks++
		h = max(h, p.lay.Height())
		x += p.w + rowGap
	}
	return h
}

// paraAt returns the paragraph under the page point (x, y), or nil.
func (pg *page) paraAt(x, y float32) *para {
	for _, p := range pg.paras {
		if x >= p.x && x < p.x+p.w && y >= p.y && y < p.y+p.lay.Height() {
			return p
		}
	}
	return nil
}

// A selection is the selected text of one paragraph, or a caret.
type selection struct {
	para *para
	sel  text.Selection
}

// Colors of the page (Tailwind CSS v4 palette).
var (
	slate100  = gfx.OKLCH(0.968, 0.007, 247.896, 1)
	slate200  = gfx.OKLCH(0.929, 0.013, 255.508, 1)
	slate300  = gfx.OKLCH(0.869, 0.022, 252.894, 1)
	slate400  = gfx.OKLCH(0.704, 0.04, 256.788, 1)
	slate500  = gfx.OKLCH(0.554, 0.046, 257.417, 1)
	slate600  = gfx.OKLCH(0.446, 0.043, 257.281, 1)
	slate700  = gfx.OKLCH(0.372, 0.044, 257.287, 1)
	slate800  = gfx.OKLCH(0.279, 0.041, 260.031, 1)
	slate900  = gfx.OKLCH(0.208, 0.042, 265.755, 1)
	indigo200 = gfx.OKLCH(0.87, 0.065, 274.039, 1)
	indigo300 = gfx.OKLCH(0.785, 0.115, 274.713, 1)
	indigo400 = gfx.OKLCH(0.673, 0.182, 276.935, 1)
	indigo600 = gfx.OKLCH(0.511, 0.262, 276.966, 1)
	violet300 = gfx.OKLCH(0.811, 0.111, 293.571, 1)
	violet500 = gfx.OKLCH(0.606, 0.25, 292.717, 1)
	emerald3  = gfx.OKLCH(0.845, 0.143, 164.978, 1)
	amber300  = gfx.OKLCH(0.879, 0.169, 91.605, 1)
	sky300    = gfx.OKLCH(0.828, 0.111, 230.318, 1)
	rose300   = gfx.OKLCH(0.81, 0.117, 11.638, 1)
)

func black(a float32) gfx.Color { return gfx.Black.WithAlpha(a) }

// paint draws the page for a window of w x h logical pixels, scrolled by
// scroll, with the given selection (if any). It is what both the window and
// the snapshot call. arrange must have been called with w.
func (pg *page) paint(b *gfx.Builder, w, h, scroll float32, sel selection) {
	b.FillRect(gfx.Rect{X1: w, Y1: h}, slate100)
	b.PushClipRect(gfx.Rect{X1: w, Y1: h})
	b.PushTransform(gfx.Translate(0, -scroll))
	for _, bl := range pg.head {
		for _, p := range bl.row {
			p.paint(b, sel)
		}
	}
	for _, cd := range pg.cards {
		if cd.y-scroll > h || cd.y+cd.h-scroll < 0 {
			continue // not visible
		}
		cd.paint(b, sel)
	}
	b.PopTransform()
	b.PopClip()
}

// paint draws the card's box and its paragraphs.
func (cd *card) paint(b *gfx.Builder, sel selection) {
	box := gfx.NewRRect(gfx.RectXYWH(cd.x, cd.y, cd.w, cd.h), cardRadius)
	if cd.dark {
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(0, 10), Blur: 15, Spread: -3, Color: black(0.1)})
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(0, 4), Blur: 6, Spread: -4, Color: black(0.1)})
		b.FillRRect(box, slate800)
		b.FillRRect(box.Inset(1), slate900)
	} else {
		// shadow-sm plus a hairline ring.
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(0, 1), Blur: 3, Color: black(0.1)})
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(0, 1), Blur: 2, Spread: -1, Color: black(0.1)})
		b.FillRRect(box, slate200)
		b.FillRRect(box.Inset(1), gfx.White)
	}
	for _, bl := range cd.blocks {
		for _, p := range bl.row {
			p.paint(b, sel)
		}
	}
}

// Selection and caret colors.
var (
	selLight   = indigo200.WithAlpha(0.7)
	selDark    = indigo400.WithAlpha(0.45)
	caretLight = indigo600
	caretDark  = gfx.White
)

// paint draws the paragraph: the selection highlight under the glyphs, the
// glyphs, the inline boxes, and the caret of a collapsed selection.
func (p *para) paint(b *gfx.Builder, sel selection) {
	selected := sel.para == p
	if p.clip {
		b.PushClipRect(gfx.Rect{X0: p.x, Y0: p.y, X1: p.x + p.w, Y1: p.y + p.lay.Height()})
		defer b.PopClip()
	}
	if selected && !sel.sel.IsCollapsed() {
		col := selLight
		if p.dark {
			col = selDark
		}
		for _, r := range p.lay.SelectionRects(sel.sel, nil) {
			b.FillRect(r.Translate(p.x, p.y), col)
		}
	}
	p.lay.Paint(b, gfx.Pt(p.x, p.y))
	if p.icon != nil {
		for i := 0; i < p.boxes; i++ {
			if r, ok := p.lay.BoxRect(i); ok {
				p.icon(b, r.Translate(p.x, p.y), i)
			}
		}
	}
	if selected && sel.sel.IsCollapsed() {
		col := caretLight
		if p.dark {
			col = caretDark
		}
		b.FillRect(p.lay.CursorRect(sel.sel.Focus, 1.5).Translate(p.x, p.y), col)
	}
}
