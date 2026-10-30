//go:build ignore

// gen.go draws the two dune wallpapers of the dashboard (dunes-light.jpg
// and dunes-dark.jpg): a sky with a soft warm glow, four ranges of dunes
// lit from the right with a bright rim on each crest, a light blur to take
// the aliasing off the crests and a little grain so the gradients do not
// band. Both are made from nothing but arithmetic, so they carry no
// third-party copyright.
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

// W and H are the size of the pictures: the window's aspect (16:10), large
// enough for a 1920-pixel-wide window at scale 1. They are drawn with
// bg-cover, and the panes blur all of them but the gutters between them.
const W, H = 1920, 1200

func clamp(v float64) uint8 { return uint8(max(0, min(255, v+0.5))) }

// ridge is one range of dunes: its crest line (a sum of three sines) and
// its colors.
type ridge struct {
	base, amp, freq, phase float64 // crest height (fraction of H), swing, waves across, offset
	top, bottom, rim       [3]float64
	rimStrength            float64
}

func (r ridge) crest(x float64) float64 {
	u := x / W
	return (r.base + r.amp*(math.Sin(u*r.freq*6.28+r.phase)*0.6+
		math.Sin(u*r.freq*13.1+r.phase*2.3)*0.28+
		math.Sin(u*r.freq*27.7+r.phase*0.7)*0.12)) * H
}

func dunes(path string, sky0, sky1, glow [3]float64, glowAmount float64, ridges []ridge) {
	px := make([][3]float64, W*H)
	for y := range H {
		t := float64(y) / H
		for x := range W {
			dx, dy := float64(x)/W-0.78, (t-0.55)*1.4
			g := math.Exp(-(dx*dx+dy*dy)*7) * glowAmount
			for k := range 3 {
				px[y*W+x][k] = sky0[k] + (sky1[k]-sky0[k])*t + glow[k]*g
			}
		}
	}
	for _, r := range ridges {
		for x := range W {
			y0 := r.crest(float64(x))
			slope := (r.crest(float64(x)+2) - r.crest(float64(x)-2)) * 1.5
			for y := max(int(y0), 0); y < H; y++ {
				d := (float64(y) - y0) / (H - y0 + 1)
				// Light from the right: slopes that face it are lighter
				// near the crest, and the crest itself has a bright rim.
				lit := 0.5 * (1 - math.Tanh(slope/3)) * 0.55 * math.Exp(-(float64(y)-y0)/73)
				rim := r.rimStrength * math.Exp(-(float64(y)-y0)/3.3)
				var c [3]float64
				for k := range 3 {
					c[k] = r.top[k] + (r.bottom[k]-r.top[k])*math.Min(1, d*1.3)
					c[k] += lit*(r.rim[k]-c[k])*0.35 + rim*(r.rim[k]-c[k])
				}
				px[y*W+x] = c
			}
		}
	}
	// Two passes of a horizontal box of 5 soften the crests.
	tmp := make([][3]float64, len(px))
	for range 2 {
		for y := range H {
			for x := range W {
				var s [3]float64
				for k := -2; k <= 2; k++ {
					p := px[y*W+min(W-1, max(0, x+k))]
					for c := range 3 {
						s[c] += p[c]
					}
				}
				for c := range 3 {
					tmp[y*W+x][c] = s[c] / 5
				}
			}
		}
		px, tmp = tmp, px
	}
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	rng := rand.New(rand.NewSource(3))
	for y := range H {
		for x := range W {
			p := px[y*W+x]
			n := (rng.Float64() - 0.5) * 3
			img.SetRGBA(x, y, color.RGBA{clamp(p[0] + n), clamp(p[1] + n), clamp(p[2] + n), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		panic(err)
	}
}

func main() {
	dunes("examples/dashboard/img/dunes-dark.jpg",
		[3]float64{14, 17, 24}, [3]float64{40, 47, 62}, [3]float64{120, 78, 34}, 1.25,
		[]ridge{
			{0.50, 0.07, 0.9, 0.3, [3]float64{66, 76, 98}, [3]float64{34, 40, 54}, [3]float64{200, 160, 120}, 0.16},
			{0.58, 0.08, 1.2, 1.7, [3]float64{52, 60, 79}, [3]float64{28, 33, 45}, [3]float64{230, 170, 100}, 0.22},
			{0.68, 0.09, 1.0, 3.1, [3]float64{38, 44, 59}, [3]float64{20, 24, 33}, [3]float64{245, 175, 95}, 0.30},
			{0.80, 0.08, 0.8, 4.4, [3]float64{26, 30, 41}, [3]float64{12, 14, 20}, [3]float64{200, 140, 75}, 0.18},
		})
	dunes("examples/dashboard/img/dunes-light.jpg",
		[3]float64{226, 228, 233}, [3]float64{243, 240, 236}, [3]float64{255, 222, 180}, 0.8,
		[]ridge{
			{0.50, 0.07, 0.9, 0.3, [3]float64{205, 208, 217}, [3]float64{224, 225, 231}, [3]float64{255, 236, 210}, 0.30},
			{0.58, 0.08, 1.2, 1.7, [3]float64{190, 194, 205}, [3]float64{214, 216, 224}, [3]float64{255, 230, 195}, 0.34},
			{0.68, 0.09, 1.0, 3.1, [3]float64{172, 177, 191}, [3]float64{200, 203, 214}, [3]float64{255, 222, 180}, 0.40},
			{0.80, 0.08, 0.8, 4.4, [3]float64{150, 156, 173}, [3]float64{186, 190, 203}, [3]float64{255, 214, 165}, 0.30},
		})
}
