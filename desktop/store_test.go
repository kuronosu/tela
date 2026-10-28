package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

// A file that exists but cannot be read is not overwritten by a write: the
// write fails until the file can be read, and then merges with it.
func TestStoreUnreadableFileIsNotOverwritten(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs file permissions that deny the owner")
	}
	path := filepath.Join(t.TempDir(), "locked.json")
	const content = `{"a": 1, "b": 2}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err == nil || s == nil {
		t.Fatalf("OpenStore of an unreadable file: %v, %v", s, err)
	}
	_ = s.Set("c", 3)
	_ = s.Set("a", 10)
	if err := s.Flush(); err == nil {
		t.Fatal("Flush wrote over a file that could not be read")
	}
	if s.Err() == nil {
		t.Error("Err() is nil after the refused write")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != content {
		t.Fatalf("the file changed: %q", got)
	}
	// Readable again: the next write merges the file's keys with the
	// changes made meanwhile, which win.
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	forget(path)
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var a, b, c int
	s2.Get("a", &a)
	s2.Get("b", &b)
	s2.Get("c", &c)
	if a != 10 || b != 2 || c != 3 || s.Err() != nil {
		t.Errorf("after the retry a=%d b=%d c=%d (want 10 2 3), Err %v", a, b, c, s.Err())
	}
}

// Fields missing from the stored value keep the default's; the default's
// slices and maps are never modified; a type mismatch reads as the default.
func TestStoreGetKeepsDefaults(t *testing.T) {
	type editor struct {
		FontSize int
		Wrap     bool
		Tags     []string
		Extra    map[string]int
		Inner    struct{ A, B int }
		Ptr      *int
	}
	path := filepath.Join(t.TempDir(), "d.json")
	if err := os.WriteFile(path, []byte(`{
  "editor": {"FontSize": 16, "Tags": ["x"], "Extra": {"k": 2}, "Inner": {"A": 5}},
  "wrong": "text"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := OpenStore(path)
	one := 1
	def := editor{FontSize: 14, Wrap: true, Tags: make([]string, 3, 8), Extra: map[string]int{"d": 1}, Ptr: &one}
	def.Tags[0], def.Tags[1], def.Tags[2] = "a", "b", "c"
	def.Inner.B = 9
	for range 2 { // twice: the default must be as it was
		got := def
		if !s.Get("editor", &got) {
			t.Fatal("Get = false")
		}
		if got.FontSize != 16 || !got.Wrap || got.Inner.A != 5 || got.Inner.B != 9 || got.Ptr == nil || *got.Ptr != 1 {
			t.Errorf("Get = %+v, want the stored FontSize and Inner.A over the default's other fields", got)
		}
		if len(got.Tags) != 1 || got.Tags[0] != "x" {
			t.Errorf("Tags = %v, want the stored slice", got.Tags)
		}
		if got.Extra["k"] != 2 || got.Extra["d"] != 1 {
			t.Errorf("Extra = %v, want the stored and the default entries", got.Extra)
		}
		got.Extra["z"] = 1 // the result is the caller's too
		*got.Ptr = 7
	}
	if strings.Join(def.Tags, ",") != "a,b,c" || def.Tags[:cap(def.Tags)][0] != "a" || len(def.Extra) != 1 || *def.Ptr != 1 {
		t.Errorf("the default was modified: %+v", def)
	}
	n := editor{FontSize: 3}
	if s.Get("wrong", &n) || n.FontSize != 3 {
		t.Errorf("a value of another type changed the default: %+v", n)
	}
	// A partly decoded value that then fails leaves *ptr as it was.
	_ = s.Set("half", map[string]any{"FontSize": 20, "Wrap": "not a bool"})
	h := editor{FontSize: 3}
	if s.Get("half", &h) || h.FontSize != 3 {
		t.Errorf("a failed decode changed the value: %+v", h)
	}
}

// A value loaded from the (indented) file equals the same value Set
// encodes: setting it again is not a change.
func TestStoreSetEqualAfterReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eq.json")
	type pt struct{ X, Y int }
	s, _ := OpenStore(path)
	_ = s.Set("pt", pt{1, 2})
	_ = s.Set("list", []string{"a", "<b>"})
	_ = s.Set("m", map[string]int{"z": 1, "a": 2})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	forget(path)
	s2, _ := OpenStore(path)
	changes := 0
	s2.Watch("", func(string) { changes++ })
	_ = s2.Set("pt", pt{1, 2})
	_ = s2.Set("list", []string{"a", "<b>"})
	_ = s2.Set("m", map[string]int{"a": 2, "z": 1})
	if changes != 0 {
		t.Errorf("setting equal values after a reload counted %d changes", changes)
	}
	_ = s2.Set("pt", pt{1, 3})
	if changes != 1 {
		t.Errorf("a real change counted %d times", changes)
	}
}
