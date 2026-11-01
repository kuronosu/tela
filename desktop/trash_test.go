package desktop_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// MoveToTrash moves the file out of its place in telatest (into the test's
// own folder), reports a missing file, an unsupported trash and an injected
// failure, and leaves the file in place when it fails.
func TestMoveToTrash(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(file, []byte("milk"), 0o644); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 1)
	app := telatest.MountFunc(t, func() tela.Node {
		return Button(OnClick(func() {
			go func() { results <- desktop.MoveToTrash(file) }()
		}), Text("Trash"))
	})
	trash := func() error {
		t.Helper()
		app.Click(telatest.ByRole("button", "Trash"))
		return <-results
	}

	app.Unsupported(telatest.Trash)
	if err := trash(); !errors.Is(err, desktop.ErrUnsupported) {
		t.Errorf("MoveToTrash without a trash = %v, want ErrUnsupported", err)
	}
	app.Unsupported()
	busy := errors.New("the file is in use")
	app.FailTrash(busy)
	if err := trash(); !errors.Is(err, busy) {
		t.Errorf("MoveToTrash with FailTrash = %v", err)
	}
	app.FailTrash(nil)
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("a failed call moved the file: %v", err)
	}
	if got := app.Trashed(); len(got) != 0 {
		t.Errorf("failed calls were recorded: %v", got)
	}

	if err := trash(); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file is still in its place (%v)", err)
	}
	if got := app.Trashed(); len(got) != 1 || got[0] != file {
		t.Errorf("Trashed = %v, want [%s]", got, file)
	}
	if err := trash(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveToTrash of a missing file = %v, want fs.ErrNotExist", err)
	}
}

func TestMoveToTrashWithoutApp(t *testing.T) {
	if err := desktop.MoveToTrash(filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("MoveToTrash without a running app = nil")
	}
}
