package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Static text selection and the aria hooks (ADR-271).

// A press on a pressable element neither starts a selection nor changes
// the one there is, as React Aria's usePress; AllowTextSelectionOnPress
// lets it select.
func TestPressNoTextSelection(t *testing.T) {
	page := func() tela.Node {
		press := aria.UsePress(aria.PressOptions{})
		allow := aria.UsePress(aria.PressOptions{AllowTextSelectionOnPress: true})
		return Div(Class("p-4 w-80 flex flex-col gap-4"),
			P(TestID("text"), Class("h-6"), Text("Plain text to select")),
			Div(press.Props, TestID("press"), Class("h-6"), Text("Pressable label")),
			Div(allow.Props, TestID("allow"), Class("h-6"), Text("Selectable label")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 200))
	row := func(id string) (x0, x1, y float32) {
		b := app.Bounds(telatest.ByTestID(id))
		return b.X + 1, b.X + 60, b.Y + b.Height/2
	}
	x0, x1, y := row("text")
	app.DragFromTo(tela.Point{X: x0, Y: y}, tela.Point{X: x1, Y: y})
	sel := app.SelectedText()
	if sel == "" {
		t.Fatal("a drag on text selected nothing")
	}
	x0, x1, y = row("press")
	app.DragFromTo(tela.Point{X: x0, Y: y}, tela.Point{X: x1, Y: y})
	if got := app.SelectedText(); got != sel {
		t.Errorf("a drag on a pressable changed the selection to %q, want %q", got, sel)
	}
	x0, x1, y = row("allow")
	app.DragFromTo(tela.Point{X: x0, Y: y}, tela.Point{X: x1, Y: y})
	if got := app.SelectedText(); got == sel || got == "" {
		t.Errorf("AllowTextSelectionOnPress: the drag selected %q", got)
	}
}

// The context menu keys open an area's menu at a selection of its static
// text, with the first item focused: from the area itself when nothing is
// focused and the area is the content, and from core's context menu event
// when the key reaches no area.
func TestContextMenuKeysAtTextSelection(t *testing.T) {
	for _, wrap := range []bool{true, false} {
		var ev events
		page := func() tela.Node {
			cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
			area := Div(cm.TriggerProps, TestID("area"), Class("h-[120px] w-[260px] p-4"),
				P(TestID("text"), Text("Select some words here")))
			menu := Portal(If(cm.Open, ctxMenu{cm, &ev}))
			if wrap {
				return Div(Class("h-full p-5"), area, menu)
			}
			// The area is not the outermost element: a key with nothing
			// focused reaches the outer one only.
			return Div(Class("h-full p-5 flex flex-col gap-2"), P(Text("Above")), area, menu)
		}
		app := telatest.MountFunc(t, page, telatest.Size(400, 300))
		b := app.Bounds(telatest.ByTestID("text"))
		y := b.Y + b.Height/2
		app.DragFromTo(tela.Point{X: b.X + 1, Y: y}, tela.Point{X: b.X + 60, Y: y})
		if app.SelectedText() == "" {
			t.Fatalf("wrap %v: nothing selected", wrap)
		}
		app.Press("ContextMenu")
		app.ExpectCount(ctxMenuLoc, 1)
		m := app.Bounds(ctxMenuLoc)
		if m.X < b.X-1 || m.X > b.X+2 || m.Y < y-1 || m.Y > y+1 {
			t.Errorf("wrap %v: menu at (%v, %v), want the selection's start (%v, %v)", wrap, m.X, m.Y, b.X, y)
		}
		app.Expect(telatest.ByRole("menuitem", "Copy"), telatest.Focused(true))
	}
}
