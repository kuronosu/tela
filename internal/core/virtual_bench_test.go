package core

import (
	"image"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// The phase-4 scrolling benchmarks (ROADMAP: a 100,000-row VirtualList and
// a 10,000-row list without virtualization scroll smoothly, p95 frame time
// at most 16 ms at 800x600, scale 2). Rows are 40 px: an avatar with
// initials, a name, a secondary line and a number, nine Elements and four
// paragraphs each. The virtual list estimates them exactly, except in the
// Estimated variant (48 px), where every new row is measured again.
//
//	CGO_ENABLED=0 go test -run '^$' -bench 'Mount|ScrollFrame|ScrollP95' -benchtime 1200x ./internal/core/
//
// The p95 benchmarks run one frame per iteration of a scripted scroll
// (wheel notches every fourth frame, Page Down every second), so
// -benchtime Nx sets the number of frames; they report the median, p95
// and worst frame of the pipeline alone (Frame), with the window's
// framebuffer brought to the frame as a Host does (RasterizeFrame: shifted
// pixels and damaged regions, layer.go), and with a full rasterization of
// the window (the behavior before damage tracking).

const (
	benchW, benchH = 800, 600
	benchScale     = 2
	frameStep      = time.Second / 60
)

// personRow is a 40 px row (h-10, its 1 px border included).
func personRow(i int) Node {
	return div(Class("flex h-10 items-center gap-3 border-b border-zinc-100 px-4"),
		div(Class("flex size-7 shrink-0 items-center justify-center rounded-full bg-indigo-500 text-xs font-semibold text-white"),
			Text(string(rune('A'+i%26))+string(rune('A'+i/26%26)))),
		span(Class("truncate text-sm font-medium"), Textf("Person %d", i)),
		span(Class("truncate text-sm text-zinc-500"), Text("Engineer · Lisbon")),
		span(Class("ml-auto text-xs text-zinc-400 tabular-nums"), Textf("#%d", i+1)),
	)
}

func personNode(i int) Node { return ComponentProps(personRow, i) }

func est48(int) float32 { return 48 }

// virtualPage is a page whose list fills the window.
func virtualPage(n int, est func(int) float32) Node {
	return div(Class("flex h-screen flex-col"), VirtualList(n, est, personNode, Class("flex-1")))
}

// plainPage is virtualPage without virtualization: n rows in a scroll
// container.
func plainPage(n int) Node {
	rows := make([]Node, n)
	for i := range rows {
		rows[i] = WithKey(personNode(i), i)
	}
	return div(Class("flex h-screen flex-col"), div(Class("flex-1 overflow-y-auto"), List(rows)))
}

// benchTree mounts root at 800x600, scale 2, and settles it.
func benchTree(b testing.TB, root Node) *Tree {
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: benchW, H: benchH})
	tr.SetScale(benchScale)
	tr.Mount(root)
	for range 10 {
		tr.Frame()
		tr.FlushEffects()
		if !tr.Dirty() {
			break
		}
	}
	return tr
}

// mainScroller returns the scroller of the page's list: the one with the
// largest scroll range.
func mainScroller(b testing.TB, tr *Tree) *scroller {
	var best *scroller
	for _, s := range tr.scr.list {
		if s != nil && (best == nil || s.max.Y > best.max.Y) {
			best = s
		}
	}
	if best == nil || best.max.Y < 10_000 {
		b.Fatalf("no scrolling list: %+v", best)
	}
	return best
}

func heapMB() float64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func benchMount(b *testing.B, root func() Node) {
	b.ReportAllocs()
	var heap float64
	for b.Loop() {
		b.StopTimer()
		h0 := heapMB()
		b.StartTimer()
		tr := benchTree(b, root())
		b.StopTimer()
		heap = heapMB() - h0
		runtime.KeepAlive(tr)
		b.StartTimer()
	}
	b.ReportMetric(heap, "heap-MB")
}

// BenchmarkVirtualMount100k mounts a 100,000-row VirtualList and runs the
// frames until it settles.
func BenchmarkVirtualMount100k(b *testing.B) {
	benchMount(b, func() Node { return virtualPage(100_000, est40) })
}

// BenchmarkPlainMount10k mounts 10,000 rows without virtualization.
func BenchmarkPlainMount10k(b *testing.B) { benchMount(b, func() Node { return plainPage(10_000) }) }

// benchScroll is one steady scroll frame per iteration: the list moves by
// a wheel notch at once (53 px, the worst case of a notch animation) and
// Frame runs; with fb, the window's framebuffer is brought to the frame
// too (RasterizeFrame).
func benchScroll(b *testing.B, root Node, withRaster bool) {
	tr := benchTree(b, root)
	s := mainScroller(b, tr)
	var fb *image.RGBA
	var r raster.Renderer
	if withRaster {
		phys := platform.PhysicalSize(platform.Size{W: benchW, H: benchH}, benchScale)
		fb = image.NewRGBA(image.Rect(0, 0, phys.X, phys.Y))
	}
	b.ReportAllocs()
	builds := tr.Stats().Builds
	for b.Loop() {
		y := s.pos.Y + wheelNotch
		if y > s.max.Y {
			y = 0
		}
		tr.setScroll(s, gfx.Point{Y: y})
		tr.Frame()
		if withRaster {
			tr.RasterizeFrame(fb, &r, false)
		}
	}
	b.ReportMetric(float64(tr.Stats().Builds-builds)/float64(b.N), "builds/frame")
}

func BenchmarkVirtualScrollFrame100k(b *testing.B) {
	benchScroll(b, virtualPage(100_000, est40), false)
}
func BenchmarkVirtualScrollFrameRaster100k(b *testing.B) {
	benchScroll(b, virtualPage(100_000, est40), true)
}
func BenchmarkPlainScrollFrame10k(b *testing.B)       { benchScroll(b, plainPage(10_000), false) }
func BenchmarkPlainScrollFrameRaster10k(b *testing.B) { benchScroll(b, plainPage(10_000), true) }

// benchP95 runs a scripted scroll, one frame per iteration, through the
// real input paths: a wheel notch at the list every fourth frame (Chromium's
// 150 ms animation, so the list is always moving) and Page Down every
// second. It reports the median, p95 and worst frame times of the pipeline
// (Animate and Frame), of the pipeline plus bringing the window's
// framebuffer to the frame (raster: RasterizeFrame, what a Host does), and
// of the pipeline plus a full rasterization of the window at scale 2
// (full), and how many frames recorded the layers again.
func benchP95(b *testing.B, root Node) {
	tr := benchTree(b, root)
	tr.SetWindowFocused(true)
	s := mainScroller(b, tr)
	phys := platform.PhysicalSize(platform.Size{W: benchW, H: benchH}, benchScale)
	fb := image.NewRGBA(image.Rect(0, 0, phys.X, phys.Y))
	fbFull := image.NewRGBA(fb.Rect)
	var r, rFull raster.Renderer
	tr.RasterizeFrame(fb, &r, true)
	var pipe, incr, full []time.Duration
	recorded := 0
	now := time.Duration(0)
	frame := 0
	at := platform.Point{X: benchW / 2, Y: benchH / 2}
	start := s.pos.Y
	for b.Loop() {
		now += frameStep
		switch {
		case frame%60 == 30:
			tr.HandleKey(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodePageDown, Key: platform.KeyPageDown, Time: now})
			tr.HandleKey(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodePageDown, Key: platform.KeyPageDown, Time: now})
		case frame%4 == 0:
			tr.HandleScroll(platform.ScrollEvent{Pos: at, Steps: platform.Point{Y: 1}, Time: now})
		}
		frame++
		gen := tr.lay.gen
		t0 := time.Now()
		tr.Animate(now)
		dl, _ := tr.Frame()
		t1 := time.Now()
		tr.RasterizeFrame(fb, &r, false)
		t2 := time.Now()
		raster.Clear(fbFull, image.Rectangle{}, tr.CanvasColor())
		rFull.Render(fbFull, dl, raster.Options{Scale: benchScale})
		t3 := time.Now()
		tr.FlushEffects()
		pipe = append(pipe, t1.Sub(t0))
		incr = append(incr, t2.Sub(t0))
		full = append(full, t1.Sub(t0)+t3.Sub(t2))
		if tr.lay.gen != gen {
			recorded++
		}
	}
	if s.pos.Y <= start {
		b.Fatalf("the list did not scroll: %v", s.pos.Y)
	}
	q := func(d []time.Duration, p float64) float64 {
		d = slices.Clone(d)
		slices.Sort(d)
		return float64(d[min(int(float64(len(d))*p), len(d)-1)]) / float64(time.Millisecond)
	}
	b.ReportMetric(q(pipe, 0.5), "pipe-p50-ms")
	b.ReportMetric(q(pipe, 0.95), "pipe-p95-ms")
	b.ReportMetric(q(pipe, 1), "pipe-max-ms")
	b.ReportMetric(q(incr, 0.5), "raster-p50-ms")
	b.ReportMetric(q(incr, 0.95), "raster-p95-ms")
	b.ReportMetric(q(incr, 1), "raster-max-ms")
	b.ReportMetric(float64(recorded)/float64(len(pipe)), "recorded/frame")
	b.ReportMetric(q(full, 0.5), "full-p50-ms")
	b.ReportMetric(q(full, 0.95), "full-p95-ms")
	b.ReportMetric(q(full, 1), "full-max-ms")
	b.ReportMetric(float64(s.pos.Y-start), "px")
}

func BenchmarkScrollP95Virtual100k(b *testing.B) { benchP95(b, virtualPage(100_000, est40)) }
func BenchmarkScrollP95Virtual100kEstimated(b *testing.B) {
	benchP95(b, virtualPage(100_000, est48))
}
func BenchmarkScrollP95Plain10k(b *testing.B) { benchP95(b, plainPage(10_000)) }

// BenchmarkPlainUnmount5kOf10k unmounts the last 5,000 rows of a plain
// 10,000-row list (10,000 of its 20,000 truncate scroll containers) in one
// frame: the frame of the rebuild, without the mount.
//
//	CGO_ENABLED=0 go test -run '^$' -bench PlainUnmount -benchtime 5x ./internal/core/
func BenchmarkPlainUnmount5kOf10k(b *testing.B) {
	var setN func(int)
	page := func() Node {
		n, set := UseStateSet(10_000)
		setN = set
		rows := make([]Node, n)
		for i := range rows {
			rows[i] = WithKey(personNode(i), i)
		}
		return div(Class("flex h-screen flex-col"), div(Class("flex-1 overflow-y-auto"), List(rows)))
	}
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		tr := benchTree(b, Component(page))
		before := tr.scr.scrollers()
		setN(5_000)
		b.StartTimer()
		tr.Frame()
		b.StopTimer()
		if after := tr.scr.scrollers(); after > before-9_000 {
			b.Fatalf("%d scroll containers left of %d", after, before)
		}
		b.StartTimer()
	}
}

// TestBenchPagesScroll keeps the benchmark pages honest: both lists are
// 40 px rows that scroll, and the virtual one builds few of them.
func TestBenchPagesScroll(t *testing.T) {
	for _, est := range []func(int) float32{est40, est48} {
		tr := benchTree(t, virtualPage(100_000, est))
		s := mainScroller(t, tr)
		vl := onlyList(t, tr)
		n := len(vl.built)
		if want := float32(vl.sizes.total()) - benchH; s.max.Y != want || n > 30 || vl.sizes.at(3) != 40 {
			t.Errorf("virtual page: max %v (want %v), %d rows built, row 3 is %v px", s.max.Y, want, n, vl.sizes.at(3))
		}
	}
	tr2 := benchTree(t, plainPage(300))
	if s2 := mainScroller(t, tr2); s2.max.Y != 300*40-benchH {
		t.Errorf("plain page: max %v", s2.max.Y)
	}
}
