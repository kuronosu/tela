// Command counter is the Counter example of ARCHITECTURE section 1: a card
// with a label, a number and a button on a stone background, styled only with
// Tailwind classes. It is the phase-3 showcase: `tela snapshot
// ./examples/counter` renders it without a window, and the tests compare that
// render with Chromium's rendering of the same markup styled by Tailwind's own
// compiled CSS.
//
// The button reacts: a click increments the count, and the hover: and active:
// classes restyle it under the pointer, and the transition class animates the
// change on the frame clock.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Counter is the example of ARCHITECTURE section 1 and the README, in the
// phase-7 API: Class takes several strings, a handler that ignores its event
// is a func(), and the state updates from the previous one. Counter is the
// public name, with its component boundary; counter has the hooks.
func Counter() tela.Node { return tela.C(counter) }

func counter() tela.Node {
	count, setCount := tela.UseState(0)

	return Div(Class("flex items-center justify-center h-full bg-stone-200"),
		Div(Class("flex flex-col items-center gap-6 w-80 p-10 rounded-2xl",
			"bg-stone-50 border border-stone-300 shadow-xl text-stone-800"),
			Span(Class("text-sm font-medium tracking-widest text-stone-500"), Text("COUNT")),
			H1(Class("text-8xl font-bold tracking-tight tabular-nums"), Textf("%d", count)),
			Button(Class("w-full py-3 rounded-xl bg-stone-800 text-stone-100 font-medium",
				"hover:bg-stone-700 active:scale-95 transition"),
				OnClick(func() { setCount.Update(func(n int) int { return n + 1 }) }),
				Text("Increment"),
			),
		),
	)
}

func main() {
	tela.Main(tela.App{Title: "Demo", Width: 800, Height: 600, Root: Counter()})
}

// Preview is the screen of `tela snapshot` and `tela inspect`. Counter() is
// the component boundary around counter, so its UseState is not a hook of
// Preview.
func Preview() tela.Node { return Counter() }
