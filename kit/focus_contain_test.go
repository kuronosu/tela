package kit_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The focus containment of modal dialogs (ADR-160): a press on the scrim
// of an alert dialog leaves the focus inside, overlays opened from inside
// still take and give back the focus, and a focus change to an element
// outside that is still usable comes back.

var (
	cancelButton = telatest.ByRole("button", "Cancel")
	fruitButton  = telatest.ByRole("button", "Select a fruit Fruit")
)

// focusLog is a pair of focus handlers for a dialog's panel: focus events
// bubble, so it sees every focus change inside the panel.
type focusLog struct{ ev []string }

func (l *focusLog) props() tela.Prop {
	return tela.MergeProps(h.OnFocus(func() { l.ev = append(l.ev, "focus") }), h.OnBlur(func() { l.ev = append(l.ev, "blur") }))
}

func (l *focusLog) take() string {
	s := strings.Join(l.ev, " ")
	l.ev = l.ev[:0]
	return s
}

func TestAlertDialogScrimPressKeepsFocus(t *testing.T) {
	var log focusLog
	app := telatest.MountFunc(t, pageUnder(deleteDialog(kit.AlertDialog{Props: log.props()})), telatest.Theme(kit.Theme()))
	app.Click(deleteButton)
	app.ExpectFocused(cancelButton)
	if got := log.take(); got != "focus" {
		t.Errorf("events on open %q, want focus", got)
	}

	// The scrim (it hits the window root, outside the modal dialog): the
	// focus stays on Cancel, and nothing blurs or focuses.
	app.ClickAt(10, 10)
	app.ExpectCount(alertDlg, 1)
	app.ExpectFocused(cancelButton)
	if got := log.take(); got != "" {
		t.Errorf("events on the scrim press %q, want none", got)
	}

	// From Delete too, and the Tab order goes on from there.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Delete"))
	app.ClickAt(10, 10)
	app.ExpectFocused(telatest.ByRole("button", "Delete"))
	app.Press("Tab")
	app.ExpectFocused(cancelButton)
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByRole("button", "Delete"))
	if got := log.take(); got != "blur focus blur focus blur focus" {
		t.Errorf("events of the Tab presses %q, want three blur/focus pairs", got)
	}

	// Escape still closes it and gives the focus back.
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("alertdialog", ""), 0)
	app.ExpectFocused(deleteButton)
}

// A dismissable dialog still closes on a press on its scrim, and the focus
// goes back to the trigger.
func TestDialogScrimPressStillCloses(t *testing.T) {
	app := telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{})), telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.ExpectFocused(telatest.ByRole("textbox", "Name"))
	app.ClickAt(10, 10)
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	app.ExpectFocused(editButton)
}

// A Select and a Popover opened from inside an alert dialog are inside its
// focus scope: they take the focus and give it back, and the dialog does
// not fight them.
func TestOverlaysInsideAlertDialog(t *testing.T) {
	content := h.Div(h.Class("flex gap-2"),
		kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit", AriaLabel: "Fruit"},
		dimsPopover(kit.Popover{}))
	app := telatest.MountFunc(t, pageUnder(deleteDialog(kit.AlertDialog{Props: content})), telatest.Theme(kit.Theme()))
	app.Click(deleteButton)
	app.ExpectFocused(fruitButton) // the first tabbable element of the dialog

	// The select: its list takes the focus, Escape closes only the list.
	app.Click(fruitButton)
	app.ExpectFocused(telatest.ByRole("listbox", ""))
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectCount(alertDlg, 1)
	app.ExpectFocused(fruitButton)

	// Choosing with the pointer closes the list and focuses the trigger.
	app.Click(fruitButton)
	app.Click(telatest.ByRole("option", "Blueberry"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectFocused(telatest.ByRole("button", "Blueberry Fruit"))

	// A press on the scrim with the list open closes the list only.
	app.Click(telatest.ByRole("button", "Blueberry Fruit"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 1)
	app.ClickAt(10, 10)
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectCount(alertDlg, 1)
	app.ExpectFocused(telatest.ByRole("button", "Blueberry Fruit"))

	// The popover: its field takes the focus, Escape gives it back.
	app.Click(dimsButton)
	app.ExpectCount(dimsPanel, 1)
	app.ExpectFocused(telatest.ByRole("textbox", "Width"))
	app.Press("Escape")
	app.ExpectCount(dimsPanel, 0)
	app.ExpectCount(alertDlg, 1)
	app.ExpectFocused(dimsButton)
}

// An overlay mounted after the alert dialog (a notification) stays usable
// above it, but the focus does not go there: a press on its button and a
// programmatic focus leave it in the dialog, with no focus events.
func TestAlertDialogKeepsFocusFromOverlayAbove(t *testing.T) {
	var log focusLog
	var setNote tela.Setter[bool]
	undone := 0
	note := func() tela.Node {
		ov := aria.UseOverlay(aria.OverlayOptions{Open: true})
		return h.Div(ov.Props, h.Role("status"), h.Class("fixed bottom-4 left-4 p-2"),
			h.Button(h.OnClick(func() { undone++ }), h.Text("Undo")))
	}
	page := func() tela.Node {
		show, set := tela.UseState(false)
		setNote = set
		return h.Div(tela.C(pageUnder(deleteDialog(kit.AlertDialog{Props: log.props()}))),
			h.If(show, h.Portal(tela.C(note))))
	}
	undo := telatest.ByRole("button", "Undo")
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(deleteButton)
	app.ExpectFocused(cancelButton)
	setNote.Set(true)
	app.ExpectCount(undo, 1)
	log.take()

	app.Click(undo)
	if undone != 1 {
		t.Errorf("the press on Undo clicked it %d times, want 1", undone)
	}
	app.ExpectFocused(cancelButton)
	app.Focus(undo) // focus() from code
	app.ExpectFocused(cancelButton)
	if got := log.take(); got != "" {
		t.Errorf("events %q, want none: the focus never left", got)
	}
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Delete"))
}
