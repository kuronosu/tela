package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// countNodes counts the nodes of the semantics tree with the role and name.
func countNodes(n *semantics.Node, role, name string) int {
	c := 0
	if n.Role == role && n.Name == name {
		c++
	}
	for _, k := range n.Children {
		c += countNodes(k, role, name)
	}
	return c
}

func focusedNode(n *semantics.Node) *semantics.Node {
	if n.States&semantics.StateFocused != 0 {
		return n
	}
	for _, k := range n.Children {
		if f := focusedNode(k); f != nil {
			return f
		}
	}
	return nil
}

// A right click outside the open menu and outside every area closes it,
// like a primary press (Radix's DismissableLayer: any button).
func TestContextMenuRightClickOutsideCloses(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
	app.rightClick(90, 60)
	if !hasMenu(app) {
		t.Fatal("no menu")
	}
	app.rightClick(350, 250)
	if hasMenu(app) {
		t.Errorf("a right click outside left the menu open:\n%s", app.tree.AriaSnapshot())
	}
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
	// So does a middle click, which reaches nothing under it.
	app.rightClick(90, 60)
	rawwin.Pointer(app.win, platform.PointerMouse, platform.PointerDown, 340, 30, platform.ButtonMiddle, true)
	app.settle()
	rawwin.Pointer(app.win, platform.PointerMouse, platform.PointerUp, 340, 30, platform.ButtonMiddle, false)
	app.settle()
	if hasMenu(app) {
		t.Errorf("a middle click outside left the menu open")
	}
}

// A right click elsewhere on the area while the menu is open closes it
// and opens one menu at the new point, which takes the focus as a menu
// opened with the mouse does (the menu itself, no item).
func TestContextMenuRightClickElsewhereReopens(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.ClickAt(90, 60, telatest.Right())
	// Move the focus to an item, as the keyboard would.
	app.Press("ArrowDown")
	app.Expect(telatest.ByRole("menuitem", "Copy"), telatest.Focused(true))
	// On the area, below its button, outside the menu.
	app.ClickAt(40, 130, telatest.Right())
	app.ExpectCount(ctxMenuLoc, 1)
	if m := app.Bounds(ctxMenuLoc); m.X != 40 || m.Y != 130 {
		t.Errorf("menu at (%v, %v), want (40, 130)", m.X, m.Y)
	}
	app.Expect(ctxMenuLoc, telatest.Focused(true)) // the reopened menu has the focus itself
	// The keys work in the new menu.
	app.Press("ArrowDown")
	app.Expect(telatest.ByRole("menuitem", "Copy"), telatest.Focused(true))
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

// twoAreas is two side-by-side areas with a context menu each.
func twoAreas() tela.Node {
	area := func(name, cls string) tela.Node {
		return tela.C(func() tela.Node {
			cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
			return Div(cm.TriggerProps, Class("absolute top-0 h-40 w-40 "+cls),
				Portal(If(cm.Open, tela.C(func() tela.Node {
					pop := aria.UsePopover(cm.PopoverOptions())
					return Div(pop.Props, cm.MenuProps, Role("menu"), AriaLabel(name), TabIndex(-1),
						AutoFocus(true), Class("h-10 w-20"), Text(name))
				}))))
		})
	}
	return Div(Class("relative h-full"), area("Left", "left-0"), area("Right", "left-50"))
}

// A right click in another area closes the open menu and opens that
// area's menu: exactly one menu, at the new point, with the focus.
func TestContextMenuRightClickInAnotherArea(t *testing.T) {
	app := telatest.Mount(t, tela.C(twoAreas), telatest.Size(500, 300))
	left, right := telatest.ByRole("menu", "Left"), telatest.ByRole("menu", "Right")
	app.ClickAt(50, 50, telatest.Right())
	app.ExpectCount(left, 1)
	app.ClickAt(250, 60, telatest.Right())
	if app.Count(left) != 0 || app.Count(right) != 1 {
		t.Fatalf("after a right click in the other area:\n%s", app.AriaSnapshot())
	}
	app.Expect(right, telatest.Focused(true))
	if m := app.Bounds(right); m.X != 250 || m.Y != 60 {
		t.Errorf("right menu at (%v, %v), want (250, 60)", m.X, m.Y)
	}
}

// A mouse button held on the area's button for longer than a long press
// still clicks it: the area's long press is for touch and pens only, and
// a mouse press does not even take part in its gesture.
func TestContextMenuSlowMouseClick(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
	app.down(platform.PointerMouse, 50, 50)
	app.advance(800 * time.Millisecond)
	app.up(platform.PointerMouse, 50, 50)
	if got := ev.take(); got != "inner" {
		t.Errorf("slow click on the button: events %q, want inner", got)
	}
	if hasMenu(app) {
		t.Errorf("a held mouse button opened the menu")
	}
	if _, ok := app.host.TimerArmed(); ok {
		t.Errorf("a mouse press on the area armed the long-press timer")
	}
}

// A pen that stays down opens the menu like a touch (Radix's
// whenTouchOrPen), on its first item, and does not click.
func TestContextMenuLongPressPen(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
	app.down(platform.PointerPen, 50, 50)
	app.advance(600 * time.Millisecond)
	if x, y, _, _ := menuBounds(t, app); x != 50 || y != 50 {
		t.Errorf("menu at (%v, %v), want (50, 50)", x, y)
	}
	if n := rawwin.FindNode(app.tree.Semantics(), "menuitem", "Copy"); n == nil || n.States&semantics.StateFocused == 0 {
		t.Errorf("the first item does not have the focus:\n%s", app.tree.AriaSnapshot())
	}
	app.up(platform.PointerPen, 50, 50)
	if got := ev.take(); got != "" {
		t.Errorf("the pen's long press clicked: %q", got)
	}
}
