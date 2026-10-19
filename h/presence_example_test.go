package h_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// exampleT stands in for the *testing.T of a test in the examples that
// mount an app to show what happens over time.
type exampleT struct {
	testing.TB // nil: the methods telatest uses are below
	cleanups   []func()
}

func (e *exampleT) Helper()                      {}
func (e *exampleT) Name() string                 { return "example" }
func (e *exampleT) Cleanup(f func())             { e.cleanups = append(e.cleanups, f) }
func (e *exampleT) Logf(string, ...any)          {}
func (e *exampleT) Errorf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Fatalf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Failed() bool                 { return false }
func (e *exampleT) close() {
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
}

// dialogDemo opens a dialog that scales and fades in, and out again.
func dialogDemo() tela.Node {
	open, setOpen := tela.UseState(false)
	return Div(Class("relative h-full p-4"),
		Button(OnClick(func(tela.MouseEvent) { setOpen.Set(true) }), Text("Open")),
		Presence(If(open, Div(Key("dialog"), Role("dialog"), AriaLabel("Saved"),
			Class("absolute inset-x-4 top-12 rounded-xl bg-white p-6 shadow-xl transition duration-200 ease-out "+
				"opacity-100 scale-100 starting:opacity-0 starting:scale-95"),
			Exit("opacity-0 scale-95"),
			Button(OnClick(func(tela.MouseEvent) { setOpen.Set(false) }), Text("Close")),
		))),
	)
}

func ExamplePresence() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.C(dialogDemo), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Open"))
	app.Advance(300 * time.Millisecond) // the entrance is over
	app.Click(telatest.ByRole("button", "Close"))
	app.Advance(100 * time.Millisecond)
	// Halfway out the dialog is still drawn, but inert: gone for the
	// semantics tree, the pointer and the focus. (Count settles the app,
	// which ends the exit; the dialog then unmounts.)
	fmt.Println(app.Count(telatest.ByRole("dialog", "")))
	// Output: 0
}

func ExampleExit() {
	t := &exampleT{}
	defer t.close()
	var style string
	toast := func() tela.Node {
		shown, setShown := tela.UseState(true)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setShown.Set(false) }), Text("Dismiss")),
			// The backdrop and the card each have their own exit; the
			// child unmounts when both transitions have ended.
			Presence(If(shown, Div(Key("toast"),
				Div(Class("pointer-events-none fixed inset-0 bg-black/20 transition-opacity duration-300"), Exit("opacity-0")),
				Div(Role("status"), Class("rounded-lg bg-slate-900 p-3 text-white transition duration-150"),
					Exit("opacity-0 translate-y-2"), Text("Saved"))))))
	}
	app := telatest.Mount(t, tela.C(toast))
	start := app.Now()
	app.Click(telatest.ByRole("button", "Dismiss")) // waits for the exit
	if app.Now()-start >= 300*time.Millisecond {
		style = "waited for the slower exit"
	}
	fmt.Println(style, app.Count(telatest.ByRole("status", "")))
	// Output: waited for the slower exit 0
}
