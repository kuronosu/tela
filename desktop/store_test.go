package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStoreGetSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "s.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := OpenStore(path); again != s {
		t.Error("opening the same path twice gave two stores")
	}
	type editor struct {
		Font string
		Size int
	}
	if err := s.Set("editor", editor{"Inter", 14}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	var e editor
	if !s.Get("editor", &e) || e != (editor{"Inter", 14}) {
		t.Errorf("Get(editor) = %+v", e)
	}
	n := 7
	if s.Get("theme", &n) || n != 7 {
		t.Errorf("a value of another type decoded into an int: %d", n)
	}
	if s.Get("missing", &n) || n != 7 {
		t.Errorf("a missing key changed the default: %d", n)
	}
	if got := s.Keys(); strings.Join(got, ",") != "editor,theme" {
		t.Errorf("Keys() = %v", got)
	}
	if err := s.Set("f", func() {}); err == nil || s.Has("f") {
		t.Errorf("a func was stored: %v", err)
	}
	s.Delete("theme")
	if s.Has("theme") {
		t.Error("Delete kept the key")
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(data, &onDisk); err != nil || len(onDisk) != 1 || onDisk["editor"] == nil {
		t.Errorf("file %s: %v", data, err)
	}
	// No temporary files are left next to it.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want only the store", len(entries))
	}
	// A new store on the file reads what was written.
	forget(path)
	s2, err := OpenStore(path)
	if err != nil || !s2.Get("editor", &e) || e.Size != 14 {
		t.Errorf("reopened: %+v, %v", e, err)
	}
}

// Changes reach the watchers of their key (and of every key), after the
// change and once per real change.
func TestStoreWatch(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "w.json"))
	var mu sync.Mutex
	var got []string
	stop := s.Watch("a", func(k string) {
		mu.Lock()
		defer mu.Unlock()
		var v int
		s.Get(k, &v)
		got = append(got, k+"="+string(rune('0'+v)))
	})
	all := 0
	stopAll := s.Watch("", func(string) { all++ })
	_ = s.Set("a", 1)
	_ = s.Set("a", 1) // same value: nothing
	_ = s.Set("b", 2)
	s.Delete("a")
	stop()
	_ = s.Set("a", 3)
	stopAll()
	_ = s.Set("b", 4)
	if err := s.Flush(); err != nil { // before the test's folder goes
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a=1,a=0" || all != 4 {
		t.Errorf("watcher of a got %v, watcher of every key %d changes; want [a=1 a=0] and 4", got, all)
	}
}

// A file that does not parse starts the store empty, is reported, and is
// kept as .corrupt when the store writes.
func TestStoreCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err == nil || s == nil || !strings.Contains(err.Error(), ".corrupt") || s.Err() == nil {
		t.Fatalf("OpenStore of a bad file: %v, %v", s, err)
	}
	_ = s.Set("k", true)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if old, err := os.ReadFile(path + ".corrupt"); err != nil || string(old) != "{not json" {
		t.Errorf("the bad file was not kept: %q, %v", old, err)
	}
	if s.Err() != nil {
		t.Errorf("Err after a good write: %v", s.Err())
	}
}

// Changes are written by themselves shortly after, and by FlushAll.
func TestStoreWritesLater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "later.json")
	s, _ := OpenStore(path)
	_ = s.Set("k", "v")
	if err := FlushAll(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), `"k": "v"`) {
		t.Errorf("after FlushAll: %q, %v", data, err)
	}
}

// Stores are safe from several goroutines.
func TestStoreConcurrent(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "c.json"))
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				_ = s.Set("k", i*100+j)
				var v int
				s.Get("k", &v)
			}
		}()
	}
	wg.Wait()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
}

// forget drops the open store of path, as a new process would not have it.
func forget(path string) {
	abs, _ := filepath.Abs(path)
	stores.mu.Lock()
	defer stores.mu.Unlock()
	delete(stores.byPath, abs)
}
