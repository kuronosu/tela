package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// A list with a virtual focus keeps the focus on itself and highlights an
// item (aria-activedescendant): the items are not focusable.
func TestListBoxVirtualFocus(t *testing.T) {
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: fruits, TextValue: capital, AriaLabel: "Fruit", ShouldUseVirtualFocus: true, SelectionMode: "single",
		})
		focused, _ := lb.Focused()
		return Div(
			Div(lb.Props, Class("h-32 w-48 overflow-y-auto"),
				Range(len(fruits), func(i int) tela.Node {
					k := fruits[i]
					return Div(lb.ItemProps(k), Key(k), Class("h-8 shrink-0 aria-selected:bg-sky-200"), Text(capital(k)))
				})),
			Textf("highlighted: %s", focused))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	list := telatest.ByRole("listbox", "Fruit")
	app.Press("Tab")
	app.ExpectFocused(list) // the list has the focus, an item is highlighted
	app.ExpectText("highlighted: apple")
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(list)
	app.ExpectText("highlighted: blueberry")
	app.Press("Enter")
	app.Expect(option("Blueberry"), telatest.Selected(true))
	// The items are not focusable, and a click highlights and selects.
	app.Click(option("Cherry"))
	app.ExpectFocused(list)
	app.ExpectText("highlighted: cherry")
	app.Expect(option("Cherry"), telatest.Selected(true))
	// The highlighted item is scrolled into view without a DOM focus.
	app.Press("End")
	app.ExpectText("highlighted: fig")
	r, b := app.Bounds(option("Fig")), app.Bounds(list)
	if r.Y < b.Y || r.Y+r.Height > b.Y+b.Height+0.5 {
		t.Errorf("fig at %+v is outside the list at %+v", r, b)
	}
}

// ExternalFocus: another element has the focus (a combobox's input), calls
// HandleKeyDown and carries aria-activedescendant; pressing an option
// leaves the focus there.
func TestListBoxExternalFocus(t *testing.T) {
	var ev events
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: fruits, TextValue: capital, AriaLabel: "Fruit", ExternalFocus: true, SelectionMode: "single",
			OnSelectionChange: func(s aria.Selection[string]) { k, _ := s.First(); ev.add("selected " + k) },
		})
		focused, ok := lb.Focused()
		return Div(
			Input(Type("text"), AriaLabel("Search"), OnKeyDown(lb.HandleKeyDown), lb.ActiveDescendant(),
				OnFocus(func() { lb.Focus(aria.FocusFirst) }), OnBlur(lb.ClearFocus)),
			Div(lb.Props, Class("h-64 w-48"),
				Range(len(fruits), func(i int) tela.Node {
					k := fruits[i]
					return Div(lb.ItemProps(k), Key(k), Class("h-8"), Text(capital(k)))
				})),
			Textf("highlighted: %s %v", focused, ok))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	input := telatest.ByRole("textbox", "Search")
	app.Focus(input)
	app.ExpectText("highlighted: apple true")
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectText("highlighted: blueberry true")
	app.ExpectFocused(input)
	app.Press("Enter")
	app.Expect(option("Blueberry"), telatest.Selected(true))
	// The keys of the text field stay its own: Home, End, Space, letters
	// and Ctrl+A do not move the highlight or select; pages do.
	for _, key := range []string{"Home", "End", "Space", "d", "Escape"} {
		app.Press(key)
	}
	app.ExpectText("highlighted: blueberry true")
	app.Expect(option("Blueberry"), telatest.Selected(true))
	app.Press("PageDown")
	app.ExpectText("highlighted: fig true")
	// A click on an option selects it and keeps the focus on the input.
	app.Click(option("Cherry"))
	app.ExpectFocused(input)
	app.Expect(option("Cherry"), telatest.Selected(true))
	if got := ev.take(); got != "selected blueberry selected cherry" {
		t.Errorf("events %q", got)
	}
	// The list is not a tab stop.
	app.Press("Tab")
	app.Expect(input, telatest.Focused(true))
}

func manyKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("row-%04d", i)
	}
	return keys
}

// Keys of items that are not rendered can be focused: the list scrolls to
// their index and the item takes the focus when it mounts.
func TestListBoxInVirtualList(t *testing.T) {
	keys := manyKeys(1000)
	var ev events
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: keys, TextValue: func(k string) string { return k }, AriaLabel: "Rows", SelectionMode: "multiple",
			OnSelectionChange: func(s aria.Selection[string]) { ev.add(s.String()) },
		})
		return VirtualList(len(keys), func(int) float32 { return 32 },
			func(i int) tela.Node {
				return Div(lb.ItemProps(keys[i]), Class("h-8 aria-selected:bg-sky-200"), Text(keys[i]))
			}, lb.Props, ItemHeight(32), Class("h-64 w-48"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	app.Press("Tab")
	app.ExpectFocused(option("row-0000"))
	app.Press("End")
	app.ExpectFocused(option("row-0999"))
	app.Press("Home")
	app.ExpectFocused(option("row-0000"))
	// A page is a viewport of rendered items.
	app.Press("PageDown")
	app.ExpectFocused(option("row-0008"))
	app.Press("PageDown")
	app.ExpectFocused(option("row-0016"))
	// Typeahead finds an item that is far away.
	app.Press("r")
	app.Press("o")
	app.Press("w")
	app.Press("-")
	app.Press("0")
	app.Press("5")
	app.Press("0")
	app.Press("0")
	app.ExpectFocused(option("row-0500"))
	// Ctrl+A selects the whole list, rendered or not.
	app.Press("Control+a")
	app.Expect(option("row-0500"), telatest.Selected(true))
	if got := ev.take(); got != "all" {
		t.Errorf("events %q", got)
	}
	app.Press("ArrowDown")
	app.ExpectFocused(option("row-0501"))
}

func TestListBoxAutoFocus(t *testing.T) {
	page := func(o aria.ListBoxOptions[string]) func() tela.Node {
		return func() tela.Node {
			o.Keys, o.TextValue, o.AriaLabel = fruits, capital, "Fruit"
			lb := aria.UseListBox(o)
			return Div(lb.Props, Range(len(fruits), func(i int) tela.Node {
				return Div(lb.ItemProps(fruits[i]), Key(fruits[i]), Class("h-8"), Text(capital(fruits[i])))
			}))
		}
	}
	app := telatest.MountFunc(t, page(aria.ListBoxOptions[string]{FocusStrategy: aria.FocusLast}), telatest.Size(300, 400))
	app.ExpectFocused(option("Fig"))
	app = telatest.MountFunc(t, page(aria.ListBoxOptions[string]{FocusStrategy: aria.FocusFirst}), telatest.Size(300, 400))
	app.ExpectFocused(option("Apple"))
	// A selected item wins over the strategy.
	o := aria.ListBoxOptions[string]{FocusStrategy: aria.FocusFirst, SelectionMode: "single", DefaultSelected: aria.SelectionOf("date")}
	app = telatest.MountFunc(t, page(o), telatest.Size(300, 400))
	app.ExpectFocused(option("Date"))
	// AutoFocus alone focuses the list itself, with no item.
	app = telatest.MountFunc(t, page(aria.ListBoxOptions[string]{AutoFocus: true}), telatest.Size(300, 400))
	app.ExpectFocused(telatest.ByRole("listbox", "Fruit"))
	app.Press("ArrowDown")
	app.ExpectFocused(option("Apple"))
}

func TestListBoxRemovedFocusedKey(t *testing.T) {
	page := func() tela.Node {
		keys, setKeys := tela.UseState(append([]string(nil), fruits...))
		lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: keys, TextValue: capital, AriaLabel: "Fruit"})
		focused, _ := lb.Focused()
		return Div(
			Div(lb.Props, OnKeyDown(func(e tela.KeyboardEvent) {
				if e.Key == tela.KeyDelete {
					var next []string
					for _, k := range keys {
						if k != focused {
							next = append(next, k)
						}
					}
					setKeys.Set(next)
				}
			}), Range(len(keys), func(i int) tela.Node {
				return Div(lb.ItemProps(keys[i]), Key(keys[i]), Class("h-8"), Text(capital(keys[i])))
			})))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	app.Focus(option("Cherry"))
	// The focused item goes: the focus moves to the item that takes its
	// place, and the keyboard goes on from there.
	app.Press("Delete")
	app.ExpectFocused(option("Date"))
	app.Press("ArrowUp")
	app.ExpectFocused(option("Blueberry"))
	if n := app.Count(option("Cherry")); n != 0 {
		t.Errorf("cherry still listed: %d", n)
	}
	// The last item goes: the one before it is focused.
	app.Press("End")
	app.Press("Delete")
	app.ExpectFocused(option("Elderberry"))
}

func TestListBoxOptionErrors(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("no panic for an unknown selection mode")
		}
		msg := fmt.Sprint(r)
		for _, want := range []string{"SelectionMode", `"singel"`, `did you mean "single"`} {
			if !strings.Contains(msg, want) {
				t.Errorf("message %q lacks %q", msg, want)
			}
		}
	}()
	telatest.MountFunc(t, func() tela.Node {
		aria.UseListBox(aria.ListBoxOptions[string]{Keys: fruits, SelectionMode: "singel"})
		return Div()
	})
}
