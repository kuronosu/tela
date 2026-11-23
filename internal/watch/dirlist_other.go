//go:build !windows

package watch

import (
	"io/fs"
	"os"
)

// readDir lists a directory, sorted by name.
func readDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
