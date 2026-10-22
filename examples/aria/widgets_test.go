package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// TestViewMenuKeyboard is a menu of checkbox items: Space toggles one and the
// menu stays open, so several can be changed in a row; the list under it
// follows.
func TestViewMenuKeyboard(t *testing.T) {
	a := mount(t, Page{})
	tab(a, 3)
	a.ExpectFocused(viewButton)
	a.Press("ArrowDown") // opens it on the first item
	a.ExpectFocused(telatest.ByRole("menuitemcheckbox", "Branch"))
	a.ExpectAria(`
		- banner
		- menu "View":
		  - group "Columns":
		    - text: Columns
		    - menuitemcheckbox "Branch" [checked]
		    - menuitemcheckbox "Duration" [checked]
		    - menuitemcheckbox "Author"
	`)

	a.Press("Space") // unchecks Branch; the menu stays open
	a.ExpectFocused(telatest.ByRole("menuitemcheckbox", "Branch"))
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.Press("Space") // checks Author
	a.ExpectAria(`
		- banner
		- menu "View":
		  - group "Columns":
		    - text: Columns
		    - menuitemcheckbox "Branch"
		    - menuitemcheckbox "Duration" [checked]
		    - menuitemcheckbox "Author" [checked]
	`)

	a.Press("Escape")
	a.ExpectFocused(viewButton)
	// The rows lost their branch and got their author.
	a.ExpectNoText("release/5.2")
	a.ExpectCount(telatest.ByText("Ana García"), 2) // the author column and the list of people
	a.ExpectText("3m 12s")                          // the duration column stays
}

// TestTabsKeyboard: the arrows move between the tabs and select them (the
// panel changes with the focus), Home and End jump, and Tab goes from the
// selected tab to its panel.
func TestTabsKeyboard(t *testing.T) {
	a := mount(t, Page{})
	tab(a, 6)
	a.ExpectFocused(overviewTab)
	a.Press("ArrowRight")
	a.ExpectFocused(activityTab)
	expectSection(t, a, "- tablist", `
		- tablist "Sections":
		  - tab "Overview"
		  - tab "Activity" [selected]
		  - tab "Alerts"
	`)
	a.ExpectCount(telatest.ByRole("tabpanel", "Activity"), 1)
	a.ExpectText("rolled back payments-worker")
	a.Press("ArrowRight")
	a.ExpectFocused(alertsTab)
	a.ExpectText("Build cache is 91% full")
	a.Press("ArrowRight") // goes round
	a.ExpectFocused(overviewTab)
	a.Press("ArrowLeft")
	a.ExpectFocused(alertsTab)
	a.Press("Home")
	a.ExpectFocused(overviewTab)
	a.Press("End")
	a.ExpectFocused(alertsTab)
	a.Press("Tab")
	a.ExpectFocused(telatest.ByRole("tabpanel", "Alerts"))
}

// TestListBoxKeyboard: a multiple selection. Space toggles the focused
// person, Shift+arrows extend a range, Ctrl+A selects all, Escape clears,
// and typing moves the focus.
func TestListBoxKeyboard(t *testing.T) {
	a := mount(t, Page{})
	tab(a, 8)
	a.ExpectFocused(telatest.ByRole("option", "Ana García Platform lead"))
	a.ExpectText("2 of 6 selected")
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("option", "Omar Haddad Backend engineer"))
	a.Press("Space")
	a.ExpectText("3 of 6 selected")
	a.Expect(telatest.ByRole("option", "Omar Haddad Backend engineer"), telatest.Selected(true))
	a.Press("Space")
	a.Expect(telatest.ByRole("option", "Omar Haddad Backend engineer"), telatest.Selected(false))

	a.Type("k") // typeahead: Kwame
	a.ExpectFocused(telatest.ByRole("option", "Kwame Mensah Security engineer"))
	a.Advance(time.Second) // a space typed within the second would be part of the search
	a.Press("Space")
	a.ExpectText("3 of 6 selected")
	a.Press("Home")
	a.ExpectFocused(telatest.ByRole("option", "Ana García Platform lead"))

	a.Press("Control+a")
	a.ExpectText("6 of 6 selected")
	expectSection(t, a, `- listbox "People to notify"`, `
		- listbox "People to notify":
		  - option "Ana García Platform lead" [selected]
		  - option "Mei Tanaka Site reliability" [selected]
		  - option "Omar Haddad Backend engineer" [selected]
		  - option "Lena Fischer Frontend engineer" [selected]
		  - option "Sofia Rossi Product designer" [selected]
		  - option "Kwame Mensah Security engineer" [selected]
	`)
	a.Press("Escape")
	a.ExpectText("0 of 6 selected")
}

// TestTooltipKeyboard: the keyboard focus opens the tooltip at once, it
// describes the button, and Escape closes it before it does anything else.
func TestTooltipKeyboard(t *testing.T) {
	a := mount(t, Page{})
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	tab(a, 2)
	a.ExpectFocused(bell)
	expectSection(t, a, "- tooltip", `- tooltip "3 unread notifications"`)
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	a.ExpectFocused(bell) // Escape closed the tooltip, not the focus
	a.Press("Tab")
	a.ExpectFocused(viewButton)
}
