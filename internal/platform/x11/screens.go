//go:build linux

package x11

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
)

// The monitors present, for a position given at creation
// (WindowOptions.Position, a placement saved in an earlier run), which
// platform.FitOnScreens moves onto them when its monitor is gone. RandR
// 1.5's RRGetMonitors has no package in the vendored xgb (ADR-025): its
// two requests are encoded by hand, as XInput2's are.

const (
	randrOpQueryVersion = 0
	randrOpGetMonitors  = 42
)

// randrState is what the backend learned about RandR, asked the first
// time a window is created at a position.
type randrState struct {
	checked  bool
	opcode   byte
	monitors bool // RandR 1.5 or later: RRGetMonitors
}

// monitors returns the rectangles of the active monitors in root
// coordinates, the primary first: RRGetMonitors (RandR 1.5, X.Org 1.18 and
// later; it also reports the monitors a client defined with
// RRSetMonitor, which xrandr --setmonitor makes for a split screen or
// one of tiled outputs), else the root window's geometry, which covers
// every monitor.
func (s connServer) monitors() []image.Rectangle {
	b := s.b
	r := &b.randr
	if !r.checked {
		r.checked = true
		if e := b.queryExtension("RANDR"); e.present {
			req := newRequest(e.opcode, randrOpQueryVersion, 12)
			le.PutUint32(req[4:], 1)
			le.PutUint32(req[8:], 5)
			if rep, err := b.roundTrip(req, 16); err == nil {
				major, minor := le.Uint32(rep[8:]), le.Uint32(rep[12:])
				r.opcode, r.monitors = e.opcode, major > 1 || major == 1 && minor >= 5
			}
		}
	}
	if r.monitors {
		req := newRequest(r.opcode, randrOpGetMonitors, 12)
		le.PutUint32(req[4:], uint32(b.root))
		req[8] = 1 // get_active: only monitors that show something
		if rep, err := b.roundTrip(req, 32); err == nil {
			if mons := parseMonitors(rep); len(mons) > 0 {
				return mons
			}
		}
	}
	if b.screen == nil {
		return nil
	}
	return []image.Rectangle{image.Rect(0, 0, int(b.screen.WidthInPixels), int(b.screen.HeightInPixels))}
}

// parseMonitors reads an RRGetMonitors reply: a 32-byte header with the
// number of monitors at 12, then each MONITORINFO (name, primary,
// automatic, the number of outputs, x, y, width, height, the size in
// millimeters, and the outputs). The primary monitor goes first, the
// others in the server's order; monitors without a size are skipped.
func parseMonitors(rep []byte) []image.Rectangle {
	if len(rep) < 32 {
		return nil
	}
	n := int(le.Uint32(rep[12:]))
	var out []image.Rectangle
	primary := -1
	for off, i := 32, 0; i < n && off+24 <= len(rep); i++ {
		outputs := int(le.Uint16(rep[off+6:]))
		x, y := int(int16(le.Uint16(rep[off+8:]))), int(int16(le.Uint16(rep[off+10:])))
		w, h := int(le.Uint16(rep[off+12:])), int(le.Uint16(rep[off+14:]))
		if w > 0 && h > 0 {
			if rep[off+4] != 0 && primary < 0 {
				primary = len(out)
			}
			out = append(out, image.Rect(x, y, x+w, y+h))
		}
		off += 24 + 4*outputs
	}
	if primary > 0 {
		p := out[primary]
		copy(out[1:primary+1], out[:primary])
		out[0] = p
	}
	return out
}

// workAreas returns the work areas of the monitors present, the primary
// first: the monitors, with the primary one cut to _NET_WORKAREA of the
// current desktop when the window manager sets it (without the panels and
// docks that reserve struts).
//
// _NET_WORKAREA is one rectangle for the whole root window (EWMH predates
// several monitors), so with monitors of different sizes, or a panel on
// one monitor only, it can cut a strip off the others. GTK 3
// (gdk_x11_screen_get_monitor_workarea) therefore applies it to the
// primary monitor only, where the panels usually are; so does this.
// Mutter's per-monitor _GTK_WORKAREAS_D<n> is not read.
func (b *Backend) workAreas() []image.Rectangle {
	mons := b.srv.monitors()
	if len(mons) == 0 {
		return nil
	}
	a := b.atoms
	values, ok := b.srv.property32(b.root, a.netWorkarea)
	if !ok || len(values) < 4 {
		return mons
	}
	desk := 0
	if cur, ok := b.srv.property32(b.root, a.netCurrentDesktop); ok && len(cur) > 0 && int(cur[0]) >= 0 && len(values) >= 4*(int(cur[0])+1) {
		desk = int(cur[0])
	}
	v := values[4*desk:]
	work := image.Rect(int(int32(v[0])), int(int32(v[1])), int(int32(v[0]))+int(v[2]), int(int32(v[1]))+int(v[3]))
	return primaryWorkArea(mons, work)
}

// primaryWorkArea returns mons with the first (the primary) cut to work,
// unless they do not intersect (a stale or bogus _NET_WORKAREA).
func primaryWorkArea(mons []image.Rectangle, work image.Rectangle) []image.Rectangle {
	out := append([]image.Rectangle(nil), mons...)
	if in := out[0].Intersect(work); !in.Empty() {
		out[0] = in
	}
	return out
}

// fitPosition returns where a window created at opts.Position with the
// physical content size size goes: platform.FitOnScreens over the work
// areas, on the content rectangle (the frame's extents are not known
// before the window manager maps it: _NET_FRAME_EXTENTS comes after, and
// _NET_REQUEST_FRAME_EXTENTS is not universally answered). Window managers
// keep the title bar of a window whose content top is at the top of the
// work area visible themselves (mutter's, KWin's and Xfwm's constraints).
func (b *Backend) fitPosition(opts platform.WindowOptions, size image.Point) platform.WindowOptions {
	if !opts.HasPosition {
		return opts
	}
	works := b.workAreas()
	opts.Position = platform.FitOnScreens(image.Rectangle{Min: opts.Position, Max: opts.Position.Add(size)}, works).Min
	return opts
}
