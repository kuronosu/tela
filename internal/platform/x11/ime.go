//go:build linux

package x11

import (
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xim"
)

// Input methods over XIM (ADR-231). The client is asynchronous: every step
// of the protocol is a message the loop reads like any other X event, and
// nothing waits for a reply on the UI thread (libX11 blocks in most of
// them). The wire format is package internal/xim; this file is the state
// machine and its mapping onto the platform contract (ADR-068).
//
//   - Discovery: XMODIFIERS names the input method ("@im=fcitx"); none, or
//     "@im=none", means no input method and the keyboard works as without
//     one. The server owns the selection "@server=<name>" listed in the
//     root window's XIM_SERVERS. TRANSPORT must offer "X/"; LOCALES gives
//     the locale sent in XIM_OPEN (xim.ChooseLocale). A server that is not
//     running is waited for: servers rewrite XIM_SERVERS when they start
//     (xcb-imdkit and IMdkit prepend nothing to force the PropertyNotify),
//     and the client connects then.
//   - Transport version 0.0 (the one libX11 and xcb-imdkit speak): a
//     message of up to 20 bytes travels in one format-8 ClientMessage
//     (_XIM_PROTOCOL; the server's longer ones may be cut into
//     _XIM_MOREDATA parts), a longer one in a property of the receiver's
//     window announced by a format-32 ClientMessage (length, atom). A
//     server may append several messages to one property before the
//     client reads it, so properties are read from a per-atom offset with
//     delete, which deletes only once nothing is left (as xcb-imdkit
//     reads), and messages keep their order.
//   - Handshake: _XIM_XCONNECT, XIM_CONNECT, XIM_OPEN, XIM_QUERY_EXTENSION
//     (no extension is used; it is sent as libX11 does and its reply is
//     ignored), XIM_ENCODING_NEGOTIATION (UTF8_STRING, which fcitx5
//     accepts, then UTF-8, then COMPOUND_TEXT, the default and ibus's only
//     encoding, decoded by xim.DecodeCompoundText), and XIM_GET_IM_VALUES
//     of queryInputStyle. Without a reply within imeTimeout the client
//     gives up until XIM_SERVERS changes.
//   - Input styles, best first: on-the-spot (XIMPreeditCallbacks, so Tela
//     draws the preedit like on the other platforms), then over-the-spot
//     (XIMPreeditPosition: the input method shows the preedit in its own
//     window at the spot location; fcitx5 offers only this unless its XIM
//     frontend's "UseOnTheSpot" option is set), then root
//     (XIMPreeditNothing); with XIMStatusNothing or XIMStatusNone. A
//     CREATE_IC refused with BadStyle tries the next style (for the
//     input method: every later context uses it until the server
//     reconnects); another error fails that window's context only.
//   - One input context per window, created when the window first gets a
//     text input state, and focused (XIM_SET_IC_FOCUS) while the window
//     has keyboard focus and a state whose purpose is not a password: a
//     password field gets no input method (GTK uses its simple context
//     there), so its keys take the plain KeyEvent path with Compose. The
//     session (IMESessionEvent) is that focus. The spot location is the
//     bottom-left of the cursor area in physical pixels (GTK's choice:
//     candidate windows open below it), sent before the focus and when it
//     changes, at most once per loop iteration.
//   - Keys: while the context has focus, KeyPress and KeyRelease (as the
//     server's XIM_SET_EVENT_MASK asks; both until it says) go to the
//     server in XIM_FORWARD_EVENT instead of the keyboard. The server
//     consumes them or sends them back in XIM_FORWARD_EVENT, and those go
//     through the keyboard as if the X server had sent them, so dead keys
//     and Compose work once, in whichever of the two handles them. Keys
//     the server asks to get synchronously (its synchronous event mask)
//     are forwarded with the synchronous flag and the next ones wait for
//     XIM_SYNC_REPLY (at most imeSyncTimeout). Every synchronous message
//     of the server (a forwarded event, a commit, XIM_SYNC) is answered
//     with XIM_SYNC_REPLY: fcitx5 runs in sync mode and holds the
//     client's next keys until it. Keys that wait keep their order, and
//     those the forward mask leaves out go to the keyboard in their turn;
//     a release the server does not take waits for its press if that is
//     at the server (imeKeyboard), so the keyboard never sees a key's
//     release before its press.
//   - Composition: XIM_COMMIT strings (XLookupChars; a keysym alone is
//     committed as its character when it has one) and the preedit of
//     XIM_PREEDIT_START/DRAW/CARET/DONE are latched and become one
//     IMEEvent per read of the connection, as a Wayland done does, and
//     before any key the server sends back, so that ordering holds. The
//     preedit cursor is the first run of characters with the reverse (or
//     highlight) feedback, the converted clause fcitx5 highlights, else
//     the caret; XIMCaretInvisible hides it. XIM counts characters; Tela
//     counts UTF-8 bytes.
//   - ResetTextInput is XIM_RESET_IC; until its reply, preedit updates and
//     commits are not delivered (they were made before the reset), and
//     the preedit string the reply returns is dropped: the app finished
//     the composition itself.
//   - The server going away (DestroyNotify of its windows, which the
//     client selects, or a new owner of its selection) ends the sessions
//     (IMESessionEvent false), sends any held keys through the keyboard,
//     and the client waits for the server to come back. Keys the server
//     had not answered are lost, as in every XIM client.
//   - Dynamic event flow (XIM_REGISTER_TRIGGERKEYS, used by old servers
//     such as kinput2) is not implemented: such a server gets no keys
//     until it sets an event mask, so typing keeps working without it.

const (
	// imeTimeout bounds each handshake step and CREATE_IC.
	imeTimeout = 5 * time.Second
	// imeSyncTimeout bounds the wait for the XIM_SYNC_REPLY of a key
	// forwarded synchronously.
	imeSyncTimeout = 2 * time.Second
	// imeMaxMoreData bounds the parts of one _XIM_MOREDATA message.
	imeMaxMoreData = 1 << 20
	// imeClientProps is how many property atoms ("_client0".."_client20",
	// libX11's names) long messages rotate through.
	imeClientProps = 21
)

// imeEncodings are the encodings offered in XIM_ENCODING_NEGOTIATION, best
// first: the first two are UTF-8 (fcitx5 knows the first, libX11-based
// servers the second), the last is the protocol's default.
var imeEncodings = []string{"UTF8_STRING", "UTF-8", "COMPOUND_TEXT"}

// imeStylePreference ranks the input styles the client can use.
var imeStylePreference = []uint32{
	xim.PreeditCallbacks | xim.StatusNothing,
	xim.PreeditCallbacks | xim.StatusNone,
	xim.PreeditPosition | xim.StatusNothing,
	xim.PreeditPosition | xim.StatusNone,
	xim.PreeditNothing | xim.StatusNothing,
	xim.PreeditNothing | xim.StatusNone,
}

// imeState is the progress of the connection to the input method.
type imeState uint8

const (
	imeOff       imeState = iota // no input method named, or the backend is closed
	imeIdle                      // named; waiting for its server
	imeTransport                 // TRANSPORT conversion requested
	imeLocales                   // LOCALES conversion requested
	imeXConnect                  // _XIM_XCONNECT sent
	imeConnect                   // XIM_CONNECT sent
	imeOpen                      // XIM_OPEN sent
	imeEncoding                  // XIM_ENCODING_NEGOTIATION sent
	imeStyles                    // XIM_GET_IM_VALUES (queryInputStyle) sent
	imeReady                     // input contexts can be created
)

// imeClient is the backend's XIM client (UI thread).
type imeClient struct {
	name  string // the input method of XMODIFIERS
	state imeState
	win   xproto.Window // ours: selection requestor and transport window
	sel   xproto.Atom   // "@server=<name>"
	owner xproto.Window // the selection's owner
	// server is the server's transport window (XCONNECT reply); boundary
	// the longest message sent in ClientMessages.
	server   xproto.Window
	boundary int
	nextProp int
	offsets  map[xproto.Atom]uint32 // read offsets of the server's properties
	more     []byte                 // _XIM_MOREDATA parts
	locale   string

	imid     uint16
	icAttrs  map[string]uint16 // IC attribute IDs by name
	imAttrs  map[string]uint16
	utf8     bool
	styles   []uint32 // the usable styles of the server, best first
	style    uint32   // the style input contexts are created with
	triggers bool     // the server registered trigger keys (dynamic flow)
	mask     imeMask  // XIM_SET_EVENT_MASK for the whole input method

	ics      map[uint16]*xic
	pending  []*Window // windows waiting for an input context, the first one sent
	creating bool      // a CREATE_IC is waiting for its reply (for pending[0])

	stop func() // the timeout of the handshake step or CREATE_IC in flight
	err  error  // the latest failure, for tests and diagnosis
}

// imeMask is an XIM_SET_EVENT_MASK.
type imeMask struct {
	set           bool
	forward, sync uint32
}

// xic is an input context: one per window.
type xic struct {
	w       *Window
	id      uint16
	focused bool // XIM_SET_IC_FOCUS sent: the session
	mask    imeMask
	spot    [2]int16
	spotSet bool

	// The preedit as the server draws it, one feedback per character.
	text   []rune
	fb     []uint32
	caret  int
	hidden bool
	// The latch: commits and whether the preedit changed since the last
	// IMEEvent.
	commit string
	dirty  bool
	// The preedit and cursor of the last IMEEvent.
	preedit string
	cursor  [2]int
	// resetting: a RESET_IC waits for its reply.
	resetting bool

	// waiting: a key forwarded synchronously waits for XIM_SYNC_REPLY;
	// held are the keys that came after it.
	waiting  bool
	held     []xgb.Event
	stopWait func()
	// unanswered: key codes pressed and forwarded asynchronously, whose
	// release the server does not take, and not sent back yet; deferred:
	// their releases, waiting for them (imeKeyboard).
	unanswered [256]bool
	deferred   map[byte]deferredRelease
}

// deferredRelease is a release that waits for its press (imeKeyboard), and
// the stop of its timeout.
type deferredRelease struct {
	ev   xgb.Event
	stop func()
}

// initIME starts connecting to the input method XMODIFIERS names.
func (b *Backend) initIME(xmodifiers string) {
	name := xim.IMName(xmodifiers)
	if name == "" {
		return
	}
	ime := &b.ime
	*ime = imeClient{name: name, state: imeIdle}
	win, err := b.xs.createIMWindow()
	if err != nil {
		ime.state = imeOff
		ime.err = err
		return
	}
	ime.win = win
	ime.sel = b.atom(xim.ServerSelection(name))
	b.imeDiscover()
}

// imeDiscover looks for the server in XIM_SERVERS and starts the
// handshake. Without it the client stays idle until XIM_SERVERS changes.
func (b *Backend) imeDiscover() {
	ime := &b.ime
	if ime.state != imeIdle || ime.sel == 0 {
		return
	}
	p, ok := b.xs.property(b.root, b.atoms.ximServers, false)
	if !ok || p.format != 32 {
		return
	}
	listed := false
	for i := 0; i+4 <= len(p.data); i += 4 {
		if xproto.Atom(le.Uint32(p.data[i:])) == ime.sel {
			listed = true
		}
	}
	if !listed {
		return
	}
	owner := b.xs.owner(ime.sel)
	if owner == 0 {
		return
	}
	ime.owner = owner
	ime.state = imeTransport
	b.xs.convert(ime.win, ime.sel, b.atoms.ximTransport, b.atoms.ximTransport, 0)
	b.imeArm()
}

// imeArm (re)starts the timeout of the step in flight.
func (b *Backend) imeArm() {
	ime := &b.ime
	if ime.stop != nil {
		ime.stop()
	}
	state := ime.state
	ime.stop = b.after(imeTimeout, func() {
		ime.stop = nil
		switch {
		case ime.state == state && state < imeReady:
			b.imeFail(fmt.Errorf("no answer from the input method %q (step %d)", ime.name, state), false)
		case ime.state == imeReady && ime.creating:
			b.imeCreated(0, false) // CREATE_IC unanswered: skip that window
		}
	})
}

func (b *Backend) imeDisarm() {
	if ime := &b.ime; ime.stop != nil {
		ime.stop()
		ime.stop = nil
	}
}

// imeFail records why the connection failed and drops it; the client
// waits for XIM_SERVERS to change. report prints protocol failures once.
func (b *Backend) imeFail(err error, report bool) {
	ime := &b.ime
	if report {
		err = keyboard.Report(fmt.Errorf("x11: input method %q: %w (typing works without it)", ime.name, err))
	}
	ime.err = err
	b.imeDisconnect()
}

// imeDisconnect forgets the server: the sessions end, held keys go to the
// keyboard, and the client is idle again.
func (b *Backend) imeDisconnect() {
	ime := &b.ime
	if ime.state <= imeIdle {
		return
	}
	b.imeDisarm()
	for _, w := range b.loop.Windows() {
		if ic := w.ic; ic != nil {
			b.imeEndSession(ic)
			w.ic = nil
		}
		w.icFailed = false
	}
	*ime = imeClient{name: ime.name, state: imeIdle, win: ime.win, sel: ime.sel, err: ime.err, nextProp: ime.nextProp}
}

// imeServersChanged follows XIM_SERVERS: a server that starts, restarts
// (a new owner of its selection) or quits.
func (b *Backend) imeServersChanged() {
	ime := &b.ime
	switch {
	case ime.state == imeIdle:
		b.imeDiscover()
	case ime.state > imeIdle && b.xs.owner(ime.sel) != ime.owner:
		b.imeDisconnect()
		b.imeDiscover()
	}
}

// imeDestroyed handles the DestroyNotify of a window: the server's windows
// going away is the server quitting or crashing.
func (b *Backend) imeDestroyed(win xproto.Window) {
	ime := &b.ime
	if ime.state > imeIdle && win != 0 && (win == ime.server || win == ime.owner) {
		b.imeDisconnect()
		b.imeDiscover()
	}
}

// imeEvent handles the X events addressed to the client's window (the
// selection conversions and the transport) and reports whether ev was one.
func (b *Backend) imeEvent(ev xgb.Event) bool {
	ime := &b.ime
	if ime.state == imeOff || ime.win == 0 {
		return false
	}
	switch ev := ev.(type) {
	case xproto.SelectionNotifyEvent:
		if ev.Requestor != ime.win {
			return false
		}
		b.imeSelection(ev)
		return true
	case xproto.ClientMessageEvent:
		if ev.Window != ime.win {
			return false
		}
		b.imeClientMessage(ev)
		return true
	}
	return false
}

// imeSelection reads TRANSPORT, then LOCALES, then opens the transport.
func (b *Backend) imeSelection(ev xproto.SelectionNotifyEvent) {
	ime := &b.ime
	a := &b.atoms
	var value string
	if ev.Property != 0 {
		if p, ok := b.xs.property(ime.win, ev.Property, true); ok {
			value = string(p.data)
		}
	}
	switch {
	case ime.state == imeTransport && ev.Target == a.ximTransport:
		if !xim.HasXTransport(value) {
			b.imeFail(fmt.Errorf("the server offers no X transport (%q)", value), true)
			return
		}
		ime.state = imeLocales
		b.xs.convert(ime.win, ime.sel, a.ximLocales, a.ximLocales, 0)
		b.imeArm()
	case ime.state == imeLocales && ev.Target == a.ximLocales:
		ime.locale = xim.ChooseLocale(xim.ParseLocales(value), processLocale())
		ime.state = imeXConnect
		ev := xproto.ClientMessageEvent{Format: 32, Window: ime.owner, Type: a.ximXConnect,
			Data: xproto.ClientMessageDataUnionData32New([]uint32{uint32(ime.win), 0, 0, 0, 0})}
		b.xs.sendEvent(ime.owner, 0, ev.Bytes())
		b.imeArm()
	}
}

// processLocale is the process's LC_CTYPE as the environment sets it.
func processLocale() string {
	for _, v := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if s := os.Getenv(v); s != "" {
			return s
		}
	}
	return ""
}

// imeClientMessage reads the transport.
func (b *Backend) imeClientMessage(ev xproto.ClientMessageEvent) {
	ime := &b.ime
	a := &b.atoms
	switch ev.Type {
	case a.ximXConnect:
		if ime.state != imeXConnect || ev.Format != 32 {
			return
		}
		d := ev.Data.Data32
		ime.server, ime.boundary = xproto.Window(d[0]), xim.CMDataSize
		if d[1] == 0 && d[2] == 2 && d[3] >= xim.CMDataSize {
			// Transport 0.2: the server says how long a message may be
			// before it goes through a property.
			ime.boundary = int(min(d[3], 1<<16))
		}
		// Hear about the server quitting.
		b.xs.selectInput(ime.server, xproto.EventMaskStructureNotify)
		if ime.owner != ime.server {
			b.xs.selectInput(ime.owner, xproto.EventMaskStructureNotify)
		}
		ime.state = imeConnect
		b.imeSend(xim.EncodeConnect())
		b.imeArm()
	case a.ximMoreData:
		if ev.Format == 8 && len(ime.more) < imeMaxMoreData {
			ime.more = append(ime.more, ev.Data.Data8...)
		}
	case a.ximProtocol:
		var data []byte
		switch ev.Format {
		case 8:
			data = append(ime.more, ev.Data.Data8...)
			ime.more = nil
		case 32:
			data = b.imeReadProperty(xproto.Atom(ev.Data.Data32[1]), ev.Data.Data32[0])
		}
		msgs, _ := xim.Split(data) // a truncated tail is dropped
		for _, m := range msgs {
			if b.ime.state <= imeIdle {
				return // the server failed in an earlier message
			}
			b.imeHandle(m)
		}
	}
}

// imeReadProperty reads length bytes of the server's property atom on the
// client's window, from where the last read of it ended.
func (b *Backend) imeReadProperty(atom xproto.Atom, length uint32) []byte {
	ime := &b.ime
	if length == 0 || length > 1<<20 {
		return nil
	}
	off := ime.offsets[atom]
	data, after, ok := b.xs.propertyPart(ime.win, atom, off/4, (off%4+length+3)/4, true)
	if !ok {
		delete(ime.offsets, atom)
		return nil
	}
	if ime.offsets == nil {
		ime.offsets = map[xproto.Atom]uint32{}
	}
	if after > 0 {
		ime.offsets[atom] = off + length
	} else {
		delete(ime.offsets, atom) // read whole: the server deleted it
	}
	start := int(off % 4)
	if start >= len(data) {
		return nil
	}
	return data[start:min(len(data), start+int(length))]
}

// imeSend sends a message to the server.
func (b *Backend) imeSend(msg []byte) {
	ime := &b.ime
	if ime.server == 0 {
		return
	}
	a := &b.atoms
	if len(msg) <= ime.boundary {
		chunks := xim.Chunks(msg)
		for i, c := range chunks {
			typ := a.ximMoreData
			if i == len(chunks)-1 {
				typ = a.ximProtocol
			}
			ev := xproto.ClientMessageEvent{Format: 8, Window: ime.server, Type: typ, Data: xproto.ClientMessageDataUnionData8New(c[:])}
			b.xs.sendEvent(ime.server, 0, ev.Bytes())
		}
		return
	}
	prop := b.atom(fmt.Sprintf("_client%d", ime.nextProp))
	ime.nextProp = (ime.nextProp + 1) % imeClientProps
	if prop == 0 {
		return
	}
	b.xs.appendProperty(ime.server, prop, xproto.AtomString, 8, msg)
	ev := xproto.ClientMessageEvent{Format: 32, Window: ime.server, Type: a.ximProtocol,
		Data: xproto.ClientMessageDataUnionData32New([]uint32{uint32(len(msg)), uint32(prop), 0, 0, 0})}
	b.xs.sendEvent(ime.server, 0, ev.Bytes())
}

// imeHandle runs one message of the server.
func (b *Backend) imeHandle(m xim.Message) {
	ime := &b.ime
	switch m.Major {
	case xim.OpConnectReply:
		if ime.state == imeConnect {
			ime.state = imeOpen
			b.imeSend(xim.EncodeOpen(ime.locale))
			b.imeArm()
		}
	case xim.OpOpenReply:
		if ime.state != imeOpen {
			return
		}
		o, err := xim.ParseOpenReply(m.Body)
		if err != nil {
			b.imeFail(fmt.Errorf("XIM_OPEN_REPLY: %w", err), true)
			return
		}
		ime.imid = o.IMID
		ime.imAttrs, ime.icAttrs = attrIDs(o.IMAttrs), attrIDs(o.ICAttrs)
		ime.state = imeEncoding
		b.imeSend(xim.EncodeQueryExtension(ime.imid, nil))
		b.imeSend(xim.EncodeEncodingNegotiation(ime.imid, imeEncodings))
		b.imeArm()
	case xim.OpEncodingNegotiationReply:
		if ime.state != imeEncoding {
			return
		}
		_, category, index, err := xim.ParseEncodingNegotiationReply(m.Body)
		ime.utf8 = err == nil && category == 0 && index >= 0 && int(index) < len(imeEncodings) && imeEncodings[index] != "COMPOUND_TEXT"
		b.imeQueryStyles()
	case xim.OpGetIMValuesReply:
		if ime.state != imeStyles {
			return
		}
		var styles []uint32
		if _, attrs, err := xim.ParseGetIMValuesReply(m.Body); err == nil {
			for _, a := range attrs {
				if a.ID == ime.imAttrs[xim.AttrQueryInputStyle] {
					styles, _ = xim.ParseStyles(a.Value)
				}
			}
		}
		b.imeSetStyles(styles)
	case xim.OpRegisterTriggerKeys:
		ime.triggers = true
	case xim.OpSetEventMask:
		em, err := xim.ParseSetEventMask(m.Body)
		if err != nil {
			return
		}
		mask := imeMask{set: true, forward: em.Forward, sync: em.Sync}
		if em.ICID == 0 {
			ime.mask = mask
		} else if ic := ime.ics[em.ICID]; ic != nil {
			ic.mask = mask
		}
	case xim.OpCreateICReply:
		if _, icid, err := xim.ParseIDs(m.Body); err == nil && ime.creating {
			b.imeCreated(icid, true)
		}
	case xim.OpError:
		e, err := xim.ParseError(m.Body)
		if err == nil {
			b.imeError(e)
		}
	case xim.OpForwardEvent:
		if f, err := xim.ParseForwardEvent(m.Body); err == nil {
			b.imeForwarded(f)
		}
	case xim.OpCommit:
		if c, err := xim.ParseCommit(m.Body); err == nil {
			b.imeCommit(c)
		}
	case xim.OpSync:
		if imid, icid, err := xim.ParseIDs(m.Body); err == nil {
			b.imeSend(xim.EncodeIDs(xim.OpSyncReply, imid, icid))
		}
	case xim.OpSyncReply:
		if _, icid, err := xim.ParseIDs(m.Body); err == nil {
			if ic := ime.ics[icid]; ic != nil && ic.waiting {
				b.imeSynced(ic)
			}
		}
	case xim.OpPreeditStart:
		if imid, icid, err := xim.ParseIDs(m.Body); err == nil {
			if ic := ime.ics[icid]; ic != nil && ic.focused {
				ic.clearText()
			}
			b.imeSend(xim.EncodePreeditStartReply(imid, icid, -1))
		}
	case xim.OpPreeditDraw:
		if d, err := xim.ParsePreeditDraw(m.Body); err == nil {
			if ic := ime.ics[d.ICID]; ic != nil && ic.focused {
				ic.draw(d, b.imeText(d.Text))
			}
		}
	case xim.OpPreeditCaret:
		if c, err := xim.ParsePreeditCaret(m.Body); err == nil {
			pos := c.Position
			if ic := ime.ics[c.ICID]; ic != nil && ic.focused {
				pos = int32(ic.moveCaret(c))
			}
			b.imeSend(xim.EncodePreeditCaretReply(c.IMID, c.ICID, pos))
		}
	case xim.OpPreeditDone:
		if _, icid, err := xim.ParseIDs(m.Body); err == nil {
			if ic := ime.ics[icid]; ic != nil && ic.focused {
				ic.clearText()
			}
		}
	case xim.OpResetICReply:
		if _, icid, _, err := xim.ParseResetICReply(m.Body); err == nil {
			if ic := ime.ics[icid]; ic != nil && ic.resetting {
				ic.resetting = false
				ic.clearText()
				ic.commit, ic.dirty = "", false
			}
		}
	case xim.OpAuthRequired, xim.OpAuthNext, xim.OpAuthSetup, xim.OpAuthNG:
		b.imeFail(fmt.Errorf("the server wants authentication (XIM opcode %d), which Tela does not do", m.Major), true)
	default:
		// Ignored: replies to requests sent without waiting
		// (XIM_QUERY_EXTENSION, XIM_SET_IC_VALUES, XIM_DESTROY_IC,
		// XIM_CLOSE), the status of styles Tela does not ask for,
		// geometry and string conversion requests.
	}
}

// attrIDs indexes attribute descriptions by name.
func attrIDs(attrs []xim.Attr) map[string]uint16 {
	m := make(map[string]uint16, len(attrs))
	for _, a := range attrs {
		m[a.Name] = a.ID
	}
	return m
}

// imeQueryStyles asks for the input styles, or assumes the root style
// when the server does not describe them.
func (b *Backend) imeQueryStyles() {
	ime := &b.ime
	id, ok := ime.imAttrs[xim.AttrQueryInputStyle]
	if !ok {
		b.imeSetStyles([]uint32{xim.PreeditNothing | xim.StatusNothing})
		return
	}
	ime.state = imeStyles
	b.imeSend(xim.EncodeGetIMValues(ime.imid, []uint16{id}))
	b.imeArm()
}

// imeSetStyles keeps the server's styles the client can use, best first,
// and finishes the handshake.
func (b *Backend) imeSetStyles(server []uint32) {
	ime := &b.ime
	ime.styles = nil
	for _, s := range imeStylePreference {
		if slices.Contains(server, s) {
			ime.styles = append(ime.styles, s)
		}
	}
	if len(ime.styles) == 0 {
		b.imeFail(fmt.Errorf("no usable input style among %#x", server), true)
		return
	}
	ime.style = ime.styles[0]
	ime.state = imeReady
	ime.err = nil
	b.imeDisarm()
	// Windows that already have a text field get their context now.
	for _, w := range b.loop.Windows() {
		if w.textInput != nil {
			b.imeRequestIC(w)
		}
	}
}

// imeError handles XIM_ERROR: during the handshake the connection fails;
// a CREATE_IC (no IC ID) refused with BadStyle tries the next style, for
// the whole input method (the server does not support that style, for
// any window; a new connection starts from the best one again), and one
// refused with another error (BadClientWindow or BadFocusWindow for a
// window that went away, BadAlloc) fails that window's context only;
// other errors (a spot location the server rejects) are recorded only.
func (b *Backend) imeError(e xim.Error) {
	ime := &b.ime
	switch {
	case ime.state < imeReady:
		b.imeFail(e, true)
	case ime.creating && e.Flag&xim.ErrICIDValid == 0 && e.Code != xim.ErrBadStyle:
		ime.err = e
		b.imeCreated(0, false)
	case ime.creating && e.Flag&xim.ErrICIDValid == 0:
		ime.err = e
		if i := slices.Index(ime.styles, ime.style); i >= 0 && i+1 < len(ime.styles) {
			ime.style = ime.styles[i+1]
			ime.creating = false
			b.imeCreateNext()
			return
		}
		ime.err = keyboard.Report(fmt.Errorf("x11: input method %q refused every input style: %w", ime.name, e))
		b.imeCreated(0, false)
	default:
		ime.err = e
	}
}

// --- input contexts ---

// imeRequestIC queues the creation of w's input context.
func (b *Backend) imeRequestIC(w *Window) {
	ime := &b.ime
	if ime.state != imeReady || w.ic != nil || w.icFailed || w.Closed() || slices.Contains(ime.pending, w) {
		return
	}
	ime.pending = append(ime.pending, w)
	if !ime.creating {
		b.imeCreateNext()
	}
}

// imeCreateNext sends the CREATE_IC of the first pending window. Replies
// do not say which request they answer, so one is in flight at a time.
func (b *Backend) imeCreateNext() {
	ime := &b.ime
	for len(ime.pending) > 0 && !ime.creating {
		w := ime.pending[0]
		if w.Closed() || w.ic != nil {
			ime.pending = ime.pending[1:]
			continue
		}
		var attrs []xim.Attribute
		add := func(name string, value []byte) {
			if id, ok := ime.icAttrs[name]; ok {
				attrs = append(attrs, xim.Attribute{ID: id, Value: value})
			}
		}
		add(xim.AttrInputStyle, xim.Card32(ime.style))
		add(xim.AttrClientWindow, xim.Card32(uint32(w.id)))
		add(xim.AttrFocusWindow, xim.Card32(uint32(w.id)))
		if spot, ok := w.imeSpot(); ok || ime.style&xim.PreeditPosition != 0 {
			if pre, ok := b.imeSpotAttr(spot); ok {
				attrs = append(attrs, pre)
			}
		}
		ime.creating = true
		b.imeSend(xim.EncodeCreateIC(ime.imid, attrs))
		b.imeArm()
	}
}

// imeCreated finishes the CREATE_IC in flight: ok with the server's IC ID,
// or failed (the window goes without an input method).
func (b *Backend) imeCreated(icid uint16, ok bool) {
	ime := &b.ime
	ime.creating = false
	b.imeDisarm()
	if len(ime.pending) == 0 {
		return
	}
	w := ime.pending[0]
	ime.pending = ime.pending[1:]
	switch {
	case !ok:
		w.icFailed = true // until the server reconnects
	case w.Closed() || w.ic != nil || icid == 0:
		b.imeSend(xim.EncodeIDs(xim.OpDestroyIC, ime.imid, icid))
	default:
		ic := &xic{w: w, id: icid}
		if spot, ok := w.imeSpot(); ok {
			ic.spot, ic.spotSet = spot, true
		}
		if ime.ics == nil {
			ime.ics = map[uint16]*xic{}
		}
		ime.ics[icid] = ic
		w.ic = ic
		b.imeSyncWindow(w)
	}
	b.imeCreateNext()
}

// imeSpotAttr is the preeditAttributes value with the spot location.
func (b *Backend) imeSpotAttr(spot [2]int16) (xim.Attribute, bool) {
	ime := &b.ime
	pre, ok1 := ime.icAttrs[xim.AttrPreeditAttributes]
	sp, ok2 := ime.icAttrs[xim.AttrSpotLocation]
	if !ok1 || !ok2 {
		return xim.Attribute{}, false
	}
	return xim.Attribute{ID: pre, Value: xim.Nested(xim.Attribute{ID: sp, Value: xim.Point(spot[0], spot[1])})}, true
}

// imeSpot is the spot location of w's text field: the bottom-left of its
// cursor area in physical pixels of the window. ok is false when the area
// is unknown.
func (w *Window) imeSpot() ([2]int16, bool) {
	st := w.textInput
	if st == nil || st.CursorArea == (platform.Rect{}) {
		return [2]int16{}, false
	}
	s := w.b.scale
	clamp := func(v float64) int16 { return int16(max(math.MinInt16, min(math.MaxInt16, math.Round(v)))) }
	a := st.CursorArea
	return [2]int16{clamp(float64(a.X) * s), clamp(float64(a.Y+a.H) * s)}, true
}

// imeWindowClosed destroys the input context of a closing window.
func (b *Backend) imeWindowClosed(w *Window) {
	ime := &b.ime
	if ic := w.ic; ic != nil {
		w.ic = nil
		delete(ime.ics, ic.id)
		if ic.stopWait != nil {
			ic.stopWait()
		}
		for _, d := range ic.deferred {
			d.stop()
		}
		ic.deferred = nil
		b.imeSend(xim.EncodeIDs(xim.OpDestroyIC, ime.imid, ic.id))
	}
	if i := slices.Index(ime.pending, w); i > 0 || i == 0 && !ime.creating {
		ime.pending = slices.Delete(ime.pending, i, i+1)
	}
}

// shutdownIME says goodbye to the server when the backend closes.
func (b *Backend) shutdownIME() {
	ime := &b.ime
	if ime.state >= imeEncoding {
		b.imeSend(xim.EncodeIM(xim.OpClose, ime.imid))
	}
	if ime.state >= imeConnect {
		b.imeSend(xim.EncodeEmpty(xim.OpDisconnect))
	}
	b.imeDisarm()
	ime.state = imeOff
}

// --- sessions ---

// syncIME brings every window's input context in line with its keyboard
// focus and text input state (once per loop iteration, before and after
// the delivery pass).
func (b *Backend) syncIME() {
	for _, w := range b.loop.Windows() {
		b.imeSyncWindow(w)
		w.tiRestart, w.tiReset = false, false
	}
}

func (b *Backend) imeSyncWindow(w *Window) {
	ime := &b.ime
	if ime.state != imeReady || w.Closed() {
		return
	}
	st := w.textInput
	if w.ic == nil {
		if st != nil {
			b.imeRequestIC(w)
		}
		return
	}
	ic := w.ic
	want := w.focused && st != nil && st.Purpose != platform.TextPurposePassword
	if ic.focused && (!want || w.tiRestart) {
		ic.focused = false
		b.imeSend(xim.EncodeIDs(xim.OpUnsetICFocus, ime.imid, ic.id))
		b.imeEndSession(ic)
	}
	if !want {
		return
	}
	if spot, ok := w.imeSpot(); ok && (!ic.spotSet || spot != ic.spot) {
		if attr, ok := b.imeSpotAttr(spot); ok {
			b.imeSend(xim.EncodeSetICValues(ime.imid, ic.id, []xim.Attribute{attr}))
		}
		ic.spot, ic.spotSet = spot, true
	}
	switch {
	case !ic.focused:
		ic.focused = true
		ic.clearText()
		ic.commit, ic.dirty, ic.preedit, ic.cursor, ic.resetting = "", false, "", [2]int{}, false
		b.imeSend(xim.EncodeIDs(xim.OpSetICFocus, ime.imid, ic.id))
		w.setIMESession(true)
	case w.tiReset:
		ic.resetting = true
		ic.clearText()
		ic.commit, ic.dirty, ic.preedit, ic.cursor = "", false, "", [2]int{}
		b.imeSend(xim.EncodeIDs(xim.OpResetIC, ime.imid, ic.id))
	}
}

// imeEndSession ends ic's session: the composition gets nothing more, and
// keys held for a synchronous forward go to the keyboard.
func (b *Backend) imeEndSession(ic *xic) {
	ic.focused = false
	ic.clearText()
	ic.commit, ic.dirty, ic.preedit, ic.cursor, ic.resetting = "", false, "", [2]int{}, false
	if ic.stopWait != nil {
		ic.stopWait()
		ic.stopWait = nil
	}
	ic.waiting = false
	held := ic.held
	ic.held = nil
	for _, ev := range held {
		b.injectKey(ev)
	}
	b.imeFlushDeferred(ic)
	ic.w.setIMESession(false)
}

// imeFlushDeferred sends every release that waits for its press to the
// keyboard, in key code order (the session ends).
func (b *Backend) imeFlushDeferred(ic *xic) {
	for _, kc := range slices.Sorted(maps.Keys(ic.deferred)) {
		b.imeReleaseDeferred(ic, kc)
	}
	ic.unanswered = [256]bool{}
}

// setIMESession reports a session change.
func (w *Window) setIMESession(active bool) {
	if w.imeActive != active && !w.Closed() {
		w.imeActive = active
		w.b.enqueue(w, platform.IMESessionEvent{Active: active})
	}
}

// --- keys ---

// imeFilterKey forwards a key event of the focused window to the input
// method and reports whether it did; the keyboard then does not see it
// unless the server sends it back.
func (b *Backend) imeFilterKey(ev xgb.Event, win xproto.Window, kc byte, release bool) bool {
	ime := &b.ime
	if ime.state != imeReady {
		return false
	}
	w := b.windows[win]
	if w == nil || w != b.kbd.focus || w.ic == nil || !w.ic.focused || b.kbd.stale[kc] {
		return false
	}
	ic := w.ic
	if ic.waiting {
		// A synchronous forward is unanswered: every key waits behind it,
		// in order, and goes to the server or the keyboard when its turn
		// comes (imeSynced). A release outside the forward mask that went
		// to the keyboard now would come before its own press, still at
		// the server, which would send that back later: a KeyUp before
		// its KeyDown, or a KeyDown without one.
		ic.held = append(ic.held, ev)
		return true
	}
	if ime.forwardMask(ic)&keyMaskBit(release) == 0 {
		if !release || !ic.unanswered[kc] {
			return false
		}
		b.imeKeyboard(ic, ev)
		return true
	}
	b.imeForward(ic, ev)
	return true
}

// keyMaskBit is the event mask bit of a key press or release.
func keyMaskBit(release bool) uint32 {
	if release {
		return xproto.EventMaskKeyRelease
	}
	return xproto.EventMaskKeyPress
}

// forwardMask and syncMask are the event masks in effect for ic: its own,
// else the input method's, else every key (static event flow) unless the
// server registered trigger keys.
func (ime *imeClient) forwardMask(ic *xic) uint32 {
	switch {
	case ic.mask.set:
		return ic.mask.forward
	case ime.mask.set:
		return ime.mask.forward
	case ime.triggers:
		return 0
	}
	return xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease
}

func (ime *imeClient) syncMask(ic *xic) uint32 {
	switch {
	case ic.mask.set:
		return ic.mask.sync
	case ime.mask.set:
		return ime.mask.sync
	}
	return 0
}

// imeForward sends a key to the server.
func (b *Backend) imeForward(ic *xic, ev xgb.Event) {
	ime := &b.ime
	var wire []byte
	bit := uint32(xproto.EventMaskKeyPress)
	switch ev := ev.(type) {
	case xproto.KeyPressEvent:
		wire = ev.Bytes()
		le.PutUint16(wire[2:], ev.Sequence)
	case xproto.KeyReleaseEvent:
		wire = ev.Bytes()
		le.PutUint16(wire[2:], ev.Sequence)
		bit = xproto.EventMaskKeyRelease
	default:
		return
	}
	var flag uint16
	if ime.syncMask(ic)&bit != 0 {
		flag = xim.FlagSynchronous
		ic.waiting = true
		ic.stopWait = b.after(imeSyncTimeout, func() {
			ic.stopWait = nil
			if ic.waiting {
				b.imeSynced(ic)
			}
		})
	}
	if bit == xproto.EventMaskKeyPress && ime.forwardMask(ic)&xproto.EventMaskKeyRelease == 0 {
		// The server does not take this key's release (imeKeyboard).
		kc := wire[1]
		b.imeReleaseDeferred(ic, kc) // the previous press's
		if flag == 0 {
			ic.unanswered[kc] = true
		}
	}
	b.imeSend(xim.EncodeForwardEvent(ime.imid, ic.id, flag, 0, wire))
}

// imeKeyboard sends a key the server does not take to the keyboard: at
// once, except a release whose press went to the server asynchronously and
// has not come back. The keyboard must not see that release before the
// press, which the server may still send back (a KeyUp before its KeyDown,
// so a KeyDown without one); it waits for the press (imeForwarded) or for
// imeSyncTimeout, after which the press counts as consumed (the keyboard
// then drops the release of a key it never saw pressed). An asynchronous
// forward has no answer when the server consumes the key, and a
// synchronous flag the sync mask does not ask for gets none either
// (xcb-imdkit sends XIM_SYNC_REPLY only in its sync-event mode), so there
// is nothing better to wait for. fcitx5 and ibus take releases too
// (KeyPress and KeyRelease in their forward masks), so this is for the
// servers that ask for presses only.
func (b *Backend) imeKeyboard(ic *xic, ev xgb.Event) {
	r, ok := ev.(xproto.KeyReleaseEvent)
	if !ok || !ic.unanswered[r.Detail] {
		b.injectKey(ev)
		return
	}
	kc := byte(r.Detail)
	b.imeReleaseDeferred(ic, kc)
	if ic.deferred == nil {
		ic.deferred = map[byte]deferredRelease{}
	}
	ic.deferred[kc] = deferredRelease{ev: ev, stop: b.after(imeSyncTimeout, func() {
		if _, ok := ic.deferred[kc]; ok {
			delete(ic.deferred, kc)
			ic.unanswered[kc] = false
			b.injectKey(ev)
		}
	})}
}

// imeReleaseDeferred sends a release that waits for its press (imeKeyboard)
// to the keyboard now: the press came back, a new press of the key goes to
// the server, or the session ends.
func (b *Backend) imeReleaseDeferred(ic *xic, kc byte) {
	ic.unanswered[kc] = false
	d, ok := ic.deferred[kc]
	if !ok {
		return
	}
	delete(ic.deferred, kc)
	if d.stop != nil {
		d.stop()
	}
	b.injectKey(d.ev)
}

// imeSynced ends the wait of a synchronous forward: the held keys go to
// the server in order, until one waits again.
func (b *Backend) imeSynced(ic *xic) {
	ic.waiting = false
	if ic.stopWait != nil {
		ic.stopWait()
		ic.stopWait = nil
	}
	for len(ic.held) > 0 && !ic.waiting {
		ev := ic.held[0]
		ic.held = ic.held[1:]
		_, release := ev.(xproto.KeyReleaseEvent)
		switch {
		case !ic.focused || b.ime.state != imeReady || ic.w.ic != ic:
			b.injectKey(ev)
		case b.ime.forwardMask(ic)&keyMaskBit(release) != 0:
			b.imeForward(ic, ev)
		default:
			b.imeKeyboard(ic, ev)
		}
	}
}

// imeForwarded handles an event the server sends back: a key it did not
// consume (or one it makes up) goes through the keyboard, after the
// composition updates that came before it.
func (b *Backend) imeForwarded(f xim.ForwardEvent) {
	ime := &b.ime
	ic := ime.ics[f.ICID]
	var ev xgb.Event
	switch f.Event[0] & 0x7f {
	case xproto.KeyPress:
		k := xproto.KeyPressEventNew(f.Event[:]).(xproto.KeyPressEvent)
		if k.Event == 0 && ic != nil {
			k.Event = ic.w.id
		}
		ev = k
	case xproto.KeyRelease:
		k := xproto.KeyReleaseEventNew(f.Event[:]).(xproto.KeyReleaseEvent)
		if k.Event == 0 && ic != nil {
			k.Event = ic.w.id
		}
		ev = k
	}
	if ev != nil && ic != nil {
		b.imeFlush(ic)
		b.injectKey(ev)
		if k, ok := ev.(xproto.KeyPressEvent); ok {
			b.imeReleaseDeferred(ic, byte(k.Detail)) // its release, which waited
		}
	}
	if f.Flag&xim.FlagSynchronous != 0 {
		b.imeSend(xim.EncodeIDs(xim.OpSyncReply, f.IMID, f.ICID))
	}
}

// injectKey runs a key event through the keyboard as if the X server had
// sent it (a key the input method sends back, or one held for it).
func (b *Backend) injectKey(ev xgb.Event) {
	if !b.kbd.beforeEvent(ev) {
		return
	}
	switch ev := ev.(type) {
	case xproto.KeyPressEvent:
		b.kbd.keyPress(ev)
	case xproto.KeyReleaseEvent:
		b.kbd.keyRelease(ev)
	}
}

// --- composition ---

// imeText decodes a string of the server in the negotiated encoding.
func (b *Backend) imeText(s []byte) string {
	if b.ime.utf8 {
		return strings.ToValidUTF8(string(s), "�")
	}
	return xim.DecodeCompoundText(s)
}

// imeCommit latches committed text.
func (b *Backend) imeCommit(c xim.Commit) {
	ic := b.ime.ics[c.ICID]
	if ic != nil && ic.focused && !ic.resetting {
		var text string
		switch {
		case c.Flag&xim.CommitChars != 0:
			text = b.imeText(c.Text)
		case c.Flag&xim.CommitKeySym != 0:
			text = keysymChar(c.KeySym)
		}
		if text = platform.SanitizeIMEText(text); text != "" {
			ic.commit += text
			ic.dirty = true
			if b.kbd.compose != nil {
				b.kbd.compose.Reset() // the input method composed instead
			}
		}
	}
	if c.Flag&xim.FlagSynchronous != 0 {
		b.imeSend(xim.EncodeIDs(xim.OpSyncReply, c.IMID, c.ICID))
	}
}

// keysymChar is the character of a Latin-1 or Unicode keysym, "" for
// others (function keys, which a commit cannot type).
func keysymChar(ks uint32) string {
	switch {
	case ks >= 0x20 && ks <= 0x7e || ks >= 0xa0 && ks <= 0xff:
		return string(rune(ks))
	case ks&0xff000000 == 0x01000000:
		if r := rune(ks & 0xffffff); r >= 0x20 && utf8.ValidRune(r) {
			return string(r)
		}
	}
	return ""
}

// clearText forgets the server's preedit (start, done, reset, session).
func (ic *xic) clearText() {
	if len(ic.text) > 0 || ic.caret != 0 || ic.hidden {
		ic.dirty = true
	}
	ic.text, ic.fb, ic.caret, ic.hidden = nil, nil, 0, false
}

// draw applies XIM_PREEDIT_DRAW: ChgLength characters at ChgFirst become
// text, clamped to the preedit.
func (ic *xic) draw(d xim.PreeditDraw, text string) {
	var runes []rune
	for _, r := range text {
		if r != 0 {
			runes = append(runes, r)
		}
	}
	n := len(ic.text)
	first := int(max(0, min(int64(d.ChgFirst), int64(n))))
	count := int(max(0, min(int64(d.ChgLength), int64(n-first))))
	fb := make([]uint32, len(runes))
	copy(fb, d.Feedback)
	ic.text = slices.Concat(ic.text[:first], runes, ic.text[first+count:])
	ic.fb = slices.Concat(ic.fb[:first], fb, ic.fb[first+count:])
	ic.caret = int(max(0, min(int64(d.Caret), int64(len(ic.text)))))
	ic.dirty = true
}

// moveCaret applies XIM_PREEDIT_CARET and returns the new position. Word
// and line moves are approximated: a preedit has one line.
func (ic *xic) moveCaret(c xim.PreeditCaret) int {
	n := len(ic.text)
	switch c.Direction {
	case xim.CaretForwardChar, xim.CaretForwardWord:
		ic.caret++
	case xim.CaretBackwardChar, xim.CaretBackwardWord:
		ic.caret--
	case xim.CaretLineStart:
		ic.caret = 0
	case xim.CaretLineEnd:
		ic.caret = n
	case xim.CaretAbsolutePosition:
		ic.caret = int(c.Position)
	}
	ic.caret = max(0, min(ic.caret, n))
	ic.hidden = c.Style == xim.CaretInvisible
	ic.dirty = true
	return ic.caret
}

// cursorBytes is the IMEEvent cursor of the preedit: the first run of
// reverse or highlighted characters, else the caret, in bytes of pre;
// (-1, -1) when the caret is hidden.
func (ic *xic) cursorBytes(pre string) (int, int) {
	offs := make([]int, len(ic.text)+1)
	for i, r := range ic.text {
		offs[i+1] = offs[i] + utf8.RuneLen(r)
	}
	const marked = xim.FeedbackReverse | xim.FeedbackHighlight
	if i := slices.IndexFunc(ic.fb, func(f uint32) bool { return f&marked != 0 }); i >= 0 {
		j := i
		for j < len(ic.fb) && ic.fb[j]&marked != 0 {
			j++
		}
		return platform.SanitizePreeditCursor(pre, offs[i], offs[j])
	}
	if ic.hidden {
		return platform.SanitizePreeditCursor(pre, -1, -1)
	}
	return platform.SanitizePreeditCursor(pre, offs[ic.caret], offs[ic.caret])
}

// imeFlush delivers ic's latched commits and preedit as one IMEEvent, if
// they change anything.
func (b *Backend) imeFlush(ic *xic) {
	if !ic.dirty {
		return
	}
	ic.dirty = false
	commit := ic.commit
	ic.commit = ""
	if !ic.focused || ic.w.Closed() {
		return
	}
	var pre string
	cursor := [2]int{}
	if !ic.resetting {
		pre = platform.SanitizeIMEText(string(ic.text))
		cursor[0], cursor[1] = ic.cursorBytes(pre)
	}
	if commit == "" && pre == ic.preedit && cursor == ic.cursor {
		return
	}
	ic.preedit, ic.cursor = pre, cursor
	b.enqueue(ic.w, platform.IMEEvent{Commit: commit, Preedit: pre, CursorStart: cursor[0], CursorEnd: cursor[1]})
}

// flushIME delivers what every input context latched in this read.
func (b *Backend) flushIME() {
	if b.ime.state != imeReady {
		return
	}
	for _, w := range b.loop.Windows() {
		if w.ic != nil {
			b.imeFlush(w.ic)
		}
	}
}

// --- Window side ---

// SetTextInput records the focused field's state; the input context
// follows it at the end of the loop iteration (syncIME). See
// platform.Window and the notes at the top of this file.
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

// ResetTextInput makes the next sync send XIM_RESET_IC, which tells the
// input method to drop its composition.
func (w *Window) ResetTextInput() {
	if w.Closed() || w.textInput == nil {
		return
	}
	w.tiReset = true
}
