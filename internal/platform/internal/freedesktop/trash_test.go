//go:build linux

package freedesktop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
)

// No test here touches the user's trash: XDG_DATA_HOME points at a
// temporary directory, and the portal is a fake on a private bus.

func TestTrashName(t *testing.T) {
	cases := []struct {
		base string
		i    int
		want string
	}{
		{"todo.md", 1, "todo.md"},
		{"todo.md", 2, "todo.2.md"},
		{"archive.tar.gz", 3, "archive.3.tar.gz"},
		{"notes", 2, "notes.2"},
		{".bashrc", 2, ".bashrc.2"},
		{".config.old", 4, ".config.4.old"},
	}
	for _, c := range cases {
		if got := trashName(c.base, c.i); got != c.want {
			t.Errorf("trashName(%q, %d) = %q, want %q", c.base, c.i, got, c.want)
		}
	}
}

func TestEscapeTrashPath(t *testing.T) {
	cases := map[string]string{
		"/home/ana/todo.md":         "/home/ana/todo.md",
		"/home/ana/My notes/a b.md": "/home/ana/My%20notes/a%20b.md",
		"/tmp/año/€.txt":            "/tmp/a%C3%B1o/%E2%82%AC.txt",
		"/x/100%#?&=+;:@!":          "/x/100%25%23%3F%26%3D%2B%3B%3A%40%21",
		"/x/a-b_c.d~e":              "/x/a-b_c.d~e",
		"/x/\xff":                   "/x/%FF",
	}
	for in, want := range cases {
		if got := escapeTrashPath(in); got != want {
			t.Errorf("escapeTrashPath(%q) = %q, want %q", in, got, want)
		}
	}
	at := time.Date(2026, 11, 1, 9, 41, 7, 0, time.Local)
	if got, want := trashInfo("/home/ana/a b", at), "[Trash Info]\nPath=/home/ana/a%20b\nDeletionDate=2026-11-01T09:41:07\n"; got != want {
		t.Errorf("trashInfo = %q, want %q", got, want)
	}
}

// homeTrash points XDG_DATA_HOME at a temporary directory and returns its
// trash.
func homeTrash(t *testing.T) string {
	t.Helper()
	data := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", data)
	dir, err := homeTrashDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(data, "Trash"); dir != want {
		t.Fatalf("homeTrashDir = %q, want %q", dir, want)
	}
	return dir
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lstat(t *testing.T, path string) fs.FileInfo {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

// expectTrashed checks that the trash has path under name, with its info.
func expectTrashed(t *testing.T, trash, name, path, text string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s is still in its place (%v)", path, err)
	}
	moved := filepath.Join(trash, "files", name)
	if text != "" {
		if got, err := os.ReadFile(moved); err != nil || string(got) != text {
			t.Errorf("%s = %q, %v; want %q", moved, got, err, text)
		}
	} else if _, err := os.Lstat(moved); err != nil {
		t.Errorf("%s: %v", moved, err)
	}
	info, err := os.ReadFile(filepath.Join(trash, "info", name+".trashinfo"))
	if err != nil {
		t.Fatalf("the .trashinfo of %s: %v", name, err)
	}
	lines := strings.Split(string(info), "\n")
	if len(lines) != 4 || lines[0] != "[Trash Info]" || lines[1] != "Path="+escapeTrashPath(path) || !strings.HasPrefix(lines[2], "DeletionDate=") || lines[3] != "" {
		t.Errorf("the .trashinfo of %s =\n%s", name, info)
	}
	if _, err := time.ParseInLocation("2006-01-02T15:04:05", strings.TrimPrefix(lines[2], "DeletionDate="), time.Local); err != nil {
		t.Errorf("DeletionDate: %v", err)
	}
}

func TestHomeTrash(t *testing.T) {
	trash := homeTrash(t)
	src := t.TempDir()
	now := time.Now()

	a := filepath.Join(src, "My notes", "todo list.md")
	writeFile(t, a, "milk")
	if err := moveToHomeTrash(a, lstat(t, a), trash, now); err != nil {
		t.Fatal(err)
	}
	expectTrashed(t, trash, "todo list.md", a, "milk")
	for _, d := range []string{trash, filepath.Join(trash, "files"), filepath.Join(trash, "info")} {
		if fi := lstat(t, d); fi.Mode().Perm() != 0o700 {
			t.Errorf("%s has mode %v, want 0700", d, fi.Mode().Perm())
		}
	}

	// The same name again: the next one, like GIO.
	b := filepath.Join(src, "todo list.md")
	writeFile(t, b, "eggs")
	if err := moveToHomeTrash(b, lstat(t, b), trash, now); err != nil {
		t.Fatal(err)
	}
	expectTrashed(t, trash, "todo list.2.md", b, "eggs")

	// A stale entry of files/ without its info is not replaced.
	writeFile(t, filepath.Join(trash, "files", "x.txt"), "stale")
	c := filepath.Join(src, "x.txt")
	writeFile(t, c, "fresh")
	if err := moveToHomeTrash(c, lstat(t, c), trash, now); err != nil {
		t.Fatal(err)
	}
	expectTrashed(t, trash, "x.2.txt", c, "fresh")
	if got, _ := os.ReadFile(filepath.Join(trash, "files", "x.txt")); string(got) != "stale" {
		t.Errorf("the stale entry was replaced: %q", got)
	}
	if _, err := os.Stat(filepath.Join(trash, "info", "x.txt.trashinfo")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the info of the clash was left behind (%v)", err)
	}

	// A folder with its content, and a symbolic link (itself, not its
	// target).
	dir := filepath.Join(src, "My notes")
	if err := moveToHomeTrash(dir, lstat(t, dir), trash, now); err != nil {
		t.Fatal(err)
	}
	expectTrashed(t, trash, "My notes", dir, "")
	target := filepath.Join(src, "target.txt")
	writeFile(t, target, "kept")
	link := filepath.Join(src, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := moveToHomeTrash(link, lstat(t, link), trash, now); err != nil {
		t.Fatal(err)
	}
	if fi := lstat(t, filepath.Join(trash, "files", "link.txt")); fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the link was not moved as a link")
	}
	if got, _ := os.ReadFile(target); string(got) != "kept" {
		t.Errorf("the link's target changed")
	}
}

// A file on another file system stays where it is: a rename cannot move
// it, and a copy and a delete is not done.
func TestHomeTrashOtherFileSystem(t *testing.T) {
	trash := homeTrash(t)
	a := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, a, "a")
	fi := lstat(t, a)
	old := deviceOf
	t.Cleanup(func() { deviceOf = old })
	deviceOf = func(x fs.FileInfo) (uint64, bool) {
		if x == fi {
			return 1, true
		}
		return 2, true
	}
	err := moveToHomeTrash(a, fi, trash, time.Now())
	if !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("moveToHomeTrash across file systems = %v, want ErrUnsupported", err)
	}
	if got, _ := os.ReadFile(a); string(got) != "a" {
		t.Errorf("the file left its place")
	}
	if _, err := os.Stat(trash); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the trash was created for nothing (%v)", err)
	}
}

func TestHomeTrashRefuses(t *testing.T) {
	trash := homeTrash(t)
	inside := filepath.Join(trash, "files", "a")
	writeFile(t, inside, "a")
	if err := moveToHomeTrash(inside, lstat(t, inside), trash, time.Now()); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("trashing a file in the trash = %v", err)
	}
	data := filepath.Dir(trash)
	if err := moveToHomeTrash(data, lstat(t, data), trash, time.Now()); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("trashing the folder of the trash = %v", err)
	}
	t.Setenv("XDG_DATA_HOME", "relative/data")
	t.Setenv("HOME", "")
	if _, err := homeTrashDir(); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("homeTrashDir without a home = %v", err)
	}
	t.Setenv("HOME", "/home/ana")
	if dir, _ := homeTrashDir(); dir != "/home/ana/.local/share/Trash" {
		t.Errorf("homeTrashDir with a relative XDG_DATA_HOME = %q", dir)
	}
}

// --- through the Desktop, with a fake portal ---

// fakeTrashPortal is the portal's Trash interface: it records the file
// behind each descriptor and how it was opened, and trashes it into its own
// directory when it answers 1.
type fakeTrashPortal struct {
	dir string

	mu       sync.Mutex
	answer   uint32
	fail     bool
	calls    []string
	writable []bool
}

func startTrashPortal(t *testing.T, addr string) *fakeTrashPortal {
	t.Helper()
	p := startPortal(t, addr, 0)
	tp := &fakeTrashPortal{dir: t.TempDir(), answer: 1}
	if err := p.conn.ExportMethodTable(map[string]any{
		"TrashFile": func(fd dbus.UnixFD) (uint32, *dbus.Error) {
			defer unix.Close(int(fd))
			path, _ := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
			fl, _ := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
			tp.mu.Lock()
			defer tp.mu.Unlock()
			tp.calls = append(tp.calls, path)
			tp.writable = append(tp.writable, fl&unix.O_ACCMODE == unix.O_RDWR || fl&unix.O_PATH != 0)
			if tp.fail {
				return 0, dbus.NewError("org.freedesktop.portal.Error.Failed", []any{"no"})
			}
			if tp.answer == 1 {
				if err := os.Rename(path, filepath.Join(tp.dir, filepath.Base(path))); err != nil {
					return 0, nil
				}
			}
			return tp.answer, nil
		},
	}, portalPath, ifaceTrash); err != nil {
		t.Fatal(err)
	}
	return tp
}

func (p *fakeTrashPortal) set(answer uint32, fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answer, p.fail = answer, fail
}

func (p *fakeTrashPortal) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

// openedWritable reports whether the i-th descriptor could write.
func (p *fakeTrashPortal) openedWritable(i int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.writable[i]
}

func newTrashDesktop(t *testing.T, addr string) *Desktop {
	t.Helper()
	d := New(Options{AppID: "tela-test", Address: addr,
		Wake: func() {}, Post: func(func()) {}, Event: func(platform.Event) {}})
	t.Cleanup(d.Close)
	return d
}

func TestMoveToTrashPortal(t *testing.T) {
	addr := privateBus(t)
	tp := startTrashPortal(t, addr)
	trash := homeTrash(t)
	d := newTrashDesktop(t, addr)
	src := t.TempDir()

	// 1: the portal trashed it; from another goroutine, as apps call it.
	a := filepath.Join(src, "a.md")
	writeFile(t, a, "a")
	done := make(chan error)
	go func() { done <- d.MoveToTrash(a) }()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := tp.seen(); len(got) != 1 || got[0] != a || !tp.openedWritable(0) {
		t.Errorf("the portal got %v, want %s opened for writing", got, a)
	}
	if _, err := os.Stat(filepath.Join(tp.dir, "a.md")); err != nil {
		t.Errorf("the portal did not trash it: %v", err)
	}
	if _, err := os.Stat(trash); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the home trash was used although the portal took it")
	}

	// A folder goes to the portal with an O_PATH descriptor.
	folder := filepath.Join(src, "folder")
	writeFile(t, filepath.Join(folder, "x"), "x")
	if err := d.MoveToTrash(folder + "/"); err != nil {
		t.Fatal(err)
	}
	if got := tp.seen(); len(got) != 2 || got[1] != folder {
		t.Errorf("the portal got %v, want the folder", got)
	}

	// 0: the home trash takes it.
	tp.set(0, false)
	b := filepath.Join(src, "b.md")
	writeFile(t, b, "b")
	if err := d.MoveToTrash(b); err != nil {
		t.Fatal(err)
	}
	expectTrashed(t, trash, "b.md", b, "b")

	// An error: the home trash too.
	tp.set(1, true)
	c := filepath.Join(src, "c.md")
	writeFile(t, c, "c")
	if err := d.MoveToTrash(c); err != nil {
		t.Fatal(err)
	}
	expectTrashed(t, trash, "c.md", c, "c")

	// A read-only file cannot be opened for writing: the portal would
	// refuse it, so it is not asked.
	if os.Geteuid() != 0 {
		tp.set(1, false)
		ro := filepath.Join(src, "ro.md")
		writeFile(t, ro, "ro")
		if err := os.Chmod(ro, 0o400); err != nil {
			t.Fatal(err)
		}
		n := len(tp.seen())
		if err := d.MoveToTrash(ro); err != nil {
			t.Fatal(err)
		}
		if len(tp.seen()) != n {
			t.Errorf("a read-only file was handed to the portal")
		}
		expectTrashed(t, trash, "ro.md", ro, "ro")
	}

	// A symbolic link never goes to the portal (it would trash the target).
	target := filepath.Join(src, "target")
	writeFile(t, target, "t")
	link := filepath.Join(src, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	n := len(tp.seen())
	if err := d.MoveToTrash(link); err != nil {
		t.Fatal(err)
	}
	if len(tp.seen()) != n {
		t.Errorf("a symbolic link was handed to the portal")
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the link's target was trashed: %v", err)
	}
}

func TestMoveToTrashWithoutPortal(t *testing.T) {
	trash := homeTrash(t)
	src := t.TempDir()
	// A bus without a portal, and no bus at all.
	for i, addr := range []string{privateBus(t), "unix:path=" + filepath.Join(src, "no-bus")} {
		d := newTrashDesktop(t, addr)
		a := filepath.Join(src, "a.txt")
		writeFile(t, a, addr)
		if err := d.MoveToTrash(a); err != nil {
			t.Fatalf("bus %s: %v", addr, err)
		}
		expectTrashed(t, trash, trashName("a.txt", i+1), a, addr)
	}
}

func TestMoveToTrashErrors(t *testing.T) {
	homeTrash(t)
	d := newTrashDesktop(t, "unix:path="+filepath.Join(t.TempDir(), "no-bus"))
	missing := filepath.Join(t.TempDir(), "missing.md")
	if err := d.MoveToTrash(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveToTrash of a missing file = %v, want fs.ErrNotExist", err)
	}
	for _, p := range []string{"", "notes.md", "/", "a\x00b"} {
		if err := d.MoveToTrash(p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("MoveToTrash(%q) = %v, want fs.ErrInvalid", p, err)
		}
	}
	d.Close()
	a := filepath.Join(t.TempDir(), "a")
	writeFile(t, a, "a")
	if err := d.MoveToTrash(a); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("MoveToTrash after Close = %v, want ErrUnsupported", err)
	}
}
