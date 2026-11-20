package semantics

import (
	"strings"
	"testing"
)

func TestSnapshotIDs(t *testing.T) {
	root := &Node{ID: 1, Kind: KindRoot, Children: []*Node{
		{ID: 2, Kind: KindElement, Role: "heading", Name: "Title", Level: 1},
		{ID: 3, Kind: KindElement, Role: "link", Name: "Docs", URL: "https://x.dev", Children: []*Node{{ID: 4, Kind: KindText, Text: "Docs"}}},
		{ID: 5, Kind: KindElement, Role: "list", Children: []*Node{
			{ID: 6, Kind: KindElement, Role: "listitem", Children: []*Node{{ID: 7, Kind: KindText, Text: "one"}}},
		}},
	}}
	yaml, ids := SnapshotIDs(root)
	lines := strings.Split(yaml, "\n")
	if len(ids) != len(lines) {
		t.Fatalf("%d ids for %d lines:\n%s", len(ids), len(lines), yaml)
	}
	if yaml != Snapshot(root) {
		t.Errorf("SnapshotIDs and Snapshot disagree:\n%s\n%s", yaml, Snapshot(root))
	}
	want := map[string]NodeID{`- heading "Title" [level=1]`: 2, `- link "Docs":`: 3, `  - /url: https://x.dev`: 0, `- list:`: 5, `  - listitem: one`: 6}
	for i, l := range lines {
		if w, ok := want[l]; !ok || ids[i] != w {
			t.Errorf("line %d %q has id %d, want %d (known %v)", i, l, ids[i], w, ok)
		}
	}
	if y, ids := SnapshotIDs(nil); y != "" || ids != nil {
		t.Errorf("a nil tree: %q, %v", y, ids)
	}
}
