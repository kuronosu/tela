package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

var App = tela.App{
	Title: "Settings", Width: 480, Height: 380,
	Theme: &theme, RootClass: "bg-muted text-foreground",
	Root: Div(Class("flex h-full items-center justify-center"),
		kit.Card{
			Class:       "w-96",
			Title:       Text("Notifications"),
			Description: Text("Choose what Tela tells you."),
			Action:      kit.Badge{Variant: kit.Secondary, Children: Text("Pro")},
			Children: Div(Class("flex flex-col gap-4"),
				kit.Field{Label: "Email",
					Children: kit.Input{Type: "email", Placeholder: "you@example.com"}},
				Div(Class("flex items-center justify-between text-sm"),
					Text("Weekly digest"),
					kit.Switch{DefaultChecked: true, AriaLabel: "Weekly digest"}),
			),
			Footer: kit.Button{Class: "w-full", Children: Text("Save changes")},
		}),
}

func main() { tela.Main(App) }
