package clean

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func Counter() tela.Node {
	n, setN := tela.UseState(0)
	return Div(Class("flex items-center gap-2"),
		Button(Role("button"), OnClick(func() { setN.Set(n + 1) }), Textf("%d", n)))
}
