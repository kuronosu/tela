//go:build darwin

package cocoa

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The global shortcut tests register Carbon hot keys with all four
// modifiers, which no other app on a CI runner holds. The key press test
// posts a key event into the system's input stream (CGEventPost), which
// needs a permission the runner may not have: without it the test logs
// and skips.

// testAccel is the hot key of the tests: Control-Option-Shift-Command-J.
var testAccel = platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt | platform.ModShift | platform.ModSuper, Key: "j"}

func TestShortcutArguments(t *testing.T) {
	b := newBackend(t)
	desktopRun(t, b, 20*time.Second, func(w *Window) {
		d := b.Desktop()
		mustPanic(t, "RegisterShortcut without an ID", func() {
			_ = d.RegisterShortcut(platform.Shortcut{Accelerator: testAccel, Description: "x"})
		})
		mustPanic(t, "RegisterShortcut with an upper-case key", func() {
			_ = d.RegisterShortcut(platform.Shortcut{ID: "x", Accelerator: platform.Accelerator{Mods: platform.ModSuper, Key: "J"}, Description: "x"})
		})
		mustPanic(t, "RegisterShortcut without a description", func() {
			_ = d.RegisterShortcut(platform.Shortcut{ID: "x", Accelerator: testAccel})
		})
		mustPanic(t, "UnregisterShortcut with an empty ID", func() { _ = d.UnregisterShortcut("") })
		mustPanic(t, "SetAppID with a space", func() { d.SetAppID("my app") })
		mustPanic(t, "ClaimInstance with an empty ID", func() { _, _ = d.ClaimInstance("", platform.Launch{}) })
		if err := d.UnregisterShortcut("never registered"); err != nil {
			t.Errorf("UnregisterShortcut of an unknown ID = %v, want nil", err)
		}
		d.SetAppID("dev.telaui.Test")
		if b.desk.launch.appID != "dev.telaui.Test" {
			t.Errorf("SetAppID kept %q", b.desk.launch.appID)
		}
		if len(b.desk.keys.byID) != 0 {
			t.Errorf("an invalid request left %d hot keys", len(b.desk.keys.byID))
		}
	}, func(*Window, platform.Event) bool { return true })
}

// shortcutScript registers shortcuts one at a time and checks that each
// request gets exactly one status, from a later pass.
type shortcutScript struct {
	t        *testing.T
	b        *Backend
	statuses map[string][]platform.ShortcutStatusEvent
	inCall   bool
}

func (s *shortcutScript) register(id string, a platform.Accelerator) {
	s.t.Helper()
	s.inCall = true
	err := s.b.Desktop().RegisterShortcut(platform.Shortcut{ID: id, Accelerator: a, Description: "Tela test " + id})
	s.inCall = false
	if err != nil {
		s.t.Errorf("RegisterShortcut(%q, %v) = %v", id, a, err)
	}
}

func (s *shortcutScript) event(e platform.Event) {
	if st, ok := e.(platform.ShortcutStatusEvent); ok {
		if s.inCall {
			s.t.Errorf("the status of %q was delivered from inside RegisterShortcut", st.ID)
		}
		s.t.Logf("status: %+v", st)
		s.statuses[st.ID] = append(s.statuses[st.ID], st)
	}
}

func TestShortcutRegister(t *testing.T) {
	b := newBackend(t)
	s := &shortcutScript{t: t, b: b, statuses: map[string][]platform.ShortcutStatusEvent{}}
	f19 := platform.Accelerator{Mods: testAccel.Mods, Key: platform.KeyF19}
	step := 0
	var deadline time.Time
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		if err := darwin.LoadCarbon(); err != nil {
			t.Errorf("LoadCarbon: %v", err)
		}
		t.Logf("keyboard layout %q", darwin.KeyLayoutID())
		if code, err := hotkeyKeyCode(testAccel.Key, b.keyLayout()); err != nil || code != 0x26 {
			t.Logf("'j' is key code %#x (%v) on this layout; the ANSI J is 0x26", code, err)
		}
		s.register("a", testAccel)
		deadline = time.Now().Add(10 * time.Second)
	}, func(w *Window, e platform.Event) bool {
		s.event(e)
		if time.Now().After(deadline) {
			t.Errorf("step %d: no status within 10 s; statuses %v", step, s.statuses)
			return true
		}
		switch {
		case step == 0 && len(s.statuses["a"]) == 1:
			// The same combination under another ID: what Carbon says.
			s.register("b", testAccel)
			step++
		case step == 1 && len(s.statuses["b"]) == 1:
			s.register("c", f19)
			step++
		case step == 2 && len(s.statuses["c"]) == 1:
			// Registering an ID again replaces its hot key ("b" goes
			// first, in case Carbon let it share the combination).
			if err := b.Desktop().UnregisterShortcut("b"); err != nil {
				t.Errorf("UnregisterShortcut(\"b\") = %v", err)
			}
			s.register("a", testAccel)
			step++
		case step == 3 && len(s.statuses["a"]) == 2:
			for _, id := range []string{"a", "b", "c", "unknown"} {
				if err := b.Desktop().UnregisterShortcut(id); err != nil {
					t.Errorf("UnregisterShortcut(%q) = %v", id, err)
				}
			}
			if len(b.desk.keys.byID) != 0 {
				t.Errorf("after UnregisterShortcut, %d hot keys are left", len(b.desk.keys.byID))
			}
			// Once unregistered, the combination can be registered again.
			s.register("d", testAccel)
			step++
		case step == 4 && len(s.statuses["d"]) == 1:
			step++
			deadline = time.Now().Add(300 * time.Millisecond) // no extra statuses meanwhile
		case step == 5 && time.Now().Add(100*time.Millisecond).After(deadline):
			return true
		}
		return false
	})
	want := map[string][]platform.ShortcutStatus{
		"a": {platform.ShortcutBound, platform.ShortcutBound},
		"c": {platform.ShortcutBound},
		"d": {platform.ShortcutBound},
	}
	for id, ws := range want {
		got := s.statuses[id]
		if len(got) != len(ws) {
			t.Errorf("shortcut %q got %d statuses %v, want %d", id, len(got), got, len(ws))
			continue
		}
		for i, st := range got {
			if st.Status != ws[i] {
				t.Errorf("shortcut %q status %d = %v (%s), want %v", id, i, st.Status, st.Reason, ws[i])
			}
			accel := testAccel
			if id == "c" {
				accel = f19
			}
			if st.Status == platform.ShortcutBound && st.Trigger != macTrigger(accel) {
				t.Errorf("shortcut %q trigger %q, want %q", id, st.Trigger, macTrigger(accel))
			}
		}
	}
	// The same combination twice: whatever Carbon answers, once. It is
	// expected to refuse it (eventHotKeyExistsErr, ShortcutTaken).
	if got := s.statuses["b"]; len(got) != 1 {
		t.Errorf("shortcut \"b\" got statuses %v, want one", got)
	} else {
		t.Logf("the same combination under a second ID: %v (%s)", got[0].Status, got[0].Reason)
	}
	// Run returned: every hot key is gone.
	hotkeys.Lock()
	left := 0
	for _, h := range hotkeys.byNum {
		if h.b == b {
			left++
		}
	}
	hotkeys.Unlock()
	if left != 0 || len(b.desk.keys.byID) != 0 {
		t.Errorf("after Run, %d hot keys of the backend are registered", left)
	}
}

// TestShortcutPress posts the hot key's key down and up into the system
// (kCGHIDEventTap), which delivers them to the registered hot key:
// ShortcutPressed, then ShortcutReleased.
func TestShortcutPress(t *testing.T) {
	b := newBackend(t)
	var events []platform.ShortcutEvent
	var bound, posted bool
	var postedAt time.Time
	var code uint16
	var deadline time.Time
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		var err error
		if code, err = hotkeyKeyCode(testAccel.Key, b.keyLayout()); err != nil {
			t.Errorf("hotkeyKeyCode: %v", err)
		}
		if err := b.Desktop().RegisterShortcut(platform.Shortcut{ID: "press", Accelerator: testAccel, Description: "Tela test"}); err != nil {
			t.Errorf("RegisterShortcut: %v", err)
		}
		deadline = time.Now().Add(10 * time.Second)
	}, func(w *Window, e platform.Event) bool {
		switch e := e.(type) {
		case platform.ShortcutStatusEvent:
			t.Logf("status: %+v", e)
			if e.Status != platform.ShortcutBound {
				t.Errorf("the hot key was not bound: %+v", e)
				return true
			}
			bound = true
		case platform.ShortcutEvent:
			t.Logf("shortcut event %+v, %v after posting", e, time.Since(postedAt).Round(time.Millisecond))
			events = append(events, e)
		}
		if bound && !posted {
			posted, postedAt = true, time.Now()
			postKey(t, code, true)
			postKey(t, code, false)
			deadline = time.Now().Add(3 * time.Second)
		}
		return len(events) >= 2 || time.Now().After(deadline)
	})
	if !bound {
		t.Fatalf("the hot key was never bound")
	}
	allowed, known := darwin.CGPreflightPostEventAccess()
	t.Logf("CGPreflightPostEventAccess: allowed %v (known %v); events %+v", allowed, known, events)
	if len(events) == 0 {
		t.Skipf("no hot key event within 3 s of posting the keys: the process may not post events (allowed %v, known %v)", allowed, known)
	}
	if events[0].ID != "press" || events[0].Action != platform.ShortcutPressed {
		t.Errorf("first event %+v, want a press of \"press\"", events[0])
	}
	if len(events) < 2 || events[1].Action != platform.ShortcutReleased {
		t.Errorf("events %+v, want a press then a release", events)
	}
}

// postKey posts a key down or up of code with the test accelerator's
// modifiers at the HID tap.
func postKey(t *testing.T, code uint16, down bool) {
	t.Helper()
	cg := darwin.CGEventCreateKeyboardEvent(0, code, down)
	if cg == 0 {
		t.Errorf("CGEventCreateKeyboardEvent failed")
		return
	}
	defer darwin.CFRelease(cg)
	const (
		maskShift     = 1 << 17
		maskControl   = 1 << 18
		maskAlternate = 1 << 19
		maskCommand   = 1 << 20
	)
	darwin.CGEventSetFlags(cg, maskShift|maskControl|maskAlternate|maskCommand)
	if !darwin.CGEventPost(darwin.CGHIDEventTap, cg) {
		t.Errorf("CGEventPost is missing")
	}
}
