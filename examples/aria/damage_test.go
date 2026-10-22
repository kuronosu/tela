package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestDamage plays every overlay in and out, with the keyboard and the
// pointer, checking each frame against a full rendering
// (telatest.CheckDamage): the entrances and exits (Presence, starting:,
// scale and opacity transitions), the floating elements that are placed in
// the frame that shows them, the focus rings and the scrolled list of the
// combobox must paint the same pixels incrementally as from scratch.
func TestDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		a := mount(t, Page{}, telatest.Scale(scale), telatest.CheckDamage())

		// The menus.
		tab(a, 4)
		a.Press("Enter")
		a.Press("ArrowDown")
		a.Press("End")
		a.Press("Escape")
		a.Press("Shift+Tab")
		a.Press("Space") // opens the View menu on its first item
		a.Press("Space") // unchecks Branch: the column leaves the list
		a.Press("Escape")

		// The combobox, with a list that scrolls.
		a.Click(projectToggle)
		a.Hover(telatest.ByRole("option", "edge-router Service"))
		a.Scroll(telatest.ByRole("listbox", "Project"), 0, 120)
		a.Press("End")
		a.Press("Escape")
		a.Click(projectBox)
		a.Press("Control+a")
		a.Type("an")
		a.Press("ArrowDown")
		a.Press("Escape")

		// The dialog, its select, and the tooltip.
		a.Click(newDeployment)
		a.Type("v1")
		a.Press("Tab")
		a.Press("Enter")
		a.Press("ArrowDown")
		a.Press("Enter")
		a.Press("Escape")
		a.Hover(bell)
		a.Press("Tab")
	}
}
