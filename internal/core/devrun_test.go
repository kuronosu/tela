//go:build teladev

package core

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

// The child runtime reports the directories of os.DirFS file systems to the
// supervisor, which watches them (ADR-114).
func TestDevDirRoot(t *testing.T) {
	dir := t.TempDir()
	sub, err := fs.Sub(os.DirFS(dir), "img/icons")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		fsys fs.FS
		want string
	}{
		{"os.DirFS", os.DirFS(dir), dir},
		{"fs.Sub of os.DirFS", sub, filepath.Join(dir, "img", "icons")},
		{"embedded or in memory", fstest.MapFS{"a.png": {}}, ""},
		{"fs.Sub of an in-memory file system", mustSub(t, fstest.MapFS{"a/b.png": {}}, "a"), ""},
	} {
		if got := devDirRoot(reflect.ValueOf(c.fsys)); got != c.want {
			t.Errorf("%s: root %q, want %q", c.name, got, c.want)
		}
	}
}

func mustSub(t *testing.T, fsys fs.FS, dir string) fs.FS {
	t.Helper()
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestDevSeedCounts(t *testing.T) {
	got := devSeedCounts(DevSeedReport{Restored: 4, Fields: 1, Scrolls: 2, Focus: true, Remounted: []string{"Sidebar: hook 2 is new"}, Unused: 3})
	if got.Kept != 8 || got.Reset != 1 {
		t.Errorf("counts %+v, want 8 kept and 1 reset", got)
	}
}
