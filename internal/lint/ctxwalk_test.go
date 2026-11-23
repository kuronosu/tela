package lint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The context walk (ctxwalk.go) finds the packages that may declare aliases
// without listing the module; it must find what listing "./..." finds:
// every directory the go command matches and whose package mentions
// ClassAliases or SchemeColor, and none it does not match (testdata, names
// starting with "." or "_", vendor, a nested module, a symbolic link).
func TestContextWalkMatchesTheGoCommand(t *testing.T) {
	parallelLint(t)
	needGo(t)
	const mention = "// It names ClassAliases.\n"
	dir := writeModule(t, map[string]string{
		"a/a.go":             "package a\n\n" + mention,
		"a/b/b.go":           "package b\n\n// It names SchemeColor.\n",
		"plain/p.go":         "package plain\n",
		"onlytest/x_test.go": "package onlytest\n\n" + mention,
		"onlytest/x.go":      "package onlytest\n",
		"tags/t.go":          "//go:build never\n\npackage tags\n\n" + mention,
		"tags/u.go":          "package tags\n",
		"testdata/td/td.go":  "package td\n\n" + mention,
		"_under/u.go":        "package under\n\n" + mention,
		".dot/d.go":          "package dot\n\n" + mention,
		"vendor/v/v.go":      "package v\n\n" + mention,
		"nested/go.mod":      "module example.com/nested\n\ngo 1.22\n",
		"nested/n.go":        "package nested\n\n" + mention,
		"ignored/_i.go":      "package ignored\n\n" + mention,
		"ignored/i.go":       "package ignored\n",
	})
	if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "link")); err != nil {
		t.Logf("no symbolic link (%v): that case is not checked", err)
	}
	r := &run{opts: Options{Context: []string{"./..."}}, dir: dir}

	byGo := contextPackages(t, dir, []string{"./..."})
	dirs, ok := r.contextDirs()
	if !ok {
		t.Fatal("./... is not walked")
	}
	byWalk := contextPackages(t, dir, dirs)
	if !slices.Equal(byWalk, byGo) {
		t.Errorf("the walk finds %v (directories %v), go list ./... finds %v", byWalk, dirs, byGo)
	}
	if want := []string{"example.com/m/a", "example.com/m/a/b"}; !slices.Equal(byGo, want) {
		t.Errorf("go list finds %v, the test expects %v", byGo, want)
	}

	// Only directory trees are walked; anything else is listed as given.
	for pat, walked := range map[string]bool{"./a/...": true, "../" + filepath.Base(dir) + "/...": true, dir + "/...": true, "example.com/m/...": false, "./a": false, "./a/.../b": false} {
		if _, ok := (&run{opts: Options{Context: []string{pat}}, dir: dir}).contextDirs(); ok != walked {
			t.Errorf("%s: walked %v, want %v", pat, ok, walked)
		}
	}
}

// contextPackages lists patterns in dir and returns the packages that
// mentionsAliases takes, sorted.
func contextPackages(t *testing.T, dir string, patterns []string) []string {
	t.Helper()
	if len(patterns) == 0 {
		return nil
	}
	pkgs, err := goList(dir, "", patterns)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range pkgs {
		if !p.DepOnly && !p.Standard && p.Module != nil && mentionsAliases(p, nil) {
			out = append(out, p.ImportPath)
		}
	}
	slices.Sort(out)
	return out
}

// In Tela's module only the examples and fixtures are context (telaOwn),
// and the walk names packages by the module's root, not by the pattern's.
func TestContextWalkInTelasModule(t *testing.T) {
	parallelLint(t)
	needGo(t)
	const mention = "// It names ClassAliases.\n"
	dir := writeModule(t, map[string]string{
		"kit/kit.go":            "package kit\n\n" + mention,
		"examples/x/x.go":       "package x\n\n" + mention,
		"examples/x/sub/s.go":   "package sub\n\n" + mention,
		"examples/plain/p.go":   "package plain\n",
		"examples/x/lib/lib.go": "package lib\n",
	})
	writeFile(t, dir, "go.mod", "module "+modulePath+"\n\ngo 1.22\n")
	for _, pat := range []string{"./...", "./examples/...", "./examples/x/..."} {
		byGo := contextPackages(t, dir, []string{pat})
		dirs, ok := (&run{opts: Options{Context: []string{pat}}, dir: dir}).contextDirs()
		if !ok {
			t.Fatalf("%s is not walked", pat)
		}
		if byWalk := contextPackages(t, dir, dirs); !slices.Equal(byWalk, byGo) || len(byGo) != 2 {
			t.Errorf("%s: the walk finds %v, go list finds %v; want examples/x and examples/x/sub", pat, byWalk, byGo)
		}
	}
}
