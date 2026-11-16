//go:build telagpu && windows

package core_test

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/win32"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// TestLiveGPUWindows runs a small app in a real Win32 window drawn by the
// GPU core opens for it, as Run does, and presented through a Vulkan
// swapchain on the window (ADR-289): partial frames, resizes, minimizing
// and restoring, hiding and showing, the theme. After every step the
// window's pixels are read from the screen (PrintWindow with
// PW_RENDERFULLCONTENT, which reads what DWM composes for the window, a
// flip-model swapchain's images included; else a BitBlt of the screen
// where the window is) and compared with the CPU rasterizer's rendering
// of the frame, within ADR-282's tolerance.
//
// It needs a Windows machine with a Vulkan driver, TELA_LIVE=1 and the
// build tag telagpu:
//
//	go test -c -tags telagpu -o core.test.exe ./internal/core
//	set TELA_LIVE=1
//	core.test.exe -test.run TestLiveGPUWindows -test.v
//
// TELA_LIVE_DUMP=dir writes the window's pixels and the CPU's frame of a
// step that differs.
func TestLiveGPUWindows(t *testing.T) {
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run in a real window")
	}
	div := func(c ...core.Child) core.Node { return core.Primitive(style.KindDiv, c...) }
	var set core.Setter[int]
	root := core.Component(func() core.Node {
		n, setN := core.UseState(0)
		set = setN
		colors := []string{"bg-red-500", "bg-emerald-500", "bg-sky-500", "bg-amber-400"}
		return div(core.Class("relative h-screen bg-slate-100 p-6 dark:bg-slate-900"),
			div(core.Class("rounded-xl bg-white p-4 text-2xl text-slate-900 shadow-lg dark:bg-slate-800 dark:text-white"), core.Textf("Frame %d of the live test", n)),
			div(core.Class("mt-4 h-24 rounded-2xl bg-linear-to-r from-indigo-500 to-rose-500")),
			div(core.Class("mt-4 h-16 rounded-xl "+colors[n%len(colors)])),
			div(core.Class("absolute bottom-6 right-6 size-20 rounded-full border-4 border-white shadow-xl "+colors[(n+1)%len(colors)])))
	})
	next := func(name string) winStep {
		return winStep{name: name, do: func(*core.Host) { set.Update(func(v int) int { return v + 1 }) }}
	}
	resize := func(w, h float32) winStep {
		return winStep{name: fmt.Sprintf("resize to %vx%v", w, h), do: func(h0 *core.Host) { h0.Window().SetSize(platform.Size{W: w, H: h}) }}
	}
	steps := []winStep{
		next("a partial frame"), next("a second one"), next("a third one"),
		resize(760, 500), next("a frame at the new size"), resize(520, 360), resize(640, 420),
		{name: "minimize", do: func(h *core.Host) { h.Handle(h.Window()).Minimize() }, unchecked: true},
		{name: "a change while minimized", do: func(*core.Host) { set.Update(func(v int) int { return v + 1 }) }, unchecked: true},
		{name: "restore", do: func(h *core.Host) { h.Handle(h.Window()).Focus() }},
		next("a frame after the restore"),
		{name: "hide", do: func(h *core.Host) { h.Handle(h.Window()).Hide() }, unchecked: true},
		{name: "show", do: func(h *core.Host) { h.Handle(h.Window()).Show() }},
		{name: "dark", do: func(h *core.Host) { h.Tree().SetColorScheme(core.ColorSchemeDark) }},
		next("a frame in the dark"),
	}

	b, err := win32.New()
	if err != nil {
		t.Fatalf("win32: %v", err)
	}
	log := &bytes.Buffer{}
	h, err := core.NewHost(b, core.Options{
		Config:   core.Config{Fonts: stdfonts.NewCollection()},
		Window:   core.WindowSpec{Title: "tela live gpu", Width: 640, Height: 420},
		Root:     root,
		ErrorLog: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.OpenGPU()
	if h.GPU() == nil {
		h.End()
		t.Fatalf("no GPU was opened; log: %s", log)
	}
	device := h.GPU().Name()
	stop := h.Tree().ReceivePosts()
	d := &winDriver{t: t, b: b, h: h, steps: steps, started: time.Now(), since: time.Now()}
	h.AfterFrame = func() { d.frames++; d.since = time.Now() }
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				b.Wake()
			}
		}
	}()
	err = b.Run(d)
	close(done)
	stop()
	h.End()
	if err != nil {
		t.Errorf("the loop ended with %v", err)
	}
	gw, st := d.window, d.window.Swapchain
	t.Logf("%s: surface %v, direct %v; %d presents through %d swapchains of %d images (%d retried, %d hidden, %d out of date)",
		device, gw.Surface, gw.Direct, st.Shown, st.Created, st.Images, st.Retried, st.Hidden, st.OutOfDate)
	t.Logf("%d steps read from the screen (%s); against the CPU rasterizer at most %d levels (at %v), %.3f%% of the pixels over 2",
		d.checked, strings.Join(d.methods, ", "), d.worst.Max, d.worst.Worst, 100*d.worst.Over2)
	checked := 0
	for _, s := range steps {
		if !s.unchecked {
			checked++
		}
	}
	if d.checked < checked+1 {
		t.Errorf("%d steps were read and compared, want %d", d.checked, checked+1)
	}
	if !gw.Surface || !gw.Direct || st.Shown == 0 {
		t.Errorf("the window was not presented through a swapchain (surface %v, direct %v, %d presents)", gw.Surface, gw.Direct, st.Shown)
	}
	if s := strings.TrimSpace(log.String()); s != "" {
		t.Errorf("the host logged: %s", s)
	}
}

// winStep is one step of the live run.
type winStep struct {
	name string
	do   func(h *core.Host)
	// unchecked: the window shows nothing to compare after the step.
	unchecked bool
}

// winDriver handles the backend's events for the Host and plays the steps,
// each once the window is at rest and shows the frame of the one before.
type winDriver struct {
	t       *testing.T
	b       platform.Backend
	h       *core.Host
	steps   []winStep
	step    int
	started time.Time
	since   time.Time
	frames  int
	done    bool
	why     string
	// want is the CPU rasterizer's rendering of the frame numbered wantAt.
	want    *image.RGBA
	wantAt  int
	checked int
	worst   gfxtest.Diff
	methods []string
	window  core.GPUWindow
}

const (
	winSettle = 300 * time.Millisecond
	winLimit  = 20 * time.Second
)

func (d *winDriver) HandleEvent(w platform.Window, e platform.Event) {
	d.h.HandleEvent(w, e)
	if _, ok := e.(platform.CloseRequestedEvent); ok && !d.done {
		d.t.Errorf("the window was closed during step %d", d.step)
		d.finish()
	}
	if !d.done {
		d.advance()
	}
}

func (d *winDriver) finish() {
	d.done = true
	d.window = d.h.GPUWindow(d.h.Window())
	d.b.Quit()
}

func (d *winDriver) name() string {
	if d.step == 0 {
		return "the first frame"
	}
	return d.steps[d.step-1].name
}

func (d *winDriver) advance() {
	t, tr := d.t, d.h.Tree()
	if time.Since(d.started) > winLimit {
		t.Errorf("%s: not shown after %v (%d frames ran; %s)", d.name(), winLimit, d.frames, d.why)
		d.finish()
		return
	}
	unchecked := d.step > 0 && d.steps[d.step-1].unchecked
	switch {
	case unchecked:
		if time.Since(d.started) < 2*winSettle {
			return
		}
	case d.frames == 0 || tr.Dirty() || tr.Animating() || d.h.NeedsFrame() || time.Since(d.since) < winSettle:
		d.why = "the window is not at rest"
		return
	}
	if !unchecked {
		if ok, why := d.check(); !ok {
			d.why = why
			return
		}
	}
	t.Logf("%s: shown after %v", d.name(), time.Since(d.started).Round(time.Millisecond))
	if d.step == len(d.steps) {
		d.finish()
		return
	}
	d.step++
	d.started, d.since, d.why = time.Now(), time.Now(), ""
	d.steps[d.step-1].do(d.h)
}

// check compares what the screen shows of the window with the frame the
// tree rasterized last.
func (d *winDriver) check() (bool, string) {
	p, ok := d.h.Window().(platform.Win32SurfacePresenter)
	if !ok {
		return false, "the window is no Win32SurfacePresenter"
	}
	s, ok := p.Win32Surface()
	if !ok {
		return false, "the window reports no surface"
	}
	tr := d.h.Tree()
	if d.want == nil || d.wantAt != d.frames {
		d.want, d.wantAt = tr.Rasterize(tr.LastFrame(), tr.Scale()), d.frames
		for i := 3; i < len(d.want.Pix); i += 4 {
			d.want.Pix[i] = 255 // what the screen shows is opaque
		}
	}
	var why []string
	for _, c := range []struct {
		name string
		read func(hwnd uintptr) (*image.RGBA, error)
	}{{"PrintWindow", printWindow}, {"screen BitBlt", screenBlt}} {
		got, err := c.read(s.Window)
		if err != nil {
			why = append(why, fmt.Sprintf("%s: %v", c.name, err))
			continue
		}
		if got.Rect != d.want.Rect {
			why = append(why, fmt.Sprintf("%s: the window is %v, the frame %v", c.name, got.Rect.Size(), d.want.Rect.Size()))
			continue
		}
		m := gfxtest.Compare(got, d.want)
		if !m.WithinGPU() {
			if dir := os.Getenv("TELA_LIVE_DUMP"); dir != "" {
				golden.WritePNG(d.t, filepath.Join(dir, "live_window.png"), got)
				golden.WritePNG(d.t, filepath.Join(dir, "live_cpu.png"), d.want)
			}
			why = append(why, fmt.Sprintf("%s: differs from the CPU rasterizer's frame by up to %d levels at %v, %.3f%% of the pixels by more than %d", c.name, m.Max, m.Worst, 100*m.Over2, gfxtest.GPULevels))
			continue
		}
		d.checked++
		d.methods = append(d.methods, c.name)
		if m.Max > d.worst.Max {
			d.worst = m
		}
		return true, ""
	}
	return false, strings.Join(why, "; ")
}

var (
	liveUser32             = windows.NewLazySystemDLL("user32.dll")
	liveGdi32              = windows.NewLazySystemDLL("gdi32.dll")
	procLiveGetDC          = liveUser32.NewProc("GetDC")
	procLiveReleaseDC      = liveUser32.NewProc("ReleaseDC")
	procLiveGetClientRect  = liveUser32.NewProc("GetClientRect")
	procLiveClientToScreen = liveUser32.NewProc("ClientToScreen")
	procLivePrintWindow    = liveUser32.NewProc("PrintWindow")
	procLiveCreateDC       = liveGdi32.NewProc("CreateCompatibleDC")
	procLiveDeleteDC       = liveGdi32.NewProc("DeleteDC")
	procLiveCreateDIB      = liveGdi32.NewProc("CreateDIBSection")
	procLiveSelectObject   = liveGdi32.NewProc("SelectObject")
	procLiveDeleteObject   = liveGdi32.NewProc("DeleteObject")
	procLiveBitBlt         = liveGdi32.NewProc("BitBlt")
)

const (
	pwClientOnly        = 0x1
	pwRenderFullContent = 0x2
	liveSrcCopy         = 0x00CC0020
	liveCaptureBlt      = 0x40000000
)

type liveRect struct{ left, top, right, bottom int32 }

type liveBitmapInfo struct {
	size          uint32
	width, height int32
	planes, bits  uint16
	compression   uint32
	sizeImage     uint32
	xppm, yppm    int32
	used, imp     uint32
	colors        [4]byte
}

// clientSize returns the size of the window's client area.
func clientSize(hwnd uintptr) (image.Point, error) {
	var r liveRect
	if ok, _, err := procLiveGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return image.Point{}, fmt.Errorf("GetClientRect: %v", err)
	}
	return image.Pt(int(r.right-r.left), int(r.bottom-r.top)), nil
}

// capture draws into a 32-bit top-down DIB of the given size with paint,
// and returns its pixels as RGBA.
func capture(size image.Point, paint func(mem uintptr) error) (*image.RGBA, error) {
	if size.X <= 0 || size.Y <= 0 {
		return nil, fmt.Errorf("an empty client area %v", size)
	}
	screen, _, _ := procLiveGetDC.Call(0)
	if screen == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procLiveReleaseDC.Call(0, screen)
	mem, _, _ := procLiveCreateDC.Call(screen)
	if mem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procLiveDeleteDC.Call(mem)
	bi := liveBitmapInfo{size: 40, width: int32(size.X), height: -int32(size.Y), planes: 1, bits: 32}
	var bits unsafe.Pointer
	bmp, _, _ := procLiveCreateDIB.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return nil, fmt.Errorf("CreateDIBSection failed")
	}
	defer procLiveDeleteObject.Call(bmp)
	old, _, _ := procLiveSelectObject.Call(mem, bmp)
	defer procLiveSelectObject.Call(mem, old)
	if err := paint(mem); err != nil {
		return nil, err
	}
	src := unsafe.Slice((*byte)(bits), 4*size.X*size.Y)
	img := image.NewRGBA(image.Rectangle{Max: size})
	for i := 0; i < len(src); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = src[i+2], src[i+1], src[i], 255
	}
	return img, nil
}

// printWindow reads the window's client area as DWM composes it.
func printWindow(hwnd uintptr) (*image.RGBA, error) {
	size, err := clientSize(hwnd)
	if err != nil {
		return nil, err
	}
	return capture(size, func(mem uintptr) error {
		if ok, _, err := procLivePrintWindow.Call(hwnd, mem, pwClientOnly|pwRenderFullContent); ok == 0 {
			return fmt.Errorf("PrintWindow: %v", err)
		}
		return nil
	})
}

// screenBlt reads the screen where the window's client area is: the
// window must be on top.
func screenBlt(hwnd uintptr) (*image.RGBA, error) {
	size, err := clientSize(hwnd)
	if err != nil {
		return nil, err
	}
	var origin [2]int32
	procLiveClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	return capture(size, func(mem uintptr) error {
		screen, _, _ := procLiveGetDC.Call(0)
		defer procLiveReleaseDC.Call(0, screen)
		if ok, _, err := procLiveBitBlt.Call(mem, 0, 0, uintptr(size.X), uintptr(size.Y), screen, uintptr(origin[0]), uintptr(origin[1]), liveSrcCopy|liveCaptureBlt); ok == 0 {
			return fmt.Errorf("BitBlt: %v", err)
		}
		return nil
	})
}
