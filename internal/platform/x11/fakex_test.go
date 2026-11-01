//go:build linux

package x11

import (
	"errors"
	"fmt"
	"image"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// fakeX stands in for the server side of selections, drags and key grabs
// (dataServer): it records requests as text, keeps properties and
// selection owners, interns atoms, answers pointer and coordinate
// queries from a scene of rectangles, and fails the key grabs the test
// marks as taken by another client.
type fakeX struct {
	b    *Backend
	f    *fakeServer // shares its atom names
	next xproto.Atom
	sent []string

	props  map[xproto.Window]map[xproto.Atom]xproperty
	owners map[xproto.Atom]xproto.Window

	// The scene: each window's root origin and its children (top-most
	// last), and the pointer.
	origin   map[xproto.Window]image.Point
	children map[xproto.Window][]fakeChild
	pointer  image.Point
	mask     uint16

	taken     map[[2]int]bool // key grabs another client holds
	grabbed   map[[2]int]bool
	noPointer bool // the pointer grab fails
	// setOwnerHook, if set, runs after SetSelectionOwner (another client
	// taking the selection back at once).
	setOwnerHook func(sel xproto.Atom)

	// Timers: fired by fire, in creation order.
	timers []*fakeTimer
}

type fakeChild struct {
	win  xproto.Window
	rect image.Rectangle // root coordinates
}

type fakeTimer struct {
	d       time.Duration
	fn      func()
	stopped bool
}

const fakeSelWin = 900

// newFakeX installs a fakeX in a fake backend (fakeEmpty).
func newFakeX(b *Backend) *fakeX {
	x := &fakeX{
		b: b, f: b.srv.(*fakeServer), next: 1000,
		props:    map[xproto.Window]map[xproto.Atom]xproperty{},
		owners:   map[xproto.Atom]xproto.Window{},
		origin:   map[xproto.Window]image.Point{},
		children: map[xproto.Window][]fakeChild{},
		taken:    map[[2]int]bool{},
		grabbed:  map[[2]int]bool{},
	}
	for a, n := range map[xproto.Atom]string{xproto.AtomString: "STRING", xproto.AtomAtom: "ATOM",
		xproto.AtomInteger: "INTEGER", xproto.AtomWindow: "WINDOW"} {
		x.f.names[a] = n
	}
	b.xs = x
	b.cacheKnownAtoms()
	b.timers.hook = func(d time.Duration, fn func()) func() {
		t := &fakeTimer{d: d, fn: fn}
		x.timers = append(x.timers, t)
		return func() { t.stopped = true }
	}
	return x
}

// fire runs the pending timers of duration d (all pending ones for 0).
func (x *fakeX) fire(d time.Duration) int {
	n := 0
	for _, t := range slices.Clone(x.timers) {
		if !t.stopped && (d == 0 || t.d == d) {
			t.stopped = true
			t.fn()
			n++
		}
	}
	return n
}

func (x *fakeX) log(format string, args ...any) {
	x.sent = append(x.sent, fmt.Sprintf(format, args...))
}

func (x *fakeX) take() string {
	s := strings.Join(x.sent, "; ")
	x.sent = nil
	return s
}

func (x *fakeX) name(a xproto.Atom) string {
	if a == 0 {
		return "None"
	}
	return x.f.atomName(uint32(a))
}

func (x *fakeX) id(name string) xproto.Atom {
	a, _ := x.internAtom(name)
	return a
}

func (x *fakeX) internAtom(name string) (xproto.Atom, error) {
	for a, n := range x.f.names {
		if n == name {
			return a, nil
		}
	}
	x.next++
	x.f.names[x.next] = name
	return x.next, nil
}

func (x *fakeX) atomName(a xproto.Atom) (string, error) {
	if n, ok := x.f.names[a]; ok {
		return n, nil
	}
	return "", errors.New("BadAtom")
}

func (x *fakeX) createHidden() (xproto.Window, error) {
	x.log("create selection window")
	return fakeSelWin, nil
}

func (x *fakeX) setOwner(win xproto.Window, sel xproto.Atom, t uint32) {
	x.owners[sel] = win
	x.log("own %s win=%d", x.name(sel), win)
	if x.setOwnerHook != nil {
		x.setOwnerHook(sel)
	}
}

func (x *fakeX) owner(sel xproto.Atom) xproto.Window { return x.owners[sel] }

func (x *fakeX) convert(req xproto.Window, sel, target, prop xproto.Atom, t uint32) {
	x.log("convert %s %s to %s t=%d", x.name(sel), x.name(target), x.name(prop), t)
}

// setProp sets a property as another client would.
func (x *fakeX) setProp(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	if x.props[win] == nil {
		x.props[win] = map[xproto.Atom]xproperty{}
	}
	x.props[win][prop] = xproperty{typ: typ, format: format, data: data}
}

func (x *fakeX) property(win xproto.Window, prop xproto.Atom, del bool) (xproperty, bool) {
	p, ok := x.props[win][prop]
	if ok && del {
		delete(x.props[win], prop)
		x.log("read+delete %s win=%d", x.name(prop), win)
	}
	return p, ok
}

func (x *fakeX) changeProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	x.setProp(win, prop, typ, format, data)
	x.log("prop %s win=%d %s = %s", x.name(prop), win, x.name(typ), x.describe(typ, format, data))
}

func (x *fakeX) describe(typ xproto.Atom, format byte, data []byte) string {
	if format != 32 {
		if len(data) > 40 {
			return fmt.Sprintf("%d bytes", len(data))
		}
		return fmt.Sprintf("%q", data)
	}
	var parts []string
	for i := 0; i+4 <= len(data); i += 4 {
		v := le.Uint32(data[i:])
		if typ == xproto.AtomAtom || typ == x.b.atoms.atomPair {
			parts = append(parts, x.name(xproto.Atom(v)))
		} else {
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func (x *fakeX) deleteProperty(win xproto.Window, prop xproto.Atom) {
	delete(x.props[win], prop)
}

func (x *fakeX) sendEvent(dest xproto.Window, mask uint32, ev []byte) {
	switch ev[0] {
	case xproto.SelectionNotify:
		x.log("notify win=%d %s %s prop=%s", dest, x.name(xproto.Atom(le.Uint32(ev[12:]))),
			x.name(xproto.Atom(le.Uint32(ev[16:]))), x.name(xproto.Atom(le.Uint32(ev[20:]))))
	case xproto.ClientMessage:
		typ := xproto.Atom(le.Uint32(ev[8:]))
		if ev[1] == 8 {
			x.log("msg %s to=%d %q", x.name(typ), dest, strings.TrimRight(string(ev[12:32]), "\x00"))
			return
		}
		a := &x.b.atoms
		atomAt := map[xproto.Atom][]int{a.xdndEnter: {2, 3, 4}, a.xdndPosition: {4}, a.xdndStatus: {4}, a.xdndFinished: {2}}[typ]
		var parts []string
		for i := range 5 {
			v := le.Uint32(ev[12+4*i:])
			switch {
			case slices.Contains(atomAt, i):
				parts = append(parts, x.name(xproto.Atom(v)))
			case typ == a.xdndPosition && i == 2:
				parts = append(parts, fmt.Sprintf("%d,%d", int16(v>>16), int16(v)))
			default:
				parts = append(parts, fmt.Sprint(v))
			}
		}
		x.log("msg %s to=%d win=%d [%s]", x.name(typ), dest, le.Uint32(ev[4:]), strings.Join(parts, " "))
	default:
		x.log("event %d to=%d", ev[0], dest)
	}
}

func (x *fakeX) selectInput(win xproto.Window, mask uint32) {
	x.log("select win=%d mask=%#x", win, mask)
}

func (x *fakeX) queryPointer(win xproto.Window) (pointerInfo, bool) {
	o := x.origin[win]
	return pointerInfo{rootX: int16(x.pointer.X), rootY: int16(x.pointer.Y),
		winX: int16(x.pointer.X - o.X), winY: int16(x.pointer.Y - o.Y), mask: x.mask}, true
}

func (x *fakeX) translate(src, dst xproto.Window, px, py int16) (int16, int16, xproto.Window, bool) {
	o := x.origin[dst]
	var child xproto.Window
	for _, c := range x.children[dst] {
		if image.Pt(int(px), int(py)).In(c.rect) {
			child = c.win
		}
	}
	return px - int16(o.X), py - int16(o.Y), child, true
}

func (x *fakeX) grabPointer(win xproto.Window, c platform.Cursor) bool {
	if x.noPointer {
		return false
	}
	x.log("grab pointer win=%d %v", win, c)
	return true
}

func (x *fakeX) changeGrabCursor(c platform.Cursor) { x.log("cursor %v", c) }
func (x *fakeX) ungrabPointerCore()                 { x.log("ungrab pointer") }

func (x *fakeX) grabKeyboard(win xproto.Window) bool {
	x.log("grab keyboard win=%d", win)
	return true
}

func (x *fakeX) ungrabKeyboard() { x.log("ungrab keyboard") }

func (x *fakeX) grabKey(kc byte, mods uint16) error {
	k := [2]int{int(kc), int(mods)}
	if x.taken[k] {
		return xproto.AccessError{NiceName: "Access"}
	}
	x.grabbed[k] = true
	return nil
}

func (x *fakeX) ungrabKey(kc byte, mods uint16) { delete(x.grabbed, [2]int{int(kc), int(mods)}) }

// grabs returns the key grabs held, sorted.
func (x *fakeX) grabs() string {
	keys := slices.SortedFunc(maps.Keys(x.grabbed), func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	var out []string
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%d/%#x", k[0], k[1]))
	}
	return strings.Join(out, " ")
}

// --- recording the app's side ---

// appRecorder records events like recorder, with the phase-13 events,
// and runs an optional reaction inside the handler.
type appRecorder struct {
	events []string
	react  func(w platform.Window, e platform.Event)
	data   []platform.Data // DragDrop data, in order
}

func (r *appRecorder) HandleEvent(w platform.Window, e platform.Event) {
	var s string
	switch e := e.(type) {
	case platform.DragEvent:
		s = fmt.Sprintf("drag %v %v,%v %v allowed=%v mods=%b", e.Action, e.Pos.X, e.Pos.Y, e.Types, e.Allowed, e.Mods)
		if e.FromApp {
			s += " app"
		}
		if e.Action == platform.DragDrop {
			r.data = append(r.data, e.Data)
			s += fmt.Sprintf(" data=%v", e.Data.Types())
		}
	case platform.DragDoneEvent:
		s = "done " + e.Effect.String()
	case platform.ShortcutEvent:
		s = fmt.Sprintf("shortcut %s %v", e.ID, e.Action)
	case platform.ShortcutStatusEvent:
		s = fmt.Sprintf("status %s %v %q", e.ID, e.Status, e.Trigger)
		if e.Reason != "" {
			s += " (" + e.Reason + ")"
		}
	case platform.PointerEvent:
		s = fmt.Sprintf("pointer %v", e.Action)
	case platform.ConfigureEvent, platform.FrameEvent, platform.WindowStateEvent:
		return
	default:
		s = fmt.Sprintf("%T", e)
	}
	r.events = append(r.events, s)
	if r.react != nil {
		r.react(w, e)
	}
}

func (r *appRecorder) take() string {
	s := strings.Join(r.events, "; ")
	r.events = nil
	return s
}

// dataRig is a fake backend with the fake data server and window 7.
type dataRig struct {
	b *Backend
	x *fakeX
	w *Window
	r *appRecorder
}

func newDataRig() *dataRig {
	b, w := fakeBackend(1)
	x := newFakeX(b)
	g := &dataRig{b: b, x: x, w: w, r: &appRecorder{}}
	g.feed()
	g.r.take()
	return g
}

// feed translates events as one batch and dispatches; it returns what the
// app received.
func (g *dataRig) feed(evs ...xgb.Event) string {
	for _, e := range evs {
		g.b.translate(xevent{ev: e})
	}
	g.b.integ.sc.flushRelease(g.b)
	g.b.dispatch(g.r)
	return g.r.take()
}

// pass runs delivery passes until nothing is ready (posted callbacks that
// post more), returning what the app received.
func (g *dataRig) pass() string {
	var out []string
	for range 10 {
		g.b.dispatch(g.r)
		if s := g.r.take(); s != "" {
			out = append(out, s)
		}
		if !g.b.loop.Ready() {
			break
		}
	}
	return strings.Join(out, "; ")
}

// propNotify is a PropertyNotify of prop on win.
func propNotify(win xproto.Window, prop xproto.Atom, deleted bool) xproto.PropertyNotifyEvent {
	st := byte(xproto.PropertyNewValue)
	if deleted {
		st = xproto.PropertyDelete
	}
	return xproto.PropertyNotifyEvent{Window: win, Atom: prop, State: st}
}

// atomList encodes atoms as a format-32 ATOM property.
func atomList(atoms ...xproto.Atom) []byte {
	v := make([]uint32, len(atoms))
	for i, a := range atoms {
		v[i] = uint32(a)
	}
	return card32s(v...)
}

// clientMsg is a format-32 client message to win.
func clientMsg(win xproto.Window, typ xproto.Atom, data ...uint32) xproto.ClientMessageEvent {
	var d [5]uint32
	copy(d[:], data)
	return xproto.ClientMessageEvent{Format: 32, Window: win, Type: typ, Data: xproto.ClientMessageDataUnionData32New(d[:])}
}
