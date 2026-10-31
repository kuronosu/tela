package tela

import (
	"fmt"

	"github.com/kuronosu/tela/internal/core"
)

// Node is what renders: a value whose Build method returns what it
// renders (ADR-205). The elements of package h (h.Div, h.Text,
// h.Fragment...) and the components made with [C], [CP] and [Memo] are
// Nodes, and so is every struct component, a struct with a Build method:
//
//	type Card struct {
//	    Title    string
//	    OnOpen   tela.Callback[func()]
//	    Children tela.Node // children are an ordinary field
//	}
//
//	func (c Card) Build() tela.Node {
//	    open, setOpen := tela.UseState(false)
//	    return Div(Class("rounded-xl border p-4"),
//	        H2(OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text(c.Title)),
//	        If(open, c.Children))
//	}
//
//	func Inbox() tela.Node { return Card{Title: "Inbox", Children: Text("a")} }
//
// A Node is an immutable, cheap description of UI: function components
// return one, Build returns one, and every build creates new ones; the
// persistent state lives elsewhere, keyed by position and key
// (ARCHITECTURE section 6). A struct component can be returned, kept in
// a []tela.Node, given to h.Map or to a field of type Node as it is.
//
// nil renders nothing. As a child of an element it keeps the positions of
// its siblings, so h.If(cond, node) does not remount the siblings when
// cond changes; h.If and h.IfElse return nil for a branch that is not
// taken, and n == nil tells it.
//
// Every struct component is a component of its own, with its own hook
// state: hooks called in Build (and in the function literals inside it)
// belong to it. Build must use a value receiver. Its identity among its
// siblings is its type and its key ([WithKey]), so at one position a value
// of the same type keeps the state of the previous build and a value of
// another type mounts anew. When its parent builds again, its build is
// skipped if the new value equals the previous one and no state, Value or
// context it read changed, as with [CP]. The comparison is ==, field by
// field, and never panics: a value that cannot be compared (a struct
// holding a slice, a map or a func, or a field of interface type holding
// one) counts as changed, so it always builds. An element (h.Div(...)) in
// a field compares by identity: a fresh one is a change, a node kept in a
// [UseMemo] is not. Pass [Callback] values instead of funcs to keep a
// component skippable.
//
// Calling Build directly (Card{}.Build() inside another component) runs it
// inside the caller, whose hook slots it would take: dev mode panics and
// says to return or pass Card{...} instead. A Build that returns its own
// receiver (return c) would build itself forever: dev mode panics.
//
// In dev mode (see [App].Dev) a pointer (Div(&Card{})) panics, since a
// component is a value that Tela compares, and so does a [Prop] held by a
// field of type Node (Card{Children: Class("p-4")}): a Prop has a Build
// method, so Go cannot tell it from a node at compile time, and given to
// an element it would apply to that element. A field that is meant to
// take props too is declared as [Child]. For the same reason a function
// that returns h.Class(...) where a Node is expected compiles, and panics
// when the node is used.
//
// Two Nodes compare with == only when their dynamic types are
// comparable: n1 == n2 panics for two values of a struct component that
// holds a slice, a map or a func (n == nil never panics).
type Node = core.Node

// Component is [Node]: the name for a field or a parameter that holds
// what renders, such as the Children of a struct component, or the root
// of [App], [Preview] and [Window]. Both names are the same type.
type Component = Node

// Child is what an element takes: a [Node] (a child, an element or a
// struct component, which becomes a component with its own state) or a
// [Prop] (h.Class, h.Style, h.Key, h.OnClick...). Props and children may
// be mixed in any order. It has the method set of Node but is a type of
// its own: a field or parameter of type Child takes props too, one of
// type Node (or Component) is for what renders. A Child is assignable to
// a Node, so a function that returns a Node may return h.If(...). Passing
// a string or a number is a compile error; wrap text in h.Text. The
// compiler names the internal type behind the alias:
//
//	cannot use "hello" (constant of type string) as tela.Child value in
//	argument to Div: string does not implement core.Child (missing method
//	Build)
//
// means Div("hello") should be Div(Text("hello")).
type Child = core.Child

// Prop is a property of an element: classes, typed style, key, event
// handlers, an image source. Built by the functions of package h. A Prop
// is a [Child] (and, by its method set, a [Node], although it renders
// nothing: dev mode reports one held by a field of type Node, and one
// used where a node is expected panics).
type Prop = core.Prop

// WithKey returns the node of c with the key k, whatever its kind: a node
// that cannot take the h.Key prop (a component or a fragment), or a
// struct component, WithKey(Card{...}, id). k is a comparable value, such
// as an ID: a slice or a map does not compile, and a nil interface, or one
// holding a value that cannot be compared, panics. A nil c gives nil; a
// [Prop] panics.
//
//	Map(cards, func(c CardData) tela.Node {
//	    return tela.WithKey(Card{Title: c.Title}, c.ID)
//	})
func WithKey[K comparable](c Node, k K) Node { return core.WithKey(c, k) }

// MergeProps returns one [Prop] that applies props in order, for a place
// that takes a single one: kit's Props field of type [Child], or a hook's
// result next to your own props. Event handlers accumulate (every handler
// of an event runs, in order); the other props are set by the last one that
// sets them, as when they are given one after the other to an element. Nil
// and the zero Prop are skipped.
//
// It takes only props, and style props such as tela.Width. A [Node] (an
// element or a struct component) is not a prop and cannot be merged into
// one: dev mode panics, naming the argument, and otherwise it is ignored.
//
//	kit.Button{Children: Text("Save"), Props: tela.MergeProps(
//	    TestID("save"), OnMouseEnter(prefetch), AriaLabel("Save the draft"))}
func MergeProps(props ...Child) Prop {
	ps := make([]core.Prop, 0, len(props))
	for i, c := range props {
		if c == nil {
			continue // a hole
		}

		if p, ok := core.PropOf(c); ok {
			ps = append(ps, p)
			continue
		}
		notAProp(i, c)
	}
	return core.MergeProps(ps...)
}

// notAProp is MergeProps' dev-mode report of an argument that is not a prop.
func notAProp(i int, c Child) {
	if DevMode() {
		panic(fmt.Sprintf("tela: MergeProps argument %d is a %T, not a prop; MergeProps takes props (h.Class, h.OnClick, h.AriaLabel...). "+
			"To add a child or a component, give it to the element or to the Children field instead", i, c))
	}
}
