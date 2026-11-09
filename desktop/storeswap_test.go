package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// canExchange reports whether the file system of the folder dir exchanges
// files atomically (exchangeFiles).
func canExchange(t *testing.T, dir string) bool {
	t.Helper()
	a, b := filepath.Join(dir, "swap-a"), filepath.Join(dir, "swap-b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte(p), 0o600); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(p)
	}
	err := exchangeFiles(a, b)
	if err != nil && !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("exchangeFiles: %v, want nil or an unsupported error", err)
	}
	if err == nil {
		if got, _ := os.ReadFile(a); string(got) != b {
			t.Fatalf("after the exchange %s holds %q", a, got)
		}
	}
	return err == nil
}

// TestStoreWriteAtReplace (ADR-262): another program that writes the file
// in the moment between the write's check and its replace is not lost
// where the replace is an atomic exchange (Linux, macOS): the version
// replaced is read back, merged and written again. A file the other
// program creates when there was none is not replaced. Elsewhere that
// moment stays open (Windows has no exchange).
func TestStoreWriteAtReplace(t *testing.T) {
	dir := t.TempDir()
	if !canExchange(t, dir) {
		t.Skipf("%s on %s cannot exchange files atomically: the moment before the rename stays open", dir, runtime.GOOS)
	}
	for _, c := range []struct {
		name, before, theirs string
	}{
		{"replaced", `{"a": 1}`, `{"a": 1, "c": 3}`},
		{"created", "", `{"c": 3}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name+".json")
			if c.before != "" {
				if err := os.WriteFile(path, []byte(c.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			s, _ := OpenStore(path)
			s.mu.Lock()
			s.delay = time.Hour // written by Flush only
			s.mu.Unlock()
			if err := s.Set("b", 2); err != nil {
				t.Fatal(err)
			}
			tries := 0
			h := func(p string) {
				if p != path {
					return // another test's store
				}
				if tries++; tries == 1 {
					if err := os.WriteFile(path, []byte(c.theirs), 0o600); err != nil {
						t.Error(err)
					}
				}
			}
			atReplace.Store(&h)
			defer atReplace.Store(nil)
			if err := s.Flush(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`"b": 2`, `"c": 3`} {
				if !strings.Contains(string(data), want) {
					t.Errorf("the file lacks %s after the write:\n%s", want, data)
				}
			}
			if tries != 2 {
				t.Errorf("%d tries, want 2 (one merge)", tries)
			}
			if !s.Has("c") {
				t.Error("the store lacks the other program's key")
			}
			if tmp, _ := filepath.Glob(filepath.Join(dir, "."+c.name+".json.*.tmp")); len(tmp) != 0 {
				t.Errorf("temporary files left: %q", tmp)
			}
		})
	}
}

// TestStoreSeesSameStatWrite (ADR-262): a write of another program that
// keeps the file's size, modification time and inode (in place, with its
// time set back) is a change: versions of the file are compared by
// content.
func TestStoreSeesSameStatWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{"a": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	read := stampOf([]byte(`{"a": 1}`))
	if changedSince(path, read) {
		t.Fatal("the version read counts as changed")
	}
	if err := os.WriteFile(path, []byte(`{"a": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.Stat(path); now.Size() != fi.Size() || !now.ModTime().Equal(fi.ModTime()) || !os.SameFile(now, fi) {
		t.Fatalf("the rewrite changed the stat: %d bytes at %v", now.Size(), now.ModTime())
	}
	if !changedSince(path, read) {
		t.Error("a write that keeps the stat is not a change")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !changedSince(path, read) {
		t.Error("a removed file is not a change")
	}
	if changedSince(path, fileStamp{ok: true}) {
		t.Error("a file still absent is a change")
	}

	// Through a write: the store merges the other program's value.
	if err := os.WriteFile(path, []byte(`{"a": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(path)
	s, _ := OpenStore(path)
	s.mu.Lock()
	s.delay = time.Hour
	s.mu.Unlock()
	if err := s.Set("b", 2); err != nil {
		t.Fatal(err)
	}
	first := true
	h := func(p string) {
		if p != path || !first {
			return
		}
		first = false
		// After the write's reload: only the comparison can see it.
		if err := os.WriteFile(path, []byte(`{"a": 7}`), 0o600); err != nil {
			t.Error(err)
		}
		if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
			t.Error(err)
		}
	}
	beforeRename.Store(&h)
	defer beforeRename.Store(nil)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	var a int
	if s.Get("a", &a); a != 7 {
		t.Errorf("a = %d, want the other program's 7", a)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"a": 7`) || !strings.Contains(string(data), `"b": 2`) {
		t.Errorf("the file after the write:\n%s", data)
	}
}

// An exchange or exclusive rename the file system cannot do is
// unsupported, so the write falls back to a plain rename; a missing or
// existing name keeps its own error.
func TestSwapErrors(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		if err := exchangeFiles("a", "b"); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("exchangeFiles on %s = %v, want unsupported", runtime.GOOS, err)
		}
		return
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exchangeFiles(a, b); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("exchange with a missing file = %v, want ErrNotExist", err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplace(a, b); !errors.Is(err, os.ErrExist) && !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("exclusive rename over a file = %v, want ErrExist", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "b" {
		t.Errorf("the exclusive rename replaced the file: %q", got)
	}
	if err := (unsupportedSwap{errors.New("x")}); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("%v is not unsupported", err)
	}
}
