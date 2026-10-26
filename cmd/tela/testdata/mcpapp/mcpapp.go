// Package mcpapp is the screen the end-to-end test of tela mcp drives
// (mcp_e2e_test.go copies it into a module of its own and edits it).
package mcpapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// title is what the test edits to see a rebuild.
const title = "Version 1"

// Previews are the screens of the package.
var Previews = tela.Previews{
	"Form":  {Root: tela.C(form)},
	"Other": {Root: tela.C(other)},
}

func form() tela.Node {
	name, setName := tela.UseState("")
	greeting, setGreeting := tela.UseState("")
	count, setCount := tela.UseState(0)
	return Div(Class("flex min-h-screen flex-col gap-3 bg-white p-6"),
		H1(Class("text-2xl font-bold"), Text(title)),
		Form(Class("flex gap-2"),
			OnSubmit(func() { setGreeting.Set("Hello, " + name + "!") }),
			Input(AriaLabel("Name"), Class("rounded border px-2 py-1"), Value(name),
				OnChange(func(e tela.ChangeEvent) { setName.Set(e.Value) })),
		),
		If(greeting != "", P(Text(greeting))),
		Button(Class("rounded bg-sky-500 px-3 py-1 text-white"),
			OnClick(func() { setCount.Set(count + 1) }), Textf("Count: %d", count)),
		Button(Disabled(true), Text("Locked")),
		Button(OnClick(func() { panic("boom: the crash button") }), Text("Crash")),
	)
}

func other() tela.Node {
	return Div(Class("min-h-screen bg-amber-100 p-6"), H2(Text("Other screen")))
}
