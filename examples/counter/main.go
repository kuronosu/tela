// Command counter is the Counter example of ARCHITECTURE section 1: a card
// with a heading and a button, styled only with Tailwind classes. It is the
// phase-3 showcase: `tela snapshot ./examples/counter` renders it without a
// window, and the tests compare that render with Chromium's rendering of the
// same markup styled by Tailwind's own compiled CSS.
//
// The button does not react yet: events, hover and active states and
// transitions arrive in phase 4 (ADR-040), so clicking changes nothing and the
// count stays at 0.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Counter is the example of ARCHITECTURE section 1, verbatim.
func Counter() tela.Node {
	count, setCount := tela.UseState(0)

	return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl "+
		"bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
		H1(Class("text-5xl font-bold text-white tracking-tight"),
			Textf("Count: %d", count)),
		Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur "+
			"hover:bg-white/25 active:scale-95 transition"),
			OnClick(func(tela.MouseEvent) { setCount(count + 1) }),
			Text("Increment"),
		),
	)
}

func main() {
	tela.Run(tela.App{Title: "Demo", Width: 800, Height: 600, Root: Counter})
}

// Preview is the screen of `tela snapshot` and `tela inspect`. The CLI builds
// it as a component, so Counter's UseState works.
func Preview() tela.Node { return Counter() }
