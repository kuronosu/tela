package cocoa

import "github.com/kuronosu/tela/internal/platform"

// cursorSelector returns the NSCursor class method that gives the system
// cursor for c ("" for CursorNone, which hides the cursor).
//
// AppKit has no public cursor for progress, wait, help, move, all-scroll,
// zoom and the diagonal resize shapes before macOS 15
// (frameResizeCursorFromPosition:inDirections:); those show the arrow, as
// winit does. Cell uses the crosshair, alias and copy the drag cursors.
func cursorSelector(c platform.Cursor) string {
	switch c {
	case platform.CursorNone:
		return ""
	case platform.CursorContextMenu:
		return "contextualMenuCursor"
	case platform.CursorPointer:
		return "pointingHandCursor"
	case platform.CursorCell, platform.CursorCrosshair:
		return "crosshairCursor"
	case platform.CursorText:
		return "IBeamCursor"
	case platform.CursorVerticalText:
		return "IBeamCursorForVerticalLayout"
	case platform.CursorAlias:
		return "dragLinkCursor"
	case platform.CursorCopy:
		return "dragCopyCursor"
	case platform.CursorNoDrop, platform.CursorNotAllowed:
		return "operationNotAllowedCursor"
	case platform.CursorGrab:
		return "openHandCursor"
	case platform.CursorGrabbing:
		return "closedHandCursor"
	case platform.CursorEResize:
		return "resizeRightCursor"
	case platform.CursorWResize:
		return "resizeLeftCursor"
	case platform.CursorNResize:
		return "resizeUpCursor"
	case platform.CursorSResize:
		return "resizeDownCursor"
	case platform.CursorEWResize, platform.CursorColResize:
		return "resizeLeftRightCursor"
	case platform.CursorNSResize, platform.CursorRowResize:
		return "resizeUpDownCursor"
	}
	return "arrowCursor"
}
