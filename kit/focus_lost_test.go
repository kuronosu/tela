package kit_test

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The focus after the focused element of a modal dialog unmounts: the
// dialog's panel takes it (Radix's FocusScope moves it to its container),
// so the keys still reach the dialog and Tab goes on inside.

var (
	listButton = telatest.ByRole("button", "Edit list")
	listDlg    = telatest.ByRole("dialog", "Edit list")
	removeA    = telatest.ByRole("button", "Remove A")
)

// listDialog is a Dialog whose items each have a button that removes the
// item, and so the button itself.
func listDialog() tela.Node {
	items, setItems := tela.UseState([]string{"A", "B", "C"})
	rows := make([]tela.Child, 0, len(items)+1)
	rows = append(rows, h.Class("flex flex-col gap-2"))
	for _, it := range items {
		rows = append(rows, h.Div(h.Key(it), kit.Button{Variant: kit.Outline, Children: h.Text("Remove " + it),
			OnClick: func() {
				setItems.Update(func(s []string) []string {
					return slices.DeleteFunc(slices.Clone(s), func(x string) bool { return x == it })
				})
			}}))
	}
	return h.Div(h.Class("p-6"), kit.Dialog{
		Trigger:  kit.Button{Variant: kit.Outline, Children: h.Text("Edit list")},
		Title:    h.Text("Edit list"),
		Children: h.Div(rows...),
	})
}

func TestDialogKeepsFocusWhenFocusedElementUnmounts(t *testing.T) {
	app := telatest.MountFunc(t, listDialog, telatest.Theme(kit.Theme()))

	// By pointer: the removed button's focus goes to the panel.
	app.Click(listButton)
	app.ExpectFocused(removeA)
	app.Click(telatest.ByRole("button", "Remove B"))
	app.ExpectCount(telatest.ByRole("button", "Remove B"), 0)
	app.ExpectFocused(listDlg)
	// Tab goes on inside the dialog, from its start.
	app.Press("Tab")
	app.ExpectFocused(removeA)
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByRole("button", "Close"))

	// By keyboard: Enter on the focused button removes it; Escape then
	// still closes the dialog and the focus goes back to the trigger.
	app.Press("Shift+Tab") // Remove C
	app.ExpectFocused(telatest.ByRole("button", "Remove C"))
	app.Press("Enter")
	app.ExpectCount(telatest.ByRole("button", "Remove C"), 0)
	app.ExpectFocused(listDlg)
	app.Press("Escape")
	app.ExpectCount(listDlg, 0)
	app.ExpectFocused(listButton)
}
