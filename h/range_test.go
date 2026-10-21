package h_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/telatest"
)

// stateCell is a struct component with state of its own: it counts its
// clicks, so a test sees whether the state stays with the index.
type stateCell struct{ I int }

func (c stateCell) Build() tela.Node {
	n, setN := tela.UseState(0)
	return Button(OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }), Textf("cell %d: %d", c.I, n))
}

func TestRangeNodes(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Range(3, func(i int) tela.Node { return P(Textf("row %d", i)) }))
	}, telatest.Size(200, 200))
	if got, want := app.AriaSnapshot(), "- paragraph: row 0\n- paragraph: row 1\n- paragraph: row 2"; got != want {
		t.Errorf("snapshot %q, want %q", got, want)
	}
}

// A struct component needs no Fragment, and its state follows the index.
func TestRangeComponents(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		n, setN := tela.UseState(3)
		return Div(
			Button(OnClick(func() { setN.Set(5) }), Text("more")),
			Range(n, func(i int) tela.Component { return stateCell{i} }))
	}, telatest.Size(200, 300))
	app.Click(telatest.ByRole("button", "cell 1: 0"))
	app.Click(telatest.ByRole("button", "cell 1: 1"))
	app.ExpectRole("button", "cell 1: 2")
	app.Click(telatest.ByRole("button", "more"))
	if got := app.Count(telatest.ByRole("button", "cell 4: 0")); got != 1 {
		t.Errorf("the new cell 4 is missing:\n%s", app.AriaSnapshot())
	}
	app.ExpectRole("button", "cell 1: 2") // the state stayed with its index
}

type rangeNodeFunc func(int) tela.Node
type rangeCompFunc func(int) tela.Component

func TestRangeNamedFuncTypes(t *testing.T) {
	var a rangeNodeFunc = func(i int) tela.Node { return Span(Textf("a%d", i)) }
	var b rangeCompFunc = func(i int) tela.Component { return stateCell{i} }
	app := telatest.MountFunc(t, func() tela.Node { return Div(Range(2, a), Range(2, b)) }, telatest.Size(200, 200))
	for _, want := range []string{"a0", "a1", "cell 0: 0", "cell 1: 0"} {
		app.ExpectText(want)
	}
}

func TestRangeZeroAndNegative(t *testing.T) {
	render := func(i int) tela.Node { return Text("x") }
	app := telatest.MountFunc(t, func() tela.Node { return Div(Range(0, render), Text("end")) }, telatest.Size(100, 100))
	if got := app.AriaSnapshot(); got != "- text: end" {
		t.Errorf("snapshot %q", got)
	}
	// Dev mode panics with the fix; a release build renders nothing.
	msg := func() (m string) {
		defer func() {
			if r := recover(); r != nil {
				m, _ = r.(string)
			}
		}()
		Range(-1, render)
		return ""
	}()
	if !strings.Contains(msg, "Range(-1, ...)") || !strings.Contains(msg, "0 or more") {
		t.Errorf("Range(-1) panicked with %q", msg)
	}
	app = telatest.MountFunc(t, func() tela.Node { return Div(Range(-1, render), Text("end")) }, telatest.Size(100, 100), telatest.NoDev())
	if got := app.AriaSnapshot(); got != "- text: end" {
		t.Errorf("release mode: snapshot %q", got)
	}
}

func TestRangeNilRenderPanics(t *testing.T) {
	var render func(int) tela.Node
	defer func() {
		if r, _ := recover().(string); !strings.Contains(r, "Range(n, nil)") {
			t.Errorf("panic %q", r)
		}
	}()
	Range(2, render)
}

// A static range keeps no keys for the reconciler to miss: no warning,
// where Map without keys warns.
func TestRangeGivesNoMissingKeyWarning(t *testing.T) {
	warn := func(root func() tela.Node) []string {
		var warnings []string
		tr := core.NewTree(core.Config{Dev: true, Warn: func(m string) { warnings = append(warnings, m) }})
		tr.SetViewport(gfx.Size{W: 200, H: 200})
		tr.Mount(tela.C(root))
		tr.Frame()
		return warnings
	}
	if w := warn(func() tela.Node { return Div(Range(3, func(i int) tela.Node { return P(Textf("%d", i)) })) }); len(w) != 0 {
		t.Errorf("Range warned: %q", w)
	}
	if w := warn(func() tela.Node {
		return Div(Map([]int{1, 2, 3}, func(i int) tela.Node { return P(Textf("%d", i)) }))
	}); len(w) == 0 {
		t.Error("control: Map without keys should warn")
	}
}

// VirtualList rows may be struct components.
func TestVirtualListStructRows(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return VirtualList(1000, func(int) float32 { return 30 },
			func(i int) tela.Component { return stateCell{i} },
			Class("h-[150px]"))
	}, telatest.Size(200, 150))
	app.ExpectRole("button", "cell 0: 0")
	app.Click(telatest.ByRole("button", "cell 2: 0"))
	app.ExpectRole("button", "cell 2: 1")
	if got := app.Count(telatest.ByRole("listitem", "")); got < 5 || got > 12 {
		t.Errorf("%d rows built, want a window of 5 to 12", got)
	}
}

type rowNodeFunc func(int) tela.Node
type rowCompFunc func(int) tela.Component

// Existing code keeps compiling: node functions, function values and named
// func types all infer.
func TestVirtualListRenderShapes(t *testing.T) {
	node := func(i int) tela.Node { return Div(Textf("n%d", i)) }
	var named rowNodeFunc = node
	var namedComp rowCompFunc = func(i int) tela.Component { return stateCell{i} }
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("flex"),
			VirtualList(10, func(int) float32 { return 20 }, node, Class("h-[60px] w-20")),
			VirtualList(10, nil, named, ItemHeight(20), Class("h-[60px] w-20")),
			VirtualList(10, nil, namedComp, ItemHeight(30), Class("h-[60px] w-32")))
	}, telatest.Size(400, 100))
	app.ExpectText("n0")
	app.ExpectRole("button", "cell 0: 0")
}

func TestVirtualListNilAndPropPanics(t *testing.T) {
	panics := func(f func()) (m string) {
		defer func() {
			if r := recover(); r != nil {
				m, _ = r.(string)
			}
		}()
		f()
		return ""
	}
	var nilNode func(int) tela.Node
	var nilComp func(int) tela.Component
	for _, f := range []func(){
		func() { VirtualList(3, func(int) float32 { return 1 }, nilNode) },
		func() { VirtualList(3, func(int) float32 { return 1 }, nilComp) },
	} {
		if m := panics(f); !strings.Contains(m, "pass renderItem") {
			t.Errorf("a nil renderItem panicked with %q", m)
		}
	}
}
