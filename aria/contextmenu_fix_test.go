package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
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
	app.win.Inject(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: 340, Y: 30}, Button: platform.ButtonMiddle, Buttons: platform.ButtonMiddle.Mask()})
	app.settle()
	app.win.Inject(platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: 340, Y: 30}, Button: platform.ButtonMiddle})
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
	app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
	app.rightClick(90, 60)
	// Move the focus to an item, as the keyboard would.
	app.key(platform.KeyPress, platform.KeyArrowDown, platform.CodeArrowDown, false)
	app.key(platform.KeyRelease, platform.KeyArrowDown, platform.CodeArrowDown, false)
	if f := focusedNode(app.tree.Semantics()); f == nil || f.Role != "menuitem" {
		t.Fatalf("ArrowDown did not focus an item:\n%s", app.tree.AriaSnapshot())
	}
	app.rightClick(40, 130) // on the area, below its button, outside the menu
	if n := countNodes(app.tree.Semantics(), "menu", "Actions"); n != 1 {
		t.Fatalf("%d menus after the second right click:\n%s", n, app.tree.AriaSnapshot())
	}
	if x, y, _, _ := menuBounds(t, app); x != 40 || y != 130 {
		t.Errorf("menu at (%v, %v), want (40, 130)", x, y)
	}
	if f := focusedNode(app.tree.Semantics()); f == nil || f.Role != "menu" {
		t.Errorf("the reopened menu does not have the focus itself:\n%s", app.tree.AriaSnapshot())
	}
	// The keys work in the new menu.
	app.key(platform.KeyPress, platform.KeyArrowDown, platform.CodeArrowDown, false)
	app.key(platform.KeyRelease, platform.KeyArrowDown, platform.CodeArrowDown, false)
	if n := findNode(app.tree.Semantics(), "menuitem", "Copy"); n == nil || n.States&semantics.StateFocused == 0 {
		t.Errorf("ArrowDown in the reopened menu:\n%s", app.tree.AriaSnapshot())
	}
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
	app := mountRaw(t, tela.C(twoAreas), 500, 300)
	app.rightClick(50, 50)
	if findNode(app.tree.Semantics(), "menu", "Left") == nil {
		t.Fatalf("no left menu:\n%s", app.tree.AriaSnapshot())
	}
	app.rightClick(250, 60)
	s := app.tree.Semantics()
	if findNode(s, "menu", "Left") != nil || findNode(s, "menu", "Right") == nil {
		t.Fatalf("after a right click in the other area:\n%s", app.tree.AriaSnapshot())
	}
	if f := focusedNode(s); f == nil || f.Name != "Right" {
		t.Errorf("the right menu does not have the focus:\n%s", app.tree.AriaSnapshot())
	}
	n := findNode(s, "menu", "Right")
	if n.Bounds.X0 != 250 || n.Bounds.Y0 != 60 {
		t.Errorf("right menu at (%v, %v), want (250, 60)", n.Bounds.X0, n.Bounds.Y0)
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
	if n := findNode(app.tree.Semantics(), "menuitem", "Copy"); n == nil || n.States&semantics.StateFocused == 0 {
		t.Errorf("the first item does not have the focus:\n%s", app.tree.AriaSnapshot())
	}
	app.up(platform.PointerPen, 50, 50)
	if got := ev.take(); got != "" {
		t.Errorf("the pen's long press clicked: %q", got)
	}
}
