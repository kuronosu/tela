package aria_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// exampleTB stands in for the *testing.T of a real test, which examples do
// not have.
type exampleTB struct {
	testing.TB // nil: the methods telatest uses are overridden below
	cleanups   []func()
}

func (*exampleTB) Helper()      {}
func (*exampleTB) Name() string { return "example" }
func (*exampleTB) Failed() bool { return false }
func (*exampleTB) Fail()        {}
func (*exampleTB) FailNow()     { runtime.Goexit() }
func (e *exampleTB) Cleanup(f func()) {
	e.cleanups = append(e.cleanups, f)
}
func (*exampleTB) Errorf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }
func (*exampleTB) Fatalf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }
func (*exampleTB) Logf(string, ...any)               {}
func (*exampleTB) Log(...any)                        {}

func ExampleUseFloating() {
	// A menu below its button, offset by 4 px, flipped above it when the
	// window has no room below and kept 8 px from the window's edges.
	menu := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor,
			aria.Placement("bottom-start"), aria.Offset(4), aria.Flip(), aria.Shift(8))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("button"), Class("absolute left-4 top-[260px] h-8 w-24"), Text("Menu")),
			Portal(Div(fl.Props, TestID("menu"), Class("h-24 w-48 rounded-md border bg-white"),
				Textf("placement: %s", fl.Placement))))
	}
	app := telatest.Mount(&exampleTB{}, tela.C(menu), telatest.Size(400, 300))

	button := app.Bounds(telatest.ByTestID("button"))
	pop := app.Bounds(telatest.ByTestID("menu"))
	fmt.Printf("button y %.0f to %.0f\n", button.Y, button.Bottom())
	fmt.Printf("menu y %.0f to %.0f, above the button\n", pop.Y, pop.Bottom())
	app.ExpectText("placement: top-start")
	// Output:
	// button y 260 to 292
	// menu y 160 to 256, above the button
}

func ExampleSize() {
	// A long list limited to the room below its button: the popover is as
	// tall as the window allows, 8 px from the bottom, and scrolls.
	list := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Offset(4),
			aria.Size(func(a aria.Available) {
				a.MaxHeight(a.Height)
			}, aria.SizeOptions{Padding: 8}))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), Class("absolute left-4 top-[50px] h-8 w-24"), Text("Open")),
			Portal(Div(fl.Props, TestID("list"), Class("flex w-40 flex-col overflow-y-auto bg-white"),
				Range(40, func(i int) tela.Node {
					return Div(Class("h-8 shrink-0"), Textf("Item %d", i))
				}))))
	}
	app := telatest.Mount(&exampleTB{}, tela.C(list), telatest.Size(400, 300))

	r := app.Bounds(telatest.ByTestID("list"))
	fmt.Printf("list y %.0f to %.0f of 300\n", r.Y, r.Bottom())
	// Output:
	// list y 86 to 292 of 300
}

func ExampleArrow() {
	// An arrow that points at the button even though the popover shifted to
	// stay inside the window.
	tip := func() tela.Node {
		anchor, arrow := tela.UseElementRef(), tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Offset(8), aria.Flip(), aria.Shift(8), aria.Arrow(arrow))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("button"), Class("absolute left-[340px] top-10 h-8 w-10"), Text("?")),
			Portal(Div(fl.Props, TestID("tip"), Class("h-16 w-48 rounded-md bg-slate-800"),
				Div(Ref(arrow), TestID("arrow"), fl.ArrowProps, Class("size-4 rotate-45 bg-slate-800")))))
	}
	app := telatest.Mount(&exampleTB{}, tela.C(tip), telatest.Size(400, 300))

	button := app.Bounds(telatest.ByTestID("button"))
	arrow := app.Bounds(telatest.ByTestID("arrow"))
	fmt.Printf("button center x %.0f\n", button.X+button.Width/2)
	fmt.Printf("arrow center x %.0f\n", arrow.X+arrow.Width/2)
	// Output:
	// button center x 360
	// arrow center x 360
}

func ExampleAnchorRect() {
	// A context menu at the pointer: a virtual anchor, the zero ElementRef.
	var pointer *tela.Value[tela.Rect]
	menu := func() tela.Node {
		pointer = tela.UseValue(tela.Rect{X: 120, Y: 80})
		fl := aria.UseFloating(tela.ElementRef{},
			aria.Placement("bottom-start"), aria.Flip(), aria.Shift(4),
			aria.AnchorRect(pointer.Get))
		return Div(Class("h-full"),
			Portal(Div(fl.Props, TestID("menu"), Class("h-24 w-40 rounded-md bg-white"))))
	}
	app := telatest.Mount(&exampleTB{}, tela.C(menu), telatest.Size(400, 300))

	r := app.Bounds(telatest.ByTestID("menu"))
	fmt.Printf("at the pointer: x %.0f y %.0f\n", r.X, r.Y)

	pointer.Set(tela.Rect{X: 200, Y: 40})
	app.WaitFor("the menu to follow the pointer", func() bool {
		return app.Bounds(telatest.ByTestID("menu")).X == 200
	})
	r = app.Bounds(telatest.ByTestID("menu"))
	fmt.Printf("moved: x %.0f y %.0f\n", r.X, r.Y)
	// Output:
	// at the pointer: x 120 y 80
	// moved: x 200 y 40
}
