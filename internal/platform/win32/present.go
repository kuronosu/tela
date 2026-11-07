//go:build windows

package win32

import (
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// dibRGBColors is DIB_RGB_COLORS, the color-use argument of SetDIBitsToDevice.
const dibRGBColors = 0

// Present stores the damaged pixels of fb in the retained copy and blits
// them; see platform.Window. The window is shown by the first Present.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	if w.Closed() {
		return nil
	}
	if w.Metrics().Scale == 0 {
		panic("win32: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, w.Metrics())
	size := fb.Rect.Size()
	full := w.NeedsFull() || w.dib.size != size
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		return nil // every damage rectangle was clipped away: nothing changed
	}
	if full {
		w.ClearNeedsFull()
		w.dib.resize(size)
	}
	w.dib.update(fb, rects)
	// A TitleBarOverlay window: the hole for DWM's buttons and the color
	// behind them, from the pixels just stored (overlay_windows.go).
	w.syncOverlay()

	if !w.shown && w.visible {
		// Not before: the window would flash unpainted. WM_PAINT, which
		// ShowWindow brings, is answered from the copy.
		w.show()
	}
	hdc, _, e := procGetDC.Call(w.hwnd)
	if hdc == 0 {
		return winError("GetDC", e)
	}
	defer procReleaseDC.Call(w.hwnd, hdc)
	// The app's pixels lie w.top rows down the client area (the hidden band
	// of a maximized TitleBarOverlay window, zero otherwise).
	off := image.Pt(0, w.top)
	for _, r := range rects {
		w.blit(hdc, r.Add(off))
	}
	if full && w.top > 0 {
		w.blit(hdc, image.Rect(0, 0, size.X, w.top))
	}
	if full {
		// Everything was drawn: the update region Windows may have
		// accumulated (a resize, the show) needs no WM_PAINT.
		procValidateRect.Call(w.hwnd, 0)
	}
	// Where DWM put its caption buttons for the size just presented.
	w.recheckControls()
	return nil
}

// blit draws the rectangle r (physical client pixels) of the window: the
// retained copy, except for the transparent parts of a TitleBarOverlay
// window (blitParts).
func (w *Window) blit(hdc uintptr, r image.Rectangle) {
	if w.dib.pix == nil {
		return
	}
	if w.top == 0 && w.hole.Empty() {
		w.blitCopy(hdc, r)
		return
	}
	from, clear := blitParts(r, w.top, w.hole)
	for _, p := range from {
		w.blitCopy(hdc, p)
	}
	for _, p := range clear {
		w.blitClear(hdc, p)
	}
}

// blitCopy draws the rectangle r (physical client pixels, below the hidden
// band) from the retained copy.
func (w *Window) blitCopy(hdc uintptr, r image.Rectangle) {
	off := image.Pt(0, w.top)
	clipped, bits, hdr := w.dib.region(r.Sub(off))
	if bits == nil {
		return
	}
	clipped = clipped.Add(off)
	w.bmi.bitmapInfoHeader = hdr
	procSetDIBitsToDevice.Call(hdc,
		uintptr(clipped.Min.X), uintptr(clipped.Min.Y), uintptr(clipped.Dx()), uintptr(clipped.Dy()),
		uintptr(clipped.Min.X), 0, 0, uintptr(clipped.Dy()),
		uintptr(unsafe.Pointer(&bits[0])), uintptr(unsafe.Pointer(&w.bmi)), dibRGBColors)
}

// blitClear draws the rectangle r (physical client pixels) fully
// transparent: zero bytes from w.zeros, which grows to the largest
// rectangle and is never written.
func (w *Window) blitClear(hdc uintptr, r image.Rectangle) {
	if r.Empty() {
		return
	}
	if n := 4 * r.Dx() * r.Dy(); len(w.zeros) < n {
		w.zeros = make([]byte, n)
	}
	w.bmi.bitmapInfoHeader = bitmapInfoHeader{
		size: bitmapInfoHeaderSize, width: int32(r.Dx()), height: -int32(r.Dy()),
		planes: 1, bitCount: 32, compression: biRGB,
	}
	procSetDIBitsToDevice.Call(hdc,
		uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(r.Dx()), uintptr(r.Dy()),
		0, 0, 0, uintptr(r.Dy()),
		uintptr(unsafe.Pointer(&w.zeros[0])), uintptr(unsafe.Pointer(&w.bmi)), dibRGBColors)
}

// onPaint answers WM_PAINT from the retained copy: Windows asks when part of
// the window was uncovered, after a resize, and when it is shown. The app
// is not involved.
func (w *Window) onPaint() {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	w.blit(hdc, image.Rect(int(ps.rcPaint.left), int(ps.rcPaint.top), int(ps.rcPaint.right), int(ps.rcPaint.bottom)))
}
