//go:build linux

package wayland

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Hiding destroys the role objects and showing makes new ones (ADR-186).
// The fake's protoCheck fails these tests on any request that breaks the
// xdg-shell object rules, and the wire layer panics on a request to a
// destroyed object.

// waitReq returns the requests up to and including the first one that
// matches, failing the test after a timeout.
func (fc *fakeComp) waitReq(match func(freq) bool) []freq {
	fc.t.Helper()
	var got []freq
	for {
		r := fc.next()
		got = append(got, r)
		if match(r) {
			return got
		}
	}
}

// waitFor waits for the next event of type T, skipping the others.
func waitFor[T platform.Event](h *harness) T {
	h.t.Helper()
	for {
		if e, ok := h.event().ev.(T); ok {
			return e
		}
	}
}

// created returns the object created by the first request called name
// (its new_id at argument i).
func created(t *testing.T, rs []freq, name string, i int) uint32 {
	t.Helper()
	r, j := find(rs, name)
	if j < 0 {
		t.Fatalf("no %s in %v", name, names(rs))
	}
	return r.u(i)
}

// TestHideShowMapsAgain hides and shows a window three times against a
// compositor that answers a new initial commit on a kept xdg_surface (sway)
// and one that does not (Hyprland 0.56): both must map it again and resume
// its frames.
func TestHideShowMapsAgain(t *testing.T) {
	for _, hyprland := range []bool{false, true} {
		name := "sway"
		if hyprland {
			name = "hyprland"
		}
		t.Run(name, func(t *testing.T) {
			auto := newAutoState()
			auto.hyprland = hyprland
			h := newHarnessWith(t, standardGlobals(1), auto)
			presentFrames(h)
			win, err := h.b.NewWindow(platform.WindowOptions{Title: "hide and show"})
			if err != nil {
				t.Fatal(err)
			}
			w := win.(*Window)
			h.start()
			mapped := func() {
				t.Helper()
				h.fc.waitReq(func(r freq) bool { return r.is("wl_surface.attach") && r.u(0) != 0 })
				h.fc.until("wl_surface.commit")
				h.ui(func() {
					if !w.mapped || w.ctl.awaitConfigure {
						t.Errorf("mapped %v, awaitConfigure %v after the buffer", w.mapped, w.ctl.awaitConfigure)
					}
				})
			}
			mapped()
			for range 3 {
				h.ui(func() { w.SetVisible(false) })
				h.fc.waitReq(func(r freq) bool { return r.is("wl_surface.attach") && r.u(0) == 0 })
				h.ui(func() { w.SetVisible(true) })
				mapped()
				// Frames resume.
				h.ui(func() { w.RequestFrame() })
				waitFor[platform.FrameEvent](h)
			}
		})
	}
}

func TestHideShowRecreatesTheRole(t *testing.T) {
	h := newHarness(t, ctlGlobals("xdg_wm_dialog_v1", "xdg_toplevel_icon_manager_v1", "xdg_session_manager_v1", "xdg_activation_v1"))
	presentFrames(h)
	// A modal window (its parent is mapped first) with every attribute.
	pw, err := h.b.NewWindow(platform.WindowOptions{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	win, err := h.b.NewWindow(platform.WindowOptions{
		Title: "one", AppID: "org.example.hide", Frameless: true, Modal: true, Parent: pw,
		MinSize: platform.Size{W: 100, H: 50},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	h.start()
	pXs, pTop := ids(t, h.fc.until("wl_surface.commit"))
	setup := h.fc.until("wl_surface.commit")
	h.fc.sendRaw(configureFor(pTop, pXs, 400, 300, 1))
	h.fc.until("xdg_toplevel.set_parent")
	surface := created(t, setup, "wl_compositor.create_surface", 0)
	oldXs, oldTop := ids(t, setup)
	oldDeco := created(t, setup, "zxdg_decoration_manager_v1.get_toplevel_decoration", 0)
	oldDialog := created(t, setup, "xdg_wm_dialog_v1.get_xdg_dialog", 0)
	add, _ := find(setup, "xdg_session_v1.add_toplevel")
	oldSession, oldName := add.u(0), add.args[2].(string)
	h.fc.send(h.fc.id("xdg_session_v1"), 0, "sid") // created
	h.fc.sendRaw(configureFor(oldTop, oldXs, 300, 200, 2, protocols.XdgToplevelStateActivated))
	attach, _ := find(h.fc.until("wl_surface.commit"), "wl_surface.attach")
	bufID := attach.u(0)
	if fe := waitFor[platform.FocusEvent](h); !fe.Focused {
		t.Errorf("FocusEvent = %+v, want the activated window focused", fe)
	}
	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	h.ui(func() { w.SetIcon([]image.Image{icon}) })
	h.fc.until("wl_surface.commit")

	// Hiding: the null buffer, then the role objects, children first.
	h.ui(func() { w.SetVisible(false) })
	reqs := h.fc.until("xdg_surface.destroy")
	want := []struct {
		name string
		id   uint32
	}{
		{"wl_surface.attach", surface}, {"wl_surface.commit", surface},
		{"zxdg_toplevel_decoration_v1.destroy", oldDeco}, {"xdg_dialog_v1.destroy", oldDialog},
		{"xdg_toplevel_session_v1.destroy", oldSession}, {"xdg_toplevel.destroy", oldTop},
		{"xdg_surface.destroy", oldXs},
	}
	if len(reqs) != len(want) {
		t.Fatalf("hide = %v, want %d requests", names(reqs), len(want))
	}
	for i, r := range reqs {
		if !r.is(want[i].name) || r.id != want[i].id {
			t.Errorf("hide request %d = %v, want %s on #%d", i, r, want[i].name, want[i].id)
		}
	}
	if reqs[0].u(0) != 0 {
		t.Errorf("hide attached %v, want NULL", reqs[0])
	}
	if fe := waitFor[platform.FocusEvent](h); fe.Focused {
		t.Errorf("FocusEvent = %+v after hiding", fe)
	}

	// Hidden: requests are recorded, nothing is sent (the toplevel is
	// gone; a request to it would panic in the wire layer).
	h.ui(func() {
		w.SetTitle("two")
		w.SetMinSize(platform.Size{W: 120, H: 60})
		w.SetMaximized(true)
		w.SetIcon([]image.Image{icon, image.NewRGBA(image.Rect(0, 0, 32, 32))})
		w.Minimize()
		w.SetCursor(platform.CursorText)
		if _, n, _ := parseRestoreToken(w.RestoreToken()); n != oldName {
			t.Errorf("RestoreToken while hidden names %q, want the old toplevel %q", n, oldName)
		}
	})
	if got := h.fc.quiet(50*time.Millisecond, "wl_surface.commit"); len(got) > 0 {
		t.Errorf("a hidden window sent %v", names(got))
	}

	// Shown with an activation: new role objects on the same surface with
	// every attribute, the toplevel in the session under a new name, the
	// old name removed, the initial commit without a buffer.
	h.ui(func() {
		w.SetVisible(true)
		w.Activate()
	})
	reqs = h.fc.until("wl_surface.commit")
	gx, _ := find(reqs, "xdg_wm_base.get_xdg_surface")
	if gx.name == "" || gx.u(1) != surface {
		t.Fatalf("show %v: want get_xdg_surface on wl_surface#%d", names(reqs), surface)
	}
	newXs := gx.u(0)
	newTop := created(t, reqs, "xdg_surface.get_toplevel", 0)
	newDeco := created(t, reqs, "zxdg_decoration_manager_v1.get_toplevel_decoration", 0)
	newDialog := created(t, reqs, "xdg_wm_dialog_v1.get_xdg_dialog", 0)
	on := func(name string, id uint32) freq {
		t.Helper()
		r, i := find(reqs, name)
		if i < 0 || r.id != id {
			t.Errorf("show %v: want %s on #%d", reqs, name, id)
		}
		return r
	}
	if r := on("xdg_toplevel.set_title", newTop); r.name != "" && r.args[0] != "two" {
		t.Errorf("set_title = %v", r)
	}
	if r := on("xdg_toplevel.set_app_id", newTop); r.name != "" && r.args[0] != "org.example.hide" {
		t.Errorf("set_app_id = %v", r)
	}
	if r := on("xdg_toplevel.set_min_size", newTop); r.name != "" && (r.i(0) != 120 || r.i(1) != 60) {
		t.Errorf("set_min_size = %v", r)
	}
	on("xdg_toplevel.set_max_size", newTop)
	on("xdg_toplevel.set_maximized", newTop)
	if r := on("zxdg_toplevel_decoration_v1.set_mode", newDeco); r.name != "" && r.u(0) != uint32(protocols.ZxdgToplevelDecorationV1ModeClientSide) {
		t.Errorf("set_mode = %v, want client side (frameless)", r)
	}
	if r, _ := find(reqs, "zxdg_decoration_manager_v1.get_toplevel_decoration"); r.u(1) != newTop {
		t.Errorf("get_toplevel_decoration = %v, want the new toplevel #%d", r, newTop)
	}
	if r, _ := find(reqs, "xdg_wm_dialog_v1.get_xdg_dialog"); r.u(1) != newTop {
		t.Errorf("get_xdg_dialog = %v, want the new toplevel #%d", r, newTop)
	}
	on("xdg_dialog_v1.set_modal", newDialog)
	if r := on("xdg_toplevel.set_parent", newTop); r.name != "" && r.u(0) != pTop {
		t.Errorf("set_parent = %v, want the parent #%d", r, pTop)
	}
	if r, i := find(reqs, "xdg_toplevel_icon_manager_v1.set_icon"); i < 0 || r.u(0) != newTop || r.u(1) == 0 {
		t.Errorf("show %v: want set_icon with the icon on the new toplevel", names(reqs))
	}
	if n := len(slices.DeleteFunc(slices.Clone(reqs), func(r freq) bool { return !r.is("xdg_toplevel_icon_v1.add_buffer") })); n != 2 {
		t.Errorf("icon buffers = %d, want the 2 images set while hidden", n)
	}
	addNew, iAdd := find(reqs, "xdg_session_v1.add_toplevel")
	rm, iRm := find(reqs, "xdg_session_v1.remove_toplevel")
	newName, _ := addNew.args[2].(string)
	if iAdd < 0 || addNew.u(1) != newTop || newName == oldName || !validSessionName(newName) {
		t.Errorf("add_toplevel = %v, want the new toplevel under a fresh name", addNew)
	}
	if iRm < iAdd || rm.args[0] != oldName {
		t.Errorf("remove_toplevel = %v, want the old name %q after add_toplevel", rm, oldName)
	}
	if _, i := find(reqs, "wl_surface.attach"); i >= 0 {
		t.Errorf("a buffer before the configure: %v", names(reqs))
	}
	if c := reqs[len(reqs)-1]; c.id != surface {
		t.Errorf("initial commit on #%d, want the surface #%d", c.id, surface)
	}

	// The token arrives before the configure: the activation waits for
	// the map.
	h.fc.until("xdg_activation_token_v1.commit")
	h.fc.send(h.fc.id("xdg_activation_token_v1"), 0, "the-token") // done
	h.fc.quiet(50*time.Millisecond, "xdg_activation_v1.activate")

	// The configure of the new xdg_surface maps the window with its
	// retained frame, then activates it.
	h.fc.sendRaw(configureFor(newTop, newXs, 300, 200, 7, protocols.XdgToplevelStateActivated))
	reqs = h.fc.until("xdg_activation_v1.activate")
	ack, iAck := find(reqs, "xdg_surface.ack_configure")
	att, iAtt := find(reqs, "wl_surface.attach")
	_, iCommit := find(reqs, "wl_surface.commit")
	act := reqs[len(reqs)-1]
	if iAck < 0 || ack.id != newXs || ack.u(0) != 7 || iAtt < iAck || att.u(0) != bufID || iCommit < iAtt {
		t.Errorf("remap = %v, want ack_configure(7) on #%d, then the retained buffer #%d", reqs, newXs, bufID)
	}
	if act.args[0] != "the-token" || act.u(1) != surface {
		t.Errorf("activate = %v", act)
	}
	if fe := waitFor[platform.FocusEvent](h); !fe.Focused {
		t.Errorf("FocusEvent = %+v after showing", fe)
	}
	h.ui(func() {
		if _, n, _ := parseRestoreToken(w.RestoreToken()); n != newName {
			t.Errorf("RestoreToken after showing names %q, want %q", n, newName)
		}
	})

	// Close destroys the new objects (protoCheck checks the order).
	h.ui(func() { w.Close() })
	reqs = h.fc.until("wl_surface.destroy")
	for _, d := range []struct {
		name string
		id   uint32
	}{
		{"zxdg_toplevel_decoration_v1.destroy", newDeco}, {"xdg_dialog_v1.destroy", newDialog},
		{"xdg_toplevel_session_v1.destroy", addNew.u(0)}, {"xdg_toplevel.destroy", newTop}, {"xdg_surface.destroy", newXs},
	} {
		if r, i := find(reqs, d.name); i < 0 || r.id != d.id {
			t.Errorf("close %v: want %s on #%d", names(reqs), d.name, d.id)
		}
	}
}

func TestHiddenWindowCloses(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{}, 200, 100, 1)
	waitFor[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	h.ui(func() { w.SetVisible(false) })
	h.fc.until("xdg_surface.destroy")
	h.ui(func() { w.Close() })
	reqs := h.fc.until("wl_surface.destroy")
	for _, r := range reqs {
		if r.iface == "xdg_toplevel" || r.iface == "xdg_surface" || r.iface == "zxdg_toplevel_decoration_v1" || r.iface == "xdg_dialog_v1" {
			t.Errorf("closing a hidden window sent %v: its role objects are gone", r)
		}
	}
	// Calls on the closed window are no-ops.
	h.ui(func() {
		w.SetVisible(true)
		w.SetTitle("closed")
	})
	h.fc.quiet(50*time.Millisecond, "xdg_wm_base.get_xdg_surface")
}

// TestHideShowBeforeTheConfigure hides a window shown again before the
// configure of its new role arrived, and shows it again: the role objects
// made by the first show stay (nothing is mapped), and their configure
// maps the window.
func TestHideShowBeforeTheConfigure(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	w, _ := openCtl(t, h, platform.WindowOptions{}, 300, 200, 1, protocols.XdgToplevelStateActivated)
	waitFor[platform.FrameEvent](h)
	attach, _ := find(h.fc.until("wl_surface.commit"), "wl_surface.attach")
	h.ui(func() { w.SetVisible(false) })
	h.fc.until("xdg_surface.destroy")
	h.ui(func() { w.SetVisible(true) })
	reqs := h.fc.until("wl_surface.commit")
	newXs, newTop := ids(t, reqs)
	h.ui(func() {
		w.SetVisible(false)
		w.SetTitle("again")
		w.SetVisible(true)
	})
	// The title goes to the live toplevel; nothing is destroyed or made.
	for _, r := range h.fc.quiet(50*time.Millisecond, "xdg_wm_base.get_xdg_surface") {
		if r.is("xdg_toplevel.set_title") && r.id == newTop {
			continue
		}
		t.Errorf("hide and show before the configure sent %v", r)
	}
	h.fc.sendRaw(configureFor(newTop, newXs, 300, 200, 2, protocols.XdgToplevelStateActivated))
	reqs = h.fc.until("wl_surface.commit")
	ack, iAck := find(reqs, "xdg_surface.ack_configure")
	att, iAtt := find(reqs, "wl_surface.attach")
	if iAck < 0 || ack.id != newXs || iAtt < iAck || att.u(0) != attach.u(0) {
		t.Errorf("remap = %v, want ack_configure on #%d, then the retained buffer", reqs, newXs)
	}
	frame, _ := find(reqs, "wl_surface.frame")
	h.fc.send(frame.u(0), 0, uint32(1))
	h.ui(func() { w.RequestFrame() })
	waitFor[platform.FrameEvent](h)
}

// TestHiddenParentReparentsChildren checks set_parent across a parent's
// hide and show: a shown child is pointed at the parent's new toplevel when
// it maps; a hidden child gets it when shown.
func TestHiddenParentReparentsChildren(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presentFrames(h)
	pw, err := h.b.NewWindow(platform.WindowOptions{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	cw, err := h.b.NewWindow(platform.WindowOptions{Title: "child", Parent: pw})
	if err != nil {
		t.Fatal(err)
	}
	p, c := pw.(*Window), cw.(*Window)
	h.start()
	pXs, pTop := ids(t, h.fc.until("wl_surface.commit"))
	cXs, cTop := ids(t, h.fc.until("wl_surface.commit"))
	h.fc.sendRaw(configureFor(pTop, pXs, 300, 200, 1))
	h.fc.until("xdg_toplevel.set_parent")
	h.fc.sendRaw(configureFor(cTop, cXs, 200, 100, 2))
	h.fc.waitReq(func(r freq) bool { return r.is("wl_surface.attach") && r.u(0) != 0 })
	h.fc.until("wl_surface.commit")

	// show hides and shows the parent and answers its new role's configure;
	// it returns the requests up to the remap's commit.
	serial := uint32(10)
	show := func() (top uint32, reqs []freq) {
		t.Helper()
		h.ui(func() { p.SetVisible(true) })
		xs, top := ids(t, h.fc.until("wl_surface.commit"))
		serial++
		h.fc.sendRaw(configureFor(top, xs, 300, 200, serial))
		reqs = h.fc.waitReq(func(r freq) bool { return r.is("wl_surface.attach") && r.u(0) != 0 })
		return top, append(reqs, h.fc.until("wl_surface.commit")...)
	}

	// The child is shown: the parent's new toplevel becomes its parent.
	h.ui(func() { p.SetVisible(false) })
	h.fc.until("xdg_surface.destroy")
	pTop2, reqs := show()
	reqs = append(reqs, h.fc.quiet(50*time.Millisecond, "xdg_wm_base.get_xdg_surface")...)
	if r, i := find(reqs, "xdg_toplevel.set_parent"); i < 0 || r.id != cTop || r.u(0) != pTop2 {
		t.Errorf("remap %v: want set_parent(#%d) on the child #%d", reqs, pTop2, cTop)
	}

	// The child is hidden too: nothing for it when the parent maps, and
	// its new toplevel gets the parent's current one when shown.
	h.ui(func() {
		c.SetVisible(false)
		p.SetVisible(false)
	})
	h.fc.until("xdg_surface.destroy")
	h.fc.until("xdg_surface.destroy")
	pTop3, _ := show()
	h.fc.quiet(50*time.Millisecond, "xdg_toplevel.set_parent")
	h.ui(func() { c.SetVisible(true) })
	reqs = h.fc.until("wl_surface.commit")
	_, cTop2 := ids(t, reqs)
	if r, i := find(reqs, "xdg_toplevel.set_parent"); i < 0 || r.id != cTop2 || r.u(0) != pTop3 {
		t.Errorf("child shown %v: want set_parent(#%d) on its new toplevel #%d", reqs, pTop3, cTop2)
	}
}
