//go:build windows

package win32

import (
	"image"
	"image/color"
	"slices"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of window control (windowctl.go): they drive the real window
// with the messages Windows sends, like live_test.go, and read the result
// back from Windows.

var (
	procIsWindowEnabled = user32.NewProc("IsWindowEnabled")
)

const wmGetIcon = 0x007F

// screenLParam packs a screen position (physical pixels) like the
// non-client mouse messages and WM_NCHITTEST.
func screenLParam(p image.Point) uintptr { return mouseLParam(p.X, p.Y) }

// screenAt returns the screen position of a logical client point of w.
func screenAt(w *Window, x, y float64) image.Point {
	s := w.scale()
	return clientOrigin(w.hwnd).Add(image.Pt(int(x*s), int(y*s)))
}

// TestFramelessHitTest: a frameless window's whole window is client area,
// WM_NCHITTEST answers the resize band and the hit regions, and the
// non-client messages over the app's buttons arrive as pointer events.
func TestFramelessHitTest(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var events []platform.PointerEvent
	done := false
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 400, H: 300}, Frameless: true})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
		w.SetHitRegions(titleBar)
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			events = append(events, e)
		case platform.FrameEvent:
			presentSolid(t, ew, color.RGBA{0x30, 0x30, 0x30, 0xff})
			if done {
				return
			}
			done = true
			if st := w.State(); !st.ClientSideDecorations || !st.HasPosition {
				t.Errorf("frameless window state = %v, want csd with a position", st)
			}
			c, wr := getClientRect(w.hwnd), getWindowRect(w.hwnd)
			if c.width() != wr.width() || c.height() != wr.height() {
				t.Errorf("client %dx%d, window %dx%d: a frameless window is all client area", c.width(), c.height(), wr.width(), wr.height())
			}
			for _, tc := range []struct {
				x, y float64
				want uintptr
			}{
				{0, 150, htLeft},
				{200, 0, htTop},
				{399.5, 299.5, htBottomRight},
				{200, 16, htCaption},
				{150, 16, htClient},
				{290, 16, htMinButton},
				{330, 16, htMaxButton},
				{370, 16, htClose},
				{200, 200, htClient},
			} {
				if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(screenAt(w, tc.x, tc.y))); got != tc.want {
					t.Errorf("WM_NCHITTEST at %v,%v = %d, want %d", tc.x, tc.y, got, tc.want)
				}
			}
			// Hover, press and release on the app's close button.
			p := screenAt(w, 370, 16)
			sendMessage(w.hwnd, wmNCMouseMove, htClose, screenLParam(p))
			sendMessage(w.hwnd, wmNCLButtonDown, htClose, screenLParam(p))
			sendMessage(w.hwnd, wmLButtonUp, 0, mouseLParam(p.Sub(clientOrigin(w.hwnd)).X, p.Sub(clientOrigin(w.hwnd)).Y))
			b.Wake()
		case platform.WakeEvent:
			b.Quit()
		}
	})
	var actions []platform.PointerAction
	for _, e := range events {
		actions = append(actions, e.Action)
	}
	// The real mouse is elsewhere: the WM_NCMOUSELEAVE that the tracking
	// sends may add a leave.
	if len(actions) == 5 && actions[4] == platform.PointerLeave {
		actions, events = actions[:4], events[:4]
	}
	want := []platform.PointerAction{platform.PointerEnter, platform.PointerMove, platform.PointerDown, platform.PointerUp}
	if !slices.Equal(actions, want) {
		t.Fatalf("pointer actions over the close button = %v, want %v", actions, want)
	}
	for _, e := range events {
		if e.Pos.X < 369 || e.Pos.X > 371 || e.Pos.Y < 15 || e.Pos.Y > 17 {
			t.Errorf("%v at %v, want about 370,16", e.Action, e.Pos)
		}
	}
	if events[2].Button != platform.ButtonPrimary {
		t.Errorf("press button = %v", events[2].Button)
	}
}

// TestWindowStateRequests: the requests change the window and are reported
// in the state (read with the loop's pending state, right after the
// synchronous calls).
func TestWindowStateRequests(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	done := false
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 320, H: 240}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || done {
			if _, wake := e.(platform.WakeEvent); wake {
				b.Quit()
			}
			return
		}
		presentSolid(t, ew, color.RGBA{0, 0, 0, 0xff})
		done = true
		state := func() platform.WindowState { return b.loop.PendingState(w) }

		pos := state().Position.Add(image.Pt(17, 23))
		w.SetPosition(pos)
		if got := clientOrigin(w.hwnd); got != pos {
			t.Errorf("content origin after SetPosition(%v) = %v", pos, got)
		}
		if st := state(); !st.HasPosition || st.Position != pos {
			t.Errorf("state after SetPosition(%v) = %v", pos, st)
		}
		normal := getClientRect(w.hwnd)

		w.SetMaximized(true)
		if st := state(); !st.Maximized || !isZoomed(w.hwnd) {
			t.Errorf("state after SetMaximized(true) = %v", st)
		}
		if st := state(); st.Position != pos {
			t.Errorf("a maximized window reports %v, want its normal position %v", st.Position, pos)
		}
		w.SetMaximized(false)
		if st := state(); st.Maximized || isZoomed(w.hwnd) || st.Position != pos {
			t.Errorf("state after SetMaximized(false) = %v, want normal at %v", st, pos)
		}

		w.SetFullscreen(true)
		mi := monitorInfoOf(monitorFromWindow(w.hwnd))
		if st := state(); !st.Fullscreen {
			t.Errorf("state after SetFullscreen(true) = %v", st)
		}
		if c, m := getClientRect(w.hwnd), mi.monitor; c.width() != m.width() || c.height() != m.height() {
			t.Errorf("fullscreen client %dx%d, monitor %dx%d", c.width(), c.height(), m.width(), m.height())
		}
		w.SetMaximized(true) // the state it returns to
		if st := state(); !st.Fullscreen || !st.Maximized {
			t.Errorf("state after SetMaximized(true) in fullscreen = %v", st)
		}
		w.SetMaximized(false)
		w.SetFullscreen(false)
		if st := state(); st.Fullscreen || st.Maximized || st.Position != pos {
			t.Errorf("state after SetFullscreen(false) = %v, want normal at %v", st, pos)
		}
		if c := getClientRect(w.hwnd); c != normal {
			t.Errorf("client after fullscreen = %v, want %v", c, normal)
		}
		if s := getWindowLong(w.hwnd, gwlStyle); s&wsCaption != wsCaption || s&wsThickFrame == 0 {
			t.Errorf("style after fullscreen %#x lost the frame", s)
		}

		w.SetAlwaysOnTop(true)
		if getWindowLong(w.hwnd, gwlExStyle)&wsExTopmost == 0 {
			t.Errorf("SetAlwaysOnTop(true) did not make the window topmost")
		}
		w.SetAlwaysOnTop(false)
		if getWindowLong(w.hwnd, gwlExStyle)&wsExTopmost != 0 {
			t.Errorf("SetAlwaysOnTop(false) left the window topmost")
		}

		w.SetIcon([]image.Image{image.NewNRGBA(image.Rect(0, 0, 16, 16)), image.NewNRGBA(image.Rect(0, 0, 32, 32)), image.NewNRGBA(image.Rect(0, 0, 256, 256))})
		if sendMessage(w.hwnd, wmGetIcon, iconSmall, 0) == 0 || sendMessage(w.hwnd, wmGetIcon, iconBig, 0) == 0 {
			t.Errorf("SetIcon did not install both icons")
		}
		w.SetIcon(nil)
		if sendMessage(w.hwnd, wmGetIcon, iconSmall, 0) != 0 || sendMessage(w.hwnd, wmGetIcon, iconBig, 0) != 0 {
			t.Errorf("SetIcon(nil) left an icon")
		}

		w.SetMaxSize(platform.Size{W: 200, H: 150})
		if c, want := getClientRect(w.hwnd), clampPhysical(platform.PhysicalSize(platform.Size{W: 200, H: 150}, w.scale())); int(c.width()) != want.X || int(c.height()) != want.Y {
			t.Errorf("client after SetMaxSize = %dx%d, want %v", c.width(), c.height(), want)
		}
		var mm minMaxInfo
		sendMessage(w.hwnd, wmGetMinMaxInfo, 0, uintptr(unsafe.Pointer(&mm)))
		if outer := w.outerSize(clampPhysical(platform.PhysicalSize(platform.Size{W: 200, H: 150}, w.scale()))); mm.maxTrackSize.x != outer.width() || mm.maxTrackSize.y != outer.height() {
			t.Errorf("max track size = %v, want %dx%d", mm.maxTrackSize, outer.width(), outer.height())
		}

		w.Minimize()
		if st := state(); !st.Minimized || st.Position != pos {
			t.Errorf("state after Minimize = %v", st)
		}
		w.Activate()
		if st := state(); st.Minimized {
			t.Errorf("state after Activate = %v, want restored", st)
		}
		b.Wake()
	})
	if !done {
		t.Error("no FrameEvent")
	}
}

// TestFramelessMaximizedFitsWorkArea: a maximized frameless window covers
// the work area of its monitor and nothing more.
func TestFramelessMaximizedFitsWorkArea(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	done := false
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 300, H: 200}, Frameless: true, Maximized: true})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, ew, color.RGBA{0, 0, 0, 0xff})
			if done {
				return
			}
			done = true
			if !w.State().Maximized {
				t.Errorf("state = %v, want maximized", w.State())
			}
			work := monitorInfoOf(monitorFromWindow(w.hwnd)).work.image()
			c := getClientRect(w.hwnd).image().Add(clientOrigin(w.hwnd))
			if c != work {
				t.Errorf("maximized frameless client %v, work area %v", c, work)
			}
			if m := w.Metrics().Physical(); m != work.Size() {
				t.Errorf("metrics %v, work area %v", m, work.Size())
			}
			b.Wake()
		case platform.WakeEvent:
			b.Quit()
		}
	})
	if !done {
		t.Error("no FrameEvent")
	}
}

// TestModalDisablesParent: a visible modal child disables its parent; the
// parent is enabled again when the child is hidden or closed.
func TestModalDisablesParent(t *testing.T) {
	b := liveBackend(t)
	var p, c *Window
	step := 0
	enabled := func() bool {
		r, _, _ := procIsWindowEnabled.Call(p.hwnd)
		return r != 0
	}
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "parent"})
		if err != nil {
			t.Fatal(err)
		}
		p = pw.(*Window)
		cw, err := b.NewWindow(platform.WindowOptions{Title: "modal", Size: platform.Size{W: 200, H: 120}, Parent: p, Modal: true})
		if err != nil {
			t.Fatal(err)
		}
		c = cw.(*Window)
		if !enabled() {
			t.Errorf("the parent is disabled before the modal child is shown")
		}
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, ew, color.RGBA{0, 0, 0, 0xff})
			if ew != c || step != 0 {
				return
			}
			step = 1
			// Centered over the parent.
			pr, cr := getWindowRect(p.hwnd).image(), getWindowRect(c.hwnd).image()
			if d := pr.Min.Add(pr.Max).Sub(cr.Min.Add(cr.Max)); d.X < -2 || d.X > 2 || d.Y < -2 || d.Y > 2 {
				t.Errorf("modal child at %v is not centered over its parent at %v", cr, pr)
			}
			if enabled() {
				t.Errorf("the parent is enabled while its modal child is shown")
			}
			c.SetVisible(false)
			if !enabled() {
				t.Errorf("the parent is disabled after its modal child was hidden")
			}
			c.SetVisible(true)
			if enabled() {
				t.Errorf("the parent is enabled after its modal child was shown again")
			}
			c.Close()
			if !enabled() {
				t.Errorf("the parent is disabled after its modal child was closed")
			}
			b.Wake()
		case platform.WakeEvent:
			b.Quit()
		}
	})
	if step != 1 {
		t.Error("the modal child got no FrameEvent")
	}
}
