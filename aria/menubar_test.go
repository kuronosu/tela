package aria_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

var (
	barKeys  = []string{"file", "edit", "view"}
	barItems = map[string][]string{
		"file": {"new", "open", "save"},
		"edit": {"undo", "redo"},
		"view": {"zoom", "fullscreen"},
	}
)

// barMenu is the menu that is open, a component of its own with the open
// key as its key.
type barMenu struct {
	bar aria.MenuBarResult[string]
	ev  *events
}

func (m barMenu) Build() tela.Node {
	k := m.bar.OpenKey
	items := barItems[k]
	pop := aria.UsePopover(m.bar.PopoverOptions(k))
	menu := aria.UseMenu(aria.MenuOptions[string]{
		Keys: items, TextValue: capital,
		OnAction: func(i string) { m.ev.add("action:" + i) },
		OnClose:  m.bar.Close, AutoFocus: true, FocusStrategy: m.bar.FocusStrategy, Ref: m.bar.MenuRef(k),
	})
	return Div(pop.Props, m.bar.MenuProps(k), menu.Props, Class("w-40 bg-white"),
		Range(len(items), func(i int) tela.Node {
			return Div(menu.ItemProps(items[i]), Key(items[i]), Class("h-8 px-2 focus-visible:bg-sky-100"), Text(capital(items[i])))
		}))
}

// barPage is a menu bar with three menus and a button after it.
func barPage(o aria.MenuBarOptions[string], ev *events) func() tela.Node {
	return barPageWith(func() aria.MenuBarOptions[string] { return o }, ev)
}

// barPageWith is barPage with options made on every build, for a bar that
// reads state.
func barPageWith(opts func() aria.MenuBarOptions[string], ev *events) func() tela.Node {
	return func() tela.Node {
		o := opts()
		o.Keys = barKeys
		o.AriaLabel = "Application"
		bar := aria.UseMenuBar(o)
		return Div(Class("h-full"),
			Button(TestID("before"), Text("Before")),
			Div(bar.Props, Class("flex h-8 gap-1"),
				Range(len(barKeys), func(i int) tela.Node {
					k := barKeys[i]
					return Button(bar.TriggerProps(k), Key(k), Class("h-8 w-20"), Text(capital(k)))
				})),
			Button(TestID("after"), Text("After")),
			Portal(If(bar.Open, tela.WithKey(barMenu{bar, ev}, bar.OpenKey))))
	}
}

func trigger(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

func mountBar(t *testing.T, o aria.MenuBarOptions[string], ev *events) *telatest.App {
	return telatest.MountFunc(t, barPage(o, ev), telatest.Size(400, 300))
}

func TestMenuBarRovingFocus(t *testing.T) {
	var ev events
	o := aria.MenuBarOptions[string]{DisabledKeys: []string{"edit"}}
	app := mountBar(t, o, &ev)
	app.Focus(trigger("File"))
	app.Press("ArrowRight") // edit is disabled
	app.ExpectFocused(trigger("View"))
	app.Press("ArrowRight") // wraps
	app.ExpectFocused(trigger("File"))
	app.Press("ArrowLeft")
	app.ExpectFocused(trigger("View"))
	app.Press("Home")
	app.ExpectFocused(trigger("File"))
	app.Press("End")
	app.ExpectFocused(trigger("View"))
	app.ExpectNoText("Undo")
	// The bar is one tab stop, on the trigger that had the focus last.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("after"))
	app.Press("Shift+Tab")
	app.ExpectFocused(trigger("View"))
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByTestID("before"))
	app.Press("Tab")
	app.ExpectFocused(trigger("View"))
}

func TestMenuBarNoWrap(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{NoWrap: true}, &ev)
	app.Focus(trigger("File"))
	app.Press("ArrowLeft")
	app.ExpectFocused(trigger("File"))
	app.Press("End")
	app.Press("ArrowRight")
	app.ExpectFocused(trigger("View"))
}

func TestMenuBarRTL(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{Direction: "rtl"}, &ev)
	app.Focus(trigger("File"))
	app.Press("ArrowLeft")
	app.ExpectFocused(trigger("Edit"))
	app.Press("ArrowRight")
	app.ExpectFocused(trigger("File"))
	// An open menu opens its neighbor in the same direction.
	app.Press("ArrowDown")
	app.Press("ArrowLeft")
	app.ExpectFocused(menuitem("Undo"))
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("New"))
}

func TestMenuBarOpenWithKeys(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	for _, tc := range []struct{ key, focused string }{
		{"ArrowDown", "New"},
		{"ArrowUp", "Save"},
		{"Enter", "New"},
		{"Space", "New"},
	} {
		app.Focus(trigger("File"))
		app.Press(tc.key)
		app.ExpectFocused(menuitem(tc.focused))
		app.Expect(trigger("File"), telatest.Expanded(true))
		app.Press("Escape")
		app.ExpectNoText("Open")
		app.ExpectFocused(trigger("File")) // the focus returns to the trigger
		app.Expect(trigger("File"), telatest.Expanded(false))
	}
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestMenuBarArrowsMoveBetweenOpenMenus(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	app.Focus(trigger("File"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Open"))
	// Right closes File and opens Edit on its first item.
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Undo"))
	app.Expect(trigger("Edit"), telatest.Expanded(true))
	app.Expect(trigger("File"), telatest.Expanded(false))
	app.ExpectNoText("Save")
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Zoom"))
	app.Press("ArrowRight") // wraps to File
	app.ExpectFocused(menuitem("New"))
	app.Press("ArrowLeft")
	app.ExpectFocused(menuitem("Zoom"))
	app.Press("ArrowLeft")
	app.ExpectFocused(menuitem("Undo"))
	// The roving focus follows: Escape lands on the open menu's trigger.
	app.Press("Escape")
	app.ExpectFocused(trigger("Edit"))
	app.ExpectNoText("Undo")
	app.Press("ArrowRight")
	app.ExpectFocused(trigger("View"))
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestMenuBarKeyboardActionRestoresFocus(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	app.Focus(trigger("Edit"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("Enter")
	if got := ev.take(); got != "action:redo" {
		t.Errorf("events %q", got)
	}
	app.ExpectNoText("Undo")
	app.ExpectFocused(trigger("Edit"))
}

func TestMenuBarTabLeavesTheBar(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	app.Focus(trigger("Edit"))
	app.Press("ArrowDown")
	app.Press("Tab")
	app.ExpectNoText("Undo")
	app.ExpectFocused(telatest.ByTestID("after"))
	app.Focus(trigger("Edit"))
	app.Press("ArrowDown")
	app.Press("Shift+Tab")
	app.ExpectNoText("Undo")
	app.ExpectFocused(telatest.ByTestID("before"))
}

func TestMenuBarMouse(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	// Hovering with nothing open does nothing.
	app.Hover(trigger("Edit"))
	app.ExpectNoText("Undo")
	// The press opens it, with no item focused: the menu itself is.
	app.Click(trigger("File"))
	app.Expect(trigger("File"), telatest.Expanded(true))
	app.ExpectFocused(telatest.ByRole("menu", "File"))
	// Hovering another trigger opens its menu.
	app.Hover(trigger("Edit"))
	app.ExpectText("Undo")
	app.ExpectNoText("Save")
	app.Expect(trigger("Edit"), telatest.Expanded(true))
	app.Expect(trigger("File"), telatest.Expanded(false))
	app.ExpectFocused(telatest.ByRole("menu", "Edit"))
	app.Hover(trigger("View"))
	app.ExpectText("Zoom")
	// A press on the open menu's trigger closes it.
	app.Click(trigger("View"))
	app.ExpectNoText("Zoom")
	app.Expect(trigger("View"), telatest.Expanded(false))
	app.ExpectFocused(trigger("View"))
	// Hovering after a close does nothing again.
	app.Hover(trigger("File"))
	app.ExpectNoText("New")
	// Choosing an item runs the action and closes the bar's menu.
	app.Click(trigger("File"))
	app.Click(menuitem("Open"))
	if got := ev.take(); got != "action:open" {
		t.Errorf("events %q", got)
	}
	app.ExpectNoText("Save")
	app.ExpectFocused(trigger("File"))
}

// A touch opens a menu on the release, and a tap on another trigger (no
// hover first) moves to its menu.
func TestMenuBarTouch(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(barPage(aria.MenuBarOptions[string]{}, &ev)), 400, 300)
	center := func(name string) (float32, float32) {
		n := findNode(app.tree.Semantics(), "menuitem", name)
		if n == nil {
			t.Fatalf("no trigger %q", name)
		}
		return (n.Bounds.X0 + n.Bounds.X1) / 2, (n.Bounds.Y0 + n.Bounds.Y1) / 2
	}
	x, y := center("File")
	app.down(platform.PointerTouch, x, y)
	if findNode(app.tree.Semantics(), "menu", "File") != nil {
		t.Errorf("the menu opened on the touch down")
	}
	app.up(platform.PointerTouch, x, y)
	if findNode(app.tree.Semantics(), "menu", "File") == nil {
		t.Errorf("a tap did not open the menu:\n%s", app.tree.AriaSnapshot())
	}
	x, y = center("View")
	app.tap(platform.PointerTouch, x, y)
	if findNode(app.tree.Semantics(), "menu", "View") == nil || findNode(app.tree.Semantics(), "menu", "File") != nil {
		t.Errorf("a tap on another trigger did not move to its menu:\n%s", app.tree.AriaSnapshot())
	}
	app.tap(platform.PointerTouch, x, y) // and a tap on the open one closes it
	if findNode(app.tree.Semantics(), "menu", "View") != nil {
		t.Errorf("a tap on the open trigger did not close it:\n%s", app.tree.AriaSnapshot())
	}
}

func TestMenuBarOutsidePressCloses(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	app.Click(trigger("File"))
	app.ExpectText("Save")
	app.ClickAt(300, 250)
	app.ExpectNoText("Save")
	app.Expect(trigger("File"), telatest.Expanded(false))
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestMenuBarDisabled(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{DisabledKeys: []string{"edit"}}, &ev)
	app.Expect(trigger("Edit"), telatest.Disabled(true))
	app.Click(trigger("File"))
	app.Hover(trigger("View"))
	app.ExpectText("Zoom")
	app.Press("Escape")
	app.Focus(trigger("File"))
	app.Press("ArrowDown")
	app.Press("ArrowRight") // skips the disabled Edit
	app.ExpectFocused(menuitem("Zoom"))

	app2 := mountBar(t, aria.MenuBarOptions[string]{Disabled: true}, &ev)
	app2.ClickAt(30, 40)
	app2.ExpectNoText("New")
}

func TestMenuBarAriaSnapshot(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	app.ExpectAria(`
		- button "Before"
		- menubar "Application":
		  - menuitem "File"
		  - menuitem "Edit"
		  - menuitem "View"
		- button "After"
	`)
	app.Focus(trigger("Edit"))
	app.Press("ArrowDown")
	app.ExpectAria(`
		- button "Before"
		- menubar "Application":
		  - menuitem "File"
		  - menuitem "Edit" [expanded]
		  - menuitem "View"
		- button "After"
		- menu "Edit":
		  - menuitem "Undo"
		  - menuitem "Redo"
	`)
}

// A controlled bar shows the menu it is given and asks for changes.
// useControlledBar keeps the open menu of a bar in state and logs the
// changes it is asked for.
var controlledLog []string

func useControlledBar() aria.MenuBarOptions[string] {
	cur, setCur := tela.UseState("")
	return aria.MenuBarOptions[string]{
		Open: &cur,
		OnOpenChange: func(k string, isOpen bool) {
			controlledLog = append(controlledLog, k+"="+map[bool]string{true: "open", false: "closed"}[isOpen])
			if isOpen {
				setCur.Set(k)
			} else {
				setCur.Set("")
			}
		},
	}
}

func TestMenuBarControlled(t *testing.T) {
	var ev events
	controlledLog = nil
	page := barPageWith(useControlledBar, &ev)
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.Focus(trigger("File"))
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("New"))
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Undo"))
	app.Press("Escape")
	app.ExpectNoText("Undo")
	app.ExpectFocused(trigger("Edit"))
	want := "file=open edit=open edit=closed"
	if got := strings.Join(controlledLog, " "); got != want {
		t.Errorf("OnOpenChange %q, want %q", got, want)
	}
}

// An uncontrolled bar reports what opens and what closes: moving from one
// menu to another is one change, with the one that opens.
func TestMenuBarOnOpenChange(t *testing.T) {
	var ev events
	var log []string
	o := aria.MenuBarOptions[string]{OnOpenChange: func(k string, open bool) {
		log = append(log, fmt.Sprintf("%s=%v", k, open))
	}}
	app := mountBar(t, o, &ev)
	app.Click(trigger("File"))
	app.Hover(trigger("Edit"))
	app.Press("ArrowRight")
	app.Press("Escape")
	app.Focus(trigger("View"))
	app.Press("ArrowUp")
	app.Press("ArrowUp") // inside the menu now: moves between its items
	want := "file=true edit=true view=true view=false view=true"
	if got := strings.Join(log, " "); got != want {
		t.Errorf("OnOpenChange %q, want %q", got, want)
	}
}

// A controlled bar that does not store the change stays as it was.
func TestMenuBarControlledRefused(t *testing.T) {
	var ev events
	none := ""
	o := aria.MenuBarOptions[string]{Open: &none}
	app := mountBar(t, o, &ev)
	app.Focus(trigger("File"))
	app.Press("ArrowDown")
	app.ExpectNoText("New")
	app.Expect(trigger("File"), telatest.Expanded(false))

	edit := "edit"
	app2 := mountBar(t, aria.MenuBarOptions[string]{Open: &edit}, &ev)
	app2.ExpectText("Undo")
	app2.Expect(trigger("Edit"), telatest.Expanded(true))
}

func TestMenuBarDefaultOpen(t *testing.T) {
	var ev events
	view := "view"
	app := mountBar(t, aria.MenuBarOptions[string]{DefaultOpen: &view}, &ev)
	app.ExpectText("Zoom")
	app.Expect(trigger("View"), telatest.Expanded(true))
}

// A settled open menu costs no frames.
func TestMenuBarIdle(t *testing.T) {
	var ev events
	app := mountBar(t, aria.MenuBarOptions[string]{}, &ev)
	app.Focus(trigger("File"))
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("New"))
	before := app.Stats()
	app.Advance(2 * time.Second)
	if after := app.Stats(); after.Frames != before.Frames {
		t.Errorf("an open menu bar ran %d frames", after.Frames-before.Frames)
	}
}

func TestMenuBarDevErrors(t *testing.T) {
	var ev events
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("an unknown Direction did not panic")
		}
	}()
	mountBar(t, aria.MenuBarOptions[string]{Direction: "up"}, &ev)
}

func ExampleUseMenuBar() {
	// A menu bar: ArrowDown opens the File menu on its first item, ArrowRight
	// moves to the Edit menu.
	var ev events
	app := telatest.MountFunc(&exampleTB{}, barPage(aria.MenuBarOptions[string]{}, &ev), telatest.Size(400, 300))
	app.Focus(trigger("File"))
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Undo"))
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Before"
	// - menubar "Application":
	//   - menuitem "File"
	//   - menuitem "Edit" [expanded]
	//   - menuitem "View"
	// - button "After"
	// - menu "Edit":
	//   - menuitem "Undo"
	//   - menuitem "Redo"
}
