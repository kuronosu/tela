//go:build linux

package x11

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Selections (ICCCM 2): the CLIPBOARD selection of the clipboard and the
// XdndSelection of drag and drop, both ways.
//
// Owning: the backend owns a selection with its hidden selection window
// (or, for XdndSelection, the drag's source window) and answers every
// SelectionRequest from the event loop: TARGETS (the targets of every
// Data entry, SelectionTargets' aliases included), TIMESTAMP, MULTIPLE
// (ATOM_PAIR lists), and each target, converted (STRING as Latin-1,
// x-special/gnome-copied-files from the URI list). Data larger than
// incrThreshold goes with INCR (ICCCM 2.7.2): the INCR property, then a
// chunk at every PropertyNotify Delete of the requestor's property, ended
// by an empty one. SelectionClear ends the ownership.
//
// Reading: conversions run one at a time (a FIFO), each into the
// TELA_SELECTION property of the selection window, with a timeout;
// SelectionNotify with property None means the owner refused (the target
// is not offered). An INCR answer is read chunk by chunk at each
// PropertyNotify NewValue on the selection window until an empty chunk.
// Every answer reaches the app through the loop (Post).
//
// Like GTK and winit, the backend does not hand its clipboard to a
// clipboard manager (SAVE_TARGETS) when Run returns: the content goes
// away with the app.

const (
	// incrThreshold is the largest property the backend writes in one
	// piece; larger data goes with INCR. It stays below the 256 KiB of
	// the core protocol's request limit, which every requestor can read
	// (ICCCM: the owner uses INCR above the maximum request size).
	incrThreshold = 256*1024 - 1024
	// incrChunk is the size of one INCR chunk.
	incrChunk = 64 * 1024
	// selectionTimeout bounds a conversion: an owner that does not answer
	// (or stops in the middle of an INCR transfer) within it fails the
	// read; the contract's "a few seconds" for drops.
	selectionTimeout = 3 * time.Second
	// incrServeTimeout bounds how long the backend waits for a requestor
	// to delete an INCR chunk before dropping the transfer.
	incrServeTimeout = 10 * time.Second
)

// errSelectionTimeout is reported when the owner of a selection does not
// answer in time.
var errSelectionTimeout = errors.New("x11: the selection owner did not answer in time")

// ownedSelection is a selection the backend owns.
type ownedSelection struct {
	owner  xproto.Window
	time   uint32
	data   platform.Data
	offers []offer
}

// offer is one target the backend serves for a Data entry.
type offer struct {
	target string // the target name ("UTF8_STRING", "text/uri-list")
	mime   string // the Data type it is converted from
}

// offers returns the targets the backend serves for d, in order: each
// entry's SelectionTargets, each target once (the first entry offering it
// wins).
func offers(d platform.Data) []offer {
	var out []offer
	for _, e := range d {
		for _, t := range platform.SelectionTargets(e.Type) {
			if !slices.ContainsFunc(out, func(o offer) bool { return o.target == t }) {
				out = append(out, offer{target: t, mime: e.Type})
			}
		}
	}
	return out
}

// convertOffer returns the bytes of target o of d and the name of the
// property type they are written with: STRING is Latin-1, UTF8_STRING and
// TEXT are UTF8_STRING, gnome-copied-files is built from the URI list;
// every other target is its own type with the entry's bytes.
func convertOffer(d platform.Data, o offer) ([]byte, string) {
	src, _ := d.Get(o.mime)
	switch o.target {
	case "STRING":
		return latin1(strings.ToValidUTF8(string(src), "?")), "STRING"
	case "UTF8_STRING", "TEXT":
		return src, "UTF8_STRING"
	case "x-special/gnome-copied-files":
		return platform.GnomeCopiedFiles(platform.ParseURIList(src), false), o.target
	}
	return src, o.target
}

// normalizeTargets maps target names to the Data types they deliver
// (platform.SelectionType), each type once at the position of its first
// target, meta targets and non-MIME names left out.
func normalizeTargets(targets []string) []string {
	var out []string
	for _, t := range targets {
		if m, ok := platform.SelectionType(t); ok && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// pickTarget returns the offered target to read the Data type mime from:
// the first of SelectionTargets(mime) offered (the best text target,
// text/uri-list before gnome-copied-files), else the first offered
// target that SelectionType maps to mime (COMPOUND_TEXT, a "text/html;
// charset=utf-8" spelling).
func pickTarget(offered []string, mime string) (string, bool) {
	for _, t := range platform.SelectionTargets(mime) {
		if slices.Contains(offered, t) {
			return t, true
		}
	}
	for _, t := range offered {
		if m, ok := platform.SelectionType(t); ok && m == mime {
			return t, true
		}
	}
	return "", false
}

// decodeTarget turns the bytes read from target (a property of type
// typeName) into the Data type mime: text in STRING or COMPOUND_TEXT is
// Latin-1 (COMPOUND_TEXT's ASCII part; its ISO 2022 escapes are not
// decoded), gnome-copied-files becomes a URI list.
func decodeTarget(mime, target, typeName string, data []byte) []byte {
	switch {
	case mime == platform.MIMEText && (typeName == "STRING" || typeName == "COMPOUND_TEXT" ||
		target == "STRING" && typeName != "UTF8_STRING"):
		return fromLatin1(data)
	case mime == platform.MIMEURIList && target == "x-special/gnome-copied-files":
		if uris, _, ok := platform.ParseGnomeCopiedFiles(data); ok {
			return platform.URIList(uris)
		}
		return nil
	}
	return data
}

// fromLatin1 converts ISO 8859-1 bytes to UTF-8.
func fromLatin1(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		out = utf8.AppendRune(out, rune(c))
	}
	return out
}

// --- the selection window ---

// selectionWindow returns the hidden window that owns the CLIPBOARD and
// receives conversions, creating it on first use (0 if that fails).
func (b *Backend) selectionWindow() xproto.Window {
	if b.integ.selWin == 0 {
		w, err := b.xs.createHidden()
		if err != nil {
			return 0
		}
		b.integ.selWin = w
	}
	return b.integ.selWin
}

// --- owning ---

// own makes win the owner of sel with data, at the time of the latest
// user input (ICCCM: not CurrentTime when a timestamp is known). It
// checks the outcome with GetSelectionOwner.
func (b *Backend) own(sel xproto.Atom, win xproto.Window, data platform.Data) error {
	if win == 0 {
		return errors.New("x11: cannot create the selection window")
	}
	t := b.userTime
	b.xs.setOwner(win, sel, t)
	if b.xs.owner(sel) != win {
		delete(b.integ.owned, sel)
		return fmt.Errorf("x11: could not own the %s selection", b.atomName(sel))
	}
	if b.integ.owned == nil {
		b.integ.owned = map[xproto.Atom]*ownedSelection{}
	}
	b.integ.owned[sel] = &ownedSelection{owner: win, time: t, data: data, offers: offers(data)}
	return nil
}

// disown gives up sel if the backend owns it.
func (b *Backend) disown(sel xproto.Atom) {
	o := b.integ.owned[sel]
	if o == nil {
		return
	}
	delete(b.integ.owned, sel)
	b.xs.setOwner(0, sel, o.time)
}

// selectionClear: another client took a selection the backend owned.
func (b *Backend) selectionClear(ev xproto.SelectionClearEvent) {
	if o := b.integ.owned[ev.Selection]; o != nil && o.owner == ev.Owner {
		delete(b.integ.owned, ev.Selection)
	}
}

// selectionRequest answers a conversion of a selection the backend owns.
func (b *Backend) selectionRequest(ev xproto.SelectionRequestEvent) {
	o := b.integ.owned[ev.Selection]
	prop := ev.Property
	if prop == 0 {
		prop = ev.Target // an obsolete client (ICCCM 2.2)
	}
	ok := false
	switch {
	case o == nil || o.owner != ev.Owner:
	case ev.Time != 0 && ev.Time < xproto.Timestamp(o.time):
		// A request for an older ownership (ICCCM 2.2).
	case ev.Target == b.atoms.multiple:
		ok = b.serveMultiple(o, ev.Requestor, prop)
	default:
		ok = b.serveTarget(o, ev.Requestor, ev.Target, prop)
	}
	if !ok {
		prop = 0
	}
	n := xproto.SelectionNotifyEvent{Time: ev.Time, Requestor: ev.Requestor, Selection: ev.Selection, Target: ev.Target, Property: prop}
	b.xs.sendEvent(ev.Requestor, 0, n.Bytes())
}

// serveMultiple answers MULTIPLE: the requestor's property holds
// (target, property) ATOM_PAIRs; each is converted, and a refused one has
// its property replaced by None in the list written back.
func (b *Backend) serveMultiple(o *ownedSelection, req xproto.Window, prop xproto.Atom) bool {
	p, ok := b.xs.property(req, prop, false)
	if !ok || p.format != 32 {
		return false
	}
	pairs := make([]uint32, len(p.data)/4)
	for i := range pairs {
		pairs[i] = le.Uint32(p.data[4*i:])
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		target, tprop := xproto.Atom(pairs[i]), xproto.Atom(pairs[i+1])
		if tprop == 0 || target == b.atoms.multiple || !b.serveTarget(o, req, target, tprop) {
			pairs[i+1] = 0
		}
	}
	b.xs.changeProperty(req, prop, b.atoms.atomPair, 32, card32s(pairs...))
	return true
}

// serveTarget writes target of o to the requestor's property: TARGETS,
// TIMESTAMP or a data target (INCR when large). It reports false for a
// target the backend does not offer.
func (b *Backend) serveTarget(o *ownedSelection, req xproto.Window, target, prop xproto.Atom) bool {
	switch target {
	case b.atoms.targets:
		list := []uint32{uint32(b.atoms.targets), uint32(b.atoms.multiple), uint32(b.atoms.timestamp)}
		for _, of := range o.offers {
			if a := b.atom(of.target); a != 0 {
				list = append(list, uint32(a))
			}
		}
		b.xs.changeProperty(req, prop, xproto.AtomAtom, 32, card32s(list...))
		return true
	case b.atoms.timestamp:
		b.xs.changeProperty(req, prop, xproto.AtomInteger, 32, card32s(o.time))
		return true
	}
	name := b.atomName(target)
	i := slices.IndexFunc(o.offers, func(of offer) bool { return of.target == name })
	if i < 0 {
		return false
	}
	data, typName := convertOffer(o.data, o.offers[i])
	typ := b.atom(typName)
	if typ == 0 {
		return false
	}
	if len(data) > incrThreshold {
		b.startIncr(req, prop, typ, data)
		return true
	}
	b.xs.changeProperty(req, prop, typ, 8, data)
	return true
}

// incrOut is an INCR transfer the backend serves.
type incrOut struct {
	req  xproto.Window
	prop xproto.Atom
	typ  xproto.Atom
	data []byte // what is left to send
	stop func()
}

// startIncr starts serving data with INCR: PropertyChange on the
// requestor's window, then the INCR property with a lower bound of the
// size; the chunks follow its deletes.
func (b *Backend) startIncr(req xproto.Window, prop, typ xproto.Atom, data []byte) {
	b.dropIncr(req, prop) // a new request for the same property replaces it
	if !b.ownWindow(req) {
		b.xs.selectInput(req, xproto.EventMaskPropertyChange)
	}
	t := &incrOut{req: req, prop: prop, typ: typ, data: data}
	b.integ.incrOut = append(b.integ.incrOut, t)
	b.xs.changeProperty(req, prop, b.atoms.incr, 32, card32s(uint32(len(data))))
	b.armIncr(t)
}

// armIncr (re)starts the timeout of an INCR transfer.
func (b *Backend) armIncr(t *incrOut) {
	if t.stop != nil {
		t.stop()
	}
	t.stop = b.after(incrServeTimeout, func() { b.dropIncr(t.req, t.prop) })
}

// ownWindow reports whether win is a window of this backend, whose event
// mask must not be replaced.
func (b *Backend) ownWindow(win xproto.Window) bool {
	return win == b.integ.selWin || b.windows[win] != nil
}

// dropIncr forgets the INCR transfer to (req, prop), if any.
func (b *Backend) dropIncr(req xproto.Window, prop xproto.Atom) {
	i := slices.IndexFunc(b.integ.incrOut, func(t *incrOut) bool { return t.req == req && t.prop == prop })
	if i < 0 {
		return
	}
	t := b.integ.incrOut[i]
	if t.stop != nil {
		t.stop()
	}
	b.integ.incrOut = slices.Delete(b.integ.incrOut, i, i+1)
	if !b.ownWindow(req) && !slices.ContainsFunc(b.integ.incrOut, func(o *incrOut) bool { return o.req == req }) {
		b.xs.selectInput(req, 0)
	}
}

// incrDeleted sends the next chunk of a transfer whose property the
// requestor deleted; it reports whether the event belonged to one.
func (b *Backend) incrDeleted(win xproto.Window, prop xproto.Atom) bool {
	i := slices.IndexFunc(b.integ.incrOut, func(t *incrOut) bool { return t.req == win && t.prop == prop })
	if i < 0 {
		return false
	}
	t := b.integ.incrOut[i]
	n := min(len(t.data), incrChunk)
	b.xs.changeProperty(win, prop, t.typ, 8, t.data[:n])
	t.data = t.data[n:]
	if n == 0 {
		// The empty chunk ends the transfer; the requestor deletes it
		// too, but nothing more is sent.
		b.dropIncr(win, prop)
		return true
	}
	b.armIncr(t)
	return true
}

// --- reading ---

// conversion is a selection read: one ConvertSelection into the
// TELA_SELECTION property of the selection window.
type conversion struct {
	sel, target xproto.Atom
	time        uint32
	// done receives the property's type and bytes, ok false when the
	// owner refused (or there is no owner), or an error (timeout, no
	// selection window). It runs from event handling, never inside the
	// request.
	done func(typ xproto.Atom, data []byte, ok bool, err error)

	started bool
	incr    bool
	typ     xproto.Atom
	buf     []byte
	stop    func()
}

// convertSelection queues a read of target from sel.
func (b *Backend) convertSelection(sel, target xproto.Atom, t uint32, done func(typ xproto.Atom, data []byte, ok bool, err error)) {
	b.integ.reads = append(b.integ.reads, &conversion{sel: sel, target: target, time: t, done: done})
	b.nextConversion()
}

// nextConversion starts the conversion at the head of the queue.
func (b *Backend) nextConversion() {
	for len(b.integ.reads) > 0 {
		c := b.integ.reads[0]
		if c.started {
			return
		}
		win := b.selectionWindow()
		if win == 0 || c.target == 0 {
			b.integ.reads = b.integ.reads[1:]
			c.done(0, nil, false, errors.New("x11: cannot read the selection (no selection window or target atom)"))
			continue
		}
		c.started = true
		b.xs.deleteProperty(win, b.atoms.telaSelection)
		b.xs.convert(win, c.sel, c.target, b.atoms.telaSelection, c.time)
		c.stop = b.after(selectionTimeout, func() { b.finishConversion(c, 0, nil, false, errSelectionTimeout) })
		return
	}
}

// finishConversion ends c (the head of the queue) and starts the next.
func (b *Backend) finishConversion(c *conversion, typ xproto.Atom, data []byte, ok bool, err error) {
	if len(b.integ.reads) == 0 || b.integ.reads[0] != c {
		return
	}
	b.integ.reads = b.integ.reads[1:]
	if c.stop != nil {
		c.stop()
	}
	c.done(typ, data, ok, err)
	b.nextConversion()
}

// selectionNotify is the owner's answer to the conversion in progress.
func (b *Backend) selectionNotify(ev xproto.SelectionNotifyEvent) {
	if ev.Requestor != b.integ.selWin || len(b.integ.reads) == 0 {
		return
	}
	c := b.integ.reads[0]
	if !c.started || c.incr || ev.Selection != c.sel || ev.Target != c.target {
		return // a late answer to a conversion that timed out
	}
	if ev.Property == 0 {
		b.finishConversion(c, 0, nil, false, nil)
		return
	}
	p, ok := b.xs.property(b.integ.selWin, ev.Property, true)
	if !ok {
		b.finishConversion(c, 0, nil, false, nil)
		return
	}
	if p.typ == b.atoms.incr {
		// Deleting the INCR property (done by the read) asks for the
		// first chunk; each chunk arrives as a PropertyNotify NewValue.
		c.incr = true
		c.stop()
		c.stop = b.after(selectionTimeout, func() { b.finishConversion(c, 0, nil, false, errSelectionTimeout) })
		return
	}
	b.finishConversion(c, p.typ, p.data, true, nil)
}

// incrNewValue reads the next chunk of an INCR read in progress; it
// reports whether the event belonged to it.
func (b *Backend) incrNewValue(prop xproto.Atom) bool {
	if len(b.integ.reads) == 0 || prop != b.atoms.telaSelection {
		return false
	}
	c := b.integ.reads[0]
	if !c.incr {
		return false
	}
	p, ok := b.xs.property(b.integ.selWin, prop, true)
	if !ok {
		return true
	}
	if len(p.data) == 0 {
		b.finishConversion(c, c.typ, c.buf, true, nil)
		return true
	}
	c.typ = p.typ
	c.buf = append(c.buf, p.data...)
	c.stop()
	c.stop = b.after(selectionTimeout, func() { b.finishConversion(c, 0, nil, false, errSelectionTimeout) })
	return true
}

// readTargets reads the TARGETS of sel and passes their names on.
func (b *Backend) readTargets(sel xproto.Atom, t uint32, done func(names []string, err error)) {
	b.convertSelection(sel, b.atoms.targets, t, func(typ xproto.Atom, data []byte, ok bool, err error) {
		if err != nil || !ok {
			done(nil, err)
			return
		}
		atoms := make([]xproto.Atom, len(data)/4)
		for i := range atoms {
			atoms[i] = xproto.Atom(le.Uint32(data[4*i:]))
		}
		done(b.atomNamesOf(atoms), nil)
	})
}

// readType reads the Data type mime of sel, whose targets are offered:
// the best target for it, decoded. ok is false when no target delivers
// it or the owner refused.
func (b *Backend) readType(sel xproto.Atom, t uint32, offered []string, mime string, done func(data []byte, ok bool, err error)) {
	target, ok := pickTarget(offered, mime)
	if !ok {
		done(nil, false, nil)
		return
	}
	b.convertSelection(sel, b.atom(target), t, func(typ xproto.Atom, data []byte, ok bool, err error) {
		if err != nil || !ok {
			done(nil, false, err)
			return
		}
		done(decodeTarget(mime, target, b.atomName(typ), data), true, nil)
	})
}
