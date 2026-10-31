package core

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// cardBoard is a scrolling page of n cards, each a positioned box (a
// hoisted participant of the root stacking context, painted in a chunk of
// its own) with a color transition on hover, a positioned badge and a
// text that st.setters[i] changes.
func cardBoard(n int, st *sceneState) Node { return cardBoardWith(n, st, "") }

// cardBoardWith is cardBoard with more classes on the cards.
func cardBoardWith(n int, st *sceneState, cls string) Node {
	card := func(i int) Node {
		v, set := UseState(0)
		if i < len(st.setters) {
			st.setters[i] = set
		}
		return div(Key(i), Class("relative h-24 rounded-xl border border-slate-200 bg-white p-3 transition-colors duration-150 hover:bg-sky-50 hover:border-sky-300 "+cls),
			div(Class("text-sm font-medium text-slate-800"), Textf("Card %d (%d)", i, v)),
			p(Class("mt-1 text-xs text-slate-500"), Text("A note that wraps over a couple of lines in the card.")),
			span(Class("absolute right-2 top-2 rounded-full bg-sky-100 px-1.5 text-[10px] text-sky-700"), Textf("#%d", i)))
	}
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}
	return div(Class("h-screen overflow-y-auto bg-slate-100"),
		div(Class("grid grid-cols-4 gap-3 p-3"),
			Map(items, func(i int) Node { return WithKey(ComponentProps(card, i), i) })))
}

// recordedLayers counts the layers the last paint recorded, and all.
func recordedLayers(tr *Tree) (rec, all int) {
	for _, l := range tr.Layers() {
		all++
		if l.Recorded {
			rec++
		}
	}
	return rec, all
}

// recordedLeads describes the layers the last paint recorded, but the
// scrollbars: kind and the key of the box whose painting opened it.
func recordedLeads(tr *Tree) []string {
	var out []string
	L := &tr.lay
	var walk func(l *layer)
	walk = func(l *layer) {
		if l.paint == L.paints && l.kind != layerBars {
			b := l.lead
			if b == nil {
				b = l.box
			}
			key := "-"
			if b != nil && b.el != nil && b.el.key != nil {
				key = fmt.Sprint(b.el.key)
			}
			out = append(out, layerKindNames[l.kind]+" "+key)
		}
		for _, k := range l.kids {
			walk(k)
		}
	}
	if L.root != nil {
		walk(L.root)
	}
	return out
}

// TestPartialRecording: on a page of 120 positioned cards (each a chunk of
// its own, with a positioned badge), a hover that restyles a card, the
// steps of its color transition and a build that changes a card's text
// record the chunk of that card (and the scrollbars), not the page's 100
// layers; every frame shows what a full rendering shows (the damage
// harness), and with TELA_CHECK_LAYERS=1 composes what a full recording
// composes.
func TestPartialRecording(t *testing.T) {
	const n = 120
	st := &sceneState{setters: make([]Setter[int], n)}
	x := newDamageHarness(t, cardBoard(n, st), 900, 700, 1, Config{}, 1)
	tr := x.tree()
	if rec, all := recordedLayers(tr); rec != all || all < 100 {
		t.Fatalf("mount: %d of %d layers recorded, want all of more than 100", rec, all)
	}
	want := func(what string, leads ...string) {
		t.Helper()
		if got := recordedLeads(tr); fmt.Sprint(got) != fmt.Sprint(leads) {
			t.Errorf("%s recorded %v, want %v", what, got, leads)
		}
	}
	// The card at (100, 60) is card 0. Its colors transition from where
	// they are: the hover's frame paints what the last one did and
	// records nothing (sameStyle, recordstyle.go); its steps record it.
	x.move(100, 60)
	x.frame("hover")
	want("the hover")
	for i := range 12 {
		s0 := tr.Stats()
		x.frame(fmt.Sprintf("step %d", i))
		if d := work(s0, tr.Stats()); d.Paints == 0 {
			break // the transition ended
		} else if d.Builds+d.Styles+d.Layouts != 0 {
			t.Fatalf("step %d built, styled or laid out: %+v", i, d)
		}
		want(fmt.Sprintf("step %d", i), "scroll 0")
	}
	// Card 7's text changes: it builds, its title is laid out again.
	st.setters[7].Set(1)
	x.frame("text")
	want("the text", "scroll 7")
	// Leaving the card fades it back, and the card under the pointer
	// (card 24) in: both start where they are, and their first steps
	// record them.
	x.move(100, 690)
	x.frame("leave")
	want("the hover's end")
	x.frame("leave step")
	want("the first step after the hover", "scroll 0", "scroll 24")
}

// TestPartialRecordingClips: the overflow clip of a positioned card that
// clips (its radius transitions on hover) is pushed in the chunks of its
// positioned badges, and a card moved by a TranslateFn (a deferred layer)
// fades its colors: every frame shows what a full rendering shows. With
// TELA_CHECK_LAYERS=1 every partial recording is checked command for
// command.
func TestPartialRecordingClips(t *testing.T) {
	st := &sceneState{values: make([]*Value[float32], 1)}
	scene := func() Node {
		st.values[0] = UseValue[float32](0)
		x := st.values[0]
		cards := make([]Child, 0, 13)
		cards = append(cards, Class("grid grid-cols-3 gap-4 p-4"))
		for i := range 12 {
			cards = append(cards, div(Key(i), Class("relative h-32 overflow-hidden rounded-md border border-slate-300 bg-white p-3 transition-all duration-200 hover:rounded-3xl hover:bg-amber-100"),
				Textf("Card %d", i),
				span(Class("absolute -right-2 -bottom-2 size-10 rounded-full bg-sky-400"))))
		}
		return div(Class("h-screen overflow-y-auto bg-slate-100"),
			div(cards...),
			div(Key("drag"), Class("relative m-4 h-16 w-40 rounded-lg bg-white transition-colors duration-200 hover:bg-rose-200"),
				Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 })), Text("Dragged")))
	}
	x := newDamageHarness(t, Component(scene), 600, 420, 1.5, Config{}, 3)
	for _, pt := range [][2]float32{{60, 60}, {250, 70}, {250, 200}, {40, 400}} {
		x.move(pt[0], pt[1])
		x.frame("hover")
		for i := range 15 {
			if i%3 == 0 {
				st.values[0].Set(float32(5 * i))
			}
			x.frame(fmt.Sprintf("step %d", i))
		}
	}
	x.inject(platform.ScrollEvent{Pos: platform.Point{X: 200, Y: 200}, Delta: platform.Point{Y: 140}, Phase: platform.ScrollPhaseUpdate})
	x.frame("scroll")
	x.move(100, 100)
	for i := range 15 {
		x.frame(fmt.Sprintf("after scroll %d", i))
	}
}

// cardRig is a tree with cardBoard mounted, driven frame by frame
// without a window: Frame (the pipeline up to the composed frame) and the
// rasterization of its damage are timed apart.
type cardRig struct {
	tr       *Tree
	st       *sceneState
	fb       *image.RGBA
	r        raster.Renderer
	now      time.Duration
	pipe, rs time.Duration
	frames   int
	rec      int
}

func newCardRig(tb testing.TB, n int, scale float32, cls string) *cardRig {
	st := &sceneState{setters: make([]Setter[int], n)}
	dev := false
	tr := NewTree(Config{Fonts: testFonts, Dev: ResolveDev(&dev)})
	tr.SetViewport(gfx.Size{W: 900, H: 700})
	tr.SetScale(scale)
	tr.Mount(cardBoardWith(n, st, cls))
	g := &cardRig{tr: tr, st: st, fb: image.NewRGBA(image.Rect(0, 0, int(900*scale), int(700*scale)))}
	g.frame()
	g.reset()
	return g
}

func (g *cardRig) reset() { g.pipe, g.rs, g.frames, g.rec = 0, 0, 0, 0 }

func (g *cardRig) frame() {
	g.now += 16 * time.Millisecond
	t0 := time.Now()
	g.tr.Animate(g.now)
	g.tr.Frame()
	t1 := time.Now()
	g.tr.RasterizeFrame(g.fb, &g.r, false)
	g.tr.FlushEffects()
	t2 := time.Now()
	g.pipe += t1.Sub(t0)
	g.rs += t2.Sub(t1)
	g.frames++
	r, _ := recordedLayers(g.tr)
	g.rec += r
}

func (g *cardRig) report(b *testing.B) {
	n := float64(max(g.frames, 1))
	b.ReportMetric(float64(g.pipe.Microseconds())/1000/n, "pipe-ms/frame")
	b.ReportMetric(float64(g.rs.Microseconds())/1000/n, "raster-ms/frame")
	b.ReportMetric(float64(g.rec)/n, "recorded/frame")
	_, all := recordedLayers(g.tr)
	b.ReportMetric(float64(all), "layers")
}

func (g *cardRig) hover(x, y float32) {
	g.tr.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: x, Y: y}, Time: g.now})
}

// BenchmarkCardTransitionStep is a frame of a hover color transition on
// one card of cardBoard (a transition that never ends): no build, style
// or layout.
func BenchmarkCardTransitionStep(b *testing.B) {
	g := newCardRig(b, 120, 2, "duration-[1000000ms]")
	g.hover(100, 60)
	g.frame()
	g.reset()
	for b.Loop() {
		g.frame()
	}
	g.report(b)
}

// BenchmarkCardTextBuild is a frame that builds one card with a new text.
func BenchmarkCardTextBuild(b *testing.B) {
	g := newCardRig(b, 120, 2, "")
	i := 0
	for b.Loop() {
		i++
		g.st.setters[5].Set(i)
		g.frame()
	}
	g.report(b)
}

// BenchmarkCardHover is a hover moving between two cards (a restyle of
// both, the start of their transitions) and its frame.
func BenchmarkCardHover(b *testing.B) {
	g := newCardRig(b, 120, 2, "")
	i := 0
	for b.Loop() {
		i++
		g.hover(float32(100+220*(i%2)), 60)
		g.frame()
	}
	g.report(b)
}

// deferredScrollPage is the window's scroll container with rows tall enough
// to move its interest rect, and near its top a deferred layer: a card
// moved by a TranslateFn, with inner lines of text of its own.
func deferredScrollPage(rows, inner int) Node {
	card := make([]Child, 0, inner+3)
	card = append(card, Key("card"), Class("m-4 rounded-xl bg-white p-3 shadow-lg"),
		Style(style.TranslateFn(func() (float32, float32) { return 4, 0 })))
	for i := range inner {
		card = append(card, div(Class("h-6 text-sm text-slate-700"), Textf("Card line %d", i)))
	}
	page := make([]Child, 0, rows+3)
	page = append(page, Key("page"), Class("h-screen overflow-y-auto bg-slate-100"), div(card...))
	for i := range rows {
		page = append(page, div(Key(i), Class("h-10 border-b border-slate-200 px-3 text-sm"), Textf("Row %d", i)))
	}
	return div(page...)
}

// recordedOf counts the layers of kind the last paint recorded.
func recordedOf(tr *Tree, kind layerKind) int {
	n := 0
	for _, l := range tr.lay.layers {
		if l != nil && l.kind == kind && l.paint == tr.lay.paints {
			n++
		}
	}
	return n
}

// TestDeferredLayerKeptAcrossInterest: scrolling far enough to move the
// interest rect of a scroll space records its chunks again, not the
// deferred layers in it: their lists cull nothing, so they do not depend
// on the interest rect. Every frame shows what a full rendering shows,
// and with TELA_CHECK_LAYERS=1 composes what a full recording composes.
func TestDeferredLayerKeptAcrossInterest(t *testing.T) {
	const h = 300
	x := newDamageHarness(t, deferredScrollPage(300, 40), 400, h, 1, Config{InstantScroll: true}, 1)
	tr := x.tree()
	s := keyed(tr, "page").box.scr
	if s == nil || !s.chunked || countLayers(tr, "deferred") != 1 {
		t.Fatalf("want a chunked scroll space with one deferred layer: scroller %v, %d deferred layers", s != nil, countLayers(tr, "deferred"))
	}
	moved, chunks, deferred := 0, 0, 0
	for i := range 12 {
		in0 := s.interest
		tr.setScroll(s, s.pos.Add(gfx.Point{Y: 2 * h}))
		x.frame(fmt.Sprintf("scroll %d", i))
		if s.interest == in0 {
			continue
		}
		moved++
		chunks += recordedOf(tr, layerChunk)
		deferred += recordedOf(tr, layerDeferred)
	}
	t.Logf("%d frames moved the interest rect: %d chunk and %d deferred-layer recordings", moved, chunks, deferred)
	if moved < 10 || chunks < moved {
		t.Fatalf("%d frames moved the interest rect, recording %d chunks: want at least 10, each recording the content", moved, chunks)
	}
	if deferred != 0 {
		t.Errorf("the deferred layer was recorded %d times over %d frames that moved the interest rect, want 0", deferred, moved)
	}
}

// BenchmarkScrollInterestDeferred is a scroll frame that moves the
// interest rect of the window's scroll space (two scrollports at a time)
// with a deferred layer of 200 lines in it: the chunks of the space are
// recorded again, the deferred layer is kept.
func BenchmarkScrollInterestDeferred(b *testing.B) {
	const h = 600
	tr := newTestTree(b, 800, h, deferredScrollPage(400, 200), nil)
	tr.SetScale(2)
	tr.Frame()
	s := keyed(tr, "page").box.scr
	if s == nil || s.max.Y < 8*h {
		b.Fatalf("no scroll range: %+v", s)
	}
	dir := float32(2 * h)
	frames, deferred, rec := 0, 0, 0
	b.ReportAllocs()
	for b.Loop() {
		if s.pos.Y+dir > s.max.Y || s.pos.Y+dir < 0 {
			dir = -dir
		}
		tr.setScroll(s, s.pos.Add(gfx.Point{Y: dir}))
		if _, changed := tr.Frame(); !changed {
			b.Fatal("the frame did not repaint")
		}
		frames++
		deferred += recordedOf(tr, layerDeferred)
		rec += tr.lay.recorded
	}
	b.ReportMetric(float64(rec)/float64(frames), "recorded/frame")
	b.ReportMetric(float64(deferred)/float64(frames), "deferred/frame")
}
