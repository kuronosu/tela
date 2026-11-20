// Package effectanim is a test package of cmd/tela with two animations: a
// box that a passive effect starts to widen (a finite transition, which
// tela snapshot finishes) and a spinner that never ends (shown at its
// current frame, with a note on standard error).
package effectanim

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is the screen.
func Preview() tela.Node {
	wide, setWide := tela.UseState(false)
	tela.UseMount(func() func() {
		setWide.Set(true)
		return nil
	})
	w := "w-8"
	if wide {
		w = "w-64"
	}
	return Div(Class("min-h-screen bg-white p-4"),
		Div(Class("h-8 bg-black transition-all duration-300", w)),
		Div(Class("mt-4 size-6 animate-spin border-2 border-black")))
}
