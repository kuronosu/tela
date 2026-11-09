package desktop

import "golang.org/x/sys/unix"

// exchangeFiles swaps the files at from and to atomically (renamex_np
// RENAME_SWAP, which APFS and HFS+ support). It returns an error matching
// errors.ErrUnsupported when the file system cannot, fs.ErrNotExist when
// one of them is missing.
func exchangeFiles(from, to string) error {
	return swapError(unix.RenamexNp(from, to, unix.RENAME_SWAP))
}

// renameNoReplace renames from to to unless to exists (renamex_np
// RENAME_EXCL): fs.ErrExist then, errors.ErrUnsupported where the file
// system cannot.
func renameNoReplace(from, to string) error {
	return swapError(unix.RenamexNp(from, to, unix.RENAME_EXCL))
}

// swapError keeps ENOENT and EEXIST, which say what is at the names, and
// makes every other error unsupported (ENOTSUP and EINVAL come from file
// systems without the flags, such as SMB and FAT): the caller then
// replaces the file with a plain rename, which reports a real error again.
func swapError(err error) error {
	if err == nil || err == unix.ENOENT || err == unix.EEXIST {
		return err
	}
	return unsupportedSwap{err}
}
