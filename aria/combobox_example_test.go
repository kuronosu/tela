package aria_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseCombobox() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		cb := aria.UseCombobox(aria.ComboboxOptions[string]{
			Items: fruits, TextValue: capital, AriaLabel: "Fruit", MatchWidth: true,
		})
		return Div(Class("p-4"),
			Div(Class("flex"),
				Input(cb.InputProps, Class("w-48 rounded-l-md border px-3 py-2")),
				Button(cb.ButtonProps, Class("rounded-r-md border px-2"), Text("v"))),
			Portal(If(cb.Open,
				Div(cb.PopoverProps, Class("rounded-md border bg-popover p-1 shadow-md"),
					Div(cb.ListBox.Props, Class("max-h-60 overflow-auto"),
						Range(len(cb.Keys), func(i int) tela.Node {
							k := cb.Keys[i]
							return Div(cb.ListBox.ItemProps(k), Key(k), Class("rounded px-2 py-1.5"),
								If(cb.ListBox.IsFocused(k), Class("bg-accent")), Text(capital(k)))
						}))))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.Focus(telatest.ByRole("combobox", "Fruit"))
	app.Type("blu") // filters and opens the list
	fmt.Println(app.AriaSnapshot())
	app.Press("ArrowDown") // highlights the item, the input keeps the focus
	app.Press("Enter")     // chooses it
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - combobox "Fruit" [expanded]: blu
	// - button "Show suggestions" [expanded]: v
	// - listbox "Fruit":
	//   - option "Blueberry"
	// - combobox "Fruit": Blueberry
	// - button "Show suggestions": v
}

func ExampleUseCombobox_allowsCustomValue() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		cb := aria.UseCombobox(aria.ComboboxOptions[string]{
			Items: fruits, TextValue: capital, AriaLabel: "Tag", AllowsCustomValue: true,
			OnSelectionChange: func(s aria.Selection[string]) { fmt.Println("selected:", s) },
		})
		return Input(cb.InputProps, Class("w-48 border"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.Focus(telatest.ByRole("combobox", "Tag"))
	app.Fill(telatest.ByRole("combobox", "Tag"), "kiwi")
	app.Press("Tab") // the text stays, and the selection is cleared
	fmt.Println(app.AriaSnapshot())
	// Output: - combobox "Tag": kiwi
}

func ExampleContains() {
	fmt.Println(aria.Contains("São Paulo", "ao p"))
	fmt.Println(aria.StartsWith("Zürich", "ZUR"))
	fmt.Println(aria.EndsWith("Zürich", "ch"), aria.EndsWith("Zürich", "ürix"))
	// Output:
	// true
	// true
	// true false
}
