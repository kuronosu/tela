package text

import (
	"testing"

	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/assets/fonts"
)

// BenchmarkParseEmbedded parses every embedded font file one after another:
// the start-up cost of the fonts (embeddedTypefaces parses them in
// parallel).
func BenchmarkParseEmbedded(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for _, f := range fonts.All() {
			if _, err := ParseTypefaces(fonts.Data(f), ""); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// benchGlyphLoad loads the outline of every glyph of f, bypassing the
// face's cache: what a frame pays for each glyph it draws for the first
// time in a face. The typeface's parsed glyphs stay cached between
// iterations, as for a second instance of a typeface; cold drops them each
// iteration, as for the first face of a typeface.
func benchGlyphLoad(b *testing.B, f *Face, cold bool) {
	b.ReportAllocs()
	for b.Loop() {
		if g := f.tf.glyf; cold && g != nil {
			g.glyphs = newGlyphSlots[tables.Glyph](g.nGlyphs)
			if g.gvar != nil {
				g.variations = newGlyphSlots[tables.GlyphVariationData](g.nGlyphs)
			}
		}
		for g := 0; g < f.tf.nGlyphs; g++ {
			f.loadGlyph(tables.GlyphID(g))
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(f.tf.nGlyphs), "ns/glyph")
}

func interFace(inst Instance) *Face {
	// A typeface of its own, so that the cold benchmarks can drop its caches.
	tfs, err := ParseTypefaces(fonts.Data(fonts.InterVariable), "")
	if err != nil {
		panic(err)
	}
	return tfs[0].Face(inst)
}

func emojiFace() *Face {
	tfs, err := ParseTypefaces(fonts.Data(fonts.NotoColorEmoji), "")
	if err != nil {
		panic(err)
	}
	return tfs[0].Face(Instance{})
}

var interVar = Instance{Weight: 650, OpticalSize: 20}

// BenchmarkGlyphLoadInter loads Inter's outlines at the default instance.
func BenchmarkGlyphLoadInter(b *testing.B) { benchGlyphLoad(b, interFace(Instance{}), false) }

// BenchmarkGlyphLoadInterCold is BenchmarkGlyphLoadInter with the
// typeface's parsed glyphs dropped each iteration.
func BenchmarkGlyphLoadInterCold(b *testing.B) { benchGlyphLoad(b, interFace(Instance{}), true) }

// BenchmarkGlyphLoadInterVar loads Inter's outlines at wght 650, opsz 20
// (gvar deltas applied).
func BenchmarkGlyphLoadInterVar(b *testing.B) { benchGlyphLoad(b, interFace(interVar), false) }

// BenchmarkGlyphLoadInterVarCold is BenchmarkGlyphLoadInterVar with the
// typeface's parsed glyphs dropped each iteration.
func BenchmarkGlyphLoadInterVarCold(b *testing.B) { benchGlyphLoad(b, interFace(interVar), true) }

// BenchmarkGlyphLoadEmoji loads the outlines of Noto Color Emoji (the
// layers of its color glyphs).
func BenchmarkGlyphLoadEmoji(b *testing.B) { benchGlyphLoad(b, emojiFace(), false) }

// BenchmarkGlyphLoadEmojiCold is BenchmarkGlyphLoadEmoji with the
// typeface's parsed glyphs dropped each iteration.
func BenchmarkGlyphLoadEmojiCold(b *testing.B) { benchGlyphLoad(b, emojiFace(), true) }
