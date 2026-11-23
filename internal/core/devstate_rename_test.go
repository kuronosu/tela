//go:build teladev

package core

import (
	"strings"
	"testing"
)

// A component renamed between two builds keeps its state (ADR-309), and
// no other change does. Two functions of one test binary cannot share a
// declaration, which is what a renamed function keeps, so a test renames
// "from" into "to" by rewriting the captured snapshot as if from had been
// declared where to is (devAsRenamed): to the next build, to is then the
// old from under a new name, as after a rebuild.

// devDeclOf returns the identity and the declaration of the component of
// n (Component(fn) or a struct component value).
func devDeclOf(t *testing.T, n Node) (ident, decl string) {
	t.Helper()
	nd, ok := asNode(n)
	if !ok {
		nd = componentOf(n)
	}
	if nd.d == nil || nd.d.comp == nil {
		t.Fatalf("%T is not a component", n)
	}
	return devCompIdent(nd.d.comp), devCompDecl(nd.d.comp)
}

// devAsRenamed returns data with the saved nodes of from's components
// declared where to is.
func devAsRenamed(t *testing.T, data []byte, from, to Node) []byte {
	t.Helper()
	snap, err := decodeDevSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	fromID, _ := devDeclOf(t, from)
	_, toDecl := devDeclOf(t, to)
	if toDecl == "" {
		t.Fatalf("%T has no declaration", to)
	}
	snap.Idents = append(snap.Idents, toDecl)
	decl := uint32(len(snap.Idents))
	n := 0
	var walk func(d *devNode)
	walk = func(d *devNode) {
		if d.Comp && d != &snap.Root && snap.Idents[d.Ident] == fromID {
			d.Decl = decl
			n++
		}
		for i := range d.Kids {
			walk(&d.Kids[i])
		}
	}
	walk(&snap.Root)
	if n == 0 {
		t.Fatalf("no saved node of %s", fromID)
	}
	out, err := encodeDevSnapshot(snap)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// devTally is devCounter renamed (the same hooks).
func devTally() Node {
	n, _ := UseState(0)
	return div(Textf("tally %d", n))
}

// devTallyMore is devCounter renamed with a hook added.
func devTallyMore() Node {
	n, _ := UseState(0)
	s, _ := UseState("x")
	return div(Textf("tally %d %s", n, s))
}

// devBox holds a stateful child, so that a rename of it keeps the
// subtree's state; devCrate is devBox renamed.
func devBox() Node   { return div(Component(devChild)) }
func devCrate() Node { return div(Component(devChild)) }

// devTile is devCard renamed (a struct component).
type devTile struct{ Title string }

func (c devTile) Build() Node {
	n, _ := UseState(0)
	return div(Textf("%s %d", c.Title, n))
}

func TestDevStateRenamedFunction(t *testing.T) {
	a := devTestTree(t, div(Component(devSibling), Component(devCounter)), nil, nil)
	devSet(t, a, devComp(t, a, "devSibling"), 0, 1)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 7)
	data, _ := devSnap(t, a)
	b := devTestTree(t, div(Component(devSibling), Component(devTally)), devAsRenamed(t, data, Component(devCounter), Component(devTally)), nil)
	if got := devState[int](t, devComp(t, b, "devTally"), 0); got != 7 {
		t.Errorf("the renamed counter has %d, want 7", got)
	}
	rep := b.DevSeedReport()
	if len(rep.Renamed) != 1 || !strings.Contains(rep.Renamed[0], "devTally (renamed from core.devCounter)") || len(rep.Remounted) != 0 {
		t.Errorf("seed report %+v", rep)
	}
	if lines := devSeedCounts(rep).Lines; len(lines) != 1 || lines[0] != "devTally (renamed from core.devCounter) kept its state" {
		t.Errorf("the lines of -v %q", lines)
	}
}

func TestDevStateRenamedSubtree(t *testing.T) {
	a := devTestTree(t, div(Component(devBox)), nil, nil)
	devSet(t, a, devComp(t, a, "devChild"), 0, 4)
	data, _ := devSnap(t, a)
	b := devTestTree(t, div(Component(devCrate)), devAsRenamed(t, data, Component(devBox), Component(devCrate)), nil)
	if got := devState[int](t, devComp(t, b, "devChild"), 0); got != 4 {
		t.Errorf("the child of the renamed box has %d, want 4", got)
	}
}

func TestDevStateRenamedStruct(t *testing.T) {
	a := devTestTree(t, div(devCard{Title: "a"}), nil, nil)
	devSet(t, a, devComp(t, a, "devCard"), 0, 5)
	data, _ := devSnap(t, a)
	b := devTestTree(t, div(devTile{Title: "a"}), devAsRenamed(t, data, devCard{}, devTile{}), nil)
	if got := devState[int](t, devComp(t, b, "devTile"), 0); got != 5 {
		t.Errorf("the renamed card has %d, want 5", got)
	}
}

func TestDevStateRenamedKeyed(t *testing.T) {
	rows := func(fn func() Node, keys ...string) Node {
		var kids []Child
		for _, k := range keys {
			kids = append(kids, Component(fn, Key(k)))
		}
		return div(kids...)
	}
	a := devTestTree(t, rows(devCounter, "x", "y"), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter", "x"), 0, 1)
	devSet(t, a, devComp(t, a, "devCounter", "y"), 0, 2)
	data, _ := devSnap(t, a)
	// Renamed and reordered: each key keeps its own state.
	b := devTestTree(t, rows(devTally, "y", "x"), devAsRenamed(t, data, Component(devCounter), Component(devTally)), nil)
	for k, want := range map[string]int{"x": 1, "y": 2} {
		if got := devState[int](t, devComp(t, b, "devTally", k), 0); got != want {
			t.Errorf("row %s has %d, want %d", k, got, want)
		}
	}
}

// The cases that are not renames: each starts fresh.
func TestDevStateNotRenamed(t *testing.T) {
	counter, tally := Component(devCounter), Component(devTally)
	saved := func(t *testing.T, root Node, name string) []byte {
		a := devTestTree(t, root, nil, nil)
		devSet(t, a, devComp(t, a, name), 0, 7)
		data, _ := devSnap(t, a)
		return data
	}
	check := func(t *testing.T, b *Tree, name string) {
		t.Helper()
		if got := devState[int](t, devComp(t, b, name), 0); got != 0 {
			t.Errorf("%s has %d, want a fresh 0", name, got)
		}
		if r := b.DevSeedReport().Renamed; len(r) != 0 {
			t.Errorf("renamed %q", r)
		}
	}
	t.Run("another component at the same place", func(t *testing.T) {
		// devOther has the same hooks but is declared elsewhere: a
		// different component that took the place (a branch that changed).
		data := saved(t, div(counter), "devCounter")
		check(t, devTestTree(t, div(Component(devOther)), data, nil), "devOther")
	})
	t.Run("a component the old tree had", func(t *testing.T) {
		// Declared where the old one was, but the previous build already
		// had devTally elsewhere: it is not devCounter renamed.
		data := saved(t, div(counter, Component(devSibling, Key("s")), tally), "devCounter")
		b := devTestTree(t, div(tally), devAsRenamed(t, data, counter, tally), nil)
		check(t, b, "devTally")
	})
	t.Run("another position", func(t *testing.T) {
		data := saved(t, div(counter), "devCounter")
		b := devTestTree(t, div(Component(devSibling), tally), devAsRenamed(t, data, counter, tally), nil)
		check(t, b, "devTally")
	})
	t.Run("another key", func(t *testing.T) {
		data := saved(t, div(Component(devCounter, Key("a"))), "devCounter")
		b := devTestTree(t, div(Component(devTally, Key("b"))), devAsRenamed(t, data, counter, tally), nil)
		check(t, b, "devTally")
	})
	t.Run("a function where a struct component was", func(t *testing.T) {
		data := saved(t, div(devCard{}), "devCard")
		b := devTestTree(t, div(tally), devAsRenamed(t, data, devCard{}, tally), nil)
		check(t, b, "devTally")
	})
	t.Run("renamed with other hooks", func(t *testing.T) {
		// The rename is found, but the hooks changed: Fast Refresh's rule.
		data := saved(t, div(counter), "devCounter")
		b := devTestTree(t, div(Component(devTallyMore)), devAsRenamed(t, data, counter, Component(devTallyMore)), nil)
		if got := devState[int](t, devComp(t, b, "devTallyMore"), 0); got != 0 {
			t.Errorf("devTallyMore has %d, want a fresh 0", got)
		}
		if r := b.DevSeedReport().Remounted; len(r) != 1 || !strings.Contains(r[0], "renamed from core.devCounter") {
			t.Errorf("remounted %q", r)
		}
	})
}
