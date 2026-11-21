//go:build windows

package watch

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/windows"
)

// readDir lists a directory, sorted by name, through openDir's handle.
func readDir(path string) ([]fs.DirEntry, error) {
	f, err := openDir(path)
	if err != nil {
		return nil, err
	}
	ents, err := f.ReadDir(-1)
	f.Close()
	slices.SortFunc(ents, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return ents, err
}

// openDir opens a directory for listing and shares every access with
// other handles, delete included. os.Open shares reading and writing
// only, which has two effects on a watched tree: a directory the watcher
// lists cannot be renamed or moved for that moment (the rename needs
// delete access: a sharing violation), and a directory MoveFileEx is
// renaming cannot be listed: its handle, opened with delete access, stays
// open after the rename reported the new name, until MoveFileEx closes
// it, and a watcher that lists the new name right away (a directory moved
// into the tree) failed and lost the directory's files. FindFirstFile
// shares delete access for the same reason; the handle reads entries with
// GetFileInformationByHandleEx, as os.File.ReadDir does.
func openDir(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// longPath gives a path of MAX_PATH or more the \\?\ prefix that the
// wide API calls need for it, as package os does for its own calls.
func longPath(path string) string {
	if len(path) < windows.MAX_PATH-12 || strings.HasPrefix(path, `\\?\`) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if strings.HasPrefix(abs, `\\`) {
		return `\\?\UNC\` + abs[2:]
	}
	return `\\?\` + abs
}
