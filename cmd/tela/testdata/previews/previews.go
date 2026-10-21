// Package previews is a test package of cmd/tela with several named screens,
// one of them stateful.
package previews

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Previews are the screens of the package.
var Previews = tela.Previews{
	"Counter": {Root: tela.C(counter)},
	"Alert":   {Root: tela.C(alert)},
}

func alert() tela.Node {
	return Div(Class("min-h-screen bg-amber-100 p-4"),
		Div(Key("box"), Class("rounded-xl border border-amber-300 bg-amber-200 p-3 text-amber-900"),
			Text("Careful: this is an alert")))
}

func counter() tela.Node {
	count, setCount := tela.UseState(0)
	return Div(Class("min-h-screen bg-indigo-100 p-4"),
		Button(Key("inc"), Class("rounded-lg bg-indigo-600 px-4 py-2 text-white"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Textf("Count: %d", count)))
}
