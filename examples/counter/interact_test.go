package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestCounterClicks drives the example the way the window does: a click
// increments, hovering and pressing restyle the button and the pointer
// cursor shows over it.
func TestCounterClicks(t *testing.T) {
	app := telatest.MountFunc(t, Counter, telatest.Size(800, 600))
	app.ExpectText("Count: 0")
	inc := telatest.ByRole("button", "Increment")
	xBox := app.Bounds(inc)
	x, y, w, h := xBox.X, xBox.Y, xBox.Width, xBox.Height
	probe := func() (r, g, b uint8) {
		px := app.Screenshot().RGBAAt(int(x+w/2)-50, int(y+h/2))
		return px.R, px.G, px.B
	}
	r0, g0, b0 := probe()

	app.Hover(inc)
	if r, g, b := probe(); r == r0 && g == g0 && b == b0 {
		t.Error("hover:bg-white/25 did not change the button")
	}
	if got := app.Cursor(); got != "pointer" {
		t.Errorf("cursor over the button is %v, want pointer", got)
	}
	app.MouseDown()
	app.MouseUp()
	app.Click(inc)
	app.Click(inc)
	app.ExpectText("Count: 3")
}
