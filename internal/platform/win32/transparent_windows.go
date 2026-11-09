//go:build windows

package win32

import (
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Transparent windows (WindowOptions.Transparent, ADR-249) are layered
// windows presented with UpdateLayeredWindowIndirect.
//
//   - Style: WS_POPUP with the system menu and the minimize box (the task
//     bar's minimize and window menu), WS_EX_LAYERED; no caption or
//     thick frame, so the window is all client area (WM_NCCALCSIZE is
//     left to DefWindowProc), DWM draws no frame, shadow or border, and
//     Windows 11 is asked not to round it (DWMWA_WINDOW_CORNER_PREFERENCE).
//     A backdrop and title bar colors are ignored.
//   - Present copies the damage into a 32-bit top-down DIB section
//     (premultiplied B, G, R, A: what ULW_ALPHA with AC_SRC_ALPHA takes)
//     selected into a memory DC, and hands the union of the damage to
//     UpdateLayeredWindowIndirect as its dirty rectangle. The system keeps
//     the bitmap, so WM_PAINT is not involved, and the window shows at the
//     first Present like the others (a layered window shows nothing before
//     its first update). This is the per-pixel alpha path every Windows
//     version has, which Ebitengine's and GLFW's DWM blur-behind trick
//     does not need for GDI pixels; the DWM glass of ADR-187 would
//     composite the alpha too, but leaves a glass window opaque to the
//     pointer.
//   - Pointer input: Windows itself passes the pointer through the pixels
//     of a layered window whose alpha is zero (its hit test reads the
//     bitmap). For the pixels under platform.InputAlpha that are not
//     zero (the faint part of a shadow) the window answers WM_NCHITTEST
//     by setting WS_EX_TRANSPARENT, which makes the whole layered window
//     transparent to the pointer, and polls the cursor every 16 ms
//     (GetCursorPos on a WM_TIMER) until it is over the input mask again
//     or off the window, when the style goes: Electron's forwarding of
//     setIgnoreMouseEvents does the same with a mouse hook. A press keeps
//     its sequence (the window holds the capture). HTTRANSPARENT alone
//     would not do: it only passes the pointer to windows of the same
//     thread.
//   - No resize band: the edges are usually the transparent margin.

const (
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	ulwAlpha        = 0x00000002
	acSrcOver       = 0x00
	acSrcAlpha      = 0x01
	wmTimer         = 0x0113
	htTransparent   = ^uintptr(0) // HTTRANSPARENT, -1

	dwmwcpDoNotRound = 1          // DWMWCP_DONOTROUND
	dwmwaColorNone   = 0xFFFFFFFE // DWMWA_COLOR_NONE: no border

	// passTimer is the WM_TIMER ID of the cursor poll while the window
	// lets the pointer through, every passPoll milliseconds.
	passTimer = 0x7E1A
	passPoll  = 16
)

var (
	procCreateCompatibleDC          = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC                    = gdi32.NewProc("DeleteDC")
	procUpdateLayeredWindowIndirect = user32.NewProc("UpdateLayeredWindowIndirect")
	procSetTimer                    = user32.NewProc("SetTimer")
	procKillTimer                   = user32.NewProc("KillTimer")
)

// blendFunction is BLENDFUNCTION.
type blendFunction struct{ op, _, alpha, format byte } // BlendFlags (the second byte) is 0

// sizeL is SIZE.
type sizeL struct{ cx, cy int32 }

// updateLayeredWindowInfo is UPDATELAYEREDWINDOWINFO.
type updateLayeredWindowInfo struct {
	size   uint32
	hdcDst uintptr
	pptDst *point
	psize  *sizeL
	hdcSrc uintptr
	pptSrc *point
	crKey  uint32
	pblend *blendFunction
	flags  uint32
	dirty  *rect
}

// layered is the state of a transparent window: the memory DC and the DIB
// section the frames are copied into, the input mask, and whether it lets
// the pointer through (WS_EX_TRANSPARENT and the poll).
type layered struct {
	dc, bmp, old uintptr
	bits         []byte
	size         image.Point
	mask         platform.InputMask
	passing      bool
}

// transparentStyle gives a transparent window its styles (see the top of
// this file).
func (w *Window) transparentStyle() {
	w.style = wsPopup | wsSysMenu | wsMinimizeBox | wsClipSiblings | wsClipChildren
	w.exStyle |= wsExLayered
}

// initTransparent asks DWM for square corners and no border (Windows 11;
// failures are ignored).
func (w *Window) initTransparent() {
	w.setDwmUint(dwmwaWindowCornerPreference, dwmwcpDoNotRound)
	w.setDwmUint(dwmwaBorderColor, dwmwaColorNone)
}

// transparentMessage handles the messages a transparent window answers
// differently, reporting whether it did.
func (w *Window) transparentMessage(msg uint32, wparam, lparam uintptr) (uintptr, bool) {
	switch msg {
	case wmNCCalcSize:
		// No frame: the whole window is client area, as DefWindowProc
		// makes it for WS_POPUP (the frameless code would inset a
		// maximized window).
		return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam), true
	case wmNCHitTest:
		x, y := w.screenPointToApp(lparam)
		if w.ptr.held == 0 && !w.lay.mask.Contains(image.Pt(x, y)) {
			w.setPassing(true)
			return htTransparent, true
		}
	case wmTimer:
		if wparam == passTimer {
			w.pollPassing()
			return 0, true
		}
	case wmPaint:
		// The system keeps a layered window's bitmap: nothing to paint.
		procValidateRect.Call(w.hwnd, 0)
		return 0, true
	}
	return 0, false
}

// presentLayered presents the rectangles rects of fb (clipped damage;
// full for a new size) on a transparent window.
func (w *Window) presentLayered(fb *image.RGBA, rects []image.Rectangle, full bool) error {
	l := &w.lay
	size := fb.Rect.Size()
	if l.dc == 0 || l.size != size {
		if err := w.newLayeredBitmap(size); err != nil {
			return err
		}
		full, rects = true, []image.Rectangle{fb.Rect}
	}
	dirty := rects[0]
	for _, r := range rects {
		platform.CopyRGBAToBGRA(l.bits, 4*size.X, fb, r)
		dirty = dirty.Union(r)
	}
	maskChanged := l.mask.Update(fb, rects)
	src := point{}
	sz := sizeL{int32(size.X), int32(size.Y)}
	blend := blendFunction{op: acSrcOver, alpha: 255, format: acSrcAlpha}
	info := updateLayeredWindowInfo{
		size: uint32(unsafe.Sizeof(updateLayeredWindowInfo{})), psize: &sz,
		hdcSrc: l.dc, pptSrc: &src, pblend: &blend, flags: ulwAlpha,
	}
	if !full {
		d := rectFrom(dirty)
		info.dirty = &d
	}
	ok, _, e := procUpdateLayeredWindowIndirect.Call(w.hwnd, uintptr(unsafe.Pointer(&info)))
	if ok == 0 && info.dirty != nil {
		// Some versions refuse a dirty rectangle with a size: all of it.
		info.dirty = nil
		ok, _, e = procUpdateLayeredWindowIndirect.Call(w.hwnd, uintptr(unsafe.Pointer(&info)))
	}
	if ok == 0 {
		return winError("UpdateLayeredWindowIndirect", e)
	}
	if !w.shown && w.visible {
		w.show()
	}
	if maskChanged && w.lay.passing {
		w.pollPassing() // the pixels under a resting pointer changed
	}
	return nil
}

// newLayeredBitmap makes the memory DC and a DIB section of size.
func (w *Window) newLayeredBitmap(size image.Point) error {
	l := &w.lay
	if l.dc == 0 {
		dc, _, e := procCreateCompatibleDC.Call(0)
		if dc == 0 {
			return winError("CreateCompatibleDC", e)
		}
		l.dc = dc
	}
	hdr := bitmapInfo{bitmapInfoHeader: bitmapInfoHeader{
		size: bitmapInfoHeaderSize, width: int32(size.X), height: -int32(size.Y), // top-down
		planes: 1, bitCount: 32, compression: biRGB,
	}}
	var bits uintptr
	bmp, _, e := procCreateDIBSection.Call(l.dc, uintptr(unsafe.Pointer(&hdr)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == 0 {
		return winError("CreateDIBSection", e)
	}
	old, _, _ := procSelectObject.Call(l.dc, bmp)
	if l.bmp != 0 {
		procDeleteObject.Call(l.bmp) // the previous one, deselected above
	} else {
		l.old = old
	}
	l.bmp = bmp
	l.bits = unsafe.Slice((*byte)(ptrFromUintptr(bits)), 4*size.X*size.Y)
	l.size = size
	return nil
}

// releaseLayered frees the memory DC and the bitmap.
func (w *Window) releaseLayered() {
	l := &w.lay
	if l.passing {
		procKillTimer.Call(w.hwnd, passTimer)
	}
	if l.dc != 0 {
		if l.old != 0 {
			procSelectObject.Call(l.dc, l.old)
		}
		procDeleteDC.Call(l.dc)
	}
	if l.bmp != 0 {
		procDeleteObject.Call(l.bmp)
	}
	*l = layered{}
}

// setPassing lets the pointer through the whole window (on) or takes it
// back (off).
func (w *Window) setPassing(on bool) {
	l := &w.lay
	if l.passing == on {
		return
	}
	l.passing = on
	if on {
		w.exStyle |= wsExTransparent
		if w.ptr.inside {
			w.ptr.leaveMessage(w.emitFn) // the pointer left the app's input
		}
		procSetTimer.Call(w.hwnd, passTimer, passPoll, 0)
	} else {
		w.exStyle &^= wsExTransparent
		procKillTimer.Call(w.hwnd, passTimer)
	}
	setWindowLong(w.hwnd, gwlExStyle, getWindowLong(w.hwnd, gwlExStyle)&^wsExTransparent|w.exStyle&wsExTransparent)
}

// pollPassing takes the pointer back when the cursor is over the input
// mask again, or off the window (where the next hit test decides).
func (w *Window) pollPassing() {
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	x, y := w.screenToApp(int(p.x), int(p.y))
	if !w.inClient(x, y) || w.lay.mask.Contains(image.Pt(x, y)) {
		w.setPassing(false)
	}
}
