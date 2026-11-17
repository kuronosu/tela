package update

import "golang.org/x/sys/unix"

// exchange swaps the entries at a and b atomically (renamex_np
// RENAME_SWAP, which APFS and HFS+ support). Any error but ENOENT matches
// errors.ErrUnsupported: the caller then uses two renames (SMB and FAT
// volumes answer ENOTSUP or EINVAL).
func exchange(a, b string) error {
	err := unix.RenamexNp(a, b, unix.RENAME_SWAP)
	if err == nil || err == unix.ENOENT {
		return err
	}
	return unsupportedExchange{err}
}
