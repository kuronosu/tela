package core

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The drawings of the gallery and of the damage scenes.

// drawRing is the gauge of h.Canvas's documentation, progress of it done.
func drawRing(progress float64) func(*canvas.Canvas) {
	return func(c *canvas.Canvas) {
		r := min(c.Width, c.Height)/2 - 6
		c.LineWidth, c.LineCap = 8, gfx.CapRound
		c.GlobalAlpha = 0.2
		c.BeginPath()
		c.Arc(c.Width/2, c.Height/2, r, 0, 2*math.Pi)
		c.Stroke()
		c.GlobalAlpha = 1
		c.BeginPath()
		c.Arc(c.Width/2, c.Height/2, r, -math.Pi/2, (2*progress-0.5)*math.Pi)
		c.Stroke()
	}
}

// drawChart is an area chart of n points, shifted by phase.
func drawChart(n int, phase float64) func(*canvas.Canvas) {
	return func(c *canvas.Canvas) {
		at := func(i int) (x, y float64) {
			u := float64(i) / float64(n-1)
			return u * c.Width, c.Height * (0.5 - 0.35*math.Sin(u*9+phase)*math.Cos(u*4))
		}
		c.StrokeStyle = c.Color("border")
		for i := range 5 {
			y := math.Round(float64(i)*c.Height/4) + 0.5
			c.BeginPath()
			c.MoveTo(0, y)
			c.LineTo(c.Width, y)
			c.Stroke()
		}
		c.BeginPath()
		for i := range n {
			c.LineTo(at(i))
		}
		c.Save()
		c.LineTo(c.Width, c.Height)
		c.LineTo(0, c.Height)
		c.GlobalAlpha = 0.25
		c.Fill()
		c.Restore()
		c.BeginPath()
		for i := range n {
			c.LineTo(at(i))
		}
		c.StrokeStyle, c.LineWidth, c.LineJoin = c.FillStyle, 2, gfx.JoinRound
		c.Stroke()
	}
}

// drawStar is a star that crosses itself, under the rule, turned by angle.
func drawStar(rule canvas.FillRule, angle float64) func(*canvas.Canvas) {
	return func(c *canvas.Canvas) {
		c.Translate(c.Width/2, c.Height/2)
		c.Rotate(angle)
		c.Scale(c.Width/2-2, c.Height/2-2)
		c.BeginPath()
		for i := range 5 {
			a := float64(i)*4*math.Pi/5 - math.Pi/2
			c.LineTo(math.Cos(a), math.Sin(a))
		}
		c.ClosePath()
		c.FillRule = rule
		c.Fill()
		c.LineWidth = 0.06
		c.Stroke()
	}
}

// drawClips paints stripes inside a circle, and what is left open.
func drawClips(c *canvas.Canvas) {
	c.Save()
	c.Arc(c.Width/2, c.Height/2, c.Width/2-4, 0, 2*math.Pi)
	c.Clip()
	c.Rotate(math.Pi / 4)
	for i := -10; i < 20; i += 2 {
		c.FillRect(float64(i)*6, -c.Height, 6, 3*c.Height)
	}
	c.Restore()
	c.StrokeRect(0.5, 0.5, c.Width-1, c.Height-1)
	c.Clip()
	c.Save()
}

// drawLines shows the caps and the joins.
func drawLines(c *canvas.Canvas) {
	c.LineWidth = 10
	for i, lc := range []canvas.LineCap{gfx.CapButt, gfx.CapRound, gfx.CapSquare} {
		c.LineCap = lc
		c.BeginPath()
		y := 14 + float64(i)*18
		c.MoveTo(14, y)
		c.LineTo(60, y)
		c.Stroke()
	}
	c.LineCap = gfx.CapButt
	for i, j := range []canvas.LineJoin{gfx.JoinMiter, gfx.JoinRound, gfx.JoinBevel} {
		c.LineJoin = j
		c.BeginPath()
		x := 80 + float64(i)*42
		c.MoveTo(x, 56)
		c.LineTo(x+14, 16)
		c.LineTo(x+28, 56)
		c.Stroke()
	}
	c.Transform(1, 0, 0.4, 0.25, 70, 64)
	c.LineWidth = 6
	c.StrokeRect(-40, 8, 120, 30)
}

func canvasGallery() Node {
	cell := func(label string, n Node) Node {
		return div(Class("flex flex-col items-center gap-1 text-xs text-slate-500"), n, Text(label))
	}
	chart := "w-56 h-24 rounded-xl border border-slate-300 bg-white p-2 fill-indigo-600 shadow-md"
	page := func(scheme ColorScheme) Node {
		return div(SchemeProp(scheme), Class("flex flex-wrap gap-4 p-4 bg-background"),
			cell("stroke-primary", canvasNode(drawRing(0.7), Class("size-24 stroke-primary"))),
			cell("text color", canvasNode(drawRing(0.35), Class("size-24 text-rose-500"))),
			cell("chart", canvasNode(drawChart(60, 0), Class(chart))),
			cell("non-zero", canvasNode(drawStar(gfx.NonZero, 0), Class("size-20 fill-amber-300 stroke-amber-600"))),
			cell("even-odd", canvasNode(drawStar(gfx.EvenOdd, 0.3), Class("size-20 fill-amber-300 stroke-none"))),
			cell("clips", canvasNode(drawClips, Class("size-20 rounded-3xl fill-sky-500 stroke-slate-500 bg-sky-100"))),
			cell("lines", canvasNode(drawLines, Class("w-52 h-28 stroke-emerald-600"))),
			cell("rotated", canvasNode(drawRing(0.9), Class("size-20 rotate-12 bg-fuchsia-100 rounded-2xl p-1 opacity-80 stroke-fuchsia-600"))),
			cell("rounded-full", canvasNode(flood, Class("size-20 rounded-full p-2 border-4 border-slate-400 fill-teal-500"))),
			cell("fill-none", canvasNode(drawStar(gfx.NonZero, 0), Class("size-20 fill-none stroke-slate-500"))),
		)
	}
	return div(page(ColorSchemeLight), page(ColorSchemeDark))
}

// Regenerate with TELA_UPDATE=1 and look at them.
func TestCanvasGoldens(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			_, img := Render(canvasGallery(), RenderOptions{Config: Config{Fonts: testFonts}, Width: 640, Height: 824, Scale: scale})
			golden.Image(t, fmt.Sprintf("canvas_x%g", scale), img)
		})
	}
}

// canvasScene shows canvases under what an element can have around it, some
// of them moving on their own, with Values, states and styles that change.
func canvasScene(st *sceneState) Node {
	comp := func() Node {
		st.refs[0], st.refs[1] = UseElementRef(), UseElementRef()
		for i := range st.values {
			st.values[i] = UseValue[float32](0)
		}
		st.values[1].Get()
		v, set := UseState(0)
		w, setW := UseState(0)
		st.setters[0], st.setters[1] = set, setW
		x, o, k := st.values[0], st.values[1], st.values[2]
		cell := func(class string, draw func(*canvas.Canvas)) Node {
			return div(Class("m-1 h-20 w-24 shrink-0"), canvasNode(draw, Class("size-full "+class)))
		}
		read := func(f func(float64) func(*canvas.Canvas)) func(*canvas.Canvas) {
			return func(c *canvas.Canvas) { f(float64(k.Get()) / 40)(c) }
		}
		loop := func(every int) func(*canvas.Canvas) {
			return func(c *canvas.Canvas) {
				drawStar(gfx.EvenOdd, c.Time.Seconds()*3)(c)
				if w%every != every-1 {
					c.RequestFrame()
				}
			}
		}
		list := []Child{AttachRef(st.refs[1]), Key("list"), Class("h-28 overflow-y-auto bg-white")}
		for i := range 12 {
			list = append(list, div(Key(i), Class("flex h-9 items-center gap-2 px-2 text-sm odd:bg-slate-50 hover:bg-sky-100"),
				canvasNode(drawChart(12, float64(i+v)), Class("h-7 w-20 fill-indigo-500")), Textf("Row %d", i)))
		}
		kids := []Child{AttachRef(st.refs[0]), Class("h-screen overflow-y-auto bg-linear-to-br from-white to-indigo-200"),
			div(Class("flex flex-wrap"),
				cell("rounded-2xl border-2 border-indigo-500 shadow-lg stroke-indigo-600 hover:stroke-rose-500 transition-colors", read(drawRing)),
				cell("opacity-60 bg-amber-200 text-amber-700", loop(2)),
				cell("rotate-12 rounded-lg fill-sky-500", drawChart(20, float64(v))),
				cell("blur-[2px] fill-emerald-500", loop(3)),
				cell("mix-blend-multiply p-2 fill-rose-400", read(func(u float64) func(*canvas.Canvas) { return drawStar(gfx.NonZero, u) })),
				cell("mask-linear-to-r mask-from-50% rounded-full fill-violet-600", flood),
				div(Class("relative m-1 h-20 w-24"), canvasNode(loop(5), Class("size-full fill-fuchsia-500")),
					div(Class("absolute inset-3 rounded-xl border border-white/50 bg-white/20 p-1 text-xs backdrop-blur-sm"), Text("glass"))),
			),
			div(list...),
			div(Class("m-2 h-16 w-40 rounded-xl overflow-hidden"),
				Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }), style.OpacityFn(func() float32 { return o.Get() })),
				canvasNode(read(drawRing), Class("size-full hover:scale-110 transition stroke-teal-600"))),
			div(Style(style.TextColorFn(func() gfx.Color { return gfx.Color{R: k.Get() / 120, A: 1} })), Class("flex h-16"), canvasNode(drawClips, Class("size-16"))),
		}
		if v%4 == 3 {
			kids = append(kids, canvasNode(read(drawRing), Key("more"), Class("h-12 w-full stroke-slate-600"), LayoutID("c")))
		} else {
			kids = append(kids, div(Class("ml-10"), canvasNode(read(drawRing), Key("more"), Class("size-20 stroke-slate-600"), LayoutID("c"))))
		}
		return div(kids...)
	}
	return Component(comp)
}

// Incremental frames are bit-identical to full ones while canvases draw.
func TestCanvasDamageMatchesFullRender(t *testing.T) {
	t.Parallel()
	steps := 120
	if testing.Short() {
		steps = 30
	}
	runSceneWith(t, "canvas", canvasScene, commonOps, Config{InstantScroll: true, Dev: true}, steps, sceneScales(0), nil)
	runSceneWith(t, "canvas_smooth", canvasScene, commonOps, Config{}, steps, sceneScales(2), nil)
}

// canvasParticles draws 2000 commands.
func canvasParticles(c *canvas.Canvas) {
	t := c.Time.Seconds()
	for i := range 500 {
		x := math.Mod(float64(i*37)+t*40, c.Width)
		y := float64(i*53%300) + 10*math.Sin(t+float64(i))
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
	c.RequestFrame()
}

// BenchmarkCanvasRedraw2000 is a frame of a canvas that moves 2000 commands
// (500 particles of a rectangle, a circle, a line and an outline), up to the
// composed frame: the draw, the comparison with the last list, the recording
// of its group and the composition. The rasterization is
// BenchmarkCanvasRaster2000's.
func BenchmarkCanvasRedraw2000(b *testing.B) {
	tr := newTestTree(b, 400, 300, canvasNode(canvasParticles, Class("size-full fill-sky-500 stroke-slate-700")), nil)
	tr.SetRequestFrame(func() {})
	tr.Frame()
	b.ReportAllocs()
	for i := 1; b.Loop(); i++ {
		tr.Animate(time.Duration(i) * time.Second / 60)
		if _, changed := tr.Frame(); !changed {
			b.Fatal("the frame drew nothing")
		}
	}
	if n := tr.lay.recorded; n != 1 {
		b.Fatalf("%d lists recorded", n)
	}
}

// BenchmarkCanvasRedrawEqual2000 is the frame of a redraw of 2000 commands
// that draws the same.
func BenchmarkCanvasRedrawEqual2000(b *testing.B) {
	tr := newTestTree(b, 400, 300, canvasNode(func(c *canvas.Canvas) {
		c.Time = 0
		canvasParticles(c)
	}, Class("size-full fill-sky-500 stroke-slate-700")), nil)
	tr.SetRequestFrame(func() {})
	tr.Frame()
	b.ReportAllocs()
	for i := 1; b.Loop(); i++ {
		tr.Animate(time.Duration(i) * time.Second / 60)
		tr.Frame()
	}
	if n := tr.lay.recorded; n != 0 {
		b.Fatalf("%d lists recorded", n)
	}
}

// BenchmarkCanvasRaster2000 rasterizes the frame of the particles at scale 2.
func BenchmarkCanvasRaster2000(b *testing.B) {
	tr := newTestTree(b, 400, 300, canvasNode(canvasParticles, Class("size-full fill-sky-500 stroke-slate-700")), nil)
	dl, _ := tr.Frame()
	for b.Loop() {
		tr.Rasterize(dl, 2)
	}
}
