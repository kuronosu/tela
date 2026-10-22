package aria_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseSelect() {
	t := &exampleT{}
	defer t.close()
	names := []string{"apple", "banana", "cherry"}
	page := func() tela.Node {
		sel := aria.UseSelect(aria.SelectOptions[string]{
			Keys: names, TextValue: capital, Placeholder: "Pick a fruit", MatchWidth: true,
		})
		return Div(Class("flex flex-col gap-1 p-4"),
			Span(sel.LabelProps, Class("text-sm font-medium"), Text("Favorite fruit")),
			Button(sel.TriggerProps, Class("flex w-48 justify-between rounded-md border px-3 py-2"),
				Span(sel.ValueProps, Text(sel.Value))),
			Portal(If(sel.Open,
				Div(sel.PopoverProps, Class("rounded-md border bg-popover p-1 shadow-md"),
					Div(sel.ListBox.Props, Class("max-h-60 overflow-auto"),
						Range(len(names), func(i int) tela.Node {
							k := names[i]
							return Div(sel.ListBox.ItemProps(k), Key(k),
								Class("rounded px-2 py-1.5 focus-visible:bg-accent aria-selected:font-medium"),
								Text(capital(k)))
						}))))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	fmt.Println(app.AriaSnapshot())
	app.Focus(telatest.ByRole("button", "Pick a fruit Favorite fruit"))
	app.Press("ArrowDown") // opens it on the first item
	app.Press("ArrowDown")
	fmt.Println(app.AriaSnapshot())
	app.Press("Enter") // selects Banana, closes and gives the focus back
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - text: Favorite fruit
	// - button "Pick a fruit Favorite fruit": Pick a fruit
	// - text: Favorite fruit
	// - button "Pick a fruit Favorite fruit" [expanded]: Pick a fruit
	// - listbox "Favorite fruit":
	//   - option "Apple"
	//   - option "Banana"
	//   - option "Cherry"
	// - text: Favorite fruit
	// - button "Banana Favorite fruit": Banana
}

func ExampleUseSelect_controlled() {
	t := &exampleT{}
	defer t.close()
	names := []string{"apple", "banana", "cherry"}
	page := func() tela.Node {
		sel, setSel := tela.UseState(aria.SelectionOf("banana"))
		s := aria.UseSelect(aria.SelectOptions[string]{
			Keys: names, TextValue: capital, AriaLabel: "Fruit",
			Selected: sel, OnSelectionChange: setSel.Set,
		})
		return Div(
			Button(s.TriggerProps, Span(s.ValueProps, Text(s.Value))),
			Portal(If(s.Open, Div(s.PopoverProps, Div(s.ListBox.Props,
				Range(len(names), func(i int) tela.Node {
					return Div(s.ListBox.ItemProps(names[i]), Key(names[i]), Text(capital(names[i])))
				}))))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Banana Fruit"))
	app.Click(telatest.ByRole("option", "Cherry"))
	fmt.Println(app.AriaSnapshot())
	// Output: - button "Cherry Fruit": Cherry
}
