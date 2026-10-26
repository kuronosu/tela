// Package aliases declares class aliases and theme colors, which the class
// rule must know in every linted package.
package aliases

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

var aliases = map[string]string{
	"text-muted": "text-sm text-muted-foreground",
	"card-title": "text-lg font-semibold dark:text-white",
	"nested":     "card-title tracking-tight",
	"broken":     "text-smm underline",                // want class `unknown class "text-smm"`
	"loop-a":     "loop-b",                            // want class `form a cycle`
	"loop-b":     "loop-a",                            // want class `form a cycle`
	"bad name":   "underline",                         // want class `class aliases "bad name"`
	"swatch":     "size-8 bg-brand border-brand-dark", // brand colors come from the theme below
}

func theme() tela.Theme {
	t := tela.DefaultTheme()
	t.Colors["brand"] = tela.SchemeColor{}
	t.Colors["Bad Name"] = tela.SchemeColor{} // want class `"Bad Name" is not a valid token name`
	t.Colors["red-500"] = tela.SchemeColor{}  // want class `"red-500" is already a palette color`
	return t
}

func extraTheme() tela.Theme {
	return tela.Theme{Colors: map[string]tela.SchemeColor{"brand-dark": {}}}
}

func App() tela.App {
	th := theme()
	return tela.App{Title: "x", Theme: &th, ClassAliases: aliases}
}

func View() tela.Node {
	return h.Div(h.Class("text-muted card-title nested hover:card-title"),
		h.Div(h.Class("swatch bg-brand/50 text-brand-dark")),
		h.Div(h.Class("text-mutedd")),      // want class `unknown class "text-mutedd"; did you mean "text-muted"?`
		h.Div(h.Class("bg-brandd")),        // want class `unknown class "bg-brandd"`
		h.Div(h.Class("hover:card-titel")), // want class `unknown class "hover:card-titel"`
	)
}
