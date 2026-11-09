package telatest

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
)

// A kit menu item's shortcut is shown for the window's key conventions
// (PlatformKeys, not the OS the test runs on) when it names CmdOrCtrl, and
// every shortcut, glyphs included, gives assistive technologies its keys
// (aria-keyshortcuts). Internal to telatest: the shown text is hidden from
// the snapshot and the keys are not printed in it.
func TestMenuShortcutsFollowTheFlavor(t *testing.T) {
	items := []kit.MenuItem[string]{
		{Key: "copy", Role: desktop.RoleCopy},
		{Key: "find", Label: "Find", Shortcut: "CmdOrCtrl+F"},
		{Key: "redo", Label: "Redo", Shortcut: "⇧⌘Z"},
		{Key: "tab", Label: "Reopen Tab", Shortcut: "Ctrl+Shift+T"},
		{Key: "plain", Label: "Plain", Shortcut: "Soon"},
	}
	for _, c := range []struct {
		goos              string
		copyKeys, copyTxt string
		findKeys, findTxt string
	}{
		{"darwin", "Meta+C", "⌘C", "Meta+F", "⌘F"},
		{"linux", "Control+C", "Ctrl+C", "Control+F", "Ctrl+F"},
		{"windows", "Control+C", "Ctrl+C", "Control+F", "Ctrl+F"},
	} {
		t.Run(c.goos, func(t *testing.T) {
			a := MountFunc(t, func() tela.Node {
				return h.Div(h.Class("p-6"), kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items})
			}, Size(480, 400), Theme(kit.Theme()), PlatformKeys(c.goos))
			a.Click(ByRole("button", "Open"))
			root := a.tree.Semantics()
			for _, w := range []struct{ name, keys string }{
				{"Copy", c.copyKeys}, {"Find", c.findKeys},
				{"Redo", "Shift+Meta+Z"}, {"Reopen Tab", "Control+Shift+T"}, {"Plain", ""},
			} {
				if n := nodeOf(t, root, "menuitem", w.name); n.KeyShortcuts != w.keys {
					t.Errorf("%s: KeyShortcuts %q, want %q", w.name, n.KeyShortcuts, w.keys)
				}
			}
			shown := map[string]bool{}
			root.Walk(func(n *semantics.Node) bool {
				if n.Kind == semantics.KindText {
					shown[n.Text] = true
				}
				return true
			})
			for _, txt := range []string{c.copyTxt, c.findTxt, "⇧⌘Z", "Ctrl+Shift+T", "Soon"} {
				if !shown[txt] {
					t.Errorf("the shortcut %q is not shown; texts %v", txt, shown)
				}
			}
		})
	}
}
