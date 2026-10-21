// Package seeded is a test package of cmd/tela whose previews start in a
// given state (a struct component's field, a Provider in Wrap) and whose App
// sets the window: size, root class, dark scheme, theme tokens and aliases.
package seeded

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// User is a context the screen reads.
var User = tela.CreateContext("guest")

// Main is the screen: its local state starts at InitialTab.
type Main struct{ InitialTab string }

// Build shows the tab and the user, and three colored boxes.
func (m Main) Build() tela.Node {
	tab, _ := tela.UseState(m.InitialTab)
	user := tela.UseContext(User)
	return Div(Class("flex flex-col gap-2"),
		Div(Key("tab"), Class("text-sm"), Textf("tab=%s user=%s", tab, user)),
		Div(Key("token"), Class("swatch")),
		Div(Key("scheme"), Class("size-8 bg-rose-500 dark:bg-emerald-500")),
	)
}

// Previews start the screen in two states.
var Previews = tela.Previews{
	"1 Open": {Root: Main{InitialTab: "open"}},
	"2 Mocked": {
		Root: Main{InitialTab: "closed"},
		Wrap: func(n tela.Node) tela.Node { return User.Provider("Ana", n) },
	},
}

// App is the window the previews render in.
var App = tela.App{
	Title:       "Seeded",
	Width:       320,
	Height:      140,
	RootClass:   "bg-slate-800 p-4 text-white",
	ColorScheme: tela.Dark,
	Theme: &tela.Theme{Colors: map[string]tela.SchemeColor{
		"brand": {Light: tela.Hex("#ff0000"), Dark: tela.Hex("#0000ff")},
	}},
	ClassAliases: map[string]string{"swatch": "size-8 bg-brand"},
	Root:         Main{InitialTab: "app"},
}
