//go:build linux

package wayland

import (
	"fmt"
	"image"
	"slices"
	"sync/atomic"
	"time"

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
//
// Trimming: a third buffer is allocated when the compositor still holds
// both others at a Present (it held the front buffer and a queued one, or
// was slow to release once). Once two suffice again, the third stays free
// and unused (acquire prefers the buffer with the least to repair, which
// alternates between the two newest), a frame of shared memory (33 MB at
// 4K). The chain destroys a free buffer beyond keepBuffers once it has
// gone unused for trimDelay: on the backend's clock, so an idle app, which
// presents nothing, releases it too (the loop waits for trimDeadline).
// Each trim doubles the next delay (trimAfter, 2x, 4x... up to
// maxTrimAfter): a compositor that needs a third buffer only now and then
// gets its memory back within a second, and one that needs it regularly
// reallocates it at most once a minute instead of every second.

// keepBuffers is the number of buffers a chain never trims below.
const keepBuffers = 2

// trimAfter is the first delay after which an unused buffer beyond
// keepBuffers is destroyed; maxTrimAfter bounds the doubled delays.
// Variables for tests.
var (
	trimAfter    = time.Second
	maxTrimAfter = time.Minute
)

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
	used  time.Duration     // when acquire last returned it (swapchain.now)
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
	size image.Point
	bufs []*shmBuffer
	// retired holds buffers of an old size that the compositor still
	// held when they were retired. They are destroyed on release, or by
	// destroy if the window goes away first.
	retired []*shmBuffer
	prev    *shmBuffer // holds the newest presented frame; nil after a resize
	alloc   func(size image.Point) (*shmBuffer, error)
	// copyRect writes the pixels of r from fb into a buffer, converting to
	// the buffer's pixel format.
	copyRect func(dst *shmBuffer, fb *image.RGBA, r image.Rectangle)
	// now is the clock of the buffers' uses and of trims (the backend's
	// Now); nil never trims.
	now func() time.Duration
	// trims counts the trims made, which lengthen the next delay
	// (trimDelay). A resize keeps it: it describes the compositor.
	trims int
}

// acquire returns a free buffer of the given size, allocating one when
// every buffer is busy. A size change retires the old buffers: free ones
// are destroyed now, busy ones when the compositor releases them or the
// chain is destroyed, whichever comes first.
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
	if best == nil {
		b, err := c.alloc(size)
		if err != nil {
			return nil, err
		}
		b.stale = []image.Rectangle{{Max: size}}
		c.bufs = append(c.bufs, b)
		best = b
	}
	if c.now != nil {
		best.used = c.now()
	}
	return best, nil
}

// trimDelay is how long a buffer beyond keepBuffers stays unused before
// trim destroys it: trimAfter, doubled by every trim made, at most
// maxTrimAfter.
func (c *swapchain) trimDelay() time.Duration {
	d := trimAfter
	for range c.trims {
		if d >= maxTrimAfter {
			break
		}
		d *= 2
	}
	return min(d, maxTrimAfter)
}

// trimmable reports whether b may be trimmed: free, and not the newest
// frame (the source of the other buffers' repairs).
func (c *swapchain) trimmable(b *shmBuffer) bool {
	return !b.busy && b != c.prev
}

// trimDeadline returns when trim may next destroy a buffer: the oldest
// use of a trimmable buffer plus trimDelay, while the chain has more than
// keepBuffers. A busy buffer gives none: its release wakes the loop,
// which asks again.
func (c *swapchain) trimDeadline() (time.Duration, bool) {
	if c.now == nil || len(c.bufs) <= keepBuffers {
		return 0, false
	}
	var at time.Duration
	found := false
	for _, b := range c.bufs {
		if c.trimmable(b) && (!found || b.used < at) {
			at, found = b.used, true
		}
	}
	return at + c.trimDelay(), found
}

// trim destroys the trimmable buffers unused for trimDelay at now, the
// oldest first, while the chain has more than keepBuffers.
func (c *swapchain) trim(now time.Duration) {
	trimmed := false
	for len(c.bufs) > keepBuffers {
		at, ok := c.trimDeadline()
		if !ok || now < at {
			break
		}
		i := slices.IndexFunc(c.bufs, func(b *shmBuffer) bool { return c.trimmable(b) && b.used+c.trimDelay() == at })
		c.bufs[i].destroy()
		c.bufs = slices.Delete(c.bufs, i, i+1)
		trimmed = true
	}
	if trimmed {
		c.trims++
	}
}

// retire drops every buffer from the chain. Busy ones move to the retired
// list until the compositor releases them.
func (c *swapchain) retire() {
	for _, b := range c.bufs {
		b.dead = true
		if b.busy {
			c.retired = append(c.retired, b)
		} else {
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
		if i := slices.Index(c.retired, b); i >= 0 {
			c.retired = slices.Delete(c.retired, i, i+1)
		}
	}
}

// destroy releases every buffer, busy or not, including retired ones the
// compositor has not released yet (the window is going away). Destroying a
// wl_buffer the compositor still holds is allowed, and the compositor keeps
// its own mapping of the memory; this side unmaps its copy. A release that
// arrives afterwards finds a destroyed proxy and is dropped by the wire
// runtime, and shmBuffer.destroy is idempotent, so nothing is freed twice.
func (c *swapchain) destroy() {
	for _, b := range c.bufs {
		b.destroy()
	}
	for _, b := range c.retired {
		b.destroy()
	}
	c.bufs = nil
	c.retired = nil
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
	shmMappings.Add(1)
	b.free = func() {
		if !wl.Destroyed() {
			wl.Destroy()
		}
		unix.Munmap(data)
		shmMappings.Add(-1)
	}
	return b, nil
}

// trimBuffers trims the windows' unused buffers that are due (swapchain.trim).
func (b *Backend) trimBuffers(now time.Duration) {
	for _, w := range b.surfaces {
		w.chain.trim(now)
	}
}

// trimDeadline returns the earliest trim of a window's unused buffer.
func (b *Backend) trimDeadline() (time.Duration, bool) {
	var at time.Duration
	found := false
	for _, w := range b.surfaces {
		if d, ok := w.chain.trimDeadline(); ok && (!found || d < at) {
			at, found = d, true
		}
	}
	return at, found
}

// shmMappings counts the wl_shm buffers whose memory is still mapped in
// this process, for leak tests.
var shmMappings atomic.Int64
