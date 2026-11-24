package core

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

func canvasNode(draw func(*canvas.Canvas), c ...Child) Node {
	return Primitive(style.KindCanvas, append([]Child{CanvasDraw(draw)}, c...)...)
}

// flood fills the canvas with its fill style.
func flood(c *canvas.Canvas) { c.FillRect(0, 0, c.Width, c.Height) }

// drawn is what a draw function saw.
type drawn struct {
	w, h   float64
	t, d   time.Duration
	fill   canvas.Style
	stroke canvas.Style
	line   float64
}

// drawLog records the draws of a canvas.
type drawLog struct{ all []drawn }

func (l *drawLog) note(c *canvas.Canvas) {
	l.all = append(l.all, drawn{c.Width, c.Height, c.Time, c.Delta, c.FillStyle, c.StrokeStyle, c.LineWidth})
}

// take returns how many draws there were since the last call.
func (l *drawLog) take() int {
	n := len(l.all)
	l.all = l.all[:0]
	return n
}

func (l *drawLog) last() drawn { return l.all[len(l.all)-1] }

// canvasApp is a window with a canvas below a header: its fill, which is the
// color of the Value v, floods it.
type canvasApp struct {
	log    drawLog
	class  string
	v      *Value[float32]
	mode   Setter[string] // "", "none" (display: none), "invisible", "empty", "gone" (unmounted)
	again  bool           // its draws ask for another frame
	before []Child
	w      WindowHandle
	ref    ElementRef
	n      Setter[int]
}

func (a *canvasApp) draw(c *canvas.Canvas) {
	a.log.note(c)
	c.FillStyle = gfx.Color{R: a.v.Get(), A: 1}
	flood(c)
	if a.again {
		c.RequestFrame()
	}
}

func (a *canvasApp) root() Node {
	inner := func() Node {
		mode, set := UseState("")
		a.mode = set
		a.v = UseValue[float32](1)
		class := a.class
		if class == "" {
			class = "w-40 h-20"
		}
		kids := append([]Child{Class("p-4"), div(Class("h-8"), Text("header"))}, a.before...)
		switch mode {
		case "none":
			kids = append(kids, canvasNode(a.draw, Class(class+" hidden")))
		case "invisible":
			kids = append(kids, canvasNode(a.draw, Class(class+" invisible")))
		case "empty":
			kids = append(kids, canvasNode(a.draw, Class(class+" w-0")))
		case "gone":
		default:
			kids = append(kids, canvasNode(a.draw, Class(class)))
		}
		return div(kids...)
	}
	content := Component(inner)
	return Component(func() Node {
		a.w, a.ref = UseWindow(), UseElementRef()
		n, set := UseState(0)
		a.n = set
		return div(AttachRef(a.ref), Class("h-screen overflow-y-auto"), content, div(Class("h-[2000px]"), Textf("n=%d", n)))
	})
}

// step delivers one refresh of the clock.
func (x *winHarness) step() {
	x.b.Advance(x.b.FrameInterval())
	if at, ok := x.h.TimerArmed(); ok && at <= x.b.Now() {
		x.b.Wake()
	}
	x.b.Drain(x.h)
}

// A canvas draws at its first frame, and again only for a reason; a redraw
// builds, styles, lays out and places nothing, records its group alone and
// damages it alone, and an equal one does nothing at all.
func TestCanvasRedraw(t *testing.T) {
	a := canvasApp{class: "w-40 h-20 p-2 bg-black"}
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	if n := a.log.take(); n != 1 {
		t.Errorf("%d draws at the mount, want 1", n)
	}
	// The element is at (16, 48), 160x80.
	if c := x.pixel(60, 80); c != texRed {
		t.Errorf("the canvas shows %v, want red", c)
	}
	if c := x.pixel(18, 50); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("its padding shows %v", c)
	}
	x.expectIdle("after the mount")
	for i, v := range []float32{0, 0.5, 1} {
		before := tr.Stats()
		a.v.Set(v)
		x.drain()
		if n := a.log.take(); n != 1 {
			t.Errorf("%d draws for a Value it read", n)
		}
		if n := recordedOf(tr, layerGroup); n != 1 || tr.lay.recorded != 1+recordedOf(tr, layerBars) {
			t.Errorf("a redraw recorded %d lists, %d of them groups, want the canvas's group alone", tr.lay.recorded, n)
		}
		want := before
		want.Frames++
		want.Paints++
		if after := tr.Stats(); after != want {
			t.Errorf("redraw %d did %+v, want only a frame and a paint over %+v", i, after, before)
		}
		// The content box, 144x64 at (24, 56), and the pixel around it that
		// every command's bounds have.
		d := tr.LastDamage()
		if d.Full || len(d.Shifts) > 0 || !slices.Equal(d.Rects, []image.Rectangle{image.Rect(23, 55, 169, 121)}) {
			t.Errorf("a redraw damaged %+v, want the content box and a pixel around it", d)
		}
		x.expectIdle("after a redraw")
	}
	// The drawing is the same: nothing is recorded or presented.
	a.v.Set(2) // clamped to 1 when it is painted
	x.drain()
	presents := x.h.Presents()
	equal := func(what string, f func()) {
		t.Helper()
		list := keyedCanvas(tr).box.med.drawing
		a.log.take()
		f()
		x.drain()
		if n := a.log.take(); n != 1 {
			t.Errorf("%s: %d draws, want 1", what, n)
		}
		if n := tr.lay.recorded - recordedOf(tr, layerBars); n != 0 {
			t.Errorf("%s: recorded %d lists", what, n)
		}
		if n := x.h.Presents() - presents; n != 0 {
			t.Errorf("%s: presented %d frames", what, n)
		}
		if keyedCanvas(tr).box.med.drawing != list {
			t.Errorf("%s: the list is another one", what)
		}
		if d := tr.LastDamage(); d.Full || !d.Empty() {
			t.Errorf("%s: damaged %+v", what, d)
		}
	}
	equal("a Value that draws the same", func() { a.v.Set(4) })
	equal("a build that draws the same", func() { a.mode.Set("x") })
	expectFullRender(t, x.windows()[0], tr)
}

// cnvOf returns the canvases of tr, which has some.
func cnvOf(tr *Tree) *canvasTree { return tr.cnv.(*canvasTree) }

// keyedCanvas returns the state of the only canvas of tr.
func keyedCanvas(tr *Tree) *canvasBox {
	var found *canvasBox
	var walk func(el *Element)
	walk = func(el *Element) {
		if el.kind == elemPrimitive && el.box.kind == style.KindCanvas {
			found = el.box.med.cnv.(*canvasBox)
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(tr.root)
	return found
}

// What makes a canvas draw again, and what does not.
func TestCanvasWhenItDraws(t *testing.T) {
	var log, fnLog, builtLog drawLog // of a canvas whose node stays the same, one below a TextColorFn, one built again
	var setClass, setBox, setOther Setter[string]
	var setScheme Setter[ColorScheme]
	var bump Setter[int]
	var col *Value[gfx.Color]
	x := newWinHarness(t, Options{Config: Config{Dev: true, Log: func(m string) { t.Error(m) }}, Root: Component(func() Node {
		class, set := UseState("w-40 h-20")
		box, set2 := UseState("p-4")
		other, set3 := UseState("h-4")
		scheme, set4 := UseState(ColorSchemeSystem)
		setClass, setBox, setOther, setScheme = set, set2, set3, set4
		col = UseValue(gfx.Color{G: 1, A: 1})
		same := UseMemo(func() Node {
			return canvasNode(func(c *canvas.Canvas) {
				log.note(c)
				c.Arc(c.Width/2, c.Height/2, 10, 0, 6)
				c.Stroke()
			}, Class("w-full h-full hover:bg-red-500 transition-none"))
		}, Once)
		fn := UseMemo(func() Node {
			return canvasNode(fnLog.note, Class("w-10 h-full"))
		}, Once)
		return div(Class(box), SchemeProp(scheme),
			div(Class(other)),
			div(Class("text-sky-500 "+class), same),
			Component(func() Node {
				k, set := UseState(0)
				bump = set
				return div(Class("w-10 h-10"), canvasNode(func(c *canvas.Canvas) {
					_ = k
					builtLog.note(c)
				}))
			}),
			div(Style(style.TextColorFn(col.Get)), Class("h-10"), fn),
		)
	})})
	tr := x.h.Tree()
	expect := func(what string, same, fn, built int, f func()) {
		t.Helper()
		log.take()
		fnLog.take()
		builtLog.take()
		f()
		x.drain()
		if a, b, c := log.take(), fnLog.take(), builtLog.take(); a != same || b != fn || c != built {
			t.Errorf("%s: %d, %d and %d draws, want %d, %d and %d", what, a, b, c, same, fn, built)
		}
		x.expectIdle(what)
	}
	expect("nothing", 0, 0, 0, func() {})
	// A new closure each build: the function component below has one.
	expect("another component builds", 0, 0, 1, func() { bump.Set(1) })
	expect("another element of another size", 0, 0, 1, func() { setOther.Set("h-9") })
	expect("another place", 0, 0, 1, func() { setBox.Set("p-6") })
	expect("a hover", 0, 0, 0, func() { x.windows()[0].Inject(move(60, 90)) })
	if bg := x.pixel(60, 90); bg == (color.RGBA{255, 255, 255, 255}) {
		t.Error("the canvas is not hovered")
	}
	expect("a new size", 1, 0, 1, func() { setClass.Set("w-44 h-20") })
	expect("a new color", 1, 0, 1, func() { setClass.Set("w-44 h-20 text-rose-500") })
	expect("a new stroke", 1, 0, 1, func() { setClass.Set("w-44 h-20 text-rose-500 stroke-emerald-500") })
	expect("a new stroke width", 1, 0, 1, func() { setClass.Set("w-44 h-20 text-rose-500 stroke-emerald-500 stroke-3") })
	expect("a new fill", 1, 0, 1, func() { setClass.Set("w-44 h-20 text-rose-500 stroke-emerald-500 stroke-3 fill-sky-500") })
	expect("another background of its parent", 0, 0, 1, func() { setClass.Set("w-44 h-20 text-rose-500 stroke-emerald-500 stroke-3 fill-sky-500 bg-white") })
	expect("the color of a TextColorFn", 0, 1, 0, func() { col.Set(gfx.Color{B: 1, A: 1}) })
	expect("the same color", 0, 0, 0, func() { col.Set(gfx.Color{B: 1, A: 1}) })
	expect("the scheme of its position", 1, 1, 1, func() { setScheme.Set(ColorSchemeDark) })
	expect("a scheme of the system that its position overrides", 0, 0, 0, func() { tr.SetColorScheme(ColorSchemeDark) })
	expect("its position follows the system, in the same scheme", 0, 0, 1, func() { setScheme.Set(ColorSchemeSystem) })
	expect("the scheme of the system", 1, 1, 1, func() { tr.SetColorScheme(ColorSchemeLight) })
	expect("a theme", 1, 1, 1, func() { tr.SetTheme(Theme{Colors: map[string]SchemeColor{"primary": {Light: gfx.Color{R: 1, A: 1}}}}) })
	expect("a scale", 1, 1, 1, func() { x.windows()[0].SetScale(1.5) })
	expect("a window of another size", 0, 0, 0, func() { x.windows()[0].Resize(platform.Size{W: 500, H: 300}) })
}

// The size is the content box, snapped as it is painted.
func TestCanvasSize(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		for _, class := range []string{"w-40 h-20", "w-[33.3px] h-[17.7px] p-[3.1px] border-[1.5px]", "w-full aspect-video ml-[0.4px]"} {
			var log drawLog
			var ref ElementRef
			b := NewTree(Config{Fonts: testFonts})
			b.SetViewport(gfx.Size{W: 400, H: 300})
			b.SetScale(float32(scale))
			b.Mount(Component(func() Node {
				ref = UseElementRef()
				return div(Class("p-[0.3px]"), canvasNode(func(c *canvas.Canvas) {
					log.note(c)
					flood(c)
				}, AttachRef(ref), Class(class)))
			}))
			dl, _ := b.Frame()
			got := log.last()
			for _, v := range []float64{got.w, got.h} {
				if d := v * scale; d != float64(int(d+0.5)) && d-float64(int(d+0.5)) > 1e-9 {
					t.Errorf("x%g %s: %gx%g is not in whole device pixels", scale, class, got.w, got.h)
				}
			}
			// Every pixel of it is covered whole, and no other.
			img := b.Rasterize(dl, float32(scale))
			n := 0
			for y := range img.Rect.Dy() {
				for px := range img.Rect.Dx() {
					switch c := img.RGBAAt(px, y); c {
					case color.RGBA{0, 0, 0, 255}:
						n++
					case color.RGBA{255, 255, 255, 255}:
					default:
						if !strings.Contains(class, "border") {
							t.Fatalf("x%g %s: pixel %d,%d is %v", scale, class, px, y, c)
						}
					}
				}
			}
			if want := int(got.w*scale+0.5) * int(got.h*scale+0.5); !strings.Contains(class, "border") && n != want {
				t.Errorf("x%g %s: %d pixels painted, want %d (%gx%g)", scale, class, n, want, got.w, got.h)
			}
		}
	}
}

// A canvas that asks for frames gets one per refresh, on its own clock, and
// the window sleeps once it stops.
func TestCanvasRequestFrame(t *testing.T) {
	a := canvasApp{again: true}
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	ms := x.b.FrameInterval()
	var sum time.Duration
	for i, d := range a.log.all {
		want := ms
		if i == 0 {
			want = 0
		}
		sum += want
		if d.d != want || d.t != sum {
			t.Errorf("draw %d: time %v, delta %v; want %v, %v", i, d.t, d.d, sum, want)
		}
	}
	a.log.take()
	if p := tr.Pending(x.b.Now()); len(p) != 1 || p[0].Kind != PendingCanvas {
		t.Errorf("pending %+v, want the canvas", p)
	}
	if !tr.OnlyCanvasFrames() {
		t.Error("the frame asked for is not the canvas's alone")
	}
	before := tr.Stats()
	for i := range 5 {
		x.step()
		if n := len(a.log.all); n != i+1 {
			t.Fatalf("%d draws after %d frames", n, i+1)
		}
	}
	want := before
	want.Frames += 5
	want.Paints += 5
	if after := tr.Stats(); after != want {
		t.Errorf("five frames of it did %+v, want only frames and paints over %+v", after, before)
	}
	a.again = false
	a.log.take()
	x.step()
	if n := a.log.take(); n != 1 || tr.OnlyCanvasFrames() || len(tr.Pending(x.b.Now())) != 0 {
		t.Errorf("%d last draws, only canvas frames %v, pending %+v", n, tr.OnlyCanvasFrames(), tr.Pending(x.b.Now()))
	}
	frames := tr.Stats().Frames
	x.drain()
	if n := a.log.take(); n != 0 || tr.Stats().Frames != frames {
		t.Errorf("%d draws and %d frames at rest", n, tr.Stats().Frames-frames)
	}
	x.expectIdle("at rest")
}

// The clock of a canvas stands still at rest: a Value that draws it ten
// seconds later is no jump.
func TestCanvasClock(t *testing.T) {
	a := canvasApp{again: true}
	x := newWinHarness(t, Options{Root: a.root()})
	ms := x.b.FrameInterval()
	a.again = false
	x.step()
	rest := a.log.last().t
	a.log.take()
	x.b.Advance(10 * time.Second)
	x.b.Drain(x.h)
	a.v.Set(0.5)
	a.again = true
	x.b.Drain(x.h)
	x.step()
	x.step()
	x.step()
	if len(a.log.all) < 3 {
		t.Fatalf("%d draws", len(a.log.all))
	}
	for i, d := range a.log.all {
		want := time.Duration(i) * ms
		wantD := ms
		if i == 0 {
			wantD = 0
		}
		if d.t != rest+want || d.d != wantD {
			t.Errorf("draw %d after the rest: time %v, delta %v; want %v, %v", i, d.t, d.d, rest+want, wantD)
		}
	}
	// A second draw within a frame's time.
	tr := x.h.Tree()
	last := a.log.last()
	a.v.Set(0.25)
	tr.Frame()
	if d := a.log.last(); d.t != last.t || d.d != 0 {
		t.Errorf("a second draw in the frame: time %v, delta %v; want %v, 0", d.t, d.d, last.t)
	}
	// Frames that did not come for a while (a machine that slept).
	x.b.Advance(time.Hour)
	x.b.Drain(x.h)
	if d := a.log.last(); d.t != last.t || d.d != 0 {
		t.Errorf("after an hour without frames: time %v, delta %v; want %v, 0", d.t, d.d, last.t)
	}
	x.step()
	if d := a.log.last(); d.t != last.t+ms || d.d != ms {
		t.Errorf("then: time %v, delta %v; want %v, %v", d.t, d.d, last.t+ms, ms)
	}
}

// While a canvas that asks for frames is not on screen it neither draws nor
// asks, its clock stands still, and it goes on as soon as it is shown.
func TestCanvasOffScreen(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hide, show func(a *canvasApp)
	}{
		{"display none", func(a *canvasApp) { a.mode.Set("none") }, func(a *canvasApp) { a.mode.Set("") }},
		{"invisible", func(a *canvasApp) { a.mode.Set("invisible") }, func(a *canvasApp) { a.mode.Set("") }},
		{"empty", func(a *canvasApp) { a.mode.Set("empty") }, func(a *canvasApp) { a.mode.Set("") }},
		{"scrolled out", func(a *canvasApp) { a.ref.ScrollTo(0, 1500) }, func(a *canvasApp) { a.ref.ScrollTo(0, 0) }},
		{"just scrolled out", func(a *canvasApp) { a.ref.ScrollTo(0, 150) }, func(a *canvasApp) { a.ref.ScrollTo(0, 0) }},
		{"hidden window", func(a *canvasApp) { a.w.Hide() }, func(a *canvasApp) { a.w.Show() }},
		{"minimized window", func(a *canvasApp) { a.w.Minimize() }, func(a *canvasApp) { a.w.Focus() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := canvasApp{again: true}
			x := newWinHarness(t, Options{Config: Config{InstantScroll: true}, Root: a.root()})
			tr := x.h.Tree()
			ms := x.b.FrameInterval()
			tc.hide(&a)
			x.drain()
			x.drain() // the scrollbars, the windows' states
			if tr.Pending(x.b.Now()) != nil {
				x.b.Advance(5 * time.Second) // the scrollbars fade
				x.b.Wake()
				x.drain()
			}
			last := a.log.last()
			a.log.take()
			frames := tr.Stats().Frames
			for range 10 {
				x.step()
			}
			if n := a.log.take(); n != 0 {
				t.Errorf("%d draws while it was not on screen", n)
			}
			if n := tr.Stats().Frames - frames; n != 0 {
				t.Errorf("%d frames while it was not on screen", n)
			}
			if p := tr.Pending(x.b.Now()); len(p) != 0 {
				t.Errorf("pending %+v", p)
			}
			x.expectIdle("not on screen")
			tc.show(&a)
			x.b.Drain(x.h)
			for range 4 {
				x.step()
			}
			if len(a.log.all) < 2 {
				t.Fatalf("%d draws once it is shown", len(a.log.all))
			}
			for i, d := range a.log.all {
				want := ms
				if i == 0 {
					want = 0
				}
				if d.d != want {
					t.Errorf("draw %d once it is shown has delta %v, want %v", i, d.d, want)
				}
			}
			if d := a.log.all[0]; d.t != last.t {
				t.Errorf("its clock went from %v to %v while it waited", last.t, d.t)
			}
			a.again = false
			x.drain()
			x.expectIdle("at the end")
			expectFullRender(t, x.windows()[0], tr)
		})
	}
}

// A build in a hidden window, which commits without frames, draws its canvas
// with the clock where it was, and asks for no frame.
func TestCanvasHiddenCommit(t *testing.T) {
	a := canvasApp{again: true}
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	a.w.Hide()
	x.drain()
	last := a.log.last()
	a.log.take()
	for i := range 3 {
		x.b.Advance(time.Second)
		a.mode.Set(fmt.Sprint("build ", i))
		x.drain()
		if len(a.log.all) != i+1 {
			t.Fatalf("%d draws after %d builds", len(a.log.all), i+1)
		}
		if d := a.log.last(); d.t != last.t || d.d != 0 {
			t.Errorf("a commit drew at %v, delta %v; want %v, 0", d.t, d.d, last.t)
		}
		if p := tr.Pending(x.b.Now()); len(p) != 0 {
			t.Errorf("pending %+v", p)
		}
	}
	a.log.take()
	a.w.Show()
	x.drain()
	if len(a.log.all) < 2 || a.log.all[0].d != 0 || a.log.all[0].t != last.t || a.log.all[1].d != x.b.FrameInterval() {
		t.Errorf("shown again: %+v", a.log.all)
	}
	if c := x.pixel(60, 80); c != texRed {
		t.Errorf("it shows %v", c)
	}
}

// What changes while a canvas is not shown is drawn when it is, in the frame
// that shows it, and a change asks for no frame meanwhile.
func TestCanvasDeferred(t *testing.T) {
	for _, tc := range []struct {
		name       string
		class      string
		hide, show func(a *canvasApp)
		// draws: it is in a list, so it draws.
		draws bool
	}{
		{"display none", "", func(a *canvasApp) { a.mode.Set("none") }, func(a *canvasApp) { a.mode.Set("") }, false},
		{"invisible", "", func(a *canvasApp) { a.mode.Set("invisible") }, func(a *canvasApp) { a.mode.Set("") }, false},
		{"empty", "", func(a *canvasApp) { a.mode.Set("empty") }, func(a *canvasApp) { a.mode.Set("") }, false},
		{"scrolled out", "", func(a *canvasApp) { a.ref.ScrollTo(0, 1500) }, func(a *canvasApp) { a.ref.ScrollTo(0, 0) }, false},
		{"just scrolled out", "", func(a *canvasApp) { a.ref.ScrollTo(0, 150) }, func(a *canvasApp) { a.ref.ScrollTo(0, 0) }, true},
		{"scrolled out, transformed", "w-40 h-20 rotate-180", func(a *canvasApp) { a.ref.ScrollTo(0, 1500) }, func(a *canvasApp) { a.ref.ScrollTo(0, 0) }, true},
		{"hidden window", "", func(a *canvasApp) { a.w.Hide() }, func(a *canvasApp) { a.w.Show() }, false},
		{"minimized window", "", func(a *canvasApp) { a.w.Minimize() }, func(a *canvasApp) { a.w.Focus() }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := canvasApp{class: tc.class}
			x := newWinHarness(t, Options{Config: Config{InstantScroll: true}, Root: a.root()})
			tr := x.h.Tree()
			tc.hide(&a)
			x.drain()
			if tr.Pending(x.b.Now()) != nil {
				x.b.Advance(5 * time.Second) // the scrollbars fade
				x.b.Wake()
				x.drain()
			}
			a.log.take()
			frames := tr.Stats().Frames
			for i := range 5 {
				a.v.Set(float32(i) / 10)
				x.drain()
			}
			switch n := a.log.take(); {
			case tc.draws && n != 5:
				t.Errorf("%d draws, want one for each change", n)
			case !tc.draws && n != 0:
				t.Errorf("%d draws while nothing paints it", n)
			}
			if n := tr.Stats().Frames - frames; !tc.draws && n > 1 {
				t.Errorf("%d frames for five changes of a Value nothing shows", n)
			}
			a.v.Set(1)
			tc.show(&a)
			x.b.Drain(x.h)
			for i := 0; i < 5 && x.b.Pending(); i++ {
				x.step()
			}
			if n := a.log.take(); n != 1 {
				t.Errorf("%d draws to show it, want 1", n)
			}
			if c := x.pixel(60, 80); c != texRed {
				t.Errorf("shown, it shows %v, want red", c)
			}
			x.drain()
			expectFullRender(t, x.windows()[0], tr)
		})
	}
}

// A canvas that a list comes to paint without being placed again draws after
// the recording, and the frame shows it.
func TestCanvasLate(t *testing.T) {
	var log drawLog
	var ref ElementRef
	x := newWinHarness(t, Options{Config: Config{InstantScroll: true}, Root: Component(func() Node {
		ref = UseElementRef()
		kids := []Child{AttachRef(ref), Class("h-screen overflow-y-auto")}
		for i := range 40 {
			kids = append(kids, div(Key(i), Class("h-25 p-1"), canvasNode(func(c *canvas.Canvas) {
				log.note(c)
				flood(c)
			}, Class("w-40 h-10 fill-[#f00]"))))
		}
		return div(kids...)
	})})
	tr := x.h.Tree()
	first := log.take()
	if first >= 20 {
		t.Errorf("%d of 40 canvases drew at the first frame: only those in the interest rect should", first)
	}
	for _, y := range []float32{50, 900, 3000, 1500, 0} {
		ref.ScrollTo(0, y)
		x.b.Drain(x.h)
		x.step()
		img := x.windows()[0].Frame()
		for row := range 3 {
			py := row*100 + 100 - int(y)%100 + 4 + 5
			if py >= 300 {
				continue
			}
			if c := img.RGBAAt(60, py); c != texRed {
				t.Errorf("scrolled to %g, pixel 60,%d is %v, want red", y, py, c)
			}
		}
		expectFullRender(t, x.windows()[0], tr)
	}
	if n := first + log.take(); n > 40 {
		t.Errorf("%d draws of 40 canvases that nothing changed", n)
	}
	// Stale canvases far away draw when they come into the interest rect.
	tr.SetTheme(Theme{})
	x.drain()
	log.take()
	ref.ScrollTo(0, 3000)
	x.b.Drain(x.h)
	x.step()
	if n := log.take(); n == 0 || n > 15 {
		t.Errorf("%d draws after a jump in the list", n)
	}
	expectFullRender(t, x.windows()[0], tr)
}

// The items of a virtual list draw as they mount, in the frame that shows
// them, and no others.
func TestCanvasVirtualList(t *testing.T) {
	var log drawLog
	var ref ElementRef
	x := newWinHarness(t, Options{Config: Config{InstantScroll: true}, Root: Component(func() Node {
		ref = UseElementRef()
		return VirtualList(1000, func(int) float32 { return 40 }, func(i int) Node {
			return div(Key(i), Class("h-10 p-1"), canvasNode(func(c *canvas.Canvas) {
				log.note(c)
				c.FillStyle = gfx.Color{R: float32(i%2) / 2, G: float32(i%3) / 4, B: float32(i%5) / 8, A: 1}
				flood(c)
			}, Class("w-40 h-full")))
		}, AttachRef(ref), Class("h-screen"))
	})})
	for _, y := range []float32{0, 120, 4000, 20000, 3920, 0} {
		ref.ScrollTo(0, y)
		x.b.Drain(x.h)
		x.step()
		img := x.windows()[0].Frame()
		for row := range 7 {
			i := int(y)/40 + row
			want := color.RGBA{uint8(float64(i%2)/2*255 + 0.5), uint8(float64(i%3)/4*255 + 0.5), uint8(float64(i%5)/8*255 + 0.5), 255}
			if got := img.RGBAAt(60, row*40+20); got != want {
				t.Errorf("at %g, item %d shows %v, want %v", y, i, got, want)
			}
		}
		x.drain()
		expectFullRender(t, x.windows()[0], x.h.Tree())
	}
	if n := log.take(); n > 250 {
		t.Errorf("%d draws", n)
	}
	x.drain()
	if n := log.take(); n != 0 {
		t.Errorf("%d draws at rest", n)
	}
	if n := len(cnvOf(x.h.Tree()).q); n != 0 {
		t.Errorf("%d canvases are queued at rest", n)
	}
}

// The Values a draw reads are those of its last run.
func TestCanvasTracking(t *testing.T) {
	var log drawLog
	var which *Value[string]
	var a, b *Value[int]
	var setOn Setter[bool]
	x := newWinHarness(t, Options{Root: Component(func() Node {
		on, set := UseState(true)
		setOn = set
		which, a, b = UseValue("a"), UseValue(0), UseValue(0)
		if !on {
			return div()
		}
		return div(canvasNode(func(c *canvas.Canvas) {
			log.note(c)
			v := a
			if which.Get() == "b" {
				v = b
			}
			c.FillRect(float64(v.Get()), 0, 10, 10)
		}))
	})})
	tr := x.h.Tree()
	expect := func(what string, want int, f func()) {
		t.Helper()
		log.take()
		f()
		x.drain()
		if n := log.take(); n != want {
			t.Errorf("%s: %d draws, want %d", what, n, want)
		}
		x.expectIdle(what)
	}
	expect("a Value it read", 1, func() { a.Set(1) })
	expect("one it did not", 0, func() { b.Set(1) })
	expect("the choice", 1, func() { which.Set("b") })
	expect("one it reads no more", 0, func() { a.Set(2) })
	expect("the one it reads now", 1, func() { b.Set(2) })
	expect("several at once", 1, func() { b.Set(3); which.Set("a"); a.Set(3) })
	if n := len(keyedCanvas(tr).obs.subs); n != 2 {
		t.Errorf("%d subscriptions, want 2", n)
	}
	expect("its unmount", 0, func() { setOn.Set(false) })
	if n := len(which.subs) + len(a.subs) + len(b.subs); n != 0 {
		t.Errorf("%d subscriptions after its unmount", n)
	}
	expect("a Value after its unmount", 0, func() { a.Set(9) })
}

// A Value that the float pass publishes is drawn in the same frame, once.
func TestCanvasPublished(t *testing.T) {
	var log drawLog
	var left Setter[int]
	var got []float32
	x := newWinHarness(t, Options{Root: Component(func() Node {
		n, set := UseState(10)
		left = set
		anchor, at := UseElementRef(), UseValue[float32](-1)
		place := func(in FloatInput) FloatResult {
			PublishValue(at, in.Anchor.X)
			return FloatResult{X: in.Anchor.X, Y: in.Anchor.Bottom()}
		}
		draw := UseMemo(func() func(*canvas.Canvas) {
			return func(c *canvas.Canvas) {
				log.note(c)
				got = append(got, at.Get())
			}
		}, Once)
		arrow := UseMemo(func() Node { return canvasNode(draw, Class("w-10 h-4")) }, Once)
		return div(Class("relative h-screen"),
			div(AttachRef(anchor), Class(fmt.Sprintf("h-8 w-20 ml-[%dpx]", n))),
			Portal(div(Floating(FloatOptions{Anchor: anchor, Place: place}), Class("fixed"), arrow)))
	})})
	if !slices.Equal(got, []float32{10}) {
		t.Errorf("the first frame drew %v, want 10 once", got)
	}
	got = nil
	left.Set(25)
	x.drain()
	if !slices.Equal(got, []float32{25}) {
		t.Errorf("moved, it drew %v, want 25 once", got)
	}
	x.expectIdle("at the end")
}

// A draw that panics leaves the old drawing and subscriptions, and the tree
// whole: the next frame draws again.
func TestCanvasPanic(t *testing.T) {
	var fail atomic.Bool
	var log drawLog
	var v, w *Value[int]
	var other Setter[int]
	tr := NewTree(Config{Fonts: testFonts, Dev: true})
	tr.SetViewport(gfx.Size{W: 300, H: 200})
	tr.Mount(Component(func() Node {
		n, set := UseState(0)
		other = set
		v, w = UseValue(0), UseValue(0)
		return div(Textf("n=%d", n),
			canvasNode(func(c *canvas.Canvas) {
				log.note(c)
				if fail.Load() {
					w.Get()
					c.Save()
					c.Rect(0, 0, 5, 5)
					c.Clip()
					panic("boom")
				}
				c.FillRect(float64(v.Get()), 0, 10, 10)
				c.RequestFrame()
			}, Key("a")),
			canvasNode(func(c *canvas.Canvas) { flood(c) }, Class("w-10 h-10"), Key("b")),
		)
	}))
	tr.Frame()
	box := keyed(tr, "a").box.med
	list := box.drawing
	fail.Store(true)
	for range 3 {
		v.Set(5)
		other.Update(func(n int) int { return n + 1 })
		func() {
			defer func() {
				if r := recover(); r != "boom" {
					t.Errorf("recovered %v", r)
				}
			}()
			tr.Frame()
		}()
	}
	if box.drawing != list || len(v.subs) != 1 || len(w.subs) != 0 {
		t.Errorf("after the panic: another list %v, %d and %d subscriptions; want the old ones", box.drawing != list, len(v.subs), len(w.subs))
	}
	if tr.trackDepth.Load() != 0 || drawing.Load() != 0 || tr.inFrame {
		t.Errorf("after the panic: track depth %d, drawing %d, in frame %v", tr.trackDepth.Load(), drawing.Load(), tr.inFrame)
	}
	fail.Store(false)
	dl, changed := tr.Frame()
	if !changed || box.drawing == list {
		t.Error("the frame after the panic does not draw")
	}
	ref := tr.ReferenceFrame(false)
	if dl.Len() != ref.Len() {
		t.Errorf("the frame after the panic has %d commands, a full one %d", dl.Len(), ref.Len())
	}
	if !changed {
		t.Error("the frame after the panic changes nothing")
	}
}

// Hooks and changes of state in a draw panic in dev mode with a message that
// says what to do; otherwise the changes apply to the next frame.
func TestCanvasWrites(t *testing.T) {
	type env struct {
		set  Setter[int]
		upd  func(func(int) int)
		disp Dispatcher[int]
		v    *Value[float32]
		w    WindowHandle
	}
	calls := []struct {
		name string
		f    func(e *env)
		// n is what is seen of it without dev mode.
		n func(e *env, state, reduced int) bool
	}{
		{"Setter.Set", func(e *env) { e.set.Set(1) }, func(_ *env, s, _ int) bool { return s == 1 }},
		{"Setter.Update", func(e *env) { e.upd(func(int) int { return 2 }) }, func(_ *env, s, _ int) bool { return s == 2 }},
		{"Dispatcher.Dispatch", func(e *env) { e.disp.Dispatch(3) }, func(_ *env, _, r int) bool { return r == 3 }},
		{"Value.Set", func(e *env) { e.v.Set(4) }, func(e *env, _, _ int) bool { return e.v.Get() == 4 }},
		{"Value.Update", func(e *env) { e.v.Update(func(float32) float32 { return 5 }) }, func(e *env, _, _ int) bool { return e.v.Get() == 5 }},
		{"Animate", func(e *env) { Animate(e.v, 6, Duration(time.Millisecond)) }, func(e *env, _, _ int) bool { return e.v.Get() == 6 }},
		{"Post", func(e *env) { Post(func() { e.set.Set(7) }) }, func(_ *env, s, _ int) bool { return s == 7 }},
		{"SetColorScheme", func(*env) { SetColorScheme(ColorSchemeDark) }, func(e *env, _, _ int) bool { return true }},
		{"SetTheme", func(*env) { SetTheme(Theme{}) }, func(e *env, _, _ int) bool { return true }},
		{"SetDefaultLang", func(*env) { SetDefaultLang("") }, func(e *env, _, _ int) bool { return true }},
		{"WindowHandle", func(e *env) { e.w.Maximize() }, func(e *env, _, _ int) bool { return true }},
	}
	for _, dev := range []bool{true, false} {
		for _, c := range calls {
			t.Run(fmt.Sprintf("%s dev=%v", c.name, dev), func(t *testing.T) {
				var e env
				var state, reduced int
				var once, recovered atomic.Bool
				var got string
				x := newWinHarness(t, Options{Config: Config{Dev: dev}, Root: Component(func() Node {
					e.w = UseWindow()
					state, e.set = UseState(0)
					_, e.upd = UseStateUpdate(0)
					e.upd = e.set.Update
					reduced, e.disp = UseReducer(func(_, a int) int { return a }, 0)
					e.v = UseValue[float32](0)
					return canvasNode(func(*canvas.Canvas) {
						if once.Swap(true) {
							return
						}
						defer func() {
							if r := recover(); r != nil {
								recovered.Store(true)
								got = fmt.Sprint(r)
							}
						}()
						c.f(&e)
					})
				})})
				stop := x.h.Tree().ReceivePosts()
				defer stop()
				x.drain()
				if recovered.Load() != dev {
					t.Fatalf("panicked %v: %s", recovered.Load(), got)
				}
				if dev {
					if !strings.Contains(got, c.name) || !strings.Contains(got, "tela.UseRef") || !strings.Contains(got, "canvas_test.go") {
						t.Errorf("panic %q", got)
					}
					return
				}
				if !c.n(&e, state, reduced) {
					t.Errorf("not applied: state %d, reduced %d, value %g", state, reduced, e.v.Get())
				}
			})
		}
	}
	t.Run("deep in the calls of a draw", func(t *testing.T) {
		var deep func(n int, f func())
		deep = func(n int, f func()) {
			if n == 0 {
				f()
				return
			}
			deep(n-1, f)
		}
		for _, n := range []int{20, 100, 5000} {
			var set Setter[int]
			got := ""
			tr := NewTree(Config{Fonts: testFonts, Dev: true})
			tr.Mount(Component(func() Node {
				_, set = UseState(0)
				return canvasNode(func(*canvas.Canvas) {
					deep(n, func() {
						var wg sync.WaitGroup
						wg.Go(func() { set.Set(1) }) // a goroutine of the draw's is another one
						wg.Wait()
						defer func() { got = fmt.Sprint(recover()) }()
						set.Set(2)
					})
				})
			}))
			tr.Frame()
			if !strings.Contains(got, "Setter.Set called in the draw function") {
				t.Errorf("%d calls deep: recovered %q", n, got)
			}
		}
		if inDraw() || drawing.Load() != 0 {
			t.Error("after the draws, still in one")
		}
	})
	t.Run("a hook", func(t *testing.T) {
		defer func() {
			if r := fmt.Sprint(recover()); !strings.Contains(r, "UseState called in the draw function") || !strings.Contains(r, "canvas_test.go") {
				t.Errorf("recovered %q", r)
			}
		}()
		tr := NewTree(Config{Fonts: testFonts, Dev: true})
		tr.Mount(canvasNode(func(*canvas.Canvas) { UseState(0) }))
		tr.Frame()
	})
	t.Run("a hook, without dev mode", func(t *testing.T) {
		defer func() {
			if r := fmt.Sprint(recover()); !strings.Contains(r, "UseState called outside a component build") {
				t.Errorf("recovered %q", r)
			}
		}()
		tr := NewTree(Config{Fonts: testFonts})
		tr.Mount(canvasNode(func(*canvas.Canvas) { UseState(0) }))
		tr.Frame()
	})
	t.Run("a kept canvas", func(t *testing.T) {
		var kept *canvas.Canvas
		tr := NewTree(Config{Fonts: testFonts, Dev: true})
		tr.Mount(canvasNode(func(c *canvas.Canvas) { kept = c }))
		tr.Frame()
		defer func() {
			if r := fmt.Sprint(recover()); !strings.Contains(r, "Canvas.RequestFrame called after") {
				t.Errorf("recovered %q", r)
			}
		}()
		kept.RequestFrame()
	})
}

// Other goroutines change state while draws run, and never panic.
func TestCanvasConcurrentWrites(t *testing.T) {
	var set Setter[int]
	var v *Value[int]
	var bad atomic.Value
	stop, started := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	defer func() { close(stop); wg.Wait() }()
	var inside atomic.Int64
	x := newWinHarness(t, Options{Config: Config{Dev: true}, Root: Component(func() Node {
		_, s := UseState(0)
		val := UseValue(0)
		if v == nil { // the goroutines read them from then on
			set, v = s, val
		}
		return canvasNode(func(c *canvas.Canvas) {
			c.FillRect(float64(v.Get()%10), 0, 10, 10)
			inside.Add(1)
			time.Sleep(200 * time.Microsecond) // the others write meanwhile
			inside.Add(-1)
			c.RequestFrame()
		})
	})})
	var during atomic.Int64
	for i := range 4 {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					bad.Store(fmt.Sprint(r))
				}
			}()
			for k := 0; ; k++ {
				select {
				case <-stop:
					return
				default:
				}
				if inside.Load() > 0 {
					during.Add(1)
				}
				set.Set(k)
				v.Update(func(n int) int { return n + 1 })
				Post(func() {})
				if k == 20 && i == 0 {
					close(started)
				}
				time.Sleep(20 * time.Microsecond)
			}
		})
	}
	<-started
	for end := time.Now().Add(20 * time.Second); during.Load() < 50; x.step() {
		if time.Now().After(end) {
			t.Fatalf("%d writes ran during a draw in 20 s", during.Load())
		}
	}
	if r := bad.Load(); r != nil {
		t.Errorf("a goroutine panicked: %v", r)
	}
}

// A draw that marks the tree without a setter (it moves the focus) has its
// work done by the next frame.
func TestCanvasFocusInDraw(t *testing.T) {
	var ref ElementRef
	done := false
	x := newWinHarness(t, Options{Root: Component(func() Node {
		ref = UseElementRef()
		return div(div(AttachRef(ref), TabIndex(0), Class("w-10 h-10 focus:bg-[#00f]")), canvasNode(func(*canvas.Canvas) {
			if !done {
				done = true
				ref.Focus()
			}
		}))
	})})
	x.windows()[0].SetFocused(true)
	x.drain()
	if !ref.Focused() {
		t.Fatal("not focused")
	}
	if c := x.pixel(5, 5); c != texBlue {
		t.Errorf("the focused element shows %v, want blue", c)
	}
	x.expectIdle("at the end")
}

// A draw that changes, on the UI thread, a Value it read draws again in the
// next frame.
func TestCanvasScrollInDraw(t *testing.T) {
	var got []float32
	x := newWinHarness(t, Options{Config: Config{InstantScroll: true}, Root: Component(func() Node {
		ref := UseElementRef()
		_, y := UseScroll(ref)
		return div(div(AttachRef(ref), Class("h-20 overflow-y-auto"), div(Class("h-80"))), canvasNode(func(c *canvas.Canvas) {
			got = append(got, y.Get())
			if len(got) == 2 {
				ref.ScrollTo(0, 40)
			}
			if len(got) < 3 {
				c.RequestFrame()
			}
		}))
	})})
	x.drain()
	if len(got) < 3 || got[len(got)-1] != 40 || got[0] != 0 {
		t.Errorf("it drew at %v, want 0 at first and 40 at last", got)
	}
	x.b.Advance(5 * time.Second) // the scrollbars fade
	x.b.Wake()
	x.drain()
	x.expectIdle("at the end")
	expectFullRender(t, x.windows()[0], x.h.Tree())
}

// The styles a draw starts with.
func TestCanvasStyles(t *testing.T) {
	th := style.DefaultTheme()
	sky, rose, emerald := th.Colors["sky-500"], th.Colors["rose-500"], th.Colors["emerald-500"]
	half := sky
	half.A = 0.5
	for _, tc := range []struct {
		class        string
		outer        string
		fill, stroke gfx.Color
		line         float64
	}{
		{"", "", gfx.Black, gfx.Black, 1},
		{"text-sky-500", "", sky, sky, 1},
		{"", "text-sky-500", sky, sky, 1},
		{"fill-rose-500", "text-sky-500", rose, sky, 1},
		{"stroke-rose-500 stroke-3", "text-sky-500", sky, rose, 3},
		{"stroke-[1.5]", "", gfx.Black, gfx.Black, 1.5},
		{"fill-none stroke-none", "", gfx.Transparent, gfx.Transparent, 1},
		{"fill-current stroke-current", "text-sky-500", sky, sky, 1},
		{"fill-current/50", "text-sky-500", half, sky, 1},
		{"", "fill-rose-500 stroke-emerald-500 stroke-2", rose, emerald, 2},
		{"fill-emerald-500", "fill-rose-500", emerald, gfx.Black, 1},
	} {
		var log drawLog
		tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { t.Error(m) }})
		tr.SetViewport(gfx.Size{W: 400, H: 300})
		tr.Mount(div(Class(tc.outer), canvasNode(log.note, Class(tc.class))))
		tr.Frame()
		got := log.last()
		if got.fill != canvas.Style(tc.fill) || got.stroke != canvas.Style(tc.stroke) || got.line != tc.line {
			t.Errorf("%q in %q: fill %v, stroke %v, width %g; want %v, %v, %g", tc.class, tc.outer, got.fill, got.stroke, got.line, tc.fill, tc.stroke, tc.line)
		}
	}
}

// A TextColorFn colors the canvas it is on, the ones below it, and those in
// an inline or display: contents element that has it, without a build.
func TestCanvasTextColorFn(t *testing.T) {
	var on, below, contents, own drawLog
	var col *Value[gfx.Color]
	x := newWinHarness(t, Options{Root: Component(func() Node {
		col = UseValue(gfx.Color{G: 1, A: 1})
		fn := Style(style.TextColorFn(col.Get))
		return div(
			canvasNode(on.note, fn, Class("w-10 h-4")),
			div(fn, div(canvasNode(below.note, Class("w-10 h-4")), canvasNode(own.note, Class("w-10 h-4 text-[#00f]")))),
			div(fn, Class("contents"), canvasNode(contents.note, Class("w-10 h-4"))),
		)
	})})
	for i, c := range []gfx.Color{{G: 1, A: 1}, {R: 1, A: 1}, {R: 1, G: 1, A: 0.5}} {
		if i > 0 {
			before := x.h.Tree().Stats()
			col.Set(c)
			x.drain()
			if after := x.h.Tree().Stats(); after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts {
				t.Errorf("color %d built, styled or laid out: %+v over %+v", i, after, before)
			}
		}
		for name, l := range map[string]*drawLog{"on it": &on, "above": &below, "on a display: contents": &contents} {
			if n := len(l.all); n != 1 || l.last().fill != canvas.Style(c) || l.last().stroke != canvas.Style(c) {
				t.Errorf("color %d, %s: draws %+v, want one with %v", i, name, l.all, c)
			}
			l.take()
		}
		if n := own.take(); n != 1 && i == 0 || n != 0 && i > 0 {
			t.Errorf("color %d: %d draws of a canvas with a text color of its own", i, n)
		}
	}
	x.expectIdle("at the end")
}

// The fill, the stroke and its width that an icon or a canvas inherits follow
// an ancestor whose own style changes alone (a hover).
func TestInheritedFillFollowsTheAncestor(t *testing.T) {
	var log drawLog
	x := newInput(t, div(Class("w-40 h-20 fill-[#00f] hover:fill-[#f00] hover:stroke-[#0f0] hover:stroke-4"),
		div(icon(checkSVG, Key("icon"), Class("size-8"))),
		canvasNode(log.note, Class("size-8"))), 400, 300)
	log.take()
	x.send(move(50, 40))
	x.frame()
	red, green := gfx.Color{R: 1, A: 1}, gfx.Color{G: 1, A: 1}
	if cs := keyed(x.tree(), "icon").box.cs; cs.Fill.Color != red || cs.Stroke.Color != green || cs.StrokeWidth != 4 {
		t.Errorf("the icon has fill %v, stroke %v, width %g", cs.Fill, cs.Stroke, cs.StrokeWidth)
	}
	if n := len(log.all); n != 1 || log.last().fill != canvas.Style(red) || log.last().stroke != canvas.Style(green) || log.last().line != 4 {
		t.Errorf("the canvas drew %+v", log.all)
	}
}

// A transition of the text color draws each of its steps.
func TestCanvasColorTransition(t *testing.T) {
	var log drawLog
	x := newInput(t, div(Class("w-40 h-20 text-[#000] hover:text-[#f00] transition-colors duration-100"),
		canvasNode(func(c *canvas.Canvas) { log.note(c); flood(c) }, Class("size-full"))), 400, 300)
	log.take()
	x.send(move(50, 40))
	for range 12 {
		x.frame()
	}
	var reds []float32
	for _, d := range log.all {
		reds = append(reds, d.fill.(gfx.Color).R)
	}
	if len(reds) < 5 || reds[len(reds)-1] != 1 || !slices.IsSorted(reds) {
		t.Errorf("the steps are %v", reds)
	}
	log.take()
	for range 5 {
		x.frame()
	}
	if n := log.take(); n != 0 {
		t.Errorf("%d draws at rest", n)
	}
}

// Color reads the theme in the scheme of the element.
func TestCanvasThemeColor(t *testing.T) {
	var got []gfx.Color
	draw := func(c *canvas.Canvas) {
		got = append(got, c.Color("primary"), c.Color("violet-500"), c.Color("transparent"))
	}
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { t.Error(m) }})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.Mount(div(canvasNode(draw, Class("w-10 h-10")), div(SchemeProp(ColorSchemeDark), canvasNode(draw, Class("w-10 h-10")))))
	tr.Frame()
	th := style.DefaultTheme()
	light, _ := th.TokenColor("primary", false)
	dark, _ := th.TokenColor("primary", true)
	want := []gfx.Color{light, th.Colors["violet-500"], gfx.Transparent, dark, th.Colors["violet-500"], gfx.Transparent}
	if !slices.Equal(got, want) {
		t.Errorf("colors %v, want %v", got, want)
	}
	got = nil
	red := gfx.Color{R: 1, A: 1}
	tr.SetTheme(Theme{Colors: map[string]SchemeColor{"primary": {Light: red, Dark: red}}})
	tr.Frame()
	if len(got) != 6 || got[0] != red || got[3] != red {
		t.Errorf("with a theme: %v", got)
	}

	unknown := func(dev bool) (r any, c gfx.Color) {
		defer func() { r = recover() }()
		tr := NewTree(Config{Fonts: testFonts, Dev: dev})
		c = red
		tr.Mount(canvasNode(func(cv *canvas.Canvas) { c = cv.Color("primry") }))
		tr.Frame()
		return nil, c
	}
	if r, _ := unknown(true); !strings.Contains(fmt.Sprint(r), `Canvas.Color("primry")`) || !strings.Contains(fmt.Sprint(r), `did you mean "primary"`) {
		t.Errorf("an unknown color: %v", r)
	}
	if r, c := unknown(false); r != nil || c != gfx.Transparent {
		t.Errorf("an unknown color without dev mode: %v, %v", r, c)
	}
}

// A canvas is 300x150, or keeps that ratio with one size.
func TestCanvasLayout(t *testing.T) {
	for _, tc := range []struct {
		class string
		w, h  float32
	}{
		{"", 300, 150},
		{"w-full", 400, 200},
		{"h-20", 160, 80},
		{"w-10 h-10", 40, 40},
		{"w-full aspect-video", 400, 225},
		{"inline", 300, 150},
	} {
		var ref ElementRef
		tr := NewTree(Config{Fonts: testFonts})
		tr.SetViewport(gfx.Size{W: 400, H: 300})
		tr.Mount(Component(func() Node {
			ref = UseElementRef()
			return canvasNode(flood, AttachRef(ref), Class(tc.class))
		}))
		tr.Frame()
		if b := ref.Bounds(); b.Width != tc.w || b.Height != tc.h {
			t.Errorf("%q is %gx%g, want %gx%g", tc.class, b.Width, b.Height, tc.w, tc.h)
		}
	}
}

// A canvas on a line of text draws, and one that text-overflow hides waits
// until it is shown.
func TestCanvasInline(t *testing.T) {
	var log drawLog
	var set Setter[string]
	x := newWinHarness(t, Options{Root: Component(func() Node {
		class, s := UseState("w-32")
		set = s
		same := UseMemo(func() Node {
			return canvasNode(func(c *canvas.Canvas) {
				log.note(c)
				flood(c)
			}, Class("inline size-10 fill-[#f00]"))
		}, Once)
		return div(Class("truncate "+class), Text("A long line of text "), same)
	})})
	if n := log.take(); n != 0 {
		t.Errorf("%d draws while text-overflow hides it", n)
	}
	set.Set("w-96")
	x.drain()
	if n := log.take(); n != 1 {
		t.Errorf("%d draws once it is shown", n)
	}
	red := 0
	img := x.windows()[0].Frame()
	for px := range 400 {
		if img.RGBAAt(px, 20) == texRed {
			red++
		}
	}
	if red < 39 || red > 40 {
		t.Errorf("%d red pixels on its line, want 40", red)
	}
	expectFullRender(t, x.windows()[0], x.h.Tree())
}

// Nothing is drawn outside the content box, with its rounded corners.
func TestCanvasClip(t *testing.T) {
	_, img := Render(div(Class("p-4"), canvasNode(func(c *canvas.Canvas) {
		c.FillRect(-100, -100, 1000, 1000)
	}, Class("size-40 p-4 rounded-full fill-[#f00] bg-[#0f0] border-4 border-[#00f]"))),
		RenderOptions{Config: Config{Fonts: testFonts}, Width: 200, Height: 200})
	for _, p := range []struct {
		x, y int
		want color.RGBA
	}{
		{96, 96, texRed}, {17, 96, texBlue}, {28, 96, texGreen}, {37, 96, texRed},
		{56, 56, texRed}, {45, 45, texGreen}, {18, 18, color.RGBA{255, 255, 255, 255}}, {5, 5, color.RGBA{255, 255, 255, 255}},
	} {
		if got := img.RGBAAt(p.x, p.y); got != p.want {
			t.Errorf("pixel %d,%d is %v, want %v", p.x, p.y, got, p.want)
		}
	}
}

func TestCanvasSemantics(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { t.Error(m) }})
	tr.SetViewport(gfx.Size{W: 400, H: 600})
	tr.Mount(Component(func() Node {
		name := UseElementRef()
		return div(
			canvasNode(flood),
			canvasNode(flood, AriaLabel("Progress")),
			p(AttachRef(name), Text("Sales")),
			canvasNode(flood, AriaLabelledBy(name)),
			canvasNode(flood, AriaLabel("Game"), Role("application"), TabIndex(0)),
		)
	}))
	tr.Frame()
	want := "- img \"Progress\"\n- paragraph: Sales\n- img \"Sales\"\n- application \"Game\""
	if got := strings.TrimSpace(tr.AriaSnapshot()); got != want {
		t.Errorf("snapshot:\n%s\nwant:\n%s", got, want)
	}
	if got := tr.Inspect().Children[0].Children[0].Kind; got != "canvas" {
		t.Errorf("inspect names the box %q", got)
	}
}

// A canvas takes the pointer and the keyboard as any element.
func TestCanvasInput(t *testing.T) {
	var moved, clicked, keys int
	var ref ElementRef
	x := newWinHarness(t, Options{Root: Component(func() Node {
		ref = UseElementRef()
		return canvasNode(flood, AttachRef(ref), Class("w-40 h-20"), TabIndex(0),
			On("pointermove", func(PointerEvent) { moved++ }),
			On("click", func(MouseEvent) { clicked++ }),
			On("keydown", func(KeyboardEvent) { keys++ }))
	})})
	main := x.windows()[0]
	main.SetFocused(true)
	x.click(main, 80, 40)
	if moved == 0 || clicked != 1 || !ref.Focused() {
		t.Errorf("%d moves, %d clicks, focused %v", moved, clicked, ref.Focused())
	}
	main.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyA, Key: "a", Text: "a"})
	x.drain()
	if keys != 1 {
		t.Errorf("%d keys", keys)
	}
}

func TestCanvasPanics(t *testing.T) {
	expect := func(what, want string, f func()) {
		t.Helper()
		defer func() {
			t.Helper()
			if r := fmt.Sprint(recover()); !strings.Contains(r, want) {
				t.Errorf("%s: recovered %q, want %q in it", what, r, want)
			}
		}()
		f()
	}
	expect("a nil function", "h.Canvas(nil)", func() { canvasNode(nil) })
	expect("no function", "CanvasDraw", func() { Primitive(style.KindCanvas) })
	expect("children", "overlay", func() { canvasNode(flood, Text("x")) })
	expect("a function on a div", "use Canvas", func() { div(CanvasDraw(flood)) })
	expect("alt", "Alt", func() { canvasNode(flood, Alt("x")) })
	expect("a texture", "use Texture", func() { canvasNode(flood, TextureSource(&Texture{})) })
}

// Without a frame clock a canvas draws once, at time 0, whatever it asks for.
func TestCanvasRender(t *testing.T) {
	var log drawLog
	_, img := Render(canvasNode(func(c *canvas.Canvas) {
		log.note(c)
		c.FillStyle = gfx.Color{R: 1, A: 1}
		flood(c)
		c.RequestFrame()
	}), RenderOptions{Config: Config{Fonts: testFonts}, Width: 400, Height: 300, Scale: 1.5})
	if len(log.all) != 1 || log.all[0].t != 0 || log.all[0].d != 0 {
		t.Errorf("draws %+v", log.all)
	}
	if c := img.RGBAAt(449, 224); c != texRed {
		t.Errorf("pixel %v, want the canvas's 300x150 at scale 1.5", c)
	}
	if c := img.RGBAAt(451, 100); c == texRed {
		t.Error("the canvas is larger than 300x150")
	}
}

// A draw runs once in a frame that builds again before it paints (a
// published placement), and a canvas that unmounts in it is forgotten.
func TestCanvasUnmountForgets(t *testing.T) {
	a := canvasApp{again: true}
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	a.mode.Set("gone")
	x.drain()
	a.log.take()
	x.drain()
	if n := a.log.take(); n != 0 || len(cnvOf(tr).wants) != 0 || len(cnvOf(tr).q) != 0 {
		t.Errorf("after its unmount: %d draws, %d waiting, %d queued", n, len(cnvOf(tr).wants), len(cnvOf(tr).q))
	}
	x.expectIdle("after its unmount")
	// Mounted again, it starts over.
	a.mode.Set("")
	x.b.Drain(x.h)
	x.step()
	if d := a.log.all[0]; d.t != 0 || d.d != 0 {
		t.Errorf("mounted again, its clock starts at %v, delta %v", d.t, d.d)
	}
}

// Canvases of several windows, a portal and a virtual list draw in their own
// trees; a Value shared by the windows draws both.
func TestCanvasInTwoWindows(t *testing.T) {
	var v *Value[float32]
	var log [2]drawLog
	draw := func(i int) func(c *canvas.Canvas) {
		return func(c *canvas.Canvas) {
			log[i].note(c)
			c.FillStyle = gfx.Color{R: v.Get(), A: 1}
			flood(c)
		}
	}
	x := newWinHarness(t, Options{Root: Component(func() Node {
		v = UseValue[float32](1)
		return div(canvasNode(draw(0), Class("w-40 h-20")), Component(func() Node {
			UseWindowSpec(ChildWindowSpec{WindowSpec: WindowSpec{Title: "child", Width: 200, Height: 100}, OnClose: func() {},
				Root: Portal(canvasNode(draw(1), Class("w-20 h-10")))})
			return nil
		}))
	})})
	x.drain()
	child := x.child(0)
	if a, b := x.pixel(10, 10), child.Frame().RGBAAt(10, 10); a != texRed || b != texRed {
		t.Errorf("the windows show %v and %v", a, b)
	}
	log[0].take()
	log[1].take()
	v.Set(0)
	x.drain()
	if a, b := x.pixel(10, 10), child.Frame().RGBAAt(10, 10); a != (color.RGBA{0, 0, 0, 255}) || b != a {
		t.Errorf("then %v and %v, want black", a, b)
	}
	if a, b := log[0].take(), log[1].take(); a != 1 || b != 1 {
		t.Errorf("%d and %d draws", a, b)
	}
	child.SetScale(2)
	x.drain()
	if a, b := log[0].take(), log[1].take(); a != 0 || b != 1 {
		t.Errorf("a scale of the child window: %d and %d draws", a, b)
	}
}

// A canvas that mounts, unmounts or exits during a frame, with a layout
// animation and a shared layout, stays whole.
func TestCanvasPresenceAndLayout(t *testing.T) {
	var log drawLog
	var setWhere Setter[int]
	x := newInput(t, Component(func() Node {
		where, set := UseState(0)
		setWhere = set
		draw := func(c *canvas.Canvas) {
			log.note(c)
			flood(c)
			c.RequestFrame()
		}
		kids := []Child{Class("p-4")}
		switch where {
		case 0:
			kids = append(kids, canvasNode(draw, Key("a"), LayoutID("c"), Class("w-40 h-20 fill-[#f00]")))
		case 1:
			kids = append(kids, div(Class("ml-40"), canvasNode(draw, Key("b"), LayoutID("c"), Class("w-20 h-40 fill-[#f00]"))))
		}
		var exit []Child
		if where < 2 {
			exit = append(exit, canvasNode(draw, Key("p"), Class("size-10 fill-[#00f] transition-opacity duration-100 exit:opacity-0")))
		}
		return div(div(kids...), Presence(exit...))
	}), 400, 300)
	for i := range 30 {
		switch i {
		case 5:
			setWhere.Set(1)
		case 12:
			setWhere.Set(2)
		case 25:
			setWhere.Set(0)
		}
		x.frame()
		expectFullRenderNow(t, x)
	}
	if len(log.all) < 30 {
		t.Errorf("%d draws", len(log.all))
	}
}

// expectFullRenderNow compares the window with a full rendering of the last
// frame, which may have asked for another.
func expectFullRenderNow(t *testing.T, x *inputHarness) {
	t.Helper()
	tr := x.tree()
	if tr.Dirty() {
		return
	}
	got, want := x.w.Frame(), tr.Rasterize(tr.LastFrame(), tr.Scale())
	if at, same := firstDiff(got, want); !same {
		t.Fatalf("pixel %v differs from a full rendering: %v, want %v", at, got.RGBAAt(at.X, at.Y), want.RGBAAt(at.X, at.Y))
	}
}

// The scratch of the draws is released at rest.
func TestCanvasTrim(t *testing.T) {
	a := canvasApp{}
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	if cnvOf(tr).rec == nil {
		t.Fatal("no recorder after a draw")
	}
	x.b.Advance(2 * RasterSettle)
	x.b.Wake()
	x.drain()
	if cnvOf(tr).rec != nil {
		t.Error("the recorder is kept at rest")
	}
	a.v.Set(0)
	x.drain()
	if c := x.pixel(60, 80); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("drawn after the trim: %v", c)
	}
}
