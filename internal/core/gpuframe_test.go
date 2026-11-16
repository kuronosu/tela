//go:build telagpu && (linux || windows)

package core

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/gfx/gpu/gputest"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/vk"
)

// The frames a Host draws with the GPU (gpuhost.go, gpuframe.go). On every
// physical device, each frame a window presents is compared with
//
//   - a full render of the same frame by the GPU, which it must equal bit
//     for bit: the damage, the shifts and the readback bring the
//     framebuffer exactly where a full frame brings it (the renderer's
//     regions are bit-exact, internal/gfx/gpu's TestRegion);
//   - a full render of the same frame by the CPU rasterizer, within the
//     tolerance of ADR-282 (gfxtest.GPULevels, GPUShare, GPUHard).
//
// The tests skip without Vulkan. Under TELA_VK_VALIDATE=1 anything the
// validation layer reports fails them (gputest.EachDevice).

// fakeDevice wraps a device: it counts the targets alive, and fails the
// calls a test chooses.
type fakeDevice struct {
	gpu.Device
	// fail, when set, is asked before every NewTarget, Draw, ReadPixels
	// and Shift of the Host's frames: the error it returns fails the call.
	fail func(op string) error
	// bypass: the calls are the checker's own (its reference frames).
	bypass bool
	// live counts the targets created and not closed, made those created.
	live, made int
	// max, when positive, is the largest target size the device reports.
	max int
	// exports are the images exported alive, by the descriptor of their
	// dmabuf; exportsLive counts them, exportsMade those created.
	exports                  map[int]*fakeExport
	exportsLive, exportsMade int
}

// fakeDevice is an Exporter when its device is one: it counts the images
// exported, and fails NewExport and CopyFrom when a test says so.
var _ gpu.Exporter = (*fakeDevice)(nil)

func (d *fakeDevice) exporter() gpu.Exporter {
	exp, _ := d.Device.(gpu.Exporter)
	return exp
}

func (d *fakeDevice) DRMNodes() []gpu.DRMNode {
	if exp := d.exporter(); exp != nil {
		return exp.DRMNodes()
	}
	return nil
}

func (d *fakeDevice) ExportFormats() []gpu.DmabufFormat {
	if exp := d.exporter(); exp != nil {
		return exp.ExportFormats()
	}
	return nil
}

func (d *fakeDevice) NewExport(w, h int, accepted []gpu.DmabufFormat) (gpu.Export, error) {
	exp := d.exporter()
	if exp == nil {
		return nil, gpu.ErrNoExport
	}
	if err := d.failing("NewExport"); err != nil {
		return nil, err
	}
	e, err := exp.NewExport(w, h, accepted)
	if err != nil {
		return nil, err
	}
	fe := &fakeExport{Export: e, d: d, fd: e.Planes()[0].FD}
	if d.exports == nil {
		d.exports = map[int]*fakeExport{}
	}
	d.exports[fe.fd] = fe
	d.exportsLive++
	d.exportsMade++
	return fe, nil
}

// fakeExport is an image exported by a fakeDevice.
type fakeExport struct {
	gpu.Export
	d      *fakeDevice
	fd     int
	closed bool
}

func (e *fakeExport) CopyFrom(t gpu.Target, rects ...image.Rectangle) error {
	if err := e.d.failing("CopyFrom"); err != nil {
		return err
	}
	if ft, ok := t.(*fakeTarget); ok {
		t = ft.Target
	}
	return e.Export.CopyFrom(t, rects...)
}

func (e *fakeExport) Close() {
	if !e.closed {
		e.closed = true
		e.d.exportsLive--
		delete(e.d.exports, e.fd)
	}
	e.Export.Close()
}

func (d *fakeDevice) failing(op string) error {
	if d.fail == nil || d.bypass {
		return nil
	}
	return d.fail(op)
}

func (d *fakeDevice) MaxTargetSize() int {
	if d.max > 0 {
		return d.max
	}
	return d.Device.MaxTargetSize()
}

func (d *fakeDevice) NewTarget(w, h int) (gpu.Target, error) {
	if err := d.failing("NewTarget"); err != nil {
		return nil, err
	}
	if d.max > 0 && (w > d.max || h > d.max) {
		return nil, fmt.Errorf("a target of %dx%d: the size must be between 1 and %d", w, h, d.max)
	}
	t, err := d.Device.NewTarget(w, h)
	if err != nil {
		return nil, err
	}
	d.live++
	d.made++
	return &fakeTarget{Target: t, d: d}, nil
}

type fakeTarget struct {
	gpu.Target
	d      *fakeDevice
	closed bool
}

func (t *fakeTarget) Device() gpu.Device { return t.d }

func (t *fakeTarget) Draw(f *gpu.Frame) error {
	if err := t.d.failing("Draw"); err != nil {
		return err
	}
	return t.Target.Draw(f)
}

func (t *fakeTarget) ReadPixels(dst *image.RGBA, rects ...image.Rectangle) error {
	if err := t.d.failing("ReadPixels"); err != nil {
		return err
	}
	return t.Target.ReadPixels(dst, rects...)
}

func (t *fakeTarget) Shift(r image.Rectangle, dx, dy int) error {
	if err := t.d.failing("Shift"); err != nil {
		return err
	}
	return t.Target.Shift(r, dx, dy)
}

func (t *fakeTarget) Close() {
	if !t.closed {
		t.closed = true
		t.d.live--
	}
	t.Target.Close()
}

// gpuChecker checks the frames of the windows a GPU draws.
type gpuChecker struct {
	t    *testing.T
	fake *fakeDevice
	g    *GPU
	// ref is the target of the checker's full frames.
	ref gpu.Target
	// What the frames checked were: drawn by the GPU or by the CPU
	// rasterizer; full, with shifts, or of damaged rectangles.
	gpuFrames, cpuFrames   int
	full, shifted, partial int
	// worst is the largest difference from the CPU rasterizer, over2 the
	// largest share of pixels over 2 levels.
	worst gfxtest.Diff
	over2 float64
	seen  map[*Tree]*frame
	// pw maps a headless window to the window the Host knows, when the
	// test wraps its windows (gpuchain_test.go); nil when they are the
	// same.
	pw func(*headless.Window) platform.Window
}

// win returns the window the Host knows w by.
func (c *gpuChecker) win(w *headless.Window) platform.Window {
	if c.pw != nil {
		return c.pw(w)
	}
	return w
}

func newGPUChecker(t *testing.T, dev gpu.Device) *gpuChecker {
	c := &gpuChecker{t: t, fake: &fakeDevice{Device: dev}, seen: map[*Tree]*frame{}}
	c.g = NewGPU(c.fake)
	return c
}

// fullFrame renders f in full with the GPU, into a target of its own.
func (c *gpuChecker) fullFrame(f *frame, size image.Point, canvas gfx.Color) *image.RGBA {
	c.t.Helper()
	c.fake.bypass = true
	defer func() { c.fake.bypass = false }()
	if c.ref == nil || c.ref.Size() != size {
		c.closeRef()
		ref, err := c.fake.NewTarget(size.X, size.Y)
		if err != nil {
			c.t.Fatal(err)
		}
		c.ref = ref
	}
	res, err := c.g.r.Render(c.ref, f.dl, gpu.Options{Scale: f.scale, Clear: &canvas})
	if err != nil {
		c.t.Fatal(err)
	}
	if len(res.Unsupported) > 0 {
		c.t.Fatalf("the reference frame has commands that are not drawn: %v", res.Unsupported)
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := c.ref.ReadPixels(img, img.Rect); err != nil {
		c.t.Fatal(err)
	}
	return img
}

func (c *gpuChecker) closeRef() {
	if c.ref != nil {
		c.ref.Close()
		c.ref = nil
	}
}

// cpuFrame renders f in full with the CPU rasterizer.
func cpuFrame(f *frame, size image.Point, canvas gfx.Color) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: size})
	raster.Clear(img, image.Rectangle{}, canvas)
	raster.Render(img, f.dl, raster.Options{Scale: f.scale})
	return img
}

// checkWindow compares what window w shows with full renders of the frame
// its tree rasterized last (see the top of the file). A frame the CPU
// rasterizer drew (a window that fell back) must equal the CPU's full
// render.
func (c *gpuChecker) checkWindow(h *Host, w *headless.Window, what string) {
	t := c.t
	t.Helper()
	v := h.viewOf(c.win(w))
	if v == nil {
		t.Fatalf("%s: the window %q is not the host's", what, w.Title())
	}
	tr := v.tree
	f := tr.lay.ras
	got := w.Frame()
	if f == nil || got == nil {
		t.Fatalf("%s: window %q: nothing rasterized (%v) or presented (%v)", what, w.Title(), f != nil, got != nil)
	}
	// A window presented without a copy leaves its framebuffer behind
	// (TestGPUDirectReaders reads it).
	if fb := v.fb; fb != nil && fb.Rect == got.Rect && !(v.gpu.surf != nil && v.gpu.surf.Behind()) {
		// What drag images are cut from, and what the next frame's damage
		// is drawn over.
		if p, ok := firstDiff(fb, got); !ok {
			t.Fatalf("%s: window %q: the framebuffer differs from the frame presented at %v: %v, presented %v (damage %+v)",
				what, w.Title(), p, fb.RGBAAt(p.X, p.Y), got.RGBAAt(p.X, p.Y), tr.LastDamage())
		}
	}
	if c.seen[tr] != f {
		c.seen[tr] = f
		switch d := tr.LastDamage(); {
		case d.Full:
			c.full++
		case len(d.Shifts) > 0:
			c.shifted++
		case len(d.Rects) > 0:
			c.partial++
		}
	}
	size, canvas := got.Rect.Size(), tr.clearColor()
	cpu := cpuFrame(f, size, canvas)
	overlay := func(img *image.RGBA) {
		if tr.Debug() != 0 && tr.dbg.prev != nil {
			// The window holds the application and the overlays the CPU
			// rasterizer drew over it (debug.go).
			var r raster.Renderer
			r.Render(img, tr.dbg.prev, raster.Options{Scale: tr.dbg.prevScale})
		}
	}
	fail := func(want *image.RGBA, p image.Point, how string) {
		t.Helper()
		dir := os.Getenv("TELA_DAMAGE_DIR")
		if dir == "" {
			dir = t.TempDir()
		}
		save(filepath.Join(dir, "got.png"), got)
		save(filepath.Join(dir, "want.png"), want)
		t.Fatalf("%s: window %q: %s: pixel %v is %v, want %v\nlast damage %+v\nimages in %s",
			what, w.Title(), how, p, got.RGBAAt(p.X, p.Y), want.RGBAAt(p.X, p.Y), tr.LastDamage(), dir)
	}
	if !tr.lay.rasGPU {
		c.cpuFrames++
		overlay(cpu)
		if p, ok := firstDiff(got, cpu); !ok {
			fail(cpu, p, "a frame of the CPU rasterizer differs from its full render")
		}
		return
	}
	c.gpuFrames++
	full := c.fullFrame(f, size, canvas)
	overlay(full)
	overlay(cpu)
	if p, ok := firstDiff(got, full); !ok {
		fail(full, p, "the frame differs from a full frame of the GPU")
	}
	m := gfxtest.Compare(got, cpu)
	if m.Max > c.worst.Max {
		c.worst = m
	}
	c.over2 = max(c.over2, m.Over2)
	if !m.WithinGPU() {
		dir := os.Getenv("TELA_DAMAGE_DIR")
		if dir == "" {
			dir = t.TempDir()
		}
		save(filepath.Join(dir, "gpu.png"), got)
		save(filepath.Join(dir, "cpu.png"), cpu)
		t.Errorf("%s: window %q: against the CPU rasterizer the largest difference is %d at %v (limit %d) and %.3f%% of the pixels differ by more than %d levels (limit %.3f%%); images in %s",
			what, w.Title(), m.Max, m.Worst, gfxtest.GPUHard, 100*m.Over2, gfxtest.GPULevels, 100*gfxtest.GPUShare, dir)
	}
}

// finish ends the host and checks that no target is left: the windows'
// are released when they close, and the GPU's Renderer gives up what it
// keeps on the device (the validation layer reports the rest when the
// device closes).
func (c *gpuChecker) finish(h *Host) {
	c.t.Helper()
	c.closeRef()
	h.stopTimer()
	h.end()
	if c.fake.live != 0 {
		c.t.Errorf("%d of the %d targets created are left alive after the host ended", c.fake.live, c.fake.made)
	}
	if c.fake.exportsLive != 0 {
		c.t.Errorf("%d of the %d exported images created are left alive after the host ended", c.fake.exportsLive, c.fake.exportsMade)
	}
	if h.GPU() != nil {
		c.t.Error("the host still has a GPU after it ended")
	}
}

// live returns the number of targets alive that are windows' (not the
// checker's own).
func (c *gpuChecker) live() int {
	if c.ref != nil {
		return c.fake.live - 1
	}
	return c.fake.live
}

func (c *gpuChecker) summary() string {
	return fmt.Sprintf("%d GPU frames (%d full, %d shifted, %d partial), %d CPU frames; against the CPU rasterizer at most %d levels, at most %.3f%% of a frame over 2",
		c.gpuFrames, c.full, c.shifted, c.partial, c.cpuFrames, c.worst.Max, 100*c.over2)
}

// gpuRig is an app on the headless backend whose windows a GPU draws.
type gpuRig struct {
	t   *testing.T
	c   *gpuChecker
	b   *headless.Backend
	h   *Host
	log *bytes.Buffer // the host's error log
	// d, when not nil, is the backend whose windows present dmabufs
	// (gpuchain_test.go): the Host's windows wrap the headless ones.
	d *dmabufBackend
}

func newGPURig(t *testing.T, dev gpu.Device, opts Options, w, h float32, scale float64) *gpuRig {
	return newGPURigOn(t, dev, opts, w, h, scale, nil)
}

// newGPURigOn is newGPURig with windows that present dmabufs as dm says,
// when it is not nil.
func newGPURigOn(t *testing.T, dev gpu.Device, opts Options, w, h float32, scale float64, dm *dmabufConfig) *gpuRig {
	t.Helper()
	x := &gpuRig{t: t, c: newGPUChecker(t, dev), log: &bytes.Buffer{}}
	x.b = headless.New(headless.Options{Size: platform.Size{W: w, H: h}, Scale: scale})
	var pb platform.Backend = x.b
	if dm != nil {
		x.d = newDmabufBackend(t, x.b, x.c.fake, *dm)
		x.c.pw = x.d.wrapped
		pb = x.d
	}
	opts.Fonts, opts.Flavor, opts.ErrorLog = testFonts, testFlavor, x.log
	host, err := NewHost(pb, opts)
	if err != nil {
		t.Fatal(err)
	}
	x.h = host
	t.Cleanup(host.stopTimer)
	host.UseGPU(x.c.g)
	return x
}

func (x *gpuRig) main() *headless.Window {
	if x.d != nil {
		return x.h.Window().(*dmabufWindow).Window
	}
	return x.h.Window().(*headless.Window)
}

// pw returns the window the Host knows the headless window w by.
func (x *gpuRig) pw(w *headless.Window) platform.Window { return x.c.win(w) }

// drain delivers what is queued to the Host.
func (x *gpuRig) drain() {
	if x.d != nil {
		x.d.Drain(x.h)
		return
	}
	x.b.Drain(x.h)
}

// deliver handles what is queued and the timer deadlines that are due.
func (x *gpuRig) deliver() {
	if at, ok := x.h.TimerArmed(); ok && at <= x.b.Now() {
		x.b.Wake()
	}
	x.drain()
}

// step advances the clock by a frame, delivers, and checks every window.
func (x *gpuRig) step(what string) {
	x.t.Helper()
	x.b.Advance(x.b.FrameInterval())
	x.deliver()
	x.check(what)
}

func (x *gpuRig) check(what string) {
	x.t.Helper()
	for _, w := range x.b.Windows() {
		if x.h.viewOf(x.pw(w)) != nil && w.Frame() != nil {
			x.c.checkWindow(x.h, w, what)
		}
	}
}

// settle steps until no window has work left (animations, transitions).
func (x *gpuRig) settle(what string) {
	x.t.Helper()
	for i := 0; i < 120; i++ {
		x.step(fmt.Sprintf("%s (frame %d)", what, i))
		busy := false
		for _, v := range x.h.views {
			if !v.paused() && (v.tree.Dirty() || v.tree.Animating() || v.tree.asked) {
				busy = true
			}
		}
		if !busy {
			return
		}
	}
	x.t.Fatalf("%s: the windows did not come to rest", what)
}

// at returns the center of the element with key k in window w.
func (x *gpuRig) at(w *headless.Window, k any) (float32, float32) {
	x.t.Helper()
	tr := x.h.TreeOf(x.pw(w))
	el := keyed(tr, k)
	if el == nil {
		x.t.Fatalf("no element with the key %v", k)
	}
	c := tr.elemBounds(el).Center()
	return c.X, c.Y
}

func (x *gpuRig) click(w *headless.Window, px, py float32) {
	w.Inject(move(px, py))
	w.Inject(down(px, py))
	w.Inject(up(px, py))
	x.drain()
}

func (x *gpuRig) typeText(w *headless.Window, s string) {
	for _, r := range s {
		k := platform.Key(string(r))
		w.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeUnidentified, Key: k, Text: string(r)})
		w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeUnidentified, Key: k})
	}
	x.drain()
}

// view returns the host's view of window w.
func (x *gpuRig) view(w *headless.Window) *winView {
	x.t.Helper()
	v := x.h.viewOf(x.pw(w))
	if v == nil {
		x.t.Fatalf("the window %q is not the host's", w.Title())
	}
	return v
}

// held reports whether window w has a target on the GPU.
func (x *gpuRig) held(w *headless.Window) bool {
	s := x.view(w).gpu.surf
	return s != nil && s.Held()
}

// sessionState holds the handles of sessionPage.
type sessionState struct {
	list, glass ElementRef
	setDialog   Setter[int]
	setRow      Setter[int]
}

// sessionPage is a page with what an app's frames are made of: a column
// with a title, a text field, a list that scrolls over an opaque
// background (its pixels are shifted) with rows that hover, and a button;
// a glass pane (a backdrop blur over a gradient and a disc) whose
// translucent rows scroll over the blur; and, when its state says so, a
// scrim and a glass dialog over all of it. Every color has a dark variant.
func sessionPage(st *sessionState) Node {
	return Component(func() Node {
		st.list, st.glass = UseElementRef(), UseElementRef()
		dlg, setDlg := UseState(0)
		st.setDialog = setDlg
		n, setN := UseState(0)
		st.setRow = setN
		rows := []Child{AttachRef(st.list), Key("list"), Class("h-40 shrink-0 overflow-y-auto rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900")}
		for i := range 40 {
			label := fmt.Sprintf("Row %d", i)
			if i == 3 && n%2 == 1 {
				label = "Row 3 changed"
			}
			rows = append(rows, div(Key(fmt.Sprintf("row%d", i)), Class("flex h-8 items-center gap-2 px-3 text-sm text-slate-700 hover:bg-sky-100 dark:text-slate-200 dark:hover:bg-slate-700"),
				div(Class("size-3 rounded-full bg-emerald-500")), Text(label)))
		}
		cards := []Child{AttachRef(st.glass), Key("glass"), Class("h-full overflow-y-auto p-3")}
		for i := range 20 {
			cards = append(cards, div(Key(i), Class("mb-2 flex h-10 items-center rounded-lg border border-white/30 bg-white/10 px-3 text-sm text-white shadow-md"), Textf("Order %d", 1000+i)))
		}
		page := []Child{Class("relative flex h-screen gap-4 overflow-hidden bg-slate-200 p-4 dark:bg-slate-950"),
			div(Class("flex w-56 shrink-0 flex-col gap-3"),
				div(Class("rounded-xl bg-white p-3 text-lg font-semibold text-slate-900 shadow-lg dark:bg-slate-800 dark:text-white"), Text("Session")),
				input(Key("in"), Class("h-9 shrink-0 rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-900 dark:border-slate-600 dark:bg-slate-800 dark:text-white"), Placeholder("Search")),
				div(rows...),
				button(Key("open"), Class("h-9 shrink-0 rounded-lg bg-indigo-600 text-sm text-white transition hover:bg-indigo-500 active:scale-95"), Text("Open")),
			),
			div(Class("relative grow overflow-hidden rounded-2xl bg-linear-to-br from-indigo-500 to-rose-500 dark:from-slate-800 dark:to-indigo-900"),
				div(Class("absolute -top-6 left-10 size-40 rounded-full bg-amber-300")),
				div(Class("absolute inset-3 rounded-2xl border border-white/40 bg-white/20 backdrop-blur-xl"), div(cards...))),
		}
		if dlg > 0 {
			page = append(page,
				div(Key("scrim"), Class("absolute inset-0 bg-black/40")),
				div(Key("dialog"), Class("absolute top-1/2 left-1/2 w-72 -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-white/50 bg-white/60 p-5 shadow-2xl backdrop-blur-md"),
					div(Class("text-base font-semibold text-slate-900"), Text("Report")),
					p(Class("mt-2 text-sm text-slate-700"), Textf("Step %d of the dialog over the glass.", dlg))))
		}
		return div(page...)
	})
}

// TestGPUSession plays a session on a page drawn by the GPU, with
// incremental damage, at three scales: mount, hover, wheel and
// programmatic scrolling, text edits, a dialog over glass, a resize, a
// scale change, hiding and showing, minimizing, a theme switch, full
// frames, and a rest long enough for the renderer's memory to be trimmed.
// Every frame presented is checked (checkWindow).
func TestGPUSession(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		for _, scale := range []float64{1, 1.5, 2} {
			t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
				st := &sessionState{}
				x := newGPURig(t, dev, Options{Root: sessionPage(st)}, 640, 420, scale)
				playGPUSession(t, x, st, scale)
				x.c.finish(x.h)
			})
		}
	})
}

// playGPUSession plays the session of TestGPUSession on a rig whose root
// is sessionPage(st), and checks every frame. direct, when not nil, runs
// at the points of the session where a window presented without a copy
// has something more to check.
func playGPUSession(t *testing.T, x *gpuRig, st *sessionState, scale float64) {
	w := x.main()
	tr := x.h.Tree()
	w.SetFocused(true)
	x.drain()
	x.check("mount")
	if !x.held(w) || !tr.lay.rasGPU {
		t.Fatalf("the first frame was not drawn by the GPU (log: %q)", x.log)
	}

	// Hover down the list, then the button (a transition).
	for i := 0; i < 5; i++ {
		px, py := x.at(w, fmt.Sprintf("row%d", i))
		w.Inject(move(px, py))
		x.step(fmt.Sprintf("hover row %d", i))
	}
	bx, by := x.at(w, "open")
	w.Inject(move(bx, by))
	x.settle("hover the button")

	// The wheel over the list: its pixels are shifted.
	lx, ly := x.at(w, "list")
	w.Inject(move(lx, ly))
	for i := 0; i < 4; i++ {
		w.Inject(platform.ScrollEvent{Pos: platform.Point{X: lx, Y: ly}, Steps: platform.Point{Y: 1}})
		x.settle(fmt.Sprintf("wheel %d", i))
	}
	st.list.ScrollBy(0, -53)
	x.settle("list back up")
	// The rows of the glass pane scroll over its blur.
	for i := 0; i < 4; i++ {
		st.glass.ScrollBy(0, 37)
		x.settle(fmt.Sprintf("glass scroll %d", i))
	}

	// Text edits.
	ix, iy := x.at(w, "in")
	x.click(w, ix, iy)
	x.step("focus the field")
	for _, s := range []string{"H", "el", "lo GPU"} {
		x.typeText(w, s)
		x.step("type " + s)
	}
	w.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeBackspace, Key: platform.KeyBackspace})
	x.drain()
	x.step("backspace")
	x.b.Advance(600 * time.Millisecond) // the caret blinks
	x.deliver()
	x.step("caret blink")

	// A dialog over the glass, its content changing, a row
	// changing under it, and away.
	st.setDialog.Set(1)
	x.settle("open the dialog")
	st.setDialog.Set(2)
	x.settle("change the dialog")
	st.setRow.Update(func(v int) int { return v + 1 })
	x.settle("a row under the scrim")
	st.setDialog.Set(0)
	x.settle("close the dialog")

	// Resizes: the target is made again, no pixel is stale.
	for _, s := range []platform.Size{{W: 700, H: 460}, {W: 520, H: 380}, {W: 640, H: 420}} {
		made := x.c.fake.made
		w.Resize(s)
		x.settle(fmt.Sprintf("resize to %v", s))
		if x.c.fake.made == made {
			t.Errorf("resize to %v: no new target", s)
		}
	}
	// A scale change.
	w.SetScale(scale + 0.5)
	x.settle("scale up")
	st.glass.ScrollBy(0, -20)
	x.settle("glass scroll at the new scale")
	w.SetScale(scale)
	x.settle("scale back")

	// Hidden: the target goes with the framebuffer. The tree
	// commits meanwhile, and the frame that shows it is whole.
	handle := x.h.Handle(x.pw(w))
	handle.Hide()
	x.step("hide")
	if x.held(w) || x.view(w).fb != nil || x.c.live() != 0 || x.c.fake.exportsLive != 0 {
		t.Errorf("hidden: target held %v, framebuffer %v, %d targets and %d exported images alive", x.held(w), x.view(w).fb != nil, x.c.live(), x.c.fake.exportsLive)
	}
	st.setRow.Update(func(v int) int { return v + 1 })
	x.step("a row while hidden")
	handle.Show()
	x.settle("show")
	if !x.held(w) {
		t.Error("shown again: no target")
	}
	// Minimized: the target is released, the framebuffer stays
	// (as on the CPU path).
	w.Minimize()
	x.step("minimize")
	if x.held(w) || x.c.fake.exportsLive != 0 {
		t.Errorf("minimized: the target is held (%v), %d exported images alive", x.held(w), x.c.fake.exportsLive)
	}
	st.setRow.Update(func(v int) int { return v + 1 })
	x.step("a row while minimized")
	handle.Focus()
	x.settle("restore")
	if !x.held(w) {
		t.Error("restored: no target")
	}

	// The theme, there and back.
	tr.SetColorScheme(ColorSchemeDark)
	x.settle("dark")
	px, py := x.at(w, "row2")
	w.Inject(move(px, py))
	x.settle("hover in the dark")
	tr.SetColorScheme(ColorSchemeLight)
	x.settle("light")

	// Full frames on demand.
	x.h.FullFrames = true
	px, py = x.at(w, "row4")
	w.Inject(move(px, py))
	x.step("hover with full frames")
	if !tr.LastDamage().Full {
		t.Errorf("FullFrames: damage %+v", tr.LastDamage())
	}
	x.h.FullFrames = false

	// At rest the renderer's memory is trimmed by the host's
	// timer: no frame is drawn or presented for it, and the
	// frame after it is right. (A window with the focus does
	// not rest while its field's caret blinks.)
	w.SetFocused(false)
	x.settle("the window loses the focus")
	if !x.c.g.TrimDue() {
		t.Fatal("after frames, the GPU has nothing to trim")
	}
	if at := x.h.gpuTrimAt(); at == 0 {
		t.Fatal("at rest, no trim of the GPU is scheduled")
	}
	// What still waits plays out first (the list's scrollbar
	// fades): each of its frames moves the trim.
	trimmed := false
	for i := 0; i < 60 && !trimmed; i++ {
		x.b.Advance(100 * time.Millisecond)
		x.deliver()
		trimmed = !x.c.g.TrimDue()
	}
	if !trimmed || x.h.gpuTrimAt() != 0 {
		t.Errorf("after a rest the GPU was not trimmed (due %v, at %v)", x.c.g.TrimDue(), x.h.gpuTrimAt())
	}
	presents, frames := w.Presents(), tr.Stats().Frames
	for i := 0; i < 5; i++ {
		x.b.Advance(x.b.FrameInterval())
		x.deliver()
	}
	if w.Presents() != presents || tr.Stats().Frames != frames {
		t.Errorf("the trim asked for frames: %d presents and %d frames, were %d and %d", w.Presents(), tr.Stats().Frames, presents, frames)
	}
	if at, armed := x.h.TimerArmed(); armed {
		t.Errorf("at rest after the trim the host's timer is armed for %v (now %v)", at, x.b.Now())
	}
	if !x.held(w) {
		t.Error("the trim released the window's target")
	}
	x.check("after the trim")
	px, py = x.at(w, "row1")
	w.Inject(move(px, py))
	x.settle("hover after the trim")
	if d := tr.LastDamage(); d.Full {
		t.Errorf("the frame after the trim was a full frame: %+v", d)
	}

	if x.log.Len() > 0 {
		t.Errorf("the host logged: %s", x.log)
	}
	if x.c.cpuFrames != 0 || x.c.shifted == 0 || x.c.partial < 20 || x.c.full < 8 {
		t.Errorf("the session did not exercise what it should: %s", x.c.summary())
	}
	t.Log(x.c.summary())
}

// TestGPUDamageMatchesFullRender plays the random sessions of
// TestDamageMatchesFullRender (rows, pages with fixed glass headers,
// glass, boards, virtual lists: random pointer moves, wheels, touchpad
// scrolls, state changes, Values read by paint, resizes) on windows drawn
// by the GPU.
func TestGPUDamageMatchesFullRender(t *testing.T) {
	steps := 60
	if testing.Short() {
		steps = 20
	}
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var all []*gpuChecker
		setup := func(x *damageHarness) {
			c := newGPUChecker(t, dev)
			c.t = x.t
			all = append(all, c)
			x.h.UseGPU(c.g)
			x.checkFn = func(what string) {
				x.t.Helper()
				c.checkWindow(x.h, x.w, fmt.Sprintf("step %d (%s; last steps: %s)", x.step, what, strings.Join(x.trace, " | ")))
				x.frames, x.full, x.shifted, x.partial = c.full+c.shifted+c.partial, c.full, c.shifted, c.partial
			}
			x.t.Cleanup(func() { c.finish(x.h) })
		}
		scales := []float64{1, 1.5, 2}
		rows := func(st *sceneState) Node { return rowScene(st, 120, "grow overflow-y-auto bg-white") }
		runSceneWith(t, "rows", rows, commonOps, Config{}, steps, scales, setup)
		rounded := func(st *sceneState) Node {
			return rowScene(st, 60, "m-3 grow overflow-auto rounded-2xl border-2 border-slate-300 bg-white shadow-lg")
		}
		runSceneWith(t, "rounded", rounded, commonOps, Config{InstantScroll: true}, steps, scales, setup)
		runSceneWith(t, "page", pageScene, commonOps, Config{InstantScroll: true}, steps, scales, setup)
		runSceneWith(t, "glass", glassScene, commonOps, Config{InstantScroll: true}, steps, scales, setup)
		runSceneWith(t, "plainpage_classic", plainPageScene, commonOps, Config{ClassicScrollbars: true}, steps, scales, setup)
		runSceneWith(t, "board", boardScene, commonOps, Config{InstantScroll: true}, steps, scales, setup)
		runSceneWith(t, "virtual", virtualScene, commonOps, Config{InstantScroll: true}, steps, scales, setup)
		total := gpuChecker{}
		for _, c := range all {
			total.gpuFrames += c.gpuFrames
			total.cpuFrames += c.cpuFrames
			total.full += c.full
			total.shifted += c.shifted
			total.partial += c.partial
			if c.worst.Max > total.worst.Max {
				total.worst = c.worst
			}
			total.over2 = max(total.over2, c.over2)
		}
		t.Log(total.summary())
		if total.cpuFrames != 0 || total.shifted == 0 || total.partial == 0 {
			t.Errorf("the sessions did not exercise what they should: %s", total.summary())
		}
	})
}

// TestGPUTransparentWindow draws a transparent window with the GPU: the
// frame read back keeps its alpha, which the window's input mask is
// computed from (ADR-249).
func TestGPUTransparentWindow(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var setBig Setter[bool]
		root := Component(func() Node {
			big, set := UseState(false)
			setBig = set
			cls := "rounded-2xl bg-white/90 p-4 text-slate-900 shadow-xl"
			if big {
				cls += " h-40"
			}
			return div(Class("h-screen p-10"), div(Key("card"), Class(cls), Text("A card on nothing"), div(Key("dot"), Class("mt-2 size-6 rounded-full bg-rose-500 hover:bg-sky-500"))))
		})
		x := newGPURig(t, dev, Options{Window: WindowSpec{Transparent: true, Frameless: true}, Root: root}, 320, 240, 2)
		w := x.main()
		x.b.Drain(x.h)
		x.check("mount")
		got := w.Frame()
		if px := got.RGBAAt(4, 4); px.A != 0 {
			t.Errorf("the corner of the transparent window is %v", px)
		}
		cx, cy := x.at(w, "card")
		if px := got.RGBAAt(int(cx*2), int(cy*2)); px.A < 200 {
			t.Errorf("the card's center is %v", px)
		}
		if !w.TakesInput(platform.Point{X: cx, Y: cy}) || w.TakesInput(platform.Point{X: 3, Y: 3}) {
			t.Errorf("input mask: the card takes input %v, the corner %v", w.TakesInput(platform.Point{X: cx, Y: cy}), w.TakesInput(platform.Point{X: 3, Y: 3}))
		}
		dx, dy := x.at(w, "dot")
		w.Inject(move(dx, dy))
		x.settle("hover the dot")
		setBig.Set(true)
		x.settle("the card grows")
		// The mask follows the frames: the grown card takes input where
		// nothing was.
		if p := (platform.Point{X: cx, Y: cy + 60}); !w.TakesInput(p) {
			t.Errorf("after the card grew, %v does not take input", p)
		}
		setBig.Set(false)
		x.settle("the card shrinks")
		if p := (platform.Point{X: cx, Y: cy + 60}); w.TakesInput(p) {
			t.Errorf("after the card shrank, %v still takes input", p)
		}
		if x.c.cpuFrames != 0 || x.c.partial == 0 {
			t.Errorf("the session did not exercise what it should: %s", x.c.summary())
		}
		t.Log(x.c.summary())
		x.c.finish(x.h)
	})
}

// TestGPUChildWindows draws an app with child windows on one device: each
// window has its target, a closed window releases it, and the frames of
// all of them are right while they alternate (one Renderer draws them
// all).
func TestGPUChildWindows(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var setOpen Setter[int]
		var setN Setter[int]
		child := func(name string) Node {
			return Component(func() Node {
				n, set := UseState(0)
				if name == "one" {
					setN = set
				}
				return div(Class("h-screen bg-linear-to-b from-sky-200 to-indigo-300 p-4"),
					div(Key("box"), Class("rounded-xl bg-white/70 p-3 text-sm shadow-lg backdrop-blur hover:bg-white"), Textf("Window %s, state %d", name, n)))
			})
		}
		one, two := child("one"), child("two")
		root := Component(func() Node {
			open, set := UseState(2)
			setOpen = set
			c := []Child{Class("h-screen bg-slate-100 p-4"), div(Key("main"), Class("rounded-lg bg-white p-3 shadow hover:bg-amber-100"), Textf("Main, %d children", open))}
			if open >= 1 {
				c = append(c, WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "one", Width: 300, Height: 200}, Root: one}), "one"))
			}
			if open >= 2 {
				c = append(c, WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "two", Width: 260, Height: 180}, Root: two}), "two"))
			}
			return div(c...)
		})
		x := newGPURig(t, dev, Options{Root: root}, 400, 300, 1.5)
		x.settle("mount")
		ws := x.b.Windows()
		if len(ws) != 3 {
			t.Fatalf("%d windows open, want 3", len(ws))
		}
		for _, w := range ws {
			if !x.held(w) {
				t.Errorf("window %q has no target", w.Title())
			}
		}
		if x.c.live() != 3 {
			t.Errorf("%d targets alive, want 3", x.c.live())
		}
		// Frames of the three windows, alternating.
		for i := 0; i < 3; i++ {
			for _, w := range ws {
				k := "box"
				if w == x.main() {
					k = "main"
				}
				px, py := x.at(w, k)
				if i%2 == 1 {
					px, py = 2, 2
				}
				w.Inject(move(px, py))
			}
			setN.Update(func(v int) int { return v + 1 })
			x.settle(fmt.Sprintf("round %d", i))
		}
		// A child closes: its target goes.
		setOpen.Set(1)
		x.settle("close a child")
		if n := len(x.b.Windows()); n != 2 || x.c.live() != 2 {
			t.Errorf("after a child closed: %d windows, %d targets alive", n, x.c.live())
		}
		setN.Update(func(v int) int { return v + 1 })
		x.settle("the other child goes on")
		setOpen.Set(2)
		x.settle("open it again")
		if x.c.live() != 3 {
			t.Errorf("after the child opened again: %d targets alive", x.c.live())
		}
		if x.log.Len() > 0 {
			t.Errorf("the host logged: %s", x.log)
		}
		if x.c.cpuFrames != 0 {
			t.Errorf("frames drawn by the CPU rasterizer: %s", x.c.summary())
		}
		t.Log(x.c.summary())
		x.c.finish(x.h)
	})
}

// TestGPUDebugOverlays draws the debug overlays over frames of the GPU:
// the CPU rasterizer draws them over the frame read back, so the window
// shows the GPU's frame under the same overlay list as on the CPU path
// (paint-flash over what the GPU repainted, the layers' borders, the
// layout boxes, the meter, the semantics bounds), and turning them off
// restores the application's own pixels.
func TestGPUDebugOverlays(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		st := &sessionState{}
		x := newGPURig(t, dev, Options{Config: Config{Debug: DebugPaintFlash | DebugLayers}, Root: sessionPage(st)}, 640, 420, 1.5)
		playGPUDebug(t, x, st, nil)
	})
}

// playGPUDebug plays the session of TestGPUDebugOverlays on a rig whose
// root is sessionPage(st). each, when not nil, runs after the frames of
// every set of flags.
func playGPUDebug(t *testing.T, x *gpuRig, st *sessionState, each func(f DebugFlags)) {
	{
		w := x.main()
		tr := x.h.Tree()
		x.drain()
		x.check("mount")
		for _, f := range []DebugFlags{DebugPaintFlash | DebugLayers, DebugAll, 0, DebugLayout | DebugFPS, DebugPaintFlash, 0} {
			tr.SetDebug(f)
			x.step("set debug " + f.String())
			if f != 0 && (tr.dbg.prev == nil || tr.dbg.prev.Len() == 0) && f != DebugPaintFlash {
				t.Errorf("debug %s: no overlay drawn", f)
			}
			px, py := x.at(w, "list") // over the row in the middle of the list
			w.Inject(move(px, py))
			x.step("hover")
			if f&DebugPaintFlash != 0 {
				// The flash is the GPU frame's damage: the hovered row.
				if len(tr.dbg.flashes) == 0 {
					t.Errorf("debug %s: a hover left no flash", f)
				}
				if d := tr.LastDamage(); d.Full || len(d.Rects) == 0 {
					t.Errorf("debug %s: the hover's damage is %+v", f, d)
				}
			}
			st.list.ScrollBy(0, 31)
			x.step("scroll")
			st.setDialog.Set(1)
			x.step("dialog")
			st.setDialog.Set(0)
			x.step("dialog away")
			w.Inject(move(3, 3))
			for i := 0; i < 4; i++ {
				x.step("a flash fades")
			}
			if f == 0 && (tr.dbg.clean != nil || tr.dbg.prev != nil) {
				t.Errorf("overlays off but the overlay state is kept")
			}
			if each != nil {
				each(f)
			}
		}
		if x.c.cpuFrames != 0 {
			t.Errorf("frames drawn by the CPU rasterizer: %s", x.c.summary())
		}
		t.Log(x.c.summary())
		x.c.finish(x.h)
	}
}

// errInjected is the error the fallback tests inject.
var errInjected = errors.New("injected failure")

// TestGPUFallback injects a failure into each call of the device, at the
// first frame and in the middle of a session, in a frame that shifts,
// draws rectangles and reads back: the frame is the CPU rasterizer's, in
// full and exact, the window goes on with it, one line is logged, the
// target is released, and nothing panics.
func TestGPUFallback(t *testing.T) {
	type failure struct {
		op string
		// after is how many calls of op succeed first (in the frame that
		// fails, for the calls of the session's middle).
		after int
		first bool // fail at the first frame
	}
	cases := []failure{
		{"NewTarget", 0, true}, {"Draw", 0, true}, {"ReadPixels", 0, true},
		{"Shift", 0, false}, {"Draw", 0, false}, {"Draw", 1, false}, {"ReadPixels", 0, false}, {"NewTarget", 0, false},
	}
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		for _, fc := range cases {
			name := fmt.Sprintf("%s_%d", fc.op, fc.after)
			if fc.first {
				name += "_first"
			}
			t.Run(name, func(t *testing.T) {
				st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
				x := newGPURig(t, dev, Options{Config: Config{InstantScroll: true}, Root: rowScene(st, 120, "grow overflow-y-auto bg-white")}, 360, 280, 2)
				w := x.main()
				tr := x.h.Tree()
				n := 0
				arm := func() {
					n = 0
					x.c.fake.fail = func(op string) error {
						if op != fc.op {
							return nil
						}
						if n++; n > fc.after {
							x.c.fake.fail = nil
							return fmt.Errorf("%s: %w", op, errInjected)
						}
						return nil
					}
				}
				if fc.first {
					arm()
				}
				x.b.Drain(x.h)
				x.check("mount")
				if !fc.first {
					if !tr.lay.rasGPU {
						t.Fatal("the first frame was not the GPU's")
					}
					st.refs[0].ScrollBy(0, 40)
					x.step("a scroll")
					st.refs[0].ScrollBy(0, 53)
					x.step("another scroll")
					if fc.op == "NewTarget" {
						// A new target is made for a new size.
						arm()
						w.Resize(platform.Size{W: 380, H: 300})
					} else {
						// A frame that shifts the list and draws two
						// rectangles: the strip the scroll exposes, and the
						// rows whose pixels are not shifted (a gradient, a
						// positioned badge).
						arm()
						st.refs[0].ScrollBy(0, 53)
					}
					x.step("the frame that fails")
				}
				if x.c.fake.fail != nil {
					t.Fatalf("the failure of %s was not reached (%d calls)", fc.op, n)
				}
				if tr.lay.rasGPU || !tr.LastDamage().Full {
					t.Errorf("the frame that failed: drawn by the GPU %v, damage %+v", tr.lay.rasGPU, tr.LastDamage())
				}
				if !x.view(w).gpu.off || x.held(w) || x.c.live() != 0 {
					t.Errorf("after the failure: window off %v, target held %v, %d targets alive", x.view(w).gpu.off, x.held(w), x.c.live())
				}
				if x.h.GPU() == nil {
					t.Error("an error that is not a lost device closed the GPU")
				}
				if got := strings.Count(x.log.String(), "\n"); got != 1 || !strings.Contains(x.log.String(), "injected failure") || !strings.Contains(x.log.String(), "CPU rasterizer") {
					t.Errorf("the log has %d lines, want one that names the error: %q", got, x.log)
				}
				// The window goes on, drawn by the CPU rasterizer, with
				// incremental damage again.
				before := x.c.cpuFrames
				for i := 0; i < 4; i++ {
					st.refs[0].ScrollBy(0, 31)
					st.setters[i].Update(func(v int) int { return v + 1 })
					x.step(fmt.Sprintf("after the failure %d", i))
				}
				if tr.lay.rasGPU || tr.LastDamage().Full || x.c.cpuFrames-before != 4 {
					t.Errorf("after the failure: drawn by the GPU %v, damage %+v, %d CPU frames", tr.lay.rasGPU, tr.LastDamage(), x.c.cpuFrames-before)
				}
				if strings.Count(x.log.String(), "\n") != 1 {
					t.Errorf("more than one line logged: %q", x.log)
				}
				x.c.finish(x.h)
			})
		}
	})
}

// TestGPUDeviceLost loses the device in the frame of one of three windows:
// that frame is the CPU rasterizer's, every window is drawn by it from
// then on (the others from a full frame), the host closes its GPU and
// every target, and logs one line.
func TestGPUDeviceLost(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var setN [3]Setter[int]
		page := func(k int) Node {
			return Component(func() Node {
				n, set := UseState(0)
				setN[k] = set
				return div(Class("h-screen bg-linear-to-b from-sky-200 to-indigo-300 p-4"),
					div(Class("rounded-xl bg-white/70 p-3 text-sm shadow-lg backdrop-blur"), Textf("Window %d, state %d", k, n)))
			})
		}
		pages := [3]Node{page(0), page(1), page(2)}
		root := Component(func() Node {
			return div(Class("h-screen"), pages[0],
				WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "one", Width: 300, Height: 200}, Root: pages[1]}), "one"),
				WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "two", Width: 260, Height: 180}, Root: pages[2]}), "two"))
		})
		x := newGPURig(t, dev, Options{Root: root}, 400, 300, 1)
		x.settle("mount")
		if x.c.live() != 3 || x.c.cpuFrames != 0 {
			t.Fatalf("%d targets alive, %s", x.c.live(), x.c.summary())
		}
		// The device goes away in the next draw: window one's.
		x.c.fake.fail = func(op string) error {
			if op != "Draw" {
				return nil
			}
			x.c.fake.fail = nil
			return fmt.Errorf("%w: %w", gpu.ErrDeviceLost, errInjected)
		}
		setN[1].Update(func(v int) int { return v + 1 })
		x.b.Advance(x.b.FrameInterval())
		x.deliver()
		if x.c.fake.fail != nil {
			t.Fatal("no draw failed")
		}
		if x.h.GPU() != nil || !x.c.g.Lost() || x.c.live() != 0 {
			t.Errorf("after the device was lost: host GPU %v, lost %v, %d targets alive", x.h.GPU() != nil, x.c.g.Lost(), x.c.live())
		}
		if got := strings.Count(x.log.String(), "\n"); got != 1 || !strings.Contains(x.log.String(), "lost") {
			t.Errorf("the log has %d lines, want one about the lost device: %q", got, x.log)
		}
		one := x.b.Windows()[1]
		if tr := x.h.TreeOf(one); tr.lay.rasGPU || !tr.LastDamage().Full {
			t.Errorf("the frame that lost the device: drawn by the GPU %v, damage %+v", tr.lay.rasGPU, tr.LastDamage())
		}
		// Every window goes on with the CPU rasterizer: its first frame in
		// full, the next ones incremental. The frames the other windows
		// show are still the GPU's until they draw again.
		for _, w := range x.b.Windows() {
			if w != one && x.h.TreeOf(w).lay.rasGPU {
				x.c.checkWindow(x.h, w, "a window that did not draw since") // reference frames need the device: not lost for real
			}
		}
		x.c.closeRef()
		for round := 0; round < 3; round++ {
			for k := range setN {
				setN[k].Update(func(v int) int { return v + 1 })
			}
			x.settle(fmt.Sprintf("round %d after the loss", round))
			for _, w := range x.b.Windows() {
				tr := x.h.TreeOf(w)
				if tr.lay.rasGPU {
					t.Errorf("round %d: window %q is still drawn by the GPU", round, w.Title())
				}
				if d := tr.LastDamage(); round > 0 && d.Full {
					t.Errorf("round %d: window %q draws full frames: %+v", round, w.Title(), d)
				}
			}
		}
		if strings.Count(x.log.String(), "\n") != 1 {
			t.Errorf("more than one line logged: %q", x.log)
		}
		x.c.finish(x.h)
	})
}

// TestGPUUnsupported draws a page with a layer larger than the device's
// textures (a device made small for the test): the renderer says it cannot
// draw it, and the frame is the CPU rasterizer's.
func TestGPUUnsupported(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		var setBlur Setter[bool]
		root := Component(func() Node {
			blur, set := UseState(false)
			setBlur = set
			cls := "h-64 w-[600px] rounded-xl bg-rose-400"
			if blur {
				cls += " blur-md"
			}
			return div(Class("h-screen bg-white p-4"), div(Class(cls)))
		})
		x := newGPURig(t, dev, Options{Root: root}, 640, 300, 1)
		x.c.fake.max = 512 // the window is 640 wide
		x.b.Drain(x.h)
		x.check("mount")
		tr := x.h.Tree()
		if tr.lay.rasGPU || x.c.cpuFrames != 1 || !x.view(x.main()).gpu.off {
			t.Errorf("a window wider than the device's targets: drawn by the GPU %v, %s", tr.lay.rasGPU, x.c.summary())
		}
		if got := strings.Count(x.log.String(), "\n"); got != 1 {
			t.Errorf("the log has %d lines: %q", got, x.log)
		}
		x.c.finish(x.h)

		// A window that fits, and then a filter layer that does not.
		x = newGPURig(t, dev, Options{Root: root}, 500, 300, 1)
		x.c.fake.max = 512
		x.b.Drain(x.h)
		x.check("mount")
		tr = x.h.Tree()
		if !tr.lay.rasGPU {
			t.Fatalf("the first frame was not the GPU's: %q", x.log)
		}
		setBlur.Set(true)
		x.step("a layer larger than the device's textures")
		if tr.lay.rasGPU || !tr.LastDamage().Full || !strings.Contains(x.log.String(), "cannot draw") || strings.Count(x.log.String(), "\n") != 1 {
			t.Errorf("drawn by the GPU %v, damage %+v, log %q", tr.lay.rasGPU, tr.LastDamage(), x.log)
		}
		setBlur.Set(false)
		x.step("and back")
		if tr.lay.rasGPU {
			t.Error("the window went back to the GPU")
		}
		x.c.finish(x.h)
	})
}

// TestGPUAndCPUAlternate draws the frames of one tree with the GPU and the
// CPU rasterizer in turns, as a window that falls back does once: the
// first frame of each after the other is a full frame, the others are
// incremental, the CPU's frames are exact, and what the CPU rasterizer
// keeps between frames (the snapshots that damaged rectangles start from)
// is dropped when the GPU takes the tree over.
func TestGPUAndCPUAlternate(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		for _, scale := range []float32{1, 1.5} {
			st := &sessionState{}
			tr := newTestTree(t, 640, 420, sessionPage(st), nil)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(640*scale+0.5), int(420*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			g := NewGPU(dev)
			s := g.NewSurface()
			full := g.NewSurface()
			check := func(what string, onGPU bool, d Damage) {
				t.Helper()
				cpu := cpuFrame(tr.lay.cur, fb.Rect.Size(), tr.clearColor())
				if !onGPU {
					if p, ok := firstDiff(fb, cpu); !ok {
						t.Fatalf("scale %g, %s: the CPU frame differs from a full rasterization at %v (damage %+v)", scale, what, p, d)
					}
					return
				}
				// A full frame of the GPU, on another surface of the tree
				// (what the tree remembers of its last frame is put back).
				ras, rect, dmg := tr.lay.ras, tr.lay.rasRect, tr.lay.damage
				want := image.NewRGBA(fb.Rect)
				if _, err := tr.RasterizeFrameGPU(want, full, &r, true); err != nil {
					t.Fatal(err)
				}
				tr.lay.ras, tr.lay.rasRect, tr.lay.damage = ras, rect, dmg
				if p, ok := firstDiff(fb, want); !ok {
					t.Fatalf("scale %g, %s: the GPU frame differs from a full GPU frame at %v: %v, want %v (damage %+v)", scale, what, p, fb.RGBAAt(p.X, p.Y), want.RGBAAt(p.X, p.Y), d)
				}
				if m := gfxtest.Compare(fb, cpu); !m.WithinGPU() {
					t.Errorf("scale %g, %s: against the CPU rasterizer: largest difference %d at %v, %.3f%% over 2 levels", scale, what, m.Max, m.Worst, 100*m.Over2)
				}
			}
			last := ""
			for n, who := range []string{"cpu", "cpu", "cpu", "gpu", "gpu", "gpu", "cpu", "cpu", "cpu", "gpu", "cpu", "gpu", "gpu", "cpu"} {
				// The rows of the glass pane are drawn again over its blur
				// (from a snapshot, on the CPU).
				st.glass.ScrollBy(0, 7)
				tr.Frame()
				what := fmt.Sprintf("step %d (%s)", n, who)
				var d Damage
				if who == "gpu" {
					var err error
					if d, err = tr.RasterizeFrameGPU(fb, s, &r, false); err != nil {
						t.Fatal(err)
					}
					if n := tr.lay.snapBytes(); n != 0 {
						t.Errorf("%s: the CPU rasterizer's snapshots hold %d bytes under a GPU frame", what, n)
					}
				} else {
					d = tr.RasterizeFrame(fb, &r, false)
				}
				if tr.lay.rasGPU != (who == "gpu") {
					t.Errorf("scale %g, %s: the tree says its frame was drawn by the GPU: %v", scale, what, tr.lay.rasGPU)
				}
				if (who != last) != d.Full {
					t.Errorf("scale %g, %s after %q: full frame %v: %+v", scale, what, last, d.Full, d)
				}
				check(what, who == "gpu", d)
				last = who
			}
			s.Release()
			full.Release()
			g.Close()
			tr.Unmount()
		}
	})
}

// namedBackend is the headless backend under another name: a Host does not
// open a GPU by itself on the headless one.
type namedBackend struct{ *headless.Backend }

func (namedBackend) Name() string { return "test" }

// TestGPURendererChoice checks what a Host opens (openGPU, which Run
// calls): nothing on the headless backend and with TELA_RENDERER=cpu; by
// default the GPU when the device chosen is a hardware one, integrated
// ones included, and the CPU rasterizer, silently, when it is a software
// one; that device too with TELA_RENDERER=gpu; the device the main
// window's display server names before the first discrete one, and the
// one TELA_GPU_DEVICE names before
// both; and the CPU rasterizer with one line of the log when no device
// initializes, or when a device named is not used.
func TestGPURendererChoice(t *testing.T) {
	gputest.Need(t) // skips without Vulkan
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu tests"})
	if err != nil {
		t.Skip(err)
	}
	devices, err := inst.Devices()
	inst.Destroy()
	if err != nil || len(devices) == 0 {
		t.Skip("no devices")
	}
	first, _ := vk.SelectDevice(devices, "")
	hardware := func(d vk.DeviceInfo) bool { return d.Type != vk.PhysicalDeviceTypeCPU }
	type choice struct {
		name          string
		renderer, dev string
		named         bool
		server        *vk.DeviceInfo // the device the window's feedback names
		gpu           bool
		device        string // the name of the device opened, when it matters
		log           string // what the one line of the log holds; "": no line
	}
	cases := []choice{
		{name: "headless"},
		{name: "headless_gpu", renderer: "gpu"},
		{name: "default", named: true, gpu: hardware(devices[first]), device: devices[first].Name},
		{name: "gpu", renderer: "gpu", named: true, gpu: true, device: devices[first].Name},
		{name: "cpu", renderer: "cpu", named: true},
		{name: "unknown", renderer: "metal", named: true, gpu: hardware(devices[first]), log: `TELA_RENDERER="metal"`},
		{name: "no_device", dev: "no such device", named: true, log: "using the CPU rasterizer"},
	}
	for i := range devices {
		d := devices[i]
		label := gputest.Label(d.Name)
		if !hardware(d) {
			// A software device is used only when asked for.
			cases = append(cases,
				choice{name: "named_" + label, dev: d.Name, named: true, log: "TELA_RENDERER=gpu draws with the GPU"},
				choice{name: "named_gpu_" + label, renderer: "gpu", dev: d.Name, named: true, gpu: true, device: d.Name})
		} else {
			// A hardware device named is used, integrated ones included.
			cases = append(cases,
				choice{name: "named_" + label, dev: d.Name, named: true, gpu: true, device: d.Name})
		}
		if d.DRM.HasRender || d.DRM.HasPrimary {
			// The display server's device is the one opened.
			cases = append(cases,
				choice{name: "server_" + label, named: true, server: &d, gpu: hardware(d), device: d.Name},
				choice{name: "server_gpu_" + label, renderer: "gpu", named: true, server: &d, gpu: true, device: d.Name},
				choice{name: "server_named_" + label, renderer: "gpu", dev: devices[first].Name, named: true, server: &d, gpu: true, device: devices[first].Name})
		}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvRenderer, c.renderer)
			t.Setenv("TELA_GPU_DEVICE", c.dev)
			t.Setenv("TELA_VK_VALIDATE", "") // the host's own device: not under this test's validation
			t.Setenv(EnvPresent, "copy")     // the fake display server's buffers are not this test's subject
			b := headless.New(headless.Options{Size: platform.Size{W: 200, H: 120}})
			var pb platform.Backend = b
			drain := func(h *Host) { b.Drain(h) }
			switch {
			case c.server != nil:
				d := newDmabufBackend(t, b, &fakeDevice{}, dmabufConfig{})
				d.feedback.HasDevice = true
				if n := c.server.DRM; n.HasRender {
					d.feedback.DeviceMajor, d.feedback.DeviceMinor = uint32(n.RenderMajor), uint32(n.RenderMinor)
				} else {
					d.feedback.DeviceMajor, d.feedback.DeviceMinor = uint32(n.PrimaryMajor), uint32(n.PrimaryMinor)
				}
				pb = namedDmabuf{d}
				drain = func(h *Host) { d.Drain(h) }
			case c.named:
				pb = namedBackend{b}
			}
			log := &bytes.Buffer{}
			h, err := NewHost(pb, Options{Config: Config{Fonts: testFonts}, Root: div(Class("h-screen bg-sky-300")), ErrorLog: log})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { h.stopTimer(); h.end() })
			h.openGPU()
			drain(h)
			if got := h.GPU() != nil; got != c.gpu {
				t.Fatalf("the host has a GPU: %v, want %v (log %q)", got, c.gpu, log)
			}
			if c.gpu && c.device != "" && h.GPU().Name() != c.device {
				t.Errorf("the host opened %q, want %q", h.GPU().Name(), c.device)
			}
			if got := h.Tree().lay.rasGPU; got != c.gpu {
				t.Errorf("the first frame was drawn by the GPU: %v, want %v", got, c.gpu)
			}
			// The line of TELA_PRESENT=copy is this test's own.
			var lines []string
			for _, l := range strings.Split(strings.TrimSpace(log.String()), "\n") {
				if l != "" && !strings.Contains(l, EnvPresent+"=copy") {
					lines = append(lines, l)
				}
			}
			if c.log == "" && len(lines) != 0 || c.log != "" && (len(lines) != 1 || !strings.Contains(lines[0], c.log)) {
				t.Errorf("the log is %q, want one line with %q", lines, c.log)
			}
			var w *headless.Window
			if dw, ok := h.Window().(*dmabufWindow); ok {
				w = dw.Window
			} else {
				w = h.Window().(*headless.Window)
			}
			if w.Frame() == nil {
				t.Error("nothing presented")
			} else if px := w.Frame().RGBAAt(10, 10); px.A != 255 || px.B < 200 {
				t.Errorf("the window shows %v", px)
			}
		})
	}
}

// namedDmabuf is a backend whose windows present dmabufs under a name of
// its own, as namedBackend.
type namedDmabuf struct{ *dmabufBackend }

func (namedDmabuf) Name() string { return "test" }
