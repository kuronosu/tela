// Package preview is a test package of cmd/tela with a single screen.
package preview

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is the screen `tela snapshot` and `tela inspect` render.
func Preview() tela.Node {
	return Div(Class("flex min-h-screen flex-col gap-2 bg-slate-100 p-4 dark:bg-slate-900"),
		H1(Key("title"), Class("text-2xl font-bold text-slate-900 dark:text-white"), Text("Hello & welcome")),
		Div(Key("row"), Class("flex gap-2"),
			Div(Key("a"), Class("h-10 w-20 rounded-lg bg-sky-500")),
			Div(Key("b"), Class("h-10 w-20 rounded-lg bg-rose-500")),
		),
	)
}
