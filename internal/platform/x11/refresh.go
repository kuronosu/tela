//go:build linux

package x11

import (
	"image"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
)

// FrameEvent.Refresh: the refresh rate of the CRTC a window is on.
//
// RandR has no rate on its monitors (RRGetMonitors, screens.go), so the
// CRTCs are read with their modes: RRGetScreenResourcesCurrent (RandR 1.3;
// the request of 1.2 probes the outputs, which takes hundreds of
// milliseconds) and RRGetCrtcInfo. They are read at the first frame of a
// window that has a position, and again at the first one after the server
// reported a change of the screen or of a CRTC (RRSelectInput on the root
// window).
//
// The window's CRTC is the one that shows the largest part of it, as Xorg
// chooses the one Present synchronizes with; of two that show as much
// (mirrored outputs), the fastest. Under Xwayland the CRTCs are the
// compositor's outputs, with CVT modes it makes for them (Xwayland 24.1
// says 143.88 Hz for 143.998).

const (
	randrOpSelectInput               = 4
	randrOpGetCrtcInfo               = 20
	randrOpGetScreenResourcesCurrent = 25

	randrScreenChangeNotify = 0 // event offsets from the first event
	randrNotify             = 1
	randrScreenChangeMask   = 1 << 0
	randrCrtcChangeMask     = 1 << 1

	randrModeInterlace  = 1 << 4
	randrModeDoubleScan = 1 << 5
	randrModeInfoSize   = 32
)

// crtc is an active CRTC: what it shows of the root window, and its refresh
// interval (0 when unknown).
type crtc struct {
	rect    image.Rectangle
	refresh time.Duration
}

// refresh is the loop's Refresh hook: 0 for a window that is not mapped or
// whose position is not known yet.
func (w *Window) refresh() time.Duration {
	if !w.mapped || !w.osState.HasPosition {
		return 0
	}
	r := &w.b.randr
	if !r.fresh {
		// Fresh before it is read: a change reported meanwhile is one of
		// the events translated later, and makes it stale again.
		r.fresh = true
		r.crtcs = w.b.srv.activeCRTCs()
	}
	pos := w.osState.Position
	return refreshOn(r.crtcs, image.Rectangle{Min: pos, Max: pos.Add(w.osSize)})
}

// refreshOn returns the refresh interval of the CRTC that shows the largest
// part of win (root coordinates), the fastest of those that show as much; 0
// when win is off all of them.
func refreshOn(crtcs []crtc, win image.Rectangle) time.Duration {
	var best time.Duration
	area := 0
	for _, c := range crtcs {
		in := c.rect.Intersect(win)
		switch a := in.Dx() * in.Dy(); {
		case a > area:
			best, area = c.refresh, a
		case a == area && a > 0 && c.refresh > 0 && (best == 0 || c.refresh < best):
			best = c.refresh
		}
	}
	return best
}

// randrEvent handles a notification of RandR: the CRTCs are read again at
// the next frame.
func (b *Backend) randrEvent() { b.randr.fresh = false }

// activeCRTCs reads the CRTCs that show something, nil when the server has
// no RandR 1.3; the first call asks for the change notifications.
func (s connServer) activeCRTCs() []crtc {
	b := s.b
	r := b.ensureRandR()
	if !r.current {
		return nil
	}
	if !r.selected {
		// Before the state is read, so that no change is missed.
		r.selected = true
		req := newRequest(r.opcode, randrOpSelectInput, 12)
		le.PutUint32(req[4:], uint32(b.root))
		le.PutUint16(req[8:], randrScreenChangeMask|randrCrtcChangeMask)
		b.sendRequest(req)
	}
	req := newRequest(r.opcode, randrOpGetScreenResourcesCurrent, 8)
	le.PutUint32(req[4:], uint32(b.root))
	rep, err := b.roundTrip(req, 32)
	if err != nil {
		return nil
	}
	res := parseResources(rep)
	// The requests go out together: one round trip for all of them.
	cookies := make([]*xgb.Cookie, len(res.crtcs))
	for i, id := range res.crtcs {
		req := newRequest(r.opcode, randrOpGetCrtcInfo, 12)
		le.PutUint32(req[4:], id)
		le.PutUint32(req[8:], res.configTime)
		cookies[i] = b.conn.NewCookie(true, true)
		b.conn.NewRequest(req, cookies[i])
	}
	var out []crtc
	for _, cookie := range cookies {
		rep, err := cookie.Reply()
		if err != nil {
			continue // the CRTC is gone: a notification follows
		}
		if c, ok := parseCRTC(rep, res.modes); ok {
			out = append(out, c)
		}
	}
	return out
}

// resources is a decoded RRGetScreenResourcesCurrent reply: the CRTCs, and
// the refresh interval of each mode.
type resources struct {
	configTime uint32
	crtcs      []uint32
	modes      map[uint32]time.Duration
}

// parseResources reads the reply: a 32-byte header (the configuration time
// at 12, the numbers of CRTCs, outputs and modes at 16, 18 and 20), the
// CRTCs, the outputs, and a MODEINFO per mode.
func parseResources(rep []byte) resources {
	if len(rep) < 32 {
		return resources{}
	}
	res := resources{configTime: le.Uint32(rep[12:]), modes: map[uint32]time.Duration{}}
	crtcs, outputs, modes := int(le.Uint16(rep[16:])), int(le.Uint16(rep[18:])), int(le.Uint16(rep[20:]))
	off := 32
	for ; crtcs > 0 && off+4 <= len(rep); crtcs-- {
		res.crtcs = append(res.crtcs, le.Uint32(rep[off:]))
		off += 4
	}
	off += 4 * outputs
	for ; modes > 0 && off+randrModeInfoSize <= len(rep); modes-- {
		m := rep[off : off+randrModeInfoSize]
		res.modes[le.Uint32(m)] = modeRefresh(le.Uint32(m[8:]), le.Uint16(m[16:]), le.Uint16(m[24:]), le.Uint32(m[28:]))
		off += randrModeInfoSize
	}
	return res
}

// modeRefresh returns the refresh interval of a mode: its dot clock (Hz)
// over the dots of a frame, as xrandr computes it. A double scan mode sends
// each line twice; an interlaced one refreshes at each field, which has
// half the lines. 0 for a mode without timings (a virtual one).
func modeRefresh(dotClock uint32, htotal, vtotal uint16, flags uint32) time.Duration {
	dots := float64(htotal) * float64(vtotal)
	if dots == 0 {
		return 0
	}
	if flags&randrModeDoubleScan != 0 {
		dots *= 2
	}
	if flags&randrModeInterlace != 0 {
		dots /= 2
	}
	return platform.RefreshInterval(float64(dotClock) / dots)
}

// parseCRTC reads an RRGetCrtcInfo reply (the status at 1, x, y, width and
// height at 12, the mode at 20); ok is false for a CRTC that is off. The
// size is the one on the root window (rotated, or scaled by a transform).
func parseCRTC(rep []byte, modes map[uint32]time.Duration) (c crtc, ok bool) {
	if len(rep) < 32 || rep[1] != 0 {
		return crtc{}, false
	}
	x, y := int(int16(le.Uint16(rep[12:]))), int(int16(le.Uint16(rep[14:])))
	w, h := int(le.Uint16(rep[16:])), int(le.Uint16(rep[18:]))
	mode := le.Uint32(rep[20:])
	if mode == 0 || w == 0 || h == 0 {
		return crtc{}, false
	}
	return crtc{rect: image.Rect(x, y, x+w, y+h), refresh: modes[mode]}, true
}
