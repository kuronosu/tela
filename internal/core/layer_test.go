package core

import (
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

// The invariant of layers and damage (layer.go): after any sequence of
// changes, the pixels a window shows (the framebuffer rasterized
// incrementally, presented with its damage) are bit-identical to a full
// rasterization of the tree composed from scratch (Tree.ReferenceFrame).

// damageHarness runs a tree in a headless window and checks every
// presented frame against the reference.
type damageHarness struct {
	t     *testing.T
	b     *headless.Backend
	h     *Host
	w     *headless.Window
	rng   *rand.Rand
	step  int
	pos   gfx.Point
	trace []string
	// Counters of what the frames did.
	frames, full, shifted, partial, skipped int
}

func newDamageHarness(t *testing.T, root Node, w, h float32, scale float64, cfg Config, seed uint64) *damageHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}, Scale: scale})
	cfg.Fonts = testFonts
	host, err := NewHost(b, Options{Config: cfg, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	x := &damageHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window), rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
	t.Cleanup(host.stopTimer)
	b.Drain(host)
	x.check("mount")
	return x
}

func (x *damageHarness) tree() *Tree { return x.h.Tree() }

// frame advances the clock by a frame and handles what is queued, then
// checks the pixels.
func (x *damageHarness) frame(what string) {
	x.b.Advance(x.b.FrameInterval())
	x.b.Drain(x.h)
	x.check(what)
}

func (x *damageHarness) inject(e platform.Event) {
	x.w.Inject(e)
	x.b.Drain(x.h)
}

// check compares the window's pixels with the reference.
func (x *damageHarness) check(what string) {
	x.t.Helper()
	x.step++
	x.trace = append(x.trace, what)
	if len(x.trace) > 12 {
		x.trace = x.trace[1:]
	}
	tr := x.tree()
	if tr.Dirty() {
		// The frame left work for the next one (a fade, a hover after
		// content moved): the window shows an older state than the tree.
		x.skipped++
		return
	}
	d := tr.LastDamage()
	switch {
	case d.Full:
		x.full++
	case len(d.Shifts) > 0:
		x.shifted++
	case len(d.Rects) > 0:
		x.partial++
	}
	x.frames++
	got := x.w.Frame()
	if got == nil {
		x.t.Fatalf("step %d (%s): nothing presented", x.step, what)
	}
	for _, noCull := range []bool{false, x.step%7 == 0} {
		want := tr.Rasterize(tr.ReferenceFrame(noCull), tr.Scale())
		if tr.Debug() != 0 && tr.dbg.prev != nil {
			// The window holds the application and the overlays drawn
			// over it (debug.go): the reference is the same.
			var r raster.Renderer
			r.Render(want, tr.dbg.prev, raster.Options{Scale: tr.Scale()})
		}
		if p, ok := firstDiff(got, want); !ok {
			dir := os.Getenv("TELA_DAMAGE_DIR")
			if dir == "" {
				dir = x.t.TempDir()
			}
			save(filepath.Join(dir, "got.png"), got)
			save(filepath.Join(dir, "want.png"), want)
			_ = os.WriteFile(filepath.Join(dir, "got.txt"), []byte(tr.lay.cur.dl.String()), 0o644)
			_ = os.WriteFile(filepath.Join(dir, "want.txt"), []byte(tr.ReferenceFrame(noCull).String()), 0o644)
			x.t.Fatalf("step %d (%s), noCull %v: pixel %v differs: got %v, want %v\nlast damage %+v\nlast steps: %s\nimages in %s",
				x.step, what, noCull, p, got.RGBAAt(p.X, p.Y), want.RGBAAt(p.X, p.Y), d, strings.Join(x.trace, " | "), dir)
		}
		if !noCull {
			continue
		}
	}
}

func firstDiff(a, b *image.RGBA) (image.Point, bool) {
	if a.Rect != b.Rect {
		return a.Rect.Max, false
	}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		ra := a.Pix[a.PixOffset(a.Rect.Min.X, y):][:4*a.Rect.Dx()]
		rb := b.Pix[b.PixOffset(b.Rect.Min.X, y):][:4*b.Rect.Dx()]
		if string(ra) == string(rb) {
			continue
		}
		for i := range ra {
			if ra[i] != rb[i] {
				return image.Pt(a.Rect.Min.X+i/4, y), false
			}
		}
	}
	return image.Point{}, true
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

// Operations.

func (x *damageHarness) move(px, py float32) {
	x.pos = gfx.Point{X: px, Y: py}
	x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: px, Y: py}})
}

func (x *damageHarness) randomPoint() (float32, float32) {
	m := x.w.Metrics()
	return float32(x.rng.Float64()) * m.Size.W, float32(x.rng.Float64()) * m.Size.H
}

// randomOp applies a random generic operation (pointer, wheel, touchpad,
// time) and reports what it did.
func (x *damageHarness) randomOp() string {
	switch r := x.rng.IntN(100); {
	case r < 25:
		px, py := x.randomPoint()
		x.move(px, py)
		return fmt.Sprintf("move(%.0f,%.0f)", px, py)
	case r < 50:
		steps := float32(x.rng.IntN(5) - 2)
		if steps == 0 {
			steps = 1
		}
		e := platform.ScrollEvent{Pos: platform.Point{X: x.pos.X, Y: x.pos.Y}, Steps: platform.Point{Y: steps}}
		if x.rng.IntN(4) == 0 {
			e.Steps = platform.Point{X: steps}
		}
		x.inject(e)
		return fmt.Sprintf("wheel(%v)", e.Steps)
	case r < 65:
		d := float32(x.rng.IntN(161) - 80)
		x.inject(platform.ScrollEvent{Pos: platform.Point{X: x.pos.X, Y: x.pos.Y}, Delta: platform.Point{Y: d}, Phase: platform.ScrollPhaseUpdate})
		return fmt.Sprintf("touchpad(%g)", d)
	case r < 70:
		x.inject(platform.ScrollEvent{Pos: platform.Point{X: x.pos.X, Y: x.pos.Y}, Phase: platform.ScrollPhaseEnd})
		return "touchpad-end"
	case r < 80:
		x.b.Advance(time.Duration(x.rng.IntN(400)) * time.Millisecond)
		x.b.Wake()
		x.b.Drain(x.h)
		return "wait"
	case r < 85:
		x.inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: x.pos.X, Y: x.pos.Y},
			Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()})
		return "down"
	case r < 90:
		x.inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: x.pos.X, Y: x.pos.Y},
			Button: platform.ButtonPrimary})
		return "up"
	}
	return "frame"
}

// ---------------------------------------------------------------------
// Scenes.

// sceneState holds the handles a scene exposes to the operations.
type sceneState struct {
	refs    []ElementRef
	setters []Setter[int]
	values  []*Value[float32]
}

// rowScene: a header and a list of rows in a scroll container over an
// opaque background (the shift path), rows with hover colors, text that
// changes, positioned badges, gradients and images (re-rasterized), and a
// rounded, shadowed card inside the list.
func rowScene(st *sceneState, rows int, listClass string) Node {
	row := func(i int) Node {
		v, set := UseState(0)
		st.setters[i] = set
		label := fmt.Sprintf("Row %d", i)
		if v%2 == 1 {
			label = fmt.Sprintf("Changed %d (%d)", i, v)
		}
		cls := "flex h-[34px] items-center gap-2 px-3 text-sm hover:bg-slate-100"
		if v%3 == 2 {
			cls += " bg-amber-100"
		}
		var extra []Child
		if i%5 == 0 {
			extra = append(extra, div(Class("h-4 w-10 rounded-full bg-linear-to-r from-sky-400 to-pink-500")))
		}
		if i%7 == 0 {
			extra = append(extra, div(Class("relative size-5 rounded bg-indigo-200"),
				div(Class("absolute -top-1 -right-1 size-2.5 rounded-full bg-rose-500"))))
		}
		if i%11 == 0 {
			extra = append(extra, div(Class("ml-auto rounded-lg px-2 shadow-md ring-1 ring-black/10"), Text("card")))
		}
		c := []Child{Class(cls), span(Class("w-8 text-slate-400"), Textf("%d", i)), span(Text(label))}
		c = append(c, extra...)
		return div(c...)
	}
	comp := func() Node {
		st.refs[0] = UseElementRef()
		items := make([]Child, rows)
		for i := range items {
			items[i] = WithKey(ComponentProps(row, i), i)
		}
		list := append([]Child{AttachRef(st.refs[0]), Key("list"), Class(listClass)}, items...)
		return div(Class("flex h-screen flex-col bg-slate-50"),
			div(Class("h-10 shrink-0 bg-slate-800 px-3 text-white"), Text("Header")),
			div(list...),
		)
	}
	return Component(comp)
}

// pageScene: a page taller than the window (the viewport scrolls) with a
// fixed blurred header, a rounded inner list, a horizontal strip of
// shadowed cards, z-ordered positioned boxes, a filter and a backdrop
// filter, and boxes moved by TranslateFn and OpacityFn.
func pageScene(st *sceneState) Node {
	return pageSceneWith(st, "relative bg-linear-to-b from-white to-slate-100", "fixed top-0 right-0 left-0 z-20 h-9 bg-white/70 px-3 backdrop-blur")
}

// plainPageScene is pageScene on a white page with an opaque fixed
// header: the viewport's content shifts.
func plainPageScene(st *sceneState) Node {
	return pageSceneWith(st, "relative bg-white", "fixed top-0 right-0 left-0 z-20 h-9 bg-slate-100 px-3 shadow")
}

func pageSceneWith(st *sceneState, pageClass, headerClass string) Node {
	comp := func() Node {
		st.refs[0], st.refs[1] = UseElementRef(), UseElementRef()
		st.values[0] = UseValue[float32](0)
		st.values[1] = UseValue[float32](1)
		v, set := UseState(0)
		st.setters[0] = set
		x, o := st.values[0], st.values[1]
		inner := []Child{AttachRef(st.refs[0]), Key("inner"), Class("mx-4 h-[150px] overflow-y-auto rounded-xl border border-slate-200 bg-white shadow-lg")}
		for i := range 30 {
			inner = append(inner, div(Key(i), Class("h-7 px-2 text-sm odd:bg-slate-50 hover:bg-sky-100"), Textf("Inner %d", i)))
		}
		strip := []Child{AttachRef(st.refs[1]), Key("strip"), Class("flex gap-4 overflow-x-auto p-4")}
		for i := range 8 {
			strip = append(strip, div(Key(i), Class("h-20 w-28 shrink-0 rounded-2xl bg-white p-2 text-xs shadow-xl"), Textf("Card %d", i)))
		}
		title := "Page"
		if v%2 == 1 {
			title = "Page (changed)"
		}
		return div(Class(pageClass),
			div(Class(headerClass), Text(title)),
			div(Class("h-12")),
			div(inner...),
			div(strip...),
			div(Class("relative mx-4 h-28"),
				div(Class("absolute top-2 left-2 z-10 size-16 rounded-xl bg-sky-400/80")),
				div(Class("absolute top-6 left-8 size-16 rounded-xl bg-pink-400/80")),
				div(Class("absolute top-10 left-14 -z-10 size-16 rounded-xl bg-amber-300")),
				div(Class("absolute top-4 left-40 size-14 rounded-full bg-violet-500 blur-sm")),
				div(Class("absolute top-4 left-56 size-14 rotate-12 rounded-lg bg-emerald-400 shadow-lg")),
			),
			div(Class("mx-4 h-16 rounded-xl bg-indigo-500 shadow-lg"),
				Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }), style.OpacityFn(func() float32 { return o.Get() }))),
			div(Class("h-[500px] px-4 pt-4 text-sm text-slate-500"), Text("Tall content")),
		)
	}
	return Component(comp)
}

// glassScene: a transparent scroll container over a gradient (no shift:
// its backdrop is not uniform) whose content has glass cards with
// backdrop filters, blurred and drop-shadowed boxes and opacity groups.
func glassScene(st *sceneState) Node {
	comp := func() Node {
		st.refs[0] = UseElementRef()
		v, set := UseState(0)
		st.setters[0] = set
		c := []Child{AttachRef(st.refs[0]), Key("glass"), Class("h-screen overflow-y-auto")}
		for i := range 10 {
			cls := "m-3 h-16 rounded-2xl border border-white/30 bg-white/20 p-3 text-sm backdrop-blur-md"
			switch i % 4 {
			case 1:
				cls = "m-3 h-16 rounded-2xl bg-white/60 p-3 text-sm opacity-80 shadow-xl"
			case 2:
				cls = "m-3 h-16 rounded-2xl bg-rose-300 p-3 text-sm drop-shadow-lg"
			case 3:
				cls = "m-3 h-16 rounded-2xl bg-sky-300 p-3 text-sm blur-[1px]"
			}
			if i == v%10 {
				cls += " ring-4 ring-amber-400"
			}
			c = append(c, div(Key(i), Class(cls), Textf("Glass %d", i)))
		}
		return div(Class("bg-linear-to-br from-indigo-500 via-purple-500 to-pink-400"), div(c...))
	}
	return Component(comp)
}

// boardScene: columns of cards in a horizontal scroll container, some
// cards moved and faded by TranslateFn and OpacityFn (a drag), one card
// raised with z-index while it moves.
func boardScene(st *sceneState) Node {
	comp := func() Node {
		st.refs[0] = UseElementRef()
		for k := range 4 {
			st.values[k] = UseValue[float32](0)
		}
		v, set := UseState(0)
		st.setters[0] = set
		cols := []Child{AttachRef(st.refs[0]), Key("board"), Class("flex h-screen gap-3 overflow-x-auto bg-slate-100 p-3")}
		for c := range 3 {
			col := []Child{Key(c), Class("flex w-40 shrink-0 flex-col gap-2 rounded-xl bg-slate-200 p-2")}
			for k := range 4 {
				cls := "h-14 rounded-lg bg-white p-2 text-xs shadow-sm"
				var props []Child
				if k == c {
					x := st.values[k]
					cls += " relative"
					if v%2 == 1 {
						cls += " z-10 shadow-xl"
					}
					props = append(props, Style(style.TranslateFn(func() (float32, float32) { return x.Get(), x.Get() / 3 })))
				}
				if k == 3 && c == 2 {
					o := st.values[3]
					props = append(props, Style(style.OpacityFn(func() float32 { return 1 - o.Get()/120 })))
				}
				props = append(props, Key(k), Class(cls), Textf("Card %d.%d", c, k))
				col = append(col, div(props...))
			}
			cols = append(cols, div(col...))
		}
		return div(cols...)
	}
	return Component(comp)
}

// virtualScene: a header over a 5,000-item VirtualList whose items have
// variable heights (measured as they come), hover colors, state, gradient
// chips and positioned badges.
func virtualScene(st *sceneState) Node {
	item := func(i int) Node {
		v, set := UseState(0)
		if i < len(st.setters) {
			st.setters[i] = set
		}
		cls := "flex items-center gap-2 border-b border-slate-100 px-3 text-sm hover:bg-sky-50"
		h := 28 + (i*7)%23
		if v%2 == 1 {
			h += 20
			cls += " bg-amber-50"
		}
		c := []Child{Class(cls), Style(style.Height(float32(h))), span(Class("w-12 text-slate-400"), Textf("%d", i)), span(Textf("Item %d", i))}
		if i%6 == 0 {
			c = append(c, div(Class("h-3 w-8 rounded-full bg-linear-to-r from-sky-400 to-pink-500")))
		}
		if i%9 == 0 {
			c = append(c, div(Class("relative size-4 rounded bg-indigo-200"), div(Class("absolute -top-1 -right-1 size-2 rounded-full bg-rose-500"))))
		}
		return div(c...)
	}
	comp := func() Node {
		st.refs[0] = UseElementRef()
		return div(Class("flex h-screen flex-col bg-slate-50"),
			div(Class("h-10 shrink-0 bg-slate-800 px-3 text-white"), Text("Virtual")),
			VirtualList(5000, func(int) float32 { return 36 }, func(i int) Node { return WithKey(ComponentProps(item, i), i) },
				AttachRef(st.refs[0]), Class("grow bg-white")),
		)
	}
	return Component(comp)
}

// runScene drives a scene with random operations and scene-specific ones.
func runScene(t *testing.T, name string, root func(*sceneState) Node, ops func(*damageHarness, *sceneState) string, cfg Config, steps int) {
	for _, scale := range damageScales() {
		t.Run(fmt.Sprintf("%s_x%g", name, scale), func(t *testing.T) {
			st := &sceneState{refs: make([]ElementRef, 4), setters: make([]Setter[int], 200), values: make([]*Value[float32], 4)}
			x := newDamageHarness(t, root(st), 360, 280, scale, cfg, damageSeed+uint64(len(name))*1000+uint64(scale*100)/100+uint64(scale*100)%100*7)
			n := steps
			if scale == 1.25 || scale == 1.75 {
				n = max(steps/4, 10) // the other fractional scales, shorter (1.5 runs in full)
			}
			for range n {
				var what string
				if x.rng.IntN(3) == 0 {
					what = ops(x, st)
				} else {
					what = x.randomOp()
				}
				x.frame(what)
				checkScrollSets(t, x.tree())
			}
			t.Logf("%d frames checked (%d skipped): %d full, %d shifted, %d partial", x.frames, x.skipped, x.full, x.shifted, x.partial)
		})
	}
}

// commonOps are scene operations on the handles: programmatic scrolls,
// state changes, Value changes, resizes.
func commonOps(x *damageHarness, st *sceneState) string {
	switch r := x.rng.IntN(100); {
	case r < 30:
		k := x.rng.IntN(len(st.refs))
		if st.refs[k].element() == nil {
			return "noop"
		}
		dx, dy := float32(x.rng.IntN(7)-3)*17, float32(x.rng.IntN(11)-5)*23
		st.refs[k].ScrollBy(dx, dy)
		return fmt.Sprintf("scrollBy%d(%g,%g)", k, dx, dy)
	case r < 55:
		k := x.rng.IntN(len(st.setters))
		if st.setters[k] == (Setter[int]{}) {
			return "noop"
		}
		st.setters[k].Update(func(v int) int { return v + 1 })
		return fmt.Sprintf("state%d", k)
	case r < 80:
		k := x.rng.IntN(len(st.values))
		if st.values[k] == nil {
			return "noop"
		}
		v := float32(x.rng.IntN(120)) - 20
		if k == 1 {
			v = float32(x.rng.IntN(5)) / 4
		}
		st.values[k].Set(v)
		return fmt.Sprintf("value%d=%g", k, v)
	case r < 85:
		m := x.w.Metrics()
		w := min(max(m.Size.W+float32(x.rng.IntN(61)-30), 200), 420)
		h := min(max(m.Size.H+float32(x.rng.IntN(61)-30), 160), 340)
		x.w.Resize(platform.Size{W: w, H: h})
		x.b.Drain(x.h)
		return fmt.Sprintf("resize(%g,%g)", w, h)
	}
	return "frame"
}

// damageSeed and TELA_DAMAGE_STEPS vary the property test: run it with
// TELA_DAMAGE_SEED=n TELA_DAMAGE_STEPS=2000 for long sequences.
var damageSeed, _ = strconv.ParseUint(os.Getenv("TELA_DAMAGE_SEED"), 10, 64)

// damageScales are the display scales the property test runs at: the
// integer scales and the common fractional ones (Wayland's
// wp_fractional_scale_v1, Windows' 125/150/175 %). TELA_DAMAGE_SCALES
// (comma-separated) overrides them.
func damageScales() []float64 {
	s := os.Getenv("TELA_DAMAGE_SCALES")
	if s == "" {
		return []float64{1, 1.25, 1.5, 1.75, 2}
	}
	var out []float64
	for _, f := range strings.Split(s, ",") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(f), 64); err == nil && v > 0 {
			out = append(out, v)
		}
	}
	return out
}

func TestDamageMatchesFullRender(t *testing.T) {
	steps := 160
	if testing.Short() {
		steps = 40
	}
	if n, err := strconv.Atoi(os.Getenv("TELA_DAMAGE_STEPS")); err == nil {
		steps = n
	}
	rows := func(st *sceneState) Node { return rowScene(st, 120, "grow overflow-y-auto bg-white") }
	runScene(t, "rows", rows, commonOps, Config{}, steps)
	runScene(t, "rows_instant_classic", rows, commonOps, Config{InstantScroll: true, ClassicScrollbars: true}, steps)
	rounded := func(st *sceneState) Node {
		return rowScene(st, 60, "m-3 grow overflow-auto rounded-2xl border-2 border-slate-300 bg-white shadow-lg")
	}
	runScene(t, "rounded", rounded, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "page", pageScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "page_smooth", pageScene, commonOps, Config{}, steps)
	runScene(t, "glass", glassScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "plainpage", plainPageScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "plainpage_classic", plainPageScene, commonOps, Config{ClassicScrollbars: true}, steps)
	runScene(t, "board", boardScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "virtual", virtualScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "virtual_smooth", virtualScene, commonOps, Config{}, steps)
}

// TestScrollShiftsPixels checks that a steady scroll of a list over an
// opaque background shifts the pixels and rasterizes a strip.
func TestScrollShiftsPixels(t *testing.T) {
	st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
	x := newDamageHarness(t, rowScene(st, 120, "grow overflow-y-auto bg-white"), 360, 280, 2, Config{InstantScroll: true}, 1)
	st.refs[0].ScrollBy(0, 40)
	x.frame("first scroll")
	for i := range 5 {
		st.refs[0].ScrollBy(0, 53)
		x.frame(fmt.Sprintf("scroll %d", i))
		d := x.tree().LastDamage()
		if d.Full || len(d.Shifts) != 1 {
			t.Fatalf("scroll %d: damage %+v, want a shift", i, d)
		}
		area := 0
		for _, r := range d.Rects {
			area += r.Dx() * r.Dy()
		}
		// The exposed strip (106 device rows of 720 px), the rows with
		// gradients, positioned badges and cards that crossed it, the
		// scrollbar and the anti-aliased edges.
		if full := 720 * 560; area > full/2 {
			t.Errorf("scroll %d: %d px rasterized of %d: %+v", i, area, full, d)
		}
	}
}
