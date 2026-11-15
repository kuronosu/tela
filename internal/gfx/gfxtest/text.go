package gfxtest

import "github.com/kuronosu/tela/internal/gfx"

// TextScenes draw synthetic text (font.go): small and large sizes,
// fractional origins, solid and gradient paints, color glyphs, a clip with
// an opacity layer, a rotation and a non-uniform scale (path fallback), and
// glyphs too large to be cached.
var TextScenes = []Scene{
	{"text", 320, 240, func(b *gfx.Builder) {
		f := FontA
		run := func(s string, size, x, y float32, p gfx.Paint) { b.DrawGlyphRun(f.Run(s, size, x, y, p)) }
		run("olAS iolS loo Ail", 14, 8.3, 20, Slate)
		run("SolA il ? Soil", 9, 8, 34.4, Slate)
		run("AoS lio", 20, 190.6, 24.2, Indigo)
		run("SAlo*oil", 26, 8, 66, gfx.LinearGradient{Start: gfx.Pt(8, 46), End: gfx.Pt(140, 66), Stops: Stops(Indigo, Fuchsia)})
		run("o*A*S", 24, 170, 66, Rose)

		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(8, 76, 150, 40), 10))
		b.FillRect(gfx.RectXYWH(8, 76, 150, 40), TwGray100)
		b.PushLayer(0.6, gfx.SrcOver)
		run("SoAlSoAl", 30, 2.7, 108, Emerald)
		b.PopLayer()
		b.PopClip()

		b.PushTransform(gfx.RotateAbout(-0.3, gfx.Pt(240, 110)))
		run("AoS*l", 18, 196, 115, Amber)
		b.PopTransform()

		b.PushTransform(gfx.Translate(170, 140).Mul(gfx.Scale(1.6, 1)))
		run("lioS", 16, 0, 0, Sky)
		b.PopTransform()

		run("o", 110, 8, 232, Slate.WithAlpha(0.8))
		run("*", 64, 76, 230, Indigo)
		run("@", 40, 140, 230, Rose)
		run("S", 150, 196, 236, gfx.RadialGradient{Center: gfx.Pt(240, 180), Radius: gfx.Pt(60, 60), Stops: Stops(Sky, Indigo)})
	}},
}
