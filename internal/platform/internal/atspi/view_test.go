//go:build linux

package atspi

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// TestUpdateViewMatchesBuildView: the view updateView makes from the last
// one and an update's changes is the view buildView makes of the new tree,
// node for node, through random updates (ADR-277); the exposed nodes the
// update did not reach keep their pointers.
func TestUpdateViewMatchesBuildView(t *testing.T) {
	for seed := range uint64(30) {
		s := platformtest.NewAccessScript(seed)
		first := s.First()
		tree, _, err := (*platform.AccessTree)(nil).Apply(&first)
		if err != nil {
			t.Fatal(err)
		}
		v := buildView(tree)
		for step := range 300 {
			u := s.Next()
			next, removed, err := tree.Apply(&u)
			if err != nil {
				t.Fatalf("seed %d step %d: %v", seed, step, err)
			}
			ids := make([]platform.AccessID, len(u.Nodes))
			for i := range u.Nodes {
				ids[i] = u.Nodes[i].ID
			}
			got, touched := updateView(v, next, ids, removed)
			if d := diffViews(got, buildView(next)); d != "" {
				t.Fatalf("seed %d step %d: %s", seed, step, d)
			}
			kept := map[platform.AccessID]bool{}
			for _, id := range touched {
				kept[id] = true
			}
			got.nodes.Range(func(k uint64, x *vnode) bool {
				if o := v.get(platform.AccessID(k)); o != nil && o != x && !kept[platform.AccessID(k)] {
					t.Fatalf("seed %d step %d: node %d has a new exposed node but was not touched", seed, step, k)
				}
				return true
			})
			tree, v = next, got
		}
	}
}

// viewBenchTree returns an update with a tree of about n nodes: sections
// of groups (lists) of rows (list items with a checkbox and a button
// named by a text).
func viewBenchTree(n int) platform.AccessUpdate {
	u := platform.AccessUpdate{Root: 1, Focus: 1}
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Bounds: platform.Rect{W: 800, H: 600}}
	id := platform.AccessID(1)
	add := func(n platform.AccessNode) platform.AccessID {
		id++
		n.ID = id
		u.Nodes = append(u.Nodes, n)
		return id
	}
	for len(u.Nodes) < n {
		var groups []platform.AccessID
		for range 20 {
			var rows []platform.AccessID
			for range 10 {
				txt := add(platform.AccessNode{Kind: platform.AccessText, Text: "row", States: platform.AccessInline})
				box := add(platform.AccessNode{Role: "checkbox", Name: fmt.Sprint("Row ", id)})
				btn := add(platform.AccessNode{Role: "button", Name: "Open", Children: []platform.AccessID{txt}})
				rows = append(rows, add(platform.AccessNode{Role: "listitem", Children: []platform.AccessID{box, btn}}))
			}
			groups = append(groups, add(platform.AccessNode{Role: "list", Children: rows}))
		}
		root.Children = append(root.Children, add(platform.AccessNode{Role: "generic", Children: groups}))
	}
	u.Nodes = append([]platform.AccessNode{root}, u.Nodes...)
	return u
}

// benchViewChange applies a one-node change (a checkbox's checked state)
// to a tree of about 10,000 nodes and makes the view of the new tree,
// incremental or full.
func benchViewChange(b *testing.B, incremental bool) {
	first := viewBenchTree(10000)
	tree, _, err := (*platform.AccessTree)(nil).Apply(&first)
	if err != nil {
		b.Fatal(err)
	}
	v := buildView(tree)
	var n *platform.AccessNode
	tree.Walk(func(x *platform.AccessNode, _ int) bool {
		if n == nil && x.ID >= 5000 && x.Role == "checkbox" {
			n = x
		}
		return n == nil
	})
	changes := [2]platform.AccessUpdate{{Root: 1, Focus: 1, Nodes: []platform.AccessNode{n.Clone()}}, {Root: 1, Focus: 1, Nodes: []platform.AccessNode{n.Clone()}}}
	changes[0].Nodes[0].Checked = platform.ToggleTrue
	ids := []platform.AccessID{n.ID}
	b.ReportAllocs()
	b.ReportMetric(float64(tree.Len()), "nodes")
	i := 0
	for b.Loop() {
		next, removed, _ := tree.Apply(&changes[i&1])
		i++
		if incremental {
			v, _ = updateView(v, next, ids, removed)
		} else {
			v = buildView(next)
		}
		tree = next
	}
}

// BenchmarkViewChange10k: Apply and updateView for a one-node change.
func BenchmarkViewChange10k(b *testing.B) { benchViewChange(b, true) }

// BenchmarkViewBuild10k: Apply and buildView (ADR-246's rebuild) for the
// same change.
func BenchmarkViewBuild10k(b *testing.B) { benchViewChange(b, false) }
