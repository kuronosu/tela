package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// If returns c when cond holds and the zero value of its type otherwise.
// It takes a node or a prop, so it works for children and for props: the
// zero Node renders nothing but keeps the positions of the siblings after
// it (toggling cond does not remount them), and the zero Prop changes
// nothing.
//
//	Div(
//	    If(loading, Spinner()),
//	    If(selected, Class("ring-2 ring-indigo-500")),
//	    If(err != nil, P(Class("text-red-600"), Text(err.Error()))),
//	)
//
// Every argument of a call is evaluated: If(user != nil, Text(user.Name))
// dereferences a nil user. Use an ordinary if for that case.
//
// The zero value of a struct component is a component that renders, so
// give If a struct component as a tela.Component, whose zero value is nil
// (nothing): If(loading, tela.Component(Spinner{})). Dev mode panics on
// If(loading, Spinner{}).
func If[C tela.Child](cond bool, c C) C {
	core.CheckIf[C]()
	if cond {
		return c
	}
	var zero C
	return zero
}

// IfElse returns a when cond holds and b otherwise: two nodes, or two
// props.
//
//	Div(IfElse(open, Class("h-64"), Class("h-0")),
//	    IfElse(loading, Spinner(), Text(label)))
//
// Both arguments are evaluated, like those of [If].
func IfElse[C tela.Child](cond bool, a, b C) C {
	if cond {
		return a
	}
	return b
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
