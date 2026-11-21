// Package ui uses an alias that another package of the module declares.
package ui

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

func Button() tela.Node {
	return h.Button(h.Class("ctx-btn flex"))
}
