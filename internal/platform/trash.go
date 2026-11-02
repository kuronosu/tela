package platform

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The Trash (ADR-216): Desktop.MoveToTrash moves a file or folder where
// the user can restore it. See "Trash" in the package documentation.

// CheckTrashPath returns the cleaned path, or an error wrapping
// fs.ErrInvalid when path cannot be given to MoveToTrash: empty, relative,
// holding a NUL byte, or a root (a file system's top, which no trash
// takes). Backends call it first, so every backend refuses the same paths,
// also where the service is missing. Paths are data (a file the user
// chose), so a bad one is an error, not a panic.
func CheckTrashPath(path string) (string, error) {
	switch {
	case path == "" || strings.ContainsRune(path, 0):
		return "", fmt.Errorf("platform: MoveToTrash(%q): not a path: %w", path, fs.ErrInvalid)
	case !filepath.IsAbs(path):
		return "", fmt.Errorf("platform: MoveToTrash(%q): the path must be absolute: %w", path, fs.ErrInvalid)
	}
	clean := filepath.Clean(path)
	if filepath.Dir(clean) == clean {
		return "", fmt.Errorf("platform: MoveToTrash(%q): a file system's root cannot go to the trash: %w", path, fs.ErrInvalid)
	}
	return clean, nil
}

// CheckTrash checks path with CheckTrashPath and then that it exists
// (os.Lstat: a symbolic link is trashed itself, not its target). It
// returns the cleaned path and its information, or an error: an invalid
// path wraps fs.ErrInvalid and a missing one fs.ErrNotExist. Backends that
// have the service call it before moving anything.
func CheckTrash(path string) (string, fs.FileInfo, error) {
	clean, err := CheckTrashPath(path)
	if err != nil {
		return "", nil, err
	}
	fi, err := os.Lstat(clean)
	if err != nil {
		return "", nil, fmt.Errorf("platform: MoveToTrash: %w", err)
	}
	return clean, fi, nil
}
