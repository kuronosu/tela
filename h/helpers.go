package h

import (
	"fmt"
	"reflect"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// If returns c when cond holds and an empty child otherwise: one that
// renders nothing and applies nothing. It takes a node, a struct component
// or a prop, so it works for children and for props: the empty child keeps
// the positions of the siblings after it (toggling cond does not remount
// them).
//
//	Div(
//	    If(loading, Spinner{}),
//	    If(selected, Class("ring-2 ring-indigo-500")),
//	    If(err != nil, P(Class("text-red-600"), Text(err.Error()))),
//	)
//
// The result is a [tela.Child], so it goes wherever one does: the children
// of an element, Fragment, Presence, Portal and a Provider, and a field of
// type tela.Child or tela.Component. It is not a [tela.Node]: a function
// component that returns it wraps it, return Fragment(If(...)).
//
// Every argument of a call is evaluated: If(user != nil, Text(user.Name))
// dereferences a nil user. Use an ordinary if for that case.
func If(cond bool, c tela.Child) tela.Child {
	if cond {
		return c
	}
	return absent(c)
}

// IfElse returns a when cond holds and b otherwise: two nodes, two struct
// components or two props. A branch that is itself empty (the nil child,
// the zero Node, the result of an [If] that is off) renders nothing.
//
//	Div(IfElse(open, Class("h-64"), Class("h-0")),
//	    IfElse(loading, Spinner{}, Text(label)))
//
// Both arguments are evaluated, like those of [If].
func IfElse(cond bool, a, b tela.Child) tela.Child {
	if cond {
		return a
	}
	return b
}

// absent returns the empty child for a branch that is not taken: the zero
// Prop or StyleProp when the branch is one (it applies nothing and adds no
// child), and the zero Node, a hole that renders nothing, for every other
// kind.
func absent(c tela.Child) tela.Child {
	switch c.(type) {
	case tela.Prop:
		return tela.Prop{}
	case tela.StyleProp:
		return tela.StyleProp{}
	}
	return tela.Node{}
}

// Map returns the nodes of f applied to every item, as one list. Give each
// node a Key (the item's ID, not its index) so that reordering, inserting
// and removing keeps their state; dev mode (see tela.App.Dev) warns about a
// list of two or more nodes without keys, and about duplicate keys, once
// per call site of Map.
//
//	Map(todos, func(t Todo) tela.Node {
//	    return Div(Key(t.ID), Text(t.Title))
//	})
func Map[T any](items []T, f func(T) tela.Node) tela.Node { return core.Map(items, f) }

// MapIndex is [Map] with the item's index as the first argument of f: for
// numbering, alternating styles and the first or last item. The same
// rules apply to keys: use an ID, not the index, when the list changes.
//
//	MapIndex(steps, func(i int, s Step) tela.Node {
//	    return Div(Key(s.ID), Textf("%d. %s", i+1, s.Title))
//	})
func MapIndex[T any](items []T, f func(i int, item T) tela.Node) tela.Node {
	return core.MapIndex(items, f)
}

// MapKeyed returns the components of render applied to every item, as one
// keyed list: each is keyed by key(item), so reordering, inserting and
// removing items keeps the state of the component of every item that
// stays. It replaces Map(items, func(t T) tela.Node { return
// tela.WithKey(tela.CP(...), t.ID) }): render returns a node or a struct
// component, which MapKeyed keys (tela.WithKey), and K is any comparable
// type, so a slice or a map as a key does not compile. Dev mode warns
// about duplicate keys, once per call site, as it does for Map.
//
//	MapKeyed(todos, func(t Todo) int { return t.ID }, func(t Todo) tela.Component {
//	    return TodoRow{Todo: t}
//	})
//
// A render that wants a function component returns tela.CP(TodoRow, t).
// The key of a node that render keys itself, with Key, is replaced by
// key(item).
func MapKeyed[T any, K comparable](items []T, key func(T) K, render func(T) tela.Component) tela.Node {
	if key == nil {
		panic("h: MapKeyed(items, nil, ...): pass the function that returns an item's key, such as func(t Todo) int { return t.ID }")
	}
	if render == nil {
		panic("h: MapKeyed(items, key, nil): pass the function that returns an item's component, such as func(t Todo) tela.Component { return TodoRow{Todo: t} }")
	}
	nodes := make([]tela.Node, len(items))
	for i, it := range items {
		nodes[i] = tela.WithKey(render(it), key(it))
	}
	return core.List(nodes)
}

// Range returns render(0) to render(n-1) as one list keyed by index: n
// items with no data behind them, such as skeleton rows, a star rating or
// the cells of a fixed grid. The index is the key, so no missing-key
// warning is given (a static range does not reorder), and a node that
// render keys itself is keyed by its index instead. render returns a
// tela.Node or a tela.Component, so a struct component needs no Fragment.
// n is 0 or more: a negative n panics in dev mode and renders nothing
// otherwise.
//
//	Range(5, func(i int) tela.Node { return Span(Textf("%d", i)) })
//	Range(5, func(i int) tela.Component { return Star{Filled: i < rating} })
//
// For data that changes, use [MapKeyed] with the item's ID: an index as the
// key moves the state of a component to the wrong item when items are
// inserted or removed.
func Range[R ~func(int) tela.Node | ~func(int) tela.Component](n int, render R) tela.Node {
	if n < 0 && core.DevNow() {
		panic(fmt.Sprintf("h: Range(%d, ...): n is the number of items, 0 or more", n))
	}
	each := componentFunc(render)
	if each == nil {
		panic("h: Range(n, nil): pass render, a func(i int) tela.Node or func(i int) tela.Component that returns item i, such as func(i int) tela.Node { return Span(Textf(\"%d\", i)) }")
	}
	nodes := make([]tela.Node, max(n, 0))
	for i := range nodes {
		nodes[i] = tela.WithKey(each(i), i)
	}
	return core.List(nodes)
}

// componentFunc returns r, a func(int) tela.Node or a func(int)
// tela.Component (or a value of a named type of either), as a
// func(int) tela.Component; nil stays nil. The constraint of the callers
// guarantees one of the two shapes.
func componentFunc(r any) func(int) tela.Component {
	switch f := r.(type) {
	case func(int) tela.Node:
		if f == nil {
			return nil
		}
		return func(i int) tela.Component { return f(i) }
	case func(int) tela.Component:
		return f
	}
	v := reflect.ValueOf(r)
	if v.IsNil() {
		return nil
	}
	if v.Type().Out(0) == reflect.TypeFor[tela.Node]() {
		f := v.Convert(reflect.TypeFor[func(int) tela.Node]()).Interface().(func(int) tela.Node)
		return func(i int) tela.Component { return f(i) }
	}
	return v.Convert(reflect.TypeFor[func(int) tela.Component]()).Interface().(func(int) tela.Component)
}

// Nodes returns the nodes of ns as one fragment, for a slice built in a
// loop that is given as children (Go does not spread a []tela.Node into
// the children of Div(...)). The nodes keep their order and, like the
// children of Fragment, have no box of their own. A node slice that comes
// from data and changes is a list: key it with Map or MapKeyed.
//
//	var cells []tela.Node
//	for _, c := range columns {
//	    cells = append(cells, Div(Class("px-3"), Text(c)))
//	}
//	Div(Class("flex"), Nodes(cells))
func Nodes(ns []tela.Node) tela.Node { return core.NodeList(ns) }

// Scale returns a function that reads v and multiplies it by k, for the
// deferred style props that take a func() float32 (tela.WidthFn,
// tela.HeightFn, tela.OpacityFn): the Value is tracked where the prop is
// evaluated, not when the component builds. It replaces the closure
// func() float32 { return 36 * v.Get() }; tela.MapValue is the general
// form.
//
//	open := tela.UseSpring(target) // a *tela.Value[float32] from 0 to 1
//	Div(Class("overflow-hidden"), Style(tela.HeightFn(Scale(open, 36))))
func Scale(v *tela.Value[float32], k float32) func() float32 {
	if v == nil {
		panic("h: Scale(nil, k): pass the *tela.Value[float32] returned by tela.UseValue, tela.UseSpring or tela.UseTween")
	}
	return func() float32 { return k * v.Get() }
}

// Fragment groups children without a box of their own: they become
// children of the fragment's parent. It takes only Key as a prop.
//
// It is how a component returns several siblings, how a wrapper puts the
// children it receives next to its own (Go does not allow
// Div(Class("..."), H2(...), children...), but Fragment(children...) is
// one child), and how several children go in one field of type
// tela.Component. The children must be nodes or components (or Key):
//
//	// Card has no hooks, so it is a plain function that wraps its children.
//	func Card(title string, children ...tela.Child) tela.Node {
//	    return Div(Class("rounded-xl border p-4"),
//	        H2(Class("font-semibold"), Text(title)),
//	        Fragment(children...))
//	}
//
//	Panel{Title: "Inbox", Children: Fragment(Text("a"), Span(Text("b")))}
//
// Fragment(c) is also a node of one component, for a function that must
// return a tela.Node: return Fragment(Panel{...}).
func Fragment(children ...tela.Child) tela.Node { return core.Fragment(children...) }

// TextFn returns a text node whose content is read by fn in a later phase,
// not when the component builds: a change of a tela.Value that fn read
// lays out only the paragraph that holds the text, and no component
// builds. It is Text for what changes every frame, such as a clock or a
// time label:
//
//	pos := tela.UseValue(0.0) // seconds, set from a ticker
//	Span(Class("tabular-nums"), TextFn(func() string { return clock(pos.Get()) }))
//
// It lays out like Text, inside paragraphs next to other text and inline
// elements. fn runs on the UI thread, when the paragraph is assembled (and
// the Values it read changed): it should only compute the string. A
// closure that captures the component's state is replaced by the new one
// whenever the component builds.
func TextFn(fn func() string) tela.Node { return core.TextFn(fn) }
