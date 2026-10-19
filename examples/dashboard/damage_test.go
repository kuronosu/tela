package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// TestScrollDamage scrolls and hovers the dashboard in a small window at
// scales 1, 1.5 and 2, checking every frame against a full rendering.
func TestScrollDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		scrollAround(t, Dashboard, 700, 420, scale, 24)
	}
}

// TestInteractionDamage plays the interactions of the session (a period
// change, a reorder, the dialog, the toast, the expanding card, the theme)
// with every frame, animations included, checked against a full rendering:
// glass, springs, layout animations and blend layers repaint only what
// changed, and what they repaint is what a full frame has.
func TestInteractionDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		a := telatest.Mount(t, tela.C(Dashboard), telatest.Size(1100, 760), telatest.Scale(scale), telatest.CheckDamage(), telatest.Dev())
		a.Click(telatest.ByRole("tab", "90D"))
		a.Click(telatest.ByRole("tab", "Orders"))
		a.Click(telatest.ByRole("button", "Due"))
		a.Click(telatest.ByRole("checkbox", "Fix the failed invoice retries"))
		a.Scroll(telatest.ByRole("heading", "Priorities"), 0, -2000) // back to the top
		a.Click(telatest.ByRole("button", "New report"))
		a.Click(telatest.ByRole("radio", "XLSX"))
		a.Click(telatest.ByRole("button", "Create report"))
		a.Click(telatest.ByRole("button", "Show details"))
		a.Click(telatest.ByRole("switch", "Dark mode"))
		a.Click(telatest.ByRole("button", "Hide details"))
		a.Click(telatest.ByRole("switch", "Dark mode"))
	}
}

// scrollAround opens root and checks every frame against a full rendering
// (telatest.CheckDamage) while the pointer moves over the screen and the
// arrow and page keys scroll it down and back up (instantly, so that each
// key is one frame).
func scrollAround(t *testing.T, root func() tela.Node, w, h float32, scale float64, steps int) {
	t.Helper()
	a := telatest.Mount(t, tela.C(root), telatest.Size(w, h), telatest.Scale(scale), telatest.InstantScroll(), telatest.CheckDamage())
	first := string(a.Screenshot().Pix)
	for i := range steps {
		if i == steps/2 && string(a.Screenshot().Pix) == first {
			t.Errorf("the page did not scroll")
		}
		f := float32(i) / float32(steps)
		a.MouseMove(w*(0.2+0.6*f), h*(0.3+0.2*float32(i%3)))
		down := i < steps/2
		switch {
		case i%6 == 5 && down:
			a.Press("PageDown")
		case i%6 == 5:
			a.Press("PageUp")
		case down:
			a.Press("ArrowDown")
		default:
			a.Press("ArrowUp")
		}
	}
}
