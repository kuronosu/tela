package tela

import "github.com/kuronosu/tela/internal/core"

// Node is an immutable, cheap description of a piece of UI: an element
// (h.Div), a text (h.Text), a component or a fragment. Function components
// return one, and every build creates new ones; the persistent state lives
// elsewhere, keyed by position and key (ARCHITECTURE section 6).
//
// The zero Node renders nothing. As a child of an element it keeps the
// positions of its siblings, so h.If(cond, node) does not remount the
// siblings when cond changes. A Node is a [Component] whose Build returns
// the node itself.
type Node = core.Node

// Component is a unit of UI with its own state boundary: a value whose
// Build method returns what it renders. Build must use a value receiver.
//
//	type Card struct {
//	    Title    string
//	    OnOpen   tela.Callback[func()]
//	    Children tela.Component // children are an ordinary field
//	}
//
//	func (c Card) Build() tela.Node {
//	    open, setOpen := tela.UseState(false)
//	    return Div(Class("rounded-xl border p-4"),
//	        H2(OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text(c.Title)),
//	        If(open, c.Children))
//	}
//
//	Div(Card{Title: "Inbox", Children: Fragment(Text("a"), Span(Text("b")))})
//
// Every Component value given to an element as a child is a component of
// its own, with its own hook state: hooks called in Build (and in the
// function literals inside it) belong to it. Its identity among its
// siblings is its type and its key ([WithKey]), so at one position a
// value of the same type keeps the state of the previous build and a
// value of another type mounts anew. When its parent builds again, its
// build is skipped if the new value equals the previous one and no state,
// Value or context it read changed, as with [CP]. The comparison is ==,
// field by field, and never panics: a value that cannot be compared (a
// struct holding a slice, a map or a func, or a field of interface type
// holding one) counts as changed, so it always builds. A [Node] in a
// field compares by identity: a fresh h.Div(...) is a change, a node kept
// in a [UseMemo] is not. Pass [Callback] values instead of funcs to keep
// a component skippable.
//
// Calling Build directly (Card{}.Build() inside another component) runs it
// inside the caller, whose hook slots it would take: dev mode panics and
// says to pass Card{...} as a child instead. Where a [Node] is expected
// (the result of a function component, an h.Map item), give the value to
// h.Fragment, or key it with [WithKey]. h.If renders nothing for the zero
// value of its type, and the zero value of a struct renders, so pass it
// as a Component: If(loading, tela.Component(Spinner{})).
//
// In dev mode (see [App].Dev) a pointer (Div(&Card{})) panics, since a
// component is a value that Tela compares, and so does a [Prop] held by a
// field of type Component (Card{Children: Class("p-4")}): Go cannot tell
// it from a component at compile time, and given to an element it would
// apply to that element. A field that is meant to take props too is
// declared as [Child].
//
// [Node] and [Prop] implement Component as well; a function component
// becomes one with [C], [CP] or [Memo].
type Component = core.Comp

// Child is what an element takes: a [Node] (a child), a [Prop] (h.Class,
// h.Style, h.Key, h.OnClick...) or any other [Component], which becomes a
// component with its own state. Props and children may be mixed in any
// order. It has the method set of Component but is a type of its own: a
// field or parameter of type Child takes props too, one of type Component
// is for what renders. Passing a string or a number is a compile error;
// wrap text in h.Text. The compiler names the internal type behind the
// alias:
//
//	cannot use "hello" (constant of type string) as tela.Child value in
//	argument to Div: string does not implement core.Child (missing method
//	Build)
//
// means Div("hello") should be Div(Text("hello")).
type Child = core.Child

// Prop is a property of an element: classes, typed style, key, event
// handlers, an image source. Built by the functions of package h. A Prop
// is a [Child] (and, by its method set, a [Component], although it renders
// nothing: dev mode reports one held by a field of type Component).
type Prop = core.Prop

// WithKey returns the node of c with the key k, whatever its kind: a node
// that cannot take the h.Key prop (a component or a fragment), or a
// struct component, WithKey(Card{...}, id), which it also turns into a
// [Node] (for h.Map and other places that want one). k is a comparable
// value, such as an ID: a slice or a map does not compile, and a nil
// interface, or one holding a value that cannot be compared, panics. A
// nil c or the zero Node give the zero Node; a [Prop] panics.
//
//	Map(cards, func(c CardData) tela.Node {
//	    return tela.WithKey(Card{Title: c.Title}, c.ID)
//	})
func WithKey[K comparable](c Component, k K) Node { return core.WithKey(c, k) }
