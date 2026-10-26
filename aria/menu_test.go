package aria_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

var actions = []string{"edit", "duplicate", "archive", "delete"}

// testMenu is the menu of a menu button: a component of its own, rendered
// only while the menu is open, so UseMenu focuses an item when it mounts.
type testMenu struct {
	mt   aria.MenuTriggerResult
	pop  aria.PopoverResult
	opts aria.MenuOptions[string]
	ev   *events
}

func (m testMenu) Build() tela.Node {
	o := m.opts
	o.Keys, o.TextValue = actions, capital
	o.OnClose, o.AutoFocus, o.FocusStrategy, o.Ref = m.mt.Close, true, m.mt.FocusStrategy, m.mt.MenuRef
	o.OnAction = func(k string) { m.ev.add("action:" + k) }
	menu := aria.UseMenu(o)
	return Div(m.pop.Props, m.mt.MenuProps, menu.Props, Class("absolute w-40 bg-white"),
		Range(len(actions), func(i int) tela.Node {
			k := actions[i]
			return Div(menu.ItemProps(k), Key(k), Class("h-8 px-2 focus-visible:bg-sky-100 aria-checked:font-bold"), Text(capital(k)))
		}))
}

// menuApp is a menu button with a menu of actions.
func menuApp(o aria.MenuOptions[string], ev *events) func() tela.Node {
	return func() tela.Node {
		mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{})
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close, Placement: "bottom-start", Modal: true,
		})
		return Div(Class("relative h-full"),
			Button(mt.TriggerProps, Class("absolute left-4 top-4 h-8 w-24"), Text("Actions")),
			Portal(If(mt.Open, testMenu{mt: mt, pop: pop, opts: o, ev: ev})))
	}
}

func menuitem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

func TestMenuOpenWithKeys(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{}, &ev), telatest.Size(400, 300))
	trigger := telatest.ByRole("button", "Actions")
	app.Focus(trigger)
	// ArrowDown opens it and focuses the first item.
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Edit"))
	app.Press("Escape")
	app.ExpectFocused(trigger)
	app.Expect(trigger, telatest.Expanded(false))
	// ArrowUp focuses the last.
	app.Press("ArrowUp")
	app.ExpectFocused(menuitem("Delete"))
	app.Press("ArrowUp")
	app.ExpectFocused(menuitem("Archive"))
	app.Press("Home")
	app.ExpectFocused(menuitem("Edit"))
	app.Press("End")
	app.ExpectFocused(menuitem("Delete"))
	app.Press("ArrowDown") // no wrap by default
	app.ExpectFocused(menuitem("Delete"))
	app.Press("Escape")
	// Enter and Space open it as well.
	app.Press("Enter")
	app.ExpectFocused(menuitem("Edit"))
	app.Press("Escape")
	app.Press("Space")
	app.ExpectFocused(menuitem("Edit"))
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestMenuActionClosesAndRestoresFocus(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{}, &ev), telatest.Size(400, 300))
	trigger := telatest.ByRole("button", "Actions")
	app.Focus(trigger)
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("Enter")
	if got := ev.take(); got != "action:duplicate" {
		t.Errorf("events %q", got)
	}
	app.ExpectNoText("Archive")
	app.ExpectFocused(trigger) // the trigger's scope restores the focus
	// Space chooses too; the mouse chooses on a click.
	app.Press("ArrowUp")
	app.Press("Space")
	if got := ev.take(); got != "action:delete" {
		t.Errorf("events %q", got)
	}
	app.ExpectFocused(trigger)
	app.Click(trigger)
	app.Click(menuitem("Archive"))
	if got := ev.take(); got != "action:archive" {
		t.Errorf("events %q", got)
	}
	app.ExpectNoText("Archive")
}

func TestMenuMouseOpensWithNoItemFocused(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{}, &ev), telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Actions"))
	app.ExpectFocused(telatest.ByRole("menu", "Actions")) // the menu itself
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Edit"))
	app.Press("Escape")
	app.ExpectFocused(telatest.ByRole("button", "Actions"))
}

func TestMenuHoverFocusesItems(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{}, &ev), telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Actions"))
	app.Hover(menuitem("Archive"))
	app.ExpectFocused(menuitem("Archive"))
	app.Hover(menuitem("Duplicate"))
	app.ExpectFocused(menuitem("Duplicate"))
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Archive"))
}

func TestMenuDisabledAndTypeahead(t *testing.T) {
	var ev events
	o := aria.MenuOptions[string]{DisabledKeys: []string{"duplicate"}, ShouldFocusWrap: true}
	app := telatest.MountFunc(t, menuApp(o, &ev), telatest.Size(400, 300))
	app.Focus(telatest.ByRole("button", "Actions"))
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Edit"))
	app.Press("ArrowDown") // duplicate is disabled
	app.ExpectFocused(menuitem("Archive"))
	app.Press("ArrowDown")
	app.Press("ArrowDown") // wraps
	app.ExpectFocused(menuitem("Edit"))
	app.Press("d")
	app.ExpectFocused(menuitem("Delete")) // "d" skips the disabled "duplicate"
	app.Expect(menuitem("Duplicate"), telatest.Disabled(true))
	app.Press("Escape")
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestMenuAriaSnapshot(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{DisabledKeys: []string{"archive"}}, &ev), telatest.Size(400, 300))
	app.Focus(telatest.ByRole("button", "Actions"))
	app.Press("ArrowDown")
	app.ExpectAria(`
		- menu "Actions":
		  - menuitem "Edit"
		  - menuitem "Duplicate"
		  - menuitem "Archive" [disabled]
		  - menuitem "Delete"
	`)
}

// Checkbox and radio items: they show their state, a radio choice closes
// the menu and a checkbox keeps it open.
func TestMenuCheckboxItems(t *testing.T) {
	var ev events
	o := aria.MenuOptions[string]{SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("duplicate")}
	app := telatest.MountFunc(t, menuApp(o, &ev), telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Actions"))
	app.ExpectAria(`
		- menu "Actions":
		  - menuitemcheckbox "Edit"
		  - menuitemcheckbox "Duplicate" [checked]
		  - menuitemcheckbox "Archive"
		  - menuitemcheckbox "Delete"
	`)
	app.Click(telatest.ByRole("menuitemcheckbox", "Edit"))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Edit"), telatest.Checked(true))
	app.Press("ArrowDown")
	app.Press("Space")
	app.Expect(telatest.ByRole("menuitemcheckbox", "Duplicate"), telatest.Checked(false))
	// The menu is still open and the actions ran.
	app.ExpectText("Delete")
	if got := ev.take(); got != "action:edit action:duplicate" {
		t.Errorf("events %q", got)
	}
	app.Press("Escape")
	// The selection survives a close: it belongs to the menu that was
	// open, so the next one starts from DefaultSelected again.
	app.Click(telatest.ByRole("button", "Actions"))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Duplicate"), telatest.Checked(true))
}

func TestMenuRadioItems(t *testing.T) {
	var ev events
	var chosen string
	o := aria.MenuOptions[string]{SelectionMode: "single", DefaultSelected: aria.SelectionOf("archive"),
		OnSelectionChange: func(s aria.Selection[string]) { chosen, _ = s.First() }}
	app := telatest.MountFunc(t, menuApp(o, &ev), telatest.Size(400, 300))
	trigger := telatest.ByRole("button", "Actions")
	app.Focus(trigger)
	app.Press("ArrowDown")
	// The selected radio item takes the focus when the menu opens.
	app.ExpectFocused(telatest.ByRole("menuitemradio", "Archive"))
	app.ExpectAria(`
		- menu "Actions":
		  - menuitemradio "Edit"
		  - menuitemradio "Duplicate"
		  - menuitemradio "Archive" [checked]
		  - menuitemradio "Delete"
	`)
	app.Press("ArrowUp")
	app.Press("Enter")
	if chosen != "duplicate" {
		t.Errorf("chosen %q", chosen)
	}
	app.ExpectNoText("Archive")
	app.ExpectFocused(trigger)
	// Choosing the checked radio item again keeps it checked.
	app.Press("Enter")
	app.Press("Enter")
	if chosen != "duplicate" {
		t.Errorf("chosen %q after choosing the same item", chosen)
	}
}

func TestMenuCloseOnSelect(t *testing.T) {
	var ev events
	no, yes := false, true
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{CloseOnSelect: &no}, &ev), telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Actions"))
	app.Click(menuitem("Edit"))
	app.ExpectText("Archive") // still open
	if got := ev.take(); got != "action:edit" {
		t.Errorf("events %q", got)
	}
	app2 := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{SelectionMode: "multiple", CloseOnSelect: &yes}, &ev), telatest.Size(400, 300))
	app2.Click(telatest.ByRole("button", "Actions"))
	app2.Click(telatest.ByRole("menuitemcheckbox", "Edit"))
	app2.ExpectNoText("Archive")
}

func ExampleUseMenu() {
	// A menu button: ArrowDown opens the menu with the first item focused,
	// Enter chooses it, the menu closes and the focus goes back.
	var ev events
	app := telatest.MountFunc(&exampleTB{}, menuApp(aria.MenuOptions[string]{}, &ev), telatest.Size(400, 300))
	trigger := telatest.ByRole("button", "Actions")
	app.Focus(trigger)
	app.Press("ArrowDown")
	fmt.Println(app.AriaSnapshot())
	app.Press("Enter")
	app.ExpectFocused(trigger)
	fmt.Println(ev.take())
	// Output:
	// - menu "Actions":
	//   - menuitem "Edit"
	//   - menuitem "Duplicate"
	//   - menuitem "Archive"
	//   - menuitem "Delete"
	// action:edit
}

// TestMenuMouseLeavingAnItemClearsTheHighlight is Radix's onItemLeave: when
// the mouse leaves an item (for the outside of the menu, or a disabled
// item) no item is highlighted, and the focus stays on the menu, so the keys
// still work from there.
func TestMenuMouseLeavingAnItemClearsTheHighlight(t *testing.T) {
	var ev events
	o := aria.MenuOptions[string]{DisabledKeys: []string{"duplicate"}}
	app := telatest.MountFunc(t, menuApp(o, &ev), telatest.Size(400, 300))
	menu := telatest.ByRole("menu", "Actions")
	app.Click(telatest.ByRole("button", "Actions"))
	app.Hover(menuitem("Archive"))
	app.ExpectFocused(menuitem("Archive"))
	app.MouseMove(350, 250) // outside the menu
	app.ExpectFocused(menu)
	// From the menu itself: ArrowDown goes to the first item, ArrowUp to
	// the last, and typeahead finds an item.
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Edit"))
	app.Hover(menuitem("Archive"))
	app.MouseMove(350, 250)
	app.ExpectFocused(menu)
	app.Press("ArrowUp")
	app.ExpectFocused(menuitem("Delete"))
	app.Hover(menuitem("Edit"))
	app.MouseMove(350, 250)
	app.Press("a")
	app.ExpectFocused(menuitem("Archive"))
	// A disabled item takes no highlight, and clears the one before it.
	app.Hover(menuitem("Edit"))
	app.ExpectFocused(menuitem("Edit"))
	app.Hover(menuitem("Duplicate"))
	app.ExpectFocused(menu)
	// Moving from one item to the next is not a gap: the next one is
	// highlighted.
	app.Hover(menuitem("Archive"))
	app.Hover(menuitem("Delete"))
	app.ExpectFocused(menuitem("Delete"))
}

// TestMenuKeysScrollUnderARestingMouse: the keys scroll a long menu under
// a mouse that does not move. The leave that the new hit test fires for
// the item that scrolled away is not the mouse leaving: the highlight
// stays on the item the keys reached and ArrowDown goes on from it. A real
// move out of the menu still clears it.
func TestMenuKeysScrollUnderARestingMouse(t *testing.T) {
	keys := make([]string, 20)
	for i := range keys {
		keys[i] = fmt.Sprintf("Item %02d", i)
	}
	app := telatest.MountFunc(t, func() tela.Node {
		menu := aria.UseMenu(aria.MenuOptions[string]{Keys: keys, TextValue: func(s string) string { return s }, AriaLabel: "Long"})
		return Div(Class("p-4"),
			Div(menu.Props, Class("h-40 w-40 overflow-y-auto"),
				Range(len(keys), func(i int) tela.Node {
					return Div(menu.ItemProps(keys[i]), Key(keys[i]), Class("h-8 px-2"), Text(keys[i]))
				})))
	}, telatest.Size(300, 300))
	app.Hover(menuitem("Item 02"))
	app.ExpectFocused(menuitem("Item 02"))
	for i := 3; i < 12; i++ { // the menu shows five items: from Item 05 on, every key scrolls
		app.Press("ArrowDown")
		app.ExpectFocused(menuitem(keys[i]))
	}
	app.MouseMove(250, 250)
	app.ExpectFocused(telatest.ByRole("menu", "Long"))
	app.Press("ArrowDown")
	app.ExpectFocused(menuitem("Item 00"))
}

// TestMenuReopensWithTheRightItem: opened with the mouse nothing is
// highlighted, whatever the last use of the menu was; opened with the
// keyboard (Enter, Space or ArrowDown on the trigger) the first item is.
func TestMenuReopensWithTheRightItem(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{}, &ev), telatest.Size(400, 300))
	trigger := telatest.ByRole("button", "Actions")
	menu := telatest.ByRole("menu", "Actions")
	for range 2 {
		app.Click(trigger)
		app.ExpectFocused(menu)
		app.Hover(menuitem("Archive"))
		app.ExpectFocused(menuitem("Archive"))
		app.MouseMove(350, 250)
		app.ExpectFocused(menu)
		app.Press("Escape")
	}
	app.Focus(trigger)
	for _, key := range []string{"Enter", "Space", "ArrowDown"} {
		app.Press(key)
		app.ExpectFocused(menuitem("Edit"))
		app.Hover(menuitem("Archive"))
		app.MouseMove(350, 250)
		app.Press("Escape")
		app.ExpectFocused(trigger)
	}
	app.Click(trigger)
	app.ExpectFocused(menu)
}

// TestMenuCheckboxAndRadioItemsHighlightLikeOthers: the items that can be
// checked follow the same rule as any other item.
func TestMenuCheckboxAndRadioItemsHighlightLikeOthers(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuApp(aria.MenuOptions[string]{SelectionMode: "multiple"}, &ev), telatest.Size(400, 300))
	menu := telatest.ByRole("menu", "Actions")
	app.Click(telatest.ByRole("button", "Actions"))
	app.Hover(telatest.ByRole("menuitemcheckbox", "Duplicate"))
	app.ExpectFocused(telatest.ByRole("menuitemcheckbox", "Duplicate"))
	app.MouseMove(350, 250)
	app.ExpectFocused(menu)
}
