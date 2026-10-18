package raster

import (
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// Scenes of blend modes, mask layers and focal gradients. The first three
// (blend, masks, focal) are also written as HTML (blendHTML, masksHTML,
// focalHTML) and compared with Chromium (chromium4_test.go); the others
// cover what CSS cannot express (glyph masks, under rotations, nested in
// filter layers, non-isolated alpha).

// Colors outside the sRGB gamut (ADR-046): a solid color clips per channel
// and a gradient interpolates the unclipped stops.
var (
	oogGreen  = gfx.OKLCH(0.75, 0.3, 150, 1)
	oogYellow = gfx.OKLCH(0.9, 0.25, 100, 1)
)

// allBlendModes lists every mode in declaration order.
var allBlendModes = func() []gfx.BlendMode {
	var out []gfx.BlendMode
	for m := gfx.SrcOver; m <= gfx.PlusLighter; m++ {
		out = append(out, m)
	}
	return out
}()

// blendCell is a 100x100 cell of the blend scene.
type blendCell struct {
	mode gfx.BlendMode
	x, y float32 // cell origin
	clip bool    // under a rounded clip, over the page (not isolated)
}

func blendCells() []blendCell {
	var cells []blendCell
	for i, m := range allBlendModes {
		cells = append(cells, blendCell{mode: m, x: float32(100 * (i % 6)), y: float32(100 * (i / 6))})
	}
	return append(cells, blendCell{mode: gfx.Difference, x: 500, y: 200, clip: true})
}

// drawBlendCell draws a varied backdrop (an opaque gradient, a translucent
// one and an out-of-gamut solid stripe) in an isolated group, and over it,
// with the cell's blend mode at opacity 0.9, a source whose gradient goes
// from an out-of-gamut color to a translucent one. The clip cell draws the
// backdrop on the page and the source under a rounded clip.
func drawBlendCell(b *gfx.Builder, c blendCell) {
	gx, gy := c.x+4, c.y+4
	if !c.clip {
		b.PushLayer(1, gfx.SrcOver) // isolation: isolate
	}
	b.FillRect(gfx.RectXYWH(gx, gy, 92, 60), gfx.LinearGradient{Start: gfx.Pt(gx, gy), End: gfx.Pt(gx+92, gy),
		Stops: stops(red, lime, sky), Interp: gfx.InterpSRGB})
	b.FillRect(gfx.RectXYWH(gx, gy+60, 92, 32), gfx.LinearGradient{Start: gfx.Pt(gx, gy), End: gfx.Pt(gx+92, gy),
		Stops: stops(indigo.WithAlpha(0.6), amber.WithAlpha(0.3)), Interp: gfx.InterpSRGB})
	b.FillRect(gfx.RectXYWH(gx+60, gy, 16, 92), oogGreen)
	src := gfx.RectXYWH(gx+14, gy+14, 64, 64)
	if c.clip {
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(gx+10, gy+10, 72, 72), 20))
		src = gfx.RectXYWH(gx+4, gy+4, 84, 84)
	}
	b.PushLayer(0.9, c.mode)
	b.FillRect(src, gfx.LinearGradient{Start: gfx.Pt(src.X0, src.Y0), End: gfx.Pt(src.X0, src.Y1),
		Stops: stops(oogYellow, fuchsia, sky.WithAlpha(0.4))})
	b.PopLayer()
	if c.clip {
		b.PopClip()
	} else {
		b.PopLayer()
	}
}

// maskCell is a 160x130 cell of the masks scene. Its element is the box
// (10, 10, 140, 110) of the cell.
type maskCell struct {
	name string
	x, y float32
}

var maskCells = []maskCell{
	{"alpha linear", 0, 0}, {"luminance", 160, 0}, {"radial", 320, 0},
	{"nested", 0, 130}, {"clip+rotate", 160, 130}, {"opacity+multiply", 320, 130},
}

// maskContent paints the content of a masked element at e: a gradient and
// a translucent child overlapping it, so that masking the group differs
// from masking each command.
func maskContent(b *gfx.Builder, e gfx.Rect) {
	b.FillRect(e, gfx.LinearGradient{Start: gfx.Pt(e.X0, e.Y0), End: gfx.Pt(e.X1, e.Y0), Stops: stops(indigo, rose), Interp: gfx.InterpSRGB})
	b.FillRect(gfx.RectXYWH(e.X0+30, e.Y0+30, 80, 50), sky.WithAlpha(0.7))
}

// maskList records a mask: draw paints it.
func maskList(draw func(b *gfx.Builder)) *gfx.DisplayList { return build(draw) }

func drawMaskCell(b *gfx.Builder, c maskCell) {
	e := gfx.RectXYWH(c.x+10, c.y+10, 140, 110)
	// The page under every cell: a backdrop for the masked group.
	b.FillRect(gfx.RectXYWH(c.x+4, c.y+4, 152, 122), gfx.LinearGradient{Start: gfx.Pt(c.x, c.y+4), End: gfx.Pt(c.x, c.y+126),
		Stops: stops(amber.WithAlpha(0.35), emerald.WithAlpha(0.35)), Interp: gfx.InterpSRGB})
	toBottom := func(e gfx.Rect, s ...gfx.Color) gfx.Paint {
		return gfx.LinearGradient{Start: gfx.Pt(e.X0, e.Y0), End: gfx.Pt(e.X0, e.Y1), Stops: stops(s...), Interp: gfx.InterpSRGB}
	}
	toRight := func(e gfx.Rect, s ...gfx.Color) gfx.Paint {
		return gfx.LinearGradient{Start: gfx.Pt(e.X0, e.Y0), End: gfx.Pt(e.X1, e.Y0), Stops: stops(s...), Interp: gfx.InterpSRGB}
	}
	radial := func(e gfx.Rect) gfx.Paint {
		return gfx.RadialGradient{Center: e.Center(), Radius: gfx.Pt(e.Width()/2, e.Height()/2),
			Stops: []gfx.GradientStop{stop(0.6, gfx.Black), stop(1, gfx.Transparent)}, Interp: gfx.InterpSRGB}
	}
	switch c.name {
	case "alpha linear":
		m := maskList(func(mb *gfx.Builder) { mb.FillRect(e, toBottom(e, gfx.Black, gfx.Transparent)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 1, gfx.SrcOver)
		maskContent(b, e)
		b.PopLayer()
	case "luminance":
		m := maskList(func(mb *gfx.Builder) {
			mb.FillRect(e, toRight(e, gfx.White, gfx.RGB8(255, 0, 0), gfx.RGBA8(0, 255, 0, 128), gfx.Black))
		})
		b.PushMaskLayer(gfx.Mask{List: m, Mode: gfx.MaskLuminance}, 1, gfx.SrcOver)
		maskContent(b, e)
		b.PopLayer()
	case "radial":
		m := maskList(func(mb *gfx.Builder) { mb.FillRect(e, radial(e)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 1, gfx.SrcOver)
		maskContent(b, e)
		b.PopLayer()
	case "nested":
		outer := maskList(func(mb *gfx.Builder) { mb.FillRect(e, toRight(e, gfx.Black, gfx.Transparent)) })
		in := gfx.RectXYWH(e.X0+20, e.Y0+15, 100, 80)
		inner := maskList(func(mb *gfx.Builder) { mb.FillRect(in, radial(in)) })
		b.PushMaskLayer(gfx.Mask{List: outer}, 1, gfx.SrcOver)
		b.FillRect(e, toBottom(e, emerald, slate))
		b.PushMaskLayer(gfx.Mask{List: inner}, 1, gfx.SrcOver)
		b.FillRect(in, rose)
		b.FillRect(gfx.RectXYWH(in.X0+40, in.Y0, 20, 80), yellow)
		b.PopLayer()
		b.PopLayer()
	case "clip+rotate":
		b.PushClipRRect(gfx.NewRRect(e, 24))
		b.PushTransform(gfx.RotateAbout(float32(12*math.Pi/180), e.Center()))
		m := maskList(func(mb *gfx.Builder) { mb.FillRect(e, toBottom(e, gfx.Black, gfx.Transparent)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 1, gfx.SrcOver)
		maskContent(b, e)
		b.PopLayer()
		b.PopTransform()
		b.PopClip()
	case "opacity+multiply":
		b.FillRect(gfx.RectXYWH(e.X0, e.Y0+40, 140, 30), sky)
		m := maskList(func(mb *gfx.Builder) { mb.FillRect(e, radial(e)) })
		b.PushMaskLayer(gfx.Mask{List: m}, 0.7, gfx.Multiply)
		maskContent(b, e)
		b.PopLayer()
	}
}

// focalCell is a 150x150 cell of the focal scene, filled with a two-point
// gradient (fx, fy, fr) -> (cx, cy, r), relative to the cell's center,
// under an optional vertical scale about the center.
type focalCell struct {
	name       string
	fx, fy, fr float32
	cx, cy, r  float32
	stops      []gfx.Color
	scaleY     float32 // 0 means 1
	x, y       float32 // cell origin
	interp     gfx.ColorSpace
}

func focalCells() []focalCell {
	cells := []focalCell{
		{name: "focal", fx: -30, fy: -25, cx: 0, cy: 0, r: 60, stops: []gfx.Color{amber, rose, indigo}},
		{name: "focal radius", fx: 20, fy: 10, fr: 15, r: 65, stops: []gfx.Color{sky, emerald, slate}},
		{name: "focal on edge", fx: 60, r: 60, stops: []gfx.Color{lime, sky, indigo}},
		{name: "cone", fx: -45, fr: 10, cx: 30, r: 30, stops: []gfx.Color{rose, amber, lime}},
		{name: "concentric", fr: 20, r: 60, stops: []gfx.Color{indigo, amber, rose}},
		{name: "strip", fx: -30, fr: 25, cx: 30, r: 25, stops: []gfx.Color{emerald, fuchsia}},
		{name: "ellipse", fx: -30, fy: -25, r: 60, stops: []gfx.Color{amber, rose, indigo}, scaleY: 0.6},
		{name: "shrinking", fr: 60, cx: 20, cy: -10, r: 5, stops: []gfx.Color{slate, sky, yellow}},
	}
	for i := range cells {
		cells[i].x, cells[i].y = float32(150*(i%4)), float32(150*(i/4))
		cells[i].interp = gfx.InterpSRGB
	}
	return cells
}

func (c focalCell) paint() gfx.FocalGradient {
	ox, oy := c.x+75, c.y+75
	return gfx.FocalGradient{Focal: gfx.Pt(ox+c.fx, oy+c.fy), FocalRadius: c.fr, Center: gfx.Pt(ox+c.cx, oy+c.cy), Radius: c.r,
		Stops: stops(c.stops...), Interp: c.interp}
}

func drawFocalCell(b *gfx.Builder, c focalCell) {
	cell := gfx.RectXYWH(c.x, c.y, 150, 150)
	if c.scaleY != 0 {
		b.PushTransform(gfx.ScaleAbout(1, c.scaleY, gfx.Pt(c.x+75, c.y+75)))
		cell = gfx.RectXYWH(c.x, c.y+75-75/c.scaleY, 150, 150/c.scaleY)
		b.PushClipRect(cell)
		b.FillRect(cell, c.paint())
		b.PopClip()
		b.PopTransform()
		return
	}
	b.FillRect(cell, c.paint())
}

var scenes4 = []scene{
	{"blend", 600, 300, func(b *gfx.Builder) {
		for _, c := range blendCells() {
			drawBlendCell(b, c)
		}
	}},
	{"masks", 480, 260, func(b *gfx.Builder) {
		for _, c := range maskCells {
			drawMaskCell(b, c)
		}
	}},
	{"focal", 600, 300, func(b *gfx.Builder) {
		for _, c := range focalCells() {
			drawFocalCell(b, c)
		}
	}},
	{"blend_more", 240, 200, drawBlendMore},
	{"masks_more", 240, 200, drawMasksMore},
	{"focal_more", 240, 120, drawFocalMore},
}

// drawBlendMore covers blend layers that CSS pages do not make: blending
// onto a transparent backdrop (the source comes out unchanged), a blend
// layer under a rotation and an anti-aliased path clip, and a blend layer
// with a blur filter (filters run before blending).
func drawBlendMore(b *gfx.Builder) {
	b.PushLayer(1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(10, 10, 60, 60), gfx.RadialGradient{Center: gfx.Pt(40, 40), Radius: gfx.Pt(30, 30),
		Stops: stops(yellow, rose.WithAlpha(0.5), gfx.Transparent)})
	b.PushLayer(1, gfx.ColorDodge)
	b.FillRect(gfx.RectXYWH(25, 0, 30, 80), sky.WithAlpha(0.8))
	b.PopLayer()
	b.PopLayer()

	b.FillRect(gfx.RectXYWH(90, 10, 140, 70), gfx.ConicGradient{Center: gfx.Pt(160, 45), Stops: stops(red, yellow, lime, sky, indigo, red)})
	b.PushClipPath(starPath(160, 48, 44), gfx.NonZero)
	b.PushTransform(gfx.RotateAbout(0.4, gfx.Pt(160, 45)))
	for i, m := range []gfx.BlendMode{gfx.Hue, gfx.Saturation, gfx.ColorBlend, gfx.Luminosity} {
		b.PushLayer(0.8, m)
		b.FillRect(gfx.RectXYWH(110+float32(i)*25, 15, 22, 60), []gfx.Color{slate, emerald, fuchsia, amber}[i])
		b.PopLayer()
	}
	b.PopTransform()
	b.PopClip()

	checker(b, gfx.RectXYWH(10, 100, 220, 90), 10)
	for i, m := range []gfx.BlendMode{gfx.SoftLight, gfx.Overlay, gfx.PlusLighter, gfx.Exclusion} {
		x := 20 + float32(i)*52
		b.PushLayer(1, m, gfx.Blur{StdDev: 3})
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, 112, 40, 66), 10), gfx.LinearGradient{Start: gfx.Pt(x, 112), End: gfx.Pt(x, 178),
			Stops: stops(gfx.White, gfx.Color{R: 0.5, G: 0.5, B: 0.5, A: 1}, gfx.Black)})
		b.PopLayer()
	}
}

// drawMasksMore covers glyph masks (background-clip: text with the
// synthetic font, color glyphs included), a luminance mask under a
// rotation whose list has its own layers and transforms, a mask with a
// blurred (filter) list, and a masked filter layer.
func drawMasksMore(b *gfx.Builder) {
	text := maskList(func(mb *gfx.Builder) {
		mb.DrawGlyphRun(testFontA.run("AoSil*", 36, 12, 50, gfx.Black))
	})
	b.PushMaskLayer(gfx.Mask{List: text}, 1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(10, 10, 220, 50), gfx.LinearGradient{Start: gfx.Pt(10, 10), End: gfx.Pt(230, 60), Stops: stops(indigo, fuchsia, amber)})
	b.PopLayer()

	lum := maskList(func(mb *gfx.Builder) {
		mb.FillRect(gfx.RectXYWH(10, 70, 100, 120), gfx.Color{R: 0.3, G: 0.3, B: 0.3, A: 1})
		mb.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(60, 130)))
		mb.PushLayer(0.8, gfx.SrcOver)
		mb.FillRRect(gfx.NewRRect(gfx.RectXYWH(30, 100, 60, 60), 14), gfx.White)
		mb.PopLayer()
		mb.PopTransform()
	})
	b.PushTransform(gfx.RotateAbout(-0.2, gfx.Pt(60, 130)))
	b.PushMaskLayer(gfx.Mask{List: lum, Mode: gfx.MaskLuminance}, 1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(0, 60, 120, 140), gfx.RadialGradient{Center: gfx.Pt(60, 130), Radius: gfx.Pt(60, 60), Stops: stops(rose, indigo)})
	b.PopLayer()
	b.PopTransform()

	blurred := maskList(func(mb *gfx.Builder) {
		mb.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 6})
		mb.FillPath(starPath(175, 130, 50), gfx.NonZero, gfx.Black)
		mb.PopLayer()
	})
	b.PushMaskLayer(gfx.Mask{List: blurred}, 1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(4, 4), StdDev: 2, Color: slate})
	b.FillRect(gfx.RectXYWH(130, 80, 90, 100), gfx.LinearGradient{Start: gfx.Pt(130, 80), End: gfx.Pt(220, 180), Stops: stops(emerald, sky)})
	b.FillRect(gfx.RectXYWH(150, 100, 30, 30), gfx.White)
	b.PopLayer()
}

// drawFocalMore covers what the Chromium cells do not: Oklab
// interpolation, translucent stops, a rotation and hard stops.
func drawFocalMore(b *gfx.Builder) {
	b.FillRect(gfx.RectXYWH(0, 0, 120, 120), gfx.FocalGradient{Focal: gfx.Pt(40, 45), Center: gfx.Pt(60, 60), Radius: 55,
		Stops: stops(oogYellow, fuchsia.WithAlpha(0.5), sky)})
	b.PushTransform(gfx.RotateAbout(0.6, gfx.Pt(180, 60)))
	b.FillRect(gfx.RectXYWH(120, 0, 120, 120), gfx.FocalGradient{Focal: gfx.Pt(150, 60), FocalRadius: 8, Center: gfx.Pt(195, 60), Radius: 20,
		Stops: []gfx.GradientStop{stop(0, rose), stop(0.5, rose), stop(0.5, amber), stop(1, sky.WithAlpha(0))}, Interp: gfx.InterpSRGB})
	b.PopTransform()
}

// ---------------------------------------------------------------------------
// HTML for Chromium
// ---------------------------------------------------------------------------

// cssGradientStops formats stops as CSS color stops.
func cssStops(cs []gfx.Color) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = cssColorOf(c)
	}
	return strings.Join(parts, ", ")
}

// cssColorOf is cssColor, except for the out-of-gamut test colors, which
// are written in oklch() so that Chromium converts them itself.
func cssColorOf(c gfx.Color) string {
	switch c {
	case oogGreen:
		return "oklch(0.75 0.3 150)"
	case oogYellow:
		return "oklch(0.9 0.25 100)"
	}
	return cssColor(c)
}

const htmlHead = `<!doctype html>
<html><head><meta charset="utf-8"><style>
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
div { position: absolute; box-sizing: border-box; }
</style></head><body>
`

func px(v float32) string { return fmt.Sprintf("%gpx", v) }

func box(x, y, w, h float32) string {
	return fmt.Sprintf("left: %s; top: %s; width: %s; height: %s;", px(x), px(y), px(w), px(h))
}

// blendHTML is the blend scene as CSS: isolation, mix-blend-mode, opacity
// and gradients.
func blendHTML() string {
	var sb strings.Builder
	sb.WriteString(htmlHead)
	for _, c := range blendCells() {
		gx, gy := c.x+4, c.y+4
		backdrop := fmt.Sprintf(`<div style="%s background: linear-gradient(to right, %s);"></div>
<div style="%s background: linear-gradient(to right, %s);"></div>
<div style="%s background: %s;"></div>
`, box(0, 0, 92, 60), cssStops([]gfx.Color{red, lime, sky}),
			box(0, 60, 92, 32), cssStops([]gfx.Color{indigo.WithAlpha(0.6), amber.WithAlpha(0.3)}),
			box(60, 0, 16, 92), cssColorOf(oogGreen))
		srcBg := "linear-gradient(in oklab to bottom, " + cssStops([]gfx.Color{oogYellow, fuchsia, sky.WithAlpha(0.4)}) + ")"
		if c.clip {
			fmt.Fprintf(&sb, `<div title="clip %s" style="%s">
%s<div style="%s border-radius: 20px; overflow: hidden;"><div style="%s opacity: 0.9; mix-blend-mode: %s; background: %s;"></div></div>
</div>
`, c.mode, box(gx, gy, 92, 92), backdrop, box(10, 10, 72, 72), box(-6, -6, 84, 84), c.mode, srcBg)
			continue
		}
		fmt.Fprintf(&sb, `<div title="%s" style="%s isolation: isolate;">
%s<div style="%s opacity: 0.9; mix-blend-mode: %s; background: %s;"></div>
</div>
`, c.mode, box(gx, gy, 92, 92), backdrop, box(14, 14, 64, 64), c.mode, srcBg)
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// masksHTML is the masks scene as CSS mask-image.
func masksHTML() string {
	var sb strings.Builder
	sb.WriteString(htmlHead)
	content := fmt.Sprintf(`<div style="%s background: linear-gradient(to right, %s);"></div><div style="%s background: %s;"></div>`,
		box(0, 0, 140, 110), cssStops([]gfx.Color{indigo, rose}), box(30, 30, 80, 50), cssColor(sky.WithAlpha(0.7)))
	const radial = "radial-gradient(closest-side, black 60%, transparent)"
	for _, c := range maskCells {
		fmt.Fprintf(&sb, `<div title=%q style="%s background: linear-gradient(to bottom, %s);"></div>
`, c.name, box(c.x+4, c.y+4, 152, 122), cssStops([]gfx.Color{amber.WithAlpha(0.35), emerald.WithAlpha(0.35)}))
		e := box(c.x+10, c.y+10, 140, 110)
		switch c.name {
		case "alpha linear":
			fmt.Fprintf(&sb, `<div style="%s mask-image: linear-gradient(to bottom, black, transparent);">%s</div>
`, e, content)
		case "luminance":
			fmt.Fprintf(&sb, `<div style="%s mask-image: linear-gradient(to right, white, rgb(255 0 0), rgb(0 255 0 / 0.50196), black); mask-mode: luminance;">%s</div>
`, e, content)
		case "radial":
			fmt.Fprintf(&sb, `<div style="%s mask-image: %s;">%s</div>
`, e, radial, content)
		case "nested":
			fmt.Fprintf(&sb, `<div style="%s mask-image: linear-gradient(to right, black, transparent); background: linear-gradient(to bottom, %s);"><div style="%s mask-image: %s;"><div style="%s background: %s;"></div><div style="%s background: %s;"></div></div></div>
`, e, cssStops([]gfx.Color{emerald, slate}), box(20, 15, 100, 80), radial, box(0, 0, 100, 80), cssColor(rose), box(40, 0, 20, 80), cssColor(yellow))
		case "clip+rotate":
			fmt.Fprintf(&sb, `<div style="%s border-radius: 24px; overflow: hidden;"><div style="%s transform: rotate(12deg); mask-image: linear-gradient(to bottom, black, transparent);">%s</div></div>
`, e, box(0, 0, 140, 110), content)
		case "opacity+multiply":
			fmt.Fprintf(&sb, `<div style="%s background: %s;"></div><div style="%s mask-image: %s; opacity: 0.7; mix-blend-mode: multiply;">%s</div>
`, box(c.x+10, c.y+50, 140, 30), cssColor(sky), e, radial, content)
		}
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// focalHTML is the focal scene as canvas createRadialGradient, the
// two-point conical gradient of the web platform (CSS radial gradients
// have no focal point).
func focalHTML() string {
	var items []string
	for _, c := range focalCells() {
		var cols []string
		for _, s := range c.stops {
			cols = append(cols, fmt.Sprintf("%q", cssColor(s)))
		}
		sy := c.scaleY
		if sy == 0 {
			sy = 1
		}
		items = append(items, fmt.Sprintf("[%g, %g, %g, %g, %g, %g, %g, %g, %g, [%s]]",
			c.x, c.y, c.fx, c.fy, c.fr, c.cx, c.cy, c.r, sy, strings.Join(cols, ", ")))
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { display: block; width: 600px; height: 300px; }
</style></head><body><canvas id="c"></canvas>
<script>
const cells = [
%s
];
const dpr = window.devicePixelRatio, cv = document.getElementById("c");
cv.width = 600 * dpr; cv.height = 300 * dpr;
const ctx = cv.getContext("2d");
ctx.scale(dpr, dpr);
for (const [x, y, fx, fy, fr, cx, cy, r, sy, cols] of cells) {
  const ox = x + 75, oy = y + 75;
  ctx.save();
  ctx.beginPath(); ctx.rect(x, y, 150, 150); ctx.clip();
  ctx.translate(ox, oy); ctx.scale(1, sy); ctx.translate(-ox, -oy);
  const g = ctx.createRadialGradient(ox + fx, oy + fy, fr, ox + cx, oy + cy, r);
  cols.forEach((c, i) => g.addColorStop(i / (cols.length - 1), c));
  ctx.fillStyle = g;
  ctx.fillRect(x, oy - 75 / sy, 150, 150 / sy);
  ctx.restore();
}
</script></body></html>
`, strings.Join(items, ",\n"))
}
