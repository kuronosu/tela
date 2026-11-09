package cocoa

import "github.com/kuronosu/tela/internal/platform"

// cursorSelector returns the NSCursor class method that gives the system
// cursor for c ("" for CursorNone, which hides the cursor).
//
// AppKit has no public cursor for progress, wait, help, move, all-scroll
// and zoom; those show the arrow, as winit does. The diagonal resize
// shapes have one from macOS 15 and a private one before (diagonalCursor;
// the arrow here is the fallback when neither answers). Cell uses the
// crosshair, alias and copy the drag cursors.
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

// NSCursorFrameResizePosition and NSCursorFrameResizeDirections (macOS 15
// SDK, NSCursor.h).
const (
	frameResizeTop    = 1 << 0
	frameResizeLeft   = 1 << 1
	frameResizeBottom = 1 << 2
	frameResizeRight  = 1 << 3

	frameResizeOutward = 1 << 1
	frameResizeAll     = 1<<0 | 1<<1
)

// diagonalCursor returns how AppKit draws the diagonal resize cursor c
// (ADR-261): the position and directions of macOS 15's public
// +frameResizeCursorFromPosition:inDirections:, and the private class
// method older AppKit answers (+_windowResizeNorthWestSouthEastCursor,
// +_windowResizeNorthEastSouthWestCursor, the cursors of its own window
// frame), which the backend calls only when the class respondsToSelector:
// it, as winit does (its mapping: the one-way CSS shapes are Outward, the
// two-way ones All). ok is false for the other cursors.
func diagonalCursor(c platform.Cursor) (position, directions uint, private string, ok bool) {
	const nwse, nesw = "_windowResizeNorthWestSouthEastCursor", "_windowResizeNorthEastSouthWestCursor"
	switch c {
	case platform.CursorNWResize:
		return frameResizeTop | frameResizeLeft, frameResizeOutward, nwse, true
	case platform.CursorSEResize:
		return frameResizeBottom | frameResizeRight, frameResizeOutward, nwse, true
	case platform.CursorNEResize:
		return frameResizeTop | frameResizeRight, frameResizeOutward, nesw, true
	case platform.CursorSWResize:
		return frameResizeBottom | frameResizeLeft, frameResizeOutward, nesw, true
	case platform.CursorNWSEResize:
		return frameResizeTop | frameResizeLeft, frameResizeAll, nwse, true
	case platform.CursorNESWResize:
		return frameResizeTop | frameResizeRight, frameResizeAll, nesw, true
	}
	return 0, 0, "", false
}

// handleCursor returns the cursor of a resize handle of edge e: the
// two-way shapes of AppKit's own window frame (a side resizes both ways,
// a corner along its diagonal), not Edge.Cursor's one-way CSS names.
func handleCursor(e platform.Edge) platform.Cursor {
	switch e {
	case platform.EdgeLeft, platform.EdgeRight:
		return platform.CursorEWResize
	case platform.EdgeTop, platform.EdgeBottom:
		return platform.CursorNSResize
	case platform.EdgeTop | platform.EdgeLeft, platform.EdgeBottom | platform.EdgeRight:
		return platform.CursorNWSEResize
	case platform.EdgeTop | platform.EdgeRight, platform.EdgeBottom | platform.EdgeLeft:
		return platform.CursorNESWResize
	}
	return platform.CursorDefault
}
