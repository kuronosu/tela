package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// OnKeyDown calls handler when a key is pressed (and again for each repeat
// while it is held, with e.Repeat) while the element or one of its
// descendants has the focus; the event bubbles. With nothing focused, key
// events go to the outermost element of the window's content.
// e.PreventDefault() cancels the key's default action: the focus move of
// Tab, the click of Enter or Space on a focused Button.
//
//	Div(OnKeyDown(func(e tela.KeyboardEvent) {
//	    if e.Key == "Escape" {
//	        close()
//	    }
//	    if e.Ctrl && e.Code == tela.CodeKeyS {
//	        save()
//	    }
//	}))
func OnKeyDown[F ~func() | ~func(tela.KeyboardEvent)](handler F) tela.Prop {
	return onEvent[tela.KeyboardEvent]("keydown", "OnKeyDown", handler)
}

// OnKeyUp calls handler when a key is released while the element or one
// of its descendants has the focus; the event bubbles. Keys held when the
// window loses the focus are released then. e.PreventDefault() cancels
// the click of Space on a focused Button.
//
//	Div(OnKeyUp(func(e tela.KeyboardEvent) {
//	    if e.Key == tela.KeyShift {
//	        setMulti.Set(false)
//	    }
//	}))
func OnKeyUp[F ~func() | ~func(tela.KeyboardEvent)](handler F) tela.Prop {
	return onEvent[tela.KeyboardEvent]("keyup", "OnKeyUp", handler)
}

// OnFocus calls handler when the element or one of its descendants gains
// the focus (React's onFocus: it bubbles), and again when the window
// regains the keyboard focus.
//
//	Div(TabIndex(0), OnFocus(func() { setActive.Set(true) }))
func OnFocus[F ~func() | ~func(tela.FocusEvent)](handler F) tela.Prop {
	return onEvent[tela.FocusEvent]("focus", "OnFocus", handler)
}

// OnBlur calls handler when the element or one of its descendants loses
// the focus (React's onBlur: it bubbles), also when the window loses the
// keyboard focus. Unmounting the focused element does not call it.
//
//	Div(TabIndex(0), OnBlur(func() { commit() }))
func OnBlur[F ~func() | ~func(tela.FocusEvent)](handler F) tela.Prop {
	return onEvent[tela.FocusEvent]("blur", "OnBlur", handler)
}

// TabIndex makes an element focusable (a Button already is) and places it
// in the Tab order: 0 in tree order, -1 out of it (focusable by click and
// by ElementRef.Focus only). A positive index puts the element before
// every 0, in ascending order; avoid it, since the Tab order then no
// longer follows the layout. Disabled, display: none and invisible
// elements are not focusable; the focused element is blurred when it
// becomes one of them.
//
//	Div(TabIndex(0), Class("rounded-lg p-4 focus-visible:ring-2 focus-visible:ring-indigo-500"),
//	    OnKeyDown(func(e tela.KeyboardEvent) {
//	        if e.Key == tela.KeyEnter {
//	            open()
//	        }
//	    }),
//	    Text("Inbox"))
func TabIndex(i int) tela.Prop { return core.TabIndex(i) }

// AutoFocus focuses the element when it first appears, in the frame that
// shows it (React's autoFocus), unless another element took the focus
// since it was mounted. When several AutoFocus elements appear at once,
// the first in tree order wins.
//
//	Button(AutoFocus(true), OnClick(confirm), Text("OK"))
func AutoFocus(on bool) tela.Prop { return core.AutoFocus(on) }

// Ref attaches ref, from tela.UseElementRef, to the element: while it is
// mounted, ref.Focus(), ref.Blur(), ref.Focused() and the scroll methods
// (ref.ScrollTo, ScrollBy, ScrollIntoView, ScrollOffset) act on it, and
// tela.UseScroll(ref) follows its scroll offsets. It panics on the zero
// ElementRef.
//
//	input := tela.UseElementRef()
//	tela.UseMount(func() func() {
//	    input.Focus()
//	    return nil
//	})
//	return Div(Ref(input), TabIndex(0), Class("focus:ring-2"), Text("Name"))
func Ref(ref tela.ElementRef) tela.Prop { return core.AttachRef(ref) }
