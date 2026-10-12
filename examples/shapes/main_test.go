package main

import (
	"bytes"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// reference renders Scene the way the CLI snapshot does, at a logical size and
// scale.
func reference(size platform.Size, scale float64) *image.RGBA {
	m := platform.Metrics{Size: size, Scale: scale}
	img := image.NewRGBA(image.Rectangle{Max: m.Physical()})
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, Scene(size.W, size.H), raster.Options{Scale: float32(scale)})
	return img
}

// start creates a headless backend with a window and the app handler, and
// delivers the first events.
func start(t *testing.T, scale float64) (*headless.Backend, *headless.Window, *app, *bytes.Buffer) {
	t.Helper()
	be := headless.New(headless.Options{Scale: scale})
	win, err := be.NewWindow(platform.WindowOptions{Title: "Tela — shapes", AppID: "dev.tela.shapes", Size: platform.Size{W: 960, H: 640}})
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	a := newApp(be, win, &log)
	be.Drain(a)
	return be, win.(*headless.Window), a, &log
}

func samePixels(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	if got.Rect != want.Rect {
		t.Fatalf("frame bounds = %v, want %v", got.Rect, want.Rect)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		n := 0
		for i := range got.Pix {
			if got.Pix[i] != want.Pix[i] {
				n++
			}
		}
		t.Fatalf("frame differs from raster.Render(Scene) in %d bytes", n)
	}
}

func TestPresentsScene(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(map[float64]string{1: "x1", 2: "x2"}[scale], func(t *testing.T) {
			_, win, a, log := start(t, scale)
			if win.Presents() != 1 {
				t.Fatalf("Presents() = %d after the first configure and frame, want 1", win.Presents())
			}
			samePixels(t, win.Frame(), reference(platform.Size{W: 960, H: 640}, scale))
			if a.frameCount != 1 {
				t.Errorf("frameCount = %d, want 1", a.frameCount)
			}
			if want := "shapes: headless configure 960x640 @"; !strings.HasPrefix(log.String(), want) {
				t.Errorf("log = %q, want a line starting with %q", log.String(), want)
			}
		})
	}
}

func TestConfigureLogLine(t *testing.T) {
	_, _, _, log := start(t, 2)
	if got, want := log.String(), "shapes: headless configure 960x640 @2 → 1920x1280 px\n"; got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

func TestResize(t *testing.T) {
	be, win, a, _ := start(t, 1)
	win.SetSize(platform.Size{W: 700, H: 500})
	be.Drain(a)
	if win.Presents() != 2 {
		t.Fatalf("Presents() = %d after a resize, want 2", win.Presents())
	}
	samePixels(t, win.Frame(), reference(platform.Size{W: 700, H: 500}, 1))

	// A scale change is answered with a frame of the new physical size too.
	win.SetScale(1.5)
	be.Drain(a)
	if win.Presents() != 3 {
		t.Fatalf("Presents() = %d after a scale change, want 3", win.Presents())
	}
	samePixels(t, win.Frame(), reference(platform.Size{W: 700, H: 500}, 1.5))
}

func TestIdleDoesNothing(t *testing.T) {
	be, win, a, _ := start(t, 1)
	if be.Pending() || win.FrameRequested() {
		t.Fatalf("work pending after the first frame: Pending=%v FrameRequested=%v", be.Pending(), win.FrameRequested())
	}
	for i := 0; i < 5; i++ {
		be.Advance(time.Second)
		if n := be.Drain(a); n != 0 {
			t.Fatalf("idle Drain delivered %d events, want 0", n)
		}
	}
	if win.Presents() != 1 || a.frameCount != 1 {
		t.Errorf("Presents() = %d, frames drawn = %d while idle, want 1 and 1", win.Presents(), a.frameCount)
	}
}

func TestCloseRequest(t *testing.T) {
	be, win, a, _ := start(t, 1)
	win.RequestClose()
	be.Drain(a)
	if !win.Closed() || !be.Quitting() {
		t.Errorf("Closed() = %v, Quitting() = %v after CloseRequestedEvent, want both true", win.Closed(), be.Quitting())
	}
}

// TestGolden allows a difference of one level per channel: the scene is
// ordinary Go float code, which the compiler may fuse into multiply-adds on
// arm64 and other architectures, moving coordinates by an ulp. The
// rasterizer itself is bit-identical across architectures (ADR-027) and is
// tested exactly in internal/gfx/raster.
func TestGolden(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"shapes_x1", 1}, {"shapes_x2", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			golden.Image(t, tc.name, reference(platform.Size{W: 960, H: 640}, tc.scale), golden.MaxDelta(1))
		})
	}
}

// TestSceneSizes checks that the scene builds and renders at unusual sizes
// (small, portrait, wide, fractional) without panicking.
func TestSceneSizes(t *testing.T) {
	for _, s := range []platform.Size{{W: 1, H: 1}, {W: 200, H: 150}, {W: 480, H: 800}, {W: 1600, H: 500}, {W: 777.5, H: 433.25}} {
		img := reference(s, 1.25)
		if img.Rect.Empty() && s.W >= 1 {
			t.Errorf("size %v: empty image", s)
		}
	}
}
