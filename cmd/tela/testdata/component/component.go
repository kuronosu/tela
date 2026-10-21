// Package component is a test package of cmd/tela whose Preview returns a
// tela.Component (a struct component) instead of a tela.Node.
package component

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Card is a struct component with its own state.
type Card struct{ Title string }

// Build draws the card.
func (c Card) Build() tela.Node {
	n, _ := tela.UseState(3)
	return Div(Class("min-h-screen bg-teal-100 p-4"),
		H1(Key("title"), Class("text-xl"), Textf("%s %d", c.Title, n)))
}

// Preview is mounted as returned: a struct component needs no tela.C.
func Preview() tela.Component { return Card{Title: "Card"} }
