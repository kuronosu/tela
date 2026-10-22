package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/watch"
)

// TestDevWatcherReloads runs the supervisor with a real watcher on a
// temporary module: saving a .go file reloads, a test file does not, and an
// asset directory the child reports is watched.
func TestDevWatcherReloads(t *testing.T) {
	mod, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/w\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "main.go"), []byte("package main\n"), 0o644)
	assets, _ := filepath.EvalSymlinks(t.TempDir())

	set := newWatchSet(t.TempDir())
	set.setDeps(depSet{modules: []string{mod}})
	w, err := watch.New(watch.Options{Filter: set.relevant, SkipDir: set.skipDir, Debounce: 50 * time.Millisecond}, set.roots()...)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) {
		c.watchSet = set
		c.setRoots = func(roots []string) { w.SetRoots(roots) }
	})
	go h.s.forward(w)
	h.expect("first start", 1, 0, 0)

	os.WriteFile(filepath.Join(mod, "main.go"), []byte("package main // edited\n"), 0o644)
	h.expect("reload after a save", 2, 0, 0)

	os.WriteFile(filepath.Join(mod, "main_test.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "notes.md"), []byte("x"), 0o644)
	h.still("after irrelevant changes", 2, 0, 0, 300*time.Millisecond)

	h.s.push(evAssets{[]string{assets}})
	deadline := time.Now().Add(devWait)
	for len(w.Roots()) != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("roots %v", w.Roots())
		}
		time.Sleep(time.Millisecond)
	}
	os.WriteFile(filepath.Join(assets, "logo.svg"), []byte("<svg/>"), 0o644)
	h.expect("reload after an asset changed", 3, 0, 0)
}
