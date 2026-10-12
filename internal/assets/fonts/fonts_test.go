package fonts_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"

	"github.com/kuronosu/tela/internal/assets/fonts"
)

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

func TestAllEmbeddedFontsParse(t *testing.T) {
	all := fonts.All()
	if len(all) != 6 {
		t.Fatalf("All() returned %d fonts, want 6", len(all))
	}
	for _, f := range all {
		t.Run(fonts.Name(f), func(t *testing.T) {
			data := fonts.Data(f)
			if len(data) < 100_000 {
				t.Fatalf("embedded data is only %d bytes", len(data))
			}
			parse(t, data)
		})
	}
}

func TestDataPanicsOnUnknownFont(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Data(0) did not panic")
		}
	}()
	fonts.Data(0)
}

func TestInter(t *testing.T) {
	for _, f := range []fonts.Font{fonts.InterVariable, fonts.InterVariableItalic} {
		data := fonts.Data(f)
		a := axes(t, data)
		wght, ok := a["wght"]
		if !ok {
			t.Fatalf("%s has no wght axis (axes: %v)", fonts.Name(f), a)
		}
		if wght.Minimum != 100 || wght.Maximum != 900 {
			t.Errorf("%s wght range = %v..%v, want 100..900", fonts.Name(f), wght.Minimum, wght.Maximum)
		}
		if _, ok := a["opsz"]; !ok {
			t.Errorf("%s has no opsz axis", fonts.Name(f))
		}
		face := parse(t, data)
		requireGlyphs(t, face, "AaZz09 .,;:!?@#&é ñ ü ß Ж → ✓")
	}
}

func TestJetBrainsMono(t *testing.T) {
	for _, f := range []fonts.Font{fonts.JetBrainsMono, fonts.JetBrainsMonoItalic} {
		data := fonts.Data(f)
		wght, ok := axes(t, data)["wght"]
		if !ok {
			t.Fatalf("%s has no wght axis", fonts.Name(f))
		}
		if wght.Minimum != 100 || wght.Maximum != 800 {
			t.Errorf("%s wght range = %v..%v, want 100..800", fonts.Name(f), wght.Minimum, wght.Maximum)
		}
		face := parse(t, data)
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
				t.Errorf("%s: advance of %q = %v, want %v (monospace)", fonts.Name(f), r, adv, want)
			}
		}
	}
}

func TestNotoSansArabic(t *testing.T) {
	data := fonts.Data(fonts.NotoSansArabic)
	a := axes(t, data)
	for _, tag := range []string{"wght", "wdth"} {
		if _, ok := a[tag]; !ok {
			t.Errorf("Noto Sans Arabic has no %s axis (axes: %v)", tag, a)
		}
	}
	face := parse(t, data)
	// alef, beh, teh, meem, lam, noon, waw, yeh, plus Arabic-Indic digits.
	requireGlyphs(t, face, "ابتملنوي ٠١٢")
}

func TestNotoColorEmojiIsCOLRv1(t *testing.T) {
	data := fonts.Data(fonts.NotoColorEmoji)
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
const cjkDir = "../../text/testdata/fonts"

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
