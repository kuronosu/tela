package h_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The examples print what a screen reader would see, the aria snapshot
// that `tela inspect` prints, because it needs no window.

func ExampleDiv() {
	card := Div(Class("flex flex-col gap-2 rounded-xl bg-white p-6 shadow-md"),
		H2(Class("text-lg font-semibold"), Text("Inbox")),
		P(Class("text-slate-500"), Textf("%d unread messages", 3)),
		Button(Class("rounded-md bg-indigo-600 px-4 py-2 text-white"), Text("Open")),
	)
	yaml, _ := telatest.AriaSnapshot(card)
	fmt.Println(yaml)
	// Output:
	// - heading "Inbox" [level=2]
	// - paragraph: 3 unread messages
	// - button "Open"
}

func ExampleMap() {
	type Todo struct {
		ID   int
		Text string
	}
	todos := []Todo{{1, "Write docs"}, {2, "Ship it"}}

	list := Div(Role("list"),
		Map(todos, func(t Todo) tela.Node {
			return Div(Key(t.ID), Role("listitem"), Text(t.Text))
		}),
	)
	yaml, _ := telatest.AriaSnapshot(list)
	fmt.Println(yaml)
	// Output:
	// - list:
	//   - listitem: Write docs
	//   - listitem: Ship it
}

func ExampleMapIndex() {
	steps := []string{"Mount", "Click", "Expect"}

	list := Div(Role("list"),
		MapIndex(steps, func(i int, s string) tela.Node {
			return Div(Key(s), Role("listitem"), Textf("%d. %s", i+1, s))
		}),
	)
	yaml, _ := telatest.AriaSnapshot(list)
	fmt.Println(yaml)
	// Output:
	// - list:
	//   - listitem: 1. Mount
	//   - listitem: 2. Click
	//   - listitem: 3. Expect
}

func ExampleIf() {
	screen := func(loading bool, err error) tela.Node {
		return Div(
			If(loading, P(Text("Loading..."))),
			If(err != nil, P(Class("text-red-600"), Text("Something failed"))),
			P(Text("Done")),
		)
	}
	yaml, _ := telatest.AriaSnapshot(screen(true, nil))
	fmt.Println(yaml)
	// Output:
	// - paragraph: Loading...
	// - paragraph: Done
}

func ExampleIfElse() {
	toggle := func(on bool) tela.Node {
		return Button(
			IfElse(on, Class("bg-indigo-600 text-white"), Class("bg-slate-200 text-slate-900")),
			AriaPressed(on),
			IfElse(on, Text("On"), Text("Off")),
		)
	}
	yaml, _ := telatest.AriaSnapshot(toggle(true))
	fmt.Println(yaml)
	// Output:
	// - button "On" [pressed]
}

func ExampleClass() {
	// A class is a Tailwind v4 class: bg-red-500 paints the box red.
	swatch := Div(Class("size-16 bg-red-500"))
	img, _ := telatest.Render(swatch, telatest.Size(64, 64))
	fmt.Println(img.RGBAAt(32, 32))
	// Output:
	// {251 44 54 255}
}

func ExampleStyle() {
	// Typed style props come from package tela and win over classes.
	swatch := Div(Class("size-16 bg-red-500"), Style(tela.Bg(tela.RGB(0, 128, 255))))
	img, _ := telatest.Render(swatch, telatest.Size(64, 64))
	fmt.Println(img.RGBAAt(32, 32))
	// Output:
	// {0 128 255 255}
}

func ExampleRole() {
	enabled := true
	toggle := Button(Role("switch"), AriaChecked(enabled), AriaLabel("Wi-Fi"),
		Class("h-6 w-11 rounded-full bg-indigo-600"))
	yaml, _ := telatest.AriaSnapshot(toggle)
	fmt.Println(yaml)
	// Output:
	// - switch "Wi-Fi" [checked]
}

func ExampleDisabled() {
	save := Button(Disabled(true), Class("disabled:opacity-50"), Text("Save"))
	yaml, _ := telatest.AriaSnapshot(save)
	fmt.Println(yaml)
	// Output:
	// - button "Save" [disabled]
}

func ExampleInput() {
	// A controlled field: the value comes from state, and Bind writes
	// every edit back to it.
	signup := func() tela.Node {
		email, setEmail := tela.UseState("ana@example.com")
		return Form(Class("flex flex-col gap-2"),
			Label(For("email"), Text("Email")),
			Input(Id("email"), Type("email"), Bind(email, setEmail),
				Class("rounded-md border border-slate-300 px-3 py-2")),
			Button(Class("rounded-md bg-indigo-600 px-4 py-2 text-white"), Text("Sign up")),
		)
	}
	yaml, _ := telatest.AriaSnapshot(tela.C(signup))
	fmt.Println(yaml)
	// Output:
	// - text: Email
	// - textbox "Email": ana@example.com
	// - button "Sign up"
}

func ExampleVirtualList() {
	// A million rows cost about as much as the ones in view.
	rows := VirtualList(1_000_000,
		func(int) float32 { return 40 },
		func(i int) tela.Node { return Div(Role("listitem"), Class("h-10"), Textf("Row %d", i)) },
		Class("h-[120px]"), Overscan(0))
	yaml, _ := telatest.AriaSnapshot(rows, telatest.Size(200, 120))
	fmt.Println(yaml)
	// Output:
	// - listitem: Row 0
	// - listitem: Row 1
	// - listitem: Row 2
}

func ExamplePortal() {
	// The dialog is rendered by the card's component, but shown above
	// the page; its content comes after the page's, as if appended to the
	// body.
	page := Div(
		Div(Class("relative z-10 overflow-hidden rounded-xl p-4"),
			Text("Card"),
			Portal(Div(Class("fixed inset-0 flex items-center justify-center bg-black/40"),
				Div(Role("dialog"), AriaLabel("Confirm"), Class("rounded-xl bg-white p-6"),
					Button(Text("OK"))))),
		),
		Button(Text("Footer")),
	)
	yaml, _ := telatest.AriaSnapshot(page)
	fmt.Println(yaml)
	// Output:
	// - text: Card
	// - button "Footer"
	// - dialog "Confirm":
	//   - button "OK"
}
