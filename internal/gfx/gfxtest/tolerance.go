package gfxtest

import "image"

// The tolerance of GPU output against the CPU rasterizer's for the same
// display list (ADR-282), on premultiplied 8-bit channels: all pixels but
// a share of GPUShare differ by at most GPULevels, and none by more than
// GPUHard. The numbers only tighten. The comparison tests of the renderer
// (internal/gfx/gpu), of core's GPU frames and of the examples share them;
// the scenes that need another rule are listed, with their cause, in the
// renderer's test.
const (
	GPULevels = 2
	GPUShare  = 0.005
	GPUHard   = 24
)

// Diff describes how two images of the same size differ: the largest
// difference of a channel and where it is, the number of pixels compared
// and of those that differ at all, and the share of the pixels with a
// channel that differs by more than 1, 2 and 8 levels.
type Diff struct {
	Max                 int
	Worst               image.Point
	Pixels, Over0       int
	Over1, Over2, Over8 float64
}

// Compare compares the pixels of a with those of b at the same
// coordinates; b must hold a's rectangle.
func Compare(a, b *image.RGBA) Diff {
	var m Diff
	var n1, n2, n8 int
	r := a.Rect
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i, j := a.PixOffset(r.Min.X, y), b.PixOffset(r.Min.X, y)
		for x := r.Min.X; x < r.Max.X; x, i, j = x+1, i+4, j+4 {
			d := 0
			for c := 0; c < 4; c++ {
				d = max(d, absInt(int(a.Pix[i+c])-int(b.Pix[j+c])))
			}
			if d > m.Max {
				m.Max, m.Worst = d, image.Pt(x, y)
			}
			if d > 0 {
				m.Over0++
			}
			if d > 1 {
				n1++
			}
			if d > 2 {
				n2++
			}
			if d > 8 {
				n8++
			}
		}
	}
	m.Pixels = r.Dx() * r.Dy()
	if m.Pixels > 0 {
		p := float64(m.Pixels)
		m.Over1, m.Over2, m.Over8 = float64(n1)/p, float64(n2)/p, float64(n8)/p
	}
	return m
}

// OverShare returns the share of the pixels of a and b, the images m
// compares, that differ by more than n levels.
func (m Diff) OverShare(a, b *image.RGBA, n int) float64 {
	switch n {
	case 1:
		return m.Over1
	case 2:
		return m.Over2
	case 8:
		return m.Over8
	}
	if m.Pixels == 0 {
		return 0
	}
	count := 0
	r := a.Rect
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i, j := a.PixOffset(r.Min.X, y), b.PixOffset(r.Min.X, y)
		for x := r.Min.X; x < r.Max.X; x, i, j = x+1, i+4, j+4 {
			d := 0
			for c := 0; c < 4; c++ {
				d = max(d, absInt(int(a.Pix[i+c])-int(b.Pix[j+c])))
			}
			if d > n {
				count++
			}
		}
	}
	return float64(count) / float64(m.Pixels)
}

// WithinGPU reports whether the difference meets the general rule of
// ADR-282.
func (m Diff) WithinGPU() bool { return m.Max <= GPUHard && m.Over2 <= GPUShare }

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
