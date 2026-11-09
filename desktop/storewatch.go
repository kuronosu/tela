package desktop

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
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
// watched through its nearest existing ancestor, ADR-259, alone too, for
// the next folder of the way to appear). The watch is
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
// program added or changed since the last read. A version of the file is
// its content (a SHA-256 of it): a write of another program that keeps
// the size, the modification time and the inode is a change too (ADR-262).
// The write's replace never drops a version it did not read (store.go,
// swapIn): on Linux and macOS it exchanges the new file with the old one
// atomically (renameat2 RENAME_EXCHANGE, renamex_np RENAME_SWAP; a file
// that did not exist is created with RENAME_NOREPLACE, RENAME_EXCL), reads
// the version it replaced and, when another program wrote it after the
// read, merges it and writes again. Where the file system cannot exchange,
// and on Windows, which has no atomic exchange (ReplaceFileW is a sequence
// of renames too), the write compares the file with what it read right
// before the rename and merges it again when it changed (ADR-259), so a
// write in the moment between that check and the rename can still be
// lost. Writes of the store itself come back as changes that change
// nothing.

// watchPoll is how often the file is polled where the OS has no
// notifications (other Unix systems; macOS too if kqueue fails):
// settings change rarely.
const watchPoll = time.Second

// watchGrace is how long the watch outlives the last watcher: a screen
// swap that unmounts and mounts the components using a setting keeps the
// watch instead of closing and opening it again.
var watchGrace = time.Second

// newWatch, when set, replaces watch.New (tests). It is atomic: the watch
// of a store an earlier test left open is opened on a goroutine of its
// own, which may read it while a later test sets it (the race detector
// caught that as a plain variable).
var newWatch atomic.Pointer[func(watch.Options, ...string) (*watch.Watcher, error)]

// openFolderWatch is watch.New, or the tests' newWatch.
func openFolderWatch(opts watch.Options, roots ...string) (*watch.Watcher, error) {
	if f := newWatch.Load(); f != nil {
		return (*f)(opts, roots...)
	}
	return watch.New(opts, roots...)
}

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
// take a while), without s.mu. Every watch is flat (the folder's entries
// only, not its subtree: on Windows a recursive watch of an ancestor
// would see every change under all of %APPDATA%). A store whose folder
// does not exist yet watches its nearest existing ancestor (watchRoot) for
// the next folder on the way to appear, and moves to that one, until it
// watches the folder. Once the watch is open, and after every move, the
// file is read again, so a change another program made between Watch and
// then is not missed.
func (s *Store) openWatch() {
	path := s.path
	dir := filepath.Dir(path)
	root := watchRoot(dir)
	var w *watch.Watcher
	if root != "" {
		// An error leaves w nil: the store still works; changes of other
		// programs are not seen.
		w, _ = openFolderWatch(watch.Options{
			// The file, or a folder on the way to it (an ancestor of dir,
			// or dir) appearing in the ancestor watched.
			Filter:       func(p string, _ bool) bool { return p == path || watch.Within(dir, p) },
			Flat:         true,
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
			// Down the folders that appeared, rechecked after each move:
			// one may appear before its parent's watch is set (mkdir -p),
			// and then reports nothing; the reload below reads a file
			// written meanwhile.
			for root != dir {
				next := watchRoot(dir)
				if next == "" || next == root || !watch.Within(next, root) {
					break
				}
				root = next
				_ = w.SetRoots([]string{root})
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
	s.read = fileStamp{}
	if s.path == "" {
		return func() {}
	}
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Removed: every key not changed here goes.
		return s.merge(nil, fileStamp{ok: true})
	case err != nil:
		return func() {}
	}
	return s.merge(data, stampOf(data))
}

// merge merges data, a version of the file another program wrote (nil:
// there is none), into the values (see the top of the file), records
// stamp as the version of the file the next write may replace (data's,
// or for a version a write replaced, the write's own), and returns the
// function that runs the watchers of the keys that changed. Called with
// s.wmu held. Data that does not parse merges nothing, and then only the
// stamp of a write's own file is recorded (another program may be
// writing in place; its next change reads it again).
func (s *Store) merge(data []byte, stamp fileStamp) (notify func()) {
	notify = func() {}
	vals, err := parseStore(data)
	if err != nil {
		if stamp.own {
			s.read = stamp
		}
		return
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

// fileStamp identifies a version of a store's file by its content: what
// reload merged last, which a write compares with the version it replaces
// (ADR-262). A write of another program that keeps the size, the
// modification time and the inode is a change too, which a stat would not
// see.
type fileStamp struct {
	ok     bool // taken by a reload that merged the file, or by a write
	exists bool // false: there was no file
	sum    [sha256.Size]byte
	// own: the version a write of this store put in place.
	own bool
}

// stampOf returns the stamp of a file whose content is data.
func stampOf(data []byte) fileStamp {
	return fileStamp{ok: true, exists: true, sum: sha256.Sum256(data)}
}

// same reports whether a and b are the same version of the file: both
// absent, or the same content.
func (a fileStamp) same(b fileStamp) bool {
	return a.exists == b.exists && (!a.exists || a.sum == b.sum)
}

// changedSince reports whether the file at path is no longer the version
// of stamp (a stamp not taken never counts as changed): it reads the file
// (a store's file is small) and compares its content.
func changedSince(path string, stamp fileStamp) bool {
	if !stamp.ok {
		return false
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return stamp.exists
	case err != nil:
		return false // the rename reports what is wrong
	}
	return !stamp.same(stampOf(data))
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
