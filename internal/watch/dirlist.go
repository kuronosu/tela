package watch

import (
	"io/fs"
	"os"
	"path/filepath"
)

// walkDir visits the tree at root as filepath.WalkDir does (lexical
// order, fn sees root first, then each entry; a directory's listing error
// is a second call for it; SkipDir and SkipAll as there), but lists
// directories with readDir: on Windows, os.ReadDir's handle shares no
// delete access, so a directory could not be renamed while it was listed,
// nor listed while a rename held it (dirlist_windows.go).
func walkDir(root string, fn fs.WalkDirFunc) {
	fi, err := os.Lstat(root)
	if err != nil {
		fn(root, nil, err)
		return
	}
	walkEntry(root, fs.FileInfoToDirEntry(fi), fn)
}

// walkEntry visits path and, for a directory, its entries; the error is
// fn's SkipDir (for a file: skip the rest of its directory) or SkipAll.
func walkEntry(path string, d fs.DirEntry, fn fs.WalkDirFunc) error {
	if err := fn(path, d, nil); err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			return nil
		}
		return err
	}
	ents, err := readDir(path)
	if err != nil {
		if err := fn(path, d, err); err != nil {
			if err == filepath.SkipDir {
				return nil
			}
			return err
		}
	}
	for _, e := range ents {
		if err := walkEntry(filepath.Join(path, e.Name()), e, fn); err != nil {
			if err == filepath.SkipDir {
				return nil
			}
			return err
		}
	}
	return nil
}
