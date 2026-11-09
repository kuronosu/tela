package cocoa

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestCursorSelector checks that every cursor maps to an NSCursor class
// method that exists on macOS 13 (the minimum Go 1.27 supports), and that
// only CursorNone hides the cursor.
func TestCursorSelector(t *testing.T) {
	known := map[string]bool{
		"arrowCursor": true, "contextualMenuCursor": true, "pointingHandCursor": true,
		"crosshairCursor": true, "IBeamCursor": true, "IBeamCursorForVerticalLayout": true,
		"dragLinkCursor": true, "dragCopyCursor": true, "operationNotAllowedCursor": true,
		"openHandCursor": true, "closedHandCursor": true, "resizeRightCursor": true,
		"resizeLeftCursor": true, "resizeUpCursor": true, "resizeDownCursor": true,
		"resizeLeftRightCursor": true, "resizeUpDownCursor": true,
	}
	for c := platform.CursorDefault; c.Valid(); c++ {
		got := cursorSelector(c)
		if c == platform.CursorNone {
			if got != "" {
				t.Errorf("cursorSelector(CursorNone) = %q, want \"\" (hidden)", got)
			}
			continue
		}
		if !known[got] {
			t.Errorf("cursorSelector(%v) = %q, not an NSCursor class method of macOS 13", c, got)
		}
	}
	if got := cursorSelector(platform.CursorDefault); got != "arrowCursor" {
		t.Errorf("cursorSelector(CursorDefault) = %q, want arrowCursor", got)
	}
}

// The diagonal resize cursors (ADR-261): macOS 15's frame resize cursor
// at the corner (one-way shapes Outward, two-way All, as winit), else
// AppKit's private window frame cursor along the same diagonal; resize
// handles show the two-way shapes of AppKit's own frame.
func TestDiagonalCursor(t *testing.T) {
	const nwse, nesw = "_windowResizeNorthWestSouthEastCursor", "_windowResizeNorthEastSouthWestCursor"
	for _, c := range []struct {
		cursor    platform.Cursor
		pos, dirs uint
		private   string
	}{
		{platform.CursorNWResize, 1 | 2, 2, nwse},
		{platform.CursorSEResize, 4 | 8, 2, nwse},
		{platform.CursorNEResize, 1 | 8, 2, nesw},
		{platform.CursorSWResize, 4 | 2, 2, nesw},
		{platform.CursorNWSEResize, 1 | 2, 3, nwse},
		{platform.CursorNESWResize, 1 | 8, 3, nesw},
	} {
		pos, dirs, private, ok := diagonalCursor(c.cursor)
		if !ok || pos != c.pos || dirs != c.dirs || private != c.private {
			t.Errorf("diagonalCursor(%v) = %d, %d, %q, %v; want %d, %d, %q", c.cursor, pos, dirs, private, ok, c.pos, c.dirs, c.private)
		}
	}
	for _, c := range []platform.Cursor{platform.CursorDefault, platform.CursorEResize, platform.CursorEWResize, platform.CursorPointer} {
		if _, _, _, ok := diagonalCursor(c); ok {
			t.Errorf("diagonalCursor(%v) is diagonal", c)
		}
	}
	for e, want := range map[platform.Edge]platform.Cursor{
		platform.EdgeLeft: platform.CursorEWResize, platform.EdgeBottom: platform.CursorNSResize,
		platform.EdgeTop | platform.EdgeLeft: platform.CursorNWSEResize, platform.EdgeBottom | platform.EdgeRight: platform.CursorNWSEResize,
		platform.EdgeTop | platform.EdgeRight: platform.CursorNESWResize, platform.EdgeBottom | platform.EdgeLeft: platform.CursorNESWResize,
	} {
		if got := handleCursor(e); got != want {
			t.Errorf("handleCursor(%v) = %v, want %v", e, got, want)
		}
	}
}
