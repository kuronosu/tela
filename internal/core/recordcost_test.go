package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// What a small change records (record.go, ADR-274): a page that fits the
// window, so that everything is in the root list (no scroll content), with
// a sidebar of hover buttons and a grid of cards, each card a component
// that the scenarios below change one way: a hover, a hoisted badge that
// mounts and unmounts, a TextColorFn that mounts and moves, a TranslateFn
// that makes a card a layer and back, a layout animation that scales a
// card with corrected radii and shadows, and a card built again with equal
// BgFn, BackgroundGradientFn and TranslateFn props. Each records the lists
// around what changed, not the page.

// costPage is the state the scenarios drive.
type costPage struct {
	badge, fn, tall, rebuild Setter[bool]
	color                    *Value[gfx.Color]
	shift                    *Value[float32]
}

const costCards = 24

func (pg *costPage) node() Node {
	nav := []Child{Class("flex w-48 shrink-0 flex-col gap-1 border-r border-slate-200 bg-white p-3")}
	for i := range 10 {
		nav = append(nav, button(Key(fmt.Sprint("nav", i)), Class("flex h-9 items-center gap-2 rounded-lg px-2 text-sm text-slate-600 transition-colors duration-100 hover:bg-slate-100 hover:text-slate-900"),
			span(Class("size-4 rounded bg-slate-300")), span(Textf("Item %d", i))))
	}
	cards := []Child{Class("grid flex-1 grid-cols-6 content-start gap-3 p-3")}
	for i := range costCards {
		cards = append(cards, WithKey(ComponentProps(pg.card, i), i))
	}
	return div(Class("flex h-screen bg-slate-50"), div(nav...), div(cards...))
}

func (pg *costPage) card(i int) Node {
	cls := "rounded-xl border border-slate-200 bg-white p-2"
	kids := []Child{Key(fmt.Sprint("card", i)), Class(cls),
		div(Class("text-sm font-medium text-slate-800"), Textf("Card %d", i)),
		p(Class("mt-1 text-xs text-slate-500"), Text("Two short lines of text in the card.")),
		div(Class("mt-2 flex gap-1"), span(Class("h-2 w-6 rounded bg-sky-200")), span(Class("h-2 w-4 rounded bg-sky-300")))}
	switch i {
	case 2: // a badge that mounts: a hoisted box of the root's stacking context
		on, set := UseState(false)
		pg.badge = set
		kids[1] = Class(cls + " relative")
		if on {
			kids = append(kids, span(Key("badge"), Class("absolute right-1 top-1 size-2 rounded-full bg-rose-500")))
		}
	case 5: // a TextColorFn that mounts on the card
		on, set := UseState(false)
		pg.fn, pg.color = set, UseValue(gfx.RGB8(220, 38, 38))
		if on {
			kids = append(kids, Style(style.TextColorFn(pg.color.Get)))
		}
	case 8: // a TranslateFn: a layer while it is not zero
		pg.shift = UseValue[float32](0)
		kids = append(kids, Style(style.TranslateFn(func() (float32, float32) { return pg.shift.Get(), 0 })))
	case 11: // a layout animation with corrected radii and shadows
		tall, set := UseState(false)
		pg.tall = set
		h := "h-16"
		if tall {
			h = "h-24"
		}
		return div(Key(fmt.Sprint("card", i)), Class("h-28"),
			div(Key("anim"), Class("rounded-xl border border-slate-200 bg-white p-2 shadow-md duration-200 ease-linear "+h), AnimateLayout(),
				Textf("Card %d", i)))
	case 14: // built again with equal paint-phase props
		_, set := UseState(false)
		pg.rebuild = set
		kids = append(kids, Style(
			style.BgFn(func() gfx.Color { return gfx.RGB8(240, 249, 255) }),
			style.BackgroundGradientFn(func() style.Gradient { return angleGradient(90) }),
			style.TranslateFn(func() (float32, float32) { return 2, 0 })))
	}
	return div(kids...)
}

// costRig drives costPage frame by frame, without a window.
type costRig struct {
	tb  testing.TB
	tr  *Tree
	pg  *costPage
	now time.Duration
	at  gfx.Point
	// The frames of the last run: commands recorded (but the scrollbars)
	// and pipeline times (Animate and Frame).
	cmds  []int
	times []time.Duration
}

func newCostRig(tb testing.TB) *costRig {
	tb.Helper()
	dev := false
	g := &costRig{tb: tb, pg: &costPage{}}
	g.tr = NewTree(Config{Fonts: testFonts, Dev: ResolveDev(&dev)})
	g.tr.SetViewport(gfx.Size{W: 1200, H: 640})
	g.tr.Mount(Component(g.pg.node))
	tb.Cleanup(g.tr.Unmount)
	g.run()
	return g
}

// recordedCmds sums the commands of the lists the last paint recorded,
// but the scrollbars', and counts them.
func recordedCmds(tr *Tree) (cmds, lists int) {
	L := &tr.lay
	for _, l := range L.layers {
		if l != nil && l.paint == L.paints && l.kind != layerBars && l.dl != nil {
			cmds += l.dl.Len()
			lists++
		}
	}
	return cmds, lists
}

// run plays frames until the tree asks for none (at most 60), noting
// each one's recorded commands and pipeline time.
func (g *costRig) run() {
	g.cmds, g.times = g.cmds[:0], g.times[:0]
	for range 60 {
		if !g.tr.Dirty() && !g.tr.Animating() {
			return
		}
		g.now += 16 * time.Millisecond
		p0 := g.tr.Stats().Paints
		t0 := time.Now()
		g.tr.Animate(g.now)
		g.tr.Frame()
		d := time.Since(t0)
		g.tr.FlushEffects()
		if g.tr.Stats().Paints == p0 {
			continue
		}
		c, _ := recordedCmds(g.tr)
		g.cmds, g.times = append(g.cmds, c), append(g.times, d)
	}
	g.tb.Fatal("the page does not come to rest")
}

func (g *costRig) hover(key string) {
	c := mustFind(g.tb, g.tr, key).Rect.Center()
	g.at = c
	g.tr.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: c.X, Y: c.Y}, Time: g.now})
}

// costScenarios are the changes, each played on a page at rest and
// undone after (the undo is a scenario too).
func costScenarios() []struct {
	name string
	act  func(g *costRig)
	max  int // commands a frame may record
} {
	return []struct {
		name string
		act  func(g *costRig)
		max  int
	}{
		{"hover", func(g *costRig) { g.hover("nav3") }, 10},
		{"hover away", func(g *costRig) { g.hover("card20") }, 10},
		{"hoisted mount", func(g *costRig) { g.pg.badge.Set(true) }, 40},
		{"hoisted unmount", func(g *costRig) { g.pg.badge.Set(false) }, 40},
		{"TextColorFn mount", func(g *costRig) { g.pg.fn.Set(true) }, 20},
		{"TextColorFn value", func(g *costRig) { g.pg.color.Set(gfx.RGB8(22, 163, 74)) }, 20},
		{"hover with a TextColorFn", func(g *costRig) { g.hover("nav5") }, 10},
		{"TextColorFn unmount", func(g *costRig) { g.pg.fn.Set(false) }, 20},
		{"becomes a layer", func(g *costRig) { g.pg.shift.Set(6) }, 60},
		{"stops being a layer", func(g *costRig) { g.pg.shift.Set(0) }, 60},
		{"layout animation", func(g *costRig) { g.pg.tall.Set(true) }, 60},
		{"equal Fn props built again", func(g *costRig) { g.pg.rebuild.Set(true) }, 0},
	}
}

// TestRecordingFollowsTheChange: every frame of each scenario records at
// most a few dozen commands, while the page's root list holds hundreds
// (with TELA_CHECK_LAYERS, on in this package's tests, each partial
// recording composes what a full one does).
func TestRecordingFollowsTheChange(t *testing.T) {
	g := newCostRig(t)
	if c, _ := recordedCmds(g.tr); c < 150 {
		t.Fatalf("the mount recorded %d commands: want a page of several hundred", c)
	}
	for _, sc := range costScenarios() {
		sc.act(g)
		g.run()
		if len(g.cmds) == 0 && sc.max > 0 {
			t.Errorf("%s: no frame painted", sc.name)
		}
		t.Logf("%s: %d frames, commands %v", sc.name, len(g.cmds), g.cmds)
		for i, c := range g.cmds {
			if c > sc.max {
				t.Errorf("%s: frame %d recorded %d commands, want at most %d", sc.name, i, c, sc.max)
			}
		}
	}
}

// TestRecordingFollowsTheChangeDamage plays the changes of
// TestRecordingFollowsTheChange in a window at scale 1.5, twice, with the
// hovers moving between them: every frame shows what a full rendering
// shows (groups moving between lists, adopted or not), and every partial
// recording composes what a full one does.
func TestRecordingFollowsTheChangeDamage(t *testing.T) {
	pg := &costPage{}
	x := newDamageHarness(t, Component(pg.node), 1200, 640, 1.5, Config{}, 7)
	at := func(key string) {
		c := mustFind(t, x.tree(), key).Rect.Center()
		x.move(c.X, c.Y)
	}
	steps := []func(){
		func() { at("nav3") },
		func() { pg.badge.Set(true) },
		func() { pg.fn.Set(true) },
		func() { at("card5") },
		func() { pg.color.Set(gfx.RGB8(22, 163, 74)) },
		func() { pg.shift.Set(6) },
		func() { pg.tall.Set(true) },
		func() { pg.rebuild.Set(true) },
		func() { at("card2") },
		func() { pg.badge.Set(false) },
		func() { pg.shift.Set(0) },
		func() { pg.fn.Set(false) },
		func() { pg.tall.Set(false) },
		func() { pg.rebuild.Set(false) },
		func() { at("nav7") },
	}
	for round := range 2 {
		for i, step := range steps {
			step()
			for k := range 4 {
				x.frame(fmt.Sprintf("round %d step %d frame %d", round, i, k))
			}
		}
	}
}

// BenchmarkRecordingFollowsTheChange plays each change and its undo
// (benchmarks run without the check of partial recordings): the commands
// recorded per frame and the median pipeline time (Animate and Frame) of
// the frames that paint.
func BenchmarkRecordingFollowsTheChange(b *testing.B) {
	pairs := []struct {
		name     string
		setup    func(g *costRig)
		do, undo func(g *costRig)
	}{
		{"hover", nil, func(g *costRig) { g.hover("nav3") }, func(g *costRig) { g.hover("card20") }},
		{"hoisted", nil, func(g *costRig) { g.pg.badge.Set(true) }, func(g *costRig) { g.pg.badge.Set(false) }},
		{"TextColorFn mount", nil, func(g *costRig) { g.pg.fn.Set(true) }, func(g *costRig) { g.pg.fn.Set(false) }},
		{"TextColorFn value", func(g *costRig) { g.pg.fn.Set(true) },
			func(g *costRig) { g.pg.color.Set(gfx.RGB8(22, 163, 74)) }, func(g *costRig) { g.pg.color.Set(gfx.RGB8(220, 38, 38)) }},
		{"layer", nil, func(g *costRig) { g.pg.shift.Set(6) }, func(g *costRig) { g.pg.shift.Set(0) }},
		{"layout animation", nil, func(g *costRig) { g.pg.tall.Set(true) }, func(g *costRig) { g.pg.tall.Set(false) }},
		{"equal Fn props", nil, func(g *costRig) { g.pg.rebuild.Set(true) }, func(g *costRig) { g.pg.rebuild.Set(false) }},
	}
	for _, pr := range pairs {
		b.Run(pr.name, func(b *testing.B) {
			g := newCostRig(b)
			if pr.setup != nil {
				pr.setup(g)
				g.run()
			}
			var cmds []int
			var times []time.Duration
			for b.Loop() {
				for _, f := range []func(*costRig){pr.do, pr.undo} {
					f(g)
					g.run()
					cmds = append(cmds, g.cmds...)
					times = append(times, g.times...)
				}
			}
			slices.Sort(times)
			total := 0
			for _, c := range cmds {
				total += c
			}
			b.ReportMetric(float64(total)/float64(max(len(cmds), 1)), "cmds/frame")
			if len(times) > 0 {
				b.ReportMetric(float64(times[len(times)/2].Nanoseconds())/1000, "pipe-µs/frame")
			}
		})
	}
}
