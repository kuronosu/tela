package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func TestStyleInheritance(t *testing.T) {
	root := div(Class("text-red-500 text-lg font-bold"),
		p(Key("p"), Text("inherits")),
		div(Class("text-blue-500"),
			Component(func() Node { return div(Key("deep"), Text("through a component")) })),
		div(Class("contents text-green-500"), div(Key("lifted"), Text("from contents"))),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	for key, want := range map[string]string{
		"p":      "#fb2c36", // red-500
		"deep":   "#2b7fff", // blue-500
		"lifted": "#00c950", // green-500
	} {
		n := mustFind(t, tr, key)
		if n.Style.Color != want {
			t.Errorf("%s: color %s, want %s", key, n.Style.Color, want)
		}
		if !strings.HasPrefix(n.Style.Font, "700 18px") {
			t.Errorf("%s: font %q, want 700 18px (inherited)", key, n.Style.Font)
		}
	}
	// The anonymous-less paragraph keeps the inherited text style in its
	// layout.
	if n := mustFind(t, tr, "p"); n.Text != "inherits" || len(n.Children) != 0 {
		t.Errorf("p: %+v", n)
	}
}

func TestRestylePropagation(t *testing.T) {
	var setColor func(string)
	app := func() Node {
		color, set := UseStateSet("text-red-500")
		setColor = set
		return div(Class(color+" bg-white"),
			p(Key("a"), Text("first")),
			p(Key("b"), Class("text-black"), Text("second")),
		)
	}
	tr := newTestTree(t, 400, 300, Component(app), nil)
	tr.Frame()
	s0 := tr.Stats()

	// An inherited change restyles the children; the paragraph that
	// inherits it only takes the new color (its layout is recolored, not
	// shaped or broken again: ADR-161), and the one with its own color
	// keeps its layout.
	setColor("text-blue-500")
	tr.Frame()
	s1 := tr.Stats()
	if got := s1.Styles - s0.Styles; got != 3 {
		t.Errorf("%d style computations, want 3 (parent and two children)", got)
	}
	if got := s1.Shapes - s0.Shapes; got != 0 {
		t.Errorf("%d paragraphs shaped for a color change, want 0", got)
	}
	if got := s1.Breaks - s0.Breaks; got != 0 {
		t.Errorf("%d line breakings for a color change, want 0", got)
	}
	if c := mustFind(t, tr, "a").Style.Color; c != "#2b7fff" {
		t.Errorf("a: color %s after the parent changed", c)
	}

	// A non-inherited change restyles only the element.
	setColor("text-blue-500 p-2")
	tr.Frame()
	s2 := tr.Stats()
	if got := s2.Styles - s1.Styles; got != 1 {
		t.Errorf("%d style computations for padding, want 1", got)
	}
	if s2.Shapes != s1.Shapes {
		t.Error("a padding change reshaped a paragraph")
	}
	if y := mustFind(t, tr, "a").Rect.Y0; y != 8 {
		t.Errorf("a.y = %g after p-2, want 8", y)
	}
}

func TestStructuralVariants(t *testing.T) {
	list := func(n int) Node {
		items := make([]int, n)
		for i := range items {
			items[i] = i
		}
		return div(Map(items, func(i int) Node {
			return div(Key(i), Class("h-4 bg-white first:bg-red-500 last:bg-blue-500 even:h-8"))
		}))
	}
	tr := newTestTree(t, 400, 300, list(3), nil)
	tr.Frame()
	check := func(key, bg string, h float32) {
		t.Helper()
		n := mustFind(t, tr, key)
		if n.Style.Background != bg || n.Rect.Height() != h {
			t.Errorf("%s: background %s height %g, want %s %g", key, n.Style.Background, n.Rect.Height(), bg, h)
		}
	}
	check("0", "#fb2c36", 16)
	check("1", "#ffffff", 32)
	check("2", "#2b7fff", 16)
	// Appending moves last: to the new child.
	rerender(tr, list(4))
	check("2", "#ffffff", 16)
	check("3", "#2b7fff", 32)
}

func TestClassErrorsReportedOnce(t *testing.T) {
	var logs []string
	tree := func(n int) Node {
		kids := make([]Node, n)
		for i := range kids {
			kids[i] = div(Class("p-4 rounded-xxl"))
		}
		return div(List(kids), div(Class("bg-blu-500")))
	}
	tr := newTestTree(t, 200, 200, tree(3), &logs)
	tr.Frame()
	rerender(tr, tree(5))
	if len(logs) != 2 {
		t.Fatalf("logs = %q, want 2 messages (one per class string)", logs)
	}
	if !strings.Contains(logs[0], `unknown class "rounded-xxl"; did you mean "rounded-xl"`) &&
		!strings.Contains(logs[0], `unknown class "rounded-xxl"; did you mean "rounded-2xl"`) {
		t.Errorf("message without a suggestion: %q", logs[0])
	}
	if !strings.Contains(logs[0], `Class("p-4 rounded-xxl")`) {
		t.Errorf("message without its class string: %q", logs[0])
	}
	// The valid classes of the string still apply.
	root := tr.Inspect()
	if pad := root.Children[0].Children[0].Padding; pad.Top != 16 {
		t.Errorf("padding %v, want 16 (p-4 applies despite the error)", pad)
	}
}

func TestDevLogDefault(t *testing.T) {
	// Without dev mode and without a sink, errors are not printed.
	tr := NewTree(Config{Fonts: testFonts})
	if tr.log != nil {
		t.Error("a non-dev tree has a logger")
	}
	tr = NewTree(Config{Fonts: testFonts, Dev: true})
	if tr.log == nil {
		t.Error("a dev tree has no logger")
	}
}

func TestCanvasColor(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts})
	tr.Mount(Text("x"))
	tr.Frame()
	if c := tr.CanvasColor(); c != gfx.White {
		t.Errorf("default canvas %v, want white", c)
	}
	tr = NewTree(Config{Fonts: testFonts, RootClass: "bg-slate-900"})
	tr.Mount(Text("x"))
	tr.Frame()
	if c := tr.CanvasColor(); c.String() != "#0f172b" {
		t.Errorf("canvas %v, want slate-900", c)
	}
	tr = NewTree(Config{Fonts: testFonts, RootClass: "bg-black/50"})
	tr.Mount(Text("x"))
	tr.Frame()
	if c := tr.CanvasColor(); c.String() != "#808080" {
		t.Errorf("canvas %v, want black/50 over white", c)
	}
}

func TestViewportVariants(t *testing.T) {
	tr := newTestTree(t, 600, 300, div(Key("d"), Class("h-4 md:h-8")), nil)
	tr.Frame()
	if h := mustFind(t, tr, "d").Rect.Height(); h != 16 {
		t.Errorf("height %g at 600px, want 16", h)
	}
	tr.SetViewport(gfx.Size{W: 800, H: 300})
	tr.Frame()
	if h := mustFind(t, tr, "d").Rect.Height(); h != 32 {
		t.Errorf("height %g at 800px, want 32 (md:)", h)
	}
}

func TestClassErrorMessage(t *testing.T) {
	var logs []string
	tr := newTestTree(t, 200, 200, div(Class("p-4 rounded-xxl bg-blu-500")), &logs)
	tr.Frame()
	if len(logs) != 1 {
		t.Fatalf("logs = %q", logs)
	}
	t.Log(logs[0])
	for _, want := range []string{`unknown class "rounded-xxl"; did you mean`, `unknown class "bg-blu-500"; did you mean "bg-blue-500"?`, `Class("p-4 rounded-xxl bg-blu-500") of a <div>`} {
		if !strings.Contains(logs[0], want) {
			t.Errorf("message %q lacks %q", logs[0], want)
		}
	}
}
