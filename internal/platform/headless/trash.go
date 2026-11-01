package headless

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// --- the trash (ADR-216) ---

// TrashRecord is a file or folder the app moved to the trash with
// MoveToTrash.
type TrashRecord struct {
	// Path is the path the app gave, cleaned.
	Path string
	// Moved is where the fake put it: a file in the directory of
	// SetTrashDir; "" when no directory was set, and the file stayed where
	// it was.
	Moved string
}

// trashState is the fake trash, guarded by b.mu.
type trashState struct {
	dir     string // SetTrashDir; "" leaves files in place
	err     error  // SetTrashError
	records []TrashRecord
	n       int // names handed out in dir
}

// SetTrashDir makes MoveToTrash move what it is given into dir (a
// directory that exists, on the same file system as the files, such as a
// test's temporary directory), as a trash would take it out of its folder;
// each gets a name of its own there. "" (the default) records the calls
// and leaves the files where they are, so that a headless app driven by
// tela mcp never moves a developer's file. Safe from any goroutine.
func (b *Backend) SetTrashDir(dir string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.trash.dir = dir
}

// SetTrashError makes every MoveToTrash call fail with err (after checking
// its path) until SetTrashError(nil), and nothing is moved: a file in use,
// a folder the user may not change. (A desktop without a trash is
// SetUnsupported(ServiceTrash).) Safe from any goroutine.
func (b *Backend) SetTrashError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.trash.err = err
}

// Trashed returns what the app moved to the trash, in order.
func (b *Backend) Trashed() []TrashRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.trash.records)
}

// MoveToTrash checks the path like every backend, then answers as the
// fake is set: ErrUnsupported, the injected error, an error wrapping
// fs.ErrNotExist for a missing path, else it records the call and moves
// the file into the trash directory, if one was set.
func (d desktop) MoveToTrash(path string) error {
	if _, err := platform.CheckTrashPath(path); err != nil {
		return err
	}
	if !d.supported(ServiceTrash) {
		return platform.ErrUnsupported
	}
	b := d.b
	b.mu.Lock()
	err, dir := b.desktop.trash.err, b.desktop.trash.dir
	b.mu.Unlock()
	if err != nil {
		return err
	}
	clean, _, err := platform.CheckTrash(path)
	if err != nil {
		return err
	}
	rec := TrashRecord{Path: clean}
	if dir != "" {
		b.mu.Lock()
		b.desktop.trash.n++
		n := b.desktop.trash.n
		b.mu.Unlock()
		rec.Moved = filepath.Join(dir, fmt.Sprintf("%d-%s", n, filepath.Base(clean)))
		if err := os.Rename(clean, rec.Moved); err != nil {
			return fmt.Errorf("headless: MoveToTrash: %w", err)
		}
	}
	b.mu.Lock()
	b.desktop.trash.records = append(b.desktop.trash.records, rec)
	b.mu.Unlock()
	return nil
}
