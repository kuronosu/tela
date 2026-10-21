package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Stepper is a struct component with state (ADR-088).
type Stepper struct {
	Label string
	Step  int
}

func (s Stepper) Build() tela.Node {
	n, setN := tela.UseState(0)
	return Div(Class("flex gap-2 p-4"),
		Button(OnClick(func(tela.MouseEvent) { setN.Set(n + s.Step) }), Text(s.Label)),
		Textf("Value %d", n))
}

// The root of Mount, Render, AriaSnapshot and LayoutJSON is a
// tela.Component: a struct component works as one directly.
func TestStructComponentRoots(t *testing.T) {
	app := telatest.Mount(t, Stepper{Label: "Add", Step: 2}, telatest.Size(300, 120))
	app.Click(telatest.ByRole("button", "Add"))
	app.ExpectText("Value 2")
	if _, err := telatest.Render(Stepper{Label: "Add"}, telatest.Size(300, 120)); err != nil {
		t.Errorf("Render: %v", err)
	}
	yaml, err := telatest.AriaSnapshot(Stepper{Label: "Add"}, telatest.Size(300, 120))
	if err != nil || !strings.Contains(yaml, `button "Add"`) {
		t.Errorf("AriaSnapshot: %q, %v", yaml, err)
	}
	js, err := telatest.LayoutJSON(Stepper{Label: "Add"}, telatest.Size(300, 120))
	if err != nil || !strings.Contains(string(js), `"component": "github.com/kuronosu/tela/telatest_test.Stepper"`) {
		t.Errorf("LayoutJSON does not name the struct component: %v\n%s", err, js)
	}
	// A Prop renders nothing: Render reports it, and Mount fails.
	if _, err := telatest.Render(Class("p-4")); err == nil || !strings.Contains(err.Error(), "the root is a Prop (Class)") {
		t.Errorf("Render(Prop): %v", err)
	}
	mustFail(t, func(tb testing.TB) { telatest.Mount(tb, Class("p-4")) }, "telatest: Mount: tela: the root is a Prop (Class)")
}
