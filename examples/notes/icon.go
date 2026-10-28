package main

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// iconSizes are the sizes of the app's icon: 16, 24 and 32 for the tray,
// the rest for the title bar, the task bar and alt-tab.
var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

var (
	iconOnce   sync.Once
	iconImages []image.Image
)

// appIcon is the icon of the window and of the tray: a yellow sticky note
// with three lines of text, drawn here so the example needs no image file
// (an app ships a PNG with go:embed and image/png instead). The pictures
// are made once.
func appIcon() []image.Image {
	iconOnce.Do(func() {
		for _, size := range iconSizes {
			iconImages = append(iconImages, drawIcon(size))
		}
	})
	return iconImages
}

// drawIcon draws the icon at size x size pixels, with 4x4 samples per
// pixel for smooth edges.
func drawIcon(size int) *image.RGBA {
	const samples = 4
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range samples {
				for sx := range samples {
					x := (float64(px) + (float64(sx)+0.5)/samples) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/samples) / float64(size)
					c := iconAt(x, y)
					r += c[0] * c[3]
					g += c[1] * c[3]
					b += c[2] * c[3]
					a += c[3]
				}
			}
			n := float64(samples * samples)
			// The sums are premultiplied by alpha, as color.RGBA is.
			img.SetRGBA(px, py, color.RGBA{uint8(r/n*255 + 0.5), uint8(g/n*255 + 0.5), uint8(b/n*255 + 0.5), uint8(a/n*255 + 0.5)})
		}
	}
	return img
}

// iconAt is the color (r, g, b, a in 0..1) of the icon at (x, y) in 0..1.
func iconAt(x, y float64) [4]float64 {
	// The note: a rounded square with its bottom right corner folded.
	if !inRoundRect(x, y, 0.07, 0.07, 0.93, 0.93, 0.18) {
		return [4]float64{}
	}
	const fold = 0.26
	if u, v := x-(0.93-fold), y-(0.93-fold); u > 0 && v > 0 {
		if u+v > fold {
			return [4]float64{} // the corner that was folded away
		}
		return [4]float64{0.85, 0.55, 0.05, 1} // the flap, a darker yellow
	}
	// Text lines.
	for _, l := range [...]struct{ y, x1 float64 }{{0.34, 0.74}, {0.50, 0.74}, {0.66, 0.5}} {
		if inRoundRect(x, y, 0.26, l.y-0.035, l.x1, l.y+0.035, 0.035) {
			return [4]float64{0.47, 0.21, 0.06, 1}
		}
	}
	// Yellow, a little lighter at the top.
	t := y
	return [4]float64{lerp(0.99, 0.96, t), lerp(0.83, 0.68, t), lerp(0.30, 0.10, t), 1}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// inRoundRect reports whether (x, y) is inside the rectangle with corner
// radius r.
func inRoundRect(x, y, x0, y0, x1, y1, r float64) bool {
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	return math.Hypot(x-cx, y-cy) <= r
}
