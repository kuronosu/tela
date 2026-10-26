package kit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
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
	app := mountMenuRaw(t, contextPage(kit.ContextMenu[string]{}), 480, 320)
	app.rightClick(100, 80)
	if app.find("menu", "Actions") == nil {
		t.Fatalf("no menu:\n%s", app.snapshot())
	}
	app.rightClick(400, 280)
	if strings.Contains(app.snapshot(), "menu") {
		t.Errorf("a right click outside left the menu open:\n%s", app.snapshot())
	}
}

// A right click elsewhere on the area closes the menu and opens a new one
// at the new point (the panel is keyed by the opening, so the Presence
// does not bring the exiting one back): one menu, which has the focus.
func TestContextMenuRightClickElsewhereReopens(t *testing.T) {
	app := mountMenuRaw(t, contextPage(kit.ContextMenu[string]{}), 480, 320)
	app.rightClick(100, 80)
	app.rightClick(40, 140) // in the area, below its button, outside the menu
	if n := countMenus(app.tree.Semantics(), "Actions"); n != 1 {
		t.Fatalf("%d menus:\n%s", n, app.snapshot())
	}
	n := app.find("menu", "Actions")
	if n.Bounds.X0 != 40 || n.Bounds.Y0 != 140 {
		t.Errorf("menu at (%v, %v), want (40, 140)", n.Bounds.X0, n.Bounds.Y0)
	}
	if n.States&semantics.StateFocused == 0 {
		t.Errorf("the reopened menu does not have the focus:\n%s", app.snapshot())
	}
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
	app := mountMenuRaw(t, page, 480, 320)
	app.move(50, 50)
	app.event(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: 50, Y: 50}, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()})
	app.advance(800 * time.Millisecond)
	app.event(platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: 50, Y: 50}, Button: platform.ButtonPrimary})
	if clicks != 1 {
		t.Errorf("a slow click on the button clicked %d times, want 1", clicks)
	}
	if strings.Contains(app.snapshot(), "menu") {
		t.Errorf("a held mouse button opened the menu:\n%s", app.snapshot())
	}
}
