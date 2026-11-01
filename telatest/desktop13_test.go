package telatest_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The phase-13 helpers fail with the reason when what they act on is not
// there, and a popup menu is not a file dialog.
func TestDesktop13Failures(t *testing.T) {
	root := func() tela.Node {
		return Div(
			Button(OnClick(func() {
				_ = desktop.PopupMenu(tela.WindowHandle{}, tela.Point{}, []desktop.MenuItem{{Label: "Copy"}, {Label: "Off", Disabled: true}}, nil)
			}), Text("Menu")),
			tela.Component(desktop.GlobalShortcut{ID: "go", Keys: "Ctrl+G", Description: "Go"}),
		)
	}
	mustFail(t, func(tb testing.TB) { telatest.MountFunc(tb, root).PressShortcut("nope") }, `PressShortcut("nope"): no global shortcut has that ID`, `"go"`)
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, root)
		app.SetShortcutStatus("go", desktop.ShortcutRevoked, "")
		app.PressShortcut("go")
	}, "the shortcut is revoked, not bound")
	mustFail(t, func(tb testing.TB) { telatest.MountFunc(tb, root).ChoosePopupItem("Copy") }, "no native context menu is open")
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, root)
		app.Click(telatest.ByRole("button", "Menu"))
		app.ChoosePopupItem("Off")
	}, `"Off" is disabled`)
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, root)
		app.Click(telatest.ByRole("button", "Menu"))
		if n := len(app.FileDialogs()); n != 0 {
			tb.Errorf("FileDialogs lists %d dialogs for a popup menu", n)
		}
		app.AnswerFileDialog("/x")
	}, "no file dialog is open")
	mustFail(t, func(tb testing.TB) { telatest.MountFunc(tb, root).ChooseMenuItem("File", "New") }, "the window has no native menu bar")
	mustPass(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, root)
		app.Click(telatest.ByRole("button", "Menu"))
		app.DismissPopupMenu()
		if got := app.GlobalShortcuts(); len(got) != 1 || got[0].Keys != "Ctrl+G" {
			tb.Errorf("GlobalShortcuts = %+v", got)
		}
	})
}
