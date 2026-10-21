// Package apponly is a test package of cmd/tela that defines only an App:
// its Root is the screen, and its settings are the window.
package apponly

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// App is what tela.Main would run.
var App = tela.App{
	Title:       "App only",
	Width:       200,
	Height:      100,
	RootClass:   "bg-amber-300 dark:bg-amber-900",
	ColorScheme: tela.Light,
	Root: func() tela.Node {
		n, _ := tela.UseState(5)
		return H1(Key("n"), Class("text-lg"), Textf("n=%d", n))
	},
}
