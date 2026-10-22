package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The roles and states the hooks set print as Playwright 1.61 prints them
// (checked against Chromium for the same trees).
func TestListBoxMultipleAriaSnapshot(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: fruits[:3], SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("apple", "banana"), AriaLabel: "Fruit",
		})
		return Div(lb.Props, Range(3, func(i int) tela.Node {
			return Div(lb.ItemProps(fruits[i]), Key(fruits[i]), Class("h-8"), Text(capital(fruits[i])))
		}))
	}, telatest.Size(300, 200))
	app.ExpectAria(`
		- listbox "Fruit":
		  - option "Apple" [selected]
		  - option "Banana" [selected]
		  - option "Blueberry"
	`)
}
