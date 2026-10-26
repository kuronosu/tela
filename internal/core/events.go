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
	// rehover marks the enter and leave events of a new hit test after
	// scrolled content moved under a pointer that did not move (rehover).
	rehover bool
}

// Rehovered reports whether e is an enter or leave event that a new hit
// test fired because scrolled content moved under a resting pointer
// (Chromium's fake mouse move, ADR-057), not because the pointer moved.
// Package aria uses it so that a menu scrolled by the keyboard under the
// mouse keeps its highlight (ADR-164).
func Rehovered(e PointerEvent) bool { return e.rehover }

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

// WheelEvent is passed to OnWheel: a mouse wheel notch or a touchpad
// scroll over the element (the DOM's wheel event). It bubbles; its default
// action scrolls the nearest scroll container that can move in its
// direction (scroll.go), which PreventDefault cancels.
type WheelEvent struct {
	// X and Y are the pointer position in the window, LocalX and LocalY
	// relative to the border box of the element whose handler runs (see
	// MouseEvent).
	X, Y, LocalX, LocalY float32
	// DeltaX and DeltaY are the scroll distance in logical pixels, with
	// the DOM's signs: positive DeltaY scrolls down (toward the end of the
	// content), positive DeltaX to the right. A wheel notch is 53 pixels;
	// Shift turns a notch of a vertical wheel into a horizontal one.
	DeltaX, DeltaY float32
	// Modifier keys held during the event.
	Shift, Ctrl, Alt, Meta bool

	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e WheelEvent) StopPropagation() { e.st.stop() }

// PreventDefault cancels the scroll the event would cause.
func (e WheelEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e WheelEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// ScrollEvent is passed to OnScroll after the scroll offset of a scroll
// container changed (the DOM's scroll event: at most once per frame, on
// the scroll container only; it does not bubble). The fields are the
// DOM's Element scroll properties, in logical pixels.
type ScrollEvent struct {
	// ScrollLeft and ScrollTop are the scroll offsets: how far the
	// content is scrolled from its start. In a right-to-left container
	// (Dir("rtl")) the start is the right: ScrollLeft is 0 there and
	// negative toward the left, as in the DOM.
	ScrollLeft, ScrollTop float32
	// ScrollWidth and ScrollHeight are the size of the scrollable content
	// (at least the client size); the largest offsets are ScrollWidth -
	// ClientWidth and ScrollHeight - ClientHeight.
	ScrollWidth, ScrollHeight float32
	// ClientWidth and ClientHeight are the size of the visible area: the
	// padding box without the scrollbars' gutters.
	ClientWidth, ClientHeight float32

	st *eventState
}

// StopPropagation does nothing: scroll events do not bubble. It exists
// for symmetry with the other events.
func (e ScrollEvent) StopPropagation() { e.st.stop() }

// PreventDefault marks the event; the scroll already happened.
func (e ScrollEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e ScrollEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// InputEvent is passed to OnBeforeInput before a text field's value
// changes by an edit (the DOM's beforeinput): InputType names the edit
// with the W3C Input Events Level 2 names ("insertText",
// "insertFromPaste", "insertLineBreak", "deleteContentBackward",
// "deleteWordBackward", "deleteByCut", "historyUndo",
// "insertCompositionText"...), Data is the text it inserts (empty for
// deletions) and IsComposing reports an IME composition in progress. It
// bubbles. PreventDefault cancels the edit, except during a composition
// and for the text an IME deletes around the caret, edits that cannot be
// canceled (as in browsers).
type InputEvent struct {
	InputType   string
	Data        string
	IsComposing bool

	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e InputEvent) StopPropagation() { e.st.stop() }

// PreventDefault cancels the edit (not during a composition, nor an IME's
// deletion around the caret).
func (e InputEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e InputEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// ChangeEvent is passed to OnChange after every edit of a text field's
// value, as React's onChange (the DOM's input event, not its change
// event): Value is the new value (with the text being composed by an
// IME, as the DOM's input.value), InputType, Data and IsComposing are
// those of the edit (see InputEvent). It bubbles.
type ChangeEvent struct {
	Value       string
	InputType   string
	Data        string
	IsComposing bool

	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e ChangeEvent) StopPropagation() { e.st.stop() }

// PreventDefault marks the event; the edit already happened.
func (e ChangeEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e ChangeEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// CompositionEvent is passed to OnCompositionStart, OnCompositionUpdate
// and OnCompositionEnd while an input method (IME) composes text in a text
// field: Data is the text being composed (the DOM's CompositionEvent;
// for compositionstart, the selected text the composition replaces). It
// bubbles.
type CompositionEvent struct {
	Data string

	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e CompositionEvent) StopPropagation() { e.st.stop() }

// PreventDefault marks the event; compositions cannot be canceled.
func (e CompositionEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e CompositionEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// SelectEvent is passed to OnSelect when the user selects text in a text
// field, or code changes its selection (ElementRef.Select,
// SetSelectionRange): the DOM's select event. Start and End are the
// selection in UTF-8 byte offsets of the value. It bubbles.
type SelectEvent struct {
	Start, End int

	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e SelectEvent) StopPropagation() { e.st.stop() }

// PreventDefault marks the event; the selection already changed.
func (e SelectEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e SelectEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

// SubmitEvent is passed to OnSubmit when a Form is submitted: Enter in a
// single-line field of the form (HTML's implicit submission) or the
// activation of a submit button inside it. It bubbles. Tela does not
// navigate, so PreventDefault only marks it (it exists for familiarity).
type SubmitEvent struct {
	st *eventState
}

// StopPropagation stops the event from bubbling (see MouseEvent).
func (e SubmitEvent) StopPropagation() { e.st.stop() }

// PreventDefault marks the event.
func (e SubmitEvent) PreventDefault() { e.st.prevent() }

// DefaultPrevented reports whether a handler called PreventDefault.
func (e SubmitEvent) DefaultPrevented() bool { return e.st.defaultPrevented() }

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
	evKeyDown
	evKeyUp
	evFocus
	evBlur
	evWheel
	evScroll
	evBeforeInput
	evChange
	evCompositionStart
	evCompositionUpdate
	evCompositionEnd
	evSelect
	evSubmit
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
	evKeyDown:       "keydown",
	evKeyUp:         "keyup",
	evFocus:         "focus",
	evBlur:          "blur",
	evWheel:         "wheel",
	evScroll:        "scroll",
	// Text fields and forms (field.go). "change" is React's onChange,
	// the DOM's input event.
	evBeforeInput:       "beforeinput",
	evChange:            "change",
	evCompositionStart:  "compositionstart",
	evCompositionUpdate: "compositionupdate",
	evCompositionEnd:    "compositionend",
	evSelect:            "select",
	evSubmit:            "submit",
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
	case k == evKeyDown || k == evKeyUp:
		return "func(tela.KeyboardEvent)"
	case k == evFocus || k == evBlur:
		return "func(tela.FocusEvent)"
	case k == evWheel:
		return "func(tela.WheelEvent)"
	case k == evScroll:
		return "func(tela.ScrollEvent)"
	case k == evBeforeInput:
		return "func(tela.InputEvent)"
	case k == evChange:
		return "func(tela.ChangeEvent)"
	case k >= evCompositionStart && k <= evCompositionEnd:
		return "func(tela.CompositionEvent)"
	case k == evSelect:
		return "func(tela.SelectEvent)"
	case k == evSubmit:
		return "func(tela.SubmitEvent)"
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
	case "func(tela.KeyboardEvent)":
		f, ok := h.(func(KeyboardEvent))
		return ok && f != nil
	case "func(tela.FocusEvent)":
		f, ok := h.(func(FocusEvent))
		return ok && f != nil
	case "func(tela.WheelEvent)":
		f, ok := h.(func(WheelEvent))
		return ok && f != nil
	case "func(tela.ScrollEvent)":
		f, ok := h.(func(ScrollEvent))
		return ok && f != nil
	case "func(tela.InputEvent)":
		f, ok := h.(func(InputEvent))
		return ok && f != nil
	case "func(tela.ChangeEvent)":
		f, ok := h.(func(ChangeEvent))
		return ok && f != nil
	case "func(tela.CompositionEvent)":
		f, ok := h.(func(CompositionEvent))
		return ok && f != nil
	case "func(tela.SelectEvent)":
		f, ok := h.(func(SelectEvent))
		return ok && f != nil
	case "func(tela.SubmitEvent)":
		f, ok := h.(func(SubmitEvent))
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
	// noMouse marks a long-press handler that is not for the mouse
	// (OnTouchLongPress): it does not make a mouse press join the arena.
	noMouse bool
}

// OnTouchLongPress adds a long-press handler for touch contacts and pens
// only: unlike On("longpress") (h.OnLongPress), which recognizes a long
// press of every pointer, it does not make a mouse press take part in the
// gesture arena, so a mouse button held on the element or on a child for
// longer than the long-press time still clicks (a context menu's area,
// where only a touch or a pen that stays down opens the menu: Radix's
// ContextMenu). The handler never runs for a mouse, even when another
// handler of the element makes the mouse's press a long press. Reserved
// for package aria (UseContextMenuTrigger); it is not exposed by tela or h.
func OnTouchLongPress(handler func(PointerEvent)) Prop {
	if handler == nil {
		panic("core: OnTouchLongPress with a nil handler")
	}
	h := func(e PointerEvent) {
		if e.Kind != PointerMouse {
			handler(e)
		}
	}
	return Prop{name: "OnTouchLongPress", apply: func(d *desc) {
		d.events = append(d.events, EventProp{Name: "longpress", Handler: h, kind: evLongPress, noMouse: true})
		d.evMask |= evLongPress.mask()
	}}
}

// mouseLongPress reports whether a long-press handler of d is for the
// mouse too (any but OnTouchLongPress's).
func (d *desc) mouseLongPress() bool {
	for i := range d.events {
		if ep := &d.events[i]; ep.kind == evLongPress && !ep.noMouse {
			return true
		}
	}
	return false
}

// On adds an event handler prop. The public API wraps it (OnClick...).
// event is one of "click", "dblclick", "contextmenu", "mousedown",
// "mouseup", "mousemove", "mouseenter", "mouseleave" (handlers
// func(MouseEvent)), "pointerdown", "pointermove", "pointerup",
// "pointercancel", "pointerenter", "pointerleave", "longpress"
// (func(PointerEvent)), "dragstart", "drag" and "dragend"
// (func(DragEvent)), "keydown" and "keyup" (func(KeyboardEvent)), "focus"
// and "blur" (func(FocusEvent)), "wheel" (func(WheelEvent)), "scroll"
// (func(ScrollEvent)), "beforeinput" (func(InputEvent)), "change"
// (func(ChangeEvent)), "compositionstart", "compositionupdate" and
// "compositionend" (func(CompositionEvent)), "select" (func(SelectEvent))
// and "submit" (func(SubmitEvent)). It panics on another name or handler
// type.
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
// receive them. The element does not become :active, it and its
// descendants are not focusable (a focused one is blurred), and its
// classes' disabled: (and enabled:) variants, and the group-disabled:
// and peer-disabled: variants of others, apply.
func Disabled(disabled bool) Prop {
	return Prop{name: "Disabled", apply: func(d *desc) { d.disabled = disabled }}
}
