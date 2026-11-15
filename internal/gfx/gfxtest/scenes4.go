package gfxtest

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// AllBlendModes lists every mode in declaration order.
var AllBlendModes = func() []gfx.BlendMode {
	var out []gfx.BlendMode
	for m := gfx.SrcOver; m <= gfx.PlusLighter; m++ {
		out = append(out, m)
	}
	return out
}()

// BlendCell is a 100x100 cell of the blend scene.
type BlendCell struct {
	Mode gfx.BlendMode
	X, Y float32 // cell origin
	Clip bool    // under a rounded clip, over the page (not isolated)
}

// BlendCells returns the cells of the blend scene: every mode, then the clip cell.
func BlendCells() []BlendCell {
	var cells []BlendCell
	for i, m := range AllBlendModes {
		cells = append(cells, BlendCell{Mode: m, X: float32(100 * (i % 6)), Y: float32(100 * (i / 6))})
	}
	return append(cells, BlendCell{Mode: gfx.Difference, X: 500, Y: 200, Clip: true})
}

// DrawBlendCell draws a varied backdrop (an opaque gradient, a translucent
// one and an out-of-gamut solid stripe) in an isolated group, and over it,
// with the cell's blend mode at opacity 0.9, a source whose gradient goes
// from an out-of-gamut color to a translucent one. The clip cell draws the
// backdrop on the page and the source under a rounded clip.
func DrawBlendCell(b *gfx.Builder, c BlendCell) {
	gx, gy := c.X+4, c.Y+4
	if !c.Clip {
		b.PushLayer(1, gfx.SrcOver) // isolation: isolate
	}
	b.FillRect(gfx.RectXYWH(gx, gy, 92, 60), gfx.LinearGradient{Start: gfx.Pt(gx, gy), End: gfx.Pt(gx+92, gy),
		Stops: Stops(Red, Lime, Sky), Interp: gfx.InterpSRGB})
	b.FillRect(gfx.RectXYWH(gx, gy+60, 92, 32), gfx.LinearGradient{Start: gfx.Pt(gx, gy), End: gfx.Pt(gx+92, gy),
		Stops: Stops(Indigo.WithAlpha(0.6), Amber.WithAlpha(0.3)), Interp: gfx.InterpSRGB})
	b.FillRect(gfx.RectXYWH(gx+60, gy, 16, 92), OogGreen)
	src := gfx.RectXYWH(gx+14, gy+14, 64, 64)
	if c.Clip {
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(gx+10, gy+10, 72, 72), 20))
		src = gfx.RectXYWH(gx+4, gy+4, 84, 84)
	}
	b.PushLayer(0.9, c.Mode)
	b.FillRect(src, gfx.LinearGradient{Start: gfx.Pt(src.X0, src.Y0), End: gfx.Pt(src.X0, src.Y1),
		Stops: Stops(OogYellow, Fuchsia, Sky.WithAlpha(0.4))})
	b.PopLayer()
	if c.Clip {
		b.PopClip()
	} else {
		b.PopLayer()
	}
}

// MaskCell is a 160x130 cell of the masks scene. Its element is the box
// (10, 10, 140, 110) of the cell.
type MaskCell struct {
	Name string
	X, Y float32
}

// MaskCells are the cells of the masks scene.
var MaskCells = []MaskCell{
	{"alpha linear", 0, 0}, {"luminance", 160, 0}, {"radial", 320, 0},
	{"nested", 0, 130}, {"clip+rotate", 160, 130}, {"opacity+multiply", 320, 130},
}

// MaskContent paints the content of a masked element at e: a gradient and
// a translucent child overlapping it, so that masking the group differs
// from masking each command.
func MaskContent(b *gfx.Builder, e gfx.Rect) {
	b.FillRect(e, gfx.LinearGradient{Start: gfx.Pt(e.X0, e.Y0), End: gfx.Pt(e.X1, e.Y0), Stops: Stops(Indigo, Rose), Interp: gfx.InterpSRGB})
	b.FillRect(gfx.RectXYWH(e.X0+30, e.Y0+30, 80, 50), Sky.WithAlpha(0.7))
}

// MaskList records a mask: draw paints it.
func MaskList(draw func(b *gfx.Builder)) *gfx.DisplayList { return Build(draw) }

// DrawMaskCell draws one cell of the masks scene.
func DrawMaskCell(b *gfx.Builder, c MaskCell) {
	e := gfx.RectXYWH(c.X+10, c.Y+10, 140, 110)
	// The page under every cell: a backdrop for the masked group.
	b.FillRect(gfx.RectXYWH(c.X+4, c.Y+4, 152, 122), gfx.LinearGradient{Start: gfx.Pt(c.X, c.Y+4), End: gfx.Pt(c.X, c.Y+126),
		Stops: Stops(Amber.WithAlpha(0.35), Emerald.WithAlpha(0.35)), Interp: gfx.InterpSRGB})
	toBottom := func(e gfx.Rect, s ...gfx.Color) gfx.Paint {
		return gfx.LinearGradient{Start: gfx.Pt(e.X0, e.Y0), End: gfx.Pt(e.X0, e.Y1), Stops: Stops(s...), Interp: gfx.InterpSRGB}
	}
	toRight := func(e gfx.Rect, s ...gfx.Color) gfx.Paint {
		return gfx.LinearGradient{Start: gfx.Pt(e.X0, e.Y0), End: gfx.Pt(e.X1, e.Y0), Stops: Stops(s...), Interp: gfx.InterpSRGB}
	}
	radial := func(e gfx.Rect) gfx.Paint {
		return gfx.RadialGradient{Center: e.Center(), Radius: gfx.Pt(e.Width()/2, e.Height()/2),
			Stops: []gfx.GradientStop{Stop(0.6, gfx.Black), Stop(1, gfx.Transparent)}, Interp: gfx.InterpSRGB}
	}
	switch c.Name {
	case "alpha linear":
		m := MaskList(func(mb *gfx.Builder) { mb.FillRect(e, toBottom(e, gfx.Black, gfx.Transparent)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 1, gfx.SrcOver)
		MaskContent(b, e)
		b.PopLayer()
	case "luminance":
		m := MaskList(func(mb *gfx.Builder) {
			mb.FillRect(e, toRight(e, gfx.White, gfx.RGB8(255, 0, 0), gfx.RGBA8(0, 255, 0, 128), gfx.Black))
		})
		b.PushMaskLayer(gfx.Mask{List: m, Mode: gfx.MaskLuminance}, 1, gfx.SrcOver)
		MaskContent(b, e)
		b.PopLayer()
	case "radial":
		m := MaskList(func(mb *gfx.Builder) { mb.FillRect(e, radial(e)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 1, gfx.SrcOver)
		MaskContent(b, e)
		b.PopLayer()
	case "nested":
		outer := MaskList(func(mb *gfx.Builder) { mb.FillRect(e, toRight(e, gfx.Black, gfx.Transparent)) })
		in := gfx.RectXYWH(e.X0+20, e.Y0+15, 100, 80)
		inner := MaskList(func(mb *gfx.Builder) { mb.FillRect(in, radial(in)) })
		b.PushMaskLayer(gfx.Mask{List: outer}, 1, gfx.SrcOver)
		b.FillRect(e, toBottom(e, Emerald, Slate))
		b.PushMaskLayer(gfx.Mask{List: inner}, 1, gfx.SrcOver)
		b.FillRect(in, Rose)
		b.FillRect(gfx.RectXYWH(in.X0+40, in.Y0, 20, 80), Yellow)
		b.PopLayer()
		b.PopLayer()
	case "clip+rotate":
		b.PushClipRRect(gfx.NewRRect(e, 24))
		b.PushTransform(gfx.RotateAbout(float32(12*math.Pi/180), e.Center()))
		m := MaskList(func(mb *gfx.Builder) { mb.FillRect(e, toBottom(e, gfx.Black, gfx.Transparent)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 1, gfx.SrcOver)
		MaskContent(b, e)
		b.PopLayer()
		b.PopTransform()
		b.PopClip()
	case "opacity+multiply":
		b.FillRect(gfx.RectXYWH(e.X0, e.Y0+40, 140, 30), Sky)
		m := MaskList(func(mb *gfx.Builder) { mb.FillRect(e, radial(e)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 0.7, gfx.Multiply)
		MaskContent(b, e)
		b.PopLayer()
	}
}

// FocalCell is a 150x150 cell of the focal scene, filled with a two-point
// gradient (fx, fy, fr) -> (cx, cy, r), relative to the cell's center,
// under an optional vertical scale about the center.
type FocalCell struct {
	Name       string
	FX, FY, FR float32
	CX, CY, R  float32
	Stops      []gfx.Color
	ScaleY     float32 // 0 means 1
	X, Y       float32 // cell origin
	Interp     gfx.ColorSpace
}

// FocalCells returns the cells of the focal scene.
func FocalCells() []FocalCell {
	cells := []FocalCell{
		{Name: "focal", FX: -30, FY: -25, CX: 0, CY: 0, R: 60, Stops: []gfx.Color{Amber, Rose, Indigo}},
		{Name: "focal radius", FX: 20, FY: 10, FR: 15, R: 65, Stops: []gfx.Color{Sky, Emerald, Slate}},
		{Name: "focal on edge", FX: 60, R: 60, Stops: []gfx.Color{Lime, Sky, Indigo}},
		{Name: "cone", FX: -45, FR: 10, CX: 30, R: 30, Stops: []gfx.Color{Rose, Amber, Lime}},
		{Name: "concentric", FR: 20, R: 60, Stops: []gfx.Color{Indigo, Amber, Rose}},
		{Name: "strip", FX: -30, FR: 25, CX: 30, R: 25, Stops: []gfx.Color{Emerald, Fuchsia}},
		{Name: "ellipse", FX: -30, FY: -25, R: 60, Stops: []gfx.Color{Amber, Rose, Indigo}, ScaleY: 0.6},
		{Name: "shrinking", FR: 60, CX: 20, CY: -10, R: 5, Stops: []gfx.Color{Slate, Sky, Yellow}},
	}
	for i := range cells {
		cells[i].X, cells[i].Y = float32(150*(i%4)), float32(150*(i/4))
		cells[i].Interp = gfx.InterpSRGB
	}
	return cells
}

// Paint returns the cell's two-point gradient.
func (c FocalCell) Paint() gfx.FocalGradient {
	ox, oy := c.X+75, c.Y+75
	return gfx.FocalGradient{Focal: gfx.Pt(ox+c.FX, oy+c.FY), FocalRadius: c.FR, Center: gfx.Pt(ox+c.CX, oy+c.CY), Radius: c.R,
		Stops: Stops(c.Stops...), Interp: c.Interp}
}

// DrawFocalCell draws one cell of the focal scene.
func DrawFocalCell(b *gfx.Builder, c FocalCell) {
	cell := gfx.RectXYWH(c.X, c.Y, 150, 150)
	if c.ScaleY != 0 {
		b.PushTransform(gfx.ScaleAbout(1, c.ScaleY, gfx.Pt(c.X+75, c.Y+75)))
		cell = gfx.RectXYWH(c.X, c.Y+75-75/c.ScaleY, 150, 150/c.ScaleY)
		b.PushClipRect(cell)
		b.FillRect(cell, c.Paint())
		b.PopClip()
		b.PopTransform()
		return
	}
	b.FillRect(cell, c.Paint())
}

// Scenes4 are the blend, mask and focal scenes.
var Scenes4 = []Scene{
	{"blend", 600, 300, func(b *gfx.Builder) {
		for _, c := range BlendCells() {
			DrawBlendCell(b, c)
		}
	}},
	{"masks", 480, 260, func(b *gfx.Builder) {
		for _, c := range MaskCells {
			DrawMaskCell(b, c)
		}
	}},
	{"focal", 600, 300, func(b *gfx.Builder) {
		for _, c := range FocalCells() {
			DrawFocalCell(b, c)
		}
	}},
	{"blend_more", 240, 200, DrawBlendMore},
	{"masks_more", 240, 200, DrawMasksMore},
	{"focal_more", 240, 120, DrawFocalMore},
}

// DrawBlendMore covers blend layers that CSS pages do not make: blending
// onto a transparent backdrop (the source comes out unchanged), a blend
// layer under a rotation and an anti-aliased path clip, and a blend layer
// with a blur filter (filters run before blending).
func DrawBlendMore(b *gfx.Builder) {
	b.PushLayer(1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(10, 10, 60, 60), gfx.RadialGradient{Center: gfx.Pt(40, 40), Radius: gfx.Pt(30, 30),
		Stops: Stops(Yellow, Rose.WithAlpha(0.5), gfx.Transparent)})
	b.PushLayer(1, gfx.ColorDodge)
	b.FillRect(gfx.RectXYWH(25, 0, 30, 80), Sky.WithAlpha(0.8))
	b.PopLayer()
	b.PopLayer()

	b.FillRect(gfx.RectXYWH(90, 10, 140, 70), gfx.ConicGradient{Center: gfx.Pt(160, 45), Stops: Stops(Red, Yellow, Lime, Sky, Indigo, Red)})
	b.PushClipPath(StarPath(160, 48, 44), gfx.NonZero)
	b.PushTransform(gfx.RotateAbout(0.4, gfx.Pt(160, 45)))
	for i, m := range []gfx.BlendMode{gfx.Hue, gfx.Saturation, gfx.ColorBlend, gfx.Luminosity} {
		b.PushLayer(0.8, m)
		b.FillRect(gfx.RectXYWH(110+float32(i)*25, 15, 22, 60), []gfx.Color{Slate, Emerald, Fuchsia, Amber}[i])
		b.PopLayer()
	}
	b.PopTransform()
	b.PopClip()

	Checker(b, gfx.RectXYWH(10, 100, 220, 90), 10)
	for i, m := range []gfx.BlendMode{gfx.SoftLight, gfx.Overlay, gfx.PlusLighter, gfx.Exclusion} {
		x := 20 + float32(i)*52
		b.PushLayer(1, m, gfx.Blur{StdDev: 3})
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, 112, 40, 66), 10), gfx.LinearGradient{Start: gfx.Pt(x, 112), End: gfx.Pt(x, 178),
			Stops: Stops(gfx.White, gfx.Color{R: 0.5, G: 0.5, B: 0.5, A: 1}, gfx.Black)})
		b.PopLayer()
	}
}

// drawMasksMore covers glyph masks (background-clip: text with the
// synthetic font, color glyphs included), a luminance mask under a
// rotation whose list has its own layers and transforms, a mask with a

// DrawFocalMore covers what the Chromium cells do not: Oklab
// interpolation, translucent stops, a rotation and hard stops.
func DrawFocalMore(b *gfx.Builder) {
	b.FillRect(gfx.RectXYWH(0, 0, 120, 120), gfx.FocalGradient{Focal: gfx.Pt(40, 45), Center: gfx.Pt(60, 60), Radius: 55,
		Stops: Stops(OogYellow, Fuchsia.WithAlpha(0.5), Sky)})
	b.PushTransform(gfx.RotateAbout(0.6, gfx.Pt(180, 60)))
	b.FillRect(gfx.RectXYWH(120, 0, 120, 120), gfx.FocalGradient{Focal: gfx.Pt(150, 60), FocalRadius: 8, Center: gfx.Pt(195, 60), Radius: 20,
		Stops: []gfx.GradientStop{Stop(0, Rose), Stop(0.5, Rose), Stop(0.5, Amber), Stop(1, Sky.WithAlpha(0))}, Interp: gfx.InterpSRGB})
	b.PopTransform()
}

// DrawMasksMore covers glyph masks (background-clip: text with the
// synthetic font, color glyphs included), a luminance mask under a
// rotation whose list has its own layers and transforms, a mask with a
// blurred (filter) list, and a masked filter layer.
func DrawMasksMore(b *gfx.Builder) {
	text := MaskList(func(mb *gfx.Builder) {
		mb.DrawGlyphRun(FontA.Run("AoSil*", 36, 12, 50, gfx.Black))
	})
	b.PushMaskLayer(gfx.Mask{List: text}, 1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(10, 10, 220, 50), gfx.LinearGradient{Start: gfx.Pt(10, 10), End: gfx.Pt(230, 60), Stops: Stops(Indigo, Fuchsia, Amber)})
	b.PopLayer()

	lum := MaskList(func(mb *gfx.Builder) {
		mb.FillRect(gfx.RectXYWH(10, 70, 100, 120), gfx.Color{R: 0.3, G: 0.3, B: 0.3, A: 1})
		mb.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(60, 130)))
		mb.PushLayer(0.8, gfx.SrcOver)
		mb.FillRRect(gfx.NewRRect(gfx.RectXYWH(30, 100, 60, 60), 14), gfx.White)
		mb.PopLayer()
		mb.PopTransform()
	})
	b.PushTransform(gfx.RotateAbout(-0.2, gfx.Pt(60, 130)))
	b.PushMaskLayer(gfx.Mask{List: lum, Mode: gfx.MaskLuminance}, 1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(0, 60, 120, 140), gfx.RadialGradient{Center: gfx.Pt(60, 130), Radius: gfx.Pt(60, 60), Stops: Stops(Rose, Indigo)})
	b.PopLayer()
	b.PopTransform()

	blurred := MaskList(func(mb *gfx.Builder) {
		mb.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 6})
		mb.FillPath(StarPath(175, 130, 50), gfx.NonZero, gfx.Black)
		mb.PopLayer()
	})
	b.PushMaskLayer(gfx.Mask{List: blurred}, 1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(4, 4), StdDev: 2, Color: Slate})
	b.FillRect(gfx.RectXYWH(130, 80, 90, 100), gfx.LinearGradient{Start: gfx.Pt(130, 80), End: gfx.Pt(220, 180), Stops: Stops(Emerald, Sky)})
	b.FillRect(gfx.RectXYWH(150, 100, 30, 30), gfx.White)
	b.PopLayer()
}
