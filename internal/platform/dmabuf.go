package platform

import (
	"fmt"
	"image"
)

// Presenting dmabufs (contract v13, ADR-283). See "Presenting dmabufs" in
// the package documentation.

// The DRM formats (fourcc codes of drm_fourcc.h) of 32-bit pixels, named
// after their channels from the most significant byte of a little-endian
// word: ARGB8888 is B, G, R, A in memory, and ABGR8888 is R, G, B, A, the
// order of image.RGBA. The X formats have no alpha: the byte is ignored.
// Pixels are premultiplied (zwp_linux_dmabuf_v1 and Present agree).
const (
	DRMFormatARGB8888 uint32 = 0x34325241 // "AR24"
	DRMFormatXRGB8888 uint32 = 0x34325258 // "XR24"
	DRMFormatABGR8888 uint32 = 0x34324241 // "AB24"
	DRMFormatXBGR8888 uint32 = 0x34324258 // "XB24"
)

// DRM format modifiers with a meaning of their own.
const (
	// DRMModifierLinear (DRM_FORMAT_MOD_LINEAR) is rows of pixels one
	// after the other: the one layout every device reads, and the one to
	// use when the buffer comes from another device than the one that
	// shows it.
	DRMModifierLinear uint64 = 0
	// DRMModifierInvalid (DRM_FORMAT_MOD_INVALID) stands for no explicit
	// modifier: the layout is whatever the driver that made the dmabuf
	// attached to it, which only the same driver understands. A
	// [DmabufFeedback] lists it for the formats the OS takes that way.
	DRMModifierInvalid uint64 = 0x00ffffffffffffff
)

// DmabufFormat is a pixel format with the layout of its pixels in memory:
// a DRM fourcc and a DRM format modifier.
type DmabufFormat struct {
	Fourcc   uint32
	Modifier uint64
}

func (f DmabufFormat) String() string {
	c := f.Fourcc
	return fmt.Sprintf("%c%c%c%c:%#x", byte(c), byte(c>>8), byte(c>>16), byte(c>>24), f.Modifier)
}

// DmabufFeedback is what a window's OS accepts as dmabufs right now.
type DmabufFeedback struct {
	// Serial changes whenever anything else in the feedback does, and
	// only then: a caller keeps the Serial its buffers were made for and
	// compares it once a frame. It is never 0.
	Serial uint64
	// HasDevice reports whether the OS named its main device: the one
	// that composites, which a renderer should use so that its buffers
	// need no copy. DeviceMajor and DeviceMinor are the numbers of one of
	// that device's DRM nodes, the primary (/dev/dri/cardN) or the render
	// node (/dev/dri/renderDN), whichever the OS chose to name: compare
	// them with both nodes of a candidate.
	HasDevice                bool
	DeviceMajor, DeviceMinor uint32
	// Formats are the accepted pairs of format and modifier, the ones the
	// OS prefers first (those it can scan out without compositing, when
	// it says so), without duplicates. The slice is shared: do not modify
	// it.
	Formats []DmabufFormat
}

// Accepts reports whether the feedback lists the pair.
func (f DmabufFeedback) Accepts(fourcc uint32, modifier uint64) bool {
	for _, p := range f.Formats {
		if p.Fourcc == fourcc && p.Modifier == modifier {
			return true
		}
	}
	return false
}

// Modifiers returns the modifiers the feedback lists for a format, in
// order of preference; nil when the format is not accepted.
func (f DmabufFeedback) Modifiers(fourcc uint32) []uint64 {
	var out []uint64
	for _, p := range f.Formats {
		if p.Fourcc == fourcc {
			out = append(out, p.Modifier)
		}
	}
	return out
}

// MaxDmabufPlanes is the largest number of planes of a dmabuf buffer.
const MaxDmabufPlanes = 4

// DmabufPlane is one memory plane of a buffer: a dmabuf, where the plane
// starts in it and the bytes from one row to the next. Several planes may
// name the same dmabuf.
type DmabufPlane struct {
	FD     int
	Offset uint32
	Stride uint32
}

// DmabufDescriptor describes a buffer of pixels in dmabufs to
// [DmabufPresenter.CreateDmabuf].
type DmabufDescriptor struct {
	// Size is the size in pixels. A buffer can only be presented while it
	// is the window's Metrics().Physical().
	Size image.Point
	// Fourcc and Modifier are the format, a pair the window's feedback
	// lists.
	Fourcc   uint32
	Modifier uint64
	// Planes are the memory planes of the format with this modifier, in
	// order: one for the pixels and, with some modifiers, auxiliary ones.
	Planes []DmabufPlane
	// Alpha reports that the alpha of the pixels means something: the
	// window shows what is below it through them, as a Transparent
	// window's frames do. When false the window is opaque whatever the
	// alpha channel of a format that has one holds, as far as the OS can
	// be told (Wayland: the opaque region); a compositor may still blend
	// with it, so a renderer writes 255 or picks a format without alpha.
	Alpha bool
}

// CheckDmabuf panics with an actionable message when d cannot describe a
// buffer: an empty size, no format, no planes or more than
// [MaxDmabufPlanes], a plane without a descriptor. A plane without a
// stride is an error that wraps [ErrUnsupported] instead: the stride is
// what the driver reported for the image (vkGetImageSubresourceLayout),
// not the caller's mistake, and the caller falls back to Present. Whether
// the OS takes the buffer is another matter, and an error of CreateDmabuf.
func CheckDmabuf(d DmabufDescriptor) error {
	if d.Size.X <= 0 || d.Size.Y <= 0 {
		panicf("platform: CreateDmabuf with the size %v; a buffer has the window's Metrics().Physical(), which is never empty when a frame is due", d.Size)
	}
	if d.Fourcc == 0 {
		panic("platform: CreateDmabuf without a format; Fourcc is a DRM format code such as DRMFormatXRGB8888")
	}
	if len(d.Planes) == 0 || len(d.Planes) > MaxDmabufPlanes {
		panicf("platform: CreateDmabuf with %d planes; a buffer has 1 to %d, as many as its format has with its modifier", len(d.Planes), MaxDmabufPlanes)
	}
	for i, p := range d.Planes {
		if p.FD < 0 {
			panicf("platform: CreateDmabuf plane %d has the file descriptor %d; every plane names an open dmabuf", i, p.FD)
		}
	}
	for i, p := range d.Planes {
		if p.Stride == 0 {
			return fmt.Errorf("platform: CreateDmabuf plane %d has no stride (the number of bytes from one row to the next), which the OS cannot be told: %w", i, ErrUnsupported)
		}
	}
	return nil
}

// DmabufBuffer is a buffer registered with a window by
// [DmabufPresenter.CreateDmabuf]. Its methods must be called on the UI
// thread.
type DmabufBuffer interface {
	// Size returns the size the buffer was registered with.
	Size() image.Point
	// Busy reports whether the OS may still read the buffer: from the
	// PresentDmabuf that showed it until the OS says it no longer needs
	// it (Wayland: wl_buffer.release), which it does once another frame
	// replaced this one. A busy buffer must not be drawn into: its pixels
	// are on screen. A buffer whose present was dropped is not busy. The
	// answer changes when the backend reads the OS's messages: between two
	// calls of the handler, and inside the calls that wait for the OS
	// (CreateDmabuf, the first DmabufFeedback). No event tells: the
	// caller asks when it needs a buffer.
	Busy() bool
	// Destroy forgets the buffer. The OS keeps the dmabufs of a frame it
	// still shows; the caller may release its own side at once. Closing
	// the window destroys its buffers; Destroy after that, or twice, does
	// nothing.
	Destroy()
}

// DmabufPresenter is implemented by windows that can show buffers of
// pixels the caller rendered into dmabufs, without a copy: the zero-copy
// path of the GPU renderer (ADR-281). It is optional twice over: a
// backend's windows may not implement it (found by a type assertion, like
// [AlphaPresenter]), and a window that does may report that it cannot
// right now (no feedback). The caller then presents with Window.Present,
// which always works and may alternate with PresentDmabuf on the same
// window. See "Presenting dmabufs" in the package documentation for the
// full contract.
type DmabufPresenter interface {
	// DmabufFeedback returns what the OS accepts for this window: its
	// main device and the pairs of format and modifier. ok is false when
	// the window cannot show dmabufs: the OS lacks the protocol, the
	// window is Transparent (its input mask needs the pixels), or it is
	// closed. The answer may change while the window is open; Serial
	// tells. Buffers registered before a change stay valid. The first
	// call may wait for the OS's answer.
	DmabufFeedback() (feedback DmabufFeedback, ok bool)

	// CreateDmabuf registers a buffer with the window and returns it,
	// once the OS has imported it: the call waits for that. The
	// descriptors stay the caller's, who may close them as soon as the
	// call returns (the backend duplicates what it keeps). A descriptor
	// that cannot describe a buffer panics ([CheckDmabuf]); a buffer the
	// OS does not take is an error, after which the caller falls back to
	// Present: one that wraps [ErrUnsupported] when the window cannot
	// show dmabufs at all, the feedback does not list the format with
	// this modifier, Alpha is set on a window that does not show alpha,
	// or a plane has no stride.
	CreateDmabuf(d DmabufDescriptor) (DmabufBuffer, error)

	// PresentDmabuf shows a buffer of this window that is not busy, as
	// Present shows a framebuffer: buf.Size() must be exactly
	// Metrics().Physical(), damage lists the rectangles (buffer pixels)
	// that changed since the previous present of either kind to this
	// window, nil or empty meaning the whole buffer, and the first present
	// after a ConfigureEvent, or after a Present, is treated as full. The
	// pixels must be finished: the backend waits for no fence. Unlike
	// Present nothing is copied: the whole buffer must hold the whole
	// frame, and it is busy until the OS releases it. A present that is
	// dropped (a hidden window, a closed one, damage that is clipped
	// away) leaves the buffer free. A buffer of another window, of
	// another size, destroyed or busy panics.
	PresentDmabuf(buf DmabufBuffer, damage []image.Rectangle) error
}
