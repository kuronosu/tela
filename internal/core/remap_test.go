package core

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// byTestID returns the mounted primitive with TestID id, or nil.
func byTestID(tr *Tree, id string) *Element {
	var walk func(el *Element) *Element
	walk = func(el *Element) *Element {
		if el.kind == elemPrimitive && el.node.d.testID == id {
			return el
		}
		for _, c := range el.children {
			if f := walk(c); f != nil {
				return f
			}
		}
		return nil
	}
	return walk(tr.root)
}

// testIDRect returns the border box of the element with TestID id in
// absolute logical pixels, as Inspect gives it.
func testIDRect(t *testing.T, tr *Tree, id string) gfx.Rect {
	t.Helper()
	el := byTestID(tr, id)
	if el == nil {
		t.Fatalf("no element with TestID %q", id)
	}
	var origin gfx.Point
	for p := el.box.parent; p != nil; p = p.parent {
		origin = origin.Add(p.ld.Layout().Location)
	}
	l := el.box.ld.Layout()
	return gfx.RectFromOriginSize(origin.Add(l.Location), l.Size)
}

// stacked checks that the box of TestID lo starts below the end of the
// one of hi, at the same x: they are stacked in a column.
func stacked(t *testing.T, tr *Tree, hi, lo string) {
	t.Helper()
	h, l := testIDRect(t, tr, hi), testIDRect(t, tr, lo)
	if l.Y0 < h.Y1 || l.X0 != h.X0 {
		t.Errorf("%s %v is not below %s %v", lo, l, hi, h)
	}
}

// inRow checks that the box of TestID right starts where the one of left
// ends, at the same y: they are in a row.
func inRow(t *testing.T, tr *Tree, left, right string) {
	t.Helper()
	l, r := testIDRect(t, tr, left), testIDRect(t, tr, right)
	if !near(r.X0, l.X1, 0.01) || r.Y0 != l.Y0 {
		t.Errorf("%s %v is not right of %s %v", right, r, left, l)
	}
}

// TestRestyleWithStructureChange changes the classes of a container
// together with its content in one update, and checks each side against a
// fresh mount of it. A box's layout style is mapped by its parent's remap
// (remapPhase): a restyled container must queue its parent even when it is
// already queued itself, as the parent of a child whose structure or
// natural size changed in the same frame. It kept its old flex direction
// when its first child was wrapped in a new Div in the update that made it
// flex-col (found in examples/notes).
func TestRestyleWithStructureChange(t *testing.T) {
	a := func() Node { return div(TestID("a"), Text("aaa")) }
	b := func() Node { return div(TestID("b"), Text("bbb")) }
	c := func() Node { return div(TestID("c"), Text("ccc")) }
	ka := func() Node { return div(Key("a"), TestID("a"), Text("aaa")) }
	kb := func() Node { return div(Key("b"), TestID("b"), Text("bbb")) }
	kc := func() Node { return div(Key("c"), TestID("c"), Text("ccc")) }
	narrow := image.NewRGBA(image.Rect(0, 0, 20, 10))
	wide := image.NewRGBA(image.Rect(0, 0, 60, 10))
	pic := func(on bool) Node {
		if on {
			return img(TestID("a"), Src(wide))
		}
		return img(TestID("a"), Src(narrow))
	}
	// wrap is the update of the notes row: the first children move into a
	// new Div (which reuses the Element of the first old one, by position)
	// and another child follows.
	wrap := func(cls string) Node { return div(TestID("x"), Class(cls), div(Class("flex"), a(), b()), c()) }

	cases := []struct {
		name  string
		view  func(on bool) Node
		check func(t *testing.T, tr *Tree)
	}{
		{"class", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), a(), b())
			}
			return div(TestID("x"), Class("flex flex-col"), a(), b())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "b") }},
		{"wrap", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex flex-col"), a(), b())
			}
			return wrap("flex flex-col")
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "c"); inRow(t, tr, "a", "b") }},
		{"class and wrap", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), a(), b())
			}
			return wrap("flex flex-col")
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "c"); inRow(t, tr, "a", "b") }},
		{"class and append", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), a(), b())
			}
			return div(TestID("x"), Class("flex flex-col"), a(), b(), c())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "b"); stacked(t, tr, "b", "c") }},
		{"class and insert", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), ka(), kb())
			}
			return div(TestID("x"), Class("flex flex-col"), ka(), kc(), kb())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "c"); stacked(t, tr, "c", "b") }},
		{"class and removal", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), ka(), kb(), kc())
			}
			return div(TestID("x"), Class("flex flex-col"), ka(), kc())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "c") }},
		{"class and reorder", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), ka(), kb(), kc())
			}
			return div(TestID("x"), Class("flex flex-col"), kc(), ka(), kb())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "c", "a"); stacked(t, tr, "a", "b") }},
		{"class and text", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex"), a(), b())
			}
			return div(TestID("x"), Class("flex flex-col"), a(), div(TestID("b"), Text("bbbbbb")))
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "b") }},
		{"class and image size", func(on bool) Node {
			// The image's new natural size remaps it (updateImage) while
			// its parent is reconciled, before the style phase.
			cls := "flex items-start"
			if on {
				cls = "flex flex-col items-start"
			}
			return div(TestID("x"), Class(cls), pic(on), b())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "b") }},
		{"grid to flex", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("grid grid-cols-2"), a(), b())
			}
			return div(TestID("x"), Class("flex flex-col"), a(), b())
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "b") }},
		{"grid to flex and wrap", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("grid grid-cols-2"), a(), b())
			}
			return wrap("flex flex-col")
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "c"); inRow(t, tr, "a", "b") }},
		{"flex to grid and wrap", func(on bool) Node {
			if !on {
				return div(TestID("x"), Class("flex flex-col"), a(), b())
			}
			return wrap("grid grid-cols-2")
		}, func(t *testing.T, tr *Tree) {
			inRow(t, tr, "a", "b")
			// c is in the second column, of the 400px grid.
			if a, c := testIDRect(t, tr, "a"), testIDRect(t, tr, "c"); c.X0 != 200 || c.Y0 != a.Y0 {
				t.Errorf("c %v is not in the second column, beside a %v", c, a)
			}
		}},
		{"direction under intrinsic size", func(on bool) Node {
			// x is a flex item of a row: its width is its max-content
			// width, cached by layout (the sum of its items in a row, the
			// widest in a column), and d is placed after it.
			x := div(TestID("x"), Class("flex"), a(), b())
			if on {
				x = wrap("flex flex-col")
			}
			return div(Class("flex items-start"), x, div(TestID("d"), Text("ddd")))
		}, func(t *testing.T, tr *Tree) {
			stacked(t, tr, "a", "c")
			inRow(t, tr, "x", "d")
			if x, ab := testIDRect(t, tr, "x"), testIDRect(t, tr, "b").X1-testIDRect(t, tr, "a").X0; !near(x.Width(), ab, 0.01) {
				t.Errorf("x is %g wide, want its row of a and b, %g", x.Width(), ab)
			}
		}},
		{"direction alone under intrinsic size", func(on bool) Node {
			cls := "flex"
			if on {
				cls = "flex flex-col"
			}
			return div(Class("flex items-start"), div(TestID("x"), Class(cls), a(), b()), div(TestID("d"), Text("ddd")))
		}, func(t *testing.T, tr *Tree) { stacked(t, tr, "a", "b"); inRow(t, tr, "x", "d") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var set func(bool)
			app := func() Node {
				on, s := UseStateSet(false)
				set = s
				return tc.view(on)
			}
			tr := newTestTree(t, 400, 200, Component(app), nil)
			tr.Frame()
			// There and back: each side as a fresh mount lays it out.
			for _, on := range []bool{true, false, true} {
				set(on)
				tr.Frame()
				fresh := newTestTree(t, 400, 200, Component(func() Node { return tc.view(on) }), nil)
				fresh.Frame()
				got, want := tr.Inspect(), fresh.Inspect()
				if g, w := dump(&got), dump(&want); g != w {
					t.Fatalf("on=%v: updated tree\n%s\nfresh mount\n%s", on, g, w)
				}
			}
			tc.check(t, tr)
		})
	}
}
