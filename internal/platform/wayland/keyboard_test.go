//go:build linux

package wayland

import (
	"compress/gzip"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// wl_keyboard event opcodes and evdev codes used by the tests.
const (
	kbKeymap     = 0
	kbEnter      = 1
	kbLeave      = 2
	kbKey        = 3
	kbModifiers  = 4
	kbRepeatInfo = 5

	keyA     = 30
	keyE     = 18
	keyH     = 35
	keyI     = 23
	key1     = 2
	keyQuote = 40 // dead_acute on us(intl)
	keyShift = 42
	keyCtrl  = 29
	keyEnter = 28
	keyKP1   = 79

	maskShift   = 0x1
	maskLock    = 0x2
	maskCtrl    = 0x4
	maskNumLock = 0x10
)

// keyboardGlobals is standardGlobals with a seat that has a keyboard.
func keyboardGlobals() []fakeGlobal {
	g := standardGlobals(1)
	for i := range g {
		if g[i].iface == "wl_seat" {
			g[i].onBind = func(fc *fakeComp, id uint32) {
				fc.send(id, 0, uint32(protocols.WlSeatCapabilityPointer|protocols.WlSeatCapabilityKeyboard))
			}
		}
	}
	return g
}

// fixtureKeymap reads a keymap of the xkb package's testdata.
func fixtureKeymap(t *testing.T, name string) []byte {
	t.Helper()
	f, err := os.Open("../internal/xkb/testdata/keymaps/" + name + ".xkb.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtureCompose is the test Compose table of the xkb package.
func fixtureCompose(t *testing.T) func() *xkb.ComposeTable {
	data, err := os.ReadFile("../internal/xkb/testdata/compose/locale/en_US.UTF-8/Compose")
	if err != nil {
		t.Fatal(err)
	}
	table, err := xkb.ParseCompose(data, "Compose", xkb.ComposeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return func() *xkb.ComposeTable { return table }
}

// sendKeymap sends wl_keyboard.keymap with the text in a memfd, NUL
// terminated like libxkbcommon's string, as compositors do.
func (fc *fakeComp) sendKeymap(kb uint32, text []byte) {
	fc.t.Helper()
	fd, err := unix.MemfdCreate("keymap", unix.MFD_CLOEXEC)
	if err != nil {
		fc.t.Fatal(err)
	}
	defer unix.Close(fd)
	data := append(append([]byte(nil), text...), 0)
	if _, err := unix.Write(fd, data); err != nil {
		fc.t.Fatal(err)
	}
	msg := event(kb, kbKeymap, uint32(protocols.WlKeyboardKeymapFormatXkbV1), uint32(len(data)))
	fc.wmu.Lock()
	defer fc.wmu.Unlock()
	if err := unix.Sendmsg(fc.fd, msg, unix.UnixRights(fd), nil, 0); err != nil {
		fc.t.Fatal(err)
	}
}

func keyMsg(kb, key, state uint32) []byte {
	return event(kb, kbKey, uint32(100), uint32(0), key, state)
}

func modsMsg(kb, dep, lock, group uint32) []byte {
	return event(kb, kbModifiers, uint32(101), dep, uint32(0), lock, group)
}

func cat(msgs ...[]byte) []byte {
	var b []byte
	for _, m := range msgs {
		b = append(b, m...)
	}
	return b
}

// kbSetup opens a window, gives the seat the keymap layout with the
// given repeat info, and focuses the window's keyboard. It returns the
// wl_keyboard id.
func kbSetup(t *testing.T, h *harness, layout string, rate, delay int32, states ...protocols.XdgToplevelState) (uint32, *Window) {
	t.Helper()
	w := openWindow(t, h, platform.WindowOptions{}, 200, 100, 1, states...)
	expect[platform.ConfigureEvent](h)
	activated := len(states) > 0
	if activated {
		if fe := expect[platform.FocusEvent](h); !fe.Focused {
			t.Fatalf("FocusEvent %+v", fe)
		}
	}
	expect[platform.FrameEvent](h)
	kb := h.fc.id("wl_keyboard")
	h.fc.sendKeymap(kb, fixtureKeymap(t, layout))
	h.fc.send(kb, kbRepeatInfo, rate, delay)
	h.fc.sendRaw(cat(
		event(kb, kbEnter, uint32(50), h.fc.id("wl_surface"), []byte{}),
		modsMsg(kb, 0, 0, 0),
	))
	if !activated {
		if fe := expect[platform.FocusEvent](h); !fe.Focused {
			t.Fatalf("FocusEvent %+v, want focused at keyboard enter", fe)
		}
	}
	return kb, w
}

// expectKey waits for the next event, which must be a KeyEvent.
func expectKey(t *testing.T, h *harness) platform.KeyEvent {
	t.Helper()
	return expect[platform.KeyEvent](h)
}

func checkKey(t *testing.T, got platform.KeyEvent, action platform.KeyAction, code platform.KeyCode, key platform.Key, text string, mods platform.Modifiers) {
	t.Helper()
	if got.Action != action || got.Code != code || got.Key != key || got.Text != text || got.Mods != mods || got.Location != code.Location() {
		t.Errorf("KeyEvent %v %v %q %q mods %v loc %v, want %v %v %q %q mods %v loc %v",
			got.Action, got.Code, got.Key, got.Text, got.Mods, got.Location, action, code, key, text, mods, code.Location())
	}
}

func TestKeyboardTyping(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, _ := kbSetup(t, h, "us", 0, 0)

	// "a": press and release.
	h.fc.sendRaw(cat(keyMsg(kb, keyA, 1), keyMsg(kb, keyA, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyA, "a", "a", 0)
	checkKey(t, expectKey(t, h), platform.KeyRelease, platform.CodeKeyA, "a", "", 0)

	// Shift: the compositor sends modifiers right after the key, and Mods
	// is the state after the event.
	h.fc.sendRaw(cat(keyMsg(kb, keyShift, 1), modsMsg(kb, maskShift, 0, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeShiftLeft, platform.KeyShift, "", platform.ModShift)
	h.fc.sendRaw(cat(keyMsg(kb, key1, 1), keyMsg(kb, key1, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeDigit1, "!", "!", platform.ModShift)
	checkKey(t, expectKey(t, h), platform.KeyRelease, platform.CodeDigit1, "!", "", platform.ModShift)
	h.fc.sendRaw(cat(keyMsg(kb, keyShift, 0), modsMsg(kb, 0, 0, 0)))
	checkKey(t, expectKey(t, h), platform.KeyRelease, platform.CodeShiftLeft, platform.KeyShift, "", 0)

	// Ctrl+A: Key and Text ignore Ctrl.
	h.fc.sendRaw(cat(keyMsg(kb, keyCtrl, 1), modsMsg(kb, maskCtrl, 0, 0), keyMsg(kb, keyA, 1)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeControlLeft, platform.KeyControl, "", platform.ModCtrl)
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyA, "a", "a", platform.ModCtrl)
	h.fc.sendRaw(cat(keyMsg(kb, keyA, 0), keyMsg(kb, keyCtrl, 0), modsMsg(kb, 0, 0, 0)))
	expectKey(t, h)
	checkKey(t, expectKey(t, h), platform.KeyRelease, platform.CodeControlLeft, platform.KeyControl, "", 0)

	// Caps Lock locked: not a modifier, but it changes the level.
	h.fc.sendRaw(cat(modsMsg(kb, 0, maskLock, 0), keyMsg(kb, keyA, 1), keyMsg(kb, keyA, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyA, "A", "A", 0)
	expectKey(t, h)

	// Num Lock: keypad 1, location numpad.
	h.fc.sendRaw(cat(modsMsg(kb, 0, maskNumLock, 0), keyMsg(kb, keyKP1, 1), keyMsg(kb, keyKP1, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeNumpad1, "1", "1", 0)
	expectKey(t, h)
	h.fc.sendRaw(cat(modsMsg(kb, 0, 0, 0), keyMsg(kb, keyKP1, 1), keyMsg(kb, keyKP1, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeNumpad1, platform.KeyEnd, "", 0)
	expectKey(t, h)

	// Enter: no control character in Text.
	h.fc.sendRaw(cat(keyMsg(kb, keyEnter, 1), keyMsg(kb, keyEnter, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeEnter, platform.KeyEnter, "", 0)
	expectKey(t, h)

	// An unknown evdev code keeps its native code.
	h.fc.sendRaw(cat(keyMsg(kb, 0x2ff, 1), keyMsg(kb, 0x2ff, 0)))
	if e := expectKey(t, h); e.Code != platform.UnidentifiedKeyCode(0x2ff) || e.Key != platform.KeyUnidentified {
		t.Errorf("unknown key: %+v", e)
	}
	expectKey(t, h)
}

// TestKeyboardLateModifiers covers a modifiers event that arrives in a
// later read than its key: the key's Mods is predicted from the keymap.
func TestKeyboardLateModifiers(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, _ := kbSetup(t, h, "us", 0, 0)
	h.fc.sendRaw(keyMsg(kb, keyShift, 1))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeShiftLeft, platform.KeyShift, "", platform.ModShift)
	h.fc.sendRaw(modsMsg(kb, maskShift, 0, 0))
	h.fc.sendRaw(keyMsg(kb, keyShift, 0))
	checkKey(t, expectKey(t, h), platform.KeyRelease, platform.CodeShiftLeft, platform.KeyShift, "", 0)
	h.fc.sendRaw(modsMsg(kb, 0, 0, 0))
	h.fc.sendRaw(cat(keyMsg(kb, keyA, 1), keyMsg(kb, keyA, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyA, "a", "a", 0)
	expectKey(t, h)
}

func TestKeyboardDeadKeys(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	h.b.composeLoader = fixtureCompose(t)
	kb, _ := kbSetup(t, h, "us-intl", 0, 0)
	h.fc.sendRaw(cat(keyMsg(kb, keyQuote, 1), keyMsg(kb, keyQuote, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeQuote, platform.KeyDead, "", 0)
	expectKey(t, h)
	h.fc.sendRaw(cat(keyMsg(kb, keyE, 1), keyMsg(kb, keyE, 0)))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyE, "é", "é", 0)
	checkKey(t, expectKey(t, h), platform.KeyRelease, platform.CodeKeyE, "e", "", 0)

	// Focus loss abandons a sequence.
	h.fc.sendRaw(cat(keyMsg(kb, keyQuote, 1), keyMsg(kb, keyQuote, 0)))
	expectKey(t, h)
	expectKey(t, h)
	h.fc.send(kb, kbLeave, uint32(60), h.fc.id("wl_surface"))
	expect[platform.FocusEvent](h)
	h.fc.sendRaw(cat(event(kb, kbEnter, uint32(61), h.fc.id("wl_surface"), []byte{}), keyMsg(kb, keyE, 1)))
	expect[platform.FocusEvent](h)
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyE, "e", "e", 0)
}

// TestKeyboardNoCompose: without a Compose table (the default under go
// test), a dead key types its spacing accent.
func TestKeyboardNoCompose(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, _ := kbSetup(t, h, "us-intl", 0, 0)
	h.fc.sendRaw(keyMsg(kb, keyQuote, 1))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeQuote, "´", "´", 0)
}

func TestKeyboardRepeat(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	const delay, rate = 100 * time.Millisecond, 50 // 20 ms interval
	kb, _ := kbSetup(t, h, "us", rate, int32(delay/time.Millisecond))

	h.fc.sendRaw(keyMsg(kb, keyA, 1))
	press := expectKey(t, h)
	checkKey(t, press, platform.KeyPress, platform.CodeKeyA, "a", "a", 0)
	// Repeats come at press + delay + n·interval, stamped with that time.
	for n := range 4 {
		e := expectKey(t, h)
		want := press.Time + delay + time.Duration(n)*time.Second/rate
		if !e.Repeat || e.Code != platform.CodeKeyA || e.Text != "a" || e.Time != want {
			t.Errorf("repeat %d: %+v, want Time %v", n, e, want)
		}
		var now time.Duration
		h.ui(func() { now = h.b.Now() })
		if now < e.Time {
			t.Errorf("repeat %d delivered at %v, before its time %v", n, now, e.Time)
		}
	}
	// Shift while repeating: a non-repeating key does not stop the repeat;
	// the repeats follow the new state.
	h.fc.sendRaw(cat(keyMsg(kb, keyShift, 1), modsMsg(kb, maskShift, 0, 0)))
	for {
		e := expectKey(t, h)
		if !e.Repeat {
			checkKey(t, e, platform.KeyPress, platform.CodeShiftLeft, platform.KeyShift, "", platform.ModShift)
			break
		}
	}
	for range 2 {
		e := expectKey(t, h)
		if !e.Repeat || e.Code != platform.CodeKeyA || e.Key != "A" || e.Text != "A" || e.Mods != platform.ModShift {
			t.Errorf("repeat with Shift held: %+v, want Key and Text \"A\", Mods Shift", e)
		}
	}
	// Releasing Shift keeps repeating, back to "a".
	h.fc.sendRaw(cat(keyMsg(kb, keyShift, 0), modsMsg(kb, 0, 0, 0)))
	for {
		e := expectKey(t, h)
		if !e.Repeat {
			checkKey(t, e, platform.KeyRelease, platform.CodeShiftLeft, platform.KeyShift, "", 0)
			break
		}
	}
	for range 2 {
		e := expectKey(t, h)
		if !e.Repeat || e.Key != "a" || e.Text != "a" || e.Mods != 0 {
			t.Errorf("repeat after Shift release: %+v, want \"a\" without Mods", e)
		}
	}
	// Another repeating key stops the repeat of "a" and repeats itself.
	h.fc.sendRaw(cat(keyMsg(kb, keyShift, 1), modsMsg(kb, maskShift, 0, 0), keyMsg(kb, key1, 1)))
	sawPress := false
	for !sawPress {
		e := expectKey(t, h)
		if e.Code == platform.CodeDigit1 {
			checkKey(t, e, platform.KeyPress, platform.CodeDigit1, "!", "!", platform.ModShift)
			sawPress = true
		}
	}
	for range 2 {
		if e := expectKey(t, h); !e.Repeat || e.Code != platform.CodeDigit1 || e.Key != "!" || e.Mods != platform.ModShift {
			t.Errorf("repeat of the new key: %+v", e)
		}
	}
	// Releasing "a" (no longer repeating) does not stop the repeat of 1.
	h.fc.sendRaw(keyMsg(kb, keyA, 0))
	for {
		e := expectKey(t, h)
		if e.Action == platform.KeyRelease {
			checkKey(t, e, platform.KeyRelease, platform.CodeKeyA, "A", "", platform.ModShift)
			break
		}
	}
	if e := expectKey(t, h); !e.Repeat || e.Code != platform.CodeDigit1 {
		t.Errorf("after releasing a, want the repeat of 1 to go on: %+v", e)
	}

	// Its release stops it.
	h.fc.sendRaw(keyMsg(kb, key1, 0))
	for {
		e := expectKey(t, h)
		if e.Action == platform.KeyRelease {
			break
		}
	}
	h.noEvent(150 * time.Millisecond)

	// Idle: no key held, no timer: the loop sleeps.
	before := h.loopIterations()
	time.Sleep(200 * time.Millisecond)
	if n := h.loopIterations() - before; n > 3 {
		t.Errorf("%d loop iterations in 200 ms with no key held: a timer is still armed", n)
	}

	// Rate 0 disables repeat.
	h.fc.send(kb, kbRepeatInfo, int32(0), int32(100))
	h.fc.sendRaw(keyMsg(kb, keyE, 1))
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyE, "E", "E", platform.ModShift) // Shift is still held
	h.noEvent(250 * time.Millisecond)
}

func TestKeyboardFocusLossReleasesKeys(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, _ := kbSetup(t, h, "us", 50, 100)
	h.fc.sendRaw(cat(keyMsg(kb, keyShift, 1), modsMsg(kb, maskShift, 0, 0), keyMsg(kb, keyA, 1)))
	expectKey(t, h)
	expectKey(t, h)
	// Leave while A repeats: synthetic releases in press order, then the
	// FocusEvent; no repeat afterwards.
	h.fc.sendRaw(cat(event(kb, kbLeave, uint32(70), h.fc.id("wl_surface")), modsMsg(kb, 0, 0, 0)))
	for _, code := range []platform.KeyCode{platform.CodeShiftLeft, platform.CodeKeyA} {
		e := expectKey(t, h)
		if e.Action != platform.KeyRelease || !e.Synthetic || e.Code != code || e.Text != "" || e.Mods != 0 {
			t.Errorf("synthetic release %+v, want %v", e, code)
		}
	}
	if fe := expect[platform.FocusEvent](h); fe.Focused {
		t.Errorf("FocusEvent %+v, want unfocused", fe)
	}
	h.noEvent(200 * time.Millisecond)

	// Keys held at enter produce no press, and their release is dropped.
	h.fc.sendRaw(cat(
		event(kb, kbEnter, uint32(71), h.fc.id("wl_surface"), ne.AppendUint32(nil, keyA)),
		modsMsg(kb, 0, 0, 0),
		keyMsg(kb, keyA, 0),
		keyMsg(kb, keyE, 1),
	))
	if fe := expect[platform.FocusEvent](h); !fe.Focused {
		t.Errorf("FocusEvent %+v, want focused", fe)
	}
	checkKey(t, expectKey(t, h), platform.KeyPress, platform.CodeKeyE, "e", "e", 0)
}

// TestKeyboardFocusWithActivation: the window is focused while activated
// or holding keyboard focus; keyboard leave while still activated
// releases the keys without a FocusEvent.
func TestKeyboardFocusWithActivation(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, _ := kbSetup(t, h, "us", 0, 0, protocols.XdgToplevelStateActivated)
	h.fc.sendRaw(keyMsg(kb, keyA, 1))
	expectKey(t, h)
	h.fc.send(kb, kbLeave, uint32(80), h.fc.id("wl_surface"))
	if e := expectKey(t, h); !e.Synthetic || e.Code != platform.CodeKeyA {
		t.Errorf("synthetic release %+v", e)
	}
	h.noEvent(50 * time.Millisecond) // still activated: still focused
	h.configure(200, 100, 2)         // deactivated
	if fe := expect[platform.FocusEvent](h); fe.Focused {
		t.Errorf("FocusEvent %+v, want unfocused", fe)
	}
	// Keyboard enter before activation: one FocusEvent, before the key.
	h.fc.sendRaw(cat(event(kb, kbEnter, uint32(81), h.fc.id("wl_surface"), []byte{}), keyMsg(kb, keyE, 1)))
	if fe := expect[platform.FocusEvent](h); !fe.Focused {
		t.Errorf("FocusEvent %+v, want focused", fe)
	}
	expectKey(t, h)
	h.configure(200, 100, 3, protocols.XdgToplevelStateActivated)
	h.noEvent(50 * time.Millisecond)
}

func TestPointerEventsCarryModifiers(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, _ := kbSetup(t, h, "us", 0, 0)
	ptr, surface := h.fc.id("wl_pointer"), h.fc.id("wl_surface")
	h.fc.sendRaw(cat(keyMsg(kb, keyCtrl, 1), modsMsg(kb, maskCtrl|maskShift, maskNumLock, 0)))
	expectKey(t, h)
	h.fc.send(ptr, 0, uint32(90), surface, 10.0, 20.0) // enter
	h.fc.send(ptr, 5)                                  // frame
	if pe := expect[platform.PointerEvent](h); pe.Mods != platform.ModCtrl|platform.ModShift {
		t.Errorf("pointer enter Mods %v, want Ctrl|Shift", pe.Mods)
	}
	h.fc.send(ptr, 4, uint32(1002), uint32(0), 15.0) // axis
	h.fc.send(ptr, 5)
	if se := expect[platform.ScrollEvent](h); se.Mods != platform.ModCtrl|platform.ModShift {
		t.Errorf("scroll Mods %v, want Ctrl|Shift", se.Mods)
	}
	h.fc.sendRaw(modsMsg(kb, 0, 0, 0))
	h.fc.send(ptr, 2, uint32(1003), 11.0, 21.0) // motion
	h.fc.send(ptr, 5)
	if pe := expect[platform.PointerEvent](h); pe.Mods != 0 {
		t.Errorf("pointer move Mods %v after release, want none", pe.Mods)
	}
}

func TestKeyboardBadKeymap(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	w := openWindow(t, h, platform.WindowOptions{}, 200, 100, 1)
	_ = w
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	kb := h.fc.id("wl_keyboard")
	h.fc.sendKeymap(kb, []byte("xkb_keymap { this is not a keymap"))
	h.fc.sendRaw(cat(event(kb, kbEnter, uint32(50), h.fc.id("wl_surface"), []byte{}), keyMsg(kb, keyA, 1)))
	expect[platform.FocusEvent](h)
	// Keys still arrive with their physical code.
	if e := expectKey(t, h); e.Code != platform.CodeKeyA || e.Key != platform.KeyUnidentified || e.Text != "" {
		t.Errorf("key without a keymap: %+v", e)
	}
	h.ui(func() {
		if h.b.keyboardErr == nil || !strings.Contains(h.b.keyboardErr.Error(), "keymap") {
			t.Errorf("keyboardErr = %v", h.b.keyboardErr)
		}
	})
}

// TestKeyboardCloseFocusedWindow: closing the focused window delivers
// nothing more to it and leaves no repeat armed.
func TestKeyboardCloseFocusedWindow(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kb, w := kbSetup(t, h, "us", 50, 50)
	h.fc.sendRaw(keyMsg(kb, keyA, 1))
	expectKey(t, h)
	h.ui(func() { w.Close() })
	h.noEvent(200 * time.Millisecond)
	h.ui(func() {
		if _, ok := h.b.repeatDeadline(); ok {
			t.Error("a repeat is still armed for a closed window")
		}
	})
}
