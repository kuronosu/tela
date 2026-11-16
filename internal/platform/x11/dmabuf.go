//go:build linux

package x11

import (
	"errors"
	"fmt"
	"image"
	"io"
	"slices"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Presenting dmabufs (platform.DmabufPresenter, contract v13, ADR-283):
// the zero-copy path of the GPU renderer, over DRI3 and the Present
// extension. dri3.go has the requests and the events; this file has the
// windows' side.
//
// What the server must have. DRI3 1.2 (pixmaps of several planes with a
// format modifier), Present 1.0, and a connection over a unix socket, the
// only transport that carries file descriptors (the vendored xgb passes
// them, ADR-025). They are asked once, at the first DmabufFeedback or
// CreateDmabuf of the backend: an app that renders on the CPU sends none
// of this and gets no Present event. A server without them reports no
// feedback, and the caller presents with Present. XFixes 2.0, when
// present, gives PresentPixmap its update region.
//
// Feedback. The device is the one DRI3 Open returns for the root window: a
// file descriptor of a DRM node (the render node on Xwayland and on
// Xorg's modesetting driver), whose device numbers fstat tells and which
// is closed at once. The formats come from DRI3 GetSupportedModifiers for
// the window, its depth and 32 bits per pixel. DRI3 names no format: a
// pixmap has a depth and the server derives the layout from it, and depth
// 24 at 32 bits per pixel is DRM_FORMAT_XRGB8888 in every server (glamor's
// table: B, G, R, X bytes, the layout PutImage writes too). So the
// feedback lists XRGB8888 with each modifier of the reply, the window's
// ones (what the server can show without a copy) before the screen's, as
// the server lists them: Xwayland on NVIDIA lists DRM_FORMAT_MOD_INVALID
// (the implicit modifier) among them and no linear one. A reply without
// any modifier is a driver that takes only implicit ones, and the
// feedback lists XRGB8888 with DRM_FORMAT_MOD_INVALID. The answer is kept
// per window and asked again after its size changed (a window that
// became fullscreen may get other window modifiers, as Mesa asks at every
// new buffer); the Serial changes only when the list does.
//
// Buffers. CreateDmabuf is DRI3 PixmapFromBuffers, sent checked: the
// call waits for the server, and a failed import (BadAlloc from the
// driver, BadValue) is the call's error instead of an X error nobody
// matches to its request. X errors do not end the connection, so nothing
// has to be checked before the request for safety; the feedback and the
// protocol's 16-bit sizes are, for a clear error. The descriptors are
// sent with the request and the kernel holds its own references once the
// request is written, which it is when the call returns: they stay the
// caller's.
//
// Presenting. PresentDmabuf is PresentPixmap at the next vertical blank
// (no target MSC, no options), with an XFixes region of the damage as
// its update when the damage is partial, the server has XFixes and no
// earlier present of the window is waiting. That last rule matters: the
// server discards a waiting present when a later one for the same blank
// has no update region, and only then (present_pixmap in the X server's
// present_scmd.c and xwayland-present.c: "if (!update && pixmap)"), so
// presents with a region queue up behind each other when they are made
// faster than the server shows them, and a window Xwayland does not draw
// (a hidden workspace) completes one flip a second (its TIMER_LEN_FLIP;
// measured: an answer after 1.5 s on average). A present without region
// copies the whole pixmap, which is right for a buffer that holds the
// whole frame, and replaces the ones waiting.
//
// Busy. The server answers every PresentPixmap with a PresentIdleNotify
// for its pixmap once it no longer reads it: right after a copy or a
// skip, and for a flip (the pixmap itself is what the screen shows, which
// Xwayland does with most presents: the dmabuf becomes the Wayland
// surface's buffer) when a later present replaced it. A buffer is busy
// until then, and also while it is the window's front buffer, the one
// that holds the frame on screen, which only the next present of either
// kind ends: see Expose. A caller therefore finds its first buffer free
// again after it presented a second one, as on Wayland, and needs a third
// where the server flips: the flipped one, the one waiting, and one to
// draw into.
//
// Expose, map, hide and show. The window keeps the latest presented
// buffer as its front buffer, and its pixmap is to dmabuf frames what the
// retained copy is to Present's: an Expose is answered with a CopyArea
// from it, without the caller, and a window is mapped with it. The first
// PresentDmabuf maps the window and sends no PresentPixmap (an unmapped
// window has no contents); the Expose that follows the map paints the
// front buffer. A hidden window (SetVisible) drops its presents, as the
// contract says, and keeps its front buffer, with which it is mapped
// when it is shown again; if a present was dropped meanwhile it also
// requests a frame, in full, like the Wayland backend. Destroying the
// front buffer (a resize) only forgets the caller's handle: the pixmap
// lives until another frame replaces it. A server may make a flipped
// pixmap the window's own (Xwayland's source does: "Replace window pixmap
// with flip pixmap"); the copy is then one of a pixmap onto itself, and
// what a Present draws afterwards (PutImage) lands in a buffer that is
// still busy, which is why a flipped buffer is never taken for free
// before its PresentIdleNotify.
//
// Alternating with Present. A dmabuf present drops the retained copy and
// a Present drops the front buffer, so there is one frame to repaint from
// and the first present of a kind is a full one: Present after a dmabuf
// copies the whole framebuffer, PresentDmabuf after a Present updates the
// whole window.
//
// Pacing is unchanged: the 60 Hz timer of the backend. PresentCompleteNotify
// would be a display signal for the loop (VsyncDone), but no present is
// made, and so none completes, while the window is unmapped or iconified;
// it comes once a second for a window Xwayland does not draw; the loop's
// gate has no timeout for a signal that never comes; and the PutImage
// path, which may take over at any frame, has none. It is selected only
// to know whether a present is still waiting (the update region above).
// Two presents in one refresh are harmless: the later replaces the
// earlier.
//
// Transparent windows (ADR-249) report no feedback and register nothing:
// their input shape is computed from the pixels of the frames, which only
// Present sees. Buffers with Alpha are refused.

// presentServer is what frames need from the X connection: PutImage for
// Present, and DRI3, Present and XFixes regions for dmabufs. The backend
// uses connServer (dri3.go); unit tests replace it with a fake.
type presentServer interface {
	// putImage sends rectangle r of w's retained copy (present.go).
	putImage(w *Window, r image.Rectangle)
	// dmabufCaps asks the server what it has for dmabufs (round trips).
	dmabufCaps() dmabufCaps
	// modifiers is DRI3 GetSupportedModifiers (a round trip).
	modifiers(win xproto.Window, depth, bpp byte) (window, screen []uint64, err error)
	// selectPresent asks for the Present events of win (CompleteNotify
	// and IdleNotify) and returns the id they carry, 0 when none could be
	// allocated.
	selectPresent(win xproto.Window) uint32
	// pixmapFromBuffers is DRI3 PixmapFromBuffers, checked (a round
	// trip): the pixmap, or the server's error.
	pixmapFromBuffers(win xproto.Window, depth, bpp byte, d platform.DmabufDescriptor) (xproto.Pixmap, error)
	freePixmap(pix xproto.Pixmap)
	// presentPixmap is PresentPixmap of the whole pixmap at the next
	// vertical blank, with update as its update region (nil: everything).
	// It returns the request's sequence number, which an X error for it
	// carries.
	presentPixmap(win xproto.Window, pix xproto.Pixmap, serial uint32, update []image.Rectangle) uint16
	// copyPixmap copies r of the pixmap to the same place in the window
	// (CopyArea).
	copyPixmap(win xproto.Window, gc xproto.Gcontext, pix xproto.Pixmap, r image.Rectangle)
}

// dmabufCaps is what the server has for dmabufs.
type dmabufCaps struct {
	// ok: DRI3 1.2 or later and Present on a connection that passes file
	// descriptors; reason says what is missing otherwise.
	ok     bool
	reason string
	// The versions negotiated (the client asks for DRI3 1.2 and Present
	// 1.2), for the logs.
	dri3Major, dri3Minor       uint32
	presentMajor, presentMinor uint32
	// presentOpcode is the major opcode of Present, which its events
	// carry.
	presentOpcode byte
	// regions: XFixes 2.0, for update regions.
	regions bool
	// The device DRI3 Open returns.
	hasDevice    bool
	major, minor uint32
}

// dmabufBackend is the backend's side of presenting dmabufs.
type dmabufBackend struct {
	asked bool
	caps  dmabufCaps
	// serial is the latest Serial given to a feedback answer.
	serial uint64
	// users counts the windows that selected Present events, so that X
	// errors are only looked at while there are any (dmabufError).
	users int

	// The connection's side (dri3.go): the major opcodes and the XFixes
	// region of update regions (0 until the first one).
	dri3Op, presentOp byte
	region            uint32
}

// dmabufState is a window's side of presenting dmabufs.
type dmabufState struct {
	// eid is the id of the window's Present events, 0 until its first
	// buffer.
	eid uint32
	// reported is the latest answer of DmabufFeedback, once the server
	// answered; asked says the modifiers were asked for, and stale that
	// they must be again (the size changed).
	reported platform.DmabufFeedback
	answered bool
	asked    bool
	stale    bool
	// bufs are the registered buffers, destroyed with the window.
	bufs []*dmabufBuffer
	// front is the buffer that holds the window's frame: the latest one
	// presented, nil when the latest frame came from Present (the
	// retained copy holds it then). It may be a buffer the caller
	// destroyed.
	front *dmabufBuffer
	// sent and completed are the serials of the latest PresentPixmap and
	// of the latest PresentCompleteNotify: a present is waiting while
	// they differ.
	sent, completed uint32
	// dropped: a present was dropped while the window was hidden.
	dropped bool
	// What the server did with the presents (PresentCompleteNotify), and
	// the presents it answered with an X error, for the live test.
	flips, copies, skips, failed int
}

var _ platform.DmabufPresenter = (*Window)(nil)

// dmabufCaps returns what the server has for dmabufs, asking at the first
// call.
func (b *Backend) dmabufCaps() *dmabufCaps {
	d := &b.dmabuf
	if !d.asked {
		d.asked = true
		if b.ps != nil && !b.closed && !b.lost.Load() {
			d.caps = b.ps.dmabufCaps()
		}
	}
	return &d.caps
}

// canDmabuf reports whether the window can present dmabufs: open, opaque
// (a transparent window's input shape needs the pixels of its frames,
// ADR-249), and on a server with DRI3 1.2 and Present.
func (w *Window) canDmabuf() bool {
	return !w.Closed() && !w.opts.Transparent && w.b.dmabufCaps().ok
}

// dmabufFourcc is the DRM format of a pixmap of the window's depth at 32
// bits per pixel, as every server derives it: depth 24 is XRGB8888. The
// backend has no other opaque depth (checkPixelLayout).
const dmabufFourcc = platform.DRMFormatXRGB8888

// dmabufBpp is the bits per pixel of the pixmaps.
const dmabufBpp = 32

// DmabufFeedback returns what the server accepts for the window; see
// platform.DmabufPresenter. The first call on a backend negotiates DRI3
// and Present, and the first call of a window, and the first after each
// change of its size, asks for the modifiers: round trips.
func (w *Window) DmabufFeedback() (platform.DmabufFeedback, bool) {
	if !w.canDmabuf() {
		return platform.DmabufFeedback{}, false
	}
	b, st := w.b, &w.dmabuf
	if st.asked && !st.stale {
		return st.reported, st.answered
	}
	// Asked once per size, whatever the answer: the caller may ask every
	// frame.
	st.asked, st.stale = true, false
	window, screen, err := b.ps.modifiers(w.id, w.depth, dmabufBpp)
	if err != nil {
		// The old answer stands while the server gives no new one.
		return st.reported, st.answered
	}
	formats := dmabufFormats(window, screen)
	if !st.answered || !slices.Equal(st.reported.Formats, formats) {
		caps := b.dmabufCaps()
		b.dmabuf.serial++
		st.reported = platform.DmabufFeedback{
			Serial:      b.dmabuf.serial,
			HasDevice:   caps.hasDevice,
			DeviceMajor: caps.major,
			DeviceMinor: caps.minor,
			Formats:     formats,
		}
		st.answered = true
	}
	return st.reported, true
}

// dmabufFormats is the feedback's list for a GetSupportedModifiers reply:
// XRGB8888 with the window's modifiers, then the screen's, without
// duplicates; with the implicit modifier alone when the server lists
// none.
func dmabufFormats(window, screen []uint64) []platform.DmabufFormat {
	formats := make([]platform.DmabufFormat, 0, len(window)+len(screen))
	for _, list := range [][]uint64{window, screen} {
		for _, m := range list {
			if f := (platform.DmabufFormat{Fourcc: dmabufFourcc, Modifier: m}); !slices.Contains(formats, f) {
				formats = append(formats, f)
			}
		}
	}
	if len(formats) == 0 {
		formats = append(formats, platform.DmabufFormat{Fourcc: dmabufFourcc, Modifier: platform.DRMModifierInvalid})
	}
	return formats
}

// dmabufBuffer is a pixmap of dmabufs: a platform.DmabufBuffer.
type dmabufBuffer struct {
	w      *Window
	pixmap xproto.Pixmap // 0 once freed
	size   image.Point
	// dead: the caller destroyed it. The pixmap lives on while the buffer
	// is the window's front buffer.
	dead bool
	// pending are the PresentPixmap requests of the buffer the server has
	// not answered with a PresentIdleNotify.
	pending []dmabufPresent
}

// dmabufPresent is a PresentPixmap request: its serial, which the
// events carry, and its sequence number, which an X error carries.
type dmabufPresent struct {
	serial uint32
	seq    uint16
}

var _ platform.DmabufBuffer = (*dmabufBuffer)(nil)

// Size returns the size the buffer was registered with.
func (b *dmabufBuffer) Size() image.Point { return b.size }

// Busy reports whether the server may still read the buffer: a
// PresentPixmap of it has no PresentIdleNotify yet, or it is the window's
// front buffer (the frame on screen, which Expose is answered from) and
// no other present replaced it. The events are handled when the loop
// reads the server's messages, between two calls of the handler.
func (b *dmabufBuffer) Busy() bool {
	return !b.dead && (len(b.pending) > 0 || b.w.dmabuf.front == b)
}

// Destroy forgets the buffer and frees its pixmap, which the server keeps
// while it shows it. The front buffer's pixmap is kept until another
// frame replaces it, so that Expose still has the frame.
func (b *dmabufBuffer) Destroy() {
	if b.dead {
		return
	}
	b.dead = true
	w := b.w
	if i := slices.Index(w.dmabuf.bufs, b); i >= 0 {
		w.dmabuf.bufs = slices.Delete(w.dmabuf.bufs, i, i+1)
	}
	if w.dmabuf.front != b {
		b.free()
	}
}

// free frees the pixmap.
func (b *dmabufBuffer) free() {
	if b.pixmap != 0 {
		b.w.b.ps.freePixmap(b.pixmap)
		b.pixmap = 0
	}
	b.pending = nil
}

// CreateDmabuf registers a buffer; see platform.DmabufPresenter and the
// notes at the top of this file.
func (w *Window) CreateDmabuf(d platform.DmabufDescriptor) (platform.DmabufBuffer, error) {
	platform.CheckDmabuf(d)
	if w.Closed() {
		return nil, fmt.Errorf("x11: CreateDmabuf on a closed window: %w", platform.ErrUnsupported)
	}
	if w.opts.Transparent {
		return nil, fmt.Errorf("x11: a transparent window presents its frames with Present, which its input shape is computed from: %w", platform.ErrUnsupported)
	}
	b := w.b
	if caps := b.dmabufCaps(); !caps.ok {
		reason := caps.reason
		if reason == "" {
			reason = "the connection is closed"
		}
		return nil, fmt.Errorf("x11: no dmabufs on this server (%s): %w", reason, platform.ErrUnsupported)
	}
	if b.lost.Load() {
		return nil, errConnLost
	}
	if d.Alpha {
		return nil, fmt.Errorf("x11: a buffer with Alpha on an opaque window: %w", platform.ErrUnsupported)
	}
	format := platform.DmabufFormat{Fourcc: d.Fourcc, Modifier: d.Modifier}
	if fb, ok := w.DmabufFeedback(); !ok || !fb.Accepts(d.Fourcc, d.Modifier) {
		return nil, fmt.Errorf("x11: the server does not list the format %v for dmabufs: %w", format, platform.ErrUnsupported)
	}
	if d.Size.X > maxWindowSide || d.Size.Y > maxWindowSide {
		return nil, fmt.Errorf("x11: a %dx%d dmabuf buffer is larger than an X11 pixmap can be (%d)", d.Size.X, d.Size.Y, maxWindowSide)
	}
	st := &w.dmabuf
	if st.eid == 0 {
		// The events of this window's presents, from its first buffer on.
		if st.eid = b.ps.selectPresent(w.id); st.eid == 0 {
			return nil, fmt.Errorf("x11: no id for the window's Present events")
		}
		b.dmabuf.users++
	}
	pixmap, err := b.ps.pixmapFromBuffers(w.id, w.depth, dmabufBpp, d)
	if err != nil {
		if b.lost.Load() || errors.Is(err, io.EOF) {
			return nil, errConnLost
		}
		return nil, fmt.Errorf("x11: the server could not import a %dx%d dmabuf buffer of format %v: %w", d.Size.X, d.Size.Y, format, err)
	}
	buf := &dmabufBuffer{w: w, pixmap: pixmap, size: d.Size}
	st.bufs = append(st.bufs, buf)
	return buf, nil
}

// PresentDmabuf shows a registered buffer; see platform.DmabufPresenter.
func (w *Window) PresentDmabuf(pb platform.DmabufBuffer, damage []image.Rectangle) error {
	buf, ok := pb.(*dmabufBuffer)
	if !ok || buf == nil || buf.w != w {
		panic("x11: PresentDmabuf called with a buffer that is not this window's; a buffer is presented on the window whose CreateDmabuf returned it")
	}
	if w.Closed() {
		return nil
	}
	if buf.dead {
		panic("x11: PresentDmabuf called with a destroyed buffer")
	}
	m := w.Metrics()
	if m.Scale == 0 {
		panic("x11: PresentDmabuf called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	if size := m.Physical(); buf.size != size {
		panic(fmt.Sprintf("x11: PresentDmabuf with a %dx%d buffer on a window of %dx%d (logical %v at scale %g); "+
			"register buffers of Metrics().Physical() after every ConfigureEvent and destroy the old ones",
			buf.size.X, buf.size.Y, size.X, size.Y, m.Size, m.Scale))
	}
	if buf.Busy() {
		panic("x11: PresentDmabuf called with a busy buffer: the server still reads it; present a buffer whose Busy() is false")
	}
	b, st := w.b, &w.dmabuf
	if b.lost.Load() {
		return errConnLost
	}
	if !w.visible {
		// Dropped, like every present of a hidden window on Wayland; the
		// next present is still full, and the show asks for it.
		st.dropped = true
		return nil
	}
	bounds := image.Rectangle{Max: buf.size}
	// The first dmabuf after a Present is full: the window shows the
	// retained copy's frame, not the previous buffer's.
	rects := platform.ClipDamage(damage, bounds, w.NeedsFull() || st.front == nil)
	if len(rects) == 0 {
		return nil // everything was clipped away: nothing changed
	}
	w.ClearNeedsFull()

	// The frame is in the buffer now: the retained copy is neither the
	// frame on screen nor needed while dmabufs are presented, and its
	// absence makes the next Present a full one.
	w.pix, w.pixSize = nil, image.Point{}
	w.setFront(buf)
	if !w.mapped {
		// An unmapped window has no contents: the Expose that follows the
		// map paints the front buffer (flushExpose).
		w.mapWindow()
		return nil
	}
	if w.osUnmapped {
		return nil // iconified: the Expose of its return paints it
	}
	var update []image.Rectangle
	if b.dmabufCaps().regions && st.sent == st.completed && len(rects) <= maxUpdateRects && !(len(rects) == 1 && rects[0] == bounds) {
		update = rects
	}
	st.sent++
	seq := b.ps.presentPixmap(w.id, buf.pixmap, st.sent, update)
	buf.pending = append(buf.pending, dmabufPresent{serial: st.sent, seq: seq})
	if b.lost.Load() {
		return errConnLost
	}
	return nil
}

// maxUpdateRects bounds the rectangles of an update region; more damage
// than that is presented whole.
const maxUpdateRects = 256

// setFront makes buf the window's front buffer, or ends the dmabuf frames
// with nil (a Present). The previous front buffer is free once its
// presents are answered, and its pixmap is freed if the caller destroyed
// it meanwhile.
func (w *Window) setFront(buf *dmabufBuffer) {
	old := w.dmabuf.front
	w.dmabuf.front = buf
	if old != nil && old != buf && old.dead {
		old.free()
	}
}

// exposeDmabuf answers an Expose of r from the front buffer, when the
// window's frame is a dmabuf. It reports whether it is.
func (w *Window) exposeDmabuf(r image.Rectangle) bool {
	f := w.dmabuf.front
	if f == nil {
		return false
	}
	if r = r.Intersect(image.Rectangle{Max: f.size}); !r.Empty() && f.pixmap != 0 {
		w.b.ps.copyPixmap(w.id, w.gc, f.pixmap, r)
	}
	return true
}

// shownDmabuf runs when a hidden window is shown again: a present that
// was dropped meanwhile is asked for again, in full.
func (w *Window) shownDmabuf() {
	if w.dmabuf.dropped {
		w.dmabuf.dropped = false
		w.RequestFull()
		w.RequestFrame()
	}
}

// releaseDmabuf frees the window's pixmaps (Close). A buffer the caller
// destroys later finds nothing to do.
func (w *Window) releaseDmabuf() {
	st := &w.dmabuf
	front := st.front
	st.front = nil
	if front != nil && front.dead {
		front.free()
	}
	for len(st.bufs) > 0 {
		st.bufs[len(st.bufs)-1].Destroy()
	}
	if st.eid != 0 {
		st.eid = 0
		w.b.dmabuf.users--
	}
}

// bufferOf returns the window's buffer with the pixmap: a registered one
// or the front buffer.
func (w *Window) bufferOf(pix xproto.Pixmap) *dmabufBuffer {
	if f := w.dmabuf.front; f != nil && f.pixmap == pix {
		return f
	}
	for _, buf := range w.dmabuf.bufs {
		if buf.pixmap == pix {
			return buf
		}
	}
	return nil
}

// presentEvent handles a Present event (a GenericEvent of the extension):
// IdleNotify frees a buffer, CompleteNotify tells that a present is no
// longer waiting.
func (b *Backend) presentEvent(raw xgb.RawEvent) {
	ev, ok := decodePresentEvent(raw)
	if !ok {
		return
	}
	w := b.windows[ev.window]
	if w == nil || w.dmabuf.eid == 0 || ev.eid != w.dmabuf.eid {
		return
	}
	st := &w.dmabuf
	switch ev.kind {
	case presentEvIdle:
		if buf := w.bufferOf(ev.pixmap); buf != nil {
			buf.pending = slices.DeleteFunc(buf.pending, func(p dmabufPresent) bool { return p.serial == ev.serial })
		}
	case presentEvComplete:
		if ev.completeKind != presentKindPixmap {
			return
		}
		// Serials only grow; a present is answered once.
		if int32(ev.serial-st.completed) > 0 {
			st.completed = ev.serial
		}
		switch ev.mode {
		case presentModeFlip:
			st.flips++
		case presentModeSkip:
			st.skips++
		default:
			st.copies++
		}
	}
}

// dmabufError looks at an X error of an unchecked request: when it is the
// error of a PresentPixmap, no PresentIdleNotify or CompleteNotify will
// come for it, and the buffer would stay busy for ever.
func (b *Backend) dmabufError(err xgb.Error) {
	if b.dmabuf.users == 0 {
		return
	}
	// Sequence numbers have 16 bits and a flipped buffer waits long for
	// its answer: the opcode tells the errors of other requests apart.
	if op, ok := errorOpcode(err); ok && op != b.dmabuf.caps.presentOpcode {
		return
	}
	seq := err.SequenceId()
	for _, w := range b.loop.Windows() {
		st := &w.dmabuf
		bufs := st.bufs
		if st.front != nil && st.front.dead {
			bufs = append(slices.Clone(bufs), st.front)
		}
		for _, buf := range bufs {
			i := slices.IndexFunc(buf.pending, func(p dmabufPresent) bool { return p.seq == seq })
			if i < 0 {
				continue
			}
			if serial := buf.pending[i].serial; int32(serial-st.completed) > 0 {
				st.completed = serial
			}
			buf.pending = slices.Delete(buf.pending, i, i+1)
			st.failed++
			return
		}
	}
}

// fdDevice returns the device numbers of the DRM node a descriptor is
// open on.
func fdDevice(fd int) (major, minor uint32, ok bool) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFCHR {
		return 0, 0, false
	}
	return unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev)), true
}
