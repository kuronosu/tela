package main

import (
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/telatest"
)

// ticker is the player at 1416x846 and scale 2 (the window of the live
// measurement, 2832x1692 device pixels) in a tree without a window, its
// clock stopped: tick advances the progress by 0.1 s, runs the frame (a
// layout of the bar and, once a second, a build of the time label) and
// brings the framebuffer to it (full: a full rasterization, the behavior
// before damage tracking).
type ticker struct {
	t  *core.Tree
	tr *Transport
	fb *image.RGBA
	r  raster.Renderer
}

func newTicker(tb testing.TB) *ticker {
	old := tickEvery
	tickEvery = time.Hour // no clock: the test ticks
	tb.Cleanup(func() { tickEvery = old })
	k := &ticker{t: core.NewTree(core.Config{}), fb: image.NewRGBA(image.Rect(0, 0, 2832, 1692))}
	k.t.SetViewport(gfx.Size{W: 1416, H: 846})
	k.t.SetScale(2)
	k.t.Mount(core.Component(func() tela.Node {
		k.tr = useTransport(84)
		return page(k.tr)
	}))
	tb.Cleanup(k.t.Unmount)
	k.t.Frame()
	k.t.RasterizeFrame(k.fb, &k.r, true)
	k.t.FlushEffects()
	return k
}

// tick returns the device pixels the tick rasterized.
func (k *ticker) tick(full bool) int {
	k.tr.advance(0.1)
	k.t.Frame()
	d := k.t.RasterizeFrame(k.fb, &k.r, full)
	k.t.FlushEffects()
	pixels := 0
	if d.Full {
		pixels += k.fb.Rect.Dx() * k.fb.Rect.Dy()
	}
	for _, rc := range d.Rects {
		pixels += rc.Dx() * rc.Dy()
	}
	return pixels
}

func benchTick(b *testing.B, full bool) {
	k := newTicker(b)
	pixels := 0
	b.ReportAllocs()
	for b.Loop() {
		pixels += k.tick(full)
	}
	b.ReportMetric(float64(pixels)/float64(b.N), "rasterpx/op")
}

func BenchmarkTick(b *testing.B)     { benchTick(b, false) }
func BenchmarkTickFull(b *testing.B) { benchTick(b, true) }

// TestTickRasterizesTheBar checks that a tick rasterizes the progress bar
// and the time label, not the window, and that the frames are those of a
// full rendering.
func TestTickRasterizesTheBar(t *testing.T) {
	k := newTicker(t)
	total := 0
	for i := range 25 {
		// A tick moves the bar by a fraction of a pixel: some change
		// nothing on screen.
		px := k.tick(false)
		if px > 2832*1692/16 {
			t.Errorf("tick %d rasterized %d device pixels", i, px)
		}
		total += px
	}
	if total == 0 {
		t.Error("the ticks rasterized nothing")
	}
	want := k.t.Rasterize(k.t.ReferenceFrame(false), 2)
	if string(want.Pix) != string(k.fb.Pix) {
		t.Error("the framebuffer differs from a full rendering")
	}
}

// TestTickDamage plays with the clock running fast and hovers the controls
// at scale 2, checking every frame against a full rendering.
func TestTickDamage(t *testing.T) {
	fast(t, 5*time.Millisecond, 10)
	a := mount(t, windowW, windowH, Player, telatest.Scale(2), telatest.CheckDamage())
	for range 40 {
		time.Sleep(3 * time.Millisecond)
		a.Advance(16 * time.Millisecond)
	}
	a.Hover(telatest.ByRole("button", "Pause"))
	a.Hover(seekBar)
	a.MouseMove(5, 5)
}
