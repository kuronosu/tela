//go:build windows

package win32

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// Window styling on Windows 11 (ADR-187). See "Window styling" in the
// package documentation of platform.
//
// The title bar colors (DWMWA_CAPTION_COLOR, DWMWA_TEXT_COLOR,
// DWMWA_BORDER_COLOR) and the system backdrop (DWMWA_SYSTEMBACKDROP_TYPE)
// are DWM window attributes: a decorated window's native title bar and
// border show them with no other work, and they are ignored where DWM does
// not know the attribute (Windows 10, Wine).
//
// A backdrop in the client area needs more. DWM draws Mica or Acrylic
// behind the window and shows it where the window is "glass" and its
// pixels are transparent. So a Mica, Mica Alt or Acrylic window extends
// the frame over its whole client area (DwmExtendFrameIntoClientArea, see
// glassMargins) and keeps the alpha of the frames it
// presents (dib.alpha), which core renders transparent where the root has
// no background (platform.AlphaPresenter). A window without such a backdrop
// is untouched: the frame is not extended (that stopped Windows 11 from
// showing the snap layouts flyout over a frameless window's maximize
// button) and its pixels are opaque (a transparent window after a session
// reconnect is worse).

// DWM window attributes of Windows 11 (dwmapi.h).
const (
	dwmwaBorderColor        = 34 // DWMWA_BORDER_COLOR, build 22000
	dwmwaCaptionColor       = 35 // DWMWA_CAPTION_COLOR, build 22000
	dwmwaTextColor          = 36 // DWMWA_TEXT_COLOR, build 22000
	dwmwaSystemBackdropType = 38 // DWMWA_SYSTEMBACKDROP_TYPE, build 22621
)

// buildColors is the Windows 11 build (21H2) that introduced the
// DWMWA_*_COLOR attributes.
const buildColors = 22000

// margins is DWM's MARGINS.
type margins struct{ left, right, top, bottom int32 }

// glassMargins returns the margins that make the whole client area glass.
//
// A decorated window takes the "sheet of glass": -1 on every side. A
// frameless window must not: a window whose client area is its whole
// window and whose top margin is not zero (-1 included) gets DWM's system
// caption buttons drawn over the app's own title bar (seen on Windows 11
// 26200), and they are the likely reason a one-pixel top margin took the
// snap layouts flyout from the app's maximize button (da6d1f5). A margin of zero at the top and a wider one
// on the other sides than any window (the margins are measured from the
// client edge inward, so the left and right ones already cover the whole
// height) makes the same glass without the caption.
func glassMargins(frameless bool) margins {
	if frameless {
		return margins{left: 1 << 14, right: 1 << 14, bottom: 1 << 14}
	}
	return margins{-1, -1, -1, -1}
}

// windowsBuild returns the Windows build number (a variable for tests).
var windowsBuild = sync.OnceValue(func() uint32 { return windows.RtlGetVersion().BuildNumber })

// setDwmUint sets a DWM window attribute of 4 bytes and reports whether DWM
// accepted it.
func (w *Window) setDwmUint(attr uintptr, v uint32) bool {
	if !available(procDwmSetWindowAttribute) {
		return false
	}
	hr, _, _ := procDwmSetWindowAttribute.Call(w.hwnd, attr, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return hr == 0
}

// initStyle applies the title bar colors and the backdrop of the options,
// right after CreateWindowExW. The window is not shown yet.
func (w *Window) initStyle() {
	w.titleBar, w.backdrop = w.opts.TitleBar, w.opts.Backdrop
	w.applyTitleBar()
	w.applyBackdrop()
}

// SetTitleBarColors sets the colors of the native title bar and border
// (Windows 11); see platform.Window.
func (w *Window) SetTitleBarColors(colors platform.TitleBarColors) {
	if w.Closed() || colors == w.titleBar {
		return
	}
	w.titleBar = colors
	w.applyTitleBar()
}

// SetBackdrop sets the system backdrop (Windows 11 22H2); see
// platform.Window.
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
	if w.Closed() || backdrop == w.backdrop {
		return
	}
	w.backdrop = backdrop
	w.applyBackdrop()
}

var _ platform.AlphaPresenter = (*Window)(nil)

// PresentsAlpha reports whether the window shows the alpha of its frames: it
// has a client-area backdrop (platform.AlphaPresenter).
func (w *Window) PresentsAlpha() bool { return w.glass }

// applyTitleBar sets the three colors. Nothing is touched for a window that
// never had any, and a color that is the system's again is set to
// DWMWA_COLOR_DEFAULT.
func (w *Window) applyTitleBar() {
	if windowsBuild() < buildColors || (w.titleBar.IsZero() && !w.titleBarSet) {
		return
	}
	w.titleBarSet = true
	w.setDwmUint(dwmwaCaptionColor, colorRef(w.titleBar.Background))
	w.setDwmUint(dwmwaTextColor, colorRef(w.titleBar.Text))
	w.setDwmUint(dwmwaBorderColor, colorRef(w.titleBar.Border))
}

// applyBackdrop sets the backdrop type and, for Mica, Mica Alt and Acrylic,
// extends the frame over the client area and starts presenting alpha; any
// other backdrop undoes that.
func (w *Window) applyBackdrop() {
	build := windowsBuild()
	glass := false
	if build >= buildBackdrop && (w.backdrop != platform.BackdropDefault || w.backdropSet) {
		w.backdropSet = true
		accepted := w.setDwmUint(dwmwaSystemBackdropType, uint32(backdropType(w.backdrop)))
		glass = accepted && clientBackdrop(w.backdrop, build, compositionEnabled())
	}
	w.setGlass(glass)
}

// setGlass extends the frame over the client area (or takes it back) and
// switches the retained copy to alpha. It extends again when it is already
// on: DWM forgets the margins when the composition changes.
func (w *Window) setGlass(on bool) {
	if on {
		m := glassMargins(w.opts.Frameless)
		procDwmExtendFrameIntoClientArea.Call(w.hwnd, uintptr(unsafe.Pointer(&m)))
	} else if w.glass {
		var none margins
		procDwmExtendFrameIntoClientArea.Call(w.hwnd, uintptr(unsafe.Pointer(&none)))
	}
	w.glass = on
	if w.dib.setAlpha(on) {
		// The retained pixels have the other alpha: the next Present
		// draws everything (core renders the whole window again too).
		w.RequestFull()
	}
}

// restyle redoes what the window asked DWM for when the composition or the
// theme changed (frameChanged): the margins of a window with a client-area
// backdrop, and the attributes.
func (w *Window) restyle() {
	if !w.titleBarSet && !w.backdropSet && !w.glass {
		return // a window that never asked is left alone
	}
	w.applyTitleBar()
	w.applyBackdrop()
}
