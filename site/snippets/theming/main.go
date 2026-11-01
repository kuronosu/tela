package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

func panel(scheme tela.ColorScheme, title string) tela.Node {
	return Div(Scheme(scheme), Class("flex flex-1 flex-col gap-3 bg-background p-5 text-foreground"),
		Span(Class("text-xs font-medium tracking-widest text-muted-foreground uppercase"), Text(title)),
		kit.Card{Title: Text("Tokens"), Description: Text("bg-card, text-muted-foreground"),
			Footer: kit.Button{Children: Text("bg-primary")}},
	)
}

var App = tela.App{
	Title: "Theme", Width: 600, Height: 260,
	Theme: &theme,
	Root: Div(Class("flex h-full"),
		panel(tela.Light, "Light"),
		panel(tela.Dark, "Dark")),
}

func main() { tela.Main(App) }
