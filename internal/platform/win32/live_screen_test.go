//go:build windows

package win32

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The tests that read the screen (title bar colors, backdrops, the glass
// and modal pixels, the overlay's caption buttons) check what DWM
// composes, which no attribute of the window shows. They run wherever the
// process has an interactive desktop, CI's windows-latest runner included
// (its job runs the win32 tests first and alone), and not under Wine,
// whose null graphics driver composes nothing.
//
// A window of another program over the test window is read as the test's
// pixels: on the unattended runner, while other packages' tests ran beside
// these, a console's black (0c0c0c) was read where the title bar and the
// backdrop window were. So every read (screenFor) first keeps the test
// window above every other window (HWND_TOPMOST, not activated), and then
// asks the window manager which window the screen shows at the corners and
// the center of the rectangle it reads: when that is another window, the
// test fails naming it, instead of failing, or passing, on its pixels.

// requireScreen skips a test that reads the screen where there is none to
// read: under Wine. liveBackend skips a process without an interactive
// desktop.
func requireScreen(t *testing.T) {
	t.Helper()
	if inWine() {
		t.Skip("Wine's null graphics driver composes no screen to read; these checks run on Windows")
	}
}

// screenFor reads r of the screen, which must show the window hwnd (its
// client or non-client area): hwnd is raised above every other window
// first, and a read that would see another window there is reported with
// t.Errorf (safe from any goroutine), naming that window.
func screenFor(t testing.TB, hwnd uintptr, r image.Rectangle) *image.RGBA {
	t.Helper()
	if raiseForScreen(hwnd) {
		settle() // let DWM compose the new order
	}
	if err := screenShows(hwnd, r); err != nil {
		t.Errorf("%v", err)
	}
	return captureScreenRect(r)
}

// raiseForScreen puts hwnd in the topmost band without activating it, and
// reports whether it was not there yet.
func raiseForScreen(hwnd uintptr) bool {
	if getWindowLong(hwnd, gwlExStyle)&wsExTopmost != 0 {
		return false
	}
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	return true
}

// screenShows returns an error when the screen shows a window other than
// hwnd at a corner or the center of r.
//
// It asks WindowFromPoint from a thread of its own: WindowFromPoint sends
// WM_NCHITTEST to the windows of its calling thread (for HTTRANSPARENT),
// and a TitleBarOverlay window passes that to DwmDefWindowProc, which
// takes it as the pointer's and highlights the caption button there. Asked
// from the window's thread, the probes left DWM's buttons hot, fading, for
// the read that followed (the close glyph read as the hover's red c42b1c,
// the backgrounds tinted), on CI and on a desktop alike. From another
// thread WindowFromPoint sends nothing and still names the window on top.
// That thread calls nothing that sends a message (describeWindow's
// GetWindowTextW sends WM_GETTEXT to a window of this process): the
// window's thread waits for it without pumping messages.
func screenShows(hwnd uintptr, r image.Rectangle) error {
	type found struct {
		at   image.Point
		root uintptr
	}
	done := make(chan found, 1)
	go func() {
		// Locked, so that this goroutine never shares a thread with a
		// window's: the UI goroutine keeps its own thread locked (New).
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ps := []image.Point{r.Min, {r.Max.X - 1, r.Min.Y}, {r.Min.X, r.Max.Y - 1}, r.Max.Sub(image.Pt(1, 1)), {(r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2}}
		for _, p := range ps {
			hit := windowFromPoint(point{int32(p.X), int32(p.Y)})
			if root, _, _ := procGetAncestor.Call(hit, gaRoot); root != hwnd {
				done <- found{p, root}
				return
			}
		}
		done <- found{root: hwnd}
	}()
	f := <-done
	if f.root == hwnd {
		return nil
	}
	return fmt.Errorf("the screen at %v shows another window over the test window %#x: %s; the pixels read there are not the test's", f.at, hwnd, describeWindow(f.root))
}

// describeWindow names a top-level window for a failure: its handle,
// class, title and process.
func describeWindow(hwnd uintptr) string {
	if hwnd == 0 {
		return "no window (the desktop)"
	}
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	class := syscall.UTF16ToString(buf[:n])
	n, _, _ = procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	title := syscall.UTF16ToString(buf[:n])
	var pid uint32
	procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return fmt.Sprintf("%#x, class %q, title %q, process %d", hwnd, class, title, pid)
}

// TestScreenReadsOnlyTheTestWindow: the guard of every screen read. A
// window over the test window is reported, naming it, and raising the test
// window (raiseForScreen) puts it above that window, once. It needs no
// screen to read, only window positions: it also runs under Wine.
func TestScreenReadsOnlyTheTestWindow(t *testing.T) {
	b := liveBackend(t)
	var under, over *Window
	shown := 0
	runLive(t, b, func() {
		for i, p := range []image.Point{{100, 100}, {220, 160}} {
			pw, err := b.NewWindow(platform.WindowOptions{Title: fmt.Sprintf("screen guard %d", i), Size: platform.Size{W: 240, H: 160},
				Frameless: true, HasPosition: true, Position: p})
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				under = pw.(*Window)
			} else {
				over = pw.(*Window)
			}
		}
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || shown == 2 {
			return
		}
		presentSolid(t, w, color.RGBA{0x30, 0x60, 0x90, 0xff})
		if shown++; shown < 2 {
			return
		}
		defer b.Quit()
		procSetWindowPos.Call(over.hwnd, 0 /* HWND_TOP */, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		ur, or := getWindowRect(under.hwnd).image(), getWindowRect(over.hwnd).image()
		if !ur.Overlaps(or) {
			t.Fatalf("the windows do not overlap: %v and %v", ur, or)
		}
		alone := image.Rect(ur.Min.X, ur.Min.Y, or.Min.X, or.Min.Y)
		if err := screenShows(under.hwnd, alone); err != nil {
			t.Errorf("the part no window covers: %v", err)
		}
		err := screenShows(under.hwnd, ur)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%#x", over.hwnd)) {
			t.Errorf("a read under the other window %#x: %v, want an error naming it", over.hwnd, err)
		}
		if !raiseForScreen(under.hwnd) {
			t.Errorf("raiseForScreen of a window that is not topmost reported nothing to do")
		}
		if err := screenShows(under.hwnd, ur); err != nil {
			t.Errorf("after raiseForScreen: %v", err)
		}
		if raiseForScreen(under.hwnd) {
			t.Errorf("raiseForScreen of a topmost window raised it again")
		}
	})
}
