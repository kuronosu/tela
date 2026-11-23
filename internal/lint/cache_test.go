package lint

import (
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A Cache (cache.go) only saves work: every run through one must report
// what a run without it reports. These tests change a module between runs
// of one cache, the way an editor's session does, and compare each result
// with a fresh run's.

// cacheModule writes a module whose package dep is context (a comment
// names ClassAliases), so that a run of ./app with the module as context
// checks dep from source and caches it. dep imports base of another module
// (replaced by a folder), which is read from export data.
func cacheModule(t *testing.T) string {
	t.Helper()
	dir := writeModule(t, map[string]string{
		"dep/dep.go":         "// Package dep is context: it names ClassAliases.\npackage dep\n\nimport \"example.com/other/base\"\n\nfunc F() base.T { return 1 }\n",
		"dep/more.go":        "package dep\n\nfunc G() int { return 2 }\n",
		"dep/n.go":           "package dep\n\nconst N = 1\n",
		"app/app.go":         "package app\n\nimport \"example.com/m/dep\"\n\nfunc Use() int { return int(dep.F()) + dep.G() }\n\nvar _ [dep.N]int = [1]int{}\n",
		"other/go.mod":       "module example.com/other\n\ngo 1.22\n",
		"other/base/base.go": "package base\n\ntype T int\n",
	})
	writeFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n\nrequire example.com/other v0.0.0\n\nreplace example.com/other => ./other\n")
	return dir
}

func cacheOptions(dir string) Options {
	return Options{Dir: dir, Patterns: []string{"./app"}, Context: []string{"./..."}}
}

// sameAsFresh runs opts without a cache and with c, fails the test when the
// results differ, and returns the load errors of the cached run.
func sameAsFresh(t *testing.T, c *Cache, opts Options) string {
	t.Helper()
	opts.Cache = nil
	fresh, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.Cache = c
	got, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Diagnostics, fresh.Diagnostics) || !reflect.DeepEqual(got.LoadErrors, fresh.LoadErrors) ||
		got.Packages != fresh.Packages || !reflect.DeepEqual(got.ClassStrings, fresh.ClassStrings) {
		t.Fatalf("the cached run differs from a fresh one:\ncached: %v %v %d\nfresh:  %v %v %d",
			got.Diagnostics, got.LoadErrors, got.Packages, fresh.Diagnostics, fresh.LoadErrors, fresh.Packages)
	}
	return strings.Join(got.LoadErrors, "\n")
}

func (c *Cache) reusedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reused
}

func TestCacheReusesUnchangedContext(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir := cacheModule(t)
	c := NewCache()
	if got := sameAsFresh(t, c, cacheOptions(dir)); got != "" {
		t.Fatalf("load errors: %s", got)
	}
	before := c.reusedCount()
	if got := sameAsFresh(t, c, cacheOptions(dir)); got != "" {
		t.Fatalf("load errors: %s", got)
	}
	if c.reusedCount() == before {
		t.Fatal("the second run checked dep again: the cache is not used, and the other tests prove nothing")
	}
}

// Every kind of change a session sees: a file of the context that the
// linted package imports edited on disk (a function, a constant), deleted
// and written back, an overlay over it, a new import, and a change of what
// it imports from export data.
func TestCacheSeesChanges(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir := cacheModule(t)
	c := NewCache()
	opts := cacheOptions(dir)
	step := func(what, want string) {
		t.Helper()
		got := sameAsFresh(t, c, opts)
		if want == "" && got != "" || !strings.Contains(got, want) {
			t.Fatalf("%s: load errors %q, want %q", what, got, want)
		}
	}
	step("first run", "")

	writeFile(t, dir, "dep/dep.go", "// Package dep is context: it names ClassAliases.\npackage dep\n\nimport \"example.com/other/base\"\n\nfunc H() base.T { return 1 }\n")
	step("F renamed on disk", "undefined: dep.F")
	writeFile(t, dir, "dep/dep.go", "// Package dep is context: it names ClassAliases.\npackage dep\n\nimport \"example.com/other/base\"\n\nfunc F() base.T { return 1 }\n")
	step("F restored", "")

	if err := os.Remove(filepath.Join(dir, "dep", "more.go")); err != nil {
		t.Fatal(err)
	}
	step("more.go deleted", "undefined: dep.G")
	writeFile(t, dir, "dep/more.go", "package dep\n\nfunc G() int { return 2 }\n")
	step("more.go written back", "")

	writeFile(t, dir, "dep/n.go", "package dep\n\nconst N = 2\n")
	step("a constant changed", "[1]int{}")
	writeFile(t, dir, "dep/n.go", "package dep\n\nconst N = 1\n")
	step("the constant restored", "")

	depFile := filepath.Join(dir, "dep", "more.go")
	opts.Overlay = map[string][]byte{depFile: []byte("package dep\n\nfunc K() int { return 2 }\n")}
	step("G renamed in an overlay", "undefined: dep.G")
	opts.Overlay = nil
	step("overlay gone", "")

	// An import that only the overlay has: the run needs other export
	// data than the last one listed.
	opts.Overlay = map[string][]byte{filepath.Join(dir, "app", "extra.go"): []byte("package app\n\nimport \"strings\"\n\nvar _ = strings.ToUpper(1)\n")}
	step("a new import in an overlay", "cannot use 1")
	opts.Overlay = nil
	step("overlay gone again", "")

	writeFile(t, dir, "other/base/base.go", "package base\n\ntype T struct{}\n")
	step("base.T a struct in export data", "cannot convert")
	writeFile(t, dir, "other/base/base.go", "package base\n\ntype T int\n")
	step("base.T restored", "")
}

// A file read long after it was written is trusted by its size and time;
// a later write changes the time, even when the size stays.
func TestCacheTrustedStampSeesAWrite(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir := cacheModule(t)
	more := filepath.Join(dir, "dep", "more.go")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(more, old, old); err != nil {
		t.Fatal(err)
	}
	c := NewCache()
	if got := sameAsFresh(t, c, cacheOptions(dir)); got != "" {
		t.Fatalf("load errors: %s", got)
	}
	if st, ok := c.stamp(more); !ok || !st.trusted {
		t.Fatalf("the stamp of a file an hour old is not trusted: %+v", st)
	}
	writeFile(t, dir, "dep/more.go", "package dep\n\nfunc K() int { return 2 }\n") // the same size
	if got := sameAsFresh(t, c, cacheOptions(dir)); !strings.Contains(got, "undefined: dep.G") {
		t.Fatalf("after G was renamed, load errors = %q", got)
	}
}

// The linted package's syntax leaves the file set after each run: an
// editor lints at every pause, and the cached syntax must not grow with it.
func TestCacheFileSetDoesNotGrow(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir := cacheModule(t)
	c := NewCache()
	count := func() int {
		n := 0
		c.currentEpoch().fset.Iterate(func(*token.File) bool { n++; return true })
		return n
	}
	for i := range 3 {
		app := filepath.Join(dir, "app", "app.go")
		opts := cacheOptions(dir)
		opts.Overlay = map[string][]byte{app: []byte("package app\n\nimport \"example.com/m/dep\"\n\nfunc Use() int { return int(dep.F()) + dep.G() + " + string(rune('1'+i)) + " }\n")}
		opts.Cache = c
		if _, err := Run(opts); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(); n != 3 {
		t.Errorf("the file set holds %d files after three runs, want dep's 3", n)
	}
}
