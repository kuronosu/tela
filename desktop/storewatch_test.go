package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/watch"
)

// waitFor polls cond until it holds or 10 s pass (the file watcher's
// debounce, and polling where the OS has no notifications).
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitWatch waits until the store's watch is open (it opens on a
// goroutine of its own, ADR-259), and a moment more for its reload.
func waitWatch(t *testing.T, s *Store) *fileWatch {
	t.Helper()
	var fw *fileWatch
	waitFor(t, "the watch to open", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		fw = s.fw
		return fw != nil
	})
	time.Sleep(50 * time.Millisecond)
	return fw
}

// TestStoreSeesOtherPrograms (ADR-253): while the store has watchers, a
// write of another program to its file reaches Get and the watchers of
// the keys it changed; a key the file drops is removed.
func TestStoreSeesOtherPrograms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{"theme": "light", "size": 12}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan string, 16)
	stop := s.Watch("", func(k string) { changed <- k })
	defer stop()
	waitWatch(t, s)
	// Written in place, as a program that does not rename would.
	if err := os.WriteFile(path, []byte(`{"theme": "dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the other program's write", func() bool {
		var theme string
		return s.Get("theme", &theme) && theme == "dark" && !s.Has("size")
	})
	var keys []string
	for len(keys) < 2 {
		select {
		case k := <-changed:
			keys = append(keys, k)
		case <-time.After(5 * time.Second):
			t.Fatalf("watchers ran for %v, want theme and size", keys)
		}
	}
	if got := strings.Join(keys, ","); got != "theme,size" && got != "size,theme" {
		t.Errorf("watchers ran for %q", got)
	}
}

// TestStoreConflicts (ADR-253): for a key both sides changed, the change
// not yet written wins; the file's other keys are merged in, and the
// write keeps them.
func TestStoreConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{"a": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.delay = time.Hour // written by Flush only
	s.mu.Unlock()
	stop := s.Watch("", func(string) {})
	defer stop()
	if err := s.Set("a", 2); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"a": 3, "b": "theirs"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the other program's key", func() bool { return s.Has("b") })
	var a int
	if s.Get("a", &a); a != 2 {
		t.Errorf("a = %d after the other program's write; the change not yet written must win", a)
	}
	// Another program writes again just before this one's write: the write
	// reads it first, so nothing it added is lost.
	if err := os.WriteFile(path, []byte(`{"a": 3, "b": "theirs", "c": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"a": 2`, `"b": "theirs"`, `"c": true`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the file lacks %s after the write:\n%s", want, data)
		}
	}
	// Written: the file's next value of a wins again.
	if err := os.WriteFile(path, []byte(`{"a": 4, "b": "theirs", "c": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file's value of a written key", func() bool {
		var a int
		return s.Get("a", &a) && a == 4
	})
}

// TestStoreWatchSurvivesRemount: a watcher that comes back within the
// grace period after the last one stopped (a screen swap that unmounts and
// mounts a UseSetting) keeps the watch instead of opening a new one.
func TestStoreWatchSurvivesRemount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenStore(path)
	stop := s.Watch("x", func(string) {})
	before := waitWatch(t, s)
	stop()
	stop = s.Watch("x", func(string) {})
	defer stop()
	s.mu.Lock()
	after := s.fw
	s.mu.Unlock()
	if after != before {
		t.Errorf("the watch was %p, %p after a stop and a new watcher: want the same one", before, after)
	}
}

// TestStoreWatchOpensOffTheCaller (ADR-259): Watch returns before the
// watch of the file is open (Watch runs on the UI thread for UseSetting,
// and opening it stats, sets up inotify or scans the folder), and the
// watch is opened without the store's lock.
func TestStoreWatchOpensOffTheCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenStore(path)
	release := make(chan struct{})
	locked := make(chan bool, 1)
	open := newWatch
	newWatch = func(o watch.Options, dirs ...string) (*watch.Watcher, error) {
		<-release
		if s.mu.TryLock() {
			s.mu.Unlock()
			locked <- false
		} else {
			locked <- true
		}
		return open(o, dirs...)
	}
	defer func() { newWatch = open }()
	returned := make(chan func())
	go func() { returned <- s.Watch("x", func(string) {}) }()
	var stop func()
	select {
	case stop = <-returned:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("Watch waited for the watch of the file to open")
	}
	defer stop()
	close(release)
	if <-locked {
		t.Error("the watch was opened with the store's lock held")
	}
	waitWatch(t, s)
}

// TestStoreWatchStops: the file's watch ends with the last watcher, after
// the grace period.
func TestStoreWatchStops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenStore(path)
	stop1 := s.Watch("x", func(string) {})
	stop2 := s.Watch("y", func(string) {})
	waitWatch(t, s)
	stop1()
	stop2()
	s.mu.Lock()
	kept := s.fw != nil
	s.mu.Unlock()
	if !kept {
		t.Error("the watch stopped during the grace period")
	}
	waitFor(t, "the watch to stop", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.fw == nil
	})
}

// TestStoreWatchesMissingFolder (ADR-259): a store whose folder does not
// exist yet sees another program create the folder and write the file.
func TestStoreWatchesMissingFolder(t *testing.T) {
	// Every watch is flat: an ancestor's subtree (all of %APPDATA%, say)
	// is not watched while the folder does not exist, nor the folder's
	// subfolders once it does.
	open := newWatch
	newWatch = func(o watch.Options, dirs ...string) (*watch.Watcher, error) {
		if !o.Flat {
			t.Errorf("the watch of %v is not flat", dirs)
		}
		return open(o, dirs...)
	}
	defer func() { newWatch = open }()
	base := t.TempDir()
	dir := filepath.Join(base, "app", "conf")
	path := filepath.Join(dir, "s.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	stop := s.Watch("k", func(string) {})
	defer stop()
	waitWatch(t, s)
	// One folder of the way at a time: the watch follows each.
	if err := os.Mkdir(filepath.Join(base, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the watch of the new folder", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		r := s.fw.w.Roots()
		return len(r) == 1 && r[0] == filepath.Join(base, "app")
	})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // the folders appear one by one
	if err := os.WriteFile(path, []byte(`{"k": "v"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file in the new folder", func() bool {
		var v string
		return s.Get("k", &v) && v == "v"
	})
	// Then the folder alone is watched, and later writes are seen too.
	if err := os.WriteFile(path, []byte(`{"k": "w"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a later write", func() bool {
		var v string
		return s.Get("k", &v) && v == "w"
	})
	s.mu.Lock()
	roots := s.fw.w.Roots()
	s.mu.Unlock()
	if len(roots) != 1 || roots[0] != dir {
		t.Errorf("watching %v once the folder exists, want [%s]", roots, dir)
	}
}

// TestStoreWriteEdge (ADR-259): another program that writes the file
// after this one's write read it, and before the rename, is read and
// merged again: its key is not lost.
func TestStoreWriteEdge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{"a": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenStore(path)
	s.mu.Lock()
	s.delay = time.Hour // written by Flush only
	s.mu.Unlock()
	if err := s.Set("b", 2); err != nil {
		t.Fatal(err)
	}
	writes := 0
	hook := func(p string) {
		if p != path {
			return // another test's store
		}
		writes++
		if writes == 1 {
			if err := os.WriteFile(path, []byte(`{"a": 1, "c": 3}`), 0o600); err != nil {
				t.Error(err)
			}
		}
	}
	beforeRename.Store(&hook)
	defer beforeRename.Store(nil)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"a": 1`, `"b": 2`, `"c": 3`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the file lacks %s after the write:\n%s", want, data)
		}
	}
	if writes != 2 {
		t.Errorf("%d tries, want 2 (one merge)", writes)
	}
	if !s.Has("c") {
		t.Error("the store lacks the other program's key")
	}
}
