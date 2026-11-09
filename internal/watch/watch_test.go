package watch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/testutil/wine"
)

const timeout = 5 * time.Second

// goFilter accepts .go files and directories, like tela dev's filter in
// miniature, and rejects editor temporary files by suffix.
func goFilter(path string, dir bool) bool {
	return dir || strings.HasSuffix(path, ".go")
}

func skipHidden(path string) bool { return strings.HasPrefix(filepath.Base(path), ".") }

type harness struct {
	t    *testing.T
	w    *Watcher
	root string
}

// modes runs f with the native watcher (where there is one) and the
// poller.
func modes(t *testing.T, f func(t *testing.T, poll bool)) {
	t.Run("native", func(t *testing.T) {
		if runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
			t.Skip("no native watcher on " + runtime.GOOS)
		}
		f(t, false)
	})
	t.Run("poll", func(t *testing.T) { f(t, true) })
}

func newHarness(t *testing.T, poll bool, opts Options) *harness {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if opts.Filter == nil {
		opts.Filter = goFilter
	}
	if opts.SkipDir == nil {
		opts.SkipDir = skipHidden
	}
	if opts.Debounce == 0 {
		opts.Debounce = 30 * time.Millisecond
	}
	opts.Poll = poll
	opts.PollInterval = 20 * time.Millisecond
	return newHarnessAt(t, root, opts)
}

func newHarnessAt(t *testing.T, root string, opts Options) *harness {
	t.Helper()
	w, err := New(opts, root)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Poll && !w.Native() {
		t.Fatalf("native watching unavailable")
	}
	t.Cleanup(func() { w.Close() })
	go func() {
		for err := range w.Errors() {
			t.Logf("watch error: %v", err)
		}
	}()
	return &harness{t: t, w: w, root: root}
}

func (h *harness) path(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(h.root, filepath.FromSlash(rel))
}

func (h *harness) write(rel, data string) {
	h.t.Helper()
	p := h.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// expect waits until the batches received contain every path of want
// (relative), and fails if one of them contains a path of not.
func (h *harness) expect(what string, want []string, not ...string) []Batch {
	h.t.Helper()
	got := map[string]bool{}
	var batches []Batch
	overflows := 0
	deadline := time.After(timeout)
	for {
		missing := false
		for _, w := range want {
			missing = missing || !got[h.path(w)]
		}
		if !missing {
			break
		}
		select {
		case b, ok := <-h.w.Events():
			if !ok {
				h.t.Fatalf("%s: events closed", what)
			}
			batches = append(batches, b)
			if b.Overflow {
				overflows++
			}
			for _, p := range b.Paths {
				got[p] = true
			}
		case <-deadline:
			h.t.Fatalf("%s: timed out; got %v (%d batches, %d overflowed), want %v", what, keys(got), len(batches), overflows, want)
		}
	}
	for _, n := range not {
		if got[h.path(n)] {
			h.t.Errorf("%s: %s reported", what, n)
		}
	}
	return batches
}

// quiet checks that no batch arrives for a while.
func (h *harness) quiet(what string, d time.Duration) {
	h.t.Helper()
	select {
	case b := <-h.w.Events():
		h.t.Errorf("%s: unexpected batch %v", what, b.Paths)
	case <-time.After(d):
	}
}

// absent checks that no batch reports one of paths (relative, or absolute)
// for a while. Other paths may arrive: a file written once can be reported
// twice when a scan or an event lands between its creation and its write,
// and the second batch can come after the test saw the first.
func (h *harness) absent(what string, d time.Duration, paths ...string) {
	h.t.Helper()
	end := time.After(d)
	for {
		select {
		case b := <-h.w.Events():
			for _, p := range paths {
				if slices.Contains(b.Paths, h.path(p)) {
					h.t.Errorf("%s: %s reported (batch %v)", what, p, b.Paths)
				}
			}
		case <-end:
			return
		}
	}
}

// rename renames a directory (or file) of a test. On Windows a directory
// that a walk (the poller's, or the native watcher's of a new directory)
// is listing cannot be renamed: "being used by another process" when the
// walk holds it, "access is denied" when it holds a directory under it. A
// walk holds it for a moment only, so the rename is retried for a while, as
// a user would.
func rename(t *testing.T, from, to string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		err := os.Rename(from, to)
		if err == nil {
			return
		}
		if runtime.GOOS != "windows" || !inUse(err) || time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// inUse reports Windows' ERROR_SHARING_VIOLATION (32) and
// ERROR_ACCESS_DENIED (5); the caller checks the OS, since the numbers
// mean something else elsewhere.
func inUse(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == 32 || errno == 5)
}

func keys(m map[string]bool) []string {
	var s []string
	for k := range m {
		s = append(s, k)
	}
	slices.Sort(s)
	return s
}

func TestCreateModifyDelete(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		h.write("a.go", "package a")
		h.expect("create", []string{"a.go"})
		time.Sleep(10 * time.Millisecond) // a new mtime for the poller
		h.write("a.go", "package a // changed")
		h.expect("modify", []string{"a.go"})
		os.Remove(h.path("a.go"))
		h.expect("delete", []string{"a.go"})
	})
}

func TestRename(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		h.write("old.go", "package a")
		h.expect("create", []string{"old.go"})
		rename(t, h.path("old.go"), h.path("new.go"))
		h.expect("rename", []string{"old.go", "new.go"})
	})
}

func TestNewSubdirectories(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		// mkdir -p and a file at once: the file may exist before the
		// watch of its directory.
		h.write("x/y/z/deep.go", "package z")
		h.expect("file in new directories", []string{"x/y/z/deep.go"})
		// The new directories are watched from now on.
		h.write("x/y/z/later.go", "package z")
		h.expect("file in a directory created earlier", []string{"x/y/z/later.go"})
	})
}

func TestDirectoryMovedInAndOut(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		if poll {
			wine.Skip(t, "renaming a directory the poller scanned fails with a sharing violation")
		} else {
			wine.Skip(t, "ReadDirectoryChangesW does not report the files of a directory renamed within the watched tree")
		}
		h := newHarness(t, poll, Options{})
		outside := t.TempDir()
		if err := os.MkdirAll(filepath.Join(outside, "pkg", "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(outside, "pkg", "sub", "f.go"), []byte("package sub"), 0o644)
		rename(t, filepath.Join(outside, "pkg"), h.path("pkg"))
		h.expect("directory moved in", []string{"pkg/sub/f.go"})
		h.write("pkg/sub/g.go", "package sub")
		h.expect("file in a moved-in directory", []string{"pkg/sub/g.go"})

		// Moved within the tree: the new path is watched, the old is not.
		rename(t, h.path("pkg"), h.path("moved"))
		h.expect("directory moved within", []string{"pkg", "moved/sub/f.go"})
		h.write("moved/sub/h.go", "package sub")
		h.expect("file in the moved directory", []string{"moved/sub/h.go"}, "pkg/sub/h.go")

		// Moved away.
		rename(t, h.path("moved"), filepath.Join(outside, "gone"))
		h.expect("directory moved away", []string{"moved"})
		os.WriteFile(filepath.Join(outside, "gone", "sub", "x.go"), []byte("package sub"), 0o644)
		h.quiet("a file in a directory that left the tree", 200*time.Millisecond)
	})
}

// TestRemovedNames removes names that were in the tree before the watcher
// started: a directory is reported as one (dir = true, which goFilter
// accepts), an extensionless file is not (Windows' records do not say
// which a removed name was; the watcher knows its tree's directories).
func TestRemovedNames(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		if poll {
			wine.Skip(t, "renaming a directory the poller scanned fails with a sharing violation")
		}
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"pre/sub/f.go", "Makefile", "4913", "pkg/LICENSE"} {
			p = filepath.Join(root, filepath.FromSlash(p))
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		h := newHarnessAt(t, root, Options{Filter: goFilter, SkipDir: skipHidden, Debounce: 30 * time.Millisecond, Poll: poll, PollInterval: 20 * time.Millisecond})
		time.Sleep(50 * time.Millisecond) // the poller's first scan
		for _, p := range []string{"Makefile", "4913", "pkg/LICENSE"} {
			if err := os.Remove(h.path(p)); err != nil {
				t.Fatal(err)
			}
		}
		h.quiet("extensionless files removed", 300*time.Millisecond)
		outside := t.TempDir()
		rename(t, h.path("pre"), filepath.Join(outside, "pre"))
		h.expect("directory moved away", []string{"pre"})
	})
}

// TestDirectoryGoneRightAfterStart moves a directory away as soon as New
// returns, from a directory that a walk of the tree lists last: the
// watcher knows it is a directory (on Windows, New used to return before
// the walk that finds the tree's directories, which then missed it).
func TestDirectoryGoneRightAfterStart(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		if poll {
			wine.Skip(t, "moving away a directory the poller scanned does not complete (sharing violation, then a timeout)")
		}
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for i := range 400 {
			os.MkdirAll(filepath.Join(root, fmt.Sprintf("a%03d", i), "b"), 0o755)
		}
		if err := os.MkdirAll(filepath.Join(root, "z", "pkg", "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		h := newHarnessAt(t, root, Options{Filter: goFilter, SkipDir: skipHidden, Debounce: 30 * time.Millisecond, Poll: poll, PollInterval: 20 * time.Millisecond})
		rename(t, h.path("z/pkg"), filepath.Join(t.TempDir(), "pkg"))
		h.expect("directory moved away", []string{"z/pkg"})
	})
}

func TestFilters(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		h.write("notes.txt", "x")
		h.write(".git/objects/ab.go", "x") // a skipped directory
		h.write(".hidden/c.go", "x")
		h.quiet("filtered files", 200*time.Millisecond)
		h.write("ok.go", "x")
		h.expect("after filtered files", []string{"ok.go"}, "notes.txt", ".git/objects/ab.go", ".hidden/c.go")
	})
}

func TestDebounce(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		// Long next to the burst's 20 ms gaps: the watcher sees the writes
		// when its reader (inotify's, a completion of
		// ReadDirectoryChangesW, the poller's walk) gets them, which a
		// loaded runner can stall too, unseen by the writer below.
		const debounce = 500 * time.Millisecond
		// The burst is a burst only if no gap between its writes came
		// near the debounce: a loaded runner can stall a sleep or a write
		// for longer, and the watcher then rightly sees two bursts. Such
		// an attempt proves nothing, so it is retried.
		var (
			h           *harness
			start, last time.Time
		)
		for attempt := 1; ; attempt++ {
			h = newHarness(t, poll, Options{Debounce: debounce})
			start = time.Now()
			last = start
			var maxGap time.Duration
			for i := range 5 {
				if i > 0 {
					time.Sleep(20 * time.Millisecond)
				}
				h.write("a.go", strings.Repeat("x", i+1))
				h.write("b.go", strings.Repeat("y", i+1))
				now := time.Now()
				maxGap = max(maxGap, now.Sub(last))
				last = now
			}
			if maxGap < debounce/2 {
				break
			}
			if attempt == 3 {
				t.Skipf("the runner stalled the burst three times (a gap of %v between writes)", maxGap)
			}
			t.Logf("attempt %d: a gap of %v between writes, not a burst; again", attempt, maxGap)
			h.w.Close()
		}
		bs := h.expect("burst", []string{"a.go", "b.go"})
		got := time.Now()
		if len(bs) != 1 {
			t.Errorf("a burst of writes gave %d batches, want 1", len(bs))
		}
		if got.Sub(last) < debounce-10*time.Millisecond {
			t.Errorf("batch %v after the last write, before the debounce", got.Sub(last))
		}
		if bs[0].First.Before(start) || bs[0].First.After(last) {
			t.Errorf("Batch.First %v is not within the burst", bs[0].First)
		}
		h.quiet("after the burst", 300*time.Millisecond)
	})
}

func TestMaxDelay(t *testing.T) {
	poll := runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin"
	h := newHarness(t, poll, Options{Debounce: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond})
	stop := time.After(900 * time.Millisecond)
	// The writer stops before the temporary directory is removed (cleanups
	// run last-registered first): writing into it while it is removed fails
	// the removal on Windows ("Directory is not empty").
	quit, stopped := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(quit); <-stopped })
	n := 0
	go func() {
		defer close(stopped)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-quit:
				return
			default:
			}
			os.WriteFile(h.path("busy.go"), []byte(strings.Repeat("x", i%50+1)), 0o644)
			time.Sleep(30 * time.Millisecond)
		}
	}()
	deadline := time.After(timeout)
	for n < 2 {
		select {
		case <-h.w.Events():
			n++
		case <-deadline:
			t.Fatalf("writes that never pause gave %d batches, want them every MaxDelay", n)
		}
	}
}

// TestEditorSaves replays how editors save: vim (rename the original to a
// backup, write a new file; or write a 4913 probe first), GoLand (write a
// temporary file, rename it over the original) and VS Code (truncate and
// write in place).
func TestEditorSaves(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		h.write("main.go", "package main")
		h.expect("create", []string{"main.go"})
		time.Sleep(10 * time.Millisecond)

		// vim with backupcopy=no.
		h.write("4913", "")
		os.Remove(h.path("4913"))
		os.Rename(h.path("main.go"), h.path("main.go~"))
		h.write("main.go", "package main // vim")
		os.Remove(h.path("main.go~"))
		h.expect("vim", []string{"main.go"}, "4913", "main.go~")
		time.Sleep(10 * time.Millisecond)

		// GoLand's safe write.
		h.write("main.go___jb_tmp___", "package main // goland")
		os.Rename(h.path("main.go"), h.path("main.go___jb_old___"))
		os.Rename(h.path("main.go___jb_tmp___"), h.path("main.go"))
		os.Remove(h.path("main.go___jb_old___"))
		h.expect("goland", []string{"main.go"})
		time.Sleep(10 * time.Millisecond)

		// VS Code: in place.
		f, err := os.OpenFile(h.path("main.go"), os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("package main // vscode, longer")
		f.Close()
		h.expect("vscode", []string{"main.go"})
	})
}

func TestSetRoots(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		other, _ := filepath.EvalSymlinks(t.TempDir())
		os.WriteFile(filepath.Join(other, "x.go"), []byte("x"), 0o644)
		h.quiet("outside the roots", 150*time.Millisecond)
		if err := h.w.SetRoots([]string{h.root, other}); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(other, "y.go"), []byte("y"), 0o644)
		// expect joins relative paths to h.root; an absolute path is kept.
		h.expect("second root", []string{filepath.Join(other, "y.go")})
		h.write("z.go", "z")
		h.expect("first root", []string{"z.go"})
		if err := h.w.SetRoots([]string{h.root}); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(other, "w.go"), []byte("w"), 0o644)
		h.absent("a root that was removed", 200*time.Millisecond, filepath.Join(other, "w.go"))
		// A root that does not exist is reported, not fatal.
		if err := h.w.SetRoots([]string{h.root, filepath.Join(other, "missing")}); err != nil {
			t.Fatal(err)
		}
		h.write("still.go", "x")
		h.expect("after a missing root", []string{"still.go"})
	})
}

// TestPollSetRootsKeepsChanges: a change made between two walks under a
// root that stays is reported when SetRoots adds another root before the
// next walk (the new walk used to become the baseline and hide it), and the
// files of the new root are its baseline, not changes.
func TestPollSetRootsKeepsChanges(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	other, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(root, "a.go"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(other, "old.go"), []byte("o"), 0o644)
	// No walk happens during the test: only SetRoots walks.
	h := newHarnessAt(t, root, Options{Filter: goFilter, SkipDir: skipHidden, Debounce: 30 * time.Millisecond, Poll: true, PollInterval: time.Hour})
	h.write("a.go", "changed")
	h.write("new.go", "n")
	if err := h.w.SetRoots([]string{root, other}); err != nil {
		t.Fatal(err)
	}
	h.expect("the changes made before SetRoots", []string{"a.go", "new.go"}, filepath.Join(other, "old.go"))
}

func TestClose(t *testing.T) {
	modes(t, func(t *testing.T, poll bool) {
		h := newHarness(t, poll, Options{})
		h.w.Close()
		select {
		case _, ok := <-h.w.Events():
			if ok {
				t.Errorf("a batch after Close")
			}
		case <-time.After(timeout):
			t.Fatal("Events not closed by Close")
		}
		if err := h.w.SetRoots(nil); err != ErrClosed {
			t.Errorf("SetRoots after Close: %v", err)
		}
		h.w.Close() // idempotent
	})
}

func TestWithin(t *testing.T) {
	sep := string(filepath.Separator)
	for _, c := range []struct {
		path, root string
		want       bool
	}{
		{"/a/b", "/a", true},
		{"/a", "/a", true},
		{"/ab", "/a", false},
		{"/a/b/c", "/a/b", true},
		{"/x", "/", true},
	} {
		p := strings.ReplaceAll(c.path, "/", sep)
		r := strings.ReplaceAll(c.root, "/", sep)
		if got := within(p, r); got != c.want {
			t.Errorf("within(%q, %q) = %v", p, r, got)
		}
	}
}
