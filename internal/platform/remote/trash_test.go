package remote

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TestMoveToTrash checks that the trash is the host backend's (ADR-216),
// served off its UI thread, with its errors kept.
func TestMoveToTrash(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	d := a.b.Desktop() // kept and used off the UI thread
	trash := t.TempDir()
	p.hb.SetTrashDir(trash)
	src := t.TempDir()
	file := filepath.Join(src, "todo.md")
	if err := os.WriteFile(file, []byte("milk"), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error)
	go func() { done <- d.MoveToTrash(file) }()
	if err := <-done; err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	recs := p.hb.Trashed()
	if len(recs) != 1 || recs[0].Path != file || filepath.Dir(recs[0].Moved) != trash {
		t.Fatalf("the host trashed %+v", recs)
	}
	if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file is still in its place (%v)", err)
	}
	if err := d.MoveToTrash(file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveToTrash of a missing file = %v, want fs.ErrNotExist", err)
	}
	if err := d.MoveToTrash("todo.md"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("MoveToTrash of a relative path = %v, want fs.ErrInvalid", err)
	}
	busy := errors.New("the file is in use")
	p.hb.SetTrashError(busy)
	if err := d.MoveToTrash(src); err == nil || err.Error() != busy.Error() {
		t.Errorf("MoveToTrash with an injected error = %v", err)
	}
	p.hb.SetTrashError(nil)
	p.hb.SetUnsupported(headless.ServiceTrash)
	if err := d.MoveToTrash(src); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("MoveToTrash without a trash = %v, want ErrUnsupported", err)
	}
	if len(p.hb.Trashed()) != 1 {
		t.Errorf("failed calls were recorded: %+v", p.hb.Trashed())
	}
}
