//go:build linux

package wayland

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// answerForeign makes the fake answer export_toplevel with handles H1,
// H2, … and token commits with tokens T1, T2, … at once, as compositors
// do; exports or tokens false leave those unanswered.
func answerForeign(h *harness, exports, tokens bool) {
	n, m := 0, 0
	h.fc.setRespond(func(fc *fakeComp, r freq) {
		switch {
		case exports && r.is("zxdg_exporter_v2.export_toplevel"):
			n++
			fc.send(r.u(0), 0, fmt.Sprintf("H%d", n)) // handle
		case tokens && r.is("xdg_activation_token_v1.commit"):
			m++
			fc.send(r.id, 0, fmt.Sprintf("T%d", m)) // done
		}
	})
}

// prepare runs preparePortal on the UI thread and waits for its answer,
// which may come from a later loop iteration.
func prepare(t *testing.T, h *harness, w platform.Window, token bool) (parent, tok string) {
	t.Helper()
	type answer struct{ parent, token string }
	ch := make(chan answer, 1)
	h.ui(func() {
		h.b.preparePortal(w, token, func(p, tk string) { ch <- answer{p, tk} })
	})
	select {
	case a := <-ch:
		return a.parent, a.token
	case <-time.After(3 * time.Second):
		t.Fatal("preparePortal did not answer")
	}
	return "", ""
}

// mappedCtl opens a window with the given globals and maps it.
func mappedCtl(t *testing.T, globals []fakeGlobal, opts platform.WindowOptions) (*harness, *Window, []freq) {
	t.Helper()
	h := newHarness(t, globals)
	presentFrames(h)
	w, _ := openCtl(t, h, opts, 300, 200, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	present := h.fc.until("wl_surface.commit")
	return h, w, present
}

func TestExportParent(t *testing.T) {
	h, w, _ := mappedCtl(t, ctlGlobals("zxdg_exporter_v2"), platform.WindowOptions{})
	answerForeign(h, true, false)
	surface := h.fc.id("wl_surface")

	if p, tok := prepare(t, h, w, false); p != "wayland:H1" || tok != "" {
		t.Errorf("prepare = %q, %q; want wayland:H1 and no token", p, tok)
	}
	if r, _ := find(h.fc.until("zxdg_exporter_v2.export_toplevel"), "zxdg_exporter_v2.export_toplevel"); r.u(1) != surface {
		t.Errorf("export_toplevel = %v, want the window's surface %d", r, surface)
	}
	// One export per toplevel: the handle is kept.
	if p, _ := prepare(t, h, w, false); p != "wayland:H1" {
		t.Errorf("second prepare = %q, want the same handle", p)
	}
	h.fc.quiet(30*time.Millisecond, "zxdg_exporter_v2.export_toplevel")

	// Hiding unexports before the toplevel goes; hidden, there is no
	// parent.
	h.ui(func() { w.SetVisible(false) })
	reqs := h.fc.until("xdg_toplevel.destroy")
	if _, i := find(reqs, "zxdg_exported_v2.destroy"); i < 0 {
		t.Errorf("hide %v does not unexport", names(reqs))
	}
	if p, _ := prepare(t, h, w, false); p != "" {
		t.Errorf("prepare of a hidden window = %q, want none", p)
	}
	h.fc.quiet(30*time.Millisecond, "zxdg_exporter_v2.export_toplevel")

	// Shown again: a new toplevel, exported anew.
	h.ui(func() { w.SetVisible(true) })
	h.fc.until("wl_surface.commit")
	h.configure(300, 200, 2)
	h.fc.until("wl_surface.commit") // mapped with the retained frame
	if p, _ := prepare(t, h, w, false); p != "wayland:H2" {
		t.Errorf("prepare after showing = %q, want wayland:H2", p)
	}

	// Other windows' requests and nil have no parent.
	if p, _ := prepare(t, h, nil, false); p != "" {
		t.Errorf("prepare(nil) = %q", p)
	}
}

func TestExportParentWithoutAnAnswer(t *testing.T) {
	// A compositor that never sends the handle: the round trip after the
	// export ends the wait without a parent.
	h, w, _ := mappedCtl(t, ctlGlobals("zxdg_exporter_v2"), platform.WindowOptions{})
	if p, _ := prepare(t, h, w, false); p != "" {
		t.Errorf("prepare = %q, want none", p)
	}
	// Without the protocol, nothing is asked.
	h2, w2, _ := mappedCtl(t, standardGlobals(1), platform.WindowOptions{})
	if p, _ := prepare(t, h2, w2, false); p != "" {
		t.Errorf("prepare without xdg-foreign = %q", p)
	}
}

func TestExportUnmappedWindow(t *testing.T) {
	h := newHarness(t, ctlGlobals("zxdg_exporter_v2"))
	answerForeign(h, true, false)
	w, _ := openCtl(t, h, platform.WindowOptions{Hidden: true}, 200, 100, 1)
	expect[platform.ConfigureEvent](h)
	if p, _ := prepare(t, h, w, false); p != "" {
		t.Errorf("prepare of a never-mapped window = %q, want none", p)
	}
	h.fc.quiet(30*time.Millisecond, "zxdg_exporter_v2.export_toplevel")
}

func TestPortalActivationToken(t *testing.T) {
	h, w, _ := mappedCtl(t, ctlGlobals("zxdg_exporter_v2", "xdg_activation_v1"), platform.WindowOptions{AppID: "org.example.app"})
	answerForeign(h, true, true)
	ptr, surface, seat := h.fc.id("wl_pointer"), h.fc.id("wl_surface"), h.fc.id("wl_seat")
	h.fc.send(ptr, 0, uint32(77), surface, 1.0, 1.0) // enter
	h.fc.send(ptr, 5)
	expect[platform.PointerEvent](h)

	if p, tok := prepare(t, h, w, true); p != "wayland:H1" || tok != "T1" {
		t.Errorf("prepare = %q, %q; want wayland:H1 and T1", p, tok)
	}
	reqs := h.fc.until("xdg_activation_token_v1.commit")
	if r, i := find(reqs, "xdg_activation_token_v1.set_serial"); i < 0 || r.u(0) != 77 || r.u(1) != seat {
		t.Errorf("set_serial = %v, want the latest serial (77) and its seat", r)
	}
	if r, i := find(reqs, "xdg_activation_token_v1.set_surface"); i < 0 || r.u(0) != surface {
		t.Errorf("set_surface = %v", r)
	}
	if _, i := find(reqs, "xdg_activation_token_v1.set_app_id"); i >= 0 {
		t.Errorf("the token names an app id: %v (the portal picks the application)", names(reqs))
	}
	// A token for a request without a window still comes.
	if p, tok := prepare(t, h, nil, true); p != "" || tok != "T2" {
		t.Errorf("prepare(nil) = %q, %q; want no parent and T2", p, tok)
	}

	// A compositor that does not answer the token: none after a round
	// trip.
	answerForeign(h, true, false)
	if p, tok := prepare(t, h, w, true); p != "wayland:H1" || tok != "" {
		t.Errorf("prepare = %q, %q; want wayland:H1 and no token", p, tok)
	}
}

func TestSuspendedWindowPausesFrames(t *testing.T) {
	h, w, present := mappedCtl(t, standardGlobals(1), platform.WindowOptions{})
	// A frame request waits for the callback of the Present, outstanding
	// when the compositor suspends the window.
	frame, i := find(present, "wl_surface.frame")
	if i < 0 {
		t.Fatalf("Present %v asks no frame callback", names(present))
	}
	h.ui(func() { w.RequestFrame() })
	h.configure(300, 200, 2, protocols.XdgToplevelStateSuspended)
	expectState(t, h, platform.WindowState{Suspended: true})
	// The late callback and new requests deliver no frame, and no
	// callback is asked for.
	h.fc.send(frame.u(0), 0, uint32(1))
	h.ui(func() { w.RequestFrame() })
	h.noEvent(60 * time.Millisecond)
	h.fc.quiet(30*time.Millisecond, "wl_surface.frame")

	// Resumed: the requested frame comes at once, without a callback.
	h.configure(300, 200, 3, protocols.XdgToplevelStateActivated)
	expectState(t, h, platform.WindowState{})
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
}

// A window suspended from its first configure (opened on a hidden
// workspace) still gets the frame that maps it; then it pauses.
func TestSuspendedBeforeMapping(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{}, 300, 200, 1, protocols.XdgToplevelStateSuspended)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	if _, i := find(h.fc.until("wl_surface.commit"), "wl_surface.attach"); i < 0 {
		t.Error("the first frame did not map the window")
	}
	expectState(t, h, platform.WindowState{Suspended: true})
	h.ui(func() { w.RequestFrame() })
	h.noEvent(60 * time.Millisecond)
	h.configure(300, 200, 2)
	expectState(t, h, platform.WindowState{})
	expect[platform.FrameEvent](h)
}

// A suspended window that is hidden and shown stays paused until a
// configure without the state.
func TestSuspendedThenShown(t *testing.T) {
	h, w, _ := mappedCtl(t, standardGlobals(1), platform.WindowOptions{})
	h.configure(300, 200, 2, protocols.XdgToplevelStateSuspended)
	expectState(t, h, platform.WindowState{Suspended: true})
	h.ui(func() {
		w.SetVisible(false)
		w.SetVisible(true)
		w.RequestFrame()
	})
	h.fc.until("xdg_surface.get_toplevel")
	h.fc.until("wl_surface.commit") // the new initial commit
	// Mapped with the retained frame, still suspended: no frame.
	h.configure(300, 200, 3, protocols.XdgToplevelStateSuspended)
	reqs := h.fc.until("wl_surface.commit")
	if _, i := find(reqs, "wl_surface.attach"); i < 0 {
		t.Errorf("configure while suspended %v does not map the retained frame", names(reqs))
	}
	h.noEvent(60 * time.Millisecond)
	// Resumed: the frame requested while hidden comes at once (the
	// remap's callback is not waited for).
	h.configure(300, 200, 4)
	expectState(t, h, platform.WindowState{})
	expect[platform.FrameEvent](h)
}
