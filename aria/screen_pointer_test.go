package aria_test

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The same widgets of the screen (see screen_test.go) with the pointer: the
// hooks give a mouse the behavior they give the keyboard (focus where it
// belongs, dismissal, restoration), and a press outside an overlay closes it.

func TestScreenActionsMenuPointer(t *testing.T) {
	a := screenMount(t, screenPage{})
	a.Click(screenActionsButton)
	// A mouse opens the menu with the menu itself focused; the item under the
	// pointer takes the focus.
	a.ExpectFocused(telatest.ByRole("menu", "Actions"))
	a.Hover(telatest.ByRole("menuitem", "Redeploy"))
	a.ExpectFocused(telatest.ByRole("menuitem", "Redeploy"))

	// A disabled item takes no pointer (aria-disabled:pointer-events-none): a
	// press on it does nothing and the menu stays open.
	export := a.Bounds(telatest.ByRole("menuitem", "Export report"))
	a.ClickAt(export.X+export.Width/2, export.Y+export.Height/2)
	a.ExpectCount(telatest.ByRole("menu", "Actions"), 1)
	a.ExpectNoText("Export report: acme-api")

	// Choosing an item closes the menu, runs its action and gives the focus
	// back to the button, which stays focused after a click.
	a.Click(telatest.ByRole("menuitem", "Archive"))
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectText("Archive: acme-api")
	a.ExpectFocused(screenActionsButton)

	// A press outside closes it without an action: it is modal, so the press
	// does not reach what is below.
	a.Click(screenActionsButton)
	a.ExpectCount(telatest.ByRole("menu", "Actions"), 1)
	a.ClickAt(screenBlankX, screenBlankY)
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectText("Archive: acme-api") // the same notice: nothing ran
}

func TestScreenViewMenuPointer(t *testing.T) {
	a := screenMount(t, screenPage{})
	a.Click(screenViewButton)
	a.Click(telatest.ByRole("menuitemcheckbox", "Duration"))
	// The menu stays open, with the item unchecked, and the list lost its
	// column.
	a.Expect(telatest.ByRole("menuitemcheckbox", "Duration"), telatest.Checked(false))
	a.Click(telatest.ByRole("menuitemcheckbox", "Author"))
	a.Expect(telatest.ByRole("menuitemcheckbox", "Author"), telatest.Checked(true))
	a.Click(telatest.ByRole("menuitemcheckbox", "Duration"))
	a.Expect(telatest.ByRole("menuitemcheckbox", "Duration"), telatest.Checked(true))
	a.Press("Escape")
	a.ExpectFocused(screenViewButton)
	a.ExpectCount(telatest.ByText("Ana García"), 2)
}

func TestScreenComboboxPointer(t *testing.T) {
	a := screenMount(t, screenPage{})
	// The toggle button shows every project without moving the focus off the
	// input.
	a.Click(screenProjectToggle)
	a.ExpectFocused(screenProjectBox)
	if n := screenOptionsOf(a, "Project"); n != len(screenProjects) {
		t.Errorf("the open list has %d options, want %d", n, len(screenProjects))
	}
	// The mouse highlights what it is over, and a press chooses.
	a.Hover(telatest.ByRole("option", "edge-router"))
	a.Expect(screenProjectBox, screenActive("edge-router"))
	a.Click(telatest.ByRole("option", "edge-router"))
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
	a.Expect(screenProjectBox, telatest.HasValue("edge-router"))
	a.ExpectText("Releases of edge-router")
	a.ExpectFocused(screenProjectBox)

	// The button closes the list too.
	a.Click(screenProjectToggle)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 1)
	a.Click(screenProjectToggle)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)

	// A press elsewhere moves the focus, which commits the text and closes
	// the list; the press reaches what it hit.
	a.Click(screenProjectBox)
	a.Type("x")
	a.ExpectText("No project matches your search.")
	a.Click(screenOverviewTab)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
	a.Expect(screenProjectBox, telatest.HasValue("edge-router"))
	a.ExpectFocused(screenOverviewTab)
}

func TestScreenDialogPointer(t *testing.T) {
	a := screenMount(t, screenPage{})
	a.Click(screenNewButton)
	a.ExpectCount(screenDialog, 1)
	a.ExpectFocused(screenVersionField)

	// A press on the scrim is a press outside: it closes the dialog and the
	// button has the focus again.
	a.ClickAt(40, 700)
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(screenNewButton)

	// The close button.
	a.Click(screenNewButton)
	a.Click(screenCloseButton)
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(screenNewButton)

	// Fill the form with the mouse and deploy.
	a.Click(screenNewButton)
	a.Fill(screenVersionField, "v4.1.0")
	a.Click(telatest.ByRole("button", "Staging Environment"))
	a.Click(telatest.ByRole("option", "Production"))
	a.ExpectCount(telatest.ByRole("listbox", ""), 0)
	a.Click(telatest.ByText("Notify the team")) // the label toggles its switch
	a.Expect(screenNotifySwitch, telatest.Checked(false))
	a.Click(screenDeployButton)
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectText("Queued acme-api v4.1.0 for Production")
	a.ExpectCount(telatest.ByRole("listitem", ""), len(screenDeployments)+1)
	a.ExpectFocused(screenNewButton)

	// A press outside the select's list closes only the list.
	a.Click(screenNewButton)
	a.Click(telatest.ByRole("button", "Staging Environment"))
	a.ExpectCount(telatest.ByRole("listbox", "Environment"), 1)
	a.Click(telatest.ByText("Ship a version of acme-api to an environment."))
	a.ExpectCount(telatest.ByRole("listbox", ""), 0)
	a.ExpectCount(screenDialog, 1)
}
