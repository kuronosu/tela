// Package effectstate is a test package of cmd/tela whose screen sets state
// and shows a toast from a passive effect, which runs after the first
// present: tela snapshot and tela inspect must show both once the screen is
// at rest.
package effectstate

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Preview is the screen inside the toaster the toast needs.
func Preview() tela.Node {
	return kit.Toaster{Children: tela.C(screen)}
}

func screen() tela.Node {
	status, setStatus := tela.UseState("loading")
	toast := kit.UseToast()
	tela.UseMount(func() func() {
		setStatus.Set("ready")
		toast.Success("Saved from an effect")
		return nil
	})
	return Div(Class("min-h-screen bg-amber-100 p-4"),
		H1(Class("text-xl"), Textf("Status: %s", status)))
}
