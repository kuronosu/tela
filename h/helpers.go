package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// If returns n when cond holds and the zero Node otherwise, which renders
// nothing but keeps the positions of the siblings after it, so toggling
// cond does not remount them.
//
// Both branches of a call are evaluated: If(user != nil, Text(user.Name))
// dereferences a nil user. Use an ordinary if for that case.
func If(cond bool, n tela.Node) tela.Node { return core.If(cond, n) }

// Map returns the nodes of f applied to every item, as one list. Give each
// node a Key (the item's ID, not its index) so that reordering, inserting
// and removing keeps their state; in dev mode (TELA_DEV=1) a list of two or
// more nodes without keys is reported.
//
//	Map(todos, func(t Todo) tela.Node {
//	    return Div(Key(t.ID), Text(t.Title))
//	})
func Map[T any](items []T, f func(T) tela.Node) tela.Node { return core.Map(items, f) }

// Fragment groups children without a box of their own: they become
// children of the fragment's parent. It takes only Key as a prop.
func Fragment(children ...tela.Child) tela.Node { return core.Fragment(children...) }
