//go:build linux

package x11

import (
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Drag and drop source side (Window.StartDrag), XDND version 5:
//
//   - The backend owns XdndSelection with the source window and serves it
//     like the clipboard (selection.go); XdndTypeList and XdndActionList
//     on the source window list the targets and the allowed actions (the
//     default first), and XdndActionDescription names them when there
//     is a choice (the specification's requirement for XdndActionAsk).
//   - It grabs the pointer (a core grab replaces the implicit grab of the
//     held press, which belongs to this client) and the keyboard, so
//     Escape cancels. The window gets PointerCancel (and PointerLeave)
//     for the sequence; the queued rest of the sequence is dropped.
//   - At every motion it finds the window under the pointer by walking
//     down from the root with TranslateCoordinates, stopping at a window
//     of the app or at one with XdndAware (version 3 or later; XdndProxy
//     honored). XDND targets get XdndEnter (XdndTypeList when more than
//     three types), XdndPosition with the action DefaultDropEffect picks
//     from Allowed and the modifiers, or XdndActionAsk when the drag
//     allows a choice and the user asks for one as in GTK (a drag of the
//     secondary or middle button, or Alt alone held), so the target lets
//     the user choose at the drop and XdndFinished reports the choice
//     (the cursor shows context-menu while the target accepts the ask);
//     at most one position in flight (the next one waits for
//     XdndStatus), and XdndLeave. The app's own windows get the target-side
//     DragEvents directly, with FromApp set, as the headless backend
//     does.
//   - The release drops: XdndDrop when the target's latest status
//     accepted (after the status when one is in flight, or XdndLeave
//     after statusTimeout), then the effect of XdndFinished, or DropNone
//     after dragFinishTimeout; over the app's own window the DragDrop is
//     delivered and the effect it holds afterwards is the outcome. The
//     grabs end at the drop, and one DragDoneEvent goes to the source
//     window.
//   - The pointer shows the grabbing cursor, then copy, move, alias
//     (link) or no-drop as the target answers. DragOptions.Image follows
//     the pointer in a window of its own (dragicon.go).

const (
	// dragStatusTimeout bounds the wait for the XdndStatus of the
	// position in flight when the button is released.
	dragStatusTimeout = selectionTimeout
	// dragFinishTimeout bounds the wait for XdndFinished after XdndDrop:
	// the target converts the data first (INCR for large data).
	dragFinishTimeout = 10 * time.Second
	// maxWindowDepth bounds the walk down the window tree.
	maxWindowDepth = 16
)

// dragSource is the app's drag in progress.
type dragSource struct {
	w       *Window
	data    platform.Data
	types   []string
	allowed platform.DropEffect
	offered []xproto.Atom // XdndTypeList

	rootX, rootY int16
	mods         platform.Modifiers
	time         uint32 // server time of the latest pointer event

	// The XDND target under the pointer.
	target   xproto.Window // the XdndAware window, 0 when none
	dest     xproto.Window // where its messages go: the target or its proxy
	version  int
	waiting  bool                // a position is in flight
	moved    bool                // the pointer moved while it was
	accept   platform.DropEffect // the latest status: the accepted effect, or DropNone
	released bool                // the button was released
	dropped  bool                // XdndDrop was sent, or the own window's drop is handled
	stop     func()              // the status or finish timeout

	// The app's window under the pointer (direct delivery), and the
	// drag's position in it.
	own    *Window
	ownPos platform.Point

	// askButton: the drag started from the secondary or middle button,
	// which asks the target to let the user choose (XdndActionAsk);
	// asked: the latest status accepted the ask.
	askButton bool
	asked     bool

	// The drag image's window (dragicon.go), 0 when none; the hotspot
	// and the window's root position.
	icon    xproto.Window
	iconHot image.Point
	iconAt  image.Point

	aware      map[xproto.Window]awareInfo
	grabbedKbd bool
	ungrabbed  bool
	cursor     platform.Cursor
}

// actionNames are the XdndActionDescription strings of the actions.
var actionNames = map[platform.DropEffect]string{platform.DropCopy: "Copy", platform.DropMove: "Move", platform.DropLink: "Link"}

// requested returns the action XdndPosition requests: XdndActionAsk when
// the drag allows a choice and the user asks for one (a drag of the
// secondary or middle button, or Alt alone held: GTK's rules,
// gtk_drag_get_event_actions), else the default effect for the
// modifiers.
func (ds *dragSource) requested(b *Backend) xproto.Atom {
	choice := ds.allowed&(ds.allowed-1) != 0 // more than one effect
	if choice && (ds.askButton || ds.mods&(platform.ModAlt|platform.ModCtrl|platform.ModShift) == platform.ModAlt) {
		return b.atoms.xdndActionAsk
	}
	return b.actionOf(platform.DefaultDropEffect(ds.allowed, ds.mods))
}

// awareInfo is what a window says about XDND.
type awareInfo struct {
	version int           // XdndAware, 0 when absent or too old
	dest    xproto.Window // XdndProxy, or the window itself
}

// StartDrag starts an XDND drag of opts.Data from the window while a
// pointer button pressed in it is held (else platform.ErrNoGrab, also
// when the pointer grab fails).
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	b := w.b
	if w.Closed() || b.integ.drag != nil || w.pressed == 0 {
		return platform.ErrNoGrab
	}
	if w.pressDevice != 0 {
		// The press holds an XInput2 implicit grab, and the server
		// refuses a core grab while a grab of another type is active
		// (AlreadyGrabbed), even for the same client: release it first
		// (GTK 3 grabs with XIGrabDevice instead; the core grab keeps
		// one event path for both kinds of servers).
		b.srv.ungrabPointer(w.pressDevice)
	}
	if !b.xs.grabPointer(w.id, platform.CursorGrabbing) {
		return platform.ErrNoGrab
	}
	data := opts.Data.Clone()
	ds := &dragSource{w: w, data: data, types: data.Types(), allowed: opts.Allowed,
		aware: map[xproto.Window]awareInfo{}, cursor: platform.CursorGrabbing, time: b.userTime}
	if err := b.own(b.atoms.xdndSelection, w.id, data); err != nil {
		b.xs.ungrabPointerCore()
		return err
	}
	for _, o := range offers(data) {
		if a := b.atom(o.target); a != 0 {
			ds.offered = append(ds.offered, a)
		}
	}
	list := make([]uint32, len(ds.offered))
	for i, a := range ds.offered {
		list[i] = uint32(a)
	}
	b.xs.changeProperty(w.id, b.atoms.xdndTypeList, xproto.AtomAtom, 32, card32s(list...))
	// The allowed actions, the default first (the XDND specification:
	// the first is preselected when the target asks the user), with
	// their descriptions when there is a choice (XdndActionAsk).
	var actions []uint32
	var desc []byte
	def := platform.DefaultDropEffect(ds.allowed, 0)
	for _, e := range [...]platform.DropEffect{def, platform.DropCopy, platform.DropMove, platform.DropLink} {
		if a := uint32(b.actionOf(e)); ds.allowed.Has(e) && !slices.Contains(actions, a) {
			actions = append(actions, a)
			desc = append(append(desc, actionNames[e]...), 0)
		}
	}
	b.xs.changeProperty(w.id, b.atoms.xdndActionList, xproto.AtomAtom, 32, card32s(actions...))
	if len(actions) > 1 {
		b.xs.changeProperty(w.id, b.atoms.xdndActionDescription, xproto.AtomString, 8, desc[:len(desc)-1])
	} else {
		b.xs.deleteProperty(w.id, b.atoms.xdndActionDescription) // an earlier drag's
	}
	// A drag of the secondary or middle button asks (GTK's rule, and
	// Windows' right-drag menu).
	ds.askButton = w.pressed&platform.ButtonPrimary.Mask() == 0
	ds.grabbedKbd = b.xs.grabKeyboard(w.id)

	qp, _ := b.xs.queryPointer(b.root)
	mods := b.kbd.mods(uint32(qp.mask))
	// The OS takes the pointer: the rest of the sequence already queued
	// is the drag's, and the app gets a cancel (and the leave: the
	// pointer is the drag's now).
	b.loop.Extract(w, func(e platform.Event) bool { _, ok := e.(platform.PointerEvent); return ok })
	b.cancelPress(w, mods)
	if w.inside {
		b.leave(w, 0, mods)
	}
	w.pressed = 0

	b.integ.drag = ds
	b.showDragIcon(ds, opts.Image, opts.Hotspot, qp.rootX, qp.rootY)
	b.dragMove(qp.rootX, qp.rootY, mods, 0)
	if buttonsFromState(qp.mask) == 0 {
		// Released before the grab: drop where the pointer is.
		b.dragRelease()
	}
	b.signal()
	return nil
}

// dragInput routes the pointer and key events of the drag's grab; it
// reports whether ev was consumed.
func (b *Backend) dragInput(ev any) bool {
	ds := b.integ.drag
	if ds == nil || ds.ungrabbed {
		return false
	}
	switch ev := ev.(type) {
	case xproto.MotionNotifyEvent:
		if ev.Event == ds.w.id {
			b.dragMove(ev.RootX, ev.RootY, b.kbd.mods(uint32(ev.State)), uint32(ev.Time))
			return true
		}
	case xproto.ButtonPressEvent:
		return ev.Event == ds.w.id
	case xproto.EnterNotifyEvent, xproto.LeaveNotifyEvent:
		return true // the grab owns the pointer; the ungrab's crossings restore it
	case xproto.ButtonReleaseEvent:
		if ev.Event == ds.w.id {
			if _, steps, _ := mapButton(uint32(ev.Detail)); steps == (platform.Point{}) {
				ds.time = uint32(ev.Time)
				b.dragRelease()
			}
			return true
		}
	case xproto.KeyPressEvent:
		if b.isEscape(byte(ev.Detail)) {
			b.cancelDrag()
			return true
		}
		b.dragMods()
	case xproto.KeyReleaseEvent:
		b.dragMods()
	}
	return false
}

// dragXI routes an XInput2 pointer event of the source window during the
// drag (sent before the core grab replaced the implicit XI2 grab); it
// reports whether it was consumed.
func (b *Backend) dragXI(w *Window, ev xiDeviceEvent) bool {
	ds := b.integ.drag
	if ds == nil || ds.ungrabbed || w != ds.w {
		return false
	}
	switch ev.evtype {
	case xiMotion:
		b.dragMove(int16(ev.rootX), int16(ev.rootY), b.kbd.mods(ev.mods), ev.time)
	case xiButtonRelease:
		if _, steps, _ := mapButton(ev.detail); steps == (platform.Point{}) {
			ds.time = ev.time
			b.dragRelease()
		}
	}
	return true
}

// dragGrabbed reports whether the app's drag holds the pointer.
func (b *Backend) dragGrabbed() bool {
	ds := b.integ.drag
	return ds != nil && !ds.ungrabbed
}

// isEscape reports whether keycode kc types Escape without modifiers.
func (b *Backend) isEscape(kc byte) bool {
	if km := b.kbd.km(); km != nil {
		return xkb.NewState(km).Lookup(uint32(kc)).Key == platform.KeyEscape
	}
	return kc == 9 // evdev KEY_ESC + 8
}

// dragMods re-reads the modifiers after a key event (the pointer's mask
// already has the change) and reports a change to the target.
func (b *Backend) dragMods() {
	ds := b.integ.drag
	qp, ok := b.xs.queryPointer(b.root)
	if !ok {
		return
	}
	if mods := b.kbd.mods(uint32(qp.mask)); mods != ds.mods {
		b.dragMove(ds.rootX, ds.rootY, mods, 0)
	}
}

// dragMove moves the drag to (x, y) in root coordinates.
func (b *Backend) dragMove(x, y int16, mods platform.Modifiers, t uint32) {
	ds := b.integ.drag
	if ds == nil || ds.released {
		return
	}
	ds.rootX, ds.rootY = x, y
	if t != 0 {
		ds.time = t
	}
	b.moveDragIcon(ds, x, y)
	own, target, info := b.dragFind(x, y)
	if own != ds.own || target != ds.target {
		b.dragLeaveCurrent()
		ds.mods = mods
		switch {
		case own != nil:
			ds.own, ds.ownPos = own, b.ownPos(own, x, y)
			own.dnd.appOver, own.dnd.answer = true, dropAnswer{}
			if own.inside && own != ds.w {
				b.leave(own, 0, mods)
			}
			b.loop.QueueDrag(own, b.ownEvent(platform.DragEnter))
		case target != 0:
			ds.target, ds.dest, ds.version = target, info.dest, min(info.version, xdndVersion)
			var types [3]uint32
			for i := range min(3, len(ds.offered)) {
				types[i] = uint32(ds.offered[i])
			}
			flags := uint32(ds.version) << 24
			if len(ds.offered) > 3 {
				flags |= 1
			}
			b.sendXdndTo(ds.dest, ds.target, b.atoms.xdndEnter, uint32(ds.w.id), flags, types[0], types[1], types[2])
			b.sendPosition()
		}
		b.updateDragCursor()
		return
	}
	changed := mods != ds.mods
	ds.mods = mods
	switch {
	case own != nil:
		pos := b.ownPos(own, x, y)
		if pos != ds.ownPos || changed {
			ds.ownPos = pos
			b.loop.QueueDrag(own, b.ownEvent(platform.DragOver))
		}
	case target != 0:
		b.sendPosition()
	}
	b.updateDragCursor()
}

// dragFind returns the window under (x, y): one of the app's (own), or an
// XDND-aware one (target, with its version and message destination), or
// neither.
func (b *Backend) dragFind(x, y int16) (own *Window, target xproto.Window, info awareInfo) {
	win := b.root
	for range maxWindowDepth {
		_, _, child, ok := b.xs.translate(b.root, win, x, y)
		if !ok || child == 0 {
			return nil, 0, awareInfo{}
		}
		if w := b.windows[child]; w != nil {
			return w, 0, awareInfo{}
		}
		if info := b.awareOf(child); info.version > 0 {
			return nil, child, info
		}
		win = child
	}
	return nil, 0, awareInfo{}
}

// awareOf reads (once per drag) whether win is an XDND target: XdndAware
// with version 3 or later, and a valid XdndProxy (one whose own XdndProxy
// names itself).
func (b *Backend) awareOf(win xproto.Window) awareInfo {
	ds := b.integ.drag
	if info, ok := ds.aware[win]; ok {
		return info
	}
	info := awareInfo{dest: win}
	if p, ok := b.xs.property(win, b.atoms.xdndProxy, false); ok && p.format == 32 && len(p.data) >= 4 {
		proxy := xproto.Window(le.Uint32(p.data))
		if q, ok := b.xs.property(proxy, b.atoms.xdndProxy, false); ok && q.format == 32 && len(q.data) >= 4 &&
			xproto.Window(le.Uint32(q.data)) == proxy {
			info.dest = proxy
		}
	}
	aware := win
	if info.dest != win {
		aware = info.dest // XdndAware is on the proxy
	}
	if p, ok := b.xs.property(aware, b.atoms.xdndAware, false); ok && p.format == 32 && len(p.data) >= 4 {
		if v := int(le.Uint32(p.data)); v >= xdndMinVersion {
			info.version = v
		}
	}
	ds.aware[win] = info
	return info
}

// ownPos returns the drag's position in the app's window w.
func (b *Backend) ownPos(w *Window, x, y int16) platform.Point {
	dx, dy, _, ok := b.xs.translate(b.root, w.id, x, y)
	if !ok {
		return b.integ.drag.ownPos
	}
	return w.posf(float64(dx), float64(dy))
}

// ownEvent builds a DragEvent of the app's drag over its own window.
func (b *Backend) ownEvent(action platform.DragAction) platform.DragEvent {
	ds := b.integ.drag
	return platform.DragEvent{Action: action, Pos: ds.ownPos, Types: slices.Clone(ds.types), Allowed: ds.allowed,
		Mods: ds.mods, FromApp: true, Time: b.Now()}
}

// sendXdndTo sends an XDND message about window to dest (a proxy, or the
// window itself).
func (b *Backend) sendXdndTo(dest, window xproto.Window, typ xproto.Atom, data ...uint32) {
	var d [5]uint32
	copy(d[:], data)
	ev := xproto.ClientMessageEvent{Format: 32, Window: window, Type: typ, Data: xproto.ClientMessageDataUnionData32New(d[:])}
	b.xs.sendEvent(dest, 0, ev.Bytes())
}

// sendPosition sends XdndPosition, or remembers to once the status of the
// one in flight arrives.
func (b *Backend) sendPosition() {
	ds := b.integ.drag
	if ds.waiting {
		ds.moved = true
		return
	}
	xy := uint32(uint16(ds.rootX))<<16 | uint32(uint16(ds.rootY))
	b.sendXdndTo(ds.dest, ds.target, b.atoms.xdndPosition, uint32(ds.w.id), 0, xy, ds.time, uint32(ds.requested(b)))
	ds.waiting, ds.moved = true, false
}

// dragStatus is a target's XdndStatus.
func (b *Backend) dragStatus(w *Window, target xproto.Window, flags uint32, action xproto.Atom) {
	ds := b.integ.drag
	if ds == nil || ds.w != w || target != ds.target || ds.target == 0 || !ds.waiting || ds.dropped {
		return
	}
	ds.waiting = false
	ds.accept, ds.asked = platform.DropNone, false
	if flags&1 != 0 {
		e := b.effectOf(action)
		if !ds.allowed.Has(e) {
			// Older targets send no action; a target that will ask
			// the user answers XdndActionAsk, and XdndFinished brings
			// the choice.
			e = platform.DefaultDropEffect(ds.allowed, ds.mods)
			ds.asked = action == b.atoms.xdndActionAsk
		}
		ds.accept = e
	}
	b.updateDragCursor()
	switch {
	case ds.released:
		b.dragDrop()
	case ds.moved:
		b.sendPosition()
	}
}

// dragRelease is the release of the drag's button.
func (b *Backend) dragRelease() {
	ds := b.integ.drag
	if ds == nil || ds.released {
		return
	}
	ds.released = true
	switch {
	case ds.own != nil:
		b.ownDrop(ds)
	case ds.target != 0 && ds.waiting:
		ds.stop = b.after(dragStatusTimeout, func() {
			if b.integ.drag == ds && ds.waiting {
				ds.waiting, ds.accept = false, platform.DropNone
				b.dragDrop()
			}
		})
	case ds.target != 0:
		b.dragDrop()
	default:
		b.endDrag(ds, platform.DropNone)
	}
}

// dragDrop drops on the XDND target after the release, once no status is
// in flight: XdndDrop if it accepts, else XdndLeave.
func (b *Backend) dragDrop() {
	ds := b.integ.drag
	if ds.stop != nil {
		ds.stop()
		ds.stop = nil
	}
	if ds.accept == platform.DropNone {
		b.dragLeaveCurrent()
		b.endDrag(ds, platform.DropNone)
		return
	}
	b.sendXdndTo(ds.dest, ds.target, b.atoms.xdndDrop, uint32(ds.w.id), 0, ds.time)
	ds.dropped = true
	b.ungrabDrag()
	ds.stop = b.after(dragFinishTimeout, func() { b.endDrag(ds, platform.DropNone) })
}

// dragFinished is the target's XdndFinished.
func (b *Backend) dragFinished(w *Window, target xproto.Window, flags uint32, action xproto.Atom) {
	ds := b.integ.drag
	if ds == nil || ds.w != w || target != ds.target || !ds.dropped {
		return
	}
	effect := ds.accept
	if ds.version >= 5 {
		effect = platform.DropNone
		if flags&1 != 0 {
			effect = b.effectOf(action)
			if !ds.allowed.Has(effect) {
				effect = ds.accept
			}
		}
	}
	b.endDrag(ds, effect)
}

// ownDrop drops on the app's own window, behind the queued events (the
// answer is the one the app gave to the last of them).
func (b *Backend) ownDrop(ds *dragSource) {
	ds.dropped = true
	b.ungrabDrag()
	b.loop.Post(func() {
		ow := ds.own
		if b.integ.drag != ds || ow == nil {
			return
		}
		ds.own = nil
		ow.dnd.appOver = false
		if ow.Closed() {
			b.endDrag(ds, platform.DropNone)
			return
		}
		if ow.dnd.accepted(ds.allowed) == platform.DropNone {
			ow.dnd.answer = dropAnswer{}
			b.loop.QueueDrag(ow, b.ownEvent(platform.DragLeave))
			b.endDrag(ds, platform.DropNone)
			return
		}
		ev := b.ownEvent(platform.DragDrop)
		ev.Data = ds.data.Select(platform.DropTypes(ds.types, ow.dnd.answer.types)).Clone()
		b.loop.QueueDrag(ow, ev)
		ow.dnd.dropping = true
		b.loop.Post(func() {
			effect := ow.dnd.accepted(ds.allowed)
			ow.dnd.dropping, ow.dnd.answer = false, dropAnswer{}
			b.endDrag(ds, effect)
		})
	})
}

// dragLeaveCurrent leaves the window the drag is over.
func (b *Backend) dragLeaveCurrent() {
	ds := b.integ.drag
	if ow := ds.own; ow != nil {
		ds.own = nil
		ow.dnd.appOver, ow.dnd.answer = false, dropAnswer{}
		b.loop.QueueDrag(ow, b.ownEvent(platform.DragLeave))
	}
	if ds.target != 0 {
		b.sendXdndTo(ds.dest, ds.target, b.atoms.xdndLeave, uint32(ds.w.id))
		ds.target, ds.dest, ds.version = 0, 0, 0
		ds.waiting, ds.moved, ds.accept, ds.asked = false, false, platform.DropNone, false
	}
}

// cancelDrag ends the drag as Escape does, unless it already dropped.
func (b *Backend) cancelDrag() {
	ds := b.integ.drag
	if ds == nil || ds.dropped {
		return
	}
	b.dragLeaveCurrent()
	b.endDrag(ds, platform.DropNone)
}

// updateDragCursor shows what a drop would do.
func (b *Backend) updateDragCursor() {
	ds := b.integ.drag
	if ds.ungrabbed {
		return
	}
	effect := ds.accept
	if ds.own != nil {
		effect = ds.own.dnd.accepted(ds.allowed)
	}
	c := platform.CursorNoDrop
	switch {
	case ds.own == nil && ds.target == 0:
		c = platform.CursorGrabbing
	case ds.own == nil && ds.asked:
		c = platform.CursorContextMenu // GTK's dnd-ask cursor
	case effect == platform.DropCopy:
		c = platform.CursorCopy
	case effect == platform.DropMove:
		c = platform.CursorMove
	case effect == platform.DropLink:
		c = platform.CursorAlias
	}
	if c != ds.cursor {
		ds.cursor = c
		b.xs.changeGrabCursor(c)
	}
}

// ungrabDrag releases the pointer and keyboard grabs of the drag.
func (b *Backend) ungrabDrag() {
	ds := b.integ.drag
	if ds.ungrabbed {
		return
	}
	ds.ungrabbed = true
	b.hideDragIcon(ds)
	b.xs.ungrabPointerCore()
	if ds.grabbedKbd {
		b.xs.ungrabKeyboard()
	}
}

// endDrag ends drag ds with effect: the grabs and XdndSelection go, and
// the source window gets its DragDoneEvent.
func (b *Backend) endDrag(ds *dragSource, effect platform.DropEffect) {
	if b.integ.drag != ds {
		return
	}
	if ds.stop != nil {
		ds.stop()
		ds.stop = nil
	}
	b.ungrabDrag()
	if o := b.integ.owned[b.atoms.xdndSelection]; o != nil && o.owner == ds.w.id {
		b.disown(b.atoms.xdndSelection)
	}
	b.integ.drag = nil
	if !ds.w.Closed() {
		b.enqueue(ds.w, platform.DragDoneEvent{Effect: effect})
	}
	b.signal()
}

// dragWindowClosed handles a window closing during the app's drag: the
// source cancels it, a target of the drag is left.
func (b *Backend) dragWindowClosed(w *Window) {
	ds := b.integ.drag
	if ds == nil {
		return
	}
	if ds.own == w {
		ds.own = nil
	}
	if ds.w == w {
		b.dragLeaveCurrent()
		b.endDrag(ds, platform.DropNone)
	}
}
