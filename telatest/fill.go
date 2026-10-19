package telatest

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/core"
)

// Fill fills the text field l locates (an h.Input or h.Textarea) with
// text, like Playwright's fill: after the actionability checks of the
// pointer actions, it focuses the field, selects its value and replaces it
// with text as a single insertText edit (an empty text deletes the value),
// firing OnBeforeInput and OnChange once. MaxLength applies. It fails the
// test when the element is not an editable text field.
//
//	app.Fill(telatest.ByRole("textbox", "Email"), "ana@example.com")
//
// Use [App.Type] to send the characters one key at a time instead.
func (a *App) Fill(l Locator, text string) {
	a.tb.Helper()
	n, _ := a.target(l, "fill", true)
	if err := a.tree.FillNode(n.ID, text); err != nil {
		switch {
		case errors.Is(err, core.ErrNotEditable):
			a.fatalf("cannot fill %s: it is not an editable text field (an Input or a Textarea that is not ReadOnly)", describeNode(n))
		case errors.Is(err, core.ErrNodeGone):
			a.fatalf("cannot fill %s: it disappeared", describeNode(n))
		}
		a.fatalf("cannot fill %s: %v", describeNode(n), err)
	}
	a.settleAfter()
}

// ExpectValue waits until the text field l locates holds value
// (Playwright's toHaveValue).
//
//	app.Type("hello")
//	app.ExpectValue(telatest.ByRole("textbox", "Message"), "hello")
func (a *App) ExpectValue(l Locator, value string) {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
			got, ok := a.tree.FieldValue(nodes[0].ID)
			if !ok {
				a.fatalf("ExpectValue: %s is not a text field (an Input or a Textarea)", describeNode(nodes[0]))
			}
			if got == value {
				return true, ""
			}
			return false, fmt.Sprintf("expected %s to have the value %q, got %q", describeNode(nodes[0]), value, got)
		}
		a.fatalf("%s", a.strictMessage(l, nodes))
		return false, ""
	})
}
