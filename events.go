package tela

import "github.com/kuronosu/tela/internal/core"

// The Key and Code constants (keys_gen.go) are generated from the
// platform layer's lists, so they cover every key a backend reports.
//
//go:generate go run ./internal/gen/keys

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

// Keyboard and focus events follow the DOM too. Key events go to the
// focused element and bubble; with nothing focused they go to the
// outermost element of the window's content.
//
//	Div(OnKeyDown(func(e tela.KeyboardEvent) {
//	    if e.Key == "Escape" {
//	        close()
//	    }
//	    if e.Ctrl && e.Code == tela.CodeKeyS {
//	        e.PreventDefault()
//	        save()
//	    }
//	}))
//
// After the handlers, Tela runs the default actions unless one called
// PreventDefault: Tab and Shift+Tab move the focus through the focusable
// elements (a Button, or any element with h.TabIndex), wrapping around in
// the window; Enter on a focused Button clicks it on keydown, Space on
// keyup. A press of the primary button focuses the nearest focusable
// element under the pointer and blurs the focused one otherwise
// (PreventDefault on h.OnMouseDown or h.OnPointerDown prevents it).

// Key is a logical key: what the key means with the active layout and
// modifiers (a W3C KeyboardEvent key value). It is a named key, one of the
// Key constants ("Enter", "Escape", "ArrowLeft", "Shift"), or the
// character the key types ("a", "A", "é", " " for the space bar). Ctrl
// does not change it: Ctrl+S has Key "s". Compare it with a constant or a
// string literal:
//
//	if e.Key == tela.KeyEnter || e.Key == "+" { ... }
type Key = core.KeyValue

// Code is a physical key: its position on the keyboard whatever the
// layout (a W3C KeyboardEvent code value), one of the Code constants.
// Use it for shortcuts that depend on the position (WASD), and Key for
// those that depend on the meaning (Ctrl+Z):
//
//	if e.Code == tela.CodeKeyW || e.Code == "ArrowUp" { moveUp() }
type Code = core.CodeValue

// KeyLocation tells apart keys with the same meaning: the left and right
// Shift, Control, Alt and Meta keys, and the numeric keypad.
type KeyLocation = core.KeyLocation

// The key locations (the DOM's KeyboardEvent.location).
const (
	// KeyLocationStandard is every key not listed below.
	KeyLocationStandard = core.KeyLocationStandard
	// KeyLocationLeft is the left Shift, Control, Alt or Meta key.
	KeyLocationLeft = core.KeyLocationLeft
	// KeyLocationRight is the right Shift, Control, Alt (AltGr) or Meta key.
	KeyLocationRight = core.KeyLocationRight
	// KeyLocationNumpad is a key of the numeric keypad.
	KeyLocationNumpad = core.KeyLocationNumpad
)

// KeyboardEvent is passed to h.OnKeyDown and h.OnKeyUp: the logical Key,
// the physical Code, its Location, Repeat for a keydown generated while
// the key is held, and the modifiers held after the event (Shift, Ctrl,
// Alt, Meta; Meta is the Windows, Command or Super key). StopPropagation
// stops the bubbling; PreventDefault cancels the key's default action
// (Tab's focus move, the click of Enter or Space on a button). There is
// no text field: typing text is the job of text inputs.
//
//	OnKeyDown(func(e tela.KeyboardEvent) {
//	    switch {
//	    case e.Key == tela.KeyArrowDown:
//	        e.PreventDefault()
//	        setIndex.Update(func(i int) int { return min(i+1, n-1) })
//	    case e.Ctrl && e.Key == "a":
//	        selectAll()
//	    }
//	})
type KeyboardEvent = core.KeyboardEvent

// FocusEvent is passed to h.OnFocus and h.OnBlur. Like React's onFocus and
// onBlur, focus events bubble: a container's OnFocus also runs when one
// of its descendants gains the focus.
//
//	Div(OnFocus(func(tela.FocusEvent) { setEditing.Set(true) }),
//	    OnBlur(func(tela.FocusEvent) { setEditing.Set(false) }),
//	    ...)
type FocusEvent = core.FocusEvent
