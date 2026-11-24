package canvas

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/svg"
	"github.com/kuronosu/tela/internal/text"
)

var (
	nan = math.NaN()
	inf = math.Inf(1)
)

// solid is an image of one color.
func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// halves is an image of two pixels side by side, or one above the other.
func halves(a, b color.RGBA, vertical bool) *image.RGBA {
	r, at := image.Rect(0, 0, 2, 1), image.Pt(1, 0)
	if vertical {
		r, at = image.Rect(0, 0, 1, 2), image.Pt(0, 1)
	}
	img := image.NewRGBA(r)
	img.SetRGBA(0, 0, a)
	img.SetRGBA(at.X, at.Y, b)
	return img
}

// TestMethods draws on canvases of 100 by 50 and looks at pixels, as
// TestDrawing (canvas_test.go) does. The names in parentheses are the web
// platform tests of the same behavior.
func TestMethods(t *testing.T) {
	whole := func(c *Canvas, s Style) { c.FillStyle = s; c.FillRect(0, 0, 100, 50) }
	gradient := func(g *CanvasGradient, stops ...any) *CanvasGradient {
		for i := 0; i+1 < len(stops); i += 2 {
			o, ok := stops[i].(float64)
			if !ok {
				o = float64(stops[i].(int))
			}
			g.AddColorStop(o, stops[i+1].(gfx.Color))
		}
		return g
	}
	tests := []struct {
		name string
		draw func(c *Canvas)
		want []px
	}{
		// Curves.
		{"quadraticCurveTo without a path starts at the control point (2d.path.quadraticCurveTo.ensuresubpath.2)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.QuadraticCurveTo(0, 25, 100, 25)
			c.Stroke()
		}, at(greenPx, 50, 25, 2, 2)},
		{"quadraticCurveTo (2d.path.quadraticCurveTo.shape)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 55, green
			c.MoveTo(-1000, 1050)
			c.QuadraticCurveTo(0, -1000, 1200, 1050)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 1, 1, 48)},
		{"quadraticCurveTo NaN (2d.path.quadraticCurveTo.nonfinite)", func(c *Canvas) {
			c.MoveTo(0, 0)
			c.LineTo(100, 0)
			c.QuadraticCurveTo(nan, 50, 0, 50)
			c.QuadraticCurveTo(0, 50, 0, inf)
			c.LineTo(100, 50)
			c.LineTo(0, 50)
			c.Fill()
		}, at(blackPx, 50, 25, 98, 2, 2, 48)},
		{"bezierCurveTo without a path starts at the control point (2d.path.bezierCurveTo.ensuresubpath.2)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.BezierCurveTo(0, 25, 100, 25, 100, 25)
			c.Stroke()
		}, at(greenPx, 50, 25, 2, 2)},
		{"bezierCurveTo (2d.path.bezierCurveTo.shape)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 55, green
			c.MoveTo(-2000, 3100)
			c.BezierCurveTo(-2000, -1000, 2100, -1000, 2100, 3100)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 1, 1, 48)},
		{"a curve of no length (2d.path.stroke.prune.curve)", func(c *Canvas) {
			c.LineWidth, c.LineCap, c.LineJoin = 100, gfx.CapRound, gfx.JoinRound
			c.MoveTo(50, 25)
			c.QuadraticCurveTo(50, 25, 50, 25)
			c.BezierCurveTo(50, 25, 50, 25, 50, 25)
			c.Stroke()
		}, at(none, 50, 25)},

		// ArcTo.
		{"arcTo without a path (2d.path.arcTo.ensuresubpath.2)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.ArcTo(0, 25, 50, 250, 0.1)
			c.LineTo(100, 25)
			c.Stroke()
		}, at(greenPx, 50, 25, 2, 25)},
		{"arcTo of coinciding points (2d.path.arcTo.coincide.1)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(0, 25)
			c.ArcTo(0, 25, 50, 1000, 1)
			c.LineTo(100, 25)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 1)},
		{"arcTo of points on a line (2d.path.arcTo.collinear.1)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(0, 25)
			c.ArcTo(100, 25, 200, 25, 1)
			c.Stroke()
			c.StrokeStyle = red
			c.BeginPath()
			c.MoveTo(-100, 25)
			c.ArcTo(0, 25, 100, 25, 1)
			c.Stroke()
		}, at(greenPx, 50, 25)},
		{"arcTo back on the line (2d.path.arcTo.collinear.2)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(0, 25)
			c.ArcTo(100, 25, 10, 25, 1)
			c.Stroke()
		}, at(greenPx, 50, 25)},
		{"arcTo curves (2d.path.arcTo.shape.curve1)", func(c *Canvas) {
			tol := 1.5
			c.LineWidth, c.StrokeStyle = 10, green
			c.MoveTo(10, 25)
			c.ArcTo(75, 25, 75, 60, 20)
			c.Stroke()
			c.FillStyle = green
			c.BeginPath()
			c.Rect(10, 20, 45, 10)
			c.MoveTo(80, 45)
			c.Arc(55, 45, 25+tol, 0, -math.Pi/2, true)
			c.Arc(55, 45, 15-tol, -math.Pi/2, 0)
			c.Fill()
		}, pxs(at(greenPx, 50, 25, 55, 19, 55, 20, 65, 21, 72, 28, 73, 36, 78, 36), at(none, 80, 22, 64, 40))},
		{"arcTo from the end of the line (2d.path.arcTo.shape.start)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.MoveTo(0, 25)
			c.ArcTo(200, 25, 200, 50, 10)
			c.Stroke()
		}, at(greenPx, 1, 1, 50, 25, 98, 48)},
		{"arcTo of a negative radius adds nothing", func(c *Canvas) {
			c.LineWidth = 50
			c.MoveTo(0, 25)
			c.ArcTo(100, 25, 100, 100, -1)
			c.Stroke()
		}, at(none, 50, 25)},
		{"arcTo under a transform (2d.path.arcTo.transformation)", func(c *Canvas) {
			c.LineWidth, c.StrokeStyle = 50, green
			c.FillStyle = green
			c.BeginPath()
			c.MoveTo(0, 50)
			c.Translate(100, 0)
			c.ArcTo(50, 50, 50, 0, 50)
			c.LineTo(-100, 0)
			c.Fill()
		}, at(greenPx, 2, 2, 50, 25, 98, 48)},

		// Ellipses.
		{"an ellipse", func(c *Canvas) {
			c.Ellipse(50, 25, 40, 20, 0, 0, 2*math.Pi)
			c.Fill()
		}, pxs(at(blackPx, 50, 25, 12, 25, 88, 25, 50, 6), at(none, 8, 25, 50, 3, 15, 8))},
		{"an ellipse turned", func(c *Canvas) {
			c.Ellipse(50, 25, 40, 10, math.Pi/2, 0, 2*math.Pi)
			c.Fill()
		}, pxs(at(blackPx, 50, 1, 50, 48, 58, 25), at(none, 62, 25, 35, 25))},
		{"half an ellipse counterclockwise", func(c *Canvas) {
			c.Ellipse(50, 25, 40, 20, 0, 0, math.Pi, true)
			c.Fill()
		}, pxs(at(blackPx, 50, 10), at(none, 50, 40))},
		{"an ellipse of negative radius adds nothing", func(c *Canvas) {
			c.LineWidth = 50
			c.MoveTo(0, 25)
			c.Ellipse(100, 25, -1, 5, 0, 0, 1)
			c.Ellipse(100, 25, 1, -5, 0, 0, 1)
			c.Stroke()
		}, at(none, 50, 25)},

		// Rounded rectangles.
		{"roundRect (2d.path.roundrect.1.radius.double)", func(c *Canvas) {
			c.RoundRect(0, 0, 100, 50, 20)
			c.Fill()
		}, pxs(at(none, 1, 1, 98, 1, 98, 48, 1, 48), at(blackPx, 20, 1, 1, 20, 50, 25, 79, 48, 98, 30))},
		{"roundRect of two radii (2d.path.roundrect.2.radii.1.double)", func(c *Canvas) {
			c.RoundRect(0, 0, 100, 50, 20, 0)
			c.Fill()
		}, pxs(at(none, 1, 1, 98, 48), at(blackPx, 98, 1, 1, 48, 50, 25))},
		{"roundRect of three radii (2d.path.roundrect.3.radii.3.double)", func(c *Canvas) {
			c.RoundRect(0, 0, 100, 50, 0, 0, 20)
			c.Fill()
		}, pxs(at(none, 98, 48), at(blackPx, 1, 1, 98, 1, 1, 48))},
		{"roundRect of four radii (2d.path.roundrect.4.radii.4.double)", func(c *Canvas) {
			c.RoundRect(0, 0, 100, 50, 0, 0, 0, 20)
			c.Fill()
		}, pxs(at(none, 1, 48), at(blackPx, 1, 1, 98, 1, 98, 48))},
		{"roundRect without radii", func(c *Canvas) {
			c.RoundRect(0, 0, 100, 50)
			c.Fill()
		}, at(blackPx, 1, 1, 98, 48)},
		{"roundRect of radii too large (2d.path.roundrect.radius.intersecting.1)", func(c *Canvas) {
			c.FillStyle = green
			c.RoundRect(0, 0, 100, 50, 40, 40, 40, 40)
			c.Fill()
		}, pxs(at(greenPx, 2, 25, 50, 1, 50, 25, 50, 48, 97, 25), at(none, 1, 1, 98, 1, 1, 48, 98, 48))},
		{"roundRect to the left and up (2d.path.roundrect.negative)", func(c *Canvas) {
			c.RoundRect(100, 50, -50, -25, 10, 0, 0, 0)
			c.Fill()
		}, pxs(at(none, 98, 48, 25, 12), at(blackPx, 52, 27, 98, 27, 52, 48, 75, 37))},
		{"roundRect of a negative or NaN radius adds nothing (2d.path.roundrect.badinput)", func(c *Canvas) {
			c.RoundRect(0, 0, 100, 50, -1)
			c.RoundRect(0, 0, 100, 50, 1, nan)
			c.RoundRect(0, 0, nan, 50, 1)
			c.Fill()
		}, at(none, 50, 25)},
		{"roundRect is closed, and goes on at its corner (2d.path.roundrect.end.1)", func(c *Canvas) {
			c.StrokeStyle, c.LineWidth = green, 100
			c.RoundRect(200, 100, 400, 1000, 0)
			c.LineTo(-2000, -1000)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},
		{"roundRect goes on at its corner (2d.path.roundrect.end.4)", func(c *Canvas) {
			c.StrokeStyle, c.LineWidth = green, 10
			c.RoundRect(-50, -25, 20, 20, 8)
			c.LineTo(100, 50)
			c.Stroke()
		}, at(greenPx, 50, 25)},
		{"roundRect without height is a line (2d.path.roundrect.zero.1)", func(c *Canvas) {
			c.StrokeStyle, c.LineWidth = green, 100
			c.RoundRect(0, 50, 100, 0, 5)
			c.Stroke()
		}, at(greenPx, 50, 25, 1, 1, 98, 48)},
		{"roundRects turn where their sides say (2d.path.roundrect.winding)", func(c *Canvas) {
			c.RoundRect(0, 0, 50, 50, 4)
			c.RoundRect(100, 50, -50, -50, 4)
			c.RoundRect(0, 25, 100, -25, 4)
			c.RoundRect(100, 25, -100, 25, 4)
			c.Fill()
		}, at(none, 25, 12, 75, 12, 25, 37, 75, 37)},

		// Path2D.
		{"fillPath follows the transform of now", func(c *Canvas) {
			var p Path2D
			p.Rect(0, 0, 10, 10)
			c.Rect(0, 0, 100, 50)
			c.Translate(50, 20)
			c.Scale(2, 2)
			c.FillPath(&p)
		}, pxs(at(blackPx, 51, 21, 68, 38), at(none, 48, 25, 25, 25, 72, 25))},
		{"strokePath", func(c *Canvas) {
			p := MustPath("M0 0 H10 V10")
			c.Scale(5, 2)
			c.LineWidth = 2
			c.StrokePath(p)
		}, pxs(at(blackPx, 25, 1, 47, 15, 53, 15), at(none, 25, 10))},
		{"clipPath", func(c *Canvas) {
			p := MustPath("M0 0 H50 V50 H0 Z M10 10 H40 V40 H10 Z")
			c.FillRule = gfx.EvenOdd
			c.ClipPath(p)
			c.FillRect(0, 0, 100, 50)
		}, pxs(at(blackPx, 5, 25, 45, 25), at(none, 25, 25, 75, 25))},
		{"addPath starts a subpath where the added one ends", func(c *Canvas) {
			var p Path2D
			p.AddPath(MustPath("M0 0 L50 0"))
			p.LineTo(50, 50)
			p.LineTo(0, 50)
			c.FillPath(&p)
		}, pxs(at(blackPx, 40, 30), at(none, 10, 10))},
		{"addPath of itself", func(c *Canvas) {
			var p Path2D
			p.Rect(0, 0, 50, 50)
			p.AddPath(&p)
			c.FillRule = gfx.EvenOdd
			c.FillPath(&p)
		}, at(none, 25, 25)},
		{"a path's arcs stay round when they are scaled", func(c *Canvas) {
			var p Path2D
			p.Arc(0, 0, 0.5, 0, 2*math.Pi)
			c.Translate(50, 25)
			c.Scale(40, 40)
			c.FillPath(&p)
		}, pxs(at(blackPx, 50, 6, 63, 11, 68, 25), at(none, 66, 10, 50, 3))},

		// Dashes.
		{"a dash and a gap", func(c *Canvas) {
			c.LineWidth = 20
			c.SetLineDash(10, 20)
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			c.Stroke()
		}, pxs(at(blackPx, 5, 25, 35, 25, 65, 25, 95, 25), at(none, 20, 25, 50, 25, 80, 25))},
		{"an odd number of dashes", func(c *Canvas) {
			c.LineWidth = 20
			c.SetLineDash(10, 20, 30)
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			c.Stroke()
		}, pxs(at(blackPx, 5, 25, 40, 25, 85, 25), at(none, 20, 25, 65, 25, 93, 25))},
		{"an offset, and one that is none", func(c *Canvas) {
			c.LineWidth = 20
			c.SetLineDash(10, 10)
			c.LineDashOffset = 10
			c.MoveTo(0, 10)
			c.LineTo(50, 10)
			c.Stroke()
			c.LineDashOffset = nan
			c.BeginPath()
			c.MoveTo(0, 40)
			c.LineTo(50, 40)
			c.Stroke()
		}, pxs(at(blackPx, 15, 10, 35, 10, 5, 40, 25, 40), at(none, 5, 10, 25, 10, 15, 40))},
		{"dashes are scaled with the line", func(c *Canvas) {
			c.Scale(10, 10)
			c.SetLineDash(1, 1)
			c.LineWidth = 2
			c.MoveTo(0, 2.5)
			c.LineTo(10, 2.5)
			c.Stroke()
		}, pxs(at(blackPx, 5, 25, 25, 25), at(none, 15, 25, 35, 25))},
		{"a dash that is not a length is ignored", func(c *Canvas) {
			c.LineWidth = 20
			c.SetLineDash(10, 10)
			c.SetLineDash(1, -1)
			c.SetLineDash(1, inf)
			c.SetLineDash(nan)
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			c.Stroke()
		}, pxs(at(blackPx, 5, 25, 25, 25), at(none, 15, 25, 35, 25))},
		{"dashes of no length are no dashes", func(c *Canvas) {
			c.LineWidth = 20
			c.SetLineDash(0, 0)
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			c.Stroke()
		}, at(blackPx, 5, 25, 50, 25)},
		{"restore brings the dash back", func(c *Canvas) {
			c.LineWidth = 20
			c.SetLineDash(10, 10)
			c.Save()
			c.SetLineDash()
			c.Restore()
			c.MoveTo(0, 25)
			c.LineTo(100, 25)
			c.Stroke()
		}, pxs(at(blackPx, 5, 25), at(none, 15, 25))},

		// Gradients.
		{"a linear gradient (2d.gradient.interpolate.2color)", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(0, 0, 100, 0), 0, gfx.MustParseHex("#ff0"), 1, blue))
		}, pxs(at(color.RGBA{191, 191, 63, 255}, 25, 25), at(color.RGBA{127, 127, 127, 255}, 50, 25), at(color.RGBA{63, 63, 191, 255}, 75, 25))},
		{"alpha is not premultiplied (2d.gradient.interpolate.colouralpha)", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(0, 0, 100, 0), 0, gfx.RGBA8(255, 255, 0, 0), 1, blue))
		}, pxs(at(color.RGBA{48, 48, 16, 64}, 25, 25), at(color.RGBA{64, 64, 64, 128}, 50, 25), at(color.RGBA{48, 48, 143, 191}, 75, 25))},
		{"one stop (2d.gradient.interpolate.solid)", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(0, 0, 100, 0), 0.5, green))
		}, at(greenPx, 2, 25, 98, 25)},
		{"stops in any order, and at the same offset (2d.gradient.interpolate.overlap)", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(0, 0, 100, 0), 1, blue, 0.5, red, 0, red, 0.5, green, 0.5, blue))
		}, pxs(at(redPx, 2, 25, 48, 25), at(bluePx, 52, 25, 98, 25))},
		{"the ends of a gradient go on (2d.gradient.interpolate.outside)", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(25, 0, 75, 0), 0.4, green, 0.6, green))
		}, at(greenPx, 20, 25, 50, 25, 80, 25)},
		{"no stops (2d.gradient.empty)", func(c *Canvas) { whole(c, c.CreateLinearGradient(0, 0, 0, 50)) }, at(none, 50, 25)},
		{"a line of no length (2d.gradient.interpolate.zerosize.fillRect)", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(50, 25, 50, 25), 0, red, 1, red))
		}, at(none, 40, 20)},
		{"invalid stops and coordinates", func(c *Canvas) {
			whole(c, gradient(c.CreateLinearGradient(0, 0, nan, 0), 0, red))
			whole(c, gradient(c.CreateRadialGradient(0, 0, 0, 0, 0, -1), 0, red))
			whole(c, gradient(c.CreateRadialGradient(0, 0, 0, 0, inf, 1), 0, red))
			whole(c, gradient(c.CreateConicGradient(inf, 0, 0), 0, red))
			whole(c, gradient(c.CreateLinearGradient(0, 0, 100, 0), -0.1, red, 1.1, red, nan, red, 0.5, gfx.Color{R: float32(nan), A: 1}))
			whole(c, (*CanvasGradient)(nil))
			whole(c, (*CanvasPattern)(nil))
		}, at(none, 50, 25)},
		{"a gradient is in the coordinates of its paint (2d.gradient.linear.transform.2)", func(c *Canvas) {
			c.Translate(100, 0)
			g := gradient(c.CreateLinearGradient(0, 0, 200, 0), 0, red, 0.25, green, 0.75, green, 1, red)
			c.Translate(-100-50, 0)
			c.FillStyle = g
			c.FillRect(50, 0, 100, 50)
		}, at(greenPx, 25, 25, 50, 25, 75, 25)},
		{"not in those of the path (2d.gradient.linear.transform.3)", func(c *Canvas) {
			g := gradient(c.CreateLinearGradient(0, 0, 200, 0), 0, red, 0.25, green, 0.75, green, 1, red)
			c.FillStyle = g
			c.Rect(0, 0, 100, 50)
			c.Translate(-50, 0)
			c.Fill()
		}, at(greenPx, 25, 25, 50, 25, 75, 25)},
		{"a radial gradient outside its circles (2d.gradient.radial.outside1)", func(c *Canvas) {
			whole(c, gradient(c.CreateRadialGradient(200, 25, 10, 200, 25, 20), 0, red, 1, green))
		}, at(greenPx, 1, 1, 50, 25, 98, 48)},
		{"inside (2d.gradient.radial.inside1)", func(c *Canvas) {
			whole(c, gradient(c.CreateRadialGradient(50, 25, 100, 50, 25, 200), 0, green, 1, red))
		}, at(greenPx, 1, 1, 50, 25, 98, 48)},
		{"equal circles (2d.gradient.radial.equal)", func(c *Canvas) {
			whole(c, gradient(c.CreateRadialGradient(50, 25, 20, 50, 25, 20), 0, red, 1, red))
		}, at(none, 1, 1, 50, 25)},
		{"a cone behind (2d.gradient.radial.cone.behind)", func(c *Canvas) {
			whole(c, green)
			whole(c, gradient(c.CreateRadialGradient(120, 25, 10, 211, 25, 100), 0, red, 1, red))
		}, at(greenPx, 1, 1, 50, 25, 98, 1)},
		{"a cone in front (2d.gradient.radial.cone.front)", func(c *Canvas) {
			whole(c, gradient(c.CreateRadialGradient(311, 25, 10, 210, 25, 100), 0, green, 1, green))
		}, at(greenPx, 1, 1, 50, 25, 98, 48)},
		{"a radial gradient under a transform (2d.gradient.radial.transform.1)", func(c *Canvas) {
			g := gradient(c.CreateRadialGradient(0, 0, 0, 0, 0, 11.2), 0, green, 0.5, green, 0.51, red, 1, red)
			c.FillStyle = g
			c.Translate(50, 25)
			c.Scale(10, 10)
			c.FillRect(-5, -2.5, 10, 5)
		}, at(greenPx, 25, 25, 50, 25, 75, 25)},
		{"a conic gradient starts to the right and goes clockwise", func(c *Canvas) {
			whole(c, gradient(c.CreateConicGradient(0, 50, 25), 0, red, 0.25, red, 0.25, green, 0.5, green, 0.5, blue, 0.75, blue, 0.75, black))
		}, pxs(at(redPx, 75, 40), at(greenPx, 25, 40), at(bluePx, 25, 10), at(blackPx, 75, 10))},
		{"and follows turns and mirrors", func(c *Canvas) {
			g := gradient(c.CreateConicGradient(-math.Pi/2, 0, 0), 0, red, 0.25, red, 0.25, green, 0.5, green, 0.5, blue, 0.75, blue, 0.75, black)
			c.Translate(50, 25)
			c.Rotate(math.Pi / 2)
			c.Scale(1, -1)
			c.FillStyle = g
			c.FillRect(-50, -50, 100, 100)
		}, pxs(at(redPx, 25, 40), at(greenPx, 75, 40), at(bluePx, 75, 10), at(blackPx, 25, 10))},
		{"a gradient on a distorted line", func(c *Canvas) {
			c.StrokeStyle = gradient(c.CreateRadialGradient(5, 25, 0, 5, 25, 5), 0, red, 0.5, red, 0.5, green, 1, green)
			c.Scale(10, 1)
			c.LineWidth = 10
			c.MoveTo(0, 25)
			c.LineTo(10, 25)
			c.Stroke()
		}, pxs(at(redPx, 50, 25, 30, 25, 70, 25), at(greenPx, 20, 25, 80, 25), at(none, 50, 35))},
		{"globalAlpha and a gradient (2d.composite.globalAlpha.fill)", func(c *Canvas) {
			c.GlobalAlpha = 0.5
			whole(c, gradient(c.CreateLinearGradient(0, 0, 100, 0), 0, red, 1, red))
		}, at(halfRedPx, 2, 25, 98, 25)},
		{"a gradient that gets a stop after it painted", func(c *Canvas) {
			g := gradient(c.CreateLinearGradient(0, 0, 100, 0), 0, red)
			whole(c, g)
			g.AddColorStop(1, red)
			g.AddColorStop(0, green) // after the red one at the same offset
			c.FillRect(0, 0, 50, 50)
			g.AddColorStop(0.5, green)
			c.FillRect(0, 0, 50, 50)
		}, pxs(at(greenPx, 2, 25, 45, 25), at(redPx, 75, 25))},

		// Patterns.
		{"a pattern repeats (2d.pattern.paint.repeat.basic)", func(c *Canvas) {
			whole(c, c.CreatePattern(solid(10, 10, greenPx), gfx.RepeatBoth))
		}, at(greenPx, 1, 1, 98, 48)},
		{"to the sides (2d.pattern.paint.repeatx.basic)", func(c *Canvas) {
			whole(c, c.CreatePattern(solid(10, 10, greenPx), gfx.RepeatX))
		}, pxs(at(greenPx, 1, 1, 98, 8), at(none, 1, 12, 50, 48))},
		{"up and down (2d.pattern.paint.repeaty.basic)", func(c *Canvas) {
			whole(c, c.CreatePattern(solid(10, 10, greenPx), gfx.RepeatY))
		}, pxs(at(greenPx, 1, 1, 8, 48), at(none, 12, 1, 98, 48))},
		{"or not (2d.pattern.paint.norepeat.basic)", func(c *Canvas) {
			whole(c, c.CreatePattern(solid(10, 10, greenPx), gfx.RepeatNone))
		}, pxs(at(greenPx, 1, 1, 8, 8), at(none, 12, 1, 1, 12, 98, 48))},
		{"an unknown repetition repeats", func(c *Canvas) {
			whole(c, c.CreatePattern(solid(10, 10, greenPx), 9))
		}, at(greenPx, 98, 48)},
		{"from the origin of its paint (2d.pattern.paint.norepeat.coord2)", func(c *Canvas) {
			p := c.CreatePattern(solid(50, 50, greenPx), gfx.RepeatNone)
			c.Translate(50, 0)
			c.FillStyle = p
			c.FillRect(-50, 0, 100, 50)
		}, pxs(at(greenPx, 52, 25, 98, 48), at(none, 48, 25))},
		{"setTransform", func(c *Canvas) {
			p := c.CreatePattern(solid(10, 10, greenPx), gfx.RepeatNone)
			p.SetTransform(2, 0, 0, 2, 50, 20)
			p.SetTransform(nan, 0, 0, 1, 0, 0)
			whole(c, p)
		}, pxs(at(greenPx, 55, 25, 68, 38), at(none, 45, 25, 72, 25, 60, 45))},
		{"a pattern that collapses, or of an empty image", func(c *Canvas) {
			p := c.CreatePattern(solid(10, 10, redPx), gfx.RepeatBoth)
			p.SetTransform(1, 0, 0, 0, 0, 0)
			whole(c, p)
			whole(c, c.CreatePattern(image.NewRGBA(image.Rect(0, 0, 0, 5)), gfx.RepeatBoth))
		}, at(none, 50, 25)},
		{"globalAlpha and a pattern (2d.composite.globalAlpha.imagepattern)", func(c *Canvas) {
			c.GlobalAlpha = 0.5
			whole(c, c.CreatePattern(solid(10, 10, redPx), gfx.RepeatBoth))
		}, at(halfRedPx, 2, 25, 98, 25)},
		{"a pattern without smoothing", func(c *Canvas) {
			c.ImageSmoothing = false
			c.Scale(50, 50)
			c.FillStyle = c.CreatePattern(halves(redPx, greenPx, false), gfx.RepeatNone)
			c.FillRect(0, 0, 2, 1)
		}, pxs(at(redPx, 5, 25, 48, 25), at(greenPx, 52, 25, 95, 25))},

		// Images.
		{"drawImage (2d.drawImage.3arg)", func(c *Canvas) {
			c.DrawImage(solid(20, 10, greenPx), 10, 20)
		}, pxs(at(greenPx, 11, 21, 28, 28), at(none, 8, 25, 32, 25, 20, 32))},
		{"drawImageSize (2d.drawImage.5arg)", func(c *Canvas) {
			c.DrawImageSize(solid(20, 10, greenPx), 10, 10, 60, 30)
		}, pxs(at(greenPx, 11, 11, 68, 38), at(none, 8, 25, 72, 25))},
		{"drawImageRegion (2d.drawImage.9arg.sourcepos)", func(c *Canvas) {
			c.DrawImageRegion(halves(redPx, greenPx, false), 1, 0, 1, 1, 0, 0, 100, 50)
		}, at(greenPx, 98, 25, 75, 25)},
		{"a region beyond the image is cut (2d.drawImage.clip)", func(c *Canvas) {
			c.DrawImageRegion(solid(10, 10, greenPx), -10, -5, 20, 20, 0, 0, 100, 40)
		}, pxs(at(greenPx, 52, 12, 98, 28), at(none, 48, 20, 75, 8, 75, 32))},
		{"a region outside the image", func(c *Canvas) {
			c.DrawImageRegion(solid(10, 10, redPx), 10, 0, 5, 5, 0, 0, 100, 50)
			c.DrawImageRegion(solid(10, 10, redPx), -5, 0, 5, 5, 0, 0, 100, 50)
		}, at(none, 50, 25)},
		{"negative sizes mirror nothing (2d.drawImage.negativedest)", func(c *Canvas) {
			c.ImageSmoothing = false
			c.DrawImageRegion(halves(redPx, greenPx, false), 2, 1, -2, -1, 100, 50, -100, -50)
		}, pxs(at(redPx, 25, 25), at(greenPx, 75, 25))},
		{"no size, or not finite (2d.drawImage.zerosource, nonfinite)", func(c *Canvas) {
			img := solid(10, 10, redPx)
			c.DrawImage(img, nan, 0)
			c.DrawImageSize(img, 0, 0, 0, 50)
			c.DrawImageSize(img, 0, 0, inf, 50)
			c.DrawImageRegion(img, 0, 0, 0, 10, 0, 0, 100, 50)
			c.DrawImageRegion(img, 0, 0, 10, nan, 0, 0, 100, 50)
			c.DrawImage(image.NewRGBA(image.Rectangle{}), 0, 0)
			c.Scale(0, 1)
			c.DrawImage(img, 0, 0)
		}, at(none, 5, 5, 50, 25)},
		{"drawImage mirrored by the transform", func(c *Canvas) {
			c.ImageSmoothing = false
			c.Translate(100, 0)
			c.Scale(-50, 50)
			c.DrawImage(halves(redPx, greenPx, false), 0, 0)
		}, pxs(at(greenPx, 25, 25), at(redPx, 75, 25))},
		{"an image of which the bounds are elsewhere", func(c *Canvas) {
			img := solid(40, 40, redPx)
			for y := 20; y < 40; y++ {
				for x := 20; x < 40; x++ {
					img.SetRGBA(x, y, greenPx)
				}
			}
			c.DrawImage(img.SubImage(image.Rect(20, 20, 40, 40)), 0, 0)
		}, pxs(at(greenPx, 1, 1, 18, 18), at(none, 22, 5))},
		{"globalAlpha and an image (2d.composite.globalAlpha.image)", func(c *Canvas) {
			c.GlobalAlpha = 0.5
			c.DrawImageSize(solid(2, 2, redPx), 0, 0, 100, 50)
		}, at(halfRedPx, 2, 25, 98, 25)},
		{"a smoothed region reads the image around it", func(c *Canvas) {
			c.DrawImageRegion(halves(redPx, greenPx, false), 0, 0, 1, 1, 0, 0, 100, 50)
		}, pxs(at(redPx, 2, 25), at(color.RGBA{128, 127, 0, 255}, 99, 25))},
		{"an icon", func(c *Canvas) {
			c.FillStyle = green
			c.DrawIcon(Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 1"><path fill="currentColor" d="M0 0h1v1H0z"/><path fill="#f00" d="M1 0h1v1H1z"/></svg>`}, 100, 50, -100, -50)
		}, pxs(at(greenPx, 25, 25), at(redPx, 75, 25))},
		{"an icon of a gradient", func(c *Canvas) {
			c.FillStyle = gradient(c.CreateLinearGradient(0, 0, 0, 50), 0, green, 0.5, green, 0.5, blue)
			c.DrawIcon(Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 1"><path fill="#f00" d="M0 0h1v1H0z"/></svg>`}, 0, 0, 100, 50)
		}, pxs(at(greenPx, 25, 12), at(bluePx, 25, 37), at(none, 75, 25))},

		// Shadows.
		{"a shadow (2d.shadow.offset.positiveX)", func(c *Canvas) {
			c.ShadowColor, c.ShadowOffsetX = green, 50
			c.FillStyle = red
			c.FillRect(0, 0, 50, 50)
		}, pxs(at(redPx, 25, 25), at(greenPx, 75, 25))},
		{"a shadow is not transformed (2d.shadow.transform.1)", func(c *Canvas) {
			c.ShadowOffsetY, c.ShadowColor = 50, green
			c.Translate(100, 100)
			c.Scale(2, 2)
			c.FillRect(-50, -75, 50, 25)
		}, at(greenPx, 50, 25)},
		{"nor drawn without an offset or a blur (2d.shadow.enable.off.1)", func(c *Canvas) {
			c.ShadowColor = red
			c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = -5, nan, inf
			whole(c, green)
		}, at(greenPx, 50, 25)},
		{"a shadow's alpha is also the source's (2d.shadow.alpha.5)", func(c *Canvas) {
			c.FillStyle = gfx.Color{G: 1, A: 0.5}
			c.ShadowColor, c.ShadowOffsetY = gfx.Color{R: 1, A: 1}, 50
			c.FillRect(0, -50, 100, 50)
		}, at(halfRedPx, 50, 25)},
		{"and the canvas's (2d.shadow.alpha.4)", func(c *Canvas) {
			c.FillStyle = red
			c.ShadowColor, c.ShadowOffsetY = gfx.Color{R: 1, A: 0.5}, 50
			c.GlobalAlpha = 0.5
			c.FillRect(0, -50, 100, 50)
		}, at(color.RGBA{64, 0, 0, 64}, 50, 25)},
		{"the shadow of a stroke (2d.shadow.stroke.basic)", func(c *Canvas) {
			c.ShadowColor, c.ShadowOffsetY = green, 50
			c.LineWidth = 50
			c.MoveTo(0, -25)
			c.LineTo(100, -25)
			c.Stroke()
		}, at(greenPx, 1, 1, 50, 25, 98, 48)},
		{"of an image (2d.shadow.image.basic)", func(c *Canvas) {
			c.ShadowColor, c.ShadowOffsetY = green, 50
			c.DrawImageSize(solid(2, 2, redPx), 0, -50, 100, 50)
		}, at(greenPx, 50, 25)},
		{"of a transparent image (2d.shadow.image.alpha)", func(c *Canvas) {
			c.ShadowColor, c.ShadowOffsetY = red, 50
			c.DrawImageSize(solid(2, 2, halfRedPx), 0, -50, 100, 50)
		}, at(halfRedPx, 50, 25)},
		{"of a gradient (2d.shadow.gradient.transparent.2)", func(c *Canvas) {
			c.ShadowColor, c.ShadowOffsetY = green, 50
			c.FillStyle = gradient(c.CreateLinearGradient(0, 0, 100, 0), 0, red, 0.499, red, 0.501, gfx.Transparent, 1, gfx.Transparent)
			c.FillRect(0, -50, 100, 50)
		}, pxs(at(greenPx, 25, 25), at(none, 75, 25))},
		{"of a pattern (2d.shadow.pattern.transparent.2)", func(c *Canvas) {
			c.ShadowColor, c.ShadowOffsetY = green, 50
			c.FillStyle = c.CreatePattern(solid(50, 50, redPx), gfx.RepeatY)
			c.FillRect(0, -50, 100, 50)
		}, pxs(at(greenPx, 25, 25), at(none, 75, 25))},
		{"a shadow is clipped (2d.shadow.clip.1)", func(c *Canvas) {
			whole(c, red)
			c.Rect(50, 0, 50, 50)
			c.Clip()
			c.ShadowColor, c.ShadowOffsetX = green, 50
			c.FillRect(0, 0, 50, 50)
		}, pxs(at(redPx, 25, 25), at(greenPx, 75, 25))},
		{"a shadow is composited (2d.shadow.composite.3)", func(c *Canvas) {
			whole(c, red)
			c.GlobalCompositeOperation = Xor
			c.ShadowColor, c.ShadowOffsetX = red, 100
			c.FillRect(-100, 0, 200, 50)
		}, at(redPx, 50, 25)}, // the shadow takes the red away, and the source brings it back
		{"a blurred shadow of a rectangle is everywhere under it", func(c *Canvas) {
			c.ShadowColor, c.ShadowBlur = black, 4
			c.FillStyle = gfx.Color{R: 1, A: 0.5}
			c.FillRect(20, 10, 60, 30)
		}, pxs(at(color.RGBA{128, 0, 0, 192}, 50, 25), at(none, 8, 25, 50, 2))},

		// Compositing.
		{"destination-out erases", func(c *Canvas) {
			whole(c, red)
			c.GlobalCompositeOperation = DestinationOut
			c.FillRect(0, 0, 50, 50)
		}, pxs(at(none, 25, 25), at(redPx, 75, 25))},
		{"source-in clears what it does not cover (2d.composite.uncovered.fill.source-in)", func(c *Canvas) {
			whole(c, red)
			c.GlobalCompositeOperation = SourceIn
			c.FillStyle = green
			c.FillRect(0, 0, 50, 50)
		}, pxs(at(greenPx, 25, 25), at(none, 75, 25))},
		{"copy of nothing clears (2d.composite.uncovered.nocontext)", func(c *Canvas) {
			whole(c, red)
			c.GlobalCompositeOperation = Copy
			c.FillStyle = nil
			c.FillRect(0, 0, 10, 10)
		}, at(none, 50, 25)},
		{"and so does a rectangle of no size", func(c *Canvas) {
			whole(c, red)
			c.GlobalCompositeOperation = Copy
			c.FillRect(0, 0, 0, 10)
		}, at(none, 50, 25)},
		{"but not what is not drawn", func(c *Canvas) {
			whole(c, green)
			c.GlobalCompositeOperation = Copy
			c.DrawImage(image.NewRGBA(image.Rectangle{}), 0, 0)
			c.StrokeRect(5, 5, 0, 0)
			c.Fill() // an empty path
			c.Stroke()
			c.MoveTo(5, 5)
			c.Stroke()
			c.LineTo(10, 10)
			c.Fill()
			c.Rect(5, 5, 20, 20)
			c.Scale(0, 1)
			c.Fill()
			c.Stroke()
			c.FillRect(5, 5, 20, 20)
			c.DrawImage(solid(5, 5, redPx), 5, 5)
			c.FillText("x", 5, 20)
			c.StrokeText("x", 5, 20)
		}, at(greenPx, 50, 25, 15, 15)},
		{"an unbounded operation stays in its clip (2d.composite.clip.copy)", func(c *Canvas) {
			whole(c, green)
			c.Rect(0, 0, 50, 50)
			c.Clip()
			c.GlobalCompositeOperation = Copy
			c.FillStyle = blue
			c.FillRect(0, 0, 20, 50)
		}, pxs(at(bluePx, 10, 25), at(none, 35, 25), at(greenPx, 75, 25))},
		{"an unknown operation is source-over", func(c *Canvas) {
			whole(c, red)
			c.GlobalCompositeOperation = 99
			whole(c, green)
		}, at(greenPx, 50, 25)},
		{"globalAlpha and an operation", func(c *Canvas) {
			whole(c, gfx.Color{R: 1, A: 1})
			c.GlobalCompositeOperation, c.GlobalAlpha = DestinationIn, 0.5
			c.DrawImageSize(solid(1, 1, greenPx), 0, 0, 100, 50)
		}, at(halfRedPx, 50, 25)},
		{"clearRect of a part (2d.clearRect.basic)", func(c *Canvas) {
			whole(c, red)
			c.ClearRect(25, 10, 50, 30)
		}, pxs(at(none, 26, 11, 50, 25, 73, 38), at(redPx, 23, 25, 77, 25, 50, 5))},
		{"clearRect is clipped (2d.clearRect.clip)", func(c *Canvas) {
			whole(c, green)
			c.Rect(0, 0, 16, 16)
			c.Clip()
			c.ClearRect(0, 0, 100, 50)
		}, pxs(at(none, 8, 8), at(greenPx, 50, 25))},
		{"clearRect turned", func(c *Canvas) {
			whole(c, red)
			c.Translate(50, 25)
			c.Rotate(math.Pi / 4)
			c.ClearRect(-10, -10, 20, 20)
		}, pxs(at(none, 50, 25, 50, 13, 62, 25), at(redPx, 40, 15, 60, 35))},
		{"clearRect knows no alpha, shadow or operation (2d.clearRect.globalalpha, shadow, globalcomposite)", func(c *Canvas) {
			whole(c, red)
			c.GlobalAlpha, c.GlobalCompositeOperation = 0.1, DestinationAtop
			c.ShadowColor, c.ShadowBlur = green, 10
			c.ClearRect(0, 0, 50, 50)
		}, pxs(at(none, 25, 25), at(redPx, 75, 25))},
		{"clearRect clears whole pixels", func(c *Canvas) {
			whole(c, red)
			c.ClearRect(0, 0, 50.6, 24.4)
		}, pxs(at(none, 25, 23, 50, 23), at(redPx, 25, 24, 51, 23))},
		{"drawImage covers whole pixels", func(c *Canvas) {
			c.DrawImage(solid(8, 8, redPx), 10.3, 10.6)
			c.DrawImage(solid(8, 8, greenPx), 18.3, 10.6)
			c.Transform(0, 2, -1, 0, 40, 0) // a quarter turn
			c.DrawImage(solid(8, 8, bluePx), 5.3, 5.4)
		}, pxs(at(redPx, 10, 11, 17, 18), at(greenPx, 18, 11, 25, 18), at(none, 9, 14, 26, 14, 12, 10, 12, 19),
			at(bluePx, 27, 11, 34, 26), at(none, 26, 15, 35, 15, 30, 10, 30, 27))},

		// State.
		{"reset", func(c *Canvas) {
			whole(c, red)
			c.Rect(0, 0, 100, 50)
			c.Save()
			c.Clip()
			c.Translate(50, 0)
			c.GlobalAlpha, c.GlobalCompositeOperation = 0.5, Copy
			c.ShadowColor, c.ShadowBlur = red, 5
			c.Reset()
			c.Fill() // no path
			c.Restore()
			c.FillRect(0, 0, 50, 50)
		}, pxs(at(blackPx, 25, 25), at(none, 75, 25))},
		{"save and restore of everything", func(c *Canvas) {
			c.Save()
			c.GlobalCompositeOperation = Copy
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = red, 3, 4, 5
			c.ImageSmoothing = false
			c.Font, c.LetterSpacing, c.TextAlign, c.TextBaseline = CanvasFont{Size: 5}, 3, TextAlignCenter, BaselineTop
			c.LineDashOffset = 4
			c.Restore()
			if c.GlobalCompositeOperation != SourceOver || c.ShadowColor != (gfx.Color{}) || c.ShadowBlur != 0 || c.ShadowOffsetX != 0 || c.ShadowOffsetY != 0 ||
				!c.ImageSmoothing || c.Font.Size != 16 || c.LetterSpacing != 0 || c.TextAlign != TextAlignStart || c.TextBaseline != BaselineAlphabetic || c.LineDashOffset != 0 {
				panic("not restored")
			}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Recorder
			list, _ := r.Record(textFrame(100, 50, text.DirectionLTR, 1), tt.draw)
			img := image.NewRGBA(image.Rect(0, 0, 100, 50))
			raster.Render(img, list, raster.Options{Scale: 1})
			for _, p := range tt.want {
				if got := img.RGBAAt(p.x, p.y); !near(got, p.want) {
					t.Errorf("pixel %d,%d is %v, want %v", p.x, p.y, got, p.want)
				}
			}
		})
	}
}

// Every operation gives what the formulas of the compositing specification
// do, over a canvas that has something and nothing, and only a canvas that
// used one, or cleared a part, is a layer.
func TestOperations(t *testing.T) {
	seen := map[string]bool{}
	for o := range len(operations) {
		op := CompositeOperation(o)
		if op.String() == "" || seen[op.String()] {
			t.Errorf("operation %d is called %q", o, op)
		}
		seen[op.String()] = true
		list := record(100, 50, func(c *Canvas) {
			c.FillStyle = red
			c.FillRect(0, 0, 50, 50)
			c.GlobalCompositeOperation = op
			c.FillStyle = gfx.Color{G: 1, A: 0.5}
			c.FillRect(25, 0, 50, 50)
		})
		_, layer := list.At(0).(gfx.PushLayer)
		if layer != (op != SourceOver) {
			t.Errorf("%v: the canvas is a layer: %v", op, layer)
		}
	}
	if len(seen) != 26 {
		t.Errorf("%d operations", len(seen))
	}
	isolated := func(draw func(c *Canvas)) bool {
		list := record(100, 50, draw)
		if list.Len() == 0 {
			return false
		}
		_, ok := list.At(0).(gfx.PushLayer)
		return ok
	}
	flood := func(c *Canvas) { c.FillRect(0, 0, 100, 50) }
	for name, tt := range map[string]struct {
		want bool
		draw func(c *Canvas)
	}{
		"nothing":                      {false, func(c *Canvas) {}},
		"a shadow":                     {false, func(c *Canvas) { c.ShadowColor, c.ShadowBlur = black, 5; c.Arc(50, 25, 10, 0, 6); c.Fill() }},
		"an alpha":                     {false, func(c *Canvas) { c.GlobalAlpha = 0.5; c.DrawImage(solid(4, 4, redPx), 0, 0) }},
		"a clear of everything":        {false, func(c *Canvas) { flood(c); c.ClearRect(0, 0, 100, 50); flood(c) }},
		"a clear of a part of nothing": {false, func(c *Canvas) { c.ClearRect(0, 0, 10, 10); flood(c) }},
		"a clear of a part":            {true, func(c *Canvas) { flood(c); c.ClearRect(0, 0, 10, 10) }},
		"an operation that is forgotten": {false, func(c *Canvas) {
			flood(c)
			c.GlobalCompositeOperation = Xor
			flood(c)
			c.GlobalCompositeOperation = SourceOver
			c.ClearRect(0, 0, 100, 50)
			flood(c)
		}},
		"a shadow with an operation": {true, func(c *Canvas) {
			flood(c)
			c.ShadowColor, c.ShadowBlur, c.GlobalCompositeOperation = black, 5, Multiply
			flood(c)
		}},
	} {
		if got := isolated(tt.draw); got != tt.want {
			t.Errorf("%s: a layer: %v, want %v", name, got, tt.want)
		}
	}
	// The layer keeps what is behind the canvas.
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 100, Y1: 50}, blue)
	b.AppendList(record(100, 50, func(c *Canvas) {
		flood(c)
		c.ClearRect(0, 0, 50, 50)
	}), nil)
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	raster.Render(img, b.Finish(), raster.Options{})
	if l, r := img.RGBAAt(25, 25), img.RGBAAt(75, 25); l != bluePx || r != blackPx {
		t.Errorf("over blue: %v cleared, %v not", l, r)
	}
}

// The closed form of a blurred shadow is the filter's (but for what three boxes
// are not a Gaussian), on every part of the
// shadow, under the box too.
func TestShadowsAgree(t *testing.T) {
	for name, build := range map[string]func(c *Canvas){
		"rect":    func(c *Canvas) { c.Rect(20, 12, 60, 26) },
		"rounded": func(c *Canvas) { c.RoundRect(20, 12, 60, 26, 10) },
		"corners": func(c *Canvas) { c.RoundRect(20, 12, 60, 26, 13, 0, 4, 8) },
		"circle":  func(c *Canvas) { c.Arc(50, 25, 18, 0, 2*math.Pi) },
		"ellipse": func(c *Canvas) { c.Ellipse(50, 25, 40, 15, 0, 0, 2*math.Pi) },
		"mirrored": func(c *Canvas) {
			c.Transform(0, 1, -1, 0, 100, 0)
			c.RoundRect(12, 20, 26, 60, 8, 0, 4)
		},
	} {
		for _, blur := range []float64{2, 8, 20} {
			draw := func(split bool) *image.RGBA {
				return render(100, 50, 1, func(c *Canvas) {
					c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = black, blur, 203, -2
					c.Translate(-200, 0) // the shadow alone
					build(c)
					if split {
						c.MoveTo(0, 0)
						c.LineTo(0, 0) // the path is not a box anymore
					}
					c.Fill()
				})
			}
			a, b := draw(false), draw(true)
			worst := 0
			for i := range a.Pix {
				worst = max(worst, int(a.Pix[i])-int(b.Pix[i]), int(b.Pix[i])-int(a.Pix[i]))
			}
			if worst > 8 { // 3%: three boxes are not quite a Gaussian
				t.Errorf("%s, blur %g: the closed form is %d levels from the filter", name, blur, worst)
			}
		}
	}
}

// The miter limit that a stroke is recorded with draws as the canvas's, and
// its bounds are closer.
func TestMiterReach(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	closer := 0
	for range 3000 {
		var p gfx.Path
		pt := func() gfx.Point {
			if rng.IntN(3) == 0 { // corners on a line, and back on themselves
				return gfx.Point{X: float32(rng.IntN(4) * 20), Y: float32(rng.IntN(4) * 20)}
			}
			return gfx.Point{X: rng.Float32() * 100, Y: rng.Float32() * 100}
		}
		for range 1 + rng.IntN(2) {
			p.MoveTo(pt())
			for range 1 + rng.IntN(5) {
				switch rng.IntN(5) {
				case 0:
					p.QuadTo(pt(), pt())
				case 1:
					p.CubicTo(pt(), pt(), pt())
				default:
					p.LineTo(pt())
				}
			}
			if rng.IntN(2) == 0 {
				p.Close()
			}
		}
		s := gfx.StrokeStyle{Width: 5, MiterLimit: []float32{1, 2, 4, 10, 500}[rng.IntN(5)]}
		if rng.IntN(3) == 0 {
			s.Dashes = []float32{7, 3}
		}
		want := p.Stroke(s, 0.1)
		reach := s
		reach.MiterLimit = miterReach(&p, s.MiterLimit)
		if reach.MiterLimit > s.MiterLimit {
			t.Fatalf("limit %g over %g", reach.MiterLimit, s.MiterLimit)
		}
		if reach.MiterLimit < s.MiterLimit {
			closer++
		}
		got := p.Stroke(reach, 0.1)
		if !slices.Equal(got.Verbs(), want.Verbs()) || !slices.Equal(got.Points(), want.Points()) {
			t.Fatalf("limit %g instead of %g draws another stroke of %v %v", reach.MiterLimit, s.MiterLimit, p.Verbs(), p.Points())
		}
	}
	if closer < 1000 {
		t.Errorf("only %d of 3000 strokes have closer bounds", closer)
	}
	list := record(100, 50, func(c *Canvas) {
		c.LineWidth = 4
		c.MoveTo(20, 20)
		c.LineTo(60, 20)
		c.Stroke()
	})
	if got, want := list.BoundsAt(1), (gfx.Rect{X0: 18, Y0: 18, X1: 62, Y1: 22}); !want.Outset(0.1).ContainsRect(got) {
		t.Errorf("a line's bounds are %v, want %v", got, want)
	}
}

func TestContains(t *testing.T) {
	ring := MustPath("M0 0H40V40H0Z M10 10V30H30V10Z")
	var circle Path2D
	circle.Arc(50, 50, 40, 0, 2*math.Pi)
	var same Path2D // turns the same way
	same.Rect(0, 0, 40, 40)
	same.Rect(10, 10, 20, 20)
	curve := MustPath("M0 0 Q50 100 100 0")
	for _, tt := range []struct {
		name string
		p    *Path2D
		x, y float64
		rule []FillRule
		want bool
	}{
		{"nothing", &Path2D{}, 0, 0, nil, false},
		{"nil", nil, 0, 0, nil, false},
		{"a lone point", MustPath("M5 5"), 5, 5, nil, false},
		{"the ring", ring, 5, 20, nil, true},
		{"its hole", ring, 20, 20, nil, false},
		{"outside", ring, 45, 20, nil, false},
		{"its edge", ring, 0, 20, nil, true},
		{"its corner", ring, 40, 40, nil, true},
		{"the edge of the hole", ring, 10, 20, nil, true},
		{"on the line of an edge, beyond it", ring, 20, 41, nil, false},
		{"to the left of a corner", ring, -5, 0, nil, false},
		{"non-zero", &same, 20, 20, nil, true},
		{"even-odd", &same, 20, 20, []FillRule{gfx.EvenOdd}, false},
		{"NaN", ring, nan, 5, nil, false},
		{"infinity", ring, 5, inf, nil, false},
		{"the circle", &circle, 50 + 28.28, 50 + 28.28, nil, true},
		{"outside the circle", &circle, 50 + 28.30, 50 + 28.30, nil, false},
		{"on the circle", &circle, 90, 50, nil, true},
		{"the bottom of the circle", &circle, 50, 89.99, nil, true},
		{"below the circle", &circle, 50, 90.01, nil, false},
		{"above the curve's middle", curve, 50, 49.9, nil, true},
		{"on it", curve, 50, 50, nil, true},
		{"below it", curve, 50, 50.1, nil, false},
		{"on the line that closes it", curve, 30, 0, nil, true},
	} {
		if got := tt.p.Contains(tt.x, tt.y, tt.rule...); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
	// As the rasterizer fills.
	rng := rand.New(rand.NewPCG(3, 4))
	for range 60 {
		var p Path2D
		f := func() float64 { return rng.Float64() * 100 }
		p.MoveTo(f(), f())
		for range 4 {
			switch rng.IntN(4) {
			case 0:
				p.QuadraticCurveTo(f(), f(), f(), f())
			case 1:
				p.BezierCurveTo(f(), f(), f(), f(), f(), f())
			case 2:
				p.Ellipse(f(), f(), f()/3, f()/3, f(), f(), f(), rng.IntN(2) == 0)
			default:
				p.LineTo(f(), f())
			}
		}
		rule := FillRule(rng.IntN(2))
		img := render(100, 100, 1, func(c *Canvas) {
			c.FillRule = rule
			c.FillPath(&p)
		})
		for y := range 100 {
			for x := range 100 {
				a := img.RGBAAt(x, y).A
				if a != 0 && a != 255 {
					continue // an edge
				}
				if got := p.Contains(float64(x)+0.5, float64(y)+0.5, rule); got != (a == 255) {
					t.Fatalf("Contains(%d.5, %d.5) is %v, and its pixel's alpha %d", x, y, got, a)
				}
			}
		}
	}
}

func TestIsPointIn(t *testing.T) {
	record(100, 50, func(c *Canvas) {
		check := func(what string, got, want bool) {
			t.Helper()
			if got != want {
				t.Errorf("%s: %v", what, got)
			}
		}
		check("no path", c.IsPointInPath(0, 0), false)
		check("no path's stroke", c.IsPointInStroke(0, 0), false)
		c.Translate(50, 0)
		c.Rect(0, 0, 20, 20)
		// The point is one of the canvas, whatever the transform (2d.path.isPointInPath.transform.1).
		check("left of it", c.IsPointInPath(-10, 10), false)
		check("in it", c.IsPointInPath(60, 10), true)
		check("where the transform has it", c.IsPointInPath(10, 10), false)
		check("on its edge", c.IsPointInPath(50, 10), true)
		check("NaN", c.IsPointInPath(nan, 10), false)
		c.Rect(5, 5, 10, 10)
		check("twice in it", c.IsPointInPath(60, 10), true)
		c.FillRule = gfx.EvenOdd
		check("even-odd", c.IsPointInPath(60, 10), false)
		c.BeginPath()
		c.MoveTo(0, 25)
		c.LineTo(40, 25)
		c.LineWidth = 10
		check("in the line", c.IsPointInStroke(70, 29), true)
		check("on its edge", c.IsPointInStroke(70, 30), true)
		check("outside the line", c.IsPointInStroke(70, 31), false)
		check("not in the path", c.IsPointInPath(70, 29), false)
		c.Scale(1, 0.5)
		check("in the distorted line", c.IsPointInStroke(70, 27), true)
		check("outside the distorted line", c.IsPointInStroke(70, 29), false)
		c.SetLineDash(5, 5)
		check("in a gap", c.IsPointInStroke(57, 25), false)
		check("in a dash", c.IsPointInStroke(62, 25), true)
		c.LineWidth = 0
		check("no line", c.IsPointInStroke(62, 25), false)
	})
}

func TestParsePath(t *testing.T) {
	for d, want := range map[string]string{
		"L1 1":                "a move (M or m)",
		"M 1":                 "ends where a number",
		"M0 0 L":              "ends where a number",
		"M0 0 X":              `"X" at byte 5`,
		"M0 0 L 1 1 foo":      "where a number was expected",
		"M0 0 A1 1 0 2 0 5 5": "a flag (0 or 1)",
		"M0 0 Z 5":            "a command",
		"M0 0 L1 1,":          "ends where a number",
		"5 5":                 "a command",
		"M0 0 L L 1 1":        "a number",
	} {
		if _, err := ParsePath(d); err == nil || !strings.Contains(err.Error(), want) || !strings.HasPrefix(err.Error(), "tela.ParsePath: ") {
			t.Errorf("%q: %v, want %q", d, err, want)
		}
	}
	for _, d := range []string{"", " ", "M0 0", "m1-1.5.5 2e1 3 4z", "M0,0 10,0 10,10z m1 1 2 2", "M0 0A5 5 0 01 10 10"} {
		if _, err := ParsePath(d); err != nil {
			t.Errorf("%q: %v", d, err)
		}
	}
	expectPanic(t, `tela.MustPath("M")`, func() { MustPath("M") })
	// As the SVG document that has it.
	const d = "M10 10c20-10 40 30 60 0s20 10 20 20q-10 15-30 5t-20 0a12 8 20 1 0-25-5h-5v-10z"
	img := render(100, 50, 2, func(c *Canvas) { c.FillPath(MustPath(d)) })
	doc, err := svg.Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50"><path d="` + d + `"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	b := gfx.NewBuilder()
	doc.Draw(b, gfx.Rect{X1: 100, Y1: 50}, svg.Options{CurrentColor: black})
	want := image.NewRGBA(img.Rect)
	raster.Render(want, b.Finish(), raster.Options{Scale: 2})
	if !slices.Equal(img.Pix, want.Pix) {
		t.Error("the path is not the SVG's")
	}
}

// What is not there panics in dev mode, and does nothing otherwise.
func TestNil(t *testing.T) {
	for want, draw := range map[string]func(c *Canvas){
		"Canvas.FillPath(nil)":                   func(c *Canvas) { c.FillPath(nil) },
		"Canvas.StrokePath(nil)":                 func(c *Canvas) { c.StrokePath(nil) },
		"Canvas.ClipPath(nil)":                   func(c *Canvas) { c.ClipPath(nil) },
		"Canvas.DrawImage got a nil image":       func(c *Canvas) { c.DrawImage(nil, 0, 0) },
		"Canvas.DrawImageSize got a nil *image":  func(c *Canvas) { c.DrawImageSize((*image.RGBA)(nil), 0, 0, 1, 1) },
		"Canvas.DrawImageRegion got a nil image": func(c *Canvas) { c.DrawImageRegion(nil, 0, 0, 1, 1, 0, 0, 1, 1) },
		"Canvas.CreatePattern got a nil image":   func(c *Canvas) { c.FillStyle = c.CreatePattern(nil, gfx.RepeatBoth); c.FillRect(0, 0, 9, 9) },
	} {
		expectPanic(t, want, func() { record(10, 10, draw) })
		f := frameOf(10, 10)
		f.Dev = false
		var r Recorder
		if list, _ := r.Record(f, draw); list.Len() != 0 {
			t.Errorf("%s drew %s", want, list)
		}
	}
	expectPanic(t, "AddPath(nil)", func() { new(Path2D).AddPath(nil) })
	record(10, 10, func(c *Canvas) { c.DrawIcon(Icon{SVG: "<nope"}, 0, 0, 5, 5) })
}

func TestLineDash(t *testing.T) {
	record(100, 50, func(c *Canvas) {
		check := func(want ...float64) {
			t.Helper()
			if got := c.LineDash(); !slices.Equal(got, want) {
				t.Errorf("%v, want %v", got, want)
			}
		}
		check()
		in := []float64{1, 2, 3}
		c.SetLineDash(in...)
		in[0] = 9
		check(1, 2, 3, 1, 2, 3)
		c.LineDash()[0] = 9
		check(1, 2, 3, 1, 2, 3)
		c.SetLineDash(1, -2)
		check(1, 2, 3, 1, 2, 3)
		c.Save()
		c.SetLineDash(4, 5)
		check(4, 5)
		c.Restore()
		check(1, 2, 3, 1, 2, 3)
		c.SetLineDash()
		check()
	})
}

// Gradients and patterns are kept by the application, and paint in other
// draws and other canvases.
func TestStylesKept(t *testing.T) {
	var g *CanvasGradient
	var p *CanvasPattern
	img := solid(8, 8, greenPx)
	record(100, 50, func(c *Canvas) {
		g = c.CreateLinearGradient(0, 0, 100, 0)
		g.AddColorStop(0, red)
		p = c.CreatePattern(img, gfx.RepeatBoth)
	})
	out := render(100, 50, 1, func(c *Canvas) {
		c.FillStyle = g
		c.FillRect(0, 0, 50, 50)
		c.FillStyle = p
		c.FillRect(50, 0, 50, 50)
	})
	if a, b := out.RGBAAt(25, 25), out.RGBAAt(75, 25); a != redPx || b != greenPx {
		t.Errorf("%v, %v", a, b)
	}
}

// Images are converted once, with the mip level that their size needs.
func TestImageRecorded(t *testing.T) {
	img := photo() // 64x48
	for _, tt := range []struct {
		w      float64
		scale  float64
		smooth bool
		want   int // the width of the level
	}{
		{64, 1, true, 64}, {33, 1, true, 64}, {32, 1, true, 32}, {32, 2, true, 64}, {8, 1, true, 8}, {8, 1, false, 64}, {640, 1, true, 64},
	} {
		var r Recorder
		list, _ := r.Record(textFrame(100, 100, 0, tt.scale), func(c *Canvas) {
			c.ImageSmoothing = tt.smooth
			for range 3 {
				c.DrawImageSize(img, 0, 0, tt.w, math.Floor(tt.w*48/64))
			}
		})
		if list.Len() != 5 {
			t.Fatalf("%d commands", list.Len())
		}
		first := list.At(1).(gfx.DrawImage)
		if first.Image.Width() != tt.want || first.Image != list.At(3).(gfx.DrawImage).Image {
			t.Errorf("%+v: an image %d wide", tt, first.Image.Width())
		}
		if len(r.images) != 0 {
			t.Error("the recorder keeps the images")
		}
	}
}

// A rectangle far larger than the canvas clears what it covers, and nothing
// else: how far it goes does not make its near edges less exact.
func TestClearRectHuge(t *testing.T) {
	for _, size := range []float64{1e3, 1e6, 1e9, 1e15} {
		for _, at := range []float64{50, 1, 0.01} {
			img := render(100, 50, 1, func(c *Canvas) {
				c.FillRect(0, 0, 100, 50)
				c.ClearRect(at, 0, size, size)
			})
			if at >= 1 && img.RGBAAt(0, 25) != blackPx {
				t.Errorf("a rectangle of %g at %g cleared what is before it", size, at)
			}
			// What is left before it is less than a pixel, but it is left.
			if list := record(100, 50, func(c *Canvas) {
				c.FillRect(0, 0, 100, 50)
				c.ClearRect(at, 0, size, size)
			}); list.Len() == 0 {
				t.Errorf("a rectangle of %g at %g forgot the drawing", size, at)
			}
			if got := img.RGBAAt(99, 25); got != none {
				t.Errorf("a rectangle of %g at %g left %v", size, at, got)
			}
		}
	}
	// Turned, its near edge is where it is for a small one.
	want := -1
	for _, size := range []float64{300, 1e6, 1e9, 1e12, 1e39, 1e300} {
		img := render(100, 50, 1, func(c *Canvas) {
			c.FillRect(0, 0, 100, 50)
			c.Translate(50, 25)
			c.Rotate(1)
			c.ClearRect(0, -size, size, 2*size)
		})
		n := 0
		for y := range 50 {
			for x := range 100 {
				n += int(img.RGBAAt(x, y).A)
			}
		}
		if want < 0 {
			want = n
			if n < 2400*255 || n > 2600*255 {
				t.Errorf("half of the canvas: %d levels are left", n)
			}
		}
		if d := n - want; d < -255 || d > 255 {
			t.Errorf("turned, a rectangle of %g left %d levels, one of 300 %d", size, n, want)
		}
	}
	if img := render(100, 50, 1, func(c *Canvas) {
		c.FillRect(0, 0, 100, 50)
		c.ClearRect(50, -1e39, 1e39, 2e39)
	}); img.RGBAAt(40, 25) != blackPx || img.RGBAAt(60, 25) != none {
		t.Error("beyond float32, upright")
	}
	// The whole canvas, under transforms, is still forgotten.
	for _, angle := range []float64{0, 0.3, 1} {
		list := record(100, 50, func(c *Canvas) {
			c.FillRect(0, 0, 100, 50)
			c.Rotate(angle)
			c.Scale(1e-3, 1e3)
			c.ClearRect(-1e6, -1, 2e6, 2)
		})
		if list.Len() != 0 {
			t.Errorf("turned by %g: %d commands are left", angle, list.Len())
		}
	}
}

func gfxPath(p *Path2D) *gfx.Path {
	out := &gfx.Path{}
	p.p.gfx(out, nil)
	return out
}

// Angles too far apart for a float64 make an arc all the same.
func TestArcFarAngles(t *testing.T) {
	for _, ccw := range []bool{false, true} {
		for _, a := range [][2]float64{{1e308, -1e308}, {-1e308, 1e308}} {
			var p Path2D
			p.Ellipse(50, 25, 40, 20, 0, a[0], a[1], ccw)
			p.Arc(50, 25, 10, a[0], a[1], ccw)
			if g := gfxPath(&p); !g.IsFinite() || g.IsEmpty() {
				t.Errorf("from %g to %g, ccw %v: %v", a[0], a[1], ccw, g)
			}
		}
	}
}

// A flat ellipse ends whatever its angles are.
func TestEllipseFlatHugeAngles(t *testing.T) {
	for _, start := range []float64{0, 1, -1, 1e8, 1e16, -1e16, 3e16, 1e17, -1e17, 1e300} {
		for _, d := range []float64{1, 2, 4, 7, 1e17, -1, -2, -4, -1e17, 1e300} {
			for _, ccw := range []bool{false, true} {
				var p Path2D
				p.Ellipse(50, 25, 40, 0, 0, start, start+d, ccw)
				p.Ellipse(50, 25, 0, 20, 0, start, start+d, ccw)
				if n := len(p.p.verbs); n > 12 {
					t.Errorf("from %g by %g: %d lines", start, d, n)
				}
				// It spans what one that is hardly not flat does.
				span := func(ry float64) (lo, hi float32) {
					var p Path2D
					p.Ellipse(50, 25, 40, ry, 0, start, start+d, ccw)
					g := gfxPath(&p)
					if !g.IsFinite() {
						t.Errorf("from %g by %g: %v", start, d, g.Points())
					}
					b := g.Bounds()
					return b.X0, b.X1
				}
				lo, hi := span(0)
				if l, h := span(1e-9); math.Abs(float64(lo-l)) > 0.01 || math.Abs(float64(hi-h)) > 0.01 {
					t.Errorf("from %g by %g (ccw %v): flat from %.2f to %.2f, round from %.2f to %.2f", start, d, ccw, lo, hi, l, h)
				}
			}
		}
	}
}

// A radial gradient between circles apart is a cone, with one stop too.
func TestRadialGradientOneStop(t *testing.T) {
	img := render(200, 100, 1, func(c *Canvas) {
		g := c.CreateRadialGradient(40, 50, 5, 150, 50, 20)
		g.AddColorStop(0, gfx.Color{R: 1, A: 1})
		c.FillStyle = g
		c.FillRect(0, 0, 200, 100)
	})
	if got := img.RGBAAt(100, 50); got != redPx {
		t.Errorf("in the cone: %v", got)
	}
	for _, p := range [][2]int{{5, 5}, {5, 50}, {100, 5}, {100, 95}} {
		if got := img.RGBAAt(p[0], p[1]); got != none {
			t.Errorf("outside the cone, at %v: %v", p, got)
		}
	}
}
