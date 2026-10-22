//go:build windows

package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// rdcw watches each root with ReadDirectoryChangesW and its subtree flag,
// one directory handle and goroutine per root, with overlapped I/O so that
// close can stop a pending read (an event per root signals the stop and
// CancelIoEx aborts the read, as fsnotify does).
//
// Quirks handled:
//   - A read that returns zero bytes, or ERROR_NOTIFY_ENUM_DIR, means the
//     buffer overflowed: reported as Batch.Overflow.
//   - The subtree is watched natively, so skipped directories are filtered
//     by path: every directory between the root and the file is checked
//     with SkipDir.
//   - A directory created or renamed into the tree reports only its own
//     name, so it is scanned for files.
//   - A removed name cannot be inspected any more, and the records do not
//     say whether it was a directory (inotify's IN_ISDIR has no
//     equivalent here). Each root keeps the set of directories in its
//     tree, and a removed or renamed-away name is reported as a directory
//     (dir = true) only when it is in the set. The set is walked once the
//     first read is pending and before open returns (so a directory made
//     during the walk is walked or reported, and one removed after
//     SetRoots returns is known), again after an overflow, and updated by
//     the records. Guessing from the name instead (no extension) reported
//     vim's "4913" write probe, and any other extensionless file removed,
//     as a directory that may have held packages, which a filter must
//     accept. Keys are lowercased: NTFS names are case-insensitive.
//   - Records may carry a name's 8.3 short form (MYPACK~1) instead of the
//     long one ("it is unspecified which one", says the documentation),
//     which the set, made of long names, never matches: a removed name
//     not in the set that looks like a short name is reported both ways,
//     as the old guess did.
//   - The walks run on the root's goroutine and stop when the root or the
//     watcher is closed, so Close does not wait for a large tree.
//   - Names are relative to the root, in UTF-16; the handle is opened with
//     FILE_FLAG_BACKUP_SEMANTICS, which a directory handle requires, and
//     shares delete access so the watched trees can still be renamed.
type rdcw struct {
	w *Watcher

	closing atomic.Bool // close was called: walks stop

	mu    sync.Mutex
	roots []*rootWatch
}

type rootWatch struct {
	path string
	h    windows.Handle
	stop windows.Handle
	done chan struct{}

	stopping atomic.Bool // close was called: walks stop

	// The directories under path (lowercased, root excluded, skipped
	// directories excluded), and for each directory key the keys of the
	// directories in it, so that removing a subtree visits only it. Only
	// the root's goroutine touches them (open waits for its first walk).
	dirs     map[string]struct{}
	children map[string]map[string]struct{}
}

func dirKey(path string) string { return strings.ToLower(path) }

// addDir adds a directory (a key) to the set.
func (r *rootWatch) addDir(k string) {
	if _, ok := r.dirs[k]; ok {
		return
	}
	r.dirs[k] = struct{}{}
	parent := filepath.Dir(k)
	c := r.children[parent]
	if c == nil {
		c = map[string]struct{}{}
		r.children[parent] = c
	}
	c[k] = struct{}{}
}

// removeDirs removes dir and the directories under it from the set and
// reports whether dir was in it. It visits only the removed subtree.
func (r *rootWatch) removeDirs(dir string) bool {
	k := dirKey(dir)
	if _, ok := r.dirs[k]; !ok {
		return false
	}
	if c := r.children[filepath.Dir(k)]; c != nil {
		delete(c, k)
		if len(c) == 0 {
			delete(r.children, filepath.Dir(k))
		}
	}
	stack := []string{k}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		delete(r.dirs, x)
		for c := range r.children[x] {
			stack = append(stack, c)
		}
		delete(r.children, x)
	}
	return true
}

// shortName reports whether base looks like an NTFS 8.3 short name: at
// most eight characters, a "~" followed by digits at the end of the name
// part, and an extension of at most three characters.
func shortName(base string) bool {
	name, ext, _ := strings.Cut(base, ".")
	if len(name) > 8 || len(ext) > 3 || strings.Contains(ext, ".") {
		return false
	}
	i := strings.LastIndexByte(name, '~')
	if i < 1 || i == len(name)-1 {
		return false
	}
	for _, c := range name[i+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// stopped reports whether walks of r should stop.
func (d *rdcw) stopped(r *rootWatch) bool { return r.stopping.Load() || d.closing.Load() }

// walkTree walks dir, a directory of r's tree, adding the directories
// that are not skipped to the set and calling file, if not nil, for every
// other entry. It stops when r or the watcher is closed. Errors of
// vanished entries are ignored.
func (d *rdcw) walkTree(r *rootWatch, dir string, file func(path string)) {
	filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if d.stopped(r) {
			return filepath.SkipAll
		}
		if err != nil {
			if path == dir || errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			d.w.error(err)
			if e != nil && e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			if path != r.path {
				if d.w.skip(path) {
					return filepath.SkipDir
				}
				r.addDir(dirKey(path))
			}
			return nil
		}
		if file != nil {
			file(path)
		}
		return nil
	})
}

// scan rebuilds the set of directories of the tree.
func (d *rdcw) scan(r *rootWatch) {
	r.dirs = map[string]struct{}{}
	r.children = map[string]map[string]struct{}{}
	d.walkTree(r, r.path, nil)
}

const rdcwMask = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE |
	windows.FILE_NOTIFY_CHANGE_CREATION | windows.FILE_NOTIFY_CHANGE_ATTRIBUTES

func newNative(w *Watcher) (impl, error) { return &rdcw{w: w}, nil }

// setRoots keeps the watches of roots that stay, closes the others and
// opens the new ones.
func (d *rdcw) setRoots(roots []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	kept := map[string]*rootWatch{}
	for _, r := range d.roots {
		if slices.Contains(roots, r.path) {
			kept[r.path] = r
		} else {
			r.close()
		}
	}
	d.roots = nil
	for _, path := range roots {
		if r := kept[path]; r != nil {
			d.roots = append(d.roots, r)
			continue
		}
		if d.closing.Load() {
			break
		}
		r, err := d.open(path)
		if err != nil {
			d.w.error(err)
			continue
		}
		d.roots = append(d.roots, r)
	}
	return nil
}

func (d *rdcw) close() error {
	d.closing.Store(true) // stops a walk that holds d.mu (open)
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.roots {
		r.close()
	}
	d.roots = nil
	return nil
}

func (d *rdcw) open(path string) (*rootWatch, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("watch: opening %s: %w", path, err)
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("watch: %w", err)
	}
	r := &rootWatch{path: path, h: h, stop: stop, done: make(chan struct{})}
	ready := make(chan error, 1)
	go d.loop(r, ready)
	if err := <-ready; err != nil {
		<-r.done
		windows.CloseHandle(h)
		windows.CloseHandle(stop)
		return nil, fmt.Errorf("watch: %s: %w", path, err)
	}
	return r, nil
}

func (r *rootWatch) close() {
	r.stopping.Store(true)
	windows.SetEvent(r.stop)
	<-r.done
	windows.CloseHandle(r.h)
	windows.CloseHandle(r.stop)
}

// loop reads the changes of one root. ready receives the result of the
// first read request once the tree's directories are known, so that
// changes made after open returns are seen and removed directories are
// recognized.
func (d *rdcw) loop(r *rootWatch, ready chan<- error) {
	defer close(r.done)
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		ready <- err
		return
	}
	defer windows.CloseHandle(ev)
	// DWORD-aligned, as the API requires (a []uint64 is 8-aligned).
	words := make([]uint64, 64<<10/8)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	first, rescan := true, true
	for {
		ov := windows.Overlapped{HEvent: ev}
		windows.ResetEvent(ev)
		err := windows.ReadDirectoryChanges(r.h, &buf[0], uint32(len(buf)), true, rdcwMask, nil, &ov, 0)
		if err != nil {
			if first {
				ready <- err
			} else if !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
				d.w.error(fmt.Errorf("watch: %s: %w", r.path, err))
			}
			return
		}
		if rescan {
			// With the read pending, so a directory created during the
			// walk is either walked or reported (or both).
			rescan = false
			d.scan(r)
		}
		if first {
			first = false
			ready <- nil
		}
		which, err := windows.WaitForMultipleObjects([]windows.Handle{ev, r.stop}, false, windows.INFINITE)
		if err != nil || which != windows.WAIT_OBJECT_0 {
			windows.CancelIoEx(r.h, &ov)
			var n uint32
			windows.GetOverlappedResult(r.h, &ov, &n, true)
			return
		}
		var n uint32
		if err := windows.GetOverlappedResult(r.h, &ov, &n, false); err != nil {
			if errors.Is(err, windows.ERROR_NOTIFY_ENUM_DIR) {
				d.w.lost()
				rescan = true
				continue
			}
			if !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
				d.w.error(fmt.Errorf("watch: %s: %w", r.path, err))
			}
			return
		}
		if n == 0 {
			d.w.lost()
			rescan = true
			continue
		}
		d.parse(r, buf[:n])
	}
}

// parse reports the FILE_NOTIFY_INFORMATION records of buf.
func (d *rdcw) parse(r *rootWatch, buf []byte) {
	for off := uint32(0); int(off)+int(unsafe.Sizeof(windows.FileNotifyInformation{})) <= len(buf); {
		info := (*windows.FileNotifyInformation)(unsafe.Pointer(&buf[off]))
		name := unsafe.Slice(&info.FileName, info.FileNameLength/2)
		rel := windows.UTF16ToString(name)
		d.event(r, filepath.Join(r.path, rel), info.Action)
		if info.NextEntryOffset == 0 {
			return
		}
		off += info.NextEntryOffset
	}
}

func (d *rdcw) event(r *rootWatch, path string, action uint32) {
	// Skipped directories: check every directory between root and path.
	for dir := filepath.Dir(path); within(dir, r.path) && dir != r.path; dir = filepath.Dir(dir) {
		if d.w.skip(dir) {
			return
		}
	}
	switch action {
	case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_RENAMED_NEW_NAME:
		if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
			if !d.w.skip(path) {
				// Its subtree is watched already: report what is in it.
				d.walkTree(r, path, func(p string) { d.w.changed(p, false) })
			}
			return
		}
		d.w.changed(path, false)
	case windows.FILE_ACTION_REMOVED, windows.FILE_ACTION_RENAMED_OLD_NAME:
		dir := r.removeDirs(path)
		if !dir && shortName(filepath.Base(path)) {
			// Perhaps a known directory under its short name.
			d.w.changed(path, false)
			dir = true
		}
		d.w.changed(path, dir)
	case windows.FILE_ACTION_MODIFIED:
		if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
			return // a directory's timestamps; its entries report themselves
		}
		d.w.changed(path, false)
	}
}
