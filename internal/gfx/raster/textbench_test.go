package raster

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// benchText lays out 2,000 glyphs of 14 px text in 20 lines of 100 glyphs
// on a 1920x1080 page, at fractional origins so that every subpixel offset
// occurs.
func benchText() *gfx.DisplayList {
	b := gfx.NewBuilder()
	const line = "olAS iolS loo Ail SoAl iS lo AoS lil oSA Sol iAo lSi oAl S" // 59 glyphs
	for i := 0; i < 20; i++ {
		s := line + line
		run := testFontA.run(s[:100], 14, 40.37+float32(i)*3.1, 60+float32(i)*48.6, slate)
		b.DrawGlyphRun(run)
	}
	return b.Finish()
}

func benchTextRender(b *testing.B, cold, paths bool, workers int) {
	dl := benchText()
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	r := NewRenderer()
	r.tune.glyphPaths = paths
	cache := NewGlyphCache(DefaultGlyphCacheBytes)
	opts := Options{Workers: workers, GlyphCache: cache}
	r.Render(img, dl, opts) // warm the cache and the scratch buffers
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if cold {
			cache.Reset()
		}
		r.Render(img, dl, opts)
	}
}

// BenchmarkText1080pWarm draws 2,000 glyphs from a warm glyph cache.
func BenchmarkText1080pWarm(b *testing.B) { benchTextRender(b, false, false, 1) }

// BenchmarkText1080pCold rasterizes the glyph masks every frame.
func BenchmarkText1080pCold(b *testing.B) { benchTextRender(b, true, false, 1) }

// BenchmarkText1080pPaths fills every glyph as a path (no cache), the
// fallback under rotations.
func BenchmarkText1080pPaths(b *testing.B) { benchTextRender(b, false, true, 1) }

// BenchmarkText1080pParallel is the warm case on GOMAXPROCS goroutines.
func BenchmarkText1080pParallel(b *testing.B) { benchTextRender(b, false, false, 0) }
