package gpu

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/gfxtest/showcase"
)

// testScene is a scene of the tests: a shared scene of the rasterizer's
// goldens or one of this package's, with the color the frame starts from.
type testScene struct {
	gfxtest.Scene
	clear gfx.Color
	// list, when not nil, builds the display list instead of Draw.
	list func() *gfx.DisplayList
}

func (s *testScene) displayList() *gfx.DisplayList {
	if s.list != nil {
		return s.list()
	}
	return gfxtest.Build(s.Draw)
}

// allScenes returns the rasterizer's shared golden scenes, on white like
// its goldens, then the scenes of this package.
func allScenes() []testScene {
	var out []testScene
	for _, s := range gfxtest.Scenes() {
		out = append(out, testScene{Scene: s, clear: gfx.White})
	}
	out = append(out, testScene{Scene: gfxtest.Scene{Name: "showcase", W: 960, H: 640}, clear: gfx.White,
		list: func() *gfx.DisplayList { return showcase.Scene(960, 640) }})
	return append(append(out, gpuScenes...), textureScenes...)
}

var (
	slate   = gfxtest.Slate
	indigo  = gfxtest.Indigo
	rose    = gfxtest.Rose
	emerald = gfxtest.Emerald
	amber   = gfxtest.Amber
	sky     = gfxtest.Sky
	fuchsia = gfxtest.Fuchsia
)

func stops(cs ...gfx.Color) []gfx.GradientStop { return gfxtest.Stops(cs...) }

func stop(o float32, c gfx.Color) gfx.GradientStop { return gfxtest.Stop(o, c) }

// hardStops are four stripes with hard stops at fractional offsets and a
// smooth segment between two of them.
func hardStops() []gfx.GradientStop {
	return []gfx.GradientStop{
		stop(0, rose), stop(0.21, rose), stop(0.21, gfx.White), stop(0.43, sky),
		stop(0.43, slate), stop(0.77, amber), stop(0.77, indigo), stop(1, indigo),
	}
}

// shadowCard draws, under the transform m, a white card with per-corner
// radii, an outer shadow of the given blur and an inset one of half of it.
func shadowCard(b *gfx.Builder, m gfx.Affine, blur float32) {
	box := gfx.RRectCorners(gfx.RectXYWH(-36, -24, 72, 48), gfx.Pt(16, 16), gfx.Pt(4, 4), gfx.Pt(20, 10), gfx.Pt(0, 0))
	b.PushTransform(m)
	shadow(b, box, 3, 6, blur, -1, gfxtest.TwSlate900.WithAlpha(0.6), false)
	b.FillRRect(box, gfx.White)
	shadow(b, box, 2, 3, blur/2, 1, gfxtest.TwIndigo500.WithAlpha(0.6), true)
	b.PopTransform()
}

func shadow(b *gfx.Builder, box gfx.RRect, x, y, blur, spread float32, c gfx.Color, inset bool) {
	b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(x, y), Blur: blur, Spread: spread, Color: c, Inset: inset})
}

// gpuScenes cover the primitives of the quad pipeline beyond what the
// shared scenes do.
var gpuScenes = []testScene{
	{Scene: gfxtest.Scene{Name: "gpu_rects", W: 200, H: 120, Draw: func(b *gfx.Builder) {
		// Fractional positions and sizes, in steps of 1/7 and 1/3.
		for i := 0; i < 8; i++ {
			f := float32(i) / 7
			b.FillRect(gfx.RectXYWH(6+float32(i)*24+f, 6+f/3, 17+f, 15.5+f), indigo)
			b.FillRect(gfx.RectXYWH(6+float32(i)*24+f/3, 28+f, 20.3, 9.7), emerald.WithAlpha(0.75))
		}
		// Slivers thinner than a pixel, and pixel-sized squares between
		// pixels.
		for i := 0; i < 12; i++ {
			w := 0.08 * float32(i+1)
			b.FillRect(gfx.RectXYWH(6+float32(i)*7+0.37, 44, w, 22), slate)
			b.FillRect(gfx.RectXYWH(100, 44+float32(i)*1.9+0.21, 90, w), rose)
		}
		// Translucent colors over each other.
		for i, c := range []gfx.Color{rose, amber, sky, emerald, fuchsia} {
			b.FillRect(gfx.RectXYWH(8+float32(i)*18.4, 72.3+float32(i)*3.1, 60.5, 24.25), c.WithAlpha(0.2+0.15*float32(i)))
		}
		b.FillRect(gfx.RectXYWH(120.5, 70.5, 70, 44), slate.WithAlpha(0.5))
		b.FillRect(gfx.RectXYWH(130.25, 76.75, 70, 30), gfx.White.WithAlpha(0.5))
		b.FillRect(gfx.RectXYWH(140, 84, 40, 40), gfx.Black.WithAlpha(0.004)) // rounds to one level
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_rrects", W: 260, H: 200, Draw: func(b *gfx.Builder) {
		// Radii from a fraction of a pixel to the whole shape.
		for i, r := range []float32{0.3, 0.75, 1, 1.5, 2.25, 3, 5, 8, 13} {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(6+float32(i)*28+float32(i)/9, 6.2, 24, 24), r), indigo)
		}
		// Circles of every small size.
		for i := 0; i < 12; i++ {
			d := 1.5 + float32(i)*1.3
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(6+float32(i)*20+0.4, 36.6, d, d), 999), slate)
		}
		// Elliptical radii, each corner its own.
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(6.5, 60.5, 80, 50),
			gfx.Pt(30, 8), gfx.Pt(6, 22), gfx.Pt(40, 25), gfx.Pt(2.5, 2.5)), rose)
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(94.3, 60.7, 70, 50),
			gfx.Pt(70, 50), gfx.Pt(0, 0), gfx.Pt(0, 0), gfx.Pt(0, 0)), emerald)
		b.FillRRect(gfx.RRectXY(gfx.RectXYWH(172.6, 60.2, 80, 50), 40, 6), amber)
		// Huge radii: a circle much larger than the scene, twice, and a
		// long flat ellipse.
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(-900.3, 118.4, 1000, 1000), 500), sky.WithAlpha(0.6))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(180.7, 150.2, 3000, 3000), 1500), fuchsia.WithAlpha(0.6))
		b.FillRRect(gfx.RRectXY(gfx.RectXYWH(60.5, 180.5, 1200, 16), 600, 8), slate)
		// Translucent rounded shapes over each other.
		for i := 0; i < 4; i++ {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(110+float32(i)*14.5, 122+float32(i)*8.3, 50, 34), 9+float32(i)*4), indigo.WithAlpha(0.35))
		}
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_transforms", W: 260, H: 200, Draw: func(b *gfx.Builder) {
		card := gfx.NewRRect(gfx.RectXYWH(-32, -20, 64, 40), 10)
		// Rotations of a rectangle and of a rounded one.
		for i, a := range []float32{0.1, 0.5, 1.0, 1.4} {
			b.PushTransform(gfx.Translate(36+float32(i)*62, 30).Mul(gfx.Rotate(a)))
			b.FillRect(gfx.RectXYWH(-24, -14, 48, 28), indigo)
			b.PopTransform()
			b.PushTransform(gfx.Translate(36+float32(i)*62, 84).Mul(gfx.Rotate(-a)))
			b.FillRRect(card, rose.WithAlpha(0.85))
			b.PopTransform()
		}
		// Skews, a non-uniform scale with a rotation, flips.
		b.PushTransform(gfx.Translate(40, 140).Mul(gfx.Skew(0.5, 0)))
		b.FillRRect(card, emerald)
		b.PopTransform()
		b.PushTransform(gfx.Translate(110, 140).Mul(gfx.Skew(0.2, -0.3)))
		b.FillRect(gfx.RectXYWH(-24, -16, 48, 32), amber)
		b.PopTransform()
		b.PushTransform(gfx.Translate(175, 140).Mul(gfx.Rotate(0.7)).Mul(gfx.Scale(1.4, 0.5)))
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(-30, -20, 60, 40), gfx.Pt(20, 20), gfx.Pt(0, 0), gfx.Pt(12, 6), gfx.Pt(4, 4)), sky)
		b.PopTransform()
		b.PushTransform(gfx.Translate(232, 140).Mul(gfx.Scale(-0.7, -1.3)))
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(-30.3, -20.6, 60, 40), gfx.Pt(24, 12), gfx.Pt(0, 0), gfx.Pt(6, 6), gfx.Pt(0, 0)), slate)
		b.PopTransform()
		// Quarter turns with a scale: still axis-aligned, per-corner radii
		// turn with the shape.
		b.PushTransform(gfx.Affine{0, 1.25, -0.75, 0, 60.3, 172.4})
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(0, 0, 18, 60), gfx.Pt(9, 9), gfx.Pt(0, 0), gfx.Pt(3, 6), gfx.Pt(0, 0)), fuchsia)
		b.PopTransform()
		b.PushTransform(gfx.Translate(120, 176).Mul(gfx.Rotate(math.Pi / 2)))
		b.FillRect(gfx.RectXYWH(0.3, 0.3, 18.4, 44.5), indigo)
		b.PopTransform()
		// A thin rotated line and a tiny rotated circle.
		b.PushTransform(gfx.Translate(150, 186).Mul(gfx.Rotate(0.15)))
		b.FillRect(gfx.RectXYWH(0, 0, 90, 0.6), slate)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(96, -4, 6, 6), 3), rose)
		b.PopTransform()
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_linear", W: 260, H: 200, Draw: func(b *gfx.Builder) {
		// Every spread mode, in both color spaces, the gradient line
		// covering the middle third.
		for i, sp := range []gfx.SpreadMode{gfx.SpreadPad, gfx.SpreadReflect, gfx.SpreadRepeat} {
			for j, cs := range []gfx.ColorSpace{gfx.InterpOklab, gfx.InterpSRGB} {
				r := gfx.RectXYWH(6+float32(j)*126, 6+float32(i)*20, 120, 16)
				b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0+40, r.Y0), End: gfx.Pt(r.X0+80, r.Y1),
					Stops: stops(gfxtest.Blue, gfxtest.Yellow, rose), Interp: cs, Spread: sp})
			}
		}
		// Hard stops, horizontal at a fractional place, then repeated on a
		// slant.
		r := gfx.RectXYWH(6.4, 68.3, 248.2, 18)
		b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, 0), End: gfx.Pt(r.X1, 0), Stops: hardStops()})
		r = gfx.RectXYWH(6, 90, 248, 18)
		b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(10.37, 90.21), End: gfx.Pt(47.13, 101.4), Stops: hardStops(), Spread: gfx.SpreadRepeat, Interp: gfx.InterpSRGB})
		// Translucent stops over what is below, and out-of-gamut stops.
		b.FillRect(gfx.RectXYWH(6, 112, 248, 30), slate)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10.5, 104.5, 240, 40), 12), gfx.LinearGradient{
			Start: gfx.Pt(10, 0), End: gfx.Pt(250, 0), Stops: stops(sky.WithAlpha(0.9), gfx.Transparent, amber.WithAlpha(0.5))})
		b.FillRect(gfx.RectXYWH(6, 148, 120, 14), gfx.LinearGradient{
			Start: gfx.Pt(6, 0), End: gfx.Pt(126, 0), Stops: stops(gfxtest.OogGreen, gfxtest.OogYellow)})
		b.FillRect(gfx.RectXYWH(134, 148, 120, 14), gfx.LinearGradient{
			Start: gfx.Pt(134, 0), End: gfx.Pt(254, 0), Stops: stops(gfxtest.OogGreen, gfxtest.OogYellow), Interp: gfx.InterpSRGB})
		// Under a rotation and a skew; a vertical gradient.
		b.PushTransform(gfx.Translate(60, 182).Mul(gfx.Rotate(-0.1)).Mul(gfx.Skew(0.4, 0)))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(-50, -12, 100, 24), 8), gfx.LinearGradient{
			Start: gfx.Pt(-30, -12), End: gfx.Pt(30, 12), Stops: stops(indigo, fuchsia, amber), Spread: gfx.SpreadReflect})
		b.PopTransform()
		b.FillRect(gfx.RectXYWH(134.5, 168.5, 120, 26), gfx.LinearGradient{
			Start: gfx.Pt(0, 168.5), End: gfx.Pt(0, 194.5), Stops: stops(emerald, gfx.White, slate)})
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_radial", W: 260, H: 150, Draw: func(b *gfx.Builder) {
		for i, sp := range []gfx.SpreadMode{gfx.SpreadPad, gfx.SpreadReflect, gfx.SpreadRepeat} {
			r := gfx.RectXYWH(6+float32(i)*84, 6, 80, 60)
			b.FillRect(r, gfx.RadialGradient{Center: gfx.Pt(r.X0+30.3, r.Y0+24.6), Radius: gfx.Pt(22, 14),
				Stops: stops(amber, rose, slate), Spread: sp})
		}
		// Hard-stop rings, circular, at a fractional center.
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(6, 72, 72, 72), 14), gfx.RadialGradient{
			Center: gfx.Pt(41.7, 107.3), Radius: gfx.Pt(40, 40), Stops: hardStops(), Interp: gfx.InterpSRGB})
		// A translucent glow, and an ellipse under a skew and a rotation.
		b.FillRect(gfx.RectXYWH(86, 72, 80, 72), slate)
		b.FillRect(gfx.RectXYWH(86, 72, 80, 72), gfx.RadialGradient{Center: gfx.Pt(126, 108), Radius: gfx.Pt(44, 40),
			Stops: stops(gfx.White.WithAlpha(0.9), sky.WithAlpha(0.4), gfx.Transparent)})
		b.PushTransform(gfx.Translate(214, 108).Mul(gfx.Rotate(0.5)).Mul(gfx.Skew(0.3, 0)))
		b.FillRect(gfx.RectXYWH(-36, -24, 72, 48), gfx.RadialGradient{Radius: gfx.Pt(34, 12),
			Stops: stops(gfxtest.Lime, emerald, slate), Interp: gfx.InterpSRGB, Spread: gfx.SpreadReflect})
		b.PopTransform()
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_conic", W: 260, H: 110, Draw: func(b *gfx.Builder) {
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(6, 6, 96, 96), 48), gfx.ConicGradient{
			Center: gfx.Pt(54, 54), Interp: gfx.InterpSRGB, Stops: stops(
				gfx.RGB8(255, 0, 0), gfx.RGB8(255, 255, 0), gfx.RGB8(0, 255, 0),
				gfx.RGB8(0, 255, 255), gfx.RGB8(0, 0, 255), gfx.RGB8(255, 0, 255), gfx.RGB8(255, 0, 0))})
		// A pie with hard stops, off-center, starting at 3 o'clock.
		b.FillRect(gfx.RectXYWH(110, 6, 70, 96), gfx.ConicGradient{
			Center: gfx.Pt(138.3, 61.7), StartAngle: math.Pi / 2, Stops: hardStops()})
		// A smooth sweep with its seam, rotated, translucent.
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(220, 54)))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(190, 14, 60, 80), 12), gfx.ConicGradient{
			Center: gfx.Pt(220, 54), StartAngle: -math.Pi / 4, Stops: stops(slate, sky.WithAlpha(0.3), gfx.White)})
		b.PopTransform()
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_focal", W: 300, H: 200, Draw: func(b *gfx.Builder) {
		type cell struct {
			fx, fy, fr, cx, cy, r float32
			spread                gfx.SpreadMode
			interp                gfx.ColorSpace
		}
		for i, c := range []cell{
			{-20, -15, 0, 0, 0, 40, gfx.SpreadPad, gfx.InterpOklab},
			{-20, -15, 6, 0, 0, 30, gfx.SpreadReflect, gfx.InterpSRGB},
			{12, 8, 4, 0, 0, 18, gfx.SpreadRepeat, gfx.InterpOklab},
			{-30, 0, 8, 20, 0, 20, gfx.SpreadPad, gfx.InterpSRGB},      // a cone
			{-30, 0, 6, 20, 0, 14, gfx.SpreadRepeat, gfx.InterpSRGB},   // a narrowing cone
			{0, 0, 30, 10, -5, 4, gfx.SpreadPad, gfx.InterpOklab},      // shrinking
			{0, 0, 10, 0, 0, 40, gfx.SpreadReflect, gfx.InterpOklab},   // concentric
			{-25, 0, 20, 25, 0, 20, gfx.SpreadReflect, gfx.InterpSRGB}, // a strip
		} {
			x, y := float32(i%4)*74+4, float32(i/4)*98+4
			o := gfx.Pt(x+35.3, y+45.2) // off the pixel centers: see gpu_discontinuities
			st := stops(amber, rose, indigo)
			if i%2 == 1 {
				st = stops(sky.WithAlpha(0.2), emerald, slate, amber)
			}
			b.FillRect(gfx.RectXYWH(x, y, 70, 90), gfx.FocalGradient{
				Focal: gfx.Pt(o.X+c.fx, o.Y+c.fy), FocalRadius: c.fr, Center: gfx.Pt(o.X+c.cx, o.Y+c.cy), Radius: c.r,
				Stops: st, Spread: c.spread, Interp: c.interp})
		}
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_shadows", W: 420, H: 300, Draw: func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 420, Y1: 300}, gfxtest.TwGray100)
		col := gfxtest.TwSlate900
		// Outer shadows: blurs from none to large, with offsets and
		// positive and negative spreads, on rounded boxes.
		for i, s := range []struct{ x, y, blur, spread, radius float32 }{
			{0, 2, 0, 0, 8}, {0, 1, 1, 0, 8}, {0, 1, 2.5, 0.5, 8}, {3, 4, 6, 0, 8},
			{0, 8, 16, -4, 8}, {-6, 10, 30, 2, 8}, {0, 0, 12, 6, 0}, {4, 4, 9, -3, 30},
		} {
			box := gfx.NewRRect(gfx.RectXYWH(20+float32(i%4)*100+float32(i)/8, 20.3+float32(i/4)*80, 60, 44), s.radius)
			shadow(b, box, s.x, s.y, s.blur, s.spread, col.WithAlpha(0.55), false)
			b.FillRRect(box, gfx.White)
		}
		// Inset shadows on boxes with elliptical, per-corner radii.
		for i, s := range []struct{ x, y, blur, spread float32 }{
			{0, 2, 4, 0}, {4, 6, 12, 2}, {0, 0, 8, -3}, {-3, -3, 0, 4},
		} {
			box := gfx.RRectCorners(gfx.RectXYWH(20.5+float32(i)*100, 180.25, 64, 48),
				gfx.Pt(24, 10), gfx.Pt(4, 4), gfx.Pt(14, 22), gfx.Pt(0, 0))
			b.FillRRect(box, gfx.White)
			shadow(b, box, s.x, s.y, s.blur, s.spread, gfxtest.TwIndigo500.WithAlpha(0.7), true)
		}
		// Shapes whose radii fill them: a pill and a circle, and a box
		// much smaller than its blur.
		pill := gfx.NewRRect(gfx.RectXYWH(20, 250, 120, 30), 999)
		shadow(b, pill, 0, 6, 10, -2, col.WithAlpha(0.6), false)
		b.FillRRect(pill, gfx.White)
		circ := gfx.NewRRect(gfx.RectXYWH(170.4, 244.7, 40, 40), 20)
		shadow(b, circ, 5, 5, 3, 0, gfxtest.TwAmber400, false)
		b.FillRRect(circ, gfx.White)
		tiny := gfx.NewRRect(gfx.RectXYWH(260, 258, 10, 10), 2)
		shadow(b, tiny, 0, 0, 36, 8, gfxtest.TwIndigo500, false)
		b.FillRRect(tiny, gfxtest.TwIndigo500)
		// An inset shadow whose inner shape is empty: the box is filled.
		full := gfx.NewRRect(gfx.RectXYWH(320, 250, 60, 30), 6)
		b.FillRRect(full, gfx.White)
		shadow(b, full, 0, 0, 4, 40, gfxtest.TwSky400.WithAlpha(0.5), true)
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_shadows_rotated", W: 420, H: 230, Draw: func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 420, Y1: 230}, gfxtest.TwGray100)
		// Rotations with a uniform scale (the rasterizer evaluates these in
		// local space too), flips and different scales per axis, quarter
		// turns, and hard shadows under them.
		shadowCard(b, gfx.Translate(60, 55).Mul(gfx.Rotate(0.2)), 12)
		shadowCard(b, gfx.Translate(165, 55).Mul(gfx.Rotate(-0.9)).Mul(gfx.Scale(0.8, 0.8)), 6)
		shadowCard(b, gfx.Translate(270, 55).Mul(gfx.Scale(-1.2, 0.7)), 10)
		shadowCard(b, gfx.Translate(365, 55).Mul(gfx.Rotate(math.Pi/2)), 8)
		shadowCard(b, gfx.Translate(60, 165).Mul(gfx.Rotate(0.35)), 0)
		shadowCard(b, gfx.Translate(165, 165).Mul(gfx.Rotate(2.5)).Mul(gfx.Scale(1.3, 1.3)), 20)
		shadowCard(b, gfx.Translate(280, 165).Mul(gfx.Scale(1.4, -0.9)), 0)
		shadowCard(b, gfx.Translate(370, 165).Mul(gfx.Rotate(-math.Pi/2)), 1.5)
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_shadows_skewed", W: 300, H: 120, Draw: func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 300, Y1: 120}, gfxtest.TwGray100)
		// A skew and a rotation with different scales: the rasterizer blurs
		// a mask with the marginal standard deviations here.
		shadowCard(b, gfx.Translate(70, 60).Mul(gfx.Skew(0.4, 0)), 12)
		shadowCard(b, gfx.Translate(205, 60).Mul(gfx.Rotate(0.5)).Mul(gfx.Scale(1.3, 0.6)), 14)
	}}, clear: gfx.White},
	// Pixel centers exactly on a discontinuity of a paint: a hard stop of a
	// gradient, the seam of a conic gradient, the edge of a focal
	// gradient's cone, the wrap of a repeated gradient, the boundary of
	// two pixels of an image sampled by its nearest pixel; and hard stops
	// in a focal gradient, whose parameter is the root of a quadratic.
	// Which side a pixel on one takes depends on the last bits of the
	// paint's coordinate.
	{Scene: gfxtest.Scene{Name: "gpu_discontinuities", W: 300, H: 110, Draw: func(b *gfx.Builder) {
		half := []gfx.GradientStop{stop(0, rose), stop(0.5, rose), stop(0.5, indigo), stop(1, indigo)}
		// 101 pixels wide: the center of the middle column is at 0.5.
		r := gfx.RectXYWH(8, 8, 101, 20)
		b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, 0), End: gfx.Pt(r.X1, 0), Stops: half})
		// Repeating every 8 pixels from a pixel center.
		r = gfx.RectXYWH(8, 34, 101, 20)
		b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(8.5, 0), End: gfx.Pt(16.5, 0), Stops: stops(amber, slate), Spread: gfx.SpreadRepeat})
		// A conic gradient centered on a pixel center: the seam runs up
		// through centers, and the hard stop at half a turn down.
		b.FillRect(gfx.RectXYWH(8, 60, 44, 44), gfx.ConicGradient{Center: gfx.Pt(30.5, 82.5), Stops: half})
		// A strip (equal radii) whose edges run through pixel centers.
		b.FillRect(gfx.RectXYWH(60, 60, 60, 44), gfx.FocalGradient{
			Focal: gfx.Pt(70, 82), FocalRadius: 10.5, Center: gfx.Pt(110, 82), Radius: 10.5, Stops: stops(sky, emerald)})
		// Rings with hard stops at whole radii around a pixel center.
		b.FillRect(gfx.RectXYWH(124, 8, 88, 96), gfx.RadialGradient{Center: gfx.Pt(168.5, 56.5), Radius: gfx.Pt(40, 40),
			Stops: []gfx.GradientStop{stop(0, amber), stop(0.25, amber), stop(0.25, slate), stop(0.75, slate), stop(0.75, sky), stop(1, sky)}})
		// An image sampled by its nearest pixel, one and a half times its
		// size: every third pixel center falls on the boundary of two
		// image pixels (at scales 1 and 2).
		b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(112, 60, 9, 9), gfx.SampleNearest)
		// Hard stops in a reflected focal gradient.
		b.FillRect(gfx.RectXYWH(220, 8, 72, 96), gfx.FocalGradient{
			Focal: gfx.Pt(236.3, 41.2), FocalRadius: 6, Center: gfx.Pt(256.3, 56.2), Radius: 30,
			Stops: hardStops(), Spread: gfx.SpreadReflect, Interp: gfx.InterpSRGB})
	}}, clear: gfx.White},
	{Scene: gfxtest.Scene{Name: "gpu_clips", W: 260, H: 190, Draw: func(b *gfx.Builder) {
		grad := gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(260, 190), Stops: stops(sky, indigo, rose)}
		content := func(r gfx.Rect) {
			b.FillRect(r.Outset(20), grad)
			b.FillRRect(gfx.NewRRect(r.Inset(-6.5), 14), amber.WithAlpha(0.6))
			shadow(b, gfx.NewRRect(r.Inset(10), 6), 0, 4, 8, 0, gfx.Black.WithAlpha(0.6), false)
			b.FillRRect(gfx.NewRRect(r.Inset(10), 6), gfx.White)
		}
		// Without the shadow, which under a skew is gpu_shadows_skewed's
		// subject.
		flat := func(r gfx.Rect) {
			b.FillRect(r.Outset(20), grad)
			b.FillRRect(gfx.NewRRect(r.Inset(-6.5), 14), amber.WithAlpha(0.6))
			b.FillRect(r.Inset(10), gfx.White)
		}
		// A pixel-aligned clip (a scissor) and clips between pixels.
		for _, r := range []gfx.Rect{
			gfx.RectXYWH(8, 8, 70, 50), gfx.RectXYWH(90.3, 8.6, 70, 50.5), gfx.RectXYWH(172.5, 8.5, 70.25, 49.6),
		} {
			b.PushClipRect(r)
			content(r)
			b.PopClip()
		}
		// Clips under a rotation and under a skew.
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(48, 100)))
		r := gfx.RectXYWH(14, 76, 68, 48)
		b.PushClipRect(r)
		content(r)
		b.PopClip()
		b.PopTransform()
		b.PushTransform(gfx.Translate(130, 100).Mul(gfx.Skew(-0.3, 0.1)))
		r = gfx.RectXYWH(-34, -24, 68, 48)
		b.PushClipRect(r)
		flat(r)
		b.PopClip()
		b.PopTransform()
		// Nested clips: an aligned one inside a fractional one, a
		// fractional one inside an aligned one, and a fractional one well
		// inside another (the outer one covers its pixels entirely).
		outer := gfx.RectXYWH(176.4, 72.7, 76, 56)
		b.PushClipRect(outer)
		b.FillRect(outer, slate)
		b.PushClipRect(gfx.RectXYWH(184, 80, 30, 40))
		content(gfx.RectXYWH(184, 80, 30, 40))
		b.PopClip()
		b.PushClipRect(gfx.RectXYWH(220.5, 84.25, 24.5, 30.5))
		b.FillRect(outer, emerald)
		b.PopClip()
		b.PopClip()
		b.PushClipRect(gfx.RectXYWH(8, 136, 120, 46))
		b.PushClipRect(gfx.RectXYWH(14.5, 140.5, 300, 300))
		content(gfx.RectXYWH(8, 136, 120, 46))
		b.PopClip()
		b.PopClip()
		// A clip wider than the scene, and a quarter turn that leaves the
		// clip between pixels.
		b.PushClipRect(gfx.Rect{X0: -1e6, Y0: 140.5, X1: 1e6, Y1: 160.25})
		b.FillRect(gfx.RectXYWH(140, 130, 50, 50), fuchsia)
		b.PopClip()
		b.PushTransform(gfx.Affine{0, 1, -1, 0, 250, 138}.Mul(gfx.Scale(1.25, 1)))
		b.PushClipRect(gfx.RectXYWH(0.3, 0.3, 34, 44))
		b.FillRect(gfx.RectXYWH(-10, -10, 80, 80), grad)
		b.PopClip()
		b.PopTransform()
	}}, clear: gfx.White},
	// The same primitives over a transparent frame: the alpha channel
	// counts.
	{Scene: gfxtest.Scene{Name: "gpu_transparent", W: 200, H: 120, Draw: func(b *gfx.Builder) {
		box := gfx.NewRRect(gfx.RectXYWH(20.5, 16.5, 90, 60), 14)
		shadow(b, box, 0, 8, 18, -2, gfx.Black.WithAlpha(0.5), false)
		b.FillRRect(box, gfx.LinearGradient{Start: gfx.Pt(20, 16), End: gfx.Pt(110, 76), Stops: stops(indigo, fuchsia.WithAlpha(0.4))})
		shadow(b, box, 0, 2, 6, 0, gfx.White.WithAlpha(0.7), true)
		b.FillRect(gfx.RectXYWH(80.3, 50.7, 100, 50), sky.WithAlpha(0.4))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(130, 20, 50, 50), 25), gfx.RadialGradient{
			Center: gfx.Pt(150, 40), Radius: gfx.Pt(30, 30), Stops: stops(gfx.White, amber.WithAlpha(0.6), gfx.Transparent)})
		b.PushTransform(gfx.RotateAbout(0.4, gfx.Pt(60, 95)))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(30, 85, 60, 20), 6), emerald.WithAlpha(0.7))
		b.PopTransform()
	}}, clear: gfx.Transparent},
}
