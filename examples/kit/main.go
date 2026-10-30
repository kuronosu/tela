// Command kit is the gallery of package kit: it opens on an Introduction, and
// has a sidebar with every component and, for each one, a page that shows its
// variants, sizes and states in a small realistic composition, as shadcn/ui's
// documentation does. The header has a light/dark switch, a Style picker that
// applies a second recipe, built from kit.DefaultStyle with a few edits, to
// the whole gallery, to show how little customizing the kit takes, and a
// Language menu (English, Spanish, Arabic) that sets the language and the
// direction of the whole window, so every page can be seen right to left. An
// Internationalization page shows the messages and formats behind it.
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
// Previews is a page's name, apart from the gallery's and the translated
// ones.
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

// Previews has the whole gallery ("Gallery", "Gallery Rounded" in the
// second style on the Button page, and "Gallery Arabic" on the Date Picker
// page, right to left) and one entry per page, named as the page is: the
// page over the window's background, in the default style and in English.
// Three more show the Internationalization page in each language
// ("Internationalization es", "Internationalization ar"; the plain name is
// English).
var Previews = previews()

func previews() tela.Previews {
	p := tela.Previews{
		"Gallery":         {Root: Gallery{}},
		"Gallery Rounded": {Root: Gallery{Page: "Button", Style: "rounded"}},
		"Gallery Arabic":  {Root: Gallery{Page: "DatePicker", Locale: "ar"}},
	}
	for _, pg := range sidebar() {
		p[pg.Name] = tela.Preview{Root: Canvas{Page: pg.Name}}
	}
	for _, tag := range languages[1:] {
		p["Internationalization "+tag] = tela.Preview{Root: Canvas{Page: "Internationalization", Locale: tag}}
	}
	return p
}

func main() { tela.Main(App) }
