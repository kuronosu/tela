package win32

import "github.com/kuronosu/tela/internal/platform"

// System cursor resource identifiers (IDC_*), the argument of LoadCursorW
// with a nil instance.
const (
	idcArrow       = 32512
	idcIBeam       = 32513
	idcWait        = 32514
	idcCross       = 32515
	idcSizeNWSE    = 32642
	idcSizeNESW    = 32643
	idcSizeWE      = 32644
	idcSizeNS      = 32645
	idcSizeAll     = 32646
	idcNo          = 32648
	idcHand        = 32649
	idcAppStarting = 32650
	idcHelp        = 32651
)

// cursorResource returns the system cursor (IDC_*) that shows c. ok is false
// for CursorNone, which hides the cursor instead.
//
// Windows has no system cursor for context-menu, cell, alias, copy,
// vertical-text, zoom-in and zoom-out (browsers ship their own bitmaps for
// them); those use the closest system shape or the arrow, and grab and
// grabbing use the hand and the four-way arrow. Diagonal and axis resize
// cursors map to the four double arrows Windows has; the one-sided e-resize,
// n-resize and so on use the same double arrow as their axis.
func cursorResource(c platform.Cursor) (idc uintptr, ok bool) {
	switch c {
	case platform.CursorNone:
		return 0, false
	case platform.CursorHelp:
		return idcHelp, true
	case platform.CursorPointer, platform.CursorGrab:
		return idcHand, true
	case platform.CursorProgress:
		return idcAppStarting, true
	case platform.CursorWait:
		return idcWait, true
	case platform.CursorCell, platform.CursorCrosshair:
		return idcCross, true
	case platform.CursorText, platform.CursorVerticalText:
		return idcIBeam, true
	case platform.CursorMove, platform.CursorGrabbing, platform.CursorAllScroll:
		return idcSizeAll, true
	case platform.CursorNoDrop, platform.CursorNotAllowed:
		return idcNo, true
	case platform.CursorEResize, platform.CursorWResize, platform.CursorEWResize, platform.CursorColResize:
		return idcSizeWE, true
	case platform.CursorNResize, platform.CursorSResize, platform.CursorNSResize, platform.CursorRowResize:
		return idcSizeNS, true
	case platform.CursorNEResize, platform.CursorSWResize, platform.CursorNESWResize:
		return idcSizeNESW, true
	case platform.CursorNWResize, platform.CursorSEResize, platform.CursorNWSEResize:
		return idcSizeNWSE, true
	}
	// CursorDefault, CursorContextMenu, CursorAlias, CursorCopy, CursorZoomIn
	// and CursorZoomOut.
	return idcArrow, true
}
