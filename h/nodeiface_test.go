package h_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Tests of ADR-205: a struct component is a tela.Node, so it is returned
// from functions and callbacks without a Fragment, and a Child (h.If) is
// returned where a Node is expected.

// busy returns If(...) from a function that returns a tela.Node: a Child
// is assignable to a Node.
func busy(on bool) tela.Node { return If(on, stateCell{9}) }

// label returns a struct component from a plain function.
func label(i int) tela.Node { return stateCell{i} }

// panel composes another struct component from its Build.
type panel struct{ N int }

func (p panel) Build() tela.Node { return stateCell{p.N} }

func TestStructComponentReturnedFromFunctions(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(true)
		return Div(
			Button(OnClick(func() { setOn.Set(!on) }), Text("toggle")),
			busy(on),
			label(1),
			panel{2},
			Map([]int{3}, func(i int) tela.Node { return stateCell{i} }),
			MapKeyed([]int{4}, func(i int) int { return i }, func(i int) tela.Node { return stateCell{i} }),
			Range(1, func(i int) tela.Node { return stateCell{5 + i} }),
			Div(Class("h-10"), VirtualList(1, nil, func(i int) tela.Node { return stateCell{6 + i} }, ItemHeight(20), Class("h-10"))),
		)
	}, telatest.Size(300, 400))
	for _, want := range []string{"cell 9: 0", "cell 1: 0", "cell 2: 0", "cell 3: 0", "cell 4: 0", "cell 5: 0", "cell 6: 0"} {
		app.ExpectRole("button", want)
	}
	// Each is a component of its own: clicking one keeps the others.
	app.Click(telatest.ByRole("button", "cell 2: 0"))
	app.ExpectRole("button", "cell 2: 1")
	// If(false, ...) returned from busy is nil, a hole: the siblings after
	// it keep their state.
	app.Click(telatest.ByRole("button", "toggle"))
	app.ExpectCount(telatest.ByRole("button", "cell 9: 0"), 0)
	app.ExpectRole("button", "cell 2: 1")
	if busy(false) != nil {
		t.Error("busy(false) is not nil")
	}
}

func TestPropReturnedAsNodePanics(t *testing.T) {
	for name, f := range map[string]func() tela.Node{
		"Range": func() tela.Node { return Range(1, func(int) tela.Node { return Class("x") }) },
		"MapKeyed": func() tela.Node {
			return MapKeyed([]int{1}, func(i int) int { return i }, func(int) tela.Node { return Class("x") })
		},
		"h.Nodes": func() tela.Node { return Nodes([]tela.Node{Text("a"), Class("x")}) },
	} {
		msg := func() (m string) {
			defer func() { m, _ = recover().(string) }()
			f()
			return ""
		}()
		if !strings.Contains(msg, "of "+name+" is a Prop (Class), not a node or a component") {
			t.Errorf("%s: panic %q", name, msg)
		}
	}
}
