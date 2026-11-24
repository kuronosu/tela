package gfxtest

import (
	"image/color"

	"github.com/kuronosu/tela/internal/gfx"
)

// CompositeModes lists the operations of Canvas's globalCompositeOperation
// that are not blend modes of CSS: the Porter-Duff operators, and
// source-over and lighter, whose modes CSS shares.
var CompositeModes = func() []gfx.BlendMode {
	out := []gfx.BlendMode{gfx.SrcOver}
	for m := gfx.PlusLighter; m <= gfx.Xor; m++ {
		out = append(out, m)
	}
	return out
}()

// CompositeCell is a 100x100 cell of the composite scene: a canvas of its
// own, isolated and clipped.
type CompositeCell struct {
	Mode gfx.BlendMode
	X, Y float32 // cell origin
	Clip bool    // under a rounded clip, at opacity 0.7
}

// Name returns the cell's name.
func (c CompositeCell) Name() string {
	if c.Clip {
		return "clip " + c.Mode.String()
	}
	return c.Mode.String()
}

// CompositeCells returns the cells of the composite scene: every operation
// alone, in the first two rows, and under a clip, in the two below.
func CompositeCells() []CompositeCell {
	var cells []CompositeCell
	for k, clip := range []bool{false, true} {
		for i, m := range CompositeModes {
			cells = append(cells, CompositeCell{Mode: m, X: float32(100 * (i % 6)), Y: float32(100 * (i/6 + 2*k)), Clip: clip})
		}
	}
	return cells
}

// CompositeClip is the rounded clip of the cells that have one.
func CompositeClip(c CompositeCell) gfx.RRect {
	return gfx.NewRRect(gfx.RectXYWH(c.X+8, c.Y+12, 80, 76), 26)
}

// CompositeAngle is the rotation of the source of the cells about their
// center.
const CompositeAngle = 0.3

// DrawCompositeBackdrop draws a cell's backdrop: an opaque gradient and a
// translucent disc, with transparent pixels around them.
func DrawCompositeBackdrop(b *gfx.Builder, c CompositeCell) {
	r := gfx.RectXYWH(c.X+10, c.Y+10, 55, 55)
	b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X1, r.Y0), Stops: Stops(Red, Sky), Interp: gfx.InterpSRGB})
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(c.X+14, c.Y+48, 44, 44), 22), Indigo.WithAlpha(0.5))
}

// DrawCompositeSource draws a cell's source: a rotated rectangle whose
// gradient goes from opaque to translucent (of one color: Canvas's gradients
// interpolate colors that are not premultiplied).
func DrawCompositeSource(b *gfx.Builder, c CompositeCell) {
	b.PushTransform(gfx.RotateAbout(CompositeAngle, gfx.Pt(c.X+50, c.Y+50)))
	r := gfx.RectXYWH(c.X+30, c.Y+30, 56, 46)
	b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X0, r.Y1), Stops: Stops(Amber, Amber.WithAlpha(0.25)), Interp: gfx.InterpSRGB})
	b.PopTransform()
}

// DrawCompositeCell draws one cell of the composite scene.
func DrawCompositeCell(b *gfx.Builder, c CompositeCell) {
	b.PushClipRect(gfx.RectXYWH(c.X, c.Y, 100, 100))
	b.PushLayer(1, gfx.SrcOver)
	DrawCompositeBackdrop(b, c)
	op := float32(1)
	if c.Clip {
		b.PushClipRRect(CompositeClip(c))
		op = 0.7
	}
	b.PushLayer(op, c.Mode)
	DrawCompositeSource(b, c)
	b.PopLayer()
	if c.Clip {
		b.PopClip()
	}
	b.PopLayer()
	b.PopClip()
}

// DrawCompositeMore covers what the cells do not: operators nested in
// groups, under masks, filters and path clips, without content or at
// opacity 0, and at the root of the list, over the frame itself.
func DrawCompositeMore(b *gfx.Builder) {
	Checker(b, gfx.RectXYWH(0, 0, 240, 200), 10)
	group := func(x, y float32, draw func(x, y float32)) {
		b.PushClipRect(gfx.RectXYWH(x, y, 72, 92))
		b.PushLayer(1, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(x+6, y+6, 44, 60), Rose)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+26, y+30, 40, 56), 12), Sky.WithAlpha(0.6))
		draw(x, y)
		b.PopLayer()
		b.PopClip()
	}
	// A group inside the group: the operators touch the inner one only.
	group(4, 4, func(x, y float32) {
		b.PushLayer(0.9, gfx.SrcAtop)
		b.FillRect(gfx.RectXYWH(x+16, y+16, 50, 60), Amber)
		b.PushLayer(1, gfx.DstIn)
		b.FillPath(StarPath(x+40, y+46, 30), gfx.NonZero, gfx.Black)
		b.PopLayer()
		b.PopLayer()
	})
	// A mask and a blur: the operator clears what both leave transparent.
	group(84, 4, func(x, y float32) {
		mask := MaskList(func(mb *gfx.Builder) {
			mb.FillRect(gfx.RectXYWH(x, y, 72, 92), gfx.LinearGradient{Start: gfx.Pt(x, y), End: gfx.Pt(x, y+92), Stops: Stops(gfx.Black, gfx.Transparent), Interp: gfx.InterpSRGB})
		})
		b.PushClipPath(StarPath(x+36, y+46, 44), gfx.NonZero)
		b.PushMaskLayer(gfx.Mask{List: mask}, 1, gfx.DstAtop, gfx.Blur{StdDev: 3})
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+16, y+16, 40, 60), 10), Emerald)
		b.PopLayer()
		b.PopClip()
	})
	// No content: source-in clears the clip.
	group(164, 4, func(x, y float32) {
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(x+14.5, y+20.25, 40, 40), 20))
		b.PushLayer(1, gfx.SrcIn)
		b.PopLayer()
		b.PopClip()
	})
	// Opacity 0: copy clears too, and source-atop does nothing.
	group(4, 104, func(x, y float32) {
		b.PushClipRect(gfx.RectXYWH(x+20.5, y, 20, 92))
		b.PushLayer(0, gfx.Copy)
		b.FillRect(gfx.RectXYWH(x, y, 72, 92), Amber)
		b.PopLayer()
		b.PopClip()
		b.PushLayer(0, gfx.SrcAtop)
		b.FillRect(gfx.RectXYWH(x, y, 72, 92), Amber)
		b.PopLayer()
	})
	// Under a rotation and a rectangle clip: the clip's edges are lerped.
	group(84, 104, func(x, y float32) {
		b.PushTransform(gfx.RotateAbout(0.35, gfx.Pt(x+36, y+46)))
		b.PushClipRect(gfx.RectXYWH(x+12, y+16, 48, 60))
		b.PushLayer(0.8, gfx.Copy)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+20, y+26, 32, 40), 8), Indigo)
		b.PopLayer()
		b.PopClip()
		b.PopTransform()
	})
	// At the root, under a clip: the operator works on the frame.
	b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(168, 108, 64, 84), 18))
	b.PushLayer(1, gfx.Xor)
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(176, 116, 48, 30), 8), Fuchsia.WithAlpha(0.7))
	b.PopLayer()
	b.PushLayer(1, gfx.DstOver)
	b.FillRect(gfx.RectXYWH(160, 100, 80, 70), Yellow)
	b.PopLayer()
	b.PushLayer(0.9, gfx.DstOut)
	b.FillRect(gfx.RectXYWH(176, 150, 48, 50), gfx.Black)
	b.PopLayer()
	b.PopClip()
}

// PatternTilePixels holds the non-premultiplied pixels of PatternTile: 12x8
// colors, some of them translucent and one transparent.
var PatternTilePixels = func() []color.NRGBA {
	const w, h = 12, 8
	cols := []gfx.Color{Rose, Rose, Amber, Amber, Sky, Sky, Indigo, Indigo, Emerald, Emerald, Slate, Slate}
	out := make([]color.NRGBA, 0, w*h)
	for y := range h {
		for x := range w {
			p := cols[(x+3*(y/2))%len(cols)].NRGBA8()
			switch {
			case x == 5 && y == 3:
				p.A = 0
			case x >= 4 && x < 8 && y >= 2 && y < 6:
				p.A = 128
			}
			out = append(out, p)
		}
	}
	return out
}()

// PatternBigPixels holds the pixels of PatternBig: 32x32 colors with fine
// detail, which alias when they are not filtered.
var PatternBigPixels = func() []color.NRGBA {
	const n = 32
	out := make([]color.NRGBA, 0, n*n)
	for y := range n {
		for x := range n {
			p := color.NRGBA{R: uint8(8 * x), G: uint8(8 * y), B: 160, A: 255}
			if (x/2+y/2)%2 == 0 {
				p = color.NRGBA{R: 255, G: uint8(255 - 6*x), B: uint8(4 * y), A: 255}
			}
			out = append(out, p)
		}
	}
	return out
}()

func imageOf(w int, pix []color.NRGBA) *gfx.Image {
	out := make([]byte, 0, 4*len(pix))
	for _, p := range pix {
		c := gfx.RGBA8(p.R, p.G, p.B, p.A).Premul8()
		out = append(out, c.R, c.G, c.B, c.A)
	}
	return gfx.NewImagePremul(w, len(pix)/w, out)
}

// PatternTile is a 12x8 image and PatternBig a 32x32 one.
var (
	PatternTile = imageOf(12, PatternTilePixels)
	PatternBig  = imageOf(32, PatternBigPixels)
)

// PatternCell is a 100x100 cell of the pattern scene: a shape filled or
// stroked with a pattern under a transform about the cell's origin.
type PatternCell struct {
	Name      string
	X, Y      float32 // cell origin
	Big       bool    // PatternBig; else PatternTile
	Repeat    gfx.PatternRepeat
	Nearest   bool
	Transform gfx.Affine // of the canvas, after the translation to (X, Y)
	Pattern   gfx.Affine // of the pattern
	// Shape is "rect" (the cell), "disc" or "ring" (a stroked circle).
	Shape string
}

// PatternCells returns the cells of the pattern scene. The last ones, with
// PatternBig, are minified.
func PatternCells() []PatternCell {
	id := gfx.Identity
	// The sine and cosine of 0.4 as constants, the same everywhere.
	rot := func(s float32) gfx.Affine {
		const cos, sin = 0.921061, 0.389418
		return gfx.Affine{s * cos, s * sin, -s * sin, s * cos, 34, 6}
	}
	cells := []PatternCell{
		{Name: "repeat", Transform: gfx.Translate(5, 7), Pattern: id},
		{Name: "repeat-x", Repeat: gfx.RepeatX, Transform: gfx.Affine{2.3, 0, 0, 2.3, 13.3, 31.2}, Pattern: id},
		{Name: "repeat-y", Repeat: gfx.RepeatY, Transform: gfx.Affine{2.3, 0, 0, 2.3, 33.3, 11.2}, Pattern: id},
		{Name: "no-repeat", Repeat: gfx.RepeatNone, Transform: gfx.Affine{5, 0, 0, 5, 20, 25}, Pattern: id},
		{Name: "scaled", Transform: gfx.Affine{3, 0, 0, 3, 4, 2}, Pattern: id},
		{Name: "nearest", Nearest: true, Transform: gfx.Affine{3, 0, 0, 3, 4, 2}, Pattern: id},
		{Name: "rotated", Transform: rot(1.7), Pattern: id},
		{Name: "rotated no-repeat", Repeat: gfx.RepeatNone, Transform: rot(4), Pattern: id},
		{Name: "pattern transform", Transform: gfx.Translate(2, 3), Pattern: gfx.Affine{1.5, 0.3, -0.4, 1.2, 4, 6}, Shape: "disc"},
		{Name: "stroke", Transform: id, Pattern: gfx.Scale(1.5, 1.5), Shape: "ring"},
		{Name: "minified", Big: true, Transform: id, Pattern: gfx.Scale(0.25, 0.25)},
		{Name: "minified rotated", Big: true, Transform: rot(0.23), Pattern: id},
	}
	for i := range cells {
		cells[i].X, cells[i].Y = float32(100*(i%6)), float32(100*(i/6))
		if cells[i].Shape == "" {
			cells[i].Shape = "rect"
		}
	}
	return cells
}

// Paint returns the cell's pattern.
func (c PatternCell) Paint() gfx.ImagePattern {
	p := gfx.ImagePattern{Image: PatternTile, Repeat: c.Repeat, Transform: c.Pattern}
	if c.Big {
		p.Image = PatternBig
	}
	if c.Nearest {
		p.Sampling = gfx.SampleNearest
	}
	return p
}

// DrawPatternCell draws one cell of the pattern scene.
func DrawPatternCell(b *gfx.Builder, c PatternCell) {
	cell := gfx.RectXYWH(c.X+4, c.Y+4, 92, 92)
	b.PushClipRect(cell)
	b.PushTransform(gfx.Translate(c.X, c.Y))
	switch c.Shape {
	case "rect":
		// The whole cell, in the space of the pattern.
		b.PushTransform(c.Transform)
		b.FillRect(gfx.Rect{X0: -2000, Y0: -2000, X1: 2000, Y1: 2000}, c.Paint())
		b.PopTransform()
	case "disc":
		b.PushTransform(c.Transform)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(8, 7, 80, 80), 40), c.Paint())
		b.PopTransform()
	case "ring":
		st := gfx.NewStrokeStyle(22)
		b.StrokePath(CirclePath(50, 50, 32), st, c.Paint())
	}
	b.PopTransform()
	b.PopClip()
}

// CirclePath returns a circle of four cubics.
func CirclePath(cx, cy, r float32) gfx.Path {
	const k = 0.5522848
	q := float32(r * k)
	var p gfx.Path
	p.MoveTo(gfx.Pt(cx+r, cy))
	p.CubicTo(gfx.Pt(cx+r, cy+q), gfx.Pt(cx+q, cy+r), gfx.Pt(cx, cy+r))
	p.CubicTo(gfx.Pt(cx-q, cy+r), gfx.Pt(cx-r, cy+q), gfx.Pt(cx-r, cy))
	p.CubicTo(gfx.Pt(cx-r, cy-q), gfx.Pt(cx-q, cy-r), gfx.Pt(cx, cy-r))
	p.CubicTo(gfx.Pt(cx+q, cy-r), gfx.Pt(cx+r, cy-q), gfx.Pt(cx+r, cy))
	p.Close()
	return p
}

// DrawPatternMore covers what the cells do not: glyph runs, clips and
// layers over a pattern, negative scales, deep minification, a tile of one
// pixel, and patterns whose mapping is a translation by whole pixels.
func DrawPatternMore(b *gfx.Builder) {
	Checker(b, gfx.RectXYWH(0, 0, 240, 160), 10)
	b.DrawGlyphRun(FontA.Run("AoSil*", 36, 6, 40, gfx.ImagePattern{Image: PatternTile, Transform: gfx.Scale(0.5, 0.5)}))
	b.PushTransform(gfx.RotateAbout(0.1, gfx.Pt(170, 30)))
	b.DrawGlyphRun(FontA.Run("AoS", 36, 120, 40, gfx.ImagePattern{Image: PatternBig, Transform: gfx.Identity}))
	b.PopTransform()
	b.FillPath(StarPath(48, 104, 44), gfx.NonZero, gfx.ImagePattern{Image: PatternTile, Transform: gfx.Affine{-2, 0, 0, 2, 48, 104}})
	b.PushLayer(0.6, gfx.SrcOver)
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(100, 60, 60, 44), 12), gfx.ImagePattern{Image: PatternBig, Transform: gfx.Scale(0.06, 0.06)})
	b.PopLayer()
	b.FillRect(gfx.RectXYWH(100, 110, 60, 44), gfx.ImagePattern{Image: imageOf(1, PatternTilePixels[:1]), Repeat: gfx.RepeatX, Transform: gfx.Affine{4, 1, 0, 24, 100, 116}})
	b.FillRect(gfx.RectXYWH(170, 60, 64, 44), gfx.ImagePattern{Image: PatternTile, Repeat: gfx.RepeatX, Transform: gfx.Translate(-3, 80)})
	b.FillRect(gfx.RectXYWH(170, 110, 64, 44), gfx.ImagePattern{Image: PatternBig, Repeat: gfx.RepeatY, Transform: gfx.Translate(190, -7)})
}

// Scenes5 are the composite and pattern scenes.
var Scenes5 = []Scene{
	{"composite", 600, 400, func(b *gfx.Builder) {
		for _, c := range CompositeCells() {
			DrawCompositeCell(b, c)
		}
	}},
	{"pattern", 600, 200, func(b *gfx.Builder) {
		for _, c := range PatternCells() {
			DrawPatternCell(b, c)
		}
	}},
	{"composite_more", 240, 200, DrawCompositeMore},
	{"pattern_more", 240, 160, DrawPatternMore},
}
