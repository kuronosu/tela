package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// The glass budget. A backdrop-blur recomputes its pixels whenever what is
// behind it changes, and the backdrop of a card that scrolls over a fixed
// image changes on every scroll frame, so blur is spent where there is
// detail to frost: the header (text and cards scroll under it), the dialog
// and the toast (the whole page is behind them), the donut's disc and the
// chips over gradients. A card over the smooth aurora gets the same look
// from a translucent fill and a hairline border, without the blur.
const (
	// bar is a bar of glass over content that moves (the header): blurred.
	bar = "border-border bg-white/40 backdrop-blur-xl dark:bg-slate-950/30"
)

// Backdrop is what the glass is over: a fixed layer behind the page, so the
// content scrolls over it. The image is made for the scheme (the component
// reads it with tela.UseColorScheme, which is what that hook is for) and
// covers the window.
func Backdrop() tela.Node { return tela.C(backdrop) }

func backdrop() tela.Node {
	img := "bg-[url(img/aurora-light.jpg)]"
	if tela.UseColorScheme() == tela.Dark {
		img = "bg-[url(img/aurora-dark.jpg)]"
	}
	return Div(Class("pointer-events-none fixed inset-0 -z-10 bg-cover bg-center", img), AriaHidden(true))
}
