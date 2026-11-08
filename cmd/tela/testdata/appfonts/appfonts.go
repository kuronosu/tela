// Package appfonts is a test package of cmd/tela whose App brings a font:
// font-sans must draw it (a Sans font) while font-['Inter'] still draws
// Inter, so the two lines differ in width only if App.Fonts reached the
// render of snapshot, inspect and mcp.
package appfonts

import (
	"os"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// App is what tela.Main would run. The font is a test font of
// internal/text (tests run in the package directory); a real app embeds
// its files with //go:embed.
var App = tela.App{
	Title:  "App fonts",
	Width:  300,
	Height: 100,
	Fonts: []tela.Font{
		tela.FontFS(os.DirFS("../../../../internal/text/testdata/fonts"), "Test Face", "NotoSansJP-Subset.otf").Sans(),
	},
	Root: tela.C(func() tela.Node {
		return Div(Class("p-2 text-lg"),
			Div(Key("sans"), Class("w-fit font-sans"), Text("Hamburgefonstiv")),
			Div(Key("inter"), Class("w-fit font-['Inter']"), Text("Hamburgefonstiv")),
		)
	}),
}
