package gesture

import "time"

// Slops and timeouts. Touch values are Flutter's (kTouchSlop,
// kLongPressTimeout, kDoubleTapTimeout, kDoubleTapSlop), which follow
// Android's ViewConfiguration; mouse values are the desktop conventions.
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

	// DoubleClickTime is the longest time between two presses of a mouse
	// or pen that count as one more click: the default of Windows
	// (GetDoubleClickTime) and macOS. GTK's default is 400 ms.
	DoubleClickTime = 500 * time.Millisecond
	// DoubleClickDistance is the farthest, in logical pixels, a press may
	// be from the previous one to count as one more click (GTK's
	// gtk-double-click-distance; Windows uses a 4x4 rectangle).
	DoubleClickDistance = 5
	// DoubleTapTime and DoubleTapSlop are the same limits for touch
	// (Flutter's kDoubleTapTimeout and kDoubleTapSlop).
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

// clickLimits returns the double-click time and distance of kind k.
func clickLimits(k Kind) (time.Duration, float32) {
	if k == Touch {
		return DoubleTapTime, DoubleTapSlop
	}
	return DoubleClickTime, DoubleClickDistance
}
