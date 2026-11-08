// Copyright 2021 The go-text authors. Licensed under the BSD 3-Clause
// license (github.com/go-text/typesetting, SPDX: Unlicense OR BSD-3-Clause).
// The bitmap-table checks and strike choices repeat those of font/font.go
// (selectBitmapTable), font/bitmaps.go (newBitmap, newBitmapSubtable,
// parseBitmapDataMetrics, parseBitmapDataStandalone, chooseStrike) and
// font/glyphs.go (strikeGlyph, decodeBitmapConfig); see THIRD_PARTY.md.

package text

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/tiff"
)

// go-text's font.NewFont copies every table it parses into the heap (its
// Loader reads a table into a new buffer), and its parsed bitmap tables
// keep sub-slices of those copies: 10.6 MB of CBDT for Noto Color Emoji,
// most of 180 MB of sbix for Apple Color Emoji. Tela draws the bitmaps
// from the file bytes itself (bitmap.go); go-text uses its bitmap tables
// only for glyph extents (HarfBuzz's fallback mark positioning) and for
// what Tela does not call (bitmap glyph data, the list of strike sizes).
// So goTextLoader gives go-text a table directory in which the bitmap
// tables (CBLC/CBDT, EBLC/EBDT, bloc/bdat and sbix) are replaced by small
// stubs that give the same extents:
//
//   - Tela never sets the pixels per em of a go-text face (ShapingFace,
//     the CFF face), so go-text always reads the strike it chooses for 0
//     ppem, the first of the largest; the stubs hold that strike only.
//     Code that calls font.Face.SetPpem must make them keep every strike.
//   - The grayscale EBLC/EBDT and bloc/bdat bitmaps are hidden too: Tela
//     does not draw them, and go-text copies them like the others (CJK
//     fonts with embedded bitmaps for small sizes carry megabytes).
//   - CBLC: go-text takes the first of CBLC/CBDT, EBLC/EBDT and bloc/bdat
//     that it parses without error (every glyph record of every strike is
//     checked). The stub is that pair, rewritten as one strike (its line
//     metrics and sizes copied) of index format 1 and image format 17
//     with the metrics go-text reads for each glyph and no image data,
//     under CBLC/CBDT; when no pair parses there is none, and go-text
//     ends with no bitmap table either way.
//   - sbix: the stub is the chosen strike of what tables.ParseSbix
//     returns (it keeps what it parsed before an error, and go-text uses
//     that), with 'dupe' chains resolved as go-text resolves them and each
//     image replaced by a PNG header of the same size (go-text decodes
//     only the header for extents); an image go-text cannot decode has no
//     record.
//
// TestGoTextStubs checks that the extents of every glyph, the advances,
// the font extents and the shaping output are the same with the stubs.

var (
	tagEBLC = ot.MustNewTag("EBLC")
	tagEBDT = ot.MustNewTag("EBDT")
	tagBloc = ot.MustNewTag("bloc")
	tagBdat = ot.MustNewTag("bdat")
	tagMaxp = ot.MustNewTag("maxp")
	tagTIFF = ot.MustNewTag("tiff")
	tagJPG  = ot.MustNewTag("jpg ")
)

// goTextBitmapPairs are the bitmap location and data tables in go-text's
// order of preference (selectBitmapTable).
var goTextBitmapPairs = [...][2]ot.Tag{{tagCBLC, tagCBDT}, {tagEBLC, tagEBDT}, {tagBloc, tagBdat}}

// goTextSbixHops is how many sbix 'dupe' references go-text follows
// (strikeGlyph's maxRecursionLevel, plus one).
const goTextSbixHops = 9

func goTextHides(tag ot.Tag) bool {
	switch tag {
	case tagCBLC, tagCBDT, tagEBLC, tagEBDT, tagBloc, tagBdat, tagSbix:
		return true
	}
	return false
}

// goTextLoader returns the Loader for go-text's font.NewFont of src: src's
// own when the font has no bitmap tables or src does not read the file's
// table directory (WOFF, dfont), else a Loader over the same file whose
// directory has the stubs in place of the bitmap tables.
func goTextLoader(src tableSource) *ot.Loader {
	if src.data == nil {
		return src.ld
	}
	hide := false
	for r := src.dir; len(r) >= 16; r = r[16:] {
		if goTextHides(ot.Tag(binary.BigEndian.Uint32(r))) {
			hide = true
			break
		}
	}
	if !hide {
		return src.ld
	}
	maxp, _, err := tables.ParseMaxp(src.table(tagMaxp))
	if err != nil {
		return src.ld // font.NewFont fails on its own
	}
	nGlyphs := int(maxp.NumGlyphs)

	type stub struct {
		tag  ot.Tag
		data []byte
	}
	var stubs []stub
	for _, p := range goTextBitmapPairs {
		if loc, dat, ok := goTextBitmapStub(src.table(p[0]), src.table(p[1])); ok {
			stubs = append(stubs, stub{tagCBLC, loc}, stub{tagCBDT, dat})
			break
		}
	}
	if sb := goTextSbixStub(src.table(tagSbix), nGlyphs); sb != nil {
		stubs = append(stubs, stub{tagSbix, sb})
	}

	// The file seen by go-text: a directory, the font file unchanged (its
	// table offsets shifted by the directory's size) and the stubs.
	var kept [][]byte
	for r := src.dir; len(r) >= 16; r = r[16:] {
		if !goTextHides(ot.Tag(binary.BigEndian.Uint32(r))) {
			kept = append(kept, r[:16])
		}
	}
	nTables := len(kept) + len(stubs)
	if nTables > math.MaxUint16 {
		return src.ld
	}
	head := uint64(12 + 16*nTables)
	dataEnd := head + uint64(len(src.data))
	tail := make([]byte, (4-dataEnd%4)%4) // the stubs start 4-byte aligned
	stubOff := make([]uint64, len(stubs))
	for i, s := range stubs {
		stubOff[i] = dataEnd + uint64(len(tail))
		tail = append(tail, s.data...)
		for len(tail)%4 != 0 {
			tail = append(tail, 0)
		}
	}
	if dataEnd+uint64(len(tail)) > math.MaxUint32 {
		return src.ld
	}
	dir := make([]byte, 12, head)
	binary.BigEndian.PutUint32(dir, uint32(src.ld.Type))
	binary.BigEndian.PutUint16(dir[4:], uint16(nTables))
	for _, r := range kept {
		off := uint64(binary.BigEndian.Uint32(r[8:])) + head
		if off > math.MaxUint32 {
			return src.ld
		}
		dir = append(dir, r[:8]...)
		dir = binary.BigEndian.AppendUint32(dir, uint32(off))
		dir = append(dir, r[12:16]...)
	}
	for i, s := range stubs {
		dir = binary.BigEndian.AppendUint32(dir, uint32(s.tag))
		dir = binary.BigEndian.AppendUint32(dir, 0) // checksum, not read
		dir = binary.BigEndian.AppendUint32(dir, uint32(stubOff[i]))
		dir = binary.BigEndian.AppendUint32(dir, uint32(len(s.data)))
	}
	r := concatReader{dir, src.data, tail}
	ld, err := ot.NewLoader(io.NewSectionReader(r, 0, int64(dataEnd)+int64(len(tail))))
	if err != nil {
		return src.ld
	}
	return ld
}

// concatReader reads its parts one after another, as one file.
type concatReader [3][]byte

func (c concatReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("text: negative offset")
	}
	n := 0
	for _, part := range c {
		if off >= int64(len(part)) {
			off -= int64(len(part))
			continue
		}
		n += copy(p[n:], part[off:])
		off = 0
		if n == len(p) {
			return n, nil
		}
	}
	return n, io.EOF
}

// goTextGlyph is what go-text reads of a glyph's bitmap in a strike for
// its extents: whether it has one, and its metrics.
type goTextGlyph struct {
	found bool
	m     tables.SmallGlyphMetrics
}

// goTextRange is an index subtable of the chosen strike: its glyph range
// and the glyphs of the range.
type goTextRange struct {
	first, last tables.GlyphID
	glyphs      []goTextGlyph
}

// goTextBitmapStub returns the stub of a bitmap location table (CBLC,
// EBLC, bloc) and its data table, and false when go-text's loadBitmap
// fails on them (newBitmap's checks).
func goTextBitmapStub(locRaw, data []byte) (loc, dat []byte, ok bool) {
	if locRaw == nil || data == nil {
		return nil, nil, false
	}
	cblc, _, err := tables.ParseCBLC(locRaw)
	if err != nil || len(cblc.IndexSubTables) < len(cblc.BitmapSizes) {
		return nil, nil, false
	}
	// go-text's chooseStrike for 0 ppem: the first of the largest.
	best := 0
	for i, s := range cblc.BitmapSizes {
		b := cblc.BitmapSizes[best]
		if max(s.PpemX, s.PpemY) > max(b.PpemX, b.PpemY) {
			best = i
		}
	}
	var ranges []goTextRange
	for i := range cblc.BitmapSizes {
		for _, sub := range cblc.IndexSubTables[i] {
			r, err := goTextSubtable(sub, data, i == best)
			if err != nil {
				return nil, nil, false
			}
			if i == best {
				ranges = append(ranges, r)
			}
		}
	}

	loc = be32s(nil, 0x00030000, 0) // version 3.0, no strike
	dat = be32s(nil, 0x00030000)
	if len(cblc.BitmapSizes) == 0 {
		return loc, dat, true
	}
	binary.BigEndian.PutUint32(loc[4:], 1)
	const arrayStart = 8 + 48
	loc = append(loc, locRaw[8+48*best:8+48*best+48]...) // a valid parse holds the record
	sub := uint64(8 * len(ranges))                       // the first subtable, from the array's start
	for _, r := range ranges {
		loc = binary.BigEndian.AppendUint16(loc, uint16(r.first))
		loc = binary.BigEndian.AppendUint16(loc, uint16(r.last))
		loc = binary.BigEndian.AppendUint32(loc, uint32(sub))
		sub += 8 + 4*uint64(len(r.glyphs)+1)
	}
	for _, r := range ranges {
		loc = binary.BigEndian.AppendUint16(loc, 1)  // index format: 32-bit offsets
		loc = binary.BigEndian.AppendUint16(loc, 17) // image format: small metrics, PNG
		base := len(dat)
		loc = binary.BigEndian.AppendUint32(loc, uint32(base))
		for _, g := range r.glyphs {
			loc = binary.BigEndian.AppendUint32(loc, uint32(len(dat)-base))
			if g.found {
				dat = append(dat, g.m.Height, g.m.Width, byte(g.m.BearingX), byte(g.m.BearingY), g.m.Advance)
				dat = binary.BigEndian.AppendUint32(dat, 0) // no image data
			}
		}
		loc = binary.BigEndian.AppendUint32(loc, uint32(len(dat)-base))
	}
	rec := loc[8:arrayStart]
	binary.BigEndian.PutUint32(rec[0:], arrayStart)                  // indexSubTableArrayOffset
	binary.BigEndian.PutUint32(rec[4:], uint32(len(loc)-arrayStart)) // indexTablesSize
	binary.BigEndian.PutUint32(rec[8:], uint32(len(ranges)))         // numberOfIndexSubTables
	if uint64(len(loc)) > math.MaxUint32 || uint64(len(dat)) > math.MaxUint32 {
		return nil, nil, false
	}
	return loc, dat, true
}

// goTextSubtable checks an index subtable as go-text's newBitmapSubtable
// does and, when keep is set, returns what go-text reads for each glyph of
// its range (bitmapSubtable.image).
func goTextSubtable(sub tables.BitmapSubtable, data []byte, keep bool) (goTextRange, error) {
	r := goTextRange{first: sub.FirstGlyph, last: sub.LastGlyph}
	if uint64(len(data)) < uint64(sub.ImageDataOffset) {
		return r, errors.New("invalid bitmap table (EOF)")
	}
	if sub.LastGlyph < sub.FirstGlyph {
		return r, errors.New("invalid bitmap subtable glyph range")
	}
	img := data[sub.ImageDataOffset:]
	n := int(sub.LastGlyph) - int(sub.FirstGlyph) + 1
	if keep {
		r.glyphs = make([]goTextGlyph, n)
	}
	set := func(i int, m tables.SmallGlyphMetrics) {
		if keep && i < n {
			r.glyphs[i] = goTextGlyph{true, m}
		}
	}
	switch d := sub.IndexData.(type) {
	case tables.IndexData1:
		for i := 0; i+1 < len(d.SbitOffsets); i++ {
			if d.SbitOffsets[i] == d.SbitOffsets[i+1] {
				continue // no bitmap
			}
			m, err := goTextMetrics(img, uint32(d.SbitOffsets[i]), uint32(d.SbitOffsets[i+1]), sub.ImageFormat)
			if err != nil {
				return r, err
			}
			set(i, m)
		}
	case tables.IndexData3:
		for i := 0; i+1 < len(d.SbitOffsets); i++ {
			if d.SbitOffsets[i] == d.SbitOffsets[i+1] {
				continue
			}
			m, err := goTextMetrics(img, uint32(d.SbitOffsets[i]), uint32(d.SbitOffsets[i+1]), sub.ImageFormat)
			if err != nil {
				return r, err
			}
			set(i, m)
		}
	case tables.IndexData2:
		for i := range n {
			// uint32 products, as in go-text.
			if err := goTextStandalone(img, d.ImageSize*uint32(i), d.ImageSize*uint32(i+1), sub.ImageFormat); err != nil {
				return r, err
			}
			set(i, d.BigMetrics.SmallGlyphMetrics)
		}
	case tables.IndexData4:
		if len(d.GlyphArray) == 0 {
			return r, errors.New("invalid bitmap index format 4: empty glyph array")
		}
		for i := 0; i+1 < len(d.GlyphArray); i++ {
			cur, next := d.GlyphArray[i], d.GlyphArray[i+1]
			m, err := goTextMetrics(img, uint32(cur.SbitOffset), uint32(next.SbitOffset), sub.ImageFormat)
			if err != nil {
				return r, err
			}
			// go-text searches the array in order: the first entry of a
			// glyph wins.
			if g := int(cur.GlyphID) - int(sub.FirstGlyph); keep && g >= 0 && g < n && !r.glyphs[g].found {
				set(g, m)
			}
		}
	case tables.IndexData5:
		for i := range d.GlyphIdArray {
			if err := goTextStandalone(img, d.ImageSize*uint32(i), d.ImageSize*uint32(i+1), sub.ImageFormat); err != nil {
				return r, err
			}
		}
		if keep {
			for i := range n {
				// go-text's binary search (the array should be sorted).
				gid := sub.FirstGlyph + tables.GlyphID(i)
				for lo, hi := 0, len(d.GlyphIdArray); lo < hi; {
					h := lo + (hi-lo)/2
					if e := d.GlyphIdArray[h]; gid < e {
						hi = h
					} else if e < gid {
						lo = h + 1
					} else {
						set(i, d.BigMetrics.SmallGlyphMetrics)
						break
					}
				}
			}
		}
	}
	return r, nil
}

// goTextMetrics returns the metrics of a glyph record with metrics, as
// go-text's parseBitmapDataMetrics.
func goTextMetrics(img []byte, start, end uint32, format uint16) (tables.SmallGlyphMetrics, error) {
	if uint64(len(img)) < uint64(end) || start > end {
		return tables.SmallGlyphMetrics{}, errors.New("invalid bitmap data table (EOF)")
	}
	b := img[start:end]
	switch format {
	case 1, 2:
		d, _, err := tables.ParseBitmapData1Or2(b)
		return d.SmallGlyphMetrics, err
	case 17:
		d, _, err := tables.ParseBitmapData17(b)
		return d.SmallGlyphMetrics, err
	case 18:
		d, _, err := tables.ParseBitmapData18(b)
		return d.SmallGlyphMetrics, err
	}
	return tables.SmallGlyphMetrics{}, errors.New("unsupported bitmap image format")
}

// goTextStandalone checks a glyph record without metrics, as go-text's
// parseBitmapDataStandalone.
func goTextStandalone(img []byte, start, end uint32, format uint16) error {
	if uint64(len(img)) < uint64(end) || start > end {
		return errors.New("invalid bitmap data table (EOF)")
	}
	b := img[start:end]
	switch format {
	case 5:
		_, _, err := tables.ParseBitmapData5(b)
		return err
	case 19:
		_, _, err := tables.ParseBitmapData19(b)
		return err
	}
	return errors.New("unsupported bitmap image format")
}

// goTextSbixStub returns the stub of an sbix table, or nil when go-text
// gets no extents from it (no table, or the chosen strike has no size).
func goTextSbixStub(raw []byte, nGlyphs int) []byte {
	if raw == nil {
		return nil
	}
	strikes, _ := parseSbixStrikes(raw, nGlyphs) // go-text keeps what parsed
	if len(strikes) == 0 {
		return nil
	}
	best := 0
	for i := range strikes {
		if strikes[i].ppem > strikes[best].ppem {
			best = i
		}
	}
	st := &strikes[best]
	if st.ppem == 0 {
		return nil
	}
	const strikeStart = 12
	out := be32s(nil, 0x00010000|uint32(binary.BigEndian.Uint16(raw[2:])), 1, strikeStart) // version 1, flags; one strike
	out = binary.BigEndian.AppendUint16(out, st.ppem)
	out = binary.BigEndian.AppendUint16(out, st.ppi)
	offs := len(out)
	out = append(out, make([]byte, 4*(nGlyphs+1))...)
	for g := range nGlyphs {
		binary.BigEndian.PutUint32(out[offs+4*g:], uint32(len(out)-strikeStart))
		d := st.resolveHops(tables.GlyphID(g), goTextSbixHops)
		if d.GraphicType == 0 {
			continue
		}
		w, h, err := goTextImageSize(d)
		if err != nil {
			continue // go-text has no extents for it
		}
		out = binary.BigEndian.AppendUint16(out, uint16(d.OriginOffsetX))
		out = binary.BigEndian.AppendUint16(out, uint16(d.OriginOffsetY))
		if w > 0 && h > 0 && w <= math.MaxInt32 && h <= math.MaxInt32 {
			out = binary.BigEndian.AppendUint32(out, uint32(tagPNG))
			out = appendPNGHeader(out, w, h)
		} else { // a size a PNG cannot have: keep the image (never seen)
			out = binary.BigEndian.AppendUint32(out, uint32(d.GraphicType))
			out = append(out, d.Data...)
		}
	}
	binary.BigEndian.PutUint32(out[offs+4*nGlyphs:], uint32(len(out)-strikeStart))
	if uint64(len(out)) > math.MaxUint32 {
		return nil
	}
	return out
}

// goTextImageSize returns the size of an sbix image as go-text's
// decodeBitmapConfig reads it.
func goTextImageSize(d tables.BitmapGlyphData) (w, h int, err error) {
	var cfg image.Config
	switch d.GraphicType {
	case tagPNG:
		cfg, err = png.DecodeConfig(bytes.NewReader(d.Data))
	case tagTIFF:
		cfg, err = tiff.DecodeConfig(bytes.NewReader(d.Data))
	case tagJPG:
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(d.Data))
	default:
		err = errors.New("unsupported graphic type")
	}
	return cfg.Width, cfg.Height, err
}

// appendPNGHeader appends the signature and IHDR chunk of a w by h RGBA
// PNG: what png.DecodeConfig reads.
func appendPNGHeader(out []byte, w, h int) []byte {
	out = append(out, pngMagic...)
	chunk := binary.BigEndian.AppendUint32(nil, 13)
	chunk = append(chunk, "IHDR"...)
	chunk = be32s(chunk, uint32(w), uint32(h))
	chunk = append(chunk, 8, 6, 0, 0, 0) // 8-bit RGBA, deflate, no filter method, not interlaced
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	return append(out, chunk...)
}

// be32s appends vs big-endian.
func be32s(out []byte, vs ...uint32) []byte {
	for _, v := range vs {
		out = binary.BigEndian.AppendUint32(out, v)
	}
	return out
}
