package core

import (
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// Tests of struct components (ADR-088): Component values other than Node
// and Prop given to elements.

// compProbe records what the struct components below do.
type compProbe struct {
	builds map[string]int
	sets   map[string]Setter[int]
}

func newCompProbe() *compProbe {
	return &compProbe{builds: map[string]int{}, sets: map[string]Setter[int]{}}
}

// stateCard is a struct component with state and children.
type stateCard struct {
	Title    string
	P        *compProbe
	Children Comp
}

func (c stateCard) Build() Node {
	c.P.builds[c.Title]++
	n, set := UseState(0)
	c.P.sets[c.Title] = set
	return div(Textf("%s %d", c.Title, n), c.Children)
}

// otherCard is another struct component type, with the same shape.
type otherCard struct {
	Title string
	P     *compProbe
}

func (c otherCard) Build() Node {
	c.P.builds["other "+c.Title]++
	n, _ := UseState(100)
	return div(Textf("other %s %d", c.Title, n))
}

// listCard is a struct component that is not comparable.
type listCard struct {
	Items []string
	P     *compProbe
}

func (c listCard) Build() Node {
	c.P.builds["list"]++
	return div(Text(strings.Join(c.Items, ",")))
}

// itemsCard holds a slice of Components.
type itemsCard struct{ Items []Comp }

func (c itemsCard) Build() Node { return div(Fragment(asChildren(c.Items)...)) }

func asChildren(cs []Comp) []Child {
	out := make([]Child, len(cs))
	for i, c := range cs {
		out[i] = c
	}
	return out
}

// attrsCard takes props in a field of type Child.
type attrsCard struct{ Attrs Child }

func (c attrsCard) Build() Node { return div(c.Attrs, Text("attrs")) }

// ptrCard declares Build with a pointer receiver.
type ptrCard struct{}

func (c *ptrCard) Build() Node { return Text("ptr") }

// closureCard calls hooks from function literals inside Build.
type closureCard struct{}

func (closureCard) Build() Node {
	inner := func() int {
		n, _ := UseState(1)
		return n
	}
	return Textf("closure %d", inner())
}

// baseCard and fancyCard: a Build promoted from an embedded type.
type baseCard struct{ Label string }

func (b baseCard) Build() Node {
	n, _ := UseState(2)
	return Textf("%s %d", b.Label, n)
}

type fancyCard struct{ baseCard }

// genericCard is a generic struct component.
type genericCard[T comparable] struct{ V T }

func (g genericCard[T]) Build() Node {
	v, _ := UseState(g.V)
	inner := func() int {
		n, _ := UseState(4)
		return n
	}
	return Textf("generic %v %d", v, inner())
}

// propReturner returns a node made from a Prop.
type propReturner struct{}

func (propReturner) Build() Node { return Class("p-4").Build() }

func TestStructComponentOwnState(t *testing.T) {
	p := newCompProbe()
	page := func() Node {
		return div(stateCard{Title: "a", P: p}, stateCard{Title: "b", P: p})
	}
	tr := newTestTree(t, 300, 200, Component(page), nil)
	tr.Frame()
	if got := texts(tr); got != "a 0 b 0" {
		t.Fatalf("text %q", got)
	}
	p.sets["a"].Set(5)
	tr.Frame()
	if got := texts(tr); got != "a 5 b 0" {
		t.Errorf("after setting a: text %q, want a 5 b 0", got)
	}
	if p.builds["a"] != 2 || p.builds["b"] != 1 {
		t.Errorf("builds %v, want a:2 b:1 (each card is a boundary of its own)", p.builds)
	}
	// The page building again keeps the state, and skips both cards
	// (equal values).
	before := tr.Stats()
	tr.Mount(Component(page))
	tr.Frame()
	if got := texts(tr); got != "a 5 b 0" {
		t.Errorf("after a rebuild of the page: text %q", got)
	}
	if s := tr.Stats(); s.Skips-before.Skips != 2 || p.builds["a"] != 2 || p.builds["b"] != 1 {
		t.Errorf("%d skips, builds %v; want 2 skips and no card build", s.Skips-before.Skips, p.builds)
	}
}

func TestStructComponentIdentityIsTypeAndKey(t *testing.T) {
	p := newCompProbe()
	other := false
	page := func() Node {
		if other {
			return div(otherCard{Title: "a", P: p})
		}
		return div(stateCard{Title: "a", P: p})
	}
	tr := newTestTree(t, 300, 200, Component(page), nil)
	p.sets["a"].Set(7)
	tr.Frame()
	if got := texts(tr); got != "a 7" {
		t.Fatalf("text %q", got)
	}
	// Another type at the same position remounts.
	other = true
	m := tr.Stats().Mounts
	tr.Mount(Component(page))
	tr.Frame()
	if got := texts(tr); got != "other a 100" {
		t.Errorf("another type: text %q, want other a 100", got)
	}
	if tr.Stats().Mounts == m {
		t.Error("another type at the same position did not mount")
	}
	// Back: a fresh stateCard, with fresh state.
	other = false
	tr.Mount(Component(page))
	tr.Frame()
	if got := texts(tr); got != "a 0" {
		t.Errorf("back to the first type: text %q, want a 0 (new state)", got)
	}

	// Keys: the state follows the key when the list is reordered.
	ids := []string{"x", "y", "z"}
	list := func() Node {
		return div(Map(ids, func(id string) Node { return WithKey(stateCard{Title: id, P: p}, id) }))
	}
	tr = newTestTree(t, 300, 200, Component(list), nil)
	p.sets["y"].Set(3)
	tr.Frame()
	ids = []string{"z", "y", "x"}
	tr.Mount(Component(list))
	tr.Frame()
	if got := texts(tr); got != "z 0 y 3 x 0" {
		t.Errorf("reordered keyed list: text %q, want z 0 y 3 x 0", got)
	}
}

func TestStructComponentSkipping(t *testing.T) {
	p := newCompProbe()
	title := "a"
	var kids func() Comp
	page := func() Node {
		var children Comp
		if kids != nil {
			children = kids()
		}
		return div(stateCard{Title: title, P: p, Children: children})
	}
	rebuild := func(tr *Tree) {
		tr.Mount(Component(page))
		tr.Frame()
	}
	for _, dev := range []bool{false, true} {
		clear(p.builds)
		title, kids = "a", nil
		var tr *Tree
		if dev {
			tr = devTree(t, Component(page), nil)
		} else {
			tr = newTestTree(t, 300, 200, Component(page), nil)
		}
		rebuild(tr)
		if p.builds["a"] != 1 {
			t.Errorf("dev %v: equal values: %d builds, want 1", dev, p.builds["a"])
		}
		// Changed props rebuild.
		title = "b"
		rebuild(tr)
		if p.builds["b"] != 1 {
			t.Errorf("dev %v: changed title: builds %v", dev, p.builds)
		}
		// A fresh node in Children is a change; the same node is not.
		kids = func() Comp { return Text("fresh") }
		rebuild(tr)
		rebuild(tr)
		if p.builds["b"] != 3 {
			t.Errorf("dev %v: fresh children: %d builds of b, want 3", dev, p.builds["b"])
		}
		kept := Text("kept")
		kids = func() Comp { return kept }
		rebuild(tr)
		rebuild(tr)
		if p.builds["b"] != 4 {
			t.Errorf("dev %v: the same child node: %d builds of b, want 4", dev, p.builds["b"])
		}
		// A child that cannot be compared (a component holding a slice)
		// counts as changed, without panicking.
		kids = func() Comp { return listCard{Items: []string{"i"}, P: p} }
		rebuild(tr)
		rebuild(tr)
		if p.builds["b"] != 6 {
			t.Errorf("dev %v: non-comparable child: %d builds of b, want 6", dev, p.builds["b"])
		}
		if got := texts(tr); got != "b 0 i" {
			t.Errorf("dev %v: text %q", dev, got)
		}
	}
	// A component type that is not comparable always builds.
	clear(p.builds)
	items := func() Node { return div(listCard{Items: []string{"a", "b"}, P: p}) }
	tr := newTestTree(t, 300, 200, Component(items), nil)
	tr.Mount(Component(items))
	tr.Mount(Component(items))
	if p.builds["list"] != 3 {
		t.Errorf("non-comparable component: %d builds, want 3", p.builds["list"])
	}
}

func TestComponentEqual(t *testing.T) {
	type inner struct {
		A any
		b string
	}
	type outer struct {
		N    int
		F    float64
		I    any
		In   inner
		Arr  [2]any
		Ptr  *int
		Node Node
	}
	x, y := 1, 1
	n := Text("n")
	base := outer{N: 1, I: "s", In: inner{A: 2, b: "u"}, Arr: [2]any{1, "a"}, Ptr: &x, Node: n}
	with := func(f func(*outer)) outer {
		o := base
		f(&o)
		return o
	}
	for _, c := range []struct {
		name string
		a, b any
		want bool
	}{
		{"same", base, base, true},
		{"nil and nil", nil, nil, true},
		{"different types", stateCard{}, otherCard{}, false},
		{"field", base, with(func(o *outer) { o.N = 2 }), false},
		{"NaN", with(func(o *outer) { o.F = math.NaN() }), with(func(o *outer) { o.F = math.NaN() }), false},
		{"interface value", base, with(func(o *outer) { o.I = "t" }), false},
		{"interface type", base, with(func(o *outer) { o.I = 1 }), false},
		{"interface with a slice", with(func(o *outer) { o.I = []int{1} }), with(func(o *outer) { o.I = []int{1} }), false},
		{"nested interface with a map", with(func(o *outer) { o.In.A = map[int]int{} }), with(func(o *outer) { o.In.A = map[int]int{} }), false},
		{"unexported field", base, with(func(o *outer) { o.In.b = "v" }), false},
		{"array of interfaces", base, with(func(o *outer) { o.Arr[1] = "b" }), false},
		{"other pointer", base, with(func(o *outer) { o.Ptr = &y }), false},
		{"nil interface", with(func(o *outer) { o.I = nil }), with(func(o *outer) { o.I = nil }), true},
		{"fresh node", base, with(func(o *outer) { o.Node = Text("n") }), false},
		{"not comparable", listCard{}, listCard{}, false},
		{"plain comparable", otherCard{Title: "a"}, otherCard{Title: "a"}, true},
		{"component with a prop child", stateCard{Children: Class("x")}, stateCard{Children: Class("x")}, false},
	} {
		var got bool
		if msg := panicMessage(func() { got = componentEqual(c.a, c.b) }); msg != "" {
			t.Errorf("%s: panicked: %s", c.name, msg)
			continue
		}
		if got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStructComponentDevPanics(t *testing.T) {
	p := newCompProbe()
	for _, c := range []struct {
		name string
		root Node
		want string
	}{
		{"prop in a Component field", div(stateCard{Title: "a", P: p, Children: Class("p-4")}),
			"stateCard.Children holds a Prop (Class), not a component"},
		{"prop node in a Component field", div(stateCard{Title: "a", P: p, Children: Key("k").Build()}),
			"stateCard.Children holds a Prop (Key), not a component"},
		{"prop in a slice of Components", div(itemsCard{Items: []Comp{Text("a"), Class("x")}}),
			"itemsCard.Items[1] holds a Prop (Class)"},
		{"pointer to a value-receiver component", div(&stateCard{Title: "a", P: p}),
			"&stateCard{...} (a *stateCard) is given as a component; pass the value, stateCard{...}, without the &"},
		{"pointer receiver", div(&ptrCard{}),
			"ptrCard.Build has a pointer receiver; declare it with a value receiver, func (c ptrCard) Build() tela.Node"},
	} {
		msg := panicMessage(func() { devTree(t, c.root, nil) })
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s: panic %q\nwant it to contain %q", c.name, msg, c.want)
		}
	}
	// The check runs on updates too.
	bad := false
	page := func() Node {
		var kid Comp = Text("ok")
		if bad {
			kid = Class("p-2")
		}
		return div(stateCard{Title: "u", P: p, Children: kid})
	}
	tr := devTree(t, Component(page), nil)
	bad = true
	if msg := panicMessage(func() { tr.Mount(Component(page)) }); !strings.Contains(msg, "stateCard.Children holds a Prop (Class)") {
		t.Errorf("update: panic %q", msg)
	}

	// Outside dev mode nothing is checked: the prop applies to the div.
	if msg := panicMessage(func() { newTestTree(t, 100, 100, div(stateCard{Title: "a", P: p, Children: Class("p-4")}), nil) }); msg != "" {
		t.Errorf("non-dev: panicked: %s", msg)
	}
	// A field of type Child takes props, also in dev mode.
	tr = devTree(t, div(attrsCard{Attrs: Class("p-4")}), nil)
	tr.Frame()
	if got := texts(tr); got != "attrs" {
		t.Errorf("attrs: text %q", got)
	}
}

func TestStructComponentHookBoundary(t *testing.T) {
	p := newCompProbe()
	// Build called directly inside a function component.
	page := func() Node { return div(stateCard{Title: "a", P: p}.Build()) }
	msg := panicMessage(func() { devTree(t, Component(page), nil) })
	for _, want := range []string{
		"UseState at structcomp_test.go:",
		"is called from core.stateCard.Build, which is not the component being built",
		"stateCard.Build was called directly",
		"pass stateCard{...} as a child instead of calling Build",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("direct Build: panic %q\ndoes not contain %q", msg, want)
		}
	}
	// Build called directly inside another struct component's Build.
	outer := div(stateCard{Title: "o", P: p, Children: Fragment(closureCard{})})
	if msg := panicMessage(func() { devTree(t, outer, nil) }); msg != "" {
		t.Errorf("closure hooks in Build: panicked: %s", msg)
	}
	// Build called outside any build.
	msg = panicMessage(func() { stateCard{Title: "x", P: p}.Build() })
	if !strings.Contains(msg, "from core.stateCard.Build") || !strings.Contains(msg, "Pass the component value itself instead of calling Build") {
		t.Errorf("Build outside a build: panic %q", msg)
	}
	// A Build promoted from an embedded type owns its hooks.
	tr := devTree(t, div(fancyCard{baseCard{Label: "fancy"}}), nil)
	tr.Frame()
	if got := texts(tr); got != "fancy 2" {
		t.Errorf("promoted Build: text %q", got)
	}
	tr = devTree(t, div(closureCard{}), nil)
	tr.Frame()
	if got := texts(tr); got != "closure 1" {
		t.Errorf("closure: text %q", got)
	}
	// Generic types: two instantiations are two types.
	tr = devTree(t, div(genericCard[string]{V: "s"}, genericCard[int]{V: 7}), nil)
	tr.Frame()
	if got := texts(tr); got != "generic s 4generic 7 4" { // one paragraph
		t.Errorf("generic: text %q", got)
	}
	if a, b := componentOf(genericCard[string]{}), componentOf(genericCard[int]{}); a.d.comp.rtype == b.d.comp.rtype {
		t.Error("two instantiations share a type")
	}
}

func TestStructComponentNames(t *testing.T) {
	p := newCompProbe()
	tr := newTestTree(t, 300, 200, div(stateCard{Title: "a", P: p}), nil)
	tr.Frame()
	root := tr.Inspect()
	var names []string
	var walk func(n *InspectNode)
	walk = func(n *InspectNode) {
		if n.Component != "" {
			names = append(names, n.Component)
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&root)
	if len(names) == 0 || names[0] != "github.com/kuronosu/tela/internal/core.stateCard" {
		t.Errorf("inspect names %q, want the type name", names)
	}
	if s := componentOf(stateCard{}).String(); s != "<github.com/kuronosu/tela/internal/core.stateCard>" {
		t.Errorf("String %q", s)
	}
}

func TestNodeAndPropAsComponents(t *testing.T) {
	n := Text("x")
	if n.Build() != n {
		t.Error("Node.Build does not return the node")
	}
	// A Prop's node applies like the Prop where props go.
	d := div(Class("p-4").Build(), Text("y"))
	if len(d.d.classes) != 1 || d.d.classes[0] != "p-4" || len(d.d.children) != 1 {
		t.Errorf("div of a prop node: classes %q, %d children", d.d.classes, len(d.d.children))
	}
	if f := Fragment(Key("k").Build(), Text("z")); !f.d.keyed || f.d.key != "k" {
		t.Error("Fragment of a Key node: not keyed")
	}
	if msg := panicMessage(func() { Fragment(Class("x").Build()) }); !strings.Contains(msg, "Fragment does not take Class") {
		t.Errorf("Fragment of a Class node: panic %q", msg)
	}
	// Rendered on its own, it panics.
	if msg := panicMessage(func() { newTestTree(t, 100, 100, div(propReturner{}), nil) }); !strings.Contains(msg, "a Prop (Class) cannot be rendered on its own") {
		t.Errorf("a component returning a prop node: panic %q", msg)
	}
	// NodeOf and WithKey.
	if !NodeOf(nil).IsZero() || NodeOf(n) != n {
		t.Error("NodeOf of nil or of a node")
	}
	if msg := panicMessage(func() { NodeOf(Class("x")) }); !strings.Contains(msg, "the root is a Prop (Class)") {
		t.Errorf("NodeOf(Prop): panic %q", msg)
	}
	if msg := panicMessage(func() { WithKey(Class("x"), 1) }); !strings.Contains(msg, "the first argument of WithKey is a Prop (Class)") {
		t.Errorf("WithKey(Prop): panic %q", msg)
	}
	k := WithKey(stateCard{Title: "k"}, 7)
	if key, ok := k.Key(); !ok || key != 7 || k.d.comp.ct == nil {
		t.Errorf("WithKey of a struct component: key %v %v, struct %v", key, ok, k.d.comp.ct != nil)
	}
	if !WithKey(nil, 1).IsZero() {
		t.Error("WithKey(nil) is not the zero Node")
	}
	// A nil Component field is a hole.
	p := newCompProbe()
	tr := newTestTree(t, 100, 100, div(stateCard{Title: "h", P: p}), nil)
	tr.Frame()
	if got := texts(tr); got != "h 0" {
		t.Errorf("nil children: text %q", got)
	}
}

func TestOnceDependency(t *testing.T) {
	runs, computes, layouts := 0, 0, 0
	var set Setter[int]
	comp := func() Node {
		n, s := UseState(0)
		set = s
		UseEffect(func() func() { runs++; return nil }, Once)
		UseLayoutEffect(func() func() { layouts++; return nil }, Once)
		v := UseMemo(func() int { computes++; return n }, Once)
		return Textf("%d %d", n, v)
	}
	var logs []string
	tr := devTree(t, Component(comp), &logs)
	tr.Frame()
	tr.FlushEffects()
	for i := 1; i <= 2; i++ {
		set.Set(i)
		tr.Frame()
		tr.FlushEffects()
	}
	if runs != 1 || layouts != 1 || computes != 1 {
		t.Errorf("runs %d, layouts %d, computes %d; want 1 each", runs, layouts, computes)
	}
	if got := texts(tr); got != "2 0" {
		t.Errorf("text %q, want 2 0", got)
	}
	if len(logs) != 0 {
		t.Errorf("warnings for Once: %q", logs)
	}
	// UseUpdateEffect with Once never runs: dev mode says so.
	logs = nil
	devTree(t, Component(func() Node {
		UseUpdateEffect(func() func() { return nil }, Once)
		return Text("u")
	}), &logs)
	if all := strings.Join(logs, "\n"); !strings.Contains(all, "UseUpdateEffect(f, tela.Once)") || !strings.Contains(all, "never runs") {
		t.Errorf("UseUpdateEffect with Once: logs %q", logs)
	}
	if Once != any(onceDep{}) || Once.String() != "tela.Once" {
		t.Error("Once is not equal to itself, or misnamed")
	}
}

func TestInitialOption(t *testing.T) {
	// The values the box is painted at, frame after frame (newHarness
	// settles: it runs the animation to its end).
	paints := func(opts ...AnimOption) []float32 {
		var seen []float32
		newHarness(t, Component(func() Node {
			v := UseSpring(float32(40), opts...)
			return div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) {
				seen = append(seen, v.Get())
				return v.Get(), 0
			})))
		}), false)
		return seen
	}
	// Without Initial the value starts at the target: nothing moves.
	if seen := paints(); len(seen) != 1 || seen[0] != 40 {
		t.Errorf("without Initial: painted at %v, want [40]", seen)
	}
	// With it, the first build holds it and moves to the target.
	if seen := paints(Initial(float32(0))); len(seen) < 3 || seen[0] != 0 || seen[len(seen)-1] != 40 {
		t.Errorf("with Initial(0): painted at %v, want a spring from 0 to 40", seen)
	}

	// A mismatched type panics in dev mode and is ignored otherwise.
	mismatch := func() Node {
		p := UseSpring(Point{X: 5}, Initial(float32(0)))
		return Textf("%v", p.Get().X)
	}
	msg := panicMessage(func() { devTree(t, Component(mismatch), nil) })
	if !strings.Contains(msg, "UseSpring: Initial(float32) for a Value of core.Point") {
		t.Errorf("mismatched Initial: panic %q", msg)
	}
	tr := newTestTree(t, 100, 100, Component(mismatch), nil)
	tr.Frame()
	if got := texts(tr); got != "5" {
		t.Errorf("mismatched Initial outside dev mode: text %q, want 5 (the target)", got)
	}
	// Initial does not apply to Animate.
	var w *Value[float32]
	tr = devTree(t, Component(func() Node {
		w = UseValue(float32(0))
		return Text("a")
	}), nil)
	if msg := panicMessage(func() { Animate(w, 10, Initial(float32(3))) }); !strings.Contains(msg, "Initial applies to UseSpring and UseTween") {
		t.Errorf("Initial on Animate: panic %q", msg)
	}
	_ = tr
}
