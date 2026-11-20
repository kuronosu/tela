// Package mountfail has a preview that cannot be mounted, so that open
// falls back to the previous screen (mcp_session_test.go).
package mountfail

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Previews are the screens of the package.
var Previews = tela.Previews{
	"Good": {Root: tela.C(good)},
	"Bad":  {Root: tela.C(bad)},
}

func good() tela.Node {
	count, setCount := tela.UseState(0)
	return Div(Class("p-6"),
		Button(OnClick(func() { setCount.Set(count + 1) }), Textf("Count: %d", count)))
}

func bad() tela.Node {
	panic("this screen cannot be mounted")
}
