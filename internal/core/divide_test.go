package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/testutil/golden"
)

// divideList is a parent with the classes cls and n children of 16 px
// (py-2) before their border.
func divideList(cls string, n int) Node {
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}
	return div(Class(cls), Map(items, func(i int) Node {
		return div(Key(i), Class("py-2"))
	}))
}

func borderOf(t *testing.T, tr *Tree, key string) Edges {
	t.Helper()
	return mustFind(t, tr, key).Border
}

// TestDivideChildren: every child but the last gets the line, and it
// follows the children as they come and go (ADR-203).
func TestDivideChildren(t *testing.T) {
	tr := newTestTree(t, 300, 300, divideList("divide-y", 3), nil)
	tr.Frame()
	for i, want := range []float32{1, 1, 0} {
		if got := borderOf(t, tr, string(rune('0'+i))).Bottom; got != want {
			t.Errorf("3 children: child %d bottom border %g, want %g", i, got, want)
		}
	}
	// The line is part of the child's box.
	if h0, h2 := mustFind(t, tr, "0").Rect.Height(), mustFind(t, tr, "2").Rect.Height(); h0 != 17 || h2 != 16 {
		t.Errorf("heights %g and %g, want 17 and 16 (the line is 1 px)", h0, h2)
	}

	// Appending a child gives the old last one its line.
	rerender(tr, divideList("divide-y", 4))
	if got := borderOf(t, tr, "2").Bottom; got != 1 {
		t.Errorf("after appending: child 2 bottom border %g, want 1", got)
	}
	if got := borderOf(t, tr, "3").Bottom; got != 0 {
		t.Errorf("after appending: child 3 bottom border %g, want 0", got)
	}

	// Removing the last takes the line off the new one.
	rerender(tr, divideList("divide-y", 2))
	if got := borderOf(t, tr, "1").Bottom; got != 0 {
		t.Errorf("after removing: child 1 bottom border %g, want 0", got)
	}
	if got := borderOf(t, tr, "0").Bottom; got != 1 {
		t.Errorf("after removing: child 0 bottom border %g, want 1", got)
	}

	// A single child has no line, and the parent's classes change the
	// lines of the children that are already there.
	rerender(tr, divideList("divide-y", 1))
	if got := borderOf(t, tr, "0").Bottom; got != 0 {
		t.Errorf("one child: bottom border %g, want 0", got)
	}
	rerender(tr, divideList("divide-x-2", 3))
	if b := borderOf(t, tr, "0"); b.Right != 2 || b.Bottom != 0 {
		t.Errorf("after divide-x-2: child 0 border %+v, want a 2 px right border", b)
	}
	rerender(tr, divideList("flex", 3))
	if b := borderOf(t, tr, "0"); b != (Edges{}) {
		t.Errorf("after removing divide: child 0 border %+v, want none", b)
	}
}

// TestDivideInComponents: children that come from components count as
// children of the element, like in structuralState.
func TestDivideInComponents(t *testing.T) {
	item := func(i int) Node { return div(Key(i), Class("h-8")) }
	root := div(Class("divide-y"), Component(func() Node { return item(0) }), item(1), item(2))
	tr := newTestTree(t, 300, 300, root, nil)
	tr.Frame()
	for i, want := range []float32{1, 1, 0} {
		if got := borderOf(t, tr, string(rune('0'+i))).Bottom; got != want {
			t.Errorf("child %d bottom border %g, want %g", i, got, want)
		}
	}
}

// TestDivideChildBorderWins: a child's own border classes win over the
// parent's lines (the lines are :where() rules in Tailwind).
func TestDivideChildBorderWins(t *testing.T) {
	root := div(Class("divide-y-2 divide-red-500"),
		div(Key("a"), Class("h-8 border-b-4")),
		div(Key("b"), Class("h-8 border-b-0")),
		div(Key("c"), Class("h-8 border-t")),
		div(Key("d"), Class("h-8")))
	tr := newTestTree(t, 300, 300, root, nil)
	tr.Frame()
	for key, want := range map[string]Edges{
		"a": {Bottom: 4}, "b": {}, "c": {Top: 1, Bottom: 2}, "d": {},
	} {
		if got := borderOf(t, tr, key); got != want {
			t.Errorf("child %s border %+v, want %+v", key, got, want)
		}
	}
}

// goldenDivide shows divide-y lists and divide-x groups in both color
// schemes.
func goldenDivide() Node {
	row := func(title, sub string) Node {
		return div(Class("flex flex-col gap-0.5 px-4 py-2.5"),
			span(Class("text-sm font-medium"), Text(title)),
			span(Class("text-xs text-muted-foreground"), Text(sub)))
	}
	cell := func(label, v string) Node {
		return div(Class("flex flex-1 flex-col items-center py-2"),
			span(Class("text-lg font-semibold tabular-nums"), Text(v)),
			span(Class("text-xs text-muted-foreground"), Text(label)))
	}
	return div(Class("flex min-h-screen flex-col gap-4 bg-background p-5 text-foreground"),
		// A list: divide-y with the border token, round and clipped.
		div(Class("divide-y divide-border overflow-hidden rounded-lg border border-border bg-card"),
			row("Design review", "Tomorrow, 10:00"),
			row("Release notes", "Draft, 3 comments"),
			row("Budget", "Waiting for approval")),
		// A horizontal group: divide-x, dashed, colored by the palette.
		div(Class("flex divide-x divide-dashed divide-sky-500/60 rounded-lg border border-border bg-card"),
			cell("Open", "12"), cell("Closed", "148"), cell("Draft", "3")),
		// Reverse, a thick dotted line, and a child with its own border.
		div(Class("flex gap-4"),
			div(Class("flex flex-1 flex-row-reverse divide-x-2 divide-x-reverse divide-dotted divide-primary rounded-lg bg-muted"),
				div(Class("flex-1 p-3 text-center text-xs"), Text("reverse")), div(Class("flex-1 p-3 text-center text-xs"), Text("row"))),
			div(Class("flex flex-1 flex-col divide-y-2 divide-amber-500 rounded-lg bg-muted"),
				div(Class("p-2 text-center text-xs"), Text("own border")), div(Class("border-b-0 p-2 text-center text-xs"), Text("wins")), div(Class("p-2 text-center text-xs"), Text("last")))),
	)
}

func TestGoldenDivide(t *testing.T) {
	for _, tc := range []struct {
		name string
		dark bool
	}{{"divide_light", false}, {"divide_dark", true}} {
		for _, scale := range []float32{1, 2} {
			name := tc.name
			if scale == 2 {
				name += "_x2"
			} else {
				name += "_x1"
			}
			t.Run(name, func(t *testing.T) {
				_, img := Render(goldenDivide(), RenderOptions{
					Config: Config{Fonts: testFonts, Dark: tc.dark},
					Width:  360, Height: 420, Scale: scale,
				})
				golden.Image(t, name, img)
			})
		}
	}
}
