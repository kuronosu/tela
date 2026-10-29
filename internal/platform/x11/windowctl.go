//go:build linux

package x11

import (
	"fmt"
	"image"
	"math"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
)

// Window control (phase 12, ADR-178, ADR-179): ICCCM and EWMH requests to
// the window manager, the state it reports, frameless windows with resize
// edges drawn by the app, caption hit regions, parents and modality. See
// "Window control" in the package documentation.
//
// Two rules shape everything here:
//
//   - A withdrawn window (never mapped, or hidden by SetVisible) is not
//     managed: EWMH says its client sets _NET_WM_STATE itself, and the
//     window manager removes the property when it withdraws a window. So
//     requests made while withdrawn only update w.req, and mapWindow
//     writes the whole wanted state (and the position) right before every
//     MapWindow, as GTK does; property changes seen while withdrawn are
//     ignored, so the state is kept across hide/show.
//   - A managed window gets client messages to the root window
//     (_NET_WM_STATE, WM_CHANGE_STATE, _NET_ACTIVE_WINDOW,
//     _NET_WM_MOVERESIZE), and its state is read back from _NET_WM_STATE
//     and WM_STATE at every PropertyNotify of them.

// wantState is what the app asked the window manager for, updated with
// what the window manager reports, so that a request equal to it is free
// and a request after the user changed the state is sent again.
type wantState struct {
	maximized, fullscreen, above bool
	pos                          image.Point
	hasPos                       bool
}

// _NET_WM_STATE client message actions (EWMH).
const (
	netWMStateRemove = 0
	netWMStateAdd    = 1
)

// sourceApplication is the EWMH source indication of requests made by a
// normal application (as opposed to pagers and task bars, 2).
const sourceApplication = 1

// _NET_WM_MOVERESIZE directions (EWMH). The first eight are the resize
// edges, clockwise from the top-left corner.
const (
	moveResizeTopLeft     = 0
	moveResizeTop         = 1
	moveResizeTopRight    = 2
	moveResizeRight       = 3
	moveResizeBottomRight = 4
	moveResizeBottom      = 5
	moveResizeBottomLeft  = 6
	moveResizeLeft        = 7
	moveResizeMove        = 8

	edgeNone = -1
)

// edgeCursors are the cursors shown over the resize edges, by direction.
var edgeCursors = [...]platform.Cursor{
	moveResizeTopLeft:     platform.CursorNWResize,
	moveResizeTop:         platform.CursorNResize,
	moveResizeTopRight:    platform.CursorNEResize,
	moveResizeRight:       platform.CursorEResize,
	moveResizeBottomRight: platform.CursorSEResize,
	moveResizeBottom:      platform.CursorSResize,
	moveResizeBottomLeft:  platform.CursorSWResize,
	moveResizeLeft:        platform.CursorWResize,
}

const (
	// edgeBand is the width of the resize band along the edges of a
	// frameless window, in logical pixels. X11 frameless windows have no
	// invisible shadow margin to put it in (GTK 3 and Chromium draw the
	// shadow inside the window and resize from there), so the band is
	// inside the content, like the 4–8 px borders window managers draw.
	edgeBand = 6
	// edgeCorner is how far from a corner, along either edge, the band
	// resizes diagonally: corners are easier to hit than the band is
	// wide (GTK's corner handles work the same way).
	edgeCorner = 16

	// defaultDoubleClick is the caption double-click interval when the
	// system settings give none (the X11 backend does not read
	// XSETTINGS yet): GTK's default gtk-double-click-time.
	defaultDoubleClick = 400 * time.Millisecond

	// corePointerDevice is the XInput2 id of the virtual core pointer,
	// sent in _GTK_SHOW_WINDOW_MENU for presses that arrived as core
	// events.
	corePointerDevice = 2
)

// resizeEdge returns the _NET_WM_MOVERESIZE direction of the resize band
// at p in a window of the given logical size, or edgeNone.
func resizeEdge(p platform.Point, size platform.Size) int {
	if p.X < 0 || p.Y < 0 || p.X >= size.W || p.Y >= size.H {
		return edgeNone
	}
	left, right := p.X < edgeBand, p.X >= size.W-edgeBand
	top, bottom := p.Y < edgeBand, p.Y >= size.H-edgeBand
	if !left && !right && !top && !bottom {
		return edgeNone
	}
	nearL, nearR := p.X < edgeCorner, p.X >= size.W-edgeCorner
	nearT, nearB := p.Y < edgeCorner, p.Y >= size.H-edgeCorner
	switch {
	case nearT && nearL:
		return moveResizeTopLeft
	case nearT && nearR:
		return moveResizeTopRight
	case nearB && nearL:
		return moveResizeBottomLeft
	case nearB && nearR:
		return moveResizeBottomRight
	case top:
		return moveResizeTop
	case bottom:
		return moveResizeBottom
	case left:
		return moveResizeLeft
	}
	return moveResizeRight
}

// --- the server side, replaceable in tests ---

// wmServer is what window control needs from the X connection: requests
// to the window manager and the two queries it reads state with. The
// backend uses connServer; unit tests record the requests instead.
type wmServer interface {
	// clientMessage sends a format-32 ClientMessage about win to the root
	// window, with SubstructureRedirect|SubstructureNotify, as ICCCM and
	// EWMH require for requests to the window manager.
	clientMessage(win xproto.Window, typ xproto.Atom, data ...uint32)
	// withdrawn sends the synthetic UnmapNotify that follows the
	// UnmapWindow of a withdrawal (ICCCM 4.1.4), which tells the window
	// manager even when the window is iconified (already unmapped).
	withdrawn(win xproto.Window)
	setProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte)
	deleteProperty(win xproto.Window, prop xproto.Atom)
	configure(win xproto.Window, mask uint16, values []uint32)
	mapWindow(win xproto.Window, mapped bool)
	// ungrabPointer releases the implicit grab of a press, so that the
	// window manager can grab the pointer for a move or a menu.
	ungrabPointer(device uint16)
	setCursor(win xproto.Window, c platform.Cursor)
	// property32 reads a format-32 property (a round trip); ok is false
	// if it does not exist.
	property32(win xproto.Window, prop xproto.Atom) (values []uint32, ok bool)
	// rootPosition returns where the top-left of win is in root
	// coordinates (a TranslateCoordinates round trip).
	rootPosition(win xproto.Window) (image.Point, bool)
	// destroy frees a window and its graphics context.
	destroy(win xproto.Window, gc xproto.Gcontext)
}

// connServer implements wmServer on the backend's connection.
type connServer struct{ b *Backend }

const xiOpUngrabDevice = 52

func (s connServer) clientMessage(win xproto.Window, typ xproto.Atom, data ...uint32) {
	var d [5]uint32
	copy(d[:], data)
	ev := xproto.ClientMessageEvent{Format: 32, Window: win, Type: typ, Data: xproto.ClientMessageDataUnionData32New(d[:])}
	s.sendToRoot(ev.Bytes())
}

func (s connServer) withdrawn(win xproto.Window) {
	ev := xproto.UnmapNotifyEvent{Event: s.b.root, Window: win}
	s.sendToRoot(ev.Bytes())
}

func (s connServer) sendToRoot(ev []byte) {
	xproto.SendEvent(s.b.conn, false, s.b.root,
		xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect, string(ev))
}

// setProperty sends small properties with xgb's request and large ones
// (icons) hand-encoded: one BIG-REQUESTS request when it fits the
// backend's request limit, else a Replace followed by Appends.
func (s connServer) setProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	b := s.b
	const hdr = 24
	if hdr+len(data) <= 0xffff*4 {
		xproto.ChangeProperty(b.conn, xproto.PropModeReplace, win, prop, typ, format, uint32(len(data))/uint32(format/8), data)
		return
	}
	limit := max(b.maxRequest, 0xffff*4) - hdr - bigHeaderExtra
	mode := byte(xproto.PropModeReplace)
	for len(data) > 0 {
		n := min(len(data), limit&^3)
		big := b.bigRequests && hdr+n > 0xffff*4
		if !big {
			n = min(n, (0xffff*4-hdr)&^3)
		}
		b.conn.NewRequest(encodeChangeProperty(big, mode, win, prop, typ, format, data[:n]), b.conn.NewCookie(false, false))
		data = data[n:]
		mode = xproto.PropModeAppend
	}
}

func (s connServer) deleteProperty(win xproto.Window, prop xproto.Atom) {
	xproto.DeleteProperty(s.b.conn, win, prop)
}

func (s connServer) configure(win xproto.Window, mask uint16, values []uint32) {
	xproto.ConfigureWindow(s.b.conn, win, mask, values)
}

func (s connServer) mapWindow(win xproto.Window, mapped bool) {
	if mapped {
		xproto.MapWindow(s.b.conn, win)
	} else {
		xproto.UnmapWindow(s.b.conn, win)
	}
}

// ungrabPointer releases both kinds of implicit grab: a press delivered
// as an XInput2 event activates an XI2 grab, which the core UngrabPointer
// does not release (the server matches the grab type), so the XI2
// request goes too. GTK 3 ungrabs the seat before _NET_WM_MOVERESIZE;
// winit sends the core UngrabPointer.
func (s connServer) ungrabPointer(device uint16) {
	b := s.b
	xproto.UngrabPointer(b.conn, xproto.TimeCurrentTime)
	if b.xi.ok && device != 0 {
		req := newRequest(b.xi.opcode, xiOpUngrabDevice, 12)
		le.PutUint32(req[4:], xproto.TimeCurrentTime)
		le.PutUint16(req[8:], device)
		b.sendRequest(req)
	}
}

func (s connServer) setCursor(win xproto.Window, c platform.Cursor) {
	xproto.ChangeWindowAttributes(s.b.conn, win, xproto.CwCursor, []uint32{uint32(s.b.cursorFor(c))})
}

func (s connServer) property32(win xproto.Window, prop xproto.Atom) ([]uint32, bool) {
	r, err := xproto.GetProperty(s.b.conn, false, win, prop, xproto.GetPropertyTypeAny, 0, 1<<12).Reply()
	if err != nil || r == nil || r.Format != 32 {
		return nil, false
	}
	out := make([]uint32, len(r.Value)/4)
	for i := range out {
		out[i] = le.Uint32(r.Value[4*i:])
	}
	return out, true
}

func (s connServer) rootPosition(win xproto.Window) (image.Point, bool) {
	r, err := xproto.TranslateCoordinates(s.b.conn, win, s.b.root, 0, 0).Reply()
	if err != nil || r == nil {
		return image.Point{}, false
	}
	return image.Pt(int(r.DstX), int(r.DstY)), true
}

func (s connServer) destroy(win xproto.Window, gc xproto.Gcontext) {
	xproto.FreeGC(s.b.conn, gc)
	xproto.DestroyWindow(s.b.conn, win)
}

// readSupported reads _NET_SUPPORTED from the root window: the EWMH hints
// the running window manager implements (none without an EWMH window
// manager). It is read again when the property changes (a new window
// manager).
func (b *Backend) readSupported() {
	b.supported = map[xproto.Atom]bool{}
	values, _ := b.srv.property32(b.root, b.atoms.netSupported)
	for _, a := range values {
		b.supported[xproto.Atom(a)] = true
	}
}

// --- window setup and state ---

// initCtl sets up the window control state of a new window from its
// resolved options and writes the properties that must be in place before
// the window is first mapped. b.windows does not contain w yet.
func (w *Window) initCtl(opts platform.WindowOptions) {
	b, a := w.b, w.b.atoms
	w.visible = !opts.Hidden
	w.edge = edgeNone
	w.req = wantState{maximized: opts.Maximized, fullscreen: opts.Fullscreen, above: opts.AlwaysOnTop,
		pos: opts.Position, hasPos: opts.HasPosition}
	w.osState.ClientSideDecorations = opts.Frameless
	w.modal = opts.Modal
	w.parentWin = b.root

	windowType := a.netWMTypeNormal
	if opts.Modal {
		windowType = a.netWMTypeDialog
	}
	b.setProperty(w.id, a.netWMWindowType, xproto.AtomAtom, 32, card32s(uint32(windowType)))
	if opts.Frameless {
		b.setProperty(w.id, a.motifWMHints, a.motifWMHints, 32, motifNoDecorations())
	}
	if w.parent != nil {
		b.setProperty(w.id, xproto.AtomWmTransientFor, xproto.AtomWindow, 32, card32s(uint32(w.parent.id)))
	}
}

// parentOf checks WindowOptions.Parent: an open window of this backend.
func (b *Backend) parentOf(opts platform.WindowOptions) *Window {
	if opts.Parent == nil {
		return nil
	}
	p, ok := opts.Parent.(*Window)
	if !ok || p.b != b {
		panic(fmt.Sprintf("x11: WindowOptions.Parent is a %T of another backend; the parent must be an open window of the same backend", opts.Parent))
	}
	if p.Closed() {
		panic("x11: WindowOptions.Parent is closed; the parent must be an open window")
	}
	return p
}

// netWMStateAtoms returns the _NET_WM_STATE property a withdrawn window is
// mapped with: the wanted state.
func (w *Window) netWMStateAtoms() []byte {
	a := w.b.atoms
	var v []uint32
	if w.req.maximized {
		v = append(v, uint32(a.netWMStateMaxVert), uint32(a.netWMStateMaxHorz))
	}
	if w.req.fullscreen {
		v = append(v, uint32(a.netWMStateFull))
	}
	if w.req.above {
		v = append(v, uint32(a.netWMStateAbove))
	}
	if w.modal {
		v = append(v, uint32(a.netWMStateModal))
	}
	return card32s(v...)
}

// mapWindow hands the withdrawn window to the window manager with the
// wanted state: position (hints and the window's own position, both
// read at map with StaticGravity), WM_HINTS initial state (Iconic if
// minimized while withdrawn), _NET_WM_STATE, then MapWindow. The Expose
// that follows paints the retained frame.
func (w *Window) mapWindow() {
	b := w.b
	if w.req.hasPos {
		b.srv.configure(w.id, xproto.ConfigWindowX|xproto.ConfigWindowY,
			[]uint32{uint32(int32(w.req.pos.X)), uint32(int32(w.req.pos.Y))})
	}
	w.updateNormalHints()
	if w.startIconic != w.iconicHint {
		w.iconicHint = w.startIconic
		b.setProperty(w.id, xproto.AtomWmHints, xproto.AtomWmHints, 32, wmHints(w.iconicHint))
	}
	w.startIconic = false
	b.setProperty(w.id, b.atoms.netWMState, xproto.AtomAtom, 32, w.netWMStateAtoms())
	b.srv.mapWindow(w.id, true)
	w.mapped = true
	w.updateHidden()
}

// updateHidden pauses frames while the window is hidden by the app, unmapped
// by the window manager after being mapped (iconified), or minimized (some
// compositing window managers keep minimized windows mapped).
func (w *Window) updateHidden() {
	w.SetHidden(!w.visible || (w.mapped && w.osUnmapped) || w.osState.Minimized)
}

// setOSState records a state reported by the server and queues it for
// the app. The wanted state follows it (see wantState).
func (w *Window) setOSState(st platform.WindowState) {
	if st == w.osState || w.Closed() {
		return
	}
	old := w.osState
	if st.Maximized != old.Maximized {
		w.req.maximized = st.Maximized
	}
	if st.Fullscreen != old.Fullscreen {
		w.req.fullscreen = st.Fullscreen
	}
	if st.HasPosition && (!old.HasPosition || st.Position != old.Position) {
		w.req.pos, w.req.hasPos = st.Position, true
	}
	w.osState = st
	w.b.loop.UpdateState(w, st)
	w.updateHidden()
	if w.edge != edgeNone && (st.Maximized || st.Fullscreen) {
		w.edge = edgeNone
		w.applyCursor()
	}
}

// refreshState reads _NET_WM_STATE and WM_STATE after one of them changed
// (or the window was mapped). A withdrawn window keeps its state: the
// window manager clears the properties when it withdraws a window.
func (w *Window) refreshState() {
	b, a := w.b, w.b.atoms
	if !w.mapped || w.Closed() {
		return
	}
	st := w.osState
	st.Maximized, st.Fullscreen, st.Minimized = false, false, false
	if b.supported[a.netWMState] {
		values, _ := b.srv.property32(w.id, a.netWMState)
		var vert, horz bool
		for _, v := range values {
			switch xproto.Atom(v) {
			case a.netWMStateMaxVert:
				vert = true
			case a.netWMStateMaxHorz:
				horz = true
			case a.netWMStateFull:
				st.Fullscreen = true
			case a.netWMStateHidden:
				st.Minimized = true
			}
		}
		// Maximized in one direction only is a tiling half or a
		// vertical maximize, not "maximized" (GTK's rule).
		st.Maximized = vert && horz
		w.req.above = slices.Contains(values, uint32(a.netWMStateAbove))
	}
	if values, ok := b.srv.property32(w.id, a.wmState); ok && len(values) > 0 && values[0] == stateIconic {
		st.Minimized = true
	}
	w.setOSState(st)
}

// reportPosition records the content area's root position, while the
// window is managed.
func (w *Window) reportPosition(p image.Point) {
	if !w.mapped {
		return
	}
	st := w.osState
	st.HasPosition, st.Position = true, p
	w.setOSState(st)
}

// endBatch runs after a batch of events is translated: the positions of
// reparented windows that moved are read with one TranslateCoordinates
// each, instead of one per ConfigureNotify of a drag.
func (b *Backend) endBatch() {
	for _, w := range b.loop.Windows() {
		if !w.posDirty {
			continue
		}
		w.posDirty = false
		if !w.mapped || b.lost.Load() {
			continue
		}
		if p, ok := b.srv.rootPosition(w.id); ok {
			w.reportPosition(p)
		}
	}
}

// configured handles the position part of a ConfigureNotify. Real events
// of a window whose parent is the root carry root coordinates; a window
// reparented into a frame gets real events relative to the frame and
// synthetic ones in root coordinates (ICCCM 4.1.5), which xgb does not
// tell apart, so its position is read back at the end of the batch (GTK
// translates the same way for non-synthetic events).
func (w *Window) configured(ev xproto.ConfigureNotifyEvent) {
	if w.parentWin == w.b.root {
		bw := int(ev.BorderWidth)
		w.reportPosition(image.Pt(int(ev.X)+bw, int(ev.Y)+bw))
		return
	}
	w.posDirty = true
}

// closeChildren closes w's child windows, in creation order.
func (w *Window) closeChildren() {
	for _, c := range slices.Clone(w.b.loop.Windows()) {
		if c.parent == w {
			c.Close()
		}
	}
}

// --- platform.Window: window control ---

// sendState asks the window manager to add or remove one or two
// _NET_WM_STATE atoms of a managed window.
func (w *Window) sendState(add bool, first, second xproto.Atom) {
	action := uint32(netWMStateRemove)
	if add {
		action = netWMStateAdd
	}
	w.b.srv.clientMessage(w.id, w.b.atoms.netWMState, action, uint32(first), uint32(second), sourceApplication)
}

// deiconify maps an iconified window, which is how ICCCM 4.1.4 brings it
// back to the normal state; a minimize requested before the window was
// mapped is dropped instead.
func (w *Window) deiconify() {
	w.startIconic = false
	if w.mapped && w.osState.Minimized {
		w.b.srv.mapWindow(w.id, true)
	}
}

// Minimize iconifies the window with WM_CHANGE_STATE IconicState (ICCCM
// 4.1.4). A window not mapped yet (or hidden) is mapped iconified later.
func (w *Window) Minimize() {
	if w.Closed() || w.osState.Minimized {
		return
	}
	if !w.mapped {
		w.startIconic = true
		return
	}
	w.b.srv.clientMessage(w.id, w.b.atoms.wmChangeState, stateIconic)
}

// SetMaximized adds or removes _NET_WM_STATE_MAXIMIZED_VERT and _HORZ. It
// also brings back a minimized window, like the headless backend.
func (w *Window) SetMaximized(maximized bool) {
	if w.Closed() {
		return
	}
	w.deiconify()
	if w.req.maximized == maximized {
		return
	}
	w.req.maximized = maximized
	if w.mapped {
		w.sendState(maximized, w.b.atoms.netWMStateMaxVert, w.b.atoms.netWMStateMaxHorz)
	}
}

// SetFullscreen adds or removes _NET_WM_STATE_FULLSCREEN. It also brings
// back a minimized window.
func (w *Window) SetFullscreen(fullscreen bool) {
	if w.Closed() {
		return
	}
	w.deiconify()
	if w.req.fullscreen == fullscreen {
		return
	}
	w.req.fullscreen = fullscreen
	if w.mapped {
		w.sendState(fullscreen, w.b.atoms.netWMStateFull, 0)
	}
}

// SetVisible withdraws the window (UnmapWindow and the synthetic
// UnmapNotify of ICCCM 4.1.4) or maps it again with the state it had,
// where it was. A window never presented is mapped at its first Present.
func (w *Window) SetVisible(visible bool) {
	if w.Closed() || w.visible == visible {
		return
	}
	w.visible = visible
	switch {
	case !visible && w.mapped:
		w.startIconic = w.osState.Minimized
		w.b.srv.mapWindow(w.id, false)
		w.b.srv.withdrawn(w.id)
		w.mapped = false
	case visible && w.pix != nil:
		w.mapWindow()
	}
	w.updateHidden()
}

// SetTitleBarColors does nothing: only Windows 11 draws colored title bars.
func (w *Window) SetTitleBarColors(platform.TitleBarColors) {}

// SetBackdrop does nothing: system backdrops are a Windows 11 material.
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
}

// SetAlwaysOnTop adds or removes _NET_WM_STATE_ABOVE.
func (w *Window) SetAlwaysOnTop(onTop bool) {
	if w.Closed() || w.req.above == onTop {
		return
	}
	w.req.above = onTop
	if w.mapped {
		w.sendState(onTop, w.b.atoms.netWMStateAbove, 0)
	}
}

// SetMinSize changes the minimum size in WM_NORMAL_HINTS and asks for the
// clamped size if the window is smaller.
func (w *Window) SetMinSize(size platform.Size) {
	platform.CheckSize("SetMinSize size", size)
	w.setLimits(func() { w.opts.MinSize = size })
}

// SetMaxSize changes the maximum size in WM_NORMAL_HINTS and asks for the
// clamped size if the window is larger.
func (w *Window) SetMaxSize(size platform.Size) {
	platform.CheckSize("SetMaxSize size", size)
	w.setLimits(func() { w.opts.MaxSize = size })
}

func (w *Window) setLimits(change func()) {
	if w.Closed() {
		return
	}
	change()
	w.opts.Size = w.opts.ClampSize(w.opts.Size)
	w.updateNormalHints()
	if w.osState.Maximized || w.osState.Fullscreen {
		return // the window manager applies the limits to these sizes
	}
	if cur := w.osMetrics().Size; w.opts.ClampSize(cur) != cur {
		w.SetSize(w.opts.ClampSize(cur))
	}
}

// SetPosition moves the window so that its content area's top-left is at
// pos in root coordinates: ConfigureWindow with StaticGravity in the
// hints, and the hints' position for a window not mapped yet.
func (w *Window) SetPosition(pos image.Point) {
	if w.Closed() || w.req.hasPos && w.req.pos == pos {
		return
	}
	w.req.pos, w.req.hasPos = pos, true
	if !w.mapped {
		w.updateNormalHints()
	}
	w.b.srv.configure(w.id, xproto.ConfigWindowX|xproto.ConfigWindowY, []uint32{uint32(int32(pos.X)), uint32(int32(pos.Y))})
}

// Activate sends _NET_ACTIVE_WINDOW with source indication "application",
// the timestamp of the latest user input and the window of the app that
// has the focus (EWMH); focus stealing prevention may refuse it. A
// minimized window is mapped first (ICCCM's way back from iconic, for
// window managers that only raise on _NET_ACTIVE_WINDOW). A hidden window
// stays hidden.
func (w *Window) Activate() {
	if w.Closed() || !w.visible {
		return
	}
	w.deiconify()
	if !w.mapped {
		return // mapped at its first Present, where the window manager focuses it
	}
	var active xproto.Window
	for _, o := range w.b.loop.Windows() {
		if o.focused {
			active = o.id
		}
	}
	w.b.srv.clientMessage(w.id, w.b.atoms.netActiveWindow, sourceApplication, w.b.userTime, uint32(active))
}

// SetIcon sets _NET_WM_ICON with every size (or deletes it for nil).
func (w *Window) SetIcon(images []image.Image) {
	platform.CheckIcon("SetIcon", images)
	if w.Closed() {
		return
	}
	if len(images) == 0 {
		w.b.srv.deleteProperty(w.id, w.b.atoms.netWMIcon)
		return
	}
	w.b.setProperty(w.id, w.b.atoms.netWMIcon, xproto.AtomCardinal, 32, netWMIcon(images))
}

// SetHitRegions keeps a copy of the regions; presses are checked against
// them (osPress).
func (w *Window) SetHitRegions(regions []platform.HitRegion) {
	platform.CheckHitRegions(regions)
	if w.Closed() || platform.EqualHitRegions(w.regions, regions) {
		return
	}
	w.regions = slices.Clone(regions)
}

// RestoreToken returns "": X11 restores windows by position
// (WindowOptions.Position), which needs no token.
func (w *Window) RestoreToken() string { return "" }

// --- pointer: resize edges and caption regions ---

// edgeAt returns the resize band under p (logical window coordinates):
// only frameless windows that can be resized, outside maximized and
// fullscreen, have one.
func (w *Window) edgeAt(p platform.Point) int {
	if !w.osState.ClientSideDecorations || w.opts.FixedSize || w.osState.Maximized || w.osState.Fullscreen {
		return edgeNone
	}
	return resizeEdge(p, w.osMetrics().Size)
}

// hoverEdge shows the resize cursor while the pointer is over the band.
func (w *Window) hoverEdge(p platform.Point) {
	if e := w.edgeAt(p); e != w.edge {
		w.edge = e
		w.applyCursor()
	}
}

// applyCursor shows the app's cursor, or the resize cursor over the band.
func (w *Window) applyCursor() {
	c := w.cursor
	if w.edge != edgeNone {
		c = edgeCursors[w.edge]
	}
	if c != w.shown {
		w.shown = c
		w.b.srv.setCursor(w.id, c)
	}
}

// osPress decides whether a button press belongs to the OS: a press in
// the resize band (which wins over hit regions) or in a caption region,
// when the app holds no button. It performs the OS action and returns
// true when the press, and the rest of its sequence up to the release of
// every button, must not be delivered (w.osHeld).
func (b *Backend) osPress(p ptr, btn platform.PointerButton, pos platform.Point) bool {
	w := p.w
	if w.osHeld != 0 {
		w.osHeld |= btn.Mask()
		return true
	}
	if p.held != 0 {
		return false // a press of the app's sequence
	}
	if dir := w.edgeAt(pos); dir != edgeNone {
		w.osHeld = btn.Mask()
		if btn == platform.ButtonPrimary {
			b.moveResize(p, dir)
		}
		return true
	}
	if platform.HitTest(w.regions, pos) != platform.HitCaption {
		return false
	}
	w.osHeld = btn.Mask()
	switch btn {
	case platform.ButtonPrimary:
		if w.captions.Press(b.Now(), pos, b.Settings().DoubleClickInterval, defaultDoubleClick) {
			// The window manager handles double clicks on its own
			// title bar only; on ours the backend toggles maximize
			// (GTK does the same for its client-side title bars).
			w.SetMaximized(!w.osState.Maximized)
			return true
		}
		b.moveResize(p, moveResizeMove)
	case platform.ButtonSecondary:
		b.showWindowMenu(p)
	}
	return true
}

// osPointer reports whether a pointer event other than a press belongs to
// the OS's sequence (w.osHeld), updating it: a release ends the button's
// part, and buttons the event says are no longer held (their release went
// to another client after the window manager grabbed the pointer) are
// forgotten.
func (w *Window) osPointer(p ptr, release platform.PointerButton) bool {
	w.osHeld &= p.held
	if w.osHeld == 0 {
		return false
	}
	if release != platform.ButtonNone {
		w.osHeld &^= release.Mask()
	}
	return true
}

// rootPoint returns the press position in root coordinates, rounded.
func (p ptr) rootPoint() (uint32, uint32) {
	return uint32(int32(math.Round(p.rootX))), uint32(int32(math.Round(p.rootY)))
}

// moveResize starts an interactive move or resize by the window manager:
// the implicit grab of the press is released first (or the window
// manager's grab fails), then _NET_WM_MOVERESIZE goes with the root
// position, the direction, button 1 and source indication "application",
// as GTK 3 and winit send it. Without the hint in _NET_SUPPORTED nothing
// happens (the press is still the OS's).
func (b *Backend) moveResize(p ptr, dir int) {
	if !b.supported[b.atoms.netWMMoveResize] {
		return
	}
	x, y := p.rootPoint()
	b.srv.ungrabPointer(p.device)
	b.srv.clientMessage(p.w.id, b.atoms.netWMMoveResize, x, y, uint32(dir), 1, sourceApplication)
}

// showWindowMenu asks the window manager for its window menu at the
// pointer with _GTK_SHOW_WINDOW_MENU (device id, root x, root y; GTK 3's
// message, implemented by mutter, KWin, Xfwm4 and Openbox among others),
// when _NET_SUPPORTED lists it.
func (b *Backend) showWindowMenu(p ptr) {
	if !b.supported[b.atoms.gtkShowWindowMenu] {
		return
	}
	device := p.device
	if device == 0 {
		device = corePointerDevice
	}
	x, y := p.rootPoint()
	b.srv.ungrabPointer(p.device)
	b.srv.clientMessage(p.w.id, b.atoms.gtkShowWindowMenu, uint32(device), x, y)
}
