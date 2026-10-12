package raster

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// renderScene renders s at the given scale onto an opaque white background.
func renderScene(s scene, scale float32) *image.RGBA {
	w := int(math.Round(float64(float32(s.w) * scale)))
	h := int(math.Round(float64(float32(s.h) * scale)))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	Clear(img, image.Rectangle{}, gfx.White)
	Render(img, build(s.draw), Options{Scale: scale})
	return img
}

func TestGoldens(t *testing.T) {
	for _, s := range append(append(append([]scene(nil), scenes...), scenes2...), scenes3...) {
		for _, scale := range []float32{1, 2} {
			name := fmt.Sprintf("%s_x%g", s.name, scale)
			t.Run(name, func(t *testing.T) {
				golden.Image(t, name, renderScene(s, scale))
			})
		}
	}
}

// TestGoldenTransparent renders onto a transparent background, so the golden
// checks the premultiplied alpha channel too.
func TestGoldenTransparent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 160, 110))
	Render(img, build(scenes[len(scenes)-1].draw), Options{})
	golden.Image(t, "overlap_transparent", img)
}

// TestGoldenFractionalScale covers a non-integer display scale.
func TestGoldenFractionalScale(t *testing.T) {
	golden.Image(t, "rrects_x1.5", renderScene(scenes[1], 1.5))
}
