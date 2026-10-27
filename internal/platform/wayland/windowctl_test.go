//go:build linux

package wayland

import (
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// ctlGlobals is standardGlobals(1) plus the named optional globals
// (version 1).
func ctlGlobals(extra ...string) []fakeGlobal {
	g := standardGlobals(1)
	for _, iface := range extra {
		g = append(g, fakeGlobal{iface: iface, version: 1})
	}
	return g
}

// presentFrames makes the harness present a solid frame at every
// FrameEvent.
func presentFrames(h *harness) {
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) {
		if err := w.Present(solid(w, 0x10, 0x20, 0x30), nil); err != nil {
			h.t.Errorf("Present: %v", err)
		}
	}
}

// openCtl creates a window, starts the loop and answers the initial commit
// with a configure. It returns the window and the requests up to the
// initial commit.
func openCtl(t *testing.T, h *harness, opts platform.WindowOptions, width, height int32, serial uint32, states ...protocols.XdgToplevelState) (*Window, []freq) {
	t.Helper()
	win, err := h.b.NewWindow(opts)
	if err != nil {
		t.Fatal(err)
	}
	h.start()
	setup := h.fc.until("wl_surface.commit")
	h.configure(width, height, serial, states...)
	return win.(*Window), setup
}

// configureFor encodes a configure sequence for the given toplevel and
// xdg_surface.
func configureFor(top, xs uint32, width, height int32, serial uint32, states ...protocols.XdgToplevelState) []byte {
	var arr []byte
	for _, s := range states {
		arr = ne.AppendUint32(arr, uint32(s))
	}
	return append(event(top, 0, width, height, arr), event(xs, 0, serial)...)
}

// last returns the last request called name, and its index.
func last(rs []freq, name string) (freq, int) {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i].is(name) {
			return rs[i], i
		}
	}
	return freq{}, -1
}

// ids returns the xdg_surface and xdg_toplevel created in setup.
func ids(t *testing.T, setup []freq) (xs, top uint32) {
	t.Helper()
	x, i := find(setup, "xdg_wm_base.get_xdg_surface")
	tl, j := find(setup, "xdg_surface.get_toplevel")
	if i < 0 || j < 0 {
		t.Fatalf("no toplevel in %v", names(setup))
	}
	return x.u(0), tl.u(0)
}

func expectState(t *testing.T, h *harness, want platform.WindowState) {
	t.Helper()
	if se := expect[platform.WindowStateEvent](h); se.State != want {
		t.Errorf("WindowStateEvent = %v, want %v", se.State, want)
	}
}

func TestResizeEdge(t *testing.T) {
	size := image.Pt(300, 200)
	const (
		top    = protocols.XdgToplevelResizeEdgeTop
		bottom = protocols.XdgToplevelResizeEdgeBottom
		left   = protocols.XdgToplevelResizeEdgeLeft
		right  = protocols.XdgToplevelResizeEdgeRight
	)
	for _, c := range []struct {
		x, y  float32
		tiled protocols.XdgToplevelResizeEdge
		want  protocols.XdgToplevelResizeEdge
	}{
		{150, 100, 0, 0},              // middle
		{150, 4.9, 0, top},            // top band
		{150, 5, 0, 0},                // just below it
		{150, 195, 0, bottom},         // bottom band starts at h-5
		{0, 100, 0, left},             // left
		{299.5, 100, 0, right},        // right
		{2, 2, 0, top | left},         // corner
		{10, 2, 0, top | left},        // corner along the top edge
		{2, 10, 0, top | left},        // corner along the left edge
		{20, 2, 0, top},               // past the corner
		{298, 190, 0, bottom | right}, // bottom-right
		{2, 2, left, top},             // tiled left: no left edge
		{2, 100, left | top, 0},       // tiled: nothing
		{-1, 100, 0, 0},               // outside
		{300, 100, 0, 0},              // outside (exclusive)
	} {
		if got := resizeEdge(platform.Point{X: c.x, Y: c.y}, size, c.tiled); got != c.want {
			t.Errorf("resizeEdge(%v,%v, tiled %v) = %v, want %v", c.x, c.y, c.tiled, got, c.want)
		}
	}
	// A window narrower than two bands: the nearer side wins.
	if got := resizeEdge(platform.Point{X: 2, Y: 50}, image.Pt(6, 100), 0); got != left {
		t.Errorf("narrow window, left half = %v, want left", got)
	}
	if got := resizeEdge(platform.Point{X: 4, Y: 50}, image.Pt(6, 100), 0); got != right {
		t.Errorf("narrow window, right half = %v, want right", got)
	}
}

func TestSizeLimitsAndHints(t *testing.T) {
	pt := func(x, y int) image.Point { return image.Point{X: x, Y: y} }
	c := sizeChoice{min: pt(200, 100), max: pt(150, 400)}
	if got := c.limit(pt(1000, 50)); got != pt(200, 100) {
		t.Errorf("limit = %v, want the minimum to win", got)
	}
	if got := c.limit(pt(180, 500)); got != pt(200, 400) {
		t.Errorf("limit = %v", got)
	}
	if min, max := c.hints(); min != pt(200, 100) || max != pt(200, 400) {
		t.Errorf("hints = %v %v, want the maximum raised to the minimum", min, max)
	}
	fixed := sizeChoice{stateless: pt(300, 200), min: pt(10, 10), fixed: true}
	if min, max := fixed.hints(); min != pt(300, 200) || max != pt(300, 200) {
		t.Errorf("FixedSize hints = %v %v", min, max)
	}
	if got := floorPoint(platform.Size{W: 120.9, H: 0.3}); got != pt(120, 1) {
		t.Errorf("floorPoint = %v", got)
	}
}

func TestRestoreTokenParse(t *testing.T) {
	name := "0123456789abcdef0123456789abcdef"
	w := &Window{b: &Backend{session: session{id: "session/1:x"}}, ctl: windowCtl{sessionName: name}}
	tok := w.RestoreToken()
	if !strings.HasPrefix(tok, restoreTokenPrefix) {
		t.Fatalf("RestoreToken = %q", tok)
	}
	if id, n, ok := parseRestoreToken(tok); !ok || id != "session/1:x" || n != name {
		t.Errorf("parseRestoreToken(%q) = %q, %q, %v", tok, id, n, ok)
	}
	for _, bad := range []string{
		"",
		"headless:1",
		restoreTokenPrefix + "c2lk", // no name
		restoreTokenPrefix + "c2lk:" + strings.ToUpper(name),          // not our name form
		restoreTokenPrefix + "c2lk:" + name[:31],                      // short name
		restoreTokenPrefix + "%%%:" + name,                            // bad base64
		restoreTokenPrefix + ":" + name,                               // empty id
		restoreTokenPrefix + "AA:" + name,                             // NUL id
		restoreTokenPrefix + "_w:" + name,                             // invalid UTF-8 (0xff)
		restoreTokenPrefix + strings.Repeat("QUFB", 400) + ":" + name, // too long
	} {
		if _, _, ok := parseRestoreToken(bad); ok {
			t.Errorf("parseRestoreToken(%q) accepted", bad)
		}
	}
	if n := newSessionName(); !validSessionName(n) || n == newSessionName() {
		t.Errorf("newSessionName = %q", n)
	}
	w.b.session.gone = true
	if tok := w.RestoreToken(); tok != "" {
		t.Errorf("RestoreToken after replaced = %q", tok)
	}
}

func TestWindowStateRequests(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, setup := openCtl(t, h, platform.WindowOptions{Frameless: true, Size: platform.Size{W: 400, H: 300}}, 400, 300, 1, protocols.XdgToplevelStateActivated)
	if r, i := find(setup, "zxdg_toplevel_decoration_v1.set_mode"); i < 0 || r.u(0) != uint32(protocols.ZxdgToplevelDecorationV1ModeClientSide) {
		t.Errorf("a frameless window asked for decoration mode %v, want client side", r)
	}
	csd := platform.WindowState{ClientSideDecorations: true}
	// The first ConfigureEvent comes first, then the state.
	expect[platform.ConfigureEvent](h)
	expectState(t, h, csd)
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")

	// Requests equal to the last one are free.
	h.ui(func() {
		w.SetMaximized(true)
		w.SetMaximized(true)
	})
	h.fc.until("xdg_toplevel.set_maximized")
	h.fc.quiet(50*time.Millisecond, "xdg_toplevel.set_maximized")

	// The compositor maximizes: the state precedes the size.
	h.configure(1920, 1080, 2, protocols.XdgToplevelStateMaximized, protocols.XdgToplevelStateActivated)
	expectState(t, h, platform.WindowState{Maximized: true, ClientSideDecorations: true})
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 1920, H: 1080}) {
		t.Errorf("maximized ConfigureEvent = %v", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	h.ui(func() {
		if st := w.State(); st != (platform.WindowState{Maximized: true, ClientSideDecorations: true}) {
			t.Errorf("State() = %v", st)
		}
	})

	h.ui(func() {
		w.SetMaximized(false)
		w.SetFullscreen(true)
		w.SetAlwaysOnTop(true)          // no protocol: nothing
		w.SetPosition(image.Pt(10, 20)) // clients cannot place windows
		w.Minimize()
	})
	reqs := h.fc.until("xdg_toplevel.set_minimized")
	_, iUnmax := find(reqs, "xdg_toplevel.unset_maximized")
	full, iFull := find(reqs, "xdg_toplevel.set_fullscreen")
	if iUnmax < 0 || iFull < iUnmax || full.u(0) != 0 {
		t.Errorf("requests %v: want unset_maximized, then set_fullscreen with no output", names(reqs))
	}

	// Back to floating with 0x0: the size before maximizing.
	h.configure(0, 0, 3, protocols.XdgToplevelStateActivated)
	expectState(t, h, csd)
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 400, H: 300}) {
		t.Errorf("unmaximized ConfigureEvent = %v, want the size before", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	// The configure reset what was asked: maximizing asks again.
	h.ui(func() { w.SetMaximized(true) })
	h.fc.until("xdg_toplevel.set_maximized")
}

func TestDecorationModes(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	openWindow(t, h, platform.WindowOptions{}, 200, 100, 1, protocols.XdgToplevelStateActivated)
	// Server-side (or not yet known): the zero state, no event.
	expect[platform.ConfigureEvent](h)
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")

	// The compositor switches to client-side: the mode is latched and
	// applied with the configure that ends the sequence.
	deco := h.fc.id("zxdg_toplevel_decoration_v1")
	h.fc.sendRaw(append(event(deco, 0, uint32(protocols.ZxdgToplevelDecorationV1ModeClientSide)),
		h.configureMsg(200, 100, 2, protocols.XdgToplevelStateActivated)...))
	expectState(t, h, platform.WindowState{ClientSideDecorations: true})
	if r := h.fc.until("wl_surface.commit"); r[0].u(0) != 2 {
		t.Errorf("same-size configure answered with %v", r)
	}
	h.fc.sendRaw(append(event(deco, 0, uint32(protocols.ZxdgToplevelDecorationV1ModeServerSide)),
		h.configureMsg(200, 100, 3, protocols.XdgToplevelStateActivated)...))
	expectState(t, h, platform.WindowState{})
}

func TestNoDecorationManagerIsClientSide(t *testing.T) {
	g := slices.DeleteFunc(standardGlobals(1), func(x fakeGlobal) bool { return x.iface == "zxdg_decoration_manager_v1" })
	h := newHarness(t, g)
	_, setup := openCtl(t, h, platform.WindowOptions{}, 200, 100, 1)
	if _, i := find(setup, "zxdg_decoration_manager_v1.get_toplevel_decoration"); i >= 0 {
		t.Errorf("decoration requested without a manager: %v", names(setup))
	}
	expect[platform.ConfigureEvent](h)
	expectState(t, h, platform.WindowState{ClientSideDecorations: true})
}

func TestSizeLimits(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, setup := openCtl(t, h, platform.WindowOptions{
		Size:    platform.Size{W: 300, H: 200},
		MinSize: platform.Size{W: 200, H: 100},
		MaxSize: platform.Size{W: 150, H: 400},
	}, 0, 0, 1)
	if r, _ := find(setup, "xdg_toplevel.set_min_size"); r.i(0) != 200 || r.i(1) != 100 {
		t.Errorf("set_min_size = %v", r)
	}
	if r, _ := find(setup, "xdg_toplevel.set_max_size"); r.i(0) != 200 || r.i(1) != 400 {
		t.Errorf("set_max_size = %v, want the maximum raised to the minimum", r)
	}
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 200, H: 200}) {
		t.Errorf("ConfigureEvent = %v, want the size within the limits", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")

	// New limits apply at once (the hints are double-buffered), and a
	// floating window outside them picks the clamped size.
	h.ui(func() {
		w.SetMinSize(platform.Size{W: 50, H: 50})
		w.SetMaxSize(platform.Size{W: 120, H: 90})
		w.SetMaxSize(platform.Size{W: 120, H: 90}) // unchanged: nothing sent
	})
	reqs := h.fc.until("wl_surface.commit")
	reqs = append(reqs, h.fc.until("wl_surface.commit")...)
	if r, _ := last(reqs, "xdg_toplevel.set_min_size"); r.i(0) != 50 || r.i(1) != 50 {
		t.Errorf("set_min_size = %v", r)
	}
	if r, _ := last(reqs, "xdg_toplevel.set_max_size"); r.i(0) != 120 || r.i(1) != 90 {
		t.Errorf("set_max_size = %v", r)
	}
	// The lower minimum frees the window from the old maximum (150, which
	// the minimum overrode), then the new maximum shrinks it.
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 150, H: 200}) {
		t.Errorf("ConfigureEvent = %v, want the old maximum", ce.Metrics)
	}
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 120, H: 90}) {
		t.Errorf("ConfigureEvent = %v, want the new maximum", ce.Metrics)
	}
	expect[platform.FrameEvent](h)

	// A floating suggestion above the maximum is clamped: nothing changes.
	h.configure(1000, 1000, 2)
	if r := h.fc.until("xdg_surface.ack_configure"); r[len(r)-1].u(0) != 2 {
		t.Errorf("ack = %v", r[len(r)-1])
	}
	h.noEvent(50 * time.Millisecond)
	h.ui(func() {
		if m := w.Metrics(); m.Size != (platform.Size{W: 120, H: 90}) {
			t.Errorf("Metrics = %v", m)
		}
	})
}

func TestHideShowRemapsWithTheRetainedFrame(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{Title: "hide me", MinSize: platform.Size{W: 100, H: 50}}, 300, 200, 1, protocols.XdgToplevelStateActivated)
	expect[platform.ConfigureEvent](h)
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	attach, _ := find(h.fc.until("wl_surface.commit"), "wl_surface.attach")
	bufID := attach.u(0)

	// Hiding commits a null buffer, once.
	h.ui(func() {
		w.SetVisible(false)
		w.SetVisible(false)
	})
	reqs := h.fc.until("wl_surface.commit")
	if !slices.Equal(names(reqs), []string{"wl_surface.attach", "wl_surface.commit"}) || reqs[0].u(0) != 0 {
		t.Errorf("hide = %v, want attach(NULL) + commit", reqs)
	}
	if fe := expect[platform.FocusEvent](h); fe.Focused {
		t.Errorf("FocusEvent = %+v after hiding", fe)
	}
	h.fc.send(bufID, 0) // wl_buffer.release

	// Hidden: no frames, a Present shows nothing, a new title waits.
	h.ui(func() {
		w.RequestFrame()
		if err := w.Present(solid(w, 1, 1, 1), nil); err != nil {
			t.Errorf("Present while hidden: %v", err)
		}
		w.SetTitle("still hidden")
		w.SetMinSize(platform.Size{W: 120, H: 60})
	})
	h.noEvent(50 * time.Millisecond)
	for _, r := range h.fc.quiet(50*time.Millisecond, "wl_surface.attach") {
		if r.is("wl_surface.commit") || r.is("xdg_toplevel.set_title") {
			t.Errorf("hidden window sent %v", r)
		}
	}

	// Shown again: the toplevel's attributes and a new initial commit,
	// without a buffer.
	h.ui(func() { w.SetVisible(true) })
	reqs = h.fc.until("wl_surface.commit")
	for _, want := range []string{"xdg_toplevel.set_title", "xdg_toplevel.set_app_id", "xdg_toplevel.set_min_size",
		"xdg_toplevel.set_max_size", "zxdg_toplevel_decoration_v1.set_mode"} {
		if _, i := find(reqs, want); i < 0 {
			t.Errorf("show %v lacks %s", names(reqs), want)
		}
	}
	if r, _ := find(reqs, "xdg_toplevel.set_title"); r.name != "" && r.args[0] != "still hidden" {
		t.Errorf("set_title = %v", r)
	}
	if r, _ := find(reqs, "xdg_toplevel.set_min_size"); r.name != "" && (r.i(0) != 120 || r.i(1) != 60) {
		t.Errorf("set_min_size = %v", r)
	}
	if _, i := find(reqs, "wl_surface.attach"); i >= 0 {
		t.Errorf("a buffer before the configure: %v", names(reqs))
	}
	h.noEvent(50 * time.Millisecond) // frames wait for the configure

	// Its configure maps the window with the retained frame, then the
	// frame requested while hidden follows the frame callback.
	h.configure(0, 0, 2, protocols.XdgToplevelStateActivated)
	reqs = h.fc.until("wl_surface.commit")
	ack, iAck := find(reqs, "xdg_surface.ack_configure")
	att, iAtt := find(reqs, "wl_surface.attach")
	if iAck < 0 || iAtt < iAck || ack.u(0) != 2 || att.u(0) != bufID {
		t.Errorf("remap = %v, want ack_configure(2) then the retained buffer %d", reqs, bufID)
	}
	if fe := expect[platform.FocusEvent](h); !fe.Focused {
		t.Errorf("FocusEvent = %+v after showing", fe)
	}
	frame, _ := find(reqs, "wl_surface.frame")
	h.noEvent(30 * time.Millisecond)
	h.fc.send(frame.u(0), 0, uint32(1))
	expect[platform.FrameEvent](h)
}

func TestCreatedHiddenGetsConfigureNotFrames(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{Hidden: true}, 200, 100, 1)
	expect[platform.ConfigureEvent](h)
	h.noEvent(50 * time.Millisecond)
	h.fc.quiet(30*time.Millisecond, "wl_surface.attach")
	h.ui(func() { w.SetVisible(true) })
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	_, iAck := find(reqs, "xdg_surface.ack_configure")
	_, iAtt := find(reqs, "wl_surface.attach")
	if iAck < 0 || iAtt < iAck {
		t.Errorf("first Present after showing = %v", names(reqs))
	}
}

func TestParentModalAndClose(t *testing.T) {
	h := newHarness(t, ctlGlobals("xdg_wm_dialog_v1"))
	presentFrames(h)
	pw, err := h.b.NewWindow(platform.WindowOptions{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	cw, err := h.b.NewWindow(platform.WindowOptions{Title: "dialog", Parent: pw, Modal: true})
	if err != nil {
		t.Fatal(err)
	}
	h.start()
	pXs, pTop := ids(t, h.fc.until("wl_surface.commit"))
	csetup := h.fc.until("wl_surface.commit")
	cXs, cTop := ids(t, csetup)
	// The parent is not mapped yet: set_parent would mean no parent.
	if _, i := find(csetup, "xdg_toplevel.set_parent"); i >= 0 {
		t.Errorf("set_parent with an unmapped parent: %v", names(csetup))
	}
	if d, i := find(csetup, "xdg_wm_dialog_v1.get_xdg_dialog"); i < 0 || d.u(1) != cTop {
		t.Errorf("modal window setup %v lacks get_xdg_dialog", names(csetup))
	}
	if _, i := find(csetup, "xdg_dialog_v1.set_modal"); i < 0 {
		t.Errorf("modal window setup %v lacks set_modal", names(csetup))
	}

	// The parent maps: the child gets its parent.
	h.fc.sendRaw(configureFor(pTop, pXs, 300, 200, 1))
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	if r := h.fc.until("xdg_toplevel.set_parent"); r[len(r)-1].id != cTop || r[len(r)-1].u(0) != pTop {
		t.Errorf("set_parent = %v, want the child's parent", r[len(r)-1])
	}
	h.fc.sendRaw(configureFor(cTop, cXs, 200, 100, 2))
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")

	// Closing the parent closes the child first.
	h.ui(func() { pw.Close() })
	var reqs []freq
	for n := 0; n < 2; {
		r := h.fc.next()
		reqs = append(reqs, r)
		if r.is("wl_surface.destroy") {
			n++
		}
	}
	topDestroy := func(id uint32) int {
		for i, r := range reqs {
			if r.is("xdg_toplevel.destroy") && r.id == id {
				return i
			}
		}
		return -1
	}
	_, iDialog := find(reqs, "xdg_dialog_v1.destroy")
	iChild, iParent := topDestroy(cTop), topDestroy(pTop)
	if iDialog < 0 || iChild < iDialog || iParent < iChild {
		t.Errorf("close = %v: want the dialog object, then the child, then the parent", names(reqs))
	}
	h.ui(func() {
		if !cw.(*Window).Closed() {
			t.Errorf("the child is open after its parent closed")
		}
		defer func() {
			if recover() == nil {
				t.Errorf("NewWindow with a closed parent did not panic")
			}
		}()
		h.b.NewWindow(platform.WindowOptions{Parent: pw})
	})
	h.noEvent(50 * time.Millisecond)
}

func TestActivate(t *testing.T) {
	h := newHarness(t, ctlGlobals("xdg_activation_v1"))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{AppID: "org.example.app"}, 200, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")

	ptr, surface, seat := h.fc.id("wl_pointer"), h.fc.id("wl_surface"), h.fc.id("wl_seat")
	h.fc.send(ptr, 0, uint32(55), surface, 1.0, 1.0) // enter
	h.fc.send(ptr, 5)
	expect[platform.PointerEvent](h)

	h.ui(func() { w.Activate() })
	reqs := h.fc.until("xdg_activation_token_v1.commit")
	if r, i := find(reqs, "xdg_activation_token_v1.set_serial"); i < 0 || r.u(0) != 55 || r.u(1) != seat {
		t.Errorf("set_serial = %v, want the latest serial (55) and its seat", r)
	}
	if r, i := find(reqs, "xdg_activation_token_v1.set_surface"); i < 0 || r.u(0) != surface {
		t.Errorf("set_surface = %v", r)
	}
	if r, i := find(reqs, "xdg_activation_token_v1.set_app_id"); i < 0 || r.args[0] != "org.example.app" {
		t.Errorf("set_app_id = %v", r)
	}
	h.fc.send(h.fc.id("xdg_activation_token_v1"), 0, "the-token") // done
	reqs = h.fc.until("xdg_activation_v1.activate")
	act := reqs[len(reqs)-1]
	if act.args[0] != "the-token" || act.u(1) != surface {
		t.Errorf("activate = %v", act)
	}
	if _, i := find(reqs, "xdg_activation_token_v1.destroy"); i < 0 {
		t.Errorf("the token object was not destroyed: %v", names(reqs))
	}

	// A hidden window is not activated.
	h.ui(func() {
		w.SetVisible(false)
		w.Activate()
	})
	h.fc.quiet(50*time.Millisecond, "xdg_activation_v1.get_activation_token")
}

func TestIcon(t *testing.T) {
	h := newHarness(t, ctlGlobals("xdg_toplevel_icon_manager_v1"))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{}, 200, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	top := h.fc.id("xdg_toplevel")

	small := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := range small.Pix {
		small.Pix[i] = []byte{255, 0, 0, 128}[i%4] // half-transparent red
	}
	wide := image.NewRGBA(image.Rect(10, 10, 42, 34)) // 32x24, offset bounds
	for i := range wide.Pix {
		wide.Pix[i] = []byte{0, 0, 255, 255}[i%4]
	}
	h.ui(func() { w.SetIcon([]image.Image{small, wide}) })
	reqs := h.fc.until("wl_surface.commit")
	icon, iCreate := find(reqs, "xdg_toplevel_icon_manager_v1.create_icon")
	set, iSet := find(reqs, "xdg_toplevel_icon_manager_v1.set_icon")
	_, iDestroy := find(reqs, "xdg_toplevel_icon_v1.destroy")
	_, iBufDestroy := last(reqs, "wl_buffer.destroy")
	if iCreate < 0 || iSet < iCreate || iDestroy < iSet || iBufDestroy < iDestroy {
		t.Fatalf("SetIcon = %v: want create_icon, add_buffer…, set_icon, then the icon and its buffers destroyed", names(reqs))
	}
	if set.u(0) != top || set.u(1) != icon.u(0) {
		t.Errorf("set_icon = %v", set)
	}
	var sides []int32
	for _, r := range reqs {
		if !r.is("xdg_toplevel_icon_v1.add_buffer") {
			continue
		}
		if r.i(1) != 1 {
			t.Errorf("add_buffer scale = %d, want 1", r.i(1))
		}
		b := h.fc.buffer(r.u(0))
		if b.w != b.h || b.format != uint32(protocols.WlShmFormatArgb8888) {
			t.Errorf("icon buffer %+v: want square ARGB8888", b)
		}
		sides = append(sides, b.w)
		px := h.fc.pixels(r.u(0))
		switch b.w {
		case 16: // premultiplied BGRA
			if !slices.Equal(px[:4], []byte{0, 0, 128, 128}) {
				t.Errorf("16px icon pixel = % x, want 00 00 80 80", px[:4])
			}
		case 32: // centered: 4 transparent rows on top
			if !slices.Equal(px[:4], []byte{0, 0, 0, 0}) {
				t.Errorf("32px icon top pixel = % x, want transparent", px[:4])
			}
			if o := 4 * 32 * 4; !slices.Equal(px[o:o+4], []byte{255, 0, 0, 255}) {
				t.Errorf("32px icon pixel (0,4) = % x, want ff 00 00 ff (blue)", px[o:o+4])
			}
		}
	}
	if !slices.Equal(sides, []int32{16, 32}) {
		t.Errorf("icon sizes = %v, want [16 32]", sides)
	}

	// nil resets the icon.
	h.ui(func() { w.SetIcon(nil) })
	reqs = h.fc.until("xdg_toplevel_icon_manager_v1.set_icon")
	if r := reqs[len(reqs)-1]; r.u(0) != top || r.u(1) != 0 {
		t.Errorf("reset = %v, want set_icon(toplevel, NULL)", r)
	}
}

func TestResizeEdgesAndCaption(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{Frameless: true}, 300, 200, 1, protocols.XdgToplevelStateActivated)
	expect[platform.ConfigureEvent](h)
	expectState(t, h, platform.WindowState{ClientSideDecorations: true})
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	h.ui(func() {
		w.SetHitRegions([]platform.HitRegion{
			{Kind: platform.HitCaption, Rect: platform.Rect{W: 300, H: 30}},
			{Kind: platform.HitClose, Rect: platform.Rect{X: 270, W: 30, H: 30}},
		})
	})

	ptr, surface, seat := h.fc.id("wl_pointer"), h.fc.id("wl_surface"), h.fc.id("wl_seat")
	shape := func(want platform.Cursor) {
		t.Helper()
		r := h.fc.until("wp_cursor_shape_device_v1.set_shape")
		if got := r[len(r)-1].u(1); got != uint32(cursorShape(want)) {
			t.Errorf("cursor shape %d, want %d (%v)", got, cursorShape(want), want)
		}
	}
	move := func(x, y float64) {
		h.fc.send(ptr, 2, uint32(0), x, y) // motion
		h.fc.send(ptr, 5)                  // frame
	}
	button := func(serial, code uint32, pressed bool) {
		state := uint32(0)
		if pressed {
			state = 1
		}
		h.fc.send(ptr, 3, serial, uint32(0), code, state)
		h.fc.send(ptr, 5)
	}

	// Entering on the right edge shows the resize cursor.
	h.fc.send(ptr, 0, uint32(10), surface, 298.0, 100.0)
	h.fc.send(ptr, 5)
	expect[platform.PointerEvent](h)
	shape(platform.CursorEResize)
	// The app's cursor does not replace it there.
	h.ui(func() { w.SetCursor(platform.CursorText) })
	h.fc.quiet(30*time.Millisecond, "wp_cursor_shape_device_v1.set_shape")

	// A press starts the compositor's resize; nothing of the sequence is
	// delivered.
	button(11, btnLeft, true)
	if r := h.fc.until("xdg_toplevel.resize"); r[len(r)-1].u(0) != seat || r[len(r)-1].u(1) != 11 ||
		r[len(r)-1].u(2) != uint32(protocols.XdgToplevelResizeEdgeRight) {
		t.Errorf("resize = %v", r[len(r)-1])
	}
	move(250, 100)
	button(12, btnLeft, false)
	h.noEvent(50 * time.Millisecond)

	// Off the edge: moves are delivered and the app's cursor is back.
	move(150, 100)
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerMove {
		t.Errorf("event = %+v, want a move", pe)
	}
	shape(platform.CursorText)

	// Caption: hover is delivered; a press is not, and becomes a move
	// past the drag threshold, with the press serial.
	move(100, 12)
	expect[platform.PointerEvent](h)
	button(20, btnLeft, true)
	move(103, 12)
	h.fc.quiet(30*time.Millisecond, "xdg_toplevel.move")
	move(120, 12)
	if r := h.fc.until("xdg_toplevel.move"); r[len(r)-1].u(0) != seat || r[len(r)-1].u(1) != 20 {
		t.Errorf("move = %v, want the press serial 20", r[len(r)-1])
	}
	button(21, btnLeft, false)
	h.noEvent(50 * time.Millisecond)

	// A double click toggles maximize and starts no move.
	button(30, btnLeft, true)
	button(31, btnLeft, false)
	button(32, btnLeft, true)
	button(33, btnLeft, false)
	reqs := h.fc.until("xdg_toplevel.set_maximized")
	if _, i := find(reqs, "xdg_toplevel.move"); i >= 0 {
		t.Errorf("a double click started a move: %v", names(reqs))
	}
	h.noEvent(50 * time.Millisecond)

	// A secondary click opens the window menu at the pointer.
	button(40, btnRight, true)
	if r := h.fc.until("xdg_toplevel.show_window_menu"); r[len(r)-1].u(1) != 40 || r[len(r)-1].i(2) != 120 || r[len(r)-1].i(3) != 12 {
		t.Errorf("show_window_menu = %v", r[len(r)-1])
	}
	button(41, btnRight, false)
	h.noEvent(50 * time.Millisecond)

	// The close button is the app's.
	move(285, 12)
	expect[platform.PointerEvent](h)
	button(50, btnLeft, true)
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerDown {
		t.Errorf("press on the close button = %+v, want delivered", pe)
	}
	button(51, btnLeft, false)
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerUp {
		t.Errorf("release on the close button = %+v", pe)
	}

	// Maximized: no resize edges.
	h.configure(1920, 1080, 2, protocols.XdgToplevelStateMaximized, protocols.XdgToplevelStateActivated)
	expectState(t, h, platform.WindowState{Maximized: true, ClientSideDecorations: true})
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	move(1918, 500)
	expect[platform.PointerEvent](h)
	button(60, btnLeft, true)
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerDown {
		t.Errorf("press at the edge of a maximized window = %+v, want delivered", pe)
	}
}

func TestSessionRestore(t *testing.T) {
	// First run: a new session, the window added under a fresh name.
	h := newHarness(t, ctlGlobals("xdg_session_manager_v1"))
	win, err := h.b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	h.start()
	setup := h.fc.until("wl_surface.commit")
	gs, iGet := find(setup, "xdg_session_manager_v1.get_session")
	add, iAdd := find(setup, "xdg_session_v1.add_toplevel")
	if iGet < 0 || iAdd < iGet || gs.u(1) != uint32(protocols.XdgSessionManagerV1ReasonLaunch) || gs.args[2] != "" {
		t.Fatalf("setup %v: want get_session(launch, NULL), then add_toplevel", setup)
	}
	name := add.args[2].(string)
	if add.u(1) != h.fc.id("xdg_toplevel") || !validSessionName(name) {
		t.Errorf("add_toplevel = %v", add)
	}
	h.ui(func() {
		if tok := w.RestoreToken(); tok != "" {
			t.Errorf("RestoreToken before created = %q", tok)
		}
	})
	sess := h.fc.id("xdg_session_v1")
	h.fc.send(sess, 0, "sid-1") // created, dispatched before the configure
	h.configure(200, 100, 1)
	expect[platform.ConfigureEvent](h)
	var tok string
	h.ui(func() { tok = w.RestoreToken() })
	if id, n, ok := parseRestoreToken(tok); !ok || id != "sid-1" || n != name {
		t.Fatalf("RestoreToken = %q (%q, %q)", tok, id, n)
	}
	// Two windows never share a name: a second window with the token
	// gets a fresh one.
	h.ui(func() {
		if _, err := h.b.NewWindow(platform.WindowOptions{RestoreToken: tok}); err != nil {
			t.Error(err)
		}
	})
	reqs := h.fc.until("wl_surface.commit")
	if r, i := find(reqs, "xdg_session_v1.add_toplevel"); i < 0 || r.args[2] == name {
		t.Errorf("second window with the same token: %v", reqs)
	}
	if _, i := find(reqs, "xdg_session_manager_v1.get_session"); i >= 0 {
		t.Errorf("a second session: %v", names(reqs))
	}

	// Second run: the token reopens the session and restores the window
	// before its initial commit.
	h2 := newHarness(t, ctlGlobals("xdg_session_manager_v1"))
	win2, err := h2.b.NewWindow(platform.WindowOptions{RestoreToken: tok})
	if err != nil {
		t.Fatal(err)
	}
	w2 := win2.(*Window)
	h2.start()
	setup = h2.fc.until("wl_surface.commit")
	gs, iGet = find(setup, "xdg_session_manager_v1.get_session")
	rt, iRt := find(setup, "xdg_session_v1.restore_toplevel")
	if iGet < 0 || iRt < iGet || gs.args[2] != "sid-1" || rt.args[2] != name {
		t.Fatalf("setup %v: want get_session(launch, sid-1), then restore_toplevel(%s)", setup, name)
	}
	sess = h2.fc.id("xdg_session_v1")
	h2.fc.send(sess, 1) // restored
	// The restored size arrives with the first configure.
	h2.configure(640, 480, 1)
	if ce := expect[platform.ConfigureEvent](h2); ce.Size != (platform.Size{W: 640, H: 480}) {
		t.Errorf("restored ConfigureEvent = %v", ce.Metrics)
	}
	h2.ui(func() {
		if got := w2.RestoreToken(); got != tok {
			t.Errorf("RestoreToken after restored = %q, want %q", got, tok)
		}
	})
	// Garbage and other backends' tokens are ignored.
	h2.ui(func() {
		for _, bad := range []string{"headless:3", restoreTokenPrefix + "%%%:" + name} {
			if _, err := h2.b.NewWindow(platform.WindowOptions{RestoreToken: bad}); err != nil {
				t.Error(err)
			}
		}
	})
	for range 2 {
		reqs = h2.fc.until("wl_surface.commit")
		if _, i := find(reqs, "xdg_session_v1.add_toplevel"); i < 0 {
			t.Errorf("window with an invalid token: %v", names(reqs))
		}
	}

	// Another client takes the session over: no more tokens.
	h2.fc.send(sess, 2) // replaced
	h2.fc.until("xdg_session_v1.destroy")
	h2.ui(func() {
		if got := w2.RestoreToken(); got != "" {
			t.Errorf("RestoreToken after replaced = %q", got)
		}
	})
}
