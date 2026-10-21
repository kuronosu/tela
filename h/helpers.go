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
func If[C tela.Child](cond bool, c C) C {
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

// Fragment groups children without a box of their own: they become
// children of the fragment's parent. It takes only Key as a prop.
//
// It is how a component returns several siblings, and how a wrapper puts
// the children it receives next to its own: Go does not allow
// Div(Class("..."), H2(...), children...), but Fragment(children...) is
// one child. The children must be nodes (or Key):
//
//	// Card has no hooks, so it is a plain function that wraps its children.
//	func Card(title string, children ...tela.Child) tela.Node {
//	    return Div(Class("rounded-xl border p-4"),
//	        H2(Class("font-semibold"), Text(title)),
//	        Fragment(children...))
//	}
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
