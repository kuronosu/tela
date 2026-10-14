package platform

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// TextInputState describes the focused text field of a window to the
// platform's input method (IME): where its caret is, what kind of text it
// takes and, optionally, the text around the caret. The app sets it with
// [Window.SetTextInput] whenever the field or one of these values changes;
// see the package documentation ("Text input and IME").
//
// Every offset is in bytes of UTF-8, like Go strings; backends whose OS
// counts in UTF-16 code units (Win32, macOS) convert.
type TextInputState struct {
	// CursorArea is the caret, or the preedit or selection it belongs to,
	// in logical pixels relative to the content area (like pointer
	// positions). The IME places its candidate window next to it, below
	// by default, so it should cover the line's full height. A caret has
	// zero width. The zero Rect means unknown: backends then send no
	// cursor position.
	CursorArea Rect
	// Purpose is the kind of text the field takes.
	Purpose TextPurpose
	// Multiline marks a field that accepts line breaks (Textarea).
	Multiline bool
	// Surrounding is the field's committed text around the caret, for
	// IMEs that read context (text-input-v3's surrounding text, macOS
	// NSTextInputClient, IMM32 reconversion). Nil means not provided.
	Surrounding *SurroundingText
}

// SurroundingText is the committed text of a field (without the preedit of
// a composition in progress) with its selection, in UTF-8 byte offsets of
// Text. Cursor is the selection's focus (where the caret is) and Anchor its
// other end; they are equal when nothing is selected. During a composition
// both are the offset where the preedit is inserted (text-input-v3 replaces
// the preedit with a cursor). Offsets lie on UTF-8 character boundaries.
//
// The app may send the whole value: backends with a size limit send a
// window around the selection (text-input-v3: 4000 bytes) and adjust the
// offsets themselves.
type SurroundingText struct {
	Text           string
	Cursor, Anchor int
}

// TextPurpose is the kind of text a field takes, from which the IME and
// on-screen keyboards pick their layout and behavior (text-input-v3
// content purpose, HTML inputmode).
type TextPurpose uint8

const (
	// TextPurposeNormal is free text.
	TextPurposeNormal TextPurpose = iota
	// TextPurposePassword is a password: backends turn off composition
	// and prediction (text-input-v3: purpose password with the hidden and
	// sensitive hints; Win32: no input context; macOS: secure input), and
	// the IME must not learn or show the text.
	TextPurposePassword
	// TextPurposeEmail is an e-mail address.
	TextPurposeEmail
	// TextPurposeNumber is a number (digits, sign and separators).
	TextPurposeNumber
	// TextPurposePhone is a telephone number.
	TextPurposePhone
	// TextPurposeURL is a URL.
	TextPurposeURL
	// TextPurposeSearch is a search query (text-input-v3 has no search
	// purpose: it is sent as normal).
	TextPurposeSearch

	textPurposeCount
)

var textPurposeNames = [...]string{"normal", "password", "email", "number", "phone", "url", "search"}

func (p TextPurpose) String() string {
	if int(p) < len(textPurposeNames) {
		return textPurposeNames[p]
	}
	return "TextPurpose(invalid)"
}

// Valid reports whether p is one of the TextPurpose constants.
func (p TextPurpose) Valid() bool { return p < textPurposeCount }

// Equal reports whether s and o describe the same state (both nil, or the
// same values). Backends use it to send only changes to the OS.
func (s *TextInputState) Equal(o *TextInputState) bool {
	if s == nil || o == nil {
		return s == o
	}
	if s.CursorArea != o.CursorArea || s.Purpose != o.Purpose || s.Multiline != o.Multiline {
		return false
	}
	if s.Surrounding == nil || o.Surrounding == nil {
		return s.Surrounding == o.Surrounding
	}
	return *s.Surrounding == *o.Surrounding
}

// Clone returns a deep copy of s (nil for nil). Backends keep a clone, so
// the app may reuse and modify its state after SetTextInput returns.
func (s *TextInputState) Clone() *TextInputState {
	if s == nil {
		return nil
	}
	c := *s
	if s.Surrounding != nil {
		st := *s.Surrounding
		c.Surrounding = &st
	}
	return &c
}

// CheckTextInputState panics with an actionable message if s is not a
// valid state: a cursor area with a NaN, infinite or negative component,
// an unknown purpose, surrounding text that is not valid UTF-8, or a
// cursor or anchor outside the text or inside a UTF-8 character. nil is
// valid. Every backend calls it at the top of SetTextInput, so every
// backend accepts and rejects the same states.
func CheckTextInputState(s *TextInputState) {
	if s == nil {
		return
	}
	r := s.CursorArea
	for _, v := range [4]float32{r.X, r.Y, r.W, r.H} {
		if v != v || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 1e7 {
			panicf("platform: invalid TextInputState.CursorArea %v; coordinates must be finite logical pixels", r)
		}
	}
	if r.W < 0 || r.H < 0 {
		panicf("platform: invalid TextInputState.CursorArea %v; width and height must not be negative (a caret has zero width)", r)
	}
	if !s.Purpose.Valid() {
		panicf("platform: invalid TextInputState.Purpose %d; use one of the TextPurpose constants", s.Purpose)
	}
	if st := s.Surrounding; st != nil {
		if !utf8.ValidString(st.Text) {
			panicf("platform: TextInputState.Surrounding.Text is not valid UTF-8")
		}
		for _, o := range [...]struct {
			name string
			off  int
		}{{"Cursor", st.Cursor}, {"Anchor", st.Anchor}} {
			if o.off < 0 || o.off > len(st.Text) {
				panicf("platform: TextInputState.Surrounding.%s %d is outside the text (%d bytes); offsets are UTF-8 byte offsets of Text", o.name, o.off, len(st.Text))
			}
			if o.off < len(st.Text) && !utf8.RuneStart(st.Text[o.off]) {
				panicf("platform: TextInputState.Surrounding.%s %d is inside a UTF-8 character; offsets must lie on character boundaries", o.name, o.off)
			}
		}
	}
}

// IMEEvent is one atomic update from the input method to the focused text
// field of the window: text-input-v3's done, one IMM32 WM_IME_COMPOSITION,
// one NSTextInputClient call. The app applies its parts in field order:
//
//  1. DeleteBefore and DeleteAfter: delete that many bytes of committed
//     text before and after the cursor (or the selection, which stays;
//     during a composition, before the preedit's start and after its
//     end).
//  2. Commit: insert the text at the cursor, replacing the preedit of a
//     composition in progress (which ends it) or else the selection, with
//     the caret after it.
//  3. Preedit: show it as the new composition text at the cursor, with
//     the IME's cursor at CursorStart..CursorEnd. An empty Preedit means
//     no composition: if one was in progress and there was no Commit, the
//     IME cancelled it, and its preedit is removed.
//
// With internal/text/edit this is: DeleteSurrounding (when a length is
// set), then CommitComposition (when Commit is not empty), then SetPreedit
// (when Preedit is not empty) or, when a composition is still in progress
// and Preedit is empty, CancelComposition.
//
// Backends deliver an IMEEvent only when it changes something: a deletion,
// a commit, or a preedit (text or cursor) different from the previous one.
// Offsets and lengths are UTF-8 bytes. Backends sanitize what the OS sends
// (invalid UTF-8 becomes U+FFFD, cursors are clamped into Preedit and onto
// character boundaries), so the fields are always consistent.
type IMEEvent struct {
	// DeleteBefore and DeleteAfter are byte lengths of committed text to
	// delete before and after the cursor. Backends do not clamp them to
	// the field's text, which they do not know: the app does.
	DeleteBefore, DeleteAfter int
	// Commit is text to insert, "" for none.
	Commit string
	// Preedit is the composition text after this event, "" for none.
	Preedit string
	// CursorStart and CursorEnd are the IME's cursor (equal) or
	// highlighted range (CursorStart < CursorEnd) in Preedit, as byte
	// offsets of Preedit on character boundaries. Both are -1 when the
	// IME hides the cursor; both are 0 when Preedit is empty.
	CursorStart, CursorEnd int
}

// IMESessionEvent reports that the window's IME session started (Active
// true) or ended (Active false). A session is active while the window has
// keyboard focus, the app has a text input state set ([Window.SetTextInput]
// with a non-nil state) and the platform has attached its input method to
// the window (text-input-v3: after enter and a committed enable; headless:
// focus and a state). It says nothing about whether an IME is installed:
// without one, no IMEEvents arrive. Its order relative to FocusEvent is
// not specified: Wayland reports keyboard and text-input focus separately
// (headless ends a session before the FocusEvent and starts it after).
//
// When a session ends, a composition in progress is over: the IME sends
// nothing more for it. The app keeps its preedit as committed text
// (Chromium's behavior when a field loses focus; edit.FinishComposition)
// or removes it. Backends report every change, including those the app
// caused: SetTextInput(nil) ends the session, and moving to another field
// (SetTextInput(nil) then the new field's state) restarts it.
type IMESessionEvent struct {
	Active bool
}

func (IMEEvent) isEvent()        {}
func (IMESessionEvent) isEvent() {}

// SanitizeIMEText returns s with invalid UTF-8 replaced by U+FFFD and NUL
// bytes removed. Backends apply it to the text the IME sends.
func SanitizeIMEText(s string) string {
	if utf8.ValidString(s) && !containsNUL(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			out = utf8.AppendRune(out, utf8.RuneError)
		case r != 0:
			out = append(out, s[i:i+n]...)
		}
		i += n
	}
	return string(out)
}

func containsNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}

// SanitizePreeditCursor returns the IME's preedit cursor clamped to the
// IMEEvent rules: (-1, -1) if either offset is negative (hidden), else both
// clamped to [0, len(preedit)], moved back onto character boundaries and
// ordered. An empty preedit gives (0, 0).
func SanitizePreeditCursor(preedit string, start, end int) (int, int) {
	if preedit == "" {
		return 0, 0
	}
	if start < 0 || end < 0 {
		return -1, -1
	}
	clamp := func(o int) int {
		o = min(o, len(preedit))
		for o > 0 && o < len(preedit) && !utf8.RuneStart(preedit[o]) {
			o--
		}
		return o
	}
	start, end = clamp(start), clamp(end)
	if start > end {
		start, end = end, start
	}
	return start, end
}

// CheckIMEEvent returns why e breaks the IMEEvent rules, or "" if it does
// not. It does not check that e changes something. The headless backend
// uses it on injected events; other backends build events that pass it.
func CheckIMEEvent(e IMEEvent) string {
	switch {
	case e.DeleteBefore < 0 || e.DeleteAfter < 0:
		return fmt.Sprintf("negative DeleteBefore %d or DeleteAfter %d", e.DeleteBefore, e.DeleteAfter)
	case !utf8.ValidString(e.Commit) || !utf8.ValidString(e.Preedit):
		return "Commit or Preedit is not valid UTF-8"
	case containsNUL(e.Commit) || containsNUL(e.Preedit):
		return "Commit or Preedit contains a NUL byte"
	case e.Preedit == "" && (e.CursorStart != 0 || e.CursorEnd != 0):
		return fmt.Sprintf("CursorStart %d and CursorEnd %d with an empty Preedit; both must be 0", e.CursorStart, e.CursorEnd)
	case e.Preedit == "":
		return ""
	case e.CursorStart == -1 && e.CursorEnd == -1:
		return ""
	case e.CursorStart < 0 || e.CursorEnd < 0:
		return fmt.Sprintf("CursorStart %d and CursorEnd %d: a hidden cursor has both -1", e.CursorStart, e.CursorEnd)
	case e.CursorStart > e.CursorEnd || e.CursorEnd > len(e.Preedit):
		return fmt.Sprintf("CursorStart %d and CursorEnd %d are not an ordered range of Preedit (%d bytes)", e.CursorStart, e.CursorEnd, len(e.Preedit))
	}
	for _, o := range [2]int{e.CursorStart, e.CursorEnd} {
		if o < len(e.Preedit) && !utf8.RuneStart(e.Preedit[o]) {
			return fmt.Sprintf("preedit cursor offset %d is inside a UTF-8 character", o)
		}
	}
	return ""
}
