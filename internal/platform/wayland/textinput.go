//go:build linux

package wayland

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Text input and IME over zwp_text_input_v3. See "Text input and IME" in
// doc.go for the model; the protocol rules and quirks handled here:
//
//   - Version 1 is bound. Version 2 moves the cursor rectangle to the next
//     wl_surface.commit and adds actions and input panel requests that
//     Tela does not use yet (GTK and winit bind version 1 too).
//   - The compositor counts our commit requests and sends that count as
//     the serial of done. A done whose serial is older than our count was
//     computed by the IME for a state we have since replaced. The
//     protocol says to apply it anyway ("proceed with evaluating and
//     applying the changes as normal"), and so do GTK and Gio; what GTK
//     skips for such a done is resending its state in response. Here the
//     state is level-triggered (the app's current field, sent when it
//     changes), so there is no response to skip: the app's next state,
//     with the text the batch produced, is committed like any other, and
//     the IME's next done carries a current serial. Waiting for a
//     matching done before sending would stall forever with IMEs that
//     answer only some commits.
//   - preedit_string, commit_string and delete_surrounding_text are
//     double-buffered: they are latched and become one IMEEvent at done,
//     after which the latched values return to their initial state (an
//     empty preedit). A done without preedit_string therefore ends the
//     preedit. Only changes are delivered (see platform.IMEEvent).
//   - After enter, after disable and after leave, all state is invalid:
//     enable is followed by the full state. Requests between leave and the
//     next enter are ignored by the compositor but would still count as
//     commits, so none are sent then.
//   - The content type may not change while enabled ("subsequent attempts
//     to update them may have no effect"), so a new purpose or multiline
//     flag restarts the session (disable, commit, enable, state, commit),
//     like a move to another field; Gio re-enables the same way.
//   - set_text_change_cause is sent as "other" with enable and with
//     ResetTextInput, as GTK does (its reset and focus-in); every other
//     commit keeps the initial "input_method".
//   - set_surrounding_text is limited to 4000 bytes: longer text is cut to
//     a window around the selection (surroundingWindow), like GTK.
//   - set_cursor_rectangle is in surface-local coordinates, which are
//     Tela's logical window coordinates (no client-side decorations); the
//     rectangle is rounded outward to whole pixels and is at least 1x1.

// textInputVersion is the zwp_text_input_manager_v3 version bound.
const textInputVersion = 1

// maxSurrounding is set_surrounding_text's limit on the text, in bytes.
const maxSurrounding = 4000

// textInput is the zwp_text_input_v3 of a seat.
type textInput struct {
	s  *seat
	wl *protocols.ZwpTextInputV3

	// focus is the window the text input entered (nil: none, or not
	// ours).
	focus *Window
	// enabled reports that an enable is committed for focus and no
	// disable or leave followed; sent is the state committed with it.
	enabled bool
	sent    *platform.TextInputState
	// commits counts commit requests: the serial a current done carries.
	commits uint32
	// lastDone is the serial of the latest done; stale counts the dones
	// whose serial was not our commit count (for tests and debugging).
	lastDone uint32
	stale    int

	// Latched events, applied at done.
	pendPreedit           string
	pendBegin, pendEnd    int32
	pendCommit            string
	pendBefore, pendAfter uint32

	// The preedit and cursor of the last delivered IMEEvent.
	preedit   string
	preCursor [2]int
}

func (b *Backend) bindTextInputManager(name, version uint32) {
	if b.textInputManager != nil {
		return
	}
	b.textInputManager = protocols.WlRegistryBind[protocols.ZwpTextInputManagerV3](b.registry, name, min(version, textInputVersion))
	for _, s := range b.seats {
		s.attachTextInput()
	}
}

// attachTextInput creates the seat's text input, once the seat and the
// manager both exist.
func (s *seat) attachTextInput() {
	if s.ti != nil || s.b.textInputManager == nil {
		return
	}
	ti := &textInput{s: s}
	ti.wl = s.b.textInputManager.GetTextInput(s.wl)
	ti.wl.SetEnterHandler(ti.onEnter)
	ti.wl.SetLeaveHandler(ti.onLeave)
	ti.wl.SetPreeditStringHandler(ti.onPreedit)
	ti.wl.SetCommitStringHandler(ti.onCommit)
	ti.wl.SetDeleteSurroundingTextHandler(ti.onDeleteSurrounding)
	ti.wl.SetDoneHandler(ti.onDone)
	s.ti = ti
}

// release destroys the text input (seat removed, backend shutting down).
func (ti *textInput) release() {
	ti.end()
	ti.focus = nil
	if !ti.wl.Destroyed() {
		ti.wl.Destroy() // also disables it
	}
}

func (ti *textInput) onEnter(e protocols.ZwpTextInputV3EnterEvent) {
	if ti.focus != nil {
		ti.end() // a missed leave
	}
	ti.focus = ti.s.b.surfaces[e.Surface] // nil if not ours
	ti.clearPending()
	// The state is sent by the next sync, before the loop flushes.
}

func (ti *textInput) onLeave(protocols.ZwpTextInputV3LeaveEvent) {
	ti.end()
	ti.focus = nil
}

// end forgets the session after leave (or a missed one): the compositor
// ignores requests until the next enter, so nothing is sent.
func (ti *textInput) end() {
	if ti.enabled {
		ti.enabled = false
		ti.sent = nil
		if ti.focus != nil {
			ti.focus.imeSession(-1)
		}
	}
	ti.clearPending()
	ti.preedit, ti.preCursor = "", [2]int{}
}

func (ti *textInput) clearPending() {
	ti.pendPreedit, ti.pendBegin, ti.pendEnd = "", 0, 0
	ti.pendCommit = ""
	ti.pendBefore, ti.pendAfter = 0, 0
}

func (ti *textInput) onPreedit(e protocols.ZwpTextInputV3PreeditStringEvent) {
	ti.pendPreedit = ""
	if e.Text != nil {
		ti.pendPreedit = *e.Text
	}
	ti.pendBegin, ti.pendEnd = e.CursorBegin, e.CursorEnd
}

func (ti *textInput) onCommit(e protocols.ZwpTextInputV3CommitStringEvent) {
	ti.pendCommit = ""
	if e.Text != nil {
		ti.pendCommit = *e.Text
	}
}

func (ti *textInput) onDeleteSurrounding(e protocols.ZwpTextInputV3DeleteSurroundingTextEvent) {
	ti.pendBefore, ti.pendAfter = e.BeforeLength, e.AfterLength
}

// onDone applies the latched events as one IMEEvent, whatever the serial
// (see the notes at the top of this file).
func (ti *textInput) onDone(e protocols.ZwpTextInputV3DoneEvent) {
	ti.lastDone = e.Serial
	if e.Serial != ti.commits {
		ti.stale++
	}
	ev := platform.IMEEvent{
		DeleteBefore: int(min(ti.pendBefore, math.MaxInt32)),
		DeleteAfter:  int(min(ti.pendAfter, math.MaxInt32)),
		Commit:       platform.SanitizeIMEText(ti.pendCommit),
		Preedit:      platform.SanitizeIMEText(ti.pendPreedit),
	}
	ev.CursorStart, ev.CursorEnd = platform.SanitizePreeditCursor(ev.Preedit, int(ti.pendBegin), int(ti.pendEnd))
	ti.clearPending()
	w := ti.focus
	if w == nil || !ti.enabled || w.Closed() {
		return // a done for a session that is over
	}
	cursor := [2]int{ev.CursorStart, ev.CursorEnd}
	if ev.DeleteBefore == 0 && ev.DeleteAfter == 0 && ev.Commit == "" && ev.Preedit == ti.preedit && cursor == ti.preCursor {
		return // nothing changed (an IME answering a state update)
	}
	ti.preedit, ti.preCursor = ev.Preedit, cursor
	ti.s.b.enqueue(w, ev)
}

// sync brings the compositor's text input state in line with the focused
// window's: enable, update, restart or disable, one commit per change.
func (ti *textInput) sync() {
	w := ti.focus
	want := w != nil && !w.Closed() && w.textInput != nil
	switch {
	case !want:
		if ti.enabled {
			ti.disable()
		}
	case !ti.enabled:
		ti.enable(w.textInput)
	case w.tiRestart || w.textInput.Purpose != ti.sent.Purpose || w.textInput.Multiline != ti.sent.Multiline:
		ti.disable()
		ti.enable(w.textInput)
	case w.tiReset:
		ti.sendState(w.textInput, nil)
		ti.wl.SetTextChangeCause(protocols.ZwpTextInputV3ChangeCauseOther)
		ti.commit(w.textInput)
		ti.preedit, ti.preCursor = "", [2]int{}
	case !ti.sent.Equal(w.textInput):
		ti.sendState(w.textInput, ti.sent)
		ti.commit(w.textInput)
	}
}

// enable starts a session on ti.focus with state st.
func (ti *textInput) enable(st *platform.TextInputState) {
	ti.wl.Enable()
	ti.sendState(st, nil)
	ti.wl.SetTextChangeCause(protocols.ZwpTextInputV3ChangeCauseOther)
	ti.commit(st)
	ti.enabled = true
	ti.preedit, ti.preCursor = "", [2]int{}
	ti.focus.imeSession(+1)
}

// disable ends the session on ti.focus.
func (ti *textInput) disable() {
	ti.wl.Disable()
	ti.wl.Commit()
	ti.commits++
	ti.enabled = false
	ti.sent = nil
	ti.preedit, ti.preCursor = "", [2]int{}
	ti.clearPending()
	ti.focus.imeSession(-1)
}

func (ti *textInput) commit(st *platform.TextInputState) {
	ti.wl.Commit()
	ti.commits++
	ti.sent = st.Clone()
}

// sendState sends the parts of st that differ from prev (all of them when
// prev is nil): surrounding text, content type and cursor rectangle.
// Unknown parts (no surrounding text, a zero cursor area) are not sent.
func (ti *textInput) sendState(st, prev *platform.TextInputState) {
	if sr := st.Surrounding; sr != nil && (prev == nil || prev.Surrounding == nil || *sr != *prev.Surrounding) {
		text, cursor, anchor := surroundingWindow(*sr, maxSurrounding)
		ti.wl.SetSurroundingText(text, int32(cursor), int32(anchor))
	}
	if prev == nil {
		hint, purpose := contentType(st)
		ti.wl.SetContentType(hint, purpose)
	}
	if a := st.CursorArea; a != (platform.Rect{}) && (prev == nil || a != prev.CursorArea) {
		x, y, w, h := cursorRect(a)
		ti.wl.SetCursorRectangle(x, y, w, h)
	}
}

// contentType maps a state to text-input-v3's hint and purpose, like
// GTK's translate_hints and translate_purpose: a password is hidden and
// sensitive (the IME neither shows nor learns it); search has no purpose
// of its own.
func contentType(st *platform.TextInputState) (protocols.ZwpTextInputV3ContentHint, protocols.ZwpTextInputV3ContentPurpose) {
	hint := protocols.ZwpTextInputV3ContentHintNone
	purpose := protocols.ZwpTextInputV3ContentPurposeNormal
	switch st.Purpose {
	case platform.TextPurposePassword:
		purpose = protocols.ZwpTextInputV3ContentPurposePassword
		hint |= protocols.ZwpTextInputV3ContentHintHiddenText | protocols.ZwpTextInputV3ContentHintSensitiveData
	case platform.TextPurposeEmail:
		purpose = protocols.ZwpTextInputV3ContentPurposeEmail
	case platform.TextPurposeNumber:
		purpose = protocols.ZwpTextInputV3ContentPurposeNumber
	case platform.TextPurposePhone:
		purpose = protocols.ZwpTextInputV3ContentPurposePhone
	case platform.TextPurposeURL:
		purpose = protocols.ZwpTextInputV3ContentPurposeUrl
	}
	if st.Multiline {
		hint |= protocols.ZwpTextInputV3ContentHintMultiline
	}
	return hint, purpose
}

// cursorRect rounds a logical rectangle outward to whole surface pixels,
// at least 1x1 (a caret has zero width).
func cursorRect(r platform.Rect) (x, y, w, h int32) {
	x0 := math.Floor(float64(r.X))
	y0 := math.Floor(float64(r.Y))
	x1 := math.Ceil(float64(r.X) + float64(r.W))
	y1 := math.Ceil(float64(r.Y) + float64(r.H))
	return int32(x0), int32(y0), int32(max(1, x1-x0)), int32(max(1, y1-y0))
}

// surroundingWindow cuts st to at most limit bytes around its selection
// and returns the text with the cursor and anchor in it (GTK's
// notify_surrounding_text). A selection that fits is centered in the
// window, which is moved inside the text at its ends; a longer selection
// gets a window centered on the cursor and its anchor clamped to the
// window. The window's ends lie on character boundaries, and NUL bytes,
// which the protocol's strings cannot carry, become spaces.
func surroundingWindow(st platform.SurroundingText, limit int) (text string, cursor, anchor int) {
	text, cursor, anchor = st.Text, st.Cursor, st.Anchor
	if len(text) > limit {
		lo, hi := min(cursor, anchor), max(cursor, anchor)
		var start int
		if hi-lo <= limit {
			start = lo - (limit-(hi-lo))/2
		} else {
			start = cursor - limit/2
		}
		start = max(0, min(start, len(text)-limit))
		end := start + limit
		// Shrink to character boundaries: the start moves forward, the
		// end back, neither past the cursor (a boundary inside the
		// window).
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[start:end]
		cursor = max(0, min(cursor-start, len(text)))
		anchor = max(0, min(anchor-start, len(text)))
	}
	if strings.IndexByte(text, 0) >= 0 {
		text = strings.ReplaceAll(text, "\x00", " ")
	}
	return text, cursor, anchor
}

// --- Window side ---

// SetTextInput records the focused field's state; the seats' text inputs
// send it before the loop flushes (Backend.syncTextInputs). See
// platform.Window.
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
	if w.Closed() || w.textInput.Equal(state) {
		return
	}
	if state == nil {
		// A later state is another field: restart the session even if
		// both calls fall in one loop iteration.
		w.tiRestart = true
	}
	w.textInput = state.Clone()
}

// ResetTextInput makes the next sync resend the state with the change
// cause "other", which tells the IME to drop its composition.
func (w *Window) ResetTextInput() {
	if w.Closed() || w.textInput == nil {
		return
	}
	w.tiReset = true
}

// imeSession counts the seats with an enabled text input on the window
// and queues an IMESessionEvent when the first one starts or the last one
// ends.
func (w *Window) imeSession(delta int) {
	was := w.imeSessions > 0
	w.imeSessions += delta
	if now := w.imeSessions > 0; now != was && !w.Closed() {
		w.b.enqueue(w, platform.IMESessionEvent{Active: now})
	}
}

// closeTextInput ends the sessions on a closing window: disable while the
// surface still exists.
func (w *Window) closeTextInput() {
	for _, s := range w.b.seats {
		if ti := s.ti; ti != nil && ti.focus == w {
			if ti.enabled {
				ti.disable()
			}
			ti.end()
			ti.focus = nil
		}
	}
}

// syncTextInputs sends the text input changes of this loop iteration.
func (b *Backend) syncTextInputs() {
	if b.textInputManager == nil {
		return
	}
	for _, s := range b.seats {
		if s.ti != nil {
			s.ti.sync()
		}
	}
	for _, w := range b.loop.Windows() {
		w.tiRestart, w.tiReset = false, false
	}
}
