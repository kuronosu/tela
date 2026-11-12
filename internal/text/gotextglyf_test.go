package text

import (
	"bytes"
	"os"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts/inter"
	"github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
	"github.com/kuronosu/tela/internal/assets/fonts/notosansarabic"
	"github.com/kuronosu/tela/internal/assets/fonts/sourceserif4"
)

// systemGlyfFonts are TrueType fonts some machines have (TELA_SYSTEM_FONTS=1).
var systemGlyfFonts = []string{
	"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	"/usr/share/fonts/noto/NotoSans-Regular.ttf",
	"/usr/share/fonts/noto/NotoSansHebrew-Regular.ttf",
	"/usr/share/fonts/noto/NotoSansDevanagari-Regular.ttf",
	"/usr/share/fonts/Adwaita/AdwaitaSans-Regular.ttf",
	"/usr/share/fonts/Adwaita/AdwaitaMono-Regular.ttf",
	"/usr/share/fonts/TTF/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"C:\\Windows\\Fonts\\segoeui.ttf",
	"C:\\Windows\\Fonts\\arial.ttf",
}

// glyfTestFont is a font of TestGoTextOutlineStubs and the stubs it must
// get: a glyf of headers, and no gvar.
type glyfTestFont struct {
	name       string
	data       []byte
	glyf, gvar bool
	anyGvar    bool // an installed font: gvar is not checked
}

func glyfTestFonts(t *testing.T) []glyfTestFont {
	fonts := []glyfTestFont{
		{"Inter", inter.Upright, true, true, false},
		{"Inter Italic", inter.Italic, true, true, false},
		{"JetBrains Mono", jetbrainsmono.Upright, true, true, false},
		{"Noto Sans Arabic", notosansarabic.Data, true, true, false},
		{"Source Serif 4 Italic", sourceserif4.Italic, true, true, false},
		{"Noto Color Emoji (COLRv1, no GPOS)", notocoloremoji.Data, true, false, false},
	}
	// Variable fonts whose marks HarfBuzz may position from extents keep
	// glyf and gvar; a font with kerx keeps glyf (control point actions).
	fonts = append(fonts,
		glyfTestFont{"Inter without HVAR", renameTable(inter.Upright, "HVAR"), false, false, false},
		glyfTestFont{"Inter without GPOS", renameTable(inter.Upright, "GPOS"), false, false, false},
		glyfTestFont{"Inter with kerx", renameTableTo(inter.Upright, "STAT", "kerx"), false, false, false},
		glyfTestFont{"Inter without fvar", renameTable(inter.Upright, "fvar"), true, false, false},
	)
	if os.Getenv("TELA_SYSTEM_FONTS") == "1" { // installed fonts are opt-in (ADR-035)
		for _, f := range systemGlyfFonts {
			if data, err := os.ReadFile(f); err == nil {
				fonts = append(fonts, glyfTestFont{name: f, data: data, glyf: true, anyGvar: true})
			}
		}
	}
	return fonts
}

// renameTable hides table tag of the first font of data.
func renameTable(data []byte, tag string) []byte { return renameTableTo(data, tag, "zzzz") }

func renameTableTo(data []byte, tag, to string) []byte {
	out := bytes.Clone(data)
	for r := sfntDirectory(out, 0); len(r) >= 16; r = r[16:] {
		if string(r[:4]) == tag {
			copy(r, to)
		}
	}
	return out
}

// TestGoTextOutlineStubs checks that go-text, given the stubs of glyf and
// loca and no gvar (goTextFont), reads the same font as from the file: the
// glyph extents, advances, font extents, line metrics and shaping output
// at the default instance (compareGoTextFonts), and the advances and the
// shaping output at another instance, with combining marks.
func TestGoTextOutlineStubs(t *testing.T) {
	for _, tc := range glyfTestFonts(t) {
		lds, err := ot.NewLoaders(bytes.NewReader(tc.data))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		ld := lds[0]
		src := newTableSource(ld, tc.data, 0)
		tf, err := newTypeface(src, "")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, out, err := goTextFontOutlines(src, tf)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if (out.glyf != nil) != tc.glyf || (out.hideGvar != tc.gvar && !tc.anyGvar) {
			t.Errorf("%s: glyf stub %v, gvar hidden %v; want %v, %v", tc.name, out.glyf != nil, out.hideGvar, tc.glyf, tc.gvar)
		}
		if out.glyf != nil && len(out.glyf) > 12*tf.nGlyphs {
			t.Errorf("%s: glyf stub of %d bytes for %d glyphs", tc.name, len(out.glyf), tf.nGlyphs)
		}
		want, err := font.NewFont(goTextLoader(src, goTextOutlines{}))
		if err != nil {
			t.Fatal(err)
		}
		compareGoTextFonts(t, tc.name, tf.nGlyphs, want, got)

		if len(tf.axes) == 0 {
			continue
		}
		// Every axis at its maximum, then at its minimum.
		for _, end := range []int{0, 1} {
			design := make([]float32, len(tf.axes))
			for i, a := range tf.axes {
				design[i] = [2]float32{a.Max, a.Min}[end]
			}
			coords := want.NormalizeVariations(design)
			wf, gf := font.NewFace(want), font.NewFace(got)
			wf.SetCoords(coords)
			gf.SetCoords(coords)
			for g := range tf.nGlyphs {
				if w, a := wf.HorizontalAdvance(font.GID(g)), gf.HorizontalAdvance(font.GID(g)); w != a {
					t.Fatalf("%s at %v: glyph %d: advance %v, want %v", tc.name, design, g, a, w)
				}
			}
			compareShaping(t, tc.name, want, wf, gf)
		}
	}
}

// compareShaping shapes the characters of ft's cmap, each alone and
// followed by combining marks, with two faces, as one run of the script of
// its first letter (Hebrew runs: TestHebrewRunsShapeWithOutlines).
func compareShaping(t *testing.T, name string, ft *font.Font, wf, gf *font.Face) {
	t.Helper()
	var text []rune
	it := ft.Cmap.Iter()
	for it.Next() {
		r, _ := it.Char()
		text = append(text, r, ' ', r, 0x0301, 0x0323, ' ')
	}
	shape := func(f *font.Face) *harfbuzz.Buffer {
		buf := harfbuzz.NewBuffer()
		buf.AddRunes(text, 0, -1)
		buf.GuessSegmentProperties()
		buf.Shape(harfbuzz.NewFont(f), nil)
		return buf
	}
	wb, gb := shape(wf), shape(gf)
	if len(wb.Info) != len(gb.Info) {
		t.Fatalf("%s: shaped %d glyphs, want %d", name, len(gb.Info), len(wb.Info))
	}
	for i := range wb.Info {
		if wb.Info[i].Glyph != gb.Info[i].Glyph || wb.Pos[i] != gb.Pos[i] {
			t.Errorf("%s: shaped glyph %d: %d %+v, want %d %+v", name, i, gb.Info[i].Glyph, gb.Pos[i], wb.Info[i].Glyph, wb.Pos[i])
			return
		}
	}
}

// goTextGlyfStub keeps each glyph's header, and empty glyphs empty.
func TestGoTextGlyfStub(t *testing.T) {
	tfs, err := ParseTypefaces(inter.Upright, "")
	if err != nil {
		t.Fatal(err)
	}
	g := tfs[0].glyf
	glyf, loca, ok := goTextGlyfStub(g)
	if !ok {
		t.Fatal("no stub")
	}
	stub, err := tables.ParseLoca(loca, g.nGlyphs, g.longLoca)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := tables.ParseGlyf(glyf, stub)
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.nGlyphs {
		w := g.glyph(tables.GlyphID(i))
		p := parsed[i]
		if w.XMin != p.XMin || w.YMin != p.YMin || w.XMax != p.XMax || w.YMax != p.YMax || (w.Data == nil) != (p.Data == nil) {
			t.Fatalf("glyph %d: %+v, want the header of %+v", i, p, w)
		}
	}
}

// A Hebrew run at a variable instance of a font whose GPOS has no Hebrew
// script has its marks positioned from glyph extents (the Hebrew shaper
// disables that GPOS): it is shaped with the full go-text font, so as
// without the stubs.
func TestHebrewRunsShapeWithOutlines(t *testing.T) {
	tfs, err := ParseTypefaces(inter.Upright, "")
	if err != nil {
		t.Fatal(err)
	}
	tf := tfs[0]
	if !tf.goTextNoGvar || tf.ft.GPOS.FindScript(tagHebr) >= 0 {
		t.Fatal("Inter: want a font without gvar for go-text and no Hebrew script in GPOS")
	}
	lds, err := ot.NewLoaders(bytes.NewReader(inter.Upright))
	if err != nil {
		t.Fatal(err)
	}
	full, err := font.NewFont(lds[0])
	if err != nil {
		t.Fatal(err)
	}
	text := []rune("1\u0301 a\u0323\u0301, 2")
	shape := func(hb *harfbuzz.Font, script language.Script) *harfbuzz.Buffer {
		buf := harfbuzz.NewBuffer()
		buf.Props = harfbuzz.SegmentProperties{Direction: harfbuzz.RightToLeft, Script: script}
		buf.AddRunes(text, 0, -1)
		buf.Shape(hb, nil)
		return buf
	}
	same := func(a, b *harfbuzz.Buffer) bool {
		if len(a.Info) != len(b.Info) {
			return false
		}
		for i := range a.Info {
			if a.Info[i].Glyph != b.Info[i].Glyph || a.Pos[i] != b.Pos[i] {
				return false
			}
		}
		return true
	}
	shapeMu.Lock()
	defer shapeMu.Unlock()
	for _, w := range []float32{400, 700, 900} {
		f := tf.Face(Instance{Weight: w})
		ref := font.NewFace(full)
		ref.SetCoords(f.coords)
		want := shape(harfbuzz.NewFont(ref), language.Hebrew)
		if got := shape(f.hbFontFor(language.Hebrew), language.Hebrew); !same(got, want) {
			t.Errorf("wght %g: the Hebrew run is not shaped as with the outlines", w)
		}
		if f.hbFontFor(language.Latin) != f.hbFont() {
			t.Errorf("wght %g: a Latin run does not use the face's font", w)
		}
		if w == 700 && same(shape(f.hbFont(), language.Hebrew), want) {
			t.Error("the stubs do not change the Hebrew run: the test tests nothing")
		}
	}
}
