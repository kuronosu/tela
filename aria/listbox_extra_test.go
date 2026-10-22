package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// The booleans of the result style an item in Go, as the variants do in
// classes: focused, focus-visible (a ring only after the keyboard), selected.
func TestListBoxResultBooleans(t *testing.T) {
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: fruits[:4], TextValue: capital, SelectionMode: "single", AriaLabel: "Fruit"})
		var state []string
		for _, k := range fruits[:4] {
			if lb.IsFocused(k) {
				state = append(state, "focused:"+k)
			}
			if lb.IsFocusVisible(k) {
				state = append(state, "ring:"+k)
			}
			if lb.IsSelected(k) {
				state = append(state, "selected:"+k)
			}
		}
		return Div(
			Div(lb.Props, Range(4, func(i int) tela.Node {
				return Div(lb.ItemProps(fruits[i]), Key(fruits[i]), Class("h-8"), Text(capital(fruits[i])))
			})),
			Textf("state: [%s]", strings.Join(state, " ")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	app.ExpectText("state: []")
	// A click focuses without a ring.
	app.Click(option("Banana"))
	app.ExpectText("state: [focused:banana selected:banana]")
	// A key makes the focus visible.
	app.Press("ArrowDown")
	app.ExpectText("state: [selected:banana focused:blueberry ring:blueberry]")
	app.Press("Space")
	app.ExpectText("state: [focused:blueberry ring:blueberry selected:blueberry]")
}

func TestListBoxTouchTapToggles(t *testing.T) {
	// A touch has no modifier keys, so in "replace" behavior it toggles.
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple", SelectionBehavior: "replace"}
	app := mountRaw(t, tela.C(selPage(o, nil)), 300, 400)
	tap := func(i int) { app.tap(platform.PointerTouch, 20, float32(i)*32+16) }
	want := func(sel string) {
		t.Helper()
		if s := app.tree.AriaSnapshot(); !strings.Contains(s, "selected: "+sel) {
			t.Errorf("want %q:\n%s", sel, s)
		}
	}
	tap(0)
	want("apple")
	tap(2)
	want("apple,blueberry")
	tap(0)
	want("blueberry")
}

func TestListBoxDoubleClickRunsTheSecondaryAction(t *testing.T) {
	var ev events
	o := aria.ListBoxOptions[string]{SelectionMode: "single", SelectionBehavior: "replace", DisallowEmptySelection: true,
		OnAction: func(k string) { ev.add("open " + k) }}
	app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400))
	app.Click(option("Date"))
	app.ExpectText("selected: date")
	if got := ev.take(); got != "" {
		t.Errorf("a click acted: %q", got)
	}
	app.DoubleClick(option("Banana"))
	if got := ev.take(); got != "open banana" {
		t.Errorf("a double click: %q", got)
	}
	app.ExpectText("selected: banana")
}

func ExampleUseListBox() {
	names := []string{"apple", "banana", "cherry"}
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: names, TextValue: strings.ToUpper, SelectionMode: "single", AriaLabel: "Fruit",
		})
		return Div(lb.Props, Class("w-48 rounded-md border p-1"),
			Range(len(names), func(i int) tela.Node {
				k := names[i]
				return Div(lb.ItemProps(k), Key(k),
					Class("rounded px-2 py-1 aria-selected:bg-primary aria-selected:text-primary-foreground focus-visible:ring-2"),
					Text(k))
			}))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(300, 200))
	app.Press("Tab")
	app.Press("ArrowDown")
	app.Press("Enter")
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - listbox "Fruit":
	//   - option "apple"
	//   - option "banana" [selected]
	//   - option "cherry"
}

func ExampleUseListBox_controlled() {
	// A multiple selection that the app keeps, with the other half of the
	// page reading it.
	names := []string{"apple", "banana", "cherry"}
	page := func() tela.Node {
		sel, setSel := tela.UseState(aria.SelectionOf("banana"))
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: names, SelectionMode: "multiple", AriaLabel: "Fruit",
			Selected: sel, OnSelectionChange: setSel.Set,
		})
		return Div(
			Div(lb.Props, Range(len(names), func(i int) tela.Node {
				return Div(lb.ItemProps(names[i]), Key(names[i]), Class("h-8"), Text(names[i]))
			})),
			Textf("%d selected", sel.Resolve(names).Len()))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(300, 200))
	app.Click(telatest.ByRole("option", "cherry"))
	app.Press("Control+a")
	app.ExpectText("3 selected")
	fmt.Println("3 selected")
	// Output: 3 selected
}

func ExampleCollection_HandleKeyDown() {
	// A list whose focus stays on a text field, as in a combobox: the
	// field forwards its keys and carries aria-activedescendant.
	names := []string{"apple", "banana", "cherry"}
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: names, SelectionMode: "single", AriaLabel: "Fruit", ExternalFocus: true,
		})
		picked, _ := lb.Selected.First()
		return Div(
			Input(AriaLabel("Search"), OnKeyDown(lb.HandleKeyDown), lb.ActiveDescendant(),
				OnBlur(lb.ClearFocus)),
			Div(lb.Props, Range(len(names), func(i int) tela.Node {
				return Div(lb.ItemProps(names[i]), Key(names[i]), Class("h-8"), Text(names[i]))
			})),
			Textf("picked: %s", picked))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(300, 200))
	app.Focus(telatest.ByRole("textbox", "Search"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("Enter")
	app.ExpectText("picked: banana")
	fmt.Println("picked: banana")
	// Output: picked: banana
}
