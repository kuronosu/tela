package renamed

import (
	tl "github.com/kuronosu/tela"
	th "github.com/kuronosu/tela/h"
	k "github.com/kuronosu/tela/kit"
)

func View() tl.Node {
	return th.Div(th.Class("flex gap-2"),
		th.Div(th.Class("flex flexx")),    // want class `unknown class "flexx"`
		th.Div(th.AriaSort("up")),         // want aria `unknown value`
		th.Div(k.Button{Class: "rouded"}), // want class `unknown class "rouded"`
	)
}
