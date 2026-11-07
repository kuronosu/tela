package xim

import (
	"bytes"
	"reflect"
	"testing"
)

// openReply is an XIM_OPEN_REPLY with 17 IC attributes: the test
// vector OPEN_REPLY of the xim crate (xim-rs, MIT, xim-parser/src/lib.rs),
// see THIRD_PARTY.md.
var openReply = []byte("\x1f\x00\x59\x00\x01\x00\x18\x00\x00\x00\x0a\x00\x0f\x00\x71\x75\x65\x72\x79\x49\x6e\x70\x75\x74\x53\x74\x79\x6c\x65\x00\x00\x00\x44\x01\x00\x00\x01\x00\x03\x00\x0a\x00\x69\x6e\x70\x75\x74\x53\x74\x79\x6c\x65\x02\x00\x05\x00\x0c\x00\x63\x6c\x69\x65\x6e\x74\x57\x69\x6e\x64\x6f\x77\x00\x00\x03\x00\x05\x00\x0b\x00\x66\x6f\x63\x75\x73\x57\x69\x6e\x64\x6f\x77\x00\x00\x00\x04\x00\x03\x00\x0c\x00\x66\x69\x6c\x74\x65\x72\x45\x76\x65\x6e\x74\x73\x00\x00\x05\x00\xff\x7f\x11\x00\x70\x72\x65\x65\x64\x69\x74\x41\x74\x74\x72\x69\x62\x75\x74\x65\x73\x00\x06\x00\xff\x7f\x10\x00\x73\x74\x61\x74\x75\x73\x41\x74\x74\x72\x69\x62\x75\x74\x65\x73\x00\x00\x07\x00\x0d\x00\x07\x00\x66\x6f\x6e\x74\x53\x65\x74\x00\x00\x00\x08\x00\x0b\x00\x04\x00\x61\x72\x65\x61\x00\x00\x09\x00\x0b\x00\x0a\x00\x61\x72\x65\x61\x4e\x65\x65\x64\x65\x64\x0a\x00\x03\x00\x08\x00\x63\x6f\x6c\x6f\x72\x4d\x61\x70\x00\x00\x0b\x00\x03\x00\x0b\x00\x73\x74\x64\x43\x6f\x6c\x6f\x72\x4d\x61\x70\x00\x00\x00\x0c\x00\x03\x00\x0a\x00\x66\x6f\x72\x65\x67\x72\x6f\x75\x6e\x64\x0d\x00\x03\x00\x0a\x00\x62\x61\x63\x6b\x67\x72\x6f\x75\x6e\x64\x0e\x00\x03\x00\x10\x00\x62\x61\x63\x6b\x67\x72\x6f\x75\x6e\x64\x50\x69\x78\x6d\x61\x70\x00\x00\x0f\x00\x0c\x00\x0c\x00\x73\x70\x6f\x74\x4c\x6f\x63\x61\x74\x69\x6f\x6e\x00\x00\x10\x00\x03\x00\x09\x00\x6c\x69\x6e\x65\x53\x70\x61\x63\x65\x00\x11\x00\x00\x00\x15\x00\x73\x65\x70\x61\x72\x61\x74\x6f\x72\x6f\x66\x4e\x65\x73\x74\x65\x64\x4c\x69\x73\x74\x00")

func one(t *testing.T, data []byte) Message {
	t.Helper()
	msgs, err := Split(data)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("Split(% x) = %v, %v; want one message", data, msgs, err)
	}
	return msgs[0]
}

func TestOpenReply(t *testing.T) {
	m := one(t, openReply)
	if m.Major != OpOpenReply {
		t.Fatalf("major = %d", m.Major)
	}
	o, err := ParseOpenReply(m.Body)
	if err != nil {
		t.Fatal(err)
	}
	if o.IMID != 1 || len(o.IMAttrs) != 1 || o.IMAttrs[0] != (Attr{0, TypeStyles, AttrQueryInputStyle}) {
		t.Errorf("IM part = %d %+v", o.IMID, o.IMAttrs)
	}
	names := map[string]Attr{}
	for _, a := range o.ICAttrs {
		names[a.Name] = a
	}
	if len(o.ICAttrs) != 17 || names[AttrInputStyle] != (Attr{1, TypeCard32, AttrInputStyle}) ||
		names[AttrSpotLocation] != (Attr{15, TypePoint, AttrSpotLocation}) ||
		names[AttrPreeditAttributes] != (Attr{5, TypeNested, AttrPreeditAttributes}) ||
		names["separatorofNestedList"].ID != 17 {
		t.Errorf("IC attributes = %+v", o.ICAttrs)
	}
	if got := EncodeOpenReply(o); !bytes.Equal(got, openReply) {
		t.Errorf("EncodeOpenReply differs:\n got % x\nwant % x", got, openReply)
	}
}

func TestEncodeVectors(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want []byte
	}{
		{"connect", EncodeConnect(), []byte{1, 0, 2, 0, 0x6c, 0, 1, 0, 0, 0, 0, 0}},
		{"connect reply", EncodeConnectReply(1, 0), []byte("\x02\x00\x01\x00\x01\x00\x00\x00")},
		// xim-rs read_open (without its ClientMessage padding).
		{"open", EncodeOpen("en_US"), []byte{30, 0, 2, 0, 5, 'e', 'n', '_', 'U', 'S', 0, 0}},
		// xim-rs read_query.
		{"query extension", EncodeQueryExtension(0, []string{"XIM_EXT_MOVE"}),
			[]byte{40, 0, 5, 0, 0, 0, 13, 0, 12, 'X', 'I', 'M', '_', 'E', 'X', 'T', '_', 'M', 'O', 'V', 'E', 0, 0, 0}},
		// xim-rs set_event_mask.
		{"set event mask", EncodeSetEventMask(SetEventMask{IMID: 2, ICID: 1, Forward: 3, Sync: 0xfffffffc}),
			[]byte{37, 0, 3, 0, 2, 0, 1, 0, 3, 0, 0, 0, 252, 255, 255, 255}},
		// xim-rs write_create_ic.
		{"create ic", EncodeCreateIC(2, nil), []byte("\x32\x00\x01\x00\x02\x00\x00\x00")},
		{"encoding negotiation", EncodeEncodingNegotiation(3, []string{"UTF8_STRING", "COMPOUND_TEXT"}),
			append(append([]byte{38, 0, 9, 0, 3, 0, 26, 0, 11}, "UTF8_STRING\x0dCOMPOUND_TEXT"...), 0, 0, 0, 0, 0, 0)},
		{"get im values", EncodeGetIMValues(3, []uint16{0}), []byte{44, 0, 2, 0, 3, 0, 2, 0, 0, 0, 0, 0}},
		{"focus", EncodeIDs(OpSetICFocus, 3, 4), []byte{58, 0, 1, 0, 3, 0, 4, 0}},
		{"close", EncodeIM(OpClose, 3), []byte{32, 0, 1, 0, 3, 0, 0, 0}},
		{"disconnect", EncodeEmpty(OpDisconnect), []byte{3, 0, 0, 0}},
		{"preedit start reply", EncodePreeditStartReply(3, 4, -1), []byte{74, 0, 2, 0, 3, 0, 4, 0, 255, 255, 255, 255}},
	} {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("%s:\n got % x\nwant % x", c.name, c.got, c.want)
		}
	}
}

func TestICAttributes(t *testing.T) {
	attrs := []Attribute{
		{1, Card32(PreeditCallbacks | StatusNothing)},
		{2, Card32(0x1234)},
		{5, Nested(Attribute{15, Point(10, -2)})},
	}
	msg := EncodeCreateIC(7, attrs)
	m := one(t, msg)
	imid, got, err := ParseCreateIC(m.Body)
	if err != nil || imid != 7 || len(got) != 3 {
		t.Fatalf("ParseCreateIC = %d %v %v", imid, got, err)
	}
	if v, _ := ParseCard32(got[0].Value); v != 0x402 {
		t.Errorf("style = %#x", v)
	}
	nested := ParseAttributes(got[2].Value)
	if len(nested) != 1 || nested[0].ID != 15 {
		t.Fatalf("nested = %v", nested)
	}
	if x, y, _ := ParsePoint(nested[0].Value); x != 10 || y != -2 {
		t.Errorf("spot = %d,%d", x, y)
	}
	// The spot attribute of xim-rs spot_attr.
	if a := ParseAttributes([]byte{4, 0, 4, 0, 0, 0, 0, 0}); len(a) != 1 || a[0].ID != 4 || !bytes.Equal(a[0].Value, []byte{0, 0, 0, 0}) {
		t.Errorf("spot attr = %v", a)
	}

	set := one(t, EncodeSetICValues(7, 9, attrs[2:]))
	imid, icid, got, err := ParseSetICValues(set.Body)
	if err != nil || imid != 7 || icid != 9 || len(got) != 1 || got[0].ID != 5 {
		t.Errorf("ParseSetICValues = %d %d %v %v", imid, icid, got, err)
	}
}

func TestParseServerMessages(t *testing.T) {
	// xim-rs read_error.
	e, err := ParseError(one(t, []byte{20, 0, 7, 0, 2, 0, 1, 0, 3, 0, 2, 0, 16, 0, 0, 0,
		'i', 'n', 'v', 'a', 'l', 'i', 'd', ' ', 'i', 'm', ' ', 's', 't', 'y', 'l', 'e'}).Body)
	if err != nil || e != (Error{IMID: 2, ICID: 1, Flag: 3, Code: ErrBadStyle, Detail: "invalid im style"}) {
		t.Errorf("ParseError = %+v, %v", e, err)
	}
	if got := EncodeError(e); !bytes.Equal(got[4:], one(t, got).Body) || len(got) != 32 {
		t.Errorf("EncodeError = % x", got)
	}

	styles := EncodeStyles([]uint32{PreeditPosition | StatusArea, PreeditCallbacks | StatusNothing})
	if !bytes.Equal(styles, []byte{2, 0, 0, 0, 4, 1, 0, 0, 2, 4, 0, 0}) {
		t.Errorf("EncodeStyles = % x", styles)
	}
	imid, attrs, err := ParseGetIMValuesReply(one(t, EncodeGetIMValuesReply(3, []Attribute{{0, styles}})).Body)
	if err != nil || imid != 3 || len(attrs) != 1 {
		t.Fatalf("ParseGetIMValuesReply = %d %v %v", imid, attrs, err)
	}
	if s, _ := ParseStyles(attrs[0].Value); !reflect.DeepEqual(s, []uint32{0x104, 0x402}) {
		t.Errorf("styles = %#x", s)
	}

	if imid, cat, idx, err := ParseEncodingNegotiationReply(one(t, EncodeEncodingNegotiationReply(3, 0, -1)).Body); err != nil || imid != 3 || cat != 0 || idx != -1 {
		t.Errorf("encoding reply = %d %d %d %v", imid, cat, idx, err)
	}

	var ev [32]byte
	ev[0], ev[1] = 2, 38
	fw := one(t, EncodeForwardEvent(3, 4, FlagSynchronous, 0, ev[:]))
	if fw.Major != OpForwardEvent || len(fw.Body) != 40 {
		t.Fatalf("forward = %v", fw)
	}
	if f, err := ParseForwardEvent(fw.Body); err != nil || f.IMID != 3 || f.ICID != 4 || f.Flag != 1 || f.Event != ev {
		t.Errorf("ParseForwardEvent = %+v %v", f, err)
	}

	for _, c := range []Commit{
		{IMID: 3, ICID: 4, Flag: CommitChars | FlagSynchronous, Text: []byte("日本語")},
		{IMID: 3, ICID: 4, Flag: CommitKeySym, KeySym: 0xe9},
		{IMID: 3, ICID: 4, Flag: CommitChars | CommitKeySym, KeySym: 0x61, Text: []byte("a")},
	} {
		got, err := ParseCommit(one(t, EncodeCommit(c)).Body)
		if err != nil || !reflect.DeepEqual(got, c) {
			t.Errorf("commit round trip = %+v %v, want %+v", got, err, c)
		}
	}

	d := PreeditDraw{IMID: 3, ICID: 4, Caret: 2, ChgFirst: 0, ChgLength: 1, Text: []byte("にほ"), Feedback: []uint32{FeedbackUnderline, FeedbackReverse}}
	if got, err := ParsePreeditDraw(one(t, EncodePreeditDraw(d)).Body); err != nil || !reflect.DeepEqual(got, d) {
		t.Errorf("draw round trip = %+v %v", got, err)
	}
	// fcitx5's end of a preedit: no string, no feedback.
	end := PreeditDraw{IMID: 3, ICID: 4, ChgLength: 2, Status: DrawNoString}
	if got, err := ParsePreeditDraw(one(t, EncodePreeditDraw(end)).Body); err != nil || got.Text != nil || got.ChgLength != 2 {
		t.Errorf("empty draw = %+v %v", got, err)
	}
	// A truncated draw (no string) still parses its numbers.
	if got, err := ParsePreeditDraw([]byte{3, 0, 4, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0}); err != nil || got.Caret != 1 || got.Text != nil {
		t.Errorf("short draw = %+v %v", got, err)
	}

	c := PreeditCaret{IMID: 3, ICID: 4, Position: 5, Direction: CaretAbsolutePosition, Style: CaretPrimary}
	if got, err := ParsePreeditCaret(one(t, EncodePreeditCaret(c)).Body); err != nil || got != c {
		t.Errorf("caret = %+v %v", got, err)
	}
	if imid, icid, text, err := ParseResetICReply(one(t, EncodeResetICReply(3, 4, []byte("abc"))).Body); err != nil || imid != 3 || icid != 4 || string(text) != "abc" {
		t.Errorf("reset reply = %d %d %q %v", imid, icid, text, err)
	}
	if m, err := ParseSetEventMask(one(t, EncodeSetEventMask(SetEventMask{3, 0, 3, 0})).Body); err != nil || m.Forward != 3 {
		t.Errorf("event mask = %+v %v", m, err)
	}
	if _, _, err := ParseIDs([]byte{1}); err != ErrShort {
		t.Errorf("short IDs: %v", err)
	}
}

func TestParseClientMessages(t *testing.T) {
	if e, maj, min, err := ParseConnect(one(t, EncodeConnect()).Body); err != nil || e != EndianLittle || maj != 1 || min != 0 {
		t.Errorf("ParseConnect = %x %d %d %v", e, maj, min, err)
	}
	if l, err := ParseOpen(one(t, EncodeOpen("ja_JP.UTF-8")).Body); err != nil || l != "ja_JP.UTF-8" {
		t.Errorf("ParseOpen = %q %v", l, err)
	}
	if imid, names, err := ParseNameList(one(t, EncodeEncodingNegotiation(3, []string{"UTF8_STRING", "COMPOUND_TEXT"})).Body); err != nil || imid != 3 ||
		!reflect.DeepEqual(names, []string{"UTF8_STRING", "COMPOUND_TEXT"}) {
		t.Errorf("ParseNameList = %d %q %v", imid, names, err)
	}
	if imid, ids, err := ParseGetIMValues(one(t, EncodeGetIMValues(3, []uint16{0, 9})).Body); err != nil || imid != 3 || !reflect.DeepEqual(ids, []uint16{0, 9}) {
		t.Errorf("ParseGetIMValues = %d %v %v", imid, ids, err)
	}
}

func TestSplitAndChunks(t *testing.T) {
	a, b := EncodeIDs(OpSync, 1, 2), EncodeCommit(Commit{IMID: 1, ICID: 2, Flag: CommitChars, Text: []byte("hello world, a long commit")})
	data := append(append(append([]byte{}, a...), b...), 0, 0, 0, 0, 0, 0)
	msgs, err := Split(data)
	if err != nil || len(msgs) != 2 || msgs[0].Major != OpSync || msgs[1].Major != OpCommit {
		t.Fatalf("Split = %v %v", msgs, err)
	}
	if _, err := Split(b[:len(b)-4]); err != ErrTruncated {
		t.Errorf("truncated: %v", err)
	}
	chunks := Chunks(b)
	var joined []byte
	for _, c := range chunks {
		joined = append(joined, c[:]...)
	}
	if len(chunks) != (len(b)+19)/20 || !bytes.HasPrefix(joined, b) {
		t.Errorf("Chunks: %d chunks for %d bytes", len(chunks), len(b))
	}
	if got := Chunks(nil); len(got) != 1 {
		t.Errorf("Chunks(nil) = %d chunks", len(got))
	}
}

func TestEnvironment(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "@im=fcitx": "fcitx", "@im=ibus": "ibus", "@im=none": "", "@im=": "",
		"@foo=bar@im=kinput2": "kinput2", "im=x": "x",
	} {
		if got := IMName(in); got != want {
			t.Errorf("IMName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]bool{
		"@transport=X/": true, "@transport=tcp/host:1,X/": true, "@transport=local/": false,
		"X/": false, "@transport=X/\x00": true,
	} {
		if got := HasXTransport(in); got != want {
			t.Errorf("HasXTransport(%q) = %v", in, got)
		}
	}
	if got := ParseLocales("@locale=en_US,ja_JP.UTF-8, C\x00"); !reflect.DeepEqual(got, []string{"en_US", "ja_JP.UTF-8", "C"}) {
		t.Errorf("ParseLocales = %q", got)
	}
	server := []string{"C", "en_US", "ja_JP", "ja_JP.UTF-8", "zh_CN"}
	for env, want := range map[string]string{
		"ja_JP.UTF-8":     "ja_JP.UTF-8",
		"ja_JP.utf-8@foo": "ja_JP.UTF-8",
		"ja_JP.eucJP":     "ja_JP",
		"zh_CN.UTF-8":     "zh_CN",
		"de_DE.UTF-8":     "en_US",
		"C":               "en_US",
		"":                "en_US",
	} {
		if got := ChooseLocale(server, env); got != want {
			t.Errorf("ChooseLocale(%q) = %q, want %q", env, got, want)
		}
	}
	if got := ChooseLocale([]string{"ko_KR"}, "de_DE"); got != "ko_KR" {
		t.Errorf("no match = %q", got)
	}
	if got := ChooseLocale(nil, "de_DE"); got != "C" {
		t.Errorf("empty list = %q", got)
	}
}
