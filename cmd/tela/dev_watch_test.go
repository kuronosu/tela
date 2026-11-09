package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/watch"
)

// TestDevWatcherReloads runs the supervisor with a real watcher on a
// temporary module: saving a .go file reloads, a test file does not, and an
// asset directory the child reports is watched. A save may come as two
// batches (its records in two reads, more than the debounce apart, on
// Windows), so the reloads are told apart by their causes, the verbose
// "changed:" lines, rather than by their numbers.
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
	h := newDevHarness(t, devOptions{Verbose: true}, func(c *supervisorConfig) {
		c.watchSet = set
		c.setRoots = func(roots []string) { w.SetRoots(roots) }
	})
	go h.s.forward(w)
	h.expect("first start", 1, 0, 0)

	os.WriteFile(filepath.Join(mod, "main.go"), []byte("package main // edited\n"), 0o644)
	h.expect("reload after a save", h.reloadedBy("main.go"), 0, 0)

	os.WriteFile(filepath.Join(mod, "main_test.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "notes.md"), []byte("x"), 0o644)
	time.Sleep(300 * time.Millisecond) // the time to (wrongly) reload

	h.s.push(evAssets{[]string{assets}})
	deadline := time.Now().Add(devWait)
	for len(w.Roots()) != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("roots %v", w.Roots())
		}
		time.Sleep(time.Millisecond)
	}
	os.WriteFile(filepath.Join(assets, "logo.svg"), []byte("<svg/>"), 0o644)
	h.expect("reload after an asset changed", h.reloadedBy("logo.svg"), 0, 0)

	for _, l := range strings.Split(h.out.String(), "\n") {
		if strings.HasPrefix(l, "tela dev: changed: ") && (strings.Contains(l, "main_test.go") || strings.Contains(l, "notes.md")) {
			t.Errorf("an irrelevant change started a build: %s", l)
		}
	}
}

var buildDone = regexp.MustCompile(`tela dev: build (\d+) done in`)

// reloadedBy waits for a batch that names a file called name, and returns
// the build that it started: the first one done after its "changed:" line
// (a build superseded before it is done logs nothing).
func (h *devHarness) reloadedBy(name string) int {
	h.t.Helper()
	deadline := time.Now().Add(devWait)
	for {
		out := h.out.String()
		at := 0
		for _, l := range strings.SplitAfter(out, "\n") {
			if strings.HasPrefix(l, "tela dev: changed: ") && strings.Contains(l, name) {
				if m := buildDone.FindStringSubmatch(out[at:]); m != nil {
					gen, _ := strconv.Atoi(m[1])
					return gen
				}
				break
			}
			at += len(l)
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("no build done after a change of %s; output:\n%s", name, out)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
