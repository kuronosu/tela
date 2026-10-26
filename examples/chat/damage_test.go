package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestDamage plays a conversation in a small window at scales 1 and 2 (the
// pointer over the list, switching, scrolling the messages, typing, sending,
// the dots and the reply), checking every frame against a full rendering
// (telatest.CheckDamage): what a frame repaints is all that changed.
func TestDamage(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		a := mount(t, 900, 560, telatest.Scale(scale), telatest.InstantScroll(), telatest.CheckDamage())
		a.Hover(dev)
		a.Click(dev)
		a.Scroll(log, 0, -200)
		a.Scroll(log, 0, 200)
		a.Click(maya)

		a.Fill(composer, "Ping")
		a.Press("Shift+Enter")
		a.Type("pong")
		a.Press("Enter")
		a.Advance(replyDelay + replyDelay/2) // the dots, then the reply

		a.Click(sofia)
		a.Fill(search, "invoice")
		a.Hover(telatest.ByRole("button", "Video call"))
	}
}
