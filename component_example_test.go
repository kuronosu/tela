package tela_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Panel is a struct component: its fields are its props, Children holds
// what it wraps (several nodes through Fragment), and its state is its
// own.
type Panel struct {
	Title    string
	Children tela.Component
}

func (p Panel) Build() tela.Node {
	open, setOpen := tela.UseState(true)
	return Div(Class("rounded-xl border p-4"),
		Button(AriaExpanded(open), OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text(p.Title)),
		If(open, p.Children))
}

func ExampleComponent() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, Div(Class("flex flex-col gap-2"),
		Panel{Title: "Inbox", Children: Fragment(P(Text("2 new")), P(Text("from Ada")))},
		Panel{Title: "Archive", Children: P(Text("empty"))}))
	// Each Panel has its own state: this closes the first one only.
	app.Click(telatest.ByRole("button", "Inbox"))
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Inbox"
	// - button "Archive" [expanded]
	// - paragraph: empty
}

// Badge counts its builds, to show when they are skipped. Its fields are
// comparable (a pointer compares by identity; a map or a slice field
// would make every build count as a change).
type Badge struct {
	Label  string
	Count  int
	builds *map[string]int
}

func (b Badge) Build() tela.Node {
	(*b.builds)[b.Label]++
	return Span(Class("rounded-full bg-indigo-500 px-2 text-white"), Textf("%s %d", b.Label, b.Count))
}

func ExampleComponent_skipping() {
	t := &exampleT{}
	defer t.close()
	counts := map[string]int{}
	builds := &counts
	page := func() tela.Node {
		clicks, setClicks := tela.UseState(0)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setClicks.Set(clicks + 1) }), Text("Click")),
			Badge{Label: "Inbox", Count: 3, builds: builds},       // equal on every build: skipped
			Badge{Label: "Clicks", Count: clicks, builds: builds}, // changes: built again
		)
	}
	app := telatest.Mount(t, tela.C(page))
	app.Click(telatest.ByRole("button", "Click"))
	app.Click(telatest.ByRole("button", "Click"))
	fmt.Println(counts["Inbox"], counts["Clicks"])
	// Output: 1 3
}
