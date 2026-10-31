package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// Tests of ADR-205: Node is the interface every renderable implements, so
// a struct component is a Node wherever one is consumed, nil is the hole,
// and a Prop where a node is expected panics with the fix.

// cardOf is a struct component whose Build returns another struct
// component: composition without a Fragment.
type cardOf struct {
	Title string
	P     *compProbe
}

func (c cardOf) Build() Node { return stateCard{Title: c.Title, P: c.P} }

// propCard returns a Prop from Build, which compiles (a Prop has a Build
// method) and panics when the node is used.
type propCard struct{}

func (propCard) Build() Node { return Class("p-4") }

// stylePropCard returns a StyleProp from Build.
type stylePropCard struct{}

func (stylePropCard) Build() Node { return StyleOps(style.Op{}) }

// selfCard returns its own receiver: it would build itself forever.
type selfCard struct{ N int }

func (c selfCard) Build() Node { return c }

// selfList returns its own receiver too, but cannot be compared.
type selfList struct{ Items []int }

func (c selfList) Build() Node { return c }

// countdown returns another value of its own type until N is 0: a
// recursion that ends.
type countdown struct{ N int }

func (c countdown) Build() Node {
	if c.N == 0 {
		return Text("done")
	}
	return countdown{N: c.N - 1}
}

// TestStructComponentAsNode: a struct component is returned as a Node from
// a function component, from Build, from the callbacks of Map, MapIndex,
// CP and Memo, as an item of List and NodeList, and as the root, and it
// keeps its state (its own boundary) in every one of them.
func TestStructComponentAsNode(t *testing.T) {
	for name, c := range map[string]struct {
		root func(p *compProbe) Node
		want string
	}{
		"C": {func(p *compProbe) Node {
			return div(Component(func() Node { return stateCard{Title: "a", P: p} }))
		}, "a"},
		"Build": {func(p *compProbe) Node { return div(cardOf{Title: "a", P: p}) }, "a"},
		"Map": {func(p *compProbe) Node {
			return div(Map([]string{"a", "b"}, func(s string) Node { return WithKey(stateCard{Title: s, P: p}, s) }))
		}, "a"},
		"Map unkeyed": {func(p *compProbe) Node {
			return div(Map([]string{"a"}, func(s string) Node { return stateCard{Title: s, P: p} }))
		}, "a"},
		"MapIndex": {func(p *compProbe) Node {
			return div(MapIndex([]string{"a"}, func(_ int, s string) Node { return stateCard{Title: s, P: p} }))
		}, "a"},
		"CP": {func(p *compProbe) Node {
			return div(ComponentProps(func(title string) Node { return stateCard{Title: title, P: p} }, "a"))
		}, "a"},
		"Memo": {func(p *compProbe) Node {
			return div(Memo(func(title string) Node { return stateCard{Title: title, P: p} }, "a", func(a, b string) bool { return a == b }))
		}, "a"},
		"List":     {func(p *compProbe) Node { return div(List([]Node{stateCard{Title: "a", P: p}})) }, "a"},
		"NodeList": {func(p *compProbe) Node { return div(NodeList([]Node{stateCard{Title: "a", P: p}})) }, "a"},
		"root":     {func(p *compProbe) Node { return stateCard{Title: "a", P: p} }, "a"},
		"Fragment": {func(p *compProbe) Node { return div(Fragment(stateCard{Title: "a", P: p})) }, "a"},
	} {
		t.Run(name, func(t *testing.T) {
			p := newCompProbe()
			tr := newTestTree(t, 300, 200, c.root(p), nil)
			tr.Frame()
			if got := texts(tr); !strings.HasPrefix(got, c.want+" 0") {
				t.Fatalf("text %q, want %s 0...", got, c.want)
			}
			p.sets[c.want].Set(4)
			tr.Frame()
			// The parent builds again: the state is kept.
			tr.Mount(c.root(p))
			tr.Frame()
			if got := texts(tr); !strings.HasPrefix(got, c.want+" 4") {
				t.Errorf("after a rebuild: text %q, want %s 4...", got, c.want)
			}
		})
	}
}

// TestNilIsTheHole: a nil Node keeps the positions of its siblings, as a
// child of an element, of a Fragment and as an item of a list, so toggling
// it does not remount them; a component that returns nil renders nothing.
func TestNilIsTheHole(t *testing.T) {
	for name, wrap := range map[string]func(first Node, p *compProbe) Node{
		"element":  func(first Node, p *compProbe) Node { return div(first, stateCard{Title: "b", P: p}) },
		"fragment": func(first Node, p *compProbe) Node { return div(Fragment(first, stateCard{Title: "b", P: p})) },
		"list":     func(first Node, p *compProbe) Node { return div(List([]Node{first, stateCard{Title: "b", P: p}})) },
		"If":       func(first Node, p *compProbe) Node { return div(If(first != nil, first), stateCard{Title: "b", P: p}) },
	} {
		t.Run(name, func(t *testing.T) {
			p := newCompProbe()
			show := true
			root := func() Node {
				var first Node
				if show {
					first = stateCard{Title: "a", P: p}
				}
				return wrap(first, p)
			}
			tr := newTestTree(t, 300, 200, Component(root), nil)
			tr.Frame()
			p.sets["b"].Set(3)
			tr.Frame()
			for _, s := range []bool{false, true, false} {
				show = s
				mounts := tr.Stats().Mounts
				tr.Mount(Component(root))
				tr.Frame()
				want := "b 3"
				if s {
					want = "a 0 b 3"
				}
				if got := texts(tr); got != want {
					t.Errorf("show %v: text %q, want %q (the sibling keeps its state)", s, got, want)
				}
				if !s && tr.Stats().Mounts != mounts {
					t.Errorf("show %v: %d mounts; hiding the first child remounted its sibling", s, tr.Stats().Mounts-mounts)
				}
			}
		})
	}
	tr := newTestTree(t, 100, 100, div(Component(func() Node { return nil }), Text("x")), nil)
	tr.Frame()
	if got := texts(tr); got != "x" {
		t.Errorf("a component that returns nil: text %q", got)
	}
	if If(false, Text("x")) != nil || NodeOf(nil) != nil || WithKey(nil, 1) != nil {
		t.Error("an absent node is not nil")
	}
}

// TestPropAsNodePanics: a Prop where a node is consumed compiles (it has
// a Build method) and panics with a message that names the place and the
// fix.
func TestPropAsNodePanics(t *testing.T) {
	for name, c := range map[string]struct {
		mount func(t *testing.T)
		want  string
	}{
		"Build": {func(t *testing.T) { newTestTree(t, 100, 100, div(propCard{}), nil) },
			"tela: propCard.Build returned a Prop (Class), not a node or a component; props go to elements, such as Div(Class(...), ...)"},
		"StyleProp from Build": {func(t *testing.T) { newTestTree(t, 100, 100, div(stylePropCard{}), nil) },
			"stylePropCard.Build returned a Prop (Style)"},
		"C": {func(t *testing.T) {
			newTestTree(t, 100, 100, div(Component(func() Node { return Class("x") })), nil)
		}, "core.TestPropAsNodePanics.func"},
		"Map": {func(t *testing.T) {
			div(Map([]int{1, 2}, func(i int) Node {
				if i == 2 {
					return Class("x")
				}
				return Text("a")
			}))
		}, "item 1 of Map is a Prop (Class)"},
		"List":     {func(t *testing.T) { List([]Node{Text("a"), Key("k")}) }, "item 1 of a list is a Prop (Key)"},
		"NodeList": {func(t *testing.T) { NodeList([]Node{Class("x")}) }, "item 0 of h.Nodes is a Prop (Class)"},
		"root":     {func(t *testing.T) { newTestTree(t, 100, 100, Class("x"), nil) }, "the root is a Prop (Class)"},
		"VirtualList": {func(t *testing.T) {
			tr := newTestTree(t, 100, 100, div(Class("h-24"), VirtualList(3, func(int) float32 { return 20 }, func(int) Node { return Class("x") }, Class("h-full"))), nil)
			settleTree(t, tr)
		}, "item 0 of a VirtualList is a Prop (Class)"},
	} {
		t.Run(name, func(t *testing.T) {
			msg := panicMessage(func() { c.mount(t) })
			if !strings.Contains(msg, c.want) || !strings.Contains(msg, "not a node or a component; props go to elements") {
				t.Errorf("panic %q, want it to contain %q and the fix", msg, c.want)
			}
		})
	}
}

// TestBuildReturningItselfPanics: in dev mode a Build that returns its own
// receiver panics instead of building itself until the stack overflows; a
// component that returns other values of its type until it stops is fine.
func TestBuildReturningItselfPanics(t *testing.T) {
	if msg := panicMessage(func() { devTree(t, div(selfCard{N: 1}), nil) }); !strings.Contains(msg, "tela: selfCard.Build returned its own receiver") {
		t.Errorf("comparable: panic %q", msg)
	}
	if msg := panicMessage(func() { devTree(t, div(selfList{Items: []int{1}}), nil) }); !strings.Contains(msg, "1000 selfLists are built one inside the other") {
		t.Errorf("not comparable: panic %q", msg)
	}
	for _, dev := range []bool{false, true} {
		var tr *Tree
		if dev {
			tr = devTree(t, div(countdown{N: 20}), nil)
		} else {
			tr = newTestTree(t, 100, 100, div(countdown{N: 20}), nil)
		}
		tr.Frame()
		if got := texts(tr); got != "done" {
			t.Errorf("dev %v: countdown: text %q", dev, got)
		}
	}
}

// TestNodeFieldComparesWithoutPanicking: a struct component in a Node
// field of another is compared by value, and one that cannot be compared
// counts as changed instead of panicking (Node == Node would).
func TestNodeFieldComparesWithoutPanicking(t *testing.T) {
	a := stateCard{Title: "x", Children: listCard{Items: []string{"i"}}}
	b := stateCard{Title: "x", Children: listCard{Items: []string{"i"}}}
	if componentEqual(a, b) {
		t.Error("two values holding a non-comparable component are equal")
	}
	c := stateCard{Title: "x", Children: otherCard{Title: "y"}}
	d := stateCard{Title: "x", Children: otherCard{Title: "y"}}
	if !componentEqual(c, d) {
		t.Error("two values holding equal components differ")
	}
}
