//go:build unix

package text

import (
	"os"

	"golang.org/x/sys/unix"
)

// mapFile maps the first size bytes of f read-only (MAP_PRIVATE, as
// FreeType's Unix stream does).
func mapFile(f *os.File, size int) ([]byte, error) {
	return unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_PRIVATE)
}

// unmapFile undoes mapFile. Only tests call it, on mappings nothing else
// references (readFontFile never unmaps).
func unmapFile(b []byte) error { return unix.Munmap(b) }
