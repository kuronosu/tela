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
	imm32    = windows.NewLazySystemDLL("imm32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll") // desktop services (ADR-178)
	ole32    = windows.NewLazySystemDLL("ole32.dll")   // desktop services (ADR-178)

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
	procInvalidateRect             = user32.NewProc("InvalidateRect")
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
	procClientToScreen             = user32.NewProc("ClientToScreen")
	procCreateCaret                = user32.NewProc("CreateCaret")
	procDestroyCaret               = user32.NewProc("DestroyCaret")
	procSetCaretPos                = user32.NewProc("SetCaretPos")
	procSendMessageW               = user32.NewProc("SendMessageW")
	procGetWindowLongW             = user32.NewProc("GetWindowLongW")
	procSetWindowLongW             = user32.NewProc("SetWindowLongW")
	procGetWindowRect              = user32.NewProc("GetWindowRect")
	procGetWindowPlacement         = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement         = user32.NewProc("SetWindowPlacement")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
	procMonitorFromRect            = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	procEnableWindow               = user32.NewProc("EnableWindow")
	procSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	procFlashWindowEx              = user32.NewProc("FlashWindowEx")
	procGetCursorPos               = user32.NewProc("GetCursorPos")
	procWindowFromPoint            = user32.NewProc("WindowFromPoint")
	procGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	procCreateIconIndirect         = user32.NewProc("CreateIconIndirect")
	procDestroyIcon                = user32.NewProc("DestroyIcon")

	procImmGetContext            = imm32.NewProc("ImmGetContext")
	procImmReleaseContext        = imm32.NewProc("ImmReleaseContext")
	procImmAssociateContextEx    = imm32.NewProc("ImmAssociateContextEx")
	procImmGetCompositionStringW = imm32.NewProc("ImmGetCompositionStringW")
	procImmSetCompositionWindow  = imm32.NewProc("ImmSetCompositionWindow")
	procImmSetCandidateWindow    = imm32.NewProc("ImmSetCandidateWindow")
	procImmNotifyIME             = imm32.NewProc("ImmNotifyIME")

	// Optional (Windows 10 1607 and 1703, 8.1, Vista).
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procAdjustWindowRectExForDpi      = user32.NewProc("AdjustWindowRectExForDpi")
	procGetSystemMetricsForDpi        = user32.NewProc("GetSystemMetricsForDpi")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwareness        = shcore.NewProc("SetProcessDpiAwareness")
	procGetDpiForMonitor              = shcore.NewProc("GetDpiForMonitor")

	procSetDIBitsToDevice = gdi32.NewProc("SetDIBitsToDevice")
	procGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	procCreateDIBSection  = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap      = gdi32.NewProc("CreateBitmap")
	procDeleteObject      = gdi32.NewProc("DeleteObject")

	procDwmFlush                = dwmapi.NewProc("DwmFlush")
	procDwmIsCompositionEnabled = dwmapi.NewProc("DwmIsCompositionEnabled")
	procDwmSetWindowAttribute   = dwmapi.NewProc("DwmSetWindowAttribute") // Windows 10 1809 (19) and 1903 (20)

	procDwmExtendFrameIntoClientArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalSize       = kernel32.NewProc("GlobalSize")

	// Desktop services (services.go, dialog_windows.go, shell_windows.go,
	// tray_windows.go).
	procShellExecuteW               = shell32.NewProc("ShellExecuteW")
	procShellNotifyIconW            = shell32.NewProc("Shell_NotifyIconW")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procExtractIconExW              = shell32.NewProc("ExtractIconExW")
	procCoInitializeEx              = ole32.NewProc("CoInitializeEx")
	procCoUninitialize              = ole32.NewProc("CoUninitialize")
	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree               = ole32.NewProc("CoTaskMemFree")
	procMessageBoxW                 = user32.NewProc("MessageBoxW")
	procCreatePopupMenu             = user32.NewProc("CreatePopupMenu")
	procAppendMenuW                 = user32.NewProc("AppendMenuW")
	procDestroyMenu                 = user32.NewProc("DestroyMenu")
	procTrackPopupMenuEx            = user32.NewProc("TrackPopupMenuEx")
	procRegisterWindowMessageW      = user32.NewProc("RegisterWindowMessageW")
	procChangeWindowMessageFilterEx = user32.NewProc("ChangeWindowMessageFilterEx") // Windows 7
	procLoadIconW                   = user32.NewProc("LoadIconW")
)

// available reports whether an optional procedure exists on this system.
func available(p *windows.LazyProc) bool { return p.Find() == nil }

// Window messages. The ones that pure decoders need are in msg.go.
const (
	wmSize                  = 0x0005
	wmSettingChange         = 0x001A
	wmInputLangChange       = 0x0051
	wmSysCommand            = 0x0112
	wmExitMenuLoop          = 0x0212
	wmExitSizeMove          = 0x0232
	wmSetFocus              = 0x0007
	wmKillFocus             = 0x0008
	wmPaint                 = 0x000F
	wmClose                 = 0x0010
	wmEraseBkgnd            = 0x0014
	wmSetCursor             = 0x0020
	wmGetMinMaxInfo         = 0x0024
	wmNCDestroy             = 0x0082
	wmCancelMode            = 0x001F
	wmMouseMove             = 0x0200
	wmCaptureChanged        = 0x0215
	wmMouseLeave            = 0x02A3
	wmDPIChanged            = 0x02E0
	wmMove                  = 0x0003
	wmShowWindow            = 0x0018
	wmSetIcon               = 0x0080
	wmNCCalcSize            = 0x0083
	wmSetText               = 0x000C
	wmDisplayChange         = 0x007E
	wmNCPaint               = 0x0085
	wmNCActivate            = 0x0086
	wmNCUAHDrawCaption      = 0x00AE // undocumented: DefWindowProc paints the classic caption
	wmNCUAHDrawFrame        = 0x00AF // undocumented: DefWindowProc paints the classic frame
	wmPowerBroadcast        = 0x0218
	wmThemeChanged          = 0x031A
	wmDwmCompositionChanged = 0x031E
	pbtAPMResumeSuspend     = 0x0007
	pbtAPMResumeAutomatic   = 0x0012
	wsVisible               = 0x10000000
	wmNCHitTest             = 0x0084
	wmTelaNotify            = 0x8000 + 1 // WM_APP + 1: a Wake or Quit is waiting
	wmTelaVsync             = 0x8000 + 2 // WM_APP + 2: the display signalled a vsync
	pmRemove                = 0x0001
	pmNoRemove              = 0x0000
	hwndMessage             = ^uintptr(2) // HWND_MESSAGE, (HWND)-3
	swHide                  = 0
	swShowMaximized         = 3
	swShowNoActivate        = 4
	swShow                  = 5
	swMinimize              = 6
	swShowMinNoActive       = 7
	swShowNA                = 8
	swRestore               = 9
	swParentClosing         = 1      // WM_SHOWWINDOW lParam: the owner is being minimized
	swParentOpening         = 3      // WM_SHOWWINDOW lParam: the owner is being restored
	wpfRestoreToMaximized   = 0x0002 // WINDOWPLACEMENT.flags: a minimized window restores to maximized
	swpNoSize               = 0x0001
	swpFrameChanged         = 0x0020
	swpNoOwnerZOrder        = 0x0200
	hwndTopmost             = ^uintptr(0)  // HWND_TOPMOST, (HWND)-1
	hwndNoTopmost           = ^uintptr(1)  // HWND_NOTOPMOST, (HWND)-2
	gwlStyle                = ^uintptr(15) // GWL_STYLE, -16
	gwlExStyle              = ^uintptr(19) // GWL_EXSTYLE, -20
	wsMaximize              = 0x01000000
	wsExTopmost             = 0x00000008
	// WS_EX_DLGMODALFRAME, WS_EX_WINDOWEDGE, WS_EX_CLIENTEDGE and
	// WS_EX_STATICEDGE: the edges a fullscreen window drops.
	wsExFrameEdges    = 0x00000001 | 0x00000100 | 0x00000200 | 0x00020000
	iconSmall         = 0
	iconBig           = 1
	smCxIcon          = 11
	smCxSizeFrame     = 32
	smCxSmIcon        = 49
	smCxPaddedBorder  = 92
	flashwTray        = 0x00000002
	flashwTimerNoFG   = 0x0000000C
	tmeNonClient      = 0x00000010
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

	// windowPlacement is WINDOWPLACEMENT. rcNormalPosition is in workspace
	// coordinates (see workspaceOffset).
	windowPlacement struct {
		length           uint32
		flags            uint32
		showCmd          uint32
		ptMinPosition    point
		ptMaxPosition    point
		rcNormalPosition rect
	}

	monitorInfo struct {
		size    uint32
		monitor rect
		work    rect
		flags   uint32
	}

	flashWInfo struct {
		size    uint32
		hwnd    uintptr
		flags   uint32
		count   uint32
		timeout uint32
	}

	iconInfo struct {
		icon     int32 // TRUE: an icon, not a cursor
		xHotspot uint32
		yHotspot uint32
		mask     uintptr
		color    uintptr
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
