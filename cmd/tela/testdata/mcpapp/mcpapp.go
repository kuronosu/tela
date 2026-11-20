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
	"Form":    {Root: tela.C(form)},
	"Other":   {Root: tela.C(other)},
	"List":    {Root: tela.C(list)},
	"Long":    {Root: tela.C(long)},
	"Spinner": {Root: tela.C(busy)},
}

func form() tela.Node {
	name, setName := tela.UseState("")
	greeting, setGreeting := tela.UseState("")
	count, setCount := tela.UseState(0)
	doubled, setDoubled := tela.UseState(false)
	return Div(Class("flex min-h-screen flex-col gap-3 bg-white p-6"),
		H1(Class("text-2xl font-bold"), OnDoubleClick(func() { setDoubled.Set(true) }), Text(title)),
		If(doubled, P(Text("The title was double-clicked"))),
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

// list has buttons with the same name, each removing its own row: a ref
// must stay with its row when the ones before it go.
func list() tela.Node {
	items, setItems := tela.UseState([]int{1, 2, 3})
	return Div(Class("flex flex-col gap-2 p-6"), Map(items, func(i int) tela.Node {
		return Div(Key(i), Class("flex gap-2"), Span(Textf("Item %d", i)),
			Button(Text("Remove"), OnClick(func() {
				var rest []int
				for _, j := range items {
					if j != i {
						rest = append(rest, j)
					}
				}
				setItems.Set(rest)
			})))
	}))
}

// long is a page that scrolls as a whole: a wheel at the center of the
// window moves it, and "Scrolled" says it did.
func long() tela.Node {
	scrolled, setScrolled := tela.UseState(false)
	return Div(Class("h-screen overflow-y-auto"), OnScroll(func(tela.ScrollEvent) { setScrolled.Set(true) }),
		If(scrolled, P(Class("fixed right-2 top-2"), Text("Scrolled"))),
		Map(rows, func(i int) tela.Node {
			return Div(Key(i), Class("h-10 border-b px-4"), Textf("Row %d", i))
		}))
}

var rows = func() []int {
	r := make([]int, 40)
	for i := range r {
		r[i] = i
	}
	return r
}()

// busy never goes idle: a spinner asks for a frame at every vsync, so a
// long wait takes real time.
func busy() tela.Node {
	ticks, setTicks := tela.UseState(0)
	return Div(Class("flex flex-col gap-3 p-6"),
		Div(Class("size-8 animate-spin rounded-full border-4 border-stone-300 border-t-stone-800")),
		Button(OnClick(func() { setTicks.Set(ticks + 1) }), Textf("Ticks: %d", ticks)))
}
