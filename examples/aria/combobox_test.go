package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// highlighted is the option the combobox's virtual focus is on: the input
// keeps the focus and the list marks one item (aria-activedescendant).
var highlighted = telatest.ByRole("option", "").Filter(telatest.HasClass("bg-accent"))

// TestComboboxKeyboard types in the project field: the list opens and
// filters while the focus stays on the input, the arrows highlight an item,
// Enter chooses it, and Escape and Tab put the text back.
func TestComboboxKeyboard(t *testing.T) {
	a := mount(t, Page{})
	a.Press("Tab")
	a.ExpectFocused(projectBox)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)

	// Replace the text with "se": the list opens with the projects that
	// contain it, whatever the case or the accents.
	a.Press("Control+a")
	a.Type("se")
	a.ExpectFocused(projectBox)
	expectSection(t, a, `- listbox "Project"`, `
		- listbox "Project":
		  - option "billing-service Service"
		  - option "analytics-warehouse Database"
		  - option "sessions-cache Database"
		  - option "users-db Database"
		  - option "security-policy Docs"
	`)
	a.ExpectCount(highlighted, 0) // nothing is highlighted until an arrow says so
	a.Expect(projectBox, telatest.Expanded(true), telatest.HasValue("se"))

	// The arrows highlight; the input keeps the focus.
	a.Press("ArrowDown")
	a.ExpectCount(highlighted, 1)
	a.Expect(highlighted, telatest.HasName("billing-service Service"))
	a.ExpectFocused(projectBox)
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.Expect(highlighted, telatest.HasName("sessions-cache Database"))
	a.Press("ArrowUp")
	a.Expect(highlighted, telatest.HasName("analytics-warehouse Database"))
	a.ExpectFocused(projectBox)

	// Typing again filters again and drops the highlight.
	a.Type("s")
	expectSection(t, a, `- listbox "Project"`, `
		- listbox "Project":
		  - option "sessions-cache Database"
	`)
	a.ExpectCount(highlighted, 0)
	a.Press("ArrowDown")
	a.Expect(highlighted, telatest.HasName("sessions-cache Database"))

	// Enter chooses the highlighted item: the text and the selection agree,
	// the list closes, the page follows and the focus stays on the input.
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
	a.Expect(projectBox, telatest.Expanded(false), telatest.HasValue("sessions-cache"))
	a.ExpectText("Releases of sessions-cache")
	a.ExpectFocused(projectBox)

	// Nothing matches: the list says so. Escape puts the chosen text back.
	a.Type("x")
	a.Expect(projectBox, telatest.HasValue("sessions-cachex"))
	a.ExpectText("No project matches your search.")
	a.Press("Escape")
	a.Expect(projectBox, telatest.HasValue("sessions-cache"))
	a.ExpectNoText("No project matches your search.")

	// ArrowDown on the closed field opens it with every project, the chosen
	// one highlighted and selected.
	a.Press("ArrowDown")
	if n := optionsOf(a, "Project"); n != 30 {
		t.Errorf("the open list has %d options, want the 30 projects", n)
	}
	a.Expect(highlighted, telatest.HasName("sessions-cache Database"))
	a.Expect(telatest.ByRole("option", "sessions-cache Database"), telatest.Selected(true))
	a.Press("ArrowDown")
	a.Expect(highlighted, telatest.HasName("users-db Database"))
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)

	// Tab commits what is typed: a text that is no project goes back to the
	// chosen one, and the focus moves on.
	a.Type("zz")
	a.Press("Tab")
	a.ExpectFocused(bell)
	a.Expect(projectBox, telatest.HasValue("sessions-cache"))
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
}
