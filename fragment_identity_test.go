package tela_test

import (
	"bytes"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The `fragment` rule of tela lint (ADR-205) removes Fragment(x) around a
// single node. These tests check what it relies on: the node keeps its
// state and the screen is the same, with and without the wrapper, in every
// place the rule fixes; and the one place where they differ (a place that
// takes a group at other times) is the one the rule leaves alone.

// tally is a struct component with state: a button that counts its clicks.
type tally struct{ Label string }

func (c tally) Build() tela.Node {
	n, setN := tela.UseState(0)
	return Button(Textf("%s=%d", c.Label, n), OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
}

// wrapper is how a test spells the node at the place under test: the
// wrapped form before the fix, the bare one after it.
type wrapper func(tela.Node) tela.Node

var (
	wrapped wrapper = func(n tela.Node) tela.Node { return Fragment(n) } //tela:ignore fragment -- the test compares the two forms
	bare    wrapper = func(n tela.Node) tela.Node { return n }
)

// fragmentPlaces are the places the rule fixes, each with the tally "a"
// in it. above is a node that comes and goes before it.
var fragmentPlaces = map[string]func(w wrapper, above tela.Node) tela.Node{
	"element child": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, w(tally{"a"}))
	},
	"field of a component": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, fieldHost{Content: w(tally{"a"})})
	},
	"result of Build": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, buildHost{wrap: w})
	},
	"callback of Range": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, Range(1, func(int) tela.Node { return w(tally{"a"}) }))
	},
	"IfElse": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, IfElse(true, w(tally{"a"}), nil))
	},
	"If": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, If(true, w(tally{"a"})))
	},
	"nested": func(w wrapper, above tela.Node) tela.Node {
		return Div(above, w(w(tally{"a"})))
	},
}

type fieldHost struct{ Content tela.Node }

func (h fieldHost) Build() tela.Node { return Div(h.Content) }

type buildHost struct{ wrap wrapper }

func (h buildHost) Build() tela.Node { return h.wrap(tally{"a"}) }

// buildHost holds a func, so it is not comparable: the build is not
// skipped, which is what this test wants.

func TestFragmentRemovalKeepsStateAndPixels(t *testing.T) {
	shots := map[string][]byte{}
	for name, place := range fragmentPlaces {
		for form, w := range map[string]wrapper{"wrapped": wrapped, "bare": bare} {
			t.Run(name+"/"+form, func(t *testing.T) {
				var flip func()
				app := telatest.MountFunc(t, func() tela.Node {
					on, setOn := tela.UseState(false)
					flip = func() { setOn.Update(func(b bool) bool { return !b }) }
					return place(w, If(on, P(Text("banner"))))
				}, telatest.Size(200, 120))
				app.Click(telatest.ByRole("button", "a=0"))
				app.ExpectText("a=1")
				flip() // a sibling above appears
				app.ExpectText("banner")
				app.ExpectText("a=1")
				app.Click(telatest.ByRole("button", "a=1"))
				app.ExpectText("a=2")
				flip() // and goes
				app.ExpectNoText("banner")
				app.ExpectText("a=2")
				var buf bytes.Buffer
				img := app.Screenshot()
				buf.Write(img.Pix)
				if prev, ok := shots[name]; ok && !bytes.Equal(prev, buf.Bytes()) {
					t.Errorf("the screen with the wrapper differs from the one without it")
				}
				shots[name] = buf.Bytes()
			})
		}
	}
}

// A keyed list keeps the state of each item with its key when the list is
// reordered, whether the item is wrapped or not.
func TestFragmentRemovalKeepsKeyedItemState(t *testing.T) {
	for form, w := range map[string]wrapper{"wrapped": wrapped, "bare": bare} {
		t.Run(form, func(t *testing.T) {
			var reverse func()
			app := telatest.MountFunc(t, func() tela.Node {
				rev, setRev := tela.UseState(false)
				reverse = func() { setRev.Update(func(b bool) bool { return !b }) }
				items := []string{"a", "b", "c"}
				if rev {
					items = []string{"c", "b", "a"}
				}
				return Div(MapKeyed(items, func(s string) string { return s }, func(s string) tela.Node {
					return w(tally{s})
				}))
			}, telatest.Size(200, 160))
			app.Click(telatest.ByRole("button", "a=0"))
			app.Click(telatest.ByRole("button", "a=1"))
			app.Click(telatest.ByRole("button", "c=0"))
			reverse()
			app.ExpectText("a=2")
			app.ExpectText("c=1")
			app.ExpectText("b=0")
		})
	}
}

// spacer is a component that renders nothing.
type spacer struct{}

func (spacer) Build() tela.Node { return Div(Class("h-1")) }

// groupOrOne is a place that takes a group at other times.
type groupOrOne struct {
	Group bool
	One   wrapper
}

func (g groupOrOne) Build() tela.Node {
	if g.Group {
		return Fragment(tally{"a"}, spacer{})
	}
	return g.One(tally{"a"})
}

// A node that moves between Fragment(x) and Fragment(x, y) keeps its
// state, and one that moves between a bare x and Fragment(x, y) does not.
// That is why the rule keeps the single Fragment of a place with a group
// in it.
func TestFragmentVariantOfGroupKeepsIdentityOnlyWrapped(t *testing.T) {
	for form, tc := range map[string]struct {
		w    wrapper
		keep bool
	}{
		"wrapped": {wrapped, true},
		"bare":    {bare, false},
	} {
		t.Run(form, func(t *testing.T) {
			var flip func()
			app := telatest.MountFunc(t, func() tela.Node {
				group, setGroup := tela.UseState(false)
				flip = func() { setGroup.Update(func(b bool) bool { return !b }) }
				return Div(groupOrOne{Group: group, One: tc.w})
			}, telatest.Size(200, 100))
			app.Click(telatest.ByRole("button", "a=0"))
			app.ExpectText("a=1")
			flip()
			want := "a=0"
			if tc.keep {
				want = "a=1"
			}
			app.ExpectText(want)
		})
	}
}
