// Package lspapp is the package the tests of tela lsp lint: they open
// buffers in it, as an editor would, and the aliases below are the app's.
package lspapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// App declares the class aliases of the screen.
var App = tela.App{
	ClassAliases: map[string]string{
		"btn":     "px-4 py-2 rounded-md bg-primary text-primary-foreground",
		"btn-big": "btn text-lg",
	},
	Root: tela.C(Screen),
}

// Screen is a screen that uses an alias and plain classes.
func Screen() tela.Node {
	return Div(Class("flex items-center gap-2 p-4"), Button(Class("btn hover:btn-big"), Text("Go")))
}
