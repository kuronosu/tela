//go:build darwin

package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// kqueue watches every directory of the trees, and every regular file in
// them that Filter accepts, with a descriptor opened O_EVTONLY (it does
// not keep a volume from being unmounted) and an EVFILT_VNODE event, as
// fsnotify's kqueue backend does (ADR-259). A directory's NOTE_WRITE (an
// entry added, removed or renamed) scans it again against the entries
// seen before; a file's NOTE_WRITE, NOTE_EXTEND or NOTE_ATTRIB is a write
// in place; NOTE_DELETE, NOTE_RENAME and NOTE_REVOKE close the descriptor
// (the directory reports the name). FSEvents, the other macOS API, would
// need CoreServices through purego and a dispatch queue calling into Go;
// kqueue is plain syscalls.
//
// Quirks handled:
//   - kqueue is not recursive and takes one descriptor per watched file,
//     so only the files Filter accepts get one (the sources of tela dev,
//     a store's file), and the directories. Go raises the soft limit of
//     open files to the hard one at start; EMFILE is reported once, and
//     such a file is then seen only through its directory (created,
//     renamed, removed, replaced by a rename; not written in place).
//   - A save that renames a new file over the old one (most editors, the
//     stores) changes the name's inode: the scan compares inodes too and
//     moves the file's watch to the new one.
//   - The OS reuses descriptor numbers: the descriptors of watches removed
//     while a batch of events is handled are closed after the batch, so
//     a watch the batch opens never takes the number of one whose events
//     are still in it (closing a descriptor drops its pending events).
//   - Close wakes the goroutine blocked in kevent with an EVFILT_USER
//     event, which then closes every descriptor.
type kqueue struct {
	w    *Watcher
	kq   int
	done chan struct{}

	mu      sync.Mutex
	roots   []string
	dirs    map[string]*kdir // by path
	files   map[string]int   // watched files: path -> descriptor
	byFD    map[int]kent
	limited bool // EMFILE was reported
	// closing are the descriptors of watches removed while a batch of
	// events is handled, closed after it (closeRemoved): a descriptor
	// closed at once could be reused by a watch the same batch opens, and
	// a later event of the batch for the old one would be taken for the
	// new one's.
	closing []int
}

// kdir is a watched directory and the entries its last scan saw.
type kdir struct {
	fd      int
	entries map[string]kstamp // by name
}

// kstamp is what a scan compares of an entry.
type kstamp struct {
	dir  bool
	ino  uint64
	size int64
	mod  int64
}

// kent is what a descriptor watches.
type kent struct {
	path string
	dir  bool
}

const (
	kqVnodeFlags = unix.NOTE_DELETE | unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_RENAME | unix.NOTE_REVOKE
	kqGone       = unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE
	kqWake       = 1 // the EVFILT_USER event's identifier
)

func newNative(w *Watcher) (impl, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("watch: kqueue: %w (falling back to polling)", err)
	}
	unix.CloseOnExec(kq)
	wake := []unix.Kevent_t{{Ident: kqWake, Filter: unix.EVFILT_USER, Flags: unix.EV_ADD | unix.EV_CLEAR}}
	if _, err := unix.Kevent(kq, wake, nil, nil); err != nil {
		unix.Close(kq)
		return nil, fmt.Errorf("watch: kqueue EVFILT_USER: %w (falling back to polling)", err)
	}
	k := &kqueue{
		w:     w,
		kq:    kq,
		done:  make(chan struct{}),
		dirs:  map[string]*kdir{},
		files: map[string]int{},
		byFD:  map[int]kent{},
	}
	go k.read()
	return k, nil
}

func (k *kqueue) setRoots(roots []string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	for p := range k.dirs {
		if !under(p, roots) {
			k.unwatchDir(p)
		}
	}
	for p := range k.files {
		if !under(p, roots) {
			k.unwatchFile(p)
		}
	}
	k.roots = roots
	for _, r := range roots {
		k.w.walk(r, k.addDir, nil)
	}
	k.closeRemoved()
	return nil
}

// closeRemoved closes the descriptors of the watches removed. k.mu is
// held.
func (k *kqueue) closeRemoved() {
	for _, fd := range k.closing {
		unix.Close(fd)
	}
	k.closing = k.closing[:0]
}

func (k *kqueue) close() error {
	trigger := []unix.Kevent_t{{Ident: kqWake, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER}}
	if _, err := unix.Kevent(k.kq, trigger, nil, nil); err != nil {
		return err
	}
	<-k.done
	return nil
}

// open returns a descriptor of path watched for vnode events.
func (k *kqueue) open(path string, dir bool) (int, error) {
	fd, err := unix.Open(path, unix.O_EVTONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	var ev unix.Kevent_t
	unix.SetKevent(&ev, fd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_ENABLE|unix.EV_CLEAR)
	ev.Fflags = kqVnodeFlags
	if _, err := unix.Kevent(k.kq, []unix.Kevent_t{ev}, nil, nil); err != nil {
		unix.Close(fd)
		return -1, err
	}
	k.byFD[fd] = kent{path: path, dir: dir}
	return fd, nil
}

// failed reports an error of watching path (vanished entries are not
// errors). k.mu is held.
func (k *kqueue) failed(path string, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, unix.ENOTDIR):
	case errors.Is(err, unix.EMFILE), errors.Is(err, unix.ENFILE):
		if !k.limited {
			k.limited = true
			k.w.error(fmt.Errorf("watch: %s: too many open files for kqueue; files beyond the limit are seen only when created, renamed or removed (raise it with ulimit -n)", path))
		}
	default:
		k.w.error(fmt.Errorf("watch: %s: %w", path, err))
	}
}

// addDir watches the directory path and the files in it that Filter
// accepts (walk's directory callback). k.mu is held.
func (k *kqueue) addDir(path string) error {
	if _, ok := k.dirs[path]; ok {
		return nil
	}
	fd, err := k.open(path, true)
	if err != nil {
		k.failed(path, err)
		return err
	}
	d := &kdir{fd: fd, entries: map[string]kstamp{}}
	k.dirs[path] = d
	// After the watch exists, so an entry added meanwhile is not missed.
	ents, _ := os.ReadDir(path)
	for _, e := range ents {
		st, ok := entryStamp(e)
		if !ok {
			continue
		}
		d.entries[e.Name()] = st
		if !st.dir {
			k.addFile(filepath.Join(path, e.Name()), e)
		}
	}
	return nil
}

// addFile watches the file path when it is a regular file that Filter
// accepts. k.mu is held.
func (k *kqueue) addFile(path string, e fs.DirEntry) {
	if _, ok := k.files[path]; ok || !e.Type().IsRegular() {
		return
	}
	if f := k.w.opts.Filter; f != nil && !f(path, false) {
		return
	}
	fd, err := k.open(path, false)
	if err != nil {
		k.failed(path, err)
		return
	}
	k.files[path] = fd
}

// unwatchFile stops watching the file path. k.mu is held.
func (k *kqueue) unwatchFile(path string) {
	if fd, ok := k.files[path]; ok {
		k.closing = append(k.closing, fd)
		delete(k.byFD, fd)
		delete(k.files, path)
	}
}

// unwatchDir stops watching the directory path (not its subdirectories,
// nor its files). k.mu is held.
func (k *kqueue) unwatchDir(path string) {
	if d, ok := k.dirs[path]; ok {
		k.closing = append(k.closing, d.fd)
		delete(k.byFD, d.fd)
		delete(k.dirs, path)
	}
}

// unwatchTree stops watching the tree at path. k.mu is held.
func (k *kqueue) unwatchTree(path string) {
	for p := range k.dirs {
		if within(p, path) {
			k.unwatchDir(p)
		}
	}
	for p := range k.files {
		if within(p, path) {
			k.unwatchFile(p)
		}
	}
}

func (k *kqueue) read() {
	defer close(k.done)
	evs := make([]unix.Kevent_t, 64)
	for {
		n, err := unix.Kevent(k.kq, nil, evs, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			k.w.error(fmt.Errorf("watch: reading kqueue events: %w", err))
			k.shutdown()
			return
		}
		for _, ev := range evs[:n] {
			if ev.Filter == unix.EVFILT_USER {
				k.shutdown()
				return
			}
			k.event(int(ev.Ident), ev.Fflags)
		}
		k.mu.Lock()
		k.closeRemoved()
		k.mu.Unlock()
	}
}

// shutdown closes every descriptor and the queue.
func (k *kqueue) shutdown() {
	k.mu.Lock()
	defer k.mu.Unlock()
	for fd := range k.byFD {
		unix.Close(fd)
	}
	clear(k.byFD)
	clear(k.dirs)
	clear(k.files)
	k.closeRemoved()
	unix.Close(k.kq)
}

// change is a change to report once k.mu is released.
type change struct {
	path string
	dir  bool
}

func (k *kqueue) event(fd int, fflags uint32) {
	k.mu.Lock()
	ent, ok := k.byFD[fd]
	if !ok {
		k.mu.Unlock()
		return
	}
	var out []change
	switch {
	case !ent.dir:
		if fflags&kqGone != 0 {
			// Removed or renamed away: the directory's scan reports the
			// name, and watches a file that takes it.
			k.unwatchFile(ent.path)
		}
		out = append(out, change{ent.path, false})
	case fflags&kqGone != 0:
		// A directory deleted or moved away: its parent's scan reports
		// it, except for a root, which has none watched.
		isRoot := false
		for _, r := range k.roots {
			isRoot = isRoot || r == ent.path
		}
		if isRoot {
			k.unwatchTree(ent.path)
			out = append(out, change{ent.path, true})
		}
	case fflags&unix.NOTE_WRITE != 0:
		out = k.scan(ent.path)
	}
	k.mu.Unlock()
	for _, c := range out {
		k.w.changed(c.path, c.dir)
	}
}

// scan compares the entries of the watched directory path with its last
// scan and returns the changes. k.mu is held.
func (k *kqueue) scan(path string) []change {
	d, ok := k.dirs[path]
	if !ok {
		return nil
	}
	ents, err := os.ReadDir(path)
	if err != nil && len(ents) == 0 {
		return nil // gone: its own event, or its parent's scan, reports it
	}
	var out []change
	seen := make(map[string]bool, len(ents))
	for _, e := range ents {
		name := e.Name()
		st, ok := entryStamp(e)
		if !ok {
			continue
		}
		seen[name] = true
		p := filepath.Join(path, name)
		old, had := d.entries[name]
		d.entries[name] = st
		if had && old == st {
			continue
		}
		if had && old.dir != st.dir {
			out = append(out, k.removed(p, old.dir)...)
			had = false
		}
		switch {
		case st.dir && !had:
			if k.w.skip(p) {
				continue
			}
			// Watch the new tree first, then report what is already in it.
			k.w.walk(p, k.addDir, func(f string, _ fs.DirEntry) { out = append(out, change{f, false}) })
		case st.dir:
			// A directory's own times: its entries report themselves.
		default:
			if had && old.ino != st.ino {
				k.unwatchFile(p) // replaced by a rename: watch the new file
			}
			k.addFile(p, e)
			out = append(out, change{p, false})
		}
	}
	for name, old := range d.entries {
		if !seen[name] {
			delete(d.entries, name)
			out = append(out, k.removed(filepath.Join(path, name), old.dir)...)
		}
	}
	return out
}

// removed stops watching the entry path, gone, and returns its change.
// k.mu is held.
func (k *kqueue) removed(path string, dir bool) []change {
	if dir {
		k.unwatchTree(path)
		return []change{{path, true}}
	}
	k.unwatchFile(path)
	return []change{{path, false}}
}

// entryStamp returns what a scan compares of e (ok false when it vanished).
func entryStamp(e fs.DirEntry) (kstamp, bool) {
	fi, err := e.Info()
	if err != nil {
		return kstamp{}, false
	}
	st := kstamp{dir: fi.IsDir(), size: fi.Size(), mod: fi.ModTime().UnixNano()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.ino = sys.Ino
	}
	if st.dir {
		st.size, st.mod = 0, 0 // a directory changes with its entries
	}
	return st, true
}
