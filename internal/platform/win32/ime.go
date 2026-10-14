package win32

import (
	"encoding/binary"
	"image"
	"math"
	utf16pkg "unicode/utf16"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/utf16x"
)

// The IME through IMM32 (ADR-071). This file is the OS-independent half:
// turning what one WM_IME_COMPOSITION carries into one platform.IMEEvent,
// placing the IME's windows, and the RECONVERTSTRING answers. ime_windows.go
// reads the messages and calls IMM32. See "IME" in doc.go for the design.

// IMM32 values that the pure logic depends on (imm.h); the messages and
// the rest live in ime_windows.go.
const (
	// WM_IME_COMPOSITION lParam: what changed.
	gcsCompReadStr = 0x0001
	gcsCompStr     = 0x0008
	gcsCompAttr    = 0x0010
	gcsCompClause  = 0x0020
	gcsCursorPos   = 0x0080
	gcsDeltaStart  = 0x0100
	gcsResultStr   = 0x0800

	// GCS_COMPATTR values, one byte per UTF-16 code unit of the composition.
	attrInput              = 0x00
	attrTargetConverted    = 0x01
	attrConverted          = 0x02
	attrTargetNotConverted = 0x03

	// COMPOSITIONFORM and CANDIDATEFORM styles.
	cfsPoint        = 0x0002
	cfsCandidatePos = 0x0040
	cfsExclude      = 0x0080

	// Primary language of the input locale: the IMEs of these languages
	// place their windows differently (Chromium's imm32_manager.cc).
	langChinese = 0x04
)

// compUpdate is what the backend read from the input context for one
// WM_IME_COMPOSITION: the flags of lParam, and the strings those flags say
// changed (UTF-16, as IMM32 returns them).
type compUpdate struct {
	flags  uint32
	result []uint16 // GCS_RESULTSTR, when flagged
	comp   []uint16 // GCS_COMPSTR, when compChanged
	attrs  []byte   // GCS_COMPATTR, one byte per code unit of comp
	cursor int      // GCS_CURSORPOS in code units of comp; negative: none
}

// compChanged reports whether lParam says that the composition string, its
// attributes or its cursor changed; the backend then reads all three.
func compChanged(flags uint32) bool {
	return flags&(gcsCompStr|gcsCompAttr|gcsCompClause|gcsCursorPos|gcsDeltaStart) != 0
}

// imeComposer is the composition of one window as the app has been told:
// the preedit and cursor of the last IMEEvent. It decides which IMEEvent
// (if any) each message makes.
type imeComposer struct {
	composing bool // between WM_IME_STARTCOMPOSITION and WM_IME_ENDCOMPOSITION
	preedit   string
	cursor    [2]int
}

// open reports whether the IME has a composition the app knows of: started
// and not ended, or a preedit shown.
func (c *imeComposer) open() bool { return c.composing || c.preedit != "" }

// clear forgets the composition (the session ended, or the app reset it).
func (c *imeComposer) clear() { *c = imeComposer{} }

// start handles WM_IME_STARTCOMPOSITION. It produces no event: the first
// WM_IME_COMPOSITION carries the preedit.
func (c *imeComposer) start() { c.composing = true }

// update handles one WM_IME_COMPOSITION: the result string becomes Commit,
// the composition string the new Preedit, both in one event when one
// message carries both (Japanese IMEs commit a clause and go on composing
// the rest; Google Japanese Input and ATOK send both). lParam 0 clears the
// composition (the IME cancelled it). A result without a composition string
// ends the composition. Flags that change neither (the reading string) keep
// the preedit. The event is returned only when it changes something.
func (c *imeComposer) update(u compUpdate) (platform.IMEEvent, bool) {
	var ev platform.IMEEvent
	if u.flags&gcsResultStr != 0 {
		ev.Commit = imeText(u.result)
	}
	switch {
	case compChanged(u.flags):
		ev.Preedit = imeText(u.comp)
		ev.CursorStart, ev.CursorEnd = preeditCursor(u.comp, ev.Preedit, u.attrs, u.cursor)
	case u.flags == 0 || u.flags&gcsResultStr != 0:
		// Cleared, or committed without a new composition.
	default:
		ev.Preedit, ev.CursorStart, ev.CursorEnd = c.preedit, c.cursor[0], c.cursor[1]
	}
	return c.deliver(ev)
}

// end handles WM_IME_ENDCOMPOSITION. A preedit still shown was cancelled:
// the IME ended the composition without a result (Escape, deleting the last
// character with an IME that sends no empty composition first). The
// Microsoft Korean IME ends the composition before it sends the result
// (crbug.com/647150): the preedit is removed here and the late result is a
// Commit without a composition, which the app inserts like typed text.
func (c *imeComposer) end() (platform.IMEEvent, bool) {
	c.composing = false
	return c.deliver(platform.IMEEvent{})
}

// deliver records ev as the state the app has and reports whether it
// changes anything.
func (c *imeComposer) deliver(ev platform.IMEEvent) (platform.IMEEvent, bool) {
	cursor := [2]int{ev.CursorStart, ev.CursorEnd}
	if ev.Commit == "" && ev.Preedit == c.preedit && cursor == c.cursor {
		return ev, false
	}
	c.preedit, c.cursor = ev.Preedit, cursor
	return ev, true
}

// imeText converts IME text to UTF-8: unpaired surrogates become U+FFFD and
// NUL is dropped (platform.SanitizeIMEText).
func imeText(u []uint16) string {
	return platform.SanitizeIMEText(string(utf16pkg.Decode(u)))
}

// preeditCursor returns the preedit cursor as UTF-8 byte offsets of preedit
// (the UTF-8 form of comp). The target clause, the one the conversion
// candidates are for (GCS_COMPATTR: ATTR_TARGET_CONVERTED, or
// ATTR_TARGET_NOTCONVERTED for unconverted text the user selected), is the
// highlighted range, as winit and Chromium do: Microsoft IME puts the
// cursor at the end of the target clause during a conversion, and the app
// shows which clause Space converts next. Without a target clause (typing)
// the cursor is GCS_CURSORPOS; without either it is hidden (-1, -1), as
// for the Korean IME, which has no cursor in its one-syllable composition.
func preeditCursor(comp []uint16, preedit string, attrs []byte, cursor int) (int, int) {
	if preedit == "" {
		return 0, 0
	}
	if len(attrs) == len(comp) {
		first, last := -1, len(comp)
		for i, a := range attrs {
			target := a == attrTargetConverted || a == attrTargetNotConverted
			if first < 0 && target {
				first = i
			} else if first >= 0 && !target {
				last = i
				break
			}
		}
		if first >= 0 {
			return platform.SanitizePreeditCursor(preedit, utf16x.ToByte(preedit, first), utf16x.ToByte(preedit, last))
		}
	}
	if cursor < 0 {
		return -1, -1
	}
	b := utf16x.ToByte(preedit, cursor)
	return platform.SanitizePreeditCursor(preedit, b, b)
}

// imeCaretRect converts the cursor area of a text input state (logical
// pixels) to physical client pixels, rounded outwards and at least one
// pixel wide and high (a caret has no width, and a zero-size rectangle
// tells some IMEs nothing).
func imeCaretRect(a platform.Rect, scale float64) image.Rectangle {
	x0 := math.Floor(float64(a.X) * scale)
	y0 := math.Floor(float64(a.Y) * scale)
	x1 := math.Ceil((float64(a.X) + float64(a.W)) * scale)
	y1 := math.Ceil((float64(a.Y) + float64(a.H)) * scale)
	r := image.Rect(int(x0), int(y0), int(x1), int(y1))
	r.Max.X = max(r.Max.X, r.Min.X+1)
	r.Max.Y = max(r.Max.Y, r.Min.Y+1)
	return r
}

// candForm is one ImmSetCandidateWindow call (index 0).
type candForm struct {
	style uint32
	pos   image.Point
	area  image.Rectangle
}

// imePlacement is where the IME's windows go for a caret rectangle.
type imePlacement struct {
	comp  image.Point // ImmSetCompositionWindow, CFS_POINT
	cands []candForm  // ImmSetCandidateWindow calls, in order
	caret image.Point // SetCaretPos of the system caret, when there is one
}

// placeIME follows Chromium's IMM32Manager::MoveImeWindow, which encodes
// what each family of IMEs reads:
//
//   - the composition window (hidden: Tela draws the preedit) at the
//     caret's top-left corner, CFS_POINT (Chromium and Firefox), for IMEs
//     that place their candidates relative to it;
//   - CFS_CANDIDATEPOS at the top-left corner: Chinese and Japanese IMEs
//     put the candidate list below the line that starts there;
//   - CFS_EXCLUDE with the caret rectangle, the rectangle the candidate
//     list must not cover, for Japanese and Korean IMEs (Korean ones one
//     pixel lower, as they take the bottom-left corner of the caret);
//     Chinese IMEs get no CFS_EXCLUDE, some of them hang on it;
//   - the system caret, which some Chinese and Japanese IMEs read with
//     GetCaretPos instead: at the caret's top-left corner, at its
//     bottom-left for Japanese IMEs.
func placeIME(caret image.Rectangle, lang uint16) imePlacement {
	x, y := caret.Min.X, caret.Min.Y
	p := imePlacement{comp: caret.Min, caret: caret.Min}
	if lang == langJapanese {
		p.caret = image.Pt(x, caret.Max.Y)
	}
	p.cands = append(p.cands, candForm{style: cfsCandidatePos, pos: image.Pt(x, y)})
	if lang == langChinese {
		return p
	}
	if lang == langKorean {
		y++
	}
	p.cands = append(p.cands, candForm{style: cfsExclude, pos: image.Pt(x, y),
		area: image.Rect(x, y, x+caret.Dx(), y+caret.Dy())})
	return p
}

// needsSystemCaret reports whether the IMEs of a language may read the
// system caret to place their windows (Chromium creates one for Chinese and
// Japanese IMEs during a composition).
func needsSystemCaret(lang uint16) bool { return lang == langChinese || lang == langJapanese }

// reconvertHeader is the size of RECONVERTSTRING: eight DWORDs, followed by
// the string.
const reconvertHeader = 32

// reconvertString builds a RECONVERTSTRING with its string: text, with the
// composition and target ranges both [off, off+n) in code units of text
// (the header stores the offsets in bytes).
func reconvertString(text []uint16, off, n int) []byte {
	buf := make([]byte, reconvertHeader+2*len(text))
	le := binary.LittleEndian
	le.PutUint32(buf[0:], uint32(len(buf)))  // dwSize
	le.PutUint32(buf[4:], 0)                 // dwVersion
	le.PutUint32(buf[8:], uint32(len(text))) // dwStrLen
	le.PutUint32(buf[12:], reconvertHeader)  // dwStrOffset
	le.PutUint32(buf[16:], uint32(n))        // dwCompStrLen
	le.PutUint32(buf[20:], uint32(2*off))    // dwCompStrOffset
	le.PutUint32(buf[24:], uint32(n))        // dwTargetStrLen
	le.PutUint32(buf[28:], uint32(2*off))    // dwTargetStrOffset
	for i, u := range text {
		le.PutUint16(buf[reconvertHeader+2*i:], u)
	}
	return buf
}

// feedContext is how much committed text IMR_DOCUMENTFEED gives the IME on
// each side of the caret, in characters: enough for the context the
// conversion looks at, without copying a long value on every request.
const feedContext = 256

// documentFeed answers IMR_DOCUMENTFEED, with which Japanese IMEs read the
// text around the composition to convert it better: the field's text as
// shown (the committed text with the preedit at the caret), cut to
// feedContext characters on each side, with the preedit as the composition
// and target range, or an empty range at the selection's start when there
// is no composition (Chromium's OnDocumentFeed). nil without surrounding
// text.
func documentFeed(st *platform.SurroundingText, preedit string) []byte {
	if st == nil {
		return nil
	}
	at := min(st.Cursor, st.Anchor)
	if preedit != "" {
		at = st.Cursor
	}
	before := []rune(st.Text[:at])
	after := []rune(st.Text[at:]) // a selection is part of the context after the caret
	before = before[max(0, len(before)-feedContext):]
	after = after[:min(len(after), feedContext)]
	text := utf16pkg.Encode(before)
	off := len(text)
	pre := utf16pkg.Encode([]rune(preedit))
	text = append(text, pre...)
	text = append(text, utf16pkg.Encode(after)...)
	return reconvertString(text, off, len(pre))
}

// reconvertSelection answers IMR_RECONVERTSTRING, with which the IME
// reconverts committed text (the conversion key on a selection): the
// selected text, as composition and target, when there is a selection and
// no composition (Chromium's OnReconvertString). The IME then starts a
// composition with that text, and the app's first preedit replaces the
// selection. nil when there is nothing to reconvert.
func reconvertSelection(st *platform.SurroundingText, composing bool) []byte {
	if st == nil || composing || st.Cursor == st.Anchor {
		return nil
	}
	sel := utf16pkg.Encode([]rune(st.Text[min(st.Cursor, st.Anchor):max(st.Cursor, st.Anchor)]))
	return reconvertString(sel, 0, len(sel))
}
