package tela

import "github.com/kuronosu/tela/internal/core"

// Node is an immutable, cheap description of a piece of UI: an element
// (h.Div), a text (h.Text), a component or a fragment. Function components
// return one, and every build creates new ones; the persistent state lives
// elsewhere, keyed by position and key (ARCHITECTURE section 6).
//
// The zero Node renders nothing. As a child of an element it keeps the
// positions of its siblings, so h.If(cond, node) does not remount the
// siblings when cond changes.
type Node = core.Node

// Child is what an element takes: a [Node] (a child) or a [Prop] (h.Class,
// h.Style, h.Key, h.OnClick...). Props and children may be mixed in any
// order. The interface is sealed: passing a string or any other value is a
// compile error; wrap text in h.Text.
type Child = core.Child

// Prop is a property of an element: classes, typed style, key, event
// handlers, an image source. Built by the functions of package h.
type Prop = core.Prop

// WithKey returns a copy of n with the key k, whatever its kind. Use it to
// key a node that cannot take the h.Key prop, such as a component or a
// fragment. k must be a non-nil comparable value, such as an ID; otherwise
// WithKey panics.
func WithKey(n Node, k any) Node { return core.WithKey(n, k) }
