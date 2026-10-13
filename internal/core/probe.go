package core

import (
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// Queries of telatest on the last semantics tree (semantics.go): they map
// between semantics nodes and the Elements behind them. Like the actions
// of semantics.go they use the tree built by the last Semantics call.

// HitNode returns the ID of the semantics node of the element under the
// point (x, y) of the window, in logical pixels: the element an event
// there targets, or its nearest ancestor that has a node in the last
// semantics tree. It is 0 for a point outside the window and before the
// first frame.
func (t *Tree) HitNode(x, y float32) semantics.NodeID {
	for el := t.HitTest(x, y); el != nil; el = el.parent {
		if el.semID != 0 && t.sem.byID[semantics.NodeID(el.semID)] == el {
			return semantics.NodeID(el.semID)
		}
	}
	return 0
}

// NodeClasses returns the Tailwind class names (the tokens of the Class
// props, in order of appearance) of the element of node id in the last
// semantics tree. It is nil for a text, a node that is gone and an
// element without classes.
func (t *Tree) NodeClasses(id semantics.NodeID) []string {
	el := t.sem.byID[id]
	if el == nil || !el.mounted || el.kind != elemPrimitive {
		return nil
	}
	var out []string
	for _, c := range el.node.d.classes {
		out = append(out, strings.Fields(c)...)
	}
	return out
}

// BreakClickSequence ends the sequence of consecutive clicks: the next
// press counts as a first click (ClickCount 1) whatever the time and
// distance since the last one. telatest calls it before each separate
// click, as Playwright gives every click() a click count of 1.
func (t *Tree) BreakClickSequence() { t.in.clicks.Reset() }
