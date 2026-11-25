package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Transitions of the SVG paints (fill, stroke, stroke-width) of icons and
// canvases.

const (
	heartSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/></svg>`
	// discSVG has no presentation attributes.
	discSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/></svg>`
	// linear200 is the timing of the transitions here.
	linear200 = " duration-200 ease-linear "
)

// hoverAt moves the pointer over the group of a test, runs the frame that
// starts the transitions and returns their start, which is the start of
// those of the box with key k.
func (x *inputHarness) hoverAt(px, py float32, k string) time.Duration {
	x.t.Helper()
	x.settle()
	x.send(move(px, py))
	x.frame()
	return x.transStart(k)
}

func (x *inputHarness) props(k string) []anim.Prop {
	var out []anim.Prop
	if ba := x.cs(k).anim; ba != nil {
		for i := range ba.trans {
			out = append(out, ba.trans[i].Prop)
		}
	}
	return out
}

func nearPaint(a, b gfx.Color) bool {
	return near32(a.R, b.R, 2e-3) && near32(a.G, b.G, 2e-3) && near32(a.B, b.B, 2e-3) && near32(a.A, b.A, 2e-3)
}

var (
	pRed  = gfx.Color{R: 1, A: 1}
	pLime = gfx.Color{G: 1, A: 1}
	pBlue = gfx.Color{B: 1, A: 1}
)

// The paints of an icon and of a canvas transition like other colors, and a
// step paints them and does nothing else: no build, no style, no layout.
func TestPaintTransitionsOnlyPaint(t *testing.T) {
	var log drawLog
	var still drawLog
	const cls = "size-10 fill-[#ff0000] stroke-[#ff0000] stroke-1 group-hover:fill-[#0000ff] group-hover:stroke-[#00ff00] group-hover:stroke-3 transition-all" + linear200
	x := newInput(t, div(
		div(Class("group flex size-[100px]"),
			icon(heartSVG, Key("icon"), Class(cls)),
			canvasNode(func(c *canvas.Canvas) { log.note(c); flood(c) }, Key("canvas"), Class(cls)),
			canvasNode(func(c *canvas.Canvas) { still.note(c); flood(c) }, Class("size-5 fill-[#ff0000]")),
		)), 300, 200)
	start := x.hoverAt(5, 5, "icon")
	tr := x.tree()
	still.take()
	frames := 0
	for x.b.Now() < start+150*time.Millisecond {
		log.take()
		recs, before := tr.svgRecords, tr.Stats()
		x.frame()
		after := tr.Stats()
		if after.Frames == before.Frames {
			t.Fatal("no frame while it transitions")
		}
		frames++
		if after.Builds != before.Builds || after.Styles != before.Styles || after.Structures != before.Structures ||
			after.Layouts != before.Layouts || after.Remapped != before.Remapped || after.LaidOut != before.LaidOut {
			t.Fatalf("a step costs more than paint: %+v -> %+v", before, after)
		}
		if n := log.take(); n != 1 {
			t.Errorf("a step drew the canvas %d times", n)
		}
		if n := tr.svgRecords - recs; n != 1 {
			t.Errorf("a step recorded %d documents", n)
		}
		d := tr.LastDamage()
		for _, r := range d.Rects {
			if d.Full || r.Max.X > 84 || r.Max.Y > 44 {
				t.Errorf("a step damages %+v", d)
			}
		}
	}
	if frames < 5 || still.take() != 0 {
		t.Errorf("%d steps, or they drew another canvas", frames)
	}
	f := float32(x.b.Now()-start) / float32(200*time.Millisecond)
	for _, k := range []string{"icon", "canvas"} {
		cs := x.cs(k).cs
		if !nearPaint(cs.Fill.Color, gfx.Color{R: 1 - f, B: f, A: 1}) || !nearPaint(cs.Stroke.Color, gfx.Color{R: 1 - f, G: f, A: 1}) ||
			!near32(cs.StrokeWidth, 1+2*f, 1e-3) {
			t.Errorf("%s at %.2f: fill %v, stroke %v, width %v", k, f, cs.Fill, cs.Stroke, cs.StrokeWidth)
		}
	}
	x.settle()
	for _, k := range []string{"icon", "canvas"} {
		cs := x.cs(k)
		if cs.cs.Fill.Color != pBlue || cs.cs.Stroke.Color != pLime || cs.cs.StrokeWidth != 3 || cs.anim != nil {
			t.Errorf("%s at the end: fill %v, stroke %v, width %v, anim %v", k, cs.cs.Fill, cs.cs.Stroke, cs.cs.StrokeWidth, cs.anim)
		}
	}
	if log.last().fill != canvas.Style(pBlue) || log.last().line != 3 {
		t.Errorf("the canvas drew %+v last", log.last())
	}
	expectFullRenderNow(t, x)
	// Idle: no frame is asked for, and none draws.
	before := tr.Stats().Frames
	log.take()
	x.b.Advance(time.Second)
	x.b.Drain(x.h)
	if n := tr.Stats().Frames - before; n != 0 || log.take() != 0 || tr.Animating() {
		t.Errorf("%d frames at rest", n)
	}
}

// A transition of an ancestor moves what inherits from it at every step,
// with no transition of its own, whatever the timing of its classes (as for
// the text color), and nothing else.
func TestInheritedPaintTransition(t *testing.T) {
	var log, own drawLog
	note := func(l *drawLog) func(*canvas.Canvas) { return func(c *canvas.Canvas) { l.note(c); flood(c) } }
	x := newInput(t, div(
		div(Key("p"), Class("flex size-[100px] fill-[#ff0000] stroke-1 hover:fill-[#0000ff] hover:stroke-3 transition-all"+linear200),
			div(Key("mid"), Text("text"), icon(heartSVG, Key("icon"), Class("size-5 transition-all duration-500"))),
			canvasNode(note(&log), Key("canvas"), Class("size-5")),
			canvasNode(note(&own), Class("size-5 fill-[#00ff00] stroke-2")),
		)), 300, 200)
	start := x.hoverAt(90, 90, "p")
	own.take()
	tr := x.tree()
	for range 5 {
		log.take()
		before := tr.Stats()
		x.frame()
		after := tr.Stats()
		if after.Builds != before.Builds || after.Structures != before.Structures || after.LaidOut != before.LaidOut || after.Shapes != before.Shapes {
			t.Errorf("a step builds or lays out: %+v -> %+v", before, after)
		}
		if n := log.take(); n != 1 {
			t.Errorf("a step drew the canvas %d times", n)
		}
	}
	x.advanceTo(start + 100*time.Millisecond)
	for _, k := range []string{"p", "mid", "icon", "canvas"} {
		cs := x.cs(k).cs
		if !nearPaint(cs.Fill.Color, gfx.Color{R: 0.5, B: 0.5, A: 1}) || cs.StrokeWidth != 2 {
			t.Errorf("%s in the middle: fill %v, width %v", k, cs.Fill, cs.StrokeWidth)
		}
	}
	if !nearPaint(log.last().fill.(gfx.Color), gfx.Color{R: 0.5, B: 0.5, A: 1}) || log.last().line != 2 {
		t.Errorf("the canvas drew %+v", log.last())
	}
	if p := x.props("icon"); p != nil {
		t.Errorf("the icon transitions %v, which it inherits", p)
	}
	if own.take() != 0 {
		t.Error("a canvas with paints of its own drew")
	}
	x.settle()
	if cs := x.cs("icon").cs; cs.Fill.Color != pBlue || cs.StrokeWidth != 3 {
		t.Errorf("at the end: %v, %v", cs.Fill, cs.StrokeWidth)
	}
	expectFullRenderNow(t, x)

	// A value of its own in the same restyle transitions.
	y := newInput(t, div(
		div(Class("group size-[100px] fill-[#ff0000] hover:fill-[#0000ff]"),
			icon(heartSVG, Key("own"), Class("size-5 fill-[#ff0000] group-hover:fill-[#00ff00] transition"+linear200)),
			icon(heartSVG, Key("inh"), Class("size-5 transition"+linear200)))), 300, 200)
	start = y.hoverAt(90, 90, "own")
	y.advanceTo(start + 100*time.Millisecond)
	if c := y.cs("own").cs.Fill.Color; !nearPaint(c, gfx.Color{R: 0.5, G: 0.5, A: 1}) {
		t.Errorf("a fill of its own is %v", c)
	}
	if c := y.cs("inh").cs.Fill.Color; c != pBlue {
		t.Errorf("the inherited fill is %v, want the parent's", c)
	}

	// So does its end, when the element goes back to what it inherits or
	// draws, whatever else of its parent it inherits changes with it.
	z := newInput(t, div(
		div(Class("group size-[100px] text-[#000000] hover:text-[#ffffff] transition-colors"+linear200),
			icon(discSVG, Key("i"), Class("size-5 group-hover:fill-[#0000ff] transition-[fill]"+linear200)),
			canvasNode(flood, Key("c"), Class("size-5 group-hover:stroke-3 transition-all"+linear200)))), 300, 200)
	z.hoverAt(90, 90, "i")
	z.settle()
	z.send(move(250, 150))
	z.frame()
	start = z.transStart("i")
	z.advanceTo(start + 100*time.Millisecond)
	if c := z.cs("i").cs.Fill.Color; !nearPaint(c, gfx.Color{B: 0.5, A: 1}) {
		t.Errorf("back, the fill is %v in the middle", c)
	}
	if w := z.cs("c").cs.StrokeWidth; !near32(w, 2, 1e-3) {
		t.Errorf("back, the stroke width is %v in the middle", w)
	}
	z.settle()
	if z.cs("i").cs.Fill.Kind != style.SVGPaintUnset || z.cs("c").cs.StrokeWidthSet {
		t.Error("back at rest, the paints are set")
	}
}

// fill-current and stroke-current are the text color of each moment: with
// it, they neither lag nor transition a second time, and a transition from
// or to them mixes the current text color (Chromium 155: 143 48 64 and 48
// 16 191 at a quarter).
func TestCurrentColorPaintTransitions(t *testing.T) {
	x := newInput(t, div(
		div(Class("group flex size-[100px] text-[#ff0000] hover:text-[#00ff00] transition-colors"+linear200),
			icon(heartSVG, Key("follows"), Class("size-5 fill-current transition-colors"+linear200)),
			icon(heartSVG, Key("own"), Class("size-5 fill-current text-[#ff0000] group-hover:text-[#00ff00] transition-colors"+linear200)),
			icon(heartSVG, Key("from"), Class("size-5 fill-current group-hover:fill-[#0000ff] transition-colors"+linear200)),
			icon(heartSVG, Key("to"), Class("size-5 fill-[#0000ff] group-hover:fill-current transition-colors"+linear200)),
			icon(heartSVG, Key("snap"), Class("size-5 fill-[#0000ff] text-[#ff0000] group-hover:fill-current group-hover:text-[#00ff00] transition-[fill]"+linear200)),
		)), 300, 200)
	start := x.hoverAt(90, 90, "from")
	for _, c := range []struct {
		k    string
		want string
	}{{"follows", "[]"}, {"own", "[color border-color outline-color]"}, {"from", "[fill]"}, {"to", "[fill]"}, {"snap", "[fill]"}} {
		if got := fmt.Sprint(x.props(c.k)); got != c.want {
			t.Errorf("%s transitions %s, want %s", c.k, got, c.want)
		}
	}
	x.advanceTo(start + 50*time.Millisecond)
	mid := gfx.RGBA8(191, 64, 0, 255)
	for _, c := range []struct {
		k    string
		want gfx.Color
	}{{"follows", mid}, {"own", mid}, {"from", gfx.RGBA8(143, 48, 64, 255)}, {"to", gfx.RGBA8(48, 16, 191, 255)}, {"snap", gfx.RGBA8(0, 64, 191, 255)}} {
		cs := x.cs(c.k).cs
		if got := cs.Fill.Resolve(cs.Text.Color); !near32(got.R, c.want.R, 4e-3) || !near32(got.G, c.want.G, 4e-3) || !near32(got.B, c.want.B, 4e-3) {
			t.Errorf("%s at a quarter: %v, want %v", c.k, got, c.want)
		}
	}
	x.settle()
	if f := x.cs("to").cs.Fill; f.Kind != style.SVGPaintCurrent {
		t.Errorf("the transition to currentColor ends at %+v", f)
	}

	// The fill of an icon whose text color starts later has the text color
	// of each step, too.
	var set Setter[string]
	y := newInput(t, Component(func() Node {
		cls, s := UseState("text-[#ff0000] fill-current")
		set = s
		return div(icon(heartSVG, Key("i"), Class("size-5 transition-[fill,color] duration-1000 ease-linear "+cls)))
	}), 300, 200)
	y.settle()
	set.Set("text-[#ff0000] fill-[#0000ff]")
	y.frame()
	fill := y.transStart("i")
	y.advanceTo(fill + 250*time.Millisecond)
	set.Set("text-[#00ff00] fill-[#0000ff]")
	y.b.Drain(y.h)
	y.frame()
	if got := fmt.Sprint(y.props("i")); got != "[color fill]" {
		t.Fatalf("it transitions %s", got)
	}
	col := y.transStart("i")
	y.advanceTo(fill + 750*time.Millisecond)
	c := float32(fill+750*time.Millisecond-col) / float32(time.Second)
	want := gfx.Color{R: 0.25 * (1 - c), G: 0.25 * c, B: 0.75, A: 1}
	if got := y.cs("i").cs.Fill.Color; c < 0.4 || !nearPaint(got, want) {
		t.Errorf("the fill is %v, want %v: not the text color of the step", got, want)
	}
}

// What has no paint of its own transitions from what it shows: an icon from
// the attributes of its root, as in browsers, a canvas from its text color.
// none, and an ancestor without a paint, do not interpolate.
func TestUnsetPaintTransitions(t *testing.T) {
	const hover = " text-[#ff0000] group-hover:fill-[#0000ff] group-hover:stroke-[#0000ff] group-hover:stroke-4 transition-all" + linear200
	var log drawLog
	grad := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2" fill="url(#g)" stroke-width="10%"><linearGradient id="g"/><rect width="2" height="2"/></svg>`
	x := newInput(t, div(
		div(Class("group flex size-[100px]"),
			icon(heartSVG, Key("icon"), Class("size-5"+hover)),
			icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><rect width="2" height="2"/></svg>`, Key("bare"), Class("size-5"+hover)),
			icon(grad, Key("grad"), Class("size-5"+hover)),
			img(SrcBytes([]byte(heartSVG)), Key("img"), Class("size-5"+hover)),
			canvasNode(log.note, Key("canvas"), Class("size-5"+hover)),
			div(Key("div"), Class("size-5"+hover)),
		)), 300, 200)
	start := x.hoverAt(90, 90, "icon")
	for _, c := range []struct{ k, want string }{
		{"icon", "[stroke stroke-width]"}, {"img", "[stroke stroke-width]"},
		{"bare", "[fill stroke-width]"}, {"grad", "[]"},
		{"canvas", "[fill stroke stroke-width]"}, {"div", "[]"},
	} {
		if got := fmt.Sprint(x.props(c.k)); got != c.want {
			t.Errorf("%s transitions %s, want %s", c.k, got, c.want)
		}
	}
	x.advanceTo(start + 100*time.Millisecond)
	half := gfx.Color{R: 0.5, B: 0.5, A: 1}
	if cs := x.cs("icon").cs; cs.Fill.Color != pBlue || !nearPaint(cs.Stroke.Color, half) || cs.StrokeWidth != 3 {
		t.Errorf("the icon: fill %v, stroke %v, width %v", cs.Fill, cs.Stroke, cs.StrokeWidth)
	}
	if cs := x.cs("bare").cs; !nearPaint(cs.Fill.Color, gfx.Color{B: 0.5, A: 1}) || cs.Stroke.Color != pBlue || cs.StrokeWidth != 2.5 {
		t.Errorf("without attributes: fill %v, stroke %v, width %v", cs.Fill, cs.Stroke, cs.StrokeWidth)
	}
	if d := log.last(); !nearPaint(d.fill.(gfx.Color), half) || !nearPaint(d.stroke.(gfx.Color), half) || d.line != 2.5 {
		t.Errorf("the canvas: %+v", d)
	}
	if cs := x.cs("div").cs; cs.Fill.Color != pBlue || cs.StrokeWidth != 4 {
		t.Errorf("a box that draws nothing: fill %v, width %v", cs.Fill, cs.StrokeWidth)
	}
	// Back, from the middle: a shorter transition to the unset values.
	x.send(move(250, 150))
	x.frame()
	if got := fmt.Sprint(x.props("icon")); got != "[stroke stroke-width]" {
		t.Errorf("back, the icon transitions %s", got)
	}
	for i := range x.cs("icon").anim.trans {
		if tr := &x.cs("icon").anim.trans[i]; tr.Duration > 120*time.Millisecond || tr.Duration < 80*time.Millisecond {
			t.Errorf("the way back of %v takes %v, want about half", tr.Prop, tr.Duration)
		}
	}
	x.b.Advance(50 * time.Millisecond)
	x.b.Drain(x.h)
	if cs := x.cs("icon").cs; !near32(cs.StrokeWidth, 2.5, 0.2) || cs.Stroke.Color.R < 0.6 || cs.Stroke.Color.R > 0.9 {
		t.Errorf("on the way back: stroke %v, width %v", cs.Stroke, cs.StrokeWidth)
	}
	x.settle()
	if cs := x.cs("icon").cs; cs.Stroke.Kind != style.SVGPaintUnset || cs.StrokeWidthSet {
		t.Errorf("back at rest: stroke %v, width %v", cs.Stroke, cs.StrokeWidth)
	}
	if d := log.last(); d.fill != canvas.Style(pRed) || d.line != 1 {
		t.Errorf("the canvas at rest: %+v", d)
	}
	expectFullRenderNow(t, x)

	// A canvas whose fill becomes unset halfway keeps going: it is the same.
	var set Setter[string]
	y := newInput(t, Component(func() Node {
		cls, s := UseState("fill-[#0000ff]")
		set = s
		return div(canvasNode(log.note, Key("c"), Class("size-5 transition-colors"+linear200+cls)))
	}), 300, 200)
	y.settle()
	set.Set("fill-current")
	y.frame()
	start = y.transStart("c")
	y.advanceTo(start + 100*time.Millisecond)
	set.Set("")
	y.b.Drain(y.h)
	y.frame()
	if at := y.transStart("c"); at != start {
		t.Errorf("it started again at %v", at-start)
	}
	if c := y.cs("c").cs.Fill.Color; c.B < 0.3 || c.B > 0.5 {
		t.Errorf("its fill is %v", c)
	}
	y.settle()
}

// No transitions of the paints with reduced motion, nor for a new theme or
// scheme; starting: and Exit classes transition them.
func TestPaintTransitionsEnvironment(t *testing.T) {
	const cls = "size-10 fill-[#ff0000] hover:fill-[#0000ff] dark:fill-[#00ff00] transition-colors" + linear200
	b := headless.New(headless.Options{Size: platform.Size{W: 200, H: 200}, Settings: platform.SystemSettings{ReducedMotion: true}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: div(icon(heartSVG, Key("i"), Class(cls)))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.stopTimer)
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	x.settle()
	x.send(move(10, 10))
	x.frame()
	if f := x.cs("i").cs.Fill.Color; f != pBlue || x.tree().Animating() {
		t.Errorf("with reduced motion: %v, animating %v", f, x.tree().Animating())
	}

	for name, root := range map[string]Node{
		"icon":   div(icon(heartSVG, Key("i"), Class(cls))),
		"parent": div(div(Key("i"), Class(cls), icon(heartSVG))),
		"canvas": div(canvasNode(flood, Key("i"), Class(cls))),
	} {
		x := newInput(t, root, 200, 200)
		start := x.hoverAt(10, 10, "i")
		x.advanceTo(start + 100*time.Millisecond)
		x.tree().SetColorScheme(ColorSchemeDark)
		x.frame()
		// hover: comes before dark: in the cascade of the classes.
		if f := x.cs("i").cs.Fill.Color; f != pLime || x.tree().Animating() {
			t.Errorf("%s, a new scheme: %v, animating %v", name, f, x.tree().Animating())
		}
	}

	var set Setter[bool]
	y := newInput(t, Component(func() Node {
		on, s := UseState(false)
		set = s
		return div(Presence(If(on, canvasNode(flood, Key("i"),
			Class("size-10 fill-[#ff0000] starting:fill-[#0000ff] transition-colors"+linear200), Exit("fill-[#00ff00]")))))
	}), 200, 200)
	y.settle()
	set.Set(true)
	y.frame()
	start := y.transStart("i")
	y.advanceTo(start + 100*time.Millisecond)
	if f := y.cs("i").cs.Fill.Color; !nearPaint(f, gfx.Color{R: 0.5, B: 0.5, A: 1}) {
		t.Errorf("entering: %v", f)
	}
	y.settle()
	set.Set(false)
	y.frame()
	start = y.transStart("i")
	y.advanceTo(start + 100*time.Millisecond)
	if f := y.cs("i").cs.Fill.Color; !nearPaint(f, gfx.Color{R: 0.5, G: 0.5, A: 1}) {
		t.Errorf("leaving: %v", f)
	}
	y.settle()
	if keyed(y.tree(), "i") != nil {
		t.Error("it is still mounted after its exit")
	}
}

// paintScene shows icons and canvases whose paints transition when the
// pointer is over them.
func paintScene() Node {
	ring := func(c *canvas.Canvas) {
		c.FillRect(4, 4, c.Width-8, c.Height-8)
		c.StrokeRect(4, 4, c.Width-8, c.Height-8)
	}
	const t = " transition-all" + linear200
	return div(Class("group flex flex-wrap gap-3 p-3 bg-white size-full text-slate-400 hover:text-emerald-600"+t),
		icon(heartSVG, Key("icon"), Class("size-12 stroke-slate-500 fill-slate-100 group-hover:fill-rose-500 group-hover:stroke-rose-700"+t)),
		icon(heartSVG, Class("size-12 group-hover:stroke-sky-600 group-hover:stroke-3"+t)),
		icon(heartSVG, Class("size-12 group-hover:fill-current")),
		canvasNode(ring, Class("size-12 fill-amber-200 stroke-amber-500 group-hover:fill-violet-600 group-hover:stroke-4"+t)),
		canvasNode(ring, Class("size-12 group-hover:fill-sky-300 group-hover:stroke-8"+t)),
		div(Class("flex gap-3 rounded-xl p-2 border border-slate-200 fill-sky-200 stroke-1 hover:fill-rose-500 hover:stroke-2"+t),
			icon(heartSVG, Class("size-8 rotate-12 opacity-80")),
			canvasNode(ring, Class("size-8 stroke-sky-700 rounded-lg")),
			canvasNode(ring, Class("size-8 fill-none blur-[1px]"))),
		img(SrcBytes([]byte(heartSVG)), Class("size-12 group-hover:stroke-rose-500"+t)),
		div(Class("text-sm"), Text("A heart "), icon(heartSVG, Class("inline size-5 stroke-slate-500 group-hover:stroke-rose-600 group-hover:stroke-4"+t)), Text(" inline")),
	)
}

func TestPaintTransitionsDamage(t *testing.T) {
	x := newDamageHarness(t, paintScene(), 260, 180, 2, Config{}, 3)
	hover := func(px, py float32) {
		x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: px, Y: py}})
	}
	x.frame("start")
	hover(36, 36)
	x.frame("hover")
	if ba := keyed(x.tree(), "icon").box.anim; ba == nil || len(ba.trans) < 2 {
		t.Fatalf("no transitions: %+v", ba)
	}
	for i := range 8 {
		x.frame(fmt.Sprintf("hover %d", i))
	}
	hover(80, 90)
	for i := range 16 {
		x.frame(fmt.Sprintf("the inner group %d", i))
	}
	x.inject(platform.PointerEvent{Action: platform.PointerLeave})
	for i := range 20 {
		x.frame(fmt.Sprintf("away %d", i))
	}
	if x.partial == 0 || x.tree().Animating() {
		t.Errorf("%d partial frames, animating %v", x.partial, x.tree().Animating())
	}
}

// Icons and canvases in the middle of their transitions. Regenerate with
// TELA_UPDATE=1 and look at it.
func TestPaintTransitionsGolden(t *testing.T) {
	b := headless.New(headless.Options{Size: platform.Size{W: 260, H: 180}, Scale: 2})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: paintScene()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.stopTimer)
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	start := x.hoverAt(80, 90, "icon")
	for _, at := range []time.Duration{0, 100, 200} {
		x.advanceTo(start + at*time.Millisecond)
		golden.Image(t, fmt.Sprintf("paint_transition_%d", at), x.w.Frame())
	}
}
