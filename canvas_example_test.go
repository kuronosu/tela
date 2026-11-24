package tela_test

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// A gauge that follows a Value: a change draws it again, and builds nothing.
func ExampleCanvas() {
	t := &exampleT{}
	defer t.close()
	gauge := func() tela.Node {
		v := tela.UseValue(0.25)
		return Div(
			Canvas(func(c *tela.Canvas) {
				r := min(c.Width, c.Height)/2 - 8
				c.LineWidth, c.LineCap = 8, tela.CapButt
				c.BeginPath()
				c.Arc(c.Width/2, c.Height/2, r, 0, 2*math.Pi*v.Get())
				c.Stroke()
			}, Class("size-32 stroke-[#4f46e5]"), AriaLabel("Progress")),
			Button(OnClick(func() { v.Set(0.75) }), Text("More")))
	}
	app := telatest.Mount(t, tela.C(gauge))
	fmt.Println(app.AriaSnapshot())
	fmt.Println(app.PixelAt(8, 64)) // the left of the ring
	app.Click(telatest.ByRole("button", "More"))
	fmt.Println(app.PixelAt(8, 64))
	// Output:
	// - img "Progress"
	// - button "More"
	// {255 255 255 255}
	// {79 70 229 255}
}

// The state of a simulation is kept in a UseRef, and moved by the time since
// the last draw.
func ExampleCanvas_RequestFrame() {
	t := &exampleT{}
	defer t.close()
	var x float64
	ball := func() tela.Node {
		pos := tela.UseRef(0.0)
		return Canvas(func(c *tela.Canvas) {
			pos.Current += 60 * c.Delta.Seconds() // 60 px per second
			x = pos.Current
			c.BeginPath()
			c.Arc(pos.Current, c.Height/2, 12, 0, 2*math.Pi)
			c.Fill()
			if pos.Current < c.Width {
				c.RequestFrame() // the window sleeps once it is out of view
			}
		}, Class("w-30 h-16 fill-sky-500"))
	}
	app := telatest.Mount(t, tela.C(ball))
	for range 3 {
		app.Advance(500 * time.Millisecond)
		fmt.Printf("%.0f\n", x)
	}
	app.Advance(10 * time.Second)
	app.ExpectIdle()
	// Output:
	// 30
	// 60
	// 90
}

func ExampleCanvas_Color() {
	t := &exampleT{}
	defer t.close()
	bars := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			for i, token := range []string{"primary", "red-500"} {
				c.FillStyle = c.Color(token)
				c.FillRect(float64(i)*c.Width/2, 0, c.Width/2, c.Height)
			}
		}, Class("w-20 h-10"))
	}
	for _, scheme := range []tela.ColorScheme{tela.Light, tela.Dark} {
		app := telatest.Mount(t, tela.C(bars), telatest.ColorScheme(scheme))
		fmt.Println(app.PixelAt(10, 5), app.PixelAt(60, 5))
	}
	// Output:
	// {23 23 23 255} {251 44 54 255}
	// {229 229 229 255} {251 44 54 255}
}

func ExampleCanvas_Save() {
	t := &exampleT{}
	defer t.close()
	fan := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			c.Translate(c.Width/2, c.Height/2)
			for i := range 4 {
				c.Save()
				c.Rotate(float64(i) * math.Pi / 2)
				c.FillRect(20, -5, 30, 10)
				c.Restore()
			}
		}, Class("size-30"))
	}
	app := telatest.Mount(t, tela.C(fan))
	fmt.Println(app.PixelAt(95, 60).A, app.PixelAt(95, 60).R, app.PixelAt(60, 95).R, app.PixelAt(60, 60).R)
	// Output: 255 0 0 255
}

func ExampleCanvas_Clip() {
	t := &exampleT{}
	defer t.close()
	view := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			c.Arc(c.Width/2, c.Height/2, c.Width/2, 0, 2*math.Pi)
			c.Clip()
			c.FillStyle = c.Color("emerald-500")
			c.FillRect(0, 0, c.Width, c.Height/2)
		}, Class("size-20"))
	}
	app := telatest.Mount(t, tela.C(view))
	fmt.Println(app.PixelAt(40, 20), app.PixelAt(2, 2), app.PixelAt(40, 60))
	// Output: {0 188 125 255} {255 255 255 255} {255 255 255 255}
}

// A gradient along the bars of a chart, whatever their number.
func ExampleCanvasGradient() {
	t := &exampleT{}
	defer t.close()
	bars := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			g := c.CreateLinearGradient(0, c.Height, 0, 0) // from the bottom up
			g.AddColorStop(0, tela.Hex("#000000"))
			g.AddColorStop(1, tela.Hex("#ff0000"))
			c.FillStyle = g
			for i, v := range []float64{0.25, 1, 0.5} {
				c.FillRect(float64(i)*20, c.Height*(1-v), 18, c.Height*v)
			}
		}, Class("w-15 h-25"))
	}
	app := telatest.Mount(t, tela.C(bars))
	fmt.Println(app.PixelAt(25, 1), app.PixelAt(25, 50), app.PixelAt(5, 50))
	// Output: {251 0 0 255} {126 0 0 255} {255 255 255 255}
}

// An image that repeats behind what is drawn.
func ExampleCanvasPattern() {
	t := &exampleT{}
	defer t.close()
	dots := image.NewRGBA(image.Rect(0, 0, 10, 10))
	dots.SetRGBA(5, 5, color.RGBA{255, 0, 0, 255})
	paper := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			c.FillStyle = c.CreatePattern(dots, tela.Repeat)
			c.FillRect(0, 0, c.Width, c.Height)
		}, Class("w-40 h-20"))
	}
	app := telatest.Mount(t, tela.C(paper))
	fmt.Println(app.PixelAt(5, 5), app.PixelAt(75, 45), app.PixelAt(10, 10))
	// Output: {255 0 0 255} {255 0 0 255} {255 255 255 255}
}

// Text in the font of the element, in the middle of the canvas, over a box
// as wide as it.
func ExampleCanvas_FillText() {
	t := &exampleT{}
	defer t.close()
	badge := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			c.Font.Weight = 600 // the family and the size are the element's
			c.TextAlign, c.TextBaseline = tela.TextAlignCenter, tela.BaselineMiddle
			w := c.MeasureText("42 %").Width + 16
			c.BeginPath()
			c.RoundRect((c.Width-w)/2, 10, w, 20, 10)
			c.FillStyle = c.Color("sky-500")
			c.Fill()
			c.FillStyle = tela.White
			c.FillText("42 %", c.Width/2, 20)
			fmt.Printf("%.0f wide\n", w)
		}, Class("w-40 h-10 text-sm"))
	}
	app := telatest.Mount(t, tela.C(badge))
	fmt.Println(app.PixelAt(80, 11), app.PixelAt(80, 5))
	// Output:
	// 52 wide
	// {0 166 244 255} {255 255 255 255}
}

// A click finds the shape under it, without a draw.
func ExamplePath2D() {
	t := &exampleT{}
	defer t.close()
	slice := func(from, to float64) *tela.Path2D {
		p := new(tela.Path2D)
		p.MoveTo(50, 50)
		p.Arc(50, 50, 40, from*2*math.Pi, to*2*math.Pi)
		p.ClosePath()
		return p
	}
	slices := []*tela.Path2D{slice(0, 0.25), slice(0.25, 1)}
	pie := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			for i, p := range slices {
				c.FillStyle = c.Color([]string{"sky-500", "rose-500"}[i])
				c.FillPath(p)
			}
		}, Class("size-25"), AriaLabel("Sales"), OnClick(func(e tela.MouseEvent) {
			for i, p := range slices {
				if p.Contains(float64(e.LocalX), float64(e.LocalY)) {
					fmt.Println("slice", i)
				}
			}
		}))
	}
	app := telatest.Mount(t, tela.C(pie))
	app.ClickAt(70, 70)
	app.ClickAt(30, 30)
	app.ClickAt(95, 5) // outside both
	// Output:
	// slice 0
	// slice 1
}

func ExampleParsePath() {
	play, err := tela.ParsePath("M8 5v14l11-7z")
	fmt.Println(play.Contains(10, 12), play.Contains(5, 12), err)
	_, err = tela.ParsePath("M8 5 14l")
	fmt.Println(err)
	// Output:
	// true false <nil>
	// tela.ParsePath: path data has "l" at byte 7, where a number was expected
}

// An eraser: what it passes over shows what is behind the canvas.
func ExampleCompositeOperation() {
	t := &exampleT{}
	defer t.close()
	scratch := func() tela.Node {
		return Div(Class("w-25 h-12 bg-[#00ff00]"),
			Canvas(func(c *tela.Canvas) {
				c.FillStyle = tela.Hex("#888888")
				c.FillRect(0, 0, c.Width, c.Height)
				c.GlobalCompositeOperation = tela.CompositeDestinationOut
				c.LineWidth, c.LineCap = 20, tela.CapRound
				c.BeginPath()
				c.MoveTo(20, 24)
				c.LineTo(60, 24)
				c.Stroke()
			}, Class("size-full")))
	}
	app := telatest.Mount(t, tela.C(scratch))
	fmt.Println(app.PixelAt(40, 24), app.PixelAt(90, 24))
	// Output: {0 255 0 255} {136 136 136 255}
}

// Shadows follow the shape, also that of text and images.
func ExampleCanvas_shadow() {
	t := &exampleT{}
	defer t.close()
	card := func() tela.Node {
		return Canvas(func(c *tela.Canvas) {
			c.ShadowColor = tela.Black.WithAlpha(0.5)
			c.ShadowOffsetX, c.ShadowOffsetY = 10, 10
			c.FillStyle = tela.Hex("#ff0000")
			c.FillRect(10, 10, 40, 40)
		}, Class("size-20"))
	}
	app := telatest.Mount(t, tela.C(card))
	fmt.Println(app.PixelAt(30, 30), app.PixelAt(55, 55), app.PixelAt(5, 5))
	// Output: {255 0 0 255} {127 127 127 255} {255 255 255 255}
}
