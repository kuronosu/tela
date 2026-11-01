package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

const card = "rounded-xl border border-stone-200 bg-white p-4 text-sm font-medium " +
	"dark:border-stone-800 dark:bg-stone-900"

var App = tela.App{
	Title: "Grid", Width: 480, Height: 380,
	RootClass: "bg-stone-100 text-stone-900 dark:bg-stone-950 dark:text-stone-50",
	Root: Div(Class("grid h-full grid-cols-3 grid-rows-[1fr_1fr_auto] gap-3 p-4"),
		Div(Class(card, "col-span-2 row-span-2 flex items-end text-lg"),
			Text("col-span-2 row-span-2")),
		Div(Class(card), Text("1")),
		Div(Class(card), Text("2")),
		Div(Class(card, "col-span-3 flex items-center justify-between"),
			Span(Text("flex")), Span(Class("text-stone-500"), Text("justify-between"))),
	),
}

func main() { tela.Main(App) }
