package main

import (
	"testing"

	"github.com/kuronosu/tela/internal/testutil/damagetest"
	"github.com/kuronosu/tela/telatest"
)

// TestScrollDamage scrolls and hovers the dashboard in a small window at
// scales 1, 1.5 and 2, checking every frame against a full rendering.
func TestScrollDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App { return telatest.MountApp(t, App, opts...) }, 700, 420, scale, 24)
	}
}

// TestInteractionDamage plays the interactions of the session (a period
// change, a reorder, the dialog, the toast, the expanding card, the theme)
// with every frame, animations included, checked against a full rendering:
// glass, springs, layout animations and blend layers repaint only what
// changed, and what they repaint is what a full frame has.
func TestInteractionDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		a := telatest.MountApp(t, App, telatest.Size(1100, 760), telatest.Scale(scale), telatest.CheckDamage())
		a.Click(telatest.ByRole("radio", "90D"))
		a.Click(telatest.ByRole("tab", "Orders"))
		a.Click(telatest.ByRole("radio", "Due"))
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
