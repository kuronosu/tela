//go:build linux || darwin

package remote

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// createSegment creates a segment of size bytes and maps it read-write. f
// is the descriptor to pass to the host (the writer closes it once sent;
// the mapping stays valid); name is unused on Unix.
func createSegment(size int) (seg *segment, f *os.File, name string, err error) {
	f, err = newShmFile(size)
	if err != nil {
		return nil, nil, "", err
	}
	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, nil, "", fmt.Errorf("remote: mmap shared buffer: %w", err)
	}
	return &segment{data: data, release: func() { unix.Munmap(data) }}, f, "", nil
}

// openSegment maps the segment of a received descriptor read-only and
// closes the descriptor. It checks the file's size first: mapping past the
// end of a file that a buggy peer truncated would fault (SIGBUS) on read.
func openSegment(f *os.File, name string, size int) (*segment, error) {
	if f == nil {
		return nil, fmt.Errorf("remote: buffers message without a descriptor")
	}
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return nil, fmt.Errorf("remote: stat shared buffer: %w", err)
	}
	if st.Size < int64(size) {
		return nil, fmt.Errorf("remote: shared buffer has %d bytes, want %d", st.Size, size)
	}
	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("remote: mmap shared buffer: %w", err)
	}
	return &segment{data: data, release: func() { unix.Munmap(data) }}, nil
}
