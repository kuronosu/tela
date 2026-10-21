package h_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// findBox renders root and returns the box with the key, or a zero box.
func findBox(t *testing.T, root tela.Node, w, h float32, key string) *box {
	t.Helper()
	b := inspect(t, root, w, h)
	if n := b.find(key); n != nil {
		return n
	}
	return &box{}
}

func TestIfWithNodes(t *testing.T) {
	// The empty child keeps the positions of the siblings after it.
	screen := func(show bool) tela.Node {
		return Div(Class("w-[100px]"),
			If(show, Div(Key("a"), Class("h-4"))),
			Div(Key("b"), Class("h-4")))
	}
	if got := findBox(t, screen(true), 200, 100, "b").Rect.Y; got != 16 {
		t.Errorf("b at y %g with the first child, want 16", got)
	}
	if got := findBox(t, screen(false), 200, 100, "b").Rect.Y; got != 0 {
		t.Errorf("b at y %g without it, want 0", got)
	}
	// The absent branch of a node is the zero Node; of a struct component
	// too (it renders nothing), of a prop the zero Prop.
	for name, c := range map[string]tela.Child{
		"node":      If(false, Div()),
		"component": If(false, spinner{}),
		"nil":       If(false, nil),
	} {
		if n, ok := c.(tela.Node); !ok || !n.IsZero() {
			t.Errorf("If(false, %s) = %#v, want the zero Node", name, c)
		}
	}
	if p, ok := If(false, Class("x")).(tela.Prop); !ok || !reflect.ValueOf(p).IsZero() {
		t.Errorf("If(false, prop) = %#v, want the zero Prop", If(false, Class("x")))
	}
	if c := If(true, Div()); c == nil {
		t.Error("If(true, node) is nil")
	}
}

// dump returns the JSON of a box tree, to search it for text.
func dump(b box) string {
	js, _ := json.Marshal(b)
	return string(js)
}

// spinner is a struct component: its zero value renders.
type spinner struct{ Size int }

func (s spinner) Build() tela.Node { return Div(Class("animate-spin"), Textf("spin %d", s.Size)) }

// TestIfWithStructComponents: a struct component goes straight to If, and
// when the condition is false nothing renders (ADR-094).
func TestIfWithStructComponents(t *testing.T) {
	var kids tela.Component = Text("kids")
	if c := If(true, kids); c != kids {
		t.Errorf("If(true, component) = %v", c)
	}
	screen := func(show bool) tela.Node {
		return Div(If(show, spinner{Size: 3}), Div(Key("b"), Class("h-4")))
	}
	if got := findBox(t, screen(true), 200, 100, "b").Rect.Y; got == 0 {
		t.Error("b at y 0 with the spinner")
	}
	if got := findBox(t, screen(false), 200, 100, "b").Rect.Y; got != 0 {
		t.Errorf("b at y %g without the spinner, want 0", got)
	}
	// IfElse with two struct components.
	pick := func(a bool) tela.Node {
		return Div(Key("p"), IfElse(a, spinner{Size: 1}, other{}))
	}
	if got := dump(inspect(t, pick(true), 200, 100)); !strings.Contains(got, "spin 1") {
		t.Errorf("IfElse(true) text %q", got)
	}
	if got := dump(inspect(t, pick(false), 200, 100)); !strings.Contains(got, "other") {
		t.Errorf("IfElse(false) text %q", got)
	}
}

type other struct{}

func (other) Build() tela.Node { return Text("other") }

// TestIfStructComponentKeepsState: toggling the condition mounts and
// unmounts the component, and the sibling after it keeps its state.
func TestIfStructComponentKeepsState(t *testing.T) {
	var setShow tela.Setter[bool]
	counter := func() tela.Node {
		n := tela.UseRef(0)
		n.Current++
		return Div(Key("counter"), Textf("%d", n.Current))
	}
	root := tela.C(func() tela.Node {
		show, set := tela.UseState(false)
		setShow = set
		return Div(If(show, spinner{}), tela.C(counter))
	})
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(root)
	tr.Frame()
	setShow.Set(true)
	tr.Frame()
	setShow.Set(false)
	tr.Frame()
	js, _ := json.Marshal(tr.Inspect())
	if !strings.Contains(string(js), `"counter"`) {
		t.Errorf("the counter is gone:\n%s", js)
	}
}

// TestIfReturnsAChild: the result is a Child, so a function component
// wraps it with Fragment, and it goes into a field of type tela.Child or
// tela.Component.
func TestIfReturnsAChild(t *testing.T) {
	fn := func() tela.Node { return Fragment(If(true, spinner{Size: 4})) }
	if got := dump(inspect(t, Div(Key("r"), tela.C(fn)), 200, 100)); !strings.Contains(got, "spin 4") {
		t.Errorf("Fragment(If(...)) text %q", got)
	}
	var comp tela.Component = If(false, spinner{})
	if got := dump(inspect(t, Div(comp, Div(Key("after"))), 200, 100)); strings.Contains(got, "spin") {
		t.Error("a Component field holding an absent If renders")
	}
}

func TestIfWithProps(t *testing.T) {
	screen := func(selected bool) tela.Node {
		return Div(Class("p-2"),
			Div(Key("card"), Class("h-4 w-4 bg-sky-500"),
				If(selected, Class("bg-red-500")),
				If(selected, Key("selected")),
				If(selected, Disabled(true))))
	}
	if got := findBox(t, screen(true), 100, 100, "selected"); got == nil || got.Style.Background != "#fb2c36" {
		t.Errorf("selected card: %+v", got)
	}
	off := inspect(t, screen(false), 100, 100)
	card := off.find("card")
	if card == nil || card.Style.Classes != "h-4 w-4 bg-sky-500" || card.Style.Background != "#00a6f4" {
		t.Errorf("unselected card: %+v", card)
	}
	// An absent prop adds no child, so no position changes.
	if len(card.Children) != 0 {
		t.Errorf("an absent prop added children: %+v", card.Children)
	}
}

// TestIfAbsentInEveryPosition: the false branch of If and IfElse goes
// wherever a Child can: the children of an element, a Fragment, a
// Provider, a Presence and a Portal, and the prop positions of each.
func TestIfAbsentInEveryPosition(t *testing.T) {
	ctx := tela.CreateContext(1)
	for name, branches := range map[string][]tela.Child{
		"node":      {If(false, Div(Key("x")))},
		"component": {If(false, spinner{})},
		"prop":      {If(false, Class("x")), If(false, tela.Width(9)), If(false, Key("k"))},
		"ifelse":    {IfElse(false, spinner{}, tela.Node{}), IfElse(false, Class("a"), If(false, Class("b")))},
		"nested":    {If(false, If(true, spinner{})), If(true, If(false, spinner{}))},
	} {
		for pos, build := range map[string]func(c ...tela.Child) tela.Node{
			"element":  func(c ...tela.Child) tela.Node { return Div(c...) },
			"fragment": func(c ...tela.Child) tela.Node { return Div(Fragment(c...)) },
			"provider": func(c ...tela.Child) tela.Node { return Div(ctx.Provider(2, c...)) },
			"presence": func(c ...tela.Child) tela.Node { return Div(Presence(c...)) },
			"portal":   func(c ...tela.Child) tela.Node { return Div(Portal(c...)) },
		} {
			root := build(append(append([]tela.Child{}, branches...), Div(Key("after"), Class("h-4")))...)
			b := inspect(t, Div(Class("w-[100px]"), root), 200, 100)
			if n := b.find("after"); n == nil {
				t.Errorf("%s in %s: the sibling is missing", name, pos)
			}
			if b.find("x") != nil || strings.Contains(dump(b), "spin") {
				t.Errorf("%s in %s: an absent branch rendered", name, pos)
			}
		}
	}
	// A present spinner appears in every container.
	for pos, build := range map[string]func(c ...tela.Child) tela.Node{
		"element":  func(c ...tela.Child) tela.Node { return Div(c...) },
		"fragment": func(c ...tela.Child) tela.Node { return Div(Fragment(c...)) },
		"provider": func(c ...tela.Child) tela.Node { return Div(ctx.Provider(2, c...)) },
		"presence": func(c ...tela.Child) tela.Node { return Div(Presence(c...)) },
		"portal":   func(c ...tela.Child) tela.Node { return Div(Portal(c...)) },
	} {
		b := inspect(t, build(If(true, spinner{Size: 7})), 200, 100)
		if !strings.Contains(dump(b), "spin 7") {
			t.Errorf("a present struct component in %s did not render", pos)
		}
	}
}

func TestIfWithPropsInFragmentsAndProviders(t *testing.T) {
	// The zero Prop is accepted where only Key is.
	ctx := tela.CreateContext(1)
	root := Div(
		Fragment(If(false, Class("x")), Key("f"), Div(Key("in-fragment"), Class("h-4"))),
		ctx.Provider(2, If(false, Class("x")), Div(Key("in-provider"), Class("h-4"))),
	)
	b := inspect(t, root, 100, 100)
	if len(b.Children) != 1 || len(b.Children[0].Children) != 2 || b.find("in-provider") == nil {
		t.Errorf("children missing:\n%+v", b)
	}
}

func TestIfElse(t *testing.T) {
	screen := func(open bool) tela.Node {
		return Div(
			Div(Key("panel"), IfElse(open, Class("h-8"), Class("h-2"))),
			IfElse(open, Div(Key("open"), Class("h-4")), Div(Key("closed"), Class("h-6"))))
	}
	on, off := inspect(t, screen(true), 100, 100), inspect(t, screen(false), 100, 100)
	if got := on.find("panel").Rect.Height; got != 32 || on.find("open") == nil || on.find("closed") != nil {
		t.Errorf("open: panel %g\n%+v", got, on)
	}
	if got := off.find("panel").Rect.Height; got != 8 || off.find("closed") == nil || off.find("open") != nil {
		t.Errorf("closed: panel %g\n%+v", got, off)
	}
	// Both branches are zero when they are: a hole.
	if z := IfElse(true, tela.Node{}, Div()); z != (tela.Node{}) {
		t.Error("IfElse picked the wrong branch")
	}
}

func TestMapIndex(t *testing.T) {
	items := []string{"x", "y", "z"}
	root := Div(Class("w-[100px]"), MapIndex(items, func(i int, s string) tela.Node {
		return Div(Key(s), Class("h-4"), Textf("%d:%s", i, s))
	}))
	b := inspect(t, root, 200, 100)
	for i, s := range items {
		n := b.find(s)
		if n == nil || n.Rect.Y != float32(16*i) {
			t.Fatalf("item %s: %+v", s, n)
		}
		if want := fmt.Sprintf("%d:%s", i, s); n.Text != want {
			t.Errorf("item %s text %q, want %q", s, n.Text, want)
		}
	}
}

func TestMapIndexKeepsStateByKey(t *testing.T) {
	// Reordering keyed items keeps each one's state, whatever its index:
	// the component of each item counts its builds.
	type item struct{ id string }
	builds := map[string]int{}
	row := func(it item) tela.Node {
		n := tela.UseRef(0)
		n.Current++
		builds[it.id] = n.Current
		return Div(Class("h-4"))
	}
	var set tela.Setter[[]item]
	var list []item
	comp := func() tela.Node {
		l, s := tela.UseState([]item{{"a"}, {"b"}, {"c"}})
		set, list = s, l
		return Div(MapIndex(l, func(i int, it item) tela.Node {
			return tela.WithKey(tela.C(func() tela.Node { return row(it) }), it.id)
		}))
	}
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(tela.C(comp))
	tr.Frame()
	set.Set([]item{{"c"}, {"a"}, {"b"}})
	tr.Frame()
	for id, n := range builds {
		if n != 2 {
			t.Errorf("item %s was built %d times, want 2 (state kept across the reorder)", id, n)
		}
	}
	if len(builds) != 3 || list[0].id != "c" {
		t.Errorf("builds %v, list %v", builds, list)
	}
}

func TestTextFnIsText(t *testing.T) {
	root := P(Key("para"), Class("w-[200px]"), Text("a "), Strong(TextFn(func() string { return "b" })), Text(" c"))
	if got := findBox(t, root, 300, 100, "para").Text; got != "a b c" {
		t.Errorf("paragraph %q", got)
	}
}
