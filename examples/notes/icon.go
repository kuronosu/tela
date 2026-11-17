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

// appIcon is the icon of the window and of the tray, the same picture as
// icon.svg (the packages' icon): a graphite rounded badge with the
// notebook-pen glyph, the notebook in light ink and the pen in amber, drawn
// here so the example needs no image file (an app ships a PNG with go:embed
// and image/png instead). The pictures are made once.
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

// The notebook-pen glyph of Lucide (the one of the title bar), as the
// polylines of its strokes in its 24 x 24 box: the notebook with its four
// rings, and the pen. The outline of the pen is closed.
var (
	notebook = [][][2]float64{
		{{13.4, 2}, {6, 2}, {4, 4}, {4, 20}, {6, 22}, {18, 22}, {20, 20}, {20, 12.6}},
		{{2, 6}, {6, 6}}, {{2, 10}, {6, 10}}, {{2, 14}, {6, 14}}, {{2, 18}, {6, 18}},
		{{21.38, 5.63}, {21.6, 4.1}, {20.9, 2.9}, {19.7, 2.4}, {18.4, 2.62}, {13.36, 7.63}, {12.86, 8.49},
			{12.02, 11.36}, {12.64, 11.98}, {15.51, 11.14}, {16.37, 10.64}, {21.38, 5.63}},
	}
)

// iconAt is the color (r, g, b, a in 0..1) of the icon at (x, y) in 0..1:
// a graphite rounded square, lighter at the top, with a thin lighter rim and
// the notebook-pen glyph in the middle (icon.svg's 512 px drawing).
func iconAt(x, y float64) [4]float64 {
	if !inRoundRect(x, y, 20.0/512, 20.0/512, 492.0/512, 492.0/512, 113.0/512) {
		return [4]float64{}
	}
	// The glyph takes 58% of the square, centered; its strokes are 2.4
	// units wide (Lucide's 2, a little bolder so that it holds at 16 px).
	const scale, half = 0.58, 1.2
	gx, gy := (x-0.5)/scale*24+12, (y-0.5)/scale*24+12
	for i, line := range notebook {
		for j := 1; j < len(line); j++ {
			if distToSegment(gx, gy, line[j-1], line[j]) <= half {
				if i == len(notebook)-1 { // the pen
					return [4]float64{0.941, 0.694, 0.227, 1} // #f0b13a
				}
				return [4]float64{0.925, 0.925, 0.933, 1} // #ececee
			}
		}
	}
	if !inRoundRect(x, y, 28.0/512, 28.0/512, 484.0/512, 484.0/512, 105.0/512) {
		return [4]float64{0.227, 0.227, 0.247, 1} // the rim, #3a3a3f
	}
	// #2a2a2d at the top to #131314 at the bottom.
	return [4]float64{lerp(0.165, 0.075, y), lerp(0.165, 0.075, y), lerp(0.176, 0.078, y), 1}
}

// distToSegment is the distance from (x, y) to the segment a-b.
func distToSegment(x, y float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	t := math.Max(0, math.Min(1, ((x-a[0])*dx+(y-a[1])*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-(a[0]+t*dx), y-(a[1]+t*dy))
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
