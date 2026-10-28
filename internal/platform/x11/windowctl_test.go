//go:build linux

package x11

import (
	"encoding/binary"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
)

// fakeServer stands in for the X server and the window manager in unit
// tests: it records the window control requests as text, keeps the
// properties the backend writes, and answers property and position
// queries from what the test sets.
type fakeServer struct {
	b     *Backend
	names map[xproto.Atom]string
	sent  []string
	props map[xproto.Window]map[xproto.Atom][]uint32
	pos   map[xproto.Window]image.Point
}

// newFakeServer gives every atom of b a distinct value and makes the
// window manager support every EWMH hint the backend uses.
func newFakeServer(b *Backend) *fakeServer {
	f := &fakeServer{
		b: b,
		names: map[xproto.Atom]string{
			xproto.AtomWmHints:        "WM_HINTS",
			xproto.AtomWmNormalHints:  "WM_NORMAL_HINTS",
			xproto.AtomWmTransientFor: "WM_TRANSIENT_FOR",
		},
		props: map[xproto.Window]map[xproto.Atom][]uint32{},
		pos:   map[xproto.Window]image.Point{},
	}
	var all []uint32
	for i, e := range b.atoms.list() {
		*e.dst = xproto.Atom(100 + i)
		f.names[*e.dst] = e.name
		all = append(all, uint32(*e.dst))
	}
	f.set(b.root, b.atoms.netSupported, all...)
	return f
}

// take returns the requests sent since the last call, joined with "; ".
func (f *fakeServer) take() string {
	s := strings.Join(f.sent, "; ")
	f.sent = nil
	return s
}

func (f *fakeServer) log(format string, args ...any) {
	f.sent = append(f.sent, fmt.Sprintf(format, args...))
}

// set and unset change a property as the window manager would (the test
// then feeds the PropertyNotify).
func (f *fakeServer) set(win xproto.Window, prop xproto.Atom, values ...uint32) {
	if f.props[win] == nil {
		f.props[win] = map[xproto.Atom][]uint32{}
	}
	f.props[win][prop] = values
}

func (f *fakeServer) unset(win xproto.Window, prop xproto.Atom) { delete(f.props[win], prop) }

func (f *fakeServer) atomName(a uint32) string {
	if n, ok := f.names[xproto.Atom(a)]; ok {
		return n
	}
	return fmt.Sprint(a)
}

func (f *fakeServer) clientMessage(win xproto.Window, typ xproto.Atom, data ...uint32) {
	parts := make([]string, len(data))
	for i, v := range data {
		parts[i] = fmt.Sprint(v)
		if typ == f.b.atoms.netWMState && (i == 1 || i == 2) && v != 0 {
			parts[i] = f.atomName(v)
		}
	}
	f.log("msg %s win=%d [%s]", f.atomName(uint32(typ)), win, strings.Join(parts, " "))
}

func (f *fakeServer) withdrawn(win xproto.Window) { f.log("withdrawn win=%d", win) }

func (f *fakeServer) setProperty(win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	var values []uint32
	if format == 32 {
		for i := 0; i+4 <= len(data); i += 4 {
			values = append(values, binary.LittleEndian.Uint32(data[i:]))
		}
		f.set(win, prop, values...)
	}
	var desc string
	switch {
	case prop == f.b.atoms.netWMIcon:
		desc = fmt.Sprintf("%d bytes", len(data))
	case prop == xproto.AtomWmNormalHints:
		desc = formatHints(values)
	case prop == xproto.AtomWmHints:
		desc = fmt.Sprintf("state=%d", values[2])
	case typ == xproto.AtomAtom:
		names := make([]string, len(values))
		for i, v := range values {
			names[i] = f.atomName(v)
		}
		desc = "[" + strings.Join(names, " ") + "]"
	case format == 32:
		desc = fmt.Sprint(values)
	default:
		desc = fmt.Sprintf("%q", data)
	}
	f.log("prop %s win=%d = %s", f.atomName(uint32(prop)), win, desc)
}

// formatHints describes WM_NORMAL_HINTS by the fields its flags set.
func formatHints(v []uint32) string {
	var parts []string
	if v[0]&hintUSPosition != 0 {
		parts = append(parts, fmt.Sprintf("pos=%d,%d", int32(v[1]), int32(v[2])))
	}
	if v[0]&hintPMinSize != 0 {
		parts = append(parts, fmt.Sprintf("min=%dx%d", v[5], v[6]))
	}
	if v[0]&hintPMaxSize != 0 {
		parts = append(parts, fmt.Sprintf("max=%dx%d", v[7], v[8]))
	}
	if v[0]&hintPWinGravity != 0 {
		parts = append(parts, fmt.Sprintf("gravity=%d", v[17]))
	}
	return strings.Join(parts, " ")
}

func (f *fakeServer) deleteProperty(win xproto.Window, prop xproto.Atom) {
	f.unset(win, prop)
	f.log("delete %s win=%d", f.atomName(uint32(prop)), win)
}

func (f *fakeServer) configure(win xproto.Window, mask uint16, values []uint32) {
	var parts []string
	i := 0
	for _, field := range []struct {
		bit  uint16
		name string
	}{{xproto.ConfigWindowX, "x"}, {xproto.ConfigWindowY, "y"}, {xproto.ConfigWindowWidth, "w"}, {xproto.ConfigWindowHeight, "h"}} {
		if mask&field.bit != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", field.name, int32(values[i])))
			i++
		}
	}
	f.log("configure win=%d %s", win, strings.Join(parts, " "))
}

func (f *fakeServer) mapWindow(win xproto.Window, mapped bool) {
	if mapped {
		f.log("map win=%d", win)
	} else {
		f.log("unmap win=%d", win)
	}
}

func (f *fakeServer) ungrabPointer(device uint16) { f.log("ungrab dev=%d", device) }

func (f *fakeServer) setCursor(win xproto.Window, c platform.Cursor) {
	f.log("cursor win=%d %v", win, c)
}

func (f *fakeServer) property32(win xproto.Window, prop xproto.Atom) ([]uint32, bool) {
	v, ok := f.props[win][prop]
	return v, ok
}

func (f *fakeServer) rootPosition(win xproto.Window) (image.Point, bool) {
	p, ok := f.pos[win]
	return p, ok
}

func (f *fakeServer) destroy(win xproto.Window, _ xproto.Gcontext) { f.log("destroy win=%d", win) }

// fakeWindow adds a window to a fake backend the way NewWindow does after
// creating the server window: control state, first ConfigureEvent, initial
// WindowStateEvent.
func fakeWindow(b *Backend, id xproto.Window, size image.Point, opts platform.WindowOptions) *Window {
	opts = opts.Resolve()
	w := &Window{b: b, id: id, osSize: size, opts: opts, parent: b.parentOf(opts)}
	w.initCtl(opts)
	b.windows[id] = w
	b.loop.Add(w)
	b.queueConfigure(w)
	b.loop.UpdateState(w, w.osState)
	w.updateHidden()
	return w
}

// rig drives a fake backend: feed translates X events as one batch and
// dispatches, returning what the app received and what was sent to the
// server.
type rig struct {
	t *testing.T
	b *Backend
	f *fakeServer
	r *recorder
}

func newRig(t *testing.T) *rig {
	b := fakeEmpty(1)
	return &rig{t: t, b: b, f: b.srv.(*fakeServer), r: &recorder{}}
}

func (g *rig) feed(evs ...xgb.Event) (events, sent string) {
	for _, e := range evs {
		g.b.translate(xevent{ev: e})
	}
	g.b.endBatch()
	g.b.dispatch(g.r)
	return g.r.take(), g.f.take()
}

// expect feeds evs and checks both logs.
func (g *rig) expect(what string, wantEvents, wantSent string, evs ...xgb.Event) {
	g.t.Helper()
	events, sent := g.feed(evs...)
	if events != wantEvents {
		g.t.Errorf("%s: events\n got %s\nwant %s", what, events, wantEvents)
	}
	if sent != wantSent {
		g.t.Errorf("%s: requests\n got %s\nwant %s", what, sent, wantSent)
	}
}

// sent checks the requests sent by direct calls.
func (g *rig) sent(what, want string) {
	g.t.Helper()
	if got := g.f.take(); got != want {
		g.t.Errorf("%s: requests\n got %s\nwant %s", what, got, want)
	}
}

// present presents a full frame of w (its first maps it).
func (g *rig) present(w *Window) {
	g.t.Helper()
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	if err := w.Present(fb, nil); err != nil {
		g.t.Fatalf("Present: %v", err)
	}
}

func TestResizeEdge(t *testing.T) {
	size := platform.Size{W: 400, H: 300}
	for _, tc := range []struct {
		x, y float32
		want int
	}{
		{200, 150, edgeNone},
		{50, 150, edgeNone},
		{2, 150, moveResizeLeft},
		{397, 150, moveResizeRight},
		{200, 2, moveResizeTop},
		{20, 3, moveResizeTop}, // beyond the corner length
		{200, 297, moveResizeBottom},
		{2, 2, moveResizeTopLeft},
		{10, 3, moveResizeTopLeft},
		{3, 10, moveResizeTopLeft},
		{390, 2, moveResizeTopRight},
		{2, 290, moveResizeBottomLeft},
		{395, 295, moveResizeBottomRight},
		{-1, 5, edgeNone},
		{400, 5, edgeNone},
		{5, 300, edgeNone},
	} {
		if got := resizeEdge(platform.Point{X: tc.x, Y: tc.y}, size); got != tc.want {
			t.Errorf("resizeEdge(%v,%v) = %d, want %d", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestWindowControlEncodings(t *testing.T) {
	le := binary.LittleEndian
	if got := motifNoDecorations(); !slices.Equal(got, card32s(2, 0, 0, 0, 0)) {
		t.Errorf("_MOTIF_WM_HINTS = % x", got)
	}

	// Icons: non-premultiplied ARGB, width and height first.
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(img.Pix, []byte{128, 0, 0, 128, 0, 255, 0, 255}) // premultiplied
	small := image.NewNRGBA(image.Rect(5, 5, 6, 6))       // origin not at 0,0
	copy(small.Pix, []byte{1, 2, 3, 255})
	if got, want := netWMIcon([]image.Image{img, small}), card32s(2, 1, 0x80ff0000, 0xff00ff00, 1, 1, 0xff010203); !slices.Equal(got, want) {
		t.Errorf("_NET_WM_ICON = % x\nwant % x", got, want)
	}

	data := card32s(11, 12)
	req := encodeChangeProperty(false, xproto.PropModeReplace, 5, 6, 7, 32, data)
	if len(req) != 32 || req[0] != changePropertyOpcode || req[1] != xproto.PropModeReplace || le.Uint16(req[2:]) != 8 ||
		le.Uint32(req[4:]) != 5 || le.Uint32(req[8:]) != 6 || le.Uint32(req[12:]) != 7 || req[16] != 32 ||
		le.Uint32(req[20:]) != 2 || le.Uint32(req[24:]) != 11 || le.Uint32(req[28:]) != 12 {
		t.Errorf("ChangeProperty = % x", req)
	}
	req = encodeChangeProperty(true, xproto.PropModeAppend, 5, 6, 7, 32, data)
	if len(req) != 36 || req[1] != xproto.PropModeAppend || le.Uint16(req[2:]) != 0 || le.Uint32(req[4:]) != 9 ||
		le.Uint32(req[8:]) != 5 || le.Uint32(req[12:]) != 6 || le.Uint32(req[16:]) != 7 || req[20] != 32 ||
		le.Uint32(req[24:]) != 2 || le.Uint32(req[28:]) != 11 {
		t.Errorf("big ChangeProperty = % x", req)
	}

	h := sizeHints(normalHints{size: image.Pt(800, 600), min: image.Pt(0, 100), max: image.Pt(900, 0), pos: image.Pt(-20, 30), hasPos: true})
	v := make([]uint32, 18)
	for i := range v {
		v[i] = le.Uint32(h[4*i:])
	}
	if got, want := formatHints(v), "pos=-20,30 min=1x100 max=900x32767 gravity=10"; got != want || v[0]&hintPPosition == 0 {
		t.Errorf("hints = %s (flags %#x), want %s with PPosition", got, v[0], want)
	}
}

// TestWindowStateRequests checks requests made while the window is
// withdrawn (stored and written at map) and while it is managed (client
// messages), the state read back from the properties, and hide/show.
func TestWindowStateRequests(t *testing.T) {
	g := newRig(t)
	a := &g.b.atoms
	w := fakeWindow(g.b, 9, image.Pt(400, 300), platform.WindowOptions{Size: platform.Size{W: 400, H: 300},
		Maximized: true, AlwaysOnTop: true, HasPosition: true, Position: image.Pt(30, 40)})
	g.sent("NewWindow", "prop _NET_WM_WINDOW_TYPE win=9 = [_NET_WM_WINDOW_TYPE_NORMAL]")
	g.expect("first dispatch", "configure 400x300@1x (400x300 px); frame", "")

	// Before the first map: requests are only stored.
	w.SetFullscreen(true)
	w.SetMaximized(true)
	w.Minimize()
	w.SetPosition(image.Pt(30, 40))
	g.sent("requests before map", "")
	g.present(w)
	g.sent("first Present", strings.Join([]string{
		"configure win=9 x=30 y=40",
		"prop WM_NORMAL_HINTS win=9 = pos=30,40 gravity=10",
		"prop WM_HINTS win=9 = state=3",
		"prop _NET_WM_STATE win=9 = [_NET_WM_STATE_MAXIMIZED_VERT _NET_WM_STATE_MAXIMIZED_HORZ _NET_WM_STATE_FULLSCREEN _NET_WM_STATE_ABOVE]",
		"map win=9",
	}, "; "))

	// The window manager places it and maps it iconified.
	g.f.set(9, a.wmState, stateIconic, 0)
	g.f.set(9, a.netWMState, uint32(a.netWMStateMaxVert), uint32(a.netWMStateMaxHorz), uint32(a.netWMStateFull),
		uint32(a.netWMStateAbove), uint32(a.netWMStateHidden))
	g.expect("placed and iconified", "state {maximized fullscreen minimized at 30,40}", "",
		xproto.ConfigureNotifyEvent{Event: 9, Window: 9, X: 30, Y: 40, Width: 400, Height: 300},
		xproto.PropertyNotifyEvent{Window: 9, Atom: a.wmState})
	if !w.Hidden() {
		t.Errorf("a minimized window gets frames")
	}

	// Activate maps the iconified window and asks for the focus with the
	// latest user time.
	g.b.userTime = 1234
	w.Activate()
	g.sent("Activate", "map win=9; msg _NET_ACTIVE_WINDOW win=9 [1 1234 0]")
	g.f.set(9, a.wmState, stateNormal, 0)
	g.f.set(9, a.netWMState, uint32(a.netWMStateMaxVert), uint32(a.netWMStateMaxHorz), uint32(a.netWMStateAbove))
	g.expect("restored", "state {maximized at 30,40}", "", xproto.MapNotifyEvent{Event: 9, Window: 9})
	if w.Hidden() {
		t.Errorf("a restored window gets no frames")
	}

	// Managed: requests equal to the state are free, others are client
	// messages.
	w.SetMaximized(true)
	w.SetFullscreen(false)
	w.SetAlwaysOnTop(true)
	w.SetPosition(image.Pt(30, 40))
	g.sent("requests equal to the state", "")
	w.SetMaximized(false)
	w.SetMaximized(false)
	w.SetFullscreen(true)
	w.SetAlwaysOnTop(false)
	w.Minimize()
	w.SetPosition(image.Pt(50, 60))
	w.SetIcon(nil)
	w.SetIcon([]image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))})
	g.sent("requests", strings.Join([]string{
		"msg _NET_WM_STATE win=9 [0 _NET_WM_STATE_MAXIMIZED_VERT _NET_WM_STATE_MAXIMIZED_HORZ 1]",
		"msg _NET_WM_STATE win=9 [1 _NET_WM_STATE_FULLSCREEN 0 1]",
		"msg _NET_WM_STATE win=9 [0 _NET_WM_STATE_ABOVE 0 1]",
		"msg WM_CHANGE_STATE win=9 [3]",
		"configure win=9 x=50 y=60",
		"delete _NET_WM_ICON win=9",
		"prop _NET_WM_ICON win=9 = 1032 bytes",
	}, "; "))

	// Hiding withdraws the window; the window manager's cleanup of the
	// properties does not change the state.
	w.SetVisible(false)
	g.sent("SetVisible(false)", "unmap win=9; withdrawn win=9")
	if !w.Hidden() {
		t.Errorf("a hidden window gets frames")
	}
	g.f.unset(9, a.netWMState)
	g.expect("withdrawn", "", "", xproto.PropertyNotifyEvent{Window: 9, Atom: a.netWMState}, xproto.UnmapNotifyEvent{Event: 9, Window: 9})
	if got, want := w.State(), (platform.WindowState{Maximized: true, HasPosition: true, Position: image.Pt(30, 40)}); got != want {
		t.Errorf("State after hiding = %v, want %v", got, want)
	}

	// Showing maps it again with the wanted state and position.
	w.SetVisible(true)
	g.sent("SetVisible(true)", strings.Join([]string{
		"configure win=9 x=50 y=60",
		"prop WM_NORMAL_HINTS win=9 = pos=50,60 gravity=10",
		"prop WM_HINTS win=9 = state=1",
		"prop _NET_WM_STATE win=9 = [_NET_WM_STATE_FULLSCREEN]",
		"map win=9",
	}, "; "))
	if !w.Hidden() {
		t.Errorf("frames resumed before the window was mapped")
	}
	g.expect("mapped again", "state {fullscreen at 30,40}", "", xproto.MapNotifyEvent{Event: 9, Window: 9})
	if w.Hidden() {
		t.Errorf("a shown window gets no frames")
	}
}

// TestHiddenWindows: a Hidden window gets its ConfigureEvent but no frames,
// keeps a frame presented while hidden, and is mapped when shown (or at
// its first Present after that).
func TestHiddenWindows(t *testing.T) {
	g := newRig(t)
	h := fakeWindow(g.b, 30, image.Pt(200, 100), platform.WindowOptions{Size: platform.Size{W: 200, H: 100}, Hidden: true})
	g.f.take()
	g.expect("created hidden", "configure 200x100@1x (200x100 px)", "")
	g.present(h)
	g.sent("Present while hidden", "")
	h.SetVisible(true)
	g.sent("shown", "prop WM_NORMAL_HINTS win=30 = gravity=10; prop _NET_WM_STATE win=30 = []; map win=30")
	g.expect("shown", "frame", "")

	g = newRig(t)
	n := fakeWindow(g.b, 31, image.Pt(200, 100), platform.WindowOptions{Size: platform.Size{W: 200, H: 100}, Hidden: true})
	g.f.take()
	g.expect("created hidden", "configure 200x100@1x (200x100 px)", "")
	n.SetVisible(true)
	g.sent("shown before any Present", "")
	g.expect("shown", "frame", "")
	g.present(n)
	g.sent("first Present", "prop WM_NORMAL_HINTS win=31 = gravity=10; prop _NET_WM_STATE win=31 = []; map win=31")
}

// TestCaptionAndEdges checks the presses the OS owns on a frameless window:
// caption moves, double clicks, the window menu, resize edges with their
// cursor, and what stays app input.
func TestCaptionAndEdges(t *testing.T) {
	g := newRig(t)
	a := &g.b.atoms
	w := fakeWindow(g.b, 9, image.Pt(400, 300), platform.WindowOptions{Size: platform.Size{W: 400, H: 300}, Frameless: true})
	g.sent("NewWindow", "prop _NET_WM_WINDOW_TYPE win=9 = [_NET_WM_WINDOW_TYPE_NORMAL]; prop _MOTIF_WM_HINTS win=9 = [2 0 0 0 0]")
	g.expect("first dispatch", "configure 400x300@1x (400x300 px); state {csd}; frame", "")
	g.present(w)
	g.sent("first Present", "prop WM_NORMAL_HINTS win=9 = gravity=10; prop _NET_WM_STATE win=9 = []; map win=9")
	w.SetHitRegions([]platform.HitRegion{
		{Kind: platform.HitCaption, Rect: platform.Rect{W: 400, H: 32}},
		{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 4, W: 80, H: 24}},
		{Kind: platform.HitClose, Rect: platform.Rect{X: 360, W: 40, H: 32}},
	})
	const b1, b3 = xproto.KeyButMaskButton1, xproto.KeyButMaskButton3
	press := func(button xproto.Button, x, y int16, state uint16) xproto.ButtonPressEvent {
		return xproto.ButtonPressEvent{Event: 9, Detail: button, EventX: x, EventY: y, RootX: x + 100, RootY: y + 100, State: state}
	}
	release := func(button xproto.Button, x, y int16, state uint16) xproto.ButtonReleaseEvent {
		return xproto.ButtonReleaseEvent{Event: 9, Detail: button, EventX: x, EventY: y, RootX: x + 100, RootY: y + 100, State: state}
	}
	motion := func(x, y int16, state uint16) xproto.MotionNotifyEvent {
		return xproto.MotionNotifyEvent{Event: 9, EventX: x, EventY: y, RootX: x + 100, RootY: y + 100, State: state}
	}

	g.expect("enter", "pointer enter 50,16 btn=0 held=0 mods=0", "", xproto.EnterNotifyEvent{Event: 9, EventX: 50, EventY: 16})
	// A primary press in the caption starts a move; its sequence is the
	// OS's until the window manager grabs the pointer.
	g.expect("caption press", "", "ungrab dev=0; msg _NET_WM_MOVERESIZE win=9 [150 116 8 1 1]", press(1, 50, 16, 0))
	g.expect("motion of the move", "", "", motion(60, 20, b1))
	g.expect("window manager grab", "pointer leave 60,20 btn=0 held=0 mods=0", "",
		xproto.LeaveNotifyEvent{Event: 9, EventX: 60, EventY: 20, State: b1, Mode: xproto.NotifyModeGrab})
	g.expect("grab ends", "pointer enter 52,17 btn=0 held=0 mods=0", "",
		xproto.EnterNotifyEvent{Event: 9, EventX: 52, EventY: 17, Mode: xproto.NotifyModeUngrab})
	// A second press within the interval toggles maximize.
	g.expect("double click", "", "msg _NET_WM_STATE win=9 [1 _NET_WM_STATE_MAXIMIZED_VERT _NET_WM_STATE_MAXIMIZED_HORZ 1]",
		press(1, 52, 17, 0), release(1, 52, 17, b1))
	g.expect("hover", "pointer move 53,17 btn=0 held=0 mods=0", "", motion(53, 17, 0))
	// A secondary press opens the window menu.
	g.expect("caption menu", "", "ungrab dev=0; msg _GTK_SHOW_WINDOW_MENU win=9 [2 153 117]", press(3, 53, 17, 0))
	g.expect("menu grab", "pointer leave 53,17 btn=0 held=0 mods=0; pointer enter 53,17 btn=0 held=0 mods=0", "",
		xproto.LeaveNotifyEvent{Event: 9, EventX: 53, EventY: 17, State: b3, Mode: xproto.NotifyModeGrab},
		xproto.EnterNotifyEvent{Event: 9, EventX: 53, EventY: 17, Mode: xproto.NotifyModeUngrab})

	// A client hole, the window buttons and the app's own sequences are
	// app input.
	g.expect("client hole", "pointer down 120,16 btn=1 held=1 mods=0; pointer up 120,16 btn=1 held=0 mods=0", "",
		press(1, 120, 16, 0), release(1, 120, 16, b1))
	g.expect("close button", "pointer down 370,16 btn=1 held=1 mods=0; pointer up 370,16 btn=1 held=0 mods=0", "",
		press(1, 370, 16, 0), release(1, 370, 16, b1))
	g.expect("app sequence over the caption", strings.Join([]string{
		"pointer down 120,16 btn=1 held=1 mods=0",
		"pointer move 50,16 btn=0 held=1 mods=0",
		"pointer down 50,16 btn=2 held=11 mods=0",
		"pointer up 50,16 btn=2 held=1 mods=0",
		"pointer up 50,16 btn=1 held=0 mods=0",
	}, "; "), "", press(1, 120, 16, 0), motion(50, 16, b1), press(3, 50, 16, b1), release(3, 50, 16, b1|b3), release(1, 50, 16, b1))

	// Resize edges: the cursor follows the band, a press starts a resize.
	g.expect("into the band", "pointer move 2,150 btn=0 held=0 mods=0", "cursor win=9 w-resize", motion(2, 150, 0))
	g.expect("edge press", "", "ungrab dev=0; msg _NET_WM_MOVERESIZE win=9 [102 250 7 1 1]", press(1, 2, 150, 0), release(1, 2, 150, b1))
	g.expect("out of the band", "pointer move 200,150 btn=0 held=0 mods=0", "cursor win=9 default", motion(200, 150, 0))
	g.expect("corner", "pointer move 395,295 btn=0 held=0 mods=0", "cursor win=9 se-resize", motion(395, 295, 0))
	w.SetCursor(platform.CursorText)
	g.sent("SetCursor over the band", "")
	g.expect("app cursor", "pointer move 200,150 btn=0 held=0 mods=0", "cursor win=9 text", motion(200, 150, 0))

	// Maximized: no band.
	g.f.set(9, a.netWMState, uint32(a.netWMStateMaxVert), uint32(a.netWMStateMaxHorz))
	g.expect("maximized", "state {maximized csd}", "", xproto.PropertyNotifyEvent{Window: 9, Atom: a.netWMState})
	g.expect("edge of a maximized window", "pointer move 2,150 btn=0 held=0 mods=0; pointer down 2,150 btn=1 held=1 mods=0; pointer up 2,150 btn=1 held=0 mods=0", "",
		motion(2, 150, 0), press(1, 2, 150, 0), release(1, 2, 150, b1))

	// A window manager without the hints: the presses are still the OS's.
	delete(g.b.supported, a.netWMMoveResize)
	delete(g.b.supported, a.gtkShowWindowMenu)
	g.expect("caption without _NET_WM_MOVERESIZE", "", "", press(1, 50, 16, 0), release(1, 50, 16, b1), press(3, 50, 16, 0), release(3, 50, 16, b3))
}

// TestPositions: root coordinates from ConfigureNotify while the parent is
// the root, TranslateCoordinates once per batch when reparented, the state
// queued before the size when both change in one batch, nothing while
// hidden.
func TestPositions(t *testing.T) {
	g := newRig(t)
	a := &g.b.atoms
	w := fakeWindow(g.b, 7, image.Pt(300, 200), platform.WindowOptions{})
	g.f.take()
	g.feed()
	g.expect("unmapped window moves", "", "", xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 5, Y: 5, Width: 300, Height: 200})
	g.present(w)
	g.f.take()
	g.expect("parent is the root", "state {at 10,20}", "", xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 10, Y: 20, Width: 300, Height: 200})
	g.f.pos[7] = image.Pt(15, 45)
	g.expect("reparented", "state {at 15,45}", "", xproto.ReparentNotifyEvent{Event: 7, Window: 7, Parent: 500, X: 0, Y: 30})
	g.f.pos[7] = image.Pt(100, 200)
	// The position is read once the batch is handled, after the size was
	// queued; the loop still delivers the state before the size.
	g.expect("moved and resized in its frame", "state {at 100,200}; configure 320x200@1x (320x200 px); frame", "",
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 0, Y: 30, Width: 320, Height: 200},
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 0, Y: 30, Width: 320, Height: 200})
	g.f.pos[7] = image.Pt(0, 30)
	g.f.set(7, a.netWMState, uint32(a.netWMStateMaxVert), uint32(a.netWMStateMaxHorz))
	g.expect("maximized", "state {maximized at 0,30}; configure 1920x1050@1x (1920x1050 px); frame", "",
		xproto.PropertyNotifyEvent{Window: 7, Atom: a.netWMState},
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 0, Y: 30, Width: 1920, Height: 1050})
	// Maximized in one direction is not maximized.
	g.f.set(7, a.netWMState, uint32(a.netWMStateMaxVert))
	g.expect("half", "state {at 0,30}", "", xproto.PropertyNotifyEvent{Window: 7, Atom: a.netWMState})

	w.SetVisible(false)
	g.sent("hide", "unmap win=7; withdrawn win=7")
	g.f.pos[7] = image.Pt(1, 1)
	g.expect("hidden window moves", "", "",
		xproto.UnmapNotifyEvent{Event: 7, Window: 7},
		xproto.ReparentNotifyEvent{Event: 7, Window: 7, Parent: 1},
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 1, Y: 1, Width: 1920, Height: 1050})
}

// TestSizeLimits: SetMinSize and SetMaxSize rewrite WM_NORMAL_HINTS and ask
// for the clamped size when the window is outside the new limits.
func TestSizeLimits(t *testing.T) {
	g := newRig(t)
	w := fakeWindow(g.b, 7, image.Pt(300, 200), platform.WindowOptions{})
	g.f.take()
	w.SetMinSize(platform.Size{W: 400})
	g.sent("SetMinSize", "prop WM_NORMAL_HINTS win=7 = min=400x1 gravity=10; configure win=7 w=400 h=200")
	w.SetMaxSize(platform.Size{W: 500, H: 150})
	g.sent("SetMaxSize", "prop WM_NORMAL_HINTS win=7 = min=400x1 max=500x150 gravity=10; configure win=7 w=400 h=150")
	w.SetMaxSize(platform.Size{W: 300, H: 150}) // below the minimum: the minimum wins
	g.sent("SetMaxSize below the minimum", "prop WM_NORMAL_HINTS win=7 = min=400x1 max=400x150 gravity=10; configure win=7 w=400 h=150")
	w.SetMinSize(platform.Size{})
	w.SetMaxSize(platform.Size{})
	g.sent("no limits", "prop WM_NORMAL_HINTS win=7 = max=300x150 gravity=10; configure win=7 w=300 h=150; prop WM_NORMAL_HINTS win=7 = gravity=10")
}

// TestParentsAndModality: WM_TRANSIENT_FOR, the dialog type and
// _NET_WM_STATE_MODAL of a modal child, and children closed first.
func TestParentsAndModality(t *testing.T) {
	g := newRig(t)
	p := fakeWindow(g.b, 20, image.Pt(300, 200), platform.WindowOptions{})
	m := fakeWindow(g.b, 21, image.Pt(200, 100), platform.WindowOptions{Size: platform.Size{W: 200, H: 100}, Parent: p, Modal: true})
	c := fakeWindow(g.b, 22, image.Pt(200, 100), platform.WindowOptions{Size: platform.Size{W: 200, H: 100}, Parent: p})
	g.sent("NewWindow", strings.Join([]string{
		"prop _NET_WM_WINDOW_TYPE win=20 = [_NET_WM_WINDOW_TYPE_NORMAL]",
		"prop _NET_WM_WINDOW_TYPE win=21 = [_NET_WM_WINDOW_TYPE_DIALOG]",
		"prop WM_TRANSIENT_FOR win=21 = [20]",
		"prop _NET_WM_WINDOW_TYPE win=22 = [_NET_WM_WINDOW_TYPE_NORMAL]",
		"prop WM_TRANSIENT_FOR win=22 = [20]",
	}, "; "))
	g.feed()
	g.present(m)
	g.sent("modal mapped", "prop WM_NORMAL_HINTS win=21 = gravity=10; prop _NET_WM_STATE win=21 = [_NET_WM_STATE_MODAL]; map win=21")

	p.Close()
	g.sent("Close", "destroy win=21; destroy win=22; destroy win=20")
	if !m.Closed() || !c.Closed() {
		t.Errorf("children not closed with their parent")
	}
	if len(g.b.loop.Windows()) != 0 || len(g.b.windows) != 0 {
		t.Errorf("closed windows are still known")
	}

	expectPanic := func(what string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", what)
			}
		}()
		f()
	}
	expectPanic("a closed parent", func() { fakeWindow(g.b, 23, image.Pt(10, 10), platform.WindowOptions{Parent: p}) })
	other := fakeEmpty(1)
	o := fakeWindow(other, 24, image.Pt(10, 10), platform.WindowOptions{})
	expectPanic("a parent of another backend", func() { fakeWindow(g.b, 25, image.Pt(10, 10), platform.WindowOptions{Parent: o}) })
}
