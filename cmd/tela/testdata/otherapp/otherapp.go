// Package otherapp is a test package of cmd/tela with a Preview and a
// variable App of another type than tela.App (its server, say), which the
// CLI must leave alone.
package otherapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// server is not a tela.App.
type server struct{ Addr string }

func newServer() *server { return &server{Addr: ":8080"} }

// App is the package's server, built by a call the CLI cannot classify
// from the source.
var App = newServer()

// Preview is the screen.
func Preview() tela.Node {
	return Div(Class("min-h-screen bg-sky-100"), H1(Key("title"), Text("Other app "+App.Addr)))
}
