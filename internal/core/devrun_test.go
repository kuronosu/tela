//go:build teladev

package core

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// A tela.Window is keyed by its element path under tela dev (ADR-184): the
// same tree gives the same keys in another mount (another build), sibling
// windows get different keys, and a nested window's key starts with its
// parent window's.
func TestDevWindowKeys(t *testing.T) {
	keys := func() map[string]string {
		inner := win(WindowSpec{Title: "Inner", OnClose: func() {}, Root: div(Text("inner"))})
		root := Component(func() Node {
			return div(
				win(WindowSpec{Title: "Outer", OnClose: func() {}, Root: div(Text("outer"), inner)}),
				win(WindowSpec{Title: "Other", OnClose: func() {}, Root: div(Text("other"))}),
			)
		})
		x := newWinHarness(t, Options{Root: root})
		out := map[string]string{}
		for _, v := range x.h.views[1:] {
			out[v.slot.spec.Title] = devWindowKey(v.slot.el)
		}
		return out
	}
	a, b := keys(), keys()
	if !reflect.DeepEqual(a, b) || len(a) != 3 {
		t.Fatalf("keys differ between two mounts of one tree:\n%v\n%v", a, b)
	}
	if a["Outer"] == a["Other"] {
		t.Errorf("sibling windows share the key %q", a["Outer"])
	}
	if !strings.HasPrefix(a["Inner"], a["Outer"]+">") {
		t.Errorf("the nested window's key %q does not start with its parent's %q", a["Inner"], a["Outer"])
	}
}
