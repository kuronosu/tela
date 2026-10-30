package aria_test

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The tests of this file use the keyboard only, the way someone who cannot
// use a pointer does: Tab, Shift+Tab, the arrows, Enter, Space, Escape, Home,
// End and typing. What they assert is what that person's screen reader reads
// (the aria snapshot) and where the focus is. They are integration tests of
// the hooks together (see screen_test.go); what a single hook already tests
// by itself is left to its own test file.

// TestScreenTabOrder walks the whole screen with Tab: the combobox, the bell,
// the two menu buttons, the dialog button, the selected tab, its panel and
// the list box (which focuses its first selected item), and back with
// Shift+Tab. The toggle button of the combobox is not a tab stop (it is for
// the pointer; the arrows do its job), and nothing else on the page takes the
// focus.
func TestScreenTabOrder(t *testing.T) {
	a := screenMount(t, screenPage{})
	order := []telatest.Locator{
		screenProjectBox, screenBellButton, screenViewButton, screenActionsButton, screenNewButton,
		screenOverviewTab,
		telatest.ByRole("tabpanel", "Overview"),
		telatest.ByRole("option", "Ana García Platform lead"),
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

// TestScreenActionsMenuKeyboard opens a menu with sections, a separator and a
// disabled item, and walks it: while it is open the rest of the window is
// inert, the arrows skip the disabled item in both directions across the
// sections, Home and End go to the ends, typeahead finds an item and keeps
// the focus when nothing matches, and an item that is chosen runs its action
// in the page and gives the focus back to the button.
func TestScreenActionsMenuKeyboard(t *testing.T) {
	a := screenMount(t, screenPage{})
	screenPressTab(a, 4)
	a.ExpectFocused(screenActionsButton)
	a.Expect(screenActionsButton, telatest.Expanded(false))

	// Enter opens it and focuses its first item. Everything else is inert
	// while it is open: the aria snapshot is the menu, with its sections, its
	// separator and the disabled item (what remains of the page is its empty
	// landmark).
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

	// The arrows move the focus across the sections; the disabled item is
	// skipped both ways.
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
	a.Expect(screenActionsButton, telatest.Expanded(false))
	a.ExpectFocused(screenActionsButton)
	a.ExpectCount(telatest.ByRole("menu", ""), 0)

	// Enter on an item closes the menu, runs its action (the page shows it)
	// and gives the focus back.
	a.Press("Space")
	a.Press("ArrowDown")
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectFocused(screenActionsButton)
	a.ExpectText("Duplicate: acme-api")
}

// TestScreenViewMenuKeyboard is a menu of checkbox items whose state belongs
// to the page: Space toggles one and the menu stays open, so several can be
// changed in a row, and the list under it follows.
func TestScreenViewMenuKeyboard(t *testing.T) {
	a := screenMount(t, screenPage{})
	screenPressTab(a, 3)
	a.ExpectFocused(screenViewButton)
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
	a.ExpectFocused(screenViewButton)
	// The rows lost their branch and got their author.
	a.ExpectNoText("release/5.2")
	a.ExpectCount(telatest.ByText("Ana García"), 2) // the author column and the list of people
	a.ExpectText("3m 12s")                          // the duration column stays
}

// TestScreenComboboxKeyboard types in the project field of the page: the list
// opens and filters (aria.Contains: case and accents ignored) while the focus
// stays on the input, the arrows highlight an item through
// aria-activedescendant, Enter chooses it and the page follows, and Escape and
// Tab put the chosen text back.
func TestScreenComboboxKeyboard(t *testing.T) {
	a := screenMount(t, screenPage{})
	a.Press("Tab")
	a.ExpectFocused(screenProjectBox)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)

	// Replace the text with "se": the list opens with the projects that
	// contain it.
	a.Press("Control+a")
	a.Type("se")
	a.ExpectFocused(screenProjectBox)
	screenExpectSection(t, a, `- listbox "Project"`, `
		- listbox "Project":
		  - option "billing-service"
		  - option "analytics-warehouse"
		  - option "sessions-cache"
		  - option "users-db"
		  - option "security-policy"
	`)
	a.Expect(screenProjectBox, telatest.NoActiveDescendant()) // nothing is highlighted until an arrow says so
	a.Expect(screenProjectBox, telatest.Expanded(true), telatest.HasValue("se"))

	// The arrows highlight; the input keeps the focus.
	a.Press("ArrowDown")
	a.Expect(screenProjectBox, screenActive("billing-service"))
	a.ExpectFocused(screenProjectBox)
	a.Press("ArrowDown")
	a.Press("ArrowDown")
	a.Expect(screenProjectBox, screenActive("sessions-cache"))
	a.Press("ArrowUp")
	a.Expect(screenProjectBox, screenActive("analytics-warehouse"))
	a.ExpectFocused(screenProjectBox)

	// Typing again filters again and drops the highlight.
	a.Type("s")
	screenExpectSection(t, a, `- listbox "Project"`, `
		- listbox "Project":
		  - option "sessions-cache"
	`)
	a.Expect(screenProjectBox, telatest.NoActiveDescendant())
	a.Press("ArrowDown")
	a.Expect(screenProjectBox, screenActive("sessions-cache"))

	// Enter chooses the highlighted item: the text and the selection agree,
	// the list closes, the page follows and the focus stays on the input.
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
	a.Expect(screenProjectBox, telatest.Expanded(false), telatest.HasValue("sessions-cache"))
	a.ExpectText("Releases of sessions-cache")
	a.ExpectFocused(screenProjectBox)

	// Nothing matches: the list says so (AllowsEmptyCollection). Escape puts
	// the chosen text back.
	a.Type("x")
	a.Expect(screenProjectBox, telatest.HasValue("sessions-cachex"))
	a.ExpectText("No project matches your search.")
	a.Press("Escape")
	a.Expect(screenProjectBox, telatest.HasValue("sessions-cache"))
	a.ExpectNoText("No project matches your search.")

	// ArrowDown on the closed field opens it with every project, the chosen
	// one highlighted and selected.
	a.Press("ArrowDown")
	if n := screenOptionsOf(a, "Project"); n != len(screenProjects) {
		t.Errorf("the open list has %d options, want the %d projects", n, len(screenProjects))
	}
	a.Expect(screenProjectBox, screenActive("sessions-cache"))
	a.Expect(telatest.ByRole("option", "sessions-cache"), telatest.Selected(true))
	a.Press("ArrowDown")
	a.Expect(screenProjectBox, screenActive("users-db"))
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)

	// Tab commits what is typed: a text that is no project goes back to the
	// chosen one, and the focus moves on past the toggle button.
	a.Type("zz")
	a.Press("Tab")
	a.ExpectFocused(screenBellButton)
	a.Expect(screenProjectBox, telatest.HasValue("sessions-cache"))
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
}

// TestScreenDialogKeyboard opens the dialog from its button, fills the form
// with the keyboard, chooses an environment in the select inside the modal
// dialog, flips the switch and submits. The focus goes in, stays in while Tab
// goes round (the disabled submit button is skipped), and comes back to the
// button.
func TestScreenDialogKeyboard(t *testing.T) {
	a := screenMount(t, screenPage{})
	screenPressTab(a, 5)
	a.ExpectFocused(screenNewButton)
	a.Expect(screenNewButton, telatest.Expanded(false))
	a.Press("Enter")

	// The dialog is the only thing on the screen for a screen reader: the
	// page is inert (what remains of it is its empty landmark).
	a.ExpectAria(`
		- banner
		- dialog "New deployment":
		  - heading "New deployment" [level=2]
		  - paragraph: Ship a version of acme-api to an environment.
		  - text: Version
		  - textbox "Version"
		  - text: Environment
		  - button "Staging Environment": Staging
		  - text: Notify the team
		  - paragraph: Post the release notes when it finishes.
		  - switch "Notify the team" [checked]
		  - button "Cancel"
		  - button "Deploy" [disabled]
		  - button "Close"
	`)
	// The first field has the focus, and the page cannot be reached.
	a.ExpectFocused(screenVersionField)
	a.ExpectCount(telatest.ByRole("tab", ""), 0)

	// Tab goes round the dialog (the disabled button is skipped) and never
	// leaves it; Shift+Tab goes back.
	for _, l := range []telatest.Locator{
		telatest.ByRole("button", "Staging Environment"), screenNotifySwitch, screenCancelButton, screenCloseButton, screenVersionField,
	} {
		a.Press("Tab")
		a.ExpectFocused(l)
	}
	a.Press("Shift+Tab")
	a.ExpectFocused(screenCloseButton)
	a.Press("Tab")

	// Type a version: the Deploy button is enabled.
	a.Type("v3.0.0")
	a.Expect(screenDeployButton, telatest.Disabled(false))

	// The select opens with the chosen environment focused; ArrowDown moves
	// to the next, Enter chooses it, closes the list and gives the focus back
	// to the select's button, with the dialog still open.
	a.Press("Tab")
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("option", "Staging"))
	screenExpectSection(t, a, `- listbox "Environment"`, `
		- listbox "Environment":
		  - option "Production"
		  - option "Staging" [selected]
		  - option "Preview"
		  - option "Development"
	`)
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("option", "Preview"))
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("listbox", ""), 0)
	a.ExpectFocused(telatest.ByRole("button", "Preview Environment"))
	a.ExpectCount(screenDialog, 1)

	// Escape closes the select only: the dialog is under it.
	a.Press("Enter")
	a.ExpectFocused(telatest.ByRole("option", "Preview"))
	a.Press("Escape")
	a.ExpectFocused(telatest.ByRole("button", "Preview Environment"))
	a.ExpectCount(screenDialog, 1)

	// Space flips the switch.
	a.Press("Tab")
	a.Expect(screenNotifySwitch, telatest.Checked(true))
	a.Press("Space")
	a.Expect(screenNotifySwitch, telatest.Checked(false))

	// Enter on the Deploy button submits: the dialog closes, the deployment
	// is queued at the top of the list, and the focus is back on the button
	// that opened the dialog.
	a.Press("Tab") // Cancel
	a.Press("Tab") // Deploy
	a.ExpectFocused(screenDeployButton)
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(screenNewButton)
	a.ExpectText("Queued acme-api v3.0.0 for Preview")
	a.ExpectCount(telatest.ByRole("listitem", ""), len(screenDeployments)+1)
}

// TestScreenDialogImplicitSubmissionAndCancel: Enter in the text field
// submits the form, as in a browser, unless the submit button is disabled;
// Escape and Cancel close the dialog without it, and every way out gives the
// focus back to the button.
func TestScreenDialogImplicitSubmissionAndCancel(t *testing.T) {
	a := screenMount(t, screenPage{})
	screenPressTab(a, 5)
	a.Press("Space")
	a.ExpectFocused(screenVersionField)

	// Enter with the field empty does nothing: the submit button is disabled.
	a.Press("Enter")
	a.ExpectCount(screenDialog, 1)

	// Escape closes the dialog and nothing was queued.
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(screenNewButton)
	a.ExpectCount(telatest.ByRole("listitem", ""), len(screenDeployments))

	// Cancel, with Space.
	a.Press("Enter")
	a.Type("v9")
	screenPressTab(a, 3)
	a.ExpectFocused(screenCancelButton)
	a.Press("Space")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(screenNewButton)
	a.ExpectCount(telatest.ByRole("listitem", ""), len(screenDeployments))

	// The form starts empty again; Enter in the field submits.
	a.Press("Enter")
	a.Expect(screenVersionField, telatest.HasValue(""))
	a.Type("v2.15.0")
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(screenNewButton)
	a.ExpectText("Queued acme-api v2.15.0 for Staging") // the default environment
	a.ExpectCount(telatest.ByRole("listitem", ""), len(screenDeployments)+1)
}
