package text

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"image/png"
	"sort"
	"sync"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/gfx"
)

// Bitmap color glyphs: PNG images in strikes, each drawn for one pixel
// size (pixels per em), from the CBDT/CBLC tables (Noto Color Emoji, the
// emoji font of most Linux distributions) or the sbix table (Apple Color
// Emoji). The tables are parsed when a glyph first needs them; a glyph's
// placement in every strike is found once and kept by the typeface, and
// the decoded images are kept in a bounded cache shared by every face (see
// bitmapCache). Black-and-white and grayscale bitmaps (EBDT, CBDT formats
// other than PNG) and the JPEG and TIFF images of sbix are not drawn: a
// glyph that has only those has no bitmap (it draws its outline, if any).

var (
	tagCBLC   = ot.MustNewTag("CBLC")
	tagCBDT   = ot.MustNewTag("CBDT")
	tagSbix   = ot.MustNewTag("sbix")
	tagPNG    = ot.MustNewTag("png ")
	tagDupe   = ot.MustNewTag("dupe")
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	notBitmap = &bitmapGlyph{}
	noBitmaps = &bitmapTables{}
)

// maxDupes is the length of the chains of sbix 'dupe' references that are
// drawn (go-text follows one more, see goTextSbixHops).
const maxDupes = 8

// bitmapTables are the strikes of a font, in increasing pixel size.
type bitmapTables struct {
	cbdt    []byte
	strikes []bitmapStrike
}

// bitmapStrike is one strike: CBLC index subtables, or the glyph data of
// an sbix strike (one entry per glyph).
type bitmapStrike struct {
	ppemX, ppemY uint16
	ppem         uint16 // max(ppemX, ppemY): what strikes are chosen by
	index        []tables.BitmapSubtable
	sbix         *sbixStrike
}

// bitmapGlyph is what a typeface keeps of the bitmaps of one glyph: the
// strikes that have a PNG image for it, in increasing size, and the union
// of their em-space rectangles (what GlyphBounds returns).
type bitmapGlyph struct {
	bounds  gfx.Rect
	strikes []strikeGlyph
}

// strikeGlyph is the image of a glyph in one strike and where it goes:
// left and top are its edges relative to the pen position (y down), w and
// h its size, all in the strike's pixels; rect is the same in em.
type strikeGlyph struct {
	png       []byte
	left, top int
	w, h      int
	rect      gfx.Rect
	ppem      uint16 // the strike's max(ppemX, ppemY)
}

// hasBitmapTables reports whether the font has color bitmap tables, without
// parsing them.
func (tf *Typeface) hasBitmapTables() bool {
	return len(tf.sbixRaw) > 0 || (len(tf.cblcRaw) > 0 && len(tf.cbdtRaw) > 0)
}

// bitmaps returns the parsed strikes, or nil when the font has none (or
// they cannot be parsed).
func (tf *Typeface) bitmaps() *bitmapTables {
	if !tf.hasBitmapTables() {
		return nil
	}
	tf.bitmapOnce.Do(func() {
		tf.bitmapTabs = parseBitmapTables(tf.cblcRaw, tf.cbdtRaw, tf.sbixRaw, tf.nGlyphs)
		if len(tf.bitmapTabs.strikes) > 0 {
			tf.bitmapGlyphs = newGlyphSlots[bitmapGlyph](tf.nGlyphs)
		}
	})
	if len(tf.bitmapTabs.strikes) == 0 {
		return nil
	}
	return tf.bitmapTabs
}

// parseBitmapTables parses sbix, else CBLC (go-text's order: a font with
// both is drawn from sbix). Strikes without a pixel size are dropped.
func parseBitmapTables(cblc, cbdt, sbix []byte, nGlyphs int) *bitmapTables {
	bt := &bitmapTables{}
	if len(sbix) > 0 {
		if strikes, ok := parseSbixStrikes(sbix, nGlyphs); ok {
			for i := range strikes {
				if s := &strikes[i]; s.ppem > 0 {
					bt.strikes = append(bt.strikes, bitmapStrike{ppemX: s.ppem, ppemY: s.ppem, ppem: s.ppem, sbix: s})
				}
			}
		}
	}
	if len(bt.strikes) == 0 && len(cblc) > 0 && len(cbdt) > 0 {
		if loc, _, err := tables.ParseCBLC(cblc); err == nil {
			bt.cbdt = cbdt
			for i, s := range loc.BitmapSizes {
				if s.PpemX == 0 || s.PpemY == 0 || i >= len(loc.IndexSubTables) {
					continue
				}
				x, y := uint16(s.PpemX), uint16(s.PpemY)
				bt.strikes = append(bt.strikes, bitmapStrike{ppemX: x, ppemY: y, ppem: max(x, y), index: loc.IndexSubTables[i]})
			}
		}
	}
	sort.SliceStable(bt.strikes, func(i, j int) bool { return bt.strikes[i].ppem < bt.strikes[j].ppem })
	if len(bt.strikes) == 0 {
		return noBitmaps
	}
	return bt
}

// bitmapGlyph returns the bitmaps of glyph g, or nil when it has none.
func (tf *Typeface) bitmapGlyph(g gfx.GlyphID) *bitmapGlyph {
	bt := tf.bitmaps()
	if bt == nil {
		return nil
	}
	slot := tf.bitmapGlyphs.slot(int(g))
	if slot == nil {
		return nil
	}
	b := slot.Load()
	if b == nil {
		b = bt.load(tables.GlyphID(g))
		if !slot.CompareAndSwap(nil, b) {
			b = slot.Load()
		}
	}
	if b == notBitmap {
		return nil
	}
	return b
}

// load finds the PNG image of gid in every strike.
func (bt *bitmapTables) load(gid tables.GlyphID) *bitmapGlyph {
	var b bitmapGlyph
	for si := range bt.strikes {
		st := &bt.strikes[si]
		sg, ok := bt.glyph(st, gid)
		if !ok {
			continue
		}
		sg.ppem = st.ppem
		// Correctly rounded divisions of small integers: the same on every
		// machine.
		px, py := float64(st.ppemX), float64(st.ppemY)
		sg.rect = gfx.Rect{
			X0: float32(float64(sg.left) / px), Y0: float32(float64(sg.top) / py),
			X1: float32(float64(sg.left+sg.w) / px), Y1: float32(float64(sg.top+sg.h) / py),
		}
		b.bounds = b.bounds.Union(sg.rect)
		b.strikes = append(b.strikes, sg)
	}
	if len(b.strikes) == 0 {
		return notBitmap
	}
	return &b
}

// glyph returns the PNG image of gid in strike st and its placement.
func (bt *bitmapTables) glyph(st *bitmapStrike, gid tables.GlyphID) (strikeGlyph, bool) {
	var sg strikeGlyph
	if st.sbix != nil {
		d := st.sbix.resolve(gid)
		if d.GraphicType != tagPNG {
			return sg, false // 'jpg ', 'tiff', 'mask' or none
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(d.Data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			return sg, false
		}
		// The offsets place the bottom-left corner of the image, in the
		// strike's pixels with y up (HarfBuzz's and go-text's reading of
		// the sbix specification).
		sg.png, sg.w, sg.h = d.Data, cfg.Width, cfg.Height
		sg.left, sg.top = int(d.OriginOffsetX), -(int(d.OriginOffsetY) + cfg.Height)
		return sg, true
	}
	data, format, m, ok := bt.cbdtData(st, gid)
	if !ok {
		return sg, false
	}
	switch format {
	case 17:
		d, _, err := tables.ParseBitmapData17(data)
		if err != nil {
			return sg, false
		}
		sg.png, m = d.Image, d.SmallGlyphMetrics
	case 18:
		d, _, err := tables.ParseBitmapData18(data)
		if err != nil {
			return sg, false
		}
		sg.png, m = d.Image, d.SmallGlyphMetrics
	case 19:
		d, _, err := tables.ParseBitmapData19(data)
		if err != nil {
			return sg, false
		}
		sg.png = d.Image
	default:
		return sg, false // black-and-white or uncompressed color bitmaps
	}
	if m.Width == 0 || m.Height == 0 || !bytes.HasPrefix(sg.png, pngMagic) {
		return sg, false
	}
	sg.w, sg.h = int(m.Width), int(m.Height)
	sg.left, sg.top = int(m.BearingX), -int(m.BearingY)
	return sg, true
}

// cbdtData returns the CBDT bytes of gid in strike st, their image format
// and, for the formats whose metrics live in the index (19), the metrics.
func (bt *bitmapTables) cbdtData(st *bitmapStrike, gid tables.GlyphID) ([]byte, uint16, tables.SmallGlyphMetrics, bool) {
	var m tables.SmallGlyphMetrics
	for i := range st.index {
		sub := &st.index[i]
		if gid < sub.FirstGlyph || gid > sub.LastGlyph {
			continue
		}
		k := uint64(gid - sub.FirstGlyph)
		var start, end uint64
		switch d := sub.IndexData.(type) {
		case tables.IndexData1:
			if k+1 >= uint64(len(d.SbitOffsets)) {
				return nil, 0, m, false
			}
			start, end = uint64(d.SbitOffsets[k]), uint64(d.SbitOffsets[k+1])
		case tables.IndexData3:
			if k+1 >= uint64(len(d.SbitOffsets)) {
				return nil, 0, m, false
			}
			start, end = uint64(d.SbitOffsets[k]), uint64(d.SbitOffsets[k+1])
		case tables.IndexData2:
			start, end = k*uint64(d.ImageSize), (k+1)*uint64(d.ImageSize)
			m = d.BigMetrics.SmallGlyphMetrics
		case tables.IndexData4:
			found := false
			for j := 0; j+1 < len(d.GlyphArray); j++ {
				if d.GlyphArray[j].GlyphID == gid {
					start, end = uint64(d.GlyphArray[j].SbitOffset), uint64(d.GlyphArray[j+1].SbitOffset)
					found = true
					break
				}
			}
			if !found {
				return nil, 0, m, false
			}
		case tables.IndexData5:
			j := sort.Search(len(d.GlyphIdArray), func(j int) bool { return d.GlyphIdArray[j] >= gid })
			if j == len(d.GlyphIdArray) || d.GlyphIdArray[j] != gid {
				return nil, 0, m, false
			}
			start, end = uint64(j)*uint64(d.ImageSize), uint64(j+1)*uint64(d.ImageSize)
			m = d.BigMetrics.SmallGlyphMetrics
		default:
			return nil, 0, m, false
		}
		base := uint64(sub.ImageDataOffset)
		if start >= end || base+end > uint64(len(bt.cbdt)) {
			return nil, 0, m, false // no image for the glyph, or a bad offset
		}
		return bt.cbdt[base+start : base+end], sub.ImageFormat, m, true
	}
	return nil, 0, m, false
}

// sbixStrike is one strike of an sbix table, read on demand: its glyph
// records are parsed when a glyph is looked up, so that finding the
// strikes touches only their headers and offset arrays (the strikes of
// Apple Color Emoji are most of its 180 MB).
type sbixStrike struct {
	ppem, ppi uint16
	src       []byte // from the strike's header to the end of the table
	n         int    // the glyphs before n have valid records
}

// parseSbixStrikes returns the strikes of an sbix table as
// tables.ParseSbix does, and whether ParseSbix succeeds. Like ParseSbix,
// it stops at the first bad strike: that strike keeps its size and the
// glyphs before the bad record (none when its offset array is short), and
// the strikes after it are empty; a strike with a null offset is empty.
// It checks only offsets, which is all ParseSbix's errors depend on (a
// record must hold its 8-byte header; the image data is not read).
func parseSbixStrikes(raw []byte, nGlyphs int) ([]sbixStrike, bool) {
	if len(raw) < 8 {
		return nil, false
	}
	n := uint64(binary.BigEndian.Uint32(raw[4:]))
	if uint64(len(raw)) < 8+4*n {
		return nil, false
	}
	strikes := make([]sbixStrike, n)
	for i := range strikes {
		off := uint64(binary.BigEndian.Uint32(raw[8+4*i:]))
		if off == 0 {
			continue
		}
		if uint64(len(raw)) < off || uint64(len(raw))-off < 4 {
			return strikes, false
		}
		s := raw[off:]
		st := &strikes[i]
		st.ppem, st.ppi = binary.BigEndian.Uint16(s), binary.BigEndian.Uint16(s[2:])
		st.src = s
		if uint64(len(s)-4) < 4*(uint64(nGlyphs)+1) {
			return strikes, false
		}
		for g := range nGlyphs {
			start, end := st.offset(g), st.offset(g+1)
			if start == end {
				continue
			}
			if start > end || uint64(len(s)) < end || end-start < 8 {
				st.n = g
				return strikes, false
			}
		}
		st.n = nGlyphs
	}
	return strikes, true
}

// offset returns the offset of glyph g's record from the strike's start.
func (st *sbixStrike) offset(g int) uint64 {
	return uint64(binary.BigEndian.Uint32(st.src[4+4*g:]))
}

// glyph returns the record of gid, as tables.ParseSbix parses it (the zero
// value when the glyph has none).
func (st *sbixStrike) glyph(gid tables.GlyphID) tables.BitmapGlyphData {
	if int(gid) >= st.n {
		return tables.BitmapGlyphData{}
	}
	start, end := st.offset(int(gid)), st.offset(int(gid)+1)
	if start == end {
		return tables.BitmapGlyphData{}
	}
	r := st.src[start:end:end]
	return tables.BitmapGlyphData{
		OriginOffsetX: int16(binary.BigEndian.Uint16(r)),
		OriginOffsetY: int16(binary.BigEndian.Uint16(r[2:])),
		GraphicType:   ot.Tag(binary.BigEndian.Uint32(r[4:])),
		Data:          r[8:],
	}
}

// resolve returns the record of gid, following up to maxDupes 'dupe'
// references.
func (st *sbixStrike) resolve(gid tables.GlyphID) tables.BitmapGlyphData {
	return st.resolveHops(gid, maxDupes)
}

// resolveHops returns the record of gid, following up to hops 'dupe'
// references; a longer chain, or a reference without a glyph ID, gives
// the zero value.
func (st *sbixStrike) resolveHops(gid tables.GlyphID, hops int) tables.BitmapGlyphData {
	for range hops + 1 {
		d := st.glyph(gid)
		if d.GraphicType != tagDupe {
			return d
		}
		if len(d.Data) < 2 {
			break
		}
		gid = tables.GlyphID(binary.BigEndian.Uint16(d.Data))
	}
	return tables.BitmapGlyphData{}
}

// choose returns the strike to draw at ppem device pixels per em: the
// smallest at least that large, else the largest (a ppem of 0 or less, or
// not a number, chooses the largest).
func (b *bitmapGlyph) choose(ppem float32) *strikeGlyph {
	if ppem > 0 {
		for i := range b.strikes {
			if float32(b.strikes[i].ppem) >= ppem {
				return &b.strikes[i]
			}
		}
	}
	return &b.strikes[len(b.strikes)-1]
}

// bitmapList returns the display list that draws glyph g of the face at
// ppem device pixels per em (0: at the size of its largest strike). The
// image is the mip level of the strike's image whose size is the smallest
// that is not smaller than the size drawn (Chromium's choice for
// downscaled images, as in core), drawn with bilinear filtering over the
// glyph's em rectangle. A PNG that cannot be decoded draws nothing.
func (tf *Typeface) bitmapList(b *bitmapGlyph, ppem float32) *gfx.DisplayList {
	sg := b.choose(ppem)
	img := bitmapImages.image(tf, sg)
	bl := gfx.NewBuilder()
	if img != nil {
		// k is the scale from the strike's pixels to device pixels; the
		// doublings are exact.
		level := 0
		if k := float64(ppem) / float64(sg.ppem); k > 0 {
			for k*2 <= 1 && level < 16 {
				k *= 2
				level++
			}
		}
		bl.DrawImage(img.Level(level), gfx.Rect{}, sg.rect, gfx.SampleLinear)
	}
	return bl.Finish()
}

// bitmapCacheBytes is the budget of the decoded bitmap glyphs, mip levels
// included: 16 MiB, about 180 emoji of Noto Color Emoji (136 by 128 pixels).
// The images are needed only when the rasterizer renders a glyph it has
// not cached (its glyph cache keeps the glyphs at the sizes drawn), so the
// budget only has to hold the emoji of a few frames.
const bitmapCacheBytes = 16 << 20

var bitmapImages = newBitmapCache(bitmapCacheBytes)

// bitmapCache holds decoded glyph images, least recently used first out.
// It is safe for concurrent use (raster workers decode in parallel). Its
// content never changes pixels: a PNG decodes to the same image every
// time.
type bitmapCache struct {
	mu    sync.Mutex
	m     map[bitmapKey]*list.Element
	lru   list.List // of *bitmapItem, most recent first
	bytes int64
	limit int64
}

// bitmapKey identifies an image by the address of its data in the font
// file, which sbix 'dupe' glyphs share.
type bitmapKey struct {
	tf  *Typeface
	png *byte
}

type bitmapItem struct {
	key  bitmapKey
	img  *gfx.Image // nil: the data does not decode
	size int64
}

func newBitmapCache(limit int64) *bitmapCache {
	return &bitmapCache{m: map[bitmapKey]*list.Element{}, limit: limit}
}

// image returns the decoded image of sg, decoding it on a miss.
func (c *bitmapCache) image(tf *Typeface, sg *strikeGlyph) *gfx.Image {
	k := bitmapKey{tf, &sg.png[0]}
	c.mu.Lock()
	if e := c.m[k]; e != nil {
		c.lru.MoveToFront(e)
		img := e.Value.(*bitmapItem).img
		c.mu.Unlock()
		return img
	}
	c.mu.Unlock()

	var img *gfx.Image
	if src, err := png.Decode(bytes.NewReader(sg.png)); err == nil && src.Bounds().Dx() > 0 && src.Bounds().Dy() > 0 {
		img = gfx.NewImage(src)
	}
	it := &bitmapItem{key: k, img: img, size: 256}
	if img != nil {
		// The pixels and the mip levels (a third more).
		it.size += int64(len(img.Pix())) * 4 / 3
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.m[k]; e != nil { // decoded by another goroutine meanwhile
		c.lru.MoveToFront(e)
		return e.Value.(*bitmapItem).img
	}
	if it.size > c.limit {
		return img
	}
	for c.bytes+it.size > c.limit {
		last := c.lru.Back()
		old := last.Value.(*bitmapItem)
		c.lru.Remove(last)
		delete(c.m, old.key)
		c.bytes -= old.size
	}
	c.m[k] = c.lru.PushFront(it)
	c.bytes += it.size
	return img
}

// len returns the number of cached images and their charged size.
func (c *bitmapCache) len() (int, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m), c.bytes
}
