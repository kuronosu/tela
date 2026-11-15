package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Scenes of what comes from textures: glyphs, paths, strokes, images and
// clips of every kind. They have no layers, so every one is compared with
// the rasterizer.

var font = gfxtest.FontA

func text(b *gfx.Builder, s string, size, x, y float32, p gfx.Paint) {
	b.DrawGlyphRun(font.Run(s, size, x, y, p))
}

// photo is a 96x64 opaque image with detail at every scale: a smooth
// gradient, a fine checker and thin diagonal lines.
var photo = func() *gfx.Image {
	const w, h = 96, 64
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl := uint8(255*x/(w-1)), uint8(255*y/(h-1)), uint8(128+127*((x/8+y/8)%2))
			switch {
			case (x+y)%12 == 0:
				r, g, bl = 15, 23, 42
			case x >= 40 && x < 56 && y >= 24 && y < 40 && (x+y)%2 == 0:
				r, g, bl = 255, 255, 255
			}
			copy(img.Pix[img.PixOffset(x, y):], []uint8{r, g, bl, 255})
		}
	}
	return gfx.NewImage(img)
}()

// blob is a closed shape of curves, in a box of about 2 x 2 around the
// origin, that crosses itself.
func blob() gfx.Path {
	var p gfx.Path
	p.MoveTo(gfx.Pt(-1, -0.2))
	p.CubicTo(gfx.Pt(-0.6, -1.4), gfx.Pt(0.9, -1.1), gfx.Pt(0.7, 0.1))
	p.QuadTo(gfx.Pt(0.5, 1.2), gfx.Pt(-0.4, 0.6))
	p.CubicTo(gfx.Pt(-1.3, 0), gfx.Pt(1.4, -0.6), gfx.Pt(0.2, 0.9))
	p.Close()
	return p
}

func zigzag(x, y, step, amp float32, n int) gfx.Path {
	var p gfx.Path
	p.MoveTo(gfx.Pt(x, y))
	for i := 1; i <= n; i++ {
		dy := amp
		if i%2 == 0 {
			dy = 0
		}
		p.LineTo(gfx.Pt(x+float32(i)*step, y+dy))
	}
	return p
}

var textureScenes = []testScene{
	// Text through the glyph cache: sizes from 7 to 40, pens at fractional
	// places (every quarter-pixel offset at some scale), on flat light and
	// dark backgrounds, translucent, painted with gradients, and the color
	// glyphs as sprites, opaque and faded.
	{Scene: gfxtest.Scene{Name: "gpu_text", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		y := float32(14)
		for i, size := range []float32{7, 9, 11, 13, 14, 16, 18, 21, 24} {
			text(b, "SolA il oSi Aolis lAoS ioAl", size, 6+float32(i)*0.27, y+float32(i)*0.13, slate)
			y += size + 5
		}
		b.FillRect(gfx.RectXYWH(0, y-6, 400, 60), slate)
		text(b, "AoSil oAlS iSo", 22, 8.6, y+16.4, gfx.White)
		text(b, "olAS iolS loo Ail SolA", 12, 8.3, y+34.7, gfx.White.WithAlpha(0.7))
		text(b, "Sio lAo", 12, 250.25, y+16, amber)
		text(b, "ASolli", 12, 250.5, y+34, sky.WithAlpha(0.5))
		y += 66
		text(b, "SAlo*oil%S", 30, 6.4, y+24, gfx.LinearGradient{Start: gfx.Pt(6, y), End: gfx.Pt(200, y+24), Stops: stops(indigo, fuchsia, amber)})
		text(b, "o*A%S", 24, 230.75, y+22, rose)
		text(b, "*%*", 24, 330.3, y+22.6, emerald.WithAlpha(0.5))
		text(b, "AoSl", 40, 6, y+68.5, gfx.RadialGradient{Center: gfx.Pt(60, y+50), Radius: gfx.Pt(70, 30), Stops: stops(sky, indigo, rose), Spread: gfx.SpreadReflect})
		text(b, "Sil", 40, 150.5, y+68, slate.WithAlpha(0.35))
		// Over each other, and off the edges of the target.
		text(b, "ooo", 40, 240, y+68, rose.WithAlpha(0.6))
		text(b, "ooo", 40, 252.5, y+62, sky.WithAlpha(0.6))
		text(b, "SAS", 36, -14.5, 296, indigo)
		text(b, "AoA", 36, 372.3, 310.5, indigo)
	}}, clear: gfx.White},
	// Glyphs the rasterizer does not cache: rotations, skews, flips, a
	// scale per axis (cached, at its own size), sizes beyond the cache,
	// and a color glyph drawn as a list, which draws itself again.
	{Scene: gfxtest.Scene{Name: "gpu_text_transformed", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		for i, a := range []float32{0.12, -0.35, 0.8, math.Pi / 2} {
			b.PushTransform(gfx.Translate(30+float32(i)*95, 50).Mul(gfx.Rotate(a)))
			text(b, "AoSl i@", 15+float32(i)*3, -20, 6, []gfx.Color{slate, indigo, rose, emerald}[i])
			b.PopTransform()
		}
		b.PushTransform(gfx.Translate(20, 120).Mul(gfx.Skew(-0.4, 0)))
		text(b, "SolAil", 22, 0, 0, gfx.LinearGradient{Start: gfx.Pt(0, -20), End: gfx.Pt(90, 0), Stops: stops(indigo, rose)})
		b.PopTransform()
		b.PushTransform(gfx.Translate(140, 104).Mul(gfx.Skew(0.15, 0.2)))
		text(b, "oSAl", 20, 0, 0, slate.WithAlpha(0.8))
		b.PopTransform()
		b.PushTransform(gfx.Translate(330, 120).Mul(gfx.Scale(-1, 1)))
		text(b, "SAil", 22, 0, 0, amber)
		b.PopTransform()
		b.PushTransform(gfx.Translate(250, 96).Mul(gfx.Scale(1, -1)))
		text(b, "Ao", 18, 0, 0, sky)
		b.PopTransform()
		b.PushTransform(gfx.Translate(8.3, 150.6).Mul(gfx.Scale(1.7, 0.8)))
		text(b, "lioS Aol", 16, 0, 0, fuchsia)
		b.PopTransform()
		// Larger than the cache holds (256 pixels of em at scale 1), cut by
		// the target; a huge translucent glyph over it.
		text(b, "S", 280, 190.4, 330.7, gfx.RadialGradient{Center: gfx.Pt(260, 230), Radius: gfx.Pt(90, 90), Stops: stops(sky, indigo)})
		text(b, "o", 520, -40.5, 420.25, rose.WithAlpha(0.25))
		text(b, "@", 150, 20, 296, slate)
	}}, clear: gfx.White},
	// Fills: both rules, shapes that cross themselves, shapes smaller than
	// a pixel, shapes much larger than the target and than a page of the
	// atlas, with solid and gradient paints.
	{Scene: gfxtest.Scene{Name: "gpu_paths", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		// Larger than everything: a disc of radius 1500 cut by the target,
		// and a ring around the whole scene.
		b.FillPath(gfx.CirclePath(gfx.Pt(200, 1620), 1500), gfx.NonZero, gfx.LinearGradient{
			Start: gfx.Pt(0, 120), End: gfx.Pt(400, 300), Stops: stops(sky.WithAlpha(0.35), indigo.WithAlpha(0.35))})
		var ring gfx.Path
		ring.AddEllipse(gfx.Pt(200, 150), 1900, 1900)
		ring.AddEllipse(gfx.Pt(200, 150), 236, 180)
		b.FillPath(ring, gfx.EvenOdd, slate.WithAlpha(0.5))
		for i, rule := range []gfx.FillRule{gfx.NonZero, gfx.EvenOdd} {
			x := 50 + float32(i)*190
			b.FillPath(gfxtest.StarPath(x, 60, 44), rule, []gfx.Color{indigo, rose}[i])
			b.PushTransform(gfx.Translate(x+90, 62.3).Mul(gfx.Scale(40, 34)))
			b.FillPath(blob(), rule, gfx.ConicGradient{Center: gfx.Pt(0, 0), Stops: stops(amber, emerald, sky, amber)})
			b.PopTransform()
		}
		// Rings and holes, translucent over each other.
		var holes gfx.Path
		holes.AddRect(gfx.RectXYWH(20.5, 120.5, 110, 70))
		holes.AddEllipse(gfx.Pt(55, 155), 22, 26)
		holes.AddRect(gfx.RectXYWH(90.25, 130.75, 30, 50))
		b.FillPath(holes, gfx.EvenOdd, emerald.WithAlpha(0.7))
		b.FillPath(holes.Transform(gfx.Translate(18.3, 14.6)), gfx.NonZero, fuchsia.WithAlpha(0.4))
		// Tiny: discs from a tenth of a pixel to three pixels, and slivers.
		for i := 0; i < 16; i++ {
			r := 0.1 + float32(i)*0.19
			b.FillPath(gfx.CirclePath(gfx.Pt(160.3+float32(i)*13.37, 130.6), r), gfx.NonZero, slate)
			var sliver gfx.Path
			sliver.MoveTo(gfx.Pt(158+float32(i)*13.37, 142))
			sliver.LineTo(gfx.Pt(158.2+float32(i)*13.37+r/4, 142.1))
			sliver.LineTo(gfx.Pt(166+float32(i)*13.37, 168))
			sliver.Close()
			b.FillPath(sliver, gfx.NonZero, rose)
		}
		// Curves under a rotation and a skew, with a radial gradient.
		b.PushTransform(gfx.Translate(210, 232).Mul(gfx.Rotate(0.35)).Mul(gfx.Skew(0.3, 0)).Mul(gfx.Scale(70, 44)))
		b.FillPath(blob(), gfx.NonZero, gfx.RadialGradient{Radius: gfx.Pt(1.2, 1.2), Stops: stops(gfx.White, amber, rose)})
		b.PopTransform()
		// The same shape at whole and fractional places: one picture, and
		// one per fraction.
		for i := 0; i < 5; i++ {
			b.FillPath(gfxtest.StarPath(300+float32(i)*20, 215, 9), gfx.NonZero, indigo)
			b.FillPath(gfxtest.StarPath(300.37+float32(i)*20.21, 240.64, 9), gfx.EvenOdd, slate)
		}
		b.FillPath(gfxtest.TightCurve(300, 250), gfx.NonZero, emerald)
	}}, clear: gfx.White},
	// Strokes: every join and cap, miter limits, dashes with offsets,
	// hairlines, closed shapes, under transforms, with gradients.
	{Scene: gfxtest.Scene{Name: "gpu_strokes", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		for j, join := range []gfx.LineJoin{gfx.JoinMiter, gfx.JoinRound, gfx.JoinBevel} {
			for c, cp := range []gfx.LineCap{gfx.CapButt, gfx.CapRound, gfx.CapSquare} {
				st := gfx.NewStrokeStyle(7)
				st.Join, st.Cap = join, cp
				b.StrokePath(zigzag(10+float32(c)*130, 12+float32(j)*30, 22, 16, 5), st, []gfx.Color{indigo, rose, emerald}[j])
			}
		}
		// A sharp angle under three miter limits.
		for i, lim := range []float32{1, 4, 12} {
			st := gfx.NewStrokeStyle(5)
			st.MiterLimit = lim
			var p gfx.Path
			p.MoveTo(gfx.Pt(14+float32(i)*60, 132))
			p.LineTo(gfx.Pt(40+float32(i)*60, 104))
			p.LineTo(gfx.Pt(50+float32(i)*60, 132))
			b.StrokePath(p, st, slate)
		}
		// Dashes: patterns, offsets and caps, on a line and on curves.
		for i := 0; i < 5; i++ {
			st := gfx.NewStrokeStyle(1 + float32(i))
			st.Dashes = [][]float32{{6, 3}, {1, 4}, {10, 2, 2, 2}, {0.5, 6}, {14, 5}}[i]
			st.DashOffset = float32(i) * 2.5
			st.Cap = []gfx.LineCap{gfx.CapButt, gfx.CapRound, gfx.CapSquare, gfx.CapRound, gfx.CapButt}[i]
			var l gfx.Path
			l.MoveTo(gfx.Pt(200.5, 104.5+float32(i)*8))
			l.LineTo(gfx.Pt(392, 104.5+float32(i)*8.4))
			b.StrokePath(l, st, []gfx.Color{slate, rose, indigo, emerald, amber}[i])
		}
		dash := gfx.NewStrokeStyle(4)
		dash.Dashes, dash.Join = []float32{9, 5}, gfx.JoinRound
		b.StrokePath(gfx.EllipsePath(gfx.Pt(60, 180), 46, 26), dash, gfx.LinearGradient{Start: gfx.Pt(14, 154), End: gfx.Pt(106, 206), Stops: stops(sky, fuchsia)})
		b.StrokePath(gfxtest.TightCurve(120, 152), dash, indigo)
		// Hairlines and thin strokes at fractional places.
		for i := 0; i < 8; i++ {
			st := gfx.NewStrokeStyle(0.15 + float32(i)*0.2)
			var l gfx.Path
			l.MoveTo(gfx.Pt(210+float32(i)*22.3, 150))
			l.LineTo(gfx.Pt(226+float32(i)*22.3, 206.4))
			l.LineTo(gfx.Pt(214+float32(i)*22.3, 206.4))
			b.StrokePath(l, st, slate)
		}
		// A wide translucent stroke that overlaps itself, closed shapes, and
		// strokes under a rotation and a scale per axis.
		wide := gfx.NewStrokeStyle(16)
		wide.Join, wide.Cap = gfx.JoinRound, gfx.CapRound
		b.StrokePath(gfxtest.StarPath(60, 252, 34), wide, rose.WithAlpha(0.45))
		thin := gfx.NewStrokeStyle(2.5)
		b.StrokePath(gfxtest.StarPath(60, 252, 34), thin, slate)
		b.PushTransform(gfx.Translate(170, 250).Mul(gfx.Rotate(0.4)).Mul(gfx.Scale(1.8, 0.7)))
		b.StrokePath(gfx.RectPath(gfx.RectXYWH(-24, -24, 48, 48)), dash, emerald)
		b.StrokePath(gfx.CirclePath(gfx.Pt(0, 0), 14), thin, amber)
		b.PopTransform()
		// Larger than the target: a circle of radius 900 whose arc crosses
		// the corner.
		b.StrokePath(gfx.CirclePath(gfx.Pt(1100, 1020), 900), gfx.NewStrokeStyle(9), gfx.LinearGradient{
			Start: gfx.Pt(240, 220), End: gfx.Pt(400, 300), Stops: stops(indigo, sky)})
		b.StrokePath(zigzag(250, 232, 18, 30, 8), gfx.NewStrokeStyle(3), fuchsia.WithAlpha(0.7))
	}}, clear: gfx.White},
	// Images: scaled up and down with both samplings, mip levels,
	// sub-rectangles, fractional places, flips, rotations, skews,
	// translucent pixels over a pattern.
	{Scene: gfxtest.Scene{Name: "gpu_images", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 0, 400, 300), 12)
		small := gfxtest.TestImage
		for i, s := range []gfx.Sampling{gfx.SampleLinear, gfx.SampleNearest} {
			y := 6 + float32(i)*70
			b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(6, y, 96, 64), s)             // 1:1 at whole pixels
			b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(108.4, y+0.3, 57.5, 38.3), s) // down, between pixels
			b.DrawImage(photo, gfx.Rect{X0: 36, Y0: 20, X1: 60, Y1: 44}, gfx.RectXYWH(172, y, 64, 64), s)
			b.DrawImage(photo.Level(1), gfx.Rect{}, gfx.RectXYWH(242.5, y, 40, 26.5), s)
			b.DrawImage(photo.Level(2), gfx.Rect{}, gfx.RectXYWH(242.5, y+32, 20.5, 14), s)
			b.DrawImage(photo.Level(9), gfx.Rect{}, gfx.RectXYWH(270, y+32, 12, 12), s) // 1x1
			b.DrawImage(small, gfx.Rect{}, gfx.RectXYWH(290.3, y+0.6, 66.6, 62.2), s)
			b.DrawImage(small, gfx.Rect{X0: 1.5, Y0: 1.5, X1: 4.5, Y1: 5}, gfx.RectXYWH(362, y, 30, 34), s)
			// Src partly outside, and narrower than a pixel.
			b.DrawImage(small, gfx.Rect{X0: -2, Y0: 3, X1: 4, Y1: 9}, gfx.RectXYWH(362, y+38, 30, 26), s)
			b.DrawImage(photo, gfx.Rect{X0: 47.3, Y0: 0, X1: 47.9, Y1: 64}, gfx.RectXYWH(108, y+44, 58, 16), s)
		}
		// Rotated, skewed, flipped and scaled per axis.
		for i, s := range []gfx.Sampling{gfx.SampleLinear, gfx.SampleNearest} {
			x := 50 + float32(i)*200
			b.PushTransform(gfx.Translate(x, 190).Mul(gfx.Rotate(0.4 - float32(i)*0.9)))
			b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(-36, -24, 72, 48), s)
			b.PopTransform()
			b.PushTransform(gfx.Translate(x+100, 180).Mul(gfx.Skew(0.3, -0.1)).Mul(gfx.Scale(-1, 1)))
			b.DrawImage(small, gfx.Rect{}, gfx.RectXYWH(-30, -24, 60, 48), s)
			b.PopTransform()
			b.PushTransform(gfx.Translate(x-40, 236).Mul(gfx.Scale(2.6, -0.9)))
			// (Off the places where a pixel center falls on the boundary of
			// two image pixels: see gpu_discontinuities.)
			b.DrawImage(photo, gfx.Rect{X0: 8, Y0: 8, X1: 88, Y1: 56}, gfx.RectXYWH(0, -60.3, 70, 54), s)
			b.PopTransform()
		}
		// Much larger than the target, and upside down by a half turn.
		b.PushTransform(gfx.Translate(396, 296).Mul(gfx.Rotate(math.Pi)))
		b.DrawImage(small, gfx.Rect{}, gfx.RectXYWH(0, 0, 40, 40), gfx.SampleLinear)
		b.PopTransform()
	}}, clear: gfx.White},
	// Rounded and path clips: circular, elliptical and per-corner radii,
	// at whole and fractional places, under scales, rotations and skews;
	// stars and curves with both rules; everything drawn through them.
	{Scene: gfxtest.Scene{Name: "gpu_clips_shapes", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		grad := gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(400, 300), Stops: stops(sky, indigo, rose, amber)}
		// The shadow is left out under a skew, where it is
		// gpu_shadows_skewed's subject.
		skewed := false
		content := func(r gfx.Rect) {
			b.FillRect(r.Outset(30), grad)
			b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(r.X0+r.Width()/2, r.Y0-6, r.Width()/2+10, r.Height()/2+6), gfx.SampleLinear)
			text(b, "SoAl", r.Height()/2, r.X0-3.4, r.Y1-4.3, gfx.White)
			b.StrokePath(gfx.EllipsePath(gfx.Pt(r.X0+r.Width()/2, r.Y0+r.Height()/2), r.Width()/2, r.Height()/3), gfx.NewStrokeStyle(4), slate)
			if !skewed {
				shadow(b, gfx.NewRRect(gfx.RectXYWH(r.X0+8, r.Y0+6, 20, 14), 4), 0, 3, 6, 0, gfx.Black.WithAlpha(0.6), false)
			}
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(r.X0+8, r.Y0+6, 20, 14), 4), gfx.White)
		}
		rrects := []gfx.RRect{
			gfx.NewRRect(gfx.RectXYWH(8, 8, 84, 60), 12),
			gfx.RRectXY(gfx.RectXYWH(104.3, 8.6, 84.5, 60.25), 40, 9),
			gfx.RRectCorners(gfx.RectXYWH(200.5, 8.5, 84, 60), gfx.Pt(30, 30), gfx.Pt(0, 0), gfx.Pt(42, 24), gfx.Pt(5, 14)),
			gfx.NewRRect(gfx.RectXYWH(296.25, 8.75, 60, 60), 999),
		}
		for _, rr := range rrects {
			b.PushClipRRect(rr)
			content(rr.Rect)
			b.PopClip()
		}
		// Under a scale per axis, a rotation, a skew and a quarter turn.
		for i, m := range []gfx.Affine{
			gfx.Translate(50, 118).Mul(gfx.Scale(1.25, 0.8)), gfx.Translate(150, 118).Mul(gfx.Rotate(0.3)),
			gfx.Translate(250, 118).Mul(gfx.Skew(0.35, 0.1)), gfx.Translate(350, 118).Mul(gfx.Rotate(math.Pi / 2)).Mul(gfx.Scale(1, -1)),
		} {
			b.PushTransform(m)
			rr := rrects[(i+1)%4]
			rr.Rect = gfx.RectXYWH(-36, -26, 72, 52)
			rr = rr.Normalize()
			b.PushClipRRect(rr)
			skewed = i == 2
			content(rr.Rect)
			skewed = false
			b.PopClip()
			b.PopTransform()
		}
		// Paths: a star with each rule, a shape of curves, a ring, a path
		// under a rotation, and a path larger than the target.
		for i, rule := range []gfx.FillRule{gfx.NonZero, gfx.EvenOdd} {
			b.PushClipPath(gfxtest.StarPath(52+float32(i)*96, 200, 46), rule)
			content(gfx.RectXYWH(8+float32(i)*96, 156, 88, 88))
			b.PopClip()
		}
		b.PushClipPath(blob().Transform(gfx.Translate(246, 200.4).Mul(gfx.Scale(44, 40))), gfx.NonZero)
		content(gfx.RectXYWH(202, 160, 88, 80))
		b.PopClip()
		b.PushTransform(gfx.RotateAbout(-0.5, gfx.Pt(346, 200)))
		var ring gfx.Path
		ring.AddEllipse(gfx.Pt(346, 200), 44, 36)
		ring.AddRect(gfx.RectXYWH(326.5, 186.5, 40, 26))
		b.PushClipPath(ring, gfx.EvenOdd)
		content(gfx.RectXYWH(302, 164, 88, 72))
		b.PopClip()
		b.PopTransform()
		b.PushClipPath(gfx.CirclePath(gfx.Pt(200, 2250), 2000), gfx.NonZero)
		b.FillRect(gfx.RectXYWH(0, 240, 400, 60), grad)
		text(b, "olAS iolS loo Ail SolA oSil", 24, 8.4, 290.6, gfx.White)
		b.PopClip()
	}}, clear: gfx.White},
	// Clips inside clips, of every kind in every order, deeper and deeper,
	// with edges that share pixels; under transforms that differ from one
	// level to the next.
	{Scene: gfxtest.Scene{Name: "gpu_clips_nested", W: 400, H: 300, Draw: func(b *gfx.Builder) {
		grad := gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(400, 300), Stops: stops(amber, rose, indigo, sky)}
		// push pushes the clip of a kind for a cell of about 90 x 90 at the
		// origin, and returns what pops it. The kinds under a transform
		// keep it for what is inside them: every level turns a bit more.
		push := func(kind byte) (pop func()) {
			const x, y = 0, 0
			turned := func(a float32) func() {
				b.PushTransform(gfx.RotateAbout(a, gfx.Pt(x+45, y+45)))
				return func() { b.PopClip(); b.PopTransform() }
			}
			switch kind {
			case 'r': // a rectangle between pixels
				b.PushClipRect(gfx.RectXYWH(x+4.4, y+8.6, 78.3, 70.5))
			case 'R': // a rounded rectangle
				b.PushClipRRect(gfx.RRectXY(gfx.RectXYWH(x+2.5, y+2.5, 84, 84), 30, 18))
			case 'P': // a star
				b.PushClipPath(gfxtest.StarPath(x+45, y+47, 46), gfx.NonZero)
			case 'O': // a disc, as a path
				b.PushClipPath(gfx.CirclePath(gfx.Pt(x+44.3, y+44.7), 41.5), gfx.NonZero)
			case 't': // a rotated rectangle
				pop = turned(0.35)
				b.PushClipRect(gfx.RectXYWH(x+8, y+12, 74, 66))
				return pop
			case 'T': // a rotated rounded rectangle
				pop = turned(-0.25)
				b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(x+6, y+10, 78, 70), 16))
				return pop
			case 'a': // a rectangle on pixel boundaries (a scissor)
				b.PushClipRect(gfx.RectXYWH(x+10, y+6, 70, 80))
			}
			return b.PopClip
		}
		content := func(x, y float32) {
			b.FillRect(gfx.RectXYWH(x-5, y-5, 100, 100), grad)
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+20.5, y+24.5, 50, 40), 12), gfx.White.WithAlpha(0.8))
			text(b, "SoA", 30, x+14.3, y+58.6, slate)
			b.StrokePath(gfx.CirclePath(gfx.Pt(x+45, y+45), 38), gfx.NewStrokeStyle(5), emerald.WithAlpha(0.8))
			b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(x+56, y+2, 30, 30), gfx.SampleLinear)
		}
		for i, stack := range []string{
			"rR", "Rr", "RR", "RP", "PR", "PO", "tT", "TP", // two
			"rRP", "RPO", "aRT", "PaR", "tRO", "TtP", "OOO", "RaP", // three
			"rRPO", "RPOt", "TaROP", "PRtOT", "aRaPaO", "rtRTPO", "OPTRtr", "RRRRRR", // more
		} {
			x, y := 6+float32(i%8)*49, 6+float32(i/8)*98
			b.PushTransform(gfx.Translate(x, y).Mul(gfx.Scale(0.5, 1)))
			var pops []func()
			for _, kind := range []byte(stack) {
				pops = append(pops, push(kind))
			}
			content(0, 0)
			for k := len(pops) - 1; k >= 0; k-- {
				pops[k]()
			}
			b.PopTransform()
		}
	}}, clear: gfx.White},
	// The same over a transparent frame, where the alpha channel counts:
	// text, a path, a stroke, an image and a sprite, clipped by a path
	// inside a rounded rectangle.
	{Scene: gfxtest.Scene{Name: "gpu_textures_transparent", W: 240, H: 140, Draw: func(b *gfx.Builder) {
		text(b, "SolA*il%", 26, 8.3, 30.4, indigo)
		b.FillPath(gfxtest.StarPath(40, 84, 34), gfx.EvenOdd, rose.WithAlpha(0.7))
		b.StrokePath(gfxtest.TightCurve(84, 44), gfx.NewStrokeStyle(4), emerald)
		b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(170.5, 44.5, 60, 60), gfx.SampleLinear)
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(60.5, 70.5, 110, 62), 20))
		b.PushClipPath(gfx.EllipsePath(gfx.Pt(120, 100), 62, 28), gfx.NonZero)
		b.FillRect(gfx.RectXYWH(50, 60, 140, 80), gfx.LinearGradient{Start: gfx.Pt(60, 70), End: gfx.Pt(170, 130), Stops: stops(amber.WithAlpha(0.3), sky)})
		text(b, "oAS*", 30, 76.6, 112.3, gfx.White.WithAlpha(0.9))
		b.PopClip()
		b.PopClip()
	}}, clear: gfx.Transparent},
}
