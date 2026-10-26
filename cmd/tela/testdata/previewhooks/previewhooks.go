// Package previewhooks is a test package of cmd/tela whose Preview, a
// func() tela.Node, uses hooks: the commands mount it as tela.C(Preview).
package previewhooks

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is a component of its own, with its state.
func Preview() tela.Node {
	n, _ := tela.UseState(7)
	return Div(Class("min-h-screen bg-teal-100 p-4"),
		H1(Class("text-xl"), Textf("Hooks %d", n)))
}
