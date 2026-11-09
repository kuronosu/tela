package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/kuronosu/tela/internal/watch"
)

// Changes other programs make to a store's file (ADR-253).
//
// While a store has watchers (Store.Watch, so every mounted UseSetting),
// its file's folder is watched (package internal/watch: inotify on Linux,
// ReadDirectoryChangesW on Windows, polling elsewhere; only the folder,
// not its subfolders, and only the store's file counts). A change reads
// the file again and merges it: each key takes the file's value, unless
// this process changed the key since its last write (the change not yet
// written wins: it is the later one as far as the app knows, and is
// written within a moment); the watchers of the keys that changed run, so
// UseSetting builds again. A key the file no longer has is removed. A
// file that does not parse is skipped (another program may be writing it
// in place; its next change reads it again). Every write reads the file
// first and merges it the same way, so a write never drops a key another
// program added or changed since the last read. Writes of the store
// itself come back as changes that change nothing.

// watchPoll is how often the file is polled where the OS has no
// notifications (macOS, ADR-115's watcher): settings change rarely.
const watchPoll = time.Second

// fileWatch is the watch of a store's file.
type fileWatch struct {
	w *watch.Watcher
}

// startWatch watches the file while the store has watchers, once its
// folder exists (a store whose file was never written has none: the first
// write starts it). Called with s.mu held.
func (s *Store) startWatch() {
	if s.fw != nil || s.path == "" || len(s.watchers) == 0 {
		return
	}
	dir := filepath.Dir(s.path)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return
	}
	path := s.path
	w, err := watch.New(watch.Options{
		Filter:       func(p string, _ bool) bool { return p == path },
		SkipDir:      func(string) bool { return true },
		PollInterval: watchPoll,
	}, dir)
	if err != nil {
		return // the store still works; changes of other programs are not seen
	}
	s.fw = &fileWatch{w: w}
	go func() {
		for range w.Events() {
			// Under the write lock: a file read just before a write of
			// this store, applied after it, would bring back the value
			// the write replaced.
			s.wmu.Lock()
			notify := s.reload()
			s.wmu.Unlock()
			notify()
		}
	}()
}

// stopWatch stops the watch when the last watcher stopped. Called with
// s.mu held.
func (s *Store) stopWatch() {
	if s.fw == nil || len(s.watchers) > 0 {
		return
	}
	_ = s.fw.w.Close()
	s.fw = nil
}

// reload reads the file again and merges it (see the top of the file),
// and returns the function that runs the watchers of the keys that
// changed. Called with s.wmu held, so no write of the store runs between
// the read and the merge; the caller runs the watchers after releasing
// it (they may write the store).
func (s *Store) reload() (notify func()) {
	notify = func() {}
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	var vals map[string]json.RawMessage
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Removed: every key not changed here goes.
	case err != nil:
		return
	default:
		if vals, err = parseStore(data); err != nil {
			return
		}
	}
	s.mu.Lock()
	if s.unread || s.corrupt {
		// The file was never read (retryLoad merges it at the next write)
		// or did not parse (it is kept aside at the next write).
		s.mu.Unlock()
		return
	}
	var changed []string
	for k, v := range vals {
		if s.pending[k] {
			continue
		}
		if old, ok := s.vals[k]; !ok || !bytes.Equal(old, v) {
			s.vals[k] = v
			changed = append(changed, k)
		}
	}
	for k := range s.vals {
		if _, ok := vals[k]; !ok && !s.pending[k] {
			delete(s.vals, k)
			changed = append(changed, k)
		}
	}
	ws := s.watchersOf(changed)
	s.mu.Unlock()
	return func() {
		for i, k := range changed {
			for _, w := range ws[i] {
				if !w.stopped.Load() {
					w.f(k)
				}
			}
		}
	}
}

// watchersOf returns, for each key, the watchers to run. Called with s.mu
// held.
func (s *Store) watchersOf(keys []string) [][]*storeWatcher {
	out := make([][]*storeWatcher, len(keys))
	for i, k := range keys {
		for _, w := range s.watchers {
			if w.key == "" || w.key == k {
				out[i] = append(out[i], w)
			}
		}
	}
	return out
}
