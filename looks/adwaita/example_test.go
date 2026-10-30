package adwaita_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks/adwaita"
)

// The look's theme goes to the window and its style to the tree. A look with
// a ToolbarLead draws a header bar over the sidebar.
func ExampleLook() {
	l := adwaita.Look()
	app := tela.App{
		Title: "Files",
		Theme: &l.Theme,
		Root:  kit.StyleProvider{Style: l.Style, Children: Div(Class(l.Regions.Window), Text("Hello"))},
	}
	_ = app // tela.Main(app)
	fmt.Println(l.Name, l.Regions.ToolbarLead != "")
	// Output: GNOME true
}
