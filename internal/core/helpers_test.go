package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Element constructors in the shape of package h.

func div(c ...Child) Node    { return Primitive(style.KindDiv, c...) }
func span(c ...Child) Node   { return Primitive(style.KindSpan, c...) }
func p(c ...Child) Node      { return Primitive(style.KindP, c...) }
func h1(c ...Child) Node     { return Primitive(style.KindH1, c...) }
func button(c ...Child) Node { return Primitive(style.KindButton, c...) }
func strong(c ...Child) Node { return Primitive(style.KindStrong, c...) }
func em(c ...Child) Node     { return Primitive(style.KindEm, c...) }
func code(c ...Child) Node   { return Primitive(style.KindCode, c...) }
func img(c ...Child) Node    { return Primitive(style.KindImg, c...) }

// testFonts is shared by the tests: the embedded fonts only.
var testFonts = text.NewDefaultCollection()

// newTestTree returns a tree of the given viewport with root mounted,
// with dev messages collected in *logs.
func newTestTree(t testing.TB, w, h float32, root Node, logs *[]string) *Tree {
	t.Helper()
	cfg := Config{Fonts: testFonts}
	if logs != nil {
		cfg.Log = func(m string) { *logs = append(*logs, m) }
	}
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.Mount(root)
	return tr
}

// find returns the inspect node with the given key, depth first.
func find(n *InspectNode, key string) *InspectNode {
	if n.Key == key {
		return n
	}
	for i := range n.Children {
		if f := find(&n.Children[i], key); f != nil {
			return f
		}
	}
	return nil
}

// mustFind returns the box with key after a frame.
func mustFind(t testing.TB, tr *Tree, key string) *InspectNode {
	t.Helper()
	root := tr.Inspect()
	n := find(&root, key)
	if n == nil {
		t.Fatalf("no box with key %q in\n%s", key, dump(&root))
	}
	return n
}

// dump formats an inspect tree for failure messages.
func dump(n *InspectNode) string {
	var sb strings.Builder
	var walk func(n *InspectNode, depth int)
	walk = func(n *InspectNode, depth int) {
		fmt.Fprintf(&sb, "%s%s key=%q rect=(%g,%g %gx%g)", strings.Repeat("  ", depth), n.Kind, n.Key,
			n.Rect.X0, n.Rect.Y0, n.Rect.Width(), n.Rect.Height())
		if n.Text != "" {
			fmt.Fprintf(&sb, " text=%q lines=%d", n.Text, n.Lines)
		}
		sb.WriteByte('\n')
		for i := range n.Children {
			walk(&n.Children[i], depth+1)
		}
	}
	walk(n, 0)
	return sb.String()
}

func near(a, b, eps float32) bool { return a-b <= eps && b-a <= eps }
