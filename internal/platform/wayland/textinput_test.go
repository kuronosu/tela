//go:build linux

package wayland

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// zwp_text_input_v3 event opcodes.
const (
	tiEnter          = 0
	tiLeave          = 1
	tiPreedit        = 2
	tiCommitString   = 3
	tiDeleteSurround = 4
	tiDone           = 5
)

// tiGlobals is keyboardGlobals with zwp_text_input_manager_v3 at version
// 2 (the backend binds 1).
func tiGlobals() []fakeGlobal {
	return append(keyboardGlobals(), fakeGlobal{iface: "zwp_text_input_manager_v3", version: 2})
}

// tiSetup opens a focused window and returns the text input's id.
func tiSetup(t *testing.T, h *harness) (ti uint32, w *Window) {
	t.Helper()
	_, w = kbSetup(t, h, "us", 0, 0)
	ti = h.fc.id("zwp_text_input_v3")
	if ti == 0 {
		t.Fatal("no zwp_text_input_v3 was created")
	}
	return ti, w
}

// tiRequests returns the text input requests up to and including the next
// commit.
func tiRequests(h *harness) []freq {
	h.t.Helper()
	var out []freq
	for _, r := range h.fc.until("zwp_text_input_v3.commit") {
		if r.iface == "zwp_text_input_v3" {
			out = append(out, r)
		}
	}
	return out
}

func tiNames(rs []freq) string {
	var n []string
	for _, r := range rs {
		n = append(n, r.name)
	}
	return strings.Join(n, " ")
}

// tiCommits returns the commit count of the seat's text input.
func tiCommits(h *harness) (n uint32, stale int) {
	h.ui(func() { n, stale = h.b.seats[0].ti.commits, h.b.seats[0].ti.stale })
	return n, stale
}

// preeditMsg encodes preedit_string; a nil text is the protocol's null.
func preeditMsg(ti uint32, text *string, begin, end int32) []byte {
	if text == nil {
		return event(ti, tiPreedit, uint32(0), begin, end)
	}
	return event(ti, tiPreedit, *text, begin, end)
}

func ptr(s string) *string { return &s }

func expectSession(t *testing.T, h *harness, active bool) {
	t.Helper()
	if e := expect[platform.IMESessionEvent](h); e.Active != active {
		t.Errorf("IMESessionEvent{Active: %v}, want %v", e.Active, active)
	}
}

func checkIME(t *testing.T, got, want platform.IMEEvent) {
	t.Helper()
	if got != want {
		t.Errorf("IMEEvent %+v, want %+v", got, want)
	}
}

var tiState = platform.TextInputState{
	CursorArea:  platform.Rect{X: 10.5, Y: 20.25, W: 0, H: 18},
	Purpose:     platform.TextPurposeEmail,
	Surrounding: &platform.SurroundingText{Text: "héllo", Cursor: 3, Anchor: 1},
}

func TestTextInputEnableAndUpdates(t *testing.T) {
	h := newHarness(t, tiGlobals())
	ti, w := tiSetup(t, h)
	h.ui(func() {
		if v := h.b.textInputManager.Version(); v != 1 {
			t.Errorf("zwp_text_input_manager_v3 bound at version %d, want 1", v)
		}
	})

	// A state before the text input entered the window: nothing is sent.
	h.ui(func() { w.SetTextInput(&tiState) })
	h.fc.quiet(50*time.Millisecond, "zwp_text_input_v3.commit")

	// Enter: enable with the whole state, cause "other", one commit.
	h.fc.send(ti, tiEnter, h.fc.id("wl_surface"))
	rs := tiRequests(h)
	if got, want := tiNames(rs), "enable set_surrounding_text set_content_type set_cursor_rectangle set_text_change_cause commit"; got != want {
		t.Fatalf("enable requests: %s, want %s", got, want)
	}
	if r := rs[1]; r.args[0] != "héllo" || r.i(1) != 3 || r.i(2) != 1 {
		t.Errorf("set_surrounding_text %v", r.args)
	}
	if r := rs[2]; r.u(0) != uint32(protocols.ZwpTextInputV3ContentHintNone) || r.u(1) != uint32(protocols.ZwpTextInputV3ContentPurposeEmail) {
		t.Errorf("set_content_type %v", r.args)
	}
	// Surface-local logical pixels rounded outward, at least 1 wide.
	if r := rs[3]; r.i(0) != 10 || r.i(1) != 20 || r.i(2) != 1 || r.i(3) != 19 {
		t.Errorf("set_cursor_rectangle %v, want [10 20 1 19]", r.args)
	}
	if r := rs[4]; r.u(0) != uint32(protocols.ZwpTextInputV3ChangeCauseOther) {
		t.Errorf("set_text_change_cause %v", r.args)
	}
	expectSession(t, h, true)
	if n, _ := tiCommits(h); n != 1 {
		t.Errorf("commits = %d, want 1", n)
	}

	// An equal state sends nothing; several changes in one iteration are
	// one commit with only the parts that changed.
	h.ui(func() { w.SetTextInput(tiState.Clone()) })
	h.fc.quiet(50*time.Millisecond, "zwp_text_input_v3.commit")
	h.ui(func() {
		st := tiState.Clone()
		st.CursorArea.X = 30
		w.SetTextInput(st)
		st.CursorArea.X = 40
		w.SetTextInput(st)
	})
	rs = tiRequests(h)
	if got := tiNames(rs); got != "set_cursor_rectangle commit" {
		t.Errorf("cursor move: %s", got)
	} else if rs[0].i(0) != 40 {
		t.Errorf("set_cursor_rectangle %v, want x 40", rs[0].args)
	}
	h.ui(func() {
		st := tiState.Clone()
		st.CursorArea.X = 40
		st.Surrounding.Cursor = 5
		w.SetTextInput(st)
	})
	if got := tiNames(tiRequests(h)); got != "set_surrounding_text commit" {
		t.Errorf("caret move: %s", got)
	}

	// Reset: the whole state again with cause "other".
	h.ui(w.ResetTextInput)
	if got := tiNames(tiRequests(h)); got != "set_surrounding_text set_content_type set_cursor_rectangle set_text_change_cause commit" {
		t.Errorf("reset: %s", got)
	}

	// A new purpose restarts the session.
	h.ui(func() {
		w.SetTextInput(&platform.TextInputState{Purpose: platform.TextPurposePassword, Multiline: true})
	})
	if got := tiNames(tiRequests(h)); got != "disable commit" {
		t.Errorf("restart, first commit: %s", got)
	}
	rs = tiRequests(h)
	if got := tiNames(rs); got != "enable set_content_type set_text_change_cause commit" {
		t.Errorf("restart, second commit: %s", got)
	}
	wantHint := protocols.ZwpTextInputV3ContentHintHiddenText | protocols.ZwpTextInputV3ContentHintSensitiveData | protocols.ZwpTextInputV3ContentHintMultiline
	if r := rs[1]; r.u(0) != uint32(wantHint) || r.u(1) != uint32(protocols.ZwpTextInputV3ContentPurposePassword) {
		t.Errorf("password set_content_type %v", r.args)
	}
	expectSession(t, h, false)
	expectSession(t, h, true)

	// Another field: nil then a state in one iteration restarts too.
	h.ui(func() {
		w.SetTextInput(nil)
		w.SetTextInput(&platform.TextInputState{Purpose: platform.TextPurposePassword, Multiline: true})
	})
	if got := tiNames(tiRequests(h)) + " / " + tiNames(tiRequests(h)); got != "disable commit / enable set_content_type set_text_change_cause commit" {
		t.Errorf("field change: %s", got)
	}
	expectSession(t, h, false)
	expectSession(t, h, true)

	// nil disables.
	h.ui(func() { w.SetTextInput(nil) })
	if got := tiNames(tiRequests(h)); got != "disable commit" {
		t.Errorf("nil: %s", got)
	}
	expectSession(t, h, false)
	if n, _ := tiCommits(h); n != 9 {
		t.Errorf("commits = %d, want 9", n)
	}
	h.ui(w.ResetTextInput) // without a state: nothing
	h.fc.quiet(50*time.Millisecond, "zwp_text_input_v3.commit")

	// Leave ends the session without requests (the compositor would
	// ignore them); enter enables again.
	h.ui(func() { w.SetTextInput(&tiState) })
	tiRequests(h)
	expectSession(t, h, true)
	h.fc.send(ti, tiLeave, h.fc.id("wl_surface"))
	expectSession(t, h, false)
	h.fc.quiet(50*time.Millisecond, "zwp_text_input_v3.commit")
	h.fc.send(ti, tiEnter, h.fc.id("wl_surface"))
	if got := tiNames(tiRequests(h)); !strings.HasPrefix(got, "enable ") {
		t.Errorf("enter again: %s", got)
	}
	expectSession(t, h, true)

	// Closing the window disables before the surface goes away.
	h.ui(w.Close)
	var got []string
	for _, r := range h.fc.until("wl_surface.destroy") {
		if r.iface == "zwp_text_input_v3" {
			got = append(got, r.name)
		}
	}
	if strings.Join(got, " ") != "disable commit" {
		t.Errorf("close: %v", got)
	}
	h.noEvent(50 * time.Millisecond)
}

func TestTextInputComposition(t *testing.T) {
	h := newHarness(t, tiGlobals())
	ti, w := tiSetup(t, h)
	h.ui(func() { w.SetTextInput(&tiState) })
	h.fc.send(ti, tiEnter, h.fc.id("wl_surface"))
	tiRequests(h)
	expectSession(t, h, true)
	serial, _ := tiCommits(h)

	// Preedit updates: applied only at done.
	h.fc.send(ti, tiPreedit, "k", int32(1), int32(1))
	h.noEvent(30 * time.Millisecond)
	h.fc.send(ti, tiDone, serial)
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{Preedit: "k", CursorStart: 1, CursorEnd: 1})
	h.fc.sendRaw(cat(preeditMsg(ti, ptr("漢字"), 0, 6), event(ti, tiDone, serial)))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{Preedit: "漢字", CursorStart: 0, CursorEnd: 6})
	// The same preedit again changes nothing.
	h.fc.sendRaw(cat(preeditMsg(ti, ptr("漢字"), 0, 6), event(ti, tiDone, serial)))
	h.noEvent(30 * time.Millisecond)

	// Commit: the done without preedit_string ends the preedit.
	h.fc.sendRaw(cat(event(ti, tiCommitString, "漢字"), event(ti, tiDone, serial)))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{Commit: "漢字"})
	h.fc.send(ti, tiDone, serial) // nothing pending: nothing to deliver
	h.noEvent(30 * time.Millisecond)

	// Everything in one done, in protocol order, whatever the event order.
	h.fc.sendRaw(cat(
		preeditMsg(ti, ptr("y"), -1, -1),
		event(ti, tiCommitString, "x"),
		event(ti, tiDeleteSurround, uint32(2), uint32(1)),
		event(ti, tiDone, serial),
	))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{DeleteBefore: 2, DeleteAfter: 1, Commit: "x", Preedit: "y", CursorStart: -1, CursorEnd: -1})

	// A cancelled composition: an empty (null) preedit.
	h.fc.sendRaw(cat(preeditMsg(ti, nil, 0, 0), event(ti, tiDone, serial)))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{})

	// What the IME sends is sanitized: invalid UTF-8, a cursor past the
	// end or inside a character.
	h.fc.sendRaw(cat(preeditMsg(ti, ptr("a\xffé"), 5, 9), event(ti, tiCommitString, "\xfe"), event(ti, tiDone, serial)))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{Commit: "�", Preedit: "a�é", CursorStart: 4, CursorEnd: 6})

	// A stale done (computed for a state we have replaced) is applied
	// anyway, and the app's next state is committed without waiting.
	h.ui(func() {
		st := tiState.Clone()
		st.Surrounding.Cursor = 5
		w.SetTextInput(st)
	})
	tiRequests(h)
	h.fc.sendRaw(cat(preeditMsg(ti, ptr("z"), 1, 1), event(ti, tiDone, serial)))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{Preedit: "z", CursorStart: 1, CursorEnd: 1})
	if n, stale := tiCommits(h); n != serial+1 || stale != 1 {
		t.Errorf("commits %d, stale dones %d; want %d, 1", n, stale, serial+1)
	}
	h.ui(func() {
		st := tiState.Clone()
		st.Surrounding.Cursor = 0
		w.SetTextInput(st)
	})
	if got := tiNames(tiRequests(h)); got != "set_surrounding_text commit" {
		t.Errorf("update after a stale done: %s", got)
	}

	// After a reset the IME's preedit is new again for the app.
	serial, _ = tiCommits(h)
	h.ui(w.ResetTextInput)
	tiRequests(h)
	h.fc.sendRaw(cat(preeditMsg(ti, ptr("z"), 1, 1), event(ti, tiDone, serial+1)))
	checkIME(t, expect[platform.IMEEvent](h), platform.IMEEvent{Preedit: "z", CursorStart: 1, CursorEnd: 1})

	// Leave mid-composition ends the session; a late done is dropped.
	h.fc.sendRaw(cat(event(ti, tiLeave, h.fc.id("wl_surface")), event(ti, tiCommitString, "late"), event(ti, tiDone, serial+1)))
	expectSession(t, h, false)
	h.noEvent(50 * time.Millisecond)
}

func TestTextInputWithoutManager(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	_, w := kbSetup(t, h, "us", 0, 0)
	h.ui(func() {
		w.SetTextInput(&tiState)
		w.ResetTextInput()
		w.SetTextInput(nil)
		w.SetTextInput(&tiState)
	})
	for _, r := range h.fc.quiet(50*time.Millisecond, "zwp_text_input_v3.commit") {
		if strings.HasPrefix(r.iface, "zwp_text_input") {
			t.Errorf("request %v without the manager", r)
		}
	}
	h.noEvent(30 * time.Millisecond)
	if h.fc.id("zwp_text_input_v3") != 0 {
		t.Errorf("a text input was created without the manager")
	}
}

func TestTextInputManagerAfterSeat(t *testing.T) {
	// The registry announces the manager before the seat.
	g := append([]fakeGlobal{{iface: "zwp_text_input_manager_v3", version: 1}}, keyboardGlobals()...)
	h := newHarness(t, g)
	ti, w := tiSetup(t, h)
	h.ui(func() { w.SetTextInput(&platform.TextInputState{}) })
	h.fc.send(ti, tiEnter, h.fc.id("wl_surface"))
	// No cursor area and no surrounding text: neither is sent.
	if got := tiNames(tiRequests(h)); got != "enable set_content_type set_text_change_cause commit" {
		t.Errorf("enable: %s", got)
	}
	expectSession(t, h, true)
}

func TestSurroundingWindow(t *testing.T) {
	long := strings.Repeat("a", 3000) + strings.Repeat("é", 2000) + strings.Repeat("b", 3000) // 9000 bytes
	for _, tc := range []struct {
		name                 string
		st                   platform.SurroundingText
		limit                int
		text                 string
		cursor, anchor       int
		checkLen, checkPiece bool
	}{
		{name: "short", st: platform.SurroundingText{Text: "abc", Cursor: 1, Anchor: 2}, limit: 10, text: "abc", cursor: 1, anchor: 2},
		{name: "nul", st: platform.SurroundingText{Text: "a\x00b", Cursor: 3, Anchor: 3}, limit: 10, text: "a b", cursor: 3, anchor: 3},
		{name: "start", st: platform.SurroundingText{Text: "abcdefghij", Cursor: 1, Anchor: 0}, limit: 4, text: "abcd", cursor: 1, anchor: 0},
		{name: "end", st: platform.SurroundingText{Text: "abcdefghij", Cursor: 10, Anchor: 10}, limit: 4, text: "ghij", cursor: 4, anchor: 4},
		{name: "middle", st: platform.SurroundingText{Text: "abcdefghij", Cursor: 5, Anchor: 5}, limit: 4, text: "defg", cursor: 2, anchor: 2},
		{name: "selection centered", st: platform.SurroundingText{Text: "abcdefghij", Cursor: 6, Anchor: 4}, limit: 4, text: "defg", cursor: 3, anchor: 1},
		{name: "long selection", st: platform.SurroundingText{Text: "abcdefghij", Cursor: 9, Anchor: 1}, limit: 4, text: "ghij", cursor: 3, anchor: 0},
		// Window edges inside "é" (2 bytes) move inward.
		{name: "utf8", st: platform.SurroundingText{Text: "éééé", Cursor: 4, Anchor: 4}, limit: 5, text: "éé", cursor: 2, anchor: 2},
	} {
		text, cursor, anchor := surroundingWindow(tc.st, tc.limit)
		if text != tc.text || cursor != tc.cursor || anchor != tc.anchor {
			t.Errorf("%s: surroundingWindow = %q %d %d, want %q %d %d", tc.name, text, cursor, anchor, tc.text, tc.cursor, tc.anchor)
		}
	}
	// The protocol limit, on text with multi-byte characters around the
	// cursor.
	c := 3000 + 2000 // inside the "é" run, on a boundary
	text, cursor, anchor := surroundingWindow(platform.SurroundingText{Text: long, Cursor: c, Anchor: c}, maxSurrounding)
	if len(text) > maxSurrounding || !strings.Contains(long, text) || text[cursor:] != long[c:c+len(text)-cursor] || cursor != anchor {
		t.Errorf("4000-byte window: len %d, cursor %d, anchor %d", len(text), cursor, anchor)
	}
	if len(text) < maxSurrounding-1 {
		t.Errorf("window of %d bytes, want about %d", len(text), maxSurrounding)
	}
}

func TestTextInputLongSurroundingSent(t *testing.T) {
	h := newHarness(t, tiGlobals())
	ti, w := tiSetup(t, h)
	long := strings.Repeat("x", 10000)
	h.ui(func() {
		w.SetTextInput(&platform.TextInputState{Surrounding: &platform.SurroundingText{Text: long, Cursor: 9000, Anchor: 8990}})
	})
	h.fc.send(ti, tiEnter, h.fc.id("wl_surface"))
	r, i := find(tiRequests(h), "zwp_text_input_v3.set_surrounding_text")
	if i < 0 {
		t.Fatal("no set_surrounding_text")
	}
	if s := r.args[0].(string); len(s) != maxSurrounding || r.i(1) != 9000-(10000-maxSurrounding) || r.i(2) != 8990-(10000-maxSurrounding) {
		t.Errorf("set_surrounding_text: %d bytes, cursor %d, anchor %d", len(s), r.i(1), r.i(2))
	}
}

func TestContentType(t *testing.T) {
	for _, tc := range []struct {
		st      platform.TextInputState
		hint    protocols.ZwpTextInputV3ContentHint
		purpose protocols.ZwpTextInputV3ContentPurpose
	}{
		{platform.TextInputState{}, protocols.ZwpTextInputV3ContentHintNone, protocols.ZwpTextInputV3ContentPurposeNormal},
		{platform.TextInputState{Multiline: true}, protocols.ZwpTextInputV3ContentHintMultiline, protocols.ZwpTextInputV3ContentPurposeNormal},
		{platform.TextInputState{Purpose: platform.TextPurposePassword}, protocols.ZwpTextInputV3ContentHintHiddenText | protocols.ZwpTextInputV3ContentHintSensitiveData, protocols.ZwpTextInputV3ContentPurposePassword},
		{platform.TextInputState{Purpose: platform.TextPurposeEmail}, 0, protocols.ZwpTextInputV3ContentPurposeEmail},
		{platform.TextInputState{Purpose: platform.TextPurposeNumber}, 0, protocols.ZwpTextInputV3ContentPurposeNumber},
		{platform.TextInputState{Purpose: platform.TextPurposePhone}, 0, protocols.ZwpTextInputV3ContentPurposePhone},
		{platform.TextInputState{Purpose: platform.TextPurposeURL}, 0, protocols.ZwpTextInputV3ContentPurposeUrl},
		{platform.TextInputState{Purpose: platform.TextPurposeSearch}, 0, protocols.ZwpTextInputV3ContentPurposeNormal},
	} {
		hint, purpose := contentType(&tc.st)
		if hint != tc.hint || purpose != tc.purpose {
			t.Errorf("contentType(%v, multiline %v) = %v %v, want %v %v", tc.st.Purpose, tc.st.Multiline, hint, purpose, tc.hint, tc.purpose)
		}
	}
}

func TestCursorRect(t *testing.T) {
	for _, tc := range []struct {
		r          platform.Rect
		x, y, w, h int32
	}{
		{platform.Rect{X: 10, Y: 20, W: 0, H: 18}, 10, 20, 1, 18},
		{platform.Rect{X: 10.5, Y: 20.25, W: 2, H: 18}, 10, 20, 3, 19},
		{platform.Rect{X: -3.5, Y: 0, W: 0, H: 0}, -4, 0, 1, 1},
	} {
		x, y, w, h := cursorRect(tc.r)
		if x != tc.x || y != tc.y || w != tc.w || h != tc.h {
			t.Errorf("cursorRect(%v) = %d %d %d %d, want %d %d %d %d", tc.r, x, y, w, h, tc.x, tc.y, tc.w, tc.h)
		}
	}
}
