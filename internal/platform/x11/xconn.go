//go:build linux

package x11

import (
	"errors"
	"image"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// dataServer is what the phase-13 features (selections, drag and drop,
// key grabs) need from the X connection, beside window control
// (wmServer). The backend uses connServer; unit tests replace it with a
// fake that records requests and answers queries, so the protocol state
// machines run without a server.
type dataServer interface {
	internAtom(name string) (xproto.Atom, error) // a round trip
	atomName(a xproto.Atom) (string, error)      // a round trip
	// createHidden creates the backend's selection window: an unmapped
	// InputOnly child of the root with PropertyChange selected (INCR
	// reads, timestamps).
	createHidden() (xproto.Window, error)
	setOwner(win xproto.Window, sel xproto.Atom, time uint32)
	owner(sel xproto.Atom) xproto.Window // a round trip
	convert(requestor xproto.Window, sel, target, prop xproto.Atom, time uint32)
	// property reads a whole property (a round trip), deleting it when
	// del is set and it was read whole. ok is false when it does not
	// exist.
	property(win xproto.Window, prop xproto.Atom, del bool) (p xproperty, ok bool)
	changeProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte)
	deleteProperty(win xproto.Window, prop xproto.Atom)
	// sendEvent sends a 32-byte event to dest (SendEvent, no
	// propagation).
	sendEvent(dest xproto.Window, mask uint32, ev []byte)
	// selectInput sets this client's event mask on another client's
	// window (PropertyChange on an INCR requestor).
	selectInput(win xproto.Window, mask uint32)
	queryPointer(win xproto.Window) (pointerInfo, bool) // a round trip
	// translate maps (x, y) from src to dst coordinates and returns the
	// child of dst that contains the point (0 if none); a round trip.
	translate(src, dst xproto.Window, x, y int16) (dx, dy int16, child xproto.Window, ok bool)
	grabPointer(win xproto.Window, c platform.Cursor) bool // a round trip
	changeGrabCursor(c platform.Cursor)
	ungrabPointerCore()
	grabKeyboard(win xproto.Window) bool // a round trip
	ungrabKeyboard()
	// grabKey makes a passive grab of keycode kc with modifiers mods on
	// the root window (checked: a round trip; BadAccess when another
	// client holds the combination).
	grabKey(kc byte, mods uint16) error
	ungrabKey(kc byte, mods uint16)
	// inputShapes reports whether the server has SHAPE 1.1 or later
	// (input shapes); a round trip the first time.
	inputShapes() bool
	// createDragIcon creates the unmapped window of a drag image at r
	// (root coordinates; dragicon.go has the details): the ARGB visual
	// when argb, pix (32-bpp ZPixmap, stride 4*r.Dx()) as its
	// background, an empty input shape, and the bounding shape bounding
	// unless it is nil. 0 when the server refuses it (a round trip).
	createDragIcon(r image.Rectangle, argb bool, pix []byte, bounding []xproto.Rectangle) xproto.Window
	// moveIcon moves the drag image's window to (x, y) on top of its
	// siblings; mapIcon shows it, destroyIcon destroys it.
	moveIcon(win xproto.Window, x, y int)
	mapIcon(win xproto.Window)
	destroyIcon(win xproto.Window)

	// The input method client (ime.go).
	//
	// createIMWindow creates the XIM client's window: an unmapped
	// InputOnly child of the root that receives the transport's
	// ClientMessages and the selection conversions.
	createIMWindow() (xproto.Window, error)
	// propertyPart reads length 4-byte units of a property from offset
	// (in 4-byte units), deleting it when del is set and nothing is left
	// after the part (a round trip). after is the bytes left.
	propertyPart(win xproto.Window, prop xproto.Atom, offset, length uint32, del bool) (data []byte, after uint32, ok bool)
	// appendProperty appends to a property (PropModeAppend).
	appendProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte)
}

// xproperty is a property's value.
type xproperty struct {
	typ    xproto.Atom
	format byte
	data   []byte
}

// pointerInfo is a QueryPointer reply.
type pointerInfo struct {
	rootX, rootY int16
	winX, winY   int16
	child        xproto.Window
	mask         uint16
}

// --- connServer: the real connection ---

func (s connServer) internAtom(name string) (xproto.Atom, error) {
	r, err := xproto.InternAtom(s.b.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	if r == nil {
		return 0, errConnLost
	}
	return r.Atom, nil
}

func (s connServer) atomName(a xproto.Atom) (string, error) {
	r, err := xproto.GetAtomName(s.b.conn, a).Reply()
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", errConnLost
	}
	return r.Name, nil
}

func (s connServer) createHidden() (xproto.Window, error) {
	b := s.b
	id, err := xproto.NewWindowId(b.conn)
	if err != nil {
		return 0, err
	}
	err = xproto.CreateWindowChecked(b.conn, 0, id, b.root, -100, -100, 1, 1, 0,
		xproto.WindowClassInputOnly, 0, xproto.CwOverrideRedirect|xproto.CwEventMask,
		[]uint32{1, xproto.EventMaskPropertyChange}).Check()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s connServer) setOwner(win xproto.Window, sel xproto.Atom, time uint32) {
	xproto.SetSelectionOwner(s.b.conn, win, sel, xproto.Timestamp(time))
}

func (s connServer) owner(sel xproto.Atom) xproto.Window {
	r, err := xproto.GetSelectionOwner(s.b.conn, sel).Reply()
	if err != nil || r == nil {
		return 0
	}
	return r.Owner
}

func (s connServer) convert(requestor xproto.Window, sel, target, prop xproto.Atom, time uint32) {
	xproto.ConvertSelection(s.b.conn, requestor, sel, target, prop, xproto.Timestamp(time))
}

// maxPropertyRead bounds one GetProperty, in 4-byte units (1 GiB).
const maxPropertyRead = 1 << 28

func (s connServer) property(win xproto.Window, prop xproto.Atom, del bool) (xproperty, bool) {
	r, err := xproto.GetProperty(s.b.conn, del, win, prop, xproto.GetPropertyTypeAny, 0, maxPropertyRead).Reply()
	if err != nil || r == nil || r.Type == 0 {
		return xproperty{}, false
	}
	n := int(r.ValueLen) * int(r.Format) / 8
	return xproperty{typ: r.Type, format: r.Format, data: r.Value[:min(n, len(r.Value))]}, true
}

func (s connServer) changeProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	s.setProperty(win, prop, typ, format, data)
}

func (s connServer) sendEvent(dest xproto.Window, mask uint32, ev []byte) {
	xproto.SendEvent(s.b.conn, false, dest, mask, string(ev))
}

func (s connServer) selectInput(win xproto.Window, mask uint32) {
	xproto.ChangeWindowAttributes(s.b.conn, win, xproto.CwEventMask, []uint32{mask})
}

func (s connServer) queryPointer(win xproto.Window) (pointerInfo, bool) {
	r, err := xproto.QueryPointer(s.b.conn, win).Reply()
	if err != nil || r == nil || !r.SameScreen {
		return pointerInfo{}, false
	}
	return pointerInfo{rootX: r.RootX, rootY: r.RootY, winX: r.WinX, winY: r.WinY, child: r.Child, mask: r.Mask}, true
}

func (s connServer) translate(src, dst xproto.Window, x, y int16) (int16, int16, xproto.Window, bool) {
	r, err := xproto.TranslateCoordinates(s.b.conn, src, dst, x, y).Reply()
	if err != nil || r == nil || !r.SameScreen {
		return 0, 0, 0, false
	}
	return r.DstX, r.DstY, r.Child, true
}

// dragPointerEvents are the pointer events of the drag grab.
const dragPointerEvents = xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskPointerMotion

func (s connServer) grabPointer(win xproto.Window, c platform.Cursor) bool {
	b := s.b
	r, err := xproto.GrabPointer(b.conn, false, win, dragPointerEvents, xproto.GrabModeAsync, xproto.GrabModeAsync,
		0, b.cursorFor(c), xproto.TimeCurrentTime).Reply()
	return err == nil && r != nil && r.Status == xproto.GrabStatusSuccess
}

func (s connServer) changeGrabCursor(c platform.Cursor) {
	xproto.ChangeActivePointerGrab(s.b.conn, s.b.cursorFor(c), xproto.TimeCurrentTime, dragPointerEvents)
}

func (s connServer) ungrabPointerCore() { xproto.UngrabPointer(s.b.conn, xproto.TimeCurrentTime) }

func (s connServer) grabKeyboard(win xproto.Window) bool {
	r, err := xproto.GrabKeyboard(s.b.conn, false, win, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
	return err == nil && r != nil && r.Status == xproto.GrabStatusSuccess
}

func (s connServer) ungrabKeyboard() { xproto.UngrabKeyboard(s.b.conn, xproto.TimeCurrentTime) }

func (s connServer) grabKey(kc byte, mods uint16) error {
	// owner_events false: every press of the combination is reported to
	// the root window, even while one of the app's windows has the
	// focus, so shortcuts never reach the app's key handlers.
	return xproto.GrabKeyChecked(s.b.conn, false, s.b.root, mods, xproto.Keycode(kc), xproto.GrabModeAsync, xproto.GrabModeAsync).Check()
}

func (s connServer) ungrabKey(kc byte, mods uint16) {
	xproto.UngrabKey(s.b.conn, xproto.Keycode(kc), s.b.root, mods)
}

func (s connServer) createIMWindow() (xproto.Window, error) {
	b := s.b
	id, err := xproto.NewWindowId(b.conn)
	if err != nil {
		return 0, err
	}
	err = xproto.CreateWindowChecked(b.conn, 0, id, b.root, -100, -100, 1, 1, 0,
		xproto.WindowClassInputOnly, 0, xproto.CwOverrideRedirect, []uint32{1}).Check()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s connServer) propertyPart(win xproto.Window, prop xproto.Atom, offset, length uint32, del bool) ([]byte, uint32, bool) {
	r, err := xproto.GetProperty(s.b.conn, del, win, prop, xproto.GetPropertyTypeAny, offset, length).Reply()
	if err != nil || r == nil || r.Type == 0 {
		return nil, 0, false
	}
	n := int(r.ValueLen) * int(r.Format) / 8
	return r.Value[:min(n, len(r.Value))], r.BytesAfter, true
}

func (s connServer) appendProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	xproto.ChangeProperty(s.b.conn, xproto.PropModeAppend, win, prop, typ, format, uint32(len(data))/uint32(format/8), data)
}

// isAccessError reports whether err is the X error BadAccess.
func isAccessError(err error) bool {
	var ae xproto.AccessError
	return errors.As(err, &ae)
}

// --- atoms by name ---

// atom returns the atom of name, interning it on first use (a round
// trip). 0 when the server fails.
func (b *Backend) atom(name string) xproto.Atom {
	if a, ok := b.atomIDs[name]; ok {
		return a
	}
	a, err := b.xs.internAtom(name)
	if err != nil || a == 0 {
		return 0
	}
	b.cacheAtom(a, name)
	return a
}

// atomName returns the name of a (a round trip on first use); "" for 0 or
// an unknown atom.
func (b *Backend) atomName(a xproto.Atom) string {
	if a == 0 {
		return ""
	}
	if n, ok := b.atomNames[a]; ok {
		return n
	}
	n, err := b.xs.atomName(a)
	if err != nil {
		return ""
	}
	b.cacheAtom(a, n)
	return n
}

// atomNamesOf returns the names of atoms.
func (b *Backend) atomNamesOf(atoms []xproto.Atom) []string {
	out := make([]string, 0, len(atoms))
	for _, a := range atoms {
		if n := b.atomName(a); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (b *Backend) cacheAtom(a xproto.Atom, name string) {
	if b.atomIDs == nil {
		b.atomIDs, b.atomNames = map[string]xproto.Atom{}, map[xproto.Atom]string{}
	}
	b.atomIDs[name], b.atomNames[a] = a, name
}

// cacheKnownAtoms seeds the cache with the atoms interned at startup and
// the predefined ones the selections use.
func (b *Backend) cacheKnownAtoms() {
	for _, e := range b.atoms.list() {
		b.cacheAtom(*e.dst, e.name)
	}
	b.cacheAtom(xproto.AtomString, "STRING")
	b.cacheAtom(xproto.AtomAtom, "ATOM")
	b.cacheAtom(xproto.AtomInteger, "INTEGER")
	b.cacheAtom(xproto.AtomWindow, "WINDOW")
}

// --- timers on the UI thread ---

// timers runs callbacks on the UI thread after a delay: time.AfterFunc
// moves them to an inbox and wakes the loop, which runs them before the
// next delivery pass (runTimers). Timeouts of selection transfers and
// drags use it; nothing ticks while idle.
type timers struct {
	mu    sync.Mutex
	inbox []func()
	// hook replaces time.AfterFunc in tests.
	hook func(d time.Duration, fn func()) (stop func())
}

// after runs fn on the UI thread after d, unless stop is called first.
func (b *Backend) after(d time.Duration, fn func()) (stop func()) {
	if b.timers.hook != nil {
		return b.timers.hook(d, fn)
	}
	var mu sync.Mutex
	stopped := false
	t := time.AfterFunc(d, func() {
		b.timers.mu.Lock()
		b.timers.inbox = append(b.timers.inbox, func() {
			mu.Lock()
			s := stopped
			mu.Unlock()
			if !s {
				fn()
			}
		})
		b.timers.mu.Unlock()
		b.signal()
	})
	return func() {
		mu.Lock()
		stopped = true
		mu.Unlock()
		t.Stop()
	}
}

// runTimers runs the callbacks of the timers that fired.
func (b *Backend) runTimers() {
	b.timers.mu.Lock()
	fns := b.timers.inbox
	b.timers.inbox = nil
	b.timers.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}
