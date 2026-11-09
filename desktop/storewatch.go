package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/watch"
)

// Changes other programs make to a store's file (ADR-253).
//
// While a store has watchers (Store.Watch, so every mounted UseSetting),
// and for watchGrace after the last one stops, its file's folder is
// watched (package internal/watch: inotify on Linux, ReadDirectoryChangesW
// on Windows, kqueue on macOS; only the folder, not its subfolders, and
// only the store's file counts; a folder that does not exist yet is
// watched through its nearest existing ancestor, ADR-259). The watch is
// opened on a goroutine of its own, never on the caller's (the UI thread
// for UseSetting). A change reads
// the file again and merges it: each key takes the file's value, unless
// this process changed the key since its last write (the change not yet
// written wins: it is the later one as far as the app knows, and is
// written within a moment); the watchers of the keys that changed run, so
// UseSetting builds again. A key the file no longer has is removed. A
// file that does not parse is skipped (another program may be writing it
// in place; its next change reads it again). Every write reads the file
// first and merges it the same way, so a write never drops a key another
// program added or changed since the last read; right before the rename
// that replaces the file, the write compares the file with what it read
// (size, modification time, identity) and, when another program wrote it
// meanwhile, reads and merges it again (ADR-259), so only a write in the
// moment between that check and the rename can still be lost. Writes of
// the store itself come back as changes that change nothing.

// watchPoll is how often the file is polled where the OS has no
// notifications (other Unix systems; macOS too if kqueue fails):
// settings change rarely.
const watchPoll = time.Second

// watchGrace is how long the watch outlives the last watcher: a screen
// swap that unmounts and mounts the components using a setting keeps the
// watch instead of closing and opening it again.
var watchGrace = time.Second

// newWatch opens the watch of a folder (a variable for the tests).
var newWatch = watch.New

// fileWatch is the watch of a store's file.
type fileWatch struct {
	w *watch.Watcher
}

// claimWatch reports whether the caller is to open the watch of the file
// with openWatch, after releasing s.mu: the store has watchers and no
// watch is open or being opened. A stop pending for the grace period is
// called off. Called with s.mu held.
func (s *Store) claimWatch() bool {
	if len(s.watchers) == 0 {
		return false
	}
	if s.stopT != nil {
		s.stopT.Stop()
		s.stopT = nil
	}
	if s.fw != nil || s.opening || s.path == "" {
		return false
	}
	s.opening = true
	return true
}

// openWatch watches the file. It runs on a goroutine of its own
// (claimWatch's caller starts it: Watch runs on the UI thread for
// UseSetting, and the stat, the inotify setup or the first scan of a poll
// take a while), without s.mu. A store whose folder does not exist yet
// watches its nearest existing ancestor, with only the folders on the way
// to the file (watchRoot), and moves to the folder once it appears. Once
// the watch is open the file is read again, so a change another program
// made between Watch and then is not missed.
func (s *Store) openWatch() {
	path := s.path
	dir := filepath.Dir(path)
	root := watchRoot(dir)
	var w *watch.Watcher
	if root != "" {
		// An error leaves w nil: the store still works; changes of other
		// programs are not seen.
		w, _ = newWatch(watch.Options{
			Filter: func(p string, _ bool) bool { return p == path },
			// Only the folders between the root and the store's folder
			// (the root is an ancestor while the folder does not exist).
			SkipDir:      func(p string) bool { return !within(dir, p) },
			PollInterval: watchPoll,
		}, root)
	}
	s.mu.Lock()
	s.opening = false
	if w == nil || len(s.watchers) == 0 {
		// No folder at all, or every watcher stopped meanwhile.
		s.mu.Unlock()
		if w != nil {
			_ = w.Close()
		}
		return
	}
	s.fw = &fileWatch{w: w}
	s.mu.Unlock()
	go func() {
		s.wmu.Lock()
		notify := s.reload()
		s.wmu.Unlock()
		notify()
		for range w.Events() {
			if root != dir {
				if st, err := os.Stat(dir); err == nil && st.IsDir() {
					// The folder exists now: watch it alone.
					root = dir
					_ = w.SetRoots([]string{dir})
				}
			}
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

// watchRoot returns the folder to watch for a store in dir: dir itself,
// else its nearest ancestor that exists ("" when none does).
func watchRoot(dir string) string {
	for {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// within reports whether path is root or inside it (both clean and
// absolute).
func within(path, root string) bool {
	if path == root {
		return true
	}
	if !strings.HasPrefix(path, root) {
		return false
	}
	return os.IsPathSeparator(path[len(root)]) || os.IsPathSeparator(root[len(root)-1])
}

// stopWatch stops the watch watchGrace after the last watcher stopped,
// unless one comes back meanwhile (claimWatch). Called with s.mu held.
func (s *Store) stopWatch() {
	if s.fw == nil || len(s.watchers) > 0 || s.stopT != nil {
		return
	}
	var t *time.Timer
	t = time.AfterFunc(watchGrace, func() {
		s.mu.Lock()
		if s.stopT != t || len(s.watchers) > 0 {
			s.mu.Unlock()
			return
		}
		fw := s.fw
		s.fw, s.stopT = nil, nil
		s.mu.Unlock()
		if fw != nil {
			_ = fw.w.Close()
		}
	})
	s.stopT = t
}

// reload reads the file again and merges it (see the top of the file),
// and returns the function that runs the watchers of the keys that
// changed. Called with s.wmu held, so no write of the store runs between
// the read and the merge; the caller runs the watchers after releasing
// it (they may write the store).
func (s *Store) reload() (notify func()) {
	notify = func() {}
	s.read = fileStamp{}
	if s.path == "" {
		return
	}
	// The stamp before the read: a change between the two makes the
	// write's check read the file once more, never miss it.
	stamp, err := statStamp(s.path)
	if err != nil {
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
	s.read = stamp
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

// fileStamp identifies a version of a store's file: what reload merged
// last, which a write compares with the file right before its rename.
type fileStamp struct {
	ok bool        // taken by a reload that merged the file
	fi os.FileInfo // nil: there was no file
}

// statStamp returns the stamp of the file at path now (an absent file is
// a stamp too; other errors are returned).
func statStamp(path string) (fileStamp, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileStamp{ok: true}, nil
	}
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{ok: true, fi: fi}, nil
}

// same reports whether a and b are the same version of the file: both
// absent, or the same file (inode, file ID) with the same size and
// modification time. A write of another program that keeps all three
// (within the file system's timestamp resolution) is not seen.
func (a fileStamp) same(b fileStamp) bool {
	if a.fi == nil || b.fi == nil {
		return a.fi == nil && b.fi == nil
	}
	return a.fi.Size() == b.fi.Size() && a.fi.ModTime().Equal(b.fi.ModTime()) && os.SameFile(a.fi, b.fi)
}

// changedSince reports whether the file at path is no longer the version
// of stamp (a stamp not taken never counts as changed).
func changedSince(path string, stamp fileStamp) bool {
	if !stamp.ok {
		return false
	}
	now, err := statStamp(path)
	return err == nil && !stamp.same(now)
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
