package canvas

import (
	"image"
	"image/color"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// scene is a drawing that goldens keep, written with what a browser's canvas
// has too, and with numbers only, so that the same can be drawn there.
type scene struct {
	name string
	w, h float64
	dir  text.Direction
	draw func(c *pen)
	// cells are the parts compared with Chromium, one by one.
	cells []sceneCell
}

// The initial state of the scenes: black, in Inter of 16 px, and lines of 1.

var (
	indigo = gfx.MustParseHex("#4f46e5")
	rose   = gfx.MustParseHex("#e11d48")
	amber  = gfx.MustParseHex("#f59e0b")
	slate  = gfx.MustParseHex("#475569")
	sky    = gfx.MustParseHex("#38bdf8")
)

func scenes() []scene {
	return []scene{
		{"paths", 400, 300, 0, drawPaths, nil},
		{"dashes", 400, 200, 0, drawDashes, nil},
		{"gradients", 400, 320, 0, drawGradients, nil},
		{"patterns", 400, 240, 0, drawPatterns, nil},
		{"text", 480, 420, 0, drawTexts, nil},
		{"text_rtl", 320, 160, text.DirectionRTL, drawTextsRTL, nil},
		{"images", 400, 300, 0, drawImages, nil},
		{"shadows", 480, 400, 0, drawShadows, nil},
		{"operations", 480, 400, 0, drawOperations, nil},
	}
}

// drawPaths shows every kind of segment.
func drawPaths(c *pen) {
	c.LineWidth = 3
	c.FillStyle, c.StrokeStyle = sky, indigo
	shape := func(x, y float64, build func()) {
		c.Save()
		c.Translate(x, y)
		c.BeginPath()
		build()
		c.Fill()
		c.Stroke()
		c.Restore()
	}
	shape(10, 10, func() {
		c.MoveTo(0, 60)
		c.QuadraticCurveTo(30, -40, 60, 60)
		c.QuadraticCurveTo(60, 20, 80, 30)
	})
	shape(110, 10, func() {
		c.MoveTo(0, 40)
		c.BezierCurveTo(0, -20, 80, -20, 80, 40)
		c.BezierCurveTo(80, 80, 20, 20, 0, 60)
		c.ClosePath()
	})
	shape(210, 10, func() {
		c.MoveTo(0, 60)
		c.ArcTo(40, -20, 80, 60, 15)
		c.ArcTo(80, 60, 0, 60, 8)
		c.ArcTo(0, 60, 40, -20, 8)
		c.ClosePath()
	})
	shape(310, 10, func() {
		c.Ellipse(40, 35, 40, 20, math.Pi/6, 0, 2*math.Pi)
	})
	shape(10, 100, func() { c.RoundRect(0, 0, 80, 60, 16) })
	shape(110, 100, func() { c.RoundRect(0, 0, 80, 60, 30, 4) })
	shape(210, 100, func() { c.RoundRect(0, 0, 80, 60, 0, 12, 28) })
	shape(310, 100, func() { c.RoundRect(80, 60, -80, -60, 28, 20, 10, 0) })
	shape(10, 190, func() {
		c.Arc(40, 40, 40, 0.3, 2*math.Pi-0.3)
		c.LineTo(40, 40)
		c.ClosePath()
	})
	shape(110, 190, func() {
		c.Ellipse(40, 40, 40, 25, 0, 0, math.Pi, true)
		c.Ellipse(40, 50, 30, 20, 0, 0, math.Pi)
	})
	heart := MustPath("M12 21s-7-4.6-9.2-9A5 5 0 0 1 12 6.5 5 5 0 0 1 21.2 12C19 16.400 12 21 12 21z")
	c.Save()
	c.Translate(205, 185)
	c.Scale(3.6, 3.6)
	c.LineWidth = 1
	c.FillStyle, c.StrokeStyle = rose, slate
	c.FillPath(heart)
	c.StrokePath(heart)
	c.Restore()
	var rings Path2D
	rings.Arc(40, 40, 40, 0, 2*math.Pi)
	rings.Rect(15, 15, 50, 50)
	inner := MustPath("M40 20 L60 60 L20 60 Z")
	rings.AddPath(inner)
	c.Translate(310, 190)
	c.FillRule = gfx.EvenOdd
	c.FillPath(&rings)
	c.ClipPath(&rings)
	c.FillStyle = amber
	c.Transform(1, 0.3, 0, 1, 0, 0)
	c.FillRect(0, 20, 80, 12)
}

// drawDashes shows line dashes.
func drawDashes(c *pen) {
	c.LineWidth = 6
	c.StrokeStyle = indigo
	line := func(y float64) {
		c.BeginPath()
		c.MoveTo(10, y)
		c.LineTo(190, y)
		c.Stroke()
	}
	c.SetLineDash(20, 10)
	line(15)
	c.LineDashOffset = 15
	line(35)
	c.LineDashOffset = 0
	c.SetLineDash(12)
	line(55)
	c.SetLineDash(1, 12, 12, 12)
	c.LineCap = gfx.CapRound
	line(75)
	c.SetLineDash(0, 14)
	line(95)
	c.LineCap = gfx.CapSquare
	line(115)
	c.SetLineDash()
	line(135)
	c.LineCap = gfx.CapButt
	c.SetLineDash(10, 4)
	c.SetLineDash(3, -1) // ignored
	line(155)
	c.StrokeStyle = rose
	c.LineWidth = 4
	c.StrokeRect(220, 15, 80, 60)
	c.BeginPath()
	c.Arc(350, 45, 32, 0, 2*math.Pi)
	c.Stroke()
	c.Translate(220, 100)
	c.Scale(3, 1)
	c.StrokeRect(0, 0, 55, 70)
}

// drawGradients shows gradients, transformed and on lines and text.
func drawGradients(c *pen) {
	stops := func(g *CanvasGradient) *CanvasGradient {
		g.AddColorStop(1, indigo)
		g.AddColorStop(0, amber)
		g.AddColorStop(0.5, gfx.Color{R: 1, A: 0})
		return g
	}
	c.FillStyle = stops(c.CreateLinearGradient(10, 0, 90, 30))
	c.FillRect(0, 10, 100, 60)
	c.FillStyle = stops(c.CreateRadialGradient(150, 40, 0, 150, 40, 40))
	c.FillRect(110, 10, 80, 60)
	c.FillStyle = stops(c.CreateRadialGradient(235, 35, 8, 250, 40, 38))
	c.FillRect(210, 10, 80, 60)
	c.FillStyle = stops(c.CreateRadialGradient(320, 40, 5, 370, 40, 20))
	c.FillRect(305, 10, 90, 60)
	c.FillStyle = stops(c.CreateConicGradient(math.Pi/4, 50, 120))
	c.FillRect(10, 85, 80, 70)
	hard := c.CreateLinearGradient(0, 85, 0, 155)
	for i, s := range []gfx.Color{rose, rose, sky, sky, slate, slate} {
		hard.AddColorStop(float64((i+1)/2)/3, s)
	}
	c.FillStyle = hard
	c.FillRect(110, 85, 80, 70)
	// One gradient, moved by the transform.
	g := c.CreateLinearGradient(0, 0, 40, 0)
	g.AddColorStop(0, sky)
	g.AddColorStop(1, indigo)
	c.FillStyle, c.StrokeStyle = g, g
	for i := range 4 {
		c.Save()
		c.Translate(210+float64(i)*45, 90)
		c.Rotate(float64(i) * math.Pi / 12)
		c.FillRect(0, 0, 40, 24)
		c.Restore()
	}
	c.LineWidth = 12
	c.Save()
	c.Translate(230, 125)
	c.Scale(4, 1)
	c.BeginPath()
	c.Arc(20, 15, 10, 0, 2*math.Pi)
	c.Stroke()
	c.Restore()
	// Circles under transforms that distort them.
	c.Save()
	c.Translate(10, 170)
	c.Transform(1.5, 0.2, 0.5, 0.8, 0, 0)
	c.FillStyle = stops(c.CreateRadialGradient(20, 25, 5, 30, 30, 30))
	c.BeginPath()
	c.Arc(30, 30, 28, 0, 2*math.Pi)
	c.Fill()
	c.Restore()
	c.Save()
	c.Translate(130, 170)
	c.Scale(1.5, 0.8)
	c.FillStyle = stops(c.CreateConicGradient(0, 30, 40))
	c.FillRect(0, 0, 60, 80)
	c.Restore()
	c.Save()
	c.Translate(330, 170)
	c.Scale(-1, 1)
	c.StrokeStyle = stops(c.CreateConicGradient(0, 40, 35))
	c.LineWidth = 16
	c.StrokeRect(10, 10, 60, 50)
	c.Restore()
	// Text.
	words := c.CreateLinearGradient(0, 0, 390, 0)
	words.AddColorStop(0, rose)
	words.AddColorStop(0.5, amber)
	words.AddColorStop(1, indigo)
	c.Font.Size, c.Font.Weight = 36, 700
	c.FillStyle, c.StrokeStyle = words, words
	c.FillText("Gradients", 10, 290)
	c.LineWidth = 1.5
	c.StrokeText("in text", 220, 290)
	c.FillStyle = stops(c.CreateConicGradient(0, 240, 270))
	c.GlobalAlpha = 0.5
	c.FillRect(340, 180, 50, 50)
	c.FillStyle = stops(c.CreateLinearGradient(5, 5, 5, 5)) // nothing
	c.FillRect(0, 0, 400, 320)
	c.FillStyle = c.CreateLinearGradient(0, 0, 5, 5) // nothing
	c.FillRect(0, 0, 400, 320)
}

// tile is the image of the patterns: 16 by 16, with a translucent corner.
func tile() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			switch {
			case x < 8 && y < 8:
				img.SetNRGBA(x, y, color.NRGBA{79, 70, 229, 255})
			case x >= 8 && y >= 8:
				img.SetNRGBA(x, y, color.NRGBA{245, 158, 11, 255})
			case x >= 12 && y < 4:
				img.SetNRGBA(x, y, color.NRGBA{225, 29, 72, 128})
			}
		}
	}
	return img
}

// photo is an image of 64 by 48 without transparency.
func photo() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), uint8(255 - x*2 - y), 255})
		}
	}
	for i := range 12 { // a mark at the top left
		for j := range 12 - i {
			img.SetRGBA(i, j, color.RGBA{255, 255, 255, 255})
		}
	}
	return img
}

// drawPatterns shows patterns.
func drawPatterns(c *pen) {
	img := tile()
	for i, repeat := range []PatternRepeat{gfx.RepeatBoth, gfx.RepeatX, gfx.RepeatY, gfx.RepeatNone} {
		c.Save()
		c.Translate(10+float64(i)*98, 10)
		c.FillStyle = c.CreatePattern(img, repeat)
		c.StrokeStyle = slate
		c.FillRect(0, 0, 85, 60)
		c.StrokeRect(0.5, 0.5, 84, 59)
		c.Restore()
	}
	p := c.CreatePattern(img, gfx.RepeatBoth)
	c.FillStyle = p
	c.Save()
	c.Translate(10, 85)
	c.BeginPath()
	c.RoundRect(-4, -4, 93, 68, 20) // the pattern goes on to the left and up
	c.Fill()
	c.Restore()
	c.Save()
	c.Translate(155, 85)
	c.Rotate(math.Pi / 6)
	c.Scale(2, 2)
	c.ImageSmoothing = false
	c.FillRect(0, 0, 40, 25)
	c.Restore()
	c.Save()
	c.Translate(235, 85)
	p.SetTransform(0.5, 0.2, -0.2, 0.5, 4, 4)
	c.GlobalAlpha = 0.5
	c.FillRect(0, 0, 70, 60)
	c.Restore()
	c.Save()
	p.SetTransform(0.3, 0, 0, 0.3, 0, 0)
	c.StrokeStyle = p
	c.LineWidth = 18
	c.BeginPath()
	c.Arc(350, 115, 25, 0, 2*math.Pi)
	c.Stroke()
	c.Restore()
	p.SetTransform(1, 0, 0, 1, 0, 0)
	c.Font.Size, c.Font.Weight = 56, 900
	c.FillText("Pattern", 10, 220)
	c.Translate(300, 170)
	c.Scale(1.5, 1.5)
	c.FillRect(0, 0, 60, 40)
}

// drawTexts shows text: alignments, baselines, fonts and all.
func drawTexts(c *pen) {
	c.StrokeStyle = rose
	c.BeginPath()
	c.MoveTo(100.5, 5)
	c.LineTo(100.5, 115)
	c.Stroke()
	for i, a := range []TextAlign{TextAlignStart, TextAlignEnd, TextAlignLeft, TextAlignRight, TextAlignCenter} {
		c.TextAlign = a
		c.FillText([]string{"start", "end", "left", "right", "center"}[i], 100, 22+float64(i)*22)
	}
	c.TextAlign = TextAlignStart
	c.BeginPath()
	c.MoveTo(170, 60.5)
	c.LineTo(475, 60.5)
	c.Stroke()
	x := 172.0
	c.Font.Size = 14
	for i, b := range []TextBaseline{BaselineAlphabetic, BaselineTop, BaselineHanging, BaselineMiddle, BaselineIdeographic, BaselineBottom} {
		c.TextBaseline = b
		s := []string{"Alpha", "Top", "Hang", "Middle", "Ideo", "Bottom"}[i]
		c.FillText(s, x, 60)
		x += c.MeasureText(s).Width + 10
	}
	c.TextBaseline = BaselineAlphabetic
	c.Font = CanvasFont{Size: 20}
	c.FillText("Arabic مرحبا بالعالم and 🎉👩‍🚀 emoji", 10, 150)
	c.Font = CanvasFont{Size: 20, Weight: 700, Italic: true}
	c.FillText("bold italic", 10, 180)
	c.Font = CanvasFont{Family: "monospace"}
	c.FillText("mono  spaces\tand\ntabs", 140, 180)
	c.Font = CanvasFont{Family: "No such font, 'JetBrains Mono'", Weight: 800}
	c.FillText("fallback", 360, 180)
	c.Font = CanvasFont{Size: 22}
	c.FillText("as wide as it is", 10, 210)
	c.FillText("condensed to be 120 wide", 190, 210, 120)
	c.TextAlign = TextAlignRight
	c.FillText("condensed to the right", 470, 210, 120)
	c.TextAlign = TextAlignStart
	c.LetterSpacing = 4
	c.Font.Size = 14
	c.FillText("letter spacing", 10, 235)
	c.LetterSpacing = -1
	c.FillText("closer letters", 170, 235)
	c.LetterSpacing = 0
	c.Font = CanvasFont{Size: 44, Weight: 800}
	c.LineWidth, c.StrokeStyle = 2, indigo
	c.StrokeText("Stroke", 10, 285)
	c.SetLineDash(4, 2)
	c.StrokeText("dash", 180, 285, 80)
	c.SetLineDash()
	c.FillStyle = amber
	c.LineWidth, c.LineJoin = 6, gfx.JoinRound
	c.StrokeText("both", 310, 285)
	c.FillText("both", 310, 285)
	c.FillStyle = slate
	c.GlobalAlpha = 0.5
	c.Font = CanvasFont{Size: 24}
	c.FillText("faded 🎈", 10, 320)
	c.GlobalAlpha = 1
	c.Save()
	c.Translate(190, 315)
	c.Rotate(-0.1)
	c.FillText("turned 🎈", 0, 0)
	c.Restore()
	c.Save()
	c.Translate(340, 330)
	c.Scale(1, 2)
	c.FillText("scaled", 0, 0)
	c.Restore()
	// The boxes of MeasureText.
	c.Font = CanvasFont{Size: 32}
	c.TextAlign, c.TextBaseline = TextAlignCenter, BaselineMiddle
	m := c.MeasureText("Ágjfy")
	c.FillText("Ágjfy", 80, 375)
	c.StrokeStyle, c.LineWidth = rose, 1
	c.StrokeRect(80-m.ActualBoundingBoxLeft, 375-m.ActualBoundingBoxAscent, m.ActualBoundingBoxLeft+m.ActualBoundingBoxRight, m.ActualBoundingBoxAscent+m.ActualBoundingBoxDescent)
	c.StrokeStyle = sky
	c.StrokeRect(80-m.Width/2, 375-m.FontBoundingBoxAscent, m.Width, m.FontBoundingBoxAscent+m.FontBoundingBoxDescent)
	c.StrokeStyle = indigo
	c.StrokeRect(80-m.Width/2, 375-m.EmHeightAscent, m.Width, m.EmHeightAscent+m.EmHeightDescent)
}

// drawTextsRTL shows text in a right-to-left element.
func drawTextsRTL(c *pen) {
	c.StrokeStyle = rose
	c.BeginPath()
	c.MoveTo(160.5, 5)
	c.LineTo(160.5, 155)
	c.Stroke()
	for i, a := range []TextAlign{TextAlignStart, TextAlignEnd, TextAlignLeft, TextAlignRight, TextAlignCenter} {
		c.TextAlign = a
		c.FillText([]string{"start: مرحبا!", "end: مرحبا!", "left, ", "right, ", "مرحبا 123"}[i], 160, 25+float64(i)*28)
	}
}

// drawImages shows images and icons.
func drawImages(c *pen) {
	img, small := photo(), tile()
	c.DrawImage(img, 10, 10)
	c.DrawImageSize(img, 90, 10, 100, 48)
	c.DrawImageRegion(img, 16, 12, 32, 24, 205, 10, 64, 48)
	c.DrawImageRegion(img, 48, 36, 32, 24, 285, 10, 64, 48)     // past its edges
	c.DrawImageRegion(img, 64, 48, -32, -24, 390, 58, -32, -24) // not mirrored
	c.DrawImageSize(small, 10, 70, 64, 64)
	c.ImageSmoothing = false
	c.DrawImageSize(small, 85, 70, 64, 64)
	c.ImageSmoothing = true
	c.DrawImageSize(img, 160, 70, 13, 9)
	c.GlobalAlpha = 0.5
	c.DrawImage(img, 180, 75)
	c.DrawImage(img, 200, 85)
	c.GlobalAlpha = 1
	c.Save()
	c.Translate(330, 100)
	c.Rotate(0.4)
	c.DrawImage(img, -32, -24)
	c.Restore()
	c.Save()
	c.Translate(80, 150)
	c.Scale(-1, 1)
	c.DrawImage(img, 0, 0) // mirrored
	c.Restore()
	c.Save()
	c.BeginPath()
	c.Arc(140, 174, 24, 0, 2*math.Pi)
	c.Clip()
	c.DrawImageSize(img, 108, 150, 64, 48)
	c.Restore()
	bell := Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.268 21a2 2 0 0 0 3.464 0"/><path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.410 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326"/></svg>`}
	badge := Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" fill="#38bdf8"/><path d="M7 12l3 3 7-7" fill="none" stroke="currentColor" stroke-width="3"/></svg>`}
	c.FillStyle = indigo
	c.DrawIcon(bell, 190, 150, 48, 48)
	c.DrawIcon(badge, 250, 150, 48, 48)
	c.GlobalAlpha = 0.4
	c.DrawIcon(badge, 310, 150, 48, 48)
	c.GlobalAlpha = 1
	g := c.CreateLinearGradient(0, 210, 0, 280)
	g.AddColorStop(0, amber)
	g.AddColorStop(1, rose)
	c.FillStyle = g
	c.DrawIcon(bell, 10, 210, 70, 70)
	c.DrawIcon(badge, 100, 210, 70, 70)
	c.FillStyle = slate
	c.Translate(230, 240)
	c.Rotate(-0.3)
	c.DrawIcon(bell, -35, -35, 100, 70) // fitted
}

// drawShadows shows shadows.
func drawShadows(c *pen) {
	cell := func(col, row int, draw func()) {
		c.Save()
		c.Translate(20+float64(col)*118, 15+float64(row)*100)
		c.ShadowColor = gfx.Black.WithAlpha(0.6)
		c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = 10, 6, 8
		c.FillStyle, c.StrokeStyle = sky, indigo
		draw()
		c.Restore()
	}
	cell(0, 0, func() { c.FillRect(0, 0, 80, 60) })
	cell(1, 0, func() {
		c.BeginPath()
		c.RoundRect(0, 0, 80, 60, 20)
		c.Fill()
	})
	cell(2, 0, func() {
		c.BeginPath()
		c.MoveTo(40, 0)
		c.LineTo(80, 60)
		c.LineTo(0, 60)
		c.Fill()
	})
	cell(3, 0, func() {
		c.LineWidth = 6
		c.StrokeRect(3, 3, 74, 54)
	})
	cell(0, 1, func() {
		c.Font.Size, c.Font.Weight = 34, 800
		c.FillText("Text", 0, 45)
	})
	cell(1, 1, func() { c.DrawImage(photo(), 8, 6) })
	cell(2, 1, func() {
		g := c.CreateLinearGradient(0, 0, 80, 0)
		g.AddColorStop(0, rose)
		g.AddColorStop(1, rose.WithAlpha(0))
		c.FillStyle = g
		c.FillRect(0, 0, 80, 60)
	})
	cell(3, 1, func() {
		c.ShadowBlur = 0
		c.ShadowColor = amber
		c.Rotate(0.2)
		c.Scale(1.2, 0.8) // the shadow is not scaled nor turned
		c.FillRect(0, 0, 60, 60)
	})
	cell(0, 2, func() {
		c.FillStyle = rose.WithAlpha(0.5) // its shadow is fainter, and shows through
		c.FillRect(0, 0, 80, 60)
	})
	cell(1, 2, func() {
		c.GlobalAlpha = 0.5
		c.BeginPath()
		c.RoundRect(0, 0, 80, 60, 20)
		c.Fill()
	})
	cell(2, 2, func() {
		c.Font.Size = 34
		c.FillText("😀", 10, 45)
	})
	cell(3, 2, func() {
		c.BeginPath()
		c.Rect(-10, -10, 80, 50)
		c.Clip()
		c.FillRect(0, 0, 60, 40)
	})
	cell(0, 3, func() {
		c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = 30, 0, 0
		c.ShadowColor = indigo
		c.Rotate(0.2)
		c.FillRect(10, 0, 70, 50)
	})
	cell(1, 3, func() {
		c.ShadowBlur, c.ShadowOffsetX = 0, 100 // only its shadow is inside
		c.ShadowColor = rose
		c.BeginPath()
		c.Arc(-60, 30, 30, 0, 2*math.Pi)
		c.Fill()
	})
	cell(2, 3, func() {
		c.FillRect(0, 0, 50, 50)
		c.ShadowColor = rose
		c.FillStyle = amber
		c.GlobalCompositeOperation = DestinationOver
		c.FillRect(30, 10, 50, 50)
	})
	cell(3, 3, func() {
		c.FillStyle = c.CreatePattern(tile(), gfx.RepeatBoth)
		c.FillRect(0, 0, 80, 60)
	})
}

// drawOperations shows the operations: a square over a circle, inside a
// clip.
func drawOperations(c *pen) {
	for i := range len(operations) {
		c.Save()
		c.Translate(6+float64(i%6)*80, 6+float64(i/6)*80)
		c.BeginPath()
		c.Rect(0, 0, 70, 70)
		c.Clip()
		g := c.CreateLinearGradient(0, 0, 45, 45)
		g.AddColorStop(0, sky)
		g.AddColorStop(1, indigo)
		c.FillStyle = g
		c.BeginPath()
		c.Arc(26, 26, 22, 0, 2*math.Pi)
		c.Fill()
		c.GlobalCompositeOperation = CompositeOperation(i)
		c.FillStyle = amber.WithAlpha(0.8)
		c.FillRect(24.5, 24.5, 40, 40)
		c.Restore()
	}
}
