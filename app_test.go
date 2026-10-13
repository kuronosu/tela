package tela

import "testing"

// TestAppOptions checks that every App field reaches the core options.
func TestAppOptions(t *testing.T) {
	app := App{Title: "T", AppID: "a.b", Width: 3, Height: 4, RootClass: "bg-white",
		ClassicScrollbars: true, InstantScroll: true, Root: func() Node { return Node{} }}
	o := app.options()
	if o.Title != "T" || o.AppID != "a.b" || o.Width != 3 || o.Height != 4 || o.RootClass != "bg-white" ||
		!o.ClassicScrollbars || !o.InstantScroll || o.Root == (Node{}) {
		t.Errorf("options %+v", o)
	}
}
