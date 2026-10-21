// Package devoff is a test package of cmd/tela: the screen of devwarn in an
// App that turns dev mode off, so nothing is reported.
package devoff

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/cmd/tela/testdata/devwarn"
)

var off = false

// App turns dev mode off.
var App = tela.App{Dev: &off}

// Preview is the screen of devwarn.
func Preview() tela.Node { return devwarn.Preview() }
