package update

import (
	"os"
	"path/filepath"
	"testing"
)

// Windows lets a running executable be renamed (not deleted or
// overwritten), which the zip folder's Apply and rollback rely on: the
// test binary renames itself aside and back.
func TestRenameRunningExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(exe); err == nil {
		t.Fatal("the running executable was deleted; the premise is wrong")
	}
	aside := filepath.Join(filepath.Dir(exe), ".update-test-aside.exe")
	if err := os.Rename(exe, aside); err != nil {
		t.Fatalf("renaming the running executable: %v", err)
	}
	if err := os.Rename(aside, exe); err != nil {
		t.Fatalf("renaming it back: %v", err)
	}
}
