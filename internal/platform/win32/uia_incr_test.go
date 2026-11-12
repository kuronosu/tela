package win32

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// diffUIASnapshots returns how a differs from b ("" when it does not):
// every node's entry, the focus and the nodes no longer exposed since old.
func diffUIASnapshots(a, b, old *uiaSnapshot) string {
	if a.ents.Len() != b.ents.Len() {
		return fmt.Sprintf("%d entries, full %d", a.ents.Len(), b.ents.Len())
	}
	diff := ""
	b.ents.Range(func(k uint64, y uiaEntry) bool {
		x, ok := a.entry(platform.AccessID(k))
		if !ok || x.exposed != y.exposed || x.up != y.up || x.pos != y.pos || x.anc != y.anc ||
			x.leaf != y.leaf || !slices.Equal(x.kids, y.kids) {
			diff = fmt.Sprintf("node %d: %+v, full %+v", k, x, y)
		}
		return diff == ""
	})
	if diff == "" && a.focus != b.focus {
		diff = fmt.Sprintf("focus %d, full %d", a.focus, b.focus)
	}
	if diff == "" && !slices.Equal(a.lostSince(old), b.lostSince(old)) {
		diff = fmt.Sprintf("lost %v, full %v", a.lostSince(old), b.lostSince(old))
	}
	return diff
}

// TestUIASnapshotUpdate: the snapshot updateUIASnapshot makes from the
// last one and an update's changes is the one newUIASnapshot makes of the
// new tree, through random updates (ADR-277).
func TestUIASnapshotUpdate(t *testing.T) {
	for seed := range uint64(60) {
		s := platformtest.NewAccessScript(seed)
		first := s.First()
		tree := applyTree(t, nil, first)
		snap := newUIASnapshot(tree)
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
			got := updateUIASnapshot(snap, next, ids, removed)
			if d := diffUIASnapshots(got, newUIASnapshot(next), snap); d != "" {
				t.Fatalf("seed %d step %d: %s", seed, step, d)
			}
			// The events are those of the full snapshot.
			full := newUIASnapshot(next)
			if a, b := eventString(uiaEvents(snap, got, &u)), eventString(uiaEvents(snap, full, &u)); a != b {
				t.Fatalf("seed %d step %d: events\n%s\nfull\n%s", seed, step, a, b)
			}
			tree, snap = next, got
		}
	}
}
