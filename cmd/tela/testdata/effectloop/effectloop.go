// Package effectloop is a test package of cmd/tela whose screen never goes
// quiet: an effect without dependencies sets state on every build.
package effectloop

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview counts its own builds, forever.
func Preview() tela.Node {
	n, setN := tela.UseState(0)
	_ = n
	tela.UseEffect(func() func() {
		setN.Update(func(n int) int { return n + 1 })
		return nil
	})
	return Div(Class("min-h-screen bg-white p-4"), H1(Text("Spinning")))
}
