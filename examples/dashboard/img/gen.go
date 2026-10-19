//go:build ignore

// gen.go draws the two aurora backdrops of the dashboard (aurora-light.jpg
// and aurora-dark.jpg): a few soft color glows over a base color, with a
// little noise so the gradients do not band. Both are made from nothing
// but arithmetic, so they carry no third-party copyright.
//
//	CGO_ENABLED=0 go run ./examples/dashboard/img/gen.go
package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand"
	"os"
)

// glow is a soft round spot: its center and radius are fractions of the
// image's width.
type glow struct {
	x, y, r  float64
	c        [3]float64
	strength float64
}

func clamp(v float64) uint8 { return uint8(max(0, min(255, v))) }

func render(name string, base [3]float64, glows []glow) {
	const w, h = 960, 600
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(7))
	for y := range h {
		for x := range w {
			px := base
			for _, g := range glows {
				dx, dy := float64(x)/w-g.x, float64(y)/w-g.y*h/w
				d2 := (dx*dx + dy*dy) / (g.r * g.r)
				a := g.strength * math.Exp(-d2*2)
				for i := range px {
					px[i] += (g.c[i] - px[i]) * a
				}
			}
			n := (rng.Float64() - 0.5) * 3
			img.Set(x, y, color.RGBA{R: clamp(px[0] + n), G: clamp(px[1] + n), B: clamp(px[2] + n), A: 255})
		}
	}
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 88}); err != nil {
		panic(err)
	}
}

func main() {
	render("examples/dashboard/img/aurora-light.jpg", [3]float64{238, 242, 255}, []glow{
		{0.12, 0.10, 0.34, [3]float64{129, 140, 248}, 0.85}, // indigo
		{0.88, 0.05, 0.30, [3]float64{232, 121, 249}, 0.70}, // fuchsia
		{0.60, 0.95, 0.38, [3]float64{56, 189, 248}, 0.65},  // sky
		{0.02, 0.85, 0.26, [3]float64{251, 191, 36}, 0.45},  // amber
		{0.95, 0.75, 0.24, [3]float64{251, 113, 133}, 0.50}, // rose
	})
	render("examples/dashboard/img/aurora-dark.jpg", [3]float64{9, 11, 26}, []glow{
		{0.10, 0.08, 0.34, [3]float64{79, 70, 229}, 0.85},  // indigo
		{0.90, 0.10, 0.30, [3]float64{192, 38, 211}, 0.60}, // fuchsia
		{0.58, 0.98, 0.36, [3]float64{8, 145, 178}, 0.60},  // cyan
		{0.02, 0.90, 0.24, [3]float64{5, 150, 105}, 0.40},  // emerald
		{0.96, 0.78, 0.22, [3]float64{225, 29, 72}, 0.35},  // rose
	})
}
