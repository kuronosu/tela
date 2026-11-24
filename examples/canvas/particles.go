package main

import (
	"math"
	"math/rand/v2"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"

	"github.com/kuronosu/tela/examples/canvas/icons"
)

// particle is one dot of the field: where it is, how fast it moves and which
// of the three colors it has.
type particle struct {
	x, y, vx, vy float64
	r            float64
	tone         int
}

// pointer is what the field knows of the pointer, which an event handler
// writes and the draw reads.
type pointer struct {
	x, y       float64
	over, down bool
}

// field is the simulation. It lives in a UseRef: what a draw moves is what
// the next one starts from.
type field struct {
	ps   []particle
	w, h float64
	rng  *rand.Rand
}

const (
	linkDistance = 84.0  // pixels between two dots that a line joins
	reach        = 230.0 // pixels from the pointer where it pulls
)

// resize makes the field fit a canvas: the dots appear on the first call,
// from a fixed seed so that a picture of the screen is always the same one,
// and stay in their place on the others.
func (f *field) resize(w, h float64) {
	if f.rng == nil {
		f.rng = rand.New(rand.NewPCG(7, 1))
		n := int(math.Min(110, math.Max(40, w*h/3600)))
		for range n {
			f.ps = append(f.ps, particle{
				x: f.rng.Float64() * w, y: f.rng.Float64() * h,
				vx: f.rng.NormFloat64() * 14, vy: f.rng.NormFloat64() * 14,
				r: 1.6 + f.rng.Float64()*2, tone: f.rng.IntN(3),
			})
		}
	} else if f.w > 0 && f.h > 0 && (f.w != w || f.h != h) {
		for i := range f.ps {
			f.ps[i].x *= w / f.w
			f.ps[i].y *= h / f.h
		}
	}
	f.w, f.h = w, h
}

// step moves every dot for dt seconds: a slow drift that keeps them turning
// round the middle, a pull to the pointer when it is near (stronger while it
// is pressed), a little friction and the walls.
func (f *field) step(dt float64, p pointer) {
	damp := math.Pow(0.4, dt)
	for i := range f.ps {
		q := &f.ps[i]
		dx, dy := q.x-f.w/2, q.y-f.h/2
		d := math.Hypot(dx, dy) + 1
		q.vx += (-dy/d*14 - dx/d*3) * dt
		q.vy += (dx/d*14 - dy/d*3) * dt
		if p.over {
			px, py := p.x-q.x, p.y-q.y
			pd := math.Hypot(px, py) + 1
			if pd < reach {
				pull := 300.0 * (1 - pd/reach)
				if p.down {
					pull *= 1.8
				}
				if pd < 70 { // a ring round the pointer, not a clump in it
					pull -= 640 * (1 - pd/70)
				}
				q.vx += (px/pd*pull - py/pd*90*(1-pd/reach)) * dt
				q.vy += (py/pd*pull + px/pd*90*(1-pd/reach)) * dt
			}
		}
		q.vx *= damp
		q.vy *= damp
		if s := math.Hypot(q.vx, q.vy); s > 320 {
			q.vx, q.vy = q.vx/s*320, q.vy/s*320
		}
		q.x += q.vx * dt
		q.y += q.vy * dt
		if q.x < 0 || q.x > f.w {
			q.x, q.vx = math.Min(math.Max(q.x, 0), f.w), -q.vx*0.8
		}
		if q.y < 0 || q.y > f.h {
			q.y, q.vy = math.Min(math.Max(q.y, 0), f.h), -q.vy*0.8
		}
	}
}

// draw paints the lines between the dots that are near each other, a halo
// under the pointer and the dots.
func (f *field) draw(c *tela.Canvas, p pointer) {
	tones := [3]tela.Color{c.Color("chart-1"), c.Color("chart-2"), c.Color("chart-3")}

	if p.over {
		halo := c.CreateRadialGradient(p.x, p.y, 0, p.x, p.y, reach/1.6)
		halo.AddColorStop(0, tones[0].WithAlpha(0.28))
		halo.AddColorStop(1, tones[0].WithAlpha(0))
		c.FillStyle = halo
		c.FillRect(p.x-reach, p.y-reach, 2*reach, 2*reach)
	}

	c.LineWidth = 1
	for i := range f.ps {
		a := &f.ps[i]
		for j := i + 1; j < len(f.ps); j++ {
			b := &f.ps[j]
			d := math.Hypot(a.x-b.x, a.y-b.y)
			if d >= linkDistance {
				continue
			}
			c.StrokeStyle = tones[a.tone].WithAlpha(float32(0.5 * (1 - d/linkDistance)))
			c.BeginPath()
			c.MoveTo(a.x, a.y)
			c.LineTo(b.x, b.y)
			c.Stroke()
		}
	}
	for i := range f.ps {
		q := &f.ps[i]
		c.FillStyle = tones[q.tone]
		c.BeginPath()
		c.Arc(q.x, q.y, q.r, 0, 2*math.Pi)
		c.Fill()
	}
}

// drawn is called at the end of every draw of the field, for tests.
var drawn func(f *field)

// ParticlesCard is the field and the button that pauses it. Still starts it
// paused, as reduced motion does.
type ParticlesCard struct{ Still bool }

func (pc ParticlesCard) Build() tela.Node {
	reduced := tela.UseReducedMotion()
	running, setRunning := tela.UseState(!pc.Still && !reduced)
	sim := tela.UseRef(&field{})
	ptr := tela.UseRef(pointer{})

	label, icon := "Pause", icons.Pause
	if !running {
		label, icon = "Play", icons.Play
	}
	status := "Running"
	if !running {
		status = "Paused"
	}

	return kit.Card{
		TitleLevel: 2,
		Class:      "min-w-0", HeaderClass: tight, ContentClass: tight, FooterClass: tight,
		Title:       cardTitle(icons.Sparkles, "Particles"),
		Description: Text("Move the pointer over the field; press to pull harder."),
		Action: kit.Button{Variant: kit.Outline, Size: kit.Sm, Icon: icon, Children: Text(label),
			OnClick: func() { setRunning.Set(!running) }},
		Children: Div(Class("flex flex-col gap-6"),
			Div(Class("h-44 w-full overflow-hidden rounded-lg border border-border bg-muted/40"), Canvas(func(c *tela.Canvas) {
				f := sim.Current
				f.resize(c.Width, c.Height)
				if running {
					f.step(math.Min(c.Delta.Seconds(), 0.05), ptr.Current)
				}
				f.draw(c, ptr.Current)
				if running {
					c.RequestFrame()
				}
				if drawn != nil {
					drawn(f)
				}
			},
				Class("block size-full cursor-crosshair"),
				TestID("particles"), Role("img"), AriaLabel("Particles that follow the pointer"),
				OnPointerMove(func(e tela.PointerEvent) {
					ptr.Current = pointer{x: float64(e.LocalX), y: float64(e.LocalY), over: true, down: e.Buttons != 0}
				}),
				OnPointerDown(func(e tela.PointerEvent) {
					ptr.Current = pointer{x: float64(e.LocalX), y: float64(e.LocalY), over: true, down: true}
				}),
				OnPointerUp(func(e tela.PointerEvent) {
					ptr.Current = pointer{x: float64(e.LocalX), y: float64(e.LocalY), over: true}
				}),
				OnPointerLeave(func() { ptr.Current = pointer{} }))),
			Div(Class("flex h-8 items-center gap-2 text-xs text-muted-foreground"), Role("status"),
				Span(Class("size-2 rounded-full "+pick(running, "bg-emerald-500", "bg-muted-foreground/50"))),
				Text(status+pick(running, ": a frame at every vsync.", ": no frames, no CPU.")))),
	}
}

// pick is a conditional string.
func pick(ok bool, a, b string) string {
	if ok {
		return a
	}
	return b
}
