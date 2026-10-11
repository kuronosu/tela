package raster

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Scenes for gradients, images, clips, layers and strokes (T4b). They are
// rendered as goldens at scales 1 and 2 and used by the region tests.

var (
	fuchsia = gfx.MustParseHex("#c026d3")
	blue    = gfx.MustParseHex("#0000ff")
	yellow  = gfx.MustParseHex("#ffff00")
	red     = gfx.MustParseHex("#ef4444")
	lime    = gfx.MustParseHex("#84cc16")
)

func stops(cs ...gfx.Color) []gfx.GradientStop {
	out := make([]gfx.GradientStop, len(cs))
	for i, c := range cs {
		out[i] = gfx.AutoStop(c)
	}
	return gfx.NormalizeStops(out)
}

func stop(o float32, c gfx.Color) gfx.GradientStop { return gfx.GradientStop{Offset: o, Color: c} }

// checker fills r with a gray checkerboard of cell size s, to show
// translucency.
func checker(b *gfx.Builder, r gfx.Rect, s float32) {
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

// testImage is a 6x6 premultiplied image: a colored border, a translucent
// center, and one fully transparent pixel.
var testImage = func() *gfx.Image {
	const n = 6
	pix := make([]uint8, 4*n*n)
	cols := []gfx.Color{indigo, rose, emerald, amber, sky, slate}
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

// spiral returns a path with a tight curve: a cubic that turns back on
// itself with a small radius, and a loop.
func tightCurve(x, y float32) gfx.Path {
	var p gfx.Path
	p.MoveTo(gfx.Pt(x, y+30))
	p.CubicTo(gfx.Pt(x+60, y-10), gfx.Pt(x-20, y-10), gfx.Pt(x+40, y+30))
	p.MoveTo(gfx.Pt(x+50, y+30))
	p.CubicTo(gfx.Pt(x+70, y), gfx.Pt(x+72, y), gfx.Pt(x+74, y+30))
	return p
}

var scenes2 = []scene{
	{"linear", 240, 200, func(b *gfx.Builder) {
		// Tailwind bg-linear-to-br from-indigo-500 to-fuchsia-600 on a card.
		card := gfx.RectXYWH(8, 8, 108, 64)
		b.FillRRect(gfx.NewRRect(card, 12), gfx.LinearGradient{
			Start: card.Origin(), End: gfx.Pt(card.X1, card.Y1), Stops: stops(indigo, fuchsia)})
		// The same three stops interpolated in oklab and in srgb.
		for i, sp := range []gfx.ColorSpace{gfx.InterpOklab, gfx.InterpSRGB} {
			y := 8 + float32(i)*34
			r := gfx.RectXYWH(124, y, 108, 30)
			b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, 0), End: gfx.Pt(r.X1, 0), Stops: stops(blue, yellow), Interp: sp})
		}
		// Hard stops at fractional positions: four stripes.
		r := gfx.RectXYWH(8.5, 80.25, 223, 22)
		b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, 0), End: gfx.Pt(r.X1, 0), Stops: []gfx.GradientStop{
			stop(0, red), stop(0.25, red), stop(0.25, gfx.White), stop(0.5, gfx.White),
			stop(0.5, blue), stop(0.7, yellow), stop(0.7, slate), stop(1, slate)}})
		// Translucent stops over a checkerboard.
		bg := gfx.RectXYWH(8, 110, 224, 40)
		checker(b, bg, 8)
		b.FillRect(gfx.RectXYWH(8, 110, 224, 20), gfx.LinearGradient{
			Start: gfx.Pt(8, 0), End: gfx.Pt(232, 0), Stops: stops(sky, gfx.Transparent)})
		b.FillRect(gfx.RectXYWH(8, 130, 224, 20), gfx.LinearGradient{
			Start: gfx.Pt(8, 0), End: gfx.Pt(232, 0), Stops: stops(emerald, rose.WithAlpha(0.4))})
		// Rotated gradient on a rotated rrect, padded beyond its ends.
		b.PushTransform(gfx.RotateAbout(-0.12, gfx.Pt(120, 176)))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(20, 160, 200, 32), 16), gfx.LinearGradient{
			Start: gfx.Pt(70, 0), End: gfx.Pt(170, 0), Stops: stops(amber, rose, indigo)})
		b.PopTransform()
	}},
	{"radial", 240, 130, func(b *gfx.Builder) {
		// A sphere-like circle.
		b.FillPath(gfx.CirclePath(gfx.Pt(40, 40), 32), gfx.NonZero, gfx.RadialGradient{
			Center: gfx.Pt(30, 30), Radius: gfx.Pt(42, 42), Stops: stops(gfx.White, sky, indigo)})
		// Elliptical gradient on a rect, padded outside the ellipse.
		b.FillRect(gfx.RectXYWH(80, 8, 152, 64), gfx.RadialGradient{
			Center: gfx.Pt(156, 40), Radius: gfx.Pt(60, 24), Stops: stops(amber, rose, slate)})
		// Hard-stop rings.
		b.FillRect(gfx.RectXYWH(8, 80, 44, 44), gfx.RadialGradient{
			Center: gfx.Pt(30.3, 102.3), Radius: gfx.Pt(22, 22), Stops: []gfx.GradientStop{
				stop(0, red), stop(0.33, red), stop(0.33, gfx.White), stop(0.66, gfx.White),
				stop(0.66, red), stop(1, red), stop(1, gfx.White)}})
		// A translucent glow over a dark panel.
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(60, 80, 80, 44), 8), slate)
		b.FillRect(gfx.RectXYWH(60, 80, 80, 44), gfx.RadialGradient{
			Center: gfx.Pt(100, 102), Radius: gfx.Pt(40, 30), Stops: stops(gfx.White.WithAlpha(0.8), sky.WithAlpha(0.3), gfx.Transparent)})
		// Under a skew and rotation: the ellipse follows the transform.
		b.PushTransform(gfx.Translate(190, 102).Mul(gfx.Rotate(0.4)).Mul(gfx.Skew(0.3, 0)))
		b.FillRect(gfx.RectXYWH(-36, -20, 72, 40), gfx.RadialGradient{
			Radius: gfx.Pt(30, 14), Stops: stops(lime, emerald, slate), Interp: gfx.InterpSRGB})
		b.PopTransform()
	}},
	{"conic", 240, 150, func(b *gfx.Builder) {
		// A color wheel, interpolated in srgb like a classic hue wheel.
		b.FillPath(gfx.CirclePath(gfx.Pt(50, 50), 40), gfx.NonZero, gfx.ConicGradient{
			Center: gfx.Pt(50, 50), Interp: gfx.InterpSRGB, Stops: stops(
				gfx.RGB8(255, 0, 0), gfx.RGB8(255, 255, 0), gfx.RGB8(0, 255, 0),
				gfx.RGB8(0, 255, 255), gfx.RGB8(0, 0, 255), gfx.RGB8(255, 0, 255), gfx.RGB8(255, 0, 0))})
		// A pie chart with hard stops, starting at 3 o'clock.
		b.FillPath(gfx.CirclePath(gfx.Pt(130.5, 50.5), 38), gfx.NonZero, gfx.ConicGradient{
			Center: gfx.Pt(130.5, 50.5), StartAngle: math.Pi / 2, Stops: []gfx.GradientStop{
				stop(0, indigo), stop(0.35, indigo), stop(0.35, rose), stop(0.6, rose),
				stop(0.6, emerald), stop(0.85, emerald), stop(0.85, amber), stop(1, amber)}})
		// A smooth sweep in oklab with its seam, rotated by a transform.
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(205, 50)))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(175, 15, 60, 70), 10), gfx.ConicGradient{
			Center: gfx.Pt(205, 50), StartAngle: -math.Pi / 4, Stops: stops(slate, sky, gfx.White)})
		b.PopTransform()
		// Translucent conics over a checkerboard: a spinner-like sweep in a
		// ring and a fading pie.
		checker(b, gfx.RectXYWH(8, 96, 224, 50), 8)
		var ring gfx.Path
		ring.AddCircle(gfx.Pt(60, 121), 22)
		ring.AddCircle(gfx.Pt(60, 121), 14)
		b.FillPath(ring, gfx.EvenOdd, gfx.ConicGradient{
			Center: gfx.Pt(60, 121), Stops: stops(indigo.WithAlpha(0), indigo)})
		b.FillPath(gfx.CirclePath(gfx.Pt(150, 121), 22), gfx.NonZero, gfx.ConicGradient{
			Center: gfx.Pt(150, 121), StartAngle: 1, Stops: stops(rose, rose.WithAlpha(0), rose)})
	}},
	{"images", 240, 130, func(b *gfx.Builder) {
		img := testImage
		checker(b, gfx.RectXYWH(0, 0, 240, 130), 10)
		// Upscaled 8x: nearest, then bilinear.
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(8, 8, 48, 48), gfx.SampleNearest)
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(64, 8, 48, 48), gfx.SampleLinear)
		// A sub-rectangle (the 2x2 center): bilinear must not bleed in the
		// border colors.
		b.DrawImage(img, gfx.Rect{X0: 2, Y0: 2, X1: 4, Y1: 4}, gfx.RectXYWH(120, 8, 48, 48), gfx.SampleLinear)
		// Src partly outside the image: only the inside part is drawn.
		b.DrawImage(img, gfx.Rect{X0: 3, Y0: -3, X1: 9, Y1: 3}, gfx.RectXYWH(176, 8, 48, 48), gfx.SampleNearest)
		// Rotated, bilinear and nearest.
		b.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(40, 92)))
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(16, 68, 48, 48), gfx.SampleLinear)
		b.PopTransform()
		b.PushTransform(gfx.RotateAbout(-0.3, gfx.Pt(100, 92)))
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(76, 68, 48, 48), gfx.SampleNearest)
		b.PopTransform()
		// Fractional position and non-uniform scale; and 1:1.
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(136.4, 70.3, 60.5, 24.2), gfx.SampleLinear)
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(210, 100, 6, 6), gfx.SampleLinear)
	}},
	{"clips", 240, 150, func(b *gfx.Builder) {
		grad := gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(240, 150), Stops: stops(sky, indigo, rose)}
		// Nested clips: a rounded rect, then a star path inside it.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(8.5, 8.5, 70, 60), 14))
		b.FillRect(gfx.RectXYWH(0, 0, 240, 150), gfx.MustParseHex("#e0e7ff"))
		b.PushClipPath(starPath(43, 42, 40), gfx.NonZero)
		b.FillRect(gfx.RectXYWH(0, 0, 240, 150), grad)
		b.PopClip()
		b.FillPath(gfx.CirclePath(gfx.Pt(78, 68), 14), gfx.NonZero, amber)
		b.PopClip()
		// A pixel-aligned rect clip (no mask) and a fractional one.
		for i, r := range []gfx.Rect{gfx.RectXYWH(88, 8, 30, 30), gfx.RectXYWH(124.3, 8.6, 30, 30.5)} {
			b.PushClipRect(r)
			c := r.Center()
			b.FillPath(gfx.CirclePath(c, 20), gfx.NonZero, []gfx.Color{emerald, rose}[i])
			b.FillRect(gfx.RectXYWH(c.X-30, c.Y-2, 60, 4), slate)
			b.PopClip()
		}
		// A clip under rotation, with stripes inside.
		b.PushTransform(gfx.RotateAbout(0.4, gfx.Pt(195, 40)))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(168, 18, 54, 44), 10))
		for i := 0; i < 8; i++ {
			c := []gfx.Color{indigo, amber}[i%2]
			b.FillRect(gfx.RectXYWH(160+float32(i)*9, 10, 9, 60), c)
		}
		b.PopClip()
		b.PopTransform()
		// An even-odd ring clip with a gradient.
		var ring gfx.Path
		ring.AddCircle(gfx.Pt(40, 112), 30)
		ring.AddCircle(gfx.Pt(40, 112), 15)
		b.PushClipPath(ring, gfx.EvenOdd)
		b.FillRect(gfx.RectXYWH(0, 80, 80, 70), gfx.ConicGradient{Center: gfx.Pt(40, 112), Stops: stops(rose, amber, emerald, sky, rose)})
		b.PopClip()
		// A rect clip under an exact 90 degree rotation and a scale
		// (axis-aligned, not pixel aligned), holding a translucent layer.
		b.PushTransform(gfx.Affine{0, 1, -1, 0, 150, 80}.Mul(gfx.Scale(1.25, 1)))
		b.PushClipRect(gfx.RectXYWH(0.3, 0.3, 50, 60))
		b.PushLayer(0.7, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(-10, -10, 80, 80), sky)
		b.FillPath(gfx.CirclePath(gfx.Pt(25, 30), 22), gfx.NonZero, slate)
		b.PopLayer()
		b.PopClip()
		b.PopTransform()
		// Clip rect wider than the canvas and a path clip far outside.
		b.PushClipRect(gfx.Rect{X0: -1e6, Y0: 128, X1: 1e6, Y1: 146.5})
		b.FillRect(gfx.RectXYWH(160, 120, 76, 40), lime)
		b.PopClip()
	}},
	{"layers", 240, 130, func(b *gfx.Builder) {
		checker(b, gfx.RectXYWH(0, 0, 240, 130), 10)
		// Two overlapping opaque shapes in a 50% layer: no inner overlap.
		b.PushLayer(0.5, gfx.SrcOver)
		b.FillPath(gfx.CirclePath(gfx.Pt(36, 36), 26), gfx.NonZero, rose)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(36, 20, 50, 40), 8), indigo)
		b.PopLayer()
		// The same shapes each at 50%: the overlap shows.
		b.FillPath(gfx.CirclePath(gfx.Pt(136, 36), 26), gfx.NonZero, rose.WithAlpha(0.5))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(136, 20, 50, 40), 8), indigo.WithAlpha(0.5))
		// A layer inside a fractional rounded clip: the clip applies to the
		// group as a whole.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(8.5, 72.5, 60, 50), 16))
		b.PushLayer(0.6, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(0, 60, 80, 70), emerald)
		b.FillPath(starPath(38, 98, 26), gfx.NonZero, amber)
		b.PopLayer()
		b.PopClip()
		// Nested layers with rotated content and a gradient.
		b.PushLayer(0.8, gfx.SrcOver)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(80, 72, 70, 50), 10), sky)
		b.PushTransform(gfx.RotateAbout(0.6, gfx.Pt(115, 97)))
		b.PushLayer(0.5, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(95, 82, 40, 30), slate)
		b.FillRect(gfx.RectXYWH(105, 77, 20, 40), gfx.LinearGradient{Start: gfx.Pt(105, 0), End: gfx.Pt(125, 0), Stops: stops(rose, amber)})
		b.PopLayer()
		b.PopTransform()
		b.PopLayer()
		// A full-opacity layer equals drawing directly (up to rounding).
		b.PushLayer(1, gfx.SrcOver)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(160, 72, 70, 50), 25), indigo.WithAlpha(0.7))
		b.PopLayer()
	}},
	{"strokes", 260, 220, func(b *gfx.Builder) {
		zig := func(x, y float32) gfx.Path {
			var p gfx.Path
			p.MoveTo(gfx.Pt(x, y+24))
			p.LineTo(gfx.Pt(x+18, y))
			p.LineTo(gfx.Pt(x+36, y+24))
			p.LineTo(gfx.Pt(x+54, y+4))
			p.LineTo(gfx.Pt(x+58, y+24))
			return p
		}
		guide := gfx.NewStrokeStyle(0.5)
		// Joins: miter, round, bevel.
		for i, j := range []gfx.LineJoin{gfx.JoinMiter, gfx.JoinRound, gfx.JoinBevel} {
			st := gfx.NewStrokeStyle(8)
			st.Join = j
			p := zig(12+float32(i)*82, 10)
			b.StrokePath(p, st, indigo)
			b.StrokePath(p, guide, gfx.White)
		}
		// Caps: butt, round, square, with the center line as a guide.
		for i, c := range []gfx.LineCap{gfx.CapButt, gfx.CapRound, gfx.CapSquare} {
			st := gfx.NewStrokeStyle(10)
			st.Cap = c
			var p gfx.Path
			x := 20 + float32(i)*82
			p.MoveTo(gfx.Pt(x, 52))
			p.LineTo(gfx.Pt(x+50, 60))
			b.StrokePath(p, st, rose)
			b.StrokePath(p, guide, gfx.White)
		}
		// Dashes: a dashed line with round caps and an offset, a dashed
		// circle and a dashed rounded rectangle.
		dash := gfx.NewStrokeStyle(4)
		dash.Cap = gfx.CapRound
		dash.Dashes = []float32{10, 6}
		dash.DashOffset = 3
		var line gfx.Path
		line.MoveTo(gfx.Pt(12, 80))
		line.LineTo(gfx.Pt(248, 84))
		b.StrokePath(line, dash, emerald)
		dash2 := gfx.NewStrokeStyle(3)
		dash2.Dashes = []float32{6, 3, 1, 3}
		b.StrokePath(gfx.CirclePath(gfx.Pt(36, 116), 22), dash2, slate)
		b.StrokePath(gfx.NewRRect(gfx.RectXYWH(70.5, 96.5, 60, 40), 10).Path(), dash2, amber)
		// Curves: an S cubic, a quadratic arch and an ellipse.
		curve := gfx.NewStrokeStyle(6)
		curve.Cap = gfx.CapRound
		var s gfx.Path
		s.MoveTo(gfx.Pt(146, 136))
		s.CubicTo(gfx.Pt(166, 80), gfx.Pt(196, 160), gfx.Pt(216, 100))
		b.StrokePath(s, curve, sky)
		var q gfx.Path
		q.MoveTo(gfx.Pt(222, 136))
		q.QuadTo(gfx.Pt(236, 90), gfx.Pt(250, 136))
		b.StrokePath(q, curve, indigo)
		b.StrokePath(gfx.EllipsePath(gfx.Pt(40, 172), 28, 14), gfx.NewStrokeStyle(2.5), rose)
		// A tight curve and a narrow loop, thick and translucent: the
		// outline self-overlaps but every pixel is painted once.
		tight := gfx.NewStrokeStyle(12)
		tight.Join = gfx.JoinRound
		b.StrokePath(tightCurve(80, 152), tight, indigo.WithAlpha(0.6))
		// A rotated, scaled star with miter joins.
		b.PushTransform(gfx.Translate(210, 180).Mul(gfx.Rotate(0.3)).Mul(gfx.Scale(1.2, 0.8)))
		b.StrokePath(starPath(0, 0, 26), gfx.NewStrokeStyle(3), emerald)
		b.PopTransform()
		// Hairlines at fractional positions.
		for i, w := range []float32{0.25, 0.5, 1, 1.5} {
			var p gfx.Path
			y := 200.3 + float32(i)*4.6
			p.MoveTo(gfx.Pt(8, y))
			p.LineTo(gfx.Pt(150, y+3))
			b.StrokePath(p, gfx.NewStrokeStyle(w), slate)
		}
	}},
}
