package cocoa

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
)

// maxMaskRects bounds the rectangles read from an input mask for the
// backdrop's mask image; past it the cover of InputMask.Rects is used.
const maxMaskRects = 1 << 16

// maskImage returns the input mask of a transparent window as an image of
// the frame's size (physical pixels): opaque white where the frame takes
// input, clear elsewhere; nil for an empty or sizeless mask. It is the
// maskImage of the window's backdrop (maskEffect, ADR-257).
func maskImage(m *platform.InputMask) *image.RGBA {
	size := m.Size()
	if size.X <= 0 || size.Y <= 0 || m.Empty() {
		return nil
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	for _, r := range m.Rects(maxMaskRects) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			row := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
			for i := range row {
				row[i] = 0xff
			}
		}
	}
	return img
}
