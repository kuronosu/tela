// Package propnode is a fixture of the prop-node rule.
package propnode

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

type Card struct{ Children tela.Component }

func (c Card) Build() tela.Node { return Div(c.Children) }

type Slot struct{ Child tela.Child }

func Result() tela.Node { return Class("p-2") } // want prop-node `h.Class(...) is a Prop, not a node`

func StyleResult() tela.Node { return tela.Width(1) } // want prop-node `tela.Width(...) is a Prop`

func Handler() tela.Node { return OnClick(func() {}) } // want prop-node `h.OnClick(...) is a Prop`

func Keyed() tela.Node { return Key(1) } // want prop-node `h.Key(...) is a Prop`

func Variable() tela.Node {
	p := Class("p-2")
	return p // want prop-node `this value is a Prop`
}

func Conditional(on bool) tela.Node {
	return If(on, Class("p-2")) // want prop-node `h.If(...) of props is a Prop`
}

func Either(on bool) tela.Node {
	return IfElse(on, Class("p-1"), Class("p-3")) // want prop-node `h.IfElse(...) of props is a Prop`
}

func Callback(xs []int) tela.Node {
	return Map(xs, func(int) tela.Node {
		return Class("p-2") // want prop-node `h.Class(...) is a Prop`
	})
}

func Slice() []tela.Node {
	return []tela.Node{Class("p-2"), Div()} // want prop-node `h.Class(...) is a Prop`
}

func Field() tela.Node {
	return Card{Children: Class("p-2")} // want prop-node `h.Class(...) is a Prop`
}

func Assigned() tela.Node {
	var n tela.Node
	n = Class("p-2") // want prop-node `h.Class(...) is a Prop`
	return n
}

func Argument() tela.Node {
	return tela.WithKey(Class("p-2"), 1) // want prop-node `h.Class(...) is a Prop`
}

func Appended(xs []tela.Node) []tela.Node {
	return append(xs, Class("p-2")) // want prop-node `h.Class(...) is a Prop`
}

// Fine.
func OnElement() tela.Node { return Div(Class("p-2"), Key(1)) }

func ChildResult() tela.Child { return Class("p-2") }

func ChildField() Slot { return Slot{Child: Class("p-2")} }

func ChildSlice() []tela.Child { return []tela.Child{Class("p-1"), Div()} }

func ConditionalChild(on bool) tela.Child { return If(on, Class("p-2")) }

func Mixed(on bool) tela.Node { return IfElse(on, Class("p-1"), Div()) }

func Conditionals() tela.Node { return If(true, Div()) }

func Group() tela.Node { return Fragment(Key("k"), Div()) }

func Values() { _ = Class("p-2") }

func Props() tela.Prop { return Class("p-2") }
