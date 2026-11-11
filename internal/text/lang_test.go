package text

import (
	"fmt"
	"image"
	"os"
	"testing"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// loclText holds characters whose forms differ between the Japanese,
// Chinese and Korean conventions in Noto Sans CJK (its locl lookups for
// ZHS, ZHT, ZHH and KOR) and that the test subset keeps.
const loclText = "返漢禁全新選遠花公語"

var loclLangs = []string{"ja", "zh-Hans", "zh-Hant", "zh-HK", "ko"}

// glyphIDs returns the glyphs of the paragraph's text in one language.
func glyphIDs(t *testing.T, lang string) []gfx.GlyphID {
	t.Helper()
	l := simple(t, loclText, ParagraphStyle{}, TextStyle{Families: []string{"Noto Sans JP"}, Size: 24, Language: lang}, 1000)
	var out []gfx.GlyphID
	for _, g := range l.glyphs {
		out = append(out, g.id)
	}
	return out
}

func TestLanguageSelectsLocalForms(t *testing.T) {
	ja := glyphIDs(t, "ja")
	if len(ja) != len([]rune(loclText)) {
		t.Fatalf("%d glyphs for %d characters", len(ja), len([]rune(loclText)))
	}
	if und := glyphIDs(t, ""); fmt.Sprint(und) != fmt.Sprint(ja) {
		t.Errorf("no language %v, want the font's default (Japanese) forms %v", und, ja)
	}
	for _, lang := range loclLangs[1:] {
		got := glyphIDs(t, lang)
		diff := 0
		for i := range got {
			if got[i] != ja[i] {
				diff++
			}
		}
		if diff == 0 {
			t.Errorf("%s: same glyphs as Japanese %v", lang, got)
		}
	}
	// 返 has a form of its own in each of the five conventions.
	seen := map[gfx.GlyphID]string{}
	for _, lang := range loclLangs {
		g := glyphIDs(t, lang)[0]
		if prev, ok := seen[g]; ok {
			t.Errorf("返: %s and %s share glyph %d", prev, lang, g)
		}
		seen[g] = lang
	}
}

func TestGoldenLanguageForms(t *testing.T) {
	c := layoutCollection(t)
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	muted := gfx.RGB8(0x6b, 0x72, 0x80)
	b := gfx.NewBuilder()
	y := float32(12)
	for _, lang := range loclLangs {
		lab := NewLayout(c, &Paragraph{Text: lang, Default: TextStyle{Families: []string{"Inter"}, Size: 13, Paint: muted, LineHeight: LineHeightPixels(40)}})
		lab.BreakLines(80)
		lab.Paint(b, gfx.Pt(12, y))
		l := NewLayout(c, &Paragraph{Text: loclText, Default: TextStyle{Families: []string{"Inter"}, Size: 28, Paint: ink, LineHeight: LineHeightPixels(40), Language: lang}})
		l.BreakLines(400)
		l.Paint(b, gfx.Pt(80, y))
		y += 40
	}
	dl := b.Finish()
	const w, h = 380, 12 + 5*40 + 8
	img := image.NewRGBA(image.Rect(0, 0, w*2, h*2))
	raster.Clear(img, image.Rectangle{}, gfx.RGB8(0xf9, 0xfa, 0xfb))
	raster.Render(img, dl, raster.Options{Scale: 2})
	golden.Image(t, "language_forms_x2", img)
}

// TestLanguageOrdersHanFallback registers two regional families for Han
// (the same subset under two names) and checks that the text's language
// puts its region first, the registration order deciding otherwise.
func TestLanguageOrdersHanFallback(t *testing.T) {
	data, err := os.ReadFile("testdata/fonts/NotoSansJP-Subset.otf")
	if err != nil {
		t.Fatal(err)
	}
	c := NewCollection()
	if _, err := c.Register(fontInter.data(), "Inter"); err != nil {
		t.Fatal(err)
	}
	for _, fam := range []string{"Noto Sans KR", "Noto Sans JP", "Noto Sans SC", "Noto Sans TC"} {
		if _, err := c.Register(data, fam); err != nil {
			t.Fatal(err)
		}
	}
	c.SetScriptFallback(language.Han, "Noto Sans KR", "Noto Sans JP", "Noto Sans SC", "Noto Sans TC")
	c.SetScriptFallback(language.Hiragana, "Noto Sans KR", "Noto Sans JP", "Noto Sans SC", "Noto Sans TC")
	for _, tc := range []struct{ lang, text, want string }{
		{"", "漢", "Noto Sans KR"},
		{"en", "漢", "Noto Sans KR"},
		{"ja", "漢", "Noto Sans JP"},
		{"zh-Hans", "漢", "Noto Sans SC"},
		{"zh-CN", "漢", "Noto Sans SC"},
		{"zh-TW", "漢", "Noto Sans TC"},
		{"zh-Hant-HK", "漢", "Noto Sans TC"}, // no HK family: the closest region
		{"ko-KR", "漢", "Noto Sans KR"},
		// Kana are Japanese whatever the language.
		{"zh-Hans", "の", "Noto Sans JP"},
	} {
		rs := []rune(tc.text)
		runs := c.Itemize(rs, 0, len(rs), Query{Families: []string{"Inter"}, Lang: tc.lang}, 16, language.LookupScript(rs[0]), nil)
		if got := runs[0].Face.Typeface().Family(); got != tc.want {
			t.Errorf("%q in %q: %s, want %s", tc.text, tc.lang, got, tc.want)
		}
	}
}

func TestCJKRegion(t *testing.T) {
	for lang, want := range map[string]string{
		"ja": "jp", "ja-JP": "jp", "ko": "kr", "zh": "sc", "zh-Hans": "sc", "zh-CN": "sc", "zh-SG": "sc",
		"zh-Hant": "tc", "zh-TW": "tc", "zh-MO": "tc", "zh-HK": "hk", "zh-Hant-HK": "hk", "zh-Hans-HK": "sc",
		"yue": "hk", "zh_TW.UTF-8": "tc", "en": "", "": "", "es-MX": "",
	} {
		if got := cjkRegion(lang); got != want {
			t.Errorf("%q: %q, want %q", lang, got, want)
		}
	}
}
