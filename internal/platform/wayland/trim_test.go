//go:build linux

package wayland

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// setTrim sets the trim delays for a test.
func setTrim(t *testing.T, after, max time.Duration) {
	t.Helper()
	oldAfter, oldMax := trimAfter, maxTrimAfter
	trimAfter, maxTrimAfter = after, max
	t.Cleanup(func() { trimAfter, maxTrimAfter = oldAfter, oldMax })
}

// shmDriver plays a compositor that holds buffers by hand with the
// scripted fake: each frame is presented, and the compositor releases the
// previous front buffer (two buffers suffice) or keeps it (it holds two,
// so the next frame needs a third). It counts the buffers the backend
// creates and destroys.
type shmDriver struct {
	t        *testing.T
	h        *harness
	w        *Window
	front    uint32 // the buffer on screen
	held     []uint32
	frame    uint8
	creates  int
	destroys []uint32
}

func newShmDriver(t *testing.T) *shmDriver {
	h := newHarness(t, standardGlobals(1, protocols.WlShmFormatXbgr8888))
	w := openWindow(t, h, platform.WindowOptions{}, 8, 8, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	return &shmDriver{t: t, h: h, w: w}
}

func (d *shmDriver) count(reqs []freq) {
	for _, r := range reqs {
		switch {
		case r.is("wl_shm_pool.create_buffer"):
			d.creates++
		case r.is("wl_buffer.destroy"):
			d.destroys = append(d.destroys, r.id)
		}
	}
}

// present shows a new frame and returns its buffer. keep holds the
// previous front buffer (not released) instead of releasing it.
func (d *shmDriver) present(keep bool) uint32 {
	d.t.Helper()
	d.frame++
	fb := solid(d.w, d.frame, 0, 0)
	damage := []image.Rectangle{image.Rect(0, 0, 2, 2)}
	if d.front == 0 {
		damage = nil
	}
	d.h.ui(func() {
		if err := d.w.Present(fb, damage); err != nil {
			d.t.Errorf("Present: %v", err)
		}
	})
	reqs := d.h.fc.until("wl_surface.commit")
	d.count(reqs)
	a, _ := find(reqs, "wl_surface.attach")
	id := a.u(0)
	for _, b := range d.held {
		if b == id {
			d.t.Fatalf("buffer %d reused while the compositor holds it", id)
		}
	}
	if d.front != 0 {
		if keep {
			d.held = append(d.held, d.front)
		} else {
			// The compositor shows the new buffer and lets go of every
			// other one.
			for _, b := range append(d.held, d.front) {
				d.h.fc.send(b, 0) // wl_buffer.release
			}
			d.held = nil
		}
	}
	d.front = id
	d.waitBusy(1 + len(d.held))
	return id
}

// waitBusy waits until the backend has read the releases sent.
func (d *shmDriver) waitBusy(n int) {
	d.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		got := 0
		d.h.ui(func() {
			for _, b := range d.w.chain.bufs {
				if b.busy {
					got++
				}
			}
		})
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			d.t.Fatalf("%d busy buffers, want %d", got, n)
		}
		time.Sleep(time.Millisecond)
	}
}

// idle waits for d without presenting, counting what the backend sends.
func (d *shmDriver) idle(dur time.Duration) {
	d.count(d.h.fc.quiet(dur, ""))
}

func (d *shmDriver) buffers() int {
	n := 0
	d.h.ui(func() { n = len(d.w.chain.bufs) })
	return n
}

// TestUnusedThirdBufferTrimmed: a third buffer allocated while the
// compositor held two is destroyed once it has been unused for trimAfter,
// also while the app presents nothing, and the two left suffice.
func TestUnusedThirdBufferTrimmed(t *testing.T) {
	setTrim(t, 300*time.Millisecond, 10*time.Second)
	baseline := shmMappings.Load()
	d := newShmDriver(t)
	first := time.Now() // the last use of the buffer that ends unused
	d.present(false)
	d.present(true)  // the compositor keeps the first buffer
	d.present(false) // both held: a third buffer
	if n := d.buffers(); n != 3 || d.creates != 3 {
		t.Fatalf("%d buffers (%d created), want 3", n, d.creates)
	}
	// Two suffice again: frames alternate between two buffers, and the
	// third goes unused.
	for range 3 {
		d.present(false)
	}
	for len(d.destroys) == 0 && time.Since(first) < 5*time.Second {
		d.idle(20 * time.Millisecond)
	}
	if len(d.destroys) != 1 {
		t.Fatalf("wl_buffer.destroy for %v while idle, want one buffer", d.destroys)
	}
	if took := time.Since(first); took < trimAfter {
		t.Errorf("trimmed %v after the buffer's last use, before trimAfter (%v)", took, trimAfter)
	}
	if d.destroys[0] == d.front {
		t.Errorf("the front buffer %d was destroyed", d.front)
	}
	if n := d.buffers(); n != keepBuffers {
		t.Errorf("%d buffers after the trim, want %d", n, keepBuffers)
	}
	if n := shmMappings.Load() - baseline; n != keepBuffers {
		t.Errorf("%d shm buffers mapped after the trim, want %d", n, keepBuffers)
	}
	// The two left keep presenting without allocating, and nothing more
	// is trimmed.
	for range 4 {
		d.present(false)
	}
	d.idle(400 * time.Millisecond)
	if d.creates != 3 || len(d.destroys) != 1 {
		t.Errorf("after the trim: %d buffers created, %d destroyed; want 3 and 1", d.creates, len(d.destroys))
	}
	d.h.ui(func() {
		if got := d.w.chain.trimDelay(); got != 2*trimAfter {
			t.Errorf("next trim delay = %v, want %v (doubled)", got, 2*trimAfter)
		}
	})
}

// TestThirdBufferNotChurned: a compositor that holds two buffers now and
// then, every 150 ms, would make a fixed 100 ms trim destroy and
// reallocate the third buffer at every burst; the doubling delay settles
// on keeping it.
func TestThirdBufferNotChurned(t *testing.T) {
	setTrim(t, 100*time.Millisecond, 10*time.Second)
	d := newShmDriver(t)
	d.present(false)
	const bursts = 8
	for range bursts {
		d.present(true)  // the compositor keeps the front buffer
		d.present(false) // a third buffer, unless the chain kept it
		// Frames that two buffers serve, for 150 ms.
		for range 3 {
			d.present(false)
			d.idle(50 * time.Millisecond)
		}
	}
	// Without the doubling, one per burst after the first; with it, the
	// first trim's 200 ms delay outlasts the 150 ms between bursts (a
	// second trim, on a loaded machine, makes it 400 ms).
	if extra := d.creates - 3; extra > 2 {
		t.Errorf("third buffer allocated %d more times over %d bursts (destroyed %d times); want at most 2", extra, bursts, len(d.destroys))
	}
	if n := d.buffers(); n != 3 {
		t.Errorf("%d buffers at the end, want the third kept (3)", n)
	}
}

// TestSwapchainTrimRules: trim keeps keepBuffers, the newest frame and the
// buffers the compositor holds; the delay doubles per trim up to
// maxTrimAfter.
func TestSwapchainTrimRules(t *testing.T) {
	setTrim(t, time.Second, 5*time.Second)
	allocs := 0
	c := memChain(&allocs)
	var clock time.Duration
	c.now = func() time.Duration { return clock }
	size := image.Point{X: 2, Y: 2}
	fb := image.NewRGBA(image.Rectangle{Max: size})
	full := []image.Rectangle{fb.Rect}
	var bufs []*shmBuffer
	for range 4 { // four held at once
		b, _ := c.acquire(size)
		c.fill(b, fb, full)
		c.submitted(b, full)
		bufs = append(bufs, b)
	}
	if _, ok := c.trimDeadline(); ok {
		t.Error("a trim deadline while every buffer is busy")
	}
	for _, b := range bufs {
		c.released(b)
	}
	clock = 10 * time.Second
	c.trim(clock)
	if len(c.bufs) != keepBuffers || !slices.Contains(c.bufs, c.prev) {
		t.Fatalf("after the trim: %d buffers, newest kept %v; want %d with it", len(c.bufs), slices.Contains(c.bufs, c.prev), keepBuffers)
	}
	if _, ok := c.trimDeadline(); ok {
		t.Error("a trim deadline with keepBuffers buffers")
	}
	for _, want := range []time.Duration{2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second} {
		if got := c.trimDelay(); got != want {
			t.Fatalf("delay after %d trims = %v, want %v", c.trims, got, want)
		}
		c.trims++
	}
}
