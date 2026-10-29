// Package edit is the editable plain-text model under core's Input and
// Textarea (ARCHITECTURE section 11, ADR-065): a UTF-8 value, a selection,
// an IME composition, an undo history and the commands that key and
// pointer handling produce. It sits on package text (cursors, selections,
// navigation and hit-testing of a Layout) and knows nothing of style,
// layout, core or the platform; it never reads a clock or the clipboard.
//
// # Value, display and selection
//
// The value (Editor.Value) is the committed text. It is always valid
// UTF-8, holds no CR, and holds LF only in multi-line mode. What is laid
// out and painted is the displayed text (Editor.Display): the value with
// the preedit of an active composition inserted where the composition
// started, and, in password mode, one Mask (U+2022) per grapheme cluster
// of that text instead of the text itself. Display and value are equal
// outside compositions and password mode; ValueToDisplay and
// DisplayToValue map offsets between them.
//
// The selection (Editor.Selection) is a text.Selection in Display offsets,
// so the caller passes it to the layout as it is (Caret, SelectionRects)
// and the selection's goal column and drag unit survive across commands.
// Its cursors sit on grapheme cluster boundaries after every operation
// (computed with UAX #29 on the value, without a layout); the layout
// snaps them again and canonicalizes affinities when it uses them. After
// an insertion the caret is Upstream, attached to the inserted text (it
// stays at the end of a line or next to a bidi run that it extends);
// after a deletion it is Downstream. SelectionRange gives the selection in
// value offsets (HTML selectionStart and selectionEnd).
//
// # Layouts
//
// The editor does not lay out text. Commands that need geometry (moving
// by character on screen, by word, by line, vertically and by page,
// pointer commands, and deletion by word or line) take the *text.Layout
// of Display, laid out and broken into lines as it is shown; every other
// command ignores the layout and may be given nil (CommandKind.NeedsLayout
// tells which). The caller keeps one layout per editor and rebuilds it
// only when Editor.DisplayVersion changed (or its style or width did):
// cursor movements never change the display, so they never reshape. A
// layout built from another string is a programming error and panics.
// Between two key events of one frame the display may change (typing,
// then an arrow key): the caller compares DisplayVersion before every
// command that needs a layout.
//
// Coordinates of pointer commands (Command.X, Command.Y) are layout
// coordinates: the caller subtracts the content box origin and the scroll
// offset first.
//
// # Commands, beforeinput and change reporting
//
// A Command (an InsertText, a DeleteWordBackward, a MoveDown with Extend,
// a ClickAt...) is turned into a Plan by Editor.Plan, which computes the
// whole effect without changing anything, and carried out by Editor.Apply.
// Between the two the caller fires beforeinput with Plan.InputType and
// Plan.Data (when InputType is set; movements and Copy have none) and does
// not apply a plan whose event was canceled. Applying a plan after any
// other change of the editor (a beforeinput handler that set the value)
// does nothing, so a plan is never applied to a state it was not computed
// for. Editor.Do plans and applies at once.
//
// Every operation returns a Result: whether the value, the selection and
// the display changed, the InputType and Data of the edit (W3C Input
// Events Level 2 names: insertText, insertFromPaste, insertLineBreak,
// deleteContentBackward, deleteWordBackward, deleteSoftLineBackward,
// deleteByCut, historyUndo, insertCompositionText, insertFromComposition,
// deleteCompositionText...), IsComposing, and the clipboard text of Copy
// and Cut. The caller builds its input event and its
// ChangeEvent{Value, InputType, Data, IsComposing} from it.
//
// Commands follow browser semantics: an insertion or deletion replaces a
// non-empty selection; Backspace and Delete with a selection delete it;
// MoveLeft and MoveRight collapse a selection to its edge; Copy and Cut
// return the text instead of touching a clipboard, and Paste takes it;
// ReadOnly refuses every change of the value but keeps selection, Copy and
// navigation. While a composition is active every command is a no-op
// (Parley's PlainEditor does the same): the IME owns the keys, and the
// caller finishes the composition (FinishComposition) before a click.
//
// # Text rules
//
//   - Inserted text (typing, paste, IME) is cleaned: invalid UTF-8 becomes
//     U+FFFD; C0 and C1 controls and DEL are removed, except tab; each
//     line break (CR LF, CR or LF) becomes LF in multi-line mode and a
//     space in single-line mode (Chromium and Firefox paste into <input>
//     this way). Typed text never has controls (ADR-050): Enter, Tab,
//     Backspace and Delete arrive as keys and map to commands.
//   - Values set by SetText follow HTML's value sanitization: line breaks
//     are stripped in single-line mode and normalized to LF in multi-line
//     mode; nothing else is changed, and maxlength does not apply.
//   - MaxLength counts UTF-16 code units, like HTML maxlength, so lengths
//     match what users see in browsers ("😀" counts 2). Inserted text is
//     truncated to the room left (the selection it replaces counts as
//     room) on a grapheme cluster boundary of the inserted text; text that
//     does not fit at all is not inserted. A composition is not limited
//     until it is committed.
//   - Backspace deletes one code point, except that emoji sequences
//     (flags, keycaps, skin tones, tag and ZWJ sequences) go as a whole and
//     a variation selector goes with its base: Blink's
//     BackspaceStateMachine. A decomposed "é" loses its accent first, and
//     Devanagari or Thai clusters lose their last code point, as in
//     Chromium. Delete removes a whole grapheme cluster. In password mode
//     both delete whole grapheme clusters (one mask each).
//   - Word movement and deletion use the layout's UAX #29 words with
//     macOS/GTK conventions or, with WordsWindows, the start of the next
//     word for ctrl+right and ctrl+Delete. In password mode the displayed
//     masks hold no words, so word movement and deletion go to the ends of
//     the text and a double click selects everything, which hides the
//     password's word structure as browsers and macOS do.
//   - Deleting to the start or end of a line at that line edge deletes
//     like Backspace or Delete (the line break).
//   - Password mode disables Copy and Cut and exposes no SelectedText.
//
// # Undo
//
// The history keeps up to Options.UndoLimit steps (DefaultUndoLimit;
// negative disables it). Undo restores the value and the selection before
// the step; Redo the value and the selection after it; both are edits with
// beforeinput (historyUndo, historyRedo). Typing coalesces into one step
// until the selection changes by anything but the typing itself, until
// another kind of edit (Backspace and Delete runs coalesce among
// themselves; paste, cut, word and line deletions, compositions and
// SetText never coalesce), until white space starts after a word (VS
// Code's rule: each word with its leading space is a step), or until a
// pause longer than Options.UndoDelay measured with the caller's clock
// Options.Now (ProseMirror's newGroupDelay, 500 ms by default; no clock
// means no pause rule). Chromium coalesces a whole typing run instead;
// the smaller steps are deliberate. See history.go.
//
// SetText with ResetHistory clears the history (a new document); with
// RecordHistory the change is one undo step. Setting the value it already
// has changes nothing, so a controlled field that echoes the value back
// keeps its history and selection.
//
// # IME composition
//
// SetPreedit shows the IME's preedit text with its cursor; the first
// preedit of a composition deletes the selection and the composition
// starts there. CommitComposition replaces the preedit with the final
// text (maxlength applies), CancelComposition removes it, and
// FinishComposition commits the preedit as it is. The committed
// composition, with the selection it replaced, is one undo step.
// DeleteSurrounding implements text-input-v3's delete_surrounding_text
// (UTF-8 byte lengths before and after the selection, around the preedit
// during a composition). Composition returns the preedit's range in
// Display, to underline, and the IME's cursor; the selection is that
// cursor. Results of preedit updates have IsComposing set and InputType
// insertCompositionText; the value (Editor.Value) never contains the
// preedit, whereas a browser's input.value does: the caller uses Display
// when it needs the browser's view.
//
// text-input-v3 sends a done event that applies, in order, the removal of
// the preedit, delete_surrounding_text, commit_string and the new preedit.
// The caller maps it to DeleteSurrounding (when lengths are set), then
// CommitComposition (when there is a commit string), then SetPreedit (when
// there is a new preedit), or CancelComposition when the composition
// ended without a commit. Windows (IMM32) and macOS (NSTextInputClient)
// count in UTF-16 code units: their backends convert with internal/utf16x.
// Replacement ranges (reconversion) are not supported.
//
// # Not here
//
// Clipboard access, key bindings (which key makes which command, per
// platform), caret blinking, scrolling the caret into view, and drawing
// (selection highlight, preedit underline) belong to the caller.
package edit
