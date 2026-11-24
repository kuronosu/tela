package gpu

import (
	"image"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// TestEncodePorterDuffPatterns checks that what the shaders do not draw yet is
// named, so that core draws the frame with the rasterizer: the layers
// composited with a Porter-Duff operator, whatever shows of them, and the
// image patterns.
func TestEncodePorterDuffPatterns(t *testing.T) {
	var e Encoder
	encode := func(draw func(b *gfx.Builder)) []string {
		return e.Encode(gfxtest.Build(draw), image.Pt(100, 100), Options{Clear: &gfx.White}).Unsupported
	}
	for m := gfx.PlusLighter + 1; m <= gfx.Xor; m++ {
		want := []string{"PushLayer (" + m.String() + ")"}
		for _, opacity := range []float32{1, 0} {
			if opacity == 0 && !m.Unbounded() {
				continue // the Builder drops it
			}
			got := encode(func(b *gfx.Builder) {
				b.FillRect(gfx.RectXYWH(0, 0, 50, 50), sky)
				b.PushLayer(opacity, m)
				b.FillRect(gfx.RectXYWH(20, 20, 50, 50), rose)
				b.PopLayer()
			})
			if !slices.Equal(got, want) {
				t.Errorf("%s at opacity %v: unsupported %q, want %q", m, opacity, got, want)
			}
		}
	}
	p := gfx.ImagePattern{Image: gfxtest.PatternTile, Transform: gfx.Identity}
	for name, draw := range map[string]func(b *gfx.Builder){
		"FillRect":     func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(0, 0, 50, 50), p) },
		"FillRRect":    func(b *gfx.Builder) { b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 50, 50), 8), p) },
		"FillPath":     func(b *gfx.Builder) { b.FillPath(gfxtest.StarPath(50, 50, 40), gfx.NonZero, p) },
		"StrokePath":   func(b *gfx.Builder) { b.StrokePath(gfxtest.StarPath(50, 50, 40), gfx.NewStrokeStyle(3), p) },
		"DrawGlyphRun": func(b *gfx.Builder) { b.DrawGlyphRun(gfxtest.FontA.Run("A", 30, 10, 50, p)) },
	} {
		if got := encode(draw); !slices.Equal(got, []string{"image pattern"}) {
			t.Errorf("%s with a pattern: unsupported %q", name, got)
		}
	}
}
