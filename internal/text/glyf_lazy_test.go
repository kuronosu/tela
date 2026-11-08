package text

import (
	"bytes"
	"encoding/binary"
	"os"
	"reflect"
	"sync"
	"testing"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/gfx"
)

// lazyDigestFaces are the faces whose every glyph lazyOutlineDigest hashes:
// each embedded typeface at its default instance (Noto Color Emoji
// included: its glyf has the layers of the color glyphs), and the variable
// fonts at instances far from their defaults.
func lazyDigestFaces() []*Face {
	c := NewDefaultCollection()
	var faces []*Face
	for _, tfs := range stdTypefaces() {
		for _, tf := range tfs {
			faces = append(faces, tf.Face(Instance{}))
		}
	}
	inter := c.Match(FamilyInter, Query{})
	interItalic := c.Match(FamilyInter, Query{Style: StyleItalic})
	return append(faces,
		inter.Face(Instance{Weight: 100}),
		inter.Face(Instance{Weight: 400, OpticalSize: 32}),
		inter.Face(Instance{Weight: 900, OpticalSize: 14}),
		interItalic.Face(Instance{Weight: 200}),
		interItalic.Face(Instance{Weight: 800, OpticalSize: 28}),
		c.Match(FamilyNotoSansArabic, Query{}).Face(Instance{Weight: 900, Stretch: 62.5}),
		c.Match(FamilyJetBrainsMono, Query{}).Face(Instance{Weight: 800}),
	)
}

// lazyOutlineDigest hashes the float64 glyf points, the em-space outline,
// the advance and the bounds of every glyph of lazyDigestFaces.
func lazyOutlineDigest(t *testing.T) uint64 {
	d := newDigest()
	for _, f := range lazyDigestFaces() {
		tf := f.tf
		if tf.glyf == nil {
			t.Fatalf("%s: not a glyf font", f.FontName())
		}
		d.h.Write([]byte(f.FontName()))
		for g := 0; g < tf.nGlyphs; g++ {
			for _, p := range tf.glyf.points(tables.GlyphID(g), f.coords) {
				d.f64(p.x, p.y)
			}
			p, _ := f.GlyphOutline(gfx.GlyphID(g))
			d.path(p)
			b := f.entry(gfx.GlyphID(g)).bounds
			d.f32(f.Advance(gfx.GlyphID(g)), b.X0, b.Y0, b.X1, b.Y1)
		}
	}
	return d.h.Sum64()
}

// wantLazyOutlineDigest was recorded with the eager parse of glyf and gvar
// (tables.ParseGlyf and tables.ParseGvar of the whole tables, before the
// outlines were loaded on demand): the lazy parse gives the same points,
// outlines, advances and bounds for every glyph.
const wantLazyOutlineDigest uint64 = 0xcdb248552677e4eb

func TestLazyOutlinesMatchEager(t *testing.T) {
	if got := lazyOutlineDigest(t); got != wantLazyOutlineDigest {
		t.Errorf("lazyOutlineDigest = %#x, want %#x (recorded with the eager parse)", got, wantLazyOutlineDigest)
	}
}

// TestLazyGlyfParse checks the per-glyph parse against go-text's parse of
// the whole tables, for every glyph of every embedded font: the glyph
// (simple, composite or empty), its gvar variation data and the shared
// tuples, and the hmtx metrics.
func TestLazyGlyfParse(t *testing.T) {
	for _, f := range allStdFonts {
		data := f.data()
		tfs, err := ParseTypefaces(data, "")
		if err != nil {
			t.Fatal(err)
		}
		tf := tfs[0]
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		raw := func(tag string) []byte {
			b, _ := lds[0].RawTable(ot.MustNewTag(tag))
			return b
		}
		name := f.String()
		if tf.glyf == nil {
			t.Fatalf("%s: no glyf outlines", name)
		}
		loca, err := tables.ParseLoca(raw("loca"), tf.nGlyphs, tf.head.IndexToLocFormat == 1)
		if err != nil {
			t.Fatal(err)
		}
		glyf, err := tables.ParseGlyf(raw("glyf"), loca)
		if err != nil {
			t.Fatal(err)
		}
		var simple, composite int
		for g := range glyf {
			got := *tf.glyf.glyph(tables.GlyphID(g))
			if !reflect.DeepEqual(got, glyf[g]) {
				t.Fatalf("%s: glyph %d: lazy parse %+v, eager %+v", name, g, got, glyf[g])
			}
			switch glyf[g].Data.(type) {
			case tables.SimpleGlyph:
				simple++
			case tables.CompositeGlyph:
				composite++
			}
		}
		t.Logf("%s: %d glyphs, %d simple, %d composite", name, len(glyf), simple, composite)
		if f != fontNotoColorEmoji && composite == 0 {
			t.Errorf("%s: no composite glyph", name)
		}

		hhea, _, _ := tables.ParseHhea(raw("hhea"))
		n := int(hhea.NumOfLongMetrics)
		hm, _, err := tables.ParseHmtx(raw("hmtx"), n, tf.nGlyphs-n)
		if err != nil {
			t.Fatal(err)
		}
		for g := 0; g <= tf.nGlyphs; g++ { // one past the end: 0
			gid := tables.GlyphID(g)
			if tf.hmtx.Advance(gid) != hm.Advance(gid) || tf.hmtx.SideBearing(gid) != hm.SideBearing(gid) {
				t.Fatalf("%s: hmtx of glyph %d: %d, %d; want %d, %d", name, g,
					tf.hmtx.Advance(gid), tf.hmtx.SideBearing(gid), hm.Advance(gid), hm.SideBearing(gid))
			}
		}

		if len(tf.axes) == 0 {
			if tf.glyf.gvar != nil {
				t.Errorf("%s: gvar on a font without axes", name)
			}
			continue
		}
		gvar, _, err := tables.ParseGvar(raw("gvar"))
		if err != nil {
			t.Fatal(err)
		}
		v := tf.glyf.gvar
		if v == nil {
			t.Fatalf("%s: no gvar", name)
		}
		var shared [][]tables.Coord
		for _, st := range gvar.SharedTuples.SharedTuples {
			shared = append(shared, st.Values)
		}
		if !reflect.DeepEqual(v.sharedTuples, shared) {
			t.Errorf("%s: shared tuples differ", name)
		}
		varied := 0
		for g, want := range gvar.GlyphVariationDatas {
			got, ok := v.glyphData(tables.GlyphID(g))
			if ok {
				varied++
			}
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(*tf.glyf.variation(tables.GlyphID(g)), want) {
				t.Fatalf("%s: glyph %d: lazy variation data %+v, eager %+v", name, g, got, want)
			}
		}
		if varied == 0 {
			t.Errorf("%s: no glyph has variation data", name)
		}
	}
}

// TestTableSource checks that the tables sliced from the file are the
// bytes go-text's Loader copies, for the embedded fonts, a CFF font and the
// fonts of a TrueType collection, and that a WOFF file is read through the
// Loader.
func TestTableSource(t *testing.T) {
	cjk, err := os.ReadFile("testdata/fonts/NotoSansJP-Subset.otf")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"NotoSansJP-Subset.otf": cjk}
	for _, f := range allStdFonts {
		files[f.String()] = f.data()
	}
	files["collection"] = makeCollection(fontJetBrainsMono.data(), cjk, fontInter.data())
	for name, data := range files {
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			t.Fatal(name, err)
		}
		for i, ld := range lds {
			src := newTableSource(ld, data, i)
			if src.data == nil {
				t.Fatalf("%s (%d): tables read through the Loader", name, i)
			}
			for _, tag := range append(ld.Tables(), ot.MustNewTag("ZZZZ")) {
				want, _ := ld.RawTable(tag)
				if got := src.table(tag); !bytes.Equal(got, want) || (got == nil) != (want == nil) {
					t.Errorf("%s (%d): table %s: %d bytes, want %d", name, i, tag, len(got), len(want))
				}
			}
		}
		if name == "collection" {
			if len(lds) != 3 {
				t.Fatalf("collection: %d fonts", len(lds))
			}
			tfs, err := ParseTypefaces(data, "")
			if err != nil || len(tfs) != 3 || tfs[0].glyf == nil || tfs[1].glyf != nil || tfs[2].glyf == nil || tfs[2].glyf.gvar == nil {
				t.Fatalf("collection: typefaces %v, %v", tfs, err)
			}
		}
	}
	if sfntDirectory([]byte("wOFF\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), 0) != nil {
		t.Error("a WOFF file has an sfnt directory")
	}
	if sfntDirectory(files["collection"], 3) != nil || sfntDirectory(fontInter.data(), 1) != nil {
		t.Error("a font index past the end has a directory")
	}
}

// makeCollection builds a TrueType collection ('ttcf') of sfnt files: each
// file is copied whole, 4-byte aligned, with its table offsets moved by its
// position.
func makeCollection(files ...[]byte) []byte {
	out := make([]byte, 12+4*len(files))
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], uint32(len(files)))
	for i, f := range files {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		base := len(out)
		binary.BigEndian.PutUint32(out[12+4*i:], uint32(base))
		out = append(out, f...)
		n := int(binary.BigEndian.Uint16(f[4:]))
		for r := 0; r < n; r++ {
			o := base + 12 + 16*r + 8
			binary.BigEndian.PutUint32(out[o:], binary.BigEndian.Uint32(out[o:])+uint32(base))
		}
	}
	return out
}

// TestGlyphSlots checks the paged slot table: out-of-range glyphs, pages
// allocated on first use, and concurrent loads that agree on one page.
func TestGlyphSlots(t *testing.T) {
	s := newGlyphSlots[int](3*slotPage + 5)
	if s.slot(-1) != nil || s.slot(3*slotPage+5) != nil {
		t.Fatal("out-of-range slot")
	}
	if len(s.pages) != 4 {
		t.Fatalf("%d pages, want 4", len(s.pages))
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for g := 0; g < s.n; g++ {
				v := g
				s.slot(g).CompareAndSwap(nil, &v)
			}
		})
	}
	wg.Wait()
	for g := 0; g < s.n; g++ {
		if p := s.slot(g).Load(); p == nil || *p != g {
			t.Fatalf("slot %d: %v", g, p)
		}
	}
	e := newGlyphSlots[int](0)
	if e.slot(0) != nil {
		t.Error("slot of an empty table")
	}
}
