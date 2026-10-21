// Command hello is the smallest Tela program that looks like something: a
// static screen with a headline, a tagline and a rule, styled only with
// Tailwind classes. It is the README's hero image: `tela snapshot` renders it
// without a window.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Hello is the whole screen.
func Hello() tela.Node {
	return Div(Class("flex flex-col justify-center h-full p-12 bg-stone-200 text-stone-800"),
		Span(Class("text-sm font-medium tracking-widest text-stone-500"),
			Text("TELA / GO UI FRAMEWORK")),
		Span(Class("mt-4 text-8xl font-bold tracking-tight"),
			Text("Hello, Tela.")),
		Span(Class("mt-2 text-4xl text-stone-600"),
			Text("Interfaces, woven with Go.")),
		Div(Class("my-8 h-px bg-stone-300")),
		Span(Class("text-sm text-stone-500"),
			Text("No cgo. No webview. Every pixel drawn by Go.")),
	)
}

func main() {
	tela.Main(tela.App{Title: "Tela", Width: 800, Height: 450, Root: Hello})
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Hello() }
