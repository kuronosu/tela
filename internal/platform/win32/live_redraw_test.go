//go:build windows

package win32

import (
	"image"
	"image/color"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of the repaint after the OS lost the window's surface and of
// the frameless window's refusal to let DefWindowProc paint a caption
// (redraw.go).

var procUpdateWindow = user32.NewProc("UpdateWindow")

// wipe overwrites the whole client area on screen with zero BGRA pixels
// (black, alpha 0), as a surface that DWM discarded would look.
func wipe(w *Window) {
	size := w.Metrics().Physical()
	zero := make([]byte, 4*size.X*size.Y)
	bmi := bitmapInfo{bitmapInfoHeader: bitmapInfoHeader{size: bitmapInfoHeaderSize, width: int32(size.X), height: -int32(size.Y), planes: 1, bitCount: 32}}
	hdc, _, _ := procGetDC.Call(w.hwnd)
	defer procReleaseDC.Call(w.hwnd, hdc)
	procSetDIBitsToDevice.Call(hdc, 0, 0, uintptr(size.X), uintptr(size.Y), 0, 0, 0, uintptr(size.Y),
		uintptr(unsafe.Pointer(&zero[0])), uintptr(unsafe.Pointer(&bmi)), dibRGBColors)
}

func pixelAt(w *Window, p image.Point) uint32 {
	hdc, _, _ := procGetDC.Call(w.hwnd)
	defer procReleaseDC.Call(w.hwnd, hdc)
	c, _, _ := procGetPixel.Call(hdc, uintptr(p.X), uintptr(p.Y))
	return uint32(c)
}

// TestRepaintAfterSurfaceLoss: the messages Windows sends when a session
// reconnects, the display or the theme changes, or the composition is
// redone make the window repaint all of its client area from the retained
// copy, and make the next Present a full one, without any Present from
// the app. Frameless and decorated windows alike.
func TestRepaintAfterSurfaceLoss(t *testing.T) {
	const want = 0x906030 // COLORREF of (0x30, 0x60, 0x90)
	msgs := []struct {
		name string
		msg  uint32
		full bool
	}{
		{"WM_DWMCOMPOSITIONCHANGED", wmDwmCompositionChanged, true},
		{"WM_THEMECHANGED", wmThemeChanged, true},
		{"WM_DISPLAYCHANGE", wmDisplayChange, true},
		{"WM_SETTINGCHANGE", wmSettingChange, false},
		{"WM_NCACTIVATE", wmNCActivate, false},
	}
	for _, frameless := range []bool{true, false} {
		b := liveBackend(t)
		var w *Window
		stage := 0
		runLive(t, b, func() {
			pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 120, H: 90}, Frameless: frameless})
			if err != nil {
				t.Fatal(err)
			}
			w = pw.(*Window)
		}, func(ew platform.Window, e platform.Event) {
			if _, ok := e.(platform.FrameEvent); !ok || stage > 0 {
				return
			}
			stage++
			defer b.Quit()
			presentSolid(t, ew, color.RGBA{0x30, 0x60, 0x90, 0xff})
			probe := image.Pt(60, 45)
			if pixelAt(w, probe) == 0xFFFFFFFF {
				t.Skip("GetPixel cannot read this window")
			}
			for _, m := range msgs {
				if m.msg == wmNCActivate && !frameless {
					continue // a decorated window's activation is DefWindowProc's
				}
				wipe(w)
				if got := pixelAt(w, probe); got == want {
					t.Skipf("the window cannot be wiped through its DC (pixel %#x)", got)
				}
				w.ClearNeedsFull()
				wparam := uintptr(0)
				if m.msg == wmNCActivate {
					wparam = 1
				}
				sendMessage(w.hwnd, m.msg, wparam, 0)
				procUpdateWindow.Call(w.hwnd) // answer the WM_PAINT now
				for _, p := range []image.Point{probe, {0, 0}, {3, 3}, {119, 89}, {0, 45}} {
					if got := pixelAt(w, p); got != want {
						t.Errorf("frameless=%v: after %s, pixel %v = %#06x, want %#06x", frameless, m.name, p, got, want)
					}
				}
				if m.full && !w.NeedsFull() {
					t.Errorf("frameless=%v: after %s the next Present is not full", frameless, m.name)
				}
			}
		})
	}
}

// TestFramelessKeepsCaptionUnpainted: a frameless window answers the
// messages that make DefWindowProc paint a caption, and WM_SETTEXT and
// WM_SETICON leave it visible, whether or not DWM composes.
func TestFramelessKeepsCaptionUnpainted(t *testing.T) {
	old := compositionEnabled
	defer func() { compositionEnabled = old }()
	for _, composing := range []bool{true, false} {
		compositionEnabled = func() bool { return composing }
		b := liveBackend(t)
		var w *Window
		stage := 0
		runLive(t, b, func() {
			pw, err := b.NewWindow(platform.WindowOptions{Title: "Notes", Size: platform.Size{W: 120, H: 90}, Frameless: true})
			if err != nil {
				t.Fatal(err)
			}
			w = pw.(*Window)
		}, func(ew platform.Window, e platform.Event) {
			if _, ok := e.(platform.FrameEvent); !ok || stage > 0 {
				return
			}
			stage++
			defer b.Quit()
			presentSolid(t, ew, color.RGBA{0x30, 0x60, 0x90, 0xff})
			for _, m := range []uint32{wmNCPaint, wmNCUAHDrawCaption, wmNCUAHDrawFrame} {
				if r := sendMessage(w.hwnd, m, 1, 0); r != 0 {
					t.Errorf("composing=%v: message %#x returned %d, want 0 (handled)", composing, m, r)
				}
			}
			ew.SetTitle("Renamed")
			sendMessage(w.hwnd, wmSetIcon, 0, 0)
			if !isWindowVisible(w.hwnd) || getWindowLong(w.hwnd, gwlStyle)&wsVisible == 0 {
				t.Errorf("composing=%v: the window is not visible after WM_SETTEXT and WM_SETICON", composing)
			}
			buf := make([]uint16, 32)
			n, _, _ := procGetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if got := windows.UTF16ToString(buf[:n]); got != "Renamed" {
				t.Errorf("composing=%v: title = %q, want Renamed", composing, got)
			}
		})
	}
}
