package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// Refs (ADR-174), Playwright MCP's model: each line of the aria snapshot
// that names an element with a role carries [ref=eN], and a tool given the
// ref acts on that element. The instance has only telatest's public
// locators, so a ref stands for ByRole(role, name) with the Nth that picks
// the line's element: the aria snapshot and the role locators walk the
// same semantics tree in the same order, skipping the same hidden and
// role-less elements. The server checks that before giving the refs out:
// for each role and name it asks the instance how many elements the
// locator matches, and a line whose count differs from the snapshot's
// (the snapshot drops explicit generics without content) gets no ref.
//
// Refs are stable: the same role, name and position among those keep their
// number in later snapshots, also after a rebuild, so a ref the agent holds
// still names the same element after an edit or an action that did not
// move it. Within one mount, an element whose name changed (a counter's
// label) also keeps the ref of the element at its role and position. After
// a remount the refs are checked against a new snapshot before one is
// used, and numbering only goes on, so a ref never moves to an element of
// another role or name.

// refKey identifies an element across snapshots of one mount: its role, its
// name and its position among the elements with both.
type refKey struct {
	role, name string
	hasName    bool
	nth        int
}

// refEntry is what a ref stands for.
type refEntry struct {
	loc      mcpLoc
	desc     string // `button "Save"`
	disabled bool
}

// refTable numbers the elements of the snapshots of one server.
type refTable struct {
	next int
	// byKey maps the latest key of each ref to it; keyOf is the inverse. A
	// ref whose element changed its name leaves its old key, so the old
	// name cannot bring it back to another element.
	byKey   map[refKey]string
	keyOf   map[string]refKey
	byPos   map[refPos]string   // the refs of the latest snapshot by position
	current map[string]refEntry // the refs of the latest snapshot
	issued  map[string]bool     // every ref ever given out
	// stale: the screen was mounted again since the latest snapshot, so
	// current must be taken again before a ref is used.
	stale bool
}

// refPos is an element's role and its position among the elements of the
// role in a snapshot.
type refPos struct {
	role string
	nth  int
}

func newRefTable() *refTable {
	return &refTable{byKey: map[refKey]string{}, keyOf: map[string]refKey{}, byPos: map[refPos]string{}, current: map[string]refEntry{}, issued: map[string]bool{}}
}

// remount records that the screen was mounted afresh: the refs of the
// latest snapshot must be checked again, and positions say nothing about
// the new mount's elements.
func (t *refTable) remount() {
	t.byPos = map[refPos]string{}
	t.current = map[string]refEntry{}
	t.stale = true
}

// snapCandidate is a line of a snapshot that may get a ref.
type snapCandidate struct {
	index int // line number
	line  semantics.SnapshotLine
	key   refKey
	pos   refPos
	loc   mcpLoc
	group int // index into the groups to verify
}

// prepareRefs parses yaml and returns the candidates and the locators whose
// counts must match the snapshot's (one per role and name, or per role
// for the lines without a printed name), with the counts the snapshot
// implies.
func prepareRefs(yaml string) (lines []string, cands []snapCandidate, groups []mcpLoc, want []int) {
	if yaml == "" {
		return nil, nil, nil, nil
	}
	lines = strings.Split(yaml, "\n")
	type gk struct {
		role, name string
		hasName    bool
	}
	groupOf := map[gk]int{}
	byRole := map[string]int{} // elements of a role so far
	byName := map[gk]int{}     // elements of a role and name so far
	for i, s := range lines {
		l, ok := semantics.ParseSnapshotLine(s)
		if !ok {
			continue
		}
		roleNth := byRole[l.Role]
		byRole[l.Role]++
		k := gk{l.Role, l.Name, l.HasName}
		var c snapCandidate
		c.index, c.line, c.pos = i, l, refPos{l.Role, roleNth}
		if l.HasName {
			nth := byName[k]
			byName[k]++
			c.key = refKey{l.Role, l.Name, true, nth}
			c.loc = mcpLoc{Role: l.Role, Name: l.Name}
			c.loc.Nth = &nth
		} else {
			// Without a printed name (none, or longer than the snapshot
			// prints) ByRole(role, "") matches every name of the role.
			k = gk{l.Role, "", false}
			c.key = refKey{l.Role, "", false, roleNth}
			c.loc = mcpLoc{Role: l.Role}
			c.loc.Nth = &roleNth
		}
		g, ok := groupOf[k]
		if !ok {
			g = len(groups)
			groupOf[k] = g
			groups = append(groups, mcpLoc{Role: k.role, Name: k.name})
			want = append(want, 0)
		}
		c.group = g
		cands = append(cands, c)
	}
	for k, g := range groupOf {
		if k.hasName {
			want[g] = byName[k]
		} else {
			want[g] = byRole[k.role]
		}
	}
	return lines, cands, groups, want
}

// assign gives refs to the candidates whose group's count matched (got[g]
// == want[g]) and returns the snapshot with them. A locator that matches
// one element needs no Nth.
//
// An element keeps its ref from earlier snapshots when its role, name and
// position among those are the same; one whose name changed (a counter's
// label) keeps the ref of the element that had its role and position in
// the previous snapshot, when no other element took it, as a re-rendered
// DOM node keeps its ref in Playwright.
func (t *refTable) assign(lines []string, cands []snapCandidate, want, got []int) string {
	refs := make([]string, len(cands))
	used := map[string]bool{}
	ok := func(c snapCandidate) bool { return c.group < len(got) && got[c.group] == want[c.group] }
	for i, c := range cands {
		if ref, found := t.byKey[c.key]; ok(c) && found && !used[ref] {
			refs[i], used[ref] = ref, true
		}
	}
	for i, c := range cands {
		if ref, found := t.byPos[c.pos]; ok(c) && refs[i] == "" && found && !used[ref] {
			refs[i], used[ref] = ref, true
		}
	}
	t.current = map[string]refEntry{}
	t.byPos = map[refPos]string{}
	t.stale = false
	for i, c := range cands {
		if !ok(c) {
			continue
		}
		ref := refs[i]
		if ref == "" {
			t.next++
			ref = "e" + strconv.Itoa(t.next)
		}
		if old, had := t.keyOf[ref]; had && old != c.key && t.byKey[old] == ref {
			delete(t.byKey, old)
		}
		t.byKey[c.key], t.keyOf[ref] = ref, c.key
		t.byPos[c.pos] = ref
		loc := c.loc
		if want[c.group] == 1 {
			loc.Nth = nil
		}
		t.current[ref] = refEntry{loc: loc, desc: c.line.Describe(), disabled: c.line.Disabled}
		t.issued[ref] = true
		lines[c.index] = c.line.WithSuffix(" [ref=" + ref + "]")
	}
	return strings.Join(lines, "\n")
}

// lookup returns the element of a ref of the latest snapshot.
func (t *refTable) lookup(ref string) (refEntry, error) {
	ref = strings.TrimSpace(ref)
	if e, ok := t.current[ref]; ok {
		return e, nil
	}
	if t.issued[ref] {
		return refEntry{}, fmt.Errorf("ref %q is not in the latest snapshot: the element is gone, or its role or name changed; take a new snapshot and use its refs", ref)
	}
	return refEntry{}, fmt.Errorf("unknown ref %q: refs such as \"e5\" come from the [ref=…] marks of the latest snapshot; call snapshot and use one of them", ref)
}
