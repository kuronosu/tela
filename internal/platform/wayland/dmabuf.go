//go:build linux

package wayland

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"slices"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// Presenting dmabufs (platform.DmabufPresenter, contract v13, ADR-283):
// the zero-copy path of the GPU renderer, over zwp_linux_dmabuf_v1.
//
// The global is only noted when the registry announces it, and bound when
// a window is first asked for its feedback: an app that renders on the
// CPU gets no dmabuf event and maps no format table.
//
// Feedback. With version 4 the compositor describes what it accepts
// through zwp_linux_dmabuf_feedback_v1 objects: its main device (a dev_t)
// and tranches of indices into a table of format and modifier pairs that
// arrives as a file descriptor, mapped read-only and private, copied and
// closed. There is one such object for the backend (get_default_feedback,
// asked with the bind) and one per window (get_surface_feedback, asked at
// the window's first DmabufFeedback), which the compositor may send again
// at any time, for example to add a tranche it can scan out while the
// window is fullscreen. A window answers with its own feedback once that
// arrived and with the default one until then, and gives the answer a new
// Serial only when its content differs from the last answer, so the
// surface feedback that repeats the default one changes nothing for the
// caller. Version 3 has no feedback objects: the global sends modifier
// events when bound, which are the list, and there is no main device.
// Versions 1 and 2 send format events only: each format is listed with
// DRM_FORMAT_MOD_INVALID, the implicit modifier. Versions 5 and 6 are not
// bound: 6 drops main_device for a sampling flag on tranches.
//
// Buffers. CreateDmabuf sends zwp_linux_buffer_params_v1.create and waits
// for created or failed with a round trip. The other request,
// create_immed, returns the wl_buffer at once, but lets the compositor
// answer a failed import with a fatal protocol error, which wlroots, KWin
// and Mutter do: the app would die of a buffer that Present could have
// shown instead. Buffers are registered once per swapchain, so the round
// trip (wl_display.sync, answered by the compositor's next dispatch) is
// paid at start and at a resize, not per frame. What the protocol makes a
// fatal error whichever request is used is checked here first and
// returned as an error: a format and modifier the feedback does not list
// (invalid_format) and planes that do not fit their dmabuf (out_of_bounds,
// with the checks of wlroots). The descriptors are duplicated by the wire
// layer when the request is encoded and the duplicates closed once sent,
// so they stay the caller's.
//
// A round trip inside a handler call dispatches whatever else arrived:
// protocol callbacks only update state and queue events (see Loop in
// doc.go), so nothing is delivered to the app until the handler returns.
// It is bounded by dmabufTimeout: a compositor that does not answer makes
// CreateDmabuf fail, and the caller present with wl_shm.
//
// Presenting. PresentDmabuf is Present without the copy: the same ack of
// the pending configure, viewport destination or buffer scale,
// damage_buffer, frame callback and commit (Window.beginCommit,
// commitBuffer), and the same rules for a hidden window and the first
// buffer, which maps the window. The buffer is busy from the commit to
// its wl_buffer.release.
//
// Alternating with Present. The two kinds of present may follow each
// other on one window. A dmabuf present retires the wl_shm swapchain (its
// free buffers are destroyed, the busy ones at their release): its newest
// buffer no longer holds the frame on screen, and a renderer that presents
// dmabufs should not also hold two or three frames of shared memory. The
// next Present therefore copies the whole framebuffer, whatever its
// damage says; a PresentDmabuf after a Present damages the whole buffer.
//
// Hide and show (ADR-186). The buffers are wl_buffers of their own and
// outlive the role objects, so they stay registered while the window is
// hidden; the compositor releases the one that was shown. The window
// keeps no frame of its own to map with when it is shown again (the
// swapchain's newest plays that part for Present, mapRetained): the
// pixels are the caller's, who may have drawn over them. Instead the
// configure of the new role requests a frame and makes the next present
// a full one, and that present maps the window (remapDmabuf).
//
// Opaque and transparent. Windows that present dmabufs are opaque: a
// buffer registered without Alpha sets the surface's opaque region to
// everything, whatever alpha channel its format has, and one with Alpha
// is refused. A transparent window (ADR-249) reports no feedback and
// registers nothing: its input region is computed from the pixels of the
// frames, which only Present sees.
//
// Synchronization is the caller's: the protocol's default is implicit
// sync through the dmabuf's reservation object, which the NVIDIA driver
// does not take part in, so a buffer is presented only when the device
// has finished drawing it (ADR-281). No explicit-sync protocol is used.

// dmabufVersion is the highest zwp_linux_dmabuf_v1 version bound.
const dmabufVersion = 4

// dmabufTimeout bounds the round trips of the dmabuf path: the first
// feedback and each buffer registration. A variable for tests.
var dmabufTimeout = 2 * time.Second

// dmabufGlobal is the backend's side of zwp_linux_dmabuf_v1.
type dmabufGlobal struct {
	// name and version are the global as the registry announced it;
	// version 0 means the compositor has none.
	name, version uint32
	// wl is the bound global, nil before the first use (ensureDmabuf) and
	// when binding failed; tried says the bind was attempted.
	wl    *protocols.ZwpLinuxDmabufV1
	tried bool
	// def is the default feedback (version 4), nil below it.
	def *dmabufFeedback
	// legacy is the list of the format and modifier events (versions 1 to
	// 3), complete after the round trip that follows the bind.
	legacy dmabufInfo
	// serial is the latest Serial given to a feedback answer.
	serial uint64
}

// dmabufInfo is the content of a feedback: what a platform.DmabufFeedback
// reports, without its serial.
type dmabufInfo struct {
	hasDevice    bool
	major, minor uint32
	formats      []platform.DmabufFormat
}

func (a *dmabufInfo) equal(b *dmabufInfo) bool {
	return a.hasDevice == b.hasDevice && a.major == b.major && a.minor == b.minor && slices.Equal(a.formats, b.formats)
}

// dmabufFeedback follows one zwp_linux_dmabuf_feedback_v1: the events of
// an update accumulate in the pending fields until done makes them the
// current feedback.
type dmabufFeedback struct {
	wl *protocols.ZwpLinuxDmabufFeedbackV1

	// table is the latest format table. It stays between updates: a
	// compositor sends a new one only when it changed.
	table []platform.DmabufFormat
	// The update in progress: the main device (kept from the previous
	// update when it is not sent again), the formats of the tranches done
	// so far, in order of preference and without duplicates, and those of
	// the tranche being received.
	device  dmabufInfo
	pending []platform.DmabufFormat
	seen    map[platform.DmabufFormat]struct{}
	tranche []platform.DmabufFormat

	// cur is the feedback in effect once ready (a done arrived), and gen
	// counts the dones that changed it.
	cur   dmabufInfo
	ready bool
	gen   uint64
}

// newDmabufFeedback follows a feedback object.
func newDmabufFeedback(wl *protocols.ZwpLinuxDmabufFeedbackV1) *dmabufFeedback {
	f := &dmabufFeedback{wl: wl, seen: map[platform.DmabufFormat]struct{}{}}
	wl.SetFormatTableHandler(func(e protocols.ZwpLinuxDmabufFeedbackV1FormatTableEvent) {
		f.table = readFormatTable(e.Fd, e.Size)
	})
	wl.SetMainDeviceHandler(func(e protocols.ZwpLinuxDmabufFeedbackV1MainDeviceEvent) {
		// A dev_t, 8 bytes on every Linux target. Another size is not a
		// device this code can name: no main device then.
		f.device.hasDevice = len(e.Device) == 8
		if f.device.hasDevice {
			dev := binary.NativeEndian.Uint64(e.Device)
			f.device.major, f.device.minor = unix.Major(dev), unix.Minor(dev)
		}
	})
	wl.SetTrancheFormatsHandler(func(e protocols.ZwpLinuxDmabufFeedbackV1TrancheFormatsEvent) {
		for i := 0; i+2 <= len(e.Indices); i += 2 {
			if idx := int(binary.NativeEndian.Uint16(e.Indices[i:])); idx < len(f.table) {
				f.tranche = append(f.tranche, f.table[idx])
			}
		}
	})
	wl.SetTrancheDoneHandler(func(protocols.ZwpLinuxDmabufFeedbackV1TrancheDoneEvent) {
		// Tranches come in descending order of preference, and a pair may
		// be in several (a scan-out tranche repeats pairs of the main
		// device's): the first mention is its place.
		for _, p := range f.tranche {
			if _, dup := f.seen[p]; !dup {
				f.seen[p] = struct{}{}
				f.pending = append(f.pending, p)
			}
		}
		f.tranche = f.tranche[:0]
	})
	wl.SetDoneHandler(func(protocols.ZwpLinuxDmabufFeedbackV1DoneEvent) {
		next := f.device
		next.formats = f.pending
		if !f.ready || !f.cur.equal(&next) {
			f.cur = next
			f.gen++
		}
		f.ready = true
		f.pending, f.tranche = nil, f.tranche[:0]
		clear(f.seen)
	})
	return f
}

// readFormatTable copies the format table of a feedback from its file
// descriptor, which it closes: size bytes of 16-byte entries, a format in
// 32 bits, 4 bytes of padding and a modifier in 64 bits, in native
// endianness. The protocol asks for a read-only private mapping. It
// returns nil when the table cannot be mapped.
func readFormatTable(fd int, size uint32) []platform.DmabufFormat {
	defer wire.CloseFd(fd)
	const entry = 16
	n := int(size) / entry
	if fd < 0 || n == 0 {
		return nil
	}
	data, err := unix.Mmap(fd, 0, int(size), unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		return nil
	}
	defer unix.Munmap(data)
	table := make([]platform.DmabufFormat, n)
	for i := range table {
		e := data[i*entry:]
		table[i] = platform.DmabufFormat{
			Fourcc:   binary.NativeEndian.Uint32(e),
			Modifier: binary.NativeEndian.Uint64(e[8:]),
		}
	}
	return table
}

// ensureDmabuf binds zwp_linux_dmabuf_v1 at its first use and waits, with
// one round trip, for what the compositor accepts: the default feedback,
// or the modifier and format events of the older versions. It reports
// whether the global is bound.
func (b *Backend) ensureDmabuf() bool {
	d := &b.dmabuf
	if d.tried {
		return d.wl != nil
	}
	if d.version == 0 || b.released || b.conn.Err() != nil {
		return false
	}
	d.tried = true
	version := min(d.version, dmabufVersion)
	wl := protocols.WlRegistryBind[protocols.ZwpLinuxDmabufV1](b.registry, d.name, version)
	if version >= 4 {
		d.def = newDmabufFeedback(wl.GetDefaultFeedback())
	} else {
		seen := map[platform.DmabufFormat]struct{}{}
		add := func(p platform.DmabufFormat) {
			if _, dup := seen[p]; !dup {
				seen[p] = struct{}{}
				d.legacy.formats = append(d.legacy.formats, p)
			}
		}
		wl.SetModifierHandler(func(e protocols.ZwpLinuxDmabufV1ModifierEvent) {
			add(platform.DmabufFormat{Fourcc: e.Format, Modifier: uint64(e.ModifierHi)<<32 | uint64(e.ModifierLo)})
		})
		if version < 3 {
			// No modifiers before version 3: the layout is the driver's
			// own, which the protocol calls the implicit modifier.
			wl.SetFormatHandler(func(e protocols.ZwpLinuxDmabufV1FormatEvent) {
				add(platform.DmabufFormat{Fourcc: e.Format, Modifier: platform.DRMModifierInvalid})
			})
		}
	}
	d.wl = wl
	// The answer is needed now: the caller chooses its device and its
	// formats from it. A compositor that does not answer in time is
	// treated as one that accepts nothing yet; what arrives later is
	// reported by the next call.
	b.conn.RoundtripTimeout(dmabufTimeout)
	return true
}

// releaseDmabuf destroys the global's objects (Backend.release).
func (b *Backend) releaseDmabuf() {
	d := &b.dmabuf
	if d.def != nil {
		d.def.wl.Destroy()
		d.def = nil
	}
	if d.wl != nil {
		d.wl.Destroy()
		d.wl = nil
	}
}

// dmabufState is a window's side of presenting dmabufs.
type dmabufState struct {
	// feedback is the surface's feedback (version 4), asked at the first
	// DmabufFeedback.
	feedback *dmabufFeedback
	// reported is the latest answer of DmabufFeedback, and src and gen
	// the feedback object and its generation it was made from (src is nil
	// for the legacy list).
	reported platform.DmabufFeedback
	answered bool
	src      *dmabufFeedback
	gen      uint64
	// bufs are the registered buffers, destroyed with the window.
	bufs []*dmabufBuffer
	// shown reports that the newest committed frame is a dmabuf: the
	// wl_shm swapchain holds nothing to map the window with.
	shown bool
	// opaque reports that the surface's opaque region is set.
	opaque bool
}

var _ platform.DmabufPresenter = (*Window)(nil)

// canDmabuf reports whether the window can present dmabufs: open, opaque
// (a transparent window's input region needs the pixels of its frames,
// ADR-249), and on a compositor with zwp_linux_dmabuf_v1.
func (w *Window) canDmabuf() bool {
	return !w.Closed() && !w.opts.Transparent && w.b.ensureDmabuf()
}

// dmabufInfo returns the feedback in effect for the window and the
// object and generation it comes from: the surface's once it arrived,
// else the default one, else the legacy list. ok is false while nothing
// arrived.
func (w *Window) dmabufInfo() (info *dmabufInfo, src *dmabufFeedback, ok bool) {
	d := &w.b.dmabuf
	if d.def == nil {
		return &d.legacy, nil, len(d.legacy.formats) > 0
	}
	if w.dmabuf.feedback == nil {
		w.dmabuf.feedback = newDmabufFeedback(d.wl.GetSurfaceFeedback(w.surface))
	}
	src = w.dmabuf.feedback
	if !src.ready {
		src = d.def
	}
	return &src.cur, src, src.ready && len(src.cur.formats) > 0
}

// DmabufFeedback returns what the compositor accepts for the window; see
// platform.DmabufPresenter. The first call on a backend binds
// zwp_linux_dmabuf_v1 and waits for the default feedback.
func (w *Window) DmabufFeedback() (platform.DmabufFeedback, bool) {
	if !w.canDmabuf() {
		return platform.DmabufFeedback{}, false
	}
	info, src, ok := w.dmabufInfo()
	if !ok {
		return platform.DmabufFeedback{}, false
	}
	st := &w.dmabuf
	gen := uint64(0)
	if src != nil {
		gen = src.gen
	}
	if st.answered && st.src == src && st.gen == gen {
		return st.reported, true
	}
	st.src, st.gen = src, gen
	// A new Serial only for new content: the surface's first feedback
	// usually repeats the default one, and the caller's buffers stay
	// right.
	last := dmabufInfo{hasDevice: st.reported.HasDevice, major: st.reported.DeviceMajor, minor: st.reported.DeviceMinor, formats: st.reported.Formats}
	if !st.answered || !last.equal(info) {
		w.b.dmabuf.serial++
		st.reported = platform.DmabufFeedback{
			Serial:      w.b.dmabuf.serial,
			HasDevice:   info.hasDevice,
			DeviceMajor: info.major,
			DeviceMinor: info.minor,
			Formats:     info.formats,
		}
		st.answered = true
	}
	return st.reported, true
}

// dmabufBuffer is a wl_buffer of dmabufs: a platform.DmabufBuffer.
type dmabufBuffer struct {
	w    *Window
	wl   *protocols.WlBuffer // nil once destroyed
	size image.Point
	// busy: committed and not released by the compositor.
	busy bool
	// alpha is the descriptor's Alpha (always false today: see
	// CreateDmabuf).
	alpha bool
}

var _ platform.DmabufBuffer = (*dmabufBuffer)(nil)

// Size returns the size the buffer was registered with.
func (b *dmabufBuffer) Size() image.Point { return b.size }

// Busy reports whether the compositor may still read the buffer: from the
// commit that attached it to its wl_buffer.release. The release is handled
// when the loop dispatches, between two calls of the handler, and inside
// the calls that wait for the compositor (CreateDmabuf, the first
// DmabufFeedback).
func (b *dmabufBuffer) Busy() bool { return b.busy }

// Destroy destroys the wl_buffer. The compositor keeps its own reference
// to the dmabufs of a buffer it still shows; a release that arrives
// afterwards finds a destroyed proxy and is dropped by the wire runtime.
func (b *dmabufBuffer) Destroy() {
	if b.wl == nil {
		return
	}
	w := b.w
	if !b.wl.Destroyed() && w.b.conn.Err() == nil {
		b.wl.Destroy()
	}
	b.wl, b.busy = nil, false
	if i := slices.Index(w.dmabuf.bufs, b); i >= 0 {
		w.dmabuf.bufs = slices.Delete(w.dmabuf.bufs, i, i+1)
	}
}

// CreateDmabuf registers a buffer; see platform.DmabufPresenter and the
// notes at the top of this file (create and a round trip, not
// create_immed; what is checked before the request).
func (w *Window) CreateDmabuf(d platform.DmabufDescriptor) (platform.DmabufBuffer, error) {
	if err := platform.CheckDmabuf(d); err != nil {
		return nil, err
	}
	if w.Closed() {
		return nil, fmt.Errorf("wayland: CreateDmabuf on a closed window: %w", platform.ErrUnsupported)
	}
	if w.opts.Transparent {
		return nil, fmt.Errorf("wayland: a transparent window presents its frames with Present, which its input region is computed from: %w", platform.ErrUnsupported)
	}
	b := w.b
	if !b.ensureDmabuf() {
		return nil, fmt.Errorf("wayland: the compositor has no %s: %w", protocols.ZwpLinuxDmabufV1InterfaceName, platform.ErrUnsupported)
	}
	if err := b.conn.Err(); err != nil {
		return nil, err
	}
	if d.Alpha {
		return nil, fmt.Errorf("wayland: a buffer with Alpha on an opaque window: %w", platform.ErrUnsupported)
	}
	// invalid_format is a fatal protocol error: only what the compositor
	// listed is sent.
	format := platform.DmabufFormat{Fourcc: d.Fourcc, Modifier: d.Modifier}
	if info, _, ok := w.dmabufInfo(); !ok || !slices.Contains(info.formats, format) {
		return nil, fmt.Errorf("wayland: the compositor does not list the format %v for dmabufs: %w", format, platform.ErrUnsupported)
	}
	if err := checkDmabufBounds(d); err != nil {
		return nil, err
	}

	params := b.dmabuf.wl.CreateParams()
	for i, p := range d.Planes {
		params.Add(p.FD, uint32(i), p.Offset, p.Stride, uint32(d.Modifier>>32), uint32(d.Modifier))
	}
	// The outcome arrives as an event; abandoned is set when the wait
	// gave up, so that a buffer created too late is not left behind.
	var (
		created   *protocols.WlBuffer
		failed    bool
		abandoned bool
	)
	params.SetCreatedHandler(func(e protocols.ZwpLinuxBufferParamsV1CreatedEvent) {
		if abandoned {
			e.Buffer.Destroy()
			params.Destroy()
			return
		}
		created = e.Buffer
	})
	params.SetFailedHandler(func(protocols.ZwpLinuxBufferParamsV1FailedEvent) {
		if abandoned {
			params.Destroy()
			return
		}
		failed = true
	})
	params.Create(int32(d.Size.X), int32(d.Size.Y), d.Fourcc, 0)
	// Every compositor answers from the request itself, so the answer
	// precedes the sync's; a second round trip covers one that imports
	// later.
	var err error
	for range 2 {
		if err = b.conn.RoundtripTimeout(dmabufTimeout); err != nil || created != nil || failed {
			break
		}
	}
	switch {
	case created != nil:
		params.Destroy()
	case failed:
		params.Destroy()
		return nil, fmt.Errorf("wayland: the compositor could not import a %dx%d dmabuf buffer of format %v", d.Size.X, d.Size.Y, format)
	default:
		abandoned = true
		if cerr := b.conn.Err(); cerr != nil {
			return nil, cerr // a protocol error or a lost connection
		}
		if err == nil {
			err = fmt.Errorf("no created or failed event")
		}
		return nil, fmt.Errorf("wayland: registering a %dx%d dmabuf buffer of format %v: %w", d.Size.X, d.Size.Y, format, err)
	}

	buf := &dmabufBuffer{w: w, wl: created, size: d.Size, alpha: d.Alpha}
	created.SetReleaseHandler(func(protocols.WlBufferReleaseEvent) { buf.busy = false })
	w.dmabuf.bufs = append(w.dmabuf.bufs, buf)
	return buf, nil
}

// checkDmabufBounds checks that the planes fit their dmabufs, as wlroots
// does before it imports: a compositor answers a plane that does not with
// the fatal out_of_bounds. A dmabuf whose size cannot be asked (lseek) is
// not checked.
func checkDmabufBounds(d platform.DmabufDescriptor) error {
	for i, p := range d.Planes {
		offset, stride, height := uint64(p.Offset), uint64(p.Stride), uint64(d.Size.Y)
		if offset+stride > math.MaxUint32 || i == 0 && offset+stride*height > math.MaxUint32 {
			return fmt.Errorf("wayland: plane %d of a dmabuf buffer (offset %d, stride %d, %d rows) exceeds 4 GiB", i, offset, stride, height)
		}
		end, err := unix.Seek(p.FD, 0, unix.SEEK_END)
		if err != nil || end <= 0 {
			continue
		}
		size := uint64(end)
		// Only the first plane has rows of the buffer's height for
		// certain (the others may be subsampled or metadata).
		if offset > size || offset+stride > size || i == 0 && offset+stride*height > size {
			return fmt.Errorf("wayland: plane %d of a dmabuf buffer (offset %d, stride %d, %d rows) does not fit its dmabuf of %d bytes", i, offset, stride, height, size)
		}
	}
	return nil
}

// PresentDmabuf shows a registered buffer; see platform.DmabufPresenter.
// It is Present without the copy into a wl_shm buffer.
func (w *Window) PresentDmabuf(pb platform.DmabufBuffer, damage []image.Rectangle) error {
	buf, ok := pb.(*dmabufBuffer)
	if !ok || buf == nil || buf.w != w {
		panic("wayland: PresentDmabuf called with a buffer that is not this window's; a buffer is presented on the window whose CreateDmabuf returned it")
	}
	if w.Closed() {
		return nil
	}
	if buf.wl == nil {
		panic("wayland: PresentDmabuf called with a destroyed buffer")
	}
	m := w.Metrics()
	if m.Scale == 0 {
		panic("wayland: PresentDmabuf called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	if size := m.Physical(); buf.size != size {
		panic(fmt.Sprintf("wayland: PresentDmabuf with a %dx%d buffer on a window of %dx%d (logical %v at scale %g); "+
			"register buffers of Metrics().Physical() after every ConfigureEvent and destroy the old ones",
			buf.size.X, buf.size.Y, size.X, size.Y, m.Size, m.Scale))
	}
	if buf.busy {
		panic("wayland: PresentDmabuf called with a busy buffer: the compositor still reads it; present a buffer whose Busy() is false")
	}
	b := w.b
	if err := b.conn.Err(); err != nil {
		return err
	}
	if !w.showable() {
		return nil // dropped, like Present's; the next present is still full
	}
	bounds := image.Rectangle{Max: buf.size}
	rects := platform.ClipDamage(damage, bounds, w.NeedsFull() || !w.dmabuf.shown)
	if len(rects) == 0 {
		return nil // everything was clipped away: nothing changed
	}
	w.ClearNeedsFull()

	w.beginCommit(m)
	if !buf.alpha && !w.dmabuf.opaque {
		// Everything is opaque, whatever alpha the format carries. The
		// compositor clips the region to the surface, so it holds at
		// every size, and it is right for Present's X formats too.
		region := b.compositor.CreateRegion()
		region.Add(0, 0, math.MaxInt32, math.MaxInt32)
		w.surface.SetOpaqueRegion(region)
		region.Destroy() // the surface keeps a copy
		w.dmabuf.opaque = true
	}
	w.commitBuffer(buf.wl, rects, buf.size)
	buf.busy = true
	w.dmabuf.shown = true
	// The swapchain's newest buffer is no longer the frame on screen, and
	// its memory is not needed while dmabufs are presented.
	w.chain.retire()
	w.committed()
	return nil
}

// remapDmabuf runs at the configure of a window shown again that nothing
// mapped: when its latest frame was a dmabuf there is no frame of the
// window's own to show (mapRetained), so the app is asked for one, in
// full; its present maps the window.
func (w *Window) remapDmabuf() {
	if w.mapped || !w.dmabuf.shown {
		return
	}
	w.RequestFull()
	w.RequestFrame()
}

// releaseDmabuf destroys the window's buffers and its feedback object
// (Close). A buffer the caller destroys later finds nothing to do.
func (w *Window) releaseDmabuf() {
	for len(w.dmabuf.bufs) > 0 {
		w.dmabuf.bufs[len(w.dmabuf.bufs)-1].Destroy()
	}
	if f := w.dmabuf.feedback; f != nil {
		if w.b.conn.Err() == nil {
			f.wl.Destroy()
		}
		w.dmabuf.feedback = nil
	}
}
