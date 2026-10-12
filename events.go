package tela

// MouseButton is a mouse button, or the touch contact or pen tip that
// stands for the primary one.
type MouseButton uint8

// The mouse buttons.
const (
	// MouseButtonNone is no button, as in a move or enter event.
	MouseButtonNone MouseButton = iota
	// MouseButtonLeft is the left button, touch contact or pen tip.
	MouseButtonLeft
	// MouseButtonRight is the right button.
	MouseButtonRight
	// MouseButtonMiddle is the wheel button.
	MouseButtonMiddle
	// MouseButtonBack is the "back" side button.
	MouseButtonBack
	// MouseButtonForward is the "forward" side button.
	MouseButtonForward
)

// String returns the button name: "left", "right"...
func (b MouseButton) String() string {
	switch b {
	case MouseButtonNone:
		return "none"
	case MouseButtonLeft:
		return "left"
	case MouseButtonRight:
		return "right"
	case MouseButtonMiddle:
		return "middle"
	case MouseButtonBack:
		return "back"
	case MouseButtonForward:
		return "forward"
	}
	return "?"
}

// MouseEvent is passed to the mouse handlers (h.OnClick, h.OnMouseDown...).
//
// Event dispatch arrives with input in phase 4: until then handlers are
// stored but never called. Phase 4 adds the modifiers and the methods to
// stop propagation.
type MouseEvent struct {
	// X and Y are the pointer position in logical pixels, relative to the
	// top-left corner of the window's content area.
	X, Y float32
	// Button is the button that changed state for a press, release or
	// click; MouseButtonNone for enter and leave.
	Button MouseButton
}
