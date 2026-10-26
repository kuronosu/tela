package dotimport

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func View() tela.Node {
	return Div(Class("flex gap-2"),
		Div(Class("flex flexx")), // want class `unknown class "flexx"`
		Div(Role("buton")),       // want role `unknown ARIA role "buton"; did you mean "button"?`
	)
}
