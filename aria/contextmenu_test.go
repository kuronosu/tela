package aria_test

import (
	"fmt"
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

var ctxActions = []string{"copy", "paste", "delete"}

// ctxMenu is the menu of a context menu: a component of its own, rendered
// only while the menu is open.
type ctxMenu struct {
	cm aria.ContextMenuTriggerResult
	ev *events
}

func (m ctxMenu) Build() tela.Node {
	pop := aria.UsePopover(m.cm.PopoverOptions())
	menu := aria.UseMenu(aria.MenuOptions[string]{
		Keys: ctxActions, TextValue: capital, AriaLabel: "Actions",
		OnAction: func(k string) { m.ev.add("action:" + k) },
		OnClose:  m.cm.Close, AutoFocus: true, FocusStrategy: m.cm.FocusStrategy, Ref: m.cm.MenuRef,
	})
	return Div(pop.Props, m.cm.MenuProps, menu.Props, TestID("menu"), Class("w-40 bg-white"),
		Range(len(ctxActions), func(i int) tela.Node {
			k := ctxActions[i]
			return Div(menu.ItemProps(k), Key(k), Class("h-8 px-2 focus-visible:bg-sky-100"), Text(capital(k)))
		}))
}

// ctxPage is an area with a button inside and a context menu on it.
func ctxPage(o aria.ContextMenuTriggerOptions, ev *events) func() tela.Node {
	return func() tela.Node {
		cm := aria.UseContextMenuTrigger(o)
		return Div(Class("relative h-full"),
			Div(cm.TriggerProps, TestID("area"), Class("absolute left-5 top-5 h-[120px] w-[200px]"),
				Button(TestID("inner"), OnClick(func() { ev.add("inner") }), Class("mt-4 ml-4 h-8 w-20"), Text("Inner"))),
			Button(TestID("outside"), Class("absolute left-[300px] top-5 h-8 w-20"), Text("Outside")),
			Portal(If(cm.Open, ctxMenu{cm, ev})))
	}
}

func menuBounds(t *testing.T, a *rawApp) (x, y, w, h float32) {
	t.Helper()
	n := rawwin.FindNode(a.tree.Semantics(), "menu", "Actions")
	if n == nil {
		t.Fatalf("no menu:\n%s", a.tree.AriaSnapshot())
	}
	return n.Bounds.X0, n.Bounds.Y0, n.Bounds.X1 - n.Bounds.X0, n.Bounds.Y1 - n.Bounds.Y0
}

func hasMenu(a *rawApp) bool { return rawwin.FindNode(a.tree.Semantics(), "menu", "Actions") != nil }

var ctxMenuLoc = telatest.ByRole("menu", "Actions")

func TestContextMenuRightClickOpensAtThePointer(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.ExpectCount(ctxMenuLoc, 0)
	app.ClickAt(90, 60, telatest.Right())
	if m := app.Bounds(ctxMenuLoc); m.X != 90 || m.Y != 60 || m.Width != 160 || m.Height != 96 {
		t.Errorf("menu at (%v, %v) size %vx%v, want (90, 60) 160x96", m.X, m.Y, m.Width, m.Height)
	}
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
	// A mouse leaves the focus to the menu itself.
	app.Expect(ctxMenuLoc, telatest.Focused(true))
	// Right clicking elsewhere on the area moves the menu there.
	app.ClickAt(40, 40, telatest.Right())
	if m := app.Bounds(ctxMenuLoc); m.X != 40 || m.Y != 40 {
		t.Errorf("after a second right click, menu at (%v, %v), want (40, 40)", m.X, m.Y)
	}
	// A primary press outside closes it.
	app.ClickAt(350, 200)
	app.ExpectCount(ctxMenuLoc, 0)
}

// The menu flips and shifts to stay in the window.
func TestContextMenuFlipsAndShifts(t *testing.T) {
	var ev events
	page := func() tela.Node {
		cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
		return Div(cm.TriggerProps, Class("h-full w-full"),
			Portal(If(cm.Open, ctxMenu{cm, &ev})))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.ClickAt(380, 280, telatest.Right())
	m := app.Bounds(ctxMenuLoc)
	if m.X+m.Width > 400 || m.Y+m.Height > 300 || m.X < 0 || m.Y < 0 {
		t.Errorf("menu at (%v, %v) size %vx%v is out of the window", m.X, m.Y, m.Width, m.Height)
	}
	if m.Y+m.Height > 280 {
		t.Errorf("menu at y %v did not flip above the pointer", m.Y)
	}
}

// This one stays a hand-made sequence: it checks that the menu is not open
// 300 ms into the press, which telatest.App.LongPress (down, hold, up) does
// not express. LongPress itself is checked by TestContextMenuLongPressAction.
func TestContextMenuLongPressOnTouch(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
	// The button inside the area, held with a finger.
	app.down(platform.PointerTouch, 50, 50)
	app.advance(300 * time.Millisecond)
	if hasMenu(app) {
		t.Errorf("the menu opened before the long press")
	}
	app.advance(300 * time.Millisecond)
	x, y, _, _ := menuBounds(t, app)
	if x != 50 || y != 50 {
		t.Errorf("menu at (%v, %v), want (50, 50)", x, y)
	}
	// The first item has the focus.
	if n := rawwin.FindNode(app.tree.Semantics(), "menuitem", "Copy"); n == nil || n.States&semantics.StateFocused == 0 {
		t.Errorf("the first item does not have the focus:\n%s", app.tree.AriaSnapshot())
	}
	app.up(platform.PointerTouch, 50, 50)
	if !hasMenu(app) {
		t.Errorf("the release closed the menu")
	}
	if got := ev.take(); got != "" {
		t.Errorf("the long press clicked: %q", got)
	}
}

// A long press of a finger on the button inside the area opens the menu
// under the finger, on its first item, and does not click.
func TestContextMenuLongPressAction(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	inner := telatest.ByTestID("inner")
	app.LongPress(inner)
	b, m := app.Bounds(inner), app.Bounds(ctxMenuLoc)
	if cx, cy := b.X+b.Width/2, b.Y+b.Height/2; m.X != cx || m.Y != cy {
		t.Errorf("menu at (%v, %v), want the finger's (%v, %v)", m.X, m.Y, cx, cy)
	}
	app.ExpectFocused(telatest.ByRole("menuitem", "Copy"))
	if got := ev.take(); got != "" {
		t.Errorf("the long press clicked: %q", got)
	}
}

// A quick tap is the button's own, and a held mouse button opens nothing.
func TestContextMenuTapAndMouseHold(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.Tap(telatest.ByTestID("inner"))
	if got := ev.take(); got != "inner" || app.Count(ctxMenuLoc) != 0 {
		t.Errorf("tap: events %q, menus %d", got, app.Count(ctxMenuLoc))
	}
	app.ClickAt(200, 100, telatest.Delay(800*time.Millisecond))
	app.ExpectCount(ctxMenuLoc, 0)
}

func TestContextMenuNoLongPress(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{NoLongPress: true}, &ev), telatest.Size(400, 300))
	app.LongPress(telatest.ByTestID("inner"))
	if n := app.Count(ctxMenuLoc); n != 0 {
		t.Errorf("NoLongPress: the long press opened the menu")
	}
	if got := ev.take(); got != "inner" {
		t.Errorf("NoLongPress: the held touch should click, events %q", got)
	}
}

func TestContextMenuKeyboard(t *testing.T) {
	for _, key := range []string{"Shift+F10", "ContextMenu"} {
		t.Run(key, func(t *testing.T) {
			var ev events
			app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
			inner := telatest.ByTestID("inner")
			app.Focus(inner)
			app.Press(key)
			app.ExpectFocused(telatest.ByRole("menuitem", "Copy"))
			r := app.Bounds(telatest.ByTestID("menu"))
			a := app.Bounds(telatest.ByTestID("area"))
			if r.X != a.X || r.Y != a.Y {
				t.Errorf("menu at (%v, %v), want the area's corner (%v, %v)", r.X, r.Y, a.X, a.Y)
			}
			app.Press("ArrowDown")
			app.ExpectFocused(telatest.ByRole("menuitem", "Paste"))
			app.Press("Escape")
			app.ExpectNoText("Paste")
			app.ExpectFocused(inner) // the focus returns
			app.Press(key)
			app.Press("Enter")
			if got := ev.take(); got != "action:copy" {
				t.Errorf("events %q", got)
			}
			app.ExpectNoText("Paste")
			app.ExpectFocused(inner)
		})
	}
}

func TestContextMenuOutsidePressAndChoose(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.ClickAt(90, 60, telatest.Right())
	app.ClickAt(350, 200)
	app.ExpectCount(ctxMenuLoc, 0)
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
	// Choosing an item runs the action and closes the menu.
	app.ClickAt(90, 60, telatest.Right())
	app.Click(telatest.ByRole("menuitem", "Paste"))
	if got := ev.take(); got != "action:paste" || app.Count(ctxMenuLoc) != 0 {
		t.Errorf("choosing: events %q, menus %d", got, app.Count(ctxMenuLoc))
	}
}

func TestContextMenuDisabled(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{Disabled: true}, &ev), telatest.Size(400, 300))
	app.ClickAt(90, 60, telatest.Right())
	if n := app.Count(ctxMenuLoc); n != 0 {
		t.Errorf("a disabled area opened its menu")
	}
}

// innerArea and outerArea are two nested areas with a context menu each.
func innerArea() tela.Node {
	cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
	return Div(cm.TriggerProps, Class("absolute left-10 top-10 h-20 w-20"),
		Portal(If(cm.Open, Div(cm.MenuProps, Role("menu"), AriaLabel("Inner"), Text("inner menu")))))
}

func outerArea() tela.Node {
	cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
	return Div(cm.TriggerProps, Class("relative h-full w-full"), tela.C(innerArea),
		Portal(If(cm.Open, Div(cm.MenuProps, Role("menu"), AriaLabel("Outer"), Text("outer menu")))))
}

// The innermost area with a context menu wins.
func TestContextMenuNested(t *testing.T) {
	app := telatest.Mount(t, tela.C(outerArea), telatest.Size(400, 300))
	inner, outer := telatest.ByRole("menu", "Inner"), telatest.ByRole("menu", "Outer")
	app.ClickAt(60, 60, telatest.Right())
	if app.Count(inner) != 1 || app.Count(outer) != 0 {
		t.Errorf("right click in the inner area:\n%s", app.AriaSnapshot())
	}
	app.ClickAt(350, 250)
	app.ClickAt(300, 200, telatest.Right())
	if app.Count(outer) != 1 {
		t.Errorf("right click in the outer area:\n%s", app.AriaSnapshot())
	}
}

func TestContextMenuControlled(t *testing.T) {
	var ev events
	var log []bool
	open := false
	page := ctxPage(aria.ContextMenuTriggerOptions{Open: &open, OnOpenChange: func(v bool) { log = append(log, v) }}, &ev)
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.ClickAt(90, 60, telatest.Right())
	if n := app.Count(ctxMenuLoc); n != 0 {
		t.Errorf("a controlled menu that the caller keeps closed opened")
	}
	if len(log) != 1 || !log[0] {
		t.Errorf("OnOpenChange %v, want [true]", log)
	}
}

func TestContextMenuAriaSnapshot(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.Focus(telatest.ByTestID("inner"))
	app.Press("ContextMenu")
	app.ExpectAria(`
		- button "Inner"
		- button "Outside"
		- menu "Actions":
		  - menuitem "Copy"
		  - menuitem "Paste"
		  - menuitem "Delete"
	`)
}

// A settled open menu costs no frames.
func TestContextMenuIdle(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.Focus(telatest.ByTestID("inner"))
	app.Press("Shift+F10")
	app.ExpectText("Paste")
	before := app.Stats()
	app.Advance(2 * time.Second)
	if after := app.Stats(); after.Frames != before.Frames {
		t.Errorf("an open context menu ran %d frames", after.Frames-before.Frames)
	}
}

func ExampleUseContextMenuTrigger() {
	// A context menu: Shift+F10 on the focused button opens it, Enter
	// chooses the first item and the focus goes back to the button.
	var ev events
	app := telatest.MountFunc(&exampleTB{}, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	app.Focus(telatest.ByTestID("inner"))
	app.Press("Shift+F10")
	fmt.Println(app.AriaSnapshot())
	app.Press("Enter")
	app.ExpectFocused(telatest.ByTestID("inner"))
	fmt.Println(ev.take())
	// Output:
	// - button "Inner"
	// - button "Outside"
	// - menu "Actions":
	//   - menuitem "Copy"
	//   - menuitem "Paste"
	//   - menuitem "Delete"
	// action:copy
}
