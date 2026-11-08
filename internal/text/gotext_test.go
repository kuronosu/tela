package text

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/harfbuzz"
	"golang.org/x/image/tiff"
)

// systemBitmapFonts are bitmap emoji fonts some machines have; the tests
// that read them skip the missing ones (and all of them under -short:
// Apple Color Emoji is 180 MB).
var systemBitmapFonts = []string{
	"/usr/share/fonts/noto/NotoColorEmoji.ttf",
	"/usr/share/fonts/google-noto-color-emoji-fonts/NotoColorEmoji.ttf",
	"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
	"/System/Library/Fonts/Apple Color Emoji.ttc",
}

// goTextTestFonts returns the bitmap fonts that TestGoTextStubs compares:
// the checked-in ones, synthetic variants of every table layout, and the
// installed emoji fonts found.
func goTextTestFonts(t *testing.T) map[string][]byte {
	fonts := map[string][]byte{}
	for _, f := range []string{cbdtTestFile, sbixTestFile} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fonts[f] = data
	}
	for name, tabs := range syntheticBitmapFonts() {
		fonts[name] = bitmapBaseFont(t, tabs)
	}
	if !testing.Short() {
		for _, f := range systemBitmapFonts {
			if data, err := os.ReadFile(f); err == nil {
				fonts[f] = data
			}
		}
	}
	return fonts
}

// TestGoTextStubs checks that go-text, given the stubs of the bitmap
// tables (goTextLoader), reads the same font as from the file: the glyph
// extents (what HarfBuzz's fallback mark positioning uses) and advances of
// every glyph, the font extents and line metrics, the cmap, and the
// shaping output, while the hidden tables are not copied.
func TestGoTextStubs(t *testing.T) {
	for name, data := range goTextTestFonts(t) {
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for i, ld := range lds {
			src := newTableSource(ld, data, i)
			stub := goTextLoader(src)
			if stub == ld {
				t.Errorf("%s (%d): no stub loader for a font with bitmap tables", name, i)
				continue
			}
			want, err := font.NewFont(ld)
			if err != nil {
				t.Fatalf("%s (%d): %v", name, i, err)
			}
			got, err := font.NewFont(stub)
			if err != nil {
				t.Fatalf("%s (%d): with the stubs: %v", name, i, err)
			}
			maxp, _, err := tables.ParseMaxp(src.table(tagMaxp))
			if err != nil {
				t.Fatal(err)
			}
			n := int(maxp.NumGlyphs)
			compareGoTextFonts(t, name, n, want, got)

			// The stubs are small: at most a few bytes per glyph.
			var size int
			for _, tag := range []ot.Tag{tagCBLC, tagCBDT, tagEBLC, tagEBDT, tagBloc, tagBdat, tagSbix} {
				raw, _ := stub.RawTable(tag)
				size += len(raw)
			}
			if limit := 64 + 64*n; size > limit {
				t.Errorf("%s (%d): the stubs are %d bytes, want at most %d", name, i, size, limit)
			}
			t.Logf("%s (%d): %d glyphs, %d bytes of stubs", name, i, n, size)
		}
	}
}

func compareGoTextFonts(t *testing.T, name string, n int, want, got *font.Font) {
	t.Helper()
	wf, gf := font.NewFace(want), font.NewFace(got)
	bad, withExtents := 0, 0
	for g := range n + 2 { // and past the end

		gid := font.GID(g)
		we, wok := wf.GlyphExtents(gid)
		ge, gok := gf.GlyphExtents(gid)
		if wok {
			withExtents++
		}
		if we != ge || wok != gok {
			if bad++; bad <= 5 {
				t.Errorf("%s: glyph %d: extents %+v %v, want %+v %v", name, g, ge, gok, we, wok)
			}
		}
		if w, g2 := wf.HorizontalAdvance(gid), gf.HorizontalAdvance(gid); w != g2 {
			t.Errorf("%s: glyph %d: advance %v, want %v", name, g, g2, w)
		}
	}
	if bad > 5 {
		t.Errorf("%s: %d glyphs with other extents", name, bad)
	}
	if withExtents == 0 {
		t.Errorf("%s: no glyph has extents: the font tests nothing", name)
	}
	we, wok := wf.FontHExtents()
	ge, gok := gf.FontHExtents()
	if we != ge || wok != gok {
		t.Errorf("%s: font extents %+v, want %+v", name, ge, we)
	}
	for _, m := range []font.LineMetric{font.XHeight, font.CapHeight, font.UnderlinePosition, font.StrikethroughPosition} {
		if w, g := wf.LineMetric(m), gf.LineMetric(m); w != g {
			t.Errorf("%s: line metric %d: %v, want %v", name, m, g, w)
		}
	}

	// The characters of the cmap, each alone and followed by a combining
	// mark (fallback mark positioning reads both glyphs' extents).
	var text []rune
	it := want.Cmap.Iter()
	for it.Next() {
		r, g := it.Char()
		if g2, ok := got.NominalGlyph(r); !ok || g2 != g {
			t.Errorf("%s: %U maps to %d, want %d", name, r, g2, g)
		}
		text = append(text, r, ' ', r, 0x20E3, ' ', r, 0x0301, ' ')
	}
	shape := func(f *font.Face) *harfbuzz.Buffer {
		buf := harfbuzz.NewBuffer()
		buf.AddRunes(text, 0, -1)
		buf.GuessSegmentProperties()
		buf.Shape(harfbuzz.NewFont(f), nil)
		return buf
	}
	wb, gb := shape(font.NewFace(want)), shape(font.NewFace(got))
	if len(wb.Info) != len(gb.Info) {
		t.Fatalf("%s: shaped %d glyphs, want %d", name, len(gb.Info), len(wb.Info))
	}
	for i := range wb.Info {
		if wb.Info[i].Glyph != gb.Info[i].Glyph || wb.Info[i].Cluster != gb.Info[i].Cluster || wb.Pos[i] != gb.Pos[i] {
			t.Errorf("%s: shaped glyph %d: %d %+v, want %d %+v", name, i, gb.Info[i].Glyph, gb.Pos[i], wb.Info[i].Glyph, wb.Pos[i])
			break
		}
	}
}

// TestGoTextLoaderUnchanged checks that fonts without bitmap tables keep
// their own Loader.
func TestGoTextLoaderUnchanged(t *testing.T) {
	data := fontInter.data()
	lds, err := ot.NewLoaders(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if ld := goTextLoader(newTableSource(lds[0], data, 0)); ld != lds[0] {
		t.Error("Inter (no bitmap tables) got another Loader")
	}
}

// TestParseSbixStrikes checks the lazy sbix reader against
// tables.ParseSbix: the same strikes and glyph records, the same error.
func TestParseSbixStrikes(t *testing.T) {
	for name, data := range goTextTestFonts(t) {
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for i, ld := range lds {
			src := newTableSource(ld, data, i)
			raw := src.table(tagSbix)
			if raw == nil {
				continue
			}
			maxp, _, err := tables.ParseMaxp(src.table(tagMaxp))
			if err != nil {
				t.Fatal(err)
			}
			n := int(maxp.NumGlyphs)
			want, _, err := tables.ParseSbix(raw, n)
			got, ok := parseSbixStrikes(raw, n)
			if ok != (err == nil) {
				t.Errorf("%s: ok %v, ParseSbix error %v", name, ok, err)
			}
			if len(got) != len(want.Strikes) {
				t.Fatalf("%s: %d strikes, want %d", name, len(got), len(want.Strikes))
			}
			for s, ws := range want.Strikes {
				gs := &got[s]
				if gs.ppem != ws.Ppem || gs.ppi != ws.Ppi {
					t.Errorf("%s: strike %d: ppem %d ppi %d, want %d %d", name, s, gs.ppem, gs.ppi, ws.Ppem, ws.Ppi)
				}
				for g := range n {
					var w tables.BitmapGlyphData
					if g < len(ws.GlyphDatas) {
						w = ws.GlyphDatas[g]
					}
					d := gs.glyph(tables.GlyphID(g))
					if d.OriginOffsetX != w.OriginOffsetX || d.OriginOffsetY != w.OriginOffsetY || d.GraphicType != w.GraphicType || !bytes.Equal(d.Data, w.Data) {
						t.Errorf("%s: strike %d glyph %d: %v %v %v, want %v %v %v", name, s, g, d.OriginOffsetX, d.OriginOffsetY, d.GraphicType, w.OriginOffsetX, w.OriginOffsetY, w.GraphicType)
					}
				}
			}
		}
	}
}

// syntheticBitmapFonts are the extra tables of fonts that exercise the
// stubs: every CBLC index format and image format go-text reads, ties and
// overlapping ranges, a CBLC go-text rejects in front of a valid EBLC,
// sbix 'dupe' chains around go-text's limit, a strike that fails to parse,
// images go-text cannot decode, and sbix and CBLC in one font. The fonts
// have 14 glyphs.
func syntheticBitmapFonts() map[string]map[string][]byte {
	small := func(h, w uint8, bx, by int8, adv uint8) []byte { return []byte{h, w, byte(bx), byte(by), adv} }
	big := func(h, w uint8, bx, by int8, adv uint8) []byte {
		return append(small(h, w, bx, by, adv), 1, 2, 3)
	}
	img := []byte("\x89PNG not decoded by go-text for extents")
	rec17 := func(m []byte, data []byte) []byte { return append(append(m, be(uint32(len(data)))...), data...) }
	rec19 := func(data []byte) []byte { return append(be(uint32(len(data))), data...) }
	rec1 := func(m []byte) []byte { return append(m, 0xAA, 0x55) }

	cblc, cbdt := bitmapLocTables([]testStrike{
		{20, 20, []testSub{
			{first: 1, last: 3, index: 1, image: 17, recs: [][]byte{rec17(small(16, 18, 1, 15, 20), img), nil, rec17(small(10, 12, -2, 8, 14), img)}},
			{first: 4, last: 5, index: 3, image: 18, recs: [][]byte{rec17(big(20, 20, 0, 16, 20), img), rec17(big(5, 6, 3, -1, 9), img)}},
		}},
		{40, 32, []testSub{ // the first of the largest: go-text's choice
			{first: 1, last: 2, index: 2, image: 19, size: big(30, 36, 2, 28, 40), recs: [][]byte{rec19(img), rec19(img)}},
			{first: 3, last: 6, index: 4, image: 17, gids: []uint16{3, 5, 5}, recs: [][]byte{
				rec17(small(31, 33, 0, 27, 40), img), rec17(small(12, 14, 4, 2, 22), img), rec17(small(1, 1, 0, 0, 1), img)}},
			{first: 4, last: 7, index: 5, image: 19, size: big(25, 26, -1, 20, 30), gids: []uint16{4, 7}, recs: [][]byte{rec19(img), rec19(img)}},
		}},
		{40, 40, []testSub{
			{first: 1, last: 7, index: 1, image: 17, recs: [][]byte{
				rec17(small(9, 9, 9, 9, 9), img), nil, nil, nil, nil, nil, rec17(small(9, 9, 9, 9, 9), img)}},
		}},
	})
	// A CBLC go-text rejects (a black-and-white format it does not read).
	badLoc, badData := bitmapLocTables([]testStrike{
		{30, 30, []testSub{{first: 1, last: 2, index: 1, image: 6, recs: [][]byte{rec1(small(4, 4, 0, 4, 5)), rec1(small(4, 4, 0, 4, 5))}}}},
	})
	ebLoc, ebData := bitmapLocTables([]testStrike{
		{12, 12, []testSub{{first: 1, last: 3, index: 1, image: 1, recs: [][]byte{rec1(small(10, 8, 1, 9, 10)), rec1(small(3, 3, 0, 1, 4)), nil}}}},
	})

	pngG := func(w, h int) []byte { return pngOf(square2(w, h)) }
	var jpg, tif bytes.Buffer
	if err := jpeg.Encode(&jpg, square2(11, 7), nil); err != nil {
		panic(err)
	}
	if err := tiff.Encode(&tif, square2(5, 9), nil); err != nil {
		panic(err)
	}
	glyph := func(ox, oy int16, tag string, data []byte) []byte {
		return append(append(be(uint16(ox), uint16(oy)), tag...), data...)
	}
	dupe := func(g uint16) []byte { return glyph(0, 0, "dupe", be(g)) }
	// Glyph 1 is a PNG; 2 to 13 a chain of 'dupe' references to the
	// glyph before (glyph k is k-1 references from the PNG).
	chain := make([][]byte, 14)
	chain[1] = glyph(3, -4, "png ", pngG(30, 20))
	for g := 2; g < 14; g++ {
		chain[g] = dupe(uint16(g - 1))
	}
	images := make([][]byte, 14)
	images[1] = glyph(1, 2, "jpg ", jpg.Bytes())
	images[2] = glyph(-1, 0, "tiff", tif.Bytes())
	images[3] = glyph(0, 0, "png ", []byte("\x89PNG\r\n\x1a\nbroken"))
	images[4] = glyph(0, 0, "jpg ", []byte{0xFF, 0xD8, 0xFF, 0xD9})
	images[5] = glyph(0, 0, "mask", []byte{1, 2, 3})
	images[6] = glyph(2, 2, "png ", pngG(8, 8))
	images[7] = glyph(0, 0, "dupe", []byte{0}) // too short a reference
	images[8] = glyph(0, 0, "dupe", be(uint16(500)))
	broken := make([][]byte, 14)
	broken[1] = glyph(0, 0, "png ", pngG(10, 10))
	broken[3] = []byte{1, 2, 3, 4, 5} // shorter than a record's header: ParseSbix stops here
	broken[4] = glyph(0, 0, "png ", pngG(12, 12))
	small1 := make([][]byte, 14)
	small1[1] = glyph(0, 0, "png ", pngG(4, 4))
	small1[2] = glyph(0, 0, "png ", pngG(5, 5))

	return map[string]map[string][]byte{
		"synthetic CBLC":                        {"CBLC": cblc, "CBDT": cbdt},
		"synthetic EBLC behind a rejected CBLC": {"CBLC": badLoc, "CBDT": badData, "EBLC": ebLoc, "EBDT": ebData},
		"synthetic EBLC":                        {"EBLC": ebLoc, "EBDT": ebData},
		"synthetic sbix chain": {"sbix": sbixTable(1, []sbixTestStrike{
			{ppem: 64, glyphs: chain}, {ppem: 20, glyphs: small1}})},
		"synthetic sbix images": {"sbix": sbixTable(0, []sbixTestStrike{
			{ppem: 20, glyphs: small1}, {null: true}, {ppem: 96, glyphs: images}, {ppem: 96, glyphs: small1}})},
		"synthetic sbix broken strike": {"sbix": sbixTable(1, []sbixTestStrike{
			{ppem: 48, glyphs: broken}, {ppem: 64, glyphs: small1}})},
		"synthetic sbix and CBLC": {"sbix": sbixTable(1, []sbixTestStrike{{ppem: 32, glyphs: small1}}), "CBLC": cblc, "CBDT": cbdt},
	}
}

func square2(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.SetNRGBA(0, 0, color.NRGBA{0xFF, 0, 0, 0xFF})
	return img
}

// bitmapBaseFont writes a 14-glyph font with the head, OS/2 and post
// tables of the sbix test font, a cmap of U+1F600 onwards to glyphs 1 to
// 13, and the tables of extra.
func bitmapBaseFont(t *testing.T, extra map[string][]byte) []byte {
	const n = 14
	base := buildSbixTestFont()
	hhea := append([]byte(nil), tableOf(base, "hhea")...)
	binary.BigEndian.PutUint16(hhea[34:], n)
	var hmtx []byte
	cmap := map[rune]uint16{}
	for g := range n {
		hmtx = append(hmtx, be(uint16(500+50*g), int16(g))...)
		if g > 0 {
			cmap[0x1F600+rune(g)-1] = uint16(g)
		}
	}
	tabs := map[string][]byte{
		"head": tableOf(base, "head"),
		"hhea": hhea,
		"hmtx": hmtx,
		"maxp": be(uint32(0x00005000), uint16(n)),
		"OS/2": tableOf(base, "OS/2"),
		"post": tableOf(base, "post"),
		"cmap": cmapTable(cmap),
		"name": bitmapNameTable("Tela Bitmap Stub Test", "Written for Tela's tests."),
	}
	for k, v := range extra {
		tabs[k] = v
	}
	return writeSFNT(tabs)
}

type testStrike struct {
	ppemX, ppemY uint8
	subs         []testSub
}

// testSub is an index subtable: recs are the records of the glyphs of the
// range (index formats 1 and 3; nil: no bitmap), of the glyphs in order
// (2), or of gids (4 and 5); size is the big metrics of formats 2 and 5.
type testSub struct {
	first, last  uint16
	index, image uint16
	size         []byte
	gids         []uint16
	recs         [][]byte
}

// bitmapLocTables writes a CBLC (or EBLC) and its data table.
func bitmapLocTables(strikes []testStrike) (loc, data []byte) {
	data = be(uint32(0x00030000))
	loc = be(uint32(0x00030000), uint32(len(strikes)))
	var arrays []byte
	arraysStart := 8 + 48*len(strikes)
	for _, st := range strikes {
		arrayOff := arraysStart + len(arrays)
		var subs []byte
		heads := len(st.subs) * 8
		var array []byte
		for _, sub := range st.subs {
			array = append(array, be(sub.first, sub.last, uint32(heads+len(subs)))...)
			base := len(data)
			subs = append(subs, be(sub.index, sub.image, uint32(base))...)
			var offs []uint32
			for _, r := range sub.recs {
				offs = append(offs, uint32(len(data)-base))
				data = append(data, r...)
			}
			offs = append(offs, uint32(len(data)-base))
			switch sub.index {
			case 1:
				for _, o := range offs {
					subs = append(subs, be(o)...)
				}
			case 2:
				subs = append(subs, be(offs[1])...)
				subs = append(subs, sub.size...)
			case 3:
				for _, o := range offs {
					subs = append(subs, be(uint16(o))...)
				}
				if len(offs)%2 != 0 {
					subs = append(subs, 0, 0)
				}
			case 4:
				subs = append(subs, be(uint32(len(sub.gids)))...)
				for i, g := range sub.gids {
					subs = append(subs, be(g, uint16(offs[i]))...)
				}
				subs = append(subs, be(uint16(0), uint16(offs[len(offs)-1]))...)
			case 5:
				subs = append(subs, be(offs[1])...)
				subs = append(subs, sub.size...)
				subs = append(subs, be(uint32(len(sub.gids)))...)
				for _, g := range sub.gids {
					subs = append(subs, be(g)...)
				}
				if len(sub.gids)%2 != 0 {
					subs = append(subs, 0, 0)
				}
			}
		}
		table := append(array, subs...)
		lines := make([]byte, 24)
		for i := range lines {
			lines[i] = byte(i + 1)
		}
		loc = append(loc, be(uint32(arrayOff), uint32(len(table)), uint32(len(st.subs)), uint32(0))...)
		loc = append(loc, lines...)
		loc = append(loc, be(uint16(1), uint16(13))...)
		loc = append(loc, st.ppemX, st.ppemY, 32, 1)
		arrays = append(arrays, table...)
	}
	return append(loc, arrays...), data
}

type sbixTestStrike struct {
	null   bool // a null offset
	ppem   uint16
	glyphs [][]byte
}

// sbixTable writes an sbix table of 14 glyphs.
func sbixTable(flags uint16, strikes []sbixTestStrike) []byte { return sbixTableN(14, flags, strikes) }

// sbixTableN writes an sbix table of n glyphs.
func sbixTableN(n int, flags uint16, strikes []sbixTestStrike) []byte {
	var bodies [][]byte
	for _, st := range strikes {
		if st.null {
			bodies = append(bodies, nil)
			continue
		}
		out := be(st.ppem, uint16(72))
		off := uint32(4 + 4*(n+1))
		var body []byte
		for g := range n {
			out = append(out, be(off+uint32(len(body)))...)
			if g < len(st.glyphs) {
				body = append(body, st.glyphs[g]...)
			}
		}
		out = append(out, be(off+uint32(len(body)))...)
		bodies = append(bodies, append(out, body...))
	}
	out := be(uint16(1), flags, uint32(len(strikes)))
	off := uint32(8 + 4*len(strikes))
	for _, b := range bodies {
		if b == nil {
			out = append(out, be(uint32(0))...)
			continue
		}
		out = append(out, be(off)...)
		off += uint32(len(b))
	}
	for _, b := range bodies {
		out = append(out, b...)
	}
	return out
}
