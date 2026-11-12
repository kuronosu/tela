package cocoa

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// diffAXTrees returns how a differs from b ("" when it does not): every
// entry, and the nodes no longer exposed since old.
func diffAXTrees(a, b, old *axTree) string {
	if a.ents.Len() != b.ents.Len() {
		return fmt.Sprintf("%d entries, full %d", a.ents.Len(), b.ents.Len())
	}
	diff := ""
	b.ents.Range(func(k uint64, y axEntry) bool {
		x, ok := a.ents.Get(k)
		if !ok || x.up != y.up || x.leaf != y.leaf || !slices.Equal(x.kids, y.kids) {
			diff = fmt.Sprintf("node %d: %+v, full %+v", k, x, y)
		}
		return diff == ""
	})
	if diff == "" && !slices.Equal(a.lostSince(old), b.lostSince(old)) {
		diff = fmt.Sprintf("lost %v, full %v", a.lostSince(old), b.lostSince(old))
	}
	return diff
}

// TestAXTreeUpdate: the structure updateAXTree makes from the last one
// and an update's changes is the one newAXTree makes of the new tree, and
// the notifications are the same, through random updates (ADR-277).
func TestAXTreeUpdate(t *testing.T) {
	for seed := range uint64(60) {
		s := platformtest.NewAccessScript(seed)
		tree := applyTree(t, nil, s.First())
		v := newAXTree(tree)
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
			got := updateAXTree(v, next, ids, removed)
			full := newAXTree(next)
			if d := diffAXTrees(got, full, v); d != "" {
				t.Fatalf("seed %d step %d: %s", seed, step, d)
			}
			if a, b := noteStrings(axNotes(v, got, &u)), noteStrings(axNotes(v, full, &u)); a != b {
				t.Fatalf("seed %d step %d: notes\n%s\nfull\n%s", seed, step, a, b)
			}
			tree, v = next, got
		}
	}
}
