package tela_test

import (
	"fmt"
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
