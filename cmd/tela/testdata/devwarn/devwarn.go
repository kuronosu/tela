// Package devwarn is a test package of cmd/tela whose screen makes dev mode
// report things: an unknown class (an error) and a list without keys (a
// warning). The CLI must print both on standard error.
package devwarn

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is a screen with mistakes.
func Preview() tela.Node {
	return tela.C(func() tela.Node {
		return Div(Class("bg-slate-100 p-4 flex-colum"),
			Map([]string{"a", "b"}, func(s string) tela.Node { return P(Text(s)) }))
	})
}
