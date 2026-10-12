package raster_test

import (
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The showcase is the phase-1 scene of showcase_scene_test.go: a 4x3 grid of
// cards with shadows, gradients, filters and strokes, rendered at a 960x640
// window. It lives in the external test package so that its helpers do not
// collide with the ones of the other scenes.

// renderShowcase renders Scene the way the CLI snapshot did, at a logical
// size and scale.
func renderShowcase(w, h float32, scale float64) *image.RGBA {
	// Physical size: logical x scale, rounded half away from zero (the
	// wp_fractional_scale_v1 rule, platform.PhysicalSize).
	phys := func(l float32) int { return int(math.Round(float64(l) * scale)) }
	img := image.NewRGBA(image.Rect(0, 0, phys(w), phys(h)))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, Scene(w, h), raster.Options{Scale: float32(scale)})
	return img
}

// TestShowcaseGolden allows a difference of one level per channel: the scene
// is ordinary Go float code, which the compiler may fuse into multiply-adds on
// arm64 and other architectures, moving coordinates by an ulp. The
// rasterizer itself is bit-identical across architectures (ADR-027) and is
// tested exactly by the other tests of this package.
func TestShowcaseGolden(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"showcase_x1", 1}, {"showcase_x2", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			golden.Image(t, tc.name, renderShowcase(960, 640, tc.scale), golden.MaxDelta(1))
		})
	}
}

// TestShowcaseSizes checks that the scene builds and renders at unusual sizes
// (small, portrait, wide, fractional) without panicking.
func TestShowcaseSizes(t *testing.T) {
	for _, s := range [][2]float32{{1, 1}, {200, 150}, {480, 800}, {1600, 500}, {777.5, 433.25}} {
		img := renderShowcase(s[0], s[1], 1.25)
		if img.Rect.Empty() && s[0] >= 1 {
			t.Errorf("size %v: empty image", s)
		}
	}
}
