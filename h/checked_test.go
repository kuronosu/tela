package h_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// checked: (and peer-checked:) match an element whose AriaChecked is
// true (ADR-100), and only that one: not AriaChecked(false), not an
// element without the prop; the change restyles only the elements whose
// classes test it.
func TestCheckedVariantMatchesAriaChecked(t *testing.T) {
	page := func() tela.Node {
		on, setOn := tela.UseState(false)
		return Div(Class("flex flex-col"),
			Button(OnClick(tela.Toggle(setOn)), Text("Toggle")),
			Div(TestID("own"), Role("checkbox"), AriaChecked(on), Class("peer h-4 w-10 checked:w-20")),
			Div(TestID("sibling"), Class("h-4 w-10 peer-checked:w-20")),
			Div(TestID("plain"), Class("h-4 w-10 checked:w-20")),
			Div(TestID("off"), Role("checkbox"), AriaChecked(false), Class("h-4 w-10 checked:w-20")),
			Div(TestID("idle"), Class("h-4 w-10")),
			Div(TestID("idle2"), Class("h-4 w-10")),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	width := func(id string) float32 { return app.Bounds(telatest.ByTestID(id)).Width }
	if width("own") != 40 || width("sibling") != 40 {
		t.Errorf("unchecked: own %g, sibling %g, want 40", width("own"), width("sibling"))
	}
	st := app.Measure(func() { app.Click(telatest.ByRole("button", "Toggle")) })
	if width("own") != 80 || width("sibling") != 80 {
		t.Errorf("checked: own %g, sibling %g, want 80", width("own"), width("sibling"))
	}
	if width("plain") != 40 || width("off") != 40 {
		t.Errorf("an element that is not checked matched: plain %g, off %g", width("plain"), width("off"))
	}
	if st.Styles != 2 {
		t.Errorf("the click restyled %d elements, want 2 (the checkbox and its peer sibling)", st.Styles)
	}
	app.Click(telatest.ByRole("button", "Toggle"))
	if width("own") != 40 || width("sibling") != 40 {
		t.Errorf("unchecked again: own %g, sibling %g", width("own"), width("sibling"))
	}
}
