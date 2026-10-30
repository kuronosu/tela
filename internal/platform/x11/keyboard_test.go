//go:build linux

package x11

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// X11 keycodes (evdev + 8) and keysyms used by the keyboard tests.
const (
	kcAE12  = 21 // dead acute on de
	kcQ     = 24
	kcE     = 26
	kcY     = 29 // z on de
	kcEnter = 36
	kcCtrl  = 37
	kcA     = 38
	kcShift = 50
	kcAlt   = 64
	kcSpace = 65
	kcCaps  = 66
	kcNum   = 77
	kcAltGr = 108
	kcSuper = 133
	kcMode  = 203

	ksShiftL    xkb.Keysym = 0xffe1
	ksControlL  xkb.Keysym = 0xffe3
	ksCapsLock  xkb.Keysym = 0xffe5
	ksMetaL     xkb.Keysym = 0xffe7
	ksAltL      xkb.Keysym = 0xffe9
	ksSuperL    xkb.Keysym = 0xffeb
	ksLevel3    xkb.Keysym = 0xfe03
	ksModeSw    xkb.Keysym = 0xff7e
	ksNumLock   xkb.Keysym = 0xff7f
	ksReturn    xkb.Keysym = 0xff0d
	ksDeadAcute xkb.Keysym = 0xfe51
	ksDeadGrave xkb.Keysym = 0xfe50
	ksEuro      xkb.Keysym = 0x20ac
	ksOmega     xkb.Keysym = 0x7d9 // Greek_OMEGA
	ksShorti    xkb.Keysym = 0x6ca // Cyrillic_shorti
	ksSHORTI    xkb.Keysym = 0x6ea
	ksEf        xkb.Keysym = 0x6c6 // Cyrillic_ef
	ksEF        xkb.Keysym = 0x6e6

	// Core state bits.
	stShift = xproto.KeyButMaskShift
	stLock  = xproto.KeyButMaskLock
	stCtrl  = xproto.KeyButMaskControl
	stMod1  = xproto.KeyButMaskMod1
	stMod2  = xproto.KeyButMaskMod2
	stMod3  = xproto.KeyButMaskMod3
	stMod4  = xproto.KeyButMaskMod4
	stMod5  = xproto.KeyButMaskMod5
	stGrp2  = 1 << 13 // XKB group 2 in a core state (XKB-aware clients)
)

// coreMapping writes keys the way an XKB server presents its keymap to
// core clients (XkbGetCoreMap): a one-group key as "ABAB" plus its extra
// levels and their repetition, a two-group key as G1L1 G1L2 G2L1 G2L2
// and the extra levels of each group. 8 keysyms per keycode.
func coreMapping(keys map[uint32][][]xkb.Keysym, mods [8][]uint32) xkb.CoreMapping {
	const per = 8
	m := xkb.CoreMapping{MinKeycode: 8, KeysymsPerKeycode: per, Keysyms: make([]xkb.Keysym, per*248), Modifiers: mods}
	lvl := func(g []xkb.Keysym, i int) xkb.Keysym {
		if i < len(g) {
			return g[i]
		}
		return 0
	}
	for kc, groups := range keys {
		l := m.Keysyms[(kc-8)*per : (kc-7)*per]
		g1 := groups[0]
		l[0], l[1] = lvl(g1, 0), lvl(g1, 1)
		if len(groups) == 1 {
			l[2], l[3] = l[0], l[1]
			n := copy(l[4:], g1[min(2, len(g1)):])
			copy(l[4+n:], g1[min(2, len(g1)):])
			continue
		}
		g2 := groups[1]
		l[2], l[3] = lvl(g2, 0), lvl(g2, 1)
		n := copy(l[4:], g1[min(2, len(g1)):])
		copy(l[4+n:], g2[min(2, len(g2)):])
	}
	return m
}

func one(levels ...xkb.Keysym) [][]xkb.Keysym { return [][]xkb.Keysym{levels} }

// deMapping is a German-like keymap: AltGr (ISO_Level3_Shift) on Mod5,
// Alt on Mod1, Super on Mod4, dead acute/grave on AE12, € and @ on AltGr.
func deMapping() xkb.CoreMapping {
	var mods [8][]uint32
	mods[0] = []uint32{kcShift}
	mods[1] = []uint32{kcCaps}
	mods[2] = []uint32{kcCtrl}
	mods[3] = []uint32{kcAlt}
	mods[4] = []uint32{kcNum}
	mods[6] = []uint32{kcSuper}
	mods[7] = []uint32{kcAltGr}
	return coreMapping(map[uint32][][]xkb.Keysym{
		kcAE12:  one(ksDeadAcute, ksDeadGrave),
		kcQ:     one('q', 'Q', '@', ksOmega),
		kcE:     one('e', 'E', ksEuro, ksEuro),
		kcY:     one('z', 'Z'),
		kcEnter: one(ksReturn),
		kcCtrl:  one(ksControlL),
		kcA:     one('a', 'A'),
		kcShift: one(ksShiftL),
		kcAlt:   one(ksAltL, ksMetaL),
		kcSpace: one(' '),
		kcCaps:  one(ksCapsLock),
		kcNum:   one(ksNumLock),
		kcAltGr: one(ksLevel3),
		kcSuper: one(ksSuperL),
	}, mods)
}

const xkbFirstEvent = 85

// kbdBackend returns a fake backend with window 7 focused and the given
// mapping. xkbOK: the client is XKB-aware (group in state bits 13-14,
// StateNotify); detectable: detectable auto-repeat is on.
func kbdBackend(t *testing.T, m xkb.CoreMapping, xkbOK, detectable bool) (*Backend, func(...xgb.Event) string) {
	t.Helper()
	b, _ := fakeBackend(1)
	b.kbd.fetch = func() (xkb.CoreMapping, error) { return m, nil }
	b.xkb = xkbExt{extension: extension{present: xkbOK, opcode: 135, firstEvent: xkbFirstEvent}, ok: xkbOK, detectable: detectable}
	r := &recorder{}
	feed := func(evs ...xgb.Event) string {
		for _, e := range evs {
			b.translate(xevent{ev: e})
		}
		b.kbd.flushAll() // the end of pollEvents
		b.dispatch(r)
		return r.take()
	}
	feed()
	if got := feed(focusIn(xproto.NotifyModeNormal), keymapNotify()); got != "focus true" {
		t.Fatalf("focus in = %q", got)
	}
	if b.kbd.err != nil {
		t.Fatalf("keyboard error: %v", b.kbd.err)
	}
	return b, feed
}

func press(kc byte, state uint16, time uint32) xproto.KeyPressEvent {
	return xproto.KeyPressEvent{Detail: xproto.Keycode(kc), Event: 7, State: state, Time: xproto.Timestamp(time)}
}

func release(kc byte, state uint16, time uint32) xproto.KeyReleaseEvent {
	return xproto.KeyReleaseEvent{Detail: xproto.Keycode(kc), Event: 7, State: state, Time: xproto.Timestamp(time)}
}

func focusIn(mode byte) xproto.FocusInEvent {
	return xproto.FocusInEvent{Event: 7, Mode: mode, Detail: xproto.NotifyDetailNonlinear}
}

func focusOut(mode byte) xproto.FocusOutEvent {
	return xproto.FocusOutEvent{Event: 7, Mode: mode, Detail: xproto.NotifyDetailNonlinear}
}

// keymapNotify returns the KeymapNotify that follows FocusIn, with the
// given keycodes down.
func keymapNotify(down ...byte) xproto.KeymapNotifyEvent {
	keys := make([]byte, 31)
	for _, kc := range down {
		i := int(kc) - 8
		keys[i/8] |= 1 << (i % 8)
	}
	return xproto.KeymapNotifyEvent{Keys: keys}
}

// stateNotify returns an XKB StateNotify caused by keycode kc.
func stateNotify(base, latched, locked byte, kc byte) xgb.RawEvent {
	e := make(xgb.RawEvent, 32)
	e[0], e[1] = xkbFirstEvent, xkbStateNotify
	e[10], e[11], e[12] = base, latched, locked
	e[28] = kc
	return e
}

func lines(s ...string) string { return strings.Join(s, "; ") }

func TestKeyboardTextAndModifiers(t *testing.T) {
	_, feed := kbdBackend(t, deMapping(), true, true)
	for _, c := range []struct {
		name string
		evs  []xgb.Event
		want string
	}{
		{"plain a", []xgb.Event{press(kcA, 0, 1), release(kcA, 0, 2)},
			lines(`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`)},
		{"shift a, Mods from StateNotify", []xgb.Event{
			press(kcShift, 0, 1), stateNotify(stShift, 0, 0, kcShift),
			press(kcA, stShift, 2), release(kcA, stShift, 3),
			release(kcShift, stShift, 4), stateNotify(0, 0, 0, kcShift),
		}, lines(
			`key press ShiftLeft "Shift" "" mods=1`,
			`key press KeyA "A" "A" mods=1`,
			`key release KeyA "A" "" mods=1`,
			`key release ShiftLeft "Shift" "" mods=0`)},
		{"shift without StateNotify: predicted", []xgb.Event{
			press(kcShift, 0, 1), release(kcShift, stShift, 2),
		}, lines(`key press ShiftLeft "Shift" "" mods=1`, `key release ShiftLeft "Shift" "" mods=0`)},
		{"caps lock", []xgb.Event{press(kcA, stLock, 1), release(kcA, stLock, 2)},
			lines(`key press KeyA "A" "A" mods=0`, `key release KeyA "A" "" mods=0`)},
		{"ctrl a: Key and Text ignore Ctrl", []xgb.Event{press(kcA, stCtrl, 1), release(kcA, stCtrl, 2)},
			lines(`key press KeyA "a" "a" mods=10`, `key release KeyA "a" "" mods=10`)},
		{"AltGr q is @, AltGr is not Alt", []xgb.Event{
			press(kcAltGr, 0, 1), stateNotify(stMod5, 0, 0, kcAltGr),
			press(kcQ, stMod5, 2), release(kcQ, stMod5, 3),
			press(kcE, stMod5|stShift, 4), release(kcE, stMod5|stShift, 5),
			release(kcAltGr, stMod5, 6), stateNotify(0, 0, 0, kcAltGr),
		}, lines(
			`key press AltRight "AltGraph" "" mods=0`,
			`key press KeyQ "@" "@" mods=0`,
			`key release KeyQ "@" "" mods=0`,
			`key press KeyE "€" "€" mods=1`,
			`key release KeyE "€" "" mods=1`,
			`key release AltRight "AltGraph" "" mods=0`)},
		{"Alt q", []xgb.Event{
			press(kcAlt, 0, 1), stateNotify(stMod1, 0, 0, kcAlt),
			press(kcQ, stMod1, 2), release(kcQ, stMod1, 3),
			release(kcAlt, stMod1, 4), stateNotify(0, 0, 0, kcAlt),
		}, lines(
			`key press AltLeft "Alt" "" mods=100`,
			`key press KeyQ "q" "q" mods=100`,
			`key release KeyQ "q" "" mods=100`,
			`key release AltLeft "Alt" "" mods=0`)},
		{"Super", []xgb.Event{press(kcSuper, 0, 1), release(kcSuper, stMod4, 2)},
			lines(`key press MetaLeft "Meta" "" mods=1000`, `key release MetaLeft "Meta" "" mods=0`)},
		{"Enter has no text", []xgb.Event{press(kcEnter, 0, 1), release(kcEnter, 0, 2)},
			lines(`key press Enter "Enter" "" mods=0`, `key release Enter "Enter" "" mods=0`)},
		{"de z on the Y position", []xgb.Event{press(kcY, 0, 1), release(kcY, 0, 2)},
			lines(`key press KeyY "z" "z" mods=0`, `key release KeyY "z" "" mods=0`)},
		{"latched shift (sticky keys) from StateNotify", []xgb.Event{
			press(kcShift, 0, 1), stateNotify(stShift, 0, 0, kcShift),
			release(kcShift, stShift, 2), stateNotify(0, stShift, 0, kcShift),
			press(kcA, stShift, 3), stateNotify(0, 0, 0, kcA),
			release(kcA, 0, 4),
		}, lines(
			`key press ShiftLeft "Shift" "" mods=1`,
			`key release ShiftLeft "Shift" "" mods=1`,
			`key press KeyA "A" "A" mods=0`,
			`key release KeyA "a" "" mods=0`)},
	} {
		if got := feed(c.evs...); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestKeyboardDeadKeys(t *testing.T) {
	m := deMapping()
	table, err := xkb.ParseCompose([]byte("<dead_acute> <e> : \"é\"\n<dead_acute> <space> : \"'\"\n"), "test", xkb.ComposeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := fakeBackend(1)
	b.kbd.composeLoader = func() *xkb.ComposeTable { return table }
	b.kbd.fetch = func() (xkb.CoreMapping, error) { return m, nil }
	b.xkb = xkbExt{ok: true, detectable: true}
	r := &recorder{}
	feed := func(evs ...xgb.Event) string {
		for _, e := range evs {
			b.translate(xevent{ev: e})
		}
		b.kbd.flushAll()
		b.dispatch(r)
		return r.take()
	}
	feed(focusIn(xproto.NotifyModeNormal))
	got := feed(press(kcAE12, 0, 1), release(kcAE12, 0, 2), press(kcE, 0, 3), release(kcE, 0, 4))
	want := lines(
		`key press Equal "Dead" "" mods=0`,
		`key release Equal "Dead" "" mods=0`,
		`key press KeyE "é" "é" mods=0`,
		`key release KeyE "e" "" mods=0`)
	if got != want {
		t.Errorf("dead acute + e:\n got %s\nwant %s", got, want)
	}
	// A repeat of a dead key does not stack accents; a key that breaks the
	// sequence types itself.
	got = feed(press(kcAE12, 0, 5), press(kcAE12, 0, 6), release(kcAE12, 0, 7), press(kcA, 0, 8), release(kcA, 0, 9))
	want = lines(
		`key press Equal "Dead" "" mods=0`,
		`key press Equal "Dead" "" mods=0 repeat`,
		`key release Equal "Dead" "" mods=0`,
		`key press KeyA "a" "a" mods=0`,
		`key release KeyA "a" "" mods=0`)
	if got != want {
		t.Errorf("dead acute repeat + a:\n got %s\nwant %s", got, want)
	}
	// Focus loss abandons the sequence.
	feed(press(kcAE12, 0, 10), release(kcAE12, 0, 11), focusOut(xproto.NotifyModeNormal), focusIn(xproto.NotifyModeNormal))
	if got := feed(press(kcE, 0, 12), release(kcE, 0, 13)); got != lines(`key press KeyE "e" "e" mods=0`, `key release KeyE "e" "" mods=0`) {
		t.Errorf("after focus loss: %s", got)
	}
}

func TestKeyboardGroups(t *testing.T) {
	// us,ru: the group comes from state bits 13-14 (XKB-aware client).
	var mods [8][]uint32
	mods[0] = []uint32{kcShift}
	m := coreMapping(map[uint32][][]xkb.Keysym{
		kcQ:     {{'q', 'Q'}, {ksShorti, ksSHORTI}},
		kcA:     {{'a', 'A'}, {ksEf, ksEF}},
		kcShift: one(ksShiftL),
	}, mods)
	_, feed := kbdBackend(t, m, true, true)
	got := feed(press(kcQ, 0, 1), release(kcQ, 0, 2), press(kcQ, stGrp2, 3), release(kcQ, stGrp2, 4), press(kcA, stGrp2|stShift, 5), release(kcA, stGrp2|stShift, 6))
	want := lines(
		`key press KeyQ "q" "q" mods=0`, `key release KeyQ "q" "" mods=0`,
		`key press KeyQ "й" "й" mods=0`, `key release KeyQ "й" "" mods=0`,
		`key press KeyA "Ф" "Ф" mods=1`, `key release KeyA "Ф" "" mods=1`)
	if got != want {
		t.Errorf("groups:\n got %s\nwant %s", got, want)
	}
}

// TestKeyboardModeSwitch: without XKB the core rule applies: the modifier
// bound to Mode_switch selects group 2, and the state has no group bits.
func TestKeyboardModeSwitch(t *testing.T) {
	var mods [8][]uint32
	mods[0] = []uint32{kcShift}
	mods[5] = []uint32{kcMode} // Mod3: Mode_switch
	m := coreMapping(map[uint32][][]xkb.Keysym{
		kcQ:     {{'q', 'Q'}, {ksShorti, ksSHORTI}},
		kcShift: one(ksShiftL),
		kcMode:  one(ksModeSw),
	}, mods)
	_, feed := kbdBackend(t, m, false, false)
	got := feed(press(kcMode, 0, 1), press(kcQ, stMod3, 2), release(kcQ, stMod3, 3), release(kcMode, stMod3, 4), press(kcQ, 0, 5), release(kcQ, 0, 6))
	// Keycode 203 (evdev 195) has no W3C code: Unidentified, with the
	// native code kept.
	want := lines(
		`key press Unidentified "ModeChange" "" mods=0`,
		`key press KeyQ "й" "й" mods=0`,
		`key release KeyQ "й" "" mods=0`,
		`key release Unidentified "ModeChange" "" mods=0`,
		`key press KeyQ "q" "q" mods=0`, `key release KeyQ "q" "" mods=0`)
	if got != want {
		t.Errorf("Mode_switch:\n got %s\nwant %s", got, want)
	}
}

func TestKeyboardRepeatDetectable(t *testing.T) {
	b, feed := kbdBackend(t, deMapping(), true, true)
	// Server repeats with detectable auto-repeat: presses only. Shift
	// pressed during the repeat changes the repeated key's text; the
	// server never repeats Shift, and a stray repeat of it is dropped.
	got := feed(
		press(kcA, 0, 1), press(kcA, 0, 600), press(kcA, 0, 640),
		press(kcShift, 0, 650), stateNotify(stShift, 0, 0, kcShift), press(kcShift, stShift, 660),
		press(kcA, stShift, 680),
		release(kcShift, stShift, 690), stateNotify(0, 0, 0, kcShift),
		press(kcA, 0, 720), release(kcA, 0, 730),
	)
	want := lines(
		`key press KeyA "a" "a" mods=0`,
		`key press KeyA "a" "a" mods=0 repeat`,
		`key press KeyA "a" "a" mods=0 repeat`,
		`key press ShiftLeft "Shift" "" mods=1`,
		`key press KeyA "A" "A" mods=1 repeat`,
		`key release ShiftLeft "Shift" "" mods=0`,
		`key press KeyA "a" "a" mods=0 repeat`,
		`key release KeyA "a" "" mods=0`)
	if got != want {
		t.Errorf("repeat:\n got %s\nwant %s", got, want)
	}
	// Keys are idle afterwards: nothing armed (repeat is the server's).
	if _, ok := b.nextDeadline(); ok || b.ready() {
		t.Errorf("the loop has a deadline or work after the keys were released")
	}
}

// TestKeyboardRepeatFallback: without detectable auto-repeat the server
// sends a release and a press with the same time per repeat; the pair is
// one repeat.
func TestKeyboardRepeatFallback(t *testing.T) {
	_, feed := kbdBackend(t, deMapping(), false, false)
	got := feed(
		press(kcA, 0, 1),
		release(kcA, 0, 600), press(kcA, 0, 600),
		release(kcA, 0, 640), press(kcA, 0, 640),
		release(kcA, 0, 700),
	)
	want := lines(
		`key press KeyA "a" "a" mods=0`,
		`key press KeyA "a" "a" mods=0 repeat`,
		`key press KeyA "a" "a" mods=0 repeat`,
		`key release KeyA "a" "" mods=0`)
	if got != want {
		t.Errorf("fallback repeat:\n got %s\nwant %s", got, want)
	}
	// A release followed by a press of the same key at another time is a
	// real release and press; a release followed by another key's event
	// is delivered before it.
	got = feed(press(kcA, 0, 800), release(kcA, 0, 810), press(kcA, 0, 820), release(kcA, 0, 830), press(kcQ, 0, 830), release(kcQ, 0, 840))
	want = lines(
		`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`,
		`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`,
		`key press KeyQ "q" "q" mods=0`, `key release KeyQ "q" "" mods=0`)
	if got != want {
		t.Errorf("fallback presses:\n got %s\nwant %s", got, want)
	}
}

func TestKeyboardFocus(t *testing.T) {
	_, feed := kbdBackend(t, deMapping(), true, true)
	// Focus loss: synthetic releases of the held keys, in press order,
	// before the FocusEvent; nothing is delivered without focus.
	got := feed(
		press(kcA, 0, 1), press(kcShift, 0, 2), stateNotify(stShift, 0, 0, kcShift),
		focusOut(xproto.NotifyModeNormal),
		press(kcQ, stShift, 3), release(kcA, stShift, 4),
	)
	want := lines(
		`key press KeyA "a" "a" mods=0`,
		`key press ShiftLeft "Shift" "" mods=1`,
		`key release KeyA "a" "" mods=0 synthetic`,
		`key release ShiftLeft "Shift" "" mods=0 synthetic`,
		"focus false")
	if got != want {
		t.Errorf("focus loss:\n got %s\nwant %s", got, want)
	}

	// Focus gain with Shift and q still down (KeymapNotify): no presses;
	// the server's repeats of q and the releases of both are dropped;
	// Shift is in the Mods of later keys.
	got = feed(
		focusIn(xproto.NotifyModeNormal), keymapNotify(kcShift, kcQ),
		press(kcQ, stShift, 10), press(kcQ, stShift, 40),
		press(kcA, stShift, 50), release(kcA, stShift, 60),
		release(kcQ, stShift, 70), release(kcShift, stShift, 80),
		press(kcQ, 0, 90), release(kcQ, 0, 100),
	)
	want = lines(
		"focus true",
		`key press KeyA "A" "A" mods=1`,
		`key release KeyA "A" "" mods=1`,
		`key press KeyQ "q" "q" mods=0`,
		`key release KeyQ "q" "" mods=0`)
	if got != want {
		t.Errorf("focus gain with keys down:\n got %s\nwant %s", got, want)
	}

	// A keyboard grab by another client (Alt+Tab switcher): the window
	// stays focused but its held keys are released; the grab pair is no
	// focus change.
	got = feed(
		press(kcAlt, 0, 200), stateNotify(stMod1, 0, 0, kcAlt),
		focusOut(xproto.NotifyModeGrab),
		focusIn(xproto.NotifyModeUngrab), keymapNotify(),
		press(kcA, 0, 300), release(kcA, 0, 310),
	)
	want = lines(
		`key press AltLeft "Alt" "" mods=100`,
		`key release AltLeft "Alt" "" mods=0 synthetic`,
		`key press KeyA "a" "a" mods=0`,
		`key release KeyA "a" "" mods=0`)
	if got != want {
		t.Errorf("keyboard grab:\n got %s\nwant %s", got, want)
	}
}

// TestKeyboardShiftHeldAtFocus: Shift pressed while the window is not
// focused yet (its press reaches the window before FocusIn, or another
// window) and still down when focus arrives. Its press is not delivered,
// but the next keys follow the server's state field: "H" with Shift,
// then the release of Shift (dropped, its press never was) leaves no
// stuck modifier. With and without XKB and detectable auto-repeat.
func TestKeyboardShiftHeldAtFocus(t *testing.T) {
	const kcH = 43
	m := deMapping()
	copy(m.Keysyms[(kcH-8)*m.KeysymsPerKeycode:], []xkb.Keysym{'h', 'H', 'h', 'H'})
	for _, c := range []struct{ xkb, detectable bool }{{true, true}, {true, false}, {false, false}} {
		_, feed := kbdBackend(t, m, c.xkb, c.detectable)
		// state wraps an XKB StateNotify: only XKB clients get one.
		state := func(base byte, kc byte) []xgb.Event {
			if !c.xkb {
				return nil
			}
			return []xgb.Event{stateNotify(base, 0, 0, kc)}
		}
		var evs []xgb.Event
		add := func(e ...xgb.Event) { evs = append(evs, e...) }
		add(focusOut(xproto.NotifyModeNormal))
		add(press(kcShift, 0, 1))
		add(state(stShift, kcShift)...)
		add(focusIn(xproto.NotifyModeNormal), keymapNotify(kcShift))
		add(press(kcH, stShift, 2), release(kcH, stShift, 3))
		add(release(kcShift, stShift, 4))
		add(state(0, kcShift)...)
		add(press(kcH, 0, 5), release(kcH, 0, 6))
		got := feed(evs...)
		want := lines(
			"focus false",
			"focus true",
			`key press KeyH "H" "H" mods=1`,
			`key release KeyH "H" "" mods=1`,
			`key press KeyH "h" "h" mods=0`,
			`key release KeyH "h" "" mods=0`)
		if got != want {
			t.Errorf("xkb %v, detectable %v:\n got %s\nwant %s", c.xkb, c.detectable, got, want)
		}
	}
}

func TestKeyboardCloseDropsKeys(t *testing.T) {
	b, feed := kbdBackend(t, deMapping(), true, true)
	w := b.windows[7]
	b.translate(xevent{ev: press(kcA, 0, 1)})
	// Close sends requests; drop the window by hand like Close does.
	b.loop.Remove(w)
	b.kbd.dropWindow(w)
	delete(b.windows, w.id)
	if got := feed(release(kcA, 0, 2), focusOut(xproto.NotifyModeNormal)); got != "" {
		t.Errorf("events after close: %q", got)
	}
}

func TestKeyboardMappingChanges(t *testing.T) {
	m := deMapping()
	b, feed := kbdBackend(t, m, true, true)
	fetches := 0
	b.kbd.fetch = func() (xkb.CoreMapping, error) { fetches++; return m, nil }
	if got := feed(press(kcY, 0, 1), release(kcY, 0, 2)); !strings.Contains(got, `"z"`) || fetches != 1 {
		t.Fatalf("before the change: %s (%d fetches)", got, fetches)
	}
	// setxkbmap us: the server sends MappingNotify (core) or MapNotify /
	// NewKeyboardNotify (XKB); the next key reads the new mapping once.
	us := deMapping()
	copy(us.Keysyms[(kcY-8)*8:], []xkb.Keysym{'y', 'Y', 'y', 'Y'})
	m = us
	newKbd := make(xgb.RawEvent, 32)
	newKbd[0], newKbd[1] = xkbFirstEvent, xkbNewKeyboardNotify
	got := feed(xproto.MappingNotifyEvent{Request: xproto.MappingKeyboard}, newKbd, press(kcY, 0, 3), release(kcY, 0, 4))
	if !strings.Contains(got, `"y"`) || fetches != 2 {
		t.Errorf("after the change: %s (%d fetches, want 2)", got, fetches)
	}
	// A failed read keeps the old keymap and records the error.
	b.kbd.fetch = func() (xkb.CoreMapping, error) { return xkb.CoreMapping{}, errors.New("boom") }
	got = feed(xproto.MappingNotifyEvent{Request: xproto.MappingModifier}, press(kcY, 0, 5), release(kcY, 0, 6))
	if !strings.Contains(got, `"y"`) || b.kbd.err == nil {
		t.Errorf("after a failed read: %s, err %v", got, b.kbd.err)
	}
	// Pointer mappings do not matter.
	b.kbd.dirty = false
	feed(xproto.MappingNotifyEvent{Request: xproto.MappingPointer})
	if b.kbd.dirty {
		t.Error("MappingPointer marked the keymap dirty")
	}
}

// TestKeyboardWithoutMapping: if the mapping cannot be read, keys keep
// their physical code with Key Unidentified, and the contract still
// holds (no text, releases paired with presses).
func TestKeyboardWithoutMapping(t *testing.T) {
	b, _ := fakeBackend(1)
	b.kbd.fetch = func() (xkb.CoreMapping, error) { return xkb.CoreMapping{}, errors.New("no mapping") }
	b.xkb = xkbExt{ok: true, detectable: true}
	r := &recorder{}
	for _, e := range []xgb.Event{focusIn(xproto.NotifyModeNormal), press(kcA, stMod1, 1), release(kcA, stMod1, 2)} {
		b.translate(xevent{ev: e})
	}
	b.kbd.flushAll()
	b.dispatch(r)
	want := lines("configure 300x200@1x (300x200 px)", "focus true",
		`key press KeyA "Unidentified" "" mods=100`, `key release KeyA "Unidentified" "" mods=100`, "frame")
	if got := r.take(); got != want {
		t.Errorf("without a mapping:\n got %s\nwant %s", got, want)
	}
	if b.kbd.err == nil {
		t.Error("the mapping error was not recorded")
	}
}

// TestPointerModsFromMapping: pointer and scroll events report Mods from
// the real modifier mapping, here an unusual one with Alt on Mod3 and
// Super on Mod2, and AltGr on Mod1 (never Alt).
func TestPointerModsFromMapping(t *testing.T) {
	var mods [8][]uint32
	mods[0] = []uint32{kcShift}
	mods[2] = []uint32{kcCtrl}
	mods[3] = []uint32{kcAltGr}
	mods[4] = []uint32{kcSuper}
	mods[5] = []uint32{kcAlt}
	m := coreMapping(map[uint32][][]xkb.Keysym{
		kcShift: one(ksShiftL), kcCtrl: one(ksControlL), kcAlt: one(ksAltL),
		kcSuper: one(ksSuperL), kcAltGr: one(ksLevel3),
	}, mods)
	_, feed := kbdBackend(t, m, true, true)
	got := feed(
		xproto.EnterNotifyEvent{Event: 7, EventX: 1, EventY: 1, State: stMod3},
		xproto.MotionNotifyEvent{Event: 7, EventX: 2, EventY: 2, State: stMod1},
		xproto.MotionNotifyEvent{Event: 7, EventX: 3, EventY: 3, State: stMod2 | stShift},
		xproto.ButtonPressEvent{Event: 7, Detail: 5, EventX: 3, EventY: 3, State: stMod3 | stCtrl},
	)
	want := lines(
		"pointer enter 1,1 btn=0 held=0 mods=100",
		"pointer move 2,2 btn=0 held=0 mods=0",
		"pointer move 3,3 btn=0 held=0 mods=1001",
		"scroll 0,1 at 3,3 mods=110")
	if got != want {
		t.Errorf("pointer Mods:\n got %s\nwant %s", got, want)
	}
}
