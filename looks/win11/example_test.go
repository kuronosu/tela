package win11_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks/win11"
)

// The look's theme goes to the window and its style to the tree.
func ExampleLook() {
	l := win11.Look()
	app := tela.App{
		Title: "Files",
		Theme: &l.Theme,
		Root:  kit.StyleProvider{Style: l.Style, Children: Div(Class(l.Regions.Window), Text("Hello"))},
	}
	_ = app // tela.Main(app)
	fmt.Println(l.Name)
	// Output: Windows 11
}
