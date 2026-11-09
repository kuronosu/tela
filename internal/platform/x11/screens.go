//go:build linux

package x11

import (
	"fmt"
	"image"
	"time"

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
// first, for the current desktop (_NET_CURRENT_DESKTOP, else the first):
//
//   - Mutter's per-monitor _GTK_WORKAREAS_D<desktop> when the window
//     manager lists _GTK_WORKAREAS in _NET_SUPPORTED (the property of a
//     window manager that ran before could be stale): x, y, width and
//     height per monitor, in root coordinates. Each monitor is cut to the
//     work areas that overlap it, as GTK 3 does
//     (_gdk_x11_screen_get_monitor_work_area), which does not rely on the
//     order of the list matching the monitors'.
//   - Else the monitors, with the primary one cut to _NET_WORKAREA of the
//     desktop. _NET_WORKAREA is one rectangle for the whole root window
//     (EWMH predates several monitors), so with monitors of different
//     sizes, or a panel on one monitor only, it can cut a strip off the
//     others. GTK 3 (gdk_x11_screen_get_monitor_workarea) therefore applies
//     it to the primary monitor only, where the panels usually are; so
//     does this.
func (b *Backend) workAreas() []image.Rectangle {
	mons := b.srv.monitors()
	if len(mons) == 0 {
		return nil
	}
	a := b.atoms
	desk := -1
	if cur, ok := b.srv.property32(b.root, a.netCurrentDesktop); ok && len(cur) > 0 && int32(cur[0]) >= 0 {
		desk = int(cur[0])
	}
	if b.supported[a.gtkWorkareas] {
		if v, ok := b.srv.namedProperty32(b.root, fmt.Sprintf("_GTK_WORKAREAS_D%d", max(desk, 0))); ok {
			if works := parseWorkAreas(v); len(works) > 0 {
				return cutToWorkAreas(mons, works)
			}
		}
	}
	values, ok := b.srv.property32(b.root, a.netWorkarea)
	if !ok || len(values) < 4 {
		return mons
	}
	if desk < 0 || len(values) < 4*(desk+1) {
		desk = 0
	}
	v := values[4*desk:]
	work := image.Rect(int(int32(v[0])), int(int32(v[1])), int(int32(v[0]))+int(v[2]), int(int32(v[1]))+int(v[3]))
	return primaryWorkArea(mons, work)
}

// parseWorkAreas reads _GTK_WORKAREAS_D<n>: x, y, width and height per
// monitor. A list whose length is not a multiple of four is rejected
// whole, as GTK does; empty rectangles are skipped.
func parseWorkAreas(v []uint32) []image.Rectangle {
	if len(v)%4 != 0 {
		return nil
	}
	var out []image.Rectangle
	for ; len(v) >= 4; v = v[4:] {
		x, y, w, h := int(int32(v[0])), int(int32(v[1])), int(int32(v[2])), int(int32(v[3]))
		if w > 0 && h > 0 {
			out = append(out, image.Rect(x, y, x+w, y+h))
		}
	}
	return out
}

// cutToWorkAreas returns each monitor cut to the work areas that overlap
// it (GTK 3's loop: each one that still intersects the area narrows it); a
// monitor no work area overlaps stays whole.
func cutToWorkAreas(mons, works []image.Rectangle) []image.Rectangle {
	out := make([]image.Rectangle, len(mons))
	for i, m := range mons {
		for _, w := range works {
			if in := m.Intersect(w); !in.Empty() {
				m = in
			}
		}
		out[i] = m
	}
	return out
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

// The frame a window manager puts around a window, for fitting a position
// given at creation: the rectangle that must stay on a monitor is the
// frame (its title bar is what the user grabs), not the content.
//
// EWMH's _NET_REQUEST_FRAME_EXTENTS asks the window manager, before map,
// to set _NET_FRAME_EXTENTS (left, right, top, bottom) to its estimate for
// the window as its properties are then; so it is sent once the window
// has all of them (type, _MOTIF_WM_HINTS, WM_TRANSIENT_FOR, hints), and
// only when _NET_SUPPORTED lists it (mutter, KWin, Xfwm4 and Openbox do;
// tiling window managers such as i3, dwm and xmonad have no frame
// extents at all). The first map waits for the answer at most
// frameExtentsTimeout after the request (the PropertyNotify ends the
// wait; the frame was already presented to the retained copy, which the
// Expose after the map paints), then fits without extents. A late answer
// is ignored. The values are mutter's visible borders (its invisible
// resize borders are not in them), which is what must be on screen.
//
// Frameless windows are not asked: their _MOTIF_WM_HINTS has no
// decorations, and mutter, Muffin and Marco report the extents of a
// decorated frame anyway (winit's frame-extents heuristic and SDL's
// X11_GetBorderValues ignore them for borderless windows for that
// reason). Neither winit nor SDL sends the request: they read
// _NET_FRAME_EXTENTS after map only, when moving a window that is
// already managed.

// frameExtentsTimeout bounds how long the first map of a window created
// at a position waits for the window manager's _NET_FRAME_EXTENTS.
const frameExtentsTimeout = 100 * time.Millisecond

// frameExtents is _NET_FRAME_EXTENTS, in physical pixels.
type frameExtents struct{ left, right, top, bottom int }

// maxFrameExtent bounds each side of a believable frame: a property with
// a larger value is ignored (a bogus window manager would otherwise push
// windows far from where they belong).
const maxFrameExtent = 1 << 12

// parseFrameExtents reads _NET_FRAME_EXTENTS (four CARDINALs: left,
// right, top, bottom).
func parseFrameExtents(v []uint32) (frameExtents, bool) {
	if len(v) < 4 {
		return frameExtents{}, false
	}
	for _, x := range v[:4] {
		if x > maxFrameExtent {
			return frameExtents{}, false
		}
	}
	return frameExtents{left: int(v[0]), right: int(v[1]), top: int(v[2]), bottom: int(v[3])}, true
}

// fitFramed returns where the content of a window goes whose content
// rectangle is content and whose frame has the extents ext:
// platform.FitOnScreens on the frame (the content grown by the extents)
// over the work areas, and the content moved with the frame.
func fitFramed(content image.Rectangle, ext frameExtents, works []image.Rectangle) image.Point {
	frame := image.Rect(content.Min.X-ext.left, content.Min.Y-ext.top, content.Max.X+ext.right, content.Max.Y+ext.bottom)
	return content.Min.Add(platform.FitOnScreens(frame, works).Min.Sub(frame.Min))
}

// requestFit prepares the fit of a position given at creation
// (WindowOptions.Position): it marks it for the first map, and asks the
// window manager for the frame extents when it can tell them (see above).
// It runs once the window has every property it is mapped with.
func (w *Window) requestFit() {
	b, a := w.b, w.b.atoms
	if !w.req.hasPos {
		return
	}
	w.fitPending = true
	if w.opts.Frameless || !b.supported[a.netReqFrameExt] {
		return
	}
	b.srv.clientMessage(w.id, a.netReqFrameExt)
	w.extWait, w.extSent = true, b.Now()
}

// fitCreation runs at the first map of a window created at a position
// (before the position is written): it waits for the frame extents if they
// were asked for and the wait is not over, returning false (the map
// happens when they arrive or the wait ends), else fits w.req.pos onto the
// monitors present.
func (w *Window) fitCreation() bool {
	b := w.b
	if w.extWait {
		if left := frameExtentsTimeout - (b.Now() - w.extSent); left > 0 {
			if !w.mapWaiting {
				w.mapWaiting = true
				w.extStop = b.after(left, w.frameExtentsDone)
			}
			return false
		}
		w.extWait = false
	}
	w.fitPending = false
	size := clampPhysical(platform.PhysicalSize(w.opts.Size, b.scale))
	w.req.pos = fitFramed(image.Rectangle{Min: w.req.pos, Max: w.req.pos.Add(size)}, w.ext, b.workAreas())
	return true
}

// frameExtentsChanged handles a PropertyNotify of _NET_FRAME_EXTENTS: the
// answer to the request of requestFit ends the wait.
func (w *Window) frameExtentsChanged() {
	if !w.extWait || w.Closed() {
		return
	}
	v, _ := w.b.srv.property32(w.id, w.b.atoms.netFrameExtents)
	ext, ok := parseFrameExtents(v)
	if !ok {
		return
	}
	w.ext = ext
	w.frameExtentsDone()
}

// frameExtentsDone ends the wait for the frame extents (they arrived, or
// the time is up) and maps the window if its first map was waiting.
func (w *Window) frameExtentsDone() {
	w.extWait = false
	if w.extStop != nil {
		w.extStop()
		w.extStop = nil
	}
	if w.mapWaiting {
		w.mapWaiting = false
		if w.visible && !w.mapped && !w.Closed() {
			w.mapWindow()
		}
	}
}
