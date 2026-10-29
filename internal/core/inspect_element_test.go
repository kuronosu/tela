package core

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// semFind returns the first node of the semantics tree with role and name.
func semFind(n *semantics.Node, role, name string) *semantics.Node {
	if n.Kind == semantics.KindElement && n.Role == role && n.Name == name {
		return n
	}
	for _, c := range n.Children {
		if f := semFind(c, role, name); f != nil {
			return f
		}
	}
	return nil
}

func TestInspectElement(t *testing.T) {
	tr := newTestTree(t, 300, 200, div(Class("p-4 flex flex-col gap-2"),
		h1(Text("Title")),
		div(Key("card"), Class("p-3 border"),
			button(Key("go"), Class("px-2 py-1"), Text("Go")),
			button(Key("other"), Text("Other"))),
		p(Key("para"), Text("Some "), Primitive(style.KindA, Key("link"), Href("https://example.com"), Text("docs")), Text(" words")),
	), nil)
	tr.Frame()
	sem := tr.Semantics()
	mustJSON := func(n InspectNode) string {
		t.Helper()
		js, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		return string(js)
	}

	t.Run("a box is its place in the whole tree", func(t *testing.T) {
		n := semFind(sem, "button", "Go")
		if n == nil {
			t.Fatalf("no button Go in the semantics tree")
		}
		got, err := tr.InspectElement(n.ID)
		if err != nil {
			t.Fatal(err)
		}
		if want := mustFind(t, tr, "go"); mustJSON(got) != mustJSON(*want) {
			t.Errorf("the subtree of the button differs from its box in the whole tree:\n got %s\nwant %s", mustJSON(got), mustJSON(*want))
		}
		if got.Kind != "button" || got.Rect.X0 == 0 || got.Rect.Y0 == 0 {
			t.Errorf("button box %+v, want an absolute rect inside the padded card", got.Rect)
		}
	})

	t.Run("a text is its parent's box", func(t *testing.T) {
		var text *semantics.Node
		var walk func(n *semantics.Node)
		walk = func(n *semantics.Node) {
			if n.Kind == semantics.KindText && n.Text == "Title" {
				text = n
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(sem)
		if text == nil {
			t.Fatal("no text Title in the semantics tree")
		}
		got, err := tr.InspectElement(text.ID)
		if err != nil || got.Kind != "h1" {
			t.Errorf("InspectElement(text) = %s, %v; want the h1 box", got.Kind, err)
		}
	})

	t.Run("an inline element is its paragraph's box", func(t *testing.T) {
		n := semFind(sem, "link", "docs")
		if n == nil {
			t.Fatal("no link in the semantics tree")
		}
		got, err := tr.InspectElement(n.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := mustFind(t, tr, "para")
		if mustJSON(got) != mustJSON(*want) {
			t.Errorf("inline element: got %s, want the paragraph %s", got.Kind, want.Kind)
		}
	})

	t.Run("an unknown node is gone", func(t *testing.T) {
		if _, err := tr.InspectElement(semantics.NodeID(1 << 40)); !errors.Is(err, ErrNodeGone) {
			t.Errorf("err = %v, want ErrNodeGone", err)
		}
	})
}
