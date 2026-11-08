package stdfonts_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/language"

	"github.com/kuronosu/tela/internal/assets/fonts/inter"
	"github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
	"github.com/kuronosu/tela/internal/assets/fonts/notosansarabic"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/text"
)

// TestRegistered checks that importing the package registers the set as
// the default collection of internal/text.
func TestRegistered(t *testing.T) {
	c := text.NewDefaultCollection()
	if c == nil {
		t.Fatal("text.NewDefaultCollection is nil with stdfonts imported")
	}
	want := []string{stdfonts.Inter, stdfonts.JetBrainsMono, stdfonts.NotoSansArabic, stdfonts.NotoColorEmoji}
	if got := c.Families(); !slices.Equal(got, want) {
		t.Fatalf("Families() = %v, want %v", got, want)
	}
	if c == text.NewDefaultCollection() {
		t.Error("NewDefaultCollection returns the same collection twice")
	}
}

// TestSetResolution checks the generic families and the script fallback
// of the set: the mapping the goldens are drawn with.
func TestSetResolution(t *testing.T) {
	c := stdfonts.NewCollection()
	for generic, want := range map[string]string{
		"sans-serif": stdfonts.Inter, "system-ui": stdfonts.Inter, "ui-sans-serif": stdfonts.Inter,
		"monospace": stdfonts.JetBrainsMono, "ui-monospace": stdfonts.JetBrainsMono,
		"emoji": stdfonts.NotoColorEmoji,
		// Not mapped: the first registered family draws them.
		"serif": stdfonts.Inter, "ui-serif": stdfonts.Inter, "cursive": stdfonts.Inter,
	} {
		f := c.Face(text.Query{Families: []string{generic}}, 16)
		if f == nil || f.Typeface().Family() != want {
			t.Errorf("%s: %v, want %s", generic, f, want)
		}
	}
	rs := []rune("Hi مرحبا 😀")
	var got []string
	for _, r := range c.Itemize(rs, 0, len(rs), text.Query{Families: []string{"ui-sans-serif", "system-ui", "sans-serif"}}, 16, language.Latin, nil) {
		got = append(got, string(rs[r.Start:r.End])+"="+r.Face.Typeface().Family())
	}
	if want := "Hi =Inter|مرحبا=Noto Sans Arabic| =Inter|😀=Noto Color Emoji"; strings.Join(got, "|") != want {
		t.Errorf("runs %s, want %s", strings.Join(got, "|"), want)
	}
	// The sources are shared: a second collection gets the same faces.
	if a, b := c.Face(text.Query{Families: []string{"Inter"}}, 16), stdfonts.NewCollection().Face(text.Query{Families: []string{"Inter"}}, 16); a != b {
		t.Error("two collections of the set have different Inter faces")
	}
}

// The tests below check the embedded files themselves (they were the tests
// of internal/assets/fonts before it was split into one package per
// family).

type file struct {
	name string
	data []byte
}

func files() []file {
	return []file{
		{"Inter Variable", inter.Upright}, {"Inter Variable Italic", inter.Italic},
		{"JetBrains Mono", jetbrainsmono.Upright}, {"JetBrains Mono Italic", jetbrainsmono.Italic},
		{"Noto Sans Arabic", notosansarabic.Data}, {"Noto Color Emoji", notocoloremoji.Data},
	}
}

func parse(t *testing.T, data []byte) *font.Face {
	t.Helper()
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("font.ParseTTF: %v", err)
	}
	return face
}

// axes returns the variation axes of a font file, keyed by tag.
func axes(t *testing.T, data []byte) map[string]tables.VariationAxisRecord {
	t.Helper()
	ld, err := ot.NewLoader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("opentype.NewLoader: %v", err)
	}
	raw, err := ld.RawTable(ot.MustNewTag("fvar"))
	if err != nil {
		t.Fatalf("no fvar table (not a variable font): %v", err)
	}
	fvar, _, err := tables.ParseFvar(raw)
	if err != nil {
		t.Fatalf("parse fvar: %v", err)
	}
	m := map[string]tables.VariationAxisRecord{}
	for _, a := range fvar.FvarRecords.Axis {
		m[a.Tag.String()] = a
	}
	return m
}

func requireGlyphs(t *testing.T, face *font.Face, s string) {
	t.Helper()
	for _, r := range s {
		if gid, ok := face.NominalGlyph(r); !ok || gid == 0 {
			t.Errorf("no glyph for %q (U+%04X)", r, r)
		}
	}
}

func TestAllFilesParse(t *testing.T) {
	for _, f := range files() {
		t.Run(f.name, func(t *testing.T) {
			if len(f.data) < 100_000 {
				t.Fatalf("embedded data is only %d bytes", len(f.data))
			}
			parse(t, f.data)
		})
	}
	for _, s := range stdfonts.Sources() {
		tfs, err := s.Typefaces()
		if err != nil || len(tfs) != len(s.Data) {
			t.Errorf("%s: %d typefaces, %v", s.Family, len(tfs), err)
		}
	}
}

func TestInter(t *testing.T) {
	for _, f := range files()[:2] {
		a := axes(t, f.data)
		wght, ok := a["wght"]
		if !ok {
			t.Fatalf("%s has no wght axis (axes: %v)", f.name, a)
		}
		if wght.Minimum != 100 || wght.Maximum != 900 {
			t.Errorf("%s wght range = %v..%v, want 100..900", f.name, wght.Minimum, wght.Maximum)
		}
		if _, ok := a["opsz"]; !ok {
			t.Errorf("%s has no opsz axis", f.name)
		}
		requireGlyphs(t, parse(t, f.data), "AaZz09 .,;:!?@#&é ñ ü ß Ж → ✓")
	}
}

func TestJetBrainsMono(t *testing.T) {
	for _, f := range files()[2:4] {
		wght, ok := axes(t, f.data)["wght"]
		if !ok {
			t.Fatalf("%s has no wght axis", f.name)
		}
		if wght.Minimum != 100 || wght.Maximum != 800 {
			t.Errorf("%s wght range = %v..%v, want 100..800", f.name, wght.Minimum, wght.Maximum)
		}
		face := parse(t, f.data)
		requireGlyphs(t, face, "AaZz09 {}[]()<>=-+*/\\|_~`'\"é")
		// Monospace: all of these advance by the same amount.
		var want float32
		for i, r := range "iMW0. " {
			gid, ok := face.NominalGlyph(r)
			if !ok {
				t.Fatalf("no glyph for %q", r)
			}
			adv := face.HorizontalAdvance(gid)
			if i == 0 {
				want = adv
			} else if adv != want {
				t.Errorf("%s: advance of %q = %v, want %v (monospace)", f.name, r, adv, want)
			}
		}
	}
}

func TestNotoSansArabic(t *testing.T) {
	a := axes(t, notosansarabic.Data)
	for _, tag := range []string{"wght", "wdth"} {
		if _, ok := a[tag]; !ok {
			t.Errorf("Noto Sans Arabic has no %s axis (axes: %v)", tag, a)
		}
	}
	// alef, beh, teh, meem, lam, noon, waw, yeh, plus Arabic-Indic digits.
	requireGlyphs(t, parse(t, notosansarabic.Data), "ابتملنوي ٠١٢")
}

func TestNotoColorEmojiIsCOLRv1(t *testing.T) {
	data := notocoloremoji.Data
	ld, err := ot.NewLoader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"CBDT", "CBLC", "sbix"} {
		if ld.HasTable(ot.MustNewTag(tag)) {
			t.Errorf("font has a %s table: this is a bitmap build, want the COLRv1 build", tag)
		}
	}
	colr, err := ld.RawTable(ot.MustNewTag("COLR"))
	if err != nil {
		t.Fatalf("no COLR table: %v", err)
	}
	if v := binary.BigEndian.Uint16(colr); v != 1 {
		t.Fatalf("COLR table version = %d, want 1", v)
	}
	face := parse(t, data)
	if face.COLR == nil {
		t.Fatal("face.COLR is nil: COLRv1 was not loaded")
	}
	for _, r := range []rune{0x1F600, 0x1F44D, 0x2764, 0x1F680} {
		gid, ok := face.NominalGlyph(r)
		if !ok {
			t.Errorf("no glyph for U+%04X", r)
			continue
		}
		if _, ok := face.GlyphDataColor(gid); !ok {
			t.Errorf("U+%04X (glyph %d) has no COLR paint", r, gid)
		}
	}
}

// cjkDir is the test-only CJK subset font. It is not embedded anywhere.
const cjkDir = "../text/testdata/fonts"

func TestCJKSubset(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(cjkDir, "NotoSansJP-Subset.otf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 1<<20 {
		t.Errorf("CJK subset is %d bytes, want under 1 MiB", len(data))
	}
	face := parse(t, data)
	requireGlyphs(t, face, "日本語中文汉字，。、「」あいうえおかなカタカナ，！ＡＢＣ０１２abcXYZ")

	sample, err := os.ReadFile(filepath.Join(cjkDir, "cjk_sample.txt"))
	if err != nil {
		t.Fatal(err)
	}
	han := map[rune]bool{}
	for _, r := range string(sample) {
		if strings.ContainsRune(" \t\r\n", r) {
			continue
		}
		if r >= 0x4E00 && r <= 0x9FFF {
			han[r] = true
		}
		if gid, ok := face.NominalGlyph(r); !ok || gid == 0 {
			t.Errorf("sample character %q (U+%04X) is missing from the subset", r, r)
		}
	}
	if len(han) < 250 {
		t.Errorf("sample has only %d distinct Han characters, want at least 250", len(han))
	}
	// Whole blocks named in the README.
	for _, blk := range [][2]rune{{0x3041, 0x3096}, {0x30A1, 0x30FA}, {0x3001, 0x3003}} {
		for r := blk[0]; r <= blk[1]; r++ {
			if gid, ok := face.NominalGlyph(r); !ok || gid == 0 {
				t.Errorf("no glyph for U+%04X", r)
			}
		}
	}
}
