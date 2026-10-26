package dirty

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func View(open bool) tela.Node {
	if open {
		tela.UseState(0)
	}
	return Div(Class("flex rounded-xxl"), Role("buton"), Div(Key("a")), Div(Key("a")))
}

func Ignored() tela.Node {
	return Div(Class("rounded-xxl")) //tela:ignore class
}
