// Package ptrapp is a test package of cmd/tela whose App is a *tela.App
// returned by a function: the CLI applies it as it applies a tela.App.
package ptrapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func newApp() *tela.App {
	return &tela.App{Title: "Pointer", Width: 180, Height: 90, RootClass: "bg-lime-300"}
}

// App is what main would run.
var App = newApp()

// Preview is the screen.
func Preview() tela.Node { return H1(Key("title"), Text("Pointer app")) }
