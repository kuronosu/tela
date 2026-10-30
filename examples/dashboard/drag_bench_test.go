package main

import (
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/telatest"
)

// The drag of a card measured from below telatest (from examples/board):
// the board at 1416x846, scale 2, in a headless window, with a drag of a
// card in progress (core directly, to read the damage of each frame).
type dragger struct {
	b    *headless.Backend
	host *core.Host
	win  *headless.Window
	at   platform.Point
}

func newDragger(tb testing.TB) *dragger {
	d := &dragger{b: headless.New(headless.Options{Size: platform.Size{W: 1416, H: 846}, Scale: 2})}
	host, err := core.NewHost(d.b, core.Options{
		Config: core.Config{Theme: (*core.Theme)(App.Theme), RootClass: App.RootClass},
		Root:   core.NodeOf(Previews["2 Board"].Root),
	})
	if err != nil {
		tb.Fatal(err)
	}
	d.host, d.win = host, host.Window().(*headless.Window)
	tb.Cleanup(host.Tree().Unmount)
	d.b.Drain(host)
	d.at = textCenter(host.Tree().Inspect(), "Fix failed invoice retries")
	if d.at == (platform.Point{}) {
		tb.Fatal("no card titled Fix failed invoice retries")
	}
	d.send(platform.PointerEvent{Action: platform.PointerMove, Pos: d.at})
	d.send(platform.PointerEvent{Action: platform.PointerDown, Pos: d.at, Button: platform.ButtonPrimary, Buttons: primary})
	for range 4 {
		d.move(5, 0)
	}
	if n := countLayers(host.Tree(), "deferred"); n != 1 {
		tb.Fatalf("%d deferred layers: the card is not being dragged", n)
	}
	return d
}

// textCenter is the center of the node whose text is title.
func textCenter(n core.InspectNode, title string) platform.Point {
	if n.Text == title {
		c := n.Rect.Center()
		return platform.Point{X: c.X, Y: c.Y}
	}
	for _, k := range n.Children {
		if p := textCenter(k, title); p != (platform.Point{}) {
			return p
		}
	}
	return platform.Point{}
}

// send delivers an event and the frame after it.
func (d *dragger) send(e platform.Event) {
	d.win.Inject(e)
	d.b.Drain(d.host)
	d.b.Advance(d.b.FrameInterval())
	d.b.Drain(d.host)
}

func (d *dragger) move(dx, dy float32) {
	d.at.X += dx
	d.at.Y += dy
	d.send(platform.PointerEvent{Action: platform.PointerMove, Pos: d.at, Buttons: primary})
}

// BenchmarkDragMove is one pointer move of a card being dragged, at
// 1416x846 and scale 2: the card is a layer moved by its TranslateFn, so
// the frame records nothing but the scrollbars, composes, and rasterizes
// the card's old and new places (under the main pane's glass).
func BenchmarkDragMove(b *testing.B) { benchDrag(b, false) }

// BenchmarkDragMoveFull is BenchmarkDragMove with a full rasterization of
// every frame.
func BenchmarkDragMoveFull(b *testing.B) { benchDrag(b, true) }

func benchDrag(b *testing.B, full bool) {
	d := newDragger(b)
	d.host.FullFrames = full
	s0 := d.host.Tree().Stats()
	pixels, n := 0, 0
	dir := float32(2)
	b.ReportAllocs()
	for b.Loop() {
		d.move(dir, 0)
		dir = -dir
		dm := d.host.Tree().LastDamage()
		if dm.Full {
			pixels += 2832 * 1692
		}
		for _, r := range dm.Rects {
			pixels += r.Dx() * r.Dy()
		}
		n++
	}
	s1 := d.host.Tree().Stats()
	b.ReportMetric(float64(pixels)/float64(n), "rasterpx/op")
	b.ReportMetric(float64(s1.Builds-s0.Builds)/float64(n), "builds/op")
}

// TestDragMoveRecordsNothing drags a card across the board: a move that
// does not change the drop target (no build) rasterizes little and records
// no layer but the scrollbars. The card is over the main pane's glass, and
// the damage of a change under a backdrop-blur-2xl grows by the blur's
// reach (about 120 px on each side), so "little" is a third of the window
// here, where the board's own test (on an opaque page) allowed a quarter.
// A transition on the cards or the columns (a hover that fades, the drop
// ring) would record every layer on each of its frames: they have none.
func TestDragMoveRecordsNothing(t *testing.T) {
	d := newDragger(t)
	tr := d.host.Tree()
	for i := range 40 {
		s0 := tr.Stats()
		d.move(7, float32(i%3)-1)
		if tr.Stats().Builds != s0.Builds {
			continue // a new drop target: the columns change
		}
		dm := tr.LastDamage()
		area := 0
		for _, r := range dm.Rects {
			area += r.Dx() * r.Dy()
		}
		if dm.Full || area > 2*1416*2*846/3 {
			t.Errorf("move %d rasterized %+v", i, dm)
		}
		for _, l := range tr.Layers() {
			if l.Recorded && l.Kind != "scrollbars" {
				t.Errorf("move %d recorded a %s layer", i, l.Kind)
			}
		}
	}
}

// TestDragDamage drags cards at scale 2, checking every frame against a
// full rendering.
func TestDragDamage(t *testing.T) {
	app := App
	app.Root = Previews["2 Board"].Root
	a := telatest.MountApp(t, app, telatest.Size(1416, 846), telatest.Scale(2), telatest.CheckDamage())
	a.DragBy(cardLoc("Fix failed invoice retries"), 420, 60, telatest.Steps(30))
	a.DragBy(cardLoc("Set up the staging pipeline"), -380, 10, telatest.Steps(30))
}

func countLayers(tr *core.Tree, kind string) int {
	n := 0
	for _, l := range tr.Layers() {
		if l.Kind == kind {
			n++
		}
	}
	return n
}
