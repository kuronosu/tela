package telatest_test

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// scene is frame n of a picture: a sky, a sun that moves one step to the
// right with n, a checkered ground, and a red border one pixel wide that
// shows whether the frame is drawn pixel for pixel. Integers only, so it is
// the same on every machine.
func scene(w, h, n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	sunX := w * (2 + n) / 10
	for y := range h {
		for x := range w {
			var c color.RGBA
			switch {
			case x == 0 || y == 0 || x == w-1 || y == h-1:
				c = color.RGBA{220, 38, 38, 255}
			case (x-sunX)*(x-sunX)+(y-h/4)*(y-h/4) < h*h/25:
				c = color.RGBA{253, 224, 71, 255}
			case y < h*3/5:
				c = color.RGBA{uint8(60 + 100*y/h), uint8(130 + 100*y/h), 230, 255}
			case (x*10/w+y*8/h)%2 == 0:
				c = color.RGBA{22, 101, 52, 255}
			default:
				c = color.RGBA{134, 239, 172, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// camera is a card with a texture and no producer: the test is the producer.
func camera() tela.Node {
	tex := tela.UseTexture()
	return Div(Class("m-4 w-72 overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-lg"),
		Texture(tex, TestID("screen"), Class("aspect-video w-full"), AriaLabel("Front door camera")),
		Div(Class("flex items-center justify-between p-3"),
			Span(Class("text-sm font-semibold text-slate-800"), Text("Front door")),
			Span(Class("text-xs text-slate-500"), Text("live"))))
}

func TestSubmitTextureGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			app := telatest.Mount(t, tela.C(camera), telatest.Size(320, 260), telatest.Scale(scale))
			screen := telatest.ByTestID("screen")

			// The element asks for its content box, 286 logical pixels wide (the
			// card's border takes two of its 288), at a 16:9 ratio.
			size := app.TextureSize(screen)
			want := map[float64]tela.TextureSize{
				1: {Width: 286, Height: 161, Scale: 1},
				2: {Width: 572, Height: 322, Scale: 2},
			}[scale]
			if size != want {
				t.Fatalf("TextureSize = %+v, want %+v", size, want)
			}
			if st := app.TextureStats(screen); st.Submitted != 0 {
				t.Errorf("before the first frame: %+v", st)
			}

			app.SubmitTexture(screen, scene(size.Width, size.Height, 0))
			if st := app.TextureStats(screen); st.Submitted != 1 || st.Shown != 1 || st.Dropped != 0 {
				t.Errorf("after one frame: %+v, want 1 submitted, 1 shown, 0 dropped", st)
			}
			// The frame is on the screen when SubmitTexture returns: the red border
			// of the frame is the border of the element's content box.
			r := app.Bounds(screen)
			if got := app.PixelAt(r.X, r.Y+r.Height/2); got != (color.RGBA{220, 38, 38, 255}) {
				t.Errorf("the left border of the frame: %v", got)
			}
			app.Golden(fmt.Sprintf("texture_card_x%g.png", scale))

			// The same element by its name.
			if app.Texture(telatest.ByRole("img", "Front door camera")) != app.Texture(screen) {
				t.Error("ByRole and ByTestID locate different textures")
			}
		})
	}
}

func TestSubmitTextureErrors(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(camera), telatest.Size(320, 260))
		app.Texture(telatest.ByText("Front door"))
	}, "is not a texture", "ByTestID")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(camera), telatest.Size(320, 260))
		app.SubmitTexture(telatest.ByTestID("screen"), nil)
	}, "SubmitTexture", "nil")
}

// game is a screen whose texture has a producer that renders at the
// window's pace: one frame for every NextFrame, forever.
func game(rendered *atomic.Int32) tela.Component {
	return tela.C(func() tela.Node {
		tex := tela.UseTexture()
		tela.UseGo(func(ctx context.Context) {
			for n := 0; ; n++ {
				if _, err := tex.NextFrame(ctx); err != nil {
					return
				}
				s := tex.Size()
				tex.SubmitImage(scene(s.Width, s.Height, n), nil)
				rendered.Add(1)
			}
		}, tela.Once)
		return Div(Class("m-4 w-72 overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-lg"),
			Texture(tex, TestID("screen"), Class("aspect-video w-full"), AriaLabel("Game")),
			Div(Class("p-3"), Button(Class("rounded-md bg-slate-900 px-3 py-1 text-sm text-white"), Text("Pause"))))
	})
}

func TestStepTextureLoop(t *testing.T) {
	var rendered atomic.Int32
	f := mustPass(t, func(tb testing.TB) {
		app := telatest.Mount(tb, game(&rendered), telatest.Size(320, 260))
		screen := telatest.ByTestID("screen")

		// The loop never goes quiet, and settling does not wait for it: an
		// action returns, without the "still requesting frames" message.
		app.Click(telatest.ByRole("button", "Pause"))

		for range 3 {
			app.StepTexture(screen)
		}
		// Frame n is the nth one the producer rendered: three, the third
		// shown now, and each one drawn pixel for pixel.
		if got := rendered.Load(); got != 3 {
			t.Errorf("the producer rendered %d frames, want 3", got)
		}
		if st := app.TextureStats(screen); st.Submitted != 3 || st.Shown != 3 || st.Dropped != 0 {
			t.Errorf("stats after three steps: %+v", st)
		}
		app.Golden("texture_loop_step3.png")

		// What keeps the app awake is named.
		var found []string
		for _, p := range app.Pending() {
			if p.Kind == telatest.PendingNextFrame {
				found = append(found, p.String())
			}
		}
		if len(found) != 1 || !strings.Contains(found[0], "Texture.NextFrame") {
			t.Errorf("Pending = %q, want one NextFrame item", found)
		}
	})
	for _, l := range f.logs {
		if strings.Contains(l, "requesting frames") || strings.Contains(l, "kept handling work") {
			t.Errorf("settling gave up on the loop: %s", l)
		}
	}

	// The loop is not idle: it renders at every frame of the window.
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, game(new(atomic.Int32)), telatest.Size(320, 260))
		app.StepTexture(telatest.ByTestID("screen"))
		app.ExpectIdle()
	}, "not idle", "Texture.NextFrame")
}

func TestStepTextureMountedFrameByFrame(t *testing.T) {
	// The clock steps are exact: the same three steps show the same frame.
	var shots [2]*image.RGBA
	for i := range shots {
		app := telatest.Mount(t, game(new(atomic.Int32)), telatest.Size(320, 260))
		for range 4 {
			app.StepTexture(telatest.ByTestID("screen"))
		}
		shots[i] = app.Screenshot()
	}
	if !slices.Equal(shots[0].Pix, shots[1].Pix) {
		t.Error("two runs of four steps drew different pictures")
	}
}

func TestAdvanceLetsTheLoopRun(t *testing.T) {
	var rendered atomic.Int32
	app := telatest.Mount(t, game(&rendered), telatest.Size(320, 260))
	// The frames of the clock pace the producer, without stepping it.
	app.WaitFor("five frames", func() bool { return rendered.Load() >= 5 })
	app.Advance(100 * time.Millisecond)
	if rendered.Load() < 5 {
		t.Error("Advance did not run the loop")
	}
}

func TestStepTextureNeedsAProducer(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(camera), telatest.Size(320, 260), telatest.Timeout(100*time.Millisecond))
		app.StepTexture(telatest.ByTestID("screen"))
	}, "no goroutine waits in Texture.NextFrame", "tela.UseGo")
}

// ---------------------------------------------------------------------
// The fake device.

// gpuApp is the code path of an application that renders with its own GPU:
// it imports one dmabuf per buffer, in the format that Device lists, and
// submits them with a fence. The test plays the allocator and the GPU.
type gpuApp struct {
	mu       sync.Mutex
	tex      *tela.Texture
	bufs     []*tela.TextureBuffer
	released []int
}

func (g *gpuApp) view() tela.Node {
	tex := tela.UseTexture()
	g.tex = tex
	return Div(Class("m-4 w-72 overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-lg"),
		Texture(tex, TestID("screen"), Class("aspect-video w-full bg-slate-900"), AriaLabel("Player")),
		Div(Class("flex items-center justify-between p-3"),
			Span(Class("text-sm font-semibold text-slate-800"), Text("Player")),
			Span(Class("text-xs text-slate-500"), Text("imported dmabufs"))))
}

// start imports n buffers of size w x h, as an app does once its texture has
// the size it renders at.
func (g *gpuApp) start(w, h, n int) error {
	dev := g.tex.Device()
	if !dev.GPU || len(dev.Formats) == 0 {
		return errors.New("no GPU that imports dmabufs: a real app would fall back to SubmitImage here")
	}
	f := dev.Formats[0]
	for i := range n {
		b, err := g.tex.Import(tela.Dmabuf{
			Width: w, Height: h, Fourcc: f.Fourcc, Modifier: f.Modifier,
			Planes: []tela.DmabufPlane{{FD: 100 + i, Stride: uint32(4 * w)}},
		})
		if err != nil {
			return err
		}
		g.bufs = append(g.bufs, b)
	}
	return nil
}

// show submits buffer i, drawn by the GPU until fence ends.
func (g *gpuApp) show(i int, fence tela.Fence) error {
	return g.tex.Submit(g.bufs[i], tela.TextureFrame{Fence: fence, Release: func() {
		g.mu.Lock()
		g.released = append(g.released, i)
		g.mu.Unlock()
	}})
}

func (g *gpuApp) releasedBuffers() []int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.released)
}

const fourccAR24 = 0x34325241

func newDevice() *telatest.FakeTextureDevice {
	return telatest.NewFakeTextureDevice(tela.TextureDevice{
		GPU: true, Name: "Fake GPU", VendorID: 0x10de, DeviceID: 0x2486,
		RenderNode: "/dev/dri/renderD128",
		Formats:    []tela.TextureFormat{{Fourcc: fourccAR24}},
	})
}

func TestFakeDeviceDmabufPath(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			dev := newDevice()
			g := &gpuApp{}
			app := telatest.Mount(t, tela.C(g.view), telatest.Size(320, 260), telatest.Scale(scale), telatest.WithTextureDevice(dev))
			screen := telatest.ByTestID("screen")
			size := app.TextureSize(screen)

			if d := app.Texture(screen).Device(); d.Name != "Fake GPU" || !d.GPU || d.RenderNode != "/dev/dri/renderD128" {
				t.Errorf("Device() = %+v", d)
			}
			if err := g.start(size.Width, size.Height, 3); err != nil {
				t.Fatal(err)
			}
			imports := dev.Imports()
			if len(imports) != 3 {
				t.Fatalf("%d buffers imported, want 3", len(imports))
			}
			if d := imports[0].Dmabuf(); d.Width != size.Width || d.Height != size.Height || d.Fourcc != fourccAR24 {
				t.Errorf("buffer 0 is %+v", d)
			}

			// The GPU draws frame 0 into buffer 0; its fence has not signalled.
			imports[0].SetPixels(scene(size.Width, size.Height, 0))
			fence := telatest.NewFakeFence()
			if err := g.show(0, fence); err != nil {
				t.Fatal(err)
			}
			app.Screenshot() // settles: the frame waits for its fence
			var kinds []telatest.PendingKind
			for _, p := range app.Pending() {
				kinds = append(kinds, p.Kind)
			}
			if !slices.Contains(kinds, telatest.PendingFence) {
				t.Errorf("Pending = %v, want a texture fence", kinds)
			}
			app.ExpectIdle() // a frame that waits for its fence costs nothing
			if reads := imports[0].Reads(); reads != 0 {
				t.Errorf("Tela read the buffer %d time(s) before its fence signalled", reads)
			}
			if st := app.TextureStats(screen); st.Shown != 0 {
				t.Errorf("shown %d frames before the fence signalled", st.Shown)
			}

			// The fence signals: by the time the next screenshot looks, the frame is on it.
			fence.Signal()
			r := app.Bounds(screen)
			if got := app.PixelAt(r.X, r.Y+r.Height/2); got != (color.RGBA{220, 38, 38, 255}) {
				t.Errorf("the left border of the imported frame: %v: the frame was not on the screen right after Signal", got)
			}
			app.Golden(fmt.Sprintf("texture_import_x%g.png", scale))
			if st := app.TextureStats(screen); st.Submitted != 1 || st.Shown != 1 || st.Dropped != 0 {
				t.Errorf("stats: %+v", st)
			}
			if reads := imports[0].Reads(); reads != 1 {
				t.Errorf("the frame read the buffer %d times, want 1", reads)
			}
			for _, p := range app.Pending() {
				if p.Kind == telatest.PendingFence {
					t.Errorf("a fence is still pending: %v", p)
				}
			}

			// A second frame: buffer 1, whose GPU work fails. It is dropped and the
			// first stays on screen.
			imports[1].Fill(color.RGBA{255, 0, 255, 255})
			failing := telatest.NewFakeFence()
			if err := g.show(1, failing); err != nil {
				t.Fatal(err)
			}
			failing.Fail(errors.New("device hung"))
			app.Screenshot()
			if st := app.TextureStats(screen); st.Submitted != 2 || st.Shown != 1 || st.Dropped != 1 {
				t.Errorf("after the failed fence: %+v", st)
			}
			app.ExpectReleases(1) // buffer 1's frame went back at once, buffer 0's is on screen
			if got := g.releasedBuffers(); !slices.Equal(got, []int{1}) {
				t.Errorf("released %v, want [1]", got)
			}

			// A third frame without a fence replaces the first, which is then released.
			imports[2].SetPixels(scene(size.Width, size.Height, 5))
			if err := g.show(2, nil); err != nil {
				t.Fatal(err)
			}
			app.Screenshot()
			app.ExpectReleases(1)
			if got := g.releasedBuffers(); !slices.Equal(got, []int{1, 0}) {
				t.Errorf("released %v, want [1 0]", got)
			}

			// Unmounting releases the last frame and gives the buffers back. A
			// Release called twice or never fails the test when it ends.
			app.Close()
			if got := g.releasedBuffers(); !slices.Equal(got, []int{1, 0, 2}) {
				t.Errorf("released %v after the app ended, want [1 0 2]", got)
			}
			for i, b := range imports {
				waitUntil(t, fmt.Sprintf("buffer %d to be closed", i), b.Closed)
			}
		})
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); !cond(); {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFakeDeviceRefusesFormats(t *testing.T) {
	dev := newDevice()
	g := &gpuApp{}
	app := telatest.Mount(t, tela.C(g.view), telatest.Size(320, 260), telatest.WithTextureDevice(dev))
	app.Texture(telatest.ByTestID("screen")) // the texture exists
	_, err := g.tex.Import(tela.Dmabuf{Width: 64, Height: 64, Fourcc: fourccAR24, Modifier: 0x0100000000000001,
		Planes: []tela.DmabufPlane{{FD: 7, Stride: 256}}})
	if err == nil || !errors.Is(err, tela.ErrTextureUnsupported) && !strings.Contains(err.Error(), "modifier") {
		t.Errorf("a modifier the device does not import: %v", err)
	}
	if n := len(dev.Imports()); n != 0 {
		t.Errorf("%d buffers imported", n)
	}
	// With another description the same format is taken.
	dev.SetDevice(tela.TextureDevice{GPU: true, Formats: []tela.TextureFormat{{Fourcc: fourccAR24, Modifier: 0x0100000000000001}}})
	if d := g.tex.Device(); len(d.Formats) != 1 || d.Formats[0].Modifier != 0x0100000000000001 {
		t.Errorf("Device() = %+v", d)
	}
	if _, err := g.tex.Import(tela.Dmabuf{Width: 64, Height: 64, Fourcc: fourccAR24, Modifier: 0x0100000000000001,
		Planes: []tela.DmabufPlane{{FD: 7, Stride: 256}}}); err != nil {
		t.Errorf("Import: %v", err)
	}
}

func TestWithoutADeviceDeviceHasNoGPU(t *testing.T) {
	g := &gpuApp{}
	app := telatest.Mount(t, tela.C(g.view), telatest.Size(320, 260))
	app.Texture(telatest.ByTestID("screen"))
	if d := g.tex.Device(); d.GPU {
		t.Errorf("a headless app without a fake device has a GPU: %+v", d)
	}
}

func TestExpectReleasesCounts(t *testing.T) {
	var frees atomic.Int32
	f := mustPass(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(camera), telatest.Size(320, 260))
		screen := telatest.ByTestID("screen")
		x := app.Texture(screen)
		size := app.TextureSize(screen)
		app.ExpectReleases(0)
		for n := range 3 {
			if err := x.SubmitImage(scene(size.Width, size.Height, n), func() { frees.Add(1) }); err != nil {
				tb.Fatal(err)
			}
			app.Screenshot()
		}
		app.ExpectReleases(1) // the one on screen
	})
	if frees.Load() != 3 { // the third at the end of the test
		t.Errorf("%d releases, want 3 (the last one when the app unmounted)", frees.Load())
	}
	if f.failed {
		t.Error(f.failures())
	}

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(camera), telatest.Size(320, 260), telatest.Timeout(50*time.Millisecond))
		screen := telatest.ByTestID("screen")
		size := app.TextureSize(screen)
		app.Texture(screen).SubmitImage(scene(size.Width, size.Height, 0), func() {})
		app.Screenshot()
		app.ExpectReleases(0)
	}, "ExpectReleases(0)", "Tela holds 1 frame")
}

func TestPendingFenceNames(t *testing.T) {
	if got := telatest.PendingFence.String(); got != "texture fence" {
		t.Errorf("PendingFence = %q", got)
	}
	if got := telatest.PendingNextFrame.String(); got != "NextFrame" {
		t.Errorf("PendingNextFrame = %q", got)
	}
}
