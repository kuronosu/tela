package raster

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// parallelSplits are the (workers, band rows) pairs of the parallel tests.
// Rows 0 is the automatic height; 1 and odd heights stress the band
// boundaries.
var parallelSplits = []struct{ workers, rows int }{
	{1, 0}, {2, 1}, {3, 7}, {4, 16}, {7, 3}, {16, 0}, {16, 1},
}

// allScenes returns every golden scene and the random scenes of the region
// tests.
func allScenes() []scene {
	all := append(append(append(append([]scene(nil), scenes...), scenes2...), scenes3...), textScenes...)
	for seed := uint64(1); seed <= 4; seed++ {
		all = append(all, randomScene(seed, 150, 110))
	}
	return all
}

// TestParallelMatchesSequential checks that a parallel render gives exactly
// the pixels of a sequential one, for every worker count and band height,
// over the full frame, odd sub-regions and sub-images, and that it leaves
// the pixels outside the region untouched.
func TestParallelMatchesSequential(t *testing.T) {
	scales := []float32{1, 1.5, 2}
	splits := parallelSplits
	if testing.Short() {
		scales = []float32{1, 1.5}
		splits = []struct{ workers, rows int }{{3, 7}, {16, 1}, {4, 0}}
	}
	for _, s := range allScenes() {
		for _, scale := range scales {
			t.Run(fmt.Sprintf("%s_x%g", s.name, scale), func(t *testing.T) {
				// Every subtest owns its Renderer and images; the scenes
				// and the display lists are only read.
				t.Parallel()
				dl := build(s.draw)
				w := int(math.Ceil(float64(float32(s.w) * scale)))
				h := int(math.Ceil(float64(float32(s.h) * scale)))
				bg := image.NewRGBA(image.Rect(0, 0, w, h))
				background(bg)
				want := cloneRGBA(bg)
				NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})

				rng := rand.New(rand.NewPCG(uint64(w), uint64(h)))
				r := NewRenderer() // reused across splits: worker scratch is reused too
				r.tune.minPixels = 1
				for _, sp := range splits {
					r.tune.bandRows = sp.rows
					regions := []image.Rectangle{
						{}, // the full frame
						image.Rect(rng.IntN(w/3), rng.IntN(h/3)+1, w-rng.IntN(w/3)-1, h-rng.IntN(h/3)),
						image.Rect(-7, h/2-3, w/2+5, h+9), // sticking out of the image
						image.Rect(w/3, 0, w/3+1, h),      // one column: one pixel per band
					}
					for _, region := range regions {
						got := cloneRGBA(bg)
						r.Render(got, dl, Options{Scale: scale, Region: region, Workers: sp.workers})
						check := region
						if check.Empty() {
							check = got.Rect
						}
						checkParallel(t, sp.workers, sp.rows, got, want, bg, check)
					}
					// A sub-image: the bands start at its origin.
					region := regions[1]
					got := cloneRGBA(bg)
					r.Render(got.SubImage(region).(*image.RGBA), dl, Options{Scale: scale, Workers: sp.workers})
					checkParallel(t, sp.workers, sp.rows, got, want, bg, region)
				}
			})
		}
	}
}

func checkParallel(t *testing.T, workers, rows int, got, full, bg *image.RGBA, region image.Rectangle) {
	t.Helper()
	for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
		for x := got.Rect.Min.X; x < got.Rect.Max.X; x++ {
			i := got.PixOffset(x, y)
			want := bg.Pix[i : i+4]
			if image.Pt(x, y).In(region) {
				want = full.Pix[i : i+4]
			}
			if !bytes.Equal(got.Pix[i:i+4], want) {
				t.Fatalf("workers %d, rows %d, region %v: pixel (%d, %d) = %v, want %v (inside region: %v)",
					workers, rows, region, x, y, got.Pix[i:i+4], want, image.Pt(x, y).In(region))
			}
		}
	}
}

// TestParallelDefaults renders with the default threshold and band height
// (Workers 0 means GOMAXPROCS) and through the pooled Render.
func TestParallelDefaults(t *testing.T) {
	dl := build(drawTailwindScene)
	for _, scale := range []float32{1, 2} {
		w, h := int(640*scale), int(600*scale)
		want := image.NewRGBA(image.Rect(0, 0, w, h))
		Clear(want, image.Rectangle{}, gfx.White)
		got := cloneRGBA(want)
		got2 := cloneRGBA(want)
		NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})
		NewRenderer().Render(got, dl, Options{Scale: scale})
		Render(got2, dl, Options{Scale: scale, Workers: 5})
		if !bytes.Equal(got.Pix, want.Pix) || !bytes.Equal(got2.Pix, want.Pix) {
			t.Errorf("scale %v: parallel render differs from the sequential one", scale)
		}
	}
}

// TestParallelSmallRegion checks that small regions and single bands take
// the sequential path: no worker Renderer is created.
func TestParallelSmallRegion(t *testing.T) {
	dl := build(scenes[0].draw)
	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	r := NewRenderer()
	r.Render(img, dl, Options{Workers: 8, Region: image.Rect(0, 0, 100, 100)})
	if r.par != nil {
		t.Errorf("a %d-pixel region was split", 100*100)
	}
	r.tune.minPixels, r.tune.bandRows = 1, 1000
	r.Render(img, dl, Options{Workers: 8})
	if r.par != nil {
		t.Errorf("a single band was split")
	}
}

func TestParallelBandRows(t *testing.T) {
	plain := build(scenes[0].draw)
	area := image.Rect(0, 0, 1920, 1080)
	for _, tc := range []struct{ workers, want int }{{2, 135}, {4, 68}, {18, 16}, {64, 16}} {
		if got := bandRowsFor(plain, 1, area, tc.workers); got != tc.want {
			t.Errorf("%d workers: %d rows, want %d", tc.workers, got, tc.want)
		}
	}
	// A full-frame blur with reach 24 asks for 64 rows; with 18 workers it
	// gets one band per worker.
	blur := benchLayerBlur()
	if got := bandRowsFor(blur, 1, area, 18); got != 60 {
		t.Errorf("blur: %d rows, want 60", got)
	}
	if got := bandRowsFor(blur, 1, area, 4); got != 68 {
		t.Errorf("blur, 4 workers: %d rows, want 68", got)
	}
	if got := bandRowsFor(blur, 1, area, 12); got != 64 {
		t.Errorf("blur, 12 workers: %d rows, want 64", got)
	}
	// A small blurred layer barely changes the height.
	small := build(func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.White)
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 32})
		b.FillRect(gfx.RectXYWH(100, 100, 100, 100), rose)
		b.PopLayer()
	})
	if got := bandRowsFor(small, 1, area, 18); got != 16 {
		t.Errorf("small blur: %d rows, want 16", got)
	}
}

// waitGoroutines waits until at most n goroutines run: worker goroutines
// exit right after they signal the end of their work.
func waitGoroutines(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > n {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines running, want at most %d", runtime.NumGoroutine(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestParallelPanic checks that a panic in any band reaches the caller with
// its original value, after every worker has stopped, that no goroutine is
// left behind and that the Renderer keeps working.
func TestParallelPanic(t *testing.T) {
	cases := []struct {
		draw func(b *gfx.Builder)
		want string
	}{
		{func(b *gfx.Builder) { // only in the bottom bands
			b.FillRect(gfx.Rect{X1: 200, Y1: 200}, sky)
			b.PushLayer(1, gfx.Screen)
			b.FillRect(gfx.RectXYWH(0, 180, 200, 20), gfx.Black)
			b.PopLayer()
		}, "raster: blend mode screen is not implemented until phase 6"},
		{func(b *gfx.Builder) { // in every band
			b.PushLayer(1, gfx.Multiply)
			b.FillRect(gfx.Rect{X1: 200, Y1: 200}, gfx.Black)
			b.PopLayer()
		}, "raster: blend mode multiply is not implemented until phase 6"},
	}
	good := build(scenes[1].draw)
	want := image.NewRGBA(image.Rect(0, 0, 200, 200))
	NewRenderer().Render(want, good, Options{Workers: 1})

	before := runtime.NumGoroutine()
	r := NewRenderer()
	r.tune.minPixels = 1
	for k := 0; k < 50; k++ {
		tc := cases[k%len(cases)]
		r.tune.bandRows = 1 + k%5
		img := image.NewRGBA(image.Rect(0, 0, 200, 200))
		func() {
			defer func() {
				v := recover()
				if s, ok := v.(string); !ok || s != tc.want {
					t.Fatalf("panic %#v, want %q", v, tc.want)
				}
			}()
			r.Render(img, build(tc.draw), Options{Workers: 2 + k%7})
			t.Fatalf("no panic")
		}()
		// The Renderer is still usable.
		got := image.NewRGBA(image.Rect(0, 0, 200, 200))
		r.Render(got, good, Options{Workers: 2 + k%7})
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("render after a panic differs")
		}
	}
	waitGoroutines(t, before)
}

// TestParallelConcurrent renders the same display lists (which share
// images) with several Renderers at once, each of them parallel, many
// times: every frame must be identical to the sequential render. Without
// the race detector (cgo is not allowed), shared mutable state between
// Renderers or workers would show up as differing pixels.
func TestParallelConcurrent(t *testing.T) {
	type job struct {
		dl    *gfx.DisplayList
		scale float32
		w, h  int
		want  *image.RGBA
	}
	var jobs []job
	for _, s := range []scene{scenes3[2], scenes2[3], scenes2[5], randomScene(7, 150, 110), randomScene(8, 150, 110)} {
		for _, scale := range []float32{1, 1.5} {
			dl := build(s.draw)
			w := int(math.Ceil(float64(float32(s.w) * scale)))
			h := int(math.Ceil(float64(float32(s.h) * scale)))
			want := image.NewRGBA(image.Rect(0, 0, w, h))
			background(want)
			NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})
			jobs = append(jobs, job{dl, scale, w, h, want})
		}
	}
	iters := 40
	if testing.Short() {
		iters = 8
	}
	before := runtime.NumGoroutine()
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := NewRenderer()
			r.tune.minPixels = 1
			for k := 0; k < iters; k++ {
				j := jobs[(g+k)%len(jobs)]
				r.tune.bandRows = []int{0, 1, 5, 16}[(g+k)%4]
				img := image.NewRGBA(image.Rect(0, 0, j.w, j.h))
				background(img)
				r.Render(img, j.dl, Options{Scale: j.scale, Workers: 2 + (g+k)%4})
				if !bytes.Equal(img.Pix, j.want.Pix) {
					errs <- fmt.Sprintf("goroutine %d, iteration %d: frame differs", g, k)
					return
				}
			}
		}()
	}
	// Two goroutines share one image, each rendering its half through
	// Region, both in parallel.
	half := jobs[0]
	shared := image.NewRGBA(image.Rect(0, 0, half.w, half.h))
	background(shared)
	for i, region := range []image.Rectangle{image.Rect(0, 0, half.w, half.h/2), image.Rect(0, half.h/2, half.w, half.h)} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := NewRenderer()
			r.tune.minPixels, r.tune.bandRows = 1, 3+i
			r.Render(shared, half.dl, Options{Scale: half.scale, Region: region, Workers: 3})
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if !bytes.Equal(shared.Pix, half.want.Pix) {
		t.Error("halves rendered concurrently into one image differ")
	}
	waitGoroutines(t, before)
}

func TestParallelOptionsValidation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	dl := build(func(b *gfx.Builder) { b.FillRect(gfx.Rect{X1: 1, Y1: 1}, gfx.Black) })
	defer func() {
		if r, _ := recover().(string); !strings.Contains(r, "Options.Workers") {
			t.Errorf("Workers -1: panic %q", r)
		}
	}()
	Render(img, dl, Options{Workers: -1})
}
