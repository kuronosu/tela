// Package gridpreview is a test package of cmd/tela: a dashboard shell
// laid out with CSS grid.
package gridpreview

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is the screen `tela inspect` lays out.
func Preview() tela.Node {
	cards := make([]tela.Child, 0, 6)
	cards = append(cards, Key("main"), Class("grid grid-cols-[repeat(auto-fill,minmax(80px,1fr))] content-start gap-2"))
	for i := range 5 {
		cards = append(cards, Div(Key(fmt.Sprintf("c%d", i)), Class("h-10 rounded-md bg-sky-500")))
	}
	return Div(Class("grid min-h-screen grid-cols-[[side]_120px_[main]_1fr] grid-rows-[48px_1fr] gap-2 bg-slate-100 p-4"),
		Div(Key("header"), Class("col-span-2 bg-slate-800")),
		Div(Key("side"), Class("col-[side] bg-slate-200")),
		Div(cards...),
	)
}
