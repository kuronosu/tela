package update

import "golang.org/x/sys/unix"

// exchange swaps the entries at a and b atomically (renameat2
// RENAME_EXCHANGE, Linux 3.15). Any error but ENOENT matches
// errors.ErrUnsupported: the caller then uses two renames, which report a
// real error again (EINVAL from file systems without the flag, ENOSYS
// from old kernels, EPERM from old seccomp filters).
func exchange(a, b string) error {
	err := unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
	if err == nil || err == unix.ENOENT {
		return err
	}
	return unsupportedExchange{err}
}
