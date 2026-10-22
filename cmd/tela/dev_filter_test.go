package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchSet(t *testing.T) {
	mod := t.TempDir()
	other := t.TempDir()
	assets := t.TempDir()
	tmp := filepath.Join(mod, "build-tmp")
	os.MkdirAll(filepath.Join(mod, "nested"), 0o755)
	os.WriteFile(filepath.Join(mod, "nested", "go.mod"), []byte("module nested\n"), 0o644)
	os.MkdirAll(filepath.Join(mod, ".hidden", "emb"), 0o755)

	s := newWatchSet(tmp)
	s.setDeps(depSet{
		modules: []string{mod, other},
		embeds:  []string{filepath.Join(mod, "ui", "logo.png"), filepath.Join(mod, ".hidden", "emb", "x.txt")},
		pkgDirs: []string{mod, filepath.Join(mod, "testdata", "app")},
	})
	s.setAssets([]string{assets})
	j := func(parts ...string) string { return filepath.Join(parts...) }

	for _, c := range []struct {
		path string
		dir  bool
		want bool
	}{
		{j(mod, "main.go"), false, true},
		{j(mod, "pkg", "x.go"), false, true},
		{j(mod, "pkg", "asm_amd64.s"), false, true},
		{j(mod, "pkg", "x_test.go"), false, false},
		{j(mod, "testdata", "x.go"), false, false},
		{j(mod, "pkg", "testdata", "y.go"), false, false},
		{j(mod, "testdata", "app", "main.go"), false, true},
		{j(mod, "go.mod"), false, true},
		{j(mod, "go.sum"), false, true},
		{j(mod, "go.work"), false, true},
		{j(mod, "README.md"), false, false},
		{j(mod, "ui", "logo.png"), false, true},
		{j(mod, "ui", "other.png"), false, false},
		{j(other, "lib.go"), false, true},
		{j(assets, "img", "a.svg"), false, true},
		{j(assets, "img", "a.svg~"), false, false},
		{j(mod, "main.go~"), false, false},
		{j(mod, ".#main.go"), false, false},
		{j(mod, "#main.go#"), false, false},
		{j(mod, ".main.go.swp"), false, false},
		{j(mod, "4913"), false, false},
		{j(mod, "main.go___jb_tmp___"), false, false},
		{j(mod, "pkg"), true, true},
		{"/elsewhere/x.go", false, false},
	} {
		if got := s.relevant(c.path, c.dir); got != c.want {
			t.Errorf("relevant(%s, %v) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}

	for _, c := range []struct {
		path string
		want bool
	}{
		{j(mod, "pkg"), false},
		{j(mod, ".git"), true},
		{j(mod, ".idea"), true},
		{j(mod, "node_modules"), true},
		{tmp, true},
		{j(mod, "nested"), true},
		{j(mod, ".hidden"), false}, // holds an embedded file
		{j(mod, "testdata"), false},
	} {
		if got := s.skipDir(c.path); got != c.want {
			t.Errorf("skipDir(%s) = %v, want %v", c.path, got, c.want)
		}
	}

	roots := s.roots()
	if len(roots) != 3 || roots[0] != mod || roots[1] != other || roots[2] != assets {
		t.Errorf("roots %v", roots)
	}
	s.setAssets([]string{mod}) // inside a module: not a separate root
	if roots := s.roots(); len(roots) != 2 {
		t.Errorf("roots with an asset dir equal to a module %v", roots)
	}
}
