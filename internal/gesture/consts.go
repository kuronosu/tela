package gesture

import (
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Slops and timeouts. Touch values are Flutter's (kTouchSlop,
// kLongPressTimeout, kDoubleTapTimeout, kDoubleTapSlop), which follow
// Android's ViewConfiguration; mouse values are the desktop conventions.
// The double-click limits of mice and pens are the platform's
// (platform.DefaultDoubleClickInterval, platform.DoubleClickSlop; ADR-192),
// shared with the title bars that backends draw themselves.
const (
	// TouchSlop is how far a finger or a pen may move, in logical pixels,
	// before a press becomes a drag (Flutter's kTouchSlop; Flutter uses
	// it for styluses too).
	TouchSlop = 18
	// MouseSlop is the same distance for a mouse: Windows' default
	// drag rectangle (SM_CXDRAG, SM_CYDRAG) and Chromium's drag
	// threshold. GTK uses 8 and Flutter 1 to 2; 4 tells a drag from a
	// shaky click without feeling sticky.
	MouseSlop = 4

	// LongPressTimeout is how long a press must stay within the slop to
	// be a long press (Flutter's kLongPressTimeout, Android's default).
	LongPressTimeout = 500 * time.Millisecond

	// DoubleTapTime and DoubleTapSlop are the double-click limits of
	// touch (Flutter's kDoubleTapTimeout and kDoubleTapSlop), whatever
	// the OS's double-click time for mice.
	DoubleTapTime = 300 * time.Millisecond
	DoubleTapSlop = 100
)

// DragSlop returns the distance a pointer of kind k must move from the
// press before a drag starts (and a long press is abandoned).
func DragSlop(k Kind) float32 {
	if k == Mouse {
		return MouseSlop
	}
	return TouchSlop
}

// clickLimits returns the double-click time and distance of kind k, where
// interval is the double-click time of mice and pens (not positive: the
// platform default).
func clickLimits(k Kind, interval time.Duration) (time.Duration, float32) {
	if k == Touch {
		return DoubleTapTime, DoubleTapSlop
	}
	return platform.DoubleClickTime(interval), platform.DoubleClickSlop
}
