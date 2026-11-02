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
