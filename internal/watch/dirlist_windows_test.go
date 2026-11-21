//go:build windows

package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// holdForRename opens path as MoveFileEx opens what it renames (delete
// access, everything shared) and keeps it open until the test ends or the
// returned function is called.
func holdForRename(t *testing.T, path string) func() {
	t.Helper()
	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p, windows.DELETE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatalf("holding %s: %v", path, err)
	}
	var once sync.Once
	release := func() { once.Do(func() { windows.CloseHandle(h) }) }
	t.Cleanup(release)
	return release
}

// TestDirectoryMovedInWhileHeld moves a directory into the tree while a
// handle with delete access is open on it, as MoveFileEx's own is between
// the rename, which reports the new name, and its close: the watcher
// lists it all the same. Go's os.ReadDir shares no delete access, so its
// open failed with a sharing violation and none of the directory's files
// were reported (TestDirectoryMovedInAndOut/native, once on
// windows-latest).
func TestDirectoryMovedInWhileHeld(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		outside := t.TempDir()
		if err := os.MkdirAll(filepath.Join(outside, "pkg", "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outside, "pkg", "sub", "f.go"), []byte("package sub"), 0o644); err != nil {
			t.Fatal(err)
		}
		release := holdForRename(t, filepath.Join(outside, "pkg"))
		rename(t, filepath.Join(outside, "pkg"), h.path("pkg"))
		h.expect("a directory moved in while held", []string{"pkg/sub/f.go"})
		release()
	})
}

// TestListingAllowsRename: a directory the watcher is listing (the
// poller's walks, the native watcher's walk of a new directory) can be
// renamed meanwhile.
func TestListingAllowsRename(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "pkg")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := openDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Rename(dir, filepath.Join(root, "moved")); err != nil {
		t.Errorf("renaming a directory being listed: %v", err)
	}
}

// TestReadBufferOffStack grows the stack of a root's goroutine while its
// first read is pending (a SkipDir that needs a deep stack, called by the
// first walk): the read's buffer and OVERLAPPED do not move with it, so
// the change made next is reported. On the goroutine's stack, the system
// wrote the read's outcome to the old stack, and the copy read as
// incomplete.
func TestReadBufferOffStack(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	skip := func(path string) bool {
		once.Do(func() { growStack(1024) })
		return skipHidden(path)
	}
	h := newHarnessAt(t, root, Options{Filter: goFilter, SkipDir: skip, Debounce: 30 * time.Millisecond})
	h.write("sub/a.go", "package sub")
	h.expect("a change after the stack grew", []string{"sub/a.go"})
}

// growStack uses about n KB of stack.
//
//go:noinline
func growStack(n int) byte {
	var b [1024]byte
	b[n%len(b)] = byte(n)
	if n == 0 {
		return b[0]
	}
	return growStack(n-1) + b[(n*7)%len(b)]
}
