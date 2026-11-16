//go:build telagpu && (linux || windows)

package core

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/gfx/gpu/gputest"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Windows presented through a swapchain on the window (gpuwin.go): Win32
// windows, faked here on the headless backend, so that the logic runs on
// every system with a Vulkan device. A window hands out a handle and the
// device makes a "swapchain" on it (fakeWindowOps, the gpu.SwapchainOps of
// a gpu.Swapchain): images in memory that a present copies the target's
// rectangles into, by reading them back, and shows whole as the headless
// window's frame. So every check of the frames a window shows
// (gpuChecker.checkWindow: bit for bit a full frame of the GPU, within
// ADR-282 of the CPU rasterizer's) applies to the images, whose pixels
// outside the damage of their own last copy are as old as the image. The
// window also checks the contract: no Present while a swapchain is on it,
// a whole Present after the swapchain's frames, no surface left when it
// closes, and the damage of a present (outside it, the image holds what
// the window showed).
//
// The real swapchain (vulkan_swapchain_windows.go) runs only on Windows,
// in the live test (live_gpu_windows_test.go).

// surfaceBackend is a headless backend whose windows are presented
// through swapchains on them, with a fake window system and presentation
// engine a test controls.
type surfaceBackend struct {
	*headless.Backend
	t    *testing.T
	dev  *fakeDevice
	wins map[*headless.Window]*surfaceWindow
	// byHandle finds a window by the handle it hands out.
	byHandle   map[uintptr]*surfaceWindow
	nextHandle uintptr
	// unsupported: the windows report no surface (as a transparent one,
	// one with a client-area backdrop). stuck: no image is ever free.
	// outOfDate: the next acquires answer out of date. lag: the next
	// Surface calls report another size than the window's. failNew and
	// failPresent fail the next NewSwapchain and every present.
	unsupported, stuck bool
	outOfDate, lag     int
	failNew            error
	failPresent        error
	// Counts: the surfaces made and alive, the swapchains made and alive.
	surfacesMade, surfacesLive, chainsMade, chainsLive int
}

func newSurfaceBackend(t *testing.T, b *headless.Backend, dev *fakeDevice) *surfaceBackend {
	s := &surfaceBackend{Backend: b, t: t, dev: dev, wins: map[*headless.Window]*surfaceWindow{}, byHandle: map[uintptr]*surfaceWindow{}}
	dev.surfaces = s
	return s
}

func (s *surfaceBackend) NewWindow(o platform.WindowOptions) (platform.Window, error) {
	if p, ok := o.Parent.(*surfaceWindow); ok {
		o.Parent = p.Window
	}
	w, err := s.Backend.NewWindow(o)
	if err != nil {
		return nil, err
	}
	s.nextHandle++
	sw := &surfaceWindow{Window: w.(*headless.Window), b: s, handle: 0x1000 + s.nextHandle, transparent: o.Transparent}
	s.wins[sw.Window] = sw
	s.byHandle[sw.handle] = sw
	return sw, nil
}

// wrapped returns the window the Host knows a headless window by.
func (s *surfaceBackend) wrapped(w *headless.Window) platform.Window {
	if sw := s.wins[w]; sw != nil {
		return sw
	}
	return w
}

// Drain delivers the headless backend's events to h under the wrapped
// windows.
func (s *surfaceBackend) Drain(h platform.Handler) int {
	return s.Backend.Drain(surfaceHandler{s, h})
}

type surfaceHandler struct {
	s *surfaceBackend
	h platform.Handler
}

func (m surfaceHandler) HandleEvent(w platform.Window, e platform.Event) {
	if hw, ok := w.(*headless.Window); ok {
		w = m.s.wrapped(hw)
	}
	m.h.HandleEvent(w, e)
}

// surfaceWindow is a headless window that is presented through a
// swapchain on it, as a Win32 window is (platform.Win32SurfacePresenter).
type surfaceWindow struct {
	*headless.Window
	b           *surfaceBackend
	handle      uintptr
	transparent bool
	// active: the latest frame came through the swapchain; full: the next
	// swapchain present must be whole.
	active, full bool
	// ops is the surface on the window, nil without one.
	ops *fakeWindowOps
	// What the window was told: swapchain presents, the whole ones among
	// them, the damage of the last one; Presents of a framebuffer.
	presented, wholes, copies int
	damage                    []image.Rectangle
}

var _ platform.Win32SurfacePresenter = (*surfaceWindow)(nil)

func (w *surfaceWindow) Win32Surface() (platform.Win32Surface, bool) {
	if w.b.unsupported || w.transparent || w.Closed() {
		return platform.Win32Surface{}, false
	}
	return platform.Win32Surface{Instance: 1, Window: w.handle, Size: w.surfaceSize(), Full: w.full || !w.active}, true
}

// surfaceSize is the client area now: empty while minimized.
func (w *surfaceWindow) surfaceSize() image.Point {
	if w.State().Minimized {
		return image.Point{}
	}
	return w.Metrics().Physical()
}

func (w *surfaceWindow) SurfacePresented(damage []image.Rectangle) {
	if w.ops == nil || w.ops.images == nil {
		w.b.t.Errorf("SurfacePresented on a window without a swapchain")
	}
	w.active = true
	w.presented++
	w.damage = slices.Clone(damage)
	if len(damage) == 0 {
		w.full = false
		w.wholes++
	}
}

// Present checks the contract's alternation: the swapchain is gone, and
// the first Present after its frames is whole.
func (w *surfaceWindow) Present(fb *image.RGBA, damage []image.Rectangle) error {
	if w.ops != nil && w.ops.images != nil {
		w.b.t.Errorf("Present while a swapchain is on the window")
	}
	if w.active && len(damage) > 0 {
		w.b.t.Errorf("the first Present after frames of the swapchain has the damage %v, want the whole frame", damage)
	}
	w.active = false
	w.copies++
	return w.Window.Present(fb, damage)
}

func (w *surfaceWindow) Close() {
	if w.ops != nil {
		w.b.t.Errorf("the window %q closes with a surface on it", w.Title())
	}
	w.Window.Close()
}

// repaint is what a Win32 window does for a WM_PAINT while its frames come
// through a swapchain: it asks for a whole frame.
func (w *surfaceWindow) repaint() {
	if w.active {
		w.full = true
	}
	w.RequestFrame()
}

// NewSwapchain makes the fake surface of a surfaceWindow (fakeDevice is a
// gpu.WindowPresenter).
func (d *fakeDevice) NewSwapchain(nw gpu.NativeWindow) (*gpu.Swapchain, error) {
	b := d.surfaces
	if b == nil {
		return nil, errors.New("test: this device presents to no window")
	}
	if err := b.failNew; err != nil {
		return nil, err
	}
	w := b.byHandle[nw.Window]
	if w == nil || nw.Instance != 1 {
		b.t.Fatalf("NewSwapchain for the handles %#x, %#x, which are no window's", nw.Instance, nw.Window)
	}
	if w.ops != nil {
		b.t.Errorf("a second surface on the window %q", w.Title())
	}
	w.ops = &fakeWindowOps{w: w, acquired: -1}
	b.surfacesMade++
	b.surfacesLive++
	return gpu.NewSwapchainOf(w.ops), nil
}

var _ gpu.WindowPresenter = (*fakeDevice)(nil)

// fakeWindowOps is a swapchain on a surfaceWindow: images in memory, of
// which the one presented last is shown and busy until the next present.
type fakeWindowOps struct {
	w        *surfaceWindow
	images   []*image.RGBA
	busy     []bool
	acquired int
	closed   bool
}

func (o *fakeWindowOps) Surface() (gpu.SurfaceInfo, error) {
	b := o.w.b
	if o.closed {
		b.t.Fatal("Surface on a closed surface")
	}
	if o.w.Closed() {
		return gpu.SurfaceInfo{}, errors.New("test: the window is gone (VK_ERROR_SURFACE_LOST_KHR)")
	}
	extent := o.w.surfaceSize()
	if b.lag > 0 && extent != (image.Point{}) {
		b.lag--
		extent = extent.Add(image.Pt(7, 3))
	}
	return gpu.SurfaceInfo{Extent: extent, MinImages: 2, Incremental: true}, nil
}

func (o *fakeWindowOps) Create(extent image.Point, n int) (int, error) {
	b := o.w.b
	if want := o.w.surfaceSize(); extent != want {
		b.t.Errorf("a swapchain of %v on a window of %v", extent, want)
	}
	if o.images == nil {
		b.chainsLive++
	}
	b.chainsMade++
	o.images, o.busy, o.acquired = make([]*image.RGBA, n), make([]bool, n), -1
	for i := range o.images {
		o.images[i] = image.NewRGBA(image.Rectangle{Max: extent})
		for j := range o.images[i].Pix {
			o.images[i].Pix[j] = 0x5a // what a driver leaves
		}
	}
	return n, nil
}

func (o *fakeWindowOps) Acquire() (int, gpu.SwapchainStatus, error) {
	b := o.w.b
	if o.images == nil {
		b.t.Fatal("Acquire without a swapchain")
	}
	if b.outOfDate > 0 {
		b.outOfDate--
		return 0, gpu.SwapchainOutOfDate, nil
	}
	if !b.stuck {
		for i := range o.images {
			if !o.busy[i] && i != o.acquired {
				o.acquired = i
				return i, gpu.SwapchainOK, nil
			}
		}
	}
	return 0, gpu.SwapchainNotReady, nil
}

func (o *fakeWindowOps) Present(t gpu.Target, index int, copies, damage []image.Rectangle) (gpu.SwapchainStatus, error) {
	b, w := o.w.b, o.w
	if index != o.acquired {
		b.t.Fatalf("presenting the image %d, the acquired one is %d", index, o.acquired)
	}
	o.acquired = -1
	if err := b.failPresent; err != nil {
		return 0, err
	}
	img := o.images[index]
	if t.Size() != img.Rect.Size() {
		b.t.Fatalf("presenting a target of %v with a swapchain of %v", t.Size(), img.Rect.Size())
	}
	if len(copies) > 0 {
		if err := t.ReadPixels(img, copies...); err != nil {
			return 0, err
		}
	}
	// Outside the damage the image holds what the window showed.
	if prev := w.Frame(); len(damage) > 0 && w.active && prev != nil && prev.Rect == img.Rect {
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				if i := img.PixOffset(x, y); string(img.Pix[i:i+4]) != string(prev.Pix[i:i+4]) && !inRects(image.Pt(x, y), damage) {
					b.t.Fatalf("a present: pixel (%d, %d) changed from %v to %v outside the damage %v", x, y, prev.RGBAAt(x, y), img.RGBAAt(x, y), damage)
				}
			}
		}
	}
	if w.Visible() {
		shown := image.NewRGBA(img.Rect)
		copy(shown.Pix, img.Pix)
		if err := w.Window.Present(shown, nil); err != nil {
			return 0, err
		}
	}
	clear(o.busy)
	o.busy[index] = true
	return gpu.SwapchainOK, nil
}

func (o *fakeWindowOps) DestroySwapchain() {
	if o.images != nil {
		o.w.b.chainsLive--
	}
	o.images, o.busy, o.acquired = nil, nil, -1
}

func (o *fakeWindowOps) Close() {
	if o.closed {
		o.w.b.t.Error("a surface closed twice")
	}
	o.DestroySwapchain()
	o.closed = true
	o.w.b.surfacesLive--
	o.w.ops = nil
}

// newSurfaceRig returns a rig whose windows are presented through
// swapchains on them.
func newSurfaceRig(t *testing.T, dev gpu.Device, opts Options, w, h float32, scale float64) *gpuRig {
	t.Helper()
	x := &gpuRig{t: t, c: newGPUChecker(t, dev), log: &bytes.Buffer{}}
	x.b = headless.New(headless.Options{Size: platform.Size{W: w, H: h}, Scale: scale})
	x.s = newSurfaceBackend(t, x.b, x.c.fake)
	x.c.pw = x.s.wrapped
	opts.Fonts, opts.Flavor, opts.ErrorLog = testFonts, testFlavor, x.log
	host, err := NewHost(x.s, opts)
	if err != nil {
		t.Fatal(err)
	}
	x.h = host
	t.Cleanup(host.stopTimer)
	host.UseGPU(x.c.g)
	return x
}

// sw returns the main window.
func (x *gpuRig) sw() *surfaceWindow { return x.h.Window().(*surfaceWindow) }

// finishSurfaces ends the host and checks that no surface or swapchain is
// left.
func (x *gpuRig) finishSurfaces() {
	x.t.Helper()
	x.c.finish(x.h)
	if s := x.s; s.surfacesLive != 0 || s.chainsLive != 0 {
		x.t.Errorf("after the host ended: %d of %d surfaces and %d of %d swapchains alive", s.surfacesLive, s.surfacesMade, s.chainsLive, s.chainsMade)
	}
}

// TestGPUSurfaceSession plays the session of TestGPUSession on a window
// presented through a swapchain on it: hover, scrolls, edits, a dialog,
// resizes, a scale change, hiding and showing, minimizing, the theme, full
// frames and a rest. Every frame the window shows is the frame; none goes
// through the framebuffer; a swapchain is made again only for a new size
// and after the window was hidden; nothing is left when the host ends.
func TestGPUSurfaceSession(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		for _, scale := range []float64{1, 1.5} {
			t.Run(fmt.Sprintf("x%v", scale), func(t *testing.T) {
				st := &sessionState{}
				x := newSurfaceRig(t, dev, Options{Root: sessionPage(st)}, 640, 420, scale)
				playGPUSession(t, x, st, scale)
				sw := x.sw()
				if sw.copies != 0 {
					t.Errorf("%d frames were presented from the framebuffer", sw.copies)
				}
				// Three resizes and two scale changes make swapchains of
				// new sizes, the show after the hide one more.
				if sw.presented < 80 || x.s.chainsMade < 6 || x.s.chainsMade > 8 || x.s.surfacesMade != 1 {
					t.Errorf("%d presents through %d swapchains on %d surfaces", sw.presented, x.s.chainsMade, x.s.surfacesMade)
				}
				gw := x.h.GPUWindow(x.h.Window())
				if !gw.Surface || !gw.Direct || gw.Swapchain.Shown != sw.presented {
					t.Errorf("GPUWindow %+v; the window was told of %d presents", gw, sw.presented)
				}
				t.Logf("%d presents (%d whole) through %d swapchains", sw.presented, sw.wholes, x.s.chainsMade)
				x.finishSurfaces()
			})
		}
	})
}

// counterFrames changes the counter page n times, with a hover each time.
func counterFrames(x *gpuRig, set *Setter[int], what string, n int) {
	x.t.Helper()
	rx, ry := x.at(x.main(), "row")
	for i := 0; i < n; i++ {
		set.Update(func(v int) int { return v + 1 })
		x.step(what)
		x.main().Inject(move(rx, ry+float32(40*(i%2))))
		x.step(what + ", hover")
	}
}

// TestGPUSurfaceRetry checks the frames that cannot be presented right
// away: no image free, a window at another size than its frame (a resize
// under way), a swapchain out of date. The frame is kept and a frame
// asked for, and the window then shows it.
func TestGPUSurfaceRetry(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		x := newSurfaceRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 1)
		sw, v := x.sw(), x.h.main
		x.drain()
		x.check("mount")
		counterFrames(x, &set, "direct", 2)

		// No image free: the frames are drawn, not presented, and each
		// asks for the next.
		x.s.stuck = true
		presented := sw.presented
		set.Update(func(v int) int { return v + 1 })
		for i := 0; i < 3; i++ {
			x.b.Advance(x.b.FrameInterval())
			x.deliver()
			if !v.gpu.owed || !x.main().FrameRequested() {
				t.Fatalf("a frame with no image free: owed %v, a frame requested %v", v.gpu.owed, x.main().FrameRequested())
			}
		}
		if sw.presented != presented {
			t.Fatalf("%d presents while no image was free", sw.presented-presented)
		}
		x.s.stuck = false
		x.step("an image is free again")
		if sw.presented != presented+1 || v.gpu.owed {
			t.Errorf("after the images came back: %d presents, owed %v", sw.presented-presented, v.gpu.owed)
		}

		// The window at another size for a few frames.
		x.s.lag = 2
		set.Update(func(v int) int { return v + 1 })
		for i := 0; i < 2; i++ {
			x.b.Advance(x.b.FrameInterval())
			x.deliver()
		}
		x.step("the window at its size again")
		counterFrames(x, &set, "after the lag", 2)

		// Out of date: made again within the present.
		made := x.s.chainsMade
		x.s.outOfDate = 1
		counterFrames(x, &set, "out of date", 1)
		if x.s.chainsMade != made+1 {
			t.Errorf("out of date: %d swapchains made, want 1", x.s.chainsMade-made)
		}
		if sw.copies != 0 || x.log.Len() != 0 {
			t.Errorf("%d frames from the framebuffer; log %q", sw.copies, x.log)
		}
		x.finishSurfaces()
	})
}

// TestGPUSurfaceRepaint checks a window that lost what it showed (a
// WM_PAINT, a reconnected session): it asks for a whole frame, which is
// presented although nothing changed; a frame with nothing new and nothing
// asked presents nothing. Minimized, nothing is presented and the
// swapchain is kept; restored, the window shows the frame.
func TestGPUSurfaceRepaint(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		x := newSurfaceRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 2)
		sw := x.sw()
		x.drain()
		x.check("mount")
		counterFrames(x, &set, "direct", 2)
		presented, wholes := sw.presented, sw.wholes
		sw.repaint()
		x.step("a repaint")
		if sw.presented != presented+1 || sw.wholes != wholes+1 || sw.damage != nil {
			t.Errorf("a repaint: %d presents, %d whole, damage %v", sw.presented-presented, sw.wholes-wholes, sw.damage)
		}
		x.main().RequestFrame()
		x.step("a frame with nothing new")
		if sw.presented != presented+1 {
			t.Errorf("a frame with nothing new was presented")
		}
		made := x.s.chainsMade
		x.main().Minimize()
		x.step("minimize")
		if x.s.chainsLive != 1 {
			t.Errorf("minimized: %d swapchains alive, want the one kept", x.s.chainsLive)
		}
		set.Update(func(v int) int { return v + 1 })
		x.step("a change while minimized")
		x.h.Handle(x.h.Window()).Focus()
		x.settle("restore")
		if x.s.chainsMade != made || sw.copies != 0 {
			t.Errorf("restored: %d swapchains made, %d frames from the framebuffer", x.s.chainsMade-made, sw.copies)
		}
		x.finishSurfaces()
	})
}

// TestGPUSurfaceFallback checks the windows that are read back instead: a
// device that cannot make the swapchain, a present that fails and a lost
// device in the middle of a session, after which the window shows the
// frame at once and every later one, read back, with one line of the log
// and no surface left; a window that cannot be presented this way, from
// the start or for a while (a backdrop set and removed), without a line;
// and the debug overlays.
func TestGPUSurfaceFallback(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		open := func(t *testing.T, before func(x *gpuRig)) *gpuRig {
			x := newSurfaceRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 2)
			if before != nil {
				before(x)
			}
			x.drain()
			x.check("mount")
			return x
		}
		after := func(t *testing.T, x *gpuRig, line string, cpu bool) {
			t.Helper()
			sw := x.sw()
			copies := sw.copies
			counterFrames(x, &set, "after the failure", 3)
			if sw.copies < copies+3 || x.direct() || x.s.surfacesLive != 0 {
				t.Errorf("after the failure: %d frames from the framebuffer (were %d), direct %v, %d surfaces", sw.copies, copies, x.direct(), x.s.surfacesLive)
			}
			if n := strings.Count(x.log.String(), "\n"); n != 1 || !strings.Contains(x.log.String(), line) {
				t.Errorf("the log is %q, want one line with %q", x.log, line)
			}
			if cpu != (x.c.cpuFrames > 0) || cpu != (x.h.GPU() == nil) {
				t.Errorf("frames of the CPU rasterizer: %d, host GPU %v; want the CPU: %v", x.c.cpuFrames, x.h.GPU() != nil, cpu)
			}
			x.finishSurfaces()
		}
		boom := errors.New("boom")
		t.Run("new_swapchain", func(t *testing.T) {
			x := open(t, func(x *gpuRig) { x.s.failNew = boom })
			if sw := x.sw(); sw.copies != 1 || sw.presented != 0 {
				t.Errorf("the first frame: %d from the framebuffer, %d through a swapchain", sw.copies, sw.presented)
			}
			after(t, x, "cannot be presented without a copy", false)
		})
		t.Run("present", func(t *testing.T) {
			x := open(t, nil)
			counterFrames(x, &set, "direct", 2)
			x.s.failPresent = boom
			after(t, x, "cannot be presented without a copy", false)
		})
		t.Run("device_lost", func(t *testing.T) {
			x := open(t, nil)
			counterFrames(x, &set, "direct", 2)
			x.c.fake.fail = func(op string) error {
				if op == "ReadPixels" {
					return fmt.Errorf("%w: test", gpu.ErrDeviceLost)
				}
				return nil
			}
			after(t, x, "the GPU device is lost", true)
		})
		t.Run("unsupported", func(t *testing.T) {
			x := open(t, func(x *gpuRig) { x.s.unsupported = true })
			counterFrames(x, &set, "read back", 3)
			if sw := x.sw(); sw.presented != 0 || x.s.surfacesMade != 0 || sw.copies < 4 || x.log.Len() != 0 {
				t.Errorf("%d presents through a swapchain, %d surfaces, %d from the framebuffer; log %q", sw.presented, x.s.surfacesMade, sw.copies, x.log)
			}
			x.finishSurfaces()
		})
		t.Run("unsupported_for_a_while", func(t *testing.T) {
			x := open(t, nil)
			sw := x.sw()
			counterFrames(x, &set, "direct", 2)
			x.s.unsupported = true
			counterFrames(x, &set, "a backdrop", 2)
			if sw.copies < 4 || x.s.chainsLive != 0 {
				t.Errorf("a window that cannot be presented through a swapchain: %d from the framebuffer, %d swapchains alive", sw.copies, x.s.chainsLive)
			}
			x.s.unsupported = false
			presented := sw.presented
			counterFrames(x, &set, "the backdrop removed", 2)
			if sw.presented < presented+4 || sw.copies > 5 || x.log.Len() != 0 {
				t.Errorf("back: %d presents through a swapchain, %d from the framebuffer; log %q", sw.presented-presented, sw.copies, x.log)
			}
			x.finishSurfaces()
		})
		t.Run("debug_overlays", func(t *testing.T) {
			st := &sessionState{}
			x := newSurfaceRig(t, dev, Options{Config: Config{Debug: DebugPaintFlash | DebugLayers}, Root: sessionPage(st)}, 640, 420, 1.5)
			sw := x.sw()
			copies, presented := 0, 0
			playGPUDebug(t, x, st, func(f DebugFlags) {
				switch {
				case f != 0 && (sw.presented != presented || sw.copies < copies+5 || x.s.chainsLive != 0 || x.direct()):
					t.Errorf("debug %s: %d presents through a swapchain (were %d), %d from the framebuffer (were %d), %d swapchains, direct %v", f, sw.presented, presented, sw.copies, copies, x.s.chainsLive, x.direct())
				case f == 0 && (sw.copies != copies || sw.presented < presented+5 || !x.direct()):
					t.Errorf("overlays off: %d presents from the framebuffer (were %d), %d through a swapchain (were %d), direct %v", sw.copies, copies, sw.presented, presented, x.direct())
				}
				copies, presented = sw.copies, sw.presented
			})
			if x.log.Len() != 0 {
				t.Errorf("the host logged: %s", x.log)
			}
			x.finishSurfaces()
		})
	})
}

// TestGPUSurfaceChildWindow presents two windows of one app through
// swapchains, each on its own surface, and closes one: its surface goes
// before it.
func TestGPUSurfaceChildWindow(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var setOpen Setter[bool]
		inner := div(Key("c"), Class("h-screen bg-emerald-200 p-2 hover:bg-emerald-400"), Text("child"))
		root := Component(func() Node {
			open, so := UseState(true)
			setOpen = so
			c := []Child{Class("h-screen bg-sky-200")}
			if open {
				c = append(c, WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "child", Width: 200, Height: 150}, Root: inner}), "child"))
			}
			return div(c...)
		})
		x := newSurfaceRig(t, dev, Options{Root: root}, 320, 240, 1)
		x.drain()
		x.settle("mount")
		if x.s.surfacesLive != 2 || len(x.s.wins) != 2 {
			t.Fatalf("%d surfaces on %d windows, want 2 and 2", x.s.surfacesLive, len(x.s.wins))
		}
		setOpen.Set(false)
		x.settle("close the child")
		if x.s.surfacesLive != 1 {
			t.Errorf("after the child closed: %d surfaces alive, want 1", x.s.surfacesLive)
		}
		x.finishSurfaces()
	})
}
