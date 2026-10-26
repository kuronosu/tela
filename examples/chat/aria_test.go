package main

import "testing"

// TestAriaSnapshot pins what `tela inspect --aria --size 1100x720
// ./examples/chat` prints with the clock of the tests (testdata/chat.aria.yml):
// the conversations as buttons named after their person and their unread
// count, the open one's heading, its messages as articles named after who
// wrote them and when, and the composer.
func TestAriaSnapshot(t *testing.T) {
	mount(t, windowW, windowH).AriaGolden("chat.aria.yml")
}
