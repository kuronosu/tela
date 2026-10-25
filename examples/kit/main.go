// Command kit is the gallery of package kit: a sidebar with every component
// and, for each one, a page that shows its variants, sizes and states in a
// small realistic composition, as shadcn/ui's documentation does. The header
// has a light/dark switch and a Style picker that applies a second recipe,
// built from kit.DefaultStyle with a few edits, to the whole gallery, to show
// how little customizing the kit takes.
//
// Everything on the screen is a kit component, laid out with the elements of
// package h and the theme's token classes. The page of a component is also a
// preview of its own, so the component can be developed against it:
//
//	tela snapshot --preview Button ./examples/kit -o button.png
//	tela dev --preview Calendar ./examples/kit
//	tela inspect --preview Select ./examples/kit
//
// `tela snapshot --preview Gallery ./examples/kit` renders the whole window,
// and `--dark` renders the dark scheme. The previews of the components show
// the page without the sidebar, over the window's background; every key of
// Previews is the name of the kit component.
//
// The calendars and date pickers pin "today" to 25 October 2026 (see today
// in dates.go), so a screenshot of the gallery does not change from one day
// to the next.
package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

// App is the window: main runs it, the tests mount it (telatest.MountApp)
// and `tela snapshot` renders it at the same size, with the same theme.
var App = tela.App{
	Title:     "Tela - kit",
	AppID:     "dev.telaui.kit",
	Width:     1280,
	Height:    800,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Gallery{},
}

// Previews has the whole gallery ("Gallery", and "Gallery Rounded" in the
// second style, on the Button page) and one entry per component, named as
// the component is: the page of that component over the window's
// background, in the default style.
var Previews = previews()

func previews() tela.Previews {
	p := tela.Previews{
		"Gallery":         {Root: Gallery{}},
		"Gallery Rounded": {Root: Gallery{Page: "Button", Style: "rounded"}},
	}
	for _, pg := range pages {
		p[pg.Name] = tela.Preview{Root: Canvas{Page: pg.Name}}
	}
	return p
}

func main() { tela.Main(App) }
