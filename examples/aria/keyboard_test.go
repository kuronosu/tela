package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The tests of this file use the keyboard only, the way someone who cannot
// use a pointer does: Tab, Shift+Tab, the arrows, Enter, Space, Escape, Home,
// End and typing. What they assert is what that person's screen reader
// reads (the aria snapshot) and where the focus is.

// TestTabOrder walks the screen with Tab: the combobox, the bell, the two
// menu buttons, the dialog button, the selected tab, its panel and the list
// box, and back with Shift+Tab. The toggle button of the combobox is not a tab
// stop (it is for the pointer; the arrows do its job), and nothing else on
// the page takes the focus.
func TestTabOrder(t *testing.T) {
	a := mount(t, Page{})
	order := []telatest.Locator{
		projectBox, bell, viewButton, actionsButton, newDeployment,
		overviewTab,
		telatest.ByRole("tabpanel", "Overview"),
		telatest.ByRole("option", "Ana García Platform lead"), // the list box focuses its first selected item
	}
	for _, l := range order {
		a.Press("Tab")
		a.ExpectFocused(l)
	}
	for i := len(order) - 2; i >= 0; i-- {
		a.Press("Shift+Tab")
		a.ExpectFocused(order[i])
	}
}

// TestActionsMenuKeyboard opens the Actions menu from its button, walks it,
// chooses with Enter and Space, and leaves it with Escape: the focus
// returns to the button every time.
func TestActionsMenuKeyboard(t *testing.T) {
	a := mount(t, Page{})
	tab(a, 4)
	a.ExpectFocused(actionsButton)
	a.Expect(actionsButton, telatest.Expanded(false))

	// Enter opens it and focuses its first item. Everything else is inert
	// while it is open: the aria snapshot is the menu, with its sections,
	// its separator and the disabled item.
	a.Press("Enter")
	a.ExpectFocused(telatest.ByRole("menuitem", "Rename project"))
	a.ExpectAria(`
		- banner
		- menu "Actions":
		  - group "Project":
		    - text: Project
		    - menuitem "Rename project"
		    - menuitem "Duplicate"
		    - menuitem "Redeploy"
		    - menuitem "Share link"
		  - separator
		  - group "Manage":
		    - text: Manage
		    - menuitem "Export report" [disabled]
		    - menuitem "Archive"
		    - menuitem "Delete project"
	`)

	// The arrows move the focus; the disabled item is skipped both ways.
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("menuitem", "Duplicate"))
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("menuitem", "Share link"))
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("menuitem", "Archive"))
	a.Press("ArrowUp")
	a.ExpectFocused(telatest.ByRole("menuitem", "Share link"))
	a.Press("End")
	a.ExpectFocused(telatest.ByRole("menuitem", "Delete project"))
	a.Press("Home")
	a.ExpectFocused(telatest.ByRole("menuitem", "Rename project"))
	// Typing moves to the item that starts with the text.
	a.Type("ar")
	a.ExpectFocused(telatest.ByRole("menuitem", "Archive"))
	a.Type("x") // no item starts with "arx": the focus stays
	a.ExpectFocused(telatest.ByRole("menuitem", "Archive"))

	// Escape closes the menu, and the button has the focus again.
	a.Press("Escape")
	a.Expect(actionsButton, telatest.Expanded(false))
	a.ExpectFocused(actionsButton)
	a.ExpectCount(telatest.ByRole("menu", ""), 0)

	// ArrowUp opens it on the last item; the menu is where the focus is, so
	// Escape twice would leave nothing open.
	a.Press("ArrowUp")
	a.ExpectFocused(telatest.ByRole("menuitem", "Delete project"))
	a.Press("Escape")
	a.ExpectFocused(actionsButton)

	// Space opens it too; Enter chooses an item, which closes the menu, runs
	// its action and gives the focus back.
	a.Press("Space")
	a.ExpectFocused(telatest.ByRole("menuitem", "Rename project"))
	a.Press("ArrowDown")
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectFocused(actionsButton)
	a.ExpectText("Duplicate: acme-api")

	// Space chooses as well.
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.Press("Space")
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectFocused(actionsButton)
	a.ExpectText("Redeploy: acme-api")
}
