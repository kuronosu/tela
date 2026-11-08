package text

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"sort"
	"testing"
	"unicode/utf16"

	"github.com/go-text/typesetting/font/opentype/tables"
)

// The bitmap test fonts of testdata/fonts are written by
// TestGenerateBitmapFonts, which runs only with TELA_GEN_BITMAP_FONTS=1:
//
//	TELA_GEN_BITMAP_FONTS=1 CGO_ENABLED=0 go test -run TestGenerateBitmapFonts ./internal/text/
//
// NotoColorEmoji-CBDT-Subset.ttf takes a few glyphs of the CBDT build of
// Noto Color Emoji (TELA_NOTO_CBDT, by default the file most Linux
// distributions install; testdata/fonts/README.md records the one used)
// with their PNG images and metrics copied unchanged. TelaSbixTest.ttf is
// synthetic: two strikes of plain shapes whose colors tell the strikes
// apart. Tests never read system fonts: they use the files checked in.

const (
	cbdtTestFile = "testdata/fonts/NotoColorEmoji-CBDT-Subset.ttf"
	sbixTestFile = "testdata/fonts/TelaSbixTest.ttf"
)

// cbdtTestRunes are the characters of the CBDT subset, glyphs 2 onwards
// (0 is .notdef and 1 the space).
var cbdtTestRunes = []rune{0x1F600, 0x1F60D, 0x1F525, 0x1F389, 0x2764, 0x1F44D, 0x1F30D, 0x1F914}

func TestGenerateBitmapFonts(t *testing.T) {
	if os.Getenv("TELA_GEN_BITMAP_FONTS") != "1" {
		t.Skip("set TELA_GEN_BITMAP_FONTS=1 to write the bitmap test fonts")
	}
	src := os.Getenv("TELA_NOTO_CBDT")
	if src == "" {
		src = "/usr/share/fonts/noto/NotoColorEmoji.ttf"
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	cbdt, err := buildCBDTSubset(data, cbdtTestRunes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cbdtTestFile, cbdt, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sbixTestFile, buildSbixTestFont(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildCBDTSubset writes a CBDT font with the glyphs of runes taken from
// the CBDT font data: its first strike's PNG records (format 17) copied
// byte for byte, its advances, and its head, hhea, OS/2 and post tables.
func buildCBDTSubset(data []byte, runes []rune) ([]byte, error) {
	tfs, err := ParseTypefaces(data, "")
	if err != nil {
		return nil, err
	}
	tf := tfs[0]
	bt := tf.bitmaps()
	st := &bt.strikes[0]
	space, ok := tf.ft.NominalGlyph(' ')
	if !ok {
		return nil, fmt.Errorf("no space glyph")
	}
	advs := []uint16{tf.hmtx.Advance(0), tf.hmtx.Advance(tables.GlyphID(space))}
	var records [][]byte
	widthMax := 0
	for _, r := range runes {
		gid, ok := tf.ft.NominalGlyph(r)
		if !ok {
			return nil, fmt.Errorf("no glyph for %U", r)
		}
		rec, format, _, ok := bt.cbdtData(st, tables.GlyphID(gid))
		if !ok || format != 17 {
			return nil, fmt.Errorf("%U: no format 17 image", r)
		}
		sg, _ := bt.glyph(st, tables.GlyphID(gid))
		widthMax = max(widthMax, sg.w)
		records = append(records, rec)
		advs = append(advs, tf.hmtx.Advance(tables.GlyphID(gid)))
	}
	n := len(advs)

	// CBDT: the header, then the records.
	cbdt := be(uint16(3), uint16(0))
	offs := []uint32{0}
	for _, r := range records {
		cbdt = append(cbdt, r...)
		offs = append(offs, offs[len(offs)-1]+uint32(len(r)))
	}
	// CBLC: one strike with one index subtable (format 1) for glyphs 2..n-1.
	locHeader, sizeLen, arrayLen := 8, 48, 8
	arrayOff := locHeader + sizeLen
	sub := be(uint16(1), uint16(17), uint32(4))
	for _, o := range offs {
		sub = append(sub, be(o)...)
	}
	line := func(asc, desc int8) []byte {
		return []byte{byte(asc), byte(desc), byte(widthMax), 0, 0, 0, 0, 0, 0, 0, 0, 0}
	}
	cblc := be(uint16(3), uint16(0), uint32(1))
	cblc = append(cblc, be(uint32(arrayOff), uint32(arrayLen+len(sub)), uint32(1), uint32(0))...)
	cblc = append(cblc, line(101, -27)...)
	cblc = append(cblc, line(101, -27)...)
	cblc = append(cblc, be(uint16(2), uint16(n-1))...)
	cblc = append(cblc, byte(st.ppemX), byte(st.ppemY), 32, 1)
	cblc = append(cblc, be(uint16(2), uint16(n-1), uint32(arrayLen))...)
	cblc = append(cblc, sub...)

	raw := func(tag string) []byte { return tableOf(data, tag) }
	hhea := append([]byte(nil), raw("hhea")...)
	binary.BigEndian.PutUint16(hhea[34:], uint16(n))
	hmtx := make([]byte, 0, 4*n)
	for _, a := range advs {
		hmtx = append(hmtx, be(a, uint16(0))...)
	}
	cmap := map[rune]uint16{' ': 1}
	for i, r := range runes {
		cmap[r] = uint16(i + 2)
	}
	return writeSFNT(map[string][]byte{
		"head": raw("head"),
		"hhea": hhea,
		"hmtx": hmtx,
		"maxp": be(uint32(0x00005000), uint16(n)),
		"OS/2": raw("OS/2"),
		"post": raw("post")[:32],
		"cmap": cmapTable(cmap),
		"name": nameTable("Noto Color Emoji CBDT Subset", "Copyright 2021 Google Inc. All Rights Reserved."),
		"CBLC": cblc,
		"CBDT": cbdt,
	}), nil
}

// buildSbixTestFont writes a synthetic sbix font (1000 units per em) with
// strikes of 20 and 64 pixels per em:
//
//   - glyph 2 (U+1F600) is a disc, red in the 20 px strike and blue in the
//     64 px one, 1 em wide, its bottom 0.2 em below the baseline;
//   - glyph 3 (U+1F601) is a 'dupe' of glyph 2 in both strikes;
//   - glyph 4 (U+1F602) is a 'jpg ' image (not drawn);
//   - glyph 5 (U+1F603) is a green square in the 64 px strike only.
func buildSbixTestFont() []byte {
	const upem, n = 1000, 6
	type glyph struct {
		ox, oy int16
		tag    string
		data   []byte
	}
	strike := func(ppem int, col color.NRGBA) []byte {
		gs := make([]glyph, n)
		gs[2] = glyph{0, int16(-ppem / 5), "png ", pngOf(disc(ppem, col))}
		gs[3] = glyph{0, 0, "dupe", be(uint16(2))}
		gs[4] = glyph{0, 0, "jpg ", []byte{0xFF, 0xD8, 0xFF, 0xD9}}
		if ppem == 64 {
			gs[5] = glyph{4, 0, "png ", pngOf(square(ppem*3/4, color.NRGBA{0x2E, 0x7D, 0x32, 0xFF}))}
		}
		out := be(uint16(ppem), uint16(72))
		off := uint32(4 + 4*(n+1))
		var body []byte
		for _, g := range gs {
			out = append(out, be(off+uint32(len(body)))...)
			if g.tag != "" {
				body = append(body, be(uint16(g.ox), uint16(g.oy))...)
				body = append(body, g.tag...)
				body = append(body, g.data...)
			}
		}
		out = append(out, be(off+uint32(len(body)))...)
		return append(out, body...)
	}
	strikes := [][]byte{strike(20, color.NRGBA{0xE5, 0x39, 0x35, 0xFF}), strike(64, color.NRGBA{0x1E, 0x88, 0xE5, 0xFF})}
	sbix := be(uint16(1), uint16(1), uint32(len(strikes)))
	off := uint32(8 + 4*len(strikes))
	for _, s := range strikes {
		sbix = append(sbix, be(off)...)
		off += uint32(len(s))
	}
	for _, s := range strikes {
		sbix = append(sbix, s...)
	}

	head := be(uint32(0x00010000), uint32(0x00010000), uint32(0), uint32(0x5F0F3CF5), uint16(0x000B), uint16(upem))
	head = append(head, make([]byte, 16)...) // created, modified
	head = append(head, be(uint16(0), uint16(0xFF38), uint16(1000), uint16(950), uint16(0), uint16(8), uint16(2), uint16(0), uint16(0))...)
	hhea := be(uint32(0x00010000), uint16(950), uint16(0xFF06), uint16(0), uint16(1000), uint16(0), uint16(0), uint16(1000),
		uint16(1), uint16(0), uint16(0), uint64(0), uint16(0), uint16(n))
	hmtx := be(uint16(500), uint16(0), uint16(250), uint16(0))
	for range n - 2 {
		hmtx = append(hmtx, be(uint16(1000), uint16(0))...)
	}
	os2 := be(uint16(4), int16(1000), uint16(400), uint16(5), uint16(0))
	os2 = append(os2, make([]byte, 20)...) // subscript, superscript, strikeout
	os2 = append(os2, make([]byte, 2+10+16+4)...)
	os2 = append(os2, be(uint16(0x40), uint16(0x20), uint16(0xFFFF), int16(950), int16(-250), int16(0), uint16(950), uint16(250))...)
	os2 = append(os2, make([]byte, 8)...) // code page ranges
	os2 = append(os2, be(int16(0), int16(0), uint16(0), uint16(0x20), uint16(0))...)
	// strikeout size and position (offsets 26 and 28)
	binary.BigEndian.PutUint16(os2[26:], 50)
	binary.BigEndian.PutUint16(os2[28:], 300)
	post := be(uint32(0x00030000), uint32(0), int16(-100), int16(50), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0))
	return writeSFNT(map[string][]byte{
		"head": head,
		"hhea": hhea,
		"hmtx": hmtx,
		"maxp": be(uint32(0x00005000), uint16(n)),
		"OS/2": os2,
		"post": post,
		"cmap": cmapTable(map[rune]uint16{' ': 1, 0x1F600: 2, 0x1F601: 3, 0x1F602: 4, 0x1F603: 5}),
		"name": nameTable("Tela Sbix Test", "Written for Tela's tests."),
		"sbix": sbix,
	})
}

// disc returns an anti-aliased disc of diameter size with a white center
// dot, so that scaled drawings show their filtering.
func disc(size int, col color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	r := float64(size) / 2
	cover := func(x, y int, rad, cx, cy float64) float64 {
		in := 0
		for sy := range 4 {
			for sx := range 4 {
				dx, dy := float64(x)+(float64(sx)+0.5)/4-cx, float64(y)+(float64(sy)+0.5)/4-cy
				if dx*dx+dy*dy <= rad*rad {
					in++
				}
			}
		}
		return float64(in) / 16
	}
	for y := range size {
		for x := range size {
			a := cover(x, y, r, r, r)
			if a == 0 {
				continue
			}
			c := col
			if w := cover(x, y, r/4, r, r); w > 0 {
				c.R = uint8(float64(c.R) + (255-float64(c.R))*w)
				c.G = uint8(float64(c.G) + (255-float64(c.G))*w)
				c.B = uint8(float64(c.B) + (255-float64(c.B))*w)
			}
			c.A = uint8(a*255 + 0.5)
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func square(size int, col color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.SetNRGBA(x, y, col)
		}
	}
	return img
}

func pngOf(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// be encodes values big-endian.
func be(vs ...any) []byte {
	var b bytes.Buffer
	for _, v := range vs {
		if err := binary.Write(&b, binary.BigEndian, v); err != nil {
			panic(err)
		}
	}
	return b.Bytes()
}

// tableOf returns table tag of a plain sfnt file.
func tableOf(data []byte, tag string) []byte {
	n := int(binary.BigEndian.Uint16(data[4:]))
	for i := range n {
		r := data[12+16*i:]
		if string(r[:4]) == tag {
			off, l := binary.BigEndian.Uint32(r[8:]), binary.BigEndian.Uint32(r[12:])
			return data[off : off+l]
		}
	}
	return nil
}

// cmapTable writes a cmap with one format 12 subtable (Windows, full
// Unicode).
func cmapTable(m map[rune]uint16) []byte {
	rs := make([]rune, 0, len(m))
	for r := range m {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
	sub := be(uint16(12), uint16(0), uint32(16+12*len(rs)), uint32(0), uint32(len(rs)))
	for _, r := range rs {
		sub = append(sub, be(uint32(r), uint32(r), uint32(m[r]))...)
	}
	out := be(uint16(0), uint16(1), uint16(3), uint16(10), uint32(12))
	return append(out, sub...)
}

// nameTable writes the family, subfamily, full and PostScript names, the
// copyright and the license notice (Windows, US English).
func nameTable(family, copyright string) []byte {
	ps := []rune{}
	for _, r := range family {
		if r != ' ' {
			ps = append(ps, r)
		}
	}
	names := []struct {
		id uint16
		s  string
	}{
		{0, copyright}, {1, family}, {2, "Regular"}, {4, family}, {6, string(ps) + "-Regular"},
		{13, "This Font Software is licensed under the SIL Open Font License, Version 1.1."},
	}
	var strs []byte
	out := be(uint16(0), uint16(len(names)), uint16(6+12*len(names)))
	for _, n := range names {
		var u []byte
		for _, c := range utf16.Encode([]rune(n.s)) {
			u = append(u, be(c)...)
		}
		out = append(out, be(uint16(3), uint16(1), uint16(0x409), n.id, uint16(len(u)), uint16(len(strs)))...)
		strs = append(strs, u...)
	}
	return append(out, strs...)
}

// writeSFNT writes a TrueType file of tables, with their checksums and the
// head checksum adjustment.
func writeSFNT(tabs map[string][]byte) []byte {
	tags := make([]string, 0, len(tabs))
	for t := range tabs {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	sr, es := 1, 0
	for sr*2 <= n {
		sr *= 2
		es++
	}
	out := be(uint32(0x00010000), uint16(n), uint16(16*sr), uint16(es), uint16(16*(n-sr)))
	dir := len(out)
	out = append(out, make([]byte, 16*n)...)
	headOff := 0
	for i, t := range tags {
		d := append([]byte(nil), tabs[t]...)
		if t == "head" {
			binary.BigEndian.PutUint32(d[8:], 0)
			headOff = len(out)
		}
		off := len(out)
		out = append(out, d...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		r := out[dir+16*i:]
		copy(r, t)
		binary.BigEndian.PutUint32(r[4:], checksum(out[off:]))
		binary.BigEndian.PutUint32(r[8:], uint32(off))
		binary.BigEndian.PutUint32(r[12:], uint32(len(d)))
	}
	binary.BigEndian.PutUint32(out[headOff+8:], 0xB1B0AFBA-checksum(out))
	return out
}

func checksum(b []byte) uint32 {
	var s uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		s += binary.BigEndian.Uint32(w[:])
	}
	return s
}
