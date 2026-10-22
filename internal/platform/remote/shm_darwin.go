//go:build darwin

package remote

import (
	"fmt"
	"os"
)

// newShmFile returns an unlinked temporary file of size bytes: macOS has
// no memfd, and shm_open needs libc. The descriptor keeps the file alive
// after the unlink, and the page cache backs both mappings.
func newShmFile(size int) (*os.File, error) {
	f, err := os.CreateTemp("", "tela-remote-*")
	if err != nil {
		return nil, fmt.Errorf("remote: shared buffer: %w", err)
	}
	os.Remove(f.Name())
	if err := f.Truncate(int64(size)); err != nil {
		f.Close()
		return nil, fmt.Errorf("remote: shared buffer: %w", err)
	}
	return f, nil
}
