package win32

import "github.com/kuronosu/tela/internal/platform"

// Window styling (ADR-187): the pure half. The DWM calls are in
// windowstyle.go.

// buildBackdrop is the Windows 11 build (22H2) that introduced
// DWMWA_SYSTEMBACKDROP_TYPE.
const buildBackdrop = 22621

// colorDefault is DWMWA_COLOR_DEFAULT: the attribute takes the system's
// color again.
const colorDefault = 0xFFFFFFFF

// colorRef converts c to the COLORREF DWM takes (0x00BBGGRR), or
// DWMWA_COLOR_DEFAULT for the system's color.
func colorRef(c platform.WindowColor) uint32 {
	if !c.Set {
		return colorDefault
	}
	return uint32(c.B)<<16 | uint32(c.G)<<8 | uint32(c.R)
}

// DWM_SYSTEMBACKDROP_TYPE.
const (
	dwmsbtAuto            = 0 // DWMSBT_AUTO
	dwmsbtNone            = 1 // DWMSBT_NONE
	dwmsbtMainWindow      = 2 // DWMSBT_MAINWINDOW: Mica
	dwmsbtTransientWindow = 3 // DWMSBT_TRANSIENTWINDOW: Acrylic
	dwmsbtTabbedWindow    = 4 // DWMSBT_TABBEDWINDOW: Mica Alt
)

// backdropType maps b to DWM_SYSTEMBACKDROP_TYPE.
func backdropType(b platform.Backdrop) int32 {
	switch b {
	case platform.BackdropNone:
		return dwmsbtNone
	case platform.BackdropMica:
		return dwmsbtMainWindow
	case platform.BackdropMicaAlt:
		return dwmsbtTabbedWindow
	case platform.BackdropAcrylic:
		return dwmsbtTransientWindow
	}
	return dwmsbtAuto
}

// clientBackdrop reports whether a window with backdrop b shows it in its
// client area on Windows build `build` with composition on or off: the
// frame is extended into the client area and the frames are presented with
// their alpha.
func clientBackdrop(b platform.Backdrop, build uint32, composing bool) bool {
	return b.ClientBackdrop() && build >= buildBackdrop && composing
}
