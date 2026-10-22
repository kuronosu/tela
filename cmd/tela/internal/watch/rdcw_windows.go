//go:build windows

package watch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
//     name, so it is scanned for files. A removed name cannot be inspected
//     any more; one without an extension is also reported as a possible
//     directory (dir = true).
//   - Names are relative to the root, in UTF-16; the handle is opened with
//     FILE_FLAG_BACKUP_SEMANTICS, which a directory handle requires, and
//     shares delete access so the watched trees can still be renamed.
type rdcw struct {
	w *Watcher

	mu    sync.Mutex
	roots []*rootWatch
}

type rootWatch struct {
	path string
	h    windows.Handle
	stop windows.Handle
	done chan struct{}
}

const rdcwMask = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE |
	windows.FILE_NOTIFY_CHANGE_CREATION | windows.FILE_NOTIFY_CHANGE_ATTRIBUTES

func newNative(w *Watcher) (impl, error) { return &rdcw{w: w}, nil }

func (d *rdcw) setRoots(roots []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.roots {
		r.close()
	}
	d.roots = nil
	for _, path := range roots {
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
	windows.SetEvent(r.stop)
	<-r.done
	windows.CloseHandle(r.h)
	windows.CloseHandle(r.stop)
}

// loop reads the changes of one root. ready receives the result of the
// first read request, so that changes made after open returns are seen.
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
	first := true
	for {
		ov := windows.Overlapped{HEvent: ev}
		windows.ResetEvent(ev)
		err := windows.ReadDirectoryChanges(r.h, &buf[0], uint32(len(buf)), true, rdcwMask, nil, &ov, 0)
		if first {
			first = false
			ready <- err
		}
		if err != nil {
			if !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
				d.w.error(fmt.Errorf("watch: %s: %w", r.path, err))
			}
			return
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
				continue
			}
			if !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
				d.w.error(fmt.Errorf("watch: %s: %w", r.path, err))
			}
			return
		}
		if n == 0 {
			d.w.lost()
			continue
		}
		d.parse(r.path, buf[:n])
	}
}

// parse reports the FILE_NOTIFY_INFORMATION records of buf.
func (d *rdcw) parse(root string, buf []byte) {
	for off := uint32(0); int(off)+int(unsafe.Sizeof(windows.FileNotifyInformation{})) <= len(buf); {
		info := (*windows.FileNotifyInformation)(unsafe.Pointer(&buf[off]))
		name := unsafe.Slice(&info.FileName, info.FileNameLength/2)
		rel := windows.UTF16ToString(name)
		d.event(root, filepath.Join(root, rel), info.Action)
		if info.NextEntryOffset == 0 {
			return
		}
		off += info.NextEntryOffset
	}
}

func (d *rdcw) event(root, path string, action uint32) {
	// Skipped directories: check every directory between root and path.
	for dir := filepath.Dir(path); within(dir, root) && dir != root; dir = filepath.Dir(dir) {
		if d.w.skip(dir) {
			return
		}
	}
	switch action {
	case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_RENAMED_NEW_NAME:
		if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
			if !d.w.skip(path) {
				// Its subtree is watched already: report what is in it.
				d.w.walk(path, nil, func(p string, _ os.DirEntry) { d.w.changed(p, false) })
			}
			return
		}
		d.w.changed(path, false)
	case windows.FILE_ACTION_REMOVED, windows.FILE_ACTION_RENAMED_OLD_NAME:
		d.w.changed(path, false)
		if filepath.Ext(path) == "" && !strings.HasSuffix(path, ".") {
			d.w.changed(path, true)
		}
	case windows.FILE_ACTION_MODIFIED:
		if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
			return // a directory's timestamps; its entries report themselves
		}
		d.w.changed(path, false)
	}
}
