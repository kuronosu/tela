package core

import "testing"

// TestGridItems checks the box structure of a grid container: each
// element child is a grid item (a span too, blockified) and each run of
// text an anonymous item, placed in the grid's cells.
func TestGridItems(t *testing.T) {
	root := div(Key("g"), Class("grid w-[300px] grid-cols-3 gap-2"),
		Text("text"),
		span(Key("s"), Text("span")),
		div(Key("d"), Class("col-span-3 h-4")),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	g := mustFind(t, tr, "g")
	if len(g.Children) != 3 || g.Children[0].Kind != "anonymous" || g.Children[0].Text != "text" {
		t.Fatalf("grid children:\n%s", dump(g))
	}
	// Columns of (300 - 16) / 3; the first row is one 24px line.
	col := float32(284) / 3
	wantRect(t, &g.Children[0], 0, 0, col, 24)
	wantRect(t, mustFind(t, tr, "s"), col+8, 0, col, 24)
	wantRect(t, mustFind(t, tr, "d"), 0, 32, 300, 16)
}

// TestGridDisplayChange switches a container between flex and grid and
// changes its template: the boxes follow, and an equal template keeps its
// layout pointer.
func TestGridDisplayChange(t *testing.T) {
	var setCls func(string)
	app := func() Node {
		cls, set := UseStateSet("grid grid-cols-[100px_1fr]")
		setCls = set
		return div(Key("c"), Class("w-[300px] "+cls),
			div(Key("a"), Class("h-4")),
			div(Key("b"), Class("h-4")),
		)
	}
	tr := newTestTree(t, 400, 300, Component(app), nil)
	tr.Frame()
	wantRect(t, mustFind(t, tr, "a"), 0, 0, 100, 16)
	wantRect(t, mustFind(t, tr, "b"), 100, 0, 200, 16)

	setCls("flex")
	tr.Frame()
	wantRect(t, mustFind(t, tr, "a"), 0, 0, 0, 16)
	wantRect(t, mustFind(t, tr, "b"), 0, 0, 0, 16)

	setCls("grid grid-cols-[1fr_50px] gap-x-2")
	tr.Frame()
	wantRect(t, mustFind(t, tr, "a"), 0, 0, 242, 16)
	wantRect(t, mustFind(t, tr, "b"), 250, 0, 50, 16)

	// The same template again (another class string): the pointer the
	// layout style holds does not change, so the box is not marked dirty.
	box := findBox(tr, "c")
	before := box.ls.Grid
	setCls("grid gap-x-2 grid-cols-[1fr_50px]")
	tr.Frame()
	if box.ls.Grid != before {
		t.Error("an equal grid template got a new pointer")
	}
	wantRect(t, mustFind(t, tr, "b"), 250, 0, 50, 16)
}

// findBox returns the box of the element with key.
func findBox(tr *Tree, key any) *Box {
	var walk func(el *Element) *Box
	walk = func(el *Element) *Box {
		if el.keyed && el.key == key && el.box != nil {
			return el.box
		}
		for _, c := range el.children {
			if b := walk(c); b != nil {
				return b
			}
		}
		return nil
	}
	return walk(tr.root)
}
