package desktop

import "golang.org/x/sys/unix"

// exchangeFiles swaps the files at from and to atomically (renameat2
// RENAME_EXCHANGE, Linux 3.15; ext4, btrfs, xfs, tmpfs and f2fs support
// it). It returns an error matching errors.ErrUnsupported when the kernel
// or the file system cannot, fs.ErrNotExist when one of them is missing.
func exchangeFiles(from, to string) error {
	return swapError(unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_EXCHANGE))
}

// renameNoReplace renames from to to unless to exists (renameat2
// RENAME_NOREPLACE): fs.ErrExist then, errors.ErrUnsupported where the
// kernel or the file system cannot.
func renameNoReplace(from, to string) error {
	return swapError(unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE))
}

// swapError keeps ENOENT and EEXIST, which say what is at the names, and
// makes every other error unsupported: the caller then replaces the file
// with a plain rename, which reports a real error again (EINVAL and
// EOPNOTSUPP come from file systems without the flags, ENOSYS from
// kernels before 3.15, EPERM from seccomp filters older than renameat2).
func swapError(err error) error {
	if err == nil || err == unix.ENOENT || err == unix.EEXIST {
		return err
	}
	return unsupportedSwap{err}
}
