//go:build windows

package watch

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestShortName(t *testing.T) {
	for name, want := range map[string]bool{
		"MYPACK~1": true, "PROGRA~12": true, "MAIN~1.GO": true, "mypack~1": true, "A~1.TXT": true,
		"4913": false, "main.go~": false, "pkg": false, "~1": false, "MYPACK~": false,
		"LONGERNAME~1": false, "MAIN~1.GOLANG": false, "MY~X": false, "A~1.B.C": false,
	} {
		if got := shortName(name); got != want {
			t.Errorf("shortName(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestRemoveDirs: removing a directory removes its subtree, and only it,
// from both the set and the children index.
func TestRemoveDirs(t *testing.T) {
	root := `c:\r`
	r := &rootWatch{path: root, dirs: map[string]struct{}{}, children: map[string]map[string]struct{}{}}
	for _, p := range []string{`a`, `a\b`, `a\b\c`, `a\bb`, `ab`, `ab\x`, `z`} {
		r.addDir(dirKey(filepath.Join(root, p)))
	}
	if !r.removeDirs(filepath.Join(root, `A\B`)) {
		t.Fatal(`removeDirs(A\B) = false for a known directory`)
	}
	if r.removeDirs(filepath.Join(root, `a\b\c`)) {
		t.Error(`a\b\c is still known after its parent was removed`)
	}
	var left []string
	for k := range r.dirs {
		left = append(left, strings.TrimPrefix(k, root+`\`))
	}
	slices.Sort(left)
	if want := []string{`a`, `a\bb`, `ab`, `ab\x`, `z`}; !slices.Equal(left, want) {
		t.Errorf("left %v, want %v", left, want)
	}
	if _, ok := r.children[dirKey(filepath.Join(root, `a\b`))]; ok {
		t.Error(`the children index keeps a\b`)
	}
	if c := r.children[dirKey(filepath.Join(root, `a`))]; len(c) != 1 {
		t.Errorf(`children of a: %v, want only a\bb`, c)
	}
	if r.removeDirs(filepath.Join(root, `4913`)) {
		t.Error("removeDirs of an unknown name = true")
	}
}

// TestWalkStopsWhenClosed: a walk of a root that is being closed, or of a
// closed watcher, visits nothing more.
func TestWalkStopsWhenClosed(t *testing.T) {
	root := t.TempDir()
	for i := range 20 {
		os.MkdirAll(filepath.Join(root, "d", string(rune('a'+i)), "e"), 0o755)
	}
	d := &rdcw{w: &Watcher{}}
	r := &rootWatch{path: root}
	d.scan(r)
	if len(r.dirs) != 41 {
		t.Fatalf("scan found %d directories, want 41", len(r.dirs))
	}
	r.stopping.Store(true)
	d.scan(r)
	if len(r.dirs) != 0 {
		t.Errorf("a stopping root's scan found %d directories", len(r.dirs))
	}
	r.stopping.Store(false)
	d.closing.Store(true)
	d.scan(r)
	if len(r.dirs) != 0 {
		t.Errorf("a closed watcher's scan found %d directories", len(r.dirs))
	}
}

// TestShortNameRemoved moves away a directory by its 8.3 short name
// (when the volume makes them): whichever name the record carries, it is
// reported as a possible directory.
func TestShortNameRemoved(t *testing.T) {
	h := newHarness(t, false, Options{})
	long := h.path("longdirectoryname")
	if err := os.MkdirAll(filepath.Join(long, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	p, _ := windows.UTF16PtrFromString(long)
	buf := make([]uint16, windows.MAX_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	short := windows.UTF16ToString(buf[:n])
	if err != nil || strings.EqualFold(filepath.Base(short), filepath.Base(long)) {
		t.Skipf("no 8.3 short name on this volume (%v)", err)
	}
	if !shortName(filepath.Base(short)) {
		t.Fatalf("shortName(%q) = false", filepath.Base(short))
	}
	if err := os.Rename(short, filepath.Join(t.TempDir(), "gone")); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(timeout)
	for {
		select {
		case b := <-h.w.Events():
			for _, got := range b.Paths {
				// The record's name is the long or the short one; the
				// root's own path may be either form too.
				if base := filepath.Base(got); strings.EqualFold(base, filepath.Base(long)) || strings.EqualFold(base, filepath.Base(short)) {
					t.Logf("reported as %s", got)
					return
				}
			}
		case <-deadline:
			t.Fatalf("the directory moved away by its short name %s was not reported", short)
		}
	}
}
