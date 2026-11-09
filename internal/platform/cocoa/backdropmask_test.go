package cocoa

import (
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestMaskImage(t *testing.T) {
	var m platform.InputMask
	if maskImage(&m) != nil {
		t.Error("an empty mask has an image")
	}
	fb := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 1; y < 4; y++ {
		for x := 2; x < 6; x++ {
			fb.SetRGBA(x, y, color.RGBA{A: 0xff})
		}
	}
	fb.SetRGBA(7, 5, color.RGBA{A: platform.InputAlpha - 1}) // a shadow: no input
	m.Update(fb, []image.Rectangle{fb.Rect})
	img := maskImage(&m)
	for y := range 6 {
		for x := range 8 {
			want := uint8(0)
			if x >= 2 && x < 6 && y >= 1 && y < 4 {
				want = 0xff
			}
			if got := img.RGBAAt(x, y).A; got != want {
				t.Errorf("mask image alpha at %d,%d = %d, want %d", x, y, got, want)
			}
		}
	}
}
