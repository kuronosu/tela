package main

import "testing"

// TestAriaSnapshot pins what `tela inspect --aria --size 960x640
// ./examples/notes` prints (testdata/notes.aria.yml): the title bar with its
// named buttons and window controls, the rail, the cards of the list, the
// folder menu and the pin, the title and text fields, and the status line.
func TestAriaSnapshot(t *testing.T) {
	sandbox(t)
	mount(t).AriaGolden("notes.aria.yml")
}
