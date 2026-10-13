package main

import (
	"testing"

	"github.com/kuronosu/tela/examples/internal/exampletest"
	"github.com/kuronosu/tela/internal/platform"
)

// TestCounterClicks drives the example the way the window does: a click
// increments, hovering and pressing restyle the button and the pointer
// cursor shows over it.
func TestCounterClicks(t *testing.T) {
	a := exampletest.Open(t, Counter, 800, 600)
	if !a.HasText("Count: 0") {
		t.Fatal("the counter does not start at 0")
	}
	c := a.Button("Increment").Rect.Center()
	probe := func() (r, g, b uint8) {
		px := a.Image().RGBAAt(int(c.X)-50, int(c.Y))
		return px.R, px.G, px.B
	}
	r0, g0, b0 := probe()

	a.Move(c.X, c.Y)
	if r, g, b := probe(); r == r0 && g == g0 && b == b0 {
		t.Error("hover:bg-white/25 did not change the button")
	}
	if got := a.Window.Cursor(); got != platform.CursorPointer {
		t.Errorf("cursor over the button is %v, want pointer", got)
	}
	a.Down()
	a.Up()
	a.Click(c.X, c.Y)
	a.Click(c.X, c.Y)
	if !a.HasText("Count: 3") {
		t.Errorf("after three clicks the heading is not Count: 3")
	}
	a.CheckLogs()
}
