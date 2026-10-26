package h_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// TestLayoutEffectFocusRestructures covers a focus moved by a layout
// effect in the frame that mounts the tree: the second style and box pass
// of that frame must rebuild the structure of elements whose display
// changed with the new focus state, even though the first pass already
// built them in the same frame.
func TestLayoutEffectFocusRestructures(t *testing.T) {
	app := telatest.Mount(t, tela.C(func() tela.Node {
		ref := tela.UseElementRef()
		tela.UseLayoutEffect(func() func() {
			ref.Focus()
			return nil
		}, tela.Once)
		return Div(Class("group p-4"),
			Div(Class("hidden group-focus-within:block"), Text("Shown while focused")),
			Button(Ref(ref), Text("Target")),
		)
	}), telatest.Size(300, 200))
	app.ExpectFocused(telatest.ByRole("button", "Target"))
	app.ExpectText("Shown while focused")
}

// TestLayoutEffectFocusRestructuresLater is the same with a panel mounted
// by a later click, in a frame that is not the first one.
func TestLayoutEffectFocusRestructuresLater(t *testing.T) {
	panel := func() tela.Node {
		ref := tela.UseElementRef()
		tela.UseLayoutEffect(func() func() {
			ref.Focus()
			return nil
		}, tela.Once)
		return Div(Class("group p-4"),
			Div(Class("hidden group-focus-within:block"), Text("Shown while focused")),
			Button(Ref(ref), Text("Target")),
		)
	}
	app := telatest.Mount(t, tela.C(func() tela.Node {
		open, setOpen := tela.UseState(false)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setOpen.Set(true) }), Text("Open")),
			If(open, tela.C(panel)),
		)
	}), telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Open"))
	app.ExpectFocused(telatest.ByRole("button", "Target"))
	app.ExpectText("Shown while focused")
}

// TestLayoutEffectScrollIntoView is tela.UseLayoutEffect's documented
// idiom (ADR-176): a layout effect that scrolls the newest row into view
// shows it at the bottom of the list in the first frame (captured with
// NoSettle) and in the frame that adds a row, with a ScrollTo of a
// second list to its end doing the same.
func TestLayoutEffectScrollIntoView(t *testing.T) {
	app := telatest.Mount(t, tela.C(func() tela.Node {
		n, setN := tela.UseState(10)
		last, other := tela.UseElementRef(), tela.UseElementRef()
		tela.UseLayoutEffect(func() func() {
			last.ScrollIntoView()
			other.ScrollTo(0, 1e6)
			return nil
		}, n)
		list := func(id string, ref tela.ElementRef, lastRef bool) tela.Node {
			c := []tela.Child{TestID(id), Class("h-[200px] w-32 overflow-y-auto")}
			if !lastRef {
				c = append(c, Ref(ref))
			}
			for i := range n {
				row := []tela.Child{Key(i), TestID(fmt.Sprintf("%s-%d", id, i)), Class("h-[50px] border-b"), Textf("Row %d", i+1)}
				if lastRef && i == n-1 {
					row = append(row, Ref(ref))
				}
				c = append(c, Div(row...))
			}
			return Div(c...)
		}
		return Div(Class("flex flex-col gap-2 p-4"),
			Button(OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }), Text("Add")),
			Div(Class("flex gap-2"), list("into", last, true), list("to", other, false)),
		)
	}), telatest.Size(400, 300), telatest.NoSettle())
	check := func(n int, what string) {
		t.Helper()
		for _, id := range []string{"into", "to"} {
			list, row := app.Bounds(telatest.ByTestID(id)), app.Bounds(telatest.ByTestID(fmt.Sprintf("%s-%d", id, n-1)))
			if got, want := row.Y+row.Height, list.Y+list.Height; got != want {
				t.Errorf("%s: the newest row of %q ends at %g, the list at %g", what, id, got, want)
			}
		}
	}
	check(10, "first frame")
	app.Click(telatest.ByRole("button", "Add"))
	check(11, "the frame that adds a row")
	app.Click(telatest.ByRole("button", "Add"))
	check(12, "the frame that adds another")
}
