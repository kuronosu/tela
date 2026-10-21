package h_test

import (
	"fmt"
	"slices"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// fruitList shows a list that a button reverses; each row animates to its
// new place.
func fruitList() tela.Node {
	items, setItems := tela.UseState([]string{"Apples", "Pears", "Plums"})
	return Div(Class("flex flex-col gap-2 p-4"),
		Button(OnClick(func(tela.MouseEvent) {
			r := slices.Clone(items)
			slices.Reverse(r)
			setItems.Set(r)
		}), Text("Reverse")),
		Map(items, func(name string) tela.Node {
			return Div(Key(name), AnimateLayout(), Class("h-10 rounded-lg bg-white px-3 shadow-sm"), Text(name))
		}),
	)
}

func ExampleAnimateLayout() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.C(fruitList))
	before := app.Bounds(telatest.ByText("Apples")).Y
	app.Click(telatest.ByRole("button", "Reverse"))
	// The row is laid out at the bottom at once and drawn sliding there;
	// telatest waits for the animation before it measures.
	after := app.Bounds(telatest.ByText("Apples")).Y
	fmt.Println(before, after)
	// Output: 48 144
}

// tabBar slides an underline to the selected tab: each tab renders the
// underline when it is selected, and the shared LayoutID moves it from
// the tab that lost it.
func tabBar() tela.Node {
	active, setActive := tela.UseState(0)
	tabs := []string{"Overview", "Activity", "Settings"}
	return Div(Class("flex gap-2 p-4"), MapIndex(tabs, func(i int, label string) tela.Node {
		return Button(Key(label), Class("relative px-3 py-2"), OnClick(func(tela.MouseEvent) { setActive.Set(i) }),
			Text(label),
			If(i == active, Div(LayoutID("underline"), Class("absolute inset-x-0 bottom-0 h-0.5 bg-indigo-500"))))
	}))
}

func ExampleLayoutID() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.C(tabBar))
	underline := telatest.ByClass("bg-indigo-500")
	x0 := app.Bounds(underline).X
	app.Click(telatest.ByRole("button", "Settings"))
	x1 := app.Bounds(underline).X
	fmt.Println(x0 < x1)
	// Output: true
}
