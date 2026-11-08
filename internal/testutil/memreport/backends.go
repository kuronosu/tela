package main

import "fmt"

// A real backend keeps its own presentation buffers next to the app's
// framebuffer. The headless backend of the measured cases keeps one copy of
// the presented frame (headless/window.go, Present) and stands in for one of
// them, so the report adds what each real backend holds for the physical
// size of the case. The sizes are computed from the backends' code, not
// measured; they are in bytes and 4 bytes per pixel.
//
//   - Wayland (wayland/shm.go): a swapchain of wl_shm buffers of the
//     window's size, in shared memory outside the Go heap (RssShmem). A
//     buffer is busy from the commit until the compositor releases it, and
//     Present allocates a new one when all are busy: it settles at two, three
//     with a compositor that holds the front buffer and one queued buffer.
//   - X11 (x11/window.go): one BGRA copy in the Go heap (pix) that answers
//     Expose; the pixels go to the server by PutImage, whose memory is the
//     server's.
//   - Win32 (win32/dib.go): one BGRA copy in the Go heap (the DIB) that
//     answers WM_PAINT and is blitted with SetDIBitsToDevice.
//   - Cocoa (cocoa/present_darwin.go): one RGBA copy in the Go heap (back)
//     and, on every Present, a CFData copy of the whole frame that the
//     CGImage of the layer keeps (the previous image is released when the new
//     contents are set, so two are alive during a Present).
type backendBuffers struct {
	name  string
	where string // what holds it
	count string // copies, as text
	min   int    // copies at least
	max   int    // copies at most
}

var backendTable = []backendBuffers{
	{"headless (the measured stand-in)", "Go heap", "1", 1, 1},
	{"Wayland", "wl_shm, shared memory", "2 to 3", 2, 3},
	{"X11", "Go heap", "1", 1, 1},
	{"Win32", "Go heap (DIB)", "1", 1, 1},
	{"Cocoa", "Go heap + CFData", "1 + 1, 2 in a Present", 2, 3},
}

// frameBytes is the size of one copy of a frame of physical size w by h.
func frameBytes(w, h int) uint64 { return 4 * uint64(w) * uint64(h) }

// describe formats the buffers of a backend for a frame size.
func (b backendBuffers) describe(w, h int) string {
	one := frameBytes(w, h)
	if b.min == b.max {
		return fmt.Sprintf("%s (%s)", mib(uint64(b.min)*one), b.where)
	}
	return fmt.Sprintf("%s to %s (%s)", mib(uint64(b.min)*one), mib(uint64(b.max)*one), b.where)
}
