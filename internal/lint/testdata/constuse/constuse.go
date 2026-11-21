// Package constuse uses the class constants of constdecl.
package constuse

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/lint/testdata/constdecl"
)

func Use() tela.Node {
	return h.Div(h.Class(constdecl.Card), h.Class(constdecl.Fine))
}
