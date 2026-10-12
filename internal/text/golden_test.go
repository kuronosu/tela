package text

import (
	"image"
	"os"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// testCollection returns the default collection plus the test-only CJK
// subset, registered for Han, Hiragana and Katakana.
func testCollection(t testing.TB) *Collection {
	t.Helper()
	c := NewDefaultCollection()
	data, err := os.ReadFile("testdata/fonts/NotoSansJP-Subset.otf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Register(data, "Noto Sans JP"); err != nil {
		t.Fatal(err)
	}
	c.SetScriptFallback(language.Han, "Noto Sans JP")
	return c
}

// glyphsOf maps text to glyphs through the cmap and lays them out with the
// face's advances (no shaping), starting at x = 0.
func glyphsOf(f *Face, size float32, text string) ([]gfx.Glyph, float32) {
	var gs []gfx.Glyph
	x := float32(0)
	for _, r := range text {
		g, ok := f.GlyphIndex(r)
		if !ok {
			continue
		}
		gs = append(gs, gfx.Glyph{ID: g, X: x})
		x += f.Advance(g) * size
	}
	return gs, x
}

// shapedGlyph returns the single glyph that go-text's shaper produces for
// an emoji sequence (flags, skin tones, ZWJ sequences); tests use it to
// reach glyphs that only GSUB maps.
func shapedGlyph(t testing.TB, f *Face, seq string) gfx.GlyphID {
	t.Helper()
	rs := []rune(seq)
	out := (&shaping.HarfbuzzShaper{}).Shape(shaping.Input{
		Text: rs, RunEnd: len(rs), Direction: di.DirectionLTR,
		Face: f.ShapingFace(), Size: 64 << 6, Script: language.Common,
	})
	if len(out.Glyphs) != 1 {
		t.Fatalf("shaping %q: %d glyphs, want 1", seq, len(out.Glyphs))
	}
	return gfx.GlyphID(out.Glyphs[0].GlyphID)
}

// goldenEmoji are the emoji of the golden and of the Chromium comparison:
// linear, radial and two-point radial gradients, transforms, SrcIn
// composites (⚠ 📖 💻) and, through the flag, SoftLight composites.
var goldenEmoji = []string{"😀", "😍", "🤔", "🔥", "🎉", "🌍", "🤩", "🌏", "⚠️", "📖", "💻", "🇯🇵"}

// emojiGlyphs returns the glyphs of goldenEmoji at size, spaced by 1.15 em.
func emojiGlyphs(t testing.TB, f *Face, size float32) []gfx.Glyph {
	var gs []gfx.Glyph
	for i, e := range goldenEmoji {
		gs = append(gs, gfx.Glyph{ID: shapedGlyph(t, f, e), X: float32(i) * size * 1.15})
	}
	return gs
}

func fontsScene(t testing.TB) (*gfx.DisplayList, int, int) {
	c := testCollection(t)
	b := gfx.NewBuilder()
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	y := float32(0)
	line := func(f *Face, size float32, text string) {
		y += size * 1.3
		gs, _ := glyphsOf(f, size, text)
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: size, Origin: gfx.Pt(16, y), Glyphs: gs, Paint: ink})
	}
	const latin = "Sphinx of black quartz, judge my vow 0123 & ffi"
	for _, w := range []float32{300, 400, 700} {
		line(c.Face(Query{Families: []string{"sans-serif"}, Weight: w}, 24), 24, latin)
	}
	line(c.Face(Query{Families: []string{"sans-serif"}, Style: StyleItalic}, 24), 24, latin)
	line(c.Face(Query{Families: []string{"Inter"}, Weight: 650}, 20), 20, "Inter 650 at 20 px (opsz 20): Hamburgefonstiv")
	line(c.Face(Query{Families: []string{"monospace"}}, 20), 20, "func main() { x := []int{1, 2}; return x[0] != 0 } // ->")
	line(c.Face(Query{Families: []string{"Noto Sans Arabic"}, Weight: 600, Stretch: 87.5}, 32), 32, "ا ب ت ث ج ح خ د ذ ر ز س ش ص ض ط ظ ع غ ف ق ك ل م ن ه و ي")
	line(c.Face(Query{Families: []string{"Noto Sans JP"}}, 32), 32, "日本語の文章、漢字かなカナ。中文简体字们说")

	emoji := c.Face(Query{Families: []string{"emoji"}}, 64)
	y += 80
	b.DrawGlyphRun(gfx.DrawGlyphRun{Font: emoji, Size: 64, Origin: gfx.Pt(16, y), Glyphs: emojiGlyphs(t, emoji, 64), Paint: ink})
	y += 32
	b.DrawGlyphRun(gfx.DrawGlyphRun{Font: emoji, Size: 20, Origin: gfx.Pt(16, y), Glyphs: emojiGlyphs(t, emoji, 20), Paint: ink})
	return b.Finish(), 920, int(y + 16)
}

func TestGoldenFonts(t *testing.T) {
	dl, w, h := fontsScene(t)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, dl, raster.Options{})
	golden.Image(t, "fonts", img)
}
