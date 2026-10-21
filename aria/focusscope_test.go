package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// dialogPage has an opener, a button after it, and a dialog (a focus scope
// that contains, restores and auto-focuses) that the opener shows.
func dialogPage() tela.Node {
	open, setOpen := tela.UseState(false)
	return Div(Class("flex flex-col gap-2"),
		Button(TestID("opener"), OnClick(func() { setOpen.Set(true) }), Text("Open")),
		If(open, tela.C(dialog(func() { setOpen.Set(false) }))),
		Button(TestID("after"), Text("After")))
}

func dialog(onClose func()) func() tela.Node {
	return func() tela.Node {
		scope := aria.UseFocusScope(aria.FocusScopeOptions{Contain: true, Restore: true, AutoFocus: true})
		return Div(scope.Props, Role("dialog"), AriaLabel("Settings"), Class("flex flex-col"),
			Button(TestID("first"), Text("First")),
			Button(TestID("second"), Text("Second")),
			Button(TestID("close"), OnClick(onClose), Text("Close")))
	}
}

func TestFocusScopeAutoFocusContainAndRestore(t *testing.T) {
	app := telatest.MountFunc(t, dialogPage, telatest.Size(300, 300))
	app.Focus(telatest.ByTestID("opener"))
	app.Press("Enter") // opens the dialog
	app.ExpectFocused(telatest.ByTestID("first"))

	// Tab stays inside, wrapping around in both directions.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("second"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("close"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("first"))
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByTestID("close"))

	// Closing gives the focus back to the opener.
	app.Press("Enter")
	app.ExpectFocused(telatest.ByTestID("opener"))
}

// An auto-focus scope leaves the focus alone when it is already inside.
func TestFocusScopeAutoFocusKeepsFocusInside(t *testing.T) {
	page := func() tela.Node {
		scope := aria.UseFocusScope(aria.FocusScopeOptions{AutoFocus: true})
		return Div(scope.Props,
			Button(TestID("a"), Text("A")),
			Button(TestID("b"), AutoFocus(true), Text("B")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.ExpectFocused(telatest.ByTestID("b"))
}

// toolbar is a row of buttons driven by the focus manager: the arrows move
// the focus, Home and End go to the ends, and a button with TabIndex(-1)
// is only reachable when Tabbable is not asked.
func toolbarPage(wrap, tabbable bool, result *[]bool) func() tela.Node {
	return func() tela.Node {
		scope := aria.UseFocusScope(aria.FocusScopeOptions{})
		o := aria.FocusOptions{Wrap: wrap, Tabbable: tabbable}
		return Div(scope.Props, Role("toolbar"), Class("flex gap-2"),
			OnKeyDown(func(e tela.KeyboardEvent) {
				var ok bool
				switch e.Key {
				case tela.KeyArrowRight:
					ok = scope.Manager.FocusNext(o)
				case tela.KeyArrowLeft:
					ok = scope.Manager.FocusPrevious(o)
				case tela.KeyHome:
					ok = scope.Manager.FocusFirst(o)
				case tela.KeyEnd:
					ok = scope.Manager.FocusLast(o)
				default:
					return
				}
				e.PreventDefault()
				*result = append(*result, ok)
			}),
			Button(TestID("a"), Text("A")),
			Button(TestID("b"), TabIndex(-1), Text("B")),
			Button(TestID("c"), Text("C")))
	}
}

func TestFocusManagerWrap(t *testing.T) {
	var res []bool
	app := telatest.MountFunc(t, toolbarPage(true, false, &res), telatest.Size(300, 100))
	app.Focus(telatest.ByTestID("a"))
	app.Press("ArrowRight")
	app.ExpectFocused(telatest.ByTestID("b")) // not tabbable-only: B counts
	app.Press("ArrowRight")
	app.ExpectFocused(telatest.ByTestID("c"))
	app.Press("ArrowRight") // wraps
	app.ExpectFocused(telatest.ByTestID("a"))
	app.Press("ArrowLeft") // wraps back
	app.ExpectFocused(telatest.ByTestID("c"))
	app.Press("Home")
	app.ExpectFocused(telatest.ByTestID("a"))
	app.Press("End")
	app.ExpectFocused(telatest.ByTestID("c"))
	for i, ok := range res {
		if !ok {
			t.Errorf("move %d reported no element", i)
		}
	}
}

func TestFocusManagerWithoutWrapAndTabbable(t *testing.T) {
	var res []bool
	app := telatest.MountFunc(t, toolbarPage(false, true, &res), telatest.Size(300, 100))
	app.Focus(telatest.ByTestID("a"))
	app.Press("ArrowRight")
	app.ExpectFocused(telatest.ByTestID("c")) // B is not tabbable
	app.Press("ArrowRight")                   // no wrap: stays, and says so
	app.ExpectFocused(telatest.ByTestID("c"))
	app.Press("Home")
	app.Press("ArrowLeft")
	app.ExpectFocused(telatest.ByTestID("a"))
	want := []bool{true, false, true, false}
	if len(res) != len(want) {
		t.Fatalf("moves %v, want %v", res, want)
	}
	for i := range want {
		if res[i] != want[i] {
			t.Errorf("moves %v, want %v", res, want)
			break
		}
	}
}

// From starts the move at another element than the focused one.
func TestFocusManagerFrom(t *testing.T) {
	var scope aria.FocusScopeResult
	var a tela.ElementRef
	page := func() tela.Node {
		scope = aria.UseFocusScope(aria.FocusScopeOptions{})
		a = tela.UseElementRef()
		return Div(scope.Props,
			Button(Ref(a), TestID("a"), Text("A")),
			Button(TestID("b"), Text("B")),
			Button(TestID("c"), Text("C")),
			Button(TestID("out"), Text("Out")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.Focus(telatest.ByTestID("c"))
	if !scope.Manager.FocusNext(aria.FocusOptions{From: a}) {
		t.Fatal("no element after A")
	}
	app.ExpectFocused(telatest.ByTestID("b"))
	if scope.Manager.FocusNext(aria.FocusOptions{From: tela.ElementRef{}}) != true {
		t.Error("from the focused element: none after B")
	}
	var zero aria.FocusManager
	if zero.FocusNext(aria.FocusOptions{}) || zero.FocusFirst(aria.FocusOptions{}) {
		t.Error("the zero manager moved the focus")
	}
}
