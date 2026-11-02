package platform

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckTrash(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "a.md", filepath.Join("..", "a.md"), file + "\x00", filepath.VolumeName(dir) + string(filepath.Separator)} {
		if _, err := CheckTrashPath(p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("CheckTrashPath(%q) = %v, want fs.ErrInvalid", p, err)
		}
	}
	got, err := CheckTrashPath(file + string(filepath.Separator) + ".")
	if err != nil || got != file {
		t.Errorf("CheckTrashPath of an unclean path = %q, %v; want %q", got, err, file)
	}
	clean, fi, err := CheckTrash(file)
	if err != nil || clean != file || fi.Name() != "a.md" {
		t.Errorf("CheckTrash = %q, %v, %v", clean, fi, err)
	}
	if _, _, err := CheckTrash(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("CheckTrash of a missing file = %v, want fs.ErrNotExist", err)
	}
	var d Desktop = UnsupportedDesktop{}
	if err := d.MoveToTrash(file); !errors.Is(err, ErrUnsupported) {
		t.Errorf("UnsupportedDesktop.MoveToTrash = %v, want ErrUnsupported", err)
	}
	if err := d.MoveToTrash("a.md"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("UnsupportedDesktop.MoveToTrash of a relative path = %v, want fs.ErrInvalid", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("the file went away: %v", err)
	}
}
