package telatest_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// t stands in for the *testing.T of a real test, which examples do not
// have: it records failures and lets the examples read like test bodies.
var t testing.TB = &fakeTB{name: "example"}

// Counter, used by the examples below, is a heading with the count and an
// Increment button that adds one.

func ExampleMount() {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))

	app.Click(telatest.ByRole("button", "Increment"))
	app.ExpectText("Count: 1")

	fmt.Println(app.AriaSnapshot())
	// Output:
	// - 'heading "Count: 1" [level=1]'
	// - button "Increment"
}

func ExampleApp_ExpectAria() {
	app := telatest.Mount(t, tela.C(Counter))
	app.Click(telatest.ByText("Increment"))

	// The expected snapshot may be indented like the code around it.
	app.ExpectAria(`
		- 'heading "Count: 1" [level=1]'
		- button "Increment"
	`)
	fmt.Println("matches")
	// Output: matches
}

func ExampleApp_Fill() {
	form := func() tela.Node {
		name, setName := tela.UseState("")
		return Div(
			Label(For("name"), Text("Name")),
			Input(Id("name"), Bind(name, setName)),
			P(Textf("Hello, %s!", name)),
		)
	}
	app := telatest.Mount(t, tela.C(form))

	app.Fill(telatest.ByRole("textbox", "Name"), "Ada")
	app.ExpectValue(telatest.ByRole("textbox", "Name"), "Ada")
	app.ExpectText("Hello, Ada!")
	fmt.Println(app.Count(telatest.ByText("Hello, Ada!")))
	// Output: 1
}

func ExampleApp_Press() {
	search := func() tela.Node {
		query, setQuery := tela.UseState("")
		submitted, setSubmitted := tela.UseState("")
		return Form(OnSubmit(func(tela.SubmitEvent) { setSubmitted.Set(query) }),
			Input(AriaLabel("Search"), Bind(query, setQuery)),
			P(Textf("Searched: %q", submitted)),
		)
	}
	app := telatest.Mount(t, tela.C(search))

	app.Focus(telatest.ByRole("textbox", "Search"))
	app.Type("tela")   // one key at a time
	app.Press("Enter") // submits the form
	app.ExpectText(`Searched: "tela"`)
	fmt.Println("ok")
	// Output: ok
}

func ExampleApp_Advance() {
	// The clock is manual: a long press fires when the app's time gets
	// to 500 ms, whatever the machine's speed.
	pressed := func() tela.Node {
		long, setLong := tela.UseState(false)
		return Button(OnLongPress(func(tela.PointerEvent) { setLong.Set(true) }),
			IfElse(long, Text("Held"), Text("Press and hold")))
	}
	app := telatest.Mount(t, tela.C(pressed))

	app.Hover(telatest.ByRole("button", ""))
	app.MouseDown()
	app.Advance(600 * time.Millisecond)
	app.MouseUp()
	app.ExpectText("Held")
	fmt.Println(app.AriaSnapshot())
	// Output: - button "Held"
}

func ExampleByRole() {
	fmt.Println(telatest.ByRole("button", "Increment"))
	fmt.Println(telatest.ByRole("heading", ""))
	// Output:
	// getByRole("button", name="Increment")
	// getByRole("heading")
}

func ExampleByText() {
	fmt.Println(telatest.ByText("Count"))
	fmt.Println(telatest.ByText("Save", telatest.Exact()))
	// Output:
	// getByText("Count")
	// getByText("Save", exact)
}

func ExampleByClass() {
	fmt.Println(telatest.ByClass("rounded-full", "shadow-sm"))
	// Output: getByClass("rounded-full shadow-sm")
}

func ExampleLocator_Filter() {
	rows := telatest.ByRole("button", "").Filter(telatest.HasText("System"))
	fmt.Println(rows)
	// Output: getByRole("button").filter(hasText="System")
}

func ExampleLocator_Nth() {
	fmt.Println(telatest.ByRole("listitem", "").Nth(0))
	fmt.Println(telatest.ByRole("listitem", "").Nth(-1)) // the last one
	// Output:
	// getByRole("listitem").nth(0)
	// getByRole("listitem").nth(-1)
}

func ExampleAriaSnapshot() {
	yaml, err := telatest.AriaSnapshot(tela.C(Counter))
	if err != nil {
		panic(err)
	}
	fmt.Println(yaml)
	// Output:
	// - 'heading "Count: 0" [level=1]'
	// - button "Increment"
}

func ExampleRender() {
	img, err := telatest.Render(Div(Class("size-8 bg-black")),
		telatest.Size(16, 16), telatest.Scale(2))
	if err != nil {
		panic(err)
	}
	fmt.Println(img.Bounds().Dx(), img.Bounds().Dy(), img.RGBAAt(2, 2))
	// Output: 32 32 {0 0 0 255}
}
