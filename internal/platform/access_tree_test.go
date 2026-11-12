package platform

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

// accessModel is a random tree that random edits change, and the updates
// that describe them (as core sends them: the nodes that changed, a
// parent whose children changed among them).
type accessModel struct {
	r      *rand.Rand
	nodes  map[AccessID]*AccessNode
	parent map[AccessID]AccessID
	next   AccessID
}

func newAccessModel(seed uint64) *accessModel {
	m := &accessModel{r: rand.New(rand.NewPCG(seed, 7)), nodes: map[AccessID]*AccessNode{}, parent: map[AccessID]AccessID{}, next: 1}
	root := m.add(0)
	root.Kind, root.Role = AccessRoot, ""
	for range 60 {
		m.add(m.pick())
	}
	return m
}

func (m *accessModel) add(parent AccessID) *AccessNode {
	id := m.next
	m.next++
	n := &AccessNode{ID: id, Kind: AccessElement, Role: "generic", Name: fmt.Sprint("n", id)}
	m.nodes[id] = n
	if parent != 0 {
		p := m.nodes[parent]
		i := m.r.IntN(len(p.Children) + 1)
		p.Children = slices.Insert(slices.Clone(p.Children), i, id)
		m.parent[id] = parent
	}
	return n
}

// pick returns a random node.
func (m *accessModel) pick() AccessID {
	ids := make([]AccessID, 0, len(m.nodes))
	for id := range m.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids[m.r.IntN(len(ids))]
}

// inside reports whether id is a or below it.
func (m *accessModel) inside(id, a AccessID) bool {
	for ; id != 0; id = m.parent[id] {
		if id == a {
			return true
		}
	}
	return false
}

func (m *accessModel) removeSubtree(id AccessID) {
	for _, c := range m.nodes[id].Children {
		m.removeSubtree(c)
	}
	delete(m.nodes, id)
	delete(m.parent, id)
}

// edit changes the tree at random and returns the IDs of the nodes that
// changed.
func (m *accessModel) edit() []AccessID {
	var changed []AccessID
	for range 1 + m.r.IntN(4) {
		switch k := m.r.IntN(5); {
		case k == 0: // a new node, sometimes with a child
			p := m.pick()
			n := m.add(p)
			changed = append(changed, p, n.ID)
			if m.r.IntN(2) == 0 {
				changed = append(changed, m.add(n.ID).ID)
			}
		case k == 1: // a removal
			id := m.pick()
			if id == 1 {
				continue
			}
			p := m.parent[id]
			pn := m.nodes[p]
			pn.Children = slices.DeleteFunc(slices.Clone(pn.Children), func(c AccessID) bool { return c == id })
			m.removeSubtree(id)
			changed = append(changed, p)
		case k == 2: // a move
			id, to := m.pick(), m.pick()
			if id == 1 || m.inside(to, id) {
				continue
			}
			from := m.parent[id]
			fn := m.nodes[from]
			fn.Children = slices.DeleteFunc(slices.Clone(fn.Children), func(c AccessID) bool { return c == id })
			tn := m.nodes[to]
			tn.Children = slices.Insert(slices.Clone(tn.Children), m.r.IntN(len(tn.Children)+1), id)
			m.parent[id] = to
			changed = append(changed, from, to)
		case k == 3: // a reorder
			p := m.nodes[m.pick()]
			if len(p.Children) > 1 {
				p.Children = slices.Clone(p.Children)
				m.r.Shuffle(len(p.Children), func(i, j int) { p.Children[i], p.Children[j] = p.Children[j], p.Children[i] })
				changed = append(changed, p.ID)
			}
		default: // a new name
			id := m.pick()
			m.nodes[id].Name += "'"
			changed = append(changed, id)
		}
	}
	return changed
}

// update returns the update of the nodes ids (those still in the tree).
func (m *accessModel) update(ids []AccessID) AccessUpdate {
	u := AccessUpdate{Root: 1, Focus: 1}
	seen := map[AccessID]bool{}
	for _, id := range ids {
		if n := m.nodes[id]; n != nil && !seen[id] {
			seen[id] = true
			u.Nodes = append(u.Nodes, n.Clone())
		}
	}
	if m.r.IntN(4) == 0 {
		u.Focus = m.pick()
	}
	return u
}

// sameTrees fails unless a and b hold the same nodes and parents.
func sameTrees(t *testing.T, a, b *AccessTree) {
	t.Helper()
	if a.Len() != b.Len() || a.Root() != b.Root() || a.Focus() != b.Focus() {
		t.Fatalf("len %d root %d focus %d, full len %d root %d focus %d", a.Len(), a.Root(), a.Focus(), b.Len(), b.Root(), b.Focus())
	}
	b.Walk(func(n *AccessNode, _ int) bool {
		m, ok := a.Node(n.ID)
		if !ok || !m.Equal(n) {
			t.Fatalf("node %d: %+v, full %+v", n.ID, m, n)
		}
		pa, _ := a.Parent(n.ID)
		pb, _ := b.Parent(n.ID)
		if pa != pb {
			t.Fatalf("node %d: parent %d, full %d", n.ID, pa, pb)
		}
		return true
	})
}

// TestAccessTreeApplyIncremental: Apply, which follows the changed
// children lists, gives what a walk of the whole tree gives, with the
// same removed nodes, for random insertions, removals, moves, reorders
// and renames; the nodes an update does not name keep their pointers; and
// invalid updates fail with applyFull's error.
func TestAccessTreeApplyIncremental(t *testing.T) {
	for seed := range uint64(20) {
		m := newAccessModel(seed)
		all := make([]AccessID, 0, len(m.nodes))
		for id := range m.nodes {
			all = append(all, id)
		}
		first := m.update(all)
		first.Focus = 1
		tree, _, err := (*AccessTree)(nil).Apply(&first)
		if err != nil {
			t.Fatal(err)
		}
		for step := range 200 {
			u := m.update(m.edit())
			got, removed, err := tree.Apply(&u)
			want, wremoved, werr := tree.applyFull(&u)
			if err != nil || werr != nil {
				t.Fatalf("seed %d step %d: %v, full %v", seed, step, err, werr)
			}
			sameTrees(t, got, want)
			slices.Sort(removed)
			slices.Sort(wremoved)
			if !slices.Equal(removed, wremoved) {
				t.Fatalf("seed %d step %d: removed %v, full %v", seed, step, removed, wremoved)
			}
			named := map[AccessID]bool{}
			for _, n := range u.Nodes {
				named[n.ID] = true
			}
			got.Walk(func(n *AccessNode, _ int) bool {
				if old, ok := tree.Node(n.ID); ok && !named[n.ID] && old != n {
					t.Fatalf("seed %d step %d: node %d, not in the update, has a new pointer", seed, step, n.ID)
				}
				return true
			})
			tree = got
		}
		// Invalid updates: a second parent, a cycle, an unknown child, a
		// focus outside the tree.
		root, _ := tree.Node(1)
		if len(root.Children) == 0 {
			continue
		}
		c := root.Children[0]
		cn, _ := tree.Node(c)
		twice := root.Clone()
		twice.Children = append(twice.Children, c)
		unknown := cn.Clone()
		unknown.Children = append(unknown.Children, 9999)
		cycle := cn.Clone()
		cycle.Children = append(cycle.Children, 1)
		for i, bad := range []AccessUpdate{
			{Root: 1, Focus: 1, Nodes: []AccessNode{twice}},
			{Root: 1, Focus: 1, Nodes: []AccessNode{unknown}},
			{Root: 1, Focus: 1, Nodes: []AccessNode{cycle}},
			{Root: 1, Focus: 9999},
		} {
			_, _, err := tree.Apply(&bad)
			_, _, werr := tree.applyFull(&bad)
			if err == nil || werr == nil || err.Error() != werr.Error() {
				t.Errorf("seed %d, invalid update %d: %v, full %v", seed, i, err, werr)
			}
		}
	}
}

// accessBenchTree returns an update with a tree of about n nodes: a root,
// sections of groups of rows of 3 nodes.
func accessBenchTree(n int) AccessUpdate {
	u := AccessUpdate{Root: 1, Focus: 1}
	id := AccessID(1)
	newNode := func(role string) AccessID {
		id++
		u.Nodes = append(u.Nodes, AccessNode{ID: id, Kind: AccessElement, Role: role, Name: fmt.Sprint("node ", id)})
		return id
	}
	root := AccessNode{ID: 1, Kind: AccessRoot}
	for len(u.Nodes) < n {
		sec := newNode("group")
		var groups []AccessID
		for range 20 {
			g := newNode("list")
			var rows []AccessID
			for range 10 {
				r := newNode("listitem")
				a, b := newNode("checkbox"), newNode("button")
				i := slices.IndexFunc(u.Nodes, func(x AccessNode) bool { return x.ID == r })
				u.Nodes[i].Children = []AccessID{a, b}
				rows = append(rows, r)
			}
			i := slices.IndexFunc(u.Nodes, func(x AccessNode) bool { return x.ID == g })
			u.Nodes[i].Children = rows
			groups = append(groups, g)
		}
		i := slices.IndexFunc(u.Nodes, func(x AccessNode) bool { return x.ID == sec })
		u.Nodes[i].Children = groups
		root.Children = append(root.Children, sec)
	}
	u.Nodes = append([]AccessNode{root}, u.Nodes...)
	return u
}

// BenchmarkAccessTreeApplyChange10k applies a one-node change (a checked
// state) to a tree of about 10,000 nodes.
func BenchmarkAccessTreeApplyChange10k(b *testing.B) {
	first := accessBenchTree(10000)
	tree, _, err := (*AccessTree)(nil).Apply(&first)
	if err != nil {
		b.Fatal(err)
	}
	n, _ := tree.Node(5000)
	changes := [2]AccessUpdate{{Root: 1, Focus: 1, Nodes: []AccessNode{n.Clone()}}, {Root: 1, Focus: 1, Nodes: []AccessNode{n.Clone()}}}
	changes[1].Nodes[0].Checked = ToggleTrue
	b.ReportAllocs()
	b.ReportMetric(float64(tree.Len()), "nodes")
	i := 0
	for b.Loop() {
		tree, _, _ = tree.Apply(&changes[i&1])
		i++
	}
}

// BenchmarkAccessTreeApplyFull10k is the same change applied by a walk of
// the whole tree (ADR-246's Apply).
func BenchmarkAccessTreeApplyFull10k(b *testing.B) {
	first := accessBenchTree(10000)
	tree, _, err := (*AccessTree)(nil).Apply(&first)
	if err != nil {
		b.Fatal(err)
	}
	n, _ := tree.Node(5000)
	changes := [2]AccessUpdate{{Root: 1, Focus: 1, Nodes: []AccessNode{n.Clone()}}, {Root: 1, Focus: 1, Nodes: []AccessNode{n.Clone()}}}
	changes[1].Nodes[0].Checked = ToggleTrue
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		tree, _, _ = tree.applyFull(&changes[i&1])
		i++
	}
}
