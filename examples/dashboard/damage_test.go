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
		damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App { return telatest.MountApp(t, App, opts...) }, 900, 560, scale, 24)
	}
}

// TestInteractionDamage plays the interactions of the session (a period
// change, a metric tab, the report dialog, the palette, the tasks, the
// board, the theme) with every frame, animations included, checked against
// a full rendering: the glass panes, springs, layout animations and
// overlays repaint only what changed, and what they repaint is what a full
// frame has.
func TestInteractionDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		a := telatest.MountApp(t, App, telatest.Size(1280, 800), telatest.Scale(scale), telatest.CheckDamage())
		a.Click(telatest.ByRole("radio", "90D"))
		a.Click(telatest.ByRole("tab", "Orders"))
		a.Hover(telatest.ByRole("button", "Orders"))
		a.Scroll(telatest.ByRole("heading", "Subscribers"), 0, 400)
		a.Click(telatest.ByRole("button", "Edit the weekly report"))
		a.Click(telatest.ByRole("radio", "XLSX"))
		a.Click(telatest.ByRole("button", "Save"))
		a.Click(telatest.ByRole("button", "Search"))
		a.Type("ta")
		a.Press("ArrowDown")
		a.Press("Enter")
		a.Click(telatest.ByRole("checkbox", "Fix failed invoice retries"))
		a.Click(telatest.ByRole("button", "Due"))
		a.Click(telatest.ByRole("radio", "Board"))
		a.Click(telatest.ByRole("button", "Actions for Ship the CSV export"))
		a.Click(telatest.ByRole("menuitem", "In progress"))
		a.Click(telatest.ByRole("button", "Switch theme"))
		a.Click(telatest.ByRole("button", "Notifications, 3 unread"))
		a.Press("Escape")
		a.Click(telatest.ByRole("button", "Switch theme"))
	}
}
