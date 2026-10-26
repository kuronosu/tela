// Package keys is a fixture of the key rule.
package keys

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

type Item struct {
	ID   int
	Name string
}

const first = "a"

func row(it Item) tela.Node { return Div(Text(it.Name)) }

func keyedRow(it Item) tela.Node { return Div(Key(it.ID), Text(it.Name)) }

func comp() tela.Node { return Div() }

type Row struct{ Item Item }

func (r Row) Build() tela.Node { return Div(Text(r.Item.Name)) }

func Duplicates(id int) tela.Node {
	return Div(
		Div(Key("a")),
		Div(Key("b")),
		Div(Key("a")),   // want key `duplicate key "a" among the children of this element`
		Div(Key(first)), // want key `duplicate key "a"`
		Div(Key(1)),
		Div(Key("1")),
		Div(Key(1)), // want key `duplicate key 1`
		Div(Key(int64(1))),
		Div(Key(id)),
		Div(Key(id)),
		tela.C(comp, Key("c")),
		tela.C(comp, Key("c")), // want key `duplicate key "c"`
		tela.WithKey(Row{}, "w"),
		tela.WithKey(Row{}, "w"), // want key `duplicate key "w"`
		Fragment(Key("f"), Div(Key("x")), Div(Key("x"))), // want key `duplicate key "x"`
		Div(Div(Key("a"))),
	)
}

func Maps(items []Item) tela.Node {
	return Div(
		Map(items, func(it Item) tela.Node { // want key `the items of this list have no key`
			return Div(Text(it.Name))
		}),
		Map(items, func(it Item) tela.Node {
			return Div(Key(it.ID), Text(it.Name))
		}),
		MapIndex(items, func(i int, it Item) tela.Node { // want key `the items of this list have no key`
			return Div(Class("p-2"), Textf("%d", i))
		}),
		Map(items, func(it Item) tela.Node { // want key `the items of this list have no key`
			if it.ID == 0 {
				return Div(Text("zero"))
			}
			return Span(Class("p-2"), Text(it.Name))
		}),
		Map(items, func(it Item) tela.Node { // returns a component: it may be keyed
			return tela.CP(row, it, Key(it.ID))
		}),
		Map(items, func(it Item) tela.Node {
			return row(it) // a helper's result: not known
		}),
		Map(items, func(it Item) tela.Node {
			if it.ID == 0 {
				return Div(Key(0))
			}
			return Div(Text(it.Name))
		}),
		Map(items, func(it Item) tela.Node {
			return Div(Props(it)) // a computed prop may be a Key
		}),
		Map(items, func(it Item) tela.Node {
			return Div(If(it.ID > 0, Key(it.ID)))
		}),
		Map(items, row), // want key `the items of this list have no key`
		Map(items, keyedRow),
		Map([]Item{{ID: 1}}, func(it Item) tela.Node { return Div() }), // a single item
		MapKeyed(items, func(it Item) int { return it.ID }, func(it Item) tela.Component { return Row{it} }),
		Range(3, func(i int) tela.Node { return Div() }),
	)
}

func Props(it Item) tela.Prop { return Key(it.ID) }
