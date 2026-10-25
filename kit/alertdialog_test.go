package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's AlertDialog parts (new-york-v4/ui/alert-dialog.tsx at
// kit.ShadcnCommit), verbatim.
const (
	shadcnAlertContent     = "group/alert-dialog-content fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-4 rounded-lg border bg-background p-6 shadow-lg duration-200 data-[size=sm]:max-w-xs data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 data-[size=default]:sm:max-w-lg"
	shadcnAlertHeader      = "grid grid-rows-[auto_1fr] place-items-center gap-1.5 text-center has-data-[slot=alert-dialog-media]:grid-rows-[auto_auto_1fr] has-data-[slot=alert-dialog-media]:gap-x-6 sm:group-data-[size=default]/alert-dialog-content:place-items-start sm:group-data-[size=default]/alert-dialog-content:text-left sm:group-data-[size=default]/alert-dialog-content:has-data-[slot=alert-dialog-media]:grid-rows-[auto_1fr]"
	shadcnAlertFooter      = "flex flex-col-reverse gap-2 group-data-[size=sm]/alert-dialog-content:grid group-data-[size=sm]/alert-dialog-content:grid-cols-2 sm:flex-row sm:justify-end"
	shadcnAlertTitle       = "text-lg font-semibold sm:group-data-[size=default]/alert-dialog-content:group-has-data-[slot=alert-dialog-media]/alert-dialog-content:col-start-2"
	shadcnAlertDescription = "text-sm text-muted-foreground"
)

// alertCell is an open alert dialog's panel on both sides.
func alertCell(name string, a kit.AlertDialog, title, description, cancel, action string) cell {
	size := "default"
	if a.Size == kit.Sm {
		size = "sm"
	}
	a.Title, a.Description = h.Text(title), h.Text(description)
	a.Cancel, a.Action = h.Text(cancel), h.Text(action)
	a.Props = h.TestID(name + "-panel")
	actionVariant := kit.Default
	if a.Destructive {
		actionVariant = kit.Destructive
	}
	cancelBtn := buttonHTML(kit.Button{Variant: kit.Outline}, cancel, "")
	actionBtn := buttonHTML(kit.Button{Variant: actionVariant}, action, "")
	inner := fmt.Sprintf(`<div data-slot="alert-dialog-header" class="%s"><h2 data-slot="alert-dialog-title" class="%s">%s</h2><p data-slot="alert-dialog-description" class="%s">%s</p></div>`,
		shadcnAlertHeader, shadcnAlertTitle, title, shadcnAlertDescription, description) +
		fmt.Sprintf(`<div data-slot="alert-dialog-footer" class="%s">%s%s</div>`, shadcnAlertFooter, cancelBtn, actionBtn)
	return cell{
		name: name, width: 560, probes: []string{name + "-panel"}, tela: kit.StaticAlertDialog(a),
		html: fmt.Sprintf(`<div role="alertdialog" data-probe="%s-panel" data-slot="alert-dialog-content" data-size="%s" class="%s">%s</div>`,
			name, size, "relative "+staticClasses(shadcnAlertContent), inner),
	}
}

func alertDialogSheet() *sheet {
	return &sheet{name: "alertdialog", cells: []cell{
		alertCell("default", kit.AlertDialog{}, "Are you absolutely sure?",
			"This action cannot be undone. This will permanently delete your account and remove your data from our servers.", "Cancel", "Continue"),
		alertCell("destructive", kit.AlertDialog{Destructive: true}, "Delete project?",
			"The project and its deployments are deleted for everyone.", "Cancel", "Delete"),
		alertCell("sm", kit.AlertDialog{Size: kit.Sm}, "Discard changes?", "Your edits will be lost.", "Keep", "Discard"),
		alertCell("sm-destructive", kit.AlertDialog{Size: kit.Sm, Destructive: true}, "Delete this file?",
			"This cannot be undone.", "Cancel", "Delete"),
	}}
}

func TestAlertDialogGolden(t *testing.T)   { alertDialogSheet().golden(t) }
func TestChromiumAlertDialog(t *testing.T) { alertDialogSheet().chromium(t) }

var (
	deleteButton = telatest.ByRole("button", "Delete project")
	alertDlg     = telatest.ByRole("alertdialog", "Are you sure?")
)

// deleteDialog is the alert dialog of the behavior tests.
func deleteDialog(a kit.AlertDialog) kit.AlertDialog {
	a.Trigger = kit.Button{Variant: kit.Outline, Children: h.Text("Delete project")}
	a.Title = h.Text("Are you sure?")
	a.Description = h.Text("This cannot be undone.")
	a.Action = h.Text("Delete")
	a.Destructive = true
	return a
}

func TestAlertDialogOpens(t *testing.T) {
	app := telatest.MountFunc(t, pageUnder(deleteDialog(kit.AlertDialog{})), telatest.Theme(kit.Theme()))
	app.Expect(deleteButton, telatest.HasPopup("dialog"), telatest.Expanded(false))
	app.Click(deleteButton)
	app.ExpectAria(`
		- alertdialog "Are you sure?":
		  - heading "Are you sure?" [level=2]
		  - paragraph: This cannot be undone.
		  - button "Cancel"
		  - button "Delete"
	`)
	// The focus is on Cancel, the safe choice.
	app.ExpectFocused(telatest.ByRole("button", "Cancel"))
}

func TestAlertDialogNeedsAnAnswer(t *testing.T) {
	var action, cancel int
	page := pageUnder(deleteDialog(kit.AlertDialog{OnAction: func() { action++ }, OnCancel: func() { cancel++ }}))
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))

	// A press outside does not close it: the page behind is inert, and the
	// dialog stays.
	app.Click(deleteButton)
	app.ClickAt(10, 10)
	app.ExpectCount(alertDlg, 1)

	// Tab stays inside, whatever the press did to the focus.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Cancel"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Delete"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Cancel"))

	// Cancel closes it and gives the focus back.
	app.Press("Enter")
	app.ExpectCount(telatest.ByRole("alertdialog", ""), 0)
	app.ExpectFocused(deleteButton)
	if cancel != 1 || action != 0 {
		t.Errorf("OnCancel %d, OnAction %d, want 1 and 0", cancel, action)
	}

	// Action calls OnAction and closes it.
	app.Click(deleteButton)
	app.Click(telatest.ByRole("button", "Delete"))
	app.ExpectCount(telatest.ByRole("alertdialog", ""), 0)
	app.ExpectFocused(deleteButton)
	if cancel != 1 || action != 1 {
		t.Errorf("OnCancel %d, OnAction %d, want 1 and 1", cancel, action)
	}

	// Escape closes it without calling either.
	app.Click(deleteButton)
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("alertdialog", ""), 0)
	if cancel != 1 || action != 1 {
		t.Errorf("OnCancel %d, OnAction %d after Escape, want 1 and 1", cancel, action)
	}
}

func TestAlertDialogDefaultLabelsAndControlled(t *testing.T) {
	var log []bool
	open := true
	app := telatest.MountFunc(t, pageUnder(kit.AlertDialog{Open: &open, OnOpenChange: func(o bool) { log = append(log, o) },
		Title: h.Text("Sure?")}), telatest.Theme(kit.Theme()))
	app.ExpectCount(telatest.ByRole("alertdialog", "Sure?"), 1)
	app.ExpectCount(telatest.ByRole("button", "Cancel"), 1)
	app.ExpectCount(telatest.ByRole("button", "Continue"), 1)
	// The request is reported; the dialog is controlled and stays.
	app.Click(telatest.ByRole("button", "Continue"))
	app.ExpectCount(telatest.ByRole("alertdialog", "Sure?"), 1)
	if fmt.Sprint(log) != "[false]" {
		t.Errorf("OnOpenChange got %v, want [false]", log)
	}
}

func TestAlertDialogSizeNeedsADefinedSize(t *testing.T) {
	_, err := telatest.AriaSnapshot(tela.C(pageUnder(kit.AlertDialog{Size: "huge", DefaultOpen: true, Title: h.Text("x")})))
	if err == nil {
		t.Fatal("an undefined Size did not panic in dev mode")
	}
}

func TestAlertDialogOpenGolden(t *testing.T) {
	openGolden(t, "alertdialog", 800, 500, pageUnder(deleteDialog(kit.AlertDialog{})), func(a *telatest.App) { a.Click(deleteButton) })
}

func TestAlertDialogIdle(t *testing.T) {
	expectIdle(t, pageUnder(deleteDialog(kit.AlertDialog{DefaultOpen: true})), nil)
}
