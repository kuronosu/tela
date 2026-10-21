package tela

import "github.com/kuronosu/tela/internal/core"

// Rect is a rectangle in logical pixels: the position of its top-left
// corner (X, Y) and its size (Width, Height). Right, Bottom and Contains
// derive from them. It is what [ElementRef].Bounds and [UseBounds] return,
// in window coordinates (the window's top-left corner is 0, 0), like the
// DOM's DOMRect.
type Rect = core.Rect

// UseBounds returns a [Value] that follows the bounds of the element ref
// is attached to with h.Ref: its border box in window coordinates, as
// [ElementRef].Bounds returns it (moved by scrolling and transforms, the
// zero Rect while the element is not mounted or displayed). It is a
// ResizeObserver that also sees the position: use it to size or place one
// element after another, or to anchor a floating element.
//
// The Value is updated at the end of every frame, when the bounds changed
// since the last one, so a component that reads it rebuilds, and a
// deferred style closure that reads it lays out or repaints, in the frame
// after the change. Frames where nothing moved cost one comparison and
// request no further frame. The same Value is returned on every build.
//
//	func Meter() tela.Node {
//	    track := tela.UseElementRef()
//	    b := tela.UseBounds(track)
//	    return Div(Ref(track), Class("h-2 rounded bg-slate-200"),
//	        Div(Class("h-2 rounded bg-indigo-500"),
//	            Style(tela.WidthFn(func() float32 { return b.Get().Width * 0.4 }))))
//	}
//
// In an event handler, [ElementRef].Bounds gives the same rectangle
// without a Value: a seek bar turns the pointer's x into a fraction with
// e.X-ref.Bounds().X over ref.Bounds().Width.
func UseBounds(ref ElementRef) *Value[Rect] { return core.UseBounds(ref) }

// UseSize returns a [Value] that follows the size of the element ref is
// attached to with h.Ref: its border box as laid out, like a
// ResizeObserver (borderBoxSize). Scrolling and transforms never change
// it, so it does not move while a list scrolls or a card animates its
// scale; it is the zero Size while the element is not mounted or not
// displayed, and for inline elements of a paragraph. Use it to size one
// element after another, or to make a popover as wide as its trigger.
//
// The Value is updated in every frame that lays out, before painting: a
// deferred style closure of the paint phase that reads it (TranslateFn,
// OpacityFn, BgFn) follows in that frame; a component that reads it
// rebuilds, and a WidthFn or HeightFn lays out, in the next frame. Frames
// that only scroll or animate transforms do not check it. The same Value
// is returned on every build.
//
//	func Tabs() tela.Node {
//	    tab := tela.UseElementRef()
//	    size := tela.UseSize(tab)
//	    return Div(Class("relative flex"),
//	        Button(Ref(tab), Class("px-4 py-2"), Text("Overview")),
//	        Div(Class("absolute bottom-0 h-0.5 bg-indigo-500"),
//	            Style(tela.WidthFn(func() float32 { return size.Get().Width }))))
//	}
func UseSize(ref ElementRef) *Value[Size] { return core.UseSize(ref) }
