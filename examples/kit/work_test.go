package main

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

// galleryRig is the gallery in a Tree without a window at 1280x800, with a
// manual clock (record_test.go's frame), for measuring a page's mount.
type galleryRig struct {
	tb  testing.TB
	tr  *core.Tree
	now time.Duration
}

func newGalleryRig(tb testing.TB, page string) *galleryRig {
	tb.Helper()
	r := &galleryRig{tb: tb, tr: core.NewTree(core.Config{Theme: (*core.Theme)(App.Theme), RootClass: App.RootClass, Dev: core.ResolveDev(nil)})}
	r.tr.SetViewport(gfx.Size{W: 1280, H: 800})
	r.tr.SetScale(1)
	stop := r.tr.ReceivePosts()
	tb.Cleanup(func() { stop(); r.tr.Unmount() })
	r.tr.Mount(core.NodeOf(Gallery{Page: page}))
	r.settle()
	return r
}

// frame runs one frame and returns its pipeline time.
func (r *galleryRig) frame() time.Duration {
	r.now += 16 * time.Millisecond
	r.tr.Animate(r.now)
	t0 := time.Now()
	r.tr.Frame()
	d := time.Since(t0)
	r.tr.FlushEffects()
	return d
}

// settle runs frames until the tree is at rest (bounded).
func (r *galleryRig) settle() {
	for i := 0; i < 300 && (i == 0 || r.tr.Dirty() || r.tr.Animating()); i++ {
		r.frame()
	}
}

// open clicks the tab of page name in the sidebar and returns the
// pipeline time and the work of the frame that mounts the page.
func (r *galleryRig) open(name string) (time.Duration, core.Stats) {
	var at platform.Point
	found := false
	r.tr.Semantics().Walk(func(n *semantics.Node) bool {
		if !found && n.Role == "tab" && n.Name == name {
			c := n.Bounds.Center()
			at, found = platform.Point{X: c.X, Y: c.Y}, true
		}
		return !found
	})
	if !found {
		r.tb.Fatalf("no tab %q", name)
	}
	r.tr.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: at, Time: r.now})
	r.frame() // the hover
	before := r.tr.Stats()
	r.tr.HandlePointer(platform.PointerEvent{Action: platform.PointerDown, Pos: at, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask(), Time: r.now})
	r.tr.HandlePointer(platform.PointerEvent{Action: platform.PointerUp, Pos: at, Button: platform.ButtonPrimary, Time: r.now})
	d := r.frame()
	after := r.tr.Stats()
	r.settle()
	return d, core.Stats{Mounts: after.Mounts - before.Mounts, Styles: after.Styles - before.Styles,
		LaidOut: after.LaidOut - before.LaidOut, Placed: after.Placed - before.Placed, Shapes: after.Shapes - before.Shapes}
}

// BenchmarkCalendarMount opens the Calendar page from the Button page (and
// goes back, unmeasured): the median pipeline time of the frame that
// mounts it, and its work.
func BenchmarkCalendarMount(b *testing.B) {
	r := newGalleryRig(b, "Button")
	var pipe []time.Duration
	var work core.Stats
	for b.Loop() {
		d, w := r.open("Calendar")
		pipe, work = append(pipe, d), w
		r.open("Button")
	}
	slices.Sort(pipe)
	b.ReportMetric(float64(pipe[len(pipe)/2].Microseconds())/1000, "pipeline-ms")
	b.ReportMetric(float64(work.Mounts), "mounts")
	b.ReportMetric(float64(work.Styles), "styles")
	b.ReportMetric(float64(work.LaidOut), "laidout")
	b.ReportMetric(float64(work.Placed), "placed")
	b.ReportMetric(float64(work.Shapes), "shapes")
}
