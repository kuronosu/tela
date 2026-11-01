package headless

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestTrashFake(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	src := t.TempDir()
	a := filepath.Join(src, "a.md")
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without a trash directory the call is recorded and the file stays,
	// so a headless app (tela mcp) never moves a developer's file.
	if err := d.MoveToTrash(a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a); err != nil {
		t.Errorf("the file moved without a trash directory: %v", err)
	}

	// With one, it moves there under a name of its own.
	trash := t.TempDir()
	b.SetTrashDir(trash)
	if err := d.MoveToTrash(a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file is still in its place (%v)", err)
	}
	recs := b.Trashed()
	if len(recs) != 2 || recs[0] != (TrashRecord{Path: a}) || recs[1].Path != a || filepath.Dir(recs[1].Moved) != trash {
		t.Fatalf("Trashed = %+v", recs)
	}
	if got, err := os.ReadFile(recs[1].Moved); err != nil || string(got) != "a" {
		t.Errorf("the trashed file = %q, %v", got, err)
	}

	// Errors: a missing file, a relative path, an injected error, no
	// trash. None is recorded.
	if err := d.MoveToTrash(a); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveToTrash of a missing file = %v", err)
	}
	if err := d.MoveToTrash("a.md"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("MoveToTrash of a relative path = %v", err)
	}
	busy := errors.New("in use")
	b.SetTrashError(busy)
	if err := d.MoveToTrash(src); err != busy {
		t.Errorf("MoveToTrash with an injected error = %v", err)
	}
	b.SetTrashError(nil)
	b.SetUnsupported(ServiceTrash)
	if err := d.MoveToTrash(src); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("MoveToTrash without a trash = %v", err)
	}
	if n := len(b.Trashed()); n != 2 {
		t.Errorf("%d calls recorded, want 2", n)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("a failed call moved the folder: %v", err)
	}
}
