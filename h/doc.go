// Package h holds the elements, props and helpers of a Tela UI. It is
// designed to be dot-imported, so that markup reads like JSX:
//
//	import (
//	    "github.com/kuronosu/tela"
//	    . "github.com/kuronosu/tela/h"
//	)
//
//	func Greeting(name string) tela.Node {
//	    return Div(Class("flex items-center gap-3 rounded-xl bg-white p-4 shadow-md"),
//	        H2(Class("text-lg font-semibold text-slate-900"), Textf("Hello, %s", name)),
//	        If(name == "", P(Class("text-slate-500"), Text("Who are you?"))),
//	    )
//	}
//
// To keep collisions with the importing package unlikely, h exports only
// element constructors (Div, Span, Text, H1 to H6, P, Button, Img, Strong,
// Em, Code, A), props (Class, Style, Key, OnClick and the other mouse
// handlers, Src, Href) and the helpers If, Map and Fragment. Everything
// else (App, Run, hooks, colors, typed style props) lives in package tela.
//
// An element takes its props and children in one list, in any order:
//
//	Div(Class("p-4"), Key(id), Text("hi"), Span(Text("there")))
//
// Props apply in order, so a later Class wins over an earlier one and
// Style wins over every class. Children are nodes only; wrap text in Text
// or Textf. The parameters are tela.Child, so passing a bare string is a
// compile error.
//
// Elements have the defaults of Tailwind's Preflight: no margins, padding
// or borders, headings in the surrounding font size, block images. A Button
// has no background of its own.
package h
