//go:build windows

package win32

import (
	"fmt"
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Live test of a frameless window that is deactivated and disabled, as a
// modal dialog does (a native file dialog with the window as its owner). The
// window's pixels must stay the app's: DefWindowProc must not paint a
// classic caption and frame over them. The window is read back from the
// screen, so it is what DWM composes that is checked.

var (
	procBitBlt              = gdi32.NewProc("BitBlt")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBmp = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procGetDIBits           = gdi32.NewProc("GetDIBits")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
)

const (
	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000
)

// captureScreenRect reads r of the screen as DWM composes it.
func captureScreenRect(r image.Rectangle) *image.RGBA {
	w, h := r.Dx(), r.Dy()
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBmp.Call(screen, uintptr(w), uintptr(h))
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	procBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(r.Min.X), uintptr(r.Min.Y), srcCopy|captureBlt)
	procSelectObject.Call(mem, old) // GetDIBits needs the bitmap deselected
	bgra := make([]byte, 4*w*h)
	bmi := bitmapInfo{bitmapInfoHeader: bitmapInfoHeader{size: bitmapInfoHeaderSize, width: int32(w), height: -int32(h), planes: 1, bitCount: 32}}
	procGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&bgra[0])), uintptr(unsafe.Pointer(&bmi)), dibRGBColors)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(bgra); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bgra[i+2], bgra[i+1], bgra[i], 0xff
	}
	return img
}

// modalProbes are the points of a w x h frameless window where a classic
// caption (about 30 px tall) and frame (about 7 px wide) would show.
func modalProbes(w, h int) []image.Point {
	var ps []image.Point
	for _, x := range []int{40, w / 2, w - 40} {
		for _, y := range []int{2, 8, 15, 25, 29} {
			ps = append(ps, image.Pt(x, y))
		}
	}
	for _, d := range []int{1, 3, 6} {
		ps = append(ps, image.Pt(d, h/2), image.Pt(w-1-d, h/2), image.Pt(w/2, h-1-d))
	}
	return ps
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// modalPixelsLost returns the probes of the screen capture that differ from want.
func modalPixelsLost(screen *image.RGBA, size image.Point, want color.RGBA) (bad []string) {
	for _, p := range modalProbes(size.X, size.Y) {
		c := screen.RGBAAt(p.X, p.Y)
		if absInt(int(c.R)-int(want.R)) > 3 || absInt(int(c.G)-int(want.G)) > 3 || absInt(int(c.B)-int(want.B)) > 3 {
			bad = append(bad, fmt.Sprintf("%v=%02x%02x%02x", p, c.R, c.G, c.B))
		}
	}
	return bad
}

// TestLiveFramelessModalKeepsPixels: a frameless window whose owner state
// changes the way a modal dialog changes it (disabled and deactivated, then
// enabled and activated again) keeps the app's pixels in the area where a
// classic caption and frame would be painted, composition on or off.
func TestLiveFramelessModalKeepsPixels(t *testing.T) {
	requireShell(t)
	for _, mode := range []string{"disabled", "filedialog"} {
		t.Run(mode, func(t *testing.T) { framelessModal(t, mode) })
	}
}

func framelessModal(t *testing.T, mode string) {
	if mode == "filedialog" {
		// The real file dialog and the screen capture behind it.
		requireLiveShell(t)
	}
	b := liveBackend(t)
	want := color.RGBA{0x30, 0x60, 0x90, 0xff}
	size := image.Pt(400, 300)
	var (
		main     *Window
		started  atomic.Bool
		problems = make(chan string, 16)
		done     = make(chan struct{})
		requests = make(chan func(), 4)
	)
	// onLoop runs f on the loop thread, where windows and dialogs must be
	// created.
	onLoop := func(f func()) {
		requests <- f
		b.Wake()
	}
	report := func(stage string) {
		time.Sleep(700 * time.Millisecond) // let DWM compose
		r := getWindowRect(main.hwnd).image()
		if bad := modalPixelsLost(captureScreenRect(r), size, want); len(bad) > 0 {
			problems <- fmt.Sprintf("%s: %d of %d probes lost the app's pixels: %v", stage, len(bad), len(modalProbes(size.X, size.Y)), bad)
		}
	}
	drive := func() {
		defer close(done)
		defer b.Quit()
		report("before")
		switch mode {
		case "disabled":
			// What a modal dialog does to its owner: disable it, then activate
			// another window of the app.
			procEnableWindow.Call(main.hwnd, 0)
			onLoop(func() {
				_, err := b.NewWindow(platform.WindowOptions{Title: "helper", Size: platform.Size{W: 120, H: 100},
					HasPosition: true, Position: image.Pt(700, 400)})
				if err != nil {
					problems <- fmt.Sprint("helper window: ", err)
				}
			})
			report("owner disabled and deactivated")
			procEnableWindow.Call(main.hwnd, 1)
			procSetForegroundWindow.Call(main.hwnd)
			report("owner enabled and activated")
		case "filedialog":
			title := fmt.Sprintf("tela modal pixels %d", time.Now().UnixNano())
			onLoop(func() {
				err := b.Desktop().OpenFiles(platform.FileDialogOptions{Parent: main, Title: title}, func([]string, error) {})
				if err != nil {
					problems <- fmt.Sprint("OpenFiles: ", err)
				}
			})
			dlg := waitWindow("#32770", title)
			if dlg == 0 {
				problems <- "the file dialog did not open"
				return
			}
			// Out of the way: the probes must read the owner, not the dialog.
			sx, _, _ := procGetSystemMetrics.Call(0)
			procSetWindowPos.Call(dlg, 0, sx-100, 0, 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
			report("owner under the file dialog")
			postMessage(dlg, wmCommand, idCancel, 0)
			report("file dialog closed")
		}
	}
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "Notes", Size: platform.Size{W: 400, H: 300},
			Frameless: true, HasPosition: true, Position: image.Pt(60, 60)})
		if err != nil {
			t.Fatal(err)
		}
		main = pw.(*Window)
	}, func(w platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			if w != platform.Window(main) {
				presentSolid(t, w, color.RGBA{0xc0, 0x40, 0x40, 0xff}) // the helper
				return
			}
			presentSolid(t, w, want)
			if !started.Swap(true) {
				go drive()
			}
		case platform.WakeEvent:
			for {
				select {
				case f := <-requests:
					f()
				default:
					return
				}
			}
		}
	})
	<-done
	close(problems)
	for p := range problems {
		t.Error(p)
	}
}
