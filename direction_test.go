package tela_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseDirection() {
	arrow := func() tela.Node {
		next := "ArrowRight"
		if tela.UseDirection() == "rtl" {
			next = "ArrowLeft"
		}
		return h.P(h.Textf("Next: %s", next))
	}
	app := func() tela.Node {
		return h.Div(tela.C(arrow), h.Div(h.Dir("rtl"), tela.C(arrow)))
	}
	yaml, _ := telatest.AriaSnapshot(tela.C(app))
	fmt.Println(yaml)
	// Output:
	// - paragraph: "Next: ArrowRight"
	// - paragraph: "Next: ArrowLeft"
}

// TestUseDirectionPublic: the hook reads the direction at the component's
// position (nested Dir props, a Portal declared in a right-to-left
// subtree, dir="auto" text), a click that changes a Dir builds again the
// readers below it and no other component, and a click that changes the
// text of an auto element flips its readers within the frame.
func TestUseDirectionPublic(t *testing.T) {
	reads := map[string][]string{}
	reader := func(name string) tela.Component {
		return tela.CP(func(string) tela.Node {
			reads[name] = append(reads[name], tela.UseDirection())
			return h.Div()
		}, name)
	}
	others := 0
	other := tela.CP(func(int) tela.Node { others++; return h.Div() }, 0)
	app := func() tela.Node {
		dir, setDir := tela.UseState("ltr")
		word, setWord := tela.UseState("hello")
		return h.Div(
			h.Button(h.OnClick(func(tela.MouseEvent) { setDir.Set("rtl") }), h.Text("Right to left")),
			h.Button(h.OnClick(func(tela.MouseEvent) { setWord.Set("مرحبا") }), h.Text("Arabic")),
			reader("top"),
			h.Div(h.Dir(dir),
				reader("switched"),
				h.Div(h.Dir("ltr"), reader("pinned")),
				h.Portal(reader("portal")),
				other,
			),
			h.Div(h.Dir("auto"), h.Span(h.Text(word)), reader("auto")),
		)
	}
	a := telatest.Mount(t, tela.C(app))
	defer a.Close()
	last := func(name string) string { r := reads[name]; return r[len(r)-1] }
	expect := func(when, name string, builds int, dir string) {
		t.Helper()
		if len(reads[name]) != builds || last(name) != dir {
			t.Errorf("%s: %s read %v, want %d builds ending in %q", when, name, reads[name], builds, dir)
		}
	}
	for _, n := range []string{"top", "switched", "pinned", "portal", "auto"} {
		expect("mount", n, 1, "ltr")
	}

	a.Click(telatest.ByRole("button", "Right to left"))
	expect("rtl", "switched", 2, "rtl")
	expect("rtl", "portal", 2, "rtl")
	expect("rtl", "pinned", 1, "ltr")
	expect("rtl", "top", 1, "ltr")
	expect("rtl", "auto", 1, "ltr")
	if others != 1 {
		t.Errorf("rtl: a component that reads no direction built %d times", others)
	}

	a.Click(telatest.ByRole("button", "Arabic"))
	expect("arabic", "auto", 2, "rtl")
	if !slices.Equal(reads["top"], []string{"ltr"}) {
		t.Errorf("arabic: top read %v", reads["top"])
	}
}
