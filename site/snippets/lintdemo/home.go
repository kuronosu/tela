package lintdemo

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func Home() tela.Node {
	return Div(Class("flex flex-col gap-4 p-6"),
		H1(Class("text-2xl font-bold"), Text("Home")),
		Div(Class("rounded-xxl border bg-white p-4"), Text("Hello")),
	)
}
