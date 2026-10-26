// Package props is a fixture of the role, aria and prop rules.
package props

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

const sortOrder = "ascendin"

func Roles(dynamic string) []tela.Node {
	return []tela.Node{
		Div(Role("button")),
		Div(Role("buton")), // want role `Role("buton"): unknown ARIA role "buton"; did you mean "button"?`
		Div(Role("switch dialog")),
		Div(Role("switch dialg")), // want role `unknown ARIA role "dialg"`
		Div(Role("zzzzzzzzzzzz")), // want role `see the WAI-ARIA 1.2 roles`
		Div(Role(dynamic)),
	}
}

func Aria() []tela.Node {
	return []tela.Node{
		Div(AriaSort("ascending"), AriaSort("None")),
		Div(AriaSort("up")),      // want aria `AriaSort("up"): unknown value`
		Div(AriaSort(sortOrder)), // want aria `AriaSort("ascendin"): unknown value; did you mean "ascending"?`
		Div(AriaCurrent("page"), AriaCurrent(""), AriaCurrent("false")),
		Div(AriaCurrent("pag")), // want aria `AriaCurrent("pag"): unknown value; did you mean "page"?`
		Div(AriaHasPopup("menu")),
		Div(AriaHasPopup("popup")), // want aria `AriaHasPopup("popup")`
		Div(AriaOrientation("vertical")),
		Div(AriaOrientation("up")), // want aria `AriaOrientation("up")`
		Div(AriaAutoComplete("both")),
		Div(AriaAutoComplete("always")), // want aria `AriaAutoComplete("always")`
	}
}

func Props() []tela.Node {
	return []tela.Node{
		Div(Dir("rtl"), Dir("AUTO"), Dir("")),
		Div(Dir("sideways")), // want prop `Dir("sideways")`
		Input(Type("email")),
		Input(Type("submit")), // want prop `Type("submit") on <input>`
		Button(Type("submit")),
		Button(Type("email")), // want prop `Type("email") on <button>`
		Div(Type("hidden")),   // want prop `Type("hidden")`
		Div(Type("tel")),
		Div(kit.Button{Type: "submit"}),
		Div(kit.Button{Type: "sumbit"}), // want prop `Type("sumbit") on <button>: unknown type; did you mean "submit"?`
		Div(kit.Input{Type: "password"}),
		Div(kit.Input{Type: "reset"}), // want prop `Type("reset") on <input>`
		Div(kit.Input{}),
	}
}

func Ease() []tela.AnimationOption {
	return []tela.AnimationOption{
		tela.Ease(tela.EaseOut),
		tela.Ease("cubic-bezier(0, 0, 0.2, 1)"),
		tela.Ease("ease-out-cubic"), // want prop `Ease("ease-out-cubic")`
		tela.Ease(tela.CubicBezier(0, 0, 0.2, 1)),
	}
}
