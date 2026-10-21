package h_test

import (
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
