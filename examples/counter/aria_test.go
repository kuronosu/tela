package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/examples/internal/exampletest"
	"github.com/kuronosu/tela/internal/semantics"
)

// TestAriaSnapshot pins what `tela inspect --aria ./examples/counter`
// prints (testdata/counter.aria.yml).
func TestAriaSnapshot(t *testing.T) {
	exampletest.AriaGolden(t, "counter", Counter, 800, 600)
}

// TestClickThroughSemantics clicks the Increment button through the
// semantics tree, as telatest and tela mcp do.
func TestClickThroughSemantics(t *testing.T) {
	a := exampletest.Open(t, Counter, 800, 600)
	tree := a.Tree()
	var inc *semantics.Node
	tree.Semantics().Walk(func(n *semantics.Node) bool {
		if n.Role == "button" && n.Name == "Increment" {
			inc = n
		}
		return inc == nil
	})
	if inc == nil {
		t.Fatalf("no Increment button in\n%s", tree.AriaSnapshot())
	}
	if err := tree.ClickNode(inc.ID); err != nil {
		t.Fatal(err)
	}
	a.Frame()
	if got := tree.AriaSnapshot(); !strings.Contains(got, `heading "Count: 1"`) {
		t.Errorf("after a semantic click:\n%s", got)
	}
	a.CheckLogs()
}
