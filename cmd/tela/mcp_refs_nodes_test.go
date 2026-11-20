package main

import (
	"strings"
	"testing"
)

// refsForNodes gives refs to a snapshot whose lines the instance identified
// (ids[i] is the identity of line i).
func refsForNodes(tab *refTable, yaml string, ids ...uint64) string {
	lines, cands, _, _ := prepareRefs(yaml)
	return tab.assignNodes(lines, cands, ids)
}

// TestRefsFollowTheirElement checks that a ref is the element, whatever
// happens to its name or to the elements around it: the old scheme, role
// and name and the position among those, gave the ref of the first Save to
// the second one when the first went.
func TestRefsFollowTheirElement(t *testing.T) {
	tab := newRefTable()
	got := refsForNodes(tab, `- button "Save"
- button "Save"
- 'button "Count: 0"'`, 10, 11, 12)
	want := `- button "Save" [ref=e1]
- button "Save" [ref=e2]
- 'button "Count: 0" [ref=e3]'`
	if got != want {
		t.Fatalf("refs:\n%s\nwant\n%s", got, want)
	}
	if e, err := tab.lookup("e2"); err != nil || e.loc.NodeID != 11 || e.loc.Ref != "e2" || e.loc.Role != "" || e.loc.Nth != nil {
		t.Errorf("e2 = %+v, %v; want the element of identity 11", e.loc, err)
	}

	// The first Save is gone, a link came before the rest and the counter
	// changed its name.
	got = refsForNodes(tab, `- link "New"
- button "Save"
- 'button "Count: 1"'`, 20, 11, 12)
	want = `- link "New" [ref=e4]
- button "Save" [ref=e2]
- 'button "Count: 1" [ref=e3]'`
	if got != want {
		t.Errorf("refs after the change:\n%s\nwant\n%s", got, want)
	}
	// A ref of an element that left the snapshot still resolves: the
	// instance knows whether the element is mounted (it may only be hidden).
	if e, err := tab.lookup("e1"); err != nil || e.loc.NodeID != 10 {
		t.Errorf("e1 = %+v, %v", e.loc, err)
	}
	// The element that comes back keeps its ref.
	got = refsForNodes(tab, `- button "Save"
- button "Save"`, 10, 11)
	if want := "- button \"Save\" [ref=e1]\n- button \"Save\" [ref=e2]"; got != want {
		t.Errorf("an element that came back:\n%s\nwant\n%s", got, want)
	}

	// After a remount the identities mean nothing: elements are matched
	// afresh by role, name and position, and numbering goes on.
	tab.remount()
	if _, err := tab.lookup("e2"); err == nil || !strings.Contains(err.Error(), "ref e2 is gone: take a new snapshot") {
		t.Errorf("a ref before the first snapshot of a new mount: %v", err)
	}
	got = refsForNodes(tab, `- link "New"
- button "Save"
- button "Other"`, 1, 2, 3)
	want = `- link "New" [ref=e4]
- button "Save" [ref=e1]
- button "Other" [ref=e5]`
	if got != want {
		t.Errorf("refs after a remount:\n%s\nwant\n%s", got, want)
	}
	// From then on this mount's identities decide, not the names.
	got = refsForNodes(tab, `- link "Renamed"
- button "Other"
- button "Save"`, 1, 3, 2)
	want = `- link "Renamed" [ref=e4]
- button "Other" [ref=e5]
- button "Save" [ref=e1]`
	if got != want {
		t.Errorf("refs by identity in the new mount:\n%s\nwant\n%s", got, want)
	}
}

func TestRefGoneError(t *testing.T) {
	req := mcpReq{Loc: &mcpLoc{NodeID: 11, Ref: "e2"}}
	got := refGoneError(req, `no element matches getByNodeId(11); the screen is settled`)
	if !strings.HasPrefix(got, "ref e2 is gone: take a new snapshot") {
		t.Errorf("gone ref: %q", got)
	}
	for _, msg := range []string{`no element matches getByNodeId(12)`, `no element matches getByRole("button")`, "other"} {
		if got := refGoneError(req, msg); got != msg {
			t.Errorf("%q became %q", msg, got)
		}
	}
	// The target of a drag.
	req = mcpReq{Loc: &mcpLoc{Role: "button"}, To: &mcpLoc{NodeID: 5, Ref: "e9"}}
	if got := refGoneError(req, `no element matches getByNodeId(5)`); !strings.HasPrefix(got, "ref e9 is gone") {
		t.Errorf("gone drag target: %q", got)
	}
}
