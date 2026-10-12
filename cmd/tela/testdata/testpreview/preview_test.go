package testpreview

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is the screen `tela snapshot` renders; it lives in a test file.
func Preview() tela.Node {
	return Div(Class("h-screen bg-emerald-500"))
}
