package main

import (
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, time.October, 30, 9, 0, 0, 0, time.UTC)

func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

// pushedNote is a note the server has as it is here.
func pushedNote(id int, uid, title string, upd time.Time) Note {
	return Note{ID: id, UID: uid, Title: title, Text: title, Saved: title, Updated: upd, Pushed: upd}
}

func TestTrack(t *testing.T) {
	a := pushedNote(1, "a", "A", at(0))
	b := pushedNote(2, "b", "B", at(0))

	// A new note gets a UID and Updated; an edit of a field the server keeps
	// is dated; a change of Path or Saved is not an edit.
	next := []Note{a, b, {ID: 3, Title: "C"}}
	next[0].Text = "A, edited"
	next[1].Path, next[1].Saved = "/x/b.md", "other"
	got, tombs := track([]Note{a, b}, next, nil, at(5))
	if got[0].Updated != at(5) || got[1].Updated != at(0) || got[2].Updated != at(5) || got[2].UID == "" || len(tombs) != 0 {
		t.Errorf("track = %+v, %+v", got, tombs)
	}

	// Deleting a note that the server has leaves a tombstone; one that was
	// never pushed leaves none.
	fresh := Note{ID: 4, UID: "d", Title: "D", Updated: at(1)}
	_, tombs = track([]Note{a, fresh}, nil, nil, at(6))
	if len(tombs) != 1 || tombs[0] != (Tombstone{"a", at(6)}) {
		t.Errorf("tombstones = %+v, want one for a", tombs)
	}

	// Undo: the note is back, dated now, and its tombstone is gone.
	got, tombs = track(nil, []Note{a}, []Tombstone{{"a", at(6)}}, at(7))
	if len(tombs) != 0 || got[0].Updated != at(7) {
		t.Errorf("after the undo: %+v, %+v", got, tombs)
	}

	// Locking takes the note off the server; the lock itself is no edit.
	locked := a
	locked.Locked, locked.Title, locked.Text, locked.Saved, locked.Sealed = true, "", "", "", "v1:x"
	got, tombs = track([]Note{a}, []Note{locked}, nil, at(8))
	if len(tombs) != 1 || tombs[0].UID != "a" || !got[0].Updated.Equal(at(0)) {
		t.Errorf("locking: %+v, %+v", got, tombs)
	}
	// Removing the lock puts it back, dated now.
	plain := a
	got, tombs = track([]Note{locked}, []Note{plain}, tombs, at(9))
	if len(tombs) != 0 || got[0].Updated != at(9) {
		t.Errorf("unlocking: %+v, %+v", got, tombs)
	}
}

func TestPlanSync(t *testing.T) {
	local := []Note{
		pushedNote(1, "same", "Same", at(1)),
		pushedNote(2, "remote-newer", "Old title", at(1)),
		pushedNote(3, "local-newer", "New title", at(5)),
		{ID: 4, UID: "only-local", Title: "Local", Updated: at(2)},
		{ID: 5, UID: "locked", Locked: true, Sealed: "v1:x", Updated: at(1)},
		pushedNote(6, "gone-remotely", "Deleted there", at(1)),
		pushedNote(7, "lost-by-server", "Server forgot me", at(1)),
	}
	tombs := []Tombstone{{"deleted-here", at(4)}, {"deleted-here-old", at(1)}, {"server-never-had", at(3)}, {"locked-before", at(4)}}
	remote := []wireNote{
		{UID: "same", Title: "Same", Text: "Same", Updated: at(1)},
		{UID: "remote-newer", Title: "New", Text: "New", Updated: at(3)},
		{UID: "local-newer", Title: "Stale", Updated: at(2)},
		{UID: "only-remote", Title: "Remote", Updated: at(2)},
		{UID: "locked", Title: "Plain copy", Updated: at(9)},
		{UID: "gone-remotely", Deleted: true, Updated: at(4)},
		{UID: "deleted-here", Title: "x", Updated: at(2)},     // we deleted later: push the deletion
		{UID: "deleted-here-old", Title: "y", Updated: at(3)}, // changed there after: it comes back
		{UID: "locked-before", Title: "z", Updated: at(1)},
		{UID: "deleted-both", Deleted: true, Updated: at(1)},
		{UID: "never-seen", Deleted: true, Updated: at(1)},
	}
	tombs = append(tombs, Tombstone{"deleted-both", at(2)})
	p := planSync(local, tombs, remote)

	uids := func(ws []wireNote) []string {
		var out []string
		for _, w := range ws {
			out = append(out, w.UID)
		}
		slices.Sort(out)
		return out
	}
	if got, want := uids(p.Apply), []string{"deleted-here-old", "gone-remotely", "only-remote", "remote-newer"}; !slices.Equal(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
	if got, want := uids(p.Push), []string{"deleted-here", "local-newer", "locked-before", "lost-by-server", "only-local"}; !slices.Equal(got, want) {
		t.Errorf("Push = %v, want %v", got, want)
	}
	for _, w := range p.Push {
		if w.UID == "deleted-here" && (!w.Deleted || !w.Updated.Equal(at(4))) {
			t.Errorf("the deletion is pushed as %+v", w)
		}
	}
	slices.Sort(p.Drop)
	if want := []string{"deleted-both", "deleted-here", "deleted-here-old", "locked-before", "server-never-had"}; !slices.Equal(p.Drop, want) {
		t.Errorf("Drop = %v, want %v", p.Drop, want)
	}
	if len(p.InSync) != 1 || !p.InSync["same"].Equal(at(1)) {
		t.Errorf("InSync = %v", p.InSync)
	}
}

func TestApplySync(t *testing.T) {
	st := libState{
		Notes: []Note{
			pushedNote(1, "a", "A", at(1)),
			pushedNote(2, "b", "B", at(1)),
			pushedNote(3, "c", "C", at(1)),
			{ID: 4, UID: "d", Title: "D", Updated: at(1)},
		},
		Tombs: []Tombstone{{"x", at(1)}, {"y", at(1)}},
	}
	res := syncResult{
		Apply: []wireNote{
			{UID: "a", Title: "A2", Text: "A2", Folder: "F", Pinned: true, Updated: at(4)}, // replaces
			{UID: "b", Deleted: true, Updated: at(4)},                                      // removes
			{UID: "c", Title: "C2", Updated: at(4)},                                        // edited here since the plan: left alone
			{UID: "new", Title: "N", Text: "n", Updated: at(4)},                            // comes in
		},
		Pushed: map[string]time.Time{"d": at(1)},
		Drop:   []string{"x"},
		Base:   map[string]time.Time{"a": at(1), "b": at(1), "c": at(0), "d": at(1)},
	}
	next := 10
	got := applySync(st, res, func() int { next++; return next })
	if len(got.Notes) != 4 {
		t.Fatalf("notes = %+v", got.Notes)
	}
	a, c, d, n := got.Notes[0], got.Notes[1], got.Notes[2], got.Notes[3]
	if a.Title != "A2" || a.Folder != "F" || !a.Pinned || !a.Updated.Equal(at(4)) || !a.Pushed.Equal(at(4)) || a.Text != a.Saved {
		t.Errorf("a = %+v", a)
	}
	if c.Title != "C" || !c.Updated.Equal(at(1)) {
		t.Errorf("c = %+v, want it untouched", c)
	}
	if !d.Pushed.Equal(at(1)) {
		t.Errorf("d = %+v, want it marked as pushed", d)
	}
	if n.ID != 11 || n.UID != "new" || n.Title != "N" || n.Dirty() || !n.Pushed.Equal(at(4)) {
		t.Errorf("the note that came in = %+v", n)
	}
	if len(got.Tombs) != 1 || got.Tombs[0].UID != "y" || got.Rev != st.Rev+1 || got.Edits != st.Edits {
		t.Errorf("tombs %+v, rev %d, edits %d", got.Tombs, got.Rev, got.Edits)
	}

	// Nothing to apply is no change.
	if same := applySync(got, syncResult{}, nil); same.Rev != got.Rev {
		t.Error("an empty result changed the library")
	}
}

func TestPending(t *testing.T) {
	ns := []Note{
		pushedNote(1, "a", "A", at(1)),
		{ID: 2, UID: "b", Updated: at(2)},
		{ID: 3, UID: "c", Updated: at(2), Locked: true},
	}
	if got := pending(ns, []Tombstone{{"x", at(1)}}); got != 2 {
		t.Errorf("pending = %d, want 2 (a note not pushed, a deletion)", got)
	}
}
