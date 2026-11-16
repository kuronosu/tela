//go:build telagpu && (linux || windows)

package core

import (
	"errors"
	"fmt"
	"image"
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/gfx/gpu/gputest"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Windows presented without a copy (gpuchain.go, gpuhost_on.go). The
// headless backend's windows present no dmabufs, so these tests wrap them
// in windows that do, as a display server would: a buffer is registered
// from the descriptors of an image the device exported, a present makes
// it busy and releases the ones before it, a hidden window drops it.
// What such a window shows is the whole content of the buffer it was
// given, read back through Vulkan from the exported image at every
// present, and stored as the headless window's frame: so every check of
// the frames a window presents (gpuChecker.checkWindow: bit for bit a
// full frame of the GPU, within ADR-282 of the CPU rasterizer's) applies
// to the buffers, whose pixels outside the damage of their own last copy
// are as old as the buffer. The window also checks the damage it is
// given: outside it, the buffer must hold what the window showed before.
//
// The tests skip without Vulkan and on a device that exports no dmabufs.

// dmabufConfig describes the fake display server of a rig.
type dmabufConfig struct {
	// fourccs are the formats it accepts, with every modifier the device
	// exports them in and the implicit one; nil means XRGB8888 alone, as
	// an X server.
	fourccs []uint32
	// hold is the number of presented buffers it keeps busy: 1 releases a
	// buffer when the next one is presented (a Wayland compositor), 2 one
	// present later (an X server that flips). 0 means 1.
	hold int
}

var (
	dmabufWayland = dmabufConfig{fourccs: []uint32{platform.DRMFormatARGB8888, platform.DRMFormatXRGB8888, platform.DRMFormatABGR8888, platform.DRMFormatXBGR8888}, hold: 1}
	dmabufX11     = dmabufConfig{hold: 2}
)

// dmabufBackend is a headless backend whose windows present dmabufs.
type dmabufBackend struct {
	*headless.Backend
	t    *testing.T
	dev  *fakeDevice
	wins map[*headless.Window]*dmabufWindow
	// hold is dmabufConfig.hold; a test changes it. random, when set,
	// releases instead each busy buffer but the latest with a chance of
	// one in two at every present, and the others at the next.
	hold   int
	random *rand.Rand
	// feedback is what the windows report; none makes them report
	// nothing.
	feedback platform.DmabufFeedback
	none     bool
	// failCreate and failPresent, when set, are the errors of the next
	// CreateDmabuf and PresentDmabuf calls; drop makes PresentDmabuf drop
	// the frame, as for a hidden window.
	failCreate, failPresent error
	drop                    bool
}

func newDmabufBackend(t *testing.T, b *headless.Backend, dev *fakeDevice, cfg dmabufConfig) *dmabufBackend {
	d := &dmabufBackend{Backend: b, t: t, dev: dev, wins: map[*headless.Window]*dmabufWindow{}, hold: max(cfg.hold, 1)}
	d.setFormats(cfg.fourccs)
	return d
}

// setFormats makes the feedback list the formats with the device's
// modifiers, the device as the main one, under a new serial.
func (d *dmabufBackend) setFormats(fourccs []uint32) {
	if fourccs == nil {
		fourccs = []uint32{platform.DRMFormatXRGB8888}
	}
	fb := platform.DmabufFeedback{Serial: d.feedback.Serial + 1}
	if nodes := d.dev.DRMNodes(); len(nodes) > 0 {
		fb.HasDevice, fb.DeviceMajor, fb.DeviceMinor = true, nodes[0].Major, nodes[0].Minor
	}
	for _, fourcc := range fourccs {
		for _, f := range d.dev.ExportFormats() {
			if f.Fourcc == fourcc {
				fb.Formats = append(fb.Formats, platform.DmabufFormat{Fourcc: fourcc, Modifier: f.Modifier})
			}
		}
		fb.Formats = append(fb.Formats, platform.DmabufFormat{Fourcc: fourcc, Modifier: platform.DRMModifierInvalid})
	}
	d.feedback = fb
}

func (d *dmabufBackend) NewWindow(o platform.WindowOptions) (platform.Window, error) {
	if p, ok := o.Parent.(*dmabufWindow); ok {
		o.Parent = p.Window
	}
	w, err := d.Backend.NewWindow(o)
	if err != nil {
		return nil, err
	}
	dw := &dmabufWindow{Window: w.(*headless.Window), b: d, transparent: o.Transparent}
	d.wins[dw.Window] = dw
	return dw, nil
}

// wrapped returns the window the Host knows a headless window by.
func (d *dmabufBackend) wrapped(w *headless.Window) platform.Window {
	if dw := d.wins[w]; dw != nil {
		return dw
	}
	return w
}

// Drain delivers the headless backend's events to h under the wrapped
// windows.
func (d *dmabufBackend) Drain(h platform.Handler) int {
	return d.Backend.Drain(dmabufHandler{d, h})
}

type dmabufHandler struct {
	d *dmabufBackend
	h platform.Handler
}

func (m dmabufHandler) HandleEvent(w platform.Window, e platform.Event) {
	if hw, ok := w.(*headless.Window); ok {
		w = m.d.wrapped(hw)
	}
	m.h.HandleEvent(w, e)
}

// dmabufWindow is a headless window that presents dmabufs.
type dmabufWindow struct {
	*headless.Window
	b           *dmabufBackend
	transparent bool
	// bufs are the buffers registered and not destroyed; held the busy
	// ones, the oldest first.
	bufs, held []*dmabufBuffer
	// What the window was asked: buffers created and destroyed, presents
	// shown and dropped, presents of a framebuffer (Present), and the
	// damage of the last PresentDmabuf (nil: everything).
	created, destroyed, shown, dropped, copies int
	damage                                     []image.Rectangle
	// dmabuf: the frame the window shows came from a dmabuf.
	dmabuf bool
}

var _ platform.DmabufPresenter = (*dmabufWindow)(nil)

type dmabufBuffer struct {
	w    *dmabufWindow
	desc platform.DmabufDescriptor
	busy bool
	dead bool
}

func (b *dmabufBuffer) Size() image.Point { return b.desc.Size }
func (b *dmabufBuffer) Busy() bool        { return b.busy && !b.dead }

func (b *dmabufBuffer) Destroy() {
	if b.dead {
		return
	}
	b.dead = true
	w := b.w
	w.destroyed++
	w.bufs = slices.DeleteFunc(w.bufs, func(o *dmabufBuffer) bool { return o == b })
	w.held = slices.DeleteFunc(w.held, func(o *dmabufBuffer) bool { return o == b })
}

func (w *dmabufWindow) DmabufFeedback() (platform.DmabufFeedback, bool) {
	if w.b.none || w.transparent || w.Closed() {
		return platform.DmabufFeedback{}, false
	}
	return w.b.feedback, true
}

func (w *dmabufWindow) CreateDmabuf(d platform.DmabufDescriptor) (platform.DmabufBuffer, error) {
	if err := platform.CheckDmabuf(d); err != nil {
		return nil, err
	}
	if err := w.b.failCreate; err != nil {
		return nil, err
	}
	if fb, ok := w.DmabufFeedback(); !ok || !fb.Accepts(d.Fourcc, d.Modifier) || d.Alpha {
		return nil, fmt.Errorf("test: the format %v is not accepted: %w", platform.DmabufFormat{Fourcc: d.Fourcc, Modifier: d.Modifier}, platform.ErrUnsupported)
	}
	if w.b.dev.exports[d.Planes[0].FD] == nil {
		w.b.t.Errorf("CreateDmabuf with the descriptor %d, which is no exported image's", d.Planes[0].FD)
	}
	d.Planes = slices.Clone(d.Planes)
	b := &dmabufBuffer{w: w, desc: d}
	w.bufs = append(w.bufs, b)
	w.created++
	if len(w.bufs) > chainMax {
		w.b.t.Errorf("%d buffers registered with one window, more than a swapchain may have (%d)", len(w.bufs), chainMax)
	}
	return b, nil
}

// Present counts the frames presented from a framebuffer.
func (w *dmabufWindow) Present(fb *image.RGBA, damage []image.Rectangle) error {
	w.copies++
	w.dmabuf = false
	// A display server shows the framebuffer's frame from now on: what it
	// held of dmabufs is released.
	for _, b := range w.held {
		b.busy = false
	}
	w.held = nil
	return w.Window.Present(fb, damage)
}

func (w *dmabufWindow) PresentDmabuf(pb platform.DmabufBuffer, damage []image.Rectangle) error {
	t := w.b.t
	b, ok := pb.(*dmabufBuffer)
	switch {
	case !ok || b.w != w:
		panic("test: PresentDmabuf with a buffer of another window")
	case b.dead:
		panic("test: PresentDmabuf with a destroyed buffer")
	case b.busy:
		panic("test: PresentDmabuf with a busy buffer")
	case w.Closed():
		return nil
	case b.desc.Size != w.Metrics().Physical():
		panic(fmt.Sprintf("test: PresentDmabuf with a buffer of %v on a window of %v", b.desc.Size, w.Metrics().Physical()))
	}
	if err := w.b.failPresent; err != nil {
		return err
	}
	if !w.Visible() || w.b.drop {
		w.dropped++
		return nil // dropped: the buffer stays free
	}
	exp := w.b.dev.exports[b.desc.Planes[0].FD]
	if exp == nil {
		t.Fatalf("PresentDmabuf with a buffer whose exported image is closed")
	}
	img := image.NewRGBA(image.Rectangle{Max: b.desc.Size})
	if err := exp.Export.ReadPixels(img); err != nil {
		t.Fatalf("reading the presented buffer: %v", err)
	}
	// Outside the damage the buffer holds what the window showed.
	if prev := w.Frame(); len(damage) > 0 && w.dmabuf && prev != nil && prev.Rect == img.Rect {
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				if i := img.PixOffset(x, y); string(img.Pix[i:i+4]) != string(prev.Pix[i:i+4]) && !inRects(image.Pt(x, y), damage) {
					t.Fatalf("PresentDmabuf: pixel (%d, %d) changed from %v to %v outside the damage %v", x, y, prev.RGBAAt(x, y), img.RGBAAt(x, y), damage)
				}
			}
		}
	}
	// The whole buffer is the frame the window shows.
	if err := w.Window.Present(img, nil); err != nil {
		return err
	}
	w.dmabuf = true
	w.shown++
	w.damage = slices.Clone(damage)
	b.busy = true
	if r := w.b.random; r != nil {
		// The buffers before the latest are released in any order.
		w.held = slices.DeleteFunc(w.held, func(o *dmabufBuffer) bool {
			if r.Intn(2) == 0 {
				o.busy = false
				return true
			}
			return false
		})
		w.held = append(w.held, b)
		w.release(chainMax - 1) // a server never keeps every buffer
		return nil
	}
	w.held = append(w.held, b)
	for len(w.held) > w.b.hold {
		w.held[0].busy = false
		w.held = w.held[1:]
	}
	return nil
}

// release frees every buffer the fake display server holds but the n
// latest.
func (w *dmabufWindow) release(n int) {
	for len(w.held) > n {
		w.held[0].busy = false
		w.held = w.held[1:]
	}
}

func inRects(p image.Point, rects []image.Rectangle) bool {
	for _, r := range rects {
		if p.In(r) {
			return true
		}
	}
	return false
}

// newDirectRig returns a rig whose windows present dmabufs, or skips the
// test on a device that exports none.
func newDirectRig(t *testing.T, dev gpu.Device, opts Options, w, h float32, scale float64, cfg dmabufConfig) *gpuRig {
	t.Helper()
	if exp, ok := dev.(gpu.Exporter); !ok || len(exp.ExportFormats()) == 0 {
		t.Skipf("%s exports no dmabufs", dev.Name())
	}
	return newGPURigOn(t, dev, opts, w, h, scale, &cfg)
}

// dw returns the dmabuf window of the rig's main window.
func (x *gpuRig) dw() *dmabufWindow { return x.h.Window().(*dmabufWindow) }

// direct reports whether the main window's last frame stayed on the
// device.
func (x *gpuRig) direct() bool {
	s := x.h.main.gpu.surf
	return s != nil && s.Held() && s.Stats.Direct
}

// processFDs counts the open file descriptors of the process, -1 where
// they cannot be counted.
func processFDs() int {
	list, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(list)
}

// TestGPUDirectSession plays the session of TestGPUSession on windows
// presented without a copy, by a display server that releases a buffer
// when the next one is presented and takes every format (as Wayland), and
// by one that flips and takes XRGB8888 alone (as X11): hover, scrolls,
// edits, a dialog, resizes, a scale change, hiding and showing,
// minimizing, the theme, full frames and a rest with its trim. Every
// buffer presented holds the whole frame; no frame goes through the
// framebuffer; the buffers are as many as the server keeps busy plus one,
// and none is left, nor a file descriptor, when the host ends.
func TestGPUDirectSession(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		for _, c := range []struct {
			name  string
			cfg   dmabufConfig
			scale float64
			min   int // buffers at least
		}{
			{"wayland_x1", dmabufWayland, 1, 2},
			{"wayland_x1.5", dmabufWayland, 1.5, 2},
			{"x11_x2", dmabufX11, 2, 3},
		} {
			t.Run(c.name, func(t *testing.T) {
				fds := processFDs()
				st := &sessionState{}
				x := newDirectRig(t, dev, Options{Root: sessionPage(st)}, 640, 420, c.scale, c.cfg)
				dw := x.dw()
				most := 0
				x.h.AfterFrame = func() {
					most = max(most, len(dw.bufs))
					if n := x.h.main.gpu.chain.Buffers(); n != len(dw.bufs) || x.c.fake.exportsLive != n {
						t.Fatalf("the swapchain has %d buffers, the window %d, the device %d exported images", n, len(dw.bufs), x.c.fake.exportsLive)
					}
				}
				playGPUSession(t, x, st, c.scale)
				if dw.copies != 0 {
					t.Errorf("%d frames were presented from the framebuffer", dw.copies)
				}
				if dw.shown < 80 || most < c.min || most > c.min+1 {
					t.Errorf("%d presents with at most %d buffers at a time, want %d or %d", dw.shown, most, c.min, c.min+1)
				}
				if f := x.h.main.gpu.chain.bufs[0].export.Format().Fourcc; c.cfg.fourccs == nil && f != gpu.FourccXRGB8888 || c.cfg.fourccs != nil && f != gpu.FourccXRGB8888 && f != gpu.FourccXBGR8888 {
					t.Errorf("the buffers have the format %#x", f)
				}
				t.Logf("%d presents, %d buffers created, %d destroyed, at most %d at a time", dw.shown, dw.created, dw.destroyed, most)
				x.h.AfterFrame = nil
				x.c.finish(x.h)
				if len(dw.bufs) != 0 || dw.created != dw.destroyed {
					t.Errorf("%d buffers left registered (%d created, %d destroyed)", len(dw.bufs), dw.created, dw.destroyed)
				}
				if got := processFDs(); fds >= 0 && got > fds {
					t.Errorf("%d file descriptors open after the host ended, %d before it started", got, fds)
				}
			})
		}
	})
}

// counterPage is a page whose pixels a counter changes in two places, a
// row that hovers, and a draggable square of the counter's color.
func counterPage(set *Setter[int]) Node {
	return Component(func() Node {
		n, setN := UseState(0)
		*set = setN
		colors := []string{"bg-red-500", "bg-emerald-500", "bg-sky-500", "bg-amber-400"}
		return div(Class("relative h-screen bg-slate-100 p-4"),
			div(Key("title"), Class("rounded-lg bg-white p-2 text-base text-slate-900 shadow"), Textf("Frame %d", n)),
			div(Key("row"), Class("mt-2 h-8 rounded bg-slate-300 hover:bg-indigo-400")),
			div(Key("src"), Class("absolute left-[20px] top-[120px] size-[50px] "+colors[n%len(colors)]), Draggable(true),
				On("dragstart", func(e DragEvent) { e.DataTransfer.SetText("x") })),
			div(Key("far"), Class("absolute bottom-2 right-2 size-10 rounded-full "+colors[(n+1)%len(colors)])))
	})
}

// TestGPUDirectBusy presents to a display server that releases late, or
// not at all: buffers are registered while every one is busy, up to the
// most a swapchain has; then the frames are drawn and not presented,
// their damage kept and a frame asked for, until a buffer is free, and
// the present that follows shows the frame whole. Buffers are released in
// random orders; the ones a slower server needed go when no present uses
// them, and at rest.
func TestGPUDirectBusy(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		x := newDirectRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 2, dmabufConfig{hold: 1})
		w, dw, v := x.main(), x.dw(), x.h.main
		x.drain()
		x.check("mount")
		// The server stops releasing.
		x.d.hold = 100
		for i := 1; i < chainMax; i++ {
			set.Set(i)
			x.step(fmt.Sprintf("frame %d, nothing released", i))
			if len(dw.bufs) != i+1 || dw.shown != i+1 {
				t.Fatalf("frame %d: %d buffers and %d presents", i, len(dw.bufs), dw.shown)
			}
		}
		// Every buffer is busy: frames are drawn, none is presented.
		shown, presents := dw.shown, x.h.Presents()
		for i := 0; i < 3; i++ {
			set.Set(10 + i)
			x.b.Advance(x.b.FrameInterval())
			x.deliver()
			if dw.shown != shown || x.h.Presents() != presents || len(dw.bufs) != chainMax {
				t.Fatalf("with every buffer busy: %d presents (were %d), %d buffers", dw.shown, shown, len(dw.bufs))
			}
			if !v.gpu.owed || !x.b.Pending() && !v.tree.asked {
				// The host asked for the frame that tries again.
				if _, ok := x.b.NextFrameTime(); !ok {
					t.Fatalf("with every buffer busy: owed %v, and no frame is requested", v.gpu.owed)
				}
			}
		}
		if st := v.gpu.chain.Stats; st.Busy < 3 {
			t.Errorf("the swapchain counted %d busy presents", st.Busy)
		}
		// One buffer is released: the next frame presents everything that
		// changed meanwhile, although nothing changes now.
		dw.release(chainMax - 1)
		x.step("a buffer was released")
		if dw.shown != shown+1 || v.gpu.owed {
			t.Fatalf("after a release: %d presents (were %d), owed %v", dw.shown, shown, v.gpu.owed)
		}
		// Its damage is what changed since the window's last present:
		// the frames it was not given, not the whole window.
		if len(dw.damage) == 0 {
			t.Errorf("the present of the frames owed has the damage %v, want their rectangles", dw.damage)
		}
		// Random release orders, with hovers and counter changes.
		dw.release(2)
		x.d.random = rand.New(rand.NewSource(7))
		rx, ry := x.at(w, "row")
		for i := 0; i < 60; i++ {
			switch i % 3 {
			case 0:
				set.Set(100 + i)
			case 1:
				w.Inject(move(rx, ry))
			default:
				w.Inject(move(3, 3))
			}
			x.settle(fmt.Sprintf("random releases, step %d", i))
		}
		// A server that needs two: the extra buffers go after chainIdle
		// presents that did not use them.
		x.d.random, x.d.hold = nil, 1
		dw.release(1)
		for i := 0; i < chainIdle+chainMax+8; i++ {
			set.Set(200 + i)
			x.step("a frame")
		}
		if len(dw.bufs) != chainKeep {
			t.Errorf("after %d presents with one buffer busy, %d buffers are registered, want %d", chainIdle+chainMax+8, len(dw.bufs), chainKeep)
		}
		// A server that flips needs three; at rest the third goes.
		x.d.hold = 2
		for i := 0; i < 4; i++ {
			set.Set(400 + i)
			x.step("a frame of a server that flips")
		}
		if len(dw.bufs) != 3 {
			t.Errorf("a server that keeps two buffers busy has %d registered, want 3", len(dw.bufs))
		}
		for i := 0; i < 40 && len(dw.bufs) > chainKeep; i++ {
			x.b.Advance(RasterSettle / 4)
			x.deliver()
		}
		if len(dw.bufs) != chainKeep {
			t.Errorf("at rest %d buffers are registered, want %d", len(dw.bufs), chainKeep)
		}
		set.Set(500)
		x.settle("a frame after the rest")
		if dw.copies != 0 || x.log.Len() != 0 {
			t.Errorf("%d frames presented from the framebuffer; log %q", dw.copies, x.log)
		}
		x.c.finish(x.h)
		if len(dw.bufs) != 0 {
			t.Errorf("%d buffers left registered", len(dw.bufs))
		}
	})
}

// TestGPUDirectOwed checks the frame a backend asks for: a present the
// window dropped (it was hidden) is owed, and the next FrameEvent presents
// the whole frame although nothing changed; without a FrameEvent nothing
// runs.
func TestGPUDirectOwed(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		x := newDirectRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 1, dmabufWayland)
		w, dw, v := x.main(), x.dw(), x.h.main
		x.drain()
		x.check("mount")
		x.d.drop = true
		set.Set(1)
		x.b.Advance(x.b.FrameInterval())
		x.deliver()
		if dw.dropped != 1 || !v.gpu.owed || dw.shown != 1 {
			t.Fatalf("a dropped present: %d dropped, %d shown, owed %v", dw.dropped, dw.shown, v.gpu.owed)
		}
		// Nothing asks for a frame: the backend does when it shows the
		// window.
		frames := v.tree.Stats().Frames
		for i := 0; i < 3; i++ {
			x.b.Advance(x.b.FrameInterval())
			x.deliver()
		}
		if v.tree.Stats().Frames != frames || dw.dropped != 1 {
			t.Errorf("a dropped present asked for frames: %d frames ran, %d presents dropped", v.tree.Stats().Frames-frames, dw.dropped)
		}
		x.d.drop = false
		w.RequestFrame()
		x.step("the frame the backend asks for")
		if dw.shown != 2 || v.gpu.owed || dw.damage != nil {
			t.Errorf("the frame after a dropped present: %d shown, owed %v, damage %v (want everything)", dw.shown, v.gpu.owed, dw.damage)
		}
		// A frame with nothing new and nothing owed presents nothing.
		w.RequestFrame()
		x.step("a frame with nothing new")
		if dw.shown != 2 {
			t.Errorf("a frame with nothing new was presented (%d presents)", dw.shown)
		}
		if dw.copies != 0 || x.log.Len() != 0 {
			t.Errorf("%d frames presented from the framebuffer; log %q", dw.copies, x.log)
		}
		x.c.finish(x.h)
	})
}

// TestGPUDirectFallback checks the windows that are read back instead: a
// buffer the display server does not import, at the start and after a
// resize, a present that fails, a copy that fails, an image the device
// cannot create, each in the middle of a session, after which the window
// shows the frame at once and every later one, read back, with one line
// of the log; a lost device, which the CPU rasterizer replaces; and the
// windows that never try (another device, no common format, no feedback,
// TELA_PRESENT=copy), with or without a line.
func TestGPUDirectFallback(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		type rigState struct {
			x   *gpuRig
			set Setter[int]
		}
		open := func(t *testing.T, before func(x *gpuRig)) *rigState {
			r := &rigState{}
			r.x = newDirectRig(t, dev, Options{Root: counterPage(&r.set)}, 320, 240, 2, dmabufWayland)
			if before != nil {
				before(r.x)
			}
			r.x.drain()
			r.x.check("mount")
			return r
		}
		frames := func(r *rigState, what string, n int) {
			t.Helper()
			rx, ry := r.x.at(r.x.main(), "row")
			for i := 0; i < n; i++ {
				r.set.Update(func(v int) int { return v + 1 })
				r.x.step(what)
				r.x.main().Inject(move(rx, ry+float32(40*(i%2))))
				r.x.step(what + ", hover")
			}
		}
		// after checks a window that fell back: read back, the line in
		// the log, nothing left of the swapchain.
		after := func(r *rigState, line string, cpu bool) {
			t.Helper()
			x, dw := r.x, r.x.dw()
			copies := dw.copies
			frames(r, "after the failure", 3)
			if dw.copies < copies+3 || x.direct() || x.c.fake.exportsLive != 0 || len(dw.bufs) != 0 {
				t.Errorf("after the failure: %d frames from the framebuffer (were %d), direct %v, %d exported images, %d buffers", dw.copies, copies, x.direct(), x.c.fake.exportsLive, len(dw.bufs))
			}
			if n := strings.Count(x.log.String(), "\n"); n != 1 || !strings.Contains(x.log.String(), line) {
				t.Errorf("the log is %q, want one line with %q", x.log, line)
			}
			if cpu != (x.c.cpuFrames > 0) || cpu != (x.h.GPU() == nil) {
				t.Errorf("frames of the CPU rasterizer: %d, host GPU %v; want the CPU: %v", x.c.cpuFrames, x.h.GPU() != nil, cpu)
			}
			x.c.finish(x.h)
		}
		boom := errors.New("boom")

		t.Run("create_at_start", func(t *testing.T) {
			r := open(t, func(x *gpuRig) { x.d.failCreate = fmt.Errorf("the server imports nothing: %w", boom) })
			if dw := r.x.dw(); dw.copies != 1 || dw.shown != 0 {
				t.Errorf("the first frame: %d from the framebuffer, %d from a dmabuf", dw.copies, dw.shown)
			}
			after(r, "cannot be presented without a copy", false)
		})
		t.Run("strideless_plane", func(t *testing.T) {
			// A plane the driver reports without a stride: an error of
			// CreateDmabuf (platform.CheckDmabuf), not a panic.
			r := open(t, func(x *gpuRig) { x.c.fake.strideless = true })
			if dw := r.x.dw(); dw.copies != 1 || dw.shown != 0 {
				t.Errorf("the first frame: %d from the framebuffer, %d from a dmabuf", dw.copies, dw.shown)
			}
			after(r, "has no stride", false)
		})
		t.Run("create_after_resize", func(t *testing.T) {
			r := open(t, nil)
			frames(r, "direct", 3)
			r.x.d.failCreate = boom
			r.x.main().Resize(platform.Size{W: 360, H: 260})
			r.x.settle("resize")
			after(r, "cannot be presented without a copy", false)
		})
		t.Run("present", func(t *testing.T) {
			r := open(t, nil)
			frames(r, "direct", 3)
			if !r.x.direct() || r.x.dw().copies != 0 {
				t.Fatalf("before the failure: direct %v, %d frames from the framebuffer", r.x.direct(), r.x.dw().copies)
			}
			r.x.d.failPresent = boom
			after(r, "cannot be presented without a copy", false)
		})
		for _, op := range []string{"CopyFrom", "NewExport"} {
			t.Run(op, func(t *testing.T) {
				r := open(t, nil)
				frames(r, "direct", 3)
				r.x.c.fake.fail = func(o string) error {
					if o == op {
						return boom
					}
					return nil
				}
				if op == "NewExport" {
					r.x.main().Resize(platform.Size{W: 300, H: 200}) // new buffers
					r.x.settle("resize")
				}
				after(r, "cannot be presented without a copy", false)
			})
		}
		t.Run("device_lost", func(t *testing.T) {
			r := open(t, nil)
			frames(r, "direct", 3)
			r.x.c.fake.fail = func(o string) error {
				if o == "CopyFrom" {
					return fmt.Errorf("%w: test", gpu.ErrDeviceLost)
				}
				return nil
			}
			after(r, "the GPU device is lost", true)
		})
		// The windows that never try.
		quiet := func(r *rigState, line string) {
			t.Helper()
			x, dw := r.x, r.x.dw()
			frames(r, "read back", 3)
			if dw.shown != 0 || dw.created != 0 || x.direct() || dw.copies < 4 {
				t.Errorf("%d presents of dmabufs, %d buffers, direct %v, %d frames from the framebuffer", dw.shown, dw.created, x.direct(), dw.copies)
			}
			if got := strings.Count(x.log.String(), "\n"); line == "" && got != 0 || line != "" && (got != 1 || !strings.Contains(x.log.String(), line)) {
				t.Errorf("the log is %q, want one line with %q", x.log, line)
			}
			x.c.finish(x.h)
		}
		other := func(x *gpuRig) { x.d.feedback.DeviceMajor, x.d.feedback.DeviceMinor = 250, 99 }
		t.Run("another_device", func(t *testing.T) {
			quiet(open(t, other), "composites with another device (DRM node 250:99)")
		})
		t.Run("another_device_forced", func(t *testing.T) {
			t.Setenv(EnvPresent, "dmabuf")
			r := open(t, other)
			frames(r, "forced", 3)
			if dw := r.x.dw(); dw.copies != 0 || dw.shown < 4 || r.x.log.Len() != 0 {
				t.Errorf("TELA_PRESENT=dmabuf: %d frames from the framebuffer, %d from dmabufs; log %q", dw.copies, dw.shown, r.x.log)
			}
			r.x.c.finish(r.x.h)
		})
		t.Run("no_common_format", func(t *testing.T) {
			quiet(open(t, func(x *gpuRig) {
				x.d.feedback.Formats = []platform.DmabufFormat{{Fourcc: platform.DRMFormatXRGB8888, Modifier: 0x0bad000000000001}, {Fourcc: platform.DRMFormatXRGB8888, Modifier: platform.DRMModifierInvalid}}
			}), "exports no dmabuf in a format and a modifier the display server accepts")
		})
		t.Run("no_feedback", func(t *testing.T) {
			quiet(open(t, func(x *gpuRig) { x.d.none = true }), "")
		})
		t.Run("no_feedback_forced", func(t *testing.T) {
			t.Setenv(EnvPresent, "dmabuf")
			quiet(open(t, func(x *gpuRig) { x.d.none = true }), "the window reports no dmabuf feedback")
		})
		t.Run("copy_forced", func(t *testing.T) {
			t.Setenv(EnvPresent, "copy")
			r := &rigState{}
			r.x = newDirectRig(t, dev, Options{Root: counterPage(&r.set)}, 320, 240, 2, dmabufWayland)
			r.x.drain()
			r.x.check("mount")
			quiet(r, "TELA_PRESENT=copy")
		})
		t.Run("unknown_value", func(t *testing.T) {
			t.Setenv(EnvPresent, "shm")
			r := &rigState{}
			r.x = newDirectRig(t, dev, Options{Root: counterPage(&r.set)}, 320, 240, 2, dmabufWayland)
			r.x.drain()
			r.x.check("mount")
			if !r.x.direct() || !strings.Contains(r.x.log.String(), `TELA_PRESENT="shm"`) {
				t.Errorf("direct %v; log %q", r.x.direct(), r.x.log)
			}
			r.x.c.finish(r.x.h)
		})
	})
}

// TestGPUDirectFeedback changes what the display server accepts while the
// window is open: a new serial with the same content keeps the buffers,
// another format has them made again (none of the old ones is presented
// after), a feedback that goes away has the frames read back, and comes
// back.
func TestGPUDirectFeedback(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		x := newDirectRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 2, dmabufConfig{fourccs: []uint32{platform.DRMFormatXRGB8888}, hold: 1})
		dw, v := x.dw(), x.h.main
		x.drain()
		x.check("mount")
		n := 0
		frames := func(what string) {
			t.Helper()
			for i := 0; i < 3; i++ {
				n++
				set.Set(n)
				x.step(what)
			}
		}
		format := func() uint32 { return v.gpu.chain.bufs[0].export.Format().Fourcc }
		frames("XRGB")
		if format() != gpu.FourccXRGB8888 || dw.created != 2 {
			t.Fatalf("the buffers have the format %#x; %d created", format(), dw.created)
		}
		x.d.feedback.Serial++ // the same content
		frames("a new serial")
		if dw.created != 2 || dw.destroyed != 0 {
			t.Errorf("a new serial with the same formats: %d buffers created, %d destroyed", dw.created, dw.destroyed)
		}
		x.d.setFormats([]uint32{platform.DRMFormatXBGR8888})
		frames("XBGR")
		if format() != gpu.FourccXBGR8888 || dw.created != 4 || dw.destroyed != 2 {
			t.Errorf("after the format changed: buffers of %#x, %d created, %d destroyed", format(), dw.created, dw.destroyed)
		}
		x.d.none = true
		frames("no feedback")
		if x.direct() || len(dw.bufs) != 0 || dw.copies != 3 {
			t.Errorf("without feedback: direct %v, %d buffers, %d frames from the framebuffer", x.direct(), len(dw.bufs), dw.copies)
		}
		x.d.none = false
		frames("feedback again")
		if !x.direct() || len(dw.bufs) != 2 || dw.copies != 3 {
			t.Errorf("with feedback again: direct %v, %d buffers, %d frames from the framebuffer", x.direct(), len(dw.bufs), dw.copies)
		}
		if x.log.Len() != 0 {
			t.Errorf("the host logged: %s", x.log)
		}
		x.c.finish(x.h)
	})
}

// TestGPUDirectReaders checks what reads a window's framebuffer while its
// frames are presented without a copy: FramePixels and a drag image read
// the frame back when they are asked, and the debug overlays, which the
// CPU rasterizer draws over the frame read back, have the window read
// back while they are on and presented without a copy again after.
func TestGPUDirectReaders(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		t.Run("pixels_and_drag", func(t *testing.T) {
			var set Setter[int]
			x := newDirectRig(t, dev, Options{Root: counterPage(&set)}, 320, 240, 2, dmabufWayland)
			w, dw, v := x.main(), x.dw(), x.h.main
			x.drain()
			x.check("mount")
			same := func(what string) {
				t.Helper()
				if !v.gpu.surf.Behind() {
					t.Fatalf("%s: the framebuffer is not behind a direct frame", what)
				}
				got, want := x.h.FramePixels(dw), w.Frame()
				if p, ok := firstDiff(got, want); !ok {
					t.Fatalf("%s: FramePixels differs from the window at %v: %v, the window shows %v", what, p, got.RGBAAt(p.X, p.Y), want.RGBAAt(p.X, p.Y))
				}
				if v.gpu.surf.Behind() {
					t.Errorf("%s: the framebuffer is still behind after FramePixels", what)
				}
			}
			same("the first frame")
			set.Set(1) // a partial frame: the square turns emerald
			x.step("a partial frame")
			if d := v.tree.LastDamage(); d.Full || d.Empty() {
				t.Fatalf("the frame's damage is %+v", d)
			}
			same("after a partial frame")
			set.Set(2) // sky
			x.step("another frame")
			// The drag image is cut from the frame the window shows.
			w.Inject(move(30, 130))
			w.Inject(down(30, 130))
			x.drain()
			w.Inject(moveHeld(44, 130))
			x.drain()
			info, ok := x.b.Drag()
			if !ok || info.Image == nil {
				t.Fatalf("no drag with an image (%v)", ok)
			}
			if c := info.Image.RGBAAt(info.Image.Rect.Min.X+50, info.Image.Rect.Min.Y+50); c.A != 255 || c.B < 200 || c.R > 60 {
				t.Errorf("the drag image's center is %v, want the square's sky blue", c)
			}
			if dw.copies != 0 || x.log.Len() != 0 {
				t.Errorf("%d frames presented from the framebuffer; log %q", dw.copies, x.log)
			}
			x.c.finish(x.h)
		})
		t.Run("debug_overlays", func(t *testing.T) {
			st := &sessionState{}
			x := newDirectRig(t, dev, Options{Config: Config{Debug: DebugPaintFlash | DebugLayers}, Root: sessionPage(st)}, 640, 420, 1.5, dmabufWayland)
			dw := x.dw()
			copies, shown := 0, 0
			playGPUDebug(t, x, st, func(f DebugFlags) {
				// Seven frames ran with these flags.
				switch {
				case f != 0 && (dw.shown != shown || dw.copies < copies+5 || len(dw.bufs) != 0 || x.direct()):
					t.Errorf("debug %s: %d presents of dmabufs (were %d), %d from the framebuffer (were %d), %d buffers, direct %v", f, dw.shown, shown, dw.copies, copies, len(dw.bufs), x.direct())
				case f == 0 && (dw.copies != copies || dw.shown < shown+5 || !x.direct()):
					t.Errorf("overlays off: %d presents from the framebuffer (were %d), %d of dmabufs (were %d), direct %v", dw.copies, copies, dw.shown, shown, x.direct())
				}
				copies, shown = dw.copies, dw.shown
			})
			if x.log.Len() != 0 {
				t.Errorf("the host logged: %s", x.log)
			}
		})
	})
}

// TestGPUDirectChildWindow presents two windows of one app without a
// copy, each with its buffers, and closes one: its buffers go with it.
func TestGPUDirectChildWindow(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		var setOpen Setter[bool]
		inner := div(Key("c"), Class("h-screen bg-emerald-200 p-2 hover:bg-emerald-400"), Text("child"))
		root := Component(func() Node {
			open, so := UseState(true)
			setOpen = so
			c := []Child{Class("h-screen bg-slate-200 p-3"), counterPage(&set)}
			if open {
				c = append(c, WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "child", Width: 200, Height: 120}, Root: inner}), "child"))
			}
			return div(c...)
		})
		x := newDirectRig(t, dev, Options{Root: root}, 320, 240, 2, dmabufX11)
		x.drain()
		x.settle("mount")
		if len(x.d.wins) != 2 {
			t.Fatalf("the page opened %d windows", len(x.d.wins))
		}
		var child *dmabufWindow
		for _, dw := range x.d.wins {
			if dw != x.dw() {
				child = dw
			}
		}
		cx, cy := x.at(child.Window, "c")
		for i := 0; i < 4; i++ {
			set.Set(i + 1)
			child.Inject(move(cx, cy+float32(200*(i%2))))
			x.settle("both windows change")
		}
		if x.dw().shown < 4 || child.shown < 3 || x.dw().copies+child.copies != 0 {
			t.Errorf("presents of dmabufs: %d and %d; from the framebuffer: %d and %d", x.dw().shown, child.shown, x.dw().copies, child.copies)
		}
		live := x.c.fake.exportsLive
		setOpen.Set(false)
		x.settle("the child closes")
		if len(child.bufs) != 0 || x.c.fake.exportsLive >= live || x.c.fake.exportsLive != len(x.dw().bufs) {
			t.Errorf("after the child closed: %d of its buffers registered, %d exported images alive (were %d)", len(child.bufs), x.c.fake.exportsLive, live)
		}
		set.Set(9)
		x.settle("the main window goes on")
		if x.log.Len() != 0 {
			t.Errorf("the host logged: %s", x.log)
		}
		x.c.finish(x.h)
	})
}

// TestGPUDirectDeviceLostElsewhere loses the device in the frame of one
// of two windows presented without a copy: the other window's framebuffer
// was left behind, and nothing can read its frame back any more, so its
// pixels are not given out (FramePixels) until the CPU rasterizer, asked
// for the frame, has drawn it.
func TestGPUDirectDeviceLostElsewhere(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var set Setter[int]
		inner := div(Key("c"), Class("h-screen bg-emerald-200 p-2"), Text("child"))
		root := Component(func() Node {
			return div(Class("h-screen bg-slate-200 p-3"), counterPage(&set),
				WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "child", Width: 200, Height: 120}, Root: inner}), "child"))
		})
		x := newDirectRig(t, dev, Options{Root: root}, 320, 240, 2, dmabufWayland)
		x.drain()
		x.settle("mount")
		var child *dmabufWindow
		for _, dw := range x.d.wins {
			if dw != x.dw() {
				child = dw
			}
		}
		if child == nil || child.shown == 0 || x.h.FramePixels(child) == nil {
			t.Fatalf("the child window was not presented without a copy")
		}
		set.Set(1)
		x.settle("both direct") // the child's framebuffer was read for FramePixels, the main window's never
		cv := x.h.viewOf(child)
		cv.gpu.surf.behind = true // as after any frame of the child
		x.c.fake.fail = func(o string) error {
			if o == "CopyFrom" {
				return fmt.Errorf("%w: test", gpu.ErrDeviceLost)
			}
			return nil
		}
		set.Set(2)
		x.b.Advance(x.b.FrameInterval())
		// The main window's frame loses the device; the child has had no
		// frame since.
		x.d.Drain(dmabufOnly{x.h, x.dw()})
		if x.h.GPU() != nil {
			t.Fatalf("the host still has a GPU (log %q)", x.log)
		}
		if px := x.h.FramePixels(child); px != nil {
			t.Errorf("the child's framebuffer, left behind, was given out after the device was lost")
		}
		// The frame the host asked for when the child lost the GPU was
		// held back with the child's other events: here it comes.
		if !cv.stale {
			t.Error("no frame of the CPU rasterizer was asked for the child")
		}
		child.RequestFrame()
		x.settle("the CPU rasterizer draws both windows")
		got, want := x.h.FramePixels(child), child.Frame()
		if got == nil {
			t.Fatal("no pixels of the child after its frame by the CPU rasterizer")
		}
		if p, ok := firstDiff(got, want); !ok {
			t.Errorf("the child's framebuffer differs from what it shows at %v", p)
		}
		if child.copies == 0 || x.c.cpuFrames == 0 {
			t.Errorf("%d frames of the child from the framebuffer, %d frames of the CPU rasterizer checked", child.copies, x.c.cpuFrames)
		}
		x.c.finish(x.h)
	})
}

// dmabufOnly delivers to a Host the events of one window, and those of
// none: the frames of the other windows wait.
type dmabufOnly struct {
	h   *Host
	win platform.Window
}

func (o dmabufOnly) HandleEvent(w platform.Window, e platform.Event) {
	if w == nil || w == o.win {
		o.h.HandleEvent(w, e)
	}
}
