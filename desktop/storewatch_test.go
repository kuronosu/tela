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
	time.Sleep(50 * time.Millisecond) // the watch is set up
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
	s.mu.Lock()
	before := s.fw
	s.mu.Unlock()
	stop()
	stop = s.Watch("x", func(string) {})
	defer stop()
	s.mu.Lock()
	after := s.fw
	s.mu.Unlock()
	if before == nil || after != before {
		t.Errorf("the watch was %p, %p after a stop and a new watcher: want the same one", before, after)
	}
}

// TestStoreWatchOpensWithoutTheLock: the watch of the file is opened
// without the store's lock (Watch runs on the UI thread for UseSetting, and
// opening it stats, sets up inotify or scans the folder).
func TestStoreWatchOpensWithoutTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenStore(path)
	locked := false
	open := newWatch
	newWatch = func(o watch.Options, dirs ...string) (*watch.Watcher, error) {
		if s.mu.TryLock() {
			s.mu.Unlock()
		} else {
			locked = true
		}
		return open(o, dirs...)
	}
	defer func() { newWatch = open }()
	stop := s.Watch("x", func(string) {})
	defer stop()
	if locked {
		t.Error("the watch was opened with the store's lock held")
	}
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
	s.mu.Lock()
	on := s.fw != nil
	s.mu.Unlock()
	stop1()
	stop2()
	s.mu.Lock()
	kept := s.fw != nil
	s.mu.Unlock()
	if !on || !kept {
		t.Errorf("watching while watched %v, during the grace period %v", on, kept)
	}
	waitFor(t, "the watch to stop", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.fw == nil
	})
}
