package win32

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// This file drives keyTranslator with the message sequences Windows
// produces (the live_keyboard_test.go cases log the real ones on Windows) and
// a scripted layout.

type layoutKey struct {
	plain, shifted string
	kind           charKind // of the plain level
	deadShifted    bool
}

// fakeEnv is a message queue and a keyboard layout.
type fakeEnv struct {
	queue    []keyMsg
	curTime  uint32
	mods     platform.Modifiers
	altGr    bool
	language uint16
	layout   map[uint32]layoutKey
	down     map[uint32]bool
	clock    time.Duration
	// inputFirst makes the queue look as a scan that finds the input queue
	// before the posted one would see it: the key messages ahead of the
	// characters, whatever order they are dispatched in.
	inputFirst bool
}

func (e *fakeEnv) now() time.Duration            { return e.clock }
func (e *fakeEnv) modifiers() platform.Modifiers { return e.mods }
func (e *fakeEnv) altGrLayout() bool             { return e.altGr }
func (e *fakeEnv) lang() uint16                  { return e.language }
func (e *fakeEnv) scancodeOf(vk uint32) uint32   { return 0 }
func (e *fakeEnv) messageTime() uint32           { return e.curTime }
func (e *fakeEnv) keyDown(vk uint32) bool        { return e.down[vk] }
func (e *fakeEnv) peekKey() (keyMsg, bool) {
	for _, m := range e.queue {
		if m.message >= wmKeyDown && m.message <= wmSysKeyUp {
			return m, true
		}
	}
	return keyMsg{}, false
}

func (e *fakeEnv) peekChar() (keyMsg, bool) {
	for _, m := range e.queue {
		if isCharMsg(m.message) {
			return m, true
		}
	}
	return keyMsg{}, false
}

func isCharMsg(m uint32) bool {
	return m == wmChar || m == wmSysChar || m == wmDeadChar || m == wmSysDeadChar
}
func (e *fakeEnv) char(vk, sc uint32) (string, charKind) {
	l, ok := e.layout[vk]
	if !ok {
		return "", charNone
	}
	if e.mods&platform.ModShift != 0 {
		if l.shifted == "" {
			return "", charNone
		}
		if l.deadShifted {
			return l.shifted, charDead
		}
		return l.shifted, charText
	}
	return l.plain, l.kind
}

// usIntl is a small slice of the US-International layout: the quote key is a
// dead acute accent, E and X are letters, 2 types "2" and "@" (Shift) and
// Q types "q" (AltGr+Q is not modelled).
func usIntl() map[uint32]layoutKey {
	return map[uint32]layoutKey{
		'A':       {plain: "a", shifted: "A"},
		'E':       {plain: "e", shifted: "E"},
		'X':       {plain: "x", shifted: "X"},
		'2':       {plain: "2", shifted: "@"},
		vkSpace:   {plain: " ", shifted: " "},
		0xDE:      {plain: "´", shifted: "¨", kind: charDead, deadShifted: true}, // VK_OEM_7
		vkDecimal: {plain: ".", shifted: "."},
	}
}

func lp(sc uint32, repeat, up, alt bool) uintptr {
	v := uintptr(1) | uintptr(sc&0xFF)<<16
	if sc&0xFF00 != 0 {
		v |= 1 << 24
	}
	if alt {
		v |= 1 << 29
	}
	if repeat || up {
		v |= 1 << 30
	}
	if up {
		v |= 1 << 31
	}
	return v
}

type tkey struct {
	msg    uint32
	wparam uintptr
	lparam uintptr
	time   uint32
}

func kd(vk, sc uint32) tkey     { return tkey{wmKeyDown, uintptr(vk), lp(sc, false, false, false), 0} }
func kdr(vk, sc uint32) tkey    { return tkey{wmKeyDown, uintptr(vk), lp(sc, true, false, false), 0} }
func ku(vk, sc uint32) tkey     { return tkey{wmKeyUp, uintptr(vk), lp(sc, true, true, false), 0} }
func skd(vk, sc uint32) tkey    { return tkey{wmSysKeyDown, uintptr(vk), lp(sc, false, false, true), 0} }
func sku(vk, sc uint32) tkey    { return tkey{wmSysKeyUp, uintptr(vk), lp(sc, true, true, true), 0} }
func ch(r rune) tkey            { return tkey{wmChar, uintptr(r), 0, 0} }
func sch(r rune) tkey           { return tkey{wmSysChar, uintptr(r), 0, 0} }
func dead(r rune) tkey          { return tkey{wmDeadChar, uintptr(r), 0, 0} }
func (k tkey) at(t uint32) tkey { k.time = t; return k }

type harness struct {
	t       *testing.T
	env     *fakeEnv
	kt      *keyTranslator
	got     []platform.KeyEvent
	pass    []bool // result of each down that asked for DefWindowProc
	orphans int
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, env: &fakeEnv{layout: usIntl(), down: map[uint32]bool{}}}
	h.kt = newKeyTranslator(func(e platform.KeyEvent) { h.got = append(h.got, e) })
	return h
}

// feed delivers the messages as the message loop would: each one is removed
// from the queue before its handler runs, and the handler can peek at the
// rest.
func (h *harness) feed(msgs ...tkey) {
	h.t.Helper()
	for i, m := range msgs {
		h.env.queue = nil
		var chars []keyMsg
		for _, n := range msgs[i+1:] {
			km := keyMsg{message: n.msg, wparam: n.wparam, lparam: n.lparam, time: n.time}
			if h.env.inputFirst && isCharMsg(n.msg) {
				chars = append(chars, km)
				continue
			}
			h.env.queue = append(h.env.queue, km)
		}
		h.env.queue = append(h.env.queue, chars...)
		// Only key messages are in the range the real peek filters on.
		h.env.curTime = m.time
		h.env.clock += time.Millisecond
		switch m.msg {
		case wmKeyDown, wmSysKeyDown:
			if h.kt.down(h.env, m.msg, m.wparam, m.lparam) {
				h.pass = append(h.pass, true)
			}
		case wmKeyUp, wmSysKeyUp:
			h.kt.up(h.env, m.msg, m.wparam, m.lparam)
		case wmChar, wmSysChar:
			if !h.kt.char(h.env, m.wparam) {
				h.orphans++
			}
		case wmDeadChar, wmSysDeadChar:
			if !h.kt.deadChar(h.env) {
				h.orphans++
			}
		}
	}
}

func (h *harness) take() []platform.KeyEvent {
	g := h.got
	h.got = nil
	return g
}

type want struct {
	action platform.KeyAction
	code   platform.KeyCode
	key    platform.Key
	text   string
	mods   platform.Modifiers
	repeat bool
}

func (h *harness) expect(ws ...want) {
	h.t.Helper()
	got := h.take()
	if len(got) != len(ws) {
		h.t.Fatalf("got %d events, want %d:\n%+v", len(got), len(ws), got)
	}
	for i, w := range ws {
		g := got[i]
		if g.Action != w.action || g.Code != w.code || g.Key != w.key || g.Text != w.text || g.Mods != w.mods || g.Repeat != w.repeat {
			h.t.Errorf("event %d = {%v %v key %q text %q mods %v repeat %v}, want {%v %v key %q text %q mods %v repeat %v}",
				i, g.Action, g.Code, g.Key, g.Text, g.Mods, g.Repeat, w.action, w.code, w.key, w.text, w.mods, w.repeat)
		}
		if g.Location != g.Code.Location() {
			h.t.Errorf("event %d: location %v for %v", i, g.Location, g.Code)
		}
	}
}

func press(c platform.KeyCode, k platform.Key, text string, mods platform.Modifiers) want {
	return want{platform.KeyPress, c, k, text, mods, false}
}
func rep(c platform.KeyCode, k platform.Key, text string, mods platform.Modifiers) want {
	return want{platform.KeyPress, c, k, text, mods, true}
}
func rel(c platform.KeyCode, k platform.Key, mods platform.Modifiers) want {
	return want{platform.KeyRelease, c, k, "", mods, false}
}

const (
	psA, psE, psX, ps2, psQuote = 0x1E, 0x12, 0x2D, 0x03, 0x28
	psLShift, psLCtrl, psLAlt   = 0x2A, 0x1D, 0x38
	psRAlt                      = 0xE038
	psEnter, psSpace, psEsc     = 0x1C, 0x39, 0x01
)

func TestKeyTypesText(t *testing.T) {
	h := newHarness(t)
	h.feed(kd('A', psA), ch('a'), ku('A', psA))
	h.expect(
		press(platform.CodeKeyA, "a", "a", 0),
		rel(platform.CodeKeyA, "a", 0),
	)

	// Shift is part of the key and of Mods; the press of Shift itself has
	// Shift in Mods (the state after the event), its release does not.
	h.env.mods = platform.ModShift
	h.feed(kd(vkShift, psLShift), kd('A', psA), ch('A'), ku('A', psA))
	h.env.mods = 0
	h.feed(ku(vkShift, psLShift))
	h.expect(
		press(platform.CodeShiftLeft, "Shift", "", platform.ModShift),
		press(platform.CodeKeyA, "A", "A", platform.ModShift),
		rel(platform.CodeKeyA, "A", platform.ModShift),
		rel(platform.CodeShiftLeft, "Shift", 0),
	)
}

func TestControlCharactersAreNotText(t *testing.T) {
	h := newHarness(t)
	// Enter, Tab, Backspace and Escape make characters that never reach
	// Text; the named keys carry their meaning in Key.
	h.feed(kd(vkReturn, psEnter), ch('\r'), ku(vkReturn, psEnter))
	h.feed(kd(vkBack, 0x0E), ch('\b'), ku(vkBack, 0x0E))
	h.feed(kd(vkEscape, psEsc), ch(0x1B), ku(vkEscape, psEsc))
	h.feed(kd(vkTab, 0x0F), ch('\t'), ku(vkTab, 0x0F))
	h.expect(
		press(platform.CodeEnter, "Enter", "", 0), rel(platform.CodeEnter, "Enter", 0),
		press(platform.CodeBackspace, "Backspace", "", 0), rel(platform.CodeBackspace, "Backspace", 0),
		press(platform.CodeEscape, "Escape", "", 0), rel(platform.CodeEscape, "Escape", 0),
		press(platform.CodeTab, "Tab", "", 0), rel(platform.CodeTab, "Tab", 0),
	)
	if h.orphans != 0 {
		t.Errorf("%d character messages were left over", h.orphans)
	}
	// A character message without a key press (an IME result) is not ours.
	h.feed(ch('x'))
	if h.orphans != 1 {
		t.Errorf("a character without a key press was taken")
	}
}

func TestDeadKeyThenEnterTypesTheAccent(t *testing.T) {
	h := newHarness(t)
	// Windows types the pending accent before the control character of
	// Enter, Tab or Backspace: the accent is text, the Enter is the key.
	h.feed(kd(0xDE, psQuote), dead('´'), ku(0xDE, psQuote), kd(vkReturn, psEnter), ch('´'), ch('\r'), ku(vkReturn, psEnter))
	h.expect(
		press(platform.CodeQuote, "Dead", "", 0),
		rel(platform.CodeQuote, "Dead", 0),
		press(platform.CodeEnter, "Enter", "´", 0),
		rel(platform.CodeEnter, "Enter", 0),
	)
}

func TestDeadKeyReleaseIsDeadWhileTheKernelHoldsTheAccent(t *testing.T) {
	h := newHarness(t)
	// While the dead key is pending in the kernel, the layout answers a
	// question about the same key with the accent. The release is still Dead.
	h.feed(kd(0xDE, psQuote), dead('´'))
	h.env.layout[0xDE] = layoutKey{plain: "´", shifted: "¨"}
	h.feed(ku(0xDE, psQuote))
	h.expect(
		press(platform.CodeQuote, "Dead", "", 0),
		rel(platform.CodeQuote, "Dead", 0),
	)
}

func TestCtrlIgnoredForKeyAndText(t *testing.T) {
	h := newHarness(t)
	h.env.mods = platform.ModCtrl
	// Windows turns Ctrl+A into U+0001.
	h.feed(kd(vkControl, psLCtrl), kd('A', psA), ch(1), ku('A', psA), ku(vkControl, psLCtrl))
	got := h.take()
	if len(got) != 4 {
		t.Fatalf("%d events: %+v", len(got), got)
	}
	if e := got[1]; e.Code != platform.CodeKeyA || e.Key != "a" || e.Text != "a" || e.Mods != platform.ModCtrl {
		t.Errorf("Ctrl+A press = %+v", e)
	}
	if e := got[2]; e.Key != "a" || e.Text != "" {
		t.Errorf("Ctrl+A release = %+v", e)
	}
}

func TestKeyWithoutCharacterMessage(t *testing.T) {
	h := newHarness(t)
	// A window that does not translate messages gets no WM_CHAR: the
	// layout answers.
	h.feed(kd('A', psA))
	h.expect(press(platform.CodeKeyA, "a", "a", 0))
}

func TestDeadKeyComposition(t *testing.T) {
	h := newHarness(t)
	// ´ then e: the dead key has Key "Dead" and no text; the next press
	// types "é", which is its Key and its Text.
	h.feed(kd(0xDE, psQuote), dead('´'), ku(0xDE, psQuote), kd('E', psE), ch('é'), ku('E', psE))
	h.expect(
		press(platform.CodeQuote, "Dead", "", 0),
		rel(platform.CodeQuote, "Dead", 0),
		press(platform.CodeKeyE, "é", "é", 0),
		rel(platform.CodeKeyE, "e", 0),
	)
}

func TestCharactersAreFoundBehindQueuedKeyMessages(t *testing.T) {
	h := newHarness(t)
	h.env.inputFirst = true
	// Injected keys put the release in the queue before the dead key's
	// press is dispatched, and a scan of the input queue finds it before the
	// characters that TranslateMessage posted.
	h.feed(kd(0xDE, psQuote), dead('´'), ku(0xDE, psQuote), kd('X', psX), ch('´'), ch('x'), ku('X', psX))
	h.expect(
		press(platform.CodeQuote, "Dead", "", 0),
		rel(platform.CodeQuote, "Dead", 0),
		press(platform.CodeKeyX, "x", "´x", 0),
		rel(platform.CodeKeyX, "x", 0),
	)
}

func TestDeadKeyThatDoesNotCompose(t *testing.T) {
	h := newHarness(t)
	// ´ then x: Windows types the accent and then the letter. The press of
	// x types both and its Key is the layout's "x".
	h.feed(kd(0xDE, psQuote), dead('´'), ku(0xDE, psQuote), kd('X', psX), ch('´'), ch('x'), ku('X', psX))
	h.expect(
		press(platform.CodeQuote, "Dead", "", 0),
		rel(platform.CodeQuote, "Dead", 0),
		press(platform.CodeKeyX, "x", "´x", 0),
		rel(platform.CodeKeyX, "x", 0),
	)
}

func TestDeadKeyTwiceTypesTheAccent(t *testing.T) {
	h := newHarness(t)
	// The kernel consumed the first ´ before the second press reaches the
	// window: the layout says "dead" again, but the character is typed.
	h.feed(kd(0xDE, psQuote), dead('´'), ku(0xDE, psQuote), kd(0xDE, psQuote), ch('´'), ku(0xDE, psQuote))
	h.expect(
		press(platform.CodeQuote, "Dead", "", 0),
		rel(platform.CodeQuote, "Dead", 0),
		press(platform.CodeQuote, "´", "´", 0),
		rel(platform.CodeQuote, "Dead", 0),
	)
}

func TestSurrogatePairs(t *testing.T) {
	h := newHarness(t)
	// A character outside the BMP arrives as two WM_CHAR code units.
	h.feed(kd('A', psA), tkey{wmChar, 0xD83D, 0, 0}, tkey{wmChar, 0xDE00, 0, 0}, ku('A', psA))
	got := h.take()
	if got[0].Text != "😀" || got[0].Key != "😀" {
		t.Errorf("surrogate pair press = key %q text %q", got[0].Key, got[0].Text)
	}
}

func TestRepeat(t *testing.T) {
	h := newHarness(t)
	h.feed(kd('A', psA), ch('a'), kdr('A', psA), ch('a'), kdr('A', psA), ch('a'), ku('A', psA))
	h.expect(
		press(platform.CodeKeyA, "a", "a", 0),
		rep(platform.CodeKeyA, "a", "a", 0),
		rep(platform.CodeKeyA, "a", "a", 0),
		rel(platform.CodeKeyA, "a", 0),
	)
}

func TestModifierRepeatIsDropped(t *testing.T) {
	h := newHarness(t)
	h.env.mods = platform.ModShift
	h.feed(kd(vkShift, psLShift), kdr(vkShift, psLShift), kdr(vkShift, psLShift))
	h.expect(press(platform.CodeShiftLeft, "Shift", "", platform.ModShift))
	// Holding a, then Shift: a's later repeats report the current state.
	h.env.mods = 0
	h.feed(ku(vkShift, psLShift), kd('A', psA), ch('a'))
	h.env.mods = platform.ModShift
	h.feed(kd(vkShift, psLShift), kdr('A', psA), ch('A'))
	got := h.take()
	last := got[len(got)-1]
	if !last.Repeat || last.Key != "A" || last.Text != "A" || last.Mods != platform.ModShift {
		t.Errorf("repeat after Shift = %+v", last)
	}
}

func TestReleaseOfKeyHeldAtFocusIsDropped(t *testing.T) {
	h := newHarness(t)
	// The press happened before the window had focus: its repeats and its
	// release are not delivered.
	h.feed(kdr('A', psA), ch('a'), ku('A', psA))
	h.expect()
}

func TestFocusLostReleasesHeldKeys(t *testing.T) {
	h := newHarness(t)
	h.feed(kd(vkShift, psLShift), kd('A', psA), ch('a'))
	h.take()
	h.kt.focusLost(h.env)
	got := h.take()
	if len(got) != 2 || got[0].Code != platform.CodeShiftLeft || got[1].Code != platform.CodeKeyA {
		t.Fatalf("synthetic releases = %+v", got)
	}
	for _, e := range got {
		if e.Action != platform.KeyRelease || !e.Synthetic || e.Text != "" || e.Mods != 0 {
			t.Errorf("synthetic release %+v", e)
		}
	}
	if got[1].Key != "a" {
		t.Errorf("synthetic release keeps the Key of its press: %q", got[1].Key)
	}
	// Nothing is held any more.
	h.feed(ku('A', psA))
	h.expect()
	h.kt.focusLost(h.env)
	h.expect()
}

func TestAltGr(t *testing.T) {
	h := newHarness(t)
	h.env.altGr = true
	h.env.layout['Q'] = layoutKey{plain: "q", shifted: "Q"}
	// AltGr arrives as a left Ctrl and a right Alt with the same time. The
	// fake Ctrl produces nothing; AltGr is neither Ctrl nor Alt.
	h.feed(
		kd(vkControl, psLCtrl).at(100), kd(vkMenu, psRAlt).at(100),
		kd('Q', 0x10), ch('@'), ku('Q', 0x10),
		ku(vkControl, psLCtrl).at(200), ku(vkMenu, psRAlt).at(200),
	)
	h.expect(
		press(platform.CodeAltRight, "AltGraph", "", 0),
		press(platform.CodeKeyQ, "@", "@", 0),
		rel(platform.CodeKeyQ, "q", 0),
		rel(platform.CodeAltRight, "AltGraph", 0),
	)
}

func TestRealCtrlWithRightAltIsNotAltGr(t *testing.T) {
	h := newHarness(t)
	h.env.altGr = true
	// The two presses have different times: Ctrl is a real key.
	h.env.mods = platform.ModCtrl
	h.feed(kd(vkControl, psLCtrl).at(100), kd(vkMenu, psRAlt).at(180))
	h.expect(
		press(platform.CodeControlLeft, "Control", "", platform.ModCtrl),
		press(platform.CodeAltRight, "AltGraph", "", platform.ModCtrl),
	)
}

func TestRightAltOnLayoutWithoutAltGr(t *testing.T) {
	h := newHarness(t)
	h.env.mods = platform.ModCtrl | platform.ModAlt
	h.feed(kd(vkControl, psLCtrl).at(100), kd(vkMenu, psRAlt).at(100))
	got := h.take()
	if len(got) != 2 || got[0].Code != platform.CodeControlLeft || got[1].Key != platform.KeyAlt {
		t.Errorf("Ctrl and Alt on a layout without AltGr = %+v", got)
	}
}

func TestSystemKeys(t *testing.T) {
	h := newHarness(t)
	// Alt+F4 and Alt+Space are for DefWindowProc, and the app sees neither
	// key; a lone Alt press is reported (and swallowed by the caller).
	h.env.mods = platform.ModAlt
	h.feed(skd(vkMenu, psLAlt), skd(vkF4, 0x3E), sku(vkF4, 0x3E), skd(vkSpace, psSpace))
	got := h.take()
	if len(got) != 1 || got[0].Code != platform.CodeAltLeft || got[0].Key != "Alt" {
		t.Errorf("events = %+v", got)
	}
	if len(h.pass) != 2 {
		t.Errorf("%d messages passed to DefWindowProc, want 2 (Alt+F4, Alt+Space)", len(h.pass))
	}
	// Alt+X: the key and its text (WM_SYSCHAR) are reported.
	h.feed(skd('X', psX), sch('x'), sku('X', psX))
	h.expect(
		press(platform.CodeKeyX, "x", "x", platform.ModAlt),
		rel(platform.CodeKeyX, "x", platform.ModAlt),
	)
}

func TestSwallowSysCommand(t *testing.T) {
	for _, tc := range []struct {
		name           string
		wparam, lparam uintptr
		want           bool
	}{
		{"lone Alt or F10", scKeyMenu, 0, true},
		{"Alt and a mnemonic", scKeyMenu, 'f', true},
		{"the low bits belong to the system", scKeyMenu | 0xF, 0, true},
		{"Alt+Space opens the window menu", scKeyMenu, ' ', false},
		{"mouse menu", 0xF090, 0, false},
		{"close", 0xF060, 0, false},
		{"maximize", 0xF030, 0, false},
	} {
		if got := swallowSysCommand(tc.wparam, tc.lparam); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if !isSystemShortcut(wmSysKeyDown, vkF4, lp(0x3E, false, false, true)) || !isSystemShortcut(wmSysKeyDown, vkSpace, lp(0x39, false, false, true)) {
		t.Errorf("Alt+F4 and Alt+Space are system shortcuts")
	}
	if isSystemShortcut(wmKeyDown, vkF4, lp(0x3E, false, false, false)) || isSystemShortcut(wmSysKeyDown, 'X', lp(0x2D, false, false, true)) {
		t.Errorf("F4 alone and Alt+X are not")
	}
}

func TestNumpad(t *testing.T) {
	h := newHarness(t)
	// NumLock on: VK_NUMPAD1 types "1"; the layout has no divide key, so
	// the fallback applies.
	h.env.layout[vkNumpad0+1] = layoutKey{plain: "1", shifted: "1"}
	h.feed(kd(vkNumpad0+1, 0x4F), ch('1'), ku(vkNumpad0+1, 0x4F))
	h.feed(kd(vkDivide, 0xE035), ch('/'), ku(vkDivide, 0xE035))
	// NumLock off: the same physical key is End, without the extended flag.
	h.feed(kd(vkEnd, 0x4F), ku(vkEnd, 0x4F))
	// The dedicated End key is extended.
	h.feed(kd(vkEnd, 0xE04F), ku(vkEnd, 0xE04F))
	h.expect(
		press(platform.CodeNumpad1, "1", "1", 0), rel(platform.CodeNumpad1, "1", 0),
		press(platform.CodeNumpadDivide, "/", "/", 0), rel(platform.CodeNumpadDivide, "/", 0),
		press(platform.CodeNumpad1, "End", "", 0), rel(platform.CodeNumpad1, "End", 0),
		press(platform.CodeEnd, "End", "", 0), rel(platform.CodeEnd, "End", 0),
	)
}

func TestNumLockAndPauseFollowThePhysicalKey(t *testing.T) {
	h := newHarness(t)
	// Windows swaps the virtual keys of NumLock and Pause while Ctrl is held.
	h.feed(kd(vkPause, 0xE045), ku(vkPause, 0xE045), kd(vkNumLock, 0x45), ku(vkNumLock, 0x45))
	h.expect(
		press(platform.CodeNumLock, "NumLock", "", 0), rel(platform.CodeNumLock, "NumLock", 0),
		press(platform.CodePause, "Pause", "", 0), rel(platform.CodePause, "Pause", 0),
	)
}

func TestUnknownKeyKeepsItsScancode(t *testing.T) {
	h := newHarness(t)
	h.feed(kd(0xFF, 0xE0AB), ku(0xFF, 0xE0AB))
	got := h.take()
	if len(got) != 2 {
		t.Fatalf("%d events", len(got))
	}
	if n, ok := got[0].Code.Native(); !ok || n != 0xE0AB || got[0].Key != platform.KeyUnidentified {
		t.Errorf("unknown key = %#v key %q", got[0].Code, got[0].Key)
	}
	if got[0].Code != got[1].Code {
		t.Errorf("press and release of an unknown key have different codes")
	}
}

func TestFakeShiftIsDropped(t *testing.T) {
	h := newHarness(t)
	h.feed(kd(vkShift, 0xE02A), kd(vkInsert, 0xE052), ku(vkInsert, 0xE052), ku(vkShift, 0xE0AA&0xE0FF))
	got := h.take()
	if len(got) != 2 || got[0].Code != platform.CodeInsert {
		t.Errorf("events = %+v", got)
	}
}

func TestInjectedUnicodeText(t *testing.T) {
	h := newHarness(t)
	// SendInput with KEYEVENTF_UNICODE (touch keyboards, remote desktops):
	// VK_PACKET with the character in the high word.
	wp := uintptr(vkPacket) | uintptr('ñ')<<16
	h.feed(tkey{wmKeyDown, wp, lp(0, false, false, false), 0}, ch('ñ'), tkey{wmKeyUp, wp, lp(0, true, true, false), 0})
	got := h.take()
	if len(got) != 2 || got[0].Key != "ñ" || got[0].Text != "ñ" || got[0].Code.Identified() {
		t.Errorf("events = %+v", got)
	}
	if got[0].Code != got[1].Code {
		t.Errorf("press and release of injected text have different codes")
	}
}

func TestReleaseStale(t *testing.T) {
	h := newHarness(t)
	h.feed(skd(vkMenu, psLAlt), kd('A', psA), ch('a'))
	h.take()
	h.env.down[vkMenu] = true // Alt is still down, a is not (a modal loop ate its release)
	h.kt.releaseStale(h.env)
	got := h.take()
	if len(got) != 1 || got[0].Code != platform.CodeKeyA || !got[0].Synthetic || got[0].Action != platform.KeyRelease {
		t.Errorf("stale releases = %+v", got)
	}
	// The later real release is dropped (already released).
	h.feed(ku('A', psA))
	h.expect()
}

func TestKeyFromText(t *testing.T) {
	for _, tc := range []struct {
		text, fallback string
		want           platform.Key
	}{
		{"a", "", "a"},
		{"é", "e", "é"},
		{"e\u0301", "e", "e\u0301"}, // one cluster
		{"´x", "x", "x"},
		{"´x", "", "´"},
		{"👍🏽", "", "👍🏽"},
		{"", "q", "q"},
		{"", "", ""},
	} {
		if got := keyFromText(tc.text, tc.fallback); got != tc.want {
			t.Errorf("keyFromText(%q, %q) = %q, want %q", tc.text, tc.fallback, got, tc.want)
		}
	}
}

func TestStripControls(t *testing.T) {
	for in, want := range map[string]string{
		"a": "a", "\r": "", "\t": "", "\x7f": "", "a\x00b": "ab", "\u0085x": "x", "é": "é",
	} {
		if got := stripControls(in); got != want {
			t.Errorf("stripControls(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScancodeFromLParam(t *testing.T) {
	if sc := scancodeFromLParam(lp(0x1E, false, false, false)); sc != 0x1E {
		t.Errorf("A = %#x", sc)
	}
	if sc := scancodeFromLParam(lp(0xE04B, false, false, false)); sc != 0xE04B {
		t.Errorf("Left = %#x", sc)
	}
	if !keyRepeat(lp(0x1E, true, false, false)) || keyRepeat(lp(0x1E, false, false, false)) {
		t.Errorf("repeat bit")
	}
}

func TestCodeFromScancode(t *testing.T) {
	for sc, want := range map[uint32]platform.KeyCode{
		0x1E: platform.CodeKeyA, 0x10: platform.CodeKeyQ, 0x2C: platform.CodeKeyZ, 0x29: platform.CodeBackquote,
		0x0B: platform.CodeDigit0, 0x56: platform.CodeIntlBackslash, 0x73: platform.CodeIntlRo,
		0xE038: platform.CodeAltRight, 0x38: platform.CodeAltLeft, 0xE01D: platform.CodeControlRight,
		0xE05B: platform.CodeMetaLeft, 0xE047: platform.CodeHome, 0x47: platform.CodeNumpad7,
		0xE01C: platform.CodeNumpadEnter, 0x1C: platform.CodeEnter, 0xE035: platform.CodeNumpadDivide,
		0xE045: platform.CodeNumLock, 0x45: platform.CodePause, 0x57: platform.CodeF11, 0x76: platform.CodeF24,
		0xE037: platform.CodePrintScreen, 0xE020: platform.CodeAudioVolumeMute,
	} {
		if got := codeFromScancode(sc); got != want {
			t.Errorf("codeFromScancode(%#x) = %v, want %v", sc, got, want)
		}
	}
	if c := codeFromScancode(0xE0AB); c.Identified() {
		t.Errorf("0xE0AB identified as %v", c)
	}
	if n, ok := codeFromScancode(0xE0AB).Native(); !ok || n != 0xE0AB {
		t.Errorf("unidentified keeps its scan code: %#x, %v", n, ok)
	}
	if codeFromScancode(0) != platform.CodeUnidentified {
		t.Errorf("scan code 0")
	}
}

// TestScancodesAreUnique: two scan codes may name the same key (Pause,
// PrintScreen, Lang keys), but a key must not be a typo of another.
func TestScancodesCoverTheKeyboard(t *testing.T) {
	seen := map[platform.KeyCode][]uint32{}
	for sc, c := range scancodes {
		seen[c] = append(seen[c], sc)
	}
	for c, scs := range seen {
		slices.Sort(scs)
		if len(scs) > 2 {
			t.Errorf("%v has %d scan codes: %#x", c, len(scs), scs)
		}
	}
	for _, c := range []platform.KeyCode{platform.CodeKeyA, platform.CodeKeyZ, platform.CodeDigit0, platform.CodeSpace, platform.CodeEnter, platform.CodeEscape, platform.CodeF12} {
		if _, ok := seen[c]; !ok {
			t.Errorf("no scan code for %v", c)
		}
	}
}

func TestNamedKey(t *testing.T) {
	for _, tc := range []struct {
		vk    uint32
		code  platform.KeyCode
		altGr bool
		lang  uint16
		want  platform.Key
		ok    bool
	}{
		{vkShift, platform.CodeShiftRight, false, 0, "Shift", true},
		{vkControl, platform.CodeControlLeft, false, 0, "Control", true},
		{vkMenu, platform.CodeAltLeft, true, 0, "Alt", true},
		{vkMenu, platform.CodeAltRight, true, 0, "AltGraph", true},
		{vkMenu, platform.CodeAltRight, false, 0, "Alt", true},
		{vkRMenu, platform.CodeAltRight, true, 0, "AltGraph", true},
		{vkLWin, platform.CodeMetaLeft, false, 0, "Meta", true},
		{vkF1, platform.CodeF1, false, 0, "F1", true},
		{vkF1 + 11, platform.CodeF12, false, 0, "F12", true},
		{vkF24, platform.CodeF24, false, 0, "F24", true},
		{vkLeft, platform.CodeArrowLeft, false, 0, "ArrowLeft", true},
		{vkPrior, platform.CodePageUp, false, 0, "PageUp", true},
		{vkNext, platform.CodePageDown, false, 0, "PageDown", true},
		{vkSnapshot, platform.CodePrintScreen, false, 0, "PrintScreen", true},
		{vkApps, platform.CodeContextMenu, false, 0, "ContextMenu", true},
		{vkCapital, platform.CodeCapsLock, false, 0, "CapsLock", true},
		{vkKana, platform.CodeKanaMode, false, langJapanese, "KanaMode", true},
		{vkKana, platform.CodeLang1, false, langKorean, "HangulMode", true},
		{vkKanji, platform.CodeLang2, false, langKorean, "HanjaMode", true},
		{vkKanji, platform.CodeLang2, false, langJapanese, "KanjiMode", true},
		{vkKana, platform.CodeLang1, false, 0x09, "", false},
		{vkConvert, platform.CodeConvert, false, 0, "Convert", true},
		{vkNonConvert, platform.CodeNonConvert, false, 0, "NonConvert", true},
		{0xB3, platform.CodeMediaPlayPause, false, 0, "MediaPlayPause", true},
		{0xAF, platform.CodeAudioVolumeUp, false, 0, "AudioVolumeUp", true},
		{0xA6, platform.CodeBrowserBack, false, 0, "BrowserBack", true},
		{vkProcessKey, platform.CodeKeyA, false, 0, "Process", true},
		{'A', platform.CodeKeyA, false, 0, "", false},
		{vkSpace, platform.CodeSpace, false, 0, "", false},
		{vkNumpad0 + 5, platform.CodeNumpad5, false, 0, "", false},
		{vkClear, platform.CodeNumpad5, false, 0, "Clear", true},
		{0xBA, platform.CodeSemicolon, false, 0, "", false},
	} {
		got, ok := namedKey(tc.vk, tc.code, tc.altGr, tc.lang)
		if got != tc.want || ok != tc.ok {
			t.Errorf("namedKey(%#x, %v, altGr %v, lang %#x) = %q, %v; want %q, %v", tc.vk, tc.code, tc.altGr, tc.lang, got, ok, tc.want, tc.ok)
		}
	}
	// Every function key vk names a function key of the platform set.
	for i := range 24 {
		k, ok := namedKey(uint32(vkF1+i), platform.CodeUnidentified, false, 0)
		if !ok || !k.IsNamed() {
			t.Errorf("F%d: %q, %v", i+1, k, ok)
		}
	}
}

func TestModifiersFromState(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		shift, lctrl, rctrl, lalt, ralt, super, altGr bool
		pressed                                       bool // the window saw the press of the left Ctrl
		want                                          platform.Modifiers
	}{
		{name: "none"},
		{name: "ctrl", lctrl: true, want: platform.ModCtrl},
		{name: "right ctrl", rctrl: true, want: platform.ModCtrl},
		{name: "alt", lalt: true, want: platform.ModAlt},
		{name: "right alt, no AltGr", ralt: true, want: platform.ModAlt},
		{name: "right alt, AltGr layout", ralt: true, altGr: true},
		{name: "AltGr with its fake Ctrl", lctrl: true, ralt: true, altGr: true},
		{name: "fake Ctrl without AltGr layout", lctrl: true, ralt: true, want: platform.ModCtrl | platform.ModAlt},
		{name: "real Ctrl, then AltGr", lctrl: true, ralt: true, altGr: true, pressed: true, want: platform.ModCtrl},
		{name: "AltGr and right Ctrl", rctrl: true, ralt: true, altGr: true, want: platform.ModCtrl},
		{name: "AltGr and left Alt", lalt: true, lctrl: true, ralt: true, altGr: true, want: platform.ModAlt},
		{name: "all", shift: true, lctrl: true, lalt: true, super: true, want: platform.ModShift | platform.ModCtrl | platform.ModAlt | platform.ModSuper},
	} {
		if got := modifiersFromState(tc.shift, tc.lctrl, tc.rctrl, tc.lalt, tc.ralt, tc.super, tc.altGr, tc.pressed); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
