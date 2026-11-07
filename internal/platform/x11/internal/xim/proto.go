package xim

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var le = binary.LittleEndian

// Major opcodes (spec section 6, XimProto.h).
const (
	OpConnect                  = 1
	OpConnectReply             = 2
	OpDisconnect               = 3
	OpDisconnectReply          = 4
	OpAuthRequired             = 10
	OpAuthReply                = 11
	OpAuthNext                 = 12
	OpAuthSetup                = 13
	OpAuthNG                   = 14
	OpError                    = 20
	OpOpen                     = 30
	OpOpenReply                = 31
	OpClose                    = 32
	OpCloseReply               = 33
	OpRegisterTriggerKeys      = 34
	OpTriggerNotify            = 35
	OpTriggerNotifyReply       = 36
	OpSetEventMask             = 37
	OpEncodingNegotiation      = 38
	OpEncodingNegotiationReply = 39
	OpQueryExtension           = 40
	OpQueryExtensionReply      = 41
	OpSetIMValues              = 42
	OpSetIMValuesReply         = 43
	OpGetIMValues              = 44
	OpGetIMValuesReply         = 45
	OpCreateIC                 = 50
	OpCreateICReply            = 51
	OpDestroyIC                = 52
	OpDestroyICReply           = 53
	OpSetICValues              = 54
	OpSetICValuesReply         = 55
	OpGetICValues              = 56
	OpGetICValuesReply         = 57
	OpSetICFocus               = 58
	OpUnsetICFocus             = 59
	OpForwardEvent             = 60
	OpSync                     = 61
	OpSyncReply                = 62
	OpCommit                   = 63
	OpResetIC                  = 64
	OpResetICReply             = 65
	OpGeometry                 = 70
	OpStrConversion            = 71
	OpStrConversionReply       = 72
	OpPreeditStart             = 73
	OpPreeditStartReply        = 74
	OpPreeditDraw              = 75
	OpPreeditCaret             = 76
	OpPreeditCaretReply        = 77
	OpPreeditDone              = 78
	OpStatusStart              = 79
	OpStatusDraw               = 80
	OpStatusDone               = 81
	OpPreeditState             = 82
)

// Input styles (XIMStyle bits).
const (
	PreeditArea      = 0x0001
	PreeditCallbacks = 0x0002
	PreeditPosition  = 0x0004
	PreeditNothing   = 0x0008
	PreeditNone      = 0x0010
	StatusArea       = 0x0100
	StatusCallbacks  = 0x0200
	StatusNothing    = 0x0400
	StatusNone       = 0x0800
)

// Attribute types (XIMATTR type field).
const (
	TypeSeparator = 0
	TypeCard8     = 1
	TypeCard16    = 2
	TypeCard32    = 3
	TypeString8   = 4
	TypeWindow    = 5
	TypeStyles    = 10
	TypeRectangle = 11
	TypePoint     = 12
	TypeFontSet   = 13
	TypeNested    = 0x7fff
)

// Attribute names used by the client (XNxxx in Xlib).
const (
	AttrQueryInputStyle   = "queryInputStyle"
	AttrInputStyle        = "inputStyle"
	AttrClientWindow      = "clientWindow"
	AttrFocusWindow       = "focusWindow"
	AttrFilterEvents      = "filterEvents"
	AttrPreeditAttributes = "preeditAttributes"
	AttrStatusAttributes  = "statusAttributes"
	AttrSpotLocation      = "spotLocation"
	AttrArea              = "area"
)

// Flags of XIM_FORWARD_EVENT and XIM_COMMIT.
const (
	FlagSynchronous = 0x0001 // the receiver answers with XIM_SYNC_REPLY
	// Forward event only.
	FlagRequestFiltering    = 0x0002
	FlagRequestLookupString = 0x0004
	// Commit only.
	CommitChars  = 0x0002 // XLookupChars: a string follows
	CommitKeySym = 0x0004 // XLookupKeySym: a keysym follows
)

// XIM_PREEDIT_DRAW status bits.
const (
	DrawNoString   = 0x0001
	DrawNoFeedback = 0x0002
)

// XIMFeedback bits of a preedit character.
const (
	FeedbackReverse   = 0x0001
	FeedbackUnderline = 0x0002
	FeedbackHighlight = 0x0004
)

// XIM_PREEDIT_CARET directions and styles.
const (
	CaretForwardChar      = 0
	CaretBackwardChar     = 1
	CaretForwardWord      = 2
	CaretBackwardWord     = 3
	CaretUp               = 4
	CaretDown             = 5
	CaretNextLine         = 6
	CaretPreviousLine     = 7
	CaretLineStart        = 8
	CaretLineEnd          = 9
	CaretAbsolutePosition = 10
	CaretDontChange       = 11

	CaretInvisible = 0
	CaretPrimary   = 1
	CaretSecondary = 2
)

// XIM_ERROR flags and the codes the client looks at.
const (
	ErrIMIDValid = 0x0001
	ErrICIDValid = 0x0002

	ErrBadAlloc           = 1
	ErrBadStyle           = 2
	ErrBadClientWindow    = 3
	ErrBadFocusWindow     = 4
	ErrBadSpotLocation    = 6
	ErrBadProtocol        = 13
	ErrLocaleNotSupported = 16
	ErrBadSomething       = 999
)

// EndianLittle is the byte-order byte of XIM_CONNECT ('l').
const EndianLittle = 0x6c

// ErrShort is returned for a message body too short for its fields.
var ErrShort = errors.New("xim: message too short")

// --- writing ---

// writer builds a message body.
type writer struct{ b []byte }

func (w *writer) u8(v byte)    { w.b = append(w.b, v) }
func (w *writer) u16(v uint16) { w.b = le.AppendUint16(w.b, v) }
func (w *writer) u32(v uint32) { w.b = le.AppendUint32(w.b, v) }
func (w *writer) raw(p []byte) { w.b = append(w.b, p...) }

// pad4 pads the body to a multiple of 4 bytes.
func (w *writer) pad4() {
	for len(w.b)%4 != 0 {
		w.b = append(w.b, 0)
	}
}

// message returns the full message: header and body (padded).
func message(major byte, w *writer) []byte {
	w.pad4()
	out := make([]byte, 4, 4+len(w.b))
	out[0], out[1] = major, 0
	le.PutUint16(out[2:], uint16(len(w.b)/4))
	return append(out, w.b...)
}

// Attribute is an attribute ID and its encoded value.
type Attribute struct {
	ID    uint16
	Value []byte
}

// appendAttributes writes attributes, each padded to 4 bytes.
func appendAttributes(w *writer, attrs []Attribute) {
	for _, a := range attrs {
		w.u16(a.ID)
		w.u16(uint16(len(a.Value)))
		w.raw(a.Value)
		w.pad4()
	}
}

// attributesLen is the encoded size of attrs.
func attributesLen(attrs []Attribute) int {
	n := 0
	for _, a := range attrs {
		n += 4 + (len(a.Value)+3)&^3
	}
	return n
}

// Card32 encodes a CARD32 attribute value (inputStyle, windows).
func Card32(v uint32) []byte { return le.AppendUint32(nil, v) }

// Point encodes an XPoint attribute value (spotLocation).
func Point(x, y int16) []byte {
	b := le.AppendUint16(nil, uint16(x))
	return le.AppendUint16(b, uint16(y))
}

// Nested encodes a nested list (preeditAttributes) as an attribute value.
func Nested(attrs ...Attribute) []byte {
	w := &writer{}
	appendAttributes(w, attrs)
	return w.b
}

// EncodeConnect is XIM_CONNECT: little-endian, protocol 1.0, no
// authentication protocols.
func EncodeConnect() []byte {
	w := &writer{}
	w.u8(EndianLittle)
	w.u8(0)
	w.u16(1) // major
	w.u16(0) // minor
	w.u16(0) // number of auth protocol names
	return message(OpConnect, w)
}

// EncodeOpen is XIM_OPEN with a locale name (STR).
func EncodeOpen(locale string) []byte {
	locale = locale[:min(len(locale), 255)]
	w := &writer{}
	w.u8(byte(len(locale)))
	w.raw([]byte(locale))
	return message(OpOpen, w)
}

// EncodeIDs is a message whose body is the input-method ID and the
// input-context ID: XIM_SET_IC_FOCUS, XIM_UNSET_IC_FOCUS, XIM_SYNC,
// XIM_SYNC_REPLY, XIM_RESET_IC, XIM_DESTROY_IC (and the server's replies
// with the same layout).
func EncodeIDs(op byte, imid, icid uint16) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(icid)
	return message(op, w)
}

// EncodeIM is a message whose body is the input-method ID and two bytes
// of padding: XIM_CLOSE (and XIM_CLOSE_REPLY).
func EncodeIM(op byte, imid uint16) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(0)
	return message(op, w)
}

// EncodeEmpty is a message without a body: XIM_DISCONNECT (and its reply).
func EncodeEmpty(op byte) []byte { return message(op, &writer{}) }

// appendSTRList writes a CARD16 byte length and a list of STR (CARD8
// length and bytes).
func appendSTRList(w *writer, names []string) {
	n := 0
	for _, s := range names {
		n += 1 + len(s)
	}
	w.u16(uint16(n))
	for _, s := range names {
		w.u8(byte(len(s)))
		w.raw([]byte(s))
	}
}

// EncodeQueryExtension is XIM_QUERY_EXTENSION (an empty list asks for every
// extension).
func EncodeQueryExtension(imid uint16, names []string) []byte {
	w := &writer{}
	w.u16(imid)
	appendSTRList(w, names)
	w.pad4()
	return message(OpQueryExtension, w)
}

// EncodeEncodingNegotiation is XIM_ENCODING_NEGOTIATION with encoding
// names in order of preference and no detailed encoding information.
func EncodeEncodingNegotiation(imid uint16, names []string) []byte {
	w := &writer{}
	w.u16(imid)
	appendSTRList(w, names)
	w.pad4()
	w.u16(0) // byte length of the encoding information list
	w.u16(0) // unused
	return message(OpEncodingNegotiation, w)
}

// EncodeGetIMValues is XIM_GET_IM_VALUES for attribute IDs.
func EncodeGetIMValues(imid uint16, ids []uint16) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(uint16(2 * len(ids)))
	for _, id := range ids {
		w.u16(id)
	}
	return message(OpGetIMValues, w)
}

// EncodeCreateIC is XIM_CREATE_IC with IC attributes.
func EncodeCreateIC(imid uint16, attrs []Attribute) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(uint16(attributesLen(attrs)))
	appendAttributes(w, attrs)
	return message(OpCreateIC, w)
}

// EncodeSetICValues is XIM_SET_IC_VALUES.
func EncodeSetICValues(imid, icid uint16, attrs []Attribute) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(icid)
	w.u16(uint16(attributesLen(attrs)))
	w.u16(0) // unused
	appendAttributes(w, attrs)
	return message(OpSetICValues, w)
}

// EncodeForwardEvent is XIM_FORWARD_EVENT with a 32-byte core X event
// (wire format, client byte order). serial is the high 16 bits of the
// event's serial number.
func EncodeForwardEvent(imid, icid, flag, serial uint16, ev []byte) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(icid)
	w.u16(flag)
	w.u16(serial)
	var e [32]byte
	copy(e[:], ev)
	w.raw(e[:])
	return message(OpForwardEvent, w)
}

// EncodePreeditStartReply is XIM_PREEDIT_START_REPLY with the maximum
// preedit length the client accepts (-1: no limit).
func EncodePreeditStartReply(imid, icid uint16, ret int32) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(icid)
	w.u32(uint32(ret))
	return message(OpPreeditStartReply, w)
}

// EncodePreeditCaretReply is XIM_PREEDIT_CARET_REPLY with the caret
// position after the move.
func EncodePreeditCaretReply(imid, icid uint16, pos int32) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(icid)
	w.u32(uint32(pos))
	return message(OpPreeditCaretReply, w)
}

// --- reading ---

// Message is one decoded message: its major opcode and body.
type Message struct {
	Major, Minor byte
	Body         []byte
}

func (m Message) String() string { return fmt.Sprintf("xim op %d (%d bytes)", m.Major, len(m.Body)) }

// reader reads a message body.
type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.off+n > len(r.b) {
		r.err = ErrShort
		return false
	}
	return true
}

func (r *reader) u8() byte {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.off]
	r.off++
	return v
}

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := le.Uint16(r.b[r.off:])
	r.off += 2
	return v
}

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := le.Uint32(r.b[r.off:])
	r.off += 4
	return v
}

func (r *reader) bytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	v := r.b[r.off : r.off+n]
	r.off += n
	return v
}

// pad4 skips to the next multiple of 4 bytes of the body (which starts on
// a 4-byte boundary of the message), leniently at its end.
func (r *reader) pad4() {
	if r.err == nil {
		r.off = min((r.off+3)&^3, len(r.b))
	}
}

func (r *reader) left() int { return len(r.b) - r.off }

// sub returns a reader of the next n bytes.
func (r *reader) sub(n int) *reader { return &reader{b: r.bytes(n), err: r.err} }

// Attr describes an attribute the input method supports
// (XIM_OPEN_REPLY): its ID, type and name.
type Attr struct {
	ID   uint16
	Type uint16
	Name string
}

// readAttrs reads XIMATTR (or XICATTR) entries until r ends.
func readAttrs(r *reader) []Attr {
	var out []Attr
	for r.left() >= 6 && r.err == nil {
		a := Attr{ID: r.u16(), Type: r.u16()}
		n := int(r.u16())
		a.Name = string(r.bytes(n))
		r.pad4()
		out = append(out, a)
	}
	return out
}

// readAttributes reads ATTRIBUTE entries (ID, length, value, padding)
// until r ends.
func readAttributes(r *reader) []Attribute {
	var out []Attribute
	for r.left() >= 4 && r.err == nil {
		a := Attribute{ID: r.u16()}
		n := int(r.u16())
		a.Value = r.bytes(n)
		r.pad4()
		if r.err == nil {
			out = append(out, a)
		}
	}
	return out
}

// OpenReply is XIM_OPEN_REPLY.
type OpenReply struct {
	IMID    uint16
	IMAttrs []Attr
	ICAttrs []Attr
}

// ParseOpenReply decodes XIM_OPEN_REPLY.
func ParseOpenReply(body []byte) (OpenReply, error) {
	r := &reader{b: body}
	o := OpenReply{IMID: r.u16()}
	n := int(r.u16())
	o.IMAttrs = readAttrs(r.sub(n))
	n = int(r.u16())
	r.u16() // unused
	o.ICAttrs = readAttrs(r.sub(n))
	return o, r.err
}

// ParseIDs decodes the input-method and input-context IDs that start most
// bodies.
func ParseIDs(body []byte) (imid, icid uint16, err error) {
	r := &reader{b: body}
	imid, icid = r.u16(), r.u16()
	return imid, icid, r.err
}

// ParseConnectReply decodes XIM_CONNECT_REPLY: the server's protocol
// version.
func ParseConnectReply(body []byte) (major, minor uint16, err error) {
	return ParseIDs(body)
}

// ParseEncodingNegotiationReply decodes XIM_ENCODING_NEGOTIATION_REPLY:
// category 0 is an index into the names the client sent; -1 means the
// default, COMPOUND_TEXT.
func ParseEncodingNegotiationReply(body []byte) (imid, category uint16, index int16, err error) {
	r := &reader{b: body}
	imid, category, index = r.u16(), r.u16(), int16(r.u16())
	return imid, category, index, r.err
}

// ParseGetIMValuesReply decodes XIM_GET_IM_VALUES_REPLY.
func ParseGetIMValuesReply(body []byte) (imid uint16, attrs []Attribute, err error) {
	r := &reader{b: body}
	imid = r.u16()
	n := int(r.u16())
	attrs = readAttributes(r.sub(n))
	return imid, attrs, r.err
}

// ParseStyles decodes an XIMStyles value (queryInputStyle): a count,
// padding, and the styles.
func ParseStyles(value []byte) ([]uint32, error) {
	r := &reader{b: value}
	n := int(r.u16())
	r.u16()
	out := make([]uint32, 0, min(n, 64))
	for range n {
		v := r.u32()
		if r.err != nil {
			break
		}
		out = append(out, v)
	}
	return out, r.err
}

// SetEventMask is XIM_SET_EVENT_MASK: which events the client forwards and
// which of those it forwards synchronously (X event mask bits). ICID 0
// applies to the whole input method.
type SetEventMask struct {
	IMID, ICID    uint16
	Forward, Sync uint32
}

// ParseSetEventMask decodes XIM_SET_EVENT_MASK.
func ParseSetEventMask(body []byte) (SetEventMask, error) {
	r := &reader{b: body}
	m := SetEventMask{IMID: r.u16(), ICID: r.u16(), Forward: r.u32(), Sync: r.u32()}
	return m, r.err
}

// Error is XIM_ERROR.
type Error struct {
	IMID, ICID uint16
	Flag       uint16
	Code       uint16
	Detail     string
}

func (e Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("XIM error %d: %s", e.Code, e.Detail)
	}
	return fmt.Sprintf("XIM error %d", e.Code)
}

// ParseError decodes XIM_ERROR.
func ParseError(body []byte) (Error, error) {
	r := &reader{b: body}
	e := Error{IMID: r.u16(), ICID: r.u16(), Flag: r.u16(), Code: r.u16()}
	n := int(r.u16())
	r.u16() // type of the detail
	if n > 0 && r.left() > 0 {
		e.Detail = string(r.bytes(min(n, r.left())))
	}
	return e, r.err
}

// ForwardEvent is XIM_FORWARD_EVENT: a core X event (32 bytes, wire
// format).
type ForwardEvent struct {
	IMID, ICID uint16
	Flag       uint16
	Serial     uint16
	Event      [32]byte
}

// ParseForwardEvent decodes XIM_FORWARD_EVENT.
func ParseForwardEvent(body []byte) (ForwardEvent, error) {
	r := &reader{b: body}
	f := ForwardEvent{IMID: r.u16(), ICID: r.u16(), Flag: r.u16(), Serial: r.u16()}
	copy(f.Event[:], r.bytes(32))
	return f, r.err
}

// Commit is XIM_COMMIT: a string (CommitChars), a keysym (CommitKeySym)
// or both. Text is in the negotiated encoding.
type Commit struct {
	IMID, ICID uint16
	Flag       uint16
	KeySym     uint32
	Text       []byte
}

// ParseCommit decodes XIM_COMMIT. With only XLookupChars the string
// follows the flag; with a keysym, two bytes of padding, the keysym, and
// then (XLookupBoth) the string.
func ParseCommit(body []byte) (Commit, error) {
	r := &reader{b: body}
	c := Commit{IMID: r.u16(), ICID: r.u16(), Flag: r.u16()}
	if c.Flag&CommitKeySym != 0 {
		r.u16() // unused
		c.KeySym = r.u32()
	}
	if c.Flag&CommitChars != 0 {
		n := int(r.u16())
		c.Text = r.bytes(n)
	}
	return c, r.err
}

// PreeditDraw is XIM_PREEDIT_DRAW: replace ChgLength characters at
// ChgFirst with Text (unless Status has DrawNoString), with one feedback
// per new character; Caret is the new caret position. Positions count
// characters.
type PreeditDraw struct {
	IMID, ICID uint16
	Caret      int32
	ChgFirst   int32
	ChgLength  int32
	Status     uint32
	Text       []byte
	Feedback   []uint32
}

// ParsePreeditDraw decodes XIM_PREEDIT_DRAW, leniently: a message that
// ends before the string or the feedback list has none.
func ParsePreeditDraw(body []byte) (PreeditDraw, error) {
	r := &reader{b: body}
	d := PreeditDraw{IMID: r.u16(), ICID: r.u16(), Caret: int32(r.u32()), ChgFirst: int32(r.u32()),
		ChgLength: int32(r.u32()), Status: r.u32()}
	if r.err != nil {
		return d, r.err
	}
	if r.left() >= 2 {
		n := int(r.u16())
		d.Text = r.bytes(min(n, r.left()))
		r.pad4()
	}
	if r.left() >= 4 {
		n := int(r.u16())
		r.u16() // unused
		fr := r.sub(min(n, r.left()) &^ 3)
		for fr.left() >= 4 {
			d.Feedback = append(d.Feedback, fr.u32())
		}
	}
	if d.Status&DrawNoString != 0 {
		d.Text = nil
	}
	if d.Status&DrawNoFeedback != 0 {
		d.Feedback = nil
	}
	return d, r.err
}

// PreeditCaret is XIM_PREEDIT_CARET.
type PreeditCaret struct {
	IMID, ICID uint16
	Position   int32
	Direction  uint32
	Style      uint32
}

// ParsePreeditCaret decodes XIM_PREEDIT_CARET.
func ParsePreeditCaret(body []byte) (PreeditCaret, error) {
	r := &reader{b: body}
	c := PreeditCaret{IMID: r.u16(), ICID: r.u16(), Position: int32(r.u32()), Direction: r.u32(), Style: r.u32()}
	return c, r.err
}

// ParseResetICReply decodes XIM_RESET_IC_REPLY: the preedit string the
// input method dropped.
func ParseResetICReply(body []byte) (imid, icid uint16, text []byte, err error) {
	r := &reader{b: body}
	imid, icid = r.u16(), r.u16()
	if r.left() >= 2 {
		n := int(r.u16())
		text = r.bytes(min(n, r.left()))
	}
	return imid, icid, text, r.err
}
