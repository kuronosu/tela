// Command counter is the Counter example of ARCHITECTURE section 1: a card
// with a heading and a button, styled only with Tailwind classes. It is the
// phase-3 showcase: `tela snapshot ./examples/counter` renders it without a
// window, and the tests compare that render with Chromium's rendering of the
// same markup styled by Tailwind's own compiled CSS.
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
// is a func(), and the state updates from the previous one.
func Counter() tela.Node {
	count, setCount := tela.UseState(0)

	return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl",
		"bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
		H1(Class("text-5xl font-bold text-white tracking-tight"),
			Textf("Count: %d", count)),
		Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur",
			"hover:bg-white/25 active:scale-95 transition"),
			OnClick(func() { setCount.Update(func(n int) int { return n + 1 }) }),
			Text("Increment"),
		),
	)
}

func main() {
	tela.Main(tela.App{Title: "Demo", Width: 800, Height: 600, Root: Counter})
}

// Preview is the screen of `tela snapshot` and `tela inspect`. Counter has
// state, so it gets a component boundary of its own (calling Counter()
// here would run its UseState as a hook of Preview, which dev mode
// rejects).
func Preview() tela.Node { return tela.C(Counter) }
