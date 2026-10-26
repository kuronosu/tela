package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFingerprint(t *testing.T) {
	root := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a")
	write("sub/b.go", "package b")
	write("notes.txt", "hello")
	write(".git/c.go", "package c")
	opts := Options{Filter: goFilter, SkipDir: skipHidden}
	fp := func() uint64 { return Fingerprint(opts, root) }

	base := fp()
	if fp() != base {
		t.Fatal("two fingerprints of the same tree differ")
	}
	write("notes.txt", "a file the filter rejects")
	write(".git/c.go", "package c // in a skipped directory")
	if fp() != base {
		t.Error("a change outside the filter changed the fingerprint")
	}

	write("sub/b.go", "package b // saved")
	edited := fp()
	if edited == base {
		t.Error("an edit did not change the fingerprint")
	}
	// The same size with a new modification time changes it too.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(root, "a.go"), later, later); err != nil {
		t.Fatal(err)
	}
	touched := fp()
	if touched == edited {
		t.Error("a new modification time did not change the fingerprint")
	}
	write("new.go", "package a")
	added := fp()
	if added == touched {
		t.Error("a new file did not change the fingerprint")
	}
	if err := os.Remove(filepath.Join(root, "new.go")); err != nil {
		t.Fatal(err)
	}
	if fp() != touched {
		t.Error("removing the new file did not give the fingerprint before it")
	}
	if Fingerprint(opts, root, root) != touched {
		t.Error("a root given twice changed the fingerprint")
	}
}
