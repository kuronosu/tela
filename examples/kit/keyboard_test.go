package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The tests of this file use the keyboard only, the way someone who cannot
// use a pointer does: Tab, Shift+Tab, the arrows, Home, End, Enter, Space and
// Escape. What they assert is where the focus is and what a screen reader
// reads (the aria snapshot).

// toSidebar presses Tab until the focus is on the selected tab of the
// sidebar: the Style group, the dark switch and then the sidebar.
func toSidebar(t *testing.T, a *telatest.App, selected string) {
	t.Helper()
	a.Press("Tab")
	a.ExpectFocused(telatest.ByRole("radio", "Default"))
	a.Press("Tab")
	a.ExpectFocused(darkSwitch)
	a.Press("Tab")
	a.ExpectFocused(tabFor(selected))
}

// TestSidebarKeyboard walks the sidebar with the arrows: it is one tab stop,
// the arrows move the focus and select (the page changes with them), Home
// and End jump, and the keys wrap at the ends.
func TestSidebarKeyboard(t *testing.T) {
	a := mount(t, Gallery{Page: "AlertDialog"})
	toSidebar(t, a, "Alert Dialog")
	a.Expect(tabFor("Alert Dialog"), telatest.Selected(true))
	a.ExpectCount(heading("Alert Dialog"), 1)

	a.Press("ArrowDown")
	a.ExpectFocused(tabFor("Avatar"))
	a.Expect(tabFor("Avatar"), telatest.Selected(true))
	a.Expect(tabFor("Alert Dialog"), telatest.Selected(false))
	a.ExpectCount(heading("Avatar"), 1)
	a.ExpectCount(heading("Alert Dialog"), 0)
	a.ExpectRole("tabpanel", "Avatar")

	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.ExpectFocused(tabFor("Button"))
	a.ExpectCount(heading("Button"), 1)
	a.ExpectText("A button in six variants and eight sizes")

	a.Press("ArrowUp")
	a.ExpectFocused(tabFor("Badge"))

	a.Press("End")
	a.ExpectFocused(tabFor("Tooltip"))
	a.ExpectCount(heading("Tooltip"), 1)
	a.Press("Home")
	a.ExpectFocused(tabFor("Alert Dialog"))
	a.ExpectCount(heading("Alert Dialog"), 1)

	// The ends wrap.
	a.Press("ArrowUp")
	a.ExpectFocused(tabFor("Tooltip"))
	a.Press("ArrowDown")
	a.ExpectFocused(tabFor("Alert Dialog"))
}

// TestSidebarTabOrder: the sidebar is a single tab stop whichever tab is
// selected, Tab moves from it to the page, and Shift+Tab goes back to the
// selected tab, not to the first one.
func TestSidebarTabOrder(t *testing.T) {
	a := mount(t, Gallery{Page: "Button"})
	toSidebar(t, a, "Button")

	// Into the page: the tab panel is the next stop, then its first button.
	a.Press("Tab")
	a.ExpectFocused(telatest.ByRole("tabpanel", "Button"))
	a.Press("Tab")
	a.ExpectFocused(telatest.ByRole("button", "Button"))
	a.Press("Shift+Tab")
	a.Press("Shift+Tab")
	a.ExpectFocused(tabFor("Button"))
	a.Press("Shift+Tab")
	a.ExpectFocused(darkSwitch)
	a.Press("Shift+Tab")
	a.ExpectFocused(telatest.ByRole("radio", "Default"))
}

// TestHeaderKeyboard: the Style group is a segmented control (the arrows
// move the focus, Enter or Space choose, one is always on) and the switch
// flips with Space.
func TestHeaderKeyboard(t *testing.T) {
	a := mount(t, Gallery{Page: "Button"})
	a.Press("Tab")
	a.Expect(telatest.ByRole("radio", "Default"), telatest.Checked(true))
	a.Press("ArrowRight")
	a.ExpectFocused(rounded)
	a.Expect(rounded, telatest.Checked(false))
	a.Press("Enter")
	a.Expect(rounded, telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "Default"), telatest.Checked(false))
	if got := a.Count(telatest.ByClass("bg-violet-600")); got == 0 {
		t.Error("the Rounded style did not reach the page: no violet button")
	}

	// Pressing the one that is on does not turn it off.
	a.Press("Enter")
	a.Expect(rounded, telatest.Checked(true))

	a.Press("Tab")
	a.ExpectFocused(darkSwitch)
	a.Expect(darkSwitch, telatest.Checked(false))
	a.Press("Space")
	a.Expect(darkSwitch, telatest.Checked(true))
	if px := a.PixelAt(1000, 760); px.R > 40 {
		t.Errorf("the page is not dark after the switch is on: pixel %v", px)
	}
	a.Press("Space")
	a.Expect(darkSwitch, telatest.Checked(false))
	if px := a.PixelAt(1000, 760); px.R < 200 {
		t.Errorf("the page is not light after the switch is off: pixel %v", px)
	}
}

// TestSidebarSnapshot pins what a screen reader reads on the shell: the
// banner with its two controls, the 34 tabs and the selected page.
func TestSidebarSnapshot(t *testing.T) {
	mount(t, Gallery{}).AriaGolden("kit_gallery.aria.yml")
	a := mount(t, Gallery{Page: "DropdownMenu"})
	a.Click(telatest.ByRole("button", "Open"))
	a.AriaGolden("kit_gallery_menu_open.aria.yml")
}

// TestKeyboardOverlay opens a dialog and a menu from the page with Enter and
// leaves with Escape: the focus returns to the button every time, as the
// kit promises of every trigger slot.
func TestKeyboardOverlay(t *testing.T) {
	a := mount(t, Gallery{Page: "Dialog"})
	edit := telatest.ByRole("button", "Edit profile")
	a.Focus(edit)
	a.Press("Enter")
	a.ExpectRole("dialog", "Edit profile")
	a.ExpectFocused(telatest.ByRole("textbox", "Name"))
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("dialog", "Edit profile"), 0)
	a.ExpectFocused(edit)

	b := mount(t, Gallery{Page: "DropdownMenu"})
	open := telatest.ByRole("button", "Open")
	b.Focus(open)
	b.Press("ArrowDown")
	b.ExpectFocused(telatest.ByRole("menuitem", "Profile"))
	b.Press("ArrowDown")
	b.Press("ArrowDown")
	b.Press("ArrowDown")
	b.ExpectFocused(telatest.ByRole("menuitem", "Log out"))
	b.Press("Enter")
	b.ExpectText("Chose: logout")
	b.ExpectFocused(open)
}
