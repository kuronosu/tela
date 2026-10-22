//go:build linux

package remote

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// newShmFile returns a memfd of size bytes, sealed against resizing so the
// host's mapping can never point past its end.
func newShmFile(size int) (*os.File, error) {
	fd, err := unix.MemfdCreate("tela-remote", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, fmt.Errorf("remote: memfd_create: %w", err)
	}
	f := os.NewFile(uintptr(fd), "tela-remote")
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		f.Close()
		return nil, fmt.Errorf("remote: ftruncate shared buffer: %w", err)
	}
	// Best effort: a kernel without sealing still works (the host checks
	// the size before mapping).
	unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS, unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_SEAL)
	return f, nil
}
