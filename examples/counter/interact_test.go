package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// TestCounterClicks drives the example the way the window does: a click
// increments, hovering and pressing restyle the button and the pointer
// cursor shows over it.
func TestCounterClicks(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(800, 600), telatest.Dev())
	app.ExpectText("Count: 0")
	inc := telatest.ByRole("button", "Increment")
	x, y, w, h := app.Bounds(inc)
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
