//go:build linux

package x11

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

func TestOffersAndConversion(t *testing.T) {
	uris := platform.URIList([]string{"file:///tmp/a%20b", "file:///tmp/c"})
	d := platform.Data{
		{Type: platform.MIMEHTML, Bytes: []byte("<b>é</b>")},
		{Type: platform.MIMEText, Bytes: []byte("é€")},
		{Type: platform.MIMEURIList, Bytes: uris},
		{Type: "application/x-tela", Bytes: []byte{1, 2}},
	}
	var names []string
	for _, o := range offers(d) {
		names = append(names, o.target)
	}
	want := []string{"text/html", "text/plain;charset=utf-8", "UTF8_STRING", "text/plain", "TEXT", "STRING",
		"text/uri-list", "x-special/gnome-copied-files", "application/x-tela"}
	if !slices.Equal(names, want) {
		t.Errorf("offers = %q, want %q", names, want)
	}
	for _, c := range []struct {
		target, wantType string
		want             []byte
	}{
		{"STRING", "STRING", []byte("\xe9?")}, // Latin-1, € is not in it
		{"UTF8_STRING", "UTF8_STRING", []byte("é€")},
		{"TEXT", "UTF8_STRING", []byte("é€")},
		{"text/plain", "text/plain", []byte("é€")},
		{"x-special/gnome-copied-files", "x-special/gnome-copied-files", []byte("copy\nfile:///tmp/a%20b\nfile:///tmp/c")},
		{"text/uri-list", "text/uri-list", uris},
		{"application/x-tela", "application/x-tela", []byte{1, 2}},
	} {
		i := slices.IndexFunc(offers(d), func(o offer) bool { return o.target == c.target })
		got, typ := convertOffer(d, offers(d)[i])
		if typ != c.wantType || !bytes.Equal(got, c.want) {
			t.Errorf("convert %s = %q (%s), want %q (%s)", c.target, got, typ, c.want, c.wantType)
		}
	}
}

func TestTargetsNormalizeAndPick(t *testing.T) {
	offered := []string{"TARGETS", "MULTIPLE", "TIMESTAMP", "text/html; charset=utf-8", "STRING", "UTF8_STRING",
		"x-special/gnome-copied-files", "text/uri-list", "SAVE_TARGETS", "image/png", "COMPOUND_TEXT"}
	got := normalizeTargets(offered)
	want := []string{"text/html;charset=utf-8", platform.MIMEText, platform.MIMEURIList, "image/png"}
	if !slices.Equal(got, want) {
		t.Errorf("normalizeTargets = %q, want %q", got, want)
	}
	for _, c := range []struct{ mime, want string }{
		{platform.MIMEText, "UTF8_STRING"},
		{platform.MIMEURIList, "text/uri-list"},
		{"text/html;charset=utf-8", "text/html; charset=utf-8"},
		{"image/png", "image/png"},
		{"image/jpeg", ""},
	} {
		if got, _ := pickTarget(offered, c.mime); got != c.want {
			t.Errorf("pickTarget(%s) = %q, want %q", c.mime, got, c.want)
		}
	}
	if got, _ := pickTarget([]string{"COMPOUND_TEXT", "STRING"}, platform.MIMEText); got != "STRING" {
		t.Errorf("pickTarget prefers %q over STRING", got)
	}
	if got := decodeTarget(platform.MIMEText, "STRING", "STRING", []byte("caf\xe9")); string(got) != "café" {
		t.Errorf("Latin-1 decode = %q", got)
	}
	if got := decodeTarget(platform.MIMEURIList, "x-special/gnome-copied-files", "x-special/gnome-copied-files",
		[]byte("cut\nfile:///a\nfile:///b")); string(got) != "file:///a\r\nfile:///b\r\n" {
		t.Errorf("gnome-copied-files decode = %q", got)
	}
}

// request is a SelectionRequest of the CLIPBOARD from window 500.
func (g *dataRig) request(target, prop xproto.Atom) xproto.SelectionRequestEvent {
	return xproto.SelectionRequestEvent{Owner: fakeSelWin, Requestor: 500, Selection: g.b.atoms.clipboard,
		Target: target, Property: prop, Time: 10}
}

func TestClipboardServe(t *testing.T) {
	g := newDataRig()
	g.b.userTime = 5
	if err := g.b.Clipboard().Write(platform.Data{
		{Type: platform.MIMEText, Bytes: []byte("hé")},
		{Type: "application/x-tela", Bytes: []byte("custom")},
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := g.x.take(), "create selection window; own CLIPBOARD win=900"; got != want {
		t.Errorf("Write sent %q, want %q", got, want)
	}
	p := g.x.id("P")
	steps := []struct {
		target string
		prop   xproto.Atom
		want   string
	}{
		{"TARGETS", p, "prop P win=500 ATOM = [TARGETS MULTIPLE TIMESTAMP text/plain;charset=utf-8 UTF8_STRING text/plain TEXT STRING application/x-tela]; notify win=500 CLIPBOARD TARGETS prop=P"},
		{"TIMESTAMP", p, "prop P win=500 INTEGER = [5]; notify win=500 CLIPBOARD TIMESTAMP prop=P"},
		{"UTF8_STRING", p, `prop P win=500 UTF8_STRING = "hé"; notify win=500 CLIPBOARD UTF8_STRING prop=P`},
		{"STRING", p, `prop P win=500 STRING = "h\xe9"; notify win=500 CLIPBOARD STRING prop=P`},
		{"application/x-tela", p, `prop P win=500 application/x-tela = "custom"; notify win=500 CLIPBOARD application/x-tela prop=P`},
		{"image/png", p, "notify win=500 CLIPBOARD image/png prop=None"},
		// An obsolete client without a property: the target is the property.
		{"UTF8_STRING", 0, `prop UTF8_STRING win=500 UTF8_STRING = "hé"; notify win=500 CLIPBOARD UTF8_STRING prop=UTF8_STRING`},
	}
	for _, s := range steps {
		g.feed(g.request(g.x.id(s.target), s.prop))
		if got := g.x.take(); got != s.want {
			t.Errorf("request %s:\n got %s\nwant %s", s.target, got, s.want)
		}
	}

	// MULTIPLE: a refused pair gets None.
	pair := g.x.id("PAIRS")
	g.x.setProp(500, pair, g.b.atoms.atomPair, 32, atomList(g.x.id("UTF8_STRING"), g.x.id("A"), g.x.id("image/png"), g.x.id("B")))
	g.feed(g.request(g.b.atoms.multiple, pair))
	if got, want := g.x.take(), `prop A win=500 UTF8_STRING = "hé"; prop PAIRS win=500 ATOM_PAIR = [UTF8_STRING A image/png None]; notify win=500 CLIPBOARD MULTIPLE prop=PAIRS`; got != want {
		t.Errorf("MULTIPLE:\n got %s\nwant %s", got, want)
	}

	// A request for an older ownership, or to another owner, is refused.
	old := g.request(g.x.id("UTF8_STRING"), p)
	old.Time = 4
	g.feed(old)
	if got, want := g.x.take(), "notify win=500 CLIPBOARD UTF8_STRING prop=None"; got != want {
		t.Errorf("old request: %s", got)
	}

	// Losing the selection: reads go through the server again.
	g.feed(xproto.SelectionClearEvent{Owner: fakeSelWin, Selection: g.b.atoms.clipboard})
	g.feed(g.request(g.x.id("UTF8_STRING"), p))
	if got, want := g.x.take(), "notify win=500 CLIPBOARD UTF8_STRING prop=None"; got != want {
		t.Errorf("after SelectionClear: %s", got)
	}
}

func TestClipboardServeIncr(t *testing.T) {
	g := newDataRig()
	big := bytes.Repeat([]byte("0123456789abcdef"), (incrThreshold+incrChunk)/16+1) // two chunks and a bit
	if err := g.b.Clipboard().Write(platform.Data{{Type: "application/x-big", Bytes: big}}); err != nil {
		t.Fatal(err)
	}
	g.x.take()
	p := g.x.id("P")
	g.feed(g.request(g.x.id("application/x-big"), p))
	want := fmt.Sprintf("select win=500 mask=%#x; prop P win=500 INCR = [%d]; notify win=500 CLIPBOARD application/x-big prop=P",
		xproto.EventMaskPropertyChange, len(big))
	if got := g.x.take(); got != want {
		t.Fatalf("INCR start:\n got %s\nwant %s", got, want)
	}
	var got []byte
	for i := 0; i < 100; i++ {
		g.feed(propNotify(500, p, true))
		log := g.x.take()
		v := g.x.props[500][p]
		got = append(got, v.data...)
		if len(v.data) == 0 {
			if !strings.Contains(log, `= ""`) || !strings.HasSuffix(log, "select win=500 mask=0x0") {
				t.Errorf("end of INCR: %s", log)
			}
			break
		}
		if len(v.data) > incrChunk {
			t.Fatalf("chunk of %d bytes", len(v.data))
		}
	}
	if !bytes.Equal(got, big) {
		t.Errorf("INCR delivered %d bytes, want %d", len(got), len(big))
	}
	g.feed(propNotify(500, p, true)) // the requestor deletes the empty chunk
	if s := g.x.take(); s != "" {
		t.Errorf("after the end: %s", s)
	}

	// A requestor that stops deleting is dropped at the timeout.
	g.feed(g.request(g.x.id("application/x-big"), p))
	g.x.take()
	if g.x.fire(incrServeTimeout) == 0 || len(g.b.integ.incrOut) != 0 {
		t.Errorf("the INCR timeout did not drop the transfer")
	}
}

// answer makes the owner (window 600) answer the conversion in progress:
// the property, then SelectionNotify.
func (g *dataRig) answer(sel, target, typ xproto.Atom, format byte, data []byte) string {
	g.x.setProp(fakeSelWin, g.b.atoms.telaSelection, typ, format, data)
	return g.feed(propNotify(fakeSelWin, g.b.atoms.telaSelection, false),
		xproto.SelectionNotifyEvent{Requestor: fakeSelWin, Selection: sel, Target: target, Property: g.b.atoms.telaSelection})
}

func TestClipboardRead(t *testing.T) {
	g := newDataRig()
	cb := g.b.Clipboard()
	clip := g.b.atoms.clipboard
	var text string
	var textErr error
	textDone := false
	cb.ReadText(func(s string, err error) { text, textErr, textDone = s, err, true })
	if textDone {
		t.Fatal("ReadText answered synchronously")
	}
	if got, want := g.x.take(), "create selection window; convert CLIPBOARD TARGETS to TELA_SELECTION t=0"; got != want {
		t.Errorf("ReadText sent %q", got)
	}
	g.answer(clip, g.b.atoms.targets, xproto.AtomAtom, 32, atomList(g.b.atoms.targets, xproto.AtomString, g.x.id("UTF8_STRING"), g.x.id("text/html")))
	if got, want := g.x.take(), "read+delete TELA_SELECTION win=900; convert CLIPBOARD UTF8_STRING to TELA_SELECTION t=0"; got != want {
		t.Errorf("after TARGETS: %q", got)
	}
	g.answer(clip, g.x.id("UTF8_STRING"), g.x.id("UTF8_STRING"), 8, []byte("héllo"))
	g.pass()
	if !textDone || text != "héllo" || textErr != nil {
		t.Errorf("ReadText = %q, %v (called %v)", text, textErr, textDone)
	}

	// ReadTypes: normalized, in the owner's order.
	var types []string
	if err := cb.ReadTypes(func(ts []string, err error) { types = ts }); err != nil {
		t.Fatal(err)
	}
	g.x.take()
	g.answer(clip, g.b.atoms.targets, xproto.AtomAtom, 32, atomList(g.b.atoms.targets, g.x.id("text/html"), xproto.AtomString, g.x.id("x-special/gnome-copied-files")))
	g.pass()
	if want := []string{"text/html", platform.MIMEText, platform.MIMEURIList}; !slices.Equal(types, want) {
		t.Errorf("ReadTypes = %q, want %q", types, want)
	}

	// Read of a type offered only as STRING (Latin-1) and of one not
	// offered (nil, nil).
	var got []byte
	var called int
	cb.Read(platform.MIMEText, func(b []byte, err error) { got, called = b, called+1 })
	g.answer(clip, g.b.atoms.targets, xproto.AtomAtom, 32, atomList(xproto.AtomString))
	if s := g.x.take(); !strings.HasSuffix(s, "convert CLIPBOARD STRING to TELA_SELECTION t=0") {
		t.Errorf("STRING read sent %q", s)
	}
	g.answer(clip, xproto.AtomString, xproto.AtomString, 8, []byte("caf\xe9"))
	g.pass()
	if called != 1 || string(got) != "café" {
		t.Errorf("Read STRING = %q (%d calls)", got, called)
	}
	cb.Read("image/png", func(b []byte, err error) { got, called = b, called+1 })
	g.answer(clip, g.b.atoms.targets, xproto.AtomAtom, 32, atomList(xproto.AtomString))
	g.pass()
	if called != 2 || got != nil {
		t.Errorf("Read of a missing type = %q (%d calls)", got, called)
	}

	// No owner: the server answers with property None.
	cb.ReadText(func(s string, err error) { text, textErr, called = s, err, called+1 })
	g.feed(xproto.SelectionNotifyEvent{Requestor: fakeSelWin, Selection: clip, Target: g.b.atoms.targets})
	g.pass()
	if called != 3 || text != "" || textErr != nil {
		t.Errorf("empty clipboard: %q, %v", text, textErr)
	}

	// An owner that never answers: the read fails at the timeout.
	cb.ReadText(func(s string, err error) { textErr, called = err, called+1 })
	g.x.fire(selectionTimeout)
	g.pass()
	if called != 4 || textErr != errSelectionTimeout {
		t.Errorf("timeout: %v (%d calls)", textErr, called)
	}
	// Its late answer is ignored.
	g.x.take()
	g.feed(xproto.SelectionNotifyEvent{Requestor: fakeSelWin, Selection: clip, Target: g.b.atoms.targets, Property: g.b.atoms.telaSelection})
	if s := g.x.take(); s != "" {
		t.Errorf("late answer: %s", s)
	}
}

func TestClipboardReadIncr(t *testing.T) {
	g := newDataRig()
	clip := g.b.atoms.clipboard
	var got []byte
	done := false
	g.b.Clipboard().Read("image/png", func(b []byte, err error) { got, done = b, true })
	png := g.x.id("image/png")
	g.answer(clip, g.b.atoms.targets, xproto.AtomAtom, 32, atomList(png))
	g.x.take()
	g.answer(clip, png, g.b.atoms.incr, 32, card32s(10))
	for _, chunk := range []string{"01234", "56789", ""} {
		g.x.setProp(fakeSelWin, g.b.atoms.telaSelection, png, 8, []byte(chunk))
		g.feed(propNotify(fakeSelWin, g.b.atoms.telaSelection, false))
	}
	g.pass()
	if !done || string(got) != "0123456789" {
		t.Errorf("INCR read = %q (done %v)", got, done)
	}
	if s := g.x.take(); strings.Count(s, "read+delete") != 4 {
		t.Errorf("INCR read requests: %s", s)
	}
}

func TestClipboardLocal(t *testing.T) {
	g := newDataRig()
	cb := g.b.Clipboard()
	if err := cb.WriteText("mine"); err != nil {
		t.Fatal(err)
	}
	g.x.take()
	var text string
	var types []string
	cb.ReadText(func(s string, err error) { text = s })
	cb.ReadTypes(func(ts []string, err error) { types = ts })
	if text != "" || types != nil {
		t.Fatal("local reads answered synchronously")
	}
	g.pass()
	if text != "mine" || !slices.Equal(types, []string{platform.MIMEText}) {
		t.Errorf("local reads = %q, %q", text, types)
	}
	if s := g.x.take(); s != "" {
		t.Errorf("local reads sent %q", s)
	}
	// An empty entry reads as present and empty, not absent (nil).
	if err := cb.Write(platform.Data{{Type: "application/x-empty"}, {Type: "text/html", Bytes: []byte{}}}); err != nil {
		t.Fatal(err)
	}
	for _, mime := range []string{"application/x-empty", "text/html"} {
		var got []byte
		answered := false
		cb.Read(mime, func(b []byte, err error) { got, answered = b, true })
		g.pass()
		if !answered || got == nil || len(got) != 0 {
			t.Errorf("local read of the empty %s entry = %v (answered %v), want an empty non-nil slice", mime, got, answered)
		}
	}
	absent := []byte("x")
	cb.Read("image/png", func(b []byte, err error) { absent = b })
	g.pass()
	if absent != nil {
		t.Errorf("local read of an absent type = %v, want nil", absent)
	}
	// Empty data owns the clipboard with nothing in it.
	if err := cb.Write(nil); err != nil {
		t.Fatal(err)
	}
	cb.ReadTypes(func(ts []string, err error) { types = ts })
	g.pass()
	if types != nil {
		t.Errorf("types after Write(nil) = %q", types)
	}
	// Another client keeps the selection: Write fails.
	g.x.owners[g.b.atoms.clipboard] = 1234
	g.x.setOwnerHook = func(sel xproto.Atom) { g.x.owners[sel] = 1234 }
	if err := cb.WriteText("x"); err == nil {
		t.Errorf("Write succeeded without the ownership")
	}
}
