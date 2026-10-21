package h_test

import (
	"fmt"
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
	// The zero Node keeps the positions of the siblings after it.
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
	var n tela.Node = If(false, Div())
	if !n.IsZero() {
		t.Errorf("If(false, node) is not the zero Node")
	}
	// Existing call sites that name the child type keep compiling.
	var c tela.Child = If[tela.Child](false, Div())
	if c != nil {
		t.Errorf("If[tela.Child](false, ...) = %v", c)
	}
}

// spinner is a struct component whose zero value renders.
type spinner struct{ Size int }

func (s spinner) Build() tela.Node { return Div(Class("animate-spin"), Textf("spin %d", s.Size)) }

// TestIfWithStructComponents: the zero value of a struct component
// renders, so If panics on one in dev mode (tests run in dev mode) and
// works through tela.Component, whose zero value is nil.
func TestIfWithStructComponents(t *testing.T) {
	func() {
		defer func() {
			r := recover()
			if msg := fmt.Sprint(r); r == nil || !strings.Contains(msg, "If(cond, spinner{...}) returns spinner{} when cond is false") ||
				!strings.Contains(msg, "If(cond, tela.Component(spinner{...}))") {
				t.Errorf("If(false, spinner{}): panic %v", r)
			}
		}()
		If(false, spinner{})
	}()
	if c := If(false, tela.Component(spinner{Size: 2})); c != nil {
		t.Errorf("If(false, tela.Component(...)) = %v, want nil", c)
	}
	var kids tela.Component = Text("kids")
	if c := If(true, kids); c != kids {
		t.Errorf("If(true, component) = %v", c)
	}
	screen := func(show bool) tela.Node {
		return Div(If(show, tela.Component(spinner{Size: 3})), Div(Key("b"), Class("h-4")))
	}
	if got := findBox(t, screen(true), 200, 100, "b").Rect.Y; got == 0 {
		t.Error("b at y 0 with the spinner")
	}
	if got := findBox(t, screen(false), 200, 100, "b").Rect.Y; got != 0 {
		t.Errorf("b at y %g without the spinner, want 0", got)
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
	var z tela.Node = IfElse(true, tela.Node{}, Div())
	if !z.IsZero() {
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
