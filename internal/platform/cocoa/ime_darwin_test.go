//go:build darwin

package cocoa

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// IME tests on a real window: TelaView's NSTextInputClient methods called
// as an input method calls them (through objc_msgSend, so the NSRange and
// CGRect arguments and results cross purego's callbacks as AppKit passes
// them), key events through interpretKeyEvents: with the runner's input
// source, and a Japanese input method when one can be selected. Every
// protocol call and key decision is logged (imeTrace).

var imetsel = struct {
	setMarked, insert, unmark, hasMarked, markedRange, selectedRange darwin.SEL
	substring, validAttrs, firstRect, charIndex, str, count          darwin.SEL
	conformsTo, current                                              darwin.SEL
}{
	setMarked:     darwin.Sel("setMarkedText:selectedRange:replacementRange:"),
	insert:        darwin.Sel("insertText:replacementRange:"),
	unmark:        darwin.Sel("unmarkText"),
	hasMarked:     darwin.Sel("hasMarkedText"),
	markedRange:   darwin.Sel("markedRange"),
	selectedRange: darwin.Sel("selectedRange"),
	substring:     darwin.Sel("attributedSubstringForProposedRange:actualRange:"),
	validAttrs:    darwin.Sel("validAttributesForMarkedText"),
	firstRect:     darwin.Sel("firstRectForCharacterRange:actualRange:"),
	charIndex:     darwin.Sel("characterIndexForPoint:"),
	str:           darwin.Sel("string"),
	count:         darwin.Sel("count"),
	conformsTo:    darwin.Sel("conformsToProtocol:"),
	current:       darwin.Sel("currentInputContext"),
}

// traceIME logs the IME trace of the test and returns its lines.
func traceIME(t *testing.T) *[]string {
	var lines []string
	imeTrace = func(format string, args ...any) {
		l := fmt.Sprintf(format, args...)
		lines = append(lines, l)
		t.Logf("ime: %s", l)
	}
	t.Cleanup(func() { imeTrace = nil })
	return &lines
}

func nsr(loc, n uint) darwin.NSRange { return darwin.NSRange{Location: loc, Length: n} }

var nsNone = darwin.NSRange{Location: darwin.NSNotFound}

// eventStrings formats the window's text, key, focus and session events.
func eventStrings(evs []platform.Event) []string {
	var out []string
	for _, e := range evs {
		switch e := e.(type) {
		case platform.IMEEvent:
			out = append(out, imeEvString(e))
		case platform.IMESessionEvent:
			out = append(out, fmt.Sprintf("session %v", e.Active))
		case platform.FocusEvent:
			out = append(out, fmt.Sprintf("focus %v", e.Focused))
		case platform.KeyEvent:
			out = append(out, keyString(e))
		}
	}
	return out
}

func compareLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	for _, g := range got {
		t.Logf("%s got: %s", what, g)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s differ:\n got:\n  %s\n want:\n  %s", what, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// logContext logs how the view's input context is set up.
func logContext(s *session) {
	ctx := s.w.inputContext()
	cur := darwin.SendClass(darwin.GetClass("NSTextInputContext"), imetsel.current)
	conforms := false
	if p := darwin.ProtocolID("NSTextInputClient"); p != 0 {
		conforms = darwin.SendBool(s.w.view, imetsel.conformsTo, p)
	}
	s.t.Logf("NSTextInputClient declared %v, view conforms %v; inputContext %#x, current input context %#x (the view's: %v)",
		app.textInputClient, conforms, uintptr(ctx), uintptr(cur), ctx != 0 && ctx == cur)
}

// TestIMEProtocol calls TelaView's NSTextInputClient methods as an input
// method does and checks the IMEEvents and IMESessionEvents, the answers
// about the document (ranges, substrings, the caret rectangle in screen
// coordinates), ResetTextInput, the password purpose and focus changes.
func TestIMEProtocol(t *testing.T) {
	traceIME(t)
	type check struct {
		name string
		got  any
		want any
	}
	var checks []check
	add := func(name string, got, want any) { checks = append(checks, check{name, got, want}) }
	var rect, wantRect darwin.CGRect
	var secureOn, secureBefore, secureAfter bool
	normal := &platform.TextInputState{
		CursorArea:  platform.Rect{X: 10, Y: 20, H: 16},
		Surrounding: &platform.SurroundingText{Text: "hello world", Cursor: 5, Anchor: 5},
	}
	s := runSession(t, "tela ime protocol", true, func(s *session) {
		v := s.w.view
		mark := func(text string, sel, repl darwin.NSRange) {
			ns := darwin.NSString(text)
			darwin.Send(v, imetsel.setMarked, ns, sel, repl)
			darwin.Release(ns)
		}
		insert := func(text string, repl darwin.NSRange) {
			ns := darwin.NSString(text)
			darwin.Send(v, imetsel.insert, ns, repl)
			darwin.Release(ns)
		}
		substring := func(r darwin.NSRange) (string, darwin.NSRange) {
			actual := nsNone
			a := darwin.Send(v, imetsel.substring, r, unsafe.Pointer(&actual))
			if a == 0 {
				return "<nil>", actual
			}
			return darwin.GoString(darwin.Send(a, imetsel.str)), actual
		}
		secureBefore = darwin.SecureEventInputEnabled()

		s.w.SetTextInput(normal)
		logContext(s)
		add("hasMarkedText before", darwin.SendBool(v, imetsel.hasMarked), false)
		add("markedRange before", darwin.SendRange(v, imetsel.markedRange), nsNone)
		add("selectedRange before", darwin.SendRange(v, imetsel.selectedRange), nsr(5, 0))

		mark("にほ", nsr(2, 0), nsNone)
		add("hasMarkedText", darwin.SendBool(v, imetsel.hasMarked), true)
		add("markedRange", darwin.SendRange(v, imetsel.markedRange), nsr(5, 2))
		add("selectedRange", darwin.SendRange(v, imetsel.selectedRange), nsr(7, 0))

		// An attributed string, as most input methods send.
		darwin.Send(v, imetsel.setMarked, darwin.NSAttributedStringAutoreleased("日本"), nsr(0, 2), nsr(5, 2))
		add("selectedRange of the clause", darwin.SendRange(v, imetsel.selectedRange), nsr(5, 2))
		str, actual := substring(nsr(0, 5))
		add("substring 0,5", str+fmt.Sprint(actual), "hello"+fmt.Sprint(nsr(0, 5)))
		str, actual = substring(nsr(3, 100))
		add("substring 3,100", str+fmt.Sprint(actual), "lo日本 world"+fmt.Sprint(nsr(3, 10)))

		rect = darwin.SendRect(v, imetsel.firstRect, nsr(5, 0), unsafe.Pointer(nil))
		bottomLeft := darwin.SendPoint(v, isel.convertPointToView, darwin.CGPoint{X: 10, Y: 36}, darwin.ID(0))
		wantRect = darwin.CGRect{Origin: darwin.SendPoint(s.w.ns, isel.convertToScreen, bottomLeft), Size: darwin.CGSize{Height: 16}}

		attrs := darwin.Send(v, imetsel.validAttrs)
		add("validAttributesForMarkedText is an empty array", attrs != 0 && darwin.SendUint(attrs, imetsel.count) == 0, true)
		add("characterIndexForPoint", darwin.SendUint(v, imetsel.charIndex, darwin.CGPoint{X: 12, Y: 30}), uint64(darwin.NSNotFound))

		insert("日本", nsNone)
		add("hasMarkedText after the commit", darwin.SendBool(v, imetsel.hasMarked), false)
		add("selectedRange after the commit", darwin.SendRange(v, imetsel.selectedRange), nsr(7, 0))

		mark("a", nsr(1, 0), nsNone)
		darwin.Send(v, imetsel.unmark)
		insert("X", nsr(3, 5))          // replaces "lo日本a"
		mark("か", nsr(1, 0), nsr(3, 1)) // starts a composition over "X"
		add("markedRange over X", darwin.SendRange(v, imetsel.markedRange), nsr(3, 1))
		mark("", nsr(0, 0), nsNone)  // cancel
		mark("x", nsr(1, 0), nsNone) // reset by the app
		s.w.ResetTextInput()
		add("hasMarkedText after ResetTextInput", darwin.SendBool(v, imetsel.hasMarked), false)

		// A password field: no session, secure event input on.
		pw := normal.Clone()
		pw.Purpose = platform.TextPurposePassword
		pw.Surrounding = nil
		s.w.SetTextInput(pw)
		secureOn = darwin.SecureEventInputEnabled()
		mark("p", nsr(1, 0), nsNone) // ignored
		s.w.SetTextInput(normal)
		secureAfter = darwin.SecureEventInputEnabled()

		// Focus lost during a composition ends the session; the IME's late
		// calls are ignored; the focus back starts it again.
		mark("y", nsr(1, 0), nsNone)
		darwin.Send(s.w.delegate, darwin.Sel("windowDidResignKey:"), darwin.ID(0))
		mark("z", nsr(1, 0), nsNone)
		darwin.Send(s.w.delegate, darwin.Sel("windowDidBecomeKey:"), darwin.ID(0))
		s.w.SetTextInput(nil)
		mark("q", nsr(1, 0), nsNone)
	})
	if s == nil {
		return
	}
	got := eventStrings(s.events)
	if s.simulated && len(got) > 0 && got[0] == "focus true" {
		got = got[1:]
	}
	compareLines(t, "IME events", got, []string{
		`session true`,
		`preedit "にほ" 6,6`,
		`preedit "日本" 0,6`,
		`commit "日本" preedit "" 0,0`,
		`preedit "a" 1,1`,
		`commit "a" preedit "" 0,0`,
		`delete 9,0 commit "X" preedit "" 0,0`,
		`delete 1,0 preedit "か" 3,3`,
		`preedit "" 0,0`,
		`preedit "x" 1,1`,
		`session false`,
		`session true`,
		`preedit "y" 1,1`,
		`session false`,
		`focus false`,
		`focus true`,
		`session true`,
		`session false`,
	})
	for _, c := range checks {
		t.Logf("%s: %v (want %v)", c.name, c.got, c.want)
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
		}
	}
	t.Logf("firstRectForCharacterRange: %+v; the caret's bottom-left corner converted point by point: %+v", rect, wantRect)
	if math.Abs(rect.Origin.X-wantRect.Origin.X) > 0.5 || math.Abs(rect.Origin.Y-wantRect.Origin.Y) > 0.5 ||
		rect.Size.Width != 0 || math.Abs(rect.Size.Height-16) > 0.01 {
		t.Errorf("firstRectForCharacterRange: %+v, want the cursor area in screen coordinates, %+v", rect, wantRect)
	}
	t.Logf("secure event input: before %v, in the password field %v, after %v", secureBefore, secureOn, secureAfter)
	if s.w.focused || s.key {
		if !secureOn {
			t.Errorf("secure event input is off in a focused password field")
		}
	}
	if secureAfter && !secureBefore {
		t.Errorf("secure event input stayed on after the password field")
	}
}

// keyEventChars is session.keyEvent, but it also sets empty characters
// (a dead key's, which keyEvent would leave to CoreGraphics).
func (s *session) keyEventChars(down bool, code uint16, flags uint64, chars string) darwin.ID {
	if chars != "" {
		return s.keyEvent(down, code, flags, chars)
	}
	cg := darwin.CGEventCreateKeyboardEvent(0, code, down)
	if cg == 0 {
		return 0
	}
	defer darwin.CFRelease(cg)
	darwin.CGEventSetFlags(cg, flags)
	var none uint16
	darwin.CGEventKeyboardSetUnicodeString(cg, 0, &none)
	ev := darwin.SendClass(app.event, tsel.withCGEvent, cg)
	if ev != 0 {
		s.t.Logf("built key event: type %d keyCode %#x flags %#x characters %q ignoring modifiers %q",
			darwin.SendUint(ev, isel.typ), uint16(darwin.SendUint(ev, isel.keyCode)), darwin.SendUint(ev, isel.modifierFlags),
			darwin.GoString(darwin.Send(ev, isel.characters)), darwin.GoString(darwin.Send(ev, isel.charactersIgnoring)))
	}
	return ev
}

type testKey struct {
	code  uint16
	flags uint64
	chars string
}

func (s *session) tap(keys ...testKey) {
	for _, k := range keys {
		s.sendKey(s.keyEventChars(true, k.code, k.flags, k.chars))
		s.sendKey(s.keyEventChars(false, k.code, k.flags, k.chars))
	}
}

// TestIMEKeys sends key events through interpretKeyEvents: with the
// runner's input source (US): a letter, Backspace and an arrow must give
// the same KeyEvents as without a text field, and no IMEEvent, through
// insertText: and doCommandBySelector:. A dead key (Option+E, E) is
// logged: AppKit turns it into marked text "´" and then "é" when its input
// context composes it, or the keys carry the text; either way "é" must
// come out once.
func TestIMEKeys(t *testing.T) {
	trace := traceIME(t)
	plain := []testKey{
		{0x00, 0, "a"},    // kVK_ANSI_A
		{0x33, 0, "\x7f"}, // kVK_Delete: Backspace
		{0x7B, keyboard.MacFlagNumericPad | keyboard.MacFlagFunction, ""}, // kVK_LeftArrow
	}
	option := keyboard.MacFlagOption | keyboard.MacDeviceLeftOption
	var marks [3]int
	var traceMark int
	state := &platform.TextInputState{CursorArea: platform.Rect{X: 4, Y: 4, H: 16}, Surrounding: &platform.SurroundingText{}}
	s := runSteps(t, "tela ime keys", true, 300*time.Millisecond,
		func(s *session) {
			s.tap(plain...)
		},
		func(s *session) {
			marks[0] = len(s.events)
			s.w.SetTextInput(state)
			logContext(s)
			traceMark = len(*trace)
			s.tap(plain...)
		},
		func(s *session) {
			marks[1] = len(s.events)
			s.tap(testKey{0x0E, option, ""}, testKey{0x0E, 0, "é"}) // kVK_ANSI_E
		},
		func(s *session) {
			marks[2] = len(s.events)
			s.w.SetTextInput(nil)
		},
	)
	if s == nil {
		return
	}
	if s.simulated {
		t.Logf("note: the focus was simulated; the input context may not be the current one")
	}
	keysOnly := func(evs []platform.Event) []string {
		var out []string
		for _, l := range eventStrings(evs) {
			if !strings.HasPrefix(l, "focus") {
				out = append(out, l)
			}
		}
		return out
	}
	without := keysOnly(s.events[:marks[0]])
	with := keysOnly(s.events[marks[0]:marks[1]])
	compareLines(t, "keys with a text field", with, append([]string{"session true"}, without...))
	if len(without) != 6 {
		t.Errorf("without a text field: %d key events, want 6 (3 presses, 3 releases): %q", len(without), without)
	}
	keyTrace := strings.Join((*trace)[traceMark:], "\n")
	for _, want := range []string{`insertText:"a"`, "doCommandBySelector:deleteBackward:", "doCommandBySelector:moveLeft:"} {
		if !strings.Contains(keyTrace, want) {
			t.Errorf("interpretKeyEvents: did not call %s (trace above)", want)
		}
	}

	dead := eventStrings(s.events[marks[1]:marks[2]])
	composed := 0
	sawMarked := false
	for _, l := range dead {
		t.Logf("dead key got: %s", l)
		if strings.Contains(l, `commit "é"`) || strings.Contains(l, `text="é"`) {
			composed++
		}
		if strings.HasPrefix(l, `preedit "´"`) {
			sawMarked = true
		}
	}
	switch {
	case composed == 1 && sawMarked:
		t.Logf("dead key: the input context composed it (marked text ´, then é committed)")
	case composed == 1:
		t.Logf("dead key: no marked text; é came with the key event")
	case composed == 0:
		t.Logf("note: the dead key gave no é on this runner (its input source does not compose Option+E); not a failure")
	default:
		t.Errorf("dead key: é came out %d times", composed)
	}
	for _, e := range s.events {
		if ev, ok := e.(platform.IMEEvent); ok {
			if p := platform.CheckIMEEvent(ev); p != "" {
				t.Errorf("%s breaks the IMEEvent rules: %s", imeEvString(ev), p)
			}
		}
	}
	if tail := eventStrings(s.events[marks[2]:]); len(tail) == 0 || tail[len(tail)-1] != "session false" {
		t.Errorf("SetTextInput(nil): events %q, want the session's end", tail)
	}
}

// TestIMERejectedTextWindow: the app (this test's handler) never reports a new
// state, as a field at its MaxLength does when it rejects what is typed;
// after the passes that delivered the key and the commit, the document
// the input method reads is the app's again, so a composition and a
// reconversion after them are placed in the text the field holds.
func TestIMERejectedTextWindow(t *testing.T) {
	traceIME(t)
	state := &platform.TextInputState{
		CursorArea:  platform.Rect{X: 4, Y: 4, H: 16},
		Surrounding: &platform.SurroundingText{Text: "abc", Cursor: 3, Anchor: 3},
	}
	var selAfter, markedAfter darwin.NSRange
	var docAfter string
	mark := 0
	s := runSteps(t, "tela ime rejected", true, 300*time.Millisecond,
		func(s *session) {
			s.w.SetTextInput(state)
			s.tap(testKey{0x02, 0, "d"}) // kVK_ANSI_D, rejected
			ns := darwin.NSString("e")   // a commit outside a key, rejected
			darwin.Send(s.w.view, imetsel.insert, ns, nsNone)
			darwin.Release(ns)
		},
		func(s *session) {
			mark = len(s.events)
			v := s.w.view
			selAfter = darwin.SendRange(v, imetsel.selectedRange)
			actual := nsNone
			if a := darwin.Send(v, imetsel.substring, nsr(0, 10), unsafe.Pointer(&actual)); a != 0 {
				docAfter = darwin.GoString(darwin.Send(a, imetsel.str))
			}
			ns := darwin.NSString("か")
			darwin.Send(v, imetsel.setMarked, ns, nsr(1, 0), nsNone)
			darwin.Release(ns)
			markedAfter = darwin.SendRange(v, imetsel.markedRange)
			ns = darwin.NSString("X") // reconversion of "c" and the preedit
			darwin.Send(v, imetsel.insert, ns, nsr(2, 2))
			darwin.Release(ns)
		},
	)
	if s == nil {
		return
	}
	if docAfter != "abc" || selAfter != nsr(3, 0) {
		t.Errorf("after the rejected text: document %q, selectedRange %v; want \"abc\" and {3 0}", docAfter, selAfter)
	}
	if markedAfter != nsr(3, 1) {
		t.Errorf("markedRange of the composition after it: %v, want {3 1}", markedAfter)
	}
	compareLines(t, "IME events after the rejected text", eventStrings(s.events[mark:]), []string{
		`preedit "か" 3,3`,
		`delete 1,0 commit "X" preedit "" 0,0`,
	})
}

// japaneseSources are the identifiers of Kotoeri's romaji input mode.
var japaneseSources = []string{
	"com.apple.inputmethod.Kotoeri.RomajiTyping.Japanese",
	"com.apple.inputmethod.Kotoeri.Japanese",
}

func sourceID(src uintptr) string {
	if src == 0 {
		return ""
	}
	return darwin.GoString(darwin.ID(darwin.TISGetInputSourceProperty(src, darwin.TISPropertyInputSourceID)))
}

// findSource returns an input source with one of the ids (retained), from
// the enabled ones, or from all installed ones when all.
func findSource(ids []string, all bool) uintptr {
	for _, id := range ids {
		ns := darwin.NSString(id)
		filter := darwin.SendClass(darwin.GetClass("NSDictionary"), darwin.Sel("dictionaryWithObject:forKey:"), ns, darwin.ID(darwin.TISPropertyInputSourceID))
		darwin.Release(ns)
		list := darwin.TISCreateInputSourceList(uintptr(filter), all)
		if list == 0 {
			continue
		}
		var src uintptr
		if darwin.SendUint(darwin.ID(list), imetsel.count) > 0 {
			src = uintptr(darwin.Retain(darwin.Send(darwin.ID(list), tsel.objectAtIndex, uint(0))))
		}
		darwin.CFRelease(list)
		if src != 0 {
			return src
		}
	}
	return 0
}

// TestIMEJapanese types "nihon" and Return with Kotoeri when it is enabled
// (or, on CI, installed: it is enabled for the test and disabled after),
// and logs the composition; it checks only that every IMEEvent is valid
// and that a composition, when one happened, ended. It skips when the
// input source cannot be found.
func TestIMEJapanese(t *testing.T) {
	var src, prev uintptr
	var enabled bool
	var skip string
	onMain(func() {
		if err := darwin.Load(); err != nil {
			skip = err.Error()
			return
		}
		if err := darwin.LoadTextInputSources(); err != nil {
			skip = fmt.Sprintf("Text Input Sources unavailable: %v", err)
			return
		}
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		prev = darwin.TISCopyCurrentKeyboardInputSource()
		t.Logf("current input source: %s", sourceID(prev))
		src = findSource(japaneseSources, false)
		if src == 0 && os.Getenv("CI") != "" {
			if src = findSource(japaneseSources, true); src != 0 {
				st := darwin.TISEnableInputSource(src)
				enabled = st == 0
				t.Logf("enabled %s for the test: status %d", sourceID(src), st)
			}
		}
		if src == 0 {
			skip = "no Japanese input source (Kotoeri) is enabled or installed"
		}
	})
	if skip != "" {
		t.Skip(skip)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		if prev != 0 {
			t.Logf("restoring %s: status %d", sourceID(prev), darwin.TISSelectInputSource(prev))
			darwin.CFRelease(prev)
		}
		if enabled {
			t.Logf("disabling %s: status %d", sourceID(src), darwin.TISDisableInputSource(src))
		}
		darwin.Release(darwin.ID(src))
	}
	defer onMain(restore)
	traceIME(t)
	letters := []testKey{{0x2D, 0, "n"}, {0x22, 0, "i"}, {0x04, 0, "h"}, {0x1F, 0, "o"}, {0x2D, 0, "n"}}
	state := &platform.TextInputState{CursorArea: platform.Rect{X: 20, Y: 30, H: 18}, Surrounding: &platform.SurroundingText{}}
	s := runSteps(t, "tela ime japanese", true, time.Second,
		func(s *session) {
			s.w.SetTextInput(state)
			t.Logf("TISSelectInputSource(%s): status %d", sourceID(src), darwin.TISSelectInputSource(src))
		},
		func(s *session) {
			cur := darwin.TISCopyCurrentKeyboardInputSource()
			t.Logf("current input source now: %s", sourceID(cur))
			darwin.CFRelease(cur)
			logContext(s)
			s.tap(letters...)
		},
		func(s *session) {
			s.tap(testKey{0x24, 0, "\r"}) // kVK_Return
		},
		func(s *session) {
			s.w.SetTextInput(nil)
			restore()
		},
	)
	if s == nil {
		return
	}
	composing, preedits, commits := false, 0, []string{}
	for _, e := range s.events {
		switch ev := e.(type) {
		case platform.IMEEvent:
			t.Logf("got: %s", imeEvString(ev))
			if p := platform.CheckIMEEvent(ev); p != "" {
				t.Errorf("%s breaks the IMEEvent rules: %s", imeEvString(ev), p)
			}
			if ev.Commit != "" {
				commits = append(commits, ev.Commit)
			}
			if ev.Preedit != "" {
				preedits++
			}
			composing = ev.Preedit != ""
		case platform.KeyEvent:
			t.Logf("got: %s", keyString(ev))
		case platform.IMESessionEvent:
			t.Logf("got: session %v", ev.Active)
			if !ev.Active {
				composing = false
			}
		}
	}
	if preedits == 0 {
		t.Logf("note: no composition happened (the input method did not take the in-process key events on this runner); not a failure")
		return
	}
	t.Logf("composition: %d preedit updates, commits %q", preedits, commits)
	if composing {
		t.Errorf("the composition was still in progress at the end")
	}
}
