package core

import "testing"

// containingBlock is the one containing-block walk of paint, hit-testing,
// floating placement and the scroll chain: painted boxes are read through
// their paint style, and boxes laid out but not painted yet (which a
// layout effect may scroll into view, ADR-176) through their computed
// style, with the same result.
func TestContainingBlockBeforeAndAfterPaint(t *testing.T) {
	root := div(Key("top"),
		div(Key("rel"), Class("relative"),
			div(Key("static"),
				div(Key("abs1"), Class("absolute")),
				div(Key("fix1"), Class("fixed")),
			),
		),
		div(Key("tr"), Class("translate-x-1"),
			div(Key("inner"),
				div(Key("fix2"), Class("fixed")),
				div(Key("abs2"), Class("absolute")),
			),
		),
		div(Key("fx"), Class("fixed"), div(Key("abs3"), Class("absolute"))),
		div(Key("st"), Class("sticky top-0"), div(Key("abs4"), Class("absolute"))),
		div(Key("flow")),
	)
	tr := newTestTree(t, 300, 300, root, nil)
	tr.Frame()
	box := func(k string) *Box { return keyed(tr, k).box }
	want := map[string]*Box{
		"abs1": box("rel"), "fix1": tr.rootBox,
		"fix2": box("tr"), "abs2": box("tr"),
		"abs3": box("fx"), "abs4": box("st"),
		"flow": box("top"), "static": box("rel"),
	}
	check := func(when string) {
		t.Helper()
		for k, cb := range want {
			if got := containingBlock(box(k)); got != cb {
				t.Errorf("%s: the containing block of %s is %p, want %p", when, k, got, cb)
			}
		}
	}
	for k := range want {
		if box(k).pcs == nil {
			t.Fatalf("%s was not painted", k)
		}
	}
	check("painted")

	// The same boxes as a frame's layout leaves them before its paint.
	var walk func(b *Box)
	walk = func(b *Box) {
		b.pcs = nil
		for _, k := range b.kids {
			walk(k)
		}
	}
	walk(tr.rootBox)
	for k := range want {
		if box(k).pcs != nil {
			t.Fatalf("%s kept its paint style", k)
		}
	}
	check("not painted")
}
