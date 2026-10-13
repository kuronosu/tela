package win32

import (
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// Window message numbers and flags that the pure decoders below depend on.
// The rest of the messages live in the Windows-only files.
const (
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonDown = 0x0204
	wmRButtonUp   = 0x0205
	wmMButtonDown = 0x0207
	wmMButtonUp   = 0x0208
	wmMouseWheel  = 0x020A
	wmXButtonDown = 0x020B
	wmXButtonUp   = 0x020C
	wmMouseHWheel = 0x020E
	xButton1      = 0x0001
	xButton2      = 0x0002
	wheelDelta    = 120 // one notch of a classic wheel
)

// loword returns the low 16 bits of a message parameter (LOWORD).
func loword(v uintptr) uint16 { return uint16(v) }

// hiword returns bits 16 to 31 of a message parameter (HIWORD).
func hiword(v uintptr) uint16 { return uint16(v >> 16) }

// pointFromLParam decodes the signed client coordinates that mouse messages
// carry in lParam (GET_X_LPARAM, GET_Y_LPARAM). They are signed: while the
// mouse is captured, positions left of or above the window are negative.
func pointFromLParam(lp uintptr) (x, y int) {
	return int(int16(loword(lp))), int(int16(hiword(lp)))
}

// buttonFromMessage maps a mouse button message to the platform button and
// whether it is a press. Windows reports the logical buttons (WM_LBUTTON* is
// the primary button even when the user swapped the buttons), which is what
// platform.ButtonPrimary means. ok is false for other messages and for
// unknown X buttons.
func buttonFromMessage(msg uint32, wparam uintptr) (btn platform.PointerButton, down, ok bool) {
	switch msg {
	case wmLButtonDown:
		return platform.ButtonPrimary, true, true
	case wmLButtonUp:
		return platform.ButtonPrimary, false, true
	case wmRButtonDown:
		return platform.ButtonSecondary, true, true
	case wmRButtonUp:
		return platform.ButtonSecondary, false, true
	case wmMButtonDown:
		return platform.ButtonMiddle, true, true
	case wmMButtonUp:
		return platform.ButtonMiddle, false, true
	case wmXButtonDown, wmXButtonUp:
		down = msg == wmXButtonDown
		switch hiword(wparam) {
		case xButton1:
			return platform.ButtonBack, down, true
		case xButton2:
			return platform.ButtonForward, down, true
		}
	}
	return platform.ButtonNone, false, false
}

// wheelSteps converts a WM_MOUSEWHEEL or WM_MOUSEHWHEEL wParam to wheel
// notches in the DOM sign convention: Windows reports "wheel rolled away
// from the user" (scroll up) and "rolled right" as positive deltas, so
// vertical steps are negated (positive Y scrolls toward the end of the
// content). Wheels with fine resolution and precision touchpads report
// deltas below 120, which become fractional steps. ok is false for other
// messages.
func wheelSteps(msg uint32, wparam uintptr) (steps platform.Point, ok bool) {
	delta := float32(int16(hiword(wparam))) / wheelDelta
	switch msg {
	case wmMouseWheel:
		return platform.Point{Y: -delta}, true
	case wmMouseHWheel:
		return platform.Point{X: delta}, true
	}
	return platform.Point{}, false
}

// modifiersFrom builds the platform modifier set from the state of the four
// shortcut keys.
func modifiersFrom(shift, ctrl, alt, super bool) platform.Modifiers {
	var m platform.Modifiers
	if shift {
		m |= platform.ModShift
	}
	if ctrl {
		m |= platform.ModCtrl
	}
	if alt {
		m |= platform.ModAlt
	}
	if super {
		m |= platform.ModSuper
	}
	return m
}

// sanitizeTitle makes a window title safe for the W functions: invalid
// UTF-8 becomes U+FFFD, and NUL characters, which would end the string
// early, are dropped.
func sanitizeTitle(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "\uFFFD"), "\x00", "")
}
