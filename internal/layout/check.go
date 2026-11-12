package layout

import "fmt"

// CheckFull lays out the tree rooted at root from scratch into the given
// space, as a fresh Engine with no caches would, and compares the layout
// of every node with the one it has (the result of the incremental
// Computes so far). It then gives every node its state back (layouts and
// caches), so the check changes nothing, and returns the first node, in
// tree order, whose layout differs, with the difference. Measurers are
// called again: their results must be functions of their input (Node).
//
// It is a test aid (core's TELA_CHECK_LAYOUT): it costs a full layout.
func CheckFull(root Node, width, height AvailableSpace) (Node, error) {
	var nodes []Node
	var saved []Data
	var walk func(n Node)
	walk = func(n Node) {
		nodes = append(nodes, n)
		saved = append(saved, *n.LayoutData())
		for i, c := 0, n.LayoutChildCount(); i < c; i++ {
			walk(n.LayoutChild(i))
		}
	}
	walk(root)
	for _, n := range nodes {
		d := n.LayoutData()
		d.cache = cache{}
		d.hasLast, d.needLayout = false, false
	}
	var ref Engine
	ref.Compute(root, width, height)
	var bad Node
	var err error
	for i, n := range nodes {
		if got, want := saved[i].unrounded, n.LayoutData().unrounded; got != want {
			bad, err = n, fmt.Errorf("has %+v, a full layout gives %+v", got, want)
			break
		}
	}
	for i, n := range nodes {
		*n.LayoutData() = saved[i]
	}
	return bad, err
}
