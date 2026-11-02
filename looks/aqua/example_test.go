package aqua_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks/aqua"
)

// The look of macOS 26 (Tahoe) is the default; the theme goes to the window,
// the style to the tree.
func ExampleLook() {
	l := aqua.Look()
	app := tela.App{
		Title: "Files",
		Theme: &l.Theme,
		Root:  kit.StyleProvider{Style: l.Style, Children: Div(Class(l.Regions.Window), Text("Hello"))},
	}
	_ = app // tela.Main(app)
	fmt.Println(l.Name)
	// Output: macOS Tahoe
}

// The designs are options that set every part, and the options apply in
// order: Sequoia's controls and tokens with Tahoe's glass toolbar and sidebar.
func ExampleChrome() {
	fmt.Println(aqua.Look(aqua.Sequoia).Name)
	fmt.Println(aqua.Look(aqua.Sequoia, aqua.Chrome(aqua.Tahoe)).Name)
	// A design given last sets every part again.
	fmt.Println(aqua.Look(aqua.Chrome(aqua.Tahoe), aqua.Sequoia).Name)
	// Output:
	// macOS Sequoia
	// macOS Sequoia (mixed: Tahoe chrome)
	// macOS Sequoia
}

// Tahoe keeps its content opaque; GlassContent makes the cards glass.
func ExampleGlassContent() {
	fmt.Println(aqua.Look(aqua.Tahoe, aqua.GlassContent()).Name)
	// Output: macOS Tahoe (mixed: glass content)
}

// Controls changes the buttons, fields, toggles, checkboxes, radios, switches,
// selects, sliders and segmented controls only.
func ExampleControls() {
	l := aqua.Look(aqua.Tahoe, aqua.Controls(aqua.Sequoia))
	fmt.Println(l.Name)
	// Output: macOS Tahoe (mixed: Sequoia controls)
}
