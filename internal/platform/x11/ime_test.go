//go:build linux

package x11

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xim"
)

// fakeIM is an XIM server for the tests: it owns "@server=fake", answers
// the TRANSPORT and LOCALES conversions, speaks the X transport with the
// client through the fake X server's SendEvent and properties, records
// the client's messages as text (got), and queues its own messages as the
// events the client reads (out), which pump delivers.
type fakeIM struct {
	t *testing.T
	b *Backend
	x *fakeX

	owner, comm xproto.Window
	sel         xproto.Atom
	out         []xgb.Event
	got         []string

	// Behavior.
	styles     []uint32 // queryInputStyle
	refuse     []uint32 // styles CREATE_IC rejects with BadStyle
	failNext   uint16   // the next CREATE_IC fails with this XIM error code
	utf8       bool     // accepts UTF8_STRING
	locales    string
	syncMode   bool              // forwards and commits with the synchronous flag (fcitx5)
	boundary   uint32            // XCONNECT: transport 0.2 with this boundary when set
	moreData   bool              // long messages in _XIM_MOREDATA parts instead of properties
	sameProp   bool              // long messages appended to one property
	imMask     *xim.SetEventMask // sent after XIM_OPEN_REPLY
	icMask     *xim.SetEventMask // sent after XIM_CREATE_IC_REPLY (ICID filled in)
	ignoreSync bool              // never answers a synchronous forward
	// onKey decides what happens to a forwarded key: true consumes it
	// (and may queue messages), false sends it back.
	onKey func(press bool, kc byte) bool

	// inFlight is sent before the reply to XIM_RESET_IC: an update the
	// server made before it saw the reset.
	inFlight []byte

	nextIC   uint16
	nextProp int
	more     []byte
	props    int // client messages received through a property
}

const (
	fakeIMOwner = 950
	fakeIMComm  = 951
)

// imeRig is a fake backend with the fake X server, the fake input method
// and window 7, with keyboard focus and the de keymap.
type imeRig struct {
	b  *Backend
	x  *fakeX
	im *fakeIM
	w  *Window
	r  *recorder
}

func newIMERig(t *testing.T, configure func(im *fakeIM)) *imeRig {
	t.Helper()
	t.Setenv("LC_ALL", "ja_JP.UTF-8")
	b, w := fakeBackend(2)
	x := newFakeX(b)
	b.kbd.fetch = func() (xkb.CoreMapping, error) { return deMapping(), nil }
	b.xkb = xkbExt{ok: true, detectable: true}
	im := &fakeIM{t: t, b: b, x: x, owner: fakeIMOwner, comm: fakeIMComm, utf8: true,
		styles:  []uint32{xim.PreeditPosition | xim.StatusArea, xim.PreeditCallbacks | xim.StatusNothing, xim.PreeditNothing | xim.StatusNothing},
		locales: "@locale=C,en_US,ja_JP.UTF-8"}
	im.sel = x.id(xim.ServerSelection("fake"))
	if configure != nil {
		configure(im)
	}
	im.start()
	x.sendHook = im.sendHook
	x.convertHook = im.convertHook
	g := &imeRig{b: b, x: x, im: im, w: w, r: &recorder{}}
	g.pump()
	g.feed(focusIn(xproto.NotifyModeNormal), keymapNotify())
	g.r.take()
	return g
}

// start registers the server as an input method does when it starts.
func (im *fakeIM) start() {
	im.x.owners[im.sel] = im.owner
	im.x.setProp(im.b.root, im.b.atoms.ximServers, xproto.AtomAtom, 32, atomList(im.sel))
}

// connect starts the client as New does.
func (g *imeRig) connect() string {
	g.b.initIME("@im=fake")
	return g.pump()
}

// pump delivers the server's messages until it has nothing more to say,
// with a dispatch after each round; it returns what the app received.
func (g *imeRig) pump() string {
	var out []string
	for range 50 {
		for len(g.im.out) > 0 {
			ev := g.im.out[0]
			g.im.out = g.im.out[1:]
			g.b.translate(xevent{ev: ev})
		}
		g.b.kbd.flushAll()
		g.b.dispatch(g.r)
		if s := g.r.take(); s != "" {
			out = append(out, s)
		}
		if len(g.im.out) == 0 {
			break
		}
	}
	return strings.Join(out, "; ")
}

// feed translates X events as one read, then pumps.
func (g *imeRig) feed(evs ...xgb.Event) string {
	for _, e := range evs {
		g.b.translate(xevent{ev: e})
	}
	g.b.kbd.flushAll()
	g.b.dispatch(g.r)
	first := g.r.take()
	rest := g.pump()
	return strings.Trim(first+"; "+rest, "; ")
}

// took returns the client's messages since the last call.
func (im *fakeIM) took() string {
	s := strings.Join(im.got, "; ")
	im.got = nil
	return s
}

func (im *fakeIM) log(format string, args ...any) {
	im.got = append(im.got, fmt.Sprintf(format, args...))
}

// --- the server side of the X transport ---

func (im *fakeIM) convertHook(req xproto.Window, sel, target, prop xproto.Atom) bool {
	if sel != im.sel || im.x.owners[sel] != im.owner {
		return false
	}
	a := &im.b.atoms
	var v string
	switch target {
	case a.ximTransport:
		v = "@transport=X/"
	case a.ximLocales:
		v = im.locales
	default:
		return false
	}
	im.x.setProp(req, prop, target, 8, []byte(v))
	im.out = append(im.out, xproto.SelectionNotifyEvent{Requestor: req, Selection: sel, Target: target, Property: prop})
	return true
}

func (im *fakeIM) sendHook(dest xproto.Window, raw []byte) bool {
	if raw[0] != xproto.ClientMessage || dest != im.owner && dest != im.comm {
		return false
	}
	ev := xproto.ClientMessageEventNew(raw).(xproto.ClientMessageEvent)
	a := &im.b.atoms
	switch {
	case dest == im.owner && ev.Type == a.ximXConnect:
		im.log("xconnect from %d", ev.Data.Data32[0])
		minor, boundary := uint32(0), uint32(0)
		if im.boundary != 0 {
			minor, boundary = 2, im.boundary
		}
		im.out = append(im.out, clientMsg(fakeIMWin, a.ximXConnect, uint32(im.comm), 0, minor, boundary))
	case dest == im.comm && ev.Type == a.ximMoreData:
		im.more = append(im.more, ev.Data.Data8...)
	case dest == im.comm && ev.Type == a.ximProtocol:
		var data []byte
		if ev.Format == 8 {
			data = append(im.more, ev.Data.Data8...)
			im.more = nil
		} else {
			atom := xproto.Atom(ev.Data.Data32[1])
			p := im.x.props[im.comm][atom]
			n := int(ev.Data.Data32[0])
			if len(p.data) < n {
				im.t.Errorf("client property %s has %d bytes, announced %d", im.x.name(atom), len(p.data), n)
				return true
			}
			data = p.data[:n]
			im.x.setProp(im.comm, atom, p.typ, 8, p.data[n:])
			if len(p.data) == n {
				delete(im.x.props[im.comm], atom)
			}
			im.props++
		}
		msgs, err := xim.Split(data)
		if err != nil {
			im.t.Errorf("client sent a truncated message: % x", data)
		}
		for _, m := range msgs {
			im.handle(m)
		}
	default:
		return false
	}
	return true
}

// send queues a message to the client: one ClientMessage when it fits, else
// _XIM_MOREDATA parts or a property.
func (im *fakeIM) send(msg []byte) {
	a := &im.b.atoms
	switch {
	case len(msg) <= xim.CMDataSize || im.moreData:
		chunks := xim.Chunks(msg)
		for i, c := range chunks {
			typ := a.ximMoreData
			if i == len(chunks)-1 {
				typ = a.ximProtocol
			}
			im.out = append(im.out, xproto.ClientMessageEvent{Format: 8, Window: fakeIMWin, Type: typ, Data: xproto.ClientMessageDataUnionData8New(c[:])})
		}
	default:
		name := fmt.Sprintf("_server%d", im.nextProp)
		if !im.sameProp {
			im.nextProp++
		}
		atom := im.x.id(name)
		old := im.x.props[fakeIMWin][atom]
		im.x.setProp(fakeIMWin, atom, xproto.AtomString, 8, append(append([]byte{}, old.data...), msg...))
		im.out = append(im.out, clientMsg(fakeIMWin, a.ximProtocol, uint32(len(msg)), uint32(atom)))
	}
}

// --- the input method ---

var fakeICAttrs = []xim.Attr{
	{ID: 1, Type: xim.TypeCard32, Name: xim.AttrInputStyle},
	{ID: 2, Type: xim.TypeWindow, Name: xim.AttrClientWindow},
	{ID: 3, Type: xim.TypeWindow, Name: xim.AttrFocusWindow},
	{ID: 4, Type: xim.TypeCard32, Name: xim.AttrFilterEvents},
	{ID: 5, Type: xim.TypeNested, Name: xim.AttrPreeditAttributes},
	{ID: 6, Type: xim.TypeNested, Name: xim.AttrStatusAttributes},
	{ID: 15, Type: xim.TypePoint, Name: xim.AttrSpotLocation},
}

func (im *fakeIM) handle(m xim.Message) {
	const imid = 1
	switch m.Major {
	case xim.OpConnect:
		im.log("CONNECT")
		im.send(xim.EncodeConnectReply(1, 0))
	case xim.OpOpen:
		l, _ := xim.ParseOpen(m.Body)
		im.log("OPEN %s", l)
		im.send(xim.EncodeOpenReply(xim.OpenReply{IMID: imid,
			IMAttrs: []xim.Attr{{ID: 0, Type: xim.TypeStyles, Name: xim.AttrQueryInputStyle}}, ICAttrs: fakeICAttrs}))
		if im.imMask != nil {
			im.send(xim.EncodeSetEventMask(*im.imMask))
		}
	case xim.OpQueryExtension:
		im.log("QUERY_EXTENSION")
		im.send(xim.EncodeIDs(xim.OpQueryExtensionReply, imid, 0))
	case xim.OpEncodingNegotiation:
		_, names, _ := xim.ParseNameList(m.Body)
		im.log("ENCODING %v", names)
		idx := int16(slices.Index(names, "COMPOUND_TEXT"))
		if im.utf8 {
			idx = int16(slices.Index(names, "UTF8_STRING"))
		}
		im.send(xim.EncodeEncodingNegotiationReply(imid, 0, idx))
	case xim.OpGetIMValues:
		im.log("GET_IM_VALUES")
		im.send(xim.EncodeGetIMValuesReply(imid, []xim.Attribute{{ID: 0, Value: xim.EncodeStyles(im.styles)}}))
	case xim.OpCreateIC:
		_, attrs, _ := xim.ParseCreateIC(m.Body)
		desc, style := describeAttrs(attrs)
		if im.failNext != 0 {
			im.log("CREATE_IC%s: error %d", desc, im.failNext)
			im.send(xim.EncodeError(xim.Error{IMID: imid, Flag: xim.ErrIMIDValid, Code: im.failNext, Detail: "failed"}))
			im.failNext = 0
			return
		}
		if slices.Contains(im.refuse, style) || !slices.Contains(im.styles, style) {
			im.log("CREATE_IC%s: refused", desc)
			im.send(xim.EncodeError(xim.Error{IMID: imid, Flag: xim.ErrIMIDValid, Code: xim.ErrBadStyle, Detail: "bad style"}))
			return
		}
		im.nextIC++
		im.log("CREATE_IC%s -> %d", desc, im.nextIC)
		im.send(xim.EncodeIDs(xim.OpCreateICReply, imid, im.nextIC))
		if im.icMask != nil {
			m := *im.icMask
			m.ICID = im.nextIC
			im.send(xim.EncodeSetEventMask(m))
		}
	case xim.OpSetICValues:
		_, icid, attrs, _ := xim.ParseSetICValues(m.Body)
		desc, _ := describeAttrs(attrs)
		im.log("SET_IC_VALUES %d%s", icid, desc)
		im.send(xim.EncodeIDs(xim.OpSetICValuesReply, imid, icid))
	case xim.OpSetICFocus, xim.OpUnsetICFocus, xim.OpSyncReply, xim.OpDestroyIC, xim.OpResetIC, xim.OpSync:
		_, icid, _ := xim.ParseIDs(m.Body)
		name := map[byte]string{xim.OpSetICFocus: "FOCUS", xim.OpUnsetICFocus: "UNFOCUS", xim.OpSyncReply: "SYNC_REPLY",
			xim.OpDestroyIC: "DESTROY_IC", xim.OpResetIC: "RESET_IC", xim.OpSync: "SYNC"}[m.Major]
		im.log("%s %d", name, icid)
		switch m.Major {
		case xim.OpDestroyIC:
			im.send(xim.EncodeIDs(xim.OpDestroyICReply, imid, icid))
		case xim.OpResetIC:
			if im.inFlight != nil {
				im.send(im.inFlight)
			}
			im.send(xim.EncodeResetICReply(imid, icid, []byte("dropped")))
		case xim.OpSync:
			im.send(xim.EncodeIDs(xim.OpSyncReply, imid, icid))
		}
	case xim.OpForwardEvent:
		f, _ := xim.ParseForwardEvent(m.Body)
		press := f.Event[0] == xproto.KeyPress
		kc := f.Event[1]
		kind := "release"
		if press {
			kind = "press"
		}
		sync := ""
		if f.Flag&xim.FlagSynchronous != 0 {
			sync = " sync"
		}
		im.log("KEY %d %s %d%s", f.ICID, kind, kc, sync)
		if im.onKey == nil || !im.onKey(press, kc) {
			var flag uint16
			if im.syncMode {
				flag = xim.FlagSynchronous
			}
			im.send(xim.EncodeForwardEvent(imid, f.ICID, flag, 0, f.Event[:]))
		}
		if f.Flag&xim.FlagSynchronous != 0 && !im.ignoreSync {
			im.send(xim.EncodeIDs(xim.OpSyncReply, imid, f.ICID))
		}
	case xim.OpPreeditStartReply:
		im.log("START_REPLY %d", int32(le.Uint32(m.Body[4:])))
	case xim.OpPreeditCaretReply:
		im.log("CARET_REPLY %d", int32(le.Uint32(m.Body[4:])))
	case xim.OpClose:
		im.log("CLOSE")
	case xim.OpDisconnect:
		im.log("DISCONNECT")
	default:
		im.log("op %d", m.Major)
	}
}

// describeAttrs describes IC attributes by the fake's IDs.
func describeAttrs(attrs []xim.Attribute) (desc string, style uint32) {
	for _, a := range attrs {
		switch a.ID {
		case 1:
			style, _ = xim.ParseCard32(a.Value)
			desc += fmt.Sprintf(" style=%#x", style)
		case 2, 3:
			v, _ := xim.ParseCard32(a.Value)
			desc += fmt.Sprintf(" %s=%d", map[uint16]string{2: "client", 3: "focus"}[a.ID], v)
		case 5:
			for _, n := range xim.ParseAttributes(a.Value) {
				if n.ID == 15 {
					x, y, _ := xim.ParsePoint(n.Value)
					desc += fmt.Sprintf(" spot=%d,%d", x, y)
				}
			}
		}
	}
	return desc, style
}

// Messages the input method sends on its own.

func (im *fakeIM) commit(icid uint16, text string) {
	flag := uint16(xim.CommitChars)
	if im.syncMode {
		flag |= xim.FlagSynchronous
	}
	if !im.utf8 {
		text = "\x1b%G" + text + "\x1b%@" // UTF-8 inside COMPOUND_TEXT
	}
	im.send(xim.EncodeCommit(xim.Commit{IMID: 1, ICID: icid, Flag: flag, Text: []byte(text)}))
}

func (im *fakeIM) start1(icid uint16) { im.send(xim.EncodeIDs(xim.OpPreeditStart, 1, icid)) }
func (im *fakeIM) done(icid uint16)   { im.send(xim.EncodeIDs(xim.OpPreeditDone, 1, icid)) }

func (im *fakeIM) draw(icid uint16, caret, first, n int32, text string, fb ...uint32) {
	d := xim.PreeditDraw{IMID: 1, ICID: icid, Caret: caret, ChgFirst: first, ChgLength: n, Text: []byte(text), Feedback: fb}
	if !im.utf8 && text != "" {
		d.Text = []byte("\x1b%G" + text + "\x1b%@")
	}
	if text == "" {
		d.Status = xim.DrawNoString | xim.DrawNoFeedback
	}
	im.send(xim.EncodePreeditDraw(d))
}

// --- tests ---

// textState is a text field with its caret at (10, 20), 16 px high
// (logical; the rig's scale is 2).
func textState(p platform.TextPurpose) *platform.TextInputState {
	return &platform.TextInputState{CursorArea: platform.Rect{X: 10, Y: 20, H: 16}, Purpose: p}
}

// focusField connects, gives window 7 a text field and returns what the
// app received.
func (g *imeRig) focusField(t *testing.T) {
	t.Helper()
	g.connect()
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	if got := g.pump(); got != "session true" {
		t.Fatalf("focusing a field: app got %q, want the session", got)
	}
	g.im.took()
}

func TestIMEHandshake(t *testing.T) {
	g := newIMERig(t, nil)
	g.connect()
	if got, want := g.im.took(), lines(
		"xconnect from 902", "CONNECT", "OPEN ja_JP.UTF-8", "QUERY_EXTENSION",
		"ENCODING [UTF8_STRING UTF-8 COMPOUND_TEXT]", "GET_IM_VALUES"); got != want {
		t.Fatalf("handshake:\n got %s\nwant %s", got, want)
	}
	ime := &g.b.ime
	if ime.state != imeReady || !ime.utf8 || ime.style != xim.PreeditCallbacks|xim.StatusNothing || ime.err != nil {
		t.Fatalf("after the handshake: state %d utf8 %v style %#x err %v", ime.state, ime.utf8, ime.style, ime.err)
	}
	if ime.styles[1] != xim.PreeditNothing|xim.StatusNothing || len(ime.styles) != 2 {
		t.Errorf("usable styles = %#x (PreeditPosition|StatusArea is not usable)", ime.styles)
	}

	// The context is created with the first text input state, and focused
	// since the window has the keyboard focus. Spot: (10, 20+16) x 2.
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	if got := g.pump(); got != "session true" {
		t.Errorf("app got %q", got)
	}
	if got, want := g.im.took(), lines("CREATE_IC style=0x402 client=7 focus=7 spot=20,72 -> 1", "FOCUS 1"); got != want {
		t.Errorf("first state:\n got %s\nwant %s", got, want)
	}

	// The cursor area moves: one SET_IC_VALUES per change.
	st := textState(platform.TextPurposeNormal)
	st.CursorArea.X = 30
	g.w.SetTextInput(st)
	g.w.SetTextInput(st)
	g.pump()
	if got, want := g.im.took(), "SET_IC_VALUES 1 spot=60,72"; got != want {
		t.Errorf("moved caret: got %s, want %s", got, want)
	}

	// Keyboard focus: the session follows it.
	if got := g.feed(focusOut(xproto.NotifyModeNormal)); got != "focus false; session false" {
		t.Errorf("focus out: app got %q", got)
	}
	if got := g.feed(focusIn(xproto.NotifyModeNormal), keymapNotify()); got != "focus true; session true" {
		t.Errorf("focus in: app got %q", got)
	}
	if got, want := g.im.took(), lines("UNFOCUS 1", "FOCUS 1"); got != want {
		t.Errorf("focus changes:\n got %s\nwant %s", got, want)
	}

	// Another field: nil then a state in one iteration restarts the
	// session. nil alone ends it.
	g.w.SetTextInput(nil)
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	if got := g.pump(); got != "session false; session true" {
		t.Errorf("restart: app got %q", got)
	}
	g.w.SetTextInput(nil)
	if got := g.pump(); got != "session false" {
		t.Errorf("no field: app got %q", got)
	}
	// (The new field's caret is where the first field's was before it
	// moved.)
	if got, want := g.im.took(), lines("UNFOCUS 1", "SET_IC_VALUES 1 spot=20,72", "FOCUS 1", "UNFOCUS 1"); got != want {
		t.Errorf("restart:\n got %s\nwant %s", got, want)
	}

	// Closing the window destroys its context; closing the backend says
	// goodbye.
	g.w.Close()
	g.pump()
	g.b.shutdownIME()
	if got, want := g.im.took(), lines("DESTROY_IC 1", "CLOSE", "DISCONNECT"); got != want {
		t.Errorf("close:\n got %s\nwant %s", got, want)
	}
}

func TestIMEKeysAndComposition(t *testing.T) {
	var g *imeRig
	g = newIMERig(t, func(im *fakeIM) {
		im.onKey = func(press bool, kc byte) bool {
			switch {
			case kc == kcA && press: // a romaji vowel: あ
				im.start1(1)
				im.draw(1, 1, 0, 0, "あ", xim.FeedbackUnderline)
				return true
			case kc == kcE && press: // conversion: two clauses, the second one selected
				im.draw(1, 3, 0, 1, "日本語", xim.FeedbackUnderline, xim.FeedbackReverse, xim.FeedbackReverse)
				return true
			case kc == kcEnter && press && len(g.b.ime.ics[1].text) > 0: // commit
				im.commit(1, "日本語")
				im.draw(1, 0, 0, 3, "")
				im.done(1)
				return true
			case kc == kcQ && press: // cancel
				im.draw(1, 0, 0, 3, "")
				im.done(1)
				return true
			case (kc == kcA || kc == kcE || kc == kcEnter || kc == kcQ) && !press:
				return true // releases of consumed keys
			}
			return false
		}
	})
	g.focusField(t)

	// A key the input method does not use comes back to the keyboard.
	if got := g.feed(press(kcY, 0, 1), release(kcY, 0, 2)); got != lines(`key press KeyY "z" "z" mods=0`, `key release KeyY "z" "" mods=0`) {
		t.Errorf("passed-through key: app got %s", got)
	}
	if got, want := g.im.took(), lines("KEY 1 press 29", "KEY 1 release 29"); got != want {
		t.Errorf("forwards:\n got %s\nwant %s", got, want)
	}

	// Composition: consumed keys never arrive.
	if got, want := g.feed(press(kcA, 0, 3), release(kcA, 0, 4)), `ime "" "あ" 3,3`; got != want {
		t.Errorf("preedit: app got %s, want %s", got, want)
	}
	if got, want := g.im.took(), lines("KEY 1 press 38", "KEY 1 release 38", "START_REPLY -1"); got != want {
		t.Errorf("preedit start:\n got %s\nwant %s", got, want)
	}
	// The reverse clause is the highlighted range, in bytes.
	if got, want := g.feed(press(kcE, 0, 5), release(kcE, 0, 6)), `ime "" "日本語" 3,9`; got != want {
		t.Errorf("conversion: app got %s, want %s", got, want)
	}
	// A commit and the end of the preedit in one read: one IMEEvent.
	if got, want := g.feed(press(kcEnter, 0, 7), release(kcEnter, 0, 8)), `ime "日本語" "" 0,0`; got != want {
		t.Errorf("commit: app got %s, want %s", got, want)
	}

	// Cancel: an empty preedit without a commit.
	g.feed(press(kcA, 0, 9), release(kcA, 0, 10))
	if got, want := g.feed(press(kcQ, 0, 11), release(kcQ, 0, 12)), `ime "" "" 0,0`; got != want {
		t.Errorf("cancel: app got %s, want %s", got, want)
	}

	// The caret moves: PREEDIT_CARET is answered, and XIMCaretInvisible
	// hides the cursor.
	g.feed(press(kcA, 0, 13), release(kcA, 0, 14))
	g.im.took()
	g.im.send(xim.EncodePreeditCaret(xim.PreeditCaret{IMID: 1, ICID: 1, Direction: xim.CaretLineStart, Style: xim.CaretPrimary}))
	if got := g.pump(); got != `ime "" "あ" 0,0` {
		t.Errorf("caret to the start: app got %s", got)
	}
	g.im.send(xim.EncodePreeditCaret(xim.PreeditCaret{IMID: 1, ICID: 1, Direction: xim.CaretDontChange, Style: xim.CaretInvisible}))
	if got := g.pump(); got != `ime "" "あ" -1,-1` {
		t.Errorf("hidden caret: app got %s", got)
	}
	if got, want := g.im.took(), lines("CARET_REPLY 0", "CARET_REPLY 0"); got != want {
		t.Errorf("caret replies: got %s, want %s", got, want)
	}

	// Updates that come before a key the server sends back are delivered
	// before it (the backspace that empties the preedit, then one that
	// edits the field).
	g.im.draw(1, 0, 0, 1, "")
	g.im.done(1)
	ev := press(22, 0, 15).Bytes() // BackSpace, keycode 22
	g.im.send(xim.EncodeForwardEvent(1, 1, 0, 0, ev))
	if got, want := g.pump(), lines(`ime "" "" 0,0`, `key press Backspace "Unidentified" "" mods=0`); got != want {
		t.Errorf("update before a key:\n got %s\nwant %s", got, want)
	}
}

func TestIMESync(t *testing.T) {
	g := newIMERig(t, func(im *fakeIM) {
		im.syncMode = true
		im.icMask = &xim.SetEventMask{IMID: 1, Forward: xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease, Sync: xproto.EventMaskKeyPress}
	})
	g.focusField(t)

	// Sync mode (fcitx5): the forwarded-back key and a commit are
	// answered with SYNC_REPLY. Presses are forwarded synchronously as
	// the event mask asks: the second press waits for the first one's
	// XIM_SYNC_REPLY.
	g.im.ignoreSync = true
	if got, want := g.feed(press(kcY, 0, 1), press(kcA, 0, 2)), `key press KeyY "z" "z" mods=0`; got != want {
		t.Errorf("first forward: app got %s, want %s", got, want)
	}
	if got, want := g.im.took(), lines("KEY 1 press 29 sync", "SYNC_REPLY 1"); got != want {
		t.Errorf("first forward:\n got %s\nwant %s", got, want)
	}
	g.im.ignoreSync = false
	g.im.send(xim.EncodeIDs(xim.OpSyncReply, 1, 1))
	if got, want := g.pump(), `key press KeyA "a" "a" mods=0`; got != want {
		t.Errorf("held key: app got %s, want %s", got, want)
	}
	if got, want := g.im.took(), lines("KEY 1 press 38 sync", "SYNC_REPLY 1"); got != want {
		t.Errorf("after the sync reply:\n got %s\nwant %s", got, want)
	}

	g.im.commit(1, "x")
	if got := g.pump(); got != `ime "x" "" 0,0` {
		t.Errorf("sync commit: app got %s", got)
	}
	g.im.send(xim.EncodeIDs(xim.OpSync, 1, 1))
	g.pump()
	if got, want := g.im.took(), lines("SYNC_REPLY 1", "SYNC_REPLY 1"); got != want {
		t.Errorf("sync replies:\n got %s\nwant %s", got, want)
	}

	// A server that never answers: the held key goes to the server after
	// imeSyncTimeout.
	g.im.ignoreSync = true
	g.feed(press(kcQ, 0, 3), release(kcQ, 0, 4))
	if got, want := g.im.took(), lines("KEY 1 press 24 sync", "SYNC_REPLY 1"); got != want {
		t.Errorf("unanswered:\n got %s\nwant %s", got, want)
	}
	if n := g.x.fire(imeSyncTimeout); n != 1 {
		t.Fatalf("%d sync timers fired", n)
	}
	g.pump()
	if got, want := g.im.took(), lines("KEY 1 release 24", "SYNC_REPLY 1"); got != want {
		t.Errorf("after the timeout:\n got %s\nwant %s", got, want)
	}
}

func TestIMEPasswordAndReset(t *testing.T) {
	g := newIMERig(t, func(im *fakeIM) {
		im.onKey = func(press bool, kc byte) bool {
			if kc == kcA && press {
				im.draw(1, 1, 0, 0, "あ")
			}
			return kc == kcA
		}
	})
	g.focusField(t)

	// Reset: XIM_RESET_IC; updates made before its reply are not
	// delivered, nor is the string the reply returns.
	g.feed(press(kcA, 0, 1), release(kcA, 0, 2))
	g.w.ResetTextInput()
	g.im.inFlight = xim.EncodePreeditDraw(xim.PreeditDraw{IMID: 1, ICID: 1, Caret: 2, ChgFirst: 1, Text: []byte("い")})
	got := g.pump()
	if got != "" {
		t.Errorf("reset: app got %q", got)
	}
	if got, want := g.im.took(), lines("KEY 1 press 38", "KEY 1 release 38", "RESET_IC 1"); got != want {
		t.Errorf("reset:\n got %s\nwant %s", got, want)
	}
	if got := g.feed(press(kcA, 0, 3), release(kcA, 0, 4)); got != `ime "" "あ" 3,3` {
		t.Errorf("after the reset: app got %q", got)
	}
	g.im.took()

	// A password field has no session: keys go to the keyboard (with
	// Compose), the input method sees none.
	g.w.SetTextInput(textState(platform.TextPurposePassword))
	if got := g.pump(); got != "session false" {
		t.Errorf("password field: app got %q", got)
	}
	if got, want := g.im.took(), "UNFOCUS 1"; got != want {
		t.Errorf("password field: server got %q, want %q", got, want)
	}
	if got := g.feed(press(kcA, 0, 5), release(kcA, 0, 6)); got != lines(`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`) {
		t.Errorf("password typing: app got %s", got)
	}
	if got := g.im.took(); got != "" {
		t.Errorf("password typing: server got %q", got)
	}
	g.w.SetTextInput(textState(platform.TextPurposeEmail))
	if got := g.pump(); got != "session true" {
		t.Errorf("back to a normal field: app got %q", got)
	}
}

func TestIMEDeadKeysWithAndWithoutIM(t *testing.T) {
	table, err := xkb.ParseCompose([]byte("<dead_acute> <e> : \"é\"\n"), "test", xkb.ComposeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deadE := []xgb.Event{press(kcAE12, 0, 1), release(kcAE12, 0, 2), press(kcE, 0, 3), release(kcE, 0, 4)}
	want := lines(`key press Equal "Dead" "" mods=0`, `key release Equal "Dead" "" mods=0`,
		`key press KeyE "é" "é" mods=0`, `key release KeyE "e" "" mods=0`)

	// The input method passes the keys back: Compose runs once, in the
	// keyboard.
	g := newIMERig(t, nil)
	g.b.kbd.composeLoader = func() *xkb.ComposeTable { return table }
	g.focusField(t)
	if got := g.feed(deadE...); got != want {
		t.Errorf("passed back:\n got %s\nwant %s", got, want)
	}

	// The input method composes itself: it consumes both keys and
	// commits é; the keyboard sees nothing, so nothing is inserted twice.
	g = newIMERig(t, func(im *fakeIM) {
		im.onKey = func(press bool, kc byte) bool {
			if kc == kcE && press {
				im.commit(1, "é")
			}
			return kc == kcAE12 || kc == kcE
		}
	})
	g.b.kbd.composeLoader = func() *xkb.ComposeTable { return table }
	g.focusField(t)
	if got := g.feed(deadE...); got != `ime "é" "" 0,0` {
		t.Errorf("composed by the input method: app got %s", got)
	}

	// Without an input method (XMODIFIERS names none), the keyboard
	// composes as before.
	b, _ := fakeBackend(1)
	newFakeX(b)
	b.kbd.composeLoader = func() *xkb.ComposeTable { return table }
	b.kbd.fetch = func() (xkb.CoreMapping, error) { return deMapping(), nil }
	b.xkb = xkbExt{ok: true, detectable: true}
	b.initIME("")
	if b.ime.state != imeOff || b.ime.win != 0 {
		t.Fatalf("no input method: state %d window %d", b.ime.state, b.ime.win)
	}
	r := &recorder{}
	b.dispatch(r)
	r.take()
	for _, e := range append([]xgb.Event{focusIn(xproto.NotifyModeNormal), keymapNotify()}, deadE...) {
		b.translate(xevent{ev: e})
	}
	b.kbd.flushAll()
	b.windows[7].SetTextInput(textState(platform.TextPurposeNormal))
	b.dispatch(r)
	if got := r.take(); got != "focus true; "+want {
		t.Errorf("without an input method:\n got %s\nwant focus true; %s", got, want)
	}
}

func TestIMEServerGoneAndBack(t *testing.T) {
	g := newIMERig(t, nil)
	g.focusField(t)

	// The server crashes: its window is destroyed. The session ends and
	// keys go to the keyboard.
	delete(g.x.owners, g.im.sel)
	if got := g.feed(xproto.DestroyNotifyEvent{Event: fakeIMComm, Window: fakeIMComm}); got != "session false" {
		t.Errorf("server gone: app got %q", got)
	}
	if g.b.ime.state != imeIdle || g.w.ic != nil {
		t.Fatalf("after the server left: state %d ic %v", g.b.ime.state, g.w.ic)
	}
	if got := g.feed(press(kcA, 0, 1), release(kcA, 0, 2)); got != lines(`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`) {
		t.Errorf("typing without the server: app got %s", got)
	}
	if got := g.im.took(); got != "" {
		t.Errorf("the gone server got %q", got)
	}

	// It starts again: XIM_SERVERS changes, the client reconnects and
	// recreates the context for the field that has focus.
	g.im.start()
	g.im.nextIC = 4
	got := g.feed(propNotify(g.b.root, g.b.atoms.ximServers, false))
	if got != "session true" {
		t.Errorf("server back: app got %q", got)
	}
	if got, want := g.im.took(), lines(
		"xconnect from 902", "CONNECT", "OPEN ja_JP.UTF-8", "QUERY_EXTENSION",
		"ENCODING [UTF8_STRING UTF-8 COMPOUND_TEXT]", "GET_IM_VALUES",
		"CREATE_IC style=0x402 client=7 focus=7 spot=20,72 -> 5", "FOCUS 5"); got != want {
		t.Errorf("reconnect:\n got %s\nwant %s", got, want)
	}

	// Another instance takes the selection (a restart without a
	// DestroyNotify seen): the client reconnects to it.
	g.x.owners[g.im.sel] = 960
	g.im.owner = 960
	g.feed(propNotify(g.b.root, g.b.atoms.ximServers, false))
	if g.b.ime.state != imeReady || g.b.ime.owner != 960 {
		t.Errorf("new owner: state %d owner %d", g.b.ime.state, g.b.ime.owner)
	}
}

func TestIMEStartsLaterAndStyles(t *testing.T) {
	// The server is not running yet: the client waits.
	g := newIMERig(t, func(im *fakeIM) {
		im.styles = []uint32{xim.PreeditPosition | xim.StatusNothing, xim.PreeditNothing | xim.StatusNothing}
		im.refuse = []uint32{xim.PreeditPosition | xim.StatusNothing}
	})
	delete(g.x.props[g.b.root], g.b.atoms.ximServers)
	g.connect()
	if g.b.ime.state != imeIdle || g.im.took() != "" {
		t.Fatalf("no server: state %d", g.b.ime.state)
	}
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	if got := g.pump(); got != "" {
		t.Errorf("no server: app got %q", got)
	}
	// It starts; over-the-spot is refused at CREATE_IC, so the root style
	// is used.
	g.im.start()
	if got := g.feed(propNotify(g.b.root, g.b.atoms.ximServers, false)); got != "session true" {
		t.Errorf("server started: app got %q", got)
	}
	got := g.im.took()
	if !strings.Contains(got, "CREATE_IC style=0x404 client=7 focus=7 spot=20,72: refused; CREATE_IC style=0x408 client=7 focus=7 spot=20,72 -> 1; FOCUS 1") {
		t.Errorf("style fallback: %s", got)
	}

	// A server whose handshake stalls is given up after imeTimeout.
	g = newIMERig(t, nil)
	g.x.sendHook = func(dest xproto.Window, ev []byte) bool { return dest == fakeIMOwner } // XCONNECT unanswered
	g.connect()
	if g.b.ime.state != imeXConnect {
		t.Fatalf("stalled: state %d", g.b.ime.state)
	}
	if n := g.x.fire(imeTimeout); n != 1 {
		t.Fatalf("%d timers fired", n)
	}
	if g.b.ime.state != imeIdle || g.b.ime.err == nil {
		t.Errorf("after the timeout: state %d err %v", g.b.ime.state, g.b.ime.err)
	}
}

func TestIMETransport(t *testing.T) {
	for _, c := range []struct {
		name      string
		configure func(*fakeIM)
	}{
		{"properties", nil},
		{"one property for several messages", func(im *fakeIM) { im.sameProp = true }},
		{"moredata parts", func(im *fakeIM) { im.moreData = true }},
		{"transport 0.2 boundary", func(im *fakeIM) { im.boundary = 100 }},
		{"compound text", func(im *fakeIM) { im.utf8 = false }},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newIMERig(t, func(im *fakeIM) {
				if c.configure != nil {
					c.configure(im)
				}
				im.onKey = func(press bool, kc byte) bool {
					if press {
						// Two long messages in one read.
						im.commit(1, "a fairly long commit string")
						im.draw(1, 2, 0, 0, "かな")
					}
					return true
				}
			})
			g.focusField(t)
			if got, want := g.feed(press(kcA, 0, 1)), `ime "a fairly long commit string" "かな" 6,6`; got != want {
				t.Errorf("got %s, want %s", got, want)
			}
			log := g.im.took()
			switch {
			case g.im.boundary == 100 && g.im.props != 0:
				t.Errorf("with a 100-byte boundary the client used a property: %s", log)
			case g.im.boundary == 0 && g.im.props == 0:
				t.Errorf("CREATE_IC and forwards (longer than 20 bytes) did not go through a property: %s", log)
			}
			if len(g.b.ime.offsets) != 0 {
				t.Errorf("property offsets left: %v", g.b.ime.offsets)
			}
		})
	}
}

func TestIMECreateICFailures(t *testing.T) {
	// Every style refused: the window goes without an input method, and
	// the client does not ask again until the server reconnects.
	g := newIMERig(t, func(im *fakeIM) {
		im.styles = []uint32{xim.PreeditNothing | xim.StatusNothing}
		im.refuse = im.styles
	})
	g.connect()
	g.im.took()
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	g.pump()
	if got, want := g.im.took(), "CREATE_IC style=0x408 client=7 focus=7 spot=20,72: refused"; got != want {
		t.Errorf("refused:\n got %s\nwant %s", got, want)
	}
	g.w.SetTextInput(textState(platform.TextPurposeEmail))
	if got := g.pump(); got != "" {
		t.Errorf("app got %q", got)
	}
	if got := g.im.took(); got != "" {
		t.Errorf("asked again: %s", got)
	}
	if got := g.feed(press(kcA, 0, 1)); got != `key press KeyA "a" "a" mods=0` {
		t.Errorf("typing: app got %s", got)
	}

	// An error that is not BadStyle (the window went away: BadFocusWindow)
	// fails that window's CREATE_IC only: the style stays, and the next
	// connection's windows get the best one.
	g = newIMERig(t, func(im *fakeIM) { im.failNext = xim.ErrBadFocusWindow })
	g.connect()
	g.im.took()
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	g.pump()
	if got, want := g.im.took(), "CREATE_IC style=0x402 client=7 focus=7 spot=20,72: error 4"; got != want {
		t.Errorf("BadFocusWindow:\n got %s\nwant %s", got, want)
	}
	if want := uint32(xim.PreeditCallbacks | xim.StatusNothing); g.b.ime.style != want {
		t.Errorf("style after BadFocusWindow = %#x, want %#x (unchanged)", g.b.ime.style, want)
	}
	if !g.w.icFailed || g.b.ime.creating {
		t.Errorf("after BadFocusWindow: icFailed %v, creating %v; want the window failed and nothing in flight", g.w.icFailed, g.b.ime.creating)
	}

	// A window closed while its CREATE_IC is in flight: the context the
	// reply creates is destroyed.
	g = newIMERig(t, nil)
	g.connect()
	g.im.took()
	g.w.SetTextInput(textState(platform.TextPurposeNormal))
	g.b.dispatch(g.r) // sends CREATE_IC; the reply waits in im.out
	g.w.Close()
	g.pump()
	if got, want := g.im.took(), lines("CREATE_IC style=0x402 client=7 focus=7 spot=20,72 -> 1", "DESTROY_IC 1"); got != want {
		t.Errorf("closed while creating:\n got %s\nwant %s", got, want)
	}
}

func TestIMEReleaseOutsideForwardMask(t *testing.T) {
	// A server that takes presses only (no KeyRelease in its forward
	// mask): the release of a key whose press is at the server waits for
	// the server's answer, so the app sees the press first, then the
	// release. (Without the wait the keyboard dropped the release of a key
	// it had not seen pressed, then got the press: a key stuck down.)
	g := newIMERig(t, func(im *fakeIM) {
		im.icMask = &xim.SetEventMask{IMID: 1, Forward: xproto.EventMaskKeyPress}
	})
	g.focusField(t)
	if got, want := g.feed(press(kcA, 0, 1), release(kcA, 0, 2)), lines(`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`); got != want {
		t.Errorf("sent back: app got %s, want %s", got, want)
	}
	if got, want := g.im.took(), "KEY 1 press 38"; got != want {
		t.Errorf("sent back: server got %s, want %s", got, want)
	}
	if n := g.x.fire(imeSyncTimeout); n != 0 {
		t.Errorf("%d timers left after the press came back", n)
	}

	// A press the server consumes: its release waits until imeSyncTimeout
	// or the next press of the key, and the keyboard drops it.
	g.im.onKey = func(press bool, kc byte) bool { return kc == kcY }
	if got, want := g.feed(press(kcY, 0, 3), release(kcY, 0, 4), press(kcA, 0, 5), release(kcA, 0, 6)), lines(`key press KeyA "a" "a" mods=0`, `key release KeyA "a" "" mods=0`); got != want {
		t.Errorf("consumed: app got %s, want %s", got, want)
	}
	if n := g.x.fire(imeSyncTimeout); n != 1 {
		t.Errorf("%d timers fired, want the consumed press's", n)
	}
	if got := g.pump(); got != "" {
		t.Errorf("consumed, after the timeout: app got %s", got)
	}
	if got, want := g.feed(press(kcY, 0, 7), release(kcY, 0, 8), press(kcY, 0, 9)), ""; got != want {
		t.Errorf("consumed twice: app got %s, want %s", got, want)
	}
	g.im.took()

	// Held behind a synchronous forward the server does not answer: the
	// release goes after the press, when the wait ends.
	g.im.onKey = nil
	g.w.ic.mask.sync = xproto.EventMaskKeyPress
	g.im.ignoreSync = true
	if got, want := g.feed(press(kcQ, 0, 10), release(kcQ, 0, 11)), `key press KeyQ "q" "q" mods=0`; got != want {
		t.Errorf("unanswered: app got %s, want %s (the release waits for the answer)", got, want)
	}
	g.im.ignoreSync = false
	g.x.fire(imeSyncTimeout)
	if got, want := g.pump(), `key release KeyQ "q" "" mods=0`; got != want {
		t.Errorf("after the timeout: app got %s, want %s", got, want)
	}
	if got, want := g.im.took(), "KEY 1 press 24 sync"; got != want {
		t.Errorf("held: server got %s, want %s", got, want)
	}
}
