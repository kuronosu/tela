package canvas

import (
	"image"
	"image/color"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// cellPage is a scene of cells of w by h, each one drawn by itself: from (0,
// 0), clipped to its size, with the initial state (but fills of sky and
// strokes of indigo) and an empty path.
type cellPage struct {
	name string
	w, h float64
	cols int
	list []cell
}

type cell struct {
	name string
	draw func(c *pen)
}

func (p *cellPage) scene() scene {
	var names []string
	for _, c := range p.list {
		names = append(names, c.name)
	}
	rows := (len(p.list) + p.cols - 1) / p.cols
	return scene{
		name: p.name, w: p.w * float64(p.cols), h: p.h * float64(rows),
		cells: grid(0, 0, float32(p.w), float32(p.h), p.cols, names...),
		draw: func(c *pen) {
			for i, cell := range p.list {
				c.Save()
				c.Translate(float64(i%p.cols)*p.w, float64(i/p.cols)*p.h)
				c.BeginPath()
				c.Rect(0, 0, p.w, p.h)
				c.Clip()
				c.BeginPath()
				c.FillStyle, c.StrokeStyle = sky, indigo
				cell.draw(c)
				c.Restore()
			}
		},
	}
}

func cellPages() []cellPage {
	return []cellPage{
		{"curves", 100, 100, 8, curveCells()},
		{"lines", 100, 100, 8, lineCells()},
		{"transforms", 100, 100, 8, transformCells()},
		{"gradients2", 100, 100, 8, gradientCells()},
		{"patterns2", 100, 100, 8, patternCells()},
		{"images2", 100, 100, 8, imageCells()},
		{"clips", 100, 100, 8, clipCells()},
		{"alpha", 100, 100, 8, alphaCells()},
		{"clear", 100, 100, 8, clearCells()},
		{"nothing", 100, 100, 8, nothingCells()},
		{"sources", 100, 100, 8, sourceCells()},
		{"shadows2", 100, 100, 8, shadowCells()},
		{"shadow_ops", 100, 100, 8, shadowOpCells()},
		{"baselines", 160, 60, 5, baselineCells()},
		{"text2", 160, 60, 5, textCells()},
	}
}

// both fills and strokes the path.
func both(build func(c *pen)) func(c *pen) {
	return func(c *pen) {
		c.LineWidth = 4
		c.BeginPath()
		build(c)
		c.Fill()
		c.Stroke()
	}
}

func curveCells() []cell {
	arcTo := func(x1, y1, x2, y2, r float64) func(c *pen) {
		return both(func(c *pen) {
			c.MoveTo(20, 80)
			c.ArcTo(x1, y1, x2, y2, r)
		})
	}
	ellipse := func(args ...float64) func(c *pen) {
		return both(func(c *pen) {
			c.MoveTo(50, 50)
			c.Ellipse(args[0], args[1], args[2], args[3], args[4], args[5], args[6], len(args) > 7)
		})
	}
	return []cell{
		{"arcTo", arcTo(50, 10, 80, 80, 15)},
		{"arcTo obtuse", arcTo(50, 60, 80, 80, 40)},
		{"arcTo acute", arcTo(50, 10, 30, 80, 8)},
		{"arcTo past its start", arcTo(50, 10, 80, 80, 60)},
		{"arcTo radius 0", arcTo(50, 10, 80, 80, 0)},
		{"arcTo collinear", arcTo(50, 50, 80, 20, 20)},
		{"arcTo back", arcTo(80, 20, 40, 60, 20)},
		{"arcTo same point", arcTo(20, 80, 80, 20, 20)},
		{"arcTo equal points", arcTo(80, 20, 80, 20, 20)},
		{"arcTo first", both(func(c *pen) { c.ArcTo(20, 20, 80, 20, 20); c.LineTo(80, 80) })},
		{"arcTo skewed", both(func(c *pen) {
			c.MoveTo(20, 80)
			c.Transform(1, 0.3, -0.5, 1, 20, -10)
			c.ArcTo(50, 10, 80, 80, 20)
			c.LineTo(60, 90)
		})},
		{"arcTo after close", both(func(c *pen) {
			c.MoveTo(20, 80)
			c.LineTo(50, 20)
			c.LineTo(50, 80)
			c.ClosePath()
			c.ArcTo(90, 90, 90, 20, 20)
		})},
		{"arc tiny angle", both(func(c *pen) { c.Arc(50, 50, 40, 1, 1.02) })},
		{"arc a turn", both(func(c *pen) { c.Arc(50, 50, 40, 0.5, 0.5+2*math.Pi) })},
		{"arc many turns", both(func(c *pen) { c.Arc(50, 50, 40, 1, 20) })},
		{"arc end before start", both(func(c *pen) { c.Arc(50, 50, 40, 2, 1) })},
		{"arc ccw", both(func(c *pen) { c.Arc(50, 50, 40, 2, 1, true) })},
		{"arc ccw turn", both(func(c *pen) { c.Arc(50, 50, 40, 1, 1-2*math.Pi, true) })},
		{"arc ccw past", both(func(c *pen) { c.Arc(50, 50, 40, 1, 2, true) })},
		{"arc no sweep", both(func(c *pen) { c.MoveTo(10, 10); c.Arc(50, 50, 40, 1, 1) })},
		{"arc radius 0", both(func(c *pen) { c.MoveTo(10, 20); c.Arc(50, 50, 0, 1, 2); c.LineTo(90, 20) })},
		{"arc joined", both(func(c *pen) { c.MoveTo(10, 10); c.Arc(50, 50, 30, 0, math.Pi); c.LineTo(90, 90) })},
		{"two arcs", both(func(c *pen) { c.Arc(50, 50, 40, 0, 2*math.Pi); c.Arc(50, 50, 20, 0, 2*math.Pi, true) })},
		{"ellipse part", ellipse(50, 50, 40, 20, 0.4, 0.5, 4)},
		{"ellipse ccw", ellipse(50, 50, 40, 20, 0.4, 0.5, 4, 1)},
		{"ellipse turned far", ellipse(50, 50, 40, 20, 8, -10, -7)},
		{"ellipse flat", ellipse(50, 50, 40, 0, 0.5, 0, 5)},
		{"ellipse thin", ellipse(50, 50, 0, 40, 0.5, 0, 2*math.Pi)},
		{"ellipse point", ellipse(50, 20, 0, 0, 0, 0, 5)},
		{"ellipse ratio", ellipse(50, 50, 45, 3, 0.7, 0, 2*math.Pi)},
		{"ellipse scaled", both(func(c *pen) { c.Scale(10, 1); c.Ellipse(5, 50, 4, 40, 0.1, 0, 5) })},
		{"roundRect w<0", both(func(c *pen) { c.RoundRect(90, 10, -80, 80, 40, 10, 5, 20) })},
		{"roundRect h<0", both(func(c *pen) { c.RoundRect(10, 90, 80, -80, 40, 10, 5, 20) })},
		{"roundRect wh<0", both(func(c *pen) { c.RoundRect(90, 90, -80, -80, 40, 10, 5, 20) })},
		{"roundRect overflow", both(func(c *pen) { c.RoundRect(10, 20, 80, 60, 100, 20, 50, 10) })},
		{"roundRect w=0", both(func(c *pen) { c.RoundRect(50, 10, 0, 80, 20) })},
		{"roundRect h=0", both(func(c *pen) { c.RoundRect(10, 50, 80, 0, 20) })},
		{"roundRect no radii", both(func(c *pen) { c.RoundRect(10, 10, 80, 80) })},
		{"roundRect then line", both(func(c *pen) { c.RoundRect(30, 30, 60, 60, 20); c.LineTo(10, 80) })},
		{"roundRect winding", func(c *pen) {
			c.RoundRect(10, 10, 80, 80, 20)
			c.RoundRect(70, 30, -40, 40, 10)
			c.Fill()
		}},
		{"roundRect dashes", func(c *pen) {
			c.LineWidth = 4
			c.SetLineDash(14, 6)
			c.RoundRect(10, 10, 80, 80, 20)
			c.Stroke()
		}},
		{"roundRect skewed", both(func(c *pen) { c.Transform(1, 0.3, -0.3, 0.7, 20, 0); c.RoundRect(10, 10, 80, 80, 30, 10) })},
		{"rect negative", both(func(c *pen) { c.Rect(90, 80, -80, -60) })},
		{"rect w=0", both(func(c *pen) { c.Rect(50, 10, 0, 80) })},
		{"rect point", both(func(c *pen) { c.LineJoin = gfx.JoinRound; c.Rect(50, 50, 0, 0) })},
		{"rect then line", both(func(c *pen) { c.Rect(30, 30, 40, 50); c.LineTo(10, 90); c.LineTo(80, 90) })},
		{"quad on its start", both(func(c *pen) { c.MoveTo(10, 80); c.QuadraticCurveTo(10, 80, 90, 20) })},
		{"quad back", both(func(c *pen) { c.MoveTo(20, 50); c.QuadraticCurveTo(120, 50, 40, 50) })},
		{"quad to its start", both(func(c *pen) { c.MoveTo(20, 50); c.QuadraticCurveTo(90, 50, 20, 50) })},
		{"quad sharp", both(func(c *pen) { c.MoveTo(20, 80); c.QuadraticCurveTo(50, -50, 21, 80) })},
		{"cubic cusp", both(func(c *pen) { c.MoveTo(10, 80); c.BezierCurveTo(90, 10, 10, 10, 90, 80) })},
		{"cubic loop", both(func(c *pen) { c.MoveTo(20, 80); c.BezierCurveTo(140, 0, -40, 0, 80, 80) })},
		{"cubic straight", both(func(c *pen) { c.MoveTo(10, 10); c.BezierCurveTo(40, 40, 60, 60, 90, 90) })},
		{"cubic back", both(func(c *pen) { c.MoveTo(30, 50); c.BezierCurveTo(120, 50, -20, 50, 70, 50) })},
		{"cubic controls on ends", both(func(c *pen) { c.MoveTo(10, 80); c.BezierCurveTo(10, 80, 90, 20, 90, 20) })},
		{"cubic to its start", both(func(c *pen) { c.MoveTo(50, 80); c.BezierCurveTo(120, 0, -20, 0, 50, 80) })},
		{"cubic s", both(func(c *pen) { c.MoveTo(10, 50); c.BezierCurveTo(80, -60, 20, 160, 90, 50) })},
		{"curves first", both(func(c *pen) { c.QuadraticCurveTo(20, 20, 80, 20); c.BezierCurveTo(90, 90, 50, 90, 10, 80) })},
		{"close twice", both(func(c *pen) {
			c.MoveTo(20, 20)
			c.LineTo(80, 20)
			c.LineTo(50, 60)
			c.ClosePath()
			c.ClosePath()
			c.LineTo(20, 90)
			c.LineTo(80, 90)
		})},
		{"self-intersecting", both(func(c *pen) {
			for i := range 5 {
				a := float64(i) * 4 * math.Pi / 5
				c.LineTo(50+math.Round(40*math.Sin(a)), 50-math.Round(40*math.Cos(a)))
			}
			c.ClosePath()
		})},
		{"even-odd", func(c *pen) {
			c.FillRule = gfx.EvenOdd
			for i := range 5 {
				a := float64(i) * 4 * math.Pi / 5
				c.LineTo(50+math.Round(40*math.Sin(a)), 50-math.Round(40*math.Cos(a)))
			}
			c.Fill()
		}},
		{"far away", both(func(c *pen) { c.MoveTo(-5000, 50); c.LineTo(5000, 60); c.LineTo(50, 8000) })},
		{"huge arc", both(func(c *pen) { c.Arc(50, 10050, 10000, 0, 2*math.Pi) })},
	}
}

func lineCells() []cell {
	line := func(pts ...float64) func(c *pen) {
		return func(c *pen) {
			for i := 0; i < len(pts); i += 2 {
				c.LineTo(pts[i], pts[i+1])
			}
		}
	}
	stroke := func(set, build func(c *pen)) func(c *pen) {
		return func(c *pen) {
			c.LineWidth = 10
			set(c)
			build(c)
			c.Stroke()
		}
	}
	none := func(c *pen) {}
	corner := func(join gfx.LineJoin, limit float64, pts ...float64) func(c *pen) {
		return stroke(func(c *pen) { c.LineJoin, c.MiterLimit = join, limit }, line(pts...))
	}
	dash := func(offset float64, cap gfx.LineCap, dashes ...float64) func(c *pen) {
		return stroke(func(c *pen) {
			c.LineWidth, c.LineCap, c.LineDashOffset = 6, cap, offset
			c.SetLineDash(dashes...)
		}, func(c *pen) {
			c.MoveTo(10, 15)
			c.LineTo(90, 15)
			c.MoveTo(10, 30)
			c.LineTo(50, 85)
			c.LineTo(90, 30)
			c.ClosePath()
		})
	}
	sharp := []float64{20, 80, 50, 20, 60, 80}
	var out []cell
	for _, j := range []struct {
		name string
		join gfx.LineJoin
	}{{"miter", gfx.JoinMiter}, {"round", gfx.JoinRound}, {"bevel", gfx.JoinBevel}} {
		out = append(out, cell{"join " + j.name, corner(j.join, 10, sharp...)})
	}
	for _, l := range []float64{1, 1.5, 2, 3, 3.5, 4, 20} {
		out = append(out, cell{"miter limit " + num(l), corner(gfx.JoinMiter, l, sharp...)})
	}
	return append(out, []cell{
		{"miter at its limit", corner(gfx.JoinMiter, math.Sqrt2, 20, 80, 50, 50, 20, 20)},
		{"miter below its limit", corner(gfx.JoinMiter, 1.414, 20, 80, 50, 50, 20, 20)},
		{"back on itself miter", corner(gfx.JoinMiter, 10, 20, 50, 80, 50, 30, 50)},
		{"back on itself round", corner(gfx.JoinRound, 10, 20, 50, 80, 50, 30, 50)},
		{"back on itself bevel", corner(gfx.JoinBevel, 10, 20, 50, 80, 50, 30, 50)},
		{"short segments", stroke(func(c *pen) { c.LineWidth = 24 }, line(20, 50, 50, 50, 52, 52, 50, 54, 20, 54))},
		{"needle", corner(gfx.JoinMiter, 100, 10, 80, 90, 78, 10, 76)},
		{"wide", stroke(func(c *pen) { c.LineWidth = 60 }, line(20, 80, 50, 40, 80, 80))},
		{"hairline", stroke(func(c *pen) { c.LineWidth = 0.4 }, line(10, 80, 50, 20, 90, 70))},
		{"very thin", stroke(func(c *pen) { c.LineWidth = 0.05 }, line(10, 80, 50, 20, 90, 70))},
		{"cap round", stroke(func(c *pen) { c.LineCap = gfx.CapRound }, line(20, 80, 80, 20))},
		{"cap square", stroke(func(c *pen) { c.LineCap = gfx.CapSquare }, line(20, 80, 80, 20))},
		{"zero line", stroke(func(c *pen) { c.LineCap = gfx.CapRound }, line(50, 50, 50, 50))},
		{"zero line square", stroke(func(c *pen) { c.LineCap = gfx.CapSquare }, line(50, 50, 50, 50))},
		{"zero line closed", stroke(func(c *pen) { c.LineCap = gfx.CapRound }, func(c *pen) { c.MoveTo(50, 50); c.LineTo(50, 50); c.ClosePath() })},
		{"dot", stroke(func(c *pen) { c.LineCap = gfx.CapRound }, func(c *pen) { c.MoveTo(50, 50); c.ClosePath() })},
		{"zero line within", stroke(func(c *pen) { c.LineJoin = gfx.JoinRound }, line(20, 80, 50, 20, 50, 20, 80, 80))},
		{"tiny line", stroke(func(c *pen) { c.LineCap = gfx.CapSquare }, line(50, 50, 50.001, 50))},
		{"closed", stroke(none, func(c *pen) { line(20, 80, 50, 20, 80, 80)(c); c.ClosePath() })},
		{"closed by a line", stroke(none, line(20, 80, 50, 20, 80, 80, 20, 80))},
		{"strokeRect", func(c *pen) { c.LineWidth = 10; c.StrokeRect(20, 20, 60, 60) }},
		{"strokeRect round", func(c *pen) { c.LineWidth, c.LineJoin = 10, gfx.JoinRound; c.StrokeRect(20, 20, 60, 60) }},
		{"strokeRect bevel", func(c *pen) { c.LineWidth, c.LineJoin = 10, gfx.JoinBevel; c.StrokeRect(20, 20, 60, 60) }},
		{"strokeRect limit", func(c *pen) { c.LineWidth, c.MiterLimit = 10, 1.2; c.StrokeRect(20, 20, 60, 60) }},
		{"strokeRect negative", func(c *pen) { c.LineWidth = 5; c.StrokeRect(80, 70, -60, -40) }},
		{"strokeRect filled", func(c *pen) { c.LineWidth = 50; c.StrokeRect(30, 30, 40, 40) }},
		{"strokeRect w=0", func(c *pen) { c.LineWidth = 10; c.StrokeRect(50, 20, 0, 60) }},
		{"strokeRect w=0 round", func(c *pen) { c.LineWidth, c.LineJoin = 10, gfx.JoinRound; c.StrokeRect(50, 20, 0, 60) }},
		{"strokeRect w=0 cap", func(c *pen) { c.LineWidth, c.LineCap = 10, gfx.CapRound; c.StrokeRect(50, 20, 0, 60) }},
		{"strokeRect dashes", func(c *pen) { c.LineWidth = 5; c.SetLineDash(13, 5); c.StrokeRect(20, 20, 60, 60) }},
		{"curve joins", stroke(none, func(c *pen) { c.MoveTo(20, 80); c.QuadraticCurveTo(20, 20, 50, 50); c.QuadraticCurveTo(80, 20, 80, 80) })},
		{"curve round joins", stroke(func(c *pen) { c.LineJoin, c.LineCap = gfx.JoinRound, gfx.CapRound }, func(c *pen) { c.MoveTo(20, 80); c.QuadraticCurveTo(20, 20, 50, 50); c.QuadraticCurveTo(80, 20, 80, 80) })},
		{"tight curve", stroke(func(c *pen) { c.LineWidth = 30 }, func(c *pen) { c.MoveTo(30, 80); c.BezierCurveTo(30, 20, 60, 20, 50, 60) })},
		{"wide circle", stroke(func(c *pen) { c.LineWidth = 60 }, func(c *pen) { c.Arc(50, 50, 10, 0, 2*math.Pi) })},

		{"dash", dash(0, gfx.CapButt, 12, 6)},
		{"dash offset", dash(5, gfx.CapButt, 12, 6)},
		{"dash negative offset", dash(-7, gfx.CapButt, 12, 6)},
		{"dash large offset", dash(1000, gfx.CapButt, 12, 6)},
		{"dash odd", dash(3, gfx.CapButt, 10, 4, 2)},
		{"dash round", dash(0, gfx.CapRound, 8, 12)},
		{"dash square", dash(0, gfx.CapSquare, 4, 12)},
		{"dash dots", dash(0, gfx.CapRound, 0, 10)},
		{"dash dots square", dash(0, gfx.CapSquare, 0, 12)},
		{"dash dots butt", dash(0, gfx.CapButt, 0, 10)},
		{"dash no gaps", dash(0, gfx.CapButt, 10, 0)},
		{"dash zeros", dash(0, gfx.CapButt, 0, 0)},
		{"dash longer", dash(0, gfx.CapButt, 500, 10)},
		{"dash only gap", dash(20, gfx.CapButt, 20, 500)},
		{"dash many", dash(0, gfx.CapButt, 1, 1)},
		{"dash scaled", stroke(func(c *pen) { c.Scale(2, 0.5); c.LineWidth = 6; c.SetLineDash(8, 4) }, line(10, 20, 40, 160, 45, 20))},
		{"dash curve", stroke(func(c *pen) { c.LineWidth = 6; c.SetLineDash(9, 4) }, func(c *pen) { c.MoveTo(10, 80); c.BezierCurveTo(90, -40, 10, -40, 90, 80) })},
		{"dash ellipse", stroke(func(c *pen) { c.LineWidth = 6; c.SetLineDash(9, 4) }, func(c *pen) { c.Ellipse(50, 50, 40, 20, 0.5, 0, 2*math.Pi) })},
		{"dash dot joins", stroke(func(c *pen) { c.LineJoin = gfx.JoinRound; c.SetLineDash(20, 5) }, func(c *pen) { line(20, 80, 50, 20, 80, 80)(c); c.ClosePath() })},
		{"dash skewed", stroke(func(c *pen) { c.Transform(1, 0.5, 0, 1, 0, -20); c.LineWidth = 6; c.SetLineDash(8, 4) }, func(c *pen) { c.Rect(20, 20, 60, 50) })},
	}...)
}

func transformCells() []cell {
	marks := func(c *pen) { // what shows that the state is as before
		c.FillStyle = rose
		c.FillRect(90, 90, 10, 10)
	}
	flat := func(set func(c *pen)) func(c *pen) {
		return func(c *pen) {
			c.LineWidth = 10
			c.Save()
			c.Translate(50, 50)
			set(c)
			c.FillRect(-40, -40, 60, 60)
			c.StrokeRect(-30, -30, 60, 60)
			c.BeginPath()
			c.MoveTo(-40, -40)
			c.LineTo(40, 40)
			c.LineTo(30, -40)
			c.Fill()
			c.Stroke()
			c.Font.Size = 30
			c.FillText("Flat", -30, 0)
			c.DrawImage(photo(), -20, -20)
			c.Restore()
			marks(c)
		}
	}
	return []cell{
		{"x scale 0", flat(func(c *pen) { c.Scale(0, 1) })},
		{"y scale 0", flat(func(c *pen) { c.Scale(1, 0) })},
		{"scale 0", flat(func(c *pen) { c.Scale(0, 0) })},
		{"singular", flat(func(c *pen) { c.Transform(1, 1, 1, 1, 0, 0) })},
		{"path kept through 0", func(c *pen) {
			c.MoveTo(20, 20)
			c.LineTo(80, 20)
			c.Save()
			c.Scale(0, 0)
			c.LineTo(50, 60)
			c.Restore()
			c.LineTo(50, 80)
			c.Fill()
		}},
		{"path added flat", func(c *pen) {
			c.Save()
			c.Translate(50, 0)
			c.Scale(0, 1)
			c.LineTo(0, 10)
			c.LineTo(80, 80)
			c.Restore()
			c.LineTo(90, 50)
			c.LineWidth = 6
			c.Fill()
			c.Stroke()
		}},
		{"clip flat", func(c *pen) {
			c.Save()
			c.Scale(0, 1)
			c.Rect(0, 0, 100, 100)
			c.Clip()
			c.Restore()
			c.Rect(10, 10, 60, 60)
			c.Fill()
			marks(c)
		}},
		{"transform after path", func(c *pen) {
			c.Arc(50, 50, 30, 0, 2*math.Pi)
			c.Scale(3, 1)
			c.LineWidth = 5
			c.Stroke()
		}},
		{"transform between", func(c *pen) {
			c.MoveTo(10, 10)
			c.Rotate(0.4)
			c.LineTo(90, 10)
			c.Rotate(0.4)
			c.LineTo(90, 10)
			c.LineWidth = 6
			c.Stroke()
		}},
		{"skewed stroke", func(c *pen) {
			c.Transform(1, 0, 1.2, 1, -60, 0)
			c.LineWidth = 10
			c.StrokeRect(30, 20, 50, 60)
		}},
		{"mirrored", func(c *pen) {
			c.Translate(100, 0)
			c.Scale(-1, 1)
			c.FillRect(10, 10, 30, 30)
			c.Font.Size = 24
			c.FillText("Mirror", 10, 80)
		}},
		{"rotated rect", func(c *pen) { c.Translate(50, 50); c.Rotate(0.3); c.FillRect(-30, -30, 60, 60) }},
		{"quarter turn", func(c *pen) { c.Translate(50, 50); c.Rotate(math.Pi / 2); c.FillRect(-30.5, -10.25, 60, 30) }},
		{"fractional rect", func(c *pen) { c.FillRect(10.3, 10.6, 40.2, 30.5) }},
		{"thin rect", func(c *pen) { c.FillRect(10.3, 10, 0.3, 80); c.FillRect(30, 10.4, 60, 0.2) }},
		{"negative rect", func(c *pen) { c.FillRect(90, 80, -70, -60) }},
		{"set transform", func(c *pen) {
			c.Rotate(1)
			c.SetTransform(1, 0, 0, 1, 10, 210)
			c.FillRect(0, 0, 50, 50)
			c.ResetTransform()
			c.FillRect(60, 270, 20, 20)
		}},
		{"tiny scale", func(c *pen) {
			c.Scale(1e-4, 1e-4)
			c.LineWidth = 1e5
			c.FillRect(1e5, 1e5, 5e5, 5e5)
			c.StrokeRect(2e5, 2e5, 5e5, 5e5)
		}},
		{"large scale", func(c *pen) {
			c.Scale(1e4, 1e4)
			c.LineWidth = 1e-3
			c.Arc(5e-3, 5e-3, 4e-3, 0, 5)
			c.Fill()
			c.Stroke()
		}},
	}
}

func gradientCells() []cell {
	fill := func(g func(c *pen) *CanvasGradient, stops ...any) func(c *pen) {
		return func(c *pen) {
			gr := g(c)
			for i := 0; i < len(stops); i += 2 {
				o, ok := stops[i].(float64)
				if !ok {
					o = float64(stops[i].(int))
				}
				gr.AddColorStop(o, stops[i+1].(gfx.Color))
			}
			c.FillStyle = gr
			c.FillRect(5, 5, 90, 90)
		}
	}
	linear := func(x0, y0, x1, y1 float64) func(c *pen) *CanvasGradient {
		return func(c *pen) *CanvasGradient { return c.CreateLinearGradient(x0, y0, x1, y1) }
	}
	radial := func(x0, y0, r0, x1, y1, r1 float64) func(c *pen) *CanvasGradient {
		return func(c *pen) *CanvasGradient { return c.CreateRadialGradient(x0, y0, r0, x1, y1, r1) }
	}
	conic := func(a, x, y float64) func(c *pen) *CanvasGradient {
		return func(c *pen) *CanvasGradient { return c.CreateConicGradient(a, x, y) }
	}
	two := []any{0, rose, 1, indigo}
	three := []any{0, rose, 0.5, amber, 1, indigo}
	clear := []any{0, rose, 0.5, gfx.Color{}, 1, indigo}
	with := func(set func(c *pen), g func(c *pen) *CanvasGradient, stops ...any) func(c *pen) {
		return func(c *pen) { set(c); fill(g, stops...)(c) }
	}
	return []cell{
		{"linear", fill(linear(10, 10, 90, 60), three...)},
		{"linear to the left", fill(linear(90, 10, 10, 30), three...)},
		{"one stop", fill(linear(10, 10, 90, 60), 0.5, rose)},
		{"no stops", fill(linear(10, 10, 90, 60))},
		{"no length", fill(linear(10, 10, 10, 10), two...)},
		{"equal stops", fill(linear(10, 0, 90, 0), 0, rose, 0.5, amber, 0.5, sky, 0.5, indigo, 1, slate)},
		{"equal stops at 0", fill(linear(10, 0, 90, 0), 0, rose, 0, amber, 1, sky)},
		{"equal stops at 1", fill(linear(10, 0, 90, 0), 0, rose, 1, amber, 1, sky)},
		{"all at one", fill(linear(10, 0, 90, 0), 0.4, rose, 0.4, amber)},
		{"inner stops", fill(linear(10, 0, 90, 0), 0.3, rose, 0.7, indigo)},
		{"unordered", fill(linear(10, 0, 90, 0), 1, rose, 0, amber, 0.5, sky)},
		{"many stops", fill(linear(5, 0, 95, 0), 0, rose, 0.1, amber, 0.2, sky, 0.3, indigo, 0.4, slate, 0.5, rose, 0.6, amber, 0.7, sky, 0.8, indigo, 0.9, slate, 1, rose)},
		{"short", fill(linear(50, 50, 50.5, 50), two...)},
		{"clear", fill(linear(10, 0, 90, 0), clear...)},
		{"clear black", fill(linear(10, 0, 90, 0), 0, gfx.Color{}, 1, rose)},
		{"faded", with(func(c *pen) { c.GlobalAlpha = 0.4 }, linear(10, 0, 90, 0), clear...)},
		{"circle", fill(radial(50, 50, 0, 50, 50, 40), three...)},
		{"circle hole", fill(radial(50, 50, 15, 50, 50, 40), three...)},
		{"circle inwards", fill(radial(50, 50, 40, 50, 50, 10), three...)},
		{"focal", fill(radial(35, 40, 0, 50, 50, 40), three...)},
		{"focal circle", fill(radial(40, 40, 10, 50, 50, 40), three...)},
		{"focal edge", fill(radial(10, 50, 0, 50, 50, 40), three...)},
		{"circles touch", fill(radial(20, 50, 10, 50, 50, 40), three...)},
		{"focal inwards", fill(radial(50, 50, 40, 40, 45, 5), three...)},
		{"cone", fill(radial(20, 50, 5, 70, 50, 20), three...)},
		{"cone from a point", fill(radial(10, 20, 0, 70, 60, 20), three...)},
		{"cone inwards", fill(radial(70, 50, 20, 20, 50, 5), three...)},
		{"cylinder", fill(radial(30, 50, 20, 70, 50, 20), three...)},
		{"crossing circles", fill(radial(40, 50, 20, 60, 50, 25), three...)},
		{"equal circles", fill(radial(50, 50, 20, 50, 50, 20), three...)},
		{"points", fill(radial(30, 50, 0, 70, 50, 0), three...)},
		{"clear circle", fill(radial(50, 50, 10, 50, 50, 40), clear...)},
		{"conic", fill(conic(0, 50, 50), three...)},
		{"conic from", fill(conic(1, 40, 60), three...)},
		{"conic far", fill(conic(-20, 40, 60), three...)},
		{"conic hard", fill(conic(0, 50, 50), 0, rose, 0.25, rose, 0.25, sky, 0.6, sky, 0.6, amber, 1, amber)},
		{"conic inner stops", fill(conic(0, 50, 50), 0.3, rose, 0.6, sky)},
		{"conic clear", fill(conic(2, 50, 50), clear...)},
		{"conic outside", fill(conic(0, -20, 120), two...)},
		{"linear turned", with(func(c *pen) { c.Translate(50, 50); c.Rotate(0.5); c.Translate(-50, -50) }, linear(10, 10, 90, 60), three...)},
		{"linear skewed", with(func(c *pen) { c.Transform(1, 0.2, 0.6, 1, -20, -10) }, linear(10, 10, 90, 60), three...)},
		{"circle skewed", with(func(c *pen) { c.Transform(1, 0.2, 0.6, 1, -20, -10) }, radial(50, 50, 5, 50, 50, 40), three...)},
		{"circle stretched", with(func(c *pen) { c.Scale(1, 0.5) }, radial(50, 80, 0, 50, 80, 45), three...)},
		{"focal skewed", with(func(c *pen) { c.Transform(1, 0.2, 0.6, 1, -20, -10) }, radial(40, 40, 5, 50, 50, 40), three...)},
		{"cone skewed", with(func(c *pen) { c.Transform(1, 0.2, 0.6, 1, -20, -10) }, radial(20, 50, 5, 70, 50, 20), three...)},
		{"conic skewed", with(func(c *pen) { c.Transform(1, 0.2, 0.6, 1, -20, -10) }, conic(0.5, 50, 50), three...)},
		{"conic mirrored", with(func(c *pen) { c.Transform(-1, 0, 0, 1, 100, 0) }, conic(0.5, 40, 50), three...)},
		{"stroke", func(c *pen) {
			g := c.CreateLinearGradient(10, 10, 90, 90)
			g.AddColorStop(0, rose)
			g.AddColorStop(1, sky)
			c.StrokeStyle, c.LineWidth = g, 12
			c.StrokeRect(20, 20, 60, 60)
		}},
		{"moved later", func(c *pen) {
			g := c.CreateLinearGradient(0, 0, 40, 0)
			g.AddColorStop(0, rose)
			g.AddColorStop(1, sky)
			c.FillStyle = g
			c.Rect(10, 10, 80, 80)
			c.Translate(40, 0)
			c.Fill()
		}},
		{"stop added later", func(c *pen) {
			g := c.CreateLinearGradient(10, 0, 90, 0)
			g.AddColorStop(0, rose)
			c.FillStyle = g
			c.FillRect(10, 10, 80, 30)
			g.AddColorStop(1, sky)
			c.FillRect(10, 60, 80, 30)
		}},
	}
}

// dots is a tile of 4 by 4 with sharp details.
func dots() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			if (x+y)%2 == 0 {
				img.SetNRGBA(x, y, color.NRGBA{225, 29, 72, 255})
			} else if x == 3 {
				img.SetNRGBA(x, y, color.NRGBA{56, 189, 248, 102})
			}
		}
	}
	return img
}

// fine is an image of 32 by 32 whose pixels are black or white.
func fine() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			v := uint8(255)
			if (x*7+y*13)%5 < 2 {
				v = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return img
}

// tileImage, photoImage, dotsImage and fineImage are the images of the cells: one each,
// so that the page has them once.
var tileImage, photoImage, dotsImage, fineImage = tile(), photo(), dots(), fine()

func patternCells() []cell {
	fill := func(img image.Image, repeat PatternRepeat, set func(c *pen, p *CanvasPattern)) func(c *pen) {
		return func(c *pen) {
			p := c.CreatePattern(img, repeat)
			set(c, p)
			c.FillStyle = p
			c.FillRect(5, 5, 90, 90)
		}
	}
	place := func(m ...float64) func(c *pen, p *CanvasPattern) {
		return func(c *pen, p *CanvasPattern) { p.SetTransform(m[0], m[1], m[2], m[3], m[4], m[5]) }
	}
	turn := func(a, s, x, y float64) func(c *pen, p *CanvasPattern) {
		return place(s*math.Cos(a), s*math.Sin(a), -s*math.Sin(a), s*math.Cos(a), x, y)
	}
	flat := func(set func(c *pen, p *CanvasPattern)) func(c *pen, p *CanvasPattern) {
		return func(c *pen, p *CanvasPattern) { c.ImageSmoothing = false; set(c, p) }
	}
	b, x, y, n := gfx.RepeatBoth, gfx.RepeatX, gfx.RepeatY, gfx.RepeatNone
	return []cell{
		{"moved", fill(tileImage, b, place(1, 0, 0, 1, 7, -3))},
		{"moved a half", fill(tileImage, b, place(1, 0, 0, 1, 0.5, 0.5))},
		{"moved a bit", fill(tileImage, b, place(1, 0, 0, 1, 3.3, 5.7))},
		{"moved a bit, pixels", fill(tileImage, b, flat(place(1, 0, 0, 1, 3.3, 5.7)))},
		{"turned", fill(tileImage, b, turn(0.4, 1, 10, 5))},
		{"turned, pixels", fill(tileImage, b, flat(turn(0.4, 1, 10, 5)))},
		{"turned larger", fill(tileImage, b, turn(0.4, 3, 10, 5))},
		{"turned larger, pixels", fill(tileImage, b, flat(turn(0.4, 3, 10, 5)))},
		{"larger", fill(dotsImage, b, place(6, 0, 0, 6, 0, 0))},
		{"larger, pixels", fill(dotsImage, b, flat(place(6, 0, 0, 6, 0, 0)))},
		{"larger x", fill(dotsImage, x, place(6, 0, 0, 6, 12, 20))},
		{"larger y", fill(dotsImage, y, place(6, 0, 0, 6, 12, 20))},
		{"larger once", fill(dotsImage, n, place(10, 0, 0, 10, 30, 20))},
		{"turned once", fill(tileImage, n, turn(0.4, 4, 40, 10))},
		{"turned x", fill(tileImage, x, turn(0.4, 2, 10, 10))},
		{"skewed", fill(tileImage, b, place(1, 0.3, -0.5, 1.5, 0, 0))},
		{"mirrored", fill(tileImage, b, place(-2, 0, 0, 2, 20, 0))},
		{"singular", fill(tileImage, b, place(1, 1, 1, 1, 0, 0))},
		{"smaller", fill(tileImage, b, place(0.75, 0, 0, 0.75, 0, 0))},
		{"half", fill(tileImage, b, place(0.5, 0, 0, 0.5, 0, 0))},
		{"third", fill(tileImage, b, place(0.3, 0, 0, 0.3, 0, 0))},
		{"tenth", fill(photoImage, b, place(0.1, 0, 0, 0.1, 0, 0))},
		{"third, pixels", fill(tileImage, b, flat(place(0.3, 0, 0, 0.3, 0, 0)))},
		{"photo third", fill(photoImage, b, place(0.3, 0, 0, 0.3, 0, 0))},
		{"fine 0.7", fill(fineImage, b, place(0.7, 0, 0, 0.7, 0, 0))},
		{"fine half", fill(fineImage, b, place(0.5, 0, 0, 0.5, 0, 0))},
		{"fine 0.4", fill(fineImage, b, place(0.4, 0, 0, 0.4, 0, 0))},
		{"fine quarter", fill(fineImage, b, place(0.25, 0, 0, 0.25, 0, 0))},
		{"fine tenth", fill(fineImage, b, place(0.1, 0, 0, 0.1, 0, 0))},
		{"fine turned", fill(fineImage, b, turn(0.4, 0.4, 10, 5))},
		{"faded", fill(tileImage, b, func(c *pen, p *CanvasPattern) { c.GlobalAlpha = 0.4 })},
		{"under a transform", fill(tileImage, b, func(c *pen, p *CanvasPattern) { c.Translate(20, 5); c.Rotate(0.2); c.Scale(1.5, 0.8) })},
		{"under both", fill(dotsImage, b, func(c *pen, p *CanvasPattern) { c.Transform(1, 0.2, 0.3, 1, 0, 0); p.SetTransform(4, 1, -1, 4, 10, 10) })},
		{"stroke", func(c *pen) {
			c.StrokeStyle, c.LineWidth = c.CreatePattern(tileImage, b), 20
			c.StrokeRect(20, 20, 60, 60)
		}},
		{"stroke scaled", func(c *pen) {
			c.StrokeStyle, c.LineWidth = c.CreatePattern(tileImage, b), 10
			c.Scale(2, 1)
			c.StrokeRect(10, 20, 30, 60)
		}},
		{"text", func(c *pen) {
			c.FillStyle = c.CreatePattern(dotsImage, b)
			c.Font = CanvasFont{Size: 60, Weight: 900}
			c.FillText("Ab", 5, 70)
		}},
		{"tile outside", func(c *pen) {
			c.FillStyle = c.CreatePattern(tileImage, b)
			c.Translate(-1000, -500)
			c.FillRect(1005, 505, 90, 90)
		}},
	}
}

func imageCells() []cell {
	img, small := image.Image(photoImage), image.Image(tileImage)
	turned := func(draw func(c *pen)) func(c *pen) {
		return func(c *pen) { c.Translate(50, 50); c.Rotate(0.4); draw(c) }
	}
	flat := func(draw func(c *pen)) func(c *pen) {
		return func(c *pen) { c.ImageSmoothing = false; draw(c) }
	}
	return []cell{
		{"flip x", func(c *pen) { c.DrawImageRegion(img, 0, 0, 64, 48, 90, 20, -80, 60) }},
		{"flip y", func(c *pen) { c.DrawImageRegion(img, 0, 0, 64, 48, 10, 80, 80, -60) }},
		{"flip both", func(c *pen) { c.DrawImageRegion(img, 0, 0, 64, 48, 90, 80, -80, -60) }},
		{"source flip x", func(c *pen) { c.DrawImageRegion(img, 64, 0, -64, 48, 10, 20, 80, 60) }},
		{"source flip both", func(c *pen) { c.DrawImageRegion(img, 64, 48, -64, -48, 10, 20, 80, 60) }},
		{"all flipped", func(c *pen) { c.DrawImageRegion(img, 40, 30, -30, -20, 90, 80, -80, -60) }},
		{"size flipped", func(c *pen) { c.DrawImageSize(img, 90, 80, -80, -60) }},
		{"mirrored", func(c *pen) { c.Scale(-1, 1); c.DrawImageSize(img, -90, 20, 80, 60) }},
		{"region", func(c *pen) { c.DrawImageRegion(img, 10, 10, 20, 15, 10, 20, 80, 60) }},
		{"region fraction", func(c *pen) { c.DrawImageRegion(img, 10.5, 10.25, 20.5, 15.5, 10, 20, 80, 60) }},
		{"region tiny", func(c *pen) { c.DrawImageRegion(img, 10.2, 10.2, 0.5, 0.5, 10, 20, 80, 60) }},
		{"region pixels", flat(func(c *pen) { c.DrawImageRegion(img, 10.5, 10.25, 20.5, 15.5, 10, 20, 80, 60) })},
		{"region edges", func(c *pen) { c.DrawImageRegion(small, 8, 8, 8, 8, 10, 10, 80, 80) }},
		{"region past", func(c *pen) { c.DrawImageRegion(img, -20, -10, 100, 100, 5, 5, 90, 90) }},
		{"region outside", func(c *pen) { c.DrawImageRegion(img, 70, 0, 10, 10, 5, 5, 90, 90) }},
		{"region empty", func(c *pen) { c.DrawImageRegion(img, 10, 10, 0, 10, 5, 5, 90, 90) }},
		{"larger", func(c *pen) { c.DrawImageSize(small, 2, 2, 96, 96) }},
		{"larger, pixels", flat(func(c *pen) { c.DrawImageSize(small, 2, 2, 96, 96) })},
		{"larger fraction", func(c *pen) { c.DrawImageSize(small, 10.3, 10.7, 77.7, 66.6) }},
		{"moved a half", func(c *pen) { c.DrawImage(img, 10.5, 10.5) }},
		{"moved a bit", func(c *pen) { c.DrawImage(img, 10.3, 10.8) }},
		{"moved a bit, pixels", flat(func(c *pen) { c.DrawImage(img, 10.3, 10.8) })},
		{"turned", turned(func(c *pen) { c.DrawImage(img, -32, -24) })},
		{"turned larger", turned(func(c *pen) { c.DrawImageSize(small, -35, -35, 70, 70) })},
		{"turned, pixels", turned(flat(func(c *pen) { c.DrawImageSize(small, -35, -35, 70, 70) }))},
		{"turned region", turned(func(c *pen) { c.DrawImageRegion(small, 8, 0, 8, 12, -35, -35, 70, 70) })},
		{"quarter turn", func(c *pen) { c.Translate(90, 10); c.Rotate(math.Pi / 2); c.DrawImage(img, 0, 0) }},
		{"skewed", func(c *pen) { c.Transform(1, 0.3, 0.5, 1, 10, 10); c.DrawImage(img, 0, 0) }},
		{"0.8", func(c *pen) { c.DrawImageSize(img, 10, 10, 51.2, 38.4) }},
		{"0.6", func(c *pen) { c.DrawImageSize(img, 10, 10, 38.4, 28.8) }},
		{"half", func(c *pen) { c.DrawImageSize(img, 10, 10, 32, 24) }},
		{"0.4", func(c *pen) { c.DrawImageSize(img, 10, 10, 25.6, 19.2) }},
		{"quarter", func(c *pen) { c.DrawImageSize(img, 10, 10, 16, 12) }},
		{"tenth", func(c *pen) { c.DrawImageSize(img, 10, 10, 6, 5) }},
		{"sharp 0.8", func(c *pen) { c.DrawImageSize(small, 10, 10, 12.8, 12.8) }},
		{"sharp half", func(c *pen) { c.DrawImageSize(small, 10, 10, 8, 8) }},
		{"sharp quarter", func(c *pen) { c.DrawImageSize(small, 10, 10, 4, 4) }},
		{"half x", func(c *pen) { c.DrawImageSize(img, 10, 10, 32, 80) }},
		{"half x only", func(c *pen) { c.DrawImageSize(img, 10, 10, 32, 48) }},
		{"half, pixels", flat(func(c *pen) { c.DrawImageSize(img, 10, 10, 32, 24) })},
		{"turned half", turned(func(c *pen) { c.DrawImageSize(img, -16, -12, 32, 24) })},
		{"turned quarter", turned(func(c *pen) { c.DrawImageSize(img, -8, -6, 16, 12) })},
		{"region half", func(c *pen) { c.DrawImageRegion(img, 10, 10, 40, 30, 10, 10, 20, 15) }},
		{"region quarter", func(c *pen) { c.DrawImageRegion(img, 10, 10, 40, 30, 10, 10, 10, 7.5) }},
		{"fine 0.7", func(c *pen) { c.DrawImageSize(fineImage, 10, 10, 22.4, 22.4) }},
		{"fine half", func(c *pen) { c.DrawImageSize(fineImage, 10, 10, 16, 16) }},
		{"fine 0.4", func(c *pen) { c.DrawImageSize(fineImage, 10, 10, 12.8, 12.8) }},
		{"fine quarter", func(c *pen) { c.DrawImageSize(fineImage, 10, 10, 8, 8) }},
		{"fine tenth", func(c *pen) { c.DrawImageSize(fineImage, 10, 10, 3.2, 3.2) }},
		{"fine turned", turned(func(c *pen) { c.DrawImageSize(fineImage, -8, -8, 12.8, 12.8) })},
		{"fine half x", func(c *pen) { c.DrawImageSize(fineImage, 10, 10, 16, 64) }},
		{"fine region", func(c *pen) { c.DrawImageRegion(fineImage, 4, 4, 20, 20, 10, 10, 8, 8) }},
		{"faded", func(c *pen) { c.GlobalAlpha = 0.4; c.DrawImageSize(img, 10, 10, 80, 80) }},
		{"shadow", func(c *pen) {
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = slate, 8, 5, 5
			c.DrawImageSize(small, 20, 20, 48, 48)
		}},
	}
}

func clipCells() []cell {
	circle := func(c *pen, x, y, r float64) { c.BeginPath(); c.Arc(x, y, r, 0, 2*math.Pi); c.Clip() }
	all := func(c *pen) { c.FillRect(0, 0, 100, 100) }
	return []cell{
		{"circle", func(c *pen) { circle(c, 50, 50, 40); all(c) }},
		{"two circles", func(c *pen) { circle(c, 40, 50, 35); circle(c, 60, 50, 35); all(c) }},
		{"same twice", func(c *pen) { circle(c, 50, 50, 40); circle(c, 50, 50, 40); all(c) }},
		{"rect", func(c *pen) { c.Rect(10.5, 10.25, 60, 60); c.Clip(); all(c) }},
		{"turned rect", func(c *pen) {
			c.Translate(50, 50)
			c.Rotate(0.3)
			c.Rect(-30, -30, 60, 60)
			c.Clip()
			c.FillRect(-50, -50, 100, 100)
		}},
		{"two turned", func(c *pen) {
			c.Translate(50, 50)
			for range 2 {
				c.Rotate(0.4)
				c.BeginPath()
				c.Rect(-30, -30, 60, 60)
				c.Clip()
			}
			c.FillRect(-50, -50, 100, 100)
		}},
		{"rounded", func(c *pen) { c.RoundRect(10, 10, 80, 80, 30, 10); c.Clip(); all(c) }},
		{"even-odd", func(c *pen) {
			c.FillRule = gfx.EvenOdd
			c.Arc(50, 50, 40, 0, 2*math.Pi)
			c.Arc(50, 50, 20, 0, 2*math.Pi)
			c.Clip()
			all(c)
		}},
		{"empty path", func(c *pen) { c.Clip(); all(c) }},
		{"line", func(c *pen) { c.MoveTo(10, 10); c.LineTo(90, 90); c.Clip(); all(c) }},
		{"disjoint", func(c *pen) { circle(c, 20, 50, 15); circle(c, 80, 50, 15); all(c) }},
		{"restored", func(c *pen) {
			c.Save()
			circle(c, 50, 50, 40)
			c.Save()
			circle(c, 50, 10, 40)
			all(c)
			c.Restore()
			c.FillStyle = rose
			c.FillRect(0, 50, 100, 20)
			c.Restore()
			c.FillStyle = amber
			c.FillRect(0, 90, 100, 10)
		}},
		{"path kept", func(c *pen) { c.Arc(50, 50, 30, 0, 2*math.Pi); c.Clip(); c.LineWidth = 20; c.Fill(); c.Stroke() }},
		{"fill the clip", func(c *pen) { c.Arc(50, 50, 30, 0, 2*math.Pi); c.Clip(); c.Fill() }},
		{"text", func(c *pen) {
			circle(c, 50, 50, 30)
			c.Font = CanvasFont{Size: 40, Weight: 800}
			c.FillText("Text", 5, 65)
		}},
		{"image", func(c *pen) { circle(c, 50, 50, 30); c.DrawImageSize(photoImage, 0, 0, 100, 100) }},
		{"shadow", func(c *pen) {
			circle(c, 50, 50, 40)
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX = slate, 10, 20
			c.FillRect(20, 30, 40, 40)
		}},
		{"source-in", func(c *pen) {
			all(c)
			circle(c, 50, 50, 40)
			c.GlobalCompositeOperation = SourceIn
			c.FillStyle = rose
			c.FillRect(50, 0, 30, 100)
		}},
		{"copy", func(c *pen) {
			all(c)
			circle(c, 50, 50, 40)
			c.GlobalCompositeOperation = Copy
			c.FillStyle = rose.WithAlpha(0.4)
			c.FillRect(50, 0, 30, 100)
		}},
		{"destination-out", func(c *pen) {
			all(c)
			circle(c, 50, 50, 40)
			c.GlobalCompositeOperation = DestinationOut
			c.FillRect(50, 0, 30, 100)
		}},
		{"multiply", func(c *pen) {
			all(c)
			circle(c, 50, 50, 40)
			c.GlobalCompositeOperation = Multiply
			c.FillStyle = amber
			c.FillRect(50, 0, 30, 100)
		}},
	}
}

func alphaCells() []cell {
	faded := func(draw func(c *pen)) func(c *pen) {
		return func(c *pen) {
			c.FillStyle = slate
			c.FillRect(0, 40, 100, 20)
			c.FillStyle = sky
			c.GlobalAlpha = 0.4
			draw(c)
		}
	}
	return []cell{
		{"fill", faded(func(c *pen) { c.FillRect(10, 10, 80, 80) })},
		{"translucent", faded(func(c *pen) { c.FillStyle = rose.WithAlpha(0.6); c.FillRect(10, 10, 80, 80) })},
		{"stroke on itself", faded(func(c *pen) {
			c.LineWidth = 14
			c.MoveTo(10, 10)
			c.LineTo(90, 90)
			c.LineTo(90, 10)
			c.LineTo(10, 90)
			c.Stroke()
		})},
		{"dashes", faded(func(c *pen) {
			c.LineWidth = 14
			c.SetLineDash(10, 10)
			c.LineCap = gfx.CapSquare
			c.MoveTo(10, 20)
			c.LineTo(90, 80)
			c.Stroke()
		})},
		{"text", faded(func(c *pen) { c.Font = CanvasFont{Size: 40, Weight: 800}; c.FillText("Text", 5, 65) })},
		{"tight text", faded(func(c *pen) {
			c.Font, c.LetterSpacing = CanvasFont{Size: 40, Weight: 800}, -8
			c.FillText("ABCD", 5, 65)
		})},
		{"emoji", faded(func(c *pen) { c.Font.Size = 50; c.FillText("😀", 15, 70) })},
		{"text and emoji", faded(func(c *pen) { c.Font.Size = 30; c.FillStyle = rose.WithAlpha(0.6); c.FillText("a😀b", 5, 60) })},
		{"strokeText", faded(func(c *pen) { c.Font = CanvasFont{Size: 40, Weight: 800}; c.LineWidth = 6; c.StrokeText("Text", 5, 65) })},
		{"tight strokeText", faded(func(c *pen) {
			c.Font, c.LetterSpacing = CanvasFont{Size: 40, Weight: 800}, -6
			c.LineWidth = 6
			c.StrokeText("ABCD", 5, 65)
		})},
		{"strokeText emoji", faded(func(c *pen) { c.Font.Size = 40; c.LineWidth = 3; c.StrokeText("a😀b", 5, 65) })},
		{"image", faded(func(c *pen) { c.DrawImageSize(tileImage, 10, 10, 80, 80) })},
		{"pattern", faded(func(c *pen) { c.FillStyle = c.CreatePattern(tileImage, gfx.RepeatBoth); c.FillRect(10, 10, 80, 80) })},
		{"pattern stroke", faded(func(c *pen) {
			c.StrokeStyle, c.LineWidth = c.CreatePattern(tileImage, gfx.RepeatBoth), 16
			c.StrokeRect(20, 20, 60, 60)
		})},
		{"gradient", faded(func(c *pen) {
			g := c.CreateLinearGradient(10, 0, 90, 0)
			g.AddColorStop(0, rose)
			g.AddColorStop(1, sky.WithAlpha(0.2))
			c.FillStyle = g
			c.FillRect(10, 10, 80, 80)
		})},
		{"shadow", faded(func(c *pen) {
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = rose, 6, 10, 10
			c.FillRect(10, 10, 60, 60)
		})},
		{"shadow only offset", faded(func(c *pen) {
			c.ShadowColor, c.ShadowOffsetX, c.ShadowOffsetY = rose, 20, 20
			c.FillRect(10, 10, 60, 60)
		})},
		{"shadow of an image", faded(func(c *pen) {
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = rose, 6, 10, 10
			c.DrawImageSize(tileImage, 10, 10, 64, 64)
		})},
		{"shadow of a gradient", faded(func(c *pen) {
			g := c.CreateLinearGradient(10, 0, 80, 0)
			g.AddColorStop(0, rose)
			g.AddColorStop(1, sky.WithAlpha(0.2))
			c.FillStyle = g
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = slate, 6, 10, 10
			c.FillRect(10, 10, 60, 60)
		})},
		{"shadow of an emoji", faded(func(c *pen) {
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = slate, 6, 10, 10
			c.Font.Size = 50
			c.FillText("😀", 10, 65)
		})},
		{"source-in", faded(func(c *pen) { c.GlobalCompositeOperation = SourceIn; c.FillRect(10, 10, 80, 80) })},
		{"copy", faded(func(c *pen) { c.GlobalCompositeOperation = Copy; c.FillRect(10, 10, 80, 80) })},
		{"destination-in", faded(func(c *pen) { c.GlobalCompositeOperation = DestinationIn; c.FillRect(10, 10, 80, 80) })},
		{"multiply", faded(func(c *pen) { c.GlobalCompositeOperation = Multiply; c.FillRect(10, 10, 80, 80) })},
		{"lighter", faded(func(c *pen) { c.GlobalCompositeOperation = Lighter; c.FillRect(10, 10, 80, 80) })},
		{"lighter image", faded(func(c *pen) { c.GlobalCompositeOperation = Lighter; c.DrawImageSize(photoImage, 10, 10, 80, 80) })},
		{"xor pattern", faded(func(c *pen) {
			c.GlobalCompositeOperation = Xor
			c.FillStyle = c.CreatePattern(tileImage, gfx.RepeatBoth)
			c.FillRect(10, 10, 80, 80)
		})},
	}
}

func clearCells() []cell {
	over := func(draw func(c *pen)) func(c *pen) {
		return func(c *pen) { c.FillRect(0, 0, 100, 100); draw(c) }
	}
	return []cell{
		{"part", over(func(c *pen) { c.ClearRect(20, 20, 40, 50) })},
		{"fraction", over(func(c *pen) { c.ClearRect(20.3, 20.6, 40.5, 50.5) })},
		{"negative", over(func(c *pen) { c.ClearRect(80, 80, -40, -50) })},
		{"empty", over(func(c *pen) { c.ClearRect(80, 80, 0, -50) })},
		{"all", over(func(c *pen) { c.ClearRect(-10, -10, 120, 120) })},
		{"turned", over(func(c *pen) { c.Translate(50, 50); c.Rotate(0.4); c.ClearRect(-30, -20, 60, 40) })},
		{"skewed", over(func(c *pen) { c.Transform(1, 0.3, 0.5, 1, 0, 0); c.ClearRect(20, 20, 40, 40) })},
		{"flat", over(func(c *pen) { c.Scale(0, 1); c.ClearRect(20, 20, 40, 40) })},
		{"clipped", over(func(c *pen) { c.Arc(50, 50, 30, 0, 2*math.Pi); c.Clip(); c.ClearRect(0, 0, 50, 100) })},
		{"clip cleared whole", over(func(c *pen) { c.Arc(50, 50, 30, 0, 2*math.Pi); c.Clip(); c.ClearRect(-500, -500, 5000, 5000) })},
		{"state ignored", over(func(c *pen) {
			c.GlobalAlpha, c.GlobalCompositeOperation = 0.3, Multiply
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX = rose, 10, 20
			c.ClearRect(20, 20, 40, 50)
		})},
		{"drawn over", over(func(c *pen) {
			c.ClearRect(20, 20, 40, 50)
			c.FillStyle = rose.WithAlpha(0.6)
			c.FillRect(40, 40, 50, 50)
		})},
		{"path kept", over(func(c *pen) { c.Rect(30, 30, 20, 20); c.ClearRect(10, 10, 70, 70); c.FillStyle = rose; c.Fill() })},
	}
}

// nothingCells draw what paints nothing, over a disc, with operations that
// clear what they do not paint (task T5's question (a)).
func nothingCells() []cell {
	var out []cell
	for _, op := range []CompositeOperation{SourceIn, SourceOut, DestinationIn, DestinationAtop, Copy, SourceOver, DestinationOut} {
		for _, n := range []struct {
			name string
			draw func(c *pen)
		}{
			{"empty path", func(c *pen) { c.BeginPath(); c.Fill() }},
			{"point", func(c *pen) { c.BeginPath(); c.MoveTo(10, 10); c.Fill() }},
			{"line", func(c *pen) { c.BeginPath(); c.MoveTo(10, 10); c.LineTo(60, 60); c.Fill() }},
			{"empty stroke", func(c *pen) { c.BeginPath(); c.Stroke() }},
			{"point stroke", func(c *pen) { c.BeginPath(); c.MoveTo(10, 10); c.Stroke() }},
			{"transparent", func(c *pen) { c.FillStyle = gfx.Color{}; c.FillRect(20, 20, 50, 50) }},
			{"transparent stroke", func(c *pen) { c.StrokeStyle = gfx.Color{}; c.StrokeRect(20, 20, 50, 50) }},
			{"alpha 0", func(c *pen) { c.GlobalAlpha = 0; c.FillRect(20, 20, 50, 50) }},
			{"alpha 0 image", func(c *pen) { c.GlobalAlpha = 0; c.DrawImage(tileImage, 20, 20) }},
			{"rect w=0", func(c *pen) { c.FillRect(20, 20, 0, 50) }},
			{"rect path w=0", func(c *pen) { c.BeginPath(); c.Rect(20, 20, 0, 50); c.Fill() }},
			{"strokeRect 0", func(c *pen) { c.StrokeRect(20, 20, 0, 0) }},
			{"image 0", func(c *pen) { c.DrawImageSize(tileImage, 20, 20, 0, 50) }},
			{"image outside", func(c *pen) { c.DrawImageRegion(tileImage, 40, 40, 5, 5, 20, 20, 30, 30) }},
			{"empty text", func(c *pen) { c.FillText("", 20, 50) }},
			{"space", func(c *pen) { c.FillText(" ", 20, 50) }},
			{"empty strokeText", func(c *pen) { c.StrokeText("", 20, 50) }},
			{"no gradient", func(c *pen) { c.FillStyle = c.CreateLinearGradient(5, 5, 5, 5); c.FillRect(20, 20, 50, 50) }},
			{"no stops", func(c *pen) { c.FillStyle = c.CreateLinearGradient(0, 0, 5, 5); c.FillRect(20, 20, 50, 50) }},
			{"no radial", func(c *pen) {
				g := c.CreateRadialGradient(5, 5, 5, 5, 5, 5)
				g.AddColorStop(0, rose)
				c.FillStyle = g
				c.FillRect(20, 20, 50, 50)
			}},
			{"flat pattern", func(c *pen) {
				p := c.CreatePattern(tileImage, gfx.RepeatBoth)
				p.SetTransform(1, 1, 1, 1, 0, 0)
				c.FillStyle = p
				c.FillRect(20, 20, 50, 50)
			}},
			{"flat transform", func(c *pen) { c.Scale(0, 1); c.FillRect(20, 20, 50, 50) }},
			{"flat fill", func(c *pen) { c.BeginPath(); c.Rect(20, 20, 50, 50); c.Scale(0, 1); c.Fill() }},
			{"flat stroke", func(c *pen) { c.BeginPath(); c.Rect(20, 20, 50, 50); c.Scale(0, 1); c.Stroke() }},
			{"flat text", func(c *pen) { c.Scale(0, 1); c.FillText("Text", 20, 50) }},
			{"flat image", func(c *pen) { c.Scale(0, 1); c.DrawImage(tileImage, 20, 20) }},
			{"outside", func(c *pen) { c.FillRect(-50, -50, 20, 20) }},
			{"far outside", func(c *pen) { c.FillRect(-5000, -50, 20, 20) }},
			{"text outside", func(c *pen) { c.FillText("Text", -500, -50) }},
			{"empty clip", func(c *pen) { c.BeginPath(); c.Clip(); c.FillRect(20, 20, 50, 50) }},
			{"maxWidth 0", func(c *pen) { c.FillText("Text", 20, 50, 0) }},
			{"transparent shadow", func(c *pen) {
				c.FillStyle = gfx.Color{}
				c.ShadowColor, c.ShadowOffsetX = rose, 10
				c.FillRect(20, 20, 50, 50)
			}},
		} {
			out = append(out, cell{op.String() + " " + n.name, func(c *pen) {
				c.Arc(50, 50, 30, 0, 2*math.Pi)
				c.Fill()
				c.GlobalCompositeOperation = op
				n.draw(c)
			}})
		}
	}
	return out
}

// sourceCells draw every kind of source with the operations that clear what
// they do not paint.
func sourceCells() []cell {
	var out []cell
	for _, op := range []CompositeOperation{SourceIn, SourceOut, DestinationIn, DestinationAtop, Copy, DestinationOver, Xor, Multiply} {
		for _, n := range []struct {
			name string
			draw func(c *pen)
		}{
			{"path", func(c *pen) { c.BeginPath(); c.MoveTo(20, 30); c.LineTo(90, 40); c.LineTo(50, 90); c.Fill() }},
			{"stroke", func(c *pen) {
				c.LineWidth = 12
				c.BeginPath()
				c.MoveTo(20, 30)
				c.LineTo(90, 40)
				c.LineTo(50, 90)
				c.Stroke()
			}},
			{"text", func(c *pen) { c.Font = CanvasFont{Size: 40, Weight: 800}; c.FillText("Text", 10, 65) }},
			{"strokeText", func(c *pen) { c.Font, c.LineWidth = CanvasFont{Size: 40, Weight: 800}, 3; c.StrokeText("Text", 10, 65) }},
			{"image", func(c *pen) { c.DrawImageSize(tileImage, 30, 30, 64, 64) }},
			{"pattern", func(c *pen) { c.FillStyle = c.CreatePattern(tileImage, gfx.RepeatBoth); c.FillRect(30, 30, 64, 64) }},
			{"gradient", func(c *pen) {
				g := c.CreateLinearGradient(30, 0, 90, 0)
				g.AddColorStop(0, rose)
				g.AddColorStop(1, amber.WithAlpha(0))
				c.FillStyle = g
				c.FillRect(30, 30, 64, 64)
			}},
			{"emoji", func(c *pen) { c.Font.Size = 60; c.FillText("😀", 20, 75) }},
		} {
			out = append(out, cell{op.String() + " " + n.name, func(c *pen) {
				c.Arc(50, 50, 30, 0, 2*math.Pi)
				c.Fill()
				c.FillStyle, c.StrokeStyle = rose.WithAlpha(0.8), rose
				c.GlobalCompositeOperation = op
				n.draw(c)
			}})
		}
	}
	return out
}

func shadowCells() []cell {
	with := func(blur, dx, dy float64, color gfx.Color, draw func(c *pen)) func(c *pen) {
		return func(c *pen) {
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = color, blur, dx, dy
			draw(c)
		}
	}
	rect := func(c *pen) { c.FillRect(30, 30, 40, 40) }
	var out []cell
	for _, b := range []float64{0.5, 1, 2, 3, 5, 8, 12, 20, 40, 80} {
		out = append(out, cell{"blur " + num(b), with(b, 0, 0, gfx.Black, rect)})
	}
	for _, b := range []float64{1, 3, 8, 20} {
		out = append(out,
			cell{"blur " + num(b) + " triangle", with(b, 0, 0, gfx.Black, func(c *pen) { c.MoveTo(50, 20); c.LineTo(80, 75); c.LineTo(20, 75); c.Fill() })},
			cell{"blur " + num(b) + " rounded", with(b, 0, 0, gfx.Black, func(c *pen) { c.RoundRect(25, 25, 50, 50, 20, 5); c.Fill() })},
			cell{"blur " + num(b) + " text", with(b, 5, 5, gfx.Black, func(c *pen) { c.Font = CanvasFont{Size: 40, Weight: 800}; c.FillText("Text", 5, 60) })},
			cell{"blur " + num(b) + " image", with(b, 5, 5, gfx.Black, func(c *pen) { c.DrawImage(photoImage, 15, 20) })},
			cell{"blur " + num(b) + " only", with(b, 200, 0, gfx.Black, func(c *pen) { c.FillRect(-170, 30, 40, 40) })},
			cell{"blur " + num(b) + " circle only", with(b, 200, 0, gfx.Black, func(c *pen) { c.Arc(-150, 50, 25, 0, 2*math.Pi); c.Fill() })})
	}
	return append(out, []cell{
		{"no offset no blur", with(0, 0, 0, gfx.Black, rect)},
		{"offset fraction", with(0, 5.5, 7.3, rose, rect)},
		{"negative offset", with(0, -15, -10, rose, rect)},
		{"negative blur", with(-5, 10, 10, rose, rect)},
		{"under a scale", with(6, 10, 10, rose, func(c *pen) { c.Scale(2, 0.5); c.FillRect(15, 60, 20, 80) })},
		{"under a turn", with(6, 10, 10, rose, func(c *pen) { c.Translate(50, 50); c.Rotate(0.5); c.FillRect(-20, -20, 40, 40) })},
		{"stroke", with(6, 8, 8, slate, func(c *pen) { c.LineWidth = 8; c.StrokeRect(20, 20, 50, 50) })},
		{"dashes", with(4, 8, 8, slate, func(c *pen) { c.LineWidth = 8; c.SetLineDash(12, 6); c.StrokeRect(20, 20, 50, 50) })},
		{"strokeText", with(4, 5, 5, slate, func(c *pen) { c.Font = CanvasFont{Size: 40, Weight: 800}; c.LineWidth = 3; c.StrokeText("Text", 5, 60) })},
		{"pattern", with(4, 8, 8, slate, func(c *pen) { c.FillStyle = c.CreatePattern(tileImage, gfx.RepeatBoth); c.FillRect(20, 20, 50, 50) })},
		{"gradient", with(4, 8, 8, slate, func(c *pen) {
			g := c.CreateRadialGradient(45, 45, 5, 45, 45, 25)
			g.AddColorStop(0, rose)
			g.AddColorStop(1, rose.WithAlpha(0))
			c.FillStyle = g
			c.FillRect(20, 20, 50, 50)
		})},
		{"translucent image", with(4, 8, 8, slate, func(c *pen) { c.DrawImageSize(tileImage, 20, 20, 50, 50) })},
		{"translucent color", with(4, 12, 12, rose.WithAlpha(0.4), rect)},
		{"translucent shape", with(0, 12, 12, rose, func(c *pen) { c.FillStyle = sky.WithAlpha(0.4); rect(c) })},
		{"shadow alpha", with(0, 12, 12, rose.WithAlpha(0.2), rect)},
		{"rect w=0", with(6, 8, 8, slate, func(c *pen) { c.LineWidth = 6; c.StrokeRect(50, 20, 0, 50) })},
		{"tiny", with(6, 8, 8, gfx.Black, func(c *pen) { c.FillRect(50, 50, 1, 1) })},
		{"overlapping draws", with(6, 8, 8, slate, func(c *pen) { rect(c); c.FillStyle = amber; c.FillRect(10, 10, 40, 40) })},
	}...)
}

// shadowOpCells draw a shadow with every operation.
func shadowOpCells() []cell {
	var out []cell
	for _, blur := range []float64{0, 8} {
		for i, o := range operations {
			out = append(out, cell{o.name + " blur " + num(blur), func(c *pen) {
				c.Arc(50, 50, 32, 0, 2*math.Pi)
				c.Fill()
				c.GlobalCompositeOperation = CompositeOperation(i)
				c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = rose.WithAlpha(0.6), blur, 15, 12
				c.FillStyle = amber.WithAlpha(0.8)
				c.FillRect(40, 30, 35, 40)
			}})
		}
	}
	return out
}

var baselineList = []TextBaseline{BaselineAlphabetic, BaselineTop, BaselineHanging, BaselineMiddle, BaselineIdeographic, BaselineBottom}

func baselineCells() []cell {
	var out []cell
	for _, f := range []struct{ family, text string }{{"Inter", "Hxgy"}, {"JetBrains Mono", "Hxgy"}, {"Source Serif 4", "Hxgy"}, {"Noto Sans Arabic", "مرحبا"}, {"Noto Color Emoji", "😀"}} {
		for _, b := range baselineList {
			out = append(out, cell{f.family + " " + baselineNames[b], func(c *pen) {
				c.LineWidth = 1
				c.MoveTo(0, 30.5)
				c.LineTo(160, 30.5)
				c.Stroke()
				c.TextBaseline = b
				x := 5.0
				for _, size := range []float64{13, 20, 27} {
					c.Font = CanvasFont{Family: f.family, Size: size}
					c.FillText(f.text, x, 30)
					x += 25 + size
				}
			}})
		}
	}
	return out
}

func textCells() []cell {
	draw := func(set func(c *pen), s string, x, y float64, maxWidth ...float64) func(c *pen) {
		return func(c *pen) {
			c.Font.Size = 24
			set(c)
			c.FillText(s, x, y, maxWidth...)
		}
	}
	stroke := func(set func(c *pen), s string, x, y float64, maxWidth ...float64) func(c *pen) {
		return func(c *pen) {
			c.Font = CanvasFont{Size: 40, Weight: 700}
			c.LineWidth = 4
			set(c)
			c.StrokeText(s, x, y, maxWidth...)
		}
	}
	none := func(c *pen) {}
	words := func(c *pen) *CanvasGradient {
		g := c.CreateLinearGradient(10, 0, 150, 0)
		g.AddColorStop(0, rose)
		g.AddColorStop(0.5, sky)
		g.AddColorStop(1, indigo)
		return g
	}
	var out []cell
	for _, a := range []TextAlign{TextAlignLeft, TextAlignCenter, TextAlignRight} {
		out = append(out,
			cell{"spaced " + alignNames[a], draw(func(c *pen) { c.TextAlign, c.LetterSpacing = a, 6 }, "Abc", 80, 40)},
			cell{"condensed " + alignNames[a], draw(func(c *pen) { c.TextAlign = a }, "Condensed text", 80, 40, 60)},
			cell{"condensed stroke " + alignNames[a], stroke(func(c *pen) { c.TextAlign = a }, "Wide", 80, 45, 60)},
			cell{"spaced stroke " + alignNames[a], stroke(func(c *pen) { c.TextAlign, c.LetterSpacing = a, 6 }, "Ab", 80, 45)})
	}
	for _, w := range []float64{0.5, 1, 2, 6, 10} {
		out = append(out, cell{"stroke " + num(w), stroke(func(c *pen) { c.LineWidth = w }, "AMgw", 5, 45)})
	}
	return append(out, []cell{
		{"round joins", stroke(func(c *pen) { c.LineWidth, c.LineJoin = 8, gfx.JoinRound }, "AMw", 10, 45)},
		{"bevel joins", stroke(func(c *pen) { c.LineWidth, c.LineJoin = 8, gfx.JoinBevel }, "AMw", 10, 45)},
		{"miter limit 2", stroke(func(c *pen) { c.LineWidth, c.MiterLimit = 8, 2 }, "AMw", 10, 45)},
		{"stroke serif", stroke(func(c *pen) { c.Font.Family, c.LineWidth = "Source Serif 4", 1 }, "Rags", 10, 45)},
		{"stroke mono", stroke(func(c *pen) { c.Font.Family, c.LineWidth = "JetBrains Mono", 1 }, "R0@", 10, 45)},
		{"stroke arabic", stroke(func(c *pen) { c.LineWidth = 1 }, "مرحبا", 10, 40)},
		{"stroke overlaps", stroke(func(c *pen) { c.LineWidth, c.Font.Weight = 1, 900 }, "Aét48", 5, 45)},
		{"stroke translucent", stroke(func(c *pen) { c.StrokeStyle = rose.WithAlpha(0.4); c.LetterSpacing = -6 }, "MAWN", 5, 45)},
		{"stroke gradient", stroke(func(c *pen) { c.StrokeStyle = words(c) }, "Wide", 10, 45)},
		{"stroke gradient condensed", stroke(func(c *pen) { c.StrokeStyle = words(c) }, "Wide words", 10, 45, 140)},
		{"gradient", draw(func(c *pen) { c.FillStyle, c.Font.Weight = words(c), 900 }, "Gradient", 10, 40)},
		{"gradient condensed", draw(func(c *pen) { c.FillStyle, c.Font.Weight = words(c), 900 }, "Gradient condensed", 10, 40, 140)},
		{"pattern condensed", draw(func(c *pen) { c.FillStyle, c.Font.Weight = c.CreatePattern(tileImage, gfx.RepeatBoth), 900 }, "Pattern condensed", 10, 40, 140)},
		{"conic condensed", draw(func(c *pen) {
			g := c.CreateConicGradient(0, 80, 30)
			g.AddColorStop(0, rose)
			g.AddColorStop(1, sky)
			c.FillStyle, c.Font.Weight = g, 900
		}, "Conic condensed", 10, 40, 140)},
		{"maxWidth enough", draw(none, "Enough", 10, 40, 150)},
		{"maxWidth 0", draw(none, "Nothing", 10, 40, 0)},
		{"maxWidth negative", draw(none, "Nothing", 10, 40, -5)},
		{"condensed to 1", draw(none, "Nearly nothing", 10, 40, 1)},
		{"condensed spaced", draw(func(c *pen) { c.LetterSpacing = 5 }, "Spaced text", 10, 40, 100)},
		{"spacing 0.3", draw(func(c *pen) { c.LetterSpacing = 0.3 }, "Spacing", 10, 40)},
		{"spacing ligatures", draw(func(c *pen) { c.LetterSpacing, c.Font.Family = 3, "JetBrains Mono" }, "-> != ffi", 5, 40)},
		{"ligatures", draw(func(c *pen) { c.Font.Family = "JetBrains Mono" }, "-> != ffi", 5, 40)},
		{"spacing arabic", draw(func(c *pen) { c.LetterSpacing = 4 }, "مرحبا", 10, 40)},
		{"spacing emoji", draw(func(c *pen) { c.LetterSpacing = 4 }, "a👩‍🚀b🎉", 10, 40)},
		{"kerning", draw(none, "AVATo.Ty", 10, 40)},
		{"combining", draw(none, "e\u0301 a\u030a n\u0303", 10, 40)},
		{"bidi", draw(none, "abc مرحبا 123 def", 5, 40, 150)},
		{"small", draw(func(c *pen) { c.Font.Size = 9 }, "Small text of nine pixels", 5, 40)},
		{"fraction size", draw(func(c *pen) { c.Font.Size = 13.7 }, "Size of 13.7 pixels", 5, 40)},
		{"fraction place", draw(func(c *pen) { c.Font.Size = 14 }, "Placed at 10.4, 40.6", 10.4, 40.6)},
		{"large", draw(func(c *pen) { c.Font.Size = 60 }, "Big", 10, 50)},
		{"huge", draw(func(c *pen) { c.Font.Size = 300 }, "B", -20, 180)},
		{"bold", draw(func(c *pen) { c.Font.Weight = 700 }, "Bold", 10, 40)},
		{"weight 350", draw(func(c *pen) { c.Font.Weight = 350 }, "Weight 350", 10, 40)},
		{"italic", draw(func(c *pen) { c.Font.Italic = true }, "Italic", 10, 40)},
		{"serif italic", draw(func(c *pen) { c.Font.Family, c.Font.Italic, c.Font.Weight = "Source Serif 4", true, 600 }, "Italic serif", 10, 40)},
		{"synthetic italic", draw(func(c *pen) { c.Font.Italic = true }, "مرحبا", 10, 40)},
		{"scaled", draw(func(c *pen) { c.Scale(1.5, 1.5); c.Font.Size = 16 }, "Scaled", 5, 25)},
		{"scaled x", draw(func(c *pen) { c.Scale(2, 1); c.Font.Size = 16 }, "Scaled", 5, 40)},
		{"scaled down", draw(func(c *pen) { c.Scale(0.25, 0.25); c.Font.Size = 96 }, "Down", 40, 160)},
		{"turned", draw(func(c *pen) { c.Translate(20, 10); c.Rotate(0.2); c.Font.Size = 18 }, "Turned text", 0, 20)},
		{"skewed", draw(func(c *pen) { c.Transform(1, 0, -0.4, 1, 20, 0) }, "Skewed", 10, 40)},
		{"mirrored", draw(func(c *pen) { c.Translate(150, 0); c.Scale(-1, 1) }, "Mirrored", 10, 40)},
		{"upside down", draw(func(c *pen) { c.Translate(0, 60); c.Scale(1, -1) }, "Upside", 10, 30)},
		{"shadow", draw(func(c *pen) {
			c.ShadowColor, c.ShadowBlur, c.ShadowOffsetX, c.ShadowOffsetY = slate, 4, 4, 4
			c.Font.Weight = 800
		}, "Shadow", 10, 40)},
		{"white space", draw(func(c *pen) { c.Font.Size = 16 }, " a  b\tc\nd\fe ", 10, 40)},
		{"rtl mark", draw(none, "\u200fabc!", 10, 40)},
		{"emoji x+0.25", draw(none, "🎉", 10.25, 40)},
		{"emoji x+0.5", draw(none, "🎉", 10.5, 40)},
		{"emoji x+0.75", draw(none, "🎉", 10.75, 40)},
		{"emoji y+0.25", draw(none, "🎉", 10, 40.25)},
		{"emoji y+0.5", draw(none, "🎉", 10, 40.5)},
		{"emoji y+0.75", draw(none, "🎉", 10, 40.75)},
		{"emoji turned", draw(func(c *pen) { c.Rotate(-0.1) }, "🎉", 10, 50)},
		{"emoji after text", draw(none, "a🎉", 10, 40)},
	}...)
}
