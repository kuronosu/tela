package breeze_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks/breeze"
)

// Edit the style before giving it to the provider to adapt the look.
func ExampleLook() {
	l := breeze.Look()
	l.Style.Button.Root += " uppercase"
	app := tela.App{
		Title: "Files",
		Theme: &l.Theme,
		Root:  kit.StyleProvider{Style: l.Style, Children: Div(Class(l.Regions.Window), Text("Hello"))},
	}
	_ = app // tela.Main(app)
	fmt.Println(l.Name)
	// Output: KDE
}
