package looks_test

import (
	"fmt"

	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/breeze"
)

// A look's Regions are classes for the parts of a window that kit has no
// component for; its Style goes to a kit.StyleProvider.
func Example() {
	l := breeze.Look()
	window := kit.StyleProvider{Style: l.Style, Children: Div(Class(l.Regions.Window),
		Div(Class(l.Regions.Toolbar)),
		Div(Class("flex min-h-0 flex-1"),
			Div(Class(l.Regions.Sidebar)),
			Div(Class(l.Regions.Content))),
		Div(Class(l.Regions.StatusBar)),
	)}
	_ = window
	fmt.Println(l.Name)
	// Output: KDE
}

// The recipes of a look define two extra kit.Toggle variants.
func ExampleSidebar() {
	toggle := kit.Toggle{Variant: looks.Sidebar, Size: looks.SidebarSize, Children: Text("Inbox")}
	_ = toggle
	fmt.Println(looks.Sidebar, looks.Toolbar)
	// Output: sidebar toolbar
}
