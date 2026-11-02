package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// The command palette and the tooltips are built on aria hooks with no kit
// component (from examples/aria, ADR-195): these tests drive them with the
// keyboard and the pointer.

var (
	palette     = telatest.ByRole("dialog", "Command palette")
	paletteBox  = telatest.ByRole("combobox", "Search commands")
	searchButon = telatest.ByRole("button", "Search")
)

// TestPaletteKeyboard opens the palette with Ctrl+K, filters it by typing,
// moves the virtual focus with the arrows while the input keeps the focus,
// and runs a command with Enter.
func TestPaletteKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(telatest.ByRole("radio", "30D")) // the focus is in the page
	a.Press("ControlOrMeta+k")
	a.ExpectCount(palette, 1)
	a.ExpectFocused(paletteBox)
	a.ExpectCount(telatest.ByRole("option", ""), 7)
	// The rest of the window is inert while it is open.
	a.ExpectCount(telatest.ByRole("radio", "30D"), 0)

	a.Type("go")
	a.ExpectCount(telatest.ByRole("option", ""), 3)
	// The first match is highlighted (AutoFocusFirst); an arrow moves on.
	a.Expect(paletteBox, telatest.ActiveDescendant(telatest.ByRole("option", "Go to Overview")))
	a.Press("ArrowDown")
	a.Expect(paletteBox, telatest.ActiveDescendant(telatest.ByRole("option", "Go to Tasks 8 open")))
	a.ExpectFocused(paletteBox)
	a.Press("Enter")
	a.ExpectCount(palette, 0)
	a.ExpectRole("heading", "Tasks")

	// Enter with no arrow pressed runs the first match.
	a.Press("ControlOrMeta+k")
	a.Type("theme")
	a.ExpectCount(telatest.ByRole("option", ""), 1)
	a.Press("Enter")
	a.ExpectCount(palette, 0)
	a.Press("ControlOrMeta+k")
	a.ExpectText("Light") // the theme is dark now: the command switches to light

	// Nothing matches: the list says so. Escape closes, and the focus goes
	// back to where it was.
	a.Type("zzz")
	a.ExpectText("No command matches.")
	a.Press("Escape")
	a.ExpectCount(palette, 0)
}

// TestPalettePointer opens the palette from the search button, closes it
// with a press outside, and runs a command with a click.
func TestPalettePointer(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(searchButon)
	a.ExpectCount(palette, 1)
	a.ClickAt(80, 820) // outside the panel
	a.ExpectCount(palette, 0)
	a.ExpectFocused(searchButon)

	a.Click(searchButon)
	a.Hover(telatest.ByRole("option", "Go to Orders"))
	a.Expect(paletteBox, telatest.ActiveDescendant(telatest.ByRole("option", "Go to Orders")))
	a.Click(telatest.ByRole("option", "Go to Orders"))
	a.ExpectCount(palette, 0)
	a.ExpectRole("heading", "Orders")

	a.Click(searchButon)
	a.Click(telatest.ByRole("option", "New task"))
	a.ExpectRole("dialog", "New task")
	a.Press("Escape")
	a.ExpectRole("heading", "Tasks")
}

// TestPaletteAria pins what a screen reader finds in the open palette.
func TestPaletteAria(t *testing.T) {
	a := mountPreview(t, "4 Command palette")
	a.AriaGolden("palette.aria.yml")
}

// TestTooltipKeyboard: the keyboard focus opens a rail button's tooltip at
// once, it describes the button, and Escape closes it while the focus
// stays.
func TestTooltipKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	a.Focus(telatest.ByRole("button", "Overview"))
	a.Press("Tab")
	a.ExpectFocused(telatest.ByRole("button", "Tasks"))
	a.ExpectCount(telatest.ByRole("tooltip", "Tasks"), 1)
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	a.ExpectFocused(telatest.ByRole("button", "Tasks"))
}

// TestTooltipPointer: a tooltip opens when the pointer has rested on its
// trigger for 400 ms and closes after it leaves. The delays run on the
// frame clock, so the test drives it.
func TestTooltipPointer(t *testing.T) {
	a := mount(t, windowW, windowH, telatest.NoSettle())
	a.Advance(3 * time.Second)
	tip := telatest.ByRole("tooltip", "Switch theme")
	a.Hover(telatest.ByRole("button", "Switch theme"))
	a.Advance(300 * time.Millisecond)
	a.ExpectCount(tip, 0)
	a.Advance(200 * time.Millisecond)
	a.ExpectCount(tip, 1)
	a.MouseMove(700, 880)
	a.Advance(2 * time.Second)
	a.ExpectCount(tip, 0)
}
