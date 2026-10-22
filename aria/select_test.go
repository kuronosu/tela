package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// selectPage renders a select of fruits, with a label, and logs what its
// callbacks saw.
func selectPage(o aria.SelectOptions[string], ev *events) func() tela.Node {
	return func() tela.Node {
		if o.Keys == nil {
			o.Keys = fruits
		}
		if o.TextValue == nil {
			o.TextValue = capital
		}
		if o.OnSelectionChange == nil && ev != nil {
			o.OnSelectionChange = func(s aria.Selection[string]) {
				k, _ := s.First()
				ev.add("selected " + k)
			}
		}
		sel := aria.UseSelect(o)
		return Div(Class("relative h-full w-full"),
			Div(sel.LabelProps, Class("h-6"), Text("Favorite fruit")),
			Button(sel.TriggerProps, Class("flex h-8 w-48 items-center px-2"),
				Span(sel.ValueProps, Text(sel.Value))),
			Button(Class("h-8 w-24"), Text("Other")),
			Portal(If(sel.Open,
				Div(sel.PopoverProps, Class("w-48 bg-white shadow"),
					Div(sel.ListBox.Props, Class("max-h-48 overflow-y-auto"),
						Range(len(o.Keys), func(i int) tela.Node {
							k := o.Keys[i]
							return Div(sel.ListBox.ItemProps(k), Key(k),
								Class("h-8 px-2 aria-selected:bg-sky-200 focus-visible:ring-2"), Text(capital(k)))
						}))))))
	}
}

// clickCenter clicks the center of an element with a pointer, whatever its
// state (Click waits for it to be enabled).
func clickCenter(app *telatest.App, l telatest.Locator) {
	b := app.Bounds(l)
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
}

func selectTrigger(name string) telatest.Locator { return telatest.ByRole("button", name) }

var fruitTrigger = selectTrigger("Select a fruit Favorite fruit")

func TestSelectOpensWithClick(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, &ev), telatest.Size(400, 400))
	app.Expect(fruitTrigger, telatest.Expanded(false))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.Click(fruitTrigger)
	app.Expect(fruitTrigger, telatest.Expanded(true))
	// A mouse opens it with the list itself focused.
	app.ExpectFocused(telatest.ByRole("listbox", "Favorite fruit"))
	app.ExpectCount(telatest.ByRole("option", ""), len(fruits))
	// Clicking the trigger again closes it.
	app.Click(fruitTrigger)
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectFocused(fruitTrigger)
	// Choosing an item selects it, closes the list and gives the focus back.
	app.Click(fruitTrigger)
	app.Click(option("Cherry"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	trigger := selectTrigger("Cherry Favorite fruit")
	app.ExpectFocused(trigger)
	app.Expect(trigger, telatest.Expanded(false))
	if got := ev.take(); got != "selected cherry" {
		t.Errorf("events %q", got)
	}
	// With a selection the list opens on it, whatever opened it.
	app.Click(trigger)
	app.ExpectFocused(option("Cherry"))
	app.Expect(option("Cherry"), telatest.Selected(true))
	// Choosing the selected item closes the list without a change.
	app.Click(option("Cherry"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectFocused(trigger)
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestSelectOpensWithKeys(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, &ev), telatest.Size(400, 400))
	app.Focus(fruitTrigger)
	// ArrowDown focuses the first item, ArrowUp the last.
	app.Press("ArrowDown")
	app.ExpectFocused(option("Apple"))
	app.Press("Escape")
	app.ExpectFocused(fruitTrigger)
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.Press("ArrowUp")
	app.ExpectFocused(option("Fig"))
	app.Press("Escape")
	app.Press("Enter")
	app.ExpectFocused(option("Apple"))
	app.Press("Escape")
	app.Press("Space")
	app.ExpectFocused(option("Apple"))
	// The keyboard moves and chooses; Enter closes and restores the focus.
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(option("Blueberry"))
	app.Press("Enter")
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	trigger := selectTrigger("Blueberry Favorite fruit")
	app.ExpectFocused(trigger)
	// Space chooses too. The selected item has the focus when it opens,
	// whichever key opened it.
	app.Press("ArrowUp")
	app.ExpectFocused(option("Blueberry"))
	app.Press("End")
	app.Press("Space")
	app.ExpectFocused(selectTrigger("Fig Favorite fruit"))
	if got := ev.take(); got != "selected blueberry selected fig" {
		t.Errorf("events %q", got)
	}
}

// Typing on the closed trigger selects; the horizontal arrows too.
func TestSelectTypeaheadOnTheClosedTrigger(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, &ev), telatest.Size(400, 400))
	app.Focus(fruitTrigger)
	app.Press("b")
	app.ExpectFocused(selectTrigger("Banana Favorite fruit"))
	app.Press("l")
	app.ExpectFocused(selectTrigger("Blueberry Favorite fruit"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0) // it stays closed
	// The search expires after a second.
	app.Advance(1100 * time.Millisecond)
	app.Press("d")
	app.ExpectFocused(selectTrigger("Date Favorite fruit"))
	app.Advance(1100 * time.Millisecond)
	app.Press("f")
	app.Press("i")
	app.ExpectFocused(selectTrigger("Fig Favorite fruit"))
	app.Advance(1100 * time.Millisecond)
	app.Press("ArrowLeft")
	app.ExpectFocused(selectTrigger("Elderberry Favorite fruit"))
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	app.ExpectFocused(selectTrigger("Fig Favorite fruit")) // the last stays
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	if got := ev.take(); got != "selected banana selected blueberry selected date selected fig selected elderberry selected fig" {
		t.Errorf("events %q", got)
	}
}

// With nothing selected the arrows start from the first item.
func TestSelectArrowsFromNoSelection(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, nil), telatest.Size(400, 400))
	app.Focus(fruitTrigger)
	app.Press("ArrowRight")
	app.ExpectFocused(selectTrigger("Apple Favorite fruit"))
}

func TestSelectTypeaheadInTheList(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit", DisabledKeys: []string{"date"}}, &ev), telatest.Size(400, 400))
	app.Click(fruitTrigger)
	app.Press("c")
	app.ExpectFocused(option("Cherry"))
	app.Advance(1100 * time.Millisecond)
	app.Press("d") // date is disabled
	app.ExpectFocused(option("Cherry"))
	app.Advance(1100 * time.Millisecond)
	app.Press("e")
	app.ExpectFocused(option("Elderberry"))
	app.Press("Enter")
	app.ExpectFocused(selectTrigger("Elderberry Favorite fruit"))
	if got := ev.take(); got != "selected elderberry" {
		t.Errorf("events %q", got)
	}
}

func TestSelectDisabledItems(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{
		Placeholder: "Select a fruit", DisabledKeys: []string{"banana"},
	}, &ev), telatest.Size(400, 400))
	app.Focus(fruitTrigger)
	// Typeahead on the closed trigger skips it, and so do the arrows.
	app.Press("b")
	app.ExpectFocused(selectTrigger("Blueberry Favorite fruit"))
	app.Press("ArrowLeft")
	app.ExpectFocused(selectTrigger("Apple Favorite fruit"))
	app.Press("ArrowRight")
	app.ExpectFocused(selectTrigger("Blueberry Favorite fruit"))
	app.Press("Enter")
	app.Press("Home")
	app.Press("ArrowDown")
	app.ExpectFocused(option("Blueberry")) // banana is skipped
	app.Expect(option("Banana"), telatest.Disabled(true))
	clickCenter(app, option("Banana"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 1) // still open
	if got := ev.take(); got != "selected blueberry selected apple selected blueberry" {
		t.Errorf("events %q", got)
	}
}

func TestSelectEscapeAndOutsidePress(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, nil), telatest.Size(400, 400))
	app.Click(fruitTrigger)
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectFocused(fruitTrigger)
	// A press outside closes it and does not reach what is under it.
	app.Click(fruitTrigger)
	app.ClickAt(300, 300)
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	// Tab leaves the list and closes it.
	app.Focus(fruitTrigger)
	app.Press("ArrowDown")
	app.Press("Tab")
	app.Expect(telatest.ByRole("button", "Other"), telatest.Focused(true))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
}

func TestSelectLabelAndName(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, nil), telatest.Size(400, 400))
	app.Click(telatest.ByText("Favorite fruit"))
	app.ExpectFocused(fruitTrigger)
	app.Click(fruitTrigger)
	app.ExpectFocused(telatest.ByRole("listbox", "Favorite fruit"))
	// Without a label element, AriaLabel names the select.
	app2 := telatest.MountFunc(t, func() tela.Node {
		sel := aria.UseSelect(aria.SelectOptions[string]{Keys: fruits, TextValue: capital, AriaLabel: "Fruit", DefaultSelected: aria.SelectionOf("fig")})
		return Div(
			Button(sel.TriggerProps, Span(sel.ValueProps, Text(sel.Value))),
			Portal(If(sel.Open, Div(sel.PopoverProps, Div(sel.ListBox.Props,
				Range(len(fruits), func(i int) tela.Node {
					return Div(sel.ListBox.ItemProps(fruits[i]), Key(fruits[i]), Text(capital(fruits[i])))
				}))))))
	}, telatest.Size(400, 400))
	app2.ExpectAria(`
		- button "Fig Fruit": Fig
	`)
	app2.Press("Tab")
	app2.Press("ArrowDown")
	app2.ExpectAria(`
		- button "Fig Fruit" [expanded]: Fig
		- listbox "Fruit":
		  - option "Apple"
		  - option "Banana"
		  - option "Blueberry"
		  - option "Cherry"
		  - option "Date"
		  - option "Elderberry"
		  - option "Fig" [selected]
	`)
}

func TestSelectAriaSnapshot(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{
		Placeholder: "Select a fruit", DefaultSelected: aria.SelectionOf("banana"), DisabledKeys: []string{"cherry"},
		Keys: []string{"apple", "banana", "cherry"},
	}, nil), telatest.Size(400, 400))
	app.ExpectAria(`
		- text: Favorite fruit
		- button "Banana Favorite fruit": Banana
		- button "Other"
	`)
	app.Click(selectTrigger("Banana Favorite fruit"))
	app.ExpectAria(`
		- text: Favorite fruit
		- button "Banana Favorite fruit" [expanded]: Banana
		- button "Other"
		- listbox "Favorite fruit":
		  - option "Apple"
		  - option "Banana" [selected]
		  - option "Cherry" [disabled]
	`)
	// Without a selection the trigger shows the placeholder.
	app2 := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit"}, nil), telatest.Size(400, 400))
	app2.ExpectAria(`
		- text: Favorite fruit
		- button "Select a fruit Favorite fruit": Select a fruit
		- button "Other"
	`)
}

func TestSelectControlled(t *testing.T) {
	var ev events
	var set func(aria.Selection[string])
	open := false
	page := func() tela.Node {
		sel, setSel := tela.UseState(aria.SelectionOf("apple"))
		set = setSel.Set
		isOpen, setOpen := tela.UseState(open)
		return useSelectNode(aria.SelectOptions[string]{
			Keys: fruits, TextValue: capital, Selected: sel, Open: &isOpen,
			OnSelectionChange: func(s aria.Selection[string]) {
				k, _ := s.First()
				ev.add("selected " + k)
				if k != "cherry" { // the app refuses cherry
					setSel.Set(s)
				}
			},
			OnOpenChange: func(o bool) { ev.add(map[bool]string{true: "open", false: "close"}[o]); setOpen.Set(o) },
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 400))
	trigger := selectTrigger("Apple Favorite fruit")
	app.Click(trigger)
	app.Click(option("Banana"))
	app.ExpectFocused(selectTrigger("Banana Favorite fruit"))
	app.Click(selectTrigger("Banana Favorite fruit"))
	app.Click(option("Cherry")) // refused: the selection stays, the list closes
	app.ExpectFocused(selectTrigger("Banana Favorite fruit"))
	// The app changes it from outside.
	set(aria.SelectionOf("fig"))
	app.ExpectRole("button", "Fig Favorite fruit")
	if got := ev.take(); got != "open selected banana close open selected cherry close" {
		t.Errorf("events %q", got)
	}
}

// useSelectNode is a select without the page's label, for tests that pass all
// the options.
func useSelectNode(o aria.SelectOptions[string]) tela.Node {
	sel := aria.UseSelect(o)
	return Div(Class("relative h-full w-full"),
		Div(sel.LabelProps, Class("h-6"), Text("Favorite fruit")),
		Button(sel.TriggerProps, Class("flex h-8 w-48 items-center px-2"), Span(sel.ValueProps, Text(sel.Value))),
		Portal(If(sel.Open,
			Div(sel.PopoverProps, Class("w-48 bg-white shadow"),
				Div(sel.ListBox.Props, Class("max-h-48 overflow-y-auto"),
					Range(len(o.Keys), func(i int) tela.Node {
						k := o.Keys[i]
						return Div(sel.ListBox.ItemProps(k), Key(k), Class("h-8 px-2"), Text(capital(k)))
					}))))))
}

func TestSelectDisabled(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit", Disabled: true}, nil), telatest.Size(400, 400))
	app.Expect(fruitTrigger, telatest.Disabled(true))
	app.Click(telatest.ByText("Favorite fruit"))
	clickCenter(app, fruitTrigger)
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
}

func TestSelectMatchWidth(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit", MatchWidth: true}, nil), telatest.Size(400, 400))
	app.Click(fruitTrigger)
	trigger, list := app.Bounds(fruitTrigger), app.Bounds(telatest.ByRole("listbox", "Favorite fruit"))
	if trigger.Width != 192 || list.Width != 192 {
		t.Errorf("widths: trigger %v, list %v, want 192", trigger.Width, list.Width)
	}
	// Placed under the trigger, aligned to its start.
	if list.X < trigger.X || list.Y < trigger.Bottom() {
		t.Errorf("list at %+v, trigger at %+v", list, trigger)
	}
}

// DefaultOpen shows the list in the first frame, with the selected item
// focused.
func TestSelectDefaultOpen(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{
		DefaultOpen: true, DefaultSelected: aria.SelectionOf("date"),
	}, nil), telatest.Size(400, 400))
	app.ExpectFocused(option("Date"))
}

// A space inside a search is a character: it neither opens the list nor
// ends the search; a space that starts nothing opens it.
func TestSelectSpaceInTheSearch(t *testing.T) {
	var ev events
	keys := []string{"red apple", "red cherry", "green fig"}
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{
		Keys: keys, TextValue: func(k string) string { return k }, Placeholder: "Pick",
	}, &ev), telatest.Size(400, 400))
	app.Focus(selectTrigger("Pick Favorite fruit"))
	app.Type("red")
	app.Press("Space")
	app.Press("c")
	app.ExpectFocused(selectTrigger("red cherry Favorite fruit"))
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.Advance(1100 * time.Millisecond)
	app.Press("Space") // no search: it opens the list
	app.ExpectFocused(option("Red cherry"))
	if got := ev.take(); got != "selected red apple selected red cherry" {
		t.Errorf("events %q", got)
	}
}

// The selected item of a long list is in view when the list opens.
func TestSelectOpensOnTheSelectedItemOfALongList(t *testing.T) {
	keys := manyKeys(200)
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{
		Keys: keys, TextValue: func(k string) string { return k }, DefaultSelected: aria.SelectionOf("row-0150"),
	}, nil), telatest.Size(400, 400))
	app.Press("Tab")
	app.Press("ArrowDown")
	opt := telatest.ByRole("option", "Row-0150")
	app.ExpectFocused(opt)
	list := app.Bounds(telatest.ByRole("listbox", "Favorite fruit"))
	b := app.Bounds(opt)
	if b.Y < list.Y || b.Bottom() > list.Bottom()+0.5 {
		t.Errorf("the selected item at %+v is outside the list at %+v", b, list)
	}
}

// A modal select makes the rest of the window inert while the list is open.
func TestSelectModal(t *testing.T) {
	app := telatest.MountFunc(t, selectPage(aria.SelectOptions[string]{Placeholder: "Select a fruit", Modal: true}, nil), telatest.Size(400, 400))
	app.Click(fruitTrigger)
	app.ExpectCount(telatest.ByRole("button", "Other"), 0) // hidden from the snapshot
	app.ExpectFocused(telatest.ByRole("listbox", "Favorite fruit"))
	// A press on the trigger is outside: it closes the list.
	app.ClickAt(50, 40) // the trigger is inert and out of the snapshot
	app.ExpectCount(telatest.ByRole("listbox", ""), 0)
	app.ExpectCount(telatest.ByRole("button", "Other"), 1)
	app.ExpectFocused(fruitTrigger)
}
