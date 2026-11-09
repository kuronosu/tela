// Package previewtheme is a test package of cmd/tela whose previews give
// a theme and a color scheme of their own over its App's (ADR-250).
package previewtheme

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Main is a swatch of the theme's brand color and a box of the scheme.
type Main struct{}

// Build shows the swatch and the box.
func (Main) Build() tela.Node {
	return Div(Class("flex gap-2"),
		Div(Key("token"), Class("size-8 bg-brand")),
		Div(Key("scheme"), Class("size-8 bg-rose-500 dark:bg-emerald-500")))
}

// brand is the theme of the App: red in light, blue in dark.
var brand = &tela.Theme{Colors: map[string]tela.SchemeColor{
	"brand": {Light: tela.Hex("#ff0000"), Dark: tela.Hex("#0000ff")},
}}

// other is a preview's theme: green in light, yellow in dark.
var other = &tela.Theme{Colors: map[string]tela.SchemeColor{
	"brand": {Light: tela.Hex("#00ff00"), Dark: tela.Hex("#ffff00")},
}}

// Previews: the App's settings, a light scheme over the App's dark one, a
// theme of its own (in the App's dark scheme), and both.
var Previews = tela.Previews{
	"1 App":   {Root: Main{}},
	"2 Light": {Root: Main{}, ColorScheme: tela.Light},
	"3 Theme": {Root: Main{}, Theme: other},
	"4 Both":  {Root: Main{}, Theme: other, ColorScheme: tela.Light},
}

// App is the window the previews render in: dark, with the brand theme.
var App = tela.App{
	Title:       "Preview theme",
	Width:       120,
	Height:      60,
	RootClass:   "bg-white p-2",
	ColorScheme: tela.Dark,
	Theme:       brand,
	Root:        Main{},
}
