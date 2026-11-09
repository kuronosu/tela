package telatest_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// A click on the fallback title bar (ADR-238) is a press outside the open
// dialog, which closes as for a press on its scrim; a drag on the bar
// moves the window and leaves it open (ADR-252).
func TestFallbackTitleBarClosesDialog(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		open, setOpen := tela.UseState(true)
		return Div(Class("h-full p-4"),
			Text("Notes"),
			kit.Dialog{
				Open: &open, OnOpenChange: setOpen.Set,
				Title:    Text("Rename"),
				Children: Text("A dialog over the window."),
			},
		)
	}, telatest.Size(480, 320), telatest.Screen(1000, 700), telatest.NoServerDecorations(), telatest.Title("Notes"), telatest.Theme(kit.Theme()))
	dialog := telatest.ByRole("dialog", "Rename")
	app.ExpectCount(dialog, 1)

	// A drag on the bar's drag area moves the window: the dialog stays.
	app.DragFromTo(tela.Point{X: 120, Y: 18}, tela.Point{X: 200, Y: 60})
	if app.CaptionDrags() != 1 {
		t.Errorf("%d caption drags, want 1", app.CaptionDrags())
	}
	app.ExpectCount(dialog, 1)

	// A click on it closes the dialog.
	app.Advance(time.Second) // not a double click with the drag's press
	app.ClickAt(120, 18)
	app.ExpectCount(dialog, 0)
	if app.CaptionDrags() != 2 {
		t.Errorf("%d caption drags, want 2 (the click is the OS's too)", app.CaptionDrags())
	}
	app.ExpectWindowState(telatest.Maximized(false))
}
