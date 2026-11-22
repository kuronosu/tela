// Package checki18n is a test package of cmd/tela with a screen that fits in
// English and not in Spanish, for tela verify's language variant.
package checki18n

import (
	"embed"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
)

//go:embed locales/*.json
var locales embed.FS

func init() {
	if err := i18n.Load(locales, "locales/*.json"); err != nil {
		panic(err)
	}
}

// Preview is the screen: a label in a box 120 px wide that cannot grow.
func Preview() tela.Node { return tela.C(screen) }

func screen() tela.Node {
	t := i18n.UseT()
	return Div(Class("min-h-screen bg-white p-4 text-zinc-900"),
		Div(Class("w-[120px] overflow-hidden whitespace-nowrap"), Text(t("save"))))
}
