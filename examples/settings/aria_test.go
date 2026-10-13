package main

import "testing"

// TestAriaSnapshot pins what `tela inspect --aria --size 1280x800
// ./examples/settings` prints (testdata/settings.aria.yml): headings,
// named switches with their states, radio groups and the footer buttons.
func TestAriaSnapshot(t *testing.T) {
	mount(t, windowW, windowH).AriaGolden("settings.aria.yml")
}
