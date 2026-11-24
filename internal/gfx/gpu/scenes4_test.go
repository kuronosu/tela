package gpu

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Scenes of what Canvas adds (ADR-319): the Porter-Duff operators of layers,
// at the root of the list too, and image patterns. gfxtest's Scenes5 has the
// operators and the repeat modes one by one; these have what meets them:
// clips of every kind, masks, filters, glass panes, covers, every shape a
// pattern paints.

// operators are the Porter-Duff operators.
var operators = []gfx.BlendMode{gfx.SrcIn, gfx.SrcOut, gfx.SrcAtop, gfx.DstOver, gfx.DstIn, gfx.DstOut, gfx.DstAtop, gfx.Copy, gfx.Xor}

// clipKinds is the number of clips of pushClipKind.
const clipKinds = 6

// pushClipKind pushes a clip over the 72x72 cell at (x, y): pixel aligned (at
// whole scales), a rectangle off the grid, a rounded one, a path, a rotated
// rectangle, or two nested that make a clip mask. It returns what pops it.
func pushClipKind(b *gfx.Builder, k int, x, y float32) (pop func()) {
	switch k % clipKinds {
	case 0:
		b.PushClipRect(gfx.RectXYWH(x+6, y+6, 60, 60))
	case 1:
		b.PushClipRect(gfx.RectXYWH(x+6.3, y+5.6, 59.1, 60.5))
	case 2:
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(x+5, y+7, 62, 58), 18))
	case 3:
		b.PushClipPath(gfxtest.StarPath(x+36, y+36, 35), gfx.NonZero)
	case 4:
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(x+36, y+36)))
		b.PushClipRect(gfx.RectXYWH(x+12, y+12, 48, 48))
		return func() { b.PopClip(); b.PopTransform() }
	default:
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(x+3.5, y+4.5, 64, 62), 30))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(x+10, y+2, 50, 68), 12))
		return func() { b.PopClip(); b.PopClip() }
	}
	return b.PopClip
}

// operand paints the source of an operator in the cell at (x, y): opaque,
// translucent and, around, absent.
func operand(b *gfx.Builder, x, y float32) {
	b.FillRect(gfx.RectXYWH(x+18, y+14, 30, 40), gfx.LinearGradient{Start: gfx.Pt(x, y+14), End: gfx.Pt(x, y+54), Stops: stops(amber, fuchsia.WithAlpha(0.3))})
	b.FillPath(gfx.CirclePath(gfx.Pt(x+46, y+42), 14), gfx.NonZero, emerald.WithAlpha(0.7))
}

// backing paints what the operator of the cell at (x, y) finds: opaque,
// translucent and, around, what the frame starts from.
func backing(b *gfx.Builder, x, y float32) {
	b.FillRect(gfx.RectXYWH(x+2, y+2, 46, 40), gfx.LinearGradient{Start: gfx.Pt(x, y), End: gfx.Pt(x+48, y), Stops: stops(rose, sky)})
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+20, y+26, 48, 42), 12), indigo.WithAlpha(0.5))
}

// operatorCells draws every operator over a backing, through a clip whose
// kind changes from cell to cell.
func operatorCells(b *gfx.Builder, kind int) {
	for i, m := range operators {
		x, y := float32(72*(i%3)), float32(72*(i/3))
		backing(b, x, y)
		pop := pushClipKind(b, kind+i, x, y)
		b.PushLayer(1-0.4*float32(i%2), m)
		operand(b, x, y)
		b.PopLayer()
		pop()
	}
}

// operatorPanes is a page of glass panes over a background that operators
// have cleared in places, with operators on the panes; moved moves something
// under the first of them, and label something over it.
func operatorPanes(b *gfx.Builder, moved, label float32) {
	b.FillRect(gfx.RectXYWH(0, 0, 400, 300), slate)
	b.FillRect(gfx.RectXYWH(0, 60, 400, 30), rose)
	b.FillRect(gfx.RectXYWH(40+moved, 0, 24, 300), amber)
	// A hole under the first pane, a strip that only fades under the second.
	b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(60, 100, 120, 100), 30))
	b.PushLayer(0, gfx.Copy)
	b.PopLayer()
	b.PopClip()
	b.PushLayer(0.5, gfx.DstOut)
	b.FillRect(gfx.RectXYWH(200, 180, 200, 40), gfx.Black)
	b.PopLayer()
	for i, x := range []float32{20, 210} {
		box := gfx.NewRRect(gfx.RectXYWH(x, 20, 170, 260), 16)
		b.BackdropFilter(box, gfx.Blur{StdDev: 8})
		b.FillRRect(box, gfx.White.WithAlpha(0.25))
		// On the pane: a window through it, and something under all of it.
		b.PushClipRect(gfx.RectXYWH(x+20, 40, 60, 40))
		b.PushLayer(0.8, operators[i*7]) // source-in, copy
		b.FillPath(gfx.CirclePath(gfx.Pt(x+50, 60), 18), gfx.NonZero, emerald)
		b.PopLayer()
		b.PopClip()
		b.PushLayer(1, gfx.DstOver)
		b.FillRect(gfx.RectXYWH(x+10, 150, 100, 60), sky)
		b.PopLayer()
	}
	b.FillRect(gfx.RectXYWH(80+label, 120, 40, 12), emerald)
}

func operatorPage(moved, label float32) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) { operatorPanes(b, moved, label) })
}

// shapesOf paints every shape a pattern can, in a box of 190x110.
func shapesOf(b *gfx.Builder, p gfx.Paint) {
	b.FillRect(gfx.RectXYWH(4.3, 4.6, 50, 44), p)
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(60, 4, 54, 44), 14), p)
	b.FillPath(gfxtest.StarPath(146, 28, 26), gfx.EvenOdd, p)
	b.StrokePath(zigzag(6, 60, 12, 24, 6), gfx.NewStrokeStyle(7), p)
	text(b, "SoA*", 30, 84, 84, p)
	b.PushTransform(gfx.RotateAbout(0.35, gfx.Pt(170, 80)))
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(152, 66, 34, 28), 8), p)
	text(b, "So", 26, 120, 106, p)
	b.PopTransform()
}

// tilePattern returns a pattern of the small tile of gfxtest, which has
// translucent pixels.
func tilePattern(rep gfx.PatternRepeat, s gfx.Sampling, m gfx.Affine) gfx.ImagePattern {
	return gfx.ImagePattern{Image: gfxtest.PatternTile, Repeat: rep, Sampling: s, Transform: m}
}

// photoPattern returns a pattern of photo, which is opaque.
func photoPattern(m gfx.Affine) gfx.ImagePattern {
	return gfx.ImagePattern{Image: photo, Transform: m}
}

var canvasScenes = []testScene{
	// The operators at the root of the list, over the frame: through every
	// kind of clip, on an opaque frame and on a transparent one.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_root", W: 216, H: 216, Draw: func(b *gfx.Builder) { operatorCells(b, 0) }}},
	{clear: gfx.Transparent, Scene: gfxtest.Scene{Name: "gpu_operators_transparent", W: 216, H: 216, Draw: func(b *gfx.Builder) { operatorCells(b, 1) }}},
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_clips", W: 648, H: 432, Draw: func(b *gfx.Builder) {
		for kind := range clipKinds {
			b.PushTransform(gfx.Translate(216*float32(kind%3), 216*float32(kind/3)))
			operatorCells(b, kind)
			b.PopTransform()
		}
	}}},
	// Without a clip: an operator that clears what its source leaves
	// transparent changes the whole frame, and the layer that holds one the
	// whole of its own bounds.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_unclipped", W: 160, H: 120, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 160, 120))
		b.PushLayer(0.8, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(10, 10, 70, 80), indigo)
		b.PushLayer(1, gfx.DstIn)
		b.FillPath(gfx.CirclePath(gfx.Pt(50, 50), 30), gfx.NonZero, gfx.Black)
		b.PopLayer()
		b.PopLayer()
		b.PushLayer(0.9, gfx.DstAtop)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(20.5, 14.5, 110, 90), 30), amber.WithAlpha(0.8))
		b.PopLayer()
	}}},
	// Operators that show nothing of their layer: empty, at opacity 0, under
	// an empty mask. Those that are unbounded clear the clip. On the frame,
	// in a layer, one over another, and under what covers them.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_empty", W: 432, H: 216, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 432, 216))
		for i, m := range []gfx.BlendMode{gfx.SrcIn, gfx.SrcOut, gfx.DstIn, gfx.DstAtop, gfx.Copy, gfx.SrcIn} {
			x := float32(72 * i)
			pop := pushClipKind(b, i, x, 0)
			b.PushLayer(1, m)
			b.PopLayer()
			pop()
			// In a group, at opacity 0.
			b.PushLayer(0.7, gfx.SrcOver)
			backing(b, x, 72)
			pop = pushClipKind(b, i, x, 72)
			b.PushLayer(0, m, gfx.Blur{StdDev: 3})
			operand(b, x, 72)
			b.PopLayer()
			pop()
			b.FillRect(gfx.RectXYWH(x+30, 100, 12, 12), emerald)
			b.PopLayer()
			// Under an empty mask; then something over it, which an opaque
			// rectangle covers in part.
			pop = pushClipKind(b, i+1, x, 144)
			b.PushMaskLayer(gfx.Mask{List: gfxtest.Build(func(*gfx.Builder) {})}, 1, m)
			operand(b, x, 144)
			b.PopLayer()
			b.PushLayer(1, operators[(i+2)%len(operators)])
			operand(b, x, 144)
			b.PopLayer()
			pop()
		}
		b.FillRect(gfx.RectXYWH(0, 190, 432, 70), slate)
	}}},
	// Operators with filters and masks, nested and in groups with an
	// opacity.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_filters", W: 432, H: 216, Draw: func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 0, 432, 216), 9)
		filters := [][]gfx.Filter{
			{gfx.Blur{StdDev: 3}}, {gfx.DropShadow{Offset: gfx.Pt(5, 4), StdDev: 2, Color: gfx.Black.WithAlpha(0.6)}},
			{gfx.Invert(1)}, {gfx.Blur{StdDev: 9}, gfx.Saturate(1.6)}, nil, nil,
		}
		for i := range 18 {
			x, y := float32(72*(i%6)), float32(72*(i/6))
			m := operators[i%len(operators)]
			b.PushLayer(0.9, gfx.SrcOver)
			backing(b, x, y)
			pop := pushClipKind(b, i/2, x, y)
			mask := gfx.Mask{}
			if i%3 == 1 {
				mask = maskOf(gfx.MaskMode(i/3%2), func(mb *gfx.Builder) {
					mb.FillRect(gfx.RectXYWH(x+10, y, 40, 72), gfx.LinearGradient{Start: gfx.Pt(x, y), End: gfx.Pt(x, y+72), Stops: stops(gfx.White, gfx.Transparent)})
					// A mask is a list like any other: operators work in it.
					mb.PushClipRect(gfx.RectXYWH(x, y+30, 72, 16))
					mb.PushLayer(0.5, operators[(i+4)%len(operators)])
					mb.FillRect(gfx.RectXYWH(x+20, y+20, 36, 30), gfx.White)
					mb.PopLayer()
					mb.PopClip()
				})
			}
			if mask.List != nil {
				b.PushMaskLayer(mask, 0.9, m, filters[i%6]...)
			} else {
				b.PushLayer(0.9, m, filters[i%6]...)
			}
			operand(b, x, y)
			// Nested: in the layer, the operator works on the layer.
			b.PushLayer(1, operators[(i+5)%len(operators)], filters[(i+3)%6]...)
			b.FillRect(gfx.RectXYWH(x+26, y+20, 30, 30), sky)
			b.PopLayer()
			b.PopLayer()
			pop()
			b.PopLayer()
		}
	}}},
	// Glass panes over a background that operators cleared, operators on the
	// panes, and panes in a layer an operator composites.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_panes", W: 400, H: 300, Draw: func(b *gfx.Builder) { operatorPanes(b, 0, 0) }}},
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_operators_glass", W: 240, H: 160, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 240, 160))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, 220, 140), 24))
		b.PushLayer(1, gfx.SrcAtop)
		b.FillRect(gfx.RectXYWH(20, 20, 120, 80), amber)
		b.FillRect(gfx.RectXYWH(60, 0, 8, 160), indigo)
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(30, 30, 90, 90), 12), gfx.Blur{StdDev: 6})
		b.PushLayer(1, gfx.DstOut)
		b.FillPath(gfx.CirclePath(gfx.Pt(80, 70), 24), gfx.NonZero, gfx.Black)
		b.PopLayer()
		b.PopLayer()
		b.PushLayer(1, gfx.DstIn, gfx.Blur{StdDev: 4})
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(20, 20, 190, 120), 40), gfx.Black)
		b.PopLayer()
		b.PopClip()
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(120, 40, 100, 80), 16), gfx.Blur{StdDev: 10})
	}}},
	// Every shape with a pattern: repeated and not, translated by whole
	// pixels, scaled, rotated, skewed, mirrored, minified along one axis,
	// far from its origin.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_patterns", W: 600, H: 480, Draw: func(b *gfx.Builder) {
		for i, p := range []gfx.Paint{
			tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Translate(3, 5)),
			tilePattern(gfx.RepeatX, gfx.SampleLinear, gfx.Affine{2.5, 0, 0, 2.5, 7, 22}),
			tilePattern(gfx.RepeatY, gfx.SampleNearest, gfx.Affine{3.1, 0, 0, 3.1, 60, 5}),
			tilePattern(gfx.RepeatNone, gfx.SampleLinear, gfx.Affine{14, 1, -2, 12, 18, 6}),
			tilePattern(gfx.RepeatBoth, gfx.SampleNearest, gfx.Rotate(0.7).Mul(gfx.Scale(2.3, 1.1))),
			tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Affine{-1.7, 0, 0.9, -1.3, 30, 40}),
			tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Translate(-123456.5, 98765.25)),
			tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Affine{1.3, 0.2, 0, 1.3, 54321, -777}),
			photoPattern(gfx.Scale(0.31, 0.27)),
			photoPattern(gfx.Affine{0.04, 0, 0, 0.9, 10, 10}),
			photoPattern(gfx.Rotate(-0.4).Mul(gfx.Scale(0.12, 0.12))),
			gfx.ImagePattern{Image: photo, Repeat: gfx.RepeatNone, Transform: gfx.Affine{1.6, 0.3, -0.3, 1.1, 30, 0}},
		} {
			b.PushTransform(gfx.Translate(200*float32(i%3), 120*float32(i/3)))
			shapesOf(b, p)
			b.PopTransform()
		}
	}}},
	// Patterns through clips and clip masks, in layers with an opacity, a
	// blend mode and an operator, in a mask, and under a filter.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_patterns_layers", W: 432, H: 216, Draw: func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 0, 432, 216), 12)
		tile := tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Scale(1.4, 1.4))
		for k := range clipKinds {
			x := float32(72 * k)
			pop := pushClipKind(b, k, x, 0)
			b.FillRect(gfx.RectXYWH(x, 0, 72, 72), tile)
			pop()
			pop = pushClipKind(b, k+1, x, 72)
			b.PushLayer(0.7, []gfx.BlendMode{gfx.SrcOver, gfx.Multiply, gfx.DstOver, gfx.SrcOut, gfx.Copy, gfx.Difference}[k])
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+10, 82, 50, 50), 16), photoPattern(gfx.Scale(0.5, 0.5)))
			b.PopLayer()
			pop()
		}
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(mb *gfx.Builder) { mb.FillRect(gfx.RectXYWH(0, 144, 144, 72), tile) }), 1, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(0, 144, 144, 72), indigo)
		b.PopLayer()
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 2})
		b.FillPath(gfxtest.StarPath(216, 180, 34), gfx.NonZero, photoPattern(gfx.Translate(5, 3)))
		b.PopLayer()
		b.PushTransform(gfx.RotateAbout(0.2, gfx.Pt(340, 180)))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(300, 150, 110, 56), 20))
		b.FillRect(gfx.RectXYWH(290, 140, 140, 80), tilePattern(gfx.RepeatBoth, gfx.SampleNearest, gfx.Scale(3, 3)))
		b.PopClip()
		b.PopTransform()
	}}},
	// Opaque patterns over what they hide, whole and in part, and patterns
	// that hide nothing.
	{clear: gfx.White, Scene: gfxtest.Scene{Name: "gpu_pattern_covers", W: 360, H: 240, Draw: func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 360, 240))
		b.FillRect(gfx.RectXYWH(20, 20, 90, 90), rose)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, 120, 120), 20), photoPattern(gfx.Affine{0.7, 0.2, -0.1, 0.6, 3, 4}))
		b.FillRect(gfx.RectXYWH(150, 20, 90, 90), rose)
		b.FillRect(gfx.RectXYWH(140, 10, 110, 110), gfx.ImagePattern{Image: photo, Repeat: gfx.RepeatX, Transform: gfx.Translate(0, 30)})
		b.FillRect(gfx.RectXYWH(20, 140, 90, 90), rose)
		b.FillRect(gfx.RectXYWH(10, 130, 110, 110), tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Identity))
		b.PushLayer(1, gfx.Xor)
		b.FillRect(gfx.RectXYWH(150, 140, 90, 90), rose)
		b.PopLayer()
		b.FillRect(gfx.RectXYWH(180, 130, 170, 100), photoPattern(gfx.Scale(0.4, 0.4)))
	}}},
}
