package h_test

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleClass_parts() {
	// Parts are joined with one space and empty ones are skipped: no
	// concatenation, no trailing-space convention.
	tab := func(active bool) tela.Node {
		tone := "bg-white text-slate-500"
		if active {
			tone = "bg-indigo-600 text-white"
		}
		return Div(Class("size-16 rounded-lg", "", tone))
	}
	img, _ := telatest.Render(tab(true), telatest.Size(64, 64))
	fmt.Println(img.RGBAAt(32, 32))
	// Output: {79 57 246 255}
}

func ExampleOnClick() {
	counter := func() tela.Node {
		n, setN := tela.UseState(0)
		return Div(
			// Without the event: most handlers only set state.
			Button(OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }), Text("Add")),
			// With it, when the handler reads it.
			Button(OnClick(func(e tela.MouseEvent) { setN.Set(int(e.X)) }), Text("At x")),
			Textf("n=%d", n))
	}
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.C(counter), telatest.Size(300, 100))
	app.Click(telatest.ByRole("button", "Add"))
	app.Click(telatest.ByRole("button", "Add"))
	app.ExpectText("n=2")
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Add"
	// - button "At x"
	// - text: n=2
}

func ExampleBind_func() {
	type action struct{ text string }
	form := func() tela.Node {
		text, d := tela.UseReducer(func(_ string, a action) string { return a.text }, "")
		// A func(string) binds as well as a Setter: here, a Dispatcher.
		return Div(
			Input(AriaLabel("Name"), Bind(text, func(s string) { d.Dispatch(action{s}) })),
			P(Textf("Hello, %s", text)))
	}
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.C(form), telatest.Size(300, 100))
	app.Fill(telatest.ByRole("textbox", "Name"), "Ada")
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - textbox "Name": Ada
	// - paragraph: Hello, Ada
}

// todoItem is a struct component with state of its own.
type todoItem struct {
	ID    int
	Title string
}

func (t todoItem) Build() tela.Node {
	done, setDone := tela.UseState(false)
	return Button(AriaChecked(done), Role("checkbox"), OnClick(func() { setDone.Update(func(d bool) bool { return !d }) }), Text(t.Title))
}

func ExampleMapKeyed() {
	t := &exampleT{}
	defer t.close()
	var set tela.Setter[[]todoItem]
	list := func() tela.Node {
		todos, s := tela.UseState([]todoItem{{1, "Write"}, {2, "Test"}})
		set = s
		return Div(MapKeyed(todos, func(t todoItem) int { return t.ID }, func(t todoItem) tela.Component { return t }))
	}
	app := telatest.Mount(t, tela.C(list))
	app.Click(telatest.ByRole("checkbox", "Test")) // checks "Test"
	// A new item arrives first: "Test" keeps its state.
	set.Set([]todoItem{{3, "Plan"}, {1, "Write"}, {2, "Test"}})
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - checkbox "Plan"
	// - checkbox "Write"
	// - checkbox "Test" [checked]
}

func ExampleNodes() {
	var cells []tela.Node
	for _, name := range []string{"Name", "Role", "Team"} {
		cells = append(cells, Div(Key(name), Class("px-3"), Text(name)))
	}
	// A slice cannot be spread into Div(...): Nodes makes it one child.
	header := Div(Class("flex border-b"), Nodes(cells))
	yaml, _ := telatest.AriaSnapshot(header)
	fmt.Println(yaml)
	// Output: - text: Name Role Team
}

func ExampleScale() {
	t := &exampleT{}
	defer t.close()
	var open *tela.Value[float32]
	section := func() tela.Node {
		open = tela.UseValue(float32(0))
		// 36 px closed, 72 px at 1: the box is laid out again when open
		// changes; the component does not build.
		return Div(TestID("body"), Class("w-40 overflow-hidden bg-slate-100"), Style(tela.HeightFn(Scale(open, 72))))
	}
	app := telatest.Mount(t, tela.C(section))
	open.Set(0.5)
	app.Advance(32 * time.Millisecond)
	fmt.Println(app.Bounds(telatest.ByTestID("body")).Height)
	// Output: 36
}

func ExampleItemKey() {
	t := &exampleT{}
	defer t.close()
	var set tela.Setter[[]todoItem]
	list := func() tela.Node {
		todos, s := tela.UseState([]todoItem{{1, "Write"}, {2, "Test"}})
		set = s
		return VirtualList(len(todos), func(int) float32 { return 30 },
			func(i int) tela.Node { return Fragment(todos[i]) },
			// The state of an item follows its ID, not its position.
			ItemKey(func(i int) int { return todos[i].ID }), Class("h-24"))
	}
	app := telatest.Mount(t, tela.C(list), telatest.Size(200, 100))
	app.Click(telatest.ByRole("checkbox", "Test"))
	set.Set([]todoItem{{3, "Plan"}, {1, "Write"}, {2, "Test"}})
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - list:
	//   - listitem:
	//     - checkbox "Plan"
	//   - listitem:
	//     - checkbox "Write"
	//   - listitem:
	//     - checkbox "Test" [checked]
}

func ExampleItemHeight() {
	// Rows of one height need no estimate and are never measured.
	rows := VirtualList(1_000_000, nil,
		func(i int) tela.Node { return Div(Class("truncate"), Textf("Row %d", i)) },
		ItemHeight(40), Class("h-[120px]"), Overscan(0))
	yaml, _ := telatest.AriaSnapshot(rows, telatest.Size(200, 120))
	fmt.Println(yaml)
	// Output:
	// - list:
	//   - listitem: Row 0
	//   - listitem: Row 1
	//   - listitem: Row 2
}
