// Package appstruct is a test package of cmd/tela that defines only an App
// whose Root is a struct component with hooks (ADR-094).
package appstruct

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Screen is the root: a struct component with state seeded by its field.
type Screen struct{ Start int }

func (s Screen) Build() tela.Node {
	n, _ := tela.UseState(s.Start)
	return H1(Key("n"), Class("text-lg"), Textf("struct n=%d", n))
}

// App is what tela.Main would run.
var App = tela.App{
	Width:     160,
	Height:    80,
	RootClass: "bg-teal-300",
	Root:      Screen{Start: 7},
}
