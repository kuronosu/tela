package kit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func countMenus(n *semantics.Node, name string) int {
	c := 0
	if n.Role == "menu" && n.Name == name {
		c++
	}
	for _, k := range n.Children {
		c += countMenus(k, name)
	}
	return c
}

// A right click outside the open menu (and outside the area) closes it,
// as a primary press does: Radix's DismissableLayer counts any button.
func TestContextMenuRightClickOutsideCloses(t *testing.T) {
	app := mountContext(t, contextPage(kit.ContextMenu[string]{}), 480, 320)
	app.ClickAt(100, 80, telatest.Right())
	app.ExpectCount(cmMenu, 1)
	app.ClickAt(400, 280, telatest.Right())
	if strings.Contains(app.AriaSnapshot(), "menu") {
		t.Errorf("a right click outside left the menu open:\n%s", app.AriaSnapshot())
	}
}

// A right click elsewhere on the area closes the menu and opens a new one
// at the new point (the panel is keyed by the opening, so the Presence
// does not bring the exiting one back): one menu, which has the focus.
func TestContextMenuRightClickElsewhereReopens(t *testing.T) {
	app := mountContext(t, contextPage(kit.ContextMenu[string]{}), 480, 320)
	app.ClickAt(100, 80, telatest.Right())
	app.ClickAt(40, 140, telatest.Right()) // in the area, below its button, outside the menu
	app.ExpectCount(cmMenu, 1)
	if m := app.Bounds(cmMenu); m.X != 40 || m.Y != 140 {
		t.Errorf("menu at (%v, %v), want (40, 140)", m.X, m.Y)
	}
	app.Expect(cmMenu, telatest.Focused(true))
}

// A mouse button held on a button inside the area for longer than a long
// press still clicks it: the area's long press is for touch and pens.
func TestContextMenuSlowMouseClick(t *testing.T) {
	clicks := 0
	page := func() tela.Node {
		return h.Div(h.Class("relative h-full w-full"),
			kit.ContextMenu[string]{AriaLabel: "Actions", Class: "absolute top-5 left-5 h-40 w-60",
				Items:    []kit.MenuItem[string]{{Key: "copy", Label: "Copy"}},
				Children: h.Button(h.Class("m-4 h-8 w-20"), h.OnClick(func() { clicks++ }), h.Text("Inner"))})
	}
	app := mountContext(t, page, 480, 320)
	app.Click(telatest.ByRole("button", "Inner"), telatest.Delay(800*time.Millisecond))
	if clicks != 1 {
		t.Errorf("a slow click on the button clicked %d times, want 1", clicks)
	}
	if strings.Contains(app.AriaSnapshot(), "menu") {
		t.Errorf("a held mouse button opened the menu:\n%s", app.AriaSnapshot())
	}
}
