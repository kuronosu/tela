package detect_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks/detect"
)

// An app that follows the platform: the theme goes to the window, the style
// to the tree.
func Example() {
	l := detect.Look()
	app := tela.App{
		Title: "Files",
		Theme: &l.Theme,
		Root: kit.StyleProvider{Style: l.Style,
			Children: Div(Class(l.Regions.Window), Text("Hello"))},
	}
	_ = app // tela.Main(app)
	fmt.Println(l.Name != "")
	// Output: true
}

// For picks the look of a desktop that is not the running one.
func ExampleFor() {
	fmt.Println(detect.For("linux", "KDE").Name)
	fmt.Println(detect.For("linux", "ubuntu:GNOME").Name)
	fmt.Println(detect.For("windows", "").Name)
	// Output:
	// KDE
	// GNOME
	// Windows 11
}
