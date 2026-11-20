// Package motionapp is the screen the film end-to-end test of tela mcp
// drives (film_e2e_test.go copies it into a module of its own): a button
// whose click starts a transition, one that starts a spring, and a toast
// that fades out through a Presence.
package motionapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Previews are the screens of the package.
var Previews = tela.Previews{
	"Slide":  {Root: tela.C(slide)},
	"Spring": {Root: tela.C(springy)},
	"Toast":  {Root: tela.C(toasts)},
}

// slide moves and fades a box with a 300 ms transition.
func slide() tela.Node {
	on, setOn := tela.UseState(false)
	box := "translate-x-0 opacity-100"
	if on {
		box = "translate-x-48 opacity-25"
	}
	return Div(Class("flex min-h-screen flex-col gap-4 bg-white p-4"),
		Button(Class("w-24 rounded bg-sky-600 px-3 py-1 text-white"), OnClick(func() { setOn.Set(!on) }), Text("Slide")),
		Div(Class("size-12 rounded bg-rose-500 transition duration-300 ease-linear", box)))
}

// springy moves a box with a slow spring.
func springy() tela.Node {
	on, setOn := tela.UseState(false)
	target := float32(0)
	if on {
		target = 200
	}
	x := tela.UseSpring(target, tela.Stiffness(60), tela.Damping(8))
	return Div(Class("flex min-h-screen flex-col gap-4 bg-white p-4"),
		Button(Class("w-24 rounded bg-sky-600 px-3 py-1 text-white"), OnClick(func() { setOn.Set(!on) }), Text("Bounce")),
		Div(Class("size-12 rounded bg-emerald-500"), Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 }))))
}

// toasts shows a toast that fades in and, when dismissed, out.
func toasts() tela.Node {
	shown, setShown := tela.UseState(false)
	return Div(Class("flex min-h-screen flex-col gap-4 bg-white p-4"),
		Button(Class("w-24 rounded bg-sky-600 px-3 py-1 text-white"), OnClick(func() { setShown.Set(!shown) }), Text("Toggle")),
		Presence(If(shown, Div(Key("toast"), Role("status"),
			Class("w-48 rounded-lg bg-slate-900 px-4 py-3 text-white transition duration-300 ease-linear starting:opacity-0"),
			Exit("opacity-0"), Text("Saved")))))
}
