//go:build windows

package win32

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// The system DLLs and the procedures this backend calls. Procedures that
// only exist on newer Windows versions are optional: call sites check
// available() and fall back to an older API.
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procPeekMessageW               = user32.NewProc("PeekMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procPostMessageW               = user32.NewProc("PostMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procSetWindowTextW             = user32.NewProc("SetWindowTextW")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procGetClientRect              = user32.NewProc("GetClientRect")
	procScreenToClient             = user32.NewProc("ScreenToClient")
	procGetDC                      = user32.NewProc("GetDC")
	procReleaseDC                  = user32.NewProc("ReleaseDC")
	procBeginPaint                 = user32.NewProc("BeginPaint")
	procEndPaint                   = user32.NewProc("EndPaint")
	procValidateRect               = user32.NewProc("ValidateRect")
	procTrackMouseEvent            = user32.NewProc("TrackMouseEvent")
	procSetCapture                 = user32.NewProc("SetCapture")
	procReleaseCapture             = user32.NewProc("ReleaseCapture")
	procGetCapture                 = user32.NewProc("GetCapture")
	procGetKeyState                = user32.NewProc("GetKeyState")
	procGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	procGetKeyboardState           = user32.NewProc("GetKeyboardState")
	procGetKeyboardLayout          = user32.NewProc("GetKeyboardLayout")
	procMapVirtualKeyExW           = user32.NewProc("MapVirtualKeyExW")
	procToUnicodeEx                = user32.NewProc("ToUnicodeEx")
	procGetMessageTime             = user32.NewProc("GetMessageTime")
	procSystemParametersInfoW      = user32.NewProc("SystemParametersInfoW")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procLoadCursorW                = user32.NewProc("LoadCursorW")
	procSetCursor                  = user32.NewProc("SetCursor")
	procAdjustWindowRectEx         = user32.NewProc("AdjustWindowRectEx")
	procIsIconic                   = user32.NewProc("IsIconic")
	procIsZoomed                   = user32.NewProc("IsZoomed")
	procGetDoubleClickTime         = user32.NewProc("GetDoubleClickTime")
	procEnumDisplaySetting         = user32.NewProc("EnumDisplaySettingsW")
	procMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	procMonitorFromWindow          = user32.NewProc("MonitorFromWindow")

	// Optional (Windows 10 1607 and 1703, 8.1, Vista).
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procAdjustWindowRectExForDpi      = user32.NewProc("AdjustWindowRectExForDpi")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwareness        = shcore.NewProc("SetProcessDpiAwareness")
	procGetDpiForMonitor              = shcore.NewProc("GetDpiForMonitor")

	procSetDIBitsToDevice = gdi32.NewProc("SetDIBitsToDevice")
	procGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")

	procDwmFlush              = dwmapi.NewProc("DwmFlush")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute") // Windows 10 1809 (19) and 1903 (20)

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalSize       = kernel32.NewProc("GlobalSize")
)

// available reports whether an optional procedure exists on this system.
func available(p *windows.LazyProc) bool { return p.Find() == nil }

// Window messages. The ones that pure decoders need are in msg.go.
const (
	wmSize            = 0x0005
	wmSettingChange   = 0x001A
	wmInputLangChange = 0x0051
	wmSysCommand      = 0x0112
	wmExitMenuLoop    = 0x0212
	wmExitSizeMove    = 0x0232
	wmSetFocus        = 0x0007
	wmKillFocus       = 0x0008
	wmPaint           = 0x000F
	wmClose           = 0x0010
	wmEraseBkgnd      = 0x0014
	wmSetCursor       = 0x0020
	wmGetMinMaxInfo   = 0x0024
	wmNCDestroy       = 0x0082
	wmCancelMode      = 0x001F
	wmMouseMove       = 0x0200
	wmCaptureChanged  = 0x0215
	wmMouseLeave      = 0x02A3
	wmDPIChanged      = 0x02E0
	wmTelaNotify      = 0x8000 + 1 // WM_APP + 1: a Wake or Quit is waiting
	wmTelaVsync       = 0x8000 + 2 // WM_APP + 2: the display signalled a vsync
	htClient          = 1
	pmRemove          = 0x0001
	pmNoRemove        = 0x0000
	hwndMessage       = ^uintptr(2) // HWND_MESSAGE, (HWND)-3
	swShow            = 5
	swpNoMove         = 0x0002
	swpNoZOrder       = 0x0004
	swpNoActivate     = 0x0010
	tmeLeave          = 0x00000002
	cwUseDefault      = 0x80000000
	wsOverlapped      = 0x00000000
	wsCaption         = 0x00C00000
	wsSysMenu         = 0x00080000
	wsThickFrame      = 0x00040000
	wsMinimizeBox     = 0x00020000
	wsMaximizeBox     = 0x00010000
	wsClipSiblings    = 0x04000000
	wsClipChildren    = 0x02000000
	monitorDefaultPri = 1
	monitorDefaultNr  = 2
	mdtEffectiveDPI   = 0
	logPixelsX        = 88
	enumCurrentSets   = ^uint32(0) // ENUM_CURRENT_SETTINGS, (DWORD)-1
	errClassExists    = 1410       // ERROR_CLASS_ALREADY_EXISTS
	errAccessDenied   = 5          // ERROR_ACCESS_DENIED

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, (DPI_AWARENESS_CONTEXT)-4.
	dpiPerMonitorAwareV2 = ^uintptr(3)
	// PROCESS_PER_MONITOR_DPI_AWARE of SetProcessDpiAwareness.
	processPerMonitorDPIAware = 2
)

// Structures of the Win32 API. Go lays out the fields as C does for these
// (each field naturally aligned), so uintptr stands for handles and pointers
// on both 32 and 64-bit Windows.
type (
	point struct{ x, y int32 }
	rect  struct{ left, top, right, bottom int32 }

	msg struct {
		hwnd    uintptr
		message uint32
		wParam  uintptr
		lParam  uintptr
		time    uint32
		pt      point
		private uint32
	}

	wndClassEx struct {
		size       uint32
		style      uint32
		wndProc    uintptr
		clsExtra   int32
		wndExtra   int32
		instance   uintptr
		icon       uintptr
		cursor     uintptr
		background uintptr
		menuName   *uint16
		className  *uint16
		iconSm     uintptr
	}

	paintStruct struct {
		hdc         uintptr
		erase       int32
		rcPaint     rect
		restore     int32
		incUpdate   int32
		rgbReserved [32]byte
	}

	trackMouseEvent struct {
		size      uint32
		flags     uint32
		hwndTrack uintptr
		hoverTime uint32
	}

	minMaxInfo struct {
		reserved     point
		maxSize      point
		maxPosition  point
		minTrackSize point
		maxTrackSize point
	}

	// bitmapInfo is BITMAPINFO: the header and room for the one RGBQUAD it
	// declares (BI_RGB 32-bit DIBs have no color table).
	bitmapInfo struct {
		bitmapInfoHeader
		colors [1]uint32
	}

	// devMode is DEVMODEW, of which the backend reads the display
	// frequency. The union of printer and display fields is spelled as
	// padding.
	devMode struct {
		deviceName       [32]uint16
		specVersion      uint16
		driverVersion    uint16
		size             uint16
		driverExtra      uint16
		fields           uint32
		_                [16]byte // dmOrientation … / dmPosition … (union)
		color            int16
		duplex           int16
		yResolution      int16
		ttOption         int16
		collate          int16
		formName         [32]uint16
		logPixels        uint16
		bitsPerPel       uint32
		pelsWidth        uint32
		pelsHeight       uint32
		displayFlags     uint32
		displayFrequency uint32
		icmMethod        uint32
		icmIntent        uint32
		mediaType        uint32
		ditherType       uint32
		reserved1        uint32
		reserved2        uint32
		panningWidth     uint32
		panningHeight    uint32
	}
)

func (r rect) width() int32  { return r.right - r.left }
func (r rect) height() int32 { return r.bottom - r.top }

// ptrFromUintptr turns an address that Windows put in a message parameter
// into a pointer to its memory, which is not Go memory and never moves.
// (Converting a uintptr variable with unsafe.Pointer(v) is correct here too,
// but go vet cannot tell it from a stale Go pointer.)
func ptrFromUintptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// winError describes a failed call: what was attempted, and Windows'
// last-error text.
func winError(what string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("win32: %s failed", what)
	}
	return fmt.Errorf("win32: %s: %w", what, err)
}

// --- thin wrappers: one per procedure, typed arguments ---

func getModuleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

func defWindowProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func postMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) bool {
	r, _, _ := procPostMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r != 0
}

func postQuitMessage() { procPostQuitMessage.Call(0) }

func getClientRect(hwnd uintptr) (r rect) {
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func isIconic(hwnd uintptr) bool {
	r, _, _ := procIsIconic.Call(hwnd)
	return r != 0
}

func isZoomed(hwnd uintptr) bool {
	r, _, _ := procIsZoomed.Call(hwnd)
	return r != 0
}

func setWindowPos(hwnd uintptr, x, y, w, h int32, flags uintptr) {
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), flags)
}

func screenToClient(hwnd uintptr, x, y int) (cx, cy int) {
	p := point{int32(x), int32(y)}
	procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return int(p.x), int(p.y)
}

func keyDown(vk int) bool {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

// keyboardModifiers reads the shortcut keys held now (the state as of the
// message being processed). On a layout with AltGr a held right Alt is AltGr,
// and neither it nor the fake left Ctrl that Windows holds with it is
// reported as Ctrl or Alt. realLCtrl says that the left Ctrl is a key the
// user pressed (the window delivered its press), not the fake one.
func keyboardModifiers(realLCtrl bool) platform.Modifiers {
	ralt := keyDown(vkRMenu)
	return modifiersFromState(keyDown(vkShift), keyDown(vkLControl), keyDown(vkRControl), keyDown(vkLMenu), ralt,
		keyDown(vkLWin) || keyDown(vkRWin), ralt && layoutHasAltGr(currentLayout()), realLCtrl)
}

// adjustWindowRect returns the outer window size that has a client area of
// the given physical size at dpi.
func adjustWindowRect(client rect, style, exStyle uint32, dpi uint32) rect {
	r := client
	if available(procAdjustWindowRectExForDpi) {
		procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), 0, uintptr(exStyle), uintptr(dpi))
	} else {
		procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), 0, uintptr(exStyle))
	}
	return r
}
