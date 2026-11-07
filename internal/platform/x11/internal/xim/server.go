package xim

// The server side of the messages the client exchanges: encoders of what
// an input method sends and decoders of what a client sends. The backend
// does not need them; the fake input method of the x11 package tests and
// this package's round-trip tests do.

// EncodeConnectReply is XIM_CONNECT_REPLY.
func EncodeConnectReply(major, minor uint16) []byte { return EncodeIDs(OpConnectReply, major, minor) }

// appendAttrs writes XIMATTR entries.
func appendAttrs(w *writer, attrs []Attr) {
	for _, a := range attrs {
		w.u16(a.ID)
		w.u16(a.Type)
		w.u16(uint16(len(a.Name)))
		w.raw([]byte(a.Name))
		w.pad4()
	}
}

func attrsLen(attrs []Attr) int {
	n := 0
	for _, a := range attrs {
		n += 4 + (2+len(a.Name)+3)&^3
	}
	return n
}

// EncodeOpenReply is XIM_OPEN_REPLY.
func EncodeOpenReply(o OpenReply) []byte {
	w := &writer{}
	w.u16(o.IMID)
	w.u16(uint16(attrsLen(o.IMAttrs)))
	appendAttrs(w, o.IMAttrs)
	w.u16(uint16(attrsLen(o.ICAttrs)))
	w.u16(0)
	appendAttrs(w, o.ICAttrs)
	return message(OpOpenReply, w)
}

// EncodeEncodingNegotiationReply is XIM_ENCODING_NEGOTIATION_REPLY.
func EncodeEncodingNegotiationReply(imid, category uint16, index int16) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(category)
	w.u16(uint16(index))
	w.u16(0)
	return message(OpEncodingNegotiationReply, w)
}

// EncodeStyles encodes an XIMStyles value.
func EncodeStyles(styles []uint32) []byte {
	w := &writer{}
	w.u16(uint16(len(styles)))
	w.u16(0)
	for _, s := range styles {
		w.u32(s)
	}
	return w.b
}

// EncodeGetIMValuesReply is XIM_GET_IM_VALUES_REPLY.
func EncodeGetIMValuesReply(imid uint16, attrs []Attribute) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(uint16(attributesLen(attrs)))
	appendAttributes(w, attrs)
	return message(OpGetIMValuesReply, w)
}

// EncodeSetEventMask is XIM_SET_EVENT_MASK.
func EncodeSetEventMask(m SetEventMask) []byte {
	w := &writer{}
	w.u16(m.IMID)
	w.u16(m.ICID)
	w.u32(m.Forward)
	w.u32(m.Sync)
	return message(OpSetEventMask, w)
}

// EncodeError is XIM_ERROR.
func EncodeError(e Error) []byte {
	w := &writer{}
	w.u16(e.IMID)
	w.u16(e.ICID)
	w.u16(e.Flag)
	w.u16(e.Code)
	w.u16(uint16(len(e.Detail)))
	w.u16(0)
	w.raw([]byte(e.Detail))
	return message(OpError, w)
}

// EncodeCommit is XIM_COMMIT; its Flag says which of KeySym and Text are
// sent.
func EncodeCommit(c Commit) []byte {
	w := &writer{}
	w.u16(c.IMID)
	w.u16(c.ICID)
	w.u16(c.Flag)
	if c.Flag&CommitKeySym != 0 {
		w.u16(0)
		w.u32(c.KeySym)
	}
	if c.Flag&CommitChars != 0 {
		w.u16(uint16(len(c.Text)))
		w.raw(c.Text)
	}
	return message(OpCommit, w)
}

// EncodePreeditDraw is XIM_PREEDIT_DRAW.
func EncodePreeditDraw(d PreeditDraw) []byte {
	w := &writer{}
	w.u16(d.IMID)
	w.u16(d.ICID)
	w.u32(uint32(d.Caret))
	w.u32(uint32(d.ChgFirst))
	w.u32(uint32(d.ChgLength))
	w.u32(d.Status)
	w.u16(uint16(len(d.Text)))
	w.raw(d.Text)
	w.pad4()
	w.u16(uint16(4 * len(d.Feedback)))
	w.u16(0)
	for _, f := range d.Feedback {
		w.u32(f)
	}
	return message(OpPreeditDraw, w)
}

// EncodePreeditCaret is XIM_PREEDIT_CARET.
func EncodePreeditCaret(c PreeditCaret) []byte {
	w := &writer{}
	w.u16(c.IMID)
	w.u16(c.ICID)
	w.u32(uint32(c.Position))
	w.u32(c.Direction)
	w.u32(c.Style)
	return message(OpPreeditCaret, w)
}

// EncodeResetICReply is XIM_RESET_IC_REPLY.
func EncodeResetICReply(imid, icid uint16, text []byte) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(icid)
	w.u16(uint16(len(text)))
	w.raw(text)
	return message(OpResetICReply, w)
}

// EncodeRegisterTriggerKeys is XIM_REGISTER_TRIGGERKEYS with one on-key
// (keysym, modifier, modifier mask) and no off-keys.
func EncodeRegisterTriggerKeys(imid uint16, keysym, mods, mask uint32) []byte {
	w := &writer{}
	w.u16(imid)
	w.u16(0)
	w.u32(12)
	w.u32(keysym)
	w.u32(mods)
	w.u32(mask)
	w.u32(0)
	return message(OpRegisterTriggerKeys, w)
}

// --- what a client sends ---

// readSTRList reads a CARD16 byte length and a list of STR.
func readSTRList(r *reader) []string {
	n := int(r.u16())
	lr := r.sub(n)
	var out []string
	for lr.left() > 0 && lr.err == nil {
		l := int(lr.u8())
		s := lr.bytes(l)
		if lr.err == nil {
			out = append(out, string(s))
		}
	}
	return out
}

// ParseOpen decodes XIM_OPEN: the locale name.
func ParseOpen(body []byte) (string, error) {
	r := &reader{b: body}
	n := int(r.u8())
	return string(r.bytes(n)), r.err
}

// ParseConnect decodes XIM_CONNECT: the byte order and protocol version.
func ParseConnect(body []byte) (endian byte, major, minor uint16, err error) {
	r := &reader{b: body}
	endian = r.u8()
	r.u8()
	major, minor = r.u16(), r.u16()
	return endian, major, minor, r.err
}

// ParseNameList decodes XIM_QUERY_EXTENSION and XIM_ENCODING_NEGOTIATION:
// the input-method ID and the names.
func ParseNameList(body []byte) (imid uint16, names []string, err error) {
	r := &reader{b: body}
	imid = r.u16()
	names = readSTRList(r)
	return imid, names, r.err
}

// ParseGetIMValues decodes XIM_GET_IM_VALUES.
func ParseGetIMValues(body []byte) (imid uint16, ids []uint16, err error) {
	r := &reader{b: body}
	imid = r.u16()
	n := int(r.u16())
	lr := r.sub(n)
	for lr.left() >= 2 {
		ids = append(ids, lr.u16())
	}
	return imid, ids, r.err
}

// ParseCreateIC decodes XIM_CREATE_IC.
func ParseCreateIC(body []byte) (imid uint16, attrs []Attribute, err error) {
	r := &reader{b: body}
	imid = r.u16()
	n := int(r.u16())
	attrs = readAttributes(r.sub(n))
	return imid, attrs, r.err
}

// ParseSetICValues decodes XIM_SET_IC_VALUES.
func ParseSetICValues(body []byte) (imid, icid uint16, attrs []Attribute, err error) {
	r := &reader{b: body}
	imid, icid = r.u16(), r.u16()
	n := int(r.u16())
	r.u16()
	attrs = readAttributes(r.sub(n))
	return imid, icid, attrs, r.err
}

// ParseAttributes decodes a nested list value.
func ParseAttributes(value []byte) []Attribute { return readAttributes(&reader{b: value}) }

// ParseCard32 decodes a CARD32 attribute value.
func ParseCard32(value []byte) (uint32, error) {
	r := &reader{b: value}
	return r.u32(), r.err
}

// ParsePoint decodes an XPoint attribute value.
func ParsePoint(value []byte) (x, y int16, err error) {
	r := &reader{b: value}
	x, y = int16(r.u16()), int16(r.u16())
	return x, y, r.err
}
