//go:build linux

package watch

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// inotify watches every directory of the trees (inotify is not recursive:
// a directory that appears gets its watches, and its files are reported,
// since they may have been created before the watch existed). The file
// descriptor is non-blocking and wrapped in an os.File, so reads park in
// the runtime poller and Close unblocks them (the approach of fsnotify).
//
// Quirks handled:
//   - Watches follow inodes, not names: a directory moved within a tree
//     keeps watches registered under its old path, so a move removes the
//     watches of the old subtree and adds the new one.
//   - IN_Q_OVERFLOW (the queue limit, fs.inotify.max_queued_events) is
//     reported as Batch.Overflow and the trees are walked again for
//     directories created meanwhile.
//   - ENOSPC from inotify_add_watch is the per-user watch limit
//     (fs.inotify.max_user_watches); it is reported once with the fix.
type inotify struct {
	w  *Watcher
	fd int
	f  *os.File

	mu      sync.Mutex
	byWD    map[int32]string
	byPath  map[string]int32
	roots   []string
	limited bool // ENOSPC was reported
}

const inotifyMask = unix.IN_CREATE | unix.IN_CLOSE_WRITE | unix.IN_MODIFY | unix.IN_ATTRIB |
	unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_DELETE | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF |
	unix.IN_ONLYDIR | unix.IN_EXCL_UNLINK

func newNative(w *Watcher) (impl, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("watch: inotify_init1: %w (falling back to polling)", err)
	}
	in := &inotify{
		w:      w,
		fd:     fd,
		f:      os.NewFile(uintptr(fd), "inotify"),
		byWD:   map[int32]string{},
		byPath: map[string]int32{},
	}
	go in.read()
	return in, nil
}

func (in *inotify) setRoots(roots []string) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	for wd := range in.byWD {
		unix.InotifyRmWatch(in.fd, uint32(wd))
	}
	clear(in.byWD)
	clear(in.byPath)
	in.roots = roots
	for _, r := range roots {
		in.w.walk(r, in.add, nil)
	}
	return nil
}

func (in *inotify) close() error { return in.f.Close() }

// add watches the directory path. in.mu is held.
func (in *inotify) add(path string) error {
	if _, ok := in.byPath[path]; ok {
		return nil
	}
	wd, err := unix.InotifyAddWatch(in.fd, path, inotifyMask)
	if err != nil {
		switch {
		case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENOTDIR):
			// Gone before we got to it.
		case errors.Is(err, unix.ENOSPC):
			if !in.limited {
				in.limited = true
				in.w.error(fmt.Errorf("watch: %s: the inotify watch limit is reached; raise it with sudo sysctl fs.inotify.max_user_watches=524288", path))
			}
		default:
			in.w.error(fmt.Errorf("watch: %s: %w", path, err))
		}
		return err
	}
	if old, ok := in.byWD[int32(wd)]; ok && old != path {
		// The same inode under another name (a bind mount, or a directory
		// moved before we processed the move): keep the newer name.
		delete(in.byPath, old)
	}
	in.byWD[int32(wd)] = path
	in.byPath[path] = int32(wd)
	return nil
}

// remove drops the watches of the tree at path. in.mu is held.
func (in *inotify) remove(path string) {
	for p, wd := range in.byPath {
		if within(p, path) {
			unix.InotifyRmWatch(in.fd, uint32(wd))
			delete(in.byPath, p)
			delete(in.byWD, wd)
		}
	}
}

func (in *inotify) read() {
	buf := make([]byte, 64<<10)
	for {
		n, err := in.f.Read(buf)
		if err != nil {
			if !errors.Is(err, os.ErrClosed) {
				in.w.error(fmt.Errorf("watch: reading inotify events: %w", err))
			}
			return
		}
		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
			name := buf[off+unix.SizeofInotifyEvent : off+unix.SizeofInotifyEvent+int(ev.Len)]
			if i := bytes.IndexByte(name, 0); i >= 0 {
				name = name[:i]
			}
			in.event(ev.Wd, ev.Mask, string(name))
			off += unix.SizeofInotifyEvent + int(ev.Len)
		}
	}
}

func (in *inotify) event(wd int32, mask uint32, name string) {
	if mask&unix.IN_Q_OVERFLOW != 0 {
		in.w.lost()
		in.mu.Lock()
		for _, r := range in.roots {
			in.w.walk(r, in.add, nil)
		}
		in.mu.Unlock()
		return
	}
	in.mu.Lock()
	dir, ok := in.byWD[wd]
	if !ok {
		in.mu.Unlock()
		return
	}
	if mask&unix.IN_IGNORED != 0 {
		// The watch is gone (its directory was deleted or unmounted).
		if in.byPath[dir] == wd {
			delete(in.byPath, dir)
		}
		delete(in.byWD, wd)
		in.mu.Unlock()
		return
	}
	if name == "" {
		// An event of the watched directory itself: a root that was
		// deleted or moved away. Its parent reports the others.
		isRoot := false
		for _, r := range in.roots {
			isRoot = isRoot || r == dir
		}
		in.mu.Unlock()
		if isRoot && mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF) != 0 {
			in.w.changed(dir, true)
		}
		return
	}
	path := filepath.Join(dir, name)
	if mask&unix.IN_ISDIR == 0 {
		in.mu.Unlock()
		in.w.changed(path, false)
		return
	}
	switch {
	case mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0:
		if in.w.skip(path) {
			in.mu.Unlock()
			return
		}
		// Watch the new tree first, then report what is already in it.
		var files []string
		in.w.walk(path, in.add, func(p string, _ os.DirEntry) { files = append(files, p) })
		in.mu.Unlock()
		for _, f := range files {
			in.w.changed(f, false)
		}
	case mask&(unix.IN_MOVED_FROM|unix.IN_DELETE) != 0:
		in.remove(path)
		in.mu.Unlock()
		if mask&unix.IN_MOVED_FROM != 0 {
			// Its files went with it, without events of their own.
			in.w.changed(path, true)
		}
	default:
		in.mu.Unlock()
	}
}
