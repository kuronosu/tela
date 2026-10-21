package core

import "github.com/kuronosu/tela/internal/style"

// StyleProp is one typed style declaration, or a pack of several
// (tela.StyleProp): built by the functions of package tela and passed to
// h.Style, or given to an element directly, since it is a Child that
// applies like Style(it) (ADR-091). The zero StyleProp changes nothing.
type StyleProp struct{ ops []style.Op }

// StyleOps returns the StyleProp of ops, applied in order.
func StyleOps(ops ...style.Op) StyleProp { return StyleProp{ops: ops} }

// Ops returns the ops of p, in order. The slice must not be modified.
func (p StyleProp) Ops() []style.Op { return p.ops }

// Build returns the node of Style(p): given to an element it applies the
// typed style props after the classes, like h.Style(p); mounted on its
// own it panics, since a prop renders nothing.
func (p StyleProp) Build() Node {
	if len(p.ops) == 0 {
		return Prop{}.Build()
	}
	return Style(p.ops...).Build()
}

// StyleOpsOf returns the ops of every prop of props, in order.
func StyleOpsOf(props []StyleProp) []style.Op {
	if len(props) == 1 {
		return props[0].ops
	}
	n := 0
	for _, p := range props {
		n += len(p.ops)
	}
	ops := make([]style.Op, 0, n)
	for _, p := range props {
		ops = append(ops, p.ops...)
	}
	return ops
}
