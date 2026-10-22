package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// The same widgets with the pointer: the hooks give a mouse the behavior
// they give the keyboard (focus where it belongs, dismissal, restoration),
// and a press outside an overlay closes it.

// blank is a point of the window that no control covers: below the cards.
const blankX, blankY = 560, 735

func TestActionsMenuPointer(t *testing.T) {
	a := mount(t, Page{})
	a.Click(actionsButton)
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
	a.ExpectFocused(actionsButton)

	// A press outside closes it without an action: it is modal, so the press
	// does not reach what is below.
	a.Click(actionsButton)
	a.ExpectCount(telatest.ByRole("menu", "Actions"), 1)
	a.ClickAt(blankX, blankY)
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectText("Archive: acme-api") // the same notice: nothing ran
}

func TestViewMenuPointer(t *testing.T) {
	a := mount(t, Page{})
	a.Click(viewButton)
	a.Click(telatest.ByRole("menuitemcheckbox", "Duration"))
	// The menu stays open, with the item unchecked, and the list lost its column.
	a.Expect(telatest.ByRole("menuitemcheckbox", "Duration"), telatest.Checked(false))
	a.Click(telatest.ByRole("menuitemcheckbox", "Author"))
	a.Expect(telatest.ByRole("menuitemcheckbox", "Author"), telatest.Checked(true))
	a.Click(telatest.ByRole("menuitemcheckbox", "Duration"))
	a.Expect(telatest.ByRole("menuitemcheckbox", "Duration"), telatest.Checked(true))
	a.Press("Escape")
	a.ExpectFocused(viewButton)
	a.ExpectCount(telatest.ByText("Ana García"), 2)
}

func TestComboboxPointer(t *testing.T) {
	a := mount(t, Page{})
	// The toggle button shows every project without moving the focus off the input.
	a.Click(projectToggle)
	a.ExpectFocused(projectBox)
	if n := optionsOf(a, "Project"); n != 30 {
		t.Errorf("the open list has %d options, want 30", n)
	}
	// The mouse highlights what it is over, and a press chooses.
	a.Hover(telatest.ByRole("option", "edge-router Service"))
	a.Expect(highlighted, telatest.HasName("edge-router Service"))
	a.Click(telatest.ByRole("option", "edge-router Service"))
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
	a.Expect(projectBox, telatest.HasValue("edge-router"))
	a.ExpectText("Releases of edge-router")
	a.ExpectFocused(projectBox)

	// The button closes the list too.
	a.Click(projectToggle)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 1)
	a.Click(projectToggle)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)

	// A press elsewhere moves the focus, which commits the text and closes the
	// list; the press reaches what it hit.
	a.Click(projectBox)
	a.Type("x")
	a.ExpectText("No project matches your search.")
	a.Click(overviewTab)
	a.ExpectCount(telatest.ByRole("listbox", "Project"), 0)
	a.Expect(projectBox, telatest.HasValue("edge-router"))
	a.ExpectFocused(overviewTab)
}

func TestDialogPointer(t *testing.T) {
	a := mount(t, Page{})
	a.Click(newDeployment)
	a.ExpectCount(deployDialog, 1)
	a.ExpectFocused(versionField)

	// A press on the scrim is a press outside: it closes the dialog and the
	// button has the focus again.
	a.ClickAt(40, 700)
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(newDeployment)

	// The close button.
	a.Click(newDeployment)
	a.Click(closeButton)
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(newDeployment)

	// Fill the form with the mouse and deploy.
	a.Click(newDeployment)
	a.Fill(versionField, "v4.1.0")
	a.Click(telatest.ByRole("button", "Staging Environment"))
	a.Click(telatest.ByRole("option", "Production"))
	a.ExpectCount(telatest.ByRole("listbox", ""), 0)
	a.Click(telatest.ByText("Notify the team")) // the label toggles its switch
	a.Expect(notifySwitch, telatest.Checked(false))
	a.Click(deployButton)
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectText("Queued acme-api v4.1.0 for Production")
	a.ExpectCount(telatest.ByRole("listitem", ""), 6)
	a.ExpectFocused(newDeployment)

	// A press outside the select's list closes only the list.
	a.Click(newDeployment)
	a.Click(telatest.ByRole("button", "Staging Environment"))
	a.ExpectCount(telatest.ByRole("listbox", "Environment"), 1)
	a.Click(telatest.ByText("Ship a version of acme-api to an environment."))
	a.ExpectCount(telatest.ByRole("listbox", ""), 0)
	a.ExpectCount(deployDialog, 1)
}

func TestTabsAndListBoxPointer(t *testing.T) {
	a := mount(t, Page{})
	a.Click(alertsTab)
	a.Expect(alertsTab, telatest.Selected(true))
	a.ExpectText("payments-worker failed its health check")
	a.Click(overviewTab)
	a.ExpectText("Recent deployments")

	// A click toggles a person in the list box; the selection is multiple.
	a.ExpectText("2 of 6 selected")
	a.Click(telatest.ByRole("option", "Omar Haddad Backend engineer"))
	a.Click(telatest.ByRole("option", "Lena Fischer Frontend engineer"))
	a.ExpectText("4 of 6 selected")
	a.Click(telatest.ByRole("option", "Ana García Platform lead"))
	a.ExpectText("3 of 6 selected")
	a.Expect(telatest.ByRole("option", "Ana García Platform lead"), telatest.Selected(false))
}

// TestTooltipPointer: the tooltip opens when the pointer has rested on the
// button for half a second, stays while it is over the tooltip and closes
// after the pointer leaves. The delays run on the frame clock, so the test
// drives it.
func TestTooltipPointer(t *testing.T) {
	a := mount(t, Page{}, telatest.NoSettle())
	tip := telatest.ByRole("tooltip", "3 unread notifications")
	a.Hover(bell)
	a.Advance(400 * time.Millisecond)
	a.ExpectCount(tip, 0)
	a.Advance(200 * time.Millisecond)
	a.ExpectCount(tip, 1)
	// Leaving closes it after 0.5 s.
	a.MouseMove(blankX, blankY)
	a.Advance(300 * time.Millisecond)
	a.ExpectCount(tip, 1)
	a.Advance(300 * time.Millisecond)
	a.Advance(300 * time.Millisecond) // the exit animation
	a.ExpectCount(tip, 0)
}
