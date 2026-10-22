package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

var (
	versionField = telatest.ByRole("textbox", "Version")
	deployButton = telatest.ByRole("button", "Deploy")
	cancelButton = telatest.ByRole("button", "Cancel")
	closeButton  = telatest.ByRole("button", "Close")
	notifySwitch = telatest.ByRole("switch", "Notify the team")
	deployDialog = telatest.ByRole("dialog", "New deployment")
)

// TestDialogKeyboard opens the dialog from its button, fills the form with
// the keyboard, chooses an environment in the select, flips the switch and
// submits. The focus goes in, stays in while Tab goes round, and comes back
// to the button.
func TestDialogKeyboard(t *testing.T) {
	a := mount(t, Page{})
	tab(a, 5)
	a.ExpectFocused(newDeployment)
	a.Expect(newDeployment, telatest.Expanded(false))
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
	a.ExpectFocused(versionField)
	a.ExpectCount(telatest.ByRole("tab", ""), 0)

	// Tab goes round the dialog (the disabled button is skipped) and never
	// leaves it; Shift+Tab goes back.
	for _, l := range []telatest.Locator{
		telatest.ByRole("button", "Staging Environment"), notifySwitch, cancelButton, closeButton, versionField,
	} {
		a.Press("Tab")
		a.ExpectFocused(l)
	}
	a.Press("Shift+Tab")
	a.ExpectFocused(closeButton)
	a.Press("Tab")

	// Type a version: the Deploy button is enabled.
	a.Type("v3.0.0")
	a.Expect(deployButton, telatest.Disabled(false))

	// The select opens with the chosen environment focused; ArrowDown moves
	// to the next, Enter chooses it, closes the list and gives the focus back
	// to the button.
	a.Press("Tab")
	a.Press("ArrowDown")
	a.ExpectFocused(telatest.ByRole("option", "Staging"))
	expectSection(t, a, `- listbox "Environment"`, `
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
	a.ExpectCount(deployDialog, 1) // still open

	// Escape closes the select only: the dialog is under it.
	a.Press("Enter")
	a.ExpectFocused(telatest.ByRole("option", "Preview"))
	a.Press("Escape")
	a.ExpectFocused(telatest.ByRole("button", "Preview Environment"))
	a.ExpectCount(deployDialog, 1)

	// Space flips the switch.
	a.Press("Tab")
	a.Expect(notifySwitch, telatest.Checked(true))
	a.Press("Space")
	a.Expect(notifySwitch, telatest.Checked(false))

	// Enter on the Deploy button submits: the dialog closes, the deployment
	// is queued at the top of the list, and the focus is back on the button
	// that opened the dialog.
	a.Press("Tab") // Cancel
	a.Press("Tab") // Deploy
	a.ExpectFocused(deployButton)
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(newDeployment)
	a.ExpectText("Queued acme-api v3.0.0 for Preview")
	a.ExpectCount(telatest.ByRole("listitem", ""), 6)
}

// TestDialogImplicitSubmissionAndCancel: Enter in the text field submits the
// form, as in a browser, Escape and Cancel close the dialog without it, and
// every way out gives the focus back to the button.
func TestDialogImplicitSubmissionAndCancel(t *testing.T) {
	a := mount(t, Page{})
	tab(a, 5)
	a.Press("Space")
	a.ExpectFocused(versionField)

	// Enter with the field empty does nothing: the submit button is disabled.
	a.Press("Enter")
	a.ExpectCount(deployDialog, 1)

	// Escape closes the dialog and nothing was queued.
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(newDeployment)
	a.ExpectCount(telatest.ByRole("listitem", ""), 5)

	// Cancel, with Space.
	a.Press("Enter")
	a.Type("v9")
	tab(a, 3)
	a.ExpectFocused(cancelButton)
	a.Press("Space")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(newDeployment)
	a.ExpectCount(telatest.ByRole("listitem", ""), 5)

	// The form starts empty again; Enter in the field submits.
	a.Press("Enter")
	a.Expect(versionField, telatest.HasValue(""))
	a.Type("v2.15.0")
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectFocused(newDeployment)
	a.ExpectText("Queued acme-api v2.15.0 for Staging") // the default environment
	a.ExpectCount(telatest.ByRole("listitem", ""), 6)
}
