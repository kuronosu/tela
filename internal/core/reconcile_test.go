package core

import (
	"fmt"
	"image"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// counterProbe is a stateful component for identity tests: each instance
// registers its setter under its label, and renders "label=value".
type counterProbe struct {
	setters map[string]func(int)
	mounts  map[string]int
	inst    map[string]*int
}

func newProbe() *counterProbe {
	return &counterProbe{setters: map[string]func(int){}, mounts: map[string]int{}, inst: map[string]*int{}}
}

// item returns a component node whose state starts at 0.
func (pr *counterProbe) item(label string) Node {
	return ComponentProps(pr.render, label)
}

func (pr *counterProbe) render(label string) Node {
	v, set := UseState(0)
	// The first build stores a new pointer: a different one means a new
	// Element.
	inst, _ := UseState(new(int))
	if pr.inst[label] != inst {
		pr.inst[label] = inst
		pr.mounts[label]++
	}
	pr.setters[label] = set
	return div(Textf("%s=%d", label, v))
}

// texts returns the paragraph texts of the tree in order.
func texts(tr *Tree) string {
	var out []string
	var walk func(n *InspectNode)
	walk = func(n *InspectNode) {
		if n.Text != "" {
			out = append(out, n.Text)
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	root := tr.Inspect()
	walk(&root)
	return strings.Join(out, " ")
}

// rerender mounts root again and runs a frame.
func rerender(tr *Tree, root Node) {
	tr.Mount(root)
	tr.Frame()
}

func TestKeyedReorderKeepsState(t *testing.T) {
	pr := newProbe()
	list := func(ids ...string) Node {
		return div(Map(ids, func(id string) Node { return WithKey(pr.item(id), id) }))
	}
	tr := newTestTree(t, 300, 300, list("a", "b", "c"), nil)
	tr.Frame()
	pr.setters["b"](5)
	pr.setters["c"](7)
	tr.Frame()
	if got, want := texts(tr), "a=0 b=5 c=7"; got != want {
		t.Fatalf("before reorder: %q, want %q", got, want)
	}
	mounts := tr.Stats().Mounts

	clear(pr.setters)
	rerender(tr, list("c", "a", "b"))
	if got, want := texts(tr), "c=7 a=0 b=5"; got != want {
		t.Errorf("after reorder: %q, want %q (state must follow the keys)", got, want)
	}
	if got := tr.Stats().Mounts; got != mounts {
		t.Errorf("reorder mounted %d elements, want 0", got-mounts)
	}

	// Removing a keyed item keeps the others; adding one mounts only it.
	clear(pr.setters)
	rerender(tr, list("b", "d"))
	if got, want := texts(tr), "b=5 d=0"; got != want {
		t.Errorf("after remove/add: %q, want %q", got, want)
	}
}

func TestTypeChangeRemounts(t *testing.T) {
	pr := newProbe()
	tree := func(wrapSpan bool) Node {
		if wrapSpan {
			return div(span(pr.item("x")))
		}
		return div(div(pr.item("x")))
	}
	tr := newTestTree(t, 300, 300, tree(false), nil)
	tr.Frame()
	pr.setters["x"](3)
	tr.Frame()
	if got := texts(tr); got != "x=3" {
		t.Fatalf("got %q, want x=3", got)
	}
	clear(pr.setters)
	rerender(tr, tree(true))
	if got := texts(tr); got != "x=0" {
		t.Errorf("after the parent changed from Div to Span: %q, want x=0 (a new type remounts the subtree)", got)
	}
	if pr.mounts["x"] != 2 {
		t.Errorf("x mounted %d times, want 2", pr.mounts["x"])
	}

	// Another component function at the same position remounts too.
	other := func() Node { return Text("other") }
	rerender(tr, div(span(Component(other))))
	if got := texts(tr); got != "other" {
		t.Errorf("got %q, want other", got)
	}
}

func TestPositionFallback(t *testing.T) {
	pr := newProbe()
	tree := func(showFirst bool, extra bool) Node {
		return div(
			If(showFirst, pr.item("first")),
			pr.item("second"),
			If(extra, pr.item("third")),
		)
	}
	tr := newTestTree(t, 300, 300, tree(true, false), nil)
	tr.Frame()
	pr.setters["second"](2)
	tr.Frame()
	if got := texts(tr); got != "first=0 second=2" {
		t.Fatalf("got %q", got)
	}
	// The hole left by If(false, ...) keeps "second" at its position.
	rerender(tr, tree(false, false))
	if got := texts(tr); got != "second=2" {
		t.Errorf("after hiding the first child: %q, want second=2", got)
	}
	rerender(tr, tree(true, true))
	if got := texts(tr); got != "first=0 second=2 third=0" {
		t.Errorf("after showing first and third: %q", got)
	}
	if pr.mounts["second"] != 1 {
		t.Errorf("second mounted %d times, want 1", pr.mounts["second"])
	}

	// Unkeyed children of the same type are matched by position: the
	// state stays at the position, not with the label.
	tr2 := newTestTree(t, 300, 300, div(pr.item("p0"), pr.item("p1")), nil)
	tr2.Frame()
	pr.setters["p0"](9)
	tr2.Frame()
	rerender(tr2, div(pr.item("p1"), pr.item("p0")))
	if got := texts(tr2); got != "p1=9 p0=0" {
		t.Errorf("unkeyed swap: %q, want p1=9 p0=0 (position fallback)", got)
	}
}

func TestListDoesNotShiftSiblings(t *testing.T) {
	pr := newProbe()
	tree := func(n int) Node {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprint("i", i)
		}
		return div(
			Map(ids, func(id string) Node { return WithKey(pr.item(id), id) }),
			pr.item("footer"),
		)
	}
	tr := newTestTree(t, 300, 600, tree(2), nil)
	tr.Frame()
	pr.setters["footer"](4)
	tr.Frame()
	rerender(tr, tree(5))
	if !strings.HasSuffix(texts(tr), "footer=4") {
		t.Errorf("growing the list remounted its sibling: %q", texts(tr))
	}
	if pr.mounts["footer"] != 1 {
		t.Errorf("footer mounted %d times, want 1", pr.mounts["footer"])
	}
}

func TestMissingKeysWarnOnce(t *testing.T) {
	var logs []string
	list := func(n int) Node {
		items := make([]int, n)
		return div(Map(items, func(int) Node { return div(Text("x")) }))
	}
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { logs = append(logs, m) }})
	tr.Mount(list(3))
	tr.Frame()
	tr.Mount(list(4))
	tr.Frame()
	if len(logs) != 1 || !strings.Contains(logs[0], "have no key") {
		t.Fatalf("logs = %q, want one missing-key warning", logs)
	}

	// Static children need no keys.
	logs = nil
	tr2 := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { logs = append(logs, m) }})
	tr2.Mount(div(div(), div(), div()))
	tr2.Frame()
	if len(logs) != 0 {
		t.Errorf("static children warned: %q", logs)
	}

	// Duplicate keys are reported.
	logs = nil
	tr3 := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { logs = append(logs, m) }})
	tr3.Mount(div(List([]Node{div(Key(1)), div(Key(1))})))
	tr3.Frame()
	if len(logs) != 1 || !strings.Contains(logs[0], "duplicate key") {
		t.Errorf("logs = %q, want one duplicate-key warning", logs)
	}
}

func TestKeyedFragment(t *testing.T) {
	pr := newProbe()
	group := func(k string) Node {
		return Fragment(Key(k), pr.item(k+"1"), pr.item(k+"2"))
	}
	tr := newTestTree(t, 300, 600, div(List([]Node{group("a"), group("b")})), nil)
	tr.Frame()
	pr.setters["b1"](1)
	tr.Frame()
	rerender(tr, div(List([]Node{group("b"), group("a")})))
	if got, want := texts(tr), "b1=1 b2=0 a1=0 a2=0"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if pr.mounts["b1"] != 1 {
		t.Errorf("b1 mounted %d times, want 1", pr.mounts["b1"])
	}
}

func TestNodeConstructorsPanic(t *testing.T) {
	for name, f := range map[string]func(){
		"non-comparable key": func() { Key([]int{1}) },
		"nil key":            func() { Key(nil) },
		"fragment class":     func() { Fragment(Class("p-4")) },
		"src on div":         func() { div(Src(image.NewRGBA(image.Rect(0, 0, 1, 1)))) },
		"href on div":        func() { div(Href("https://example.com")) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
}

// TestKeyedRandomEdits applies random insertions, removals and moves to a
// keyed list (with an unkeyed sibling before and after it) and checks
// that every surviving key keeps its Element and that only new keys
// mount.
func TestKeyedRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	pr := newProbe()
	list := func(keys []string) Node {
		return div(pr.item("head"),
			Map(keys, func(k string) Node { return WithKey(pr.item(k), k) }),
			pr.item("tail"))
	}
	var cur []string
	wantMounts := map[string]int{"head": 1, "tail": 1}
	tr := newTestTree(t, 300, 2000, list(cur), nil)
	tr.Frame()
	next := 0
	for step := 0; step < 300; step++ {
		keys := slices.Clone(cur)
		switch op := rng.IntN(4); {
		case op == 0 || len(keys) < 3: // insert
			k := fmt.Sprint("k", next)
			next++
			i := rng.IntN(len(keys) + 1)
			keys = slices.Insert(keys, i, k)
		case op == 1: // remove
			i := rng.IntN(len(keys))
			keys = slices.Delete(keys, i, i+1)
		case op == 2: // move one
			i, j := rng.IntN(len(keys)), rng.IntN(len(keys))
			k := keys[i]
			keys = slices.Insert(slices.Delete(keys, i, i+1), j, k)
		default: // shuffle
			rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		}
		for _, k := range keys {
			if !slices.Contains(cur, k) {
				wantMounts[k]++
			}
		}
		cur = keys
		rerender(tr, list(cur))
		want := []string{"head=0"}
		for _, k := range cur {
			want = append(want, k+"=0")
		}
		want = append(want, "tail=0")
		if got := texts(tr); got != strings.Join(want, " ") {
			t.Fatalf("step %d: %q, want %q", step, got, strings.Join(want, " "))
		}
		for k, n := range wantMounts {
			if slices.Contains(cur, k) || k == "head" || k == "tail" {
				if pr.mounts[k] != n {
					t.Fatalf("step %d: %s mounted %d times, want %d", step, k, pr.mounts[k], n)
				}
			}
		}
	}
}
