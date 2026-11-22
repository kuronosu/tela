package telatest_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// A layout wider than the window cuts many texts for one reason: the first
// three are reported, and the rest as one violation.
func TestTextClippedGroupsTheWindowEdge(t *testing.T) {
	t.Parallel()
	row := func() tela.Node {
		return Div(Class("flex gap-2 p-2"),
			Span(Class("w-[100px] shrink-0"), Text("one")), Span(Class("w-[100px] shrink-0"), Text("two")),
			Span(Class("w-[100px] shrink-0"), Text("three")), Span(Class("w-[100px] shrink-0"), Text("four")),
			Span(Class("w-[100px] shrink-0"), Text("five")), Span(Class("w-[100px] shrink-0"), Text("six")))
	}
	vs := audit(t, row, 220, 100, "text-clipped")
	if len(vs) != 4 {
		t.Fatalf("got %d violations: %v", len(vs), vs)
	}
	if vs[3].Element != "more text cut off by the window's edge" {
		t.Errorf("the last one is %q", vs[3].Element)
	}
}
