package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A dependency is read from export data kept for the rest of the process
// (export.go). These tests change a dependency between runs of one
// process and check that no run is served the package of an earlier one.

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadErrors lints the packages in dir (in the process, so the cache of
// the earlier runs is the one in use) and returns what could not be loaded.
func loadErrors(t *testing.T, dir string, patterns ...string) string {
	t.Helper()
	res, err := Run(Options{Dir: dir, Patterns: patterns})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(res.LoadErrors, "\n")
}

// readsFromExport reports whether a run over pattern in dir imports the
// package with the import path from export data.
func readsFromExport(t *testing.T, dir, pattern, path string) bool {
	t.Helper()
	pkgs, err := goList(dir, "", []string{pattern})
	if err != nil {
		t.Fatal(err)
	}
	l := newLoader(pkgs)
	l.useExports(dir)
	return l.exported[path]
}

const staleApp = `package app

import "example.com/m/dep"

func Use() int { return dep.F() }
`

func TestDependencyChangeIsNotServedStale(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"dep/dep.go": "package dep\n\nfunc F() int { return 1 }\n",
		"app/app.go": staleApp,
	})
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("before the change: %s", got)
	}
	if !readsFromExport(t, dir, "./app", "example.com/m/dep") {
		t.Fatal("dep is not read from export data, the test would prove nothing")
	}
	// The package that the first run read is gone: F is renamed.
	writeFile(t, dir, "dep/dep.go", "package dep\n\nfunc G() int { return 1 }\n")
	if got := loadErrors(t, dir, "./app"); !strings.Contains(got, "undefined: dep.F") {
		t.Fatalf("after renaming F, load errors = %q; want undefined: dep.F", got)
	}
	// And back: the first version is read again, not an error left over.
	writeFile(t, dir, "dep/dep.go", "package dep\n\nfunc F() int { return 1 }\n")
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("after restoring F: %s", got)
	}
}

// What a dependency imports counts too: its types refer to the objects of
// the packages below it.
func TestTransitiveDependencyChangeIsNotServedStale(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"base/base.go": "package base\n\ntype T int\n",
		"dep/dep.go":   "package dep\n\nimport \"example.com/m/base\"\n\nfunc F() base.T { return 1 }\n",
		"app/app.go":   "package app\n\nimport \"example.com/m/dep\"\n\nfunc Use() int { return int(dep.F()) }\n",
	})
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("before the change: %s", got)
	}
	writeFile(t, dir, "base/base.go", "package base\n\ntype T struct{}\n")
	if got := loadErrors(t, dir, "./app"); !strings.Contains(got, "cannot convert") {
		t.Fatalf("after T became a struct, load errors = %q; want a conversion error", got)
	}
}

// Build constraints and GOOS decide which files a package has: the cached
// package of one configuration is not another's.
func TestBuildConfigurationIsPartOfTheCache(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"dep/dep.go":         "package dep\n",
		"dep/tagged.go":      "//go:build lintcachetag\n\npackage dep\n\nfunc F() int { return 1 }\n",
		"dep/dep_windows.go": "package dep\n\nfunc W() int { return 1 }\n",
		"app/app.go":         staleApp,
		"app/win/win.go":     "package win\n\nimport \"example.com/m/dep\"\n\nfunc Use() int { return dep.W() }\n",
	})
	if got := loadErrors(t, dir, "./app"); !strings.Contains(got, "undefined: dep.F") {
		t.Fatalf("without the tag, load errors = %q; want undefined: dep.F", got)
	}
	t.Setenv("GOFLAGS", "-tags=lintcachetag")
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("with the tag: %s", got)
	}
	t.Setenv("GOFLAGS", "")
	if got := loadErrors(t, dir, "./app"); !strings.Contains(got, "undefined: dep.F") {
		t.Fatalf("without the tag again, load errors = %q; want undefined: dep.F", got)
	}

	t.Setenv("GOOS", "windows")
	if got := loadErrors(t, dir, "./app/win"); got != "" {
		t.Fatalf("GOOS=windows: %s", got)
	}
	t.Setenv("GOOS", "linux")
	if got := loadErrors(t, dir, "./app/win"); !strings.Contains(got, "undefined: dep.W") {
		t.Fatalf("GOOS=linux, load errors = %q; want undefined: dep.W", got)
	}
}

// A dependency that does not compile has no export data: it is checked
// from source, its errors are ignored as before, and the run goes on.
func TestDependencyThatDoesNotCompileIsChecked(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"dep/dep.go": "package dep\n\nfunc F() int { return undefinedName }\n",
		"app/app.go": staleApp,
	})
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("a dependency with an error in a body: %s", got)
	}
	if readsFromExport(t, dir, "./app", "example.com/m/dep") {
		t.Error("dep does not compile and has no export data")
	}
	writeFile(t, dir, "dep/dep.go", "package dep\n\nfunc F() int { return 1 }\n")
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("after fixing it: %s", got)
	}
}

// The export file of a package whose files were not modified lately is
// remembered for the process (exportMemo): the next run does not ask the
// go command for it, and a write gives the file a new identity, so the run
// after it does.
func TestExportMemoSeesAWrite(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"dep/dep.go": "package dep\n\nfunc F() int { return 1 }\n",
		"app/app.go": staleApp,
	})
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "dep", "dep.go"), old, old); err != nil {
		t.Fatal(err)
	}
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("first run: %s", got)
	}
	before := exportsListed.Load()
	if got := loadErrors(t, dir, "./app"); got != "" {
		t.Fatalf("second run: %s", got)
	}
	if n := exportsListed.Load() - before; n != 0 {
		t.Errorf("the second run listed the export data of %d packages, want none", n)
	}
	writeFile(t, dir, "dep/dep.go", "package dep\n\nfunc G() int { return 1 }\n") // the same size
	if got := loadErrors(t, dir, "./app"); !strings.Contains(got, "undefined: dep.F") {
		t.Fatalf("after renaming F, load errors = %q; want undefined: dep.F", got)
	}
}
