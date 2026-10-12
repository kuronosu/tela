package h

import (
	"image"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Class adds Tailwind v4 classes ("flex items-center gap-4 hover:bg-white/10").
// It may repeat on an element; the strings concatenate in order and a later
// class wins over an earlier one that sets the same property. An unknown
// class is reported in dev mode (TELA_DEV=1) with the closest valid one.
func Class(classes string) tela.Prop { return core.Class(classes) }

// Style adds typed style props from package tela, for values computed in
// code:
//
//	Style(tela.Width(w), tela.Bg(tela.Hex("#1e1e2e")))
//
// They apply after the element's classes and win over them.
func Style(props ...tela.StyleProp) tela.Prop { return core.Style(props...) }

// Key gives an element an identity among its siblings: when a list changes,
// the element with the same key keeps its state wherever it moves. k must
// be a non-nil comparable value, such as an item ID; Key panics otherwise.
// Elements built from data (Map) should have keys.
func Key(k any) tela.Prop { return core.Key(k) }

// Src sets the image of an Img. Pass the same image.Image value on every
// build: it is converted for drawing only when another image is given.
func Src(img image.Image) tela.Prop {
	if img == nil {
		panic("h: Src(nil): pass an image.Image, for example one decoded with image.Decode")
	}
	return core.Src(img)
}

// Href sets the destination of an A. Opening links arrives with input in
// phase 4.
func Href(url string) tela.Prop { return core.Href(url) }

// OnClick calls handler when the element is clicked: pressed and released
// with the primary button on it. Handlers are stored in phase 3 and called
// from phase 4, when input arrives. OnClick panics when handler is nil.
func OnClick(handler func(tela.MouseEvent)) tela.Prop { return on("click", "OnClick", handler) }

// OnMouseDown calls handler when a mouse button is pressed on the element.
func OnMouseDown(handler func(tela.MouseEvent)) tela.Prop {
	return on("mousedown", "OnMouseDown", handler)
}

// OnMouseUp calls handler when a mouse button is released on the element.
func OnMouseUp(handler func(tela.MouseEvent)) tela.Prop {
	return on("mouseup", "OnMouseUp", handler)
}

// OnMouseEnter calls handler when the pointer enters the element.
func OnMouseEnter(handler func(tela.MouseEvent)) tela.Prop {
	return on("mouseenter", "OnMouseEnter", handler)
}

// OnMouseLeave calls handler when the pointer leaves the element.
func OnMouseLeave(handler func(tela.MouseEvent)) tela.Prop {
	return on("mouseleave", "OnMouseLeave", handler)
}

func on(event, name string, handler func(tela.MouseEvent)) tela.Prop {
	if handler == nil {
		panic("h: " + name + "(nil): pass a handler, for example " + name + "(func(e tela.MouseEvent) { ... })")
	}
	return core.On(event, handler)
}
