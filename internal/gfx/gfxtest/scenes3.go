package gfxtest

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// CSSShadow is one CSS box-shadow layer: offset-x offset-y blur spread
// color, and inset.
type CSSShadow struct {
	X, Y, Blur, Spread float32
	Color              gfx.Color
	Inset              bool
}

// Black returns black with alpha a.
func Black(a float32) gfx.Color { return gfx.Black.WithAlpha(a) }

// TwShadows are the --shadow-* and --inset-shadow-* values of Tailwind CSS
// v4.3.3 (packages/tailwindcss/theme.css), in CSS order (first on top).
var TwShadows = []struct {
	Name   string
	Layers []CSSShadow
}{
	{"shadow-2xs", []CSSShadow{{0, 1, 0, 0, Black(0.05), false}}},
	{"shadow-xs", []CSSShadow{{0, 1, 2, 0, Black(0.05), false}}},
	{"shadow-sm", []CSSShadow{{0, 1, 3, 0, Black(0.1), false}, {0, 1, 2, -1, Black(0.1), false}}},
	{"shadow-md", []CSSShadow{{0, 4, 6, -1, Black(0.1), false}, {0, 2, 4, -2, Black(0.1), false}}},
	{"shadow-lg", []CSSShadow{{0, 10, 15, -3, Black(0.1), false}, {0, 4, 6, -4, Black(0.1), false}}},
	{"shadow-xl", []CSSShadow{{0, 20, 25, -5, Black(0.1), false}, {0, 8, 10, -6, Black(0.1), false}}},
	{"shadow-2xl", []CSSShadow{{0, 25, 50, -12, Black(0.25), false}}},
	{"inset-shadow-2xs", []CSSShadow{{0, 1, 0, 0, Black(0.05), true}}},
	{"inset-shadow-xs", []CSSShadow{{0, 1, 1, 0, Black(0.05), true}}},
	{"inset-shadow-sm", []CSSShadow{{0, 2, 4, 0, Black(0.05), true}}},
}

// DrawShadows paints CSS shadow layers for box: the last layer first, so
// that the first is on top.
func DrawShadows(b *gfx.Builder, box gfx.RRect, layers []CSSShadow) {
	for i := len(layers) - 1; i >= 0; i-- {
		l := layers[i]
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(l.X, l.Y), Blur: l.Blur, Spread: l.Spread, Color: l.Color, Inset: l.Inset})
	}
}

// Card is one card of the Tailwind shadow scene.
type Card struct {
	Name    string
	Box     gfx.RRect
	Rotate  float32 // degrees, about the card's center
	Outer   []CSSShadow
	Inner   []CSSShadow // painted over the background (CSS: above it)
	BGColor gfx.Color
}

// TwCards returns the cards of the Tailwind shadow scene: white rounded-xl
// cards (12px radius) on a gray-100 page, 4 per row in 160x150 cells.
func TwCards() []Card {
	const cw, ch = 100, 70
	cell := func(i int) gfx.Rect {
		x := float32(i%4)*160 + 30
		y := float32(i/4)*150 + 30
		return gfx.RectXYWH(x, y, cw, ch)
	}
	var cards []Card
	for i, s := range TwShadows {
		c := Card{Name: s.Name, Box: gfx.NewRRect(cell(i), 12), BGColor: gfx.White}
		if s.Layers[0].Inset {
			c.Inner = s.Layers
		} else {
			c.Outer = s.Layers
		}
		cards = append(cards, c)
	}
	indigo50 := TwIndigo500.WithAlpha(0.5)
	n := len(cards)
	cards = append(cards,
		// shadow-lg shadow-indigo-500/50
		Card{Name: "shadow-lg-indigo", Box: gfx.NewRRect(cell(n), 12), BGColor: gfx.White,
			Outer: []CSSShadow{{0, 10, 15, -3, indigo50, false}, {0, 4, 6, -4, indigo50, false}}},
		// A pronounced negative spread and a strong inset.
		Card{Name: "negative-spread", Box: gfx.NewRRect(cell(n+1), 12), BGColor: gfx.White,
			Outer: []CSSShadow{{0, 18, 16, -10, Black(0.45), false}}},
		Card{Name: "inset-strong", Box: gfx.NewRRect(cell(n+2), 12), BGColor: gfx.White,
			Inner: []CSSShadow{{4, 6, 12, 2, Black(0.3), true}}},
		// rotate-12 shadow-xl
		Card{Name: "rotated", Box: gfx.NewRRect(cell(n+3), 12), Rotate: 12, BGColor: gfx.White,
			Outer: TwShadows[5].Layers},
		// rounded-full pill with shadow-md, a ring (spread, no blur) and
		// per-corner radii with shadow-lg.
		Card{Name: "pill", Box: gfx.NewRRect(gfx.RectXYWH(cell(n+4).X0, cell(n+4).Y0+20, cw, 30), 999), BGColor: gfx.White,
			Outer: TwShadows[3].Layers},
		Card{Name: "ring", Box: gfx.NewRRect(cell(n+5), 12), BGColor: gfx.White,
			Outer: []CSSShadow{{0, 0, 0, 4, indigo50, false}, {0, 4, 6, -1, Black(0.1), false}}},
	)
	return cards
}

// DrawTailwindScene draws the cards of TwCards.
func DrawTailwindScene(b *gfx.Builder) {
	b.FillRect(gfx.Rect{X1: 640, Y1: 600}, TwGray100)
	for _, c := range TwCards() {
		if c.Rotate != 0 {
			b.PushTransform(gfx.RotateAbout(float32(float64(c.Rotate)*math.Pi/180), c.Box.Rect.Center()))
		}
		DrawShadows(b, c.Box, c.Outer)
		b.FillRRect(c.Box, c.BGColor)
		DrawShadows(b, c.Box, c.Inner)
		if c.Rotate != 0 {
			b.PopTransform()
		}
	}
}

// Scenes3 are the shadow and layer-filter scenes.
var Scenes3 = []Scene{
	{"tailwind_shadows", 640, 600, DrawTailwindScene},
	{"shadows", 260, 200, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 260, Y1: 200}, TwGray100)
		// Elliptical and per-corner radii, offset and spread, at fractional
		// positions.
		DrawShadows(b, gfx.RRectXY(gfx.RectXYWH(20.5, 20.25, 90, 50), 30, 14), []CSSShadow{{6, 8, 12, 2, TwSlate900.WithAlpha(0.5), false}})
		b.FillRRect(gfx.RRectXY(gfx.RectXYWH(20.5, 20.25, 90, 50), 30, 14), gfx.White)
		pc := gfx.RRectCorners(gfx.RectXYWH(140.3, 20.7, 90, 56), gfx.Pt(0, 0), gfx.Pt(28, 28), gfx.Pt(8, 8), gfx.Pt(20, 10))
		DrawShadows(b, pc, []CSSShadow{{-4, 6, 20, 0, TwIndigo500.WithAlpha(0.6), false}})
		b.FillRRect(pc, gfx.White)
		DrawShadows(b, pc, []CSSShadow{{0, 0, 10, 0, TwSky400, true}})
		// A skewed card: the mask-and-blur path.
		b.PushTransform(gfx.Translate(65, 140).Mul(gfx.Skew(0.35, 0)))
		sk := gfx.NewRRect(gfx.RectXYWH(-40, -25, 80, 50), 10)
		DrawShadows(b, sk, []CSSShadow{{0, 10, 14, -2, Black(0.35), false}})
		b.FillRRect(sk, gfx.White)
		b.PopTransform()
		// A circle with a hard shadow and a huge blur behind a tiny box.
		circ := gfx.NewRRect(gfx.RectXYWH(150, 110, 50, 50), 999)
		DrawShadows(b, circ, []CSSShadow{{8, 8, 0, 0, TwAmber400, false}})
		b.FillRRect(circ, gfx.White)
		tiny := gfx.NewRRect(gfx.RectXYWH(222, 128, 14, 14), 3)
		DrawShadows(b, tiny, []CSSShadow{{0, 0, 40, 6, TwIndigo500, false}})
		b.FillRRect(tiny, TwIndigo500)
		// A card cut by a rounded clip: its shadow is cut by the clip too.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(122, 95, 24, 100), 8))
		cut := gfx.NewRRect(gfx.RectXYWH(100, 120, 40, 40), 6)
		DrawShadows(b, cut, []CSSShadow{{0, 4, 10, 0, TwSlate900.WithAlpha(0.6), false}})
		b.FillRRect(cut, TwSky400)
		b.PopClip()
	}},
	{"filters", 300, 230, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 300, Y1: 230}, gfx.White)
		content := func(x, y float32) {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, y, 60, 40), 8), TwIndigo500)
			b.FillPath(gfx.CirclePath(gfx.Pt(x+48, y+38), 14), gfx.NonZero, TwAmber400)
			b.FillRect(gfx.RectXYWH(x+6, y+30, 30, 3), TwSlate900)
		}
		// blur(4px) and blur(12px).
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 4})
		content(15, 15)
		b.PopLayer()
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 12})
		content(110, 15)
		b.PopLayer()
		// drop-shadow(4px 6px 6px indigo/60%) on a star, then the same
		// with a fractional offset and blur(1px) + opacity.
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(4, 6), StdDev: 3, Color: TwIndigo500.WithAlpha(0.6)})
		b.FillPath(StarPath(240, 45, 34), gfx.NonZero, TwAmber400)
		b.PopLayer()
		// Rotated by 0.5 rad: the filters' offsets and blurs rotate too.
		b.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(240, 120)))
		b.PushLayer(0.8, gfx.SrcOver, gfx.Blur{StdDev: 1}, gfx.DropShadow{Offset: gfx.Pt(-2.5, 3.5), StdDev: 1.5, Color: Black(0.5)})
		b.FillPath(StarPath(240, 120, 26), gfx.EvenOdd, TwSky400)
		b.PopLayer()
		b.PopTransform()
		// brightness(1.4), saturate(2), grayscale(1) and sepia+hue-rotate.
		for i, m := range []gfx.ColorMatrix{gfx.Brightness(1.4), gfx.Saturate(2), gfx.Grayscale(1), gfx.Sepia(1).Mul(gfx.HueRotate(1))} {
			b.PushLayer(1, gfx.SrcOver, m)
			x := 15 + float32(i)*50
			b.FillRect(gfx.RectXYWH(x, 85, 40, 60), gfx.LinearGradient{Start: gfx.Pt(x, 85), End: gfx.Pt(x+40, 145),
				Stops: Stops(Rose, Emerald, Sky.WithAlpha(0.5))})
			b.PopLayer()
		}
		// A blurred layer inside a rounded clip: the clip cuts the blurred
		// result, and content just outside the clip bleeds into it.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20, 160, 120, 55), 12))
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 3})
		b.FillRect(gfx.RectXYWH(10, 150, 40, 30), Red)
		b.FillRect(gfx.RectXYWH(143, 170, 20, 30), TwIndigo500) // outside the clip, within reach
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(85, 187)))
		b.FillPath(StarPath(85, 187, 22), gfx.NonZero, Lime)
		b.PopTransform()
		b.PopLayer()
		b.PopClip()
		// Nested filter layers with a drop shadow under a scale.
		b.PushTransform(gfx.Translate(170, 165).Mul(gfx.Scale(1.25, 0.8)))
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(3, 3), StdDev: 2, Color: Black(0.6)})
		b.PushLayer(0.9, gfx.SrcOver, gfx.Saturate(0.3))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 80, 60), 10), gfx.LinearGradient{End: gfx.Pt(80, 60), Stops: Stops(Fuchsia, Amber)})
		b.PopLayer()
		b.PopLayer()
		b.PopTransform()
	}},
}
