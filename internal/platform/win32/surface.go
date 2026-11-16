//go:build windows

package win32

import (
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Frames presented through a swapchain on the window (contract v14,
// ADR-289; the platform package's "Presenting through a swapchain").
//
// A GPU renderer presents a window's frames itself, through a Vulkan
// swapchain on its HWND (VK_KHR_win32_surface): the window hands out its
// handles and is told of each present. It never sees the graphics API.
//
// While the window's frames come that way (surfaceState.active, from the
// first SurfacePresented to the next Present), the retained copy (dib.go)
// is stale, so the window paints nothing from it: what it would repaint
// itself (WM_PAINT, and the repaints of redraw.go after a reconnect, a
// display, theme or composition change, a resume) becomes a request for a
// whole frame instead (Win32Surface().Full and RequestFrame). GDI drawing
// would not show anyway under a flip-model presentation (DXGI interop,
// what NVIDIA, AMD and Intel drivers use for windowed Vulkan), and would
// cover the swapchain's pixels with old ones under a blit-model one.
//
// Not every window can be presented this way. A Transparent window is a
// layered window whose pixels reach UpdateLayeredWindow and whose input
// mask reads them (transparent_windows.go); a window with a client-area
// backdrop shows the frame's alpha through DWM, which a swapchain's
// opaque images do not carry (VK_COMPOSITE_ALPHA_OPAQUE_BIT_KHR is what
// Windows drivers offer); a TitleBarOverlay window shows DWM's caption
// buttons through transparent pixels, and hides a band of its client area
// when maximized (overlay_windows.go). They report no surface, and their
// renderer uses Present.
//
// Pacing is unchanged: frames come at the DwmFlush signal, one a refresh,
// so a FIFO swapchain has at most one present waiting and the renderer,
// which acquires its images without waiting, never blocks the UI thread
// there. Winit and Gio hand the HWND to the graphics API the same way and
// leave the swapchain's pacing to it; here the backend keeps its own, as
// for Present, so that both paths deliver frames alike.

// surfaceState is a window's side of a renderer's swapchain.
type surfaceState struct {
	// active: the latest frame came through the swapchain (a
	// SurfacePresented since the last Present).
	active bool
	// full: the next swapchain present must be whole and happen even
	// when nothing changed.
	full bool
}

var _ platform.Win32SurfacePresenter = (*Window)(nil)

// surfaceCapable reports whether the window's frames may come through a
// swapchain (see above).
func (w *Window) surfaceCapable() bool {
	return !w.Closed() && !w.opts.Transparent && !w.glass && !w.overlay
}

// Win32Surface implements platform.Win32SurfacePresenter.
func (w *Window) Win32Surface() (platform.Win32Surface, bool) {
	if !w.surfaceCapable() {
		return platform.Win32Surface{}, false
	}
	return platform.Win32Surface{
		Instance: w.b.hinst,
		Window:   w.hwnd,
		Size:     w.client,
		// The first present through a swapchain after Presents is whole.
		Full: w.surf.full || !w.surf.active,
	}, true
}

// SurfacePresented implements platform.Win32SurfacePresenter.
func (w *Window) SurfacePresented(damage []image.Rectangle) {
	if w.Closed() {
		return
	}
	w.surf.active = true
	whole := len(damage) == 0
	if whole {
		w.surf.full = false
		w.ClearNeedsFull()
	}
	if !w.shown && w.visible {
		// Not before: the window would flash. A driver may drop what is
		// presented to a window not shown yet (a blit-model presentation
		// draws into nothing), so the frame after the show is whole too:
		// one more present at the first show and after SetVisible(true).
		w.show()
		w.surfaceRepaint()
	}
	if whole {
		// Its WM_PAINT asks for nothing: the whole frame was presented.
		procValidateRect.Call(w.hwnd, 0)
	}
}

// surfaceRepaint is repaint for a window whose frames come through a
// swapchain: the renderer is asked for the whole frame. It reports whether
// the window is one.
func (w *Window) surfaceRepaint() bool {
	if !w.surf.active {
		return false
	}
	w.surf.full = true
	w.RequestFrame()
	w.b.dirty = true // the pass that follows the message paces the frame
	return true
}

// surfacePaint answers WM_PAINT for a window whose frames come through a
// swapchain: the update region is validated and the whole frame asked
// for. It reports whether the window is one.
func (w *Window) surfacePaint() bool {
	if !w.surf.active {
		return false
	}
	var ps paintStruct
	if hdc, _, _ := procBeginPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps))); hdc != 0 {
		procEndPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	}
	return w.surfaceRepaint()
}

// surfaceShow is SetVisible(true) for a window whose latest frame came
// through a swapchain: the renderer dropped it while the window was hidden,
// so the window is shown by the next present, a whole one, which it asks
// for. It reports whether the window is one.
func (w *Window) surfaceShow() bool {
	if !w.surf.active || w.shown {
		return false
	}
	return w.surfaceRepaint()
}

// surfaceEnded is called by Present: the window's frames come from the
// retained copy again, and that Present is whole.
func (w *Window) surfaceEnded() bool {
	if !w.surf.active {
		return false
	}
	w.surf.active, w.surf.full = false, false
	return true
}
