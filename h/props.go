package h

import (
	"image"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Class adds Tailwind v4 classes ("flex items-center gap-4 hover:bg-white/10").
// It may repeat on an element; the strings concatenate in order and a later
// class wins over an earlier one that sets the same property. An unknown
// class is reported in dev mode (on in development builds, see tela.App.Dev)
// with the closest valid one.
func Class(classes string) tela.Prop { return core.Class(classes) }

// Style adds typed style props from package tela, for values computed in
// code:
//
//	Style(tela.Width(w), tela.Bg(tela.Hex("#1e1e2e")))
//
// They apply after the element's classes and win over them.
func Style(props ...tela.StyleProp) tela.Prop { return core.Style(props...) }

// Key gives an element an identity among its siblings: when a list changes,
// the element with the same key keeps its state wherever it moves. k is a
// comparable value, such as an item ID: a slice, a map or a func does not
// compile, and a nil interface, or one holding a value that cannot be
// compared, panics. Elements built from data (Map) should have keys. A
// component or a fragment takes its key from tela.WithKey.
func Key[K comparable](k K) tela.Prop { return core.Key(k) }

// Src sets the image of an Img. Pass the same image.Image value on every
// build: it is converted for drawing only when another image is given.
func Src(img image.Image) tela.Prop {
	if img == nil {
		panic("h: Src(nil): pass an image.Image, for example one decoded with image.Decode")
	}
	return core.Src(img)
}

// Href sets the destination of an A. Tela does not open links by itself
// yet: handle OnClick.
func Href(url string) tela.Prop { return core.Href(url) }

// Disabled disables an element, like a disabled form control: no pointer
// handler runs on it or on its descendants, and their events do not reach
// it (its ancestors still receive them); it does not become active, and
// neither it nor its descendants can take the focus (a focused one loses
// it). Its disabled: and enabled: variants apply, as do the
// group-disabled: and peer-disabled: variants of others.
//
//	Button(Disabled(saving), Class("bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"),
//	    OnClick(save), Text("Save"))
func Disabled(disabled bool) tela.Prop { return core.Disabled(disabled) }

// OnClick calls handler when the element is clicked: the primary button is
// pressed and released over it or its descendants (the click goes to the
// nearest element that contains both the press and the release, as in the
// DOM), even if the pointer moved in between, unless the press became a
// drag or a long press. The event bubbles to the ancestors.
//
//	Button(OnClick(func(e tela.MouseEvent) { setCount.Set(count + 1) }), Text("+1"))
//
// Every handler prop panics when handler is nil.
func OnClick(handler func(tela.MouseEvent)) tela.Prop { return onMouse("click", "OnClick", handler) }

// OnDoubleClick calls handler on the second click of a double click: two
// clicks within 500 ms and 5 px (300 ms and 100 px for touch). The first
// click is not delayed: OnClick runs for both, with e.ClickCount 1 and 2.
//
//	Div(OnDoubleClick(func(tela.MouseEvent) { setEditing.Set(true) }), Text(title))
func OnDoubleClick(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("dblclick", "OnDoubleClick", handler)
}

// OnContextMenu calls handler when the secondary (right) button is
// released over the element.
//
//	Div(OnContextMenu(func(e tela.MouseEvent) { openMenu(e.X, e.Y) }))
func OnContextMenu(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("contextmenu", "OnContextMenu", handler)
}

// OnMouseDown calls handler when a button is pressed over the element.
//
//	Div(OnMouseDown(func(e tela.MouseEvent) {
//	    if e.Button == tela.MouseButtonMiddle {
//	        closeTab()
//	    }
//	}))
func OnMouseDown(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("mousedown", "OnMouseDown", handler)
}

// OnMouseUp calls handler when a button is released over the element.
//
//	Div(OnMouseUp(func(tela.MouseEvent) { setPressed.Set(false) }))
func OnMouseUp(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("mouseup", "OnMouseUp", handler)
}

// OnMouseMove calls handler when the pointer moves over the element.
// Moves are batched: state set here is applied once per frame.
//
//	Div(OnMouseMove(func(e tela.MouseEvent) { pos.Set(e.LocalX) }))
func OnMouseMove(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("mousemove", "OnMouseMove", handler)
}

// OnMouseEnter calls handler when the pointer enters the element or one
// of its descendants from outside. It does not bubble.
//
//	Div(OnMouseEnter(func(tela.MouseEvent) { setShowTip.Set(true) }))
func OnMouseEnter(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("mouseenter", "OnMouseEnter", handler)
}

// OnMouseLeave calls handler when the pointer leaves the element and its
// descendants. It does not bubble.
//
//	Div(OnMouseLeave(func(tela.MouseEvent) { setShowTip.Set(false) }))
func OnMouseLeave(handler func(tela.MouseEvent)) tela.Prop {
	return onMouse("mouseleave", "OnMouseLeave", handler)
}

// OnPointerDown calls handler when a pointer (mouse, touch or pen) is
// pressed over the element: the first button of a mouse, the contact of a
// touch. While a touch or pen is down its events go to this element.
// e.PreventDefault() suppresses the mouse events of the press (not the
// click).
//
//	Div(OnPointerDown(func(e tela.PointerEvent) {
//	    if e.Kind == tela.PointerPen {
//	        startStroke(e.LocalX, e.LocalY)
//	    }
//	}))
func OnPointerDown(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("pointerdown", "OnPointerDown", handler)
}

// OnPointerMove calls handler when a pointer moves over the element (or,
// for a touch or pen that pressed it, anywhere), and when a button is
// pressed or released while another is held.
//
//	Div(OnPointerMove(func(e tela.PointerEvent) { addPoint(e.LocalX, e.LocalY, e.Pressure) }))
func OnPointerMove(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("pointermove", "OnPointerMove", handler)
}

// OnPointerUp calls handler when the last button of a pointer is released
// over the element.
//
//	Div(OnPointerUp(func(tela.PointerEvent) { endStroke() }))
func OnPointerUp(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("pointerup", "OnPointerUp", handler)
}

// OnPointerCancel calls handler when the system takes a pointer away
// during a press (a touch turned into a system gesture, a lost grab):
// treat it as an aborted press, not a release.
//
//	Div(OnPointerCancel(func(tela.PointerEvent) { discardStroke() }))
func OnPointerCancel(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("pointercancel", "OnPointerCancel", handler)
}

// OnPointerEnter calls handler when a pointer enters the element or one of
// its descendants. It does not bubble.
//
//	Div(OnPointerEnter(func(e tela.PointerEvent) { setHoverKind.Set(e.Kind) }))
func OnPointerEnter(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("pointerenter", "OnPointerEnter", handler)
}

// OnPointerLeave calls handler when a pointer leaves the element and its
// descendants. It does not bubble.
//
//	Div(OnPointerLeave(func(tela.PointerEvent) { setHovered.Set(false) }))
func OnPointerLeave(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("pointerleave", "OnPointerLeave", handler)
}

// OnLongPress calls handler when the primary button (or a touch) stays
// pressed on the element for 500 ms without moving more than a few pixels
// (4 for a mouse, 18 for touch and pens). The press then does not click
// or drag. It goes to the innermost element with OnLongPress and does not
// bubble.
//
//	Div(OnLongPress(func(e tela.PointerEvent) { openMenu(e.X, e.Y) }), Text(name))
func OnLongPress(handler func(tela.PointerEvent)) tela.Prop {
	return onPointer("longpress", "OnLongPress", handler)
}

// OnDragStart calls handler when a press on the element becomes a drag:
// the pointer moved more than a few pixels (4 for a mouse, 18 for touch
// and pens) before a long press. The press then does not click. Drag
// events go to the innermost element with drag handlers and do not bubble.
//
//	pos := tela.UseValue(0.0)
//	start := tela.UseRef(0.0)
//	Div(Class("size-10 rounded-full bg-indigo-500"),
//	    Style(tela.TranslateFn(func() (x, y float32) { return float32(pos.Get()), 0 })),
//	    OnDragStart(func(tela.DragEvent) { start.Current = pos.Get() }),
//	    OnDrag(func(e tela.DragEvent) { pos.Set(start.Current + float64(e.OffsetX)) }),
//	)
func OnDragStart(handler func(tela.DragEvent)) tela.Prop {
	return onDrag("dragstart", "OnDragStart", handler)
}

// OnDrag calls handler for every move of a drag (see OnDragStart), with
// the movement since the previous one (DeltaX, DeltaY) and since the press
// (OffsetX, OffsetY). Moves are batched: state set here is applied once
// per frame.
//
//	Div(OnDrag(func(e tela.DragEvent) { pos.Update(func(p float64) float64 { return p + float64(e.DeltaX) }) }))
func OnDrag(handler func(tela.DragEvent)) tela.Prop { return onDrag("drag", "OnDrag", handler) }

// OnDragEnd calls handler when a drag ends: the button is released (or the
// press cancelled). VelocityX and VelocityY give the release velocity in
// pixels per second, for flings.
//
//	Div(OnDragEnd(func(e tela.DragEvent) {
//	    if e.VelocityX > 800 {
//	        dismiss()
//	    }
//	}))
func OnDragEnd(handler func(tela.DragEvent)) tela.Prop {
	return onDrag("dragend", "OnDragEnd", handler)
}

// OnWheel calls handler when a mouse wheel turns or a touchpad scrolls
// over the element (the DOM's wheel event). The event bubbles. Unless a
// handler calls e.PreventDefault(), the nearest scrolling element (with
// overflow-auto or overflow-scroll) that can move in that direction
// scrolls, chaining to its scrolling ancestors at its edges unless it has
// overscroll-contain.
//
//	Div(OnWheel(func(e tela.WheelEvent) {
//	    if e.Ctrl {
//	        e.PreventDefault()
//	        zoomBy(-e.DeltaY)
//	    }
//	}))
func OnWheel(handler func(tela.WheelEvent)) tela.Prop {
	if handler == nil {
		panic("h: OnWheel(nil): pass a handler, for example OnWheel(func(e tela.WheelEvent) { ... })")
	}
	return core.On("wheel", handler)
}

// OnScroll calls handler after the element's scroll offsets changed, by
// the user or by code: at most once per frame, on the scrolling element
// itself (it does not bubble), with its offsets and sizes. To tie a style
// to the scroll position without rebuilding, use tela.UseScroll instead.
//
//	Div(Class("h-96 overflow-y-auto"), OnScroll(func(e tela.ScrollEvent) {
//	    setAtTop.Set(e.ScrollTop == 0)
//	}), rows)
func OnScroll(handler func(tela.ScrollEvent)) tela.Prop {
	if handler == nil {
		panic("h: OnScroll(nil): pass a handler, for example OnScroll(func(e tela.ScrollEvent) { ... })")
	}
	return core.On("scroll", handler)
}

func onMouse(event, name string, handler func(tela.MouseEvent)) tela.Prop {
	if handler == nil {
		panic("h: " + name + "(nil): pass a handler, for example " + name + "(func(e tela.MouseEvent) { ... })")
	}
	return core.On(event, handler)
}

func onPointer(event, name string, handler func(tela.PointerEvent)) tela.Prop {
	if handler == nil {
		panic("h: " + name + "(nil): pass a handler, for example " + name + "(func(e tela.PointerEvent) { ... })")
	}
	return core.On(event, handler)
}

func onDrag(event, name string, handler func(tela.DragEvent)) tela.Prop {
	if handler == nil {
		panic("h: " + name + "(nil): pass a handler, for example " + name + "(func(e tela.DragEvent) { ... })")
	}
	return core.On(event, handler)
}
