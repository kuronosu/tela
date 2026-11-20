package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sync/atomic"
	"time"
)

// Sync keeps the library and a server's notes the same, modestly: every
// note has a UID, the same on every device, and an Updated time that the
// device sets when it changes a field the server keeps (title, text, folder,
// pin). A sync gets the server's notes and compares them one by one with the
// local ones: the later Updated wins (last writer wins), a newer remote note
// replaces the local one, a newer local one is pushed, and a note deleted on
// one side is deleted on the other unless the other changed it after. A
// deletion is a tombstone with its time. Locked notes never leave the
// device. Nothing here knows the screen: planSync is a function of the two
// lists, runSync does the requests (from tela.UseAsync, off the UI thread),
// and applySync puts the outcome in the library on the UI thread.
//
// The server is a small REST API behind an OAuth sign-in (demo.go is one):
//
//	GET  /api/me           {"name": "..."}
//	GET  /api/notes        [wireNote...]   every note, deleted ones included
//	PUT  /api/notes/{uid}  wireNote        stored when its updated is later
//	                                       than the stored one; the answer is
//	                                       the note the server now has
//
// with "Authorization: Bearer <token>" on every request.

// wireNote is a note as the server keeps it.
type wireNote struct {
	UID     string    `json:"uid"`
	Title   string    `json:"title"`
	Text    string    `json:"text"`
	Folder  string    `json:"folder"`
	Pinned  bool      `json:"pinned"`
	Updated time.Time `json:"updated"`
	// Deleted marks a tombstone: the note is gone, as of Updated.
	Deleted bool `json:"deleted,omitempty"`
}

func wireOf(n Note) wireNote {
	return wireNote{UID: n.UID, Title: n.Title, Text: n.Text, Folder: n.Folder, Pinned: n.Pinned, Updated: n.Updated.UTC()}
}

// newUID is the name of a note on the server.
func newUID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// libState is what the screen keeps of the library: the notes, the
// tombstones of the notes deleted here that the server may not know, and
// two counters. Edits counts the changes the user made (a sync follows
// them); Rev counts every change of the notes (the store writes follow).
type libState struct {
	Notes []Note
	Tombs []Tombstone
	Edits int
	Rev   int
}

// syncedFieldsChanged reports whether the fields the server keeps differ.
func syncedFieldsChanged(a, b Note) bool {
	return a.Title != b.Title || a.Text != b.Text || a.Folder != b.Folder || a.Pinned != b.Pinned
}

// track returns the library after an edit by the user, from prev to next: a
// new note gets a UID and is Updated at at (also one that comes back after
// an Undo, which must beat the tombstone the server may already have), a
// note whose title, text, folder or pin changed is Updated at at, and a note
// that was deleted, or locked, leaves a tombstone if the server could have
// it. Notes that are locked are never Updated: their content is not the
// server's.
func track(prev, next []Note, tombs []Tombstone, at time.Time) ([]Note, []Tombstone) {
	old := make(map[int]Note, len(prev))
	for _, n := range prev {
		old[n.ID] = n
	}
	out := slices.Clone(next)
	tombs = slices.Clone(tombs)
	addTomb := func(uid string) {
		tombs = slices.DeleteFunc(tombs, func(t Tombstone) bool { return t.UID == uid })
		tombs = append(tombs, Tombstone{UID: uid, At: at})
	}
	kept := make(map[int]bool, len(next))
	for i, n := range out {
		kept[n.ID] = true
		if n.UID == "" {
			n.UID = newUID()
		}
		o, had := old[n.ID]
		switch {
		case n.Locked:
			if had && !o.Locked && !o.Pushed.IsZero() {
				addTomb(n.UID) // locking takes the note off the server
			}
		case !had, o.Locked, syncedFieldsChanged(o, n):
			n.Updated = at
		}
		out[i] = n
	}
	for _, o := range prev {
		if !kept[o.ID] && o.UID != "" && !o.Locked && !o.Pushed.IsZero() {
			addTomb(o.UID)
		}
	}
	// A note that is here again (an Undo, an unlocked note) is not deleted.
	tombs = slices.DeleteFunc(tombs, func(t Tombstone) bool {
		return slices.ContainsFunc(out, func(n Note) bool { return n.UID == t.UID && !n.Locked })
	})
	slices.SortFunc(tombs, func(a, b Tombstone) int { return compareStrings(a.UID, b.UID) })
	return out, tombs
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// pending counts what a sync would push: the notes changed since the server
// had them, and the deletions it does not know.
func pending(ns []Note, tombs []Tombstone) int {
	c := len(tombs)
	for _, n := range ns {
		if !n.Locked && !n.Updated.Equal(n.Pushed) {
			c++
		}
	}
	return c
}

// plan is what a sync does.
type plan struct {
	// Apply are the remote notes (and deletions) that replace or make the
	// local ones; Push are the local ones the server must have; Drop are
	// the tombstones that are no longer needed once the plan is done.
	Apply, Push []wireNote
	Drop        []string
	// InSync are the notes the server has as they are here.
	InSync map[string]time.Time
}

// planSync compares the local library with the server's notes.
func planSync(local []Note, tombs []Tombstone, remote []wireNote) plan {
	p := plan{InSync: map[string]time.Time{}}
	byUID := map[string]Note{}
	for _, n := range local {
		byUID[n.UID] = n
	}
	tomb := map[string]Tombstone{}
	for _, t := range tombs {
		tomb[t.UID] = t
	}
	seen := map[string]bool{}
	for _, r := range remote {
		seen[r.UID] = true
		l, haveL := byUID[r.UID]
		t, haveT := tomb[r.UID]
		switch {
		case haveT: // deleted here, or locked here
			p.Drop = append(p.Drop, r.UID)
			switch {
			case r.Deleted:
			case !t.At.Before(r.Updated):
				p.Push = append(p.Push, wireNote{UID: r.UID, Updated: t.At.UTC(), Deleted: true})
			case !haveL: // changed on the server after the deletion here: it comes back
				p.Apply = append(p.Apply, r)
			}
		case !haveL:
			if !r.Deleted {
				p.Apply = append(p.Apply, r)
			}
		case l.Locked: // stays on this device
		default:
			switch c := r.Updated.Compare(l.Updated); {
			case c > 0:
				p.Apply = append(p.Apply, r)
			case c < 0:
				p.Push = append(p.Push, wireOf(l))
			case !r.Deleted:
				p.InSync[l.UID] = l.Updated
			}
		}
	}
	for _, l := range local {
		if !seen[l.UID] && !l.Locked && l.UID != "" {
			p.Push = append(p.Push, wireOf(l)) // new here, or the server lost it
		}
	}
	for _, t := range tombs {
		if !seen[t.UID] {
			p.Drop = append(p.Drop, t.UID) // the server never had it
		}
	}
	return p
}

// syncResult is the outcome of a sync, for applySync.
type syncResult struct {
	// Run numbers the syncs, so an effect applies each result once.
	Run  int
	Name string
	At   time.Time
	// Apply, Pushed (UID to the Updated now on the server) and Drop come
	// from the plan; Base is the Updated each local note had when the plan
	// was made, so a note edited since is left alone.
	Apply  []wireNote
	Pushed map[string]time.Time
	Drop   []string
	Base   map[string]time.Time
}

var syncRuns atomic.Int64

// errUnauthorized is the server refusing the token.
var errUnauthorized = errors.New("the server did not accept the sign-in")

// syncer talks to the server with a client that sends (and refreshes) the
// access token.
type syncer struct {
	base string
	hc   *http.Client
}

func (s syncer) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errUnauthorized
	case resp.StatusCode/100 != 2:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(msg))
	case out == nil:
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

// runSync gets the server's notes, plans against local and pushes what the
// plan says. It runs off the UI thread and gives up when ctx is done.
func runSync(ctx context.Context, s syncer, local snapshot) (syncResult, error) {
	res := syncResult{Run: int(syncRuns.Add(1))}
	var me struct {
		Name string `json:"name"`
	}
	if err := s.do(ctx, http.MethodGet, "/api/me", nil, &me); err != nil {
		return res, err
	}
	res.Name = me.Name
	var remote []wireNote
	if err := s.do(ctx, http.MethodGet, "/api/notes", nil, &remote); err != nil {
		return res, err
	}
	p := planSync(local.Notes, local.Tombs, remote)
	res.Apply, res.Drop = p.Apply, p.Drop
	res.Pushed = p.InSync
	res.Base = map[string]time.Time{}
	for _, n := range local.Notes {
		res.Base[n.UID] = n.Updated
	}
	for _, w := range p.Push {
		var stored wireNote
		if err := s.do(ctx, http.MethodPut, "/api/notes/"+url.PathEscape(w.UID), w, &stored); err != nil {
			return res, err
		}
		if stored.Updated.Equal(w.Updated) {
			res.Pushed[w.UID] = w.Updated
		}
	}
	res.At = now()
	return res, nil
}

// applySync puts the outcome of a sync in the library: the pulled notes
// (unless the note changed since the plan), what the server now has marked
// as pushed, and the tombstones that are done. newID gives the ID of a note
// that comes from the server. It does not Update the notes it replaces from
// the server: they are the server's.
func applySync(st libState, res syncResult, newID func() int) libState {
	notes := slices.Clone(st.Notes)
	index := func(uid string) int {
		return slices.IndexFunc(notes, func(n Note) bool { return n.UID == uid })
	}
	changed := false
	for _, r := range res.Apply {
		i := index(r.UID)
		switch {
		case i >= 0 && !notes[i].Updated.Equal(res.Base[r.UID]):
			// edited since the plan: the next sync compares again
		case i >= 0 && r.Deleted:
			notes = slices.Delete(notes, i, i+1)
			changed = true
		case i >= 0:
			n := notes[i]
			// A note with no file that had no unsaved changes stays that
			// way; one tied to a file now differs from it.
			clean := n.Path == "" && n.Text == n.Saved
			n.Title, n.Text, n.Folder, n.Pinned = r.Title, r.Text, r.Folder, r.Pinned
			if clean {
				n.Saved = r.Text
			}
			n.Modified, n.Updated, n.Pushed = r.Updated, r.Updated, r.Updated
			notes[i] = n
			changed = true
		case !r.Deleted:
			notes = append(notes, Note{ID: newID(), UID: r.UID, Title: r.Title, Text: r.Text, Saved: r.Text, Folder: r.Folder,
				Pinned: r.Pinned, Modified: r.Updated, Updated: r.Updated, Pushed: r.Updated})
			changed = true
		}
	}
	for uid, at := range res.Pushed {
		if i := index(uid); i >= 0 && notes[i].Updated.Equal(at) && !notes[i].Pushed.Equal(at) {
			notes[i].Pushed = at
			changed = true
		}
	}
	tombs := slices.DeleteFunc(slices.Clone(st.Tombs), func(t Tombstone) bool { return slices.Contains(res.Drop, t.UID) })
	if !changed && len(tombs) == len(st.Tombs) {
		return st
	}
	st.Notes, st.Tombs = notes, tombs
	st.Rev++
	return st
}
