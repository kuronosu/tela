package telatest_test

import (
	"reflect"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// MouseBack and MouseForward click with the mouse's side buttons: the
// element gets pointer and mouse events with that button, and no click.
func TestClickWithTheSideButtons(t *testing.T) {
	var got []string
	log := func(what string, b tela.MouseButton) { got = append(got, what+":"+b.String()) }
	page := func() tela.Node {
		return Div(TestID("area"), Class("h-20 w-40 bg-slate-200"),
			OnPointerDown(func(e tela.PointerEvent) { log("pointerdown", e.Button) }),
			OnPointerUp(func(e tela.PointerEvent) { log("pointerup", e.Button) }),
			OnMouseDown(func(e tela.MouseEvent) { log("mousedown", e.Button) }),
			OnMouseUp(func(e tela.MouseEvent) { log("mouseup", e.Button) }),
			OnClick(func() { got = append(got, "click") }))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(200, 100))
	app.Click(telatest.ByTestID("area"), telatest.MouseBack())
	app.ClickAt(20, 20, telatest.MouseForward())
	want := []string{
		"pointerdown:back", "mousedown:back", "pointerup:back", "mouseup:back",
		"pointerdown:forward", "mousedown:forward", "pointerup:forward", "mouseup:forward",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
}
