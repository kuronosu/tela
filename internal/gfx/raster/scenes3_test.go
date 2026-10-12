package raster

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Scenes for shadows and layer filters (T6).

// Tailwind CSS v4 colors (packages/tailwindcss/theme.css, v4.3.3), converted
// from OKLCH with gfx.OKLCH.
var (
	twGray100   = gfx.OKLCH(0.967, 0.003, 264.542, 1)
	twIndigo500 = gfx.OKLCH(0.585, 0.233, 277.117, 1)
	twSlate900  = gfx.OKLCH(0.208, 0.042, 265.755, 1)
	twSky400    = gfx.OKLCH(0.746, 0.16, 232.661, 1)
	twAmber400  = gfx.OKLCH(0.828, 0.189, 84.429, 1)
)

// cssShadow is one CSS box-shadow layer: offset-x offset-y blur spread
// color, and inset.
type cssShadow struct {
	x, y, blur, spread float32
	color              gfx.Color
	inset              bool
}

func black(a float32) gfx.Color { return gfx.Black.WithAlpha(a) }

// twShadows are the --shadow-* and --inset-shadow-* values of Tailwind CSS
// v4.3.3 (packages/tailwindcss/theme.css), in CSS order (first on top).
var twShadows = []struct {
	name   string
	layers []cssShadow
}{
	{"shadow-2xs", []cssShadow{{0, 1, 0, 0, black(0.05), false}}},
	{"shadow-xs", []cssShadow{{0, 1, 2, 0, black(0.05), false}}},
	{"shadow-sm", []cssShadow{{0, 1, 3, 0, black(0.1), false}, {0, 1, 2, -1, black(0.1), false}}},
	{"shadow-md", []cssShadow{{0, 4, 6, -1, black(0.1), false}, {0, 2, 4, -2, black(0.1), false}}},
	{"shadow-lg", []cssShadow{{0, 10, 15, -3, black(0.1), false}, {0, 4, 6, -4, black(0.1), false}}},
	{"shadow-xl", []cssShadow{{0, 20, 25, -5, black(0.1), false}, {0, 8, 10, -6, black(0.1), false}}},
	{"shadow-2xl", []cssShadow{{0, 25, 50, -12, black(0.25), false}}},
	{"inset-shadow-2xs", []cssShadow{{0, 1, 0, 0, black(0.05), true}}},
	{"inset-shadow-xs", []cssShadow{{0, 1, 1, 0, black(0.05), true}}},
	{"inset-shadow-sm", []cssShadow{{0, 2, 4, 0, black(0.05), true}}},
}

// drawShadows paints CSS shadow layers for box: the last layer first, so
// that the first is on top.
func drawShadows(b *gfx.Builder, box gfx.RRect, layers []cssShadow) {
	for i := len(layers) - 1; i >= 0; i-- {
		l := layers[i]
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(l.x, l.y), Blur: l.blur, Spread: l.spread, Color: l.color, Inset: l.inset})
	}
}

// card is one card of the Tailwind shadow scene.
type card struct {
	name    string
	box     gfx.RRect
	rotate  float32 // degrees, about the card's center
	outer   []cssShadow
	inner   []cssShadow // painted over the background (CSS: above it)
	bgColor gfx.Color
}

// twCards returns the cards of the Tailwind shadow scene: white rounded-xl
// cards (12px radius) on a gray-100 page, 4 per row in 160x150 cells.
func twCards() []card {
	const cw, ch = 100, 70
	cell := func(i int) gfx.Rect {
		x := float32(i%4)*160 + 30
		y := float32(i/4)*150 + 30
		return gfx.RectXYWH(x, y, cw, ch)
	}
	var cards []card
	for i, s := range twShadows {
		c := card{name: s.name, box: gfx.NewRRect(cell(i), 12), bgColor: gfx.White}
		if s.layers[0].inset {
			c.inner = s.layers
		} else {
			c.outer = s.layers
		}
		cards = append(cards, c)
	}
	indigo50 := twIndigo500.WithAlpha(0.5)
	n := len(cards)
	cards = append(cards,
		// shadow-lg shadow-indigo-500/50
		card{name: "shadow-lg-indigo", box: gfx.NewRRect(cell(n), 12), bgColor: gfx.White,
			outer: []cssShadow{{0, 10, 15, -3, indigo50, false}, {0, 4, 6, -4, indigo50, false}}},
		// A pronounced negative spread and a strong inset.
		card{name: "negative-spread", box: gfx.NewRRect(cell(n+1), 12), bgColor: gfx.White,
			outer: []cssShadow{{0, 18, 16, -10, black(0.45), false}}},
		card{name: "inset-strong", box: gfx.NewRRect(cell(n+2), 12), bgColor: gfx.White,
			inner: []cssShadow{{4, 6, 12, 2, black(0.3), true}}},
		// rotate-12 shadow-xl
		card{name: "rotated", box: gfx.NewRRect(cell(n+3), 12), rotate: 12, bgColor: gfx.White,
			outer: twShadows[5].layers},
		// rounded-full pill with shadow-md, a ring (spread, no blur) and
		// per-corner radii with shadow-lg.
		card{name: "pill", box: gfx.NewRRect(gfx.RectXYWH(cell(n+4).X0, cell(n+4).Y0+20, cw, 30), 999), bgColor: gfx.White,
			outer: twShadows[3].layers},
		card{name: "ring", box: gfx.NewRRect(cell(n+5), 12), bgColor: gfx.White,
			outer: []cssShadow{{0, 0, 0, 4, indigo50, false}, {0, 4, 6, -1, black(0.1), false}}},
	)
	return cards
}

// drawTailwindScene draws the cards of twCards.
func drawTailwindScene(b *gfx.Builder) {
	b.FillRect(gfx.Rect{X1: 640, Y1: 600}, twGray100)
	for _, c := range twCards() {
		if c.rotate != 0 {
			b.PushTransform(gfx.RotateAbout(float32(float64(c.rotate)*math.Pi/180), c.box.Rect.Center()))
		}
		drawShadows(b, c.box, c.outer)
		b.FillRRect(c.box, c.bgColor)
		drawShadows(b, c.box, c.inner)
		if c.rotate != 0 {
			b.PopTransform()
		}
	}
}

var scenes3 = []scene{
	{"tailwind_shadows", 640, 600, drawTailwindScene},
	{"shadows", 260, 200, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 260, Y1: 200}, twGray100)
		// Elliptical and per-corner radii, offset and spread, at fractional
		// positions.
		drawShadows(b, gfx.RRectXY(gfx.RectXYWH(20.5, 20.25, 90, 50), 30, 14), []cssShadow{{6, 8, 12, 2, twSlate900.WithAlpha(0.5), false}})
		b.FillRRect(gfx.RRectXY(gfx.RectXYWH(20.5, 20.25, 90, 50), 30, 14), gfx.White)
		pc := gfx.RRectCorners(gfx.RectXYWH(140.3, 20.7, 90, 56), gfx.Pt(0, 0), gfx.Pt(28, 28), gfx.Pt(8, 8), gfx.Pt(20, 10))
		drawShadows(b, pc, []cssShadow{{-4, 6, 20, 0, twIndigo500.WithAlpha(0.6), false}})
		b.FillRRect(pc, gfx.White)
		drawShadows(b, pc, []cssShadow{{0, 0, 10, 0, twSky400, true}})
		// A skewed card: the mask-and-blur path.
		b.PushTransform(gfx.Translate(65, 140).Mul(gfx.Skew(0.35, 0)))
		sk := gfx.NewRRect(gfx.RectXYWH(-40, -25, 80, 50), 10)
		drawShadows(b, sk, []cssShadow{{0, 10, 14, -2, black(0.35), false}})
		b.FillRRect(sk, gfx.White)
		b.PopTransform()
		// A circle with a hard shadow and a huge blur behind a tiny box.
		circ := gfx.NewRRect(gfx.RectXYWH(150, 110, 50, 50), 999)
		drawShadows(b, circ, []cssShadow{{8, 8, 0, 0, twAmber400, false}})
		b.FillRRect(circ, gfx.White)
		tiny := gfx.NewRRect(gfx.RectXYWH(222, 128, 14, 14), 3)
		drawShadows(b, tiny, []cssShadow{{0, 0, 40, 6, twIndigo500, false}})
		b.FillRRect(tiny, twIndigo500)
		// A card cut by a rounded clip: its shadow is cut by the clip too.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(122, 95, 24, 100), 8))
		cut := gfx.NewRRect(gfx.RectXYWH(100, 120, 40, 40), 6)
		drawShadows(b, cut, []cssShadow{{0, 4, 10, 0, twSlate900.WithAlpha(0.6), false}})
		b.FillRRect(cut, twSky400)
		b.PopClip()
	}},
	{"filters", 300, 230, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 300, Y1: 230}, gfx.White)
		content := func(x, y float32) {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, y, 60, 40), 8), twIndigo500)
			b.FillPath(gfx.CirclePath(gfx.Pt(x+48, y+38), 14), gfx.NonZero, twAmber400)
			b.FillRect(gfx.RectXYWH(x+6, y+30, 30, 3), twSlate900)
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
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(4, 6), StdDev: 3, Color: twIndigo500.WithAlpha(0.6)})
		b.FillPath(starPath(240, 45, 34), gfx.NonZero, twAmber400)
		b.PopLayer()
		// Rotated by 0.5 rad: the filters' offsets and blurs rotate too.
		b.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(240, 120)))
		b.PushLayer(0.8, gfx.SrcOver, gfx.Blur{StdDev: 1}, gfx.DropShadow{Offset: gfx.Pt(-2.5, 3.5), StdDev: 1.5, Color: black(0.5)})
		b.FillPath(starPath(240, 120, 26), gfx.EvenOdd, twSky400)
		b.PopLayer()
		b.PopTransform()
		// brightness(1.4), saturate(2), grayscale(1) and sepia+hue-rotate.
		for i, m := range []gfx.ColorMatrix{gfx.Brightness(1.4), gfx.Saturate(2), gfx.Grayscale(1), gfx.Sepia(1).Mul(gfx.HueRotate(1))} {
			b.PushLayer(1, gfx.SrcOver, m)
			x := 15 + float32(i)*50
			b.FillRect(gfx.RectXYWH(x, 85, 40, 60), gfx.LinearGradient{Start: gfx.Pt(x, 85), End: gfx.Pt(x+40, 145),
				Stops: stops(rose, emerald, sky.WithAlpha(0.5))})
			b.PopLayer()
		}
		// A blurred layer inside a rounded clip: the clip cuts the blurred
		// result, and content just outside the clip bleeds into it.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20, 160, 120, 55), 12))
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 3})
		b.FillRect(gfx.RectXYWH(10, 150, 40, 30), red)
		b.FillRect(gfx.RectXYWH(143, 170, 20, 30), twIndigo500) // outside the clip, within reach
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(85, 187)))
		b.FillPath(starPath(85, 187, 22), gfx.NonZero, lime)
		b.PopTransform()
		b.PopLayer()
		b.PopClip()
		// Nested filter layers with a drop shadow under a scale.
		b.PushTransform(gfx.Translate(170, 165).Mul(gfx.Scale(1.25, 0.8)))
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(3, 3), StdDev: 2, Color: black(0.6)})
		b.PushLayer(0.9, gfx.SrcOver, gfx.Saturate(0.3))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 80, 60), 10), gfx.LinearGradient{End: gfx.Pt(80, 60), Stops: stops(fuchsia, amber)})
		b.PopLayer()
		b.PopLayer()
		b.PopTransform()
	}},
}
