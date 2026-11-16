//go:build linux

package x11

import (
	"fmt"
	"image"
	"reflect"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// DRI3 and Present, hand-written on the vendored xgb like XInput2, XKB and
// XFixes (ext.go), with the XFixes regions of Present's update: the wire
// side of dmabuf.go. The layouts are those of dri3proto.h and
// presentproto.h (xorgproto 2024.1); Mesa's loader_dri3 is the reference
// client. The file descriptors of PixmapFromBuffers and of Open's reply
// travel beside the bytes (xgb's NewRequestFds and NewCookieFds, a Tela
// patch).

const (
	dri3OpQueryVersion          = 0
	dri3OpOpen                  = 1
	dri3OpGetSupportedModifiers = 6 // DRI3 1.2
	dri3OpPixmapFromBuffers     = 7 // DRI3 1.2
	// The version this client speaks: 1.2 has the two requests above.
	dri3ClientMajor, dri3ClientMinor = 1, 2

	presentOpQueryVersion = 0
	presentOpPixmap       = 1
	presentOpSelectInput  = 3
	// The version this client speaks. Its requests and events are those
	// of 1.0; 1.2 is what Mesa declared before explicit synchronization.
	presentClientMajor, presentClientMinor = 1, 2

	// Present events: the evtype of a GenericEvent, and their masks.
	presentEvComplete   = 1
	presentEvIdle       = 2
	presentCompleteMask = 1 << presentEvComplete
	presentIdleMask     = 1 << presentEvIdle
	// PresentCompleteNotify: the kind (a pixmap, or a NotifyMSC) and what
	// the server did (CompleteMode: 0 and 3 are copies).
	presentKindPixmap = 0
	presentModeFlip   = 1
	presentModeSkip   = 2

	xfixesOpCreateRegion = 5 // XFixes 2.0
	xfixesOpSetRegion    = 11
)

// versionRequest encodes the QueryVersion of DRI3 and of Present: the
// client's major and minor version.
func versionRequest(opcode, minorOp byte, major, minor uint32) []byte {
	req := newRequest(opcode, minorOp, 12)
	le.PutUint32(req[4:], major)
	le.PutUint32(req[8:], minor)
	return req
}

// dri3OpenRequest encodes DRI3 Open: drawable, provider (None: the
// screen's own device).
func dri3OpenRequest(opcode byte, drawable xproto.Drawable) []byte {
	req := newRequest(opcode, dri3OpOpen, 12)
	le.PutUint32(req[4:], uint32(drawable))
	return req
}

// modifiersRequest encodes DRI3 GetSupportedModifiers: window, depth,
// bits per pixel.
func modifiersRequest(opcode byte, win xproto.Window, depth, bpp byte) []byte {
	req := newRequest(opcode, dri3OpGetSupportedModifiers, 12)
	le.PutUint32(req[4:], uint32(win))
	req[8], req[9] = depth, bpp
	return req
}

// decodeModifiers decodes the reply of GetSupportedModifiers: the counts
// of window and screen modifiers at 8 and 12, then the modifiers, 64 bits
// each, the window's first.
func decodeModifiers(r []byte) (window, screen []uint64, err error) {
	if len(r) < 32 {
		return nil, nil, errShortReply
	}
	nw, ns := int(le.Uint32(r[8:])), int(le.Uint32(r[12:]))
	if nw < 0 || ns < 0 || (len(r)-32)/8 < nw+ns {
		return nil, nil, errShortReply
	}
	list := func(off, n int) []uint64 {
		out := make([]uint64, n)
		for i := range out {
			out[i] = le.Uint64(r[off+8*i:])
		}
		return out
	}
	return list(32, nw), list(32+8*nw, ns), nil
}

// pixmapFromBuffersRequest encodes DRI3 PixmapFromBuffers: pixmap, window
// (it names the screen), the number of planes, the size, stride and
// offset of four planes, depth, bits per pixel and the modifier. One file
// descriptor per plane goes with it.
func pixmapFromBuffersRequest(opcode byte, pixmap xproto.Pixmap, win xproto.Window, depth, bpp byte, d platform.DmabufDescriptor) []byte {
	req := newRequest(opcode, dri3OpPixmapFromBuffers, 64)
	le.PutUint32(req[4:], uint32(pixmap))
	le.PutUint32(req[8:], uint32(win))
	req[12] = byte(len(d.Planes))
	le.PutUint16(req[16:], uint16(d.Size.X))
	le.PutUint16(req[18:], uint16(d.Size.Y))
	for i, p := range d.Planes {
		le.PutUint32(req[20+8*i:], p.Stride)
		le.PutUint32(req[24+8*i:], p.Offset)
	}
	req[52], req[53] = depth, bpp
	le.PutUint64(req[56:], d.Modifier)
	return req
}

// presentSelectInputRequest encodes Present SelectInput: the event id,
// the window, the event mask.
func presentSelectInputRequest(opcode byte, eid uint32, win xproto.Window, mask uint32) []byte {
	req := newRequest(opcode, presentOpSelectInput, 16)
	le.PutUint32(req[4:], eid)
	le.PutUint32(req[8:], uint32(win))
	le.PutUint32(req[12:], mask)
	return req
}

// presentPixmapRequest encodes PresentPixmap of the whole pixmap at the
// window's origin: window, pixmap, serial, the valid region (None: all of
// the pixmap), the update region (None: all of it), offsets, target CRTC
// (None: the server's choice), wait and idle fences (None), options
// (none: at a vertical blank, flipping when the server can), and target
// MSC, divisor and remainder (0: the next one).
func presentPixmapRequest(opcode byte, win xproto.Window, pixmap xproto.Pixmap, serial, update uint32) []byte {
	req := newRequest(opcode, presentOpPixmap, 72)
	le.PutUint32(req[4:], uint32(win))
	le.PutUint32(req[8:], uint32(pixmap))
	le.PutUint32(req[12:], serial)
	le.PutUint32(req[20:], update)
	return req
}

// regionRequest encodes XFixes CreateRegion and SetRegion: the region and
// its rectangles.
func regionRequest(opcode, minorOp byte, region uint32, rects []image.Rectangle) []byte {
	req := newRequest(opcode, minorOp, 8+8*len(rects))
	le.PutUint32(req[4:], region)
	for i, r := range rects {
		o := 8 + 8*i
		le.PutUint16(req[o:], uint16(int16(r.Min.X)))
		le.PutUint16(req[o+2:], uint16(int16(r.Min.Y)))
		le.PutUint16(req[o+4:], uint16(r.Dx()))
		le.PutUint16(req[o+6:], uint16(r.Dy()))
	}
	return req
}

// presentEvent is a decoded Present event.
type presentEvent struct {
	kind   uint16 // presentEvComplete or presentEvIdle
	eid    uint32
	window xproto.Window
	serial uint32
	// IdleNotify: the pixmap the server no longer reads.
	pixmap xproto.Pixmap
	// CompleteNotify: what completed and how.
	completeKind, mode byte
}

// decodePresentEvent decodes a GenericEvent of Present. Both events have
// the event type at 8, the event id at 12, the window at 16 and the
// serial at 20; IdleNotify (32 bytes) has the pixmap at 24, and
// CompleteNotify (40 bytes) its kind and mode at 10 and 11.
func decodePresentEvent(raw []byte) (presentEvent, bool) {
	if len(raw) < 32 {
		return presentEvent{}, false
	}
	ev := presentEvent{
		kind:   le.Uint16(raw[8:]),
		eid:    le.Uint32(raw[12:]),
		window: xproto.Window(le.Uint32(raw[16:])),
		serial: le.Uint32(raw[20:]),
	}
	switch ev.kind {
	case presentEvIdle:
		ev.pixmap = xproto.Pixmap(le.Uint32(raw[24:]))
	case presentEvComplete:
		if len(raw) < 40 {
			return presentEvent{}, false
		}
		ev.completeKind, ev.mode = raw[10], raw[11]
	default:
		return presentEvent{}, false
	}
	return ev, true
}

// errorOpcode returns the major opcode of the request an X error is
// about: a field of xproto's error structs, a method of the errors xgb
// has no constructor for.
func errorOpcode(err xgb.Error) (byte, bool) {
	if raw, ok := err.(xgb.RawError); ok {
		return raw.MajorOpcode(), true
	}
	v := reflect.ValueOf(err)
	if v.Kind() != reflect.Struct {
		return 0, false
	}
	f := v.FieldByName("MajorOpcode")
	if !f.IsValid() || f.Kind() != reflect.Uint8 {
		return 0, false
	}
	return byte(f.Uint()), true
}

// --- connServer ---

// ensureXFixes negotiates XFixes at its first use and reports whether the
// server has version 2.0 or later (selection events, regions).
// QueryVersion must come before any other XFixes request.
func (b *Backend) ensureXFixes() bool {
	if !b.xfixes.checked {
		b.xfixes.checked = true
		if e := b.queryExtension("XFIXES"); e.present {
			req := newRequest(e.opcode, xfixesOpQueryVersion, 12)
			le.PutUint32(req[4:], xfixesClientMajor)
			le.PutUint32(req[8:], xfixesClientMinor)
			if r, err := b.roundTrip(req, 16); err == nil && le.Uint32(r[8:]) >= 2 {
				b.xfixes = xfixesExt{checked: true, ok: true, extension: e}
			}
		}
	}
	return b.xfixes.ok
}

func (s connServer) dmabufCaps() dmabufCaps {
	b := s.b
	var c dmabufCaps
	if !b.conn.CanPassFds() {
		c.reason = "the connection is not a unix socket, which file descriptors need"
		return c
	}
	dri3 := b.queryExtension("DRI3")
	if !dri3.present {
		c.reason = "the server has no DRI3"
		return c
	}
	r, err := b.roundTrip(versionRequest(dri3.opcode, dri3OpQueryVersion, dri3ClientMajor, dri3ClientMinor), 16)
	if err != nil {
		c.reason = fmt.Sprintf("DRI3 QueryVersion: %v", err)
		return c
	}
	c.dri3Major, c.dri3Minor = le.Uint32(r[8:]), le.Uint32(r[12:])
	if c.dri3Major != 1 || c.dri3Minor < 2 {
		c.reason = fmt.Sprintf("the server has DRI3 %d.%d, and pixmaps with modifiers need 1.2", c.dri3Major, c.dri3Minor)
		return c
	}
	present := b.queryExtension("Present")
	if !present.present {
		c.reason = "the server has no Present extension"
		return c
	}
	r, err = b.roundTrip(versionRequest(present.opcode, presentOpQueryVersion, presentClientMajor, presentClientMinor), 16)
	if err != nil {
		c.reason = fmt.Sprintf("Present QueryVersion: %v", err)
		return c
	}
	c.presentMajor, c.presentMinor = le.Uint32(r[8:]), le.Uint32(r[12:])
	if c.presentMajor != 1 {
		c.reason = fmt.Sprintf("the server has Present %d.%d", c.presentMajor, c.presentMinor)
		return c
	}
	b.dmabuf.dri3Op, b.dmabuf.presentOp = dri3.opcode, present.opcode
	c.presentOpcode = present.opcode
	c.regions = b.ensureXFixes()
	c.major, c.minor, c.hasDevice = s.dri3Device()
	c.ok = true
	return c
}

// dri3Device asks DRI3 Open for the screen's device and returns the
// numbers of the DRM node the descriptor of the reply is open on.
func (s connServer) dri3Device() (major, minor uint32, ok bool) {
	b := s.b
	cookie := b.conn.NewCookieFds(true)
	b.conn.NewRequest(dri3OpenRequest(b.dmabuf.dri3Op, xproto.Drawable(b.root)), cookie)
	_, fds, err := cookie.ReplyFds()
	for i, fd := range fds {
		if i == 0 && err == nil {
			major, minor, ok = fdDevice(fd)
		}
		unix.Close(fd)
	}
	return major, minor, ok
}

func (s connServer) modifiers(win xproto.Window, depth, bpp byte) (window, screen []uint64, err error) {
	r, err := s.b.roundTrip(modifiersRequest(s.b.dmabuf.dri3Op, win, depth, bpp), 32)
	if err != nil {
		return nil, nil, err
	}
	return decodeModifiers(r)
}

func (s connServer) selectPresent(win xproto.Window) uint32 {
	b := s.b
	eid, err := b.conn.NewId()
	if err != nil {
		return 0
	}
	b.sendRequest(presentSelectInputRequest(b.dmabuf.presentOp, eid, win, presentCompleteMask|presentIdleMask))
	return eid
}

func (s connServer) pixmapFromBuffers(win xproto.Window, depth, bpp byte, d platform.DmabufDescriptor) (xproto.Pixmap, error) {
	b := s.b
	pixmap, err := xproto.NewPixmapId(b.conn)
	if err != nil {
		return 0, err
	}
	fds := make([]int, len(d.Planes))
	for i, p := range d.Planes {
		fds[i] = p.FD
	}
	// Checked: Check makes a round trip, and returns the X error of a
	// refused import (BadAlloc from the driver).
	cookie := b.conn.NewCookie(true, false)
	b.conn.NewRequestFds(pixmapFromBuffersRequest(b.dmabuf.dri3Op, pixmap, win, depth, bpp, d), fds, cookie)
	if err := cookie.Check(); err != nil {
		return 0, err
	}
	return pixmap, nil
}

func (s connServer) freePixmap(pix xproto.Pixmap) { xproto.FreePixmap(s.b.conn, pix) }

func (s connServer) presentPixmap(win xproto.Window, pix xproto.Pixmap, serial uint32, update []image.Rectangle) uint16 {
	b := s.b
	var region uint32
	if len(update) > 0 {
		// One region for every update: PresentPixmap copies it.
		if b.dmabuf.region == 0 {
			if id, err := b.conn.NewId(); err == nil {
				b.dmabuf.region = id
				b.sendRequest(regionRequest(b.xfixes.opcode, xfixesOpCreateRegion, id, update))
			}
		} else {
			b.sendRequest(regionRequest(b.xfixes.opcode, xfixesOpSetRegion, b.dmabuf.region, update))
		}
		region = b.dmabuf.region
	}
	cookie := b.conn.NewCookie(false, false)
	b.conn.NewRequest(presentPixmapRequest(b.dmabuf.presentOp, win, pix, serial, region), cookie)
	return cookie.Sequence
}

func (s connServer) copyPixmap(win xproto.Window, gc xproto.Gcontext, pix xproto.Pixmap, r image.Rectangle) {
	x, y := int16(r.Min.X), int16(r.Min.Y)
	xproto.CopyArea(s.b.conn, xproto.Drawable(pix), xproto.Drawable(win), gc, x, y, x, y, uint16(r.Dx()), uint16(r.Dy()))
}
