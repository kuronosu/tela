//go:build darwin

package cocoa

import (
	"fmt"
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
	posted := false
	t.Cleanup(func() {
		if posted {
			checkModifiersReleased(t)
		}
	})
	var events []platform.ShortcutEvent
	var bound bool
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
			postHotKey(t, code)
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

// The modifiers of testAccel as the keyboard sends them: each key's
// virtual key code and its flags (the device-independent bit and the
// left key's device bit, NX_DEVICEL*KEYMASK), pressed in this order and
// released in the reverse one.
var testModKeys = [...]struct {
	code  uint16
	flags uint64
}{
	{0x3B, 1<<18 | 0x0001}, // kVK_Control
	{0x3A, 1<<19 | 0x0020}, // kVK_Option
	{0x38, 1<<17 | 0x0002}, // kVK_Shift
	{0x37, 1<<20 | 0x0008}, // kVK_Command
}

// modFlagsMask is the device-independent bits of the four modifiers.
const modFlagsMask = 1<<17 | 1<<18 | 1<<19 | 1<<20

// postHotKey types the hot key at the HID tap as a keyboard does: the
// modifier keys down (flagsChanged with the flags held so far), the key
// down and up with all of them, the modifiers up again. The HID system
// keeps the flags it was given, also after the process exits, so every
// modifier pressed is released: posting the key alone with the flags set
// left them held for the next test processes.
func postHotKey(t *testing.T, code uint16) {
	t.Helper()
	var flags uint64
	for _, m := range testModKeys {
		flags |= m.flags
		postKeyEvent(t, m.code, true, true, flags)
	}
	postKeyEvent(t, code, true, false, flags)
	postKeyEvent(t, code, false, false, flags)
	for i := len(testModKeys) - 1; i >= 0; i-- {
		flags &^= testModKeys[i].flags
		postKeyEvent(t, testModKeys[i].code, false, true, flags)
	}
}

// postKeyEvent posts a key down or up of code, or a flagsChanged event of
// a modifier key, with flags, at the HID tap.
func postKeyEvent(t *testing.T, code uint16, down, modifier bool, flags uint64) {
	t.Helper()
	cg := darwin.CGEventCreateKeyboardEvent(0, code, down)
	if cg == 0 {
		t.Errorf("CGEventCreateKeyboardEvent failed")
		return
	}
	defer darwin.CFRelease(cg)
	if modifier {
		darwin.CGEventSetType(cg, darwin.CGEventFlagsChanged)
	}
	darwin.CGEventSetFlags(cg, flags)
	if !darwin.CGEventPost(darwin.CGHIDEventTap, cg) {
		t.Errorf("CGEventPost is missing")
	}
}

// checkModifiersReleased checks that the HID system holds none of the
// test's modifiers (logging what it holds), and releases them with
// flagsChanged events without flags if it does, so that later test
// processes do not see them held.
func checkModifiersReleased(t *testing.T) {
	t.Helper()
	// The posted events reach the HID state asynchronously.
	var flags uint64
	ok := false
	for deadline := time.Now().Add(time.Second); ; {
		flags, ok = darwin.CGEventSourceFlagsState(darwin.CGEventSourceStateHIDSystemState)
		if !ok || flags&modFlagsMask == 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Logf("HID system modifier flags after the test: %#x (known %v)", flags, ok)
	if !ok || flags&modFlagsMask == 0 {
		return
	}
	t.Errorf("the HID system still holds modifiers %#x after the hot key's keys were released", flags&modFlagsMask)
	for i := len(testModKeys) - 1; i >= 0; i-- {
		postKeyEvent(t, testModKeys[i].code, false, true, 0)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if flags, _ = darwin.CGEventSourceFlagsState(darwin.CGEventSourceStateHIDSystemState); flags&modFlagsMask == 0 {
			break
		}
	}
	t.Logf("after posting releases without flags: %#x", flags)
}

// frenchSources are AZERTY keyboard layouts: the Q key (0x0C) types a and
// the A key (0x00) types q.
var frenchSources = []string{"com.apple.keylayout.French", "com.apple.keylayout.French-PC", "com.apple.keylayout.Belgian"}

// TestShortcutRelayout registers Control-Option-Shift-Command-A, selects
// an AZERTY layout (enabled for the test on CI), and checks that the
// distributed notification moved the hot key to the key that types a
// there (0x0C), that pressing that key fires it, and that selecting the
// first layout again moves it back. No status event follows a move that
// keeps the shortcut bound. It skips when no AZERTY layout can be
// selected.
func TestShortcutRelayout(t *testing.T) {
	b := newBackend(t)
	accel := platform.Accelerator{Mods: testAccel.Mods, Key: "a"}
	var src, prev uintptr
	var enabled, posted, restored bool
	var skip string
	t.Cleanup(func() {
		if posted {
			checkModifiersReleased(t)
		}
	})
	restore := func() {
		if restored {
			return
		}
		restored = true
		if prev != 0 {
			t.Logf("selecting %s again: status %d", sourceID(prev), darwin.TISSelectInputSource(prev))
		}
		if enabled {
			t.Logf("disabling %s: status %d", sourceID(src), darwin.TISDisableInputSource(src))
		}
	}
	defer onMain(func() {
		restore()
		if prev != 0 {
			darwin.CFRelease(prev)
		}
		if src != 0 {
			darwin.Release(darwin.ID(src))
		}
	})
	var statuses []platform.ShortcutStatusEvent
	var events []platform.ShortcutEvent
	var first uint16
	step := 0
	var deadline time.Time
	desktopRun(t, b, 60*time.Second, func(w *Window) {
		if err := darwin.LoadTextInputSources(); err != nil {
			skip = fmt.Sprintf("Text Input Sources unavailable: %v", err)
			return
		}
		if err := darwin.LoadCarbon(); err != nil {
			skip = fmt.Sprintf("Carbon unavailable: %v", err)
			return
		}
		prev = darwin.TISCopyCurrentKeyboardInputSource()
		t.Logf("current input source %s, keyboard layout %s, notification %q", sourceID(prev), darwin.KeyLayoutID(), darwin.SelectedInputSourceChangedName())
		if src = findSource(frenchSources, false); src == 0 {
			if src = findSource(frenchSources, true); src != 0 {
				st := darwin.TISEnableInputSource(src)
				enabled = st == 0
				t.Logf("enabled %s for the test: status %d", sourceID(src), st)
			}
		}
		if src == 0 {
			skip = "no AZERTY keyboard layout is installed"
			return
		}
		if err := b.Desktop().RegisterShortcut(platform.Shortcut{ID: "relayout", Accelerator: accel, Description: "Tela test"}); err != nil {
			t.Errorf("RegisterShortcut: %v", err)
			skip = "RegisterShortcut failed"
			return
		}
		deadline = time.Now().Add(10 * time.Second)
	}, func(w *Window, e platform.Event) bool {
		switch e := e.(type) {
		case platform.ShortcutStatusEvent:
			t.Logf("status: %+v", e)
			statuses = append(statuses, e)
		case platform.ShortcutEvent:
			t.Logf("shortcut event %+v", e)
			events = append(events, e)
		}
		if skip != "" {
			return true
		}
		h := b.desk.keys.byID["relayout"]
		if h == nil {
			t.Errorf("the shortcut is not kept")
			return true
		}
		switch step {
		case 0: // bound under the first layout: select AZERTY
			if len(statuses) == 0 {
				break
			}
			if statuses[0].Status != platform.ShortcutBound {
				t.Errorf("the hot key was not bound: %+v", statuses[0])
				return true
			}
			first = h.target.code
			st := darwin.TISSelectInputSource(src)
			t.Logf("'a' is key code %#x on %s; TISSelectInputSource(%s): status %d", first, darwin.KeyLayoutID(), sourceID(src), st)
			if st != 0 {
				skip = fmt.Sprintf("TISSelectInputSource(%s) returned %d", sourceID(src), st)
				return true
			}
			step, deadline = 1, time.Now().Add(10*time.Second)
		case 1: // the notification moved the hot key
			if h.target.code != 0x0C {
				break
			}
			t.Logf("moved to %#x after %d notifications (layout %s, registered %v)", h.target.code, b.desk.keys.layoutChanges, darwin.KeyLayoutID(), h.ref != 0)
			if h.ref == 0 {
				t.Errorf("the hot key is not registered after the move")
			}
			posted = true
			postHotKey(t, 0x0C)
			step, deadline = 2, time.Now().Add(3*time.Second)
		case 2: // the press on the moved key
			if len(events) < 2 && time.Now().Before(deadline) {
				break
			}
			allowed, known := darwin.CGPreflightPostEventAccess()
			t.Logf("events after posting 0x0C: %+v (post access %v, known %v)", events, allowed, known)
			if len(events) == 0 {
				t.Logf("no hot key event: the process may not post events")
			} else if events[0].Action != platform.ShortcutPressed {
				t.Errorf("first event %+v, want a press", events[0])
			}
			restore()
			step, deadline = 3, time.Now().Add(10*time.Second)
		case 3: // back on the first layout's key
			if h.target.code == first {
				t.Logf("moved back to %#x after %d notifications", first, b.desk.keys.layoutChanges)
				return true
			}
		}
		if time.Now().After(deadline) {
			t.Errorf("step %d: nothing within the deadline (target %+v, %d notifications, layout %s)",
				step, h.target, b.desk.keys.layoutChanges, darwin.KeyLayoutID())
			return true
		}
		return false
	})
	if skip != "" {
		t.Skip(skip)
	}
	if len(statuses) != 1 {
		t.Errorf("statuses %+v, want one ShortcutBound (a move that keeps it bound reports nothing)", statuses)
	}
}
