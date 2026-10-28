package main

import "testing"

// TestAriaSnapshot pins what `tela inspect --aria --size 960x640
// ./examples/notes` prints (testdata/notes.aria.yml): the title bar with its
// named buttons and window controls, the list of notes, the text box and the
// status line.
func TestAriaSnapshot(t *testing.T) {
	sandbox(t)
	mount(t).AriaGolden("notes.aria.yml")
}
