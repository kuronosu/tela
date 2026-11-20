package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
)

// library is what opening the notes gives the screen: the store, and what
// was in it.
type library struct {
	store *store
	data  loaded
}

// freshLibrary is the library of a first run, which needs nothing from the
// disk to show: nothing is there yet (and in tela snapshot, where the folders
// are the developer's, it is one in memory). It is nil when there is
// something to read, which loadLibrary does off the UI thread. It looks at
// the folder, which is all the UI thread does of it.
func freshLibrary(preview bool) *library {
	if preview {
		return &library{store: memoryStore(), data: loaded{Fresh: true}}
	}
	dir, err := desktop.DataDir()
	if err != nil {
		return nil // loadLibrary falls back to a throwaway folder
	}
	for _, name := range []string{dbName, legacyName} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, fs.ErrNotExist) {
			return nil
		}
	}
	st, err := lazyStore(dir)
	if err != nil {
		return nil
	}
	return &library{store: st, data: loaded{Fresh: true}}
}

// loadLibrary opens the database in the app's data folder and reads it (an
// older notes.json is imported once). It runs off the UI thread, in
// tela.UseAsync. A database that is not one is set aside as notes.db.corrupt
// and a new one starts, so nothing the user had is lost. In tela snapshot,
// where there is no window and the folders are the developer's, the library
// is one in memory.
func loadLibrary(preview bool) (library, error) {
	if preview {
		st := memoryStore()
		return library{store: st, data: loaded{Fresh: true}}, nil
	}
	dir, err := desktop.DataDir()
	if err != nil {
		// No data folder: a throwaway one, so the app still works.
		dir = filepath.Join(os.TempDir(), "tela-notes-"+newUID())
	}
	var corrupt bool
	for attempt := 0; ; attempt++ {
		st, err := openStore(dir)
		var data loaded
		if err == nil {
			if data, err = st.Load(); err != nil {
				st.Close()
			}
		}
		if err == nil {
			data.Corrupt = data.Corrupt || corrupt
			return library{store: st, data: data}, nil
		}
		if attempt > 0 || !isCorruption(err) {
			return library{}, err
		}
		corrupt = true
		if err := setAside(dir); err != nil {
			return library{}, err
		}
	}
}

// withSyncFields gives notes that never had them (the welcome notes) a UID
// and an Updated time.
func withSyncFields(ns []Note, at time.Time) []Note {
	out := slices.Clone(ns)
	for i := range out {
		if out[i].UID == "" {
			out[i].UID = newUID()
		}
		if out[i].Updated.IsZero() {
			out[i].Updated = at
		}
	}
	return out
}

// notesSetter changes the library the way UseState's setter does, for the
// edits of the user: Update takes a function from the notes to the notes,
// and the sync fields follow (track). Rev counts every change, which the
// store follows; Edits counts the ones the server has to learn of.
type notesSetter struct{ set tela.Setter[libState] }

func (s notesSetter) Update(f func([]Note) []Note) {
	s.set.Update(func(st libState) libState {
		notes, tombs := track(st.Notes, f(st.Notes), st.Tombs, now())
		st.Notes, st.Tombs, st.Rev = notes, tombs, st.Rev+1
		if pending(notes, tombs) > 0 {
			st.Edits++
		}
		return st
	})
}

// Apply puts the outcome of a sync in the library (applySync): not an edit,
// so no sync follows it.
func (s notesSetter) Apply(res syncResult, newID func() int) {
	s.set.Update(func(st libState) libState { return applySync(st, res, newID) })
}
