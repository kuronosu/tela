package semantics

import (
	"slices"
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

// A text line names the first text node it shows; the other lines name
// none.
func TestSnapshotTexts(t *testing.T) {
	root := &Node{ID: 1, Kind: KindRoot, Children: []*Node{
		{ID: 2, Kind: KindElement, Role: "paragraph", Children: []*Node{
			{ID: 3, Kind: KindText, Text: "Read the ", Inline: true},
			{ID: 4, Kind: KindElement, Role: "link", Name: "docs", Inline: true, Children: []*Node{{ID: 5, Kind: KindText, Text: "docs", Inline: true}}},
			{ID: 6, Kind: KindText, Text: " today, ", Inline: true},
			{ID: 7, Kind: KindText, Text: "then the rest.", Inline: true},
		}},
		{ID: 8, Kind: KindText, Text: "Loose"},
	}}
	yaml, ids, texts := SnapshotTexts(root)
	if y, i := SnapshotIDs(root); y != yaml || !slices.Equal(i, ids) {
		t.Errorf("SnapshotTexts and SnapshotIDs disagree:\n%s\n%s", yaml, y)
	}
	lines := strings.Split(yaml, "\n")
	if len(texts) != len(lines) {
		t.Fatalf("%d texts for %d lines:\n%s", len(texts), len(lines), yaml)
	}
	want := map[string]NodeID{"- paragraph:": 0, "  - text: Read the": 3, `  - link "docs"`: 0, "  - text: today, then the rest.": 6, "- text: Loose": 8}
	for i, l := range lines {
		if w, ok := want[l]; !ok || texts[i] != w {
			t.Errorf("line %d %q has text %d, want %d (known %v)", i, l, texts[i], w, ok)
		}
	}
}
