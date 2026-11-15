// Package gfxtest holds the display-list scenes shared by the CPU rasterizer's
// golden tests and the GPU comparison tests, with the helpers they are drawn
// with. It is imported only by tests.
package gfxtest

import "github.com/kuronosu/tela/internal/gfx"

// Scene is a test scene: a logical size and a function that draws it with the
// Builder.
type Scene struct {
	Name string
	W, H int // logical size
	Draw func(b *gfx.Builder)
}

// Base palette of the scenes (Tailwind 500 shades and friends).
var (
	Slate   = gfx.MustParseHex("#0f172a")
	Indigo  = gfx.MustParseHex("#6366f1")
	Rose    = gfx.MustParseHex("#f43f5e")
	Emerald = gfx.MustParseHex("#10b981")
	Amber   = gfx.MustParseHex("#f59e0b")
	Sky     = gfx.MustParseHex("#0ea5e9")
)

var (
	Fuchsia = gfx.MustParseHex("#c026d3")
	Blue    = gfx.MustParseHex("#0000ff")
	Yellow  = gfx.MustParseHex("#ffff00")
	Red     = gfx.MustParseHex("#ef4444")
	Lime    = gfx.MustParseHex("#84cc16")
)

// Tailwind CSS v4 colors (packages/tailwindcss/theme.css, v4.3.3), converted
// from OKLCH with gfx.OKLCH.
var (
	TwGray100   = gfx.OKLCH(0.967, 0.003, 264.542, 1)
	TwIndigo500 = gfx.OKLCH(0.585, 0.233, 277.117, 1)
	TwSlate900  = gfx.OKLCH(0.208, 0.042, 265.755, 1)
	TwSky400    = gfx.OKLCH(0.746, 0.16, 232.661, 1)
	TwAmber400  = gfx.OKLCH(0.828, 0.189, 84.429, 1)
)

// Colors outside the sRGB gamut (ADR-046): a solid color clips per channel
// and a gradient interpolates the unclipped stops.
var (
	OogGreen  = gfx.OKLCH(0.75, 0.3, 150, 1)
	OogYellow = gfx.OKLCH(0.9, 0.25, 100, 1)
)

// Build records what draw paints and returns the finished display list.
func Build(draw func(b *gfx.Builder)) *gfx.DisplayList {
	b := gfx.NewBuilder()
	draw(b)
	return b.Finish()
}

// StarPath returns a five-pointed star drawn as one self-intersecting
// pentagram. The unit vertices are constants and the products are rounded
// explicitly, so the geometry is the same on every architecture (math.Cos
// and a fused cx + r*cos would not be).
func StarPath(cx, cy, r float32) gfx.Path {
	unit := [5][2]float32{ // cos, sin of i*144° - 90°
		{0, -1}, {0.58778525, 0.809017}, {-0.95105652, -0.309017}, {0.95105652, -0.309017}, {-0.58778525, 0.809017},
	}
	var p gfx.Path
	for i, u := range unit {
		pt := gfx.Pt(cx+float32(r*u[0]), cy+float32(r*u[1]))
		if i == 0 {
			p.MoveTo(pt)
		} else {
			p.LineTo(pt)
		}
	}
	p.Close()
	return p
}

// Stops returns evenly spaced, normalized stops for cs.
func Stops(cs ...gfx.Color) []gfx.GradientStop {
	out := make([]gfx.GradientStop, len(cs))
	for i, c := range cs {
		out[i] = gfx.AutoStop(c)
	}
	return gfx.NormalizeStops(out)
}

// Stop returns the stop of color c at offset o.
func Stop(o float32, c gfx.Color) gfx.GradientStop { return gfx.GradientStop{Offset: o, Color: c} }

// Checker fills r with a gray checkerboard of cell size s, to show
// translucency.
func Checker(b *gfx.Builder, r gfx.Rect, s float32) {
	b.FillRect(r, gfx.MustParseHex("#e2e8f0"))
	b.PushClipRect(r)
	for y, row := r.Y0, 0; y < r.Y1; y, row = y+s, row+1 {
		for x, col := r.X0, 0; x < r.X1; x, col = x+s, col+1 {
			if (row+col)%2 == 0 {
				b.FillRect(gfx.RectXYWH(x, y, s, s), gfx.MustParseHex("#94a3b8"))
			}
		}
	}
	b.PopClip()
}

// TestImage is a 6x6 premultiplied image: a colored border, a translucent
// center, and one fully transparent pixel.
var TestImage = func() *gfx.Image {
	const n = 6
	pix := make([]uint8, 4*n*n)
	cols := []gfx.Color{Indigo, Rose, Emerald, Amber, Sky, Slate}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			c := cols[(x+2*y)%len(cols)]
			switch {
			case x == 2 && y == 2:
				c = gfx.Transparent
			case x >= 2 && x <= 3 && y >= 2 && y <= 3:
				c = c.WithAlpha(0.5)
			}
			p := c.Premul8()
			copy(pix[4*(y*n+x):], []uint8{p.R, p.G, p.B, p.A})
		}
	}
	return gfx.NewImagePremul(n, n, pix)
}()

// TightCurve returns a path with a tight curve: a cubic that turns back on
// itself with a small radius, and a loop.
func TightCurve(x, y float32) gfx.Path {
	var p gfx.Path
	p.MoveTo(gfx.Pt(x, y+30))
	p.CubicTo(gfx.Pt(x+60, y-10), gfx.Pt(x-20, y-10), gfx.Pt(x+40, y+30))
	p.MoveTo(gfx.Pt(x+50, y+30))
	p.CubicTo(gfx.Pt(x+70, y), gfx.Pt(x+72, y), gfx.Pt(x+74, y+30))
	return p
}
