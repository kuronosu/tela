package canvas

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

var (
	black = gfx.Color{A: 1}
	red   = gfx.Color{R: 1, A: 1}
	green = gfx.Color{G: 1, A: 1}
	blue  = gfx.Color{B: 1, A: 1}
)

// frameOf is the frame of a test canvas: black on nothing, lines of 1.
func frameOf(w, h float64) *Frame {
	return &Frame{Width: w, Height: h, Fill: black, Stroke: black, LineWidth: 1, Dev: true}
}

// record runs draw on a canvas of w by h.
func record(w, h float64, draw func(*Canvas)) *gfx.DisplayList {
	var r Recorder
	list, _ := r.Record(frameOf(w, h), draw)
	return list
}

// render returns the pixels of a canvas of w by h, over nothing.
func render(w, h int, scale float32, draw func(*Canvas)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, int(float32(w)*scale), int(float32(h)*scale)))
	raster.Render(img, record(float64(w), float64(h), draw), raster.Options{Scale: scale})
	return img
}

// The pixels of the tests, whose canvases are 100 by 50 (the size of the
// canvases of the web platform tests, whose cases many of these are).
var (
	none      = color.RGBA{}
	redPx     = color.RGBA{255, 0, 0, 255}
	greenPx   = color.RGBA{0, 255, 0, 255}
	bluePx    = color.RGBA{0, 0, 255, 255}
	blackPx   = color.RGBA{0, 0, 0, 255}
	halfRedPx = color.RGBA{128, 0, 0, 128}
)

type px struct {
	x, y int
	want color.RGBA
}

func at(want color.RGBA, pts ...int) []px {
	var out []px
	for i := 0; i+1 < len(pts); i += 2 {
		out = append(out, px{pts[i], pts[i+1], want})
	}
	return out
}

func pxs(l ...[]px) []px {
	var out []px
	for _, p := range l {
		out = append(out, p...)
	}
	return out
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) <= 2 && int(y)-int(x) <= 2 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// TestDrawing draws on canvases of 100 by 50 and looks at pixels. The names
// in parentheses are the web platform tests of the same behavior.
func TestDrawing(t *testing.T) {
	fill := func(c *Canvas, s Style) { c.FillStyle = s; c.Fill() }
	poly := func(c *Canvas, pts ...float64) {
		c.BeginPath()
		for i := 0; i+1 < len(pts); i += 2 {
			c.LineTo(pts[i], pts[i+1])
		}
	}
	tests := []struct {
		name string
		draw func(c *Canvas)
		want []px
	}{
		// Rectangles.
		{"fillRect (2d.fillRect.basic)", func(c *Canvas) {
			c.FillStyle = green
			c.FillRect(0, 0, 100, 50)
		}, at(greenPx, 0, 0, 50, 25, 99, 49)},
		{"fillRect leaves the path (2d.fillRect.path)", func(c *Canvas) {
			c.BeginPath()
			c.Rect(0, 0, 100, 50)
			c.FillStyle = red
			c.FillRect(0, 0, 16, 32)
			fill(c, green)
		}, at(greenPx, 8, 8, 50, 25)},
		{"fillRect of no size (2d.fillRect.zero)", func(c *Canvas) {
			c.FillRect(0, 0, 100, 0)
			c.FillRect(0, 0, 0, 50)
		}, at(none, 0, 0, 50, 25)},
		{"fillRect to the left and up (2d.fillRect.negative)", func(c *Canvas) {
			c.FillStyle = green
			c.FillRect(50, 25, -50, -25)
		}, pxs(at(greenPx, 25, 12), at(none, 75, 12, 25, 37))},
		{"fillRect transformed (2d.fillRect.transform)", func(c *Canvas) {
			c.Scale(10, 10)
			c.Translate(0, 5)
			c.FillStyle = green
			c.FillRect(0, -5, 10, 5)
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},
		{"fillRect turned", func(c *Canvas) {
			c.Translate(50, 25)
			c.Rotate(math.Pi / 4)
			c.FillRect(-10, -10, 20, 20)
		}, pxs(at(blackPx, 50, 25, 50, 13, 62, 25), at(none, 40, 15, 60, 35))},
		{"fillRect with infinite and NaN arguments (2d.fillRect.nonfinite)", func(c *Canvas) {
			for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
				c.FillRect(v, 0, 100, 50)
				c.FillRect(0, v, 100, 50)
				c.FillRect(0, 0, v, 50)
				c.FillRect(0, 0, 100, v)
			}
		}, at(none, 50, 25)},
		{"fillRect clipped (2d.fillRect.clip)", func(c *Canvas) {
			c.BeginPath()
			c.Rect(0, 0, 16, 16)
			c.Clip()
			c.FillRect(0, 0, 100, 50)
		}, pxs(at(blackPx, 8, 8), at(none, 50, 25))},
		{"strokeRect (2d.strokeRect.basic)", func(c *Canvas) {
			c.StrokeStyle = green
			c.LineWidth = 10
			c.StrokeRect(25, 15, 50, 20)
		}, pxs(at(greenPx, 25, 15, 75, 35, 50, 15, 21, 11), at(none, 50, 25, 10, 25))},
		{"strokeRect of no size (2d.strokeRect.zero.1)", func(c *Canvas) {
			c.LineWidth, c.LineCap = 30, gfx.CapRound
			c.StrokeRect(50, 25, 0, 0)
		}, at(none, 50, 25)},
		{"strokeRect without width is a line (2d.strokeRect.zero.4)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 10, green
			c.LineCap = gfx.CapRound
			c.StrokeRect(50, 15, 0, 20)
		}, pxs(at(greenPx, 50, 25, 46, 25, 53, 25, 50, 11), at(none, 40, 25, 60, 25))},
		{"strokeRect distorted", func(c *Canvas) {
			c.Scale(10, 1)
			c.StrokeStyle = green
			c.LineWidth = 2
			c.StrokeRect(3, 15, 4, 20)
		}, pxs(at(greenPx, 25, 25, 35, 25, 50, 15), at(none, 18, 25, 50, 18, 50, 25))},
		{"clearRect of everything (2d.clearRect.basic)", func(c *Canvas) {
			c.FillRect(0, 0, 100, 50)
			c.ClearRect(0, 0, c.Width, c.Height)
			c.FillStyle = green
			c.FillRect(0, 0, 10, 10)
		}, pxs(at(greenPx, 5, 5), at(none, 50, 25))},
		{"clearRect of more than everything, transformed (2d.clearRect.transform)", func(c *Canvas) {
			c.FillRect(0, 0, 100, 50)
			c.Scale(10, 10)
			c.Translate(0, 5)
			c.ClearRect(10, 0, -10, -5)
		}, at(none, 0, 0, 50, 25, 99, 49)},
		{"clearRect of everything keeps the path and the state", func(c *Canvas) {
			c.Rect(0, 0, 50, 50)
			c.FillStyle = green
			c.FillRect(0, 0, 100, 50)
			c.ClearRect(0, 0, 100, 50)
			c.Fill()
		}, pxs(at(greenPx, 25, 25), at(none, 75, 25))},

		// Paths.
		{"no path (2d.path.initial)", func(c *Canvas) {
			c.ClosePath()
			c.Fill()
			c.Stroke()
		}, at(none, 50, 25)},
		{"beginPath (2d.path.beginPath)", func(c *Canvas) {
			c.Rect(0, 0, 100, 50)
			c.BeginPath()
			c.Fill()
		}, at(none, 50, 25)},
		{"moveTo alone draws nothing (2d.path.moveTo.basic)", func(c *Canvas) {
			c.LineWidth, c.LineCap = 50, gfx.CapRound
			c.MoveTo(50, 25)
			c.Stroke()
		}, at(none, 50, 25)},
		{"moveTo twice (2d.path.moveTo.multiple)", func(c *Canvas) {
			c.MoveTo(0, 25)
			c.MoveTo(100, 25)
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			c.LineWidth = 50
			c.Stroke()
		}, at(blackPx, 1, 1, 50, 25, 98, 48)},
		{"moveTo NaN (2d.path.moveTo.nonfinite)", func(c *Canvas) {
			c.MoveTo(0, 0)
			c.LineTo(100, 0)
			c.MoveTo(math.NaN(), 50)
			c.MoveTo(0, math.Inf(1))
			c.LineTo(100, 50)
			c.LineTo(0, 50)
			c.Fill()
		}, at(blackPx, 50, 25, 1, 48, 98, 1)},
		{"lineTo starts a subpath (2d.path.lineTo.ensuresubpath.1)", func(c *Canvas) {
			c.LineWidth, c.LineCap = 50, gfx.CapRound
			c.LineTo(50, 25)
			c.Stroke()
		}, at(none, 50, 25)},
		{"lineTo after the first point (2d.path.lineTo.ensuresubpath.2)", func(c *Canvas) {
			c.LineWidth = 50
			c.LineTo(0, 25)
			c.LineTo(100, 25)
			c.Stroke()
		}, at(blackPx, 50, 25, 1, 1)},
		{"closePath draws a line (2d.path.closePath.newline)", func(c *Canvas) {
			c.LineWidth = 50
			c.MoveTo(-100, 25)
			c.LineTo(-100, -100)
			c.LineTo(200, -100)
			c.LineTo(200, 25)
			c.ClosePath()
			c.Stroke()
		}, at(blackPx, 50, 25, 1, 1, 98, 48)},
		{"the next subpath starts where the closed one did (2d.path.closePath.nextpoint)", func(c *Canvas) {
			c.LineWidth = 50
			c.MoveTo(-100, 25)
			c.LineTo(-100, -1000)
			c.ClosePath()
			c.LineTo(1000, 25)
			c.Stroke()
		}, at(blackPx, 50, 25, 1, 1, 98, 48)},
		{"rect is closed (2d.path.rect.closed)", func(c *Canvas) {
			c.StrokeStyle = green
			c.LineWidth, c.LineJoin = 200, gfx.JoinMiter
			c.Rect(100, 50, 100, 100)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},
		{"the next subpath starts at the rect's corner (2d.path.rect.end.1)", func(c *Canvas) {
			c.StrokeStyle = green
			c.LineWidth = 100
			c.Rect(200, 100, 400, 1000)
			c.LineTo(-2000, -1000)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},
		{"rect without height is a line (2d.path.rect.zero.1)", func(c *Canvas) {
			c.StrokeStyle = green
			c.LineWidth = 100
			c.Rect(0, 50, 100, 0)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},
		{"rect of no size (2d.path.rect.zero.3)", func(c *Canvas) {
			c.LineWidth, c.LineCap = 100, gfx.CapRound
			c.Rect(50, 25, 0, 0)
			c.Stroke()
		}, at(none, 50, 25)},
		{"rects turn where their sides say (2d.path.rect.winding)", func(c *Canvas) {
			c.Rect(0, 0, 50, 50)
			c.Rect(100, 50, -50, -50)
			c.Rect(0, 25, 100, -25)
			c.Rect(100, 25, -100, 25)
			c.Fill()
		}, at(none, 25, 12, 75, 12, 25, 37, 75, 37)},
		{"opposite windings cancel (2d.path.fill.winding.subtract.1)", func(c *Canvas) {
			poly(c, -10, -10, 110, -10, 110, 60, -10, 60, -10, -10)
			c.LineTo(0, 0)
			c.LineTo(0, 50)
			c.LineTo(100, 50)
			c.LineTo(100, 0)
			c.Fill()
		}, at(none, 50, 25)},
		{"even-odd hole", func(c *Canvas) {
			c.FillRule = gfx.EvenOdd
			c.Rect(0, 0, 100, 50)
			c.Rect(25, 10, 50, 30)
			c.Fill()
		}, pxs(at(blackPx, 10, 25, 50, 5), at(none, 50, 25))},
		{"fill closes open subpaths (2d.path.fill.closed.basic)", func(c *Canvas) {
			poly(c, 0, 0, 100, 0, 100, 50, 0, 50)
			c.Fill()
		}, at(blackPx, 50, 25, 1, 48)},
		{"fill does not close them for what follows (2d.path.fill.closed.unaffected)", func(c *Canvas) {
			poly(c, 0, 0, 100, 0, 100, 50)
			fill(c, red)
			c.LineTo(0, 50)
			fill(c, green)
		}, at(greenPx, 90, 10, 10, 40)},
		{"points keep the transform they were added with (2d.path.transformation.changing)", func(c *Canvas) {
			c.FillStyle = green
			c.MoveTo(0, 0)
			c.Translate(100, 0)
			c.LineTo(0, 0)
			c.Translate(0, 50)
			c.LineTo(0, 0)
			c.Translate(-100, 0)
			c.LineTo(0, 0)
			c.Translate(1000, 1000)
			c.Rotate(math.Pi / 2)
			c.Scale(0.1, 0.1)
			c.Fill()
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},

		// Arcs.
		{"arc without a path starts one (2d.path.arc.empty)", func(c *Canvas) {
			c.LineWidth = 50
			c.Arc(200, 25, 5, 0, 2*math.Pi, true)
			c.Stroke()
		}, at(none, 50, 25)},
		{"arc joins the path (2d.path.arc.nonempty)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(0, 25)
			c.Arc(200, 25, 5, 0, 2*math.Pi, true)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1)},
		{"arc ends where it says (2d.path.arc.end)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(-100, 0)
			c.Arc(-100, 0, 25, -math.Pi/2, math.Pi/2, true)
			c.LineTo(100, 25)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1)},
		{"arc of no angle, clockwise (2d.path.arc.zero.2)", func(c *Canvas) {
			c.LineWidth = 100
			c.Arc(50, 25, 50, 0, 0)
			c.Stroke()
		}, at(none, 50, 20)},
		{"arc of almost no angle (2d.path.arc.twopie.1)", func(c *Canvas) {
			c.LineWidth = 100
			c.Arc(50, 25, 50, 0, 2*math.Pi-1e-4, true)
			c.Stroke()
		}, at(none, 50, 20)},
		{"arc of a whole turn counterclockwise is the rest (2d.path.arc.twopie.2)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 100, green
			c.Arc(50, 25, 50, 0, 2*math.Pi-1e-4)
			c.Stroke()
		}, at(greenPx, 50, 20)},
		{"arc clockwise to a smaller angle (2d.path.arc.angle.5)", func(c *Canvas) {
			c.MoveTo(100, 0)
			c.Arc(100, 0, 150, (1024-1)*math.Pi, (512+0.5)*math.Pi)
			c.Fill()
		}, at(none, 50, 25, 1, 1, 98, 1)},
		{"arc counterclockwise to a larger angle (2d.path.arc.angle.3)", func(c *Canvas) {
			c.MoveTo(100, 0)
			c.Arc(100, 0, 150, (512+0.5)*math.Pi, (1024-1)*math.Pi, true)
			c.Fill()
		}, at(none, 50, 25, 1, 1, 98, 1)},
		{"arc clockwise, a whole turn and more (2d.path.arc.angle.4)", func(c *Canvas) {
			c.Arc(50, 25, 60, (512+0.5)*math.Pi, (1024-1)*math.Pi)
			c.Fill()
		}, at(blackPx, 50, 25, 1, 1, 98, 1, 1, 48, 98, 48)},
		{"arc counterclockwise (2d.path.arc.angle.2)", func(c *Canvas) {
			c.MoveTo(100, 0)
			c.Arc(100, 0, 150, -3*math.Pi/2, -math.Pi, true)
			c.Fill()
		}, at(none, 50, 25, 1, 1, 98, 1)},
		{"half a circle below (2d.path.arc.shape.1)", func(c *Canvas) {
			c.StrokeStyle, c.LineWidth = green, 50
			c.Arc(50, 50, 50, 0, math.Pi)
			c.Stroke()
		}, at(none, 50, 25, 1, 1, 98, 1)},
		{"half a circle above (2d.path.arc.shape.2)", func(c *Canvas) {
			c.StrokeStyle, c.LineWidth = green, 100
			c.Arc(50, 50, 50, 0, math.Pi, true)
			c.Stroke()
		}, at(greenPx, 50, 25, 20, 48, 98, 48)},
		{"a circle filled", func(c *Canvas) {
			c.Arc(50, 25, 20, 0, 2*math.Pi)
			c.Fill()
		}, pxs(at(blackPx, 50, 25, 50, 6, 68, 25, 31, 25, 63, 38), at(none, 50, 4, 66, 41, 29, 25))},
		{"a circle is a circle when its path is scaled", func(c *Canvas) {
			c.Translate(50, 25)
			c.Scale(40, 20)
			c.Arc(0, 0, 1, 0, 2*math.Pi)
			c.Fill()
		}, pxs(at(blackPx, 50, 25, 50, 6, 88, 25, 12, 25), at(none, 85, 40, 15, 8, 91, 25))},
		{"arc of radius 0 is a point (2d.path.arc.zeroradius)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(0, 25)
			c.Arc(200, 25, 0, 0, math.Pi, true)
			c.Stroke()
		}, at(greenPx, 50, 25)},
		{"arc of a negative radius adds nothing", func(c *Canvas) {
			c.LineWidth = 50
			c.MoveTo(0, 25)
			c.Arc(200, 25, -1, 0, math.Pi)
			c.Stroke()
		}, at(none, 50, 25)},
		{"arc NaN (2d.path.arc.nonfinite)", func(c *Canvas) {
			c.LineWidth = 50
			c.MoveTo(0, 25)
			c.Arc(math.NaN(), 25, 50, 0, 2*math.Pi)
			c.Arc(200, 25, 50, 0, math.Inf(1))
			c.Stroke()
		}, at(none, 50, 25)},

		// Lines.
		{"the line of the transform of the stroke (2d.path.stroke.scale1)", func(c *Canvas) {
			c.Rect(25, 12.5, 50, 25)
			c.Save()
			c.Scale(50, 25)
			c.StrokeStyle = green
			c.Stroke()
			c.Restore()
			c.BeginPath()
			c.Rect(-25, -12.5, 150, 75)
			c.Save()
			c.Scale(50, 25)
			c.StrokeStyle = red
			c.Stroke()
			c.Restore()
		}, at(greenPx, 1, 1, 50, 25, 98, 48)},
		{"the line of a skewed transform (2d.path.stroke.skew)", func(c *Canvas) {
			c.Translate(50, 25)
			c.Transform(1, 0, 2, 1, 0, 0)
			c.LineWidth = 4
			c.MoveTo(0, -10)
			c.LineTo(0, 10)
			c.Stroke()
		}, pxs(at(blackPx, 50, 25, 66, 33, 34, 17), at(none, 50, 12, 60, 18))},
		{"the line of a turned and evenly scaled transform", func(c *Canvas) {
			c.Translate(50, 25)
			c.Rotate(math.Pi / 2)
			c.Scale(2, -2)
			c.LineWidth = 5
			c.MoveTo(-10, 0)
			c.LineTo(10, 0)
			c.Stroke()
		}, pxs(at(blackPx, 50, 25, 46, 25, 54, 25, 50, 7), at(none, 43, 25, 57, 25, 50, 3))},
		{"a transform that collapses the line", func(c *Canvas) {
			c.Rect(10, 10, 50, 20)
			c.Scale(0, 1)
			c.LineWidth = 20
			c.Stroke()
		}, at(none, 10, 20, 50, 25)},
		{"stroke leaves out segments of no length (2d.path.stroke.prune.line)", func(c *Canvas) {
			c.LineWidth, c.LineCap, c.LineJoin = 100, gfx.CapRound, gfx.JoinRound
			c.MoveTo(50, 25)
			c.LineTo(50, 25)
			c.Stroke()
		}, at(none, 50, 25)},
		{"and closed ones (2d.path.stroke.prune.closed)", func(c *Canvas) {
			c.LineWidth, c.LineCap, c.LineJoin = 100, gfx.CapRound, gfx.JoinRound
			c.MoveTo(50, 25)
			c.LineTo(50, 25)
			c.ClosePath()
			c.Stroke()
		}, at(none, 50, 25)},
		{"and arcs (2d.path.stroke.prune.arc)", func(c *Canvas) {
			c.LineWidth, c.LineCap, c.LineJoin = 100, gfx.CapRound, gfx.JoinRound
			c.MoveTo(50, 25)
			c.Arc(50, 25, 0, 0, 2*math.Pi)
			c.Stroke()
		}, at(none, 50, 25)},
		{"and at corners (2d.path.stroke.prune.corner)", func(c *Canvas) {
			c.LineWidth, c.MiterLimit = 400, 1.4
			c.MoveTo(-1000, 200)
			c.LineTo(-100, 200)
			c.LineTo(-100, 200)
			c.LineTo(-100, 200)
			c.LineTo(-100, 1000)
			c.Stroke()
		}, at(none, 50, 25)},
		{"lineWidth of nothing", func(c *Canvas) {
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			for _, w := range []float64{0, -5, math.NaN(), math.Inf(1)} {
				c.LineWidth = w
				c.Stroke()
			}
		}, at(none, 50, 25)},
		{"butt", func(c *Canvas) {
			c.LineWidth = 20
			c.MoveTo(30, 25)
			c.LineTo(70, 25)
			c.Stroke()
		}, pxs(at(blackPx, 31, 25, 31, 16), at(none, 28, 25))},
		{"round", func(c *Canvas) {
			c.LineWidth, c.LineCap = 20, gfx.CapRound
			c.MoveTo(30, 25)
			c.LineTo(70, 25)
			c.Stroke()
		}, pxs(at(blackPx, 22, 25, 78, 25), at(none, 21, 16, 18, 25))},
		{"square", func(c *Canvas) {
			c.LineWidth, c.LineCap = 20, gfx.CapSquare
			c.MoveTo(30, 25)
			c.LineTo(70, 25)
			c.Stroke()
		}, pxs(at(blackPx, 21, 16, 78, 33), at(none, 18, 25))},
		{"a miter within the limit", func(c *Canvas) {
			c.LineWidth = 20
			c.MoveTo(20, 80)
			c.LineTo(50, 20)
			c.LineTo(80, 80)
			c.Stroke()
		}, at(blackPx, 50, 4)},
		{"a miter over the limit", func(c *Canvas) {
			c.LineWidth, c.MiterLimit = 20, 2
			c.MoveTo(20, 80)
			c.LineTo(50, 20)
			c.LineTo(80, 80)
			c.Stroke()
		}, at(none, 50, 4)},
		{"a limit that is no limit is the first one", func(c *Canvas) {
			c.LineWidth, c.MiterLimit = 20, math.NaN()
			c.MoveTo(20, 80)
			c.LineTo(50, 20)
			c.LineTo(80, 80)
			c.Stroke()
		}, at(blackPx, 50, 4)},

		// Clips.
		{"an empty clip hides everything (2d.path.clip.empty)", func(c *Canvas) {
			c.BeginPath()
			c.Clip()
			c.FillRect(0, 0, 100, 50)
		}, at(none, 50, 25)},
		{"clips meet (2d.path.clip.intersect)", func(c *Canvas) {
			c.Rect(0, 0, 50, 50)
			c.Clip()
			c.BeginPath()
			c.Rect(25, 0, 50, 50)
			c.Clip()
			c.FillRect(0, 0, 100, 50)
		}, pxs(at(blackPx, 37, 25), at(none, 12, 25, 62, 25))},
		{"an even-odd clip", func(c *Canvas) {
			c.Rect(0, 0, 100, 50)
			c.Rect(25, 10, 50, 30)
			c.FillRule = gfx.EvenOdd
			c.Clip()
			c.FillRect(0, 0, 100, 50)
		}, pxs(at(blackPx, 10, 25), at(none, 50, 25))},
		{"restore ends the clip (2d.state.saverestore.clip)", func(c *Canvas) {
			c.Save()
			c.Rect(0, 0, 1, 1)
			c.Clip()
			c.Restore()
			c.FillStyle = green
			c.FillRect(0, 0, 100, 50)
		}, at(greenPx, 50, 25)},

		// State.
		{"save and restore (2d.state.saverestore.transformation)", func(c *Canvas) {
			c.FillStyle = green
			c.Save()
			c.Translate(200, 0)
			c.FillStyle = red
			c.Restore()
			c.FillRect(0, 0, 100, 50)
		}, at(greenPx, 50, 25)},
		{"restore without save (2d.state.saverestore.underflow)", func(c *Canvas) {
			for range 16 {
				c.Restore()
			}
			c.FillStyle, c.LineWidth = green, 20
			c.Restore()
			c.FillRect(0, 0, 100, 50)
		}, at(greenPx, 50, 25)},
		{"the path is no state (2d.state.saverestore.path)", func(c *Canvas) {
			c.Save()
			c.Rect(0, 0, 100, 50)
			c.Restore()
			c.Fill()
		}, at(blackPx, 50, 25)},
		{"transforms in order (2d.transformation.order)", func(c *Canvas) {
			c.Scale(2, 1)
			c.Rotate(math.Pi / 2)
			c.FillRect(0, -50, 50, 50)
		}, at(blackPx, 1, 1, 50, 25, 98, 48)},
		{"transform multiplies (2d.transformation.transform.multiply)", func(c *Canvas) {
			c.Transform(1, 2, 3, 4, 5, 6)
			c.Transform(-2, 1, 1.5, -0.5, 1, -2)
			c.FillRect(0, 0, 100, 50)
		}, at(blackPx, 50, 25, 1, 1, 98, 48)},
		{"setTransform replaces (2d.transformation.setTransform.multiple)", func(c *Canvas) {
			c.SetTransform(1.0/2, 0, 0, 1.0/2, 0, 0)
			c.SetTransform(2, 0, 0, 2, 0, 0)
			c.FillRect(0, 0, 50, 25)
		}, at(blackPx, 50, 25, 98, 48)},
		{"resetTransform", func(c *Canvas) {
			c.Scale(0, 0)
			c.ResetTransform()
			c.FillRect(0, 0, 100, 50)
		}, at(blackPx, 50, 25)},
		{"transforms NaN (2d.transformation.rotate.nonfinite)", func(c *Canvas) {
			c.Rotate(math.NaN())
			c.Scale(math.Inf(1), 1)
			c.Translate(1, math.NaN())
			c.Transform(1, 0, 0, 1, math.Inf(-1), 0)
			c.SetTransform(math.NaN(), 0, 0, 1, 0, 0)
			c.FillRect(0, 0, 100, 50)
		}, at(blackPx, 50, 25, 1, 1)},

		// Paints.
		{"globalAlpha (2d.composite.globalAlpha.fill)", func(c *Canvas) {
			c.GlobalAlpha, c.FillStyle = 0.5, red
			c.FillRect(0, 0, 100, 50)
		}, at(halfRedPx, 50, 25)},
		{"globalAlpha outside 0 to 1", func(c *Canvas) {
			c.GlobalAlpha, c.FillStyle = 2, green
			c.FillRect(0, 0, 50, 50)
			c.FillStyle = red
			for _, a := range []float64{0, -1, math.NaN()} {
				c.GlobalAlpha = a
				c.FillRect(0, 0, 100, 50)
			}
		}, pxs(at(greenPx, 25, 25), at(none, 75, 25))},
		{"no style", func(c *Canvas) {
			c.Rect(10, 10, 50, 20)
			c.FillStyle, c.StrokeStyle = nil, gfx.Transparent
			c.LineWidth = 20
			c.Fill()
			c.Stroke()
			c.FillRect(0, 0, 100, 50)
		}, at(none, 50, 25, 10, 20)},
		{"two styles in turns", func(c *Canvas) {
			c.FillStyle, c.StrokeStyle = red, blue
			c.LineWidth = 10
			for i := range 2 {
				c.BeginPath()
				c.Rect(10+float64(i)*50, 10, 30, 30)
				c.Fill()
				c.Stroke()
			}
		}, pxs(at(redPx, 25, 25, 75, 25), at(bluePx, 10, 25, 60, 25))},
		{"outside the canvas", func(c *Canvas) {
			c.FillRect(-100, -100, 1000, 1000)
		}, at(blackPx, 0, 0, 99, 49)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := render(100, 50, 1, tt.draw)
			for _, p := range tt.want {
				if got := img.RGBAAt(p.x, p.y); !near(got, p.want) {
					t.Errorf("pixel %d,%d is %v, want %v", p.x, p.y, got, p.want)
				}
			}
		})
	}
}

// A draw that leaves Save, clips and transforms open leaves nothing in the
// list or in the recorder.
func TestLeftOpen(t *testing.T) {
	var r Recorder
	for range 3 {
		list, _ := r.Record(frameOf(100, 50), func(c *Canvas) {
			c.Save()
			c.Translate(10, 0)
			c.Rect(0, 0, 50, 50)
			c.Clip()
			c.Save()
			c.Scale(2, 2)
			c.FillStyle = red
			c.FillRect(0, 0, 10, 10)
		})
		if len(r.states) != 0 || r.clips != 0 || r.b.Depth() != 0 {
			t.Fatalf("the recorder keeps %d states, %d clips, %d blocks", len(r.states), r.clips, r.b.Depth())
		}
		if got := list.Len(); got != 5 { // the canvas's clip, a clip, a rectangle, and the ends
			t.Fatalf("%d commands:\n%s", got, list)
		}
	}
	list, _ := r.Record(frameOf(100, 50), func(c *Canvas) { c.FillRect(0, 0, 10, 10) })
	if got := list.At(1).(gfx.FillRect); got.Rect != (gfx.Rect{X1: 10, Y1: 10}) || got.Paint != gfx.Paint(black) {
		t.Errorf("the next draw starts from %v", got)
	}
}

// What a draw starts from.
func TestInitialState(t *testing.T) {
	var r Recorder
	f := &Frame{Width: 40, Height: 30, Time: 5 * time.Second, Delta: time.Second, Fill: red, Stroke: blue, LineWidth: 3}
	list, again := r.Record(f, func(c *Canvas) {
		if c.Width != 40 || c.Height != 30 || c.Time != 5*time.Second || c.Delta != time.Second {
			t.Errorf("%gx%g, time %v, delta %v", c.Width, c.Height, c.Time, c.Delta)
		}
		if c.FillStyle != Style(red) || c.StrokeStyle != Style(blue) || c.LineWidth != 3 || c.MiterLimit != 10 || c.GlobalAlpha != 1 {
			t.Errorf("fill %v, stroke %v, width %g, miter limit %g, alpha %g", c.FillStyle, c.StrokeStyle, c.LineWidth, c.MiterLimit, c.GlobalAlpha)
		}
		if c.LineCap != gfx.CapButt || c.LineJoin != gfx.JoinMiter || c.FillRule != gfx.NonZero {
			t.Errorf("cap %v, join %v, rule %v", c.LineCap, c.LineJoin, c.FillRule)
		}
	})
	if list.Len() != 0 || again {
		t.Errorf("a draw that draws nothing recorded %d commands, again %v", list.Len(), again)
	}
	if _, again = r.Record(f, func(c *Canvas) { c.RequestFrame() }); !again {
		t.Error("RequestFrame is not reported")
	}
	if _, again = r.Record(f, func(*Canvas) {}); again {
		t.Error("the request of the draw before is")
	}
}

// The colors of the initial state keep their interface values: a draw of
// many commands makes one paint, not one per command.
func TestPaintsOnce(t *testing.T) {
	violet := gfx.OKLCH(0.606, 0.25, 292.717, 1) // out of gamut
	list := record(100, 50, func(c *Canvas) {
		c.FillStyle = violet
		for i := range 10 {
			c.FillRect(float64(i), 0, 1, 1)
		}
		c.GlobalAlpha = 0.5
		for i := range 10 {
			c.FillRect(float64(i), 5, 1, 1)
		}
	})
	for i := 1; i < list.Len()-1; i++ {
		got := list.At(i).(gfx.FillRect).Paint.(gfx.Color)
		want := violet.Clamp()
		if i > 10 {
			want.A = 0.5
		}
		if got != want {
			t.Errorf("command %d paints %v, want %v", i, got, want)
		}
	}
}

// A panic of the function goes on, and leaves the recorder ready.
func TestPanic(t *testing.T) {
	var r Recorder
	var kept *Canvas
	func() {
		defer func() {
			if v := recover(); v != "boom" {
				t.Errorf("recovered %v", v)
			}
		}()
		r.Record(frameOf(100, 50), func(c *Canvas) {
			kept = c
			c.Save()
			c.Rect(0, 0, 10, 10)
			c.Clip()
			c.FillRect(0, 0, 10, 10)
			c.RequestFrame()
			panic("boom")
		})
	}()
	expectPanic(t, "after the draw function returned", func() { kept.Fill() })
	list, again := r.Record(frameOf(100, 50), func(c *Canvas) { c.FillRect(0, 0, 10, 10) })
	if list.Len() != 3 || again || r.b.Depth() != 0 {
		t.Errorf("the draw after a panic: %d commands, again %v, depth %d", list.Len(), again, r.b.Depth())
	}
}

func expectPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		switch v := recover(); {
		case v == nil:
			t.Errorf("no panic; want one with %q", want)
		case !strings.Contains(fmt.Sprint(v), want):
			t.Errorf("panic %v; want one with %q", v, want)
		}
	}()
	f()
}

// methods are the methods of Canvas that draw or read.
var methods = map[string]func(c *Canvas){
	"Save": (*Canvas).Save, "Restore": (*Canvas).Restore,
	"Translate": func(c *Canvas) { c.Translate(1, 1) }, "Rotate": func(c *Canvas) { c.Rotate(1) },
	"Scale": func(c *Canvas) { c.Scale(2, 2) }, "Transform": func(c *Canvas) { c.Transform(1, 0, 0, 1, 0, 0) },
	"SetTransform": func(c *Canvas) { c.SetTransform(1, 0, 0, 1, 0, 0) }, "ResetTransform": (*Canvas).ResetTransform,
	"RequestFrame": (*Canvas).RequestFrame, "Color": func(c *Canvas) { c.Color("primary") },
	"BeginPath": (*Canvas).BeginPath, "MoveTo": func(c *Canvas) { c.MoveTo(1, 1) }, "LineTo": func(c *Canvas) { c.LineTo(1, 1) },
	"ClosePath": (*Canvas).ClosePath, "Rect": func(c *Canvas) { c.Rect(0, 0, 1, 1) },
	"Arc": func(c *Canvas) { c.Arc(0, 0, 1, 0, 1) }, "Fill": (*Canvas).Fill, "Stroke": (*Canvas).Stroke, "Clip": (*Canvas).Clip,
	"FillRect": func(c *Canvas) { c.FillRect(0, 0, 1, 1) }, "StrokeRect": func(c *Canvas) { c.StrokeRect(0, 0, 1, 1) },
	"ClearRect": func(c *Canvas) { c.ClearRect(0, 0, 1, 1) },
}

// A Canvas kept after its draw panics in dev mode, and does nothing
// otherwise, also from another goroutine while a later draw runs.
func TestKept(t *testing.T) {
	var r Recorder
	var dev, release *Canvas
	r.Record(frameOf(100, 50), func(c *Canvas) { dev = c })
	f := frameOf(100, 50)
	f.Dev = false
	r.Record(f, func(c *Canvas) { release = c })
	for name, m := range methods {
		expectPanic(t, "Canvas."+name+" called after", func() { m(dev) })
		m(release)
		m(&Canvas{}) // the zero Canvas
	}
	want := record(100, 50, func(c *Canvas) { c.FillRect(0, 0, 10, 10) })
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, m := range methods {
				m(release)
			}
		}
	})
	for range 200 {
		list, again := r.Record(f, func(c *Canvas) { c.FillRect(0, 0, 10, 10) })
		if list.Len() != want.Len() || again {
			t.Fatalf("a kept Canvas drew in a later draw:\n%s", list)
		}
	}
	close(stop)
	wg.Wait()
}

// Only colors are styles; a type outside gfx is one by embedding, and cannot
// be painted with.
func TestStyle(t *testing.T) {
	type embedded struct{ Style }
	expectPanic(t, "cannot paint with a canvas.embedded", func() {
		record(100, 50, func(c *Canvas) {
			c.FillStyle = embedded{}
			c.FillRect(0, 0, 1, 1)
		})
	})
}

// Arcs are close to the circle at every size, and whole ones end where they
// start.
func TestArcAccuracy(t *testing.T) {
	for _, radius := range []float64{0.5, 3, 20, 75, 400, 3000, 1e5} {
		for _, k := range []float64{1, 7} {
			var r Recorder
			var path gfx.Path
			r.Record(frameOf(100, 50), func(c *Canvas) {
				c.Scale(k, k)
				c.Arc(0, 0, radius/k, 0.3, 0.3+2*math.Pi)
				path = r.path.Clone()
			})
			pts := path.Points()
			if pts[0] != pts[len(pts)-1] {
				t.Errorf("radius %g: starts at %v, ends at %v", radius, pts[0], pts[len(pts)-1])
			}
			worst := 0.0
			for s := range path.Segments() {
				if s.Verb != gfx.CubicTo {
					continue
				}
				for i := 1; i < 16; i++ {
					u := float64(i) / 16
					b := func(a, b, c, d float32) float64 {
						v := 1 - u
						return v*v*v*float64(a) + 3*v*v*u*float64(b) + 3*v*u*u*float64(c) + u*u*u*float64(d)
					}
					x, y := b(s.P[0].X, s.P[1].X, s.P[2].X, s.P[3].X), b(s.P[0].Y, s.P[1].Y, s.P[2].Y, s.P[3].Y)
					worst = max(worst, math.Abs(math.Hypot(x, y)-radius))
				}
			}
			if limit := arcTolerance*1.1 + radius*1e-6; worst > limit {
				t.Errorf("radius %g at scale %g: %g from the circle, want at most %g", radius, k, worst, limit)
			}
			if n := path.Len(); radius <= 10 && n > 5 {
				t.Errorf("radius %g takes %d verbs", radius, n)
			}
		}
	}
}

func TestMatrix(t *testing.T) {
	m := rotation(0.7)
	for range 100 {
		m = m.mul(rotation(0.3))
		m = m.mul(matrix{a: 1.1, d: 1.1, e: 3, f: -2})
	}
	if k, ok := m.similar(); !ok || math.Abs(k/math.Pow(1.1, 100)-1) > 1e-9 {
		t.Errorf("turns and even scales: similar %v, scale %g", ok, k)
	}
	for _, n := range []matrix{{a: 2, d: 1}, {a: 1, c: 0.5, d: 1}, {a: 1, b: 1e-6, d: 1}} {
		if _, ok := n.similar(); ok {
			t.Errorf("%+v is not similar", n)
		}
	}
	if k, ok := (&matrix{b: 3, c: 3}).similar(); !ok || k != 3 {
		t.Errorf("a mirror: %v, %v", k, ok)
	}
	inv, ok := m.invert()
	if !ok {
		t.Fatal("not invertible")
	}
	x, y := m.at(12, -7)
	if x, y = inv.at(x, y); math.Abs(x-12) > 1e-9 || math.Abs(y+7) > 1e-9 {
		t.Errorf("there and back: %g, %g", x, y)
	}
	for _, n := range []matrix{{}, {a: 1, c: 1}, {a: 1e-200, d: 1e-200}, {a: math.Inf(1), d: 1}, {a: math.NaN(), d: 1}} {
		if _, ok := n.invert(); ok {
			t.Errorf("%+v has no inverse", n)
		}
	}
}

// particles draws a scene of 2000 commands.
func particles(c *Canvas) {
	for i := range 500 {
		x := float64(i*37%400) + c.Time.Seconds()
		y := float64(i * 53 % 300)
		c.FillRect(x, y, 3, 3)
		c.BeginPath()
		c.Arc(x, y, 4, 0, 2*math.Pi)
		c.Fill()
		c.BeginPath()
		c.MoveTo(x, y)
		c.LineTo(x+10, y+10)
		c.Stroke()
		c.StrokeRect(x, y, 6, 6)
	}
}

func BenchmarkRecord2000(b *testing.B) {
	var r Recorder
	f := frameOf(400, 300)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		f.Time = time.Duration(i) * time.Millisecond
		if list, _ := r.Record(f, particles); list.Len() < 2000 {
			b.Fatalf("%d commands", list.Len())
		}
	}
}

// A draw costs its list: nothing that grows with the number of draws.
func TestRecordAllocations(t *testing.T) {
	var r Recorder
	f := frameOf(400, 300)
	r.Record(f, particles)
	got := testing.AllocsPerRun(5, func() { r.Record(f, particles) })
	// A command each, and the two slices of the path of three of four.
	if limit := 2000.0 + 1500*2 + 32; got > limit {
		t.Errorf("%g allocations for 2000 commands, want at most %g", got, limit)
	}
	if got := testing.AllocsPerRun(5, func() { r.Record(f, func(*Canvas) {}) }); got > 3 {
		t.Errorf("%g allocations for an empty draw", got)
	}
}
