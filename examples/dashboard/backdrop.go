package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Shared class strings. The page is written once with light and dark: pairs.
//
// The glass budget. A backdrop-blur recomputes its pixels whenever what is
// behind it changes, and the backdrop of a card that scrolls over a fixed
// image changes on every scroll frame, so blur is spent where there is
// detail to frost: the header (text and cards scroll under it), the dialog
// and the toast (the whole page is behind them), the donut's disc and the
// chips over gradients. A card over the smooth aurora gets the same look
// from a translucent fill and a hairline border, without the blur.
const (
	// glass is a frosted panel: translucent white (or tinted) over the
	// backdrop, a bright hairline edge and a soft shadow.
	glass = "rounded-2xl border border-white/60 bg-white/55 dark:border-white/10 dark:bg-white/5"
	// lift is the hover reaction of an interactive panel. Only transform and
	// shadow transition: the fills do not, so switching the theme repaints
	// the page once instead of fading every card.
	lift = "transition-[transform,box-shadow] duration-300 ease-out hover:-translate-y-0.5 hover:shadow-xl hover:shadow-indigo-950/10"
	// bar is a bar of glass over content that moves (the header): blurred.
	bar = "border-white/50 bg-white/40 backdrop-blur-xl dark:border-white/10 dark:bg-slate-950/30"

	muted   = "text-slate-600 dark:text-slate-400"
	heading = "font-semibold tracking-tight text-slate-900 dark:text-white"
)

// Backdrop is what the glass is over: a fixed layer behind the page, so the
// content scrolls over it. The image is made for the scheme (the component
// reads it with tela.UseColorScheme, which is what that hook is for) and
// covers the window.
func Backdrop() tela.Node {
	img := "bg-[url(img/aurora-light.jpg)]"
	if tela.UseColorScheme() == tela.Dark {
		img = "bg-[url(img/aurora-dark.jpg)]"
	}
	return Div(Class("pointer-events-none fixed inset-0 -z-10 bg-cover bg-center", img), AriaHidden(true))
}
