//go:build linux

package wayland

import (
	"fmt"
	"image"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Buffering
//
// A window owns a swapchain of wl_shm buffers of its current physical size.
// The compositor reads a buffer from the commit that attaches it until it
// sends wl_buffer.release; meanwhile the buffer is busy and never written.
// Present takes a free buffer (allocating a new one when all are busy; with
// a compositor that holds the front buffer and one queued buffer, that
// settles at two or three) and fills it.
//
// Partial presents: the app's framebuffer is only valid inside the damage
// (the platform contract lets it rasterize only the damage), so a reused
// buffer cannot be repaired from it. Each buffer instead remembers the
// regions that changed since it last held the newest frame (stale). Present
// copies the stale regions from the previous buffer, which holds the whole
// newest frame and may be read even while the compositor holds it, then the
// current damage from the framebuffer. This is the buffer-age technique
// (EGL_EXT_buffer_age) with exact regions instead of an age.

// maxStaleRects bounds a buffer's stale list; beyond it the list collapses
// into its bounding box.
const maxStaleRects = 16

// shmBuffer is one buffer of a swapchain.
type shmBuffer struct {
	wl     *protocols.WlBuffer // nil in tests
	data   []byte              // pixels, stride bytes per row
	size   image.Point
	stride int

	busy  bool              // attached and not yet released by the compositor
	dead  bool              // no longer the chain's size: destroy on release
	stale []image.Rectangle // regions older than the newest frame
	free  func()            // releases wl and data
}

func (b *shmBuffer) destroy() {
	if b.free != nil {
		b.free()
		b.free = nil
	}
	b.data = nil
}

// staleArea is the number of pixels a reuse of b has to repair (overlaps
// counted twice: it is only used to rank buffers).
func (b *shmBuffer) staleArea() int {
	n := 0
	for _, r := range b.stale {
		n += r.Dx() * r.Dy()
	}
	return n
}

// swapchain manages the buffers of one window. It knows nothing about
// Wayland: alloc creates a buffer (wl_shm in the backend, plain memory in
// tests).
type swapchain struct {
	size  image.Point
	bufs  []*shmBuffer
	prev  *shmBuffer // holds the newest presented frame; nil after a resize
	alloc func(size image.Point) (*shmBuffer, error)
	// copyRect writes the pixels of r from fb into a buffer, converting to
	// the buffer's pixel format.
	copyRect func(dst *shmBuffer, fb *image.RGBA, r image.Rectangle)
}

// acquire returns a free buffer of the given size, allocating one when
// every buffer is busy. A size change retires the old buffers: free ones
// are destroyed now, busy ones when the compositor releases them.
func (c *swapchain) acquire(size image.Point) (*shmBuffer, error) {
	if size != c.size {
		c.retire()
		c.size = size
	}
	var best *shmBuffer
	for _, b := range c.bufs {
		if !b.busy && (best == nil || b.staleArea() < best.staleArea()) {
			best = b
		}
	}
	if best != nil {
		return best, nil
	}
	b, err := c.alloc(size)
	if err != nil {
		return nil, err
	}
	b.stale = []image.Rectangle{{Max: size}}
	c.bufs = append(c.bufs, b)
	return b, nil
}

// retire drops every buffer from the chain.
func (c *swapchain) retire() {
	for _, b := range c.bufs {
		b.dead = true
		if !b.busy {
			b.destroy()
		}
	}
	c.bufs = nil
	c.prev = nil
}

// fill writes the next frame into b: first the regions b missed, copied
// from the previous buffer, then the damage, copied from fb. damage must be
// clipped to the chain's size. Without a previous frame (first frame at
// this size) the damage must cover everything; fill then copies all of fb.
func (c *swapchain) fill(b *shmBuffer, fb *image.RGBA, damage []image.Rectangle) {
	if c.prev == nil {
		b.stale = nil
		c.copyRect(b, fb, image.Rectangle{Max: c.size})
		return
	}
	if c.prev != b {
		for _, r := range b.stale {
			copyBuffer(b, c.prev, r)
		}
	}
	b.stale = nil
	for _, r := range damage {
		c.copyRect(b, fb, r)
	}
}

// submitted records that b was attached and committed with the given
// damage: b is busy and holds the newest frame, so every other buffer
// misses the damage.
func (c *swapchain) submitted(b *shmBuffer, damage []image.Rectangle) {
	full := c.prev == nil
	b.busy = true
	c.prev = b
	for _, o := range c.bufs {
		if o == b {
			continue
		}
		if full {
			o.stale = []image.Rectangle{{Max: c.size}}
			continue
		}
		o.stale = append(o.stale, damage...)
		if len(o.stale) > maxStaleRects {
			u := o.stale[0]
			for _, r := range o.stale[1:] {
				u = u.Union(r)
			}
			o.stale = append(o.stale[:0], u)
		}
	}
}

// released handles wl_buffer.release.
func (c *swapchain) released(b *shmBuffer) {
	b.busy = false
	if b.dead {
		b.destroy()
	}
}

// destroy releases every buffer, busy or not (the window is going away).
func (c *swapchain) destroy() {
	for _, b := range c.bufs {
		b.destroy()
	}
	c.bufs = nil
	c.prev = nil
	c.size = image.Point{}
}

// copyBuffer copies the pixels of r from src to dst; both buffers have the
// same size and format.
func copyBuffer(dst, src *shmBuffer, r image.Rectangle) {
	n := 4 * r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		do := y*dst.stride + 4*r.Min.X
		so := y*src.stride + 4*r.Min.X
		copy(dst.data[do:do+n], src.data[so:so+n])
	}
}

// copyXBGR copies r from fb into a XBGR8888 buffer: R, G, B, X in memory,
// the byte order of image.RGBA, so rows are copied as is (the alpha byte is
// ignored by the compositor).
func copyXBGR(dst *shmBuffer, fb *image.RGBA, r image.Rectangle) {
	n := 4 * r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := fb.PixOffset(r.Min.X, y)
		do := y*dst.stride + 4*r.Min.X
		copy(dst.data[do:do+n], fb.Pix[so:so+n])
	}
}

// copyXRGB copies r from fb into a XRGB8888 buffer (B, G, R, X in memory).
func copyXRGB(dst *shmBuffer, fb *image.RGBA, r image.Rectangle) {
	platform.CopyRGBAToBGRA(dst.data, dst.stride, fb, r)
}

// allocShm creates a wl_shm buffer backed by its own memfd. The pool and
// the descriptor are dropped right away: the wl_buffer keeps the memory
// alive in the compositor, and the mapping keeps it alive here.
func allocShm(shm *protocols.WlShm, format protocols.WlShmFormat, size image.Point) (*shmBuffer, error) {
	stride := 4 * size.X
	n := stride * size.Y
	if size.X <= 0 || size.Y <= 0 || n/size.Y != stride || n > 1<<31-1 {
		return nil, fmt.Errorf("wayland: cannot allocate a %dx%d buffer: too large for wl_shm", size.X, size.Y)
	}
	fd, err := unix.MemfdCreate("tela-wayland-shm", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, fmt.Errorf("wayland: memfd_create failed: %w", err)
	}
	defer unix.Close(fd)
	if err := unix.Ftruncate(fd, int64(n)); err != nil {
		return nil, fmt.Errorf("wayland: cannot size a %d byte shm buffer: %w", n, err)
	}
	// Seal the size so nobody can shrink the file under the compositor's
	// mapping (it would get SIGBUS). Not fatal if unsupported.
	unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS, unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_SEAL)
	data, err := unix.Mmap(fd, 0, n, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("wayland: cannot map a %d byte shm buffer: %w", n, err)
	}
	pool := shm.CreatePool(fd, int32(n))
	wl := pool.CreateBuffer(0, int32(size.X), int32(size.Y), int32(stride), format)
	pool.Destroy()
	b := &shmBuffer{wl: wl, data: data, size: size, stride: stride}
	b.free = func() {
		if !wl.Destroyed() {
			wl.Destroy()
		}
		unix.Munmap(data)
	}
	return b, nil
}
