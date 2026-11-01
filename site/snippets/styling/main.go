package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

var App = tela.App{
	Title: "Card", Width: 480, Height: 380,
	RootClass: "bg-stone-100 text-stone-900 dark:bg-stone-950 dark:text-stone-50",
	Root: Div(Class("flex h-full items-center justify-center"),
		Div(Class("w-80 rounded-2xl border border-stone-200 bg-white p-6 shadow-xl",
			"dark:border-stone-800 dark:bg-stone-900"),
			Span(Class("rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-medium text-amber-800",
				"dark:bg-amber-400/15 dark:text-amber-300"), Text("New")),
			H2(Class("mt-4 text-2xl font-bold tracking-tight"), Text("Hot reload")),
			P(Class("mt-1 text-sm leading-relaxed text-stone-500"),
				Text("Save a file and the window keeps its state.")),
			Button(Class("mt-6 w-full rounded-xl bg-stone-900 py-2.5 font-medium text-stone-50",
				"transition hover:bg-stone-700 active:scale-95",
				"dark:bg-stone-50 dark:text-stone-900 dark:hover:bg-stone-200"),
				Text("Try it")),
		)),
}

func main() { tela.Main(App) }
