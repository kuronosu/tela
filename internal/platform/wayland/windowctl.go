//go:build linux

package wayland

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Window control (phase 12, ADR-178, ADR-179; "Window control", "Custom
// title bars", "Parent windows and modality" and "Session restore" in
// package platform). Wayland clients request and the compositor decides:
// every request here is fire-and-forget, and the outcome comes back as
// xdg_toplevel.configure states, which onConfigure (window.go) reports
// with the loop's UpdateState.

const (
	// resizeBand is the width of the resize edges of a window with
	// client-side decorations, in logical pixels, inside the content
	// (Tela draws no shadow to put them in, unlike GTK), and resizeCorner
	// how far along an edge its corners reach. Chromium's Linux frame uses
	// the same 5 and 16.
	resizeBand   = 5
	resizeCorner = 16
	// captionDragThreshold is how far (logical pixels) a primary press in
	// a caption region must move before it becomes an interactive move,
	// GTK's drag threshold: a click that does not move never starts a
	// compositor grab, so double clicks are seen whole (GtkWindowHandle
	// does the same).
	captionDragThreshold = 8
	// defaultDoubleClick is the caption double-click interval when the
	// system settings do not give one (GTK's gtk-double-click-time), and
	// doubleClickSlop how far apart (logical pixels) its two presses may
	// be (the headless backend's rule).
	defaultDoubleClick = 400 * time.Millisecond
	doubleClickSlop    = 4
)

// windowCtl is the window control state of a Window.
type windowCtl struct {
	// wantMax and wantFull are the maximized and fullscreen states the
	// window asked for last, or the compositor's latest configure states
	// (whichever came last); they are asked again when the window is
	// shown after being hidden.
	wantMax, wantFull bool

	// Visibility. hidden is the app's choice (SetVisible,
	// WindowOptions.Hidden). unmapped: the mapped window was hidden, so a
	// null buffer was committed and the role objects destroyed (unmap);
	// showing it takes new ones and a new initial commit. awaitConfigure:
	// that commit was sent and its configure has not arrived (no buffer may
	// be attached before it).
	hidden         bool
	unmapped       bool
	awaitConfigure bool

	// activationToken is an xdg_activation_v1 token that arrived while
	// the window was not mapped (Activate right after SetVisible(true)):
	// it is used when the window maps, since compositors ignore the
	// activation of a surface they do not show (sway).
	activationToken string

	// Decorations: the zxdg_toplevel_decoration_v1 mode latched by its
	// configure event (pendingDeco) and applied with the xdg_surface
	// configure that ends the sequence (decoMode); 0 until the compositor
	// reports one.
	pendingDeco, decoMode protocols.ZxdgToplevelDecorationV1Mode
	// decorated: the latest role objects include a
	// zxdg_toplevel_decoration_v1 (the compositor has xdg-decoration).
	decorated bool

	regions []platform.HitRegion // custom title bar (SetHitRegions)
	icon    []image.Image        // copies (SetIcon), sent again when shown

	// Caption double clicks: the last primary caption press that did not
	// become a move.
	lastCaption    time.Duration
	lastCaptionPos platform.Point
	hasLastCaption bool

	parent *Window                // WindowOptions.Parent
	dialog *protocols.XdgDialogV1 // modal windows with xdg_wm_dialog_v1

	// Session restore: the window's xdg_toplevel_session_v1 and its name
	// in the backend's session.
	session     *protocols.XdgToplevelSessionV1
	sessionName string
}

// checkParent returns WindowOptions.Parent as one of b's open windows, or
// nil, and panics with an actionable message otherwise (like headless).
func (b *Backend) checkParent(parent platform.Window) *Window {
	if parent == nil {
		return nil
	}
	p, ok := parent.(*Window)
	if !ok || p.b != b {
		panic(fmt.Sprintf("wayland: WindowOptions.Parent is a %T of another backend; the parent must be an open window of the same backend", parent))
	}
	if p.Closed() {
		panic("wayland: WindowOptions.Parent is closed; the parent must be an open window")
	}
	return p
}

// sendToplevelState sends the toplevel's attributes before an initial
// commit (createRole): at creation, and to the new toplevel of a hidden
// window shown again. Requests made while the window was hidden only
// recorded their values, which are sent here.
func (w *Window) sendToplevelState() {
	if w.title != "" {
		w.toplevel.SetTitle(w.title)
	}
	w.toplevel.SetAppId(w.appID())
	w.applySizeHints()
	w.sendParent()
	if w.ctl.wantMax {
		w.toplevel.SetMaximized()
	}
	if w.ctl.wantFull {
		w.toplevel.SetFullscreen(nil) // the compositor picks the output
	}
	if w.decoration != nil {
		mode := protocols.ZxdgToplevelDecorationV1ModeServerSide
		if w.opts.Frameless {
			mode = protocols.ZxdgToplevelDecorationV1ModeClientSide
		}
		w.decoration.SetMode(mode)
	}
	if w.ctl.dialog != nil {
		w.ctl.dialog.SetModal()
	}
	w.sendIcon(false)
}

// sendParent sends set_parent for a child window whose parent is mapped.
// xdg-shell treats an unmapped parent as none and drops the relation when
// the parent unmaps (a hidden parent's toplevel is even destroyed), so it
// is sent again whenever the parent maps (onMapped, with the parent's
// current toplevel) and whenever the child is shown again.
func (w *Window) sendParent() {
	if p := w.ctl.parent; p != nil && p.mapped && !p.Closed() {
		w.toplevel.SetParent(p.toplevel)
	}
}

// onMapped runs when the window's first buffer is committed (at creation
// or after being hidden): its children with a toplevel get their parent
// now (hidden ones get it when shown), and an activation that waited for
// the map is sent.
func (w *Window) onMapped() {
	for _, c := range w.b.loop.Windows() {
		if c.ctl.parent == w && c.toplevel != nil {
			c.toplevel.SetParent(w.toplevel)
		}
	}
	if tok := w.ctl.activationToken; tok != "" {
		w.ctl.activationToken = ""
		if w.b.activation != nil {
			w.b.activation.Activate(tok, w.surface)
		}
	}
}

// closeChildren closes the window's children, in creation order (their
// own children first).
func (w *Window) closeChildren() {
	for _, c := range slices.Clone(w.b.loop.Windows()) {
		if c.ctl.parent == w {
			c.Close()
		}
	}
}

// releaseCtl destroys the window's dialog and session objects; Close calls
// it before destroying the toplevel. The session keeps the window's state
// (xdg_toplevel_session_v1.destroy does not remove it), and its name stays
// reserved for this process (see session).
func (w *Window) releaseCtl() {
	if d := w.ctl.dialog; d != nil {
		d.Destroy()
		w.ctl.dialog = nil
	}
	if s := w.ctl.session; s != nil {
		if !s.Destroyed() {
			s.Destroy()
		}
		w.ctl.session = nil
	}
}

// releaseCtl destroys the session and the window control globals, after
// the windows (Backend.release).
func (b *Backend) releaseCtl() {
	if s := b.session.obj; s != nil && !s.Destroyed() {
		s.Destroy() // destroy, not remove: the next run restores from it
	}
	if b.activation != nil {
		b.activation.Destroy()
	}
	if b.dialogManager != nil {
		b.dialogManager.Destroy()
	}
	if b.iconManager != nil {
		b.iconManager.Destroy()
	}
	if b.sessionManager != nil {
		b.sessionManager.Destroy()
	}
}

// osState is the window's state as the compositor configured it last.
// Minimized cannot be known on Wayland, nor the position.
func (w *Window) osState() platform.WindowState {
	return platform.WindowState{
		Maximized:             w.states.maximized,
		Fullscreen:            w.states.fullscreen,
		ClientSideDecorations: w.csd(),
	}
}

// csd reports whether the window has client-side decorations: always when
// frameless, and when the compositor has no xdg-decoration or configured
// the client-side mode (then the app draws its title bar too).
func (w *Window) csd() bool {
	// decorated, not w.decoration: a hidden window has no decoration
	// object but keeps its mode.
	return w.opts.Frameless || !w.ctl.decorated || w.ctl.decoMode == protocols.ZxdgToplevelDecorationV1ModeClientSide
}

// showable reports whether a buffer may be committed to the surface now:
// not hidden, and not waiting for the configure of an initial commit.
func (w *Window) showable() bool {
	return !w.ctl.hidden && !w.ctl.unmapped && !w.ctl.awaitConfigure
}

// unmap hides a mapped window the way GTK 4 and Qt do: a null buffer,
// committed, unmaps the surface (xdg-shell), then the role objects are
// destroyed (destroyRole). SetVisible(true) gives the surface new ones and
// makes a new initial commit.
//
// Keeping the toplevel and making the new initial commit on it, which
// xdg-shell allows ("the client must perform the initial commit again"),
// does not work everywhere: Hyprland (0.56) never answers that commit with
// a configure, so the window would never map again. A new xdg_surface is a
// new window to every compositor. The null buffer goes first, while the
// toplevel exists: it is the plain unmap every compositor handles, and it
// leaves no buffer committed, which get_xdg_surface and
// get_toplevel_decoration require of the surface at show.
//
// The swapchain is kept, so its newest buffer can map the window again
// without the app (mapRetained).
func (w *Window) unmap() {
	w.surface.Attach(nil, 0, 0)
	w.surface.Commit()
	w.destroyRole()
	w.mapped = false
	w.ctl.unmapped = true
	w.ackPending = false // its serial belonged to the destroyed xdg_surface
	// The frame callback of an unmapped surface may never fire; nothing
	// may wait for it after the window is shown again.
	if w.VsyncPending() {
		w.VsyncDone()
	}
	// No configure tells an unmapped toplevel that it is no longer
	// activated; the keyboard's leave comes on its own.
	w.activated = false
	w.updateFocus()
}

// mapRetained maps the window with its last presented frame, when it is
// showable, configured with the frame's size and not mapped: the window
// shown again. Otherwise the app's next Present maps it (a new size gets a
// ConfigureEvent, which forces a frame).
func (w *Window) mapRetained() {
	buf := w.chain.prev
	if w.mapped || !w.showable() || !w.ackPending || buf == nil || buf.wl == nil {
		return
	}
	if w.ackSize != w.surfaceSize || logicalPoint(w.announced.Size) != w.surfaceSize || w.chain.size != w.announced.Physical() {
		return
	}
	w.xdgSurface.AckConfigure(w.ackSerial)
	w.ackPending = false
	// Attaching a buffer the compositor may still hold is allowed; its
	// pixels are the newest frame and are not written while busy.
	w.surface.Attach(buf.wl, 0, 0)
	w.damage([]image.Rectangle{{Max: w.chain.size}}, w.chain.size)
	w.requestCallback()
	w.surface.Commit()
	buf.busy = true
	w.mapped = true
	w.onMapped()
}

// --- platform.Window: window control ---
//
// State comes from loop.WindowState: the state of the latest delivered
// WindowStateEvent.

// Minimize asks the compositor to minimize the window
// (xdg_toplevel.set_minimized). Wayland never says whether it did, nor
// can a client undo it: State().Minimized stays false, and Activate
// brings the window back.
func (w *Window) Minimize() {
	if w.Closed() || w.ctl.unmapped {
		return
	}
	w.toplevel.SetMinimized()
}

// SetMaximized asks for maximized (set_maximized) or not (unset_maximized);
// the configure states report the outcome. A request equal to the last one
// asked, or to the compositor's latest configure (whichever came last), is
// free. A hidden window asks when it is shown again.
func (w *Window) SetMaximized(maximized bool) {
	if w.Closed() || maximized == w.ctl.wantMax {
		return
	}
	w.ctl.wantMax = maximized
	if w.ctl.unmapped {
		return // sendToplevelState asks at the next show
	}
	if maximized {
		w.toplevel.SetMaximized()
	} else {
		w.toplevel.UnsetMaximized()
	}
}

// SetFullscreen asks for fullscreen on the output the compositor picks
// (set_fullscreen with no output) or leaves it; like SetMaximized.
func (w *Window) SetFullscreen(fullscreen bool) {
	if w.Closed() || fullscreen == w.ctl.wantFull {
		return
	}
	w.ctl.wantFull = fullscreen
	if w.ctl.unmapped {
		return
	}
	if fullscreen {
		w.toplevel.SetFullscreen(nil)
	} else {
		w.toplevel.UnsetFullscreen()
	}
}

// SetVisible hides or shows the window.
//
// Hiding a mapped window commits a null buffer, which unmaps it, and
// destroys its xdg_toplevel and xdg_surface (see unmap); frames stop
// (internal/loop SetHidden) and the focus goes as the compositor reports.
// Showing it again creates new ones on the same wl_surface with the
// toplevel's attributes (title, app id, size limits, parent, maximized and
// fullscreen requests, decoration mode, modality, icon), adds it to the
// session again, and makes the initial commit; at its configure, a window
// of the same size is mapped with its last presented frame, and frames
// resume. A window that was never mapped (WindowOptions.Hidden, or hidden
// before its first Present or before the configure of a show) keeps its
// role objects and only resumes frames: its next Present maps it.
func (w *Window) SetVisible(visible bool) {
	if w.Closed() || visible == !w.ctl.hidden {
		return
	}
	w.ctl.hidden = !visible
	if !visible {
		w.SetHidden(true)
		w.ctl.activationToken = "" // an activation waiting for the map is void
		if w.mapped {
			w.unmap()
		}
		return
	}
	switch {
	case w.ctl.unmapped:
		w.ctl.unmapped = false
		w.createRole()
		w.rejoinSession()
		w.surface.Commit() // the initial commit of the new role objects
		w.ctl.awaitConfigure = true
		// Frames resume at its configure (onConfigure).
	case w.ctl.awaitConfigure:
		// Hidden and shown again before the configure: it resumes then.
	default:
		w.SetHidden(false)
		w.mapRetained()
	}
}

// SetAlwaysOnTop does nothing: Wayland has no protocol for it (the
// compositor decides the stacking).
func (w *Window) SetAlwaysOnTop(bool) {}

// SetTitleBarColors does nothing: only Windows 11 draws colored title bars.
func (w *Window) SetTitleBarColors(platform.TitleBarColors) {}

// SetBackdrop does nothing: system backdrops are a Windows 11 material.
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
}

// SetMinSize changes the minimum size (xdg_toplevel.set_min_size, applied
// at once if the window is shown); a floating window below it is resized.
func (w *Window) SetMinSize(size platform.Size) {
	platform.CheckSize("SetMinSize size", size)
	if w.Closed() {
		return
	}
	w.setLimits(ceilPoint(size), w.sizes.max)
}

// SetMaxSize changes the maximum size (xdg_toplevel.set_max_size); a
// floating window above it is resized. Where it is below the minimum, the
// minimum wins (and is what the compositor receives as the maximum).
func (w *Window) SetMaxSize(size platform.Size) {
	platform.CheckSize("SetMaxSize size", size)
	if w.Closed() {
		return
	}
	w.setLimits(w.sizes.min, floorPoint(size))
}

// setLimits applies new size limits. A floating window outside them picks
// the clamped size (Wayland lets floating windows choose; the compositor
// imposes maximized, fullscreen and tiled sizes, which are kept).
func (w *Window) setLimits(min, max image.Point) {
	if min == w.sizes.min && max == w.sizes.max {
		return
	}
	w.sizes.min, w.sizes.max = min, max
	floating := !w.states.constrained()
	if floating {
		w.sizes.stateless = w.sizes.limit(w.sizes.stateless)
	}
	if !w.ctl.unmapped { // else sent when shown again
		w.applySizeHints()
		if w.mapped {
			w.surface.Commit() // the hints are double-buffered
		}
	}
	if floating {
		if p := w.sizes.limit(w.sizes.current); p != w.sizes.current {
			w.adoptSize(p)
		}
	}
}

// SetPosition does nothing: Wayland clients cannot place their windows
// (see RestoreToken for what the compositor can restore).
func (w *Window) SetPosition(image.Point) {}

// Activate asks the compositor to activate the window with
// xdg_activation_v1: a token for the latest input serial of any seat and
// the surface of our focused window (else the window itself) as the
// requesting surface, then activate with it. Compositors with focus
// stealing prevention grant it only for a recent serial of the focused
// client, and otherwise usually mark the window as demanding attention.
// Without a serial (no input yet) the token is still asked, without one,
// as winit does. A granted activation arrives as a FocusEvent. Without the
// protocol, or for a hidden window, nothing happens. A window shown but not
// mapped yet is activated when it maps (the activation of an unmapped
// surface is ignored).
func (w *Window) Activate() {
	b := w.b
	if w.Closed() || b.activation == nil || w.ctl.hidden {
		return
	}
	tok := b.activation.GetActivationToken()
	if s := b.lastSeat; s != nil && b.lastSerial != 0 {
		tok.SetSerial(b.lastSerial, s.wl)
	}
	req := w
	for _, o := range b.loop.Windows() {
		if o.focused {
			req = o
			break
		}
	}
	tok.SetSurface(req.surface)
	tok.SetAppId(w.appID())
	tok.SetDoneHandler(func(e protocols.XdgActivationTokenV1DoneEvent) {
		tok.Destroy()
		if w.Closed() || w.ctl.hidden || b.activation == nil {
			return
		}
		if !w.mapped {
			// Shown, not mapped yet (SetVisible(true) then Activate):
			// activated when it maps (onMapped).
			w.ctl.activationToken = e.Token
			return
		}
		b.activation.Activate(e.Token, w.surface)
	})
	tok.Commit()
}

// SetIcon sets the window's icon with xdg_toplevel_icon_v1: every image
// becomes a square wl_shm buffer (non-square images are centered) at scale
// 1, as the protocol asks clients to give every size they have. nil or
// empty resets the icon to the default (the compositor uses the .desktop
// file of the app id). Without the protocol the icon is only recorded.
func (w *Window) SetIcon(images []image.Image) {
	platform.CheckIcon("SetIcon", images)
	if w.Closed() {
		return
	}
	w.ctl.icon = platform.CopyIcon(images)
	if w.b.iconManager == nil || w.ctl.unmapped {
		return // sent when shown again
	}
	w.sendIcon(true)
	if w.mapped {
		w.surface.Commit() // set_icon is double-buffered
	}
}

// sendIcon sends the recorded icon; with no icon it sends a reset only if
// reset is true (a toplevel starts with the default icon).
func (w *Window) sendIcon(reset bool) {
	m := w.b.iconManager
	if m == nil {
		return
	}
	if len(w.ctl.icon) == 0 {
		if reset {
			m.SetIcon(w.toplevel, nil)
		}
		return
	}
	icon := m.CreateIcon()
	var bufs []*shmBuffer
	for _, img := range w.ctl.icon {
		buf, err := iconBuffer(w.b.shm, img)
		if err != nil {
			continue // no memory for this size: the others still work
		}
		icon.AddBuffer(buf.wl, 1)
		bufs = append(bufs, buf)
	}
	m.SetIcon(w.toplevel, icon)
	// The protocol lets the icon go after set_icon (the toplevel keeps
	// it), and its buffers after the icon.
	icon.Destroy()
	for _, buf := range bufs {
		buf.destroy()
	}
}

// iconBuffer returns a square ARGB8888 (premultiplied, the wl_shm
// convention) wl_shm buffer with img centered in it.
func iconBuffer(shm *protocols.WlShm, img image.Image) (*shmBuffer, error) {
	b := img.Bounds()
	side := max(b.Dx(), b.Dy())
	sq := image.NewRGBA(image.Rect(0, 0, side, side))
	off := image.Pt((side-b.Dx())/2, (side-b.Dy())/2)
	draw.Draw(sq, image.Rectangle{Min: off, Max: off.Add(b.Size())}, img, b.Min, draw.Src)
	buf, err := allocShm(shm, protocols.WlShmFormatArgb8888, sq.Rect.Max)
	if err != nil {
		return nil, err
	}
	platform.CopyRGBAToBGRA(buf.data, buf.stride, sq, sq.Rect)
	return buf, nil
}

// SetHitRegions records the custom title bar; the pointer handlers read
// it (osButton).
func (w *Window) SetHitRegions(regions []platform.HitRegion) {
	platform.CheckHitRegions(regions)
	if w.Closed() || platform.EqualHitRegions(w.ctl.regions, regions) {
		return
	}
	w.ctl.regions = slices.Clone(regions)
}

// RestoreToken returns the window's session restore token (see session),
// or "" without xdg_session_manager_v1, before the compositor answered
// get_session, or once another client took the session over. It stays
// readable after Close, when the layer above saves it.
func (w *Window) RestoreToken() string {
	s := &w.b.session
	if s.id == "" || w.ctl.sessionName == "" || s.gone {
		return ""
	}
	return restoreTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(s.id)) + ":" + w.ctl.sessionName
}

// --- pointer: resize edges and caption regions ---

// resizeEdgeAt returns the resize edge under p (surface-local logical
// pixels), or 0: windows with client-side decorations that are mapped,
// neither maximized, fullscreen nor FixedSize have resize edges, except
// along the edges they are tiled against (GTK's rule).
func (w *Window) resizeEdgeAt(p platform.Point) protocols.XdgToplevelResizeEdge {
	if !w.mapped || !w.csd() || w.states.maximized || w.states.fullscreen || w.opts.FixedSize {
		return 0
	}
	return resizeEdge(p, w.surfaceSize, w.states.tiled)
}

// resizeEdge returns the edge of a window of the given logical size whose
// resize band contains p: within resizeBand of a side, and a corner within
// resizeCorner of the neighbouring side. Tiled edges are left out.
func resizeEdge(p platform.Point, size image.Point, tiled protocols.XdgToplevelResizeEdge) protocols.XdgToplevelResizeEdge {
	w, h := float32(size.X), float32(size.Y)
	if p.X < 0 || p.Y < 0 || p.X >= w || p.Y >= h {
		return 0
	}
	top, bottom := p.Y < resizeBand, p.Y >= h-resizeBand
	left, right := p.X < resizeBand, p.X >= w-resizeBand
	if !top && !bottom && !left && !right {
		return 0
	}
	if top || bottom {
		left = left || p.X < resizeCorner
		right = right || p.X >= w-resizeCorner
	}
	if left || right {
		top = top || p.Y < resizeCorner
		bottom = bottom || p.Y >= h-resizeCorner
	}
	// A window too small for both bands: the nearer side wins.
	if top && bottom {
		top, bottom = p.Y < h/2, p.Y >= h/2
	}
	if left && right {
		left, right = p.X < w/2, p.X >= w/2
	}
	var e protocols.XdgToplevelResizeEdge
	if top {
		e |= protocols.XdgToplevelResizeEdgeTop
	}
	if bottom {
		e |= protocols.XdgToplevelResizeEdgeBottom
	}
	if left {
		e |= protocols.XdgToplevelResizeEdgeLeft
	}
	if right {
		e |= protocols.XdgToplevelResizeEdgeRight
	}
	return e &^ tiled
}

// edgeCursor is the cursor shown over a resize edge.
func edgeCursor(e protocols.XdgToplevelResizeEdge) platform.Cursor {
	switch e {
	case protocols.XdgToplevelResizeEdgeTop:
		return platform.CursorNResize
	case protocols.XdgToplevelResizeEdgeBottom:
		return platform.CursorSResize
	case protocols.XdgToplevelResizeEdgeLeft:
		return platform.CursorWResize
	case protocols.XdgToplevelResizeEdgeRight:
		return platform.CursorEResize
	case protocols.XdgToplevelResizeEdgeTopLeft:
		return platform.CursorNWResize
	case protocols.XdgToplevelResizeEdgeTopRight:
		return platform.CursorNEResize
	case protocols.XdgToplevelResizeEdgeBottomLeft:
		return platform.CursorSWResize
	case protocols.XdgToplevelResizeEdgeBottomRight:
		return platform.CursorSEResize
	}
	return platform.CursorDefault
}

// cursorFor is the cursor the seat shows over w: the resize cursor on a
// resize edge, else the window's.
func (s *seat) cursorFor(w *Window) platform.Cursor {
	if s.ptr.edge != 0 {
		return edgeCursor(s.ptr.edge)
	}
	return w.cursor
}

// pointerMoved follows the pointer after a motion: a pending caption press
// that left the drag threshold starts the interactive move (with the
// press's serial, which the compositor checks against the button still
// held); otherwise the resize cursor follows the edges.
func (s *seat) pointerMoved() {
	p := &s.ptr
	w := p.focus
	if w == nil {
		return
	}
	if p.osHeld != 0 {
		if p.captionPending {
			dx, dy := p.pos.X-p.captionPos.X, p.pos.Y-p.captionPos.Y
			if dx*dx+dy*dy >= captionDragThreshold*captionDragThreshold {
				p.captionPending = false
				w.ctl.hasLastCaption = false // a drag is not half of a double click

				// No toplevel: hidden meanwhile, nothing to move.
				if w.toplevel != nil {
					w.toplevel.Move(s.wl, p.captionSerial)
				}
			}
		}
		return
	}
	if e := w.resizeEdgeAt(p.pos); e != p.edge {
		p.edge = e
		s.applyCursor(s.cursorFor(w))
	}
}

// osButton handles a button event that belongs to the compositor and
// reports whether it did (then it is not delivered to the app):
//
//   - Once a press went to the compositor, every button event up to the
//     release of the last button held is its own (motion too, see
//     pointerState.motion); a pointer leave ends the sequence silently.
//   - A press that starts a sequence (no app button held) on a resize
//     edge starts xdg_toplevel.resize for the primary button.
//   - A press in a HitCaption region: primary, a double click toggles
//     maximize, otherwise an interactive move starts once the pointer
//     moves past the drag threshold (pointerMoved); secondary opens the
//     window menu (show_window_menu at the pointer); others do nothing.
//
// Presses in other regions, including the window button regions
// (HitMinimize, HitMaximize, HitClose), are the app's.
func (s *seat) osButton(btn platform.PointerButton, pressed bool, serial uint32, t time.Duration) bool {
	p := &s.ptr
	w := p.focus
	if w == nil || btn == platform.ButtonNone {
		return false
	}
	if p.osHeld != 0 {
		if pressed {
			p.osHeld |= btn.Mask()
		} else {
			p.osHeld &^= btn.Mask()
		}
		if !p.osHeld.Has(platform.ButtonPrimary) {
			p.captionPending = false // released before moving: a click
		}
		return true
	}
	if !pressed || p.buttons != 0 || !w.mapped {
		// Not mapped: a press still in flight when the window was hidden
		// (its toplevel is gone) is the app's, like any.
		return false
	}
	if e := w.resizeEdgeAt(p.pos); e != 0 {
		p.osHeld = btn.Mask()
		if btn == platform.ButtonPrimary {
			w.toplevel.Resize(s.wl, serial, e)
		}
		return true
	}
	if platform.HitTest(w.ctl.regions, p.pos) != platform.HitCaption {
		return false
	}
	p.osHeld = btn.Mask()
	switch btn {
	case platform.ButtonPrimary:
		if w.captionDoubleClick(t, p.pos) {
			w.SetMaximized(!w.states.maximized)
			break
		}
		p.captionPending, p.captionSerial, p.captionPos = true, serial, p.pos
	case platform.ButtonSecondary:
		w.toplevel.ShowWindowMenu(s.wl, serial, int32(p.pos.X), int32(p.pos.Y))
	}
	return true
}

// captionDoubleClick records a primary caption press at t and pos and
// reports whether it completes a double click (the interval of the system
// settings, defaultDoubleClick without one).
func (w *Window) captionDoubleClick(t time.Duration, pos platform.Point) bool {
	interval := w.b.Settings().DoubleClickInterval
	if interval <= 0 {
		interval = defaultDoubleClick
	}
	c := &w.ctl
	dx, dy := pos.X-c.lastCaptionPos.X, pos.Y-c.lastCaptionPos.Y
	if c.hasLastCaption && t-c.lastCaption <= interval && dx*dx+dy*dy <= doubleClickSlop*doubleClickSlop {
		c.hasLastCaption = false
		return true
	}
	c.hasLastCaption, c.lastCaption, c.lastCaptionPos = true, t, pos
	return false
}

// --- session restore (xdg-session-management-v1) ---

// restoreTokenPrefix starts the restore tokens of this backend:
// "wayland-session-v1:" + the session id in unpadded base64url + ":" +
// the toplevel's name (32 lowercase hex digits). Tokens in any other form
// are ignored.
const restoreTokenPrefix = "wayland-session-v1:"

// maxSessionID bounds a session id taken from a token (Wayland strings are
// limited by the 4 KiB message size).
const maxSessionID = 1024

// session is the backend's xdg_session_v1: one per process, opened at the
// first NewWindow (with the session id of that window's token, or a new
// session), as the platform contract says.
type session struct {
	obj *protocols.XdgSessionV1
	// asked is the id get_session asked for ("" for a new session); id is
	// the session's id once the compositor answered: created's, or asked
	// after restored.
	asked, id string
	// used holds every toplevel name given out by this process. A name
	// is never given to two toplevels of one session (name_in_use is a
	// protocol error), not even after the first one closed: whether the
	// compositor still counts a closed toplevel's name as granted is not
	// specified, so a window reopened in the same run gets a new name.
	used map[string]bool
	// gone: another client took the session over (replaced); no tokens
	// from then on.
	gone bool
}

// joinSession adds the window to the backend's session, before its initial
// commit: restore_toplevel with the name of a token of this session (the
// compositor then restores the window's state, and its first configure
// carries the restored size), add_toplevel with a fresh name otherwise.
func (w *Window) joinSession(token string) {
	b := w.b
	s := &b.session
	if b.sessionManager == nil || s.gone {
		return
	}
	id, name, ok := parseRestoreToken(token)
	if s.obj == nil {
		var asked *string
		if ok {
			s.asked = id
			asked = &s.asked
		}
		s.used = map[string]bool{}
		s.obj = b.sessionManager.GetSession(protocols.XdgSessionManagerV1ReasonLaunch, asked)
		s.obj.SetCreatedHandler(func(e protocols.XdgSessionV1CreatedEvent) { s.id = e.SessionId })
		s.obj.SetRestoredHandler(func(protocols.XdgSessionV1RestoredEvent) { s.id = s.asked })
		s.obj.SetReplacedHandler(func(protocols.XdgSessionV1ReplacedEvent) { b.loseSession() })
	}
	ours := ok && (id == s.asked || id == s.id) && !s.used[name]
	if ours {
		w.ctl.session = s.obj.RestoreToplevel(w.toplevel, name)
	} else {
		name = newSessionName()
		w.ctl.session = s.obj.AddToplevel(w.toplevel, name)
	}
	s.used[name] = true
	w.ctl.sessionName = name
}

// rejoinSession adds the new toplevel of a window shown again to the
// session, before its initial commit. Its old toplevel's session object was
// destroyed with it (the session keeps that state). The new toplevel gets a
// fresh name with add_toplevel, as a window reopened in the same run does
// (see session.used: restore_toplevel with the old name could be a
// name_in_use error), so the restore token changes; the old name is
// removed (remove_toplevel), or every hide would leave an entry in the
// compositor's persistent storage. While hidden, the window's token still
// names the old entry, which the compositor keeps: an app that quits from
// a tray with the window hidden restores it next time.
func (w *Window) rejoinSession() {
	b := w.b
	s := &b.session
	old := w.ctl.sessionName
	if b.sessionManager == nil || s.gone || s.obj == nil || old == "" {
		return
	}
	w.joinSession("")
	s.obj.RemoveToplevel(old)
}

// loseSession handles xdg_session_v1.replaced: the session and its
// toplevel objects are inert and are destroyed; windows have no restore
// token from then on.
func (b *Backend) loseSession() {
	s := &b.session
	s.gone, s.id = true, ""
	for _, w := range b.loop.Windows() {
		if ts := w.ctl.session; ts != nil {
			ts.Destroy()
			w.ctl.session = nil
		}
	}
	if s.obj != nil {
		s.obj.Destroy()
	}
}

// parseRestoreToken returns the session id and toplevel name of a token
// of this backend; ok is false for anything else (another backend's token,
// garbage), which the window then ignores.
func parseRestoreToken(token string) (id, name string, ok bool) {
	rest, ok := strings.CutPrefix(token, restoreTokenPrefix)
	if !ok {
		return "", "", false
	}
	enc, name, ok := strings.Cut(rest, ":")
	if !ok || !validSessionName(name) {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	// A string the compositor would reject (invalid_session_id) must
	// never be sent: empty, too long, not UTF-8, or with a NUL.
	if err != nil || len(raw) == 0 || len(raw) > maxSessionID || !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return "", "", false
	}
	return string(raw), name, true
}

// validSessionName reports whether name has the form newSessionName
// produces.
func validSessionName(name string) bool {
	if len(name) != 32 {
		return false
	}
	for i := range len(name) {
		if c := name[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// newSessionName returns a random toplevel name: add_toplevel needs a
// name never used in the session, which persists across runs.
func newSessionName() string {
	var raw [16]byte
	rand.Read(raw[:]) // never fails (crypto/rand)
	return hex.EncodeToString(raw[:])
}
