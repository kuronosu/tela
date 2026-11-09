package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The room kept for the action follows its width when it is wider than
// shadcn's pe-18 holds (ADR-263).
func TestAlertActionRoomFollowsTheAction(t *testing.T) {
	two := tela.C(func() tela.Node {
		return h.Div(h.Class("flex gap-2"),
			kit.Button{Size: kit.Xs, Variant: kit.Outline, Children: h.Text("Discard"), Props: h.TestID("discard")},
			kit.Button{Size: kit.Xs, Children: h.Text("Save"), Props: h.TestID("save")})
	})
	const text = "You have edits that were not saved to the server yet, and a long text."
	app := telatest.Mount(t, h.Div(h.Class("w-96"), kit.Alert{Title: h.Text("Unsaved changes"),
		Description: h.Text(text), Props: h.TestID("alert"), Action: two}), telatest.Size(400, 200))
	root := app.Bounds(telatest.ByTestID("alert"))
	save, discard := app.Bounds(telatest.ByTestID("save")), app.Bounds(telatest.ByTestID("discard"))
	if got := root.X + root.Width - (save.X + save.Width); got != 13 {
		t.Errorf("the action is %v px from the end, want 13", got)
	}
	if w := save.X + save.Width - discard.X; w <= 60 {
		t.Fatalf("the action is %v px wide, the test needs more than 60", w)
	}
	// The description wraps before the action, with a gap.
	desc := app.Bounds(telatest.ByText(text))
	if right := desc.X + desc.Width; discard.X-right < 8 {
		t.Errorf("the description ends %v px before the action, want at least 8", discard.X-right)
	}

	// An action that fits keeps shadcn's pe-18 alone.
	small := telatest.Mount(t, h.Div(h.Class("w-96"), kit.Alert{Title: h.Text("x"),
		Action: kit.Button{Size: kit.Xs, Variant: kit.Outline, Children: h.Text("Restart")}}))
	small.ExpectCount(telatest.ByClass("pe-18"), 1)
	small.ExpectCount(telatest.ByClass("pe-[72px]"), 0)
}
