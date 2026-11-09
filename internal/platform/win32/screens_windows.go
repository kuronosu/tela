//go:build windows

package win32

import (
	"image"
	"sync"
	"syscall"

	"github.com/kuronosu/tela/internal/platform"
)

// The monitors present, for a position given at creation
// (WindowOptions.Position, a placement saved in an earlier run), which
// platform.FitOnScreens moves onto them when its monitor is gone.
//
// Windows does not do it itself: CreateWindowExW puts a window wherever it
// is told, off every monitor too (only SetWindowPlacement and the shell's
// own restore move a window into a work area). winit leaves it to the
// app; Chromium does it in WindowSizer (AdjustBoundsToBeVisibleOnDisplay).

// monitorinfofPrimary is MONITORINFOF_PRIMARY, the primary monitor's flag.
const monitorinfofPrimary = 1

// monitorEnum collects the monitors of one EnumDisplayMonitors call. The
// callback is made once (callbacks are never freed, and a process has a
// limited number), so it appends to this shared slice rather than to one
// of the call's, and the mutex guards it.
var monitorEnum struct {
	once sync.Once
	cb   uintptr
	mu   sync.Mutex
	mons []uintptr
}

// monitorWorkAreas returns the work areas of the monitors (rcWork: the
// monitor without the task bar and app bars), the primary first, in
// virtual-screen physical pixels (the process is per-monitor DPI aware, so
// rcWork is not scaled).
func monitorWorkAreas() []image.Rectangle {
	e := &monitorEnum
	e.once.Do(func() {
		e.cb = syscall.NewCallback(func(mon, hdc, clip, data uintptr) uintptr {
			e.mons = append(e.mons, mon)
			return 1 // continue
		})
	})
	e.mu.Lock()
	e.mons = e.mons[:0]
	procEnumDisplayMonitors.Call(0, 0, e.cb, 0)
	mons := append([]uintptr(nil), e.mons...)
	e.mu.Unlock()

	var works []image.Rectangle
	for _, m := range mons {
		mi := monitorInfoOf(m)
		if r := mi.work.image(); !r.Empty() {
			if mi.flags&monitorinfofPrimary != 0 {
				works = append([]image.Rectangle{r}, works...)
			} else {
				works = append(works, r)
			}
		}
	}
	return works
}

// fitPosition returns where the content of a window asked to open at pos
// goes so that it can be grabbed on a monitor present, and sets w.dpi to
// the DPI of the monitor there. The rectangle fitted is the outer window
// (its frame and title bar, from frameOffset and outerSize, which depend
// on the DPI), so the title bar ends up on screen; the invisible resize
// borders of Windows 10 and 11 count as part of it, which keeps the
// window a few pixels further in than needed. The DPI of the monitor the
// window lands on changes its frame and physical size: the fit is done
// again there (at most twice more).
func (w *Window) fitPosition(pos image.Point) image.Point {
	works := monitorWorkAreas()
	for range 3 {
		w.dpi = monitorDPI(monitorFromRect(rect{int32(pos.X), int32(pos.Y), int32(pos.X) + 1, int32(pos.Y) + 1}))
		off := w.frameOffset(w.style, w.exStyle)
		outer := w.outerSize(clampPhysical(platform.PhysicalSize(w.opts.Size, w.scale())))
		r := image.Rectangle{Min: pos.Add(off), Max: pos.Add(off).Add(image.Pt(int(outer.width()), int(outer.height())))}
		fit := platform.FitOnScreens(r, works)
		if fit == r {
			break
		}
		pos = fit.Min.Sub(off)
	}
	w.dpi = monitorDPI(monitorFromRect(rect{int32(pos.X), int32(pos.Y), int32(pos.X) + 1, int32(pos.Y) + 1}))
	return pos
}
