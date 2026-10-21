package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Scheme gives an element and everything inside it a color scheme of its
// own, tela.Light or tela.Dark, whatever the app's scheme: their dark:
// classes and theme colors (bg-card, text-foreground; see tela.Theme)
// follow it, and so do tela.UseColorScheme and tela.UseTheme in the
// components below. A nested Scheme wins over an outer one. tela.System
// removes the override (the element follows its parent).
//
//	// A dark sidebar in a light app.
//	Div(Scheme(tela.Dark), Class("w-64 bg-background text-foreground border-r border-border"),
//	    Sidebar(),
//	)
//
// Content in an h.Portal follows the scheme of the Portal's position. It
// panics for other values.
func Scheme(s tela.ColorScheme) tela.Prop { return core.SchemeProp(s) }
