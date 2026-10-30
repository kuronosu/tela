//go:build windows

package win32

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// These tests run the real backend: on Windows, or under Wine (the CI and
// local setup uses a prefix whose graphics driver is "null", so no window
// reaches a display). They drive the window with the messages Windows itself
// sends, posted or sent to the window handle, which makes pointer, close,
// DPI and resize handling deterministic.

var (
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procGetCursor        = user32.NewProc("GetCursor")
	procGetPixel         = gdi32.NewProc("GetPixel")
	procOpenInputDesktop = user32.NewProc("OpenInputDesktop")
	procCloseDesktop     = user32.NewProc("CloseDesktop")
)

// requireInteractiveDesktop skips the test when this process has no
// interactive input desktop, which is the case in a service or session 0
// (some hosted CI runners). Windows cannot show or focus a window there, and
// SendInput goes nowhere. Wine answers with its virtual desktop, so the
// tests keep running under Wine.
func requireInteractiveDesktop(t *testing.T) {
	t.Helper()
	const desktopReadObjects = 0x0001
	d, _, _ := procOpenInputDesktop.Call(0, 0, desktopReadObjects)
	if d == 0 {
		t.Skip("no interactive input desktop (service or session 0): live window tests need one")
	}
	procCloseDesktop.Call(d)
}

func liveBackend(t *testing.T) *Backend {
	t.Helper()
	requireInteractiveDesktop(t)
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if !b.ran {
			b.Quit()
			_ = b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {}))
		}
	})
	return b
}

// runLive calls setup and then Run with h, failing the test if Run does not
// return within the timeout.
func runLive(t *testing.T, b *Backend, setup func(), h func(w platform.Window, e platform.Event)) {
	t.Helper()
	var timedOut atomic.Bool
	timer := time.AfterFunc(15*time.Second, func() {
		timedOut.Store(true)
		b.Quit()
	})
	defer timer.Stop()
	if setup != nil {
		setup()
	}
	if err := b.Run(platform.HandlerFunc(h)); err != nil {
		t.Errorf("Run returned error: %v", err)
	}
	if timedOut.Load() {
		t.Fatalf("Run did not finish within the timeout")
	}
}

func presentSolid(t *testing.T, w platform.Window, c color.RGBA) {
	t.Helper()
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	for i := 0; i < len(fb.Pix); i += 4 {
		fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	if err := w.Present(fb, nil); err != nil {
		t.Errorf("Present: %v", err)
	}
}

func wheelWParam(delta int) uintptr { return uintptr(uint16(int16(delta))) << 16 }

func mouseLParam(x, y int) uintptr { return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16 }

func TestConformance(t *testing.T) {
	platformtest.Run(t, platformtest.Config{
		New: func(t *testing.T) platform.Backend { return liveBackend(t) },
	})
}

// TestStructLayout pins the sizes of the structures passed to Windows: a
// wrong field order or width would corrupt calls silently.
func TestStructLayout(t *testing.T) {
	ptr := unsafe.Sizeof(uintptr(0))
	pick := func(w64, w32 uintptr) uintptr {
		if ptr == 8 {
			return w64
		}
		return w32
	}
	for name, tc := range map[string]struct{ got, want uintptr }{
		"point":            {unsafe.Sizeof(point{}), 8},
		"rect":             {unsafe.Sizeof(rect{}), 16},
		"msg":              {unsafe.Sizeof(msg{}), pick(48, 28)},
		"wndClassEx":       {unsafe.Sizeof(wndClassEx{}), pick(80, 48)},
		"paintStruct":      {unsafe.Sizeof(paintStruct{}), pick(72, 64)},
		"trackMouseEvent":  {unsafe.Sizeof(trackMouseEvent{}), pick(24, 16)},
		"minMaxInfo":       {unsafe.Sizeof(minMaxInfo{}), 40},
		"bitmapInfoHeader": {unsafe.Sizeof(bitmapInfoHeader{}), 40},
		"bitmapInfo":       {unsafe.Sizeof(bitmapInfo{}), 44},
		"devMode":          {unsafe.Sizeof(devMode{}), 220},
		"compositionForm":  {unsafe.Sizeof(compositionForm{}), 28},
		"candidateForm":    {unsafe.Sizeof(candidateForm{}), 32},
		"imeCharPosition":  {unsafe.Sizeof(imeCharPosition{}), 36},
		"windowPlacement":  {unsafe.Sizeof(windowPlacement{}), 44},
		"monitorInfo":      {unsafe.Sizeof(monitorInfo{}), 40},
		"flashWInfo":       {unsafe.Sizeof(flashWInfo{}), pick(32, 20)},
		"iconInfo":         {unsafe.Sizeof(iconInfo{}), pick(32, 20)},
	} {
		if tc.got != tc.want {
			t.Errorf("sizeof(%s) = %d, want %d", name, tc.got, tc.want)
		}
	}
	var dm devMode
	if off := unsafe.Offsetof(dm.displayFrequency); off != 184 {
		t.Errorf("offsetof(devMode.displayFrequency) = %d, want 184", off)
	}
	if off := unsafe.Offsetof(dm.size); off != 68 {
		t.Errorf("offsetof(devMode.size) = %d, want 68", off)
	}
	var m msg
	if unsafe.Offsetof(m.pt) != pick(36, 20) {
		t.Errorf("offsetof(msg.pt) = %d", unsafe.Offsetof(m.pt))
	}
}

func TestProcessSetup(t *testing.T) {
	b := liveBackend(t)
	if b.interval < time.Second/1000 || b.interval > time.Second/24 {
		t.Errorf("display interval %v is out of range", b.interval)
	}
	if dpi := primaryDPI(); dpi < baseDPI/2 || dpi > 20*baseDPI {
		t.Errorf("primary DPI %d is out of range", dpi)
	}
	if s := b.Settings(); s.DoubleClickInterval <= 0 {
		t.Errorf("double click interval = %v, want a positive duration", s.DoubleClickInterval)
	}
	if b.Name() != "win32" {
		t.Errorf("Name() = %q", b.Name())
	}
}

// TestPointerMessages feeds the window the mouse messages Windows sends and
// checks the platform events, in logical pixels.
func TestPointerMessages(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var got []platform.Event
	posted := false
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "pointer", Size: platform.Size{W: 400, H: 300}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, ew, color.RGBA{0, 0, 0, 0xff})
			if posted {
				return
			}
			posted = true
			scale := ew.Metrics().Scale
			px := func(v int) int { return int(float64(v) * scale) }
			x, y := px(40), px(30)
			// Move, press, drag outside, release, wheel, middle press.
			postMessage(w.hwnd, wmMouseMove, 0, mouseLParam(x, y))
			postMessage(w.hwnd, wmLButtonDown, 0x0001, mouseLParam(x, y))
			postMessage(w.hwnd, wmMouseMove, 0x0001, mouseLParam(-px(10), y))
			postMessage(w.hwnd, wmLButtonUp, 0, mouseLParam(-px(10), y))
			var origin point
			procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
			wheelPos := mouseLParam(int(origin.x)+x, int(origin.y)+y)
			postMessage(w.hwnd, wmMouseWheel, wheelWParam(-120), wheelPos)
			postMessage(w.hwnd, wmMouseHWheel, wheelWParam(60), wheelPos)
			postMessage(w.hwnd, wmXButtonDown, uintptr(xButton1)<<16, mouseLParam(x, y))
			postMessage(w.hwnd, wmXButtonUp, uintptr(xButton1)<<16, mouseLParam(x, y))
			// A Wake now would be delivered with the first message.
			time.AfterFunc(200*time.Millisecond, b.Wake)
		case platform.PointerEvent, platform.ScrollEvent:
			got = append(got, e)
		case platform.WakeEvent:
			b.Quit()
		}
	})

	scale := w.Metrics().Scale
	px := func(v float32) float32 { return float32(float64(int(float64(v)*scale)) / scale) }
	pos := platform.Point{X: px(40), Y: px(30)}
	out := platform.Point{X: float32(float64(-int(10*scale)) / scale), Y: pos.Y}
	primary := platform.ButtonPrimary.Mask()
	back := platform.ButtonBack.Mask()
	want := []platform.Event{
		platform.PointerEvent{Action: platform.PointerEnter, Pos: pos},
		platform.PointerEvent{Action: platform.PointerMove, Pos: pos},
		platform.PointerEvent{Action: platform.PointerDown, Pos: pos, Button: platform.ButtonPrimary, Buttons: primary},
		platform.PointerEvent{Action: platform.PointerMove, Pos: out, Buttons: primary},
		platform.PointerEvent{Action: platform.PointerUp, Pos: out, Button: platform.ButtonPrimary},
		platform.PointerEvent{Action: platform.PointerLeave, Pos: out},
		platform.ScrollEvent{Pos: pos, Steps: platform.Point{Y: 1}},
		platform.ScrollEvent{Pos: pos, Steps: platform.Point{X: 0.5}},
		platform.PointerEvent{Action: platform.PointerEnter, Pos: pos},
		platform.PointerEvent{Action: platform.PointerDown, Pos: pos, Button: platform.ButtonBack, Buttons: back},
		platform.PointerEvent{Action: platform.PointerUp, Pos: pos, Button: platform.ButtonBack},
	}
	// Times and modifiers depend on the run.
	for i := range got {
		switch e := got[i].(type) {
		case platform.PointerEvent:
			if e.Time <= 0 || e.Kind != platform.PointerMouse {
				t.Errorf("event %d: Time %v, Kind %v", i, e.Time, e.Kind)
			}
			e.Time, e.Mods = 0, 0
			got[i] = e
		case platform.ScrollEvent:
			if e.Time <= 0 || e.Kind != platform.PointerMouse {
				t.Errorf("event %d: Time %v, Kind %v", i, e.Time, e.Kind)
			}
			e.Time, e.Mods, e.Kind = 0, 0, 0
			got[i] = e
		}
		if i < len(want) {
			if pe, ok := want[i].(platform.PointerEvent); ok {
				pe.Kind = platform.PointerMouse
				want[i] = pe
			}
		}
	}
	// The messages are synthetic: on a real desktop the real cursor is not
	// over the window, so Windows answers the TrackMouseEvent armed by the
	// last move with an immediate WM_MOUSELEAVE, which the backend reports
	// as a trailing leave. (Real mouse messages only arrive while the cursor
	// is over the window, where tracking is correct.) Wine sends none.
	if n := len(got); n == len(want)+1 {
		if last, ok := got[n-1].(platform.PointerEvent); ok && last.Action == platform.PointerLeave && last.Buttons == 0 {
			got = got[:n-1]
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d:\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v\n         want %+v", i, got[i], want[i])
		}
	}
}

func TestCloseRequested(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	closeRequests := 0
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, ew, color.RGBA{0, 0, 0, 0xff})
			if closeRequests == 0 {
				// What the close button and Alt+F4 do.
				postMessage(w.hwnd, wmClose, 0, 0)
			}
		case platform.CloseRequestedEvent:
			closeRequests++
			// The window stays open until the app closes it.
			if r, _, _ := procIsIconic.Call(w.hwnd); r != 0 {
				t.Errorf("window is iconic")
			}
			ew.Close()
			b.Wake()
		case platform.WakeEvent:
			b.Quit()
		}
	})
	if closeRequests != 1 {
		t.Errorf("got %d CloseRequestedEvents, want 1", closeRequests)
	}
	registry.mu.RLock()
	_, stillThere := registry.windows[w.hwnd]
	registry.mu.RUnlock()
	if stillThere {
		t.Error("the closed window is still registered")
	}
}

// TestPanicInHandlerIsReraisedByRun: the handler's panic cannot cross the
// Windows frames above WndProc; Run raises it again with the same value, and
// the backend still releases its resources.
func TestPanicInHandlerIsReraisedByRun(t *testing.T) {
	type sentinel struct{ n int }
	for name, panicOn := range map[string]func(e platform.Event) bool{
		// The very first pass runs inside Run, outside any WndProc.
		"first pass": func(e platform.Event) bool { _, ok := e.(platform.FrameEvent); return ok },
		// A later one runs inside WndProc, called by DispatchMessageW.
		"in WndProc": func(e platform.Event) bool { _, ok := e.(platform.WakeEvent); return ok },
	} {
		t.Run(name, func(t *testing.T) {
			b := liveBackend(t)
			want := sentinel{42}
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				runLive(t, b, func() {
					if _, err := b.NewWindow(platform.WindowOptions{}); err != nil {
						t.Fatal(err)
					}
					b.Wake()
				}, func(w platform.Window, e platform.Event) {
					if f, ok := e.(platform.FrameEvent); ok {
						_ = f
						presentSolid(t, w, color.RGBA{0, 0, 0, 0xff})
					}
					if panicOn(e) {
						panic(want)
					}
				})
			}()
			if recovered != want {
				t.Fatalf("Run panicked with %v (%T), want %v", recovered, recovered, want)
			}
			if !b.closed {
				t.Error("the backend was not shut down after the panic")
			}
		})
	}
}

// TestResizeInsideWndProc: a size change that Windows delivers while the app
// is not running (as during an interactive resize, where only WndProc runs)
// reaches the app as a ConfigureEvent and the FrameEvent it forces, and the
// app's Present fills the new size. The size change comes from another
// thread, which makes Windows send WM_SIZE to the UI thread's WndProc.
func TestResizeInsideWndProc(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var configures []platform.Metrics
	frames, resized := 0, false
	var dibSize image.Point
	var after int // FrameEvents after the resize's ConfigureEvent
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 300, H: 200}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			configures = append(configures, e.Metrics)
		case platform.FrameEvent:
			frames++
			presentSolid(t, ew, color.RGBA{0x10, 0x80, 0x20, 0xff})
			if !resized {
				resized = true
				hwnd := w.hwnd
				go func() {
					// Cross-thread SetWindowPos blocks until the UI
					// thread, idle in GetMessageW, handles the sent
					// messages.
					outer := w.outerSize(image.Pt(450, 330))
					setWindowPos(hwnd, 0, 0, outer.width(), outer.height(), swpNoMove|swpNoZOrder|swpNoActivate)
					b.Wake()
				}()
			} else {
				after++
			}
		case platform.WakeEvent:
			if resized && after > 0 {
				dibSize = w.dib.size
				b.Quit()
			}
		}
	})
	if len(configures) < 2 {
		t.Fatalf("got %d ConfigureEvents, want the initial one and the resize: %v", len(configures), configures)
	}
	last := configures[len(configures)-1]
	if got := last.Physical(); got != image.Pt(450, 330) {
		t.Errorf("resize reported %v (%v), want a 450x330 client area", last, got)
	}
	if after < 1 {
		t.Errorf("the resize forced no FrameEvent")
	}
	if dibSize != image.Pt(450, 330) {
		t.Errorf("retained copy is %v after the resize, want 450x330", dibSize)
	}
}

// TestDPIChanged: WM_DPICHANGED (the window moved to another monitor or the
// user changed the scale) applies the suggested rectangle and reports the new
// scale and the matching logical size.
func TestDPIChanged(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var last platform.Metrics
	var client image.Point
	wantWidth := 0 // the client width of the suggested rectangle
	changed := false
	frames := 0
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 300, H: 200}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			last = e.Metrics
		case platform.FrameEvent:
			frames++
			presentSolid(t, ew, color.RGBA{0x10, 0x20, 0x80, 0xff}) // fails the test on a size mismatch
			if !changed {
				changed = true
				base := ew.Metrics()
				// 1.5 times the physical size, at 144 dpi: the rectangle
				// Windows would suggest when a window on a 96 dpi monitor
				// enters a 144 dpi one (the monitor may be at another scale:
				// the check below follows the base size).
				wantWidth = base.Physical().X * 3 / 2
				outer := adjustWindowRect(rect{right: int32(base.Physical().X * 3 / 2), bottom: int32(base.Physical().Y * 3 / 2)}, w.style, w.exStyle, 144)
				suggested := &rect{left: 50, top: 60, right: 50 + outer.width(), bottom: 60 + outer.height()}
				go func() {
					procSendMessageW.Call(w.hwnd, wmDPIChanged, 144|144<<16, uintptr(unsafe.Pointer(suggested)))
					b.Wake()
				}()
			} else {
				r := getClientRect(w.hwnd)
				client = image.Pt(int(r.width()), int(r.height()))
				b.Quit()
			}
		}
	})
	if last.Scale != 1.5 {
		t.Fatalf("last ConfigureEvent %v, want scale 1.5", last)
	}
	// The suggested rectangle here is synthetic, so the client area is
	// whatever Windows left of it; the report must match it exactly.
	if last.Physical() != client || last.Size != platform.LogicalSize(client, 1.5) {
		t.Errorf("after WM_DPICHANGED: %v (%v), want the logical size of the %v client area", last, last.Physical(), client)
	}
	if client.X < wantWidth-10 || client.X > wantWidth+10 {
		t.Errorf("the suggested rectangle was not applied: client area %v, want a width of about %d", client, wantWidth)
	}
	if frames < 2 {
		t.Errorf("got %d frames, want one after the DPI change", frames)
	}
}

// TestIdleLoopSleeps: without events the loop blocks in GetMessageW. Frames
// requested every frame keep coming, paced, and stop with the requests.
func TestIdleLoopSleeps(t *testing.T) {
	b := liveBackend(t)
	var iterIdle int64
	frames := 0
	var animStart time.Time
	var animEnd time.Time
	var ticks int
	runLive(t, b, func() {
		if _, err := b.NewWindow(platform.WindowOptions{}); err != nil {
			t.Fatal(err)
		}
	}, func(w platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			frames++
			presentSolid(t, w, color.RGBA{0, 0, 0, 0xff})
			if frames == 1 {
				time.AfterFunc(300*time.Millisecond, b.Wake)
			}
		case platform.WakeEvent:
			ticks++
			switch ticks {
			case 1:
				// 300 ms of silence.
				iterIdle = b.iterations.Load()
				animStart = time.Now()
				w := b.loop.Windows()[0]
				w.RequestFrame()
				frames = 100 // the animation counts from here
			case 2:
				animEnd = time.Now()
				b.Quit()
			}
		}
		if f, ok := e.(platform.FrameEvent); ok && frames > 100 {
			_ = f
			if frames < 100+20 {
				b.loop.Windows()[0].RequestFrame()
			} else {
				b.Wake()
			}
		}
	})
	// The loop woke up for the window being shown (a handful of messages),
	// then slept: it must not have iterated hundreds of times in 300 ms.
	if iterIdle > 60 {
		t.Errorf("the loop returned from GetMessageW %d times before the first idle period ended; want a handful (no polling)", iterIdle)
	}
	// 20 paced frames take about 20 refresh intervals, not 20 spins.
	elapsed := animEnd.Sub(animStart)
	if min := 10 * b.interval; elapsed < min {
		t.Errorf("20 requested frames took %v, less than %v: frames are not paced by the display (interval %v)", elapsed, min, b.interval)
	}
	t.Logf("idle iterations %d; 20 frames in %v (interval %v)", iterIdle, elapsed, b.interval)
}

func TestTitleAndStyles(t *testing.T) {
	b := liveBackend(t)
	var fixed, normal *Window
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "héllo ✓", Size: platform.Size{W: 200, H: 150}, MinSize: platform.Size{W: 120, H: 90}})
		if err != nil {
			t.Fatal(err)
		}
		normal = pw.(*Window)
		pw, err = b.NewWindow(platform.WindowOptions{FixedSize: true})
		if err != nil {
			t.Fatal(err)
		}
		fixed = pw.(*Window)
		b.Wake()
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.WakeEvent); !ok {
			return
		}
		checkTitleAndStyles(t, normal, fixed)
		b.Quit()
	})
}

func checkTitleAndStyles(t *testing.T, normal, fixed *Window) {
	t.Helper()
	buf := make([]uint16, 64)
	n, _, _ := procGetWindowTextW.Call(normal.hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if got := windows.UTF16ToString(buf[:n]); got != "héllo ✓" {
		t.Errorf("window text = %q", got)
	}
	normal.SetTitle("renamed")
	n, _, _ = procGetWindowTextW.Call(normal.hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if got := windows.UTF16ToString(buf[:n]); got != "renamed" {
		t.Errorf("window text after SetTitle = %q", got)
	}

	style := func(w *Window) uintptr {
		s, _, _ := procGetWindowLongW.Call(w.hwnd, uintptr(^uint32(15))) // GWL_STYLE = -16
		return s
	}
	if s := style(normal); s&wsThickFrame == 0 || s&wsMaximizeBox == 0 || s&wsCaption != wsCaption {
		t.Errorf("a normal window lost its decorations: style %#x", s)
	}
	if s := style(fixed); s&wsThickFrame != 0 || s&wsMaximizeBox != 0 {
		t.Errorf("a FixedSize window can be resized: style %#x", s)
	}

	// MinSize answers WM_GETMINMAXINFO.
	var mm minMaxInfo
	procSendMessageW.Call(normal.hwnd, wmGetMinMaxInfo, 0, uintptr(unsafe.Pointer(&mm)))
	outer := normal.outerSize(clampPhysical(platform.PhysicalSize(platform.Size{W: 120, H: 90}, normal.scale())))
	if mm.minTrackSize.x != outer.width() || mm.minTrackSize.y != outer.height() {
		t.Errorf("min track size = %v, want %dx%d", mm.minTrackSize, outer.width(), outer.height())
	}

	// Closed windows ignore everything.
	normal.Close()
	normal.SetTitle("x")
	normal.SetSize(platform.Size{W: 1, H: 1})
	normal.SetCursor(platform.CursorWait)
	normal.RequestFrame()
}

// TestSetCursor: WM_SETCURSOR in the client area installs the cursor the app
// chose; elsewhere Windows decides.
func TestSetCursor(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var ok bool
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		if _, isFrame := e.(platform.FrameEvent); !isFrame {
			return
		}
		presentSolid(t, ew, color.RGBA{0, 0, 0, 0xff})
		ew.SetCursor(platform.CursorPointer)
		r, _, _ := procSendMessageW.Call(w.hwnd, wmSetCursor, w.hwnd, htClient)
		hand, _, _ := procLoadCursorW.Call(0, idcHand)
		cur, _, _ := procGetCursor.Call()
		ok = r == 1 && cur == hand && hand != 0
		ew.SetCursor(platform.CursorNone)
		procSendMessageW.Call(w.hwnd, wmSetCursor, w.hwnd, htClient)
		if cur, _, _ := procGetCursor.Call(); cur != 0 {
			t.Errorf("CursorNone left cursor %#x visible", cur)
		}
		b.Quit()
	})
	if !ok {
		t.Error("WM_SETCURSOR in the client area did not install the pointer cursor")
	}
}

// TestPresentIsReadBack draws a pattern with a partial update and reads the
// window's pixels back with GetPixel, which proves the BGRA conversion and
// the blit position. Windows without a drawable surface (a driver that does
// not keep window contents) report CLR_INVALID, and the test skips.
func TestPresentIsReadBack(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var colors [3]uint32
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 64, H: 48}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		p := ew.Metrics().Physical()
		fb := image.NewRGBA(image.Rectangle{Max: p})
		for i := 0; i < len(fb.Pix); i += 4 {
			fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = 0xff, 0x00, 0x00, 0xff // red
		}
		if err := ew.Present(fb, nil); err != nil {
			t.Fatal(err)
		}
		// Blue square in a damage rectangle only.
		sq := image.Rect(10, 8, 30, 20)
		for y := sq.Min.Y; y < sq.Max.Y; y++ {
			for x := sq.Min.X; x < sq.Max.X; x++ {
				i := fb.PixOffset(x, y)
				fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2] = 0x00, 0x00, 0xff
			}
		}
		if err := ew.Present(fb, []image.Rectangle{sq}); err != nil {
			t.Fatal(err)
		}
		hdc, _, _ := procGetDC.Call(w.hwnd)
		defer procReleaseDC.Call(w.hwnd, hdc)
		for i, pt := range []image.Point{{20, 14}, {5, 5}, {30, 20}} {
			c, _, _ := procGetPixel.Call(hdc, uintptr(pt.X), uintptr(pt.Y))
			colors[i] = uint32(c)
		}
		b.Quit()
	})
	if colors[0] == 0xFFFFFFFF {
		t.Skip("GetPixel cannot read this window (no drawable surface)")
	}
	const red, blue = 0x0000FF, 0xFF0000 // COLORREF is 0x00BBGGRR
	if colors != [3]uint32{blue, red, red} {
		t.Errorf("pixels = %#06x, want blue inside the damaged square, red outside: %#06x", colors, [3]uint32{blue, red, red})
	}
}

// TestRunAfterRun: a Run that ended by Quit from inside WndProc leaves
// nothing in the thread's message queue (PostQuitMessage's WM_QUIT) that
// would stop the next Run on the same thread at once. On Windows a
// PeekMessage limited to WM_QUIT does not remove it, so shutdown drains the
// queue.
func TestRunAfterRun(t *testing.T) {
	for i := range 3 {
		b := liveBackend(t)
		pw, err := b.NewWindow(platform.WindowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		hwnd := pw.(*Window).hwnd
		start := time.Now()
		woken := false
		runLive(t, b, nil, func(w platform.Window, e platform.Event) {
			switch e.(type) {
			case platform.FrameEvent:
				presentSolid(t, w, color.RGBA{0, 0, 0, 0xff})
				if i == 1 {
					postMessage(hwnd, wmClose, 0, 0) // the close button
				} else {
					time.AfterFunc(80*time.Millisecond, b.Wake)
				}
			case platform.CloseRequestedEvent:
				w.Close() // the app closes its window, then quits
				time.AfterFunc(80*time.Millisecond, b.Wake)
			case platform.WakeEvent:
				woken = true
				b.Quit() // from the WM_APP message, inside WndProc
			}
		})
		if !woken || time.Since(start) < 70*time.Millisecond {
			t.Fatalf("run %d ended after %v without its wake", i, time.Since(start))
		}
		var m msg
		if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0); r != 0 {
			t.Fatalf("run %d left message %#x in the thread's queue; the next Run on this thread would see it", i, m.message)
		}
	}
}
