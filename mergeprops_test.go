package tela_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Several props where one is taken: a kit component's Props field.
func ExampleMergeProps() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return Div(
			Button(Class("rounded border px-3"), Text("Save"),
				tela.MergeProps(
					TestID("save"),
					AriaLabel("Save the draft"),
					OnClick(func() { fmt.Println("clicked") }),
					OnClick(func() { fmt.Println("and again") }),
				)))
	}
	app := telatest.Mount(t, tela.C(page))
	app.Click(telatest.ByTestID("save"))
	app.ExpectRole("button", "Save the draft")
	// Output:
	// clicked
	// and again
}

func TestMergePropsSkipsHolesAndAppliesInOrder(t *testing.T) {
	page := func() tela.Node {
		return Div(Button(Text("x"), tela.MergeProps(nil, tela.Prop{}, AriaLabel("first"), tela.Node{}, AriaLabel("second"))))
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectRole("button", "second")
}

func TestMergePropsRejectsANodeInDevMode(t *testing.T) {
	page := func() tela.Node {
		return Div(Button(Text("x"), tela.MergeProps(AriaLabel("a"), Span(Text("child")))))
	}
	_, err := telatest.Render(tela.C(page))
	if err == nil || !strings.Contains(err.Error(), "MergeProps argument 1") || !strings.Contains(err.Error(), "not a prop") {
		t.Errorf("dev mode: error %v, want the argument and the fix reported", err)
	}
	// Outside dev mode it is ignored.
	app := telatest.Mount(t, tela.C(page), telatest.NoDev())
	app.ExpectRole("button", "a")
}
