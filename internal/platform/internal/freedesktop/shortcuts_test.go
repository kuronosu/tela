//go:build linux

package freedesktop

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

func TestShortcutTrigger(t *testing.T) {
	for _, c := range []struct {
		a    platform.Accelerator
		want string
	}{
		{platform.Accelerator{Mods: platform.ModCtrl | platform.ModShift, Key: "k"}, "CTRL+SHIFT+k"},
		{platform.Accelerator{Mods: platform.ModSuper, Key: platform.KeyF1}, "LOGO+F1"},
		{platform.Accelerator{Mods: platform.ModAlt, Key: " "}, "ALT+space"},
		{platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt | platform.ModShift | platform.ModSuper, Key: platform.KeyEnter}, "CTRL+ALT+SHIFT+LOGO+Return"},
		{platform.Accelerator{Key: platform.KeyMediaPlayPause}, "XF86AudioPlay"},
		{platform.Accelerator{Mods: platform.ModCtrl, Key: "/"}, "CTRL+slash"},
	} {
		if got, ok := shortcutTrigger(c.a); !ok || got != c.want {
			t.Errorf("shortcutTrigger(%v) = %q, %v; want %q", c.a, got, ok, c.want)
		}
	}
}

func shortcut(id string, mods platform.Modifiers, key platform.Key) platform.Shortcut {
	return platform.Shortcut{ID: id, Accelerator: platform.Accelerator{Mods: mods, Key: key}, Description: "Do " + id}
}

// status waits for the next event, which must be a ShortcutStatusEvent.
func (h *harness) status(what string) platform.ShortcutStatusEvent {
	h.t.Helper()
	e := h.event(what)
	se, ok := e.(platform.ShortcutStatusEvent)
	if !ok {
		h.t.Fatalf("%s: event %#v, want a ShortcutStatusEvent", what, e)
	}
	return se
}

// quiet checks that no event arrives for a while.
func (h *harness) quiet(what string) {
	h.t.Helper()
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case <-h.wake:
			h.d.Dispatch()
			if len(h.events) > 0 {
				h.t.Fatalf("%s: unexpected events %#v", what, h.events)
			}
		case <-deadline:
			return
		}
	}
}

func TestShortcuts(t *testing.T) {
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true, registry: true, dataDirs: []string{appEntries(t, "tela-test")}})
	h := newHarness(t, addr)
	h.d.opts.Now = func() time.Duration { return 42 * time.Second }

	show := shortcut("show", platform.ModCtrl|platform.ModShift, "k")
	if err := h.d.RegisterShortcut(show); err != nil {
		t.Fatalf("RegisterShortcut = %v", err)
	}
	if len(h.events) != 0 {
		t.Fatal("the outcome was delivered from inside the call")
	}
	if se := h.status("bound"); se != (platform.ShortcutStatusEvent{ID: "show", Status: platform.ShortcutBound, Trigger: "CTRL+SHIFT+k"}) {
		t.Fatalf("status = %+v", se)
	}
	calls, binds, sessions, _ := p.snapshot()
	if want := []string{"Register tela-test", "CreateSession", "BindShortcuts"}; !slices.Equal(calls, want) {
		t.Errorf("portal calls = %q, want %q", calls, want)
	}
	if len(binds) != 1 || len(binds[0]) != 1 || binds[0][0].ID != "show" ||
		binds[0][0].Props["preferred_trigger"].Value() != "CTRL+SHIFT+k" || binds[0][0].Props["description"].Value() != "Do show" {
		t.Errorf("BindShortcuts got %+v", binds)
	}
	session := sessions[0]

	// Activations of the session are presses and releases.
	p.emitShortcut("Activated", session, "show")
	if e := h.event("press"); e != (platform.ShortcutEvent{ID: "show", Action: platform.ShortcutPressed, Time: 42 * time.Second}) {
		t.Fatalf("event = %#v", e)
	}
	p.emitShortcut("Deactivated", session, "show")
	if e := h.event("release"); e != (platform.ShortcutEvent{ID: "show", Action: platform.ShortcutReleased, Time: 42 * time.Second}) {
		t.Fatalf("event = %#v", e)
	}
	// Unknown shortcuts and other sessions are ignored.
	p.emitShortcut("Activated", session, "nope")
	p.emitShortcut("Activated", "/org/freedesktop/portal/desktop/session/x/other", "show")
	h.quiet("foreign activations")

	// The user changes the trigger, then removes the shortcut.
	emitChanged := func(list []portalShortcut) {
		t.Helper()
		if err := p.conn.Emit(portalPath, ifaceGlobalShortcuts+".ShortcutsChanged", session, list); err != nil {
			t.Fatal(err)
		}
	}
	emitChanged(boundShortcuts("show", "Ctrl+Alt+S"))
	if se := h.status("rebound"); se != (platform.ShortcutStatusEvent{ID: "show", Status: platform.ShortcutBound, Trigger: "Ctrl+Alt+S"}) {
		t.Fatalf("status = %+v", se)
	}
	emitChanged(boundShortcuts("show", "Ctrl+Alt+S")) // no change
	emitChanged(nil)
	if se := h.status("revoked"); se.ID != "show" || se.Status != platform.ShortcutRevoked {
		t.Fatalf("status = %+v", se)
	}
	p.emitShortcut("Activated", session, "show") // not bound any more
	h.quiet("activation of a revoked shortcut")

	// Registering again binds the set in a new session and closes the old.
	hide := shortcut("hide", platform.ModSuper, platform.KeyF1)
	if err := h.d.RegisterShortcut(show); err != nil {
		t.Fatal(err)
	}
	if err := h.d.RegisterShortcut(hide); err != nil {
		t.Fatal(err)
	}
	got := []platform.ShortcutStatusEvent{h.status("show bound"), h.status("hide bound")}
	want := []platform.ShortcutStatusEvent{
		{ID: "show", Status: platform.ShortcutBound, Trigger: "CTRL+SHIFT+k"},
		{ID: "hide", Status: platform.ShortcutBound, Trigger: "LOGO+F1"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("statuses = %+v, want %+v", got, want)
	}
	_, binds, sessions, closed := p.snapshot()
	if len(sessions) != 2 || !closed[sessions[0]] || closed[sessions[1]] {
		t.Errorf("sessions %v, closed %v: want the first closed and the second open", sessions, closed)
	}
	if last := binds[len(binds)-1]; len(last) != 2 || last[0].ID != "show" || last[1].ID != "hide" {
		t.Errorf("last BindShortcuts = %+v, want show and hide", last)
	}
	p.emitShortcut("Activated", sessions[1], "hide")
	if e := h.event("hide pressed").(platform.ShortcutEvent); e.ID != "hide" {
		t.Fatalf("event = %+v", e)
	}
	p.emitShortcut("Activated", sessions[0], "show") // the closed session
	h.quiet("activation in a closed session")

	// Unregistering rebinds the rest (no event: hide is unchanged), and
	// the last one only closes the session.
	if err := h.d.UnregisterShortcut("show"); err != nil {
		t.Fatal(err)
	}
	h.d.Dispatch() // the loop iteration that hands the request to the worker
	eventually(t, "the third session", func() bool {
		_, binds, _, _ := p.snapshot()
		return len(binds) == 3
	})
	h.quiet("unregister")
	_, binds, sessions, closed = p.snapshot()
	if last := binds[2]; len(last) != 1 || last[0].ID != "hide" {
		t.Errorf("BindShortcuts after Unregister = %+v, want hide", last)
	}
	if !closed[sessions[1]] {
		t.Error("the second session was not closed")
	}
	if err := h.d.UnregisterShortcut("hide"); err != nil {
		t.Fatal(err)
	}
	if err := h.d.UnregisterShortcut("unknown"); err != nil {
		t.Errorf("UnregisterShortcut of an unknown ID = %v", err)
	}
	h.d.Dispatch()
	eventually(t, "the last session closed", func() bool {
		_, _, sessions, closed := p.snapshot()
		return closed[sessions[2]]
	})
	h.quiet("last unregister")
	if _, binds, sessions, _ := p.snapshot(); len(sessions) != 3 || len(binds) != 3 {
		t.Errorf("%d sessions and %d binds after the last unregister, want 3 and 3", len(sessions), len(binds))
	}

	// Register then unregister before the portal answers: the outcome is
	// still delivered, once.
	if err := h.d.RegisterShortcut(show); err != nil {
		t.Fatal(err)
	}
	if err := h.d.UnregisterShortcut("show"); err != nil {
		t.Fatal(err)
	}
	// The worker may take the two requests as one batch (Failed) or one
	// at a time (Bound); either way exactly one status.
	if se := h.status("register and unregister"); se.ID != "show" || (se.Status != platform.ShortcutFailed && se.Status != platform.ShortcutBound) {
		t.Fatalf("status = %+v", se)
	}
	h.quiet("after register and unregister")

	p.mu.Lock()
	if len(p.regErr) > 0 {
		t.Errorf("Registry.Register came after another portal call: %q", p.regErr)
	}
	p.mu.Unlock()
}

func TestShortcutsRefused(t *testing.T) {
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true})
	h := newHarness(t, addr)

	// The portal binds only one of two.
	p.setBind(func(list []portalShortcut) (uint32, []portalShortcut) {
		return 0, boundShortcuts("a", "Ctrl+A")
	})
	if err := h.d.RegisterShortcut(shortcut("a", platform.ModCtrl, "a")); err != nil {
		t.Fatal(err)
	}
	if err := h.d.RegisterShortcut(shortcut("b", platform.ModCtrl, "b")); err != nil {
		t.Fatal(err)
	}
	// One batch or two, a ends bound and b refused.
	final := map[string]platform.ShortcutStatusEvent{}
	h.pump("both outcomes", func() bool {
		for _, e := range h.events {
			se := e.(platform.ShortcutStatusEvent)
			final[se.ID] = se
		}
		h.events = nil
		return len(final) == 2 && final["b"].Status == platform.ShortcutRefused
	})
	if final["a"] != (platform.ShortcutStatusEvent{ID: "a", Status: platform.ShortcutBound, Trigger: "Ctrl+A"}) {
		t.Errorf("a = %+v", final["a"])
	}
	if final["b"].Reason == "" {
		t.Error("the refusal has no reason")
	}
	h.quiet("after the first outcomes")

	// The user cancels the dialog: the new shortcut is refused and the
	// bound one is revoked with its session.
	p.setBind(func([]portalShortcut) (uint32, []portalShortcut) { return 1, nil })
	if err := h.d.RegisterShortcut(shortcut("c", platform.ModCtrl, "c")); err != nil {
		t.Fatal(err)
	}
	got := map[string]platform.ShortcutStatus{}
	h.pump("cancel outcomes", func() bool {
		for _, e := range h.events {
			se := e.(platform.ShortcutStatusEvent)
			got[se.ID] = se.Status
		}
		h.events = nil
		return len(got) == 2
	})
	if got["c"] != platform.ShortcutRefused || got["a"] != platform.ShortcutRevoked {
		t.Errorf("statuses after a cancel = %v, want c refused and a revoked", got)
	}
	h.quiet("after the cancel")

	// The portal fails the request.
	p.setBind(func([]portalShortcut) (uint32, []portalShortcut) { return 2, nil })
	if err := h.d.RegisterShortcut(shortcut("d", platform.ModCtrl, "d")); err != nil {
		t.Fatal(err)
	}
	if se := h.status("failure"); se.ID != "d" || se.Status != platform.ShortcutFailed || se.Reason == "" {
		t.Errorf("status = %+v, want d failed with a reason", se)
	}

	// Registering an unbound shortcut again, unchanged, asks again.
	p.setBind(nil)
	if err := h.d.RegisterShortcut(shortcut("d", platform.ModCtrl, "d")); err != nil {
		t.Fatal(err)
	}
	h.pump("d bound", func() bool {
		return slices.ContainsFunc(h.events, func(e platform.Event) bool {
			se := e.(platform.ShortcutStatusEvent)
			return se.ID == "d" && se.Status == platform.ShortcutBound
		})
	})
}

func TestShortcutsUnsupported(t *testing.T) {
	s := shortcut("x", platform.ModCtrl, "x")
	check := func(what string, h *harness) {
		t.Helper()
		if err := h.d.RegisterShortcut(s); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("RegisterShortcut %s = %v, want ErrUnsupported", what, err)
		}
		if err := h.d.UnregisterShortcut("x"); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("UnregisterShortcut %s = %v, want ErrUnsupported", what, err)
		}
		h.quiet(what)
	}
	check("without a bus", newHarness(t, "unix:path=/nonexistent/tela-test-bus"))
	addr := privateBus(t)
	check("without a portal", newHarness(t, addr))
	startPortal13(t, addr, portal13Options{settings: map[string]map[string]dbus.Variant{}})
	check("without the GlobalShortcuts interface", newHarness(t, addr))

	// Invalid shortcuts panic before anything else.
	h := newHarness(t, addr)
	defer func() {
		if recover() == nil {
			t.Error("RegisterShortcut with an empty description did not panic")
		}
	}()
	h.d.RegisterShortcut(platform.Shortcut{ID: "x", Accelerator: s.Accelerator})
}

func TestShortcutsClose(t *testing.T) {
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true})
	h := newHarness(t, addr)
	// The dialog stays open: the request is pending when Run ends.
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	p.setBind(func([]portalShortcut) (uint32, []portalShortcut) {
		<-block
		return 0, nil
	})
	if err := h.d.RegisterShortcut(shortcut("x", platform.ModCtrl, "x")); err != nil {
		t.Fatal(err)
	}
	h.d.Dispatch()
	eventually(t, "the bind request", func() bool {
		_, binds, _, _ := p.snapshot()
		return len(binds) == 1
	})
	h.d.Close()
	h.quiet("after Close")
	if err := h.d.RegisterShortcut(shortcut("y", platform.ModCtrl, "y")); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("RegisterShortcut after Close = %v, want ErrUnsupported", err)
	}
}
