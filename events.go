package tela

import "github.com/kuronosu/tela/internal/core"

// Events follow the DOM and React: a handler receives the event by value,
// and controls propagation with its methods.
//
//	Button(OnClick(func(e tela.MouseEvent) {
//	    e.StopPropagation() // the card below does not get this click
//	    setOpen.Set(false)
//	}))
//
// The target of a pointer event is the element under the pointer (the
// topmost one as painted, inside the clips and transforms of its
// ancestors, skipping pointer-events-none and invisible elements). Events
// bubble from the target to the root; OnMouseEnter, OnMouseLeave,
// OnPointerEnter and OnPointerLeave do not bubble, and fire on every
// element the pointer enters or leaves. A Disabled element and its
// descendants receive no pointer events.
//
// Pointer events (OnPointerDown...) fire for every pointer: mouse, touch
// and pen. Mouse events (OnMouseDown..., OnClick, OnDoubleClick,
// OnContextMenu) are the compatibility view of the primary pointer, in
// the DOM order: pointerdown, mousedown, pointerup, mouseup, click.
//
// Gestures compete: a press that becomes a drag (OnDragStart...) or a
// long press (OnLongPress) does not click. After every discrete event
// (presses, releases, clicks, drag start and end, long press) state
// updates are applied and components rebuilt before the next event, so
// two clicks before a frame see each other's updates.

// MouseButton is a mouse button, or the touch contact or pen tip that
// stands for the primary one.
type MouseButton = core.MouseButton

// The mouse buttons.
const (
	// MouseButtonNone is no button, as in a move or enter event.
	MouseButtonNone = core.MouseButtonNone
	// MouseButtonLeft is the primary button: the left button (the right
	// one on a left-handed setup), a touch contact or a pen tip.
	MouseButtonLeft = core.MouseButtonLeft
	// MouseButtonRight is the secondary button.
	MouseButtonRight = core.MouseButtonRight
	// MouseButtonMiddle is the wheel button.
	MouseButtonMiddle = core.MouseButtonMiddle
	// MouseButtonBack is the "back" side button.
	MouseButtonBack = core.MouseButtonBack
	// MouseButtonForward is the "forward" side button.
	MouseButtonForward = core.MouseButtonForward
)

// MouseButtons is a set of held buttons: e.Buttons.Has(tela.MouseButtonLeft).
type MouseButtons = core.MouseButtons

// PointerKind is the device behind a pointer.
type PointerKind = core.PointerKind

// The pointer kinds.
const (
	PointerMouse = core.PointerMouse // a mouse or touchpad cursor
	PointerTouch = core.PointerTouch // a finger on a touchscreen
	PointerPen   = core.PointerPen   // a stylus
)

// MouseEvent is passed to the mouse handlers (h.OnClick, h.OnMouseDown,
// h.OnContextMenu...). X and Y are window coordinates in logical pixels;
// LocalX and LocalY are relative to the border box of the element whose
// handler runs. ClickCount is 1 for a click, 2 for the second click of a
// double click. StopPropagation stops the bubbling; PreventDefault marks
// the event (on h.OnPointerDown it suppresses the press's mouse events,
// not the click).
type MouseEvent = core.MouseEvent

// PointerEvent is passed to the pointer handlers (h.OnPointerDown...,
// h.OnLongPress): the mouse fields plus the pointer's Kind, PointerID,
// Pressure and IsPrimary.
type PointerEvent = core.PointerEvent

// DragEvent is passed to h.OnDragStart, h.OnDrag and h.OnDragEnd:
// the position, the movement since the previous drag event (DeltaX,
// DeltaY) and since the press (OffsetX, OffsetY), and the velocity in
// pixels per second (VelocityX, VelocityY; meaningful at the end).
type DragEvent = core.DragEvent
