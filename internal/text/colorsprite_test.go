package text

import (
	"image"
	"testing"

	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// fgBlindFont hides gfx.ForegroundColorFont from the rasterizer: its color
// glyphs are cached per text color, as before the interface.
type fgBlindFont struct{ gfx.SizedColorFont }

// emojiColors are the text colors of the page of TestColorSpritesShared.
var emojiColors = []gfx.Color{
	gfx.Black, gfx.White, gfx.RGB8(239, 68, 68), gfx.RGB8(59, 130, 246),
	gfx.RGB8(34, 197, 94), gfx.RGB8(168, 85, 247), {R: 0.2, G: 0.2, B: 0.2, A: 0.5}, {R: 1, A: 0.25},
}

// emojiPage renders one emoji of f in each of emojiColors, on a line of
// its own, at 24 px and display scale 2, into cache.
func emojiPage(f gfx.Font, g gfx.GlyphID, cache *raster.GlyphCache) *image.RGBA {
	b := gfx.NewBuilder()
	for i, c := range emojiColors {
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: 24, Origin: gfx.Pt(4, float32(28+32*i)), Paint: c,
			Glyphs: []gfx.Glyph{{ID: g}}})
	}
	img := image.NewRGBA(image.Rect(0, 0, 80, 2*32*len(emojiColors)))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{Scale: 2, Workers: 1, GlyphCache: cache})
	return img
}

// A COLR emoji and a CBDT emoji drawn in several text colors are rendered
// and cached once, with the pixels of sprites keyed by the color (a
// translucent color still fades them).
func TestColorSpritesShared(t *testing.T) {
	colr := NewDefaultCollection().Face(Query{Families: []string{"emoji"}}, 24)
	cbdt := bitmapTypeface(t, cbdtTestFile, familyCBDT).Face(Instance{})
	for _, tc := range []struct {
		name string
		f    *Face
	}{{"COLRv1", colr}, {"CBDT", cbdt}} {
		g, ok := tc.f.GlyphIndex('😀')
		if !ok {
			t.Fatalf("%s: no glyph for U+1F600", tc.name)
		}
		if tc.f.ColorGlyphUsesForeground(g) {
			t.Errorf("%s: 😀 uses the foreground", tc.name)
		}
		shared, perColor := raster.NewGlyphCache(8<<20), raster.NewGlyphCache(8<<20)
		got := emojiPage(tc.f, g, shared)
		want := emojiPage(fgBlindFont{tc.f}, g, perColor)
		if !equalPix(got, want) {
			t.Errorf("%s: the shared sprite changes the pixels", tc.name)
		}
		// 1 color marker + 1 sprite (the pen is at the same quarter pixel
		// on every line).
		if n := shared.Len(); n != 2 {
			t.Errorf("%s: %d cache entries for %d colors, want 2", tc.name, n, len(emojiColors))
		}
		t.Logf("%s, one emoji in %d colors: %d entries, %d bytes (keyed by color: %d entries, %d bytes)",
			tc.name, len(emojiColors), shared.Len(), shared.Bytes(), perColor.Len(), perColor.Bytes())
	}
}

func equalPix(a, b *image.RGBA) bool {
	if a.Rect != b.Rect || len(a.Pix) != len(b.Pix) {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// A COLR glyph whose graph paints with palette index 0xFFFF uses the
// foreground; outline glyphs and bitmap glyphs do not.
func TestColorGlyphUsesForeground(t *testing.T) {
	// A typeface of its own: the shared one must not be changed.
	tfs, err := ParseTypefaces(notocoloremoji.Data, "")
	if err != nil {
		t.Fatal(err)
	}
	f := tfs[0].Face(Instance{})
	smile, _ := f.GlyphIndex('😀')
	// Make another glyph a COLRv0 glyph of the smile's outline in the
	// foreground color (the emoji font has none).
	g, ok := f.GlyphIndex('😁')
	if !ok || g == smile {
		t.Fatal("no second emoji")
	}
	var outline gfx.GlyphID // a layer glyph: one with an outline
	for i := range gfx.GlyphID(f.Typeface().NumGlyphs()) {
		if _, ok := f.GlyphOutline(i); ok {
			outline = i
			break
		}
	}
	layer := tables.PaintColrLayersResolved{{GlyphID: tables.GlyphID(outline), PaletteIndex: 0xFFFF}}
	f.colors.slot(int(g)).Store(&colorEntry{paint: layer, bounds: gfx.Rect{X0: 0, Y0: -1, X1: 1, Y1: 0}})
	if !f.ColorGlyphUsesForeground(g) {
		t.Error("a COLRv0 layer in palette index 0xFFFF does not use the foreground")
	}
	if a, b := f.ColorGlyph(g, gfx.Black), f.ColorGlyph(g, gfx.White); a == b {
		t.Error("one list for two foregrounds")
	}
	inter := NewDefaultCollection().Match("Inter", Query{}).Face(Instance{})
	if a, _ := inter.GlyphIndex('a'); inter.ColorGlyphUsesForeground(a) {
		t.Error("an outline glyph uses the foreground")
	}
}
