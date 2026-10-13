package core

import (
	"fmt"
)

// Event types passed to the handlers of the public API (tela.MouseEvent,
// tela.PointerEvent, tela.DragEvent are aliases). Events are passed by
// value; StopPropagation and PreventDefault act through a pointer to the
// state of the dispatch that every copy shares.

// MouseButton is a mouse button, or the touch contact or pen tip that
// stands for the primary one. The values match platform.PointerButton.
type MouseButton uint8

// The mouse buttons.
const (
	// MouseButtonNone is no button, as in a move or enter event.
	MouseButtonNone MouseButton = iota
	// MouseButtonLeft is the primary button: the left button (the right
	// one on a left-handed setup), a touch contact or a pen tip.
	MouseButtonLeft
	// MouseButtonRight is the secondary button (or a pen barrel button).
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

// MouseButtons is a set of held buttons, one bit per MouseButton (the
// DOM's MouseEvent.buttons, in this package's button order).
type MouseButtons uint8

// Has reports whether b is in the set.
func (s MouseButtons) Has(b MouseButton) bool {
	return b != MouseButtonNone && s&(1<<(b-1)) != 0
}

// PointerKind is the device behind a pointer (the DOM's pointerType).
type PointerKind uint8

// The pointer kinds.
const (
	PointerMouse PointerKind = iota // a mouse or touchpad cursor
	PointerTouch                    // a finger on a touchscreen
	PointerPen                      // a stylus
)

// String returns "mouse", "touch" or "pen".
func (k PointerKind) String() string {
	switch k {
	case PointerMouse:
		return "mouse"
	case PointerTouch:
		return "touch"
	case PointerPen:
		return "pen"
	}
	return "?"
}

// eventState is the state of one dispatch, shared by every copy of the
// event passed to its handlers.
type eventState struct {
	stopped, prevented bool
}

func (s *eventState) stop() {
	if s != nil {
		s.stopped = true
	}
}

func (s *eventState) prevent() {
	if s != nil {
		s.prevented = true
	}
}

func (s *eventState) defaultPrevented() bool { return s != nil && s.prevented }

// MouseEvent is passed to the mouse handlers (OnClick, OnMouseDown...):
// the compatibility view of the primary pointer, as in the DOM.
type MouseEvent struct {
	// X and Y are the pointer position in logical pixels, relative to the
	// top-left corner of the window's content area.
	X, Y float32
	// LocalX and LocalY are the position relative to the border box of
	// the element whose handler runs (the DOM's currentTarget), in its own
	// coordinates (its transforms and its ancestors' undone).
	LocalX, LocalY float32
	// Button is the button that changed state for a press, release or
	// click; MouseButtonNone for moves, enter and leave.
	Button MouseButton
	// Buttons are the buttons held after the event.
	Buttons MouseButtons
	// Modifier keys held during the event; Meta is the Windows, Command
	// or Super key.
	Shift, Ctrl, Alt, Meta bool
	// ClickCount is the number of consecutive clicks (the DOM's detail):
	// 1 for a single click, 2 for the second click of a double click.
	// Presses and releases carry the count of the click they belong to;
	// moves, enter and leave carry 0.
	ClickCount int

	st *eventState
}

// StopPropagation stops the event from bubbling to the ancestors of the
// element whose handler runs. The other handlers of that element still
// run.
func (e MouseEvent) StopPropagation() { e.st.stop() }

// PreventDefault cancels Tela's default action for the event and marks
// it for the handlers that run after (DefaultPrevented). Cancelling
// OnPointerDown suppresses the mouse events of the press (OnMouseDown,
// OnMouseMove, OnMouseUp), but not OnClick, as in the DOM.
func (e MouseEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e MouseEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// PointerEvent is passed to the pointer handlers (OnPointerDown...,
// OnLongPress): every pointer, mouse, touch and pen, as in the DOM.
type PointerEvent struct {
	// X and Y are the position in the window, LocalX and LocalY relative
	// to the border box of the element whose handler runs (see
	// MouseEvent).
	X, Y, LocalX, LocalY float32
	// Button is the button that changed state (MouseButtonNone for moves,
	// enter and leave); Buttons the buttons held after the event.
	Button  MouseButton
	Buttons MouseButtons
	// Modifier keys held during the event.
	Shift, Ctrl, Alt, Meta bool
	// Kind is the device: mouse, touch or pen.
	Kind PointerKind
	// PointerID identifies the pointer among those present: the mouse is
	// always 1; each touch contact has its own.
	PointerID int
	// Pressure is the normalized pressure, 0 to 1: 0.5 while a button is
	// held and 0 otherwise when the device does not report it (the DOM's
	// rule; no backend reports pressure yet).
	Pressure float32
	// IsPrimary reports the primary pointer of its kind: the mouse, the
	// first touch contact, the pen. Mouse events are fired for it only.
	IsPrimary bool

	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e PointerEvent) StopPropagation() { e.st.stop() }

// PreventDefault cancels the default action (see MouseEvent).
func (e PointerEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e PointerEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// DragEvent is passed to OnDragStart, OnDrag and OnDragEnd. A drag starts
// when the pointer, pressed with the primary button, moves farther than a
// few pixels (4 for a mouse, 18 for touch and pens) and no long press was
// recognized before; it is delivered to the element whose drag handlers
// won the gesture (the innermost one on the pointer's path).
type DragEvent struct {
	// X and Y are the position in the window, LocalX and LocalY relative
	// to the border box of the element (see MouseEvent).
	X, Y, LocalX, LocalY float32
	// DeltaX and DeltaY are the movement since the previous drag event
	// (since the press for OnDragStart).
	DeltaX, DeltaY float32
	// OffsetX and OffsetY are the movement since the press: an element
	// moved by them follows the pointer exactly.
	OffsetX, OffsetY float32
	// VelocityX and VelocityY are the pointer velocity in logical pixels
	// per second, from the last 100 ms of moves; meaningful at
	// OnDragEnd (a fling), zero when the pointer stopped before the
	// release or the drag was cancelled.
	VelocityX, VelocityY float32
	// Button is the button of the drag (MouseButtonLeft).
	Button MouseButton
	// Modifier keys held during the event.
	Shift, Ctrl, Alt, Meta bool

	st *eventState
}

// StopPropagation does nothing for drags, which go to one element; it
// exists for symmetry with the other events.
func (e DragEvent) StopPropagation() { e.st.stop() }

// PreventDefault marks the event (see MouseEvent).
func (e DragEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e DragEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// ---------------------------------------------------------------------
// Handler props.

// eventKind is an event a handler prop listens to.
type eventKind uint8

const (
	evClick eventKind = iota + 1
	evDoubleClick
	evContextMenu
	evMouseDown
	evMouseUp
	evMouseMove
	evMouseEnter
	evMouseLeave
	evPointerDown
	evPointerMove
	evPointerUp
	evPointerCancel
	evPointerEnter
	evPointerLeave
	evDragStart
	evDrag
	evDragEnd
	evLongPress
	numEventKinds
)

// eventNames are the names On takes, the DOM's where one exists.
var eventNames = [numEventKinds]string{
	evClick:         "click",
	evDoubleClick:   "dblclick",
	evContextMenu:   "contextmenu",
	evMouseDown:     "mousedown",
	evMouseUp:       "mouseup",
	evMouseMove:     "mousemove",
	evMouseEnter:    "mouseenter",
	evMouseLeave:    "mouseleave",
	evPointerDown:   "pointerdown",
	evPointerMove:   "pointermove",
	evPointerUp:     "pointerup",
	evPointerCancel: "pointercancel",
	evPointerEnter:  "pointerenter",
	evPointerLeave:  "pointerleave",
	evDragStart:     "dragstart",
	evDrag:          "drag",
	evDragEnd:       "dragend",
	evLongPress:     "longpress",
}

func (k eventKind) String() string {
	if k > 0 && k < numEventKinds {
		return eventNames[k]
	}
	return "?"
}

func (k eventKind) mask() uint32 { return 1 << k }

// Masks of handler families.
const (
	clickMask = 1<<evClick | 1<<evDoubleClick
	dragMask  = 1<<evDragStart | 1<<evDrag | 1<<evDragEnd
)

// handlerType names the handler type of an event kind, for messages.
func (k eventKind) handlerType() string {
	switch {
	case k >= evClick && k <= evMouseLeave:
		return "func(tela.MouseEvent)"
	case k >= evDragStart && k <= evDragEnd:
		return "func(tela.DragEvent)"
	}
	return "func(tela.PointerEvent)"
}

// checkHandler reports whether h has the handler type of k.
func (k eventKind) checkHandler(h any) bool {
	switch k.handlerType() {
	case "func(tela.MouseEvent)":
		f, ok := h.(func(MouseEvent))
		return ok && f != nil
	case "func(tela.DragEvent)":
		f, ok := h.(func(DragEvent))
		return ok && f != nil
	}
	f, ok := h.(func(PointerEvent))
	return ok && f != nil
}

func eventKindOf(name string) eventKind {
	for k := evClick; k < numEventKinds; k++ {
		if eventNames[k] == name {
			return k
		}
	}
	return 0
}

// EventProp is an event handler prop, such as OnClick.
type EventProp struct {
	// Name is the event name ("click").
	Name string
	// Handler is the handler function as the public API typed it.
	Handler any

	kind eventKind
}

// On adds an event handler prop. The public API wraps it (OnClick...).
// event is one of "click", "dblclick", "contextmenu", "mousedown",
// "mouseup", "mousemove", "mouseenter", "mouseleave" (handlers
// func(MouseEvent)), "pointerdown", "pointermove", "pointerup",
// "pointercancel", "pointerenter", "pointerleave", "longpress"
// (func(PointerEvent)), "dragstart", "drag" and "dragend"
// (func(DragEvent)). It panics on another name or handler type.
func On(event string, handler any) Prop {
	k := eventKindOf(event)
	if k == 0 {
		panic(fmt.Sprintf("core: On(%q): unknown event", event))
	}
	if !k.checkHandler(handler) {
		panic(fmt.Sprintf("core: On(%q) with a %T handler; pass a non-nil %s", event, handler, k.handlerType()))
	}
	return Prop{name: "On", apply: func(d *desc) {
		d.events = append(d.events, EventProp{Name: event, Handler: handler, kind: k})
		d.evMask |= k.mask()
	}}
}

// Disabled disables a primitive (any element, like a disabled form
// control): no pointer handler runs on it or on its descendants, and
// events from its descendants do not reach it; its ancestors still
// receive them. The element does not become :active, and its classes'
// disabled: (and enabled:) variants, and the group-disabled: and
// peer-disabled: variants of others, apply.
func Disabled(disabled bool) Prop {
	return Prop{name: "Disabled", apply: func(d *desc) { d.disabled = disabled }}
}
