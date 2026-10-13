package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// buildCounter counts builds per name.
type buildCounter map[string]int

type cardProps struct {
	Title string
	N     int
}

// skipCard is a named component with comparable props.
var skipCardBuilds = buildCounter{}

func skipCard(p cardProps) Node {
	skipCardBuilds[p.Title]++
	return div(Key(p.Title), Textf("%s %d", p.Title, p.N))
}

func TestCPSkipsEqualProps(t *testing.T) {
	clear(skipCardBuilds)
	n := 0
	page := func() Node {
		return div(
			ComponentProps(skipCard, cardProps{"a", 1}),
			ComponentProps(skipCard, cardProps{"b", n}),
		)
	}
	tr := newTestTree(t, 300, 200, Component(page), nil)
	tr.Frame()
	s0 := tr.Stats()
	tr.Mount(Component(page))
	tr.Frame()
	s1 := tr.Stats()
	if got := s1.Skips - s0.Skips; got != 2 {
		t.Errorf("%d skips, want 2", got)
	}
	if got := s1.Builds - s0.Builds; got != 1 {
		t.Errorf("%d builds, want 1 (the page only)", got)
	}
	if skipCardBuilds["a"] != 1 || skipCardBuilds["b"] != 1 {
		t.Errorf("card builds %v, want one each", skipCardBuilds)
	}
	// New props build only that card.
	n = 5
	tr.Mount(Component(page))
	tr.Frame()
	if skipCardBuilds["a"] != 1 || skipCardBuilds["b"] != 2 {
		t.Errorf("card builds %v, want a:1 b:2", skipCardBuilds)
	}
	if got := texts(tr); got != "a 1 b 5" {
		t.Errorf("text %q", got)
	}
}

func TestCPWithNonComparableDynamicProps(t *testing.T) {
	// P is comparable at compile time, but an interface holding a slice
	// panics on ==: it counts as changed.
	builds := 0
	comp := func(p any) Node { builds++; return Text("x") }
	tr := newTestTree(t, 100, 100, ComponentProps(comp, any([]int{1})), nil)
	tr.Mount(ComponentProps(comp, any([]int{1})))
	if builds != 2 {
		t.Errorf("%d builds, want 2", builds)
	}
	tr.Mount(ComponentProps(comp, any(3)))
	tr.Mount(ComponentProps(comp, any(3)))
	if builds != 3 {
		t.Errorf("%d builds, want 3", builds)
	}
}

func TestMemoSkipsWithEqual(t *testing.T) {
	builds := 0
	chart := func(series []int) Node { builds++; return Textf("%v", series) }
	equal := func(a, b []int) bool { return slices.Equal(a, b) }
	tr := newTestTree(t, 200, 100, Memo(chart, []int{1, 2}, equal), nil)
	tr.Mount(Memo(chart, []int{1, 2}, equal))
	tr.Frame()
	if builds != 1 || tr.Stats().Skips != 1 {
		t.Errorf("%d builds, %d skips; want 1 and 1", builds, tr.Stats().Skips)
	}
	tr.Mount(Memo(chart, []int{1, 3}, equal))
	tr.Frame()
	if builds != 2 || texts(tr) != "[1 3]" {
		t.Errorf("%d builds, text %q", builds, texts(tr))
	}
	if msg := panicMessage(func() { Memo(chart, nil, nil) }); !strings.Contains(msg, "Memo(fn, props, nil)") {
		t.Errorf("panic %q", msg)
	}
}

func TestComponentSkipsOnlyTheSameNode(t *testing.T) {
	builds := 0
	label := "a"
	comp := func() Node { builds++; return Text(label) }
	// Closures of C are built with their parent: they may capture
	// values that changed.
	page := func() Node { return div(Component(comp)) }
	tr := newTestTree(t, 100, 100, Component(page), nil)
	label = "b"
	tr.Mount(Component(page))
	tr.Frame()
	if builds != 2 || texts(tr) != "b" {
		t.Errorf("%d builds, text %q; C must build with its parent", builds, texts(tr))
	}
	// The very same node (here kept in a variable) is not built again.
	same := Component(comp)
	page2 := func() Node { return div(same) }
	tr2 := newTestTree(t, 100, 100, Component(page2), nil)
	tr2.Mount(Component(page2))
	if builds != 3 || tr2.Stats().Skips != 1 {
		t.Errorf("%d builds, %d skips; the same node must skip", builds, tr2.Stats().Skips)
	}
}

func TestSkippedComponentKeepsNewestClosure(t *testing.T) {
	// When a CP skips, the new node is kept: a later build of its own
	// (a state change) runs the newest closure.
	var set Setter[int]
	suffix := "1"
	page := func() Node {
		s := suffix
		return div(ComponentProps(func(p int) Node {
			v, st := UseState(0)
			set = st
			return Textf("%d %d %s", p, v, s)
		}, 7))
	}
	tr := newTestTree(t, 200, 100, Component(page), nil)
	tr.Frame()
	suffix = "2"
	tr.Mount(Component(page)) // skipped: equal props
	set.Set(1)
	tr.Frame()
	if got := texts(tr); got != "7 1 2" {
		t.Errorf("text %q, want 7 1 2", got)
	}
}

// TestStateInSkippedSubtree updates state below a component that skips
// in the same frame as its parent's build: the descendant builds once,
// after its ancestors, and the skipped component does not.
func TestStateInSkippedSubtree(t *testing.T) {
	order := []string{}
	var setLeaf, setTop Setter[int]
	leaf := func() Node {
		v, s := UseState(0)
		setLeaf = s
		order = append(order, "leaf")
		return Textf("leaf %d", v)
	}
	middle := func(p cardProps) Node {
		order = append(order, "middle")
		return div(Component(leaf))
	}
	top := func() Node {
		v, s := UseState(0)
		setTop = s
		order = append(order, "top")
		return div(Textf("top %d", v), ComponentProps(middle, cardProps{Title: "m"}))
	}
	tr := newTestTree(t, 300, 200, Component(top), nil)
	tr.Frame()
	order = nil
	setLeaf.Set(1)
	setTop.Set(1)
	tr.Frame()
	if want := []string{"top", "leaf"}; !slices.Equal(order, want) {
		t.Errorf("build order %q, want %q", order, want)
	}
	if got := texts(tr); got != "top 1 leaf 1" {
		t.Errorf("text %q", got)
	}
	// Only the leaf.
	order = nil
	setLeaf.Set(2)
	tr.Frame()
	if want := []string{"leaf"}; !slices.Equal(order, want) {
		t.Errorf("build order %q, want %q", order, want)
	}
}

func TestEffectsOrderThroughSkippedComponent(t *testing.T) {
	var l effectLog
	var setLeaf, setTop Setter[int]
	leaf := func() Node {
		v, s := UseState(0)
		setLeaf = s
		UseEffect(l.effect("leaf"), v)
		return Text("leaf")
	}
	middle := func(int) Node { return div(Component(leaf)) }
	top := func() Node {
		v, s := UseState(0)
		setTop = s
		UseEffect(l.effect("top"), v)
		return div(ComponentProps(middle, 0))
	}
	tr := newTestTree(t, 300, 200, Component(top), nil)
	tr.Frame()
	tr.FlushEffects()
	l.take()
	setTop.Set(1)
	setLeaf.Set(1)
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "both updated", "~leaf", "~top", "leaf", "top")
}

func TestKeyedSkippedListReorders(t *testing.T) {
	pr := newProbe()
	ids := []string{"a", "b", "c"}
	list := func() Node {
		return div(Map(ids, func(id string) Node { return WithKey(pr.item(id), id) }))
	}
	tr := newTestTree(t, 300, 300, Component(list), nil)
	tr.Frame()
	pr.setters["b"](4)
	tr.Frame()
	s0 := tr.Stats()
	ids = []string{"c", "b", "a"}
	tr.Mount(Component(list))
	tr.Frame()
	s1 := tr.Stats()
	if got, want := texts(tr), "c=0 b=4 a=0"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
	if s1.Mounts != s0.Mounts || s1.Skips-s0.Skips != 3 {
		t.Errorf("%d mounts and %d skips, want 0 and 3", s1.Mounts-s0.Mounts, s1.Skips-s0.Skips)
	}
	// State updates of skipped items still apply.
	pr.setters["a"](9)
	tr.Frame()
	if got, want := texts(tr), "c=0 b=4 a=9"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
}

func namedCP(int) Node { return Text("named") }

func TestDevWarnsAnonymousCP(t *testing.T) {
	var logs []string
	var pr counterProbe
	pr.setters, pr.mounts, pr.inst = map[string]func(int){}, map[string]int{}, map[string]*int{}
	page := func() Node {
		items := make([]Node, 0, 6)
		for i := 0; i < 3; i++ {
			// One call site for three nodes: one warning.
			items = append(items, WithKey(ComponentProps(func(int) Node { return Text("anon") }, i), i))
		}
		items = append(items, WithKey(ComponentProps(namedCP, 1), "named"))
		items = append(items, WithKey(ComponentProps(pr.render, "m"), "method"))
		items = append(items, WithKey(Memo(func(int) Node { return Text("memo") }, 1, func(a, b int) bool { return a == b }), "memo"))
		return div(List(items))
	}
	tr := devTree(t, Component(page), &logs)
	tr.Mount(Component(page))
	if len(logs) != 3 {
		t.Fatalf("logs %q, want 3 warnings (closure, method value, Memo)", logs)
	}
	for i, want := range []string{
		"tela: CP(...) at core.TestDevWarnsAnonymousCP.func1 (skip_test.go:",
		"a method value (core.(*counterProbe).render-fm): its receiver is not compared",
		"tela: Memo(...) at core.TestDevWarnsAnonymousCP.func1 (skip_test.go:",
	} {
		if !strings.Contains(logs[i], want) {
			t.Errorf("log %d %q\ndoes not contain %q", i, logs[i], want)
		}
	}
}

// ---------------------------------------------------------------------
// Context.

var themeCtx = CreateContext("light")

func TestContextDefaultAndNearest(t *testing.T) {
	var got []string
	reader := func(name string) Node {
		got = append(got, name+"="+UseContext(themeCtx))
		return div(Text(name))
	}
	tr := newTestTree(t, 300, 200, div(
		ComponentProps(reader, "outside"),
		themeCtx.Provider("dark",
			ComponentProps(reader, "inner"),
			themeCtx.Provider("blue", div(ComponentProps(reader, "nested"))),
		),
	), nil)
	tr.Frame()
	if want := []string{"outside=light", "inner=dark", "nested=blue"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// A provider has no box: its children are its parent's.
	if got := texts(tr); got != "outside inner nested" {
		t.Errorf("texts %q", got)
	}
}

func TestContextRebuildsOnlyReaders(t *testing.T) {
	builds := buildCounter{}
	var setTheme Setter[string]
	reader := func(name string) Node {
		builds[name]++
		return div(Textf("%s:%s", name, UseContext(themeCtx)))
	}
	plain := func(name string) Node {
		builds[name]++
		return div(Text(name))
	}
	middle := func(name string) Node {
		builds[name]++
		return div(ComponentProps(plain, name+"/plain"), ComponentProps(reader, name+"/reader"))
	}
	app := func() Node {
		theme, s := UseState("dark")
		setTheme = s
		builds["app"]++
		return themeCtx.Provider(theme,
			ComponentProps(middle, "m"),
			ComponentProps(plain, "p"),
		)
	}
	tr := newTestTree(t, 300, 200, Component(app), nil)
	tr.Frame()
	clear(builds)
	s0 := tr.Stats()
	setTheme.Set("light")
	tr.Frame()
	if want := (buildCounter{"app": 1, "m/reader": 1}); fmt.Sprint(builds) != fmt.Sprint(want) {
		t.Errorf("builds %v, want %v", builds, want)
	}
	if got := tr.Stats().Skips - s0.Skips; got != 2 {
		t.Errorf("%d skips, want 2 (middle and p)", got)
	}
	if got := texts(tr); got != "m/plain m/reader:light p" {
		t.Errorf("texts %q", got)
	}
	// An equal value rebuilds no reader.
	clear(builds)
	setTheme.Set("light")
	setTheme.Update(func(s string) string { return s + "" })
	tr.Frame()
	if len(builds) != 0 {
		t.Errorf("builds %v, want none", builds)
	}
}

func TestContextEqualProviderValueSkipsReaders(t *testing.T) {
	builds := 0
	reader := func(int) Node {
		builds++
		return Text(UseContext(themeCtx))
	}
	page := func(v string) Node { return themeCtx.Provider(v, ComponentProps(reader, 0)) }
	tr := newTestTree(t, 100, 100, page("a"), nil)
	tr.Mount(page("a"))
	tr.Mount(page("a"))
	if builds != 1 {
		t.Errorf("%d builds, want 1 (equal value, equal props)", builds)
	}
	tr.Mount(page("b"))
	tr.Frame()
	if builds != 2 || texts(tr) != "b" {
		t.Errorf("%d builds, text %q", builds, texts(tr))
	}
}

func TestContextNonComparableValueAlwaysChanges(t *testing.T) {
	listCtx := CreateContext[[]string](nil)
	builds := 0
	reader := func(int) Node {
		builds++
		return Text(strings.Join(UseContext(listCtx), ","))
	}
	page := func(v []string) Node { return listCtx.Provider(v, ComponentProps(reader, 0)) }
	tr := newTestTree(t, 100, 100, page([]string{"x"}), nil)
	tr.Mount(page([]string{"x"}))
	if builds != 2 {
		t.Errorf("%d builds, want 2", builds)
	}
	// nil interface values read back as the zero value.
	errCtx := CreateContext[error](nil)
	var got error = fmt.Errorf("unset")
	tr2 := newTestTree(t, 100, 100, errCtx.Provider(nil, Component(func() Node { got = UseContext(errCtx); return Text("e") })), nil)
	tr2.Frame()
	if got != nil {
		t.Errorf("UseContext of a nil error provider = %v", got)
	}
}

func TestContextSubscriptionsFollowMounts(t *testing.T) {
	reader := func(int) Node { return Text(UseContext(themeCtx)) }
	page := func(n int) Node {
		items := make([]Node, n)
		for i := range items {
			items[i] = WithKey(ComponentProps(reader, i), i)
		}
		return themeCtx.Provider("x", List(items))
	}
	tr := newTestTree(t, 300, 300, page(4), nil)
	prov := tr.root.children[0]
	if prov.prov == nil || len(prov.prov.subs) != 4 {
		t.Fatalf("provider subscriptions %+v, want 4", prov.prov)
	}
	tr.Mount(page(1))
	if len(prov.prov.subs) != 1 {
		t.Errorf("%d subscriptions after unmounting 3 readers, want 1", len(prov.prov.subs))
	}
	for i, s := range prov.prov.subs {
		if s.idx != i || !s.el.mounted {
			t.Errorf("subscription %d: idx %d, mounted %v", i, s.idx, s.el.mounted)
		}
	}
}

func TestProviderIdentity(t *testing.T) {
	other := CreateContext("other")
	pr := newProbe()
	tr := newTestTree(t, 100, 100, themeCtx.Provider("a", pr.item("x")), nil)
	pr.setters["x"](3)
	tr.Frame()
	// A provider of another context at the same position remounts.
	tr.Mount(other.Provider("a", pr.item("x")))
	tr.Frame()
	if got := texts(tr); got != "x=0" || pr.mounts["x"] != 2 {
		t.Errorf("text %q, %d mounts; want x=0 and 2", got, pr.mounts["x"])
	}
	if msg := panicMessage(func() { themeCtx.Provider("a", Class("p-4")) }); !strings.Contains(msg, "does not take Class") {
		t.Errorf("panic %q", msg)
	}
	// Key is accepted.
	n := themeCtx.Provider("a", Key("k"), Text("t"))
	if k, ok := n.Key(); !ok || k != "k" {
		t.Errorf("provider key %v %v", k, ok)
	}
	// Inspect names the component, not the provider.
	tr2 := newTestTree(t, 100, 100, Component(func() Node {
		return themeCtx.Provider("a", div(Key("in")))
	}), nil)
	tr2.Frame()
	if c := mustFind(t, tr2, "in").Component; !strings.Contains(c, "TestProviderIdentity") {
		t.Errorf("inspect component %q", c)
	}
}

func TestUseContextSwitchesContext(t *testing.T) {
	first := true
	var got string
	reader := func() Node {
		c := themeCtx
		if !first {
			c = CreateContext("fresh")
		}
		got = UseContext(c)
		return Text(got)
	}
	tr := newTestTree(t, 100, 100, themeCtx.Provider("p", Component(reader)), nil)
	if got != "p" {
		t.Fatalf("got %q", got)
	}
	first = false
	tr.Mount(themeCtx.Provider("p", Component(reader)))
	if got != "fresh" {
		t.Errorf("got %q, want the new context's default", got)
	}
	if subs := len(tr.root.children[0].prov.subs); subs != 0 {
		t.Errorf("%d subscriptions left on the old provider", subs)
	}
}
