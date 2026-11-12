package semantics

import "testing"

// setTree returns a list of items (one hidden, one with an explicit
// position, two inside a generic wrapper) and a nested group of options.
func setTree() *Node {
	item := func(id NodeID) *Node { return &Node{ID: id, Kind: KindElement, Role: "listitem"} }
	hidden := item(3)
	hidden.States = StateHidden
	explicit := item(4)
	explicit.PosInSet = 7
	wrap := &Node{ID: 5, Kind: KindElement, Role: "generic", Children: []*Node{item(6), item(7)}}
	group := &Node{ID: 8, Kind: KindElement, Role: "group", Children: []*Node{
		{ID: 9, Kind: KindElement, Role: "listitem"}, {ID: 10, Kind: KindElement, Role: "listitem"}}}
	return &Node{ID: 1, Kind: KindElement, Role: "list", Children: []*Node{item(2), hidden, explicit, wrap, group}}
}

// TestNumberSetMatchesFillSetPositions: NumberSet gives the positions
// FillSetPositions gives, and changes the nodes it does not own through
// writable only, leaving the shared ones as they were.
func TestNumberSetMatchesFillSetPositions(t *testing.T) {
	want := setTree()
	FillSetPositions(want)

	shared := setTree()
	FillSetPositions(shared) // an earlier tree's values, shared
	shared.Children[3].Children[1].PosInSet = 99
	got := &Node{ID: 1, Kind: KindElement, Role: "list", Children: shared.Children}
	explicit := func(n *Node) (int, int) {
		if n.ID == 4 {
			return 7, 0
		}
		return 0, 0
	}
	copies := 0
	writable := func(n *Node) *Node {
		if n == got {
			return n
		}
		copies++
		c := *n
		return &c
	}
	NumberSet(got, explicit, writable)
	if shared.Children[3].Children[1].PosInSet != 99 {
		t.Error("a shared node was modified")
	}
	if copies != 2 { // the item and its wrapper
		t.Errorf("%d copies, want 2", copies)
	}
	var walk func(a, b *Node)
	walk = func(a, b *Node) {
		if a.ID != b.ID || a.PosInSet != b.PosInSet || a.SetSize != b.SetSize {
			t.Errorf("node %d: %d of %d, want %d of %d", a.ID, a.PosInSet, a.SetSize, b.PosInSet, b.SetSize)
		}
		for i := range b.Children {
			walk(a.Children[i], b.Children[i])
		}
	}
	walk(got, want)
}
