package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Scenes of layers and backdrop filters: opacity, every blend mode, masks,
// every filter alone and chained, blurs of every size, and the glass
// panels the GPU backend exists for.

// busy paints content with detail at every scale over r: stripes of one
// to four pixels, a fine checker, hard color edges, text and an image.
// A blur of it shows what a kernel does to each.
func busy(b *gfx.Builder, r gfx.Rect) {
	b.PushClipRect(r)
	b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X1, r.Y1), Stops: stops(sky, gfx.White, amber)})
	w := r.Width()
	for i, x := 0, r.X0; x < r.X0+w*0.3; i, x = i+1, x+float32(2+i%4)*2 {
		b.FillRect(gfx.RectXYWH(x, r.Y0, float32(1+i%4), r.Height()*0.45), []gfx.Color{slate, rose, gfx.White, indigo}[i%4])
	}
	gfxtest.Checker(b, gfx.RectXYWH(r.X0+w*0.32, r.Y0, w*0.2, r.Height()*0.4), 3)
	b.FillRect(gfx.RectXYWH(r.X0+w*0.55, r.Y0, w*0.2, r.Height()*0.5), emerald)
	b.FillRect(gfx.RectXYWH(r.X0+w*0.75, r.Y0, w*0.25, r.Height()*0.5), slate)
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(r.X0+w*0.78, r.Y0+6, w*0.18, r.Height()*0.3), 8), gfx.White)
	b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(r.X0+w*0.02, r.Y0+r.Height()*0.5, w*0.4, r.Height()*0.45), gfx.SampleLinear)
	for i := 0; i < 4; i++ {
		text(b, "SoAl iSo lAoS", 9+float32(i)*3, r.X0+w*0.45+float32(i)*0.4, r.Y0+r.Height()*0.6+float32(i)*float32(12+2*i), []gfx.Color{slate, rose, indigo, slate}[i])
	}
	b.PopClip()
}

// wallpaper is a 320x200 opaque picture: soft color fields, as a desktop
// wallpaper has, with a few sharp shapes.
var wallpaper = func() *gfx.Image {
	const w, h = 320, 200
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/w, float64(y)/h
			r := 0.5 + 0.5*math.Sin(6.1*fx+1.3*fy)
			g := 0.5 + 0.5*math.Sin(3.7*fy-2.2*fx+1)
			bl := 0.5 + 0.5*math.Cos(4.3*fx*fy+0.6)
			if d := math.Hypot(fx-0.7, fy-0.35); d < 0.12 {
				r, g, bl = 1, 0.95, 0.6
			}
			if (x/5+y/5)%9 == 0 {
				r, g, bl = r*0.4, g*0.4, bl*0.5
			}
			copy(img.Pix[img.PixOffset(x, y):], []uint8{uint8(255 * r), uint8(255 * g), uint8(255 * bl), 255})
		}
	}
	return gfx.NewImage(img)
}()

// glass paints a glass panel: the backdrop blurred and saturated, a
// translucent fill, a hairline border, and a soft shadow under it.
func glass(b *gfx.Builder, box gfx.RRect, blur float32, tint gfx.Color) {
	shadow(b, box, 0, 8, 24, -4, gfx.Black.WithAlpha(0.35), false)
	b.BackdropFilter(box, gfx.Blur{StdDev: blur}, gfx.Saturate(1.6))
	b.FillRRect(box, tint)
	var edge gfx.Path
	edge.AddRRect(box.Inset(0.5))
	b.StrokePath(edge, gfx.NewStrokeStyle(1), gfx.White.WithAlpha(0.35))
}

// maskOf returns a mask whose list draw records.
func maskOf(mode gfx.MaskMode, draw func(b *gfx.Builder)) gfx.Mask {
	return gfx.Mask{List: gfxtest.Build(draw), Mode: mode}
}

var layerScenes = []testScene{
	// Every blend mode over three backdrops: an opaque gradient, a
	// translucent one, and nothing (the target, which starts transparent),
	// with a source that is opaque, translucent and absent in places.
	{clear: gfx.Transparent, Scene: gfxtest.Scene{Name: "gpu_blend_modes", W: 432, H: 216, Draw: func(b *gfx.Builder) {
		for i, m := range gfxtest.AllBlendModes {
			x, y := float32(72*(i%6)), float32(72*(i/6))
			b.FillRect(gfx.RectXYWH(x+4, y+4, 64, 24), gfx.LinearGradient{Start: gfx.Pt(x+4, y), End: gfx.Pt(x+68, y),
				Stops: stops(rose, amber, emerald, sky, indigo), Interp: gfx.InterpSRGB})
			b.FillRect(gfx.RectXYWH(x+4, y+28, 64, 20), gfx.LinearGradient{Start: gfx.Pt(x+4, y), End: gfx.Pt(x+68, y),
				Stops: stops(gfx.White.WithAlpha(0.8), slate.WithAlpha(0.5), fuchsia.WithAlpha(0.2))})
			b.PushLayer(1, m)
			b.FillRect(gfx.RectXYWH(x+10, y+8, 24, 58), gfx.LinearGradient{Start: gfx.Pt(x, y+8), End: gfx.Pt(x, y+66),
				Stops: stops(amber, fuchsia, sky)})
			b.FillPath(gfx.CirclePath(gfx.Pt(x+46, y+36), 20), gfx.NonZero, indigo.WithAlpha(0.6))
			b.FillRect(gfx.RectXYWH(x+28, y+50, 36, 8), gfx.White)
			b.FillRect(gfx.RectXYWH(x+28, y+58, 36, 6), gfx.Black.WithAlpha(0.7))
			b.PopLayer()
		}
	}}},
	// Blend modes with an opacity, under rounded and path clips, a
	// rotation, and one inside another.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_blend_clips", W: 360, H: 200, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 360, 200))
		modes := []gfx.BlendMode{gfx.Multiply, gfx.Screen, gfx.Overlay, gfx.Difference, gfx.ColorDodge, gfx.SoftLight, gfx.Hue, gfx.Luminosity, gfx.PlusLighter, gfx.Exclusion}
		for i, m := range modes {
			x, y := float32(10+70*(i%5)), float32(10+95*(i/5))
			switch i % 3 {
			case 0:
				b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(x+2.5, y+2.5, 55, 75), 14))
			case 1:
				b.PushClipPath(gfxtest.StarPath(x+30, y+40, 38), gfx.NonZero)
			default:
				b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(x+30, y+40)))
			}
			b.PushLayer(0.4+0.06*float32(i), m)
			b.FillRect(gfx.RectXYWH(x, y, 60, 80), gfx.LinearGradient{Start: gfx.Pt(x, y), End: gfx.Pt(x+60, y+80), Stops: stops(rose, amber, sky)})
			b.PushLayer(0.8, modes[(i+3)%len(modes)])
			b.FillPath(gfx.CirclePath(gfx.Pt(x+30, y+40), 22), gfx.NonZero, emerald)
			b.PopLayer()
			b.PopLayer()
			if i%3 == 2 {
				b.PopTransform()
			} else {
				b.PopClip()
			}
		}
	}}},
	// Opacity: groups of overlapping content (which a layer composites
	// once), stacks three deep, under clips and transforms, an empty
	// layer, one of opacity 1, and layers that reach beyond the target.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_opacity", W: 360, H: 220, Draw: func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 0, 360, 220), 10)
		group := func(x, y float32) {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, y, 60, 40), 8), indigo)
			b.FillPath(gfx.CirclePath(gfx.Pt(x+50, y+30), 22), gfx.NonZero, rose)
			text(b, "SoA", 20, x+4, y+30, gfx.White)
		}
		for i, op := range []float32{1, 0.75, 0.5, 0.25, 0.04} {
			b.PushLayer(op, gfx.SrcOver)
			group(8+float32(i)*70, 8)
			b.PopLayer()
		}
		// Three deep, each with content of its own.
		b.PushLayer(0.8, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(10, 70, 120, 70), slate)
		b.PushLayer(0.6, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(30.5, 80.5, 90, 50), amber)
		b.PushLayer(0.5, gfx.SrcOver)
		group(50, 85)
		b.PopLayer()
		b.PopLayer()
		b.PopLayer()
		// Under a rounded clip, a path clip and a rotation.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(150.3, 70.6, 70, 50), 16))
		b.PushLayer(0.6, gfx.SrcOver)
		group(150, 72)
		b.PopLayer()
		b.PopClip()
		b.PushClipPath(gfx.CirclePath(gfx.Pt(265, 95), 30), gfx.NonZero)
		b.PushLayer(0.7, gfx.SrcOver)
		group(235, 72)
		b.PopLayer()
		b.PopClip()
		b.PushTransform(gfx.RotateAbout(-0.4, gfx.Pt(320, 100)))
		b.PushLayer(0.5, gfx.SrcOver)
		group(290, 80)
		b.PopLayer()
		b.PopTransform()
		// Nothing inside, and nothing that shows.
		b.PushLayer(0.5, gfx.SrcOver)
		b.PopLayer()
		b.PushLayer(0.5, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(0, 0, 50, 50), gfx.Transparent)
		b.PopLayer()
		// Larger than the target on every side.
		b.PushLayer(0.3, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(-200, 150, 900, 300), emerald)
		b.FillPath(gfx.CirclePath(gfx.Pt(360, 220), 60), gfx.NonZero, fuchsia)
		b.PopLayer()
		b.PushTransform(gfx.Scale(1.3, 0.8))
		b.PushLayer(0.6, gfx.SrcOver)
		group(20, 210)
		b.PopLayer()
		b.PopTransform()
	}}},
	// Masks: alpha and luminance, made of gradients, of text and of
	// shapes; with the layer's clip, an opacity, a blend mode and a
	// filter; a mask whose list has a layer and a mask of its own.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_masks", W: 400, H: 240, Draw: func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 0, 400, 240), 8)
		content := func(r gfx.Rect) {
			b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X1, r.Y1), Stops: stops(rose, amber, emerald, sky)})
			b.FillPath(gfx.CirclePath(gfx.Pt(r.X0+r.Width()/2, r.Y0+r.Height()/2), r.Height()/3), gfx.NonZero, indigo)
		}
		cell := func(i int) gfx.Rect { return gfx.RectXYWH(8+float32(i%4)*98, 8+float32(i/4)*78, 90, 70) }
		fade := func(r gfx.Rect, a, c gfx.Color) gfx.Paint {
			return gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X1, r.Y0), Stops: stops(a, c)}
		}
		// An alpha gradient, and a luminance one.
		r := cell(0)
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) { m.FillRect(r, fade(r, gfx.Black, gfx.Transparent)) }), 1, gfx.SrcOver)
		content(r)
		b.PopLayer()
		r = cell(1)
		b.PushMaskLayer(maskOf(gfx.MaskLuminance, func(m *gfx.Builder) { m.FillRect(r, fade(r, gfx.White, gfx.Black)) }), 1, gfx.SrcOver)
		content(r)
		b.PopLayer()
		// Text as a mask (background-clip: text), alpha and luminance of
		// colored text.
		r = cell(2)
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) { text(m, "SoA", 44, r.X0+2.3, r.Y0+52.6, gfx.Black) }), 1, gfx.SrcOver)
		content(r)
		b.PopLayer()
		r = cell(3)
		b.PushMaskLayer(maskOf(gfx.MaskLuminance, func(m *gfx.Builder) {
			text(m, "ioS", 44, r.X0+6, r.Y0+52, gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X1, r.Y0), Stops: stops(gfx.White, amber, rose)})
		}), 1, gfx.SrcOver)
		content(r)
		b.PopLayer()
		// A radial mask under a rounded clip, with an opacity.
		r = cell(4)
		b.PushClipRRect(gfx.NewRRect(r.Inset(6.5), 14))
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) {
			m.FillRect(r, gfx.RadialGradient{Center: gfx.Pt(r.X0+45, r.Y0+35), Radius: gfx.Pt(50, 40), Stops: stops(gfx.Black, gfx.Black.WithAlpha(0.1))})
		}), 0.7, gfx.SrcOver)
		content(r)
		b.PopLayer()
		b.PopClip()
		// A blend mode, and a filter: the mask cuts the filtered result.
		r = cell(5)
		b.FillRect(r, slate)
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) { m.FillPath(gfxtest.StarPath(r.X0+45, r.Y0+35, 40), gfx.EvenOdd, gfx.Black) }), 1, gfx.Screen)
		content(r)
		b.PopLayer()
		r = cell(6)
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) { m.FillRRect(gfx.NewRRect(r.Inset(10), 12), gfx.Black) }), 1, gfx.SrcOver, gfx.Blur{StdDev: 4})
		content(r.Inset(18))
		b.PopLayer()
		// A mask in a mask, with a layer in its list, under a rotation.
		r = cell(7)
		inner := maskOf(gfx.MaskLuminance, func(m *gfx.Builder) { m.FillRect(r, fade(r, gfx.White, gfx.Black.WithAlpha(0.4))) })
		b.PushTransform(gfx.RotateAbout(0.2, gfx.Pt(r.X0+45, r.Y0+35)))
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) {
			m.PushMaskLayer(inner, 1, gfx.SrcOver)
			m.FillPath(gfx.CirclePath(gfx.Pt(r.X0+45, r.Y0+35), 34), gfx.NonZero, gfx.Black)
			m.PushLayer(0.5, gfx.SrcOver)
			m.FillRect(r, gfx.Black)
			m.PopLayer()
			m.PopLayer()
		}), 1, gfx.SrcOver)
		content(r)
		b.PopLayer()
		b.PopTransform()
		// Masks that scroll past the target's edges, and one whose list
		// blurs its own backdrop.
		r = gfx.RectXYWH(-30, 170, 200, 90)
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) { text(m, "SolA", 80, r.X0, r.Y0+70, gfx.Black) }), 0.9, gfx.SrcOver)
		content(r)
		b.PopLayer()
		r = gfx.RectXYWH(200, 166, 190, 70)
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(m *gfx.Builder) {
			text(m, "iSoAl", 50, r.X0+4, r.Y0+52, gfx.Black)
			m.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(r.X0+60, r.Y0, 80, 70), 10), gfx.Blur{StdDev: 3})
		}), 1, gfx.SrcOver)
		content(r)
		b.PopLayer()
	}}},
	// Every filter alone, then chained in both orders, under a scale that
	// differs per axis and under a rotation.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_filters", W: 480, H: 300, Draw: func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 240, 480, 60), 6)
		subject := func(x, y float32) {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+12, y+12, 46, 30), 6), gfx.LinearGradient{Start: gfx.Pt(x+12, y), End: gfx.Pt(x+58, y), Stops: stops(rose, amber)})
			b.FillPath(gfxtest.StarPath(x+52, y+38, 16), gfx.NonZero, indigo)
			text(b, "So", 18, x+10, y+56, slate)
			b.FillRect(gfx.RectXYWH(x+40, y+46, 22, 2), emerald.WithAlpha(0.6))
		}
		drop := gfx.DropShadow{Offset: gfx.Pt(3, 4), StdDev: 2.5, Color: gfx.Black.WithAlpha(0.5)}
		chains := [][]gfx.Filter{
			{gfx.Blur{StdDev: 1.2}}, {gfx.Blur{StdDev: 3}}, {drop}, {gfx.DropShadow{Offset: gfx.Pt(-5, 2), Color: rose}},
			{gfx.Grayscale(1)}, {gfx.Sepia(0.8)}, {gfx.Invert(1)},
			{gfx.HueRotate(2)}, {gfx.Saturate(2.5)}, {gfx.Brightness(1.4), gfx.Contrast(1.6)}, {gfx.Blur{StdDev: 2}, gfx.Grayscale(1)},
			{gfx.Grayscale(1), gfx.Blur{StdDev: 2}}, {drop, gfx.Blur{StdDev: 1.5}}, {gfx.Blur{StdDev: 1.5}, drop},
			{drop, gfx.DropShadow{Offset: gfx.Pt(-4, -3), StdDev: 1, Color: sky.WithAlpha(0.8)}, gfx.Contrast(1.2)},
			{gfx.Blur{StdDev: 0.6}, gfx.Blur{StdDev: 2.2}, gfx.Invert(0.3)},
			// A color matrix that makes transparent pixels show.
			{gfx.ColorMatrix{M: [20]float32{1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0.3, 0, 0, 0, 0.8, 0.2}}},
			{gfx.DropShadow{Offset: gfx.Pt(0, 6), StdDev: 6, Color: indigo.WithAlpha(0.7)}},
		}
		for i, fs := range chains {
			x, y := float32(4+68*(i%7)), float32(4+78*(i/7))
			op := float32(1)
			if i%5 == 4 {
				op = 0.7
			}
			b.PushLayer(op, gfx.SrcOver, fs...)
			subject(x, y)
			b.PopLayer()
		}
		b.PushTransform(gfx.Translate(300, 170).Mul(gfx.Scale(1.6, 0.7)))
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 2}, drop)
		subject(0, 0)
		b.PopLayer()
		b.PopTransform()
		b.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(430, 210)))
		b.PushLayer(1, gfx.SrcOver, drop, gfx.Sepia(1))
		subject(395, 175)
		b.PopLayer()
		b.PopTransform()
		// On the checker, with a blend mode and under a clip that cuts the
		// result, not the content.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20.5, 246.5, 120, 46), 10))
		b.PushLayer(0.9, gfx.Multiply, gfx.Blur{StdDev: 3})
		b.FillRect(gfx.RectXYWH(0, 250, 200, 20), rose)
		text(b, "SoAlio", 30, 24, 286, indigo)
		b.PopLayer()
		b.PopClip()
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 1.5})
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(2, 2), StdDev: 1, Color: slate})
		text(b, "lAoS", 34, 170, 286, amber)
		b.PopLayer()
		b.PopLayer()
		// Small layers, whose steps all fit the smallest targets of the
		// pool and take each other's.
		for i, fs := range chains {
			x := 300 + float32(i%9)*20
			y := 252 + float32(i/9)*22
			b.PushLayer(1, gfx.SrcOver, fs...)
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, y, 12, 12), 3), []gfx.Color{rose, indigo, emerald}[i%3])
			b.FillRect(gfx.RectXYWH(x+4, y+4, 4, 4), gfx.White)
			b.PopLayer()
		}
	}}},
	// Blurs of every size, from a third of a pixel to 60 (120 device
	// pixels at scale 2), of content with stripes, a checker, hard edges
	// and text: the exact kernels, the three boxes, and every reduction.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_blurs", W: 720, H: 560, Draw: func(b *gfx.Builder) {
		cell := func(r gfx.Rect, sigma float32) {
			b.PushClipRect(r)
			b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: sigma})
			busy(b, r.Inset(-40))
			b.PopLayer()
			b.PopClip()
		}
		for i, s := range []float32{0.3, 0.7, 1, 1.5, 2, 2.6} {
			cell(gfx.RectXYWH(4+float32(i)*119, 4, 115, 80), s)
		}
		for i, s := range []float32{3.3, 4.5, 6, 8, 11} {
			cell(gfx.RectXYWH(4+float32(i)*143, 88, 139, 110), s)
		}
		for i, s := range []float32{15, 20, 28} {
			cell(gfx.RectXYWH(4+float32(i)*238, 202, 234, 150), s)
		}
		for i, s := range []float32{40, 60} {
			cell(gfx.RectXYWH(4+float32(i)*357, 356, 353, 200), s)
		}
	}}},
	// Blurred content at the edges: of its layer (shapes that end where
	// the layer does, isolated shapes whose blur fades into nothing), of
	// the clip, and of the target, with content that goes on beyond it.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_blur_edges", W: 400, H: 260, Draw: func(b *gfx.Builder) {
		for i, s := range []float32{1.5, 5, 9, 14, 24} {
			x := 10 + float32(i)*78
			b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: s})
			b.FillRect(gfx.RectXYWH(x, 14, 50, 50), []gfx.Color{rose, indigo, emerald, amber, sky}[i])
			b.PopLayer()
		}
		// Beyond the target on each side; the blur reads it there.
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 6})
		b.FillRect(gfx.RectXYWH(-100, 90, 130, 40), slate)
		b.FillRect(gfx.RectXYWH(370, 90, 300, 40), rose)
		b.FillRect(gfx.RectXYWH(150, 232, 100, 400), indigo)
		b.FillPath(gfx.CirclePath(gfx.Pt(200, -8), 30), gfx.NonZero, emerald)
		b.PopLayer()
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 22})
		b.FillRect(gfx.RectXYWH(-300, 180, 360, 200), amber)
		b.FillRect(gfx.RectXYWH(340, 170, 200, 200), sky)
		b.PopLayer()
		// A clip cuts the blurred picture, whose content goes on under it.
		b.PushClipRect(gfx.RectXYWH(80, 100, 90, 60))
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 8})
		busy(b, gfx.RectXYWH(40, 80, 200, 110))
		b.PopLayer()
		b.PopClip()
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(230.4, 100.4, 100, 60), 20))
		b.PushLayer(0.8, gfx.SrcOver, gfx.Blur{StdDev: 13})
		busy(b, gfx.RectXYWH(180, 60, 220, 150))
		b.PopLayer()
		b.PopClip()
		// A layer whose content fills it exactly, in a layer.
		b.PushLayer(0.9, gfx.SrcOver)
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 3.5})
		b.FillRect(gfx.RectXYWH(96, 180, 60, 50), fuchsia)
		text(b, "So", 30, 100, 216, gfx.White)
		b.PopLayer()
		b.PopLayer()
	}}},
	// Drop shadows: offsets in every direction, none, large; colors opaque
	// and translucent; from sharp to very soft; of text, of translucent
	// content, of content that is itself a shadow.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_drop_shadows", W: 480, H: 280, Draw: func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(240, 0, 240, 280), gfxtest.TwSlate900)
		subject := func(x, y float32, c gfx.Color) {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, y, 40, 26), 6), c)
			b.FillPath(gfx.CirclePath(gfx.Pt(x+44, y+24), 12), gfx.NonZero, amber.WithAlpha(0.7))
			text(b, "Ao", 16, x+2, y+44, c)
		}
		shadows := []gfx.DropShadow{
			{Offset: gfx.Pt(4, 4), StdDev: 0, Color: gfx.Black.WithAlpha(0.5)},
			{Offset: gfx.Pt(-4, 3), StdDev: 1, Color: rose},
			{Offset: gfx.Pt(0, 0), StdDev: 3, Color: indigo},
			{Offset: gfx.Pt(0, 8), StdDev: 5, Color: gfx.Black.WithAlpha(0.6)},
			{Offset: gfx.Pt(6.5, -5.5), StdDev: 2.2, Color: emerald.WithAlpha(0.8)},
			{Offset: gfx.Pt(0, 12), StdDev: 12, Color: sky},
			{Offset: gfx.Pt(-20, 14), StdDev: 0.4, Color: amber.WithAlpha(0.5)},
			{Offset: gfx.Pt(10, 10), StdDev: 22, Color: fuchsia},
		}
		for i, ds := range shadows {
			x, y := float32(16+116*(i%4)), float32(20+90*(i/4))
			c := slate
			if x > 240 {
				c = gfx.White
			}
			b.PushLayer(1, gfx.SrcOver, ds)
			subject(x, y, c)
			b.PopLayer()
		}
		// Two shadows, the second of the first; an opacity; a large one of
		// text.
		b.PushLayer(0.8, gfx.SrcOver, shadows[1], shadows[3])
		subject(20, 210, indigo)
		b.PopLayer()
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, 3), StdDev: 30, Color: amber})
		text(b, "SoAl", 56, 250, 256, gfx.White)
		b.PopLayer()
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(2, 2), StdDev: 1.5, Color: gfx.Black.WithAlpha(0.7)})
		text(b, "iSo lAoS", 26, 110, 252, rose)
		b.PopLayer()
	}}},
	// Backdrop filters over busy content: blurs of several sizes, color
	// matrices, chains; rounded and rotated shapes; an opacity; clips;
	// translucent content above them; several that overlap, each reading
	// what the ones before painted; and shapes at the target's edges,
	// where the backdrop is mirrored.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_backdrop", W: 520, H: 340, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 260, 170))
		busy(b, gfx.RectXYWH(260, 0, 260, 170))
		busy(b, gfx.RectXYWH(0, 170, 260, 170))
		b.DrawImage(wallpaper, gfx.Rect{}, gfx.RectXYWH(260, 170, 260, 170), gfx.SampleLinear)
		box := func(x, y, w, h, r float32) gfx.RRect { return gfx.NewRRect(gfx.RectXYWH(x, y, w, h), r) }
		b.BackdropFilter(box(10, 10, 90, 60, 12), gfx.Blur{StdDev: 2})
		b.BackdropFilter(box(110.5, 10.5, 90, 60, 0), gfx.Blur{StdDev: 6})
		b.BackdropFilter(box(210, 10, 90, 60, 30), gfx.Blur{StdDev: 12})
		b.BackdropFilter(box(310, 10, 90, 60, 8), gfx.Grayscale(1))
		b.BackdropFilter(box(410, 10, 100, 60, 8), gfx.Invert(1), gfx.Blur{StdDev: 3}, gfx.Saturate(2))
		b.BackdropFilter(box(10, 80, 120, 80, 16), gfx.Blur{StdDev: 20})
		b.FillRRect(box(10, 80, 120, 80, 16), gfx.White.WithAlpha(0.3))
		text(b, "Solo iA", 18, 20, 126, slate)
		b.BackdropFilterOpacity(box(140, 80, 90, 80, 10), 0.5, gfx.Blur{StdDev: 8}, gfx.Brightness(0.6))
		b.PushClipPath(gfx.CirclePath(gfx.Pt(285, 120), 42), gfx.NonZero)
		b.BackdropFilter(box(240, 80, 90, 80, 0), gfx.Blur{StdDev: 5}, gfx.HueRotate(1.5))
		b.PopClip()
		b.PushTransform(gfx.RotateAbout(0.25, gfx.Pt(390, 120)))
		b.BackdropFilter(box(350, 90, 80, 60, 14), gfx.Blur{StdDev: 4})
		b.FillRRect(box(350, 90, 80, 60, 14), sky.WithAlpha(0.25))
		b.PopTransform()
		b.BackdropFilter(box(440, 80, 100, 80, 12), gfx.Blur{StdDev: 9}) // beyond the right edge
		// Overlapping: each reads the ones before.
		b.BackdropFilter(box(20, 190, 140, 90, 18), gfx.Blur{StdDev: 7})
		b.FillRRect(box(20, 190, 140, 90, 18), rose.WithAlpha(0.2))
		b.BackdropFilter(box(90, 230, 140, 90, 18), gfx.Blur{StdDev: 7}, gfx.Contrast(1.4))
		b.FillRRect(box(90, 230, 140, 90, 18), gfx.White.WithAlpha(0.25))
		b.BackdropFilter(box(130, 180, 60, 150, 30), gfx.Sepia(1))
		// At the corners and edges of the target.
		b.BackdropFilter(box(-20, 300, 80, 60, 10), gfx.Blur{StdDev: 10})
		b.BackdropFilter(box(250, 310, 120, 50, 0), gfx.Blur{StdDev: 16})
		b.BackdropFilter(box(470, 280, 70, 80, 20), gfx.Blur{StdDev: 28})
		b.BackdropFilter(box(380, 190, 110, 70, 12), gfx.Blur{StdDev: 40})
		b.FillRRect(box(380, 190, 110, 70, 12), gfx.Black.WithAlpha(0.2))
		text(b, "lAoS", 30, 392, 238, gfx.White)
	}}},
	// Backdrop filters inside layers, whose own pixels they read: an
	// opacity layer, a layer with a blend mode, one with filters, a layer
	// in a layer, under clips; and above and below layers of the frame.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_backdrop_layers", W: 440, H: 260, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 440, 260))
		box := func(x, y, w, h, r float32) gfx.RRect { return gfx.NewRRect(gfx.RectXYWH(x, y, w, h), r) }
		// In an opacity layer: it blurs the layer's own content only.
		b.PushLayer(0.85, gfx.SrcOver)
		b.FillRRect(box(10, 10, 130, 100, 10), slate)
		text(b, "SoAl iSo", 22, 18, 50, amber)
		b.FillRect(gfx.RectXYWH(10, 70, 130, 4), rose)
		b.BackdropFilter(box(40, 30, 80, 60, 12), gfx.Blur{StdDev: 4})
		b.FillRRect(box(40, 30, 80, 60, 12), gfx.White.WithAlpha(0.15))
		b.PopLayer()
		// In a layer with a blend mode, under a rounded clip.
		b.PushClipRRect(box(150.5, 10.5, 130, 100, 24))
		b.PushLayer(1, gfx.Multiply)
		busy(b, gfx.RectXYWH(150, 10, 130, 100))
		b.BackdropFilter(box(170, 20, 100, 50, 0), gfx.Blur{StdDev: 6}, gfx.Saturate(2))
		b.PopLayer()
		b.PopClip()
		// In a layer with filters.
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, 6), StdDev: 5, Color: gfx.Black.WithAlpha(0.5)})
		b.FillRRect(box(295, 14, 130, 90, 14), gfx.LinearGradient{Start: gfx.Pt(295, 14), End: gfx.Pt(425, 104), Stops: stops(indigo, rose, amber)})
		text(b, "olAS iolS", 20, 302, 60, gfx.White)
		b.BackdropFilter(box(320, 30, 90, 56, 10), gfx.Blur{StdDev: 3})
		b.PopLayer()
		// A layer in a layer, each with a filter that reads its own.
		b.PushLayer(0.9, gfx.SrcOver)
		busy(b, gfx.RectXYWH(10, 125, 200, 125))
		b.BackdropFilter(box(20, 135, 100, 60, 10), gfx.Blur{StdDev: 5})
		b.PushLayer(0.8, gfx.SrcOver)
		b.FillRRect(box(80, 160, 120, 80, 12), emerald)
		text(b, "AoS", 40, 90, 215, gfx.White)
		b.BackdropFilter(box(100, 180, 80, 50, 25), gfx.Blur{StdDev: 3}, gfx.Invert(1))
		b.PopLayer()
		b.BackdropFilter(box(150, 130, 50, 110, 8), gfx.Blur{StdDev: 2})
		b.PopLayer()
		// On the frame, between layers.
		b.PushLayer(0.6, gfx.SrcOver)
		b.FillPath(gfx.CirclePath(gfx.Pt(300, 190), 50), gfx.NonZero, fuchsia)
		b.PopLayer()
		b.BackdropFilter(box(250, 140, 170, 100, 20), gfx.Blur{StdDev: 10})
		b.FillRRect(box(250, 140, 170, 100, 20), gfx.White.WithAlpha(0.2))
		b.PushLayer(0.7, gfx.SrcOver, gfx.Blur{StdDev: 1})
		text(b, "Sio lAo", 30, 262, 200, slate)
		b.PopLayer()
	}}},
	// A desktop: a wallpaper, glass cards with text and shadows, one over
	// another, a dock, a translucent menu and a dialog that fades in.
	{clear: gfx.Black, Scene: gfxtest.Scene{Name: "gpu_glass", W: 640, H: 400, Draw: func(b *gfx.Builder) { glassDesktop(b, 640, 400) }}},
	// A dashboard of glass panels at the size of a screen: 3840x2160 at
	// scale 2, the frame of BenchmarkGlass.
	{clear: gfx.Black, Scene: gfxtest.Scene{Name: "gpu_dashboard", W: 1920, H: 1080, Draw: glassDashboard}},
}

// glassDesktop paints the glass composition for a window of w x h.
func glassDesktop(b *gfx.Builder, w, h float32) {
	b.DrawImage(wallpaper, gfx.Rect{}, gfx.RectXYWH(0, 0, w, h), gfx.SampleLinear)
	sx, sy := w/640, h/400
	b.PushTransform(gfx.Scale(sx, sy))
	defer b.PopTransform()
	box := func(x, y, bw, bh, r float32) gfx.RRect { return gfx.NewRRect(gfx.RectXYWH(x, y, bw, bh), r) }
	// A sidebar.
	side := box(16, 16, 150, 368, 18)
	glass(b, side, 24, gfx.White.WithAlpha(0.18))
	for i, s := range []string{"SoAl", "iSo lA", "oSil", "Ao lSi", "loS"} {
		y := 50 + float32(i)*34
		if i == 1 {
			b.FillRRect(box(26, y-20, 130, 28, 8), gfx.White.WithAlpha(0.28))
		}
		text(b, s, 15, 38, y, gfx.White)
	}
	// Cards.
	for i := 0; i < 3; i++ {
		x := 184 + float32(i)*148
		card := box(x, 20, 136, 104, 16)
		glass(b, card, 16, gfx.White.WithAlpha(0.22))
		text(b, "SoAli", 12, x+14, 46, gfx.White.WithAlpha(0.8))
		text(b, []string{"Aol", "iSo", "lAS"}[i], 34, x+14, 92, gfx.White)
		b.FillRRect(box(x+14, 104, 108, 6, 3), gfx.White.WithAlpha(0.25))
		b.FillRRect(box(x+14, 104, 30+float32(i)*30, 6, 3), []gfx.Color{emerald, amber, rose}[i])
	}
	// A wide panel with rows of text.
	panel := box(184, 140, 440, 190, 18)
	glass(b, panel, 24, slate.WithAlpha(0.35))
	for i := 0; i < 5; i++ {
		y := 176 + float32(i)*30
		text(b, "SolA il oSi Aolis lAoS", 14, 204, y, gfx.White.WithAlpha(0.92))
		text(b, "ioAl", 14, 560, y, emerald)
		b.FillRect(gfx.RectXYWH(204, y+9, 400, 1), gfx.White.WithAlpha(0.12))
	}
	// A dock over the panel's edge, and a menu over both.
	dock := box(250, 340, 300, 48, 24)
	glass(b, dock, 12, gfx.White.WithAlpha(0.25))
	for i := 0; i < 6; i++ {
		b.FillRRect(box(264+float32(i)*46, 348, 32, 32, 9), []gfx.Color{rose, amber, emerald, sky, indigo, fuchsia}[i])
	}
	menu := box(430, 96, 170, 130, 12)
	glass(b, menu, 30, gfx.White.WithAlpha(0.3))
	for i, s := range []string{"Aoli So", "iSol", "lAo iS"} {
		text(b, s, 14, 446, 126+float32(i)*32, slate)
	}
	// A dialog fading in: its backdrop at its opacity, then its content.
	dlg := box(230, 110, 220, 130, 20)
	b.BackdropFilterOpacity(dlg, 0.6, gfx.Blur{StdDev: 20}, gfx.Brightness(1.1))
	b.PushLayer(0.6, gfx.SrcOver)
	b.FillRRect(dlg, gfx.White.WithAlpha(0.35))
	text(b, "SoAl iSo?", 22, 252, 160, slate)
	b.FillRRect(box(340, 190, 90, 34, 10), indigo)
	text(b, "oAl", 16, 370, 213, gfx.White)
	b.PopLayer()
}

// glassDashboard paints a dashboard of glass panels for a window of
// 1920x1080 logical pixels, the kind of frame the GPU backend exists for:
// a wallpaper, a bar, a sidebar, eight cards and a table, each a blurred
// backdrop (12 to 24 pixels) with a tint, a border, a shadow and text.
func glassDashboard(b *gfx.Builder) {
	b.DrawImage(wallpaper, gfx.Rect{}, gfx.RectXYWH(0, 0, 1920, 1080), gfx.SampleLinear)
	box := func(x, y, w, h, r float32) gfx.RRect { return gfx.NewRRect(gfx.RectXYWH(x, y, w, h), r) }
	glass(b, box(16, 16, 1888, 56, 16), 20, gfx.White.WithAlpha(0.2))
	text(b, "SoAl iSo lAoS", 22, 40, 52, gfx.White)
	side := box(16, 88, 280, 976, 20)
	glass(b, side, 24, gfx.White.WithAlpha(0.16))
	for i := 0; i < 14; i++ {
		y := 140 + float32(i)*44
		if i == 3 {
			b.FillRRect(box(28, y-28, 256, 40, 10), gfx.White.WithAlpha(0.25))
		}
		text(b, "Sol iAo lSi", 18, 52, y, gfx.White)
	}
	for i := 0; i < 8; i++ {
		x, y := 312+float32(i%4)*400, 88+float32(i/4)*216
		glass(b, box(x, y, 384, 200, 18), 16, gfx.White.WithAlpha(0.2))
		text(b, "SoAli oSi", 16, x+24, y+40, gfx.White.WithAlpha(0.8))
		text(b, "AoS il", 44, x+24, y+110, gfx.White)
		text(b, "olAS iolS loo Ail SolA", 14, x+24, y+160, gfx.White.WithAlpha(0.7))
		b.FillRRect(box(x+24, y+174, 336, 8, 4), gfx.White.WithAlpha(0.25))
		b.FillRRect(box(x+24, y+174, 60+float32(i)*30, 8, 4), []gfx.Color{emerald, amber, rose, sky}[i%4])
	}
	glass(b, box(312, 520, 1592, 544, 20), 24, slate.WithAlpha(0.3))
	for i := 0; i < 12; i++ {
		y := 574 + float32(i)*40
		text(b, "SolA il oSi Aolis lAoS ioAl SolA il oSi Aolis lAoS ioAl", 16, 344, y, gfx.White.WithAlpha(0.92))
		text(b, "ioAl", 16, 1800, y, emerald)
		b.FillRect(gfx.RectXYWH(344, y+12, 1528, 1), gfx.White.WithAlpha(0.12))
	}
}
