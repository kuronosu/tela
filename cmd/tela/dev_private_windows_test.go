//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildDirPrivate: the shared temp\tela-dev is used, and reused, while
// it is this user's own directory; a junction there (another user's
// directory would do the same) makes each session a fresh tela-dev-* of
// temp, which the hint then names.
func TestBuildDirPrivate(t *testing.T) {
	temp := t.TempDir()
	dir, exclude, err := newBuildDirIn(temp)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(temp, "tela-dev"); exclude != want || filepath.Dir(dir) != want {
		t.Errorf("newBuildDirIn = %s, %s; want a session of %s", dir, exclude, want)
	}
	again, exclude2, err := newBuildDirIn(temp)
	if err != nil {
		t.Fatal(err)
	}
	if exclude2 != exclude || again == dir || filepath.Dir(again) != exclude {
		t.Errorf("second session %s, %s; want another session of %s", again, exclude2, exclude)
	}

	if err := checkPrivateDir(os.Getenv("SystemRoot")); err == nil {
		t.Errorf("checkPrivateDir(%s) accepted a directory of the system", os.Getenv("SystemRoot"))
	} else {
		t.Logf("%%SystemRoot%%: %v", err)
	}

	temp = t.TempDir()
	target := t.TempDir()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(temp, "tela-dev"), target).CombinedOutput()
	if err != nil {
		t.Skipf("cannot make a junction: %v: %s", err, out)
	}
	if err := checkPrivateDir(filepath.Join(temp, "tela-dev")); err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Errorf("checkPrivateDir(junction) = %v, want a reparse point", err)
	}
	dir, exclude, err = newBuildDirIn(temp)
	if err != nil {
		t.Fatal(err)
	}
	if exclude != temp || filepath.Dir(dir) != temp || !strings.HasPrefix(filepath.Base(dir), "tela-dev-") {
		t.Errorf("with a junction: newBuildDirIn = %s, %s; want a tela-dev-* of %s", dir, exclude, temp)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("a session was made through the junction: %v", entries)
	}
}
