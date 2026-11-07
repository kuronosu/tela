//go:build windows

package win32

import (
	"sync"
	"unsafe"
)

// The dark menu bar (ADR-234; menubar_dark.go has the why). The system's
// themed bar asks the window to paint with two undocumented messages:
//
//   - WM_UAHDRAWMENU: fill the bar's background. The DC is a window DC, so
//     the bar's rectangle (GetMenuBarInfo, in screen coordinates) is moved
//     to window coordinates.
//   - WM_UAHDRAWMENUITEM: fill one item and draw its label. The label comes
//     from the menu (GetMenuItemInfoW) and is drawn with the menu font of
//     the window's DPI and GDI's DrawTextW, which underlines the '&'
//     mnemonic unless the system hides access keys at that moment
//     (ODS_NOACCEL: DT_HIDEPREFIX).
//
// Both are answered with TRUE only when the bar is drawn dark; a light bar
// gets DefWindowProc's. A one-pixel line under the bar belongs to the
// non-client area, which DefWindowProc paints light after WM_NCPAINT and
// WM_NCACTIVATE: the window paints it over (paintBarSeam). A change of the
// app theme redraws the bar (refreshBar).

var (
	procFillRect                   = user32.NewProc("FillRect")
	procDrawTextW                  = user32.NewProc("DrawTextW")
	procGetMenuItemInfoW           = user32.NewProc("GetMenuItemInfoW")
	procGetMenuBarInfo             = user32.NewProc("GetMenuBarInfo")
	procDrawMenuBar                = user32.NewProc("DrawMenuBar")
	procGetWindowDC                = user32.NewProc("GetWindowDC")
	procRedrawWindow               = user32.NewProc("RedrawWindow")
	procSystemParametersInfoForDpi = user32.NewProc("SystemParametersInfoForDpi") // Windows 10 1607

	procCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	procSetBkMode           = gdi32.NewProc("SetBkMode")
	procSetTextColor        = gdi32.NewProc("SetTextColor")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procCreateFontIndirectW = gdi32.NewProc("CreateFontIndirectW")
	procGetStockObject      = gdi32.NewProc("GetStockObject")
)

const (
	miimString             = 0x0040
	objidMenu              = 0xFFFFFFFD // OBJID_MENU, (LONG)-3
	transparent            = 1          // SetBkMode
	defaultGUIFont         = 17         // GetStockObject
	spiGetNonClientMetrics = 0x0029
	rdwInvalidate          = 0x0001
	rdwFrame               = 0x0400
)

// sendMsg is SendMessageW.
func sendMsg(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

// barBrushes are the solid brushes of the dark bar by color. They live as
// long as the process (a handful of tiny GDI objects shared by every
// window).
var barBrushes = struct {
	sync.Mutex
	m map[winColor]uintptr
}{m: map[winColor]uintptr{}}

func barBrush(c winColor) uintptr {
	barBrushes.Lock()
	defer barBrushes.Unlock()
	if h, ok := barBrushes.m[c]; ok {
		return h
	}
	h, _, _ := procCreateSolidBrush.Call(uintptr(c))
	barBrushes.m[c] = h
	return h
}

func fillRect(hdc uintptr, r rect, c winColor) {
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), barBrush(c))
}

// darkBar reports whether the window draws its menu bar dark now.
func (w *Window) darkBar() bool {
	if w.integ.menu == 0 || w.opts.Frameless {
		return false
	}
	s := w.b.settings
	return darkBarOn(s.DarkMode, s.HighContrast)
}

// barRect returns the menu bar's rectangle in window coordinates, and
// whether the window has a bar showing.
func (w *Window) barRect() (rect, bool) {
	mbi := menuBarInfo{size: uint32(unsafe.Sizeof(menuBarInfo{}))}
	if r, _, _ := procGetMenuBarInfo.Call(w.hwnd, objidMenu, 0, uintptr(unsafe.Pointer(&mbi))); r == 0 {
		return rect{}, false
	}
	win := getWindowRect(w.hwnd)
	r := rect(mbi.bar)
	r.left, r.right = r.left-win.left, r.right-win.left
	r.top, r.bottom = r.top-win.top, r.bottom-win.top
	return r, true
}

// onUAHDrawMenu paints the bar's background; it reports whether it did.
func (w *Window) onUAHDrawMenu(lparam uintptr) bool {
	if lparam == 0 || !w.darkBar() {
		return false
	}
	u := (*uahMenu)(ptrFromUintptr(lparam))
	r, ok := w.barRect()
	if !ok {
		return false
	}
	fillRect(u.hdc, r, darkBar.background)
	return true
}

// onUAHDrawMenuItem paints one item of the bar; it reports whether it did.
func (w *Window) onUAHDrawMenuItem(lparam uintptr) bool {
	if lparam == 0 || !w.darkBar() {
		return false
	}
	u := (*uahDrawMenuItem)(ptrFromUintptr(lparam))
	state := barItemState(u.dis.itemState)
	hdc := u.um.hdc
	fillRect(hdc, rect(u.dis.rcItem), state.fill())

	var label [256]uint16
	mii := menuItemInfo{mask: miimString, typeData: &label[0], cch: uint32(len(label) - 1)}
	mii.size = uint32(unsafe.Sizeof(mii))
	if r, _, _ := procGetMenuItemInfoW.Call(u.um.hmenu, uintptr(u.position), 1, uintptr(unsafe.Pointer(&mii))); r == 0 {
		return true // the background is painted; there is no text to show
	}
	oldFont, _, _ := procSelectObject.Call(hdc, w.barFont())
	procSetBkMode.Call(hdc, transparent)
	oldColor, _, _ := procSetTextColor.Call(hdc, uintptr(state.ink()))
	rc := rect(u.dis.rcItem)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&label[0])), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), uintptr(drawTextFlags(u.dis.itemState)))
	procSetTextColor.Call(hdc, oldColor)
	procSelectObject.Call(hdc, oldFont)
	return true
}

// paintBarSeam paints the line between the bar and the client area, which
// DefWindowProc paints light with the rest of the non-client area, in the
// bar's color. It runs after the default handling of WM_NCPAINT and
// WM_NCACTIVATE.
func (w *Window) paintBarSeam() {
	if !w.darkBar() {
		return
	}
	if _, ok := w.barRect(); !ok {
		return
	}
	origin := point{}
	procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
	win := getWindowRect(w.hwnd)
	left, top := origin.x-win.left, origin.y-win.top
	line := rect{left: left, top: top - 1, right: left + getClientRect(w.hwnd).width(), bottom: top}
	hdc, _, _ := procGetWindowDC.Call(w.hwnd)
	if hdc == 0 {
		return
	}
	fillRect(hdc, line, darkBar.background)
	procReleaseDC.Call(w.hwnd, hdc)
}

// refreshBar redraws the bar, with its non-client seam, after the theme
// changed.
func (w *Window) refreshBar() {
	if w.integ.menu == 0 || w.Closed() {
		return
	}
	procDrawMenuBar.Call(w.hwnd)
	procRedrawWindow.Call(w.hwnd, 0, 0, rdwFrame|rdwInvalidate)
}

// barFont returns the font of the bar's items at the window's DPI: the
// system's menu font (SystemParametersInfoForDpi, the default GUI font
// where that is missing or fails). It is kept until the DPI changes or the
// window closes.
func (w *Window) barFont() uintptr {
	in := &w.integ
	if in.barFont != 0 && in.barFontDPI == w.dpi {
		return in.barFont
	}
	w.releaseBarFont()
	ncm := nonClientMetrics{size: uint32(unsafe.Sizeof(nonClientMetrics{}))}
	var ok bool
	if available(procSystemParametersInfoForDpi) {
		r, _, _ := procSystemParametersInfoForDpi.Call(spiGetNonClientMetrics, uintptr(ncm.size), uintptr(unsafe.Pointer(&ncm)), 0, uintptr(w.dpi))
		ok = r != 0
	} else {
		r, _, _ := procSystemParametersInfoW.Call(spiGetNonClientMetrics, uintptr(ncm.size), uintptr(unsafe.Pointer(&ncm)), 0)
		ok = r != 0
	}
	if ok {
		if f, _, _ := procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&ncm.menuFont))); f != 0 {
			in.barFont, in.barFontOwned = f, true
		}
	}
	if in.barFont == 0 {
		in.barFont, _, _ = procGetStockObject.Call(defaultGUIFont)
	}
	in.barFontDPI = w.dpi
	return in.barFont
}

// releaseBarFont deletes the bar font, when it was created for the window.
func (w *Window) releaseBarFont() {
	in := &w.integ
	if in.barFontOwned && in.barFont != 0 {
		procDeleteObject.Call(in.barFont)
	}
	in.barFont, in.barFontDPI, in.barFontOwned = 0, 0, false
}
