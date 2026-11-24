package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
)

// Dmabufs mapped and copied, and sync files (ADR-315).

// mappedFourccs are the fourccs mapDmabuf takes: those of textures.
var mappedFourccs = [...]uint32{fourccAR24, fourccXR24, fourccAB24, fourccXB24}

// mappedFormats are the formats mapDmabuf takes.
func mappedFormats() []TextureFormat {
	out := make([]TextureFormat, len(mappedFourccs))
	for i, f := range mappedFourccs {
		out[i] = TextureFormat{Fourcc: f, Modifier: platform.DRMModifierLinear}
	}
	return out
}

// mappedDmabuf is a linear dmabuf mapped for reading.
type mappedDmabuf struct {
	fd   int    // a duplicate of the application's
	data []byte // the mapping
	// off, stride, w and h locate the picture in data; swap and opaque are
	// its fourcc's (textureFourcc).
	off, stride, w, h int
	swap, opaque      bool
}

// mapDmabuf maps d. The picture is inside the memory of its descriptor
// (a dma-buf never shrinks), so that reading it cannot fault.
func mapDmabuf(d Dmabuf) (mappedTexture, error) {
	if err := d.checkMapped(); err != nil {
		return nil, err
	}
	p := d.Planes[0]
	fail := func(what string, err error) (mappedTexture, error) {
		return nil, fmt.Errorf("tela: Texture.Import: %s: %s: %w", d.describe(), what, err)
	}
	fd, err := unix.FcntlInt(uintptr(p.FD), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return fail(fmt.Sprintf("duplicating its descriptor %d", p.FD), err)
	}
	size, err := dmabufSize(fd)
	if err != nil {
		unix.Close(fd)
		return fail("reading its size", err)
	}
	n := d.extent()
	if n > size || n > math.MaxInt {
		unix.Close(fd)
		return nil, fmt.Errorf("tela: Texture.Import: %s: %d rows with a stride of %d bytes from offset %d end at byte %d, and its memory has %d", d.describe(), d.Height, p.Stride, p.Offset, n, size)
	}
	// Only a dma-buf answers: another descriptor (a file, whose mapping
	// would fault once it is truncated) is refused.
	if err := dmabufSync(fd, dmaBufSyncStart|dmaBufSyncRead); err != nil {
		unix.Close(fd)
		if errors.Is(err, unix.ENOTTY) {
			return fail(fmt.Sprintf("the descriptor %d is not a dma-buf", p.FD), err)
		}
		return fail("asking to read it", err)
	}
	err = dmabufSync(fd, dmaBufSyncEnd|dmaBufSyncRead)
	if err != nil {
		unix.Close(fd)
		return fail("asking to read it", err)
	}
	data, err := unix.Mmap(fd, 0, int(n), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return fail("mapping it", err)
	}
	swap, opaque, _ := textureFourcc(d.Fourcc)
	return &mappedDmabuf{fd: fd, data: data, off: int(p.Offset), stride: int(p.Stride), w: d.Width, h: d.Height, swap: swap, opaque: opaque}, nil
}

// checkMapped reports why d, checked, cannot be mapped and copied.
func (d Dmabuf) checkMapped() error {
	switch {
	case d.Modifier != platform.DRMModifierLinear || len(d.Planes) != 1:
		return fmt.Errorf("tela: Texture.Import: %s: it is mapped and copied here, which takes linear memory (modifier 0) with one plane (Texture.Device lists the formats): %w", d.describe(), ErrTextureUnsupported)
	case int64(d.Planes[0].Stride) < 4*int64(d.Width):
		return fmt.Errorf("tela: Texture.Import: %s: a stride of %d bytes for rows of %d", d.describe(), d.Planes[0].Stride, 4*d.Width)
	}
	return nil
}

// extent returns the bytes of a linear one-plane picture: up to the end of
// its last row's stride.
func (d Dmabuf) extent() int64 {
	p := d.Planes[0]
	return int64(p.Offset) + int64(p.Stride)*int64(d.Height)
}

// dmabufSize returns the size of the memory of a dma-buf: where seeking to
// its end gets, which its kernel interface defines, else what stat says.
func dmabufSize(fd int) (int64, error) {
	if n, err := unix.Seek(fd, 0, unix.SEEK_END); err == nil {
		return n, nil
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return 0, err
	}
	return st.Size, nil
}

// read implements mappedTexture. The kernel waits for the writers it knows
// of (the dma-buf's implicit fences) when the read starts.
func (m *mappedDmabuf) read(dst []byte) (err error) {
	// The mapping is inside the memory, but the memory may still go (a
	// device unplugged): an error, not a crash.
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		if r := recover(); r != nil {
			if f, ok := r.(interface{ Addr() uintptr }); ok {
				err = fmt.Errorf("the memory of a dmabuf is gone (a fault at 0x%x)", f.Addr())
				return
			}
			panic(r)
		}
	}()
	if err := dmabufSync(m.fd, dmaBufSyncStart|dmaBufSyncRead); err != nil {
		return fmt.Errorf("asking to read a dmabuf: %w", err)
	}
	defer dmabufSync(m.fd, dmaBufSyncEnd|dmaBufSyncRead)
	row := 4 * m.w
	for y := range m.h {
		src := m.data[m.off+y*m.stride:][:row]
		d := dst[y*row:][:row]
		switch {
		case !m.swap && !m.opaque:
			copy(d, src)
		default:
			convertRow(d, src, m.swap, m.opaque)
		}
	}
	return nil
}

// convertRow copies the pixels of src to dst as RGBA: with swap they are
// blue first, and with opaque their fourth byte means nothing.
func convertRow(dst, src []byte, swap, opaque bool) {
	// perf: eight bytes at a time would halve it.
	var alpha uint32
	if opaque {
		alpha = 0xff000000
	}
	for i := 0; i+4 <= len(src) && i+4 <= len(dst); i += 4 {
		v := binary.LittleEndian.Uint32(src[i:])
		if swap {
			v = v&0xff00ff00 | v>>16&0xff | v&0xff<<16
		}
		binary.LittleEndian.PutUint32(dst[i:], v|alpha)
	}
}

// close implements mappedTexture.
func (m *mappedDmabuf) close() {
	unix.Munmap(m.data)
	unix.Close(m.fd)
	m.data, m.fd = nil, -1
}

// Flags of DMA_BUF_IOCTL_SYNC (linux/dma-buf.h).
const (
	dmaBufSyncRead  = 1 << 0
	dmaBufSyncStart = 0 << 2
	dmaBufSyncEnd   = 1 << 2

	dmaBufIoctlSync = 0x40086200 // _IOW('b', 0, struct dma_buf_sync), a u64
)

// dmabufSync brackets the CPU's reads of a dma-buf: DMA_BUF_IOCTL_SYNC.
// Tests replace it, to map memory that is not a dma-buf.
var dmabufSync = func(fd int, flags uint64) error {
	for {
		_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), dmaBufIoctlSync, uintptr(unsafe.Pointer(&flags)))
		switch e {
		case 0:
			return nil
		case unix.EINTR, unix.EAGAIN:
			continue
		}
		return e
	}
}

// ---------------------------------------------------------------------
// Sync files.

// syncFile is the Fence of SyncFile.
type syncFile struct {
	mu  sync.Mutex
	fd  int // -1: closed
	err error
}

func newSyncFile(fd int) Fence {
	if fd < 0 {
		fd = -1
	}
	return &syncFile{fd: fd}
}

// Wait implements Fence: it waits for the file to be readable, which a sync
// file is once its fences signaled, and closes it.
func (s *syncFile) Wait(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fd < 0 {
		return s.err // it ended before
	}
	if err := pollIn(ctx, s.fd); err != nil {
		if ctx.Err() != nil {
			return err // not waited for to the end: closeFence closes it
		}
		s.err = fmt.Errorf("waiting for a sync file: %w", err)
	} else if st, err := syncFileStatus(s.fd); err == nil && st < 0 {
		s.err = fmt.Errorf("the work of a sync file failed: %w", unix.Errno(-st))
	}
	unix.Close(s.fd)
	s.fd = -1
	return s.err
}

// closeFence implements fenceCloser.
func (s *syncFile) closeFence() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fd >= 0 {
		unix.Close(s.fd)
		s.fd, s.err = -1, errors.New("the sync file was closed before it signaled")
	}
}

// pollIn blocks until fd is readable (poll's POLLIN) or ctx is done.
func pollIn(ctx context.Context, fd int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if ctx.Done() != nil {
		wake, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
		if err != nil {
			return err
		}
		fds = append(fds, unix.PollFd{Fd: int32(wake), Events: unix.POLLIN})
		written := make(chan struct{})
		stop := context.AfterFunc(ctx, func() {
			defer close(written)
			unix.Write(wake, []byte{1, 0, 0, 0, 0, 0, 0, 0})
		})
		defer func() {
			if !stop() {
				<-written // it may be writing
			}
			unix.Close(wake)
		}()
	}
	for {
		_, err := unix.Poll(fds, -1)
		switch {
		case err == unix.EINTR || err == unix.EAGAIN:
			continue
		case err != nil:
			return err
		}
		switch r := fds[0].Revents; {
		case r&unix.POLLIN != 0:
			return nil
		case r&unix.POLLNVAL != 0:
			return unix.EBADF
		case r&(unix.POLLERR|unix.POLLHUP) != 0:
			return unix.EIO
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// syncFileInfo is struct sync_file_info (linux/sync_file.h).
type syncFileInfo struct {
	name      [32]byte
	status    int32 // 1: signaled, 0: active, negative: an error
	flags     uint32
	numFences uint32
	pad       uint32
	fences    uint64
}

const syncIocFileInfo = 0xc0383e04 // _IOWR('>', 4, struct sync_file_info)

// syncFileStatus returns the status of a sync file: 1 signaled, 0 not yet,
// a negative errno for work that failed.
func syncFileStatus(fd int) (int32, error) {
	var info syncFileInfo
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), syncIocFileInfo, uintptr(unsafe.Pointer(&info))); e != 0 {
		return 0, e
	}
	return info.status, nil
}
