// Package ui uses an alias that another package of the module declares.
package ui

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Card uses the alias ctx-card.
func Card() tela.Node { return Div(Class("ctx-card flex")) }
