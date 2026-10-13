//go:build linux

package wayland

import (
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
)

// wl_data_device and wl_data_source event opcodes used by the tests.
const (
	ddDataOffer = 0
	ddEnter     = 1
	ddLeave     = 2
	ddMotion    = 3
	ddDrop      = 4
	ddSelection = 5

	dsSend      = 1
	dsCancelled = 2
)

// clipboardGlobals is keyboardGlobals plus wl_data_device_manager 3,
// announced before or after the seat.
func clipboardGlobals(managerFirst bool) []fakeGlobal {
	g := keyboardGlobals()
	m := fakeGlobal{iface: "wl_data_device_manager", version: 3}
	if managerFirst {
		return append([]fakeGlobal{m}, g...)
	}
	return append(g, m)
}

// clipSetup opens a focused window on a seat with a keyboard and a data
// device and returns the wl_keyboard and wl_data_device ids.
func clipSetup(t *testing.T, h *harness) (kb, dev uint32) {
	t.Helper()
	kb, _ = kbSetup(t, h, "us", 0, 0)
	dev = h.fc.id("wl_data_device")
	if dev == 0 {
		t.Fatal("no wl_data_device was created")
	}
	return kb, dev
}

// offerMsg encodes a data_offer announcement with its MIME types, and
// returns the new offer's id.
func (fc *fakeComp) offerMsg(dev uint32, mimes ...string) ([]byte, uint32) {
	id := fc.newServerObject("wl_data_offer")
	b := event(dev, ddDataOffer, id)
	for _, m := range mimes {
		b = append(b, event(id, 0, m)...)
	}
	return b, id
}

// setSelection makes another client's offer with mimes the selection.
func (fc *fakeComp) setSelection(dev uint32, mimes ...string) uint32 {
	b, id := fc.offerMsg(dev, mimes...)
	fc.sendRaw(append(b, event(dev, ddSelection, id)...))
	return id
}

// readResult is a ReadText answer.
type readResult struct {
	text string
	err  error
}

// readText calls ReadText on the UI thread, checks that the callback is
// not called from inside it, and returns a channel with the answer.
func readText(t *testing.T, h *harness) <-chan readResult {
	t.Helper()
	ch := make(chan readResult, 1)
	h.ui(func() {
		inside := true
		h.b.Clipboard().ReadText(func(s string, err error) {
			if inside {
				t.Errorf("ReadText called its callback from inside ReadText")
			}
			ch <- readResult{s, err}
		})
		inside = false
	})
	return ch
}

func wait(t *testing.T, ch <-chan readResult) readResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("ReadText never called back")
	}
	return readResult{}
}

// readAllFd reads fd until EOF on another goroutine and closes it.
func readAllFd(fd int) <-chan []byte {
	ch := make(chan []byte, 1)
	go func() {
		f := os.NewFile(uintptr(fd), "pipe")
		defer f.Close()
		b, _ := io.ReadAll(f)
		ch <- b
	}()
	return ch
}

func waitBytes(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case b := <-ch:
		return b
	case <-time.After(5 * time.Second):
		t.Fatal("the transfer never ended")
	}
	return nil
}

// requestSend asks our data source for mime, as the compositor does for
// a reading client, and returns the read end of the pipe.
func requestSend(t *testing.T, h *harness, src uint32, mime string) int {
	t.Helper()
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	h.fc.sendFds(event(src, dsSend, mime), p[1])
	unix.Close(p[1])
	return p[0]
}

func writeText(t *testing.T, h *harness, text string) {
	t.Helper()
	h.ui(func() {
		if err := h.b.Clipboard().WriteText(text); err != nil {
			t.Errorf("WriteText: %v", err)
		}
	})
}

func TestClipboardWriteAndServe(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	kb, dev := clipSetup(t, h)
	// A key press: its serial (100) is the newest input serial.
	h.fc.sendRaw(cat(keyMsg(kb, keyA, 1), keyMsg(kb, keyA, 0)))
	expectKey(t, h)
	expectKey(t, h)

	text := strings.Repeat("Tela clipboard: café ✓ 0123456789\n", 40000) // 1.4 MB, far over a pipe buffer
	writeText(t, h, text)
	reqs := h.fc.until("wl_data_device.set_selection")
	src := h.fc.id("wl_data_source")
	if _, i := find(reqs, "wl_data_device_manager.create_data_source"); i < 0 {
		t.Fatalf("no create_data_source in %v", names(reqs))
	}
	var mimes []string
	for _, r := range reqs {
		if r.is("wl_data_source.offer") && r.id == src {
			mimes = append(mimes, r.args[0].(string))
		}
	}
	want := []string{"text/plain;charset=utf-8", "text/plain", "UTF8_STRING", "TEXT", "STRING"}
	if len(mimes) != len(want)+1 || !slices.Equal(mimes[:len(want)], want) || !strings.HasPrefix(mimes[len(want)], "application/x-tela-source-") {
		t.Errorf("offered MIME types %q, want %q plus the source marker", mimes, want)
	}
	sel := reqs[len(reqs)-1]
	if sel.id != dev || sel.u(0) != src || sel.u(1) != 100 {
		t.Errorf("set_selection = %v, want source %d with the key press serial 100", sel, src)
	}

	// Two readers at once, one of them asking for the large text.
	big := readAllFd(requestSend(t, h, src, "text/plain;charset=utf-8"))
	small := readAllFd(requestSend(t, h, src, "UTF8_STRING"))
	if got := waitBytes(t, big); string(got) != text {
		t.Errorf("served %d bytes, want the %d bytes of the text", len(got), len(text))
	}
	if got := waitBytes(t, small); string(got) != text {
		t.Errorf("UTF8_STRING: served %d bytes, want %d", len(got), len(text))
	}

	// STRING is ISO 8859-1 (ICCCM); the marker and unknown types are
	// empty.
	writeText(t, h, "café ✓")
	// The new source replaces the old one, which is destroyed.
	if r := h.fc.until("wl_data_source.destroy"); r[len(r)-1].id != src {
		t.Errorf("destroyed %v, want the replaced source %d", r[len(r)-1], src)
	}
	src2 := h.fc.id("wl_data_source")
	if got := waitBytes(t, readAllFd(requestSend(t, h, src2, "STRING"))); string(got) != "caf\xe9 ?" {
		t.Errorf("STRING = %q, want Latin-1 with '?'", got)
	}
	for _, m := range []string{mimes[len(want)], "image/png"} {
		if got := waitBytes(t, readAllFd(requestSend(t, h, src2, m))); len(got) != 0 {
			t.Errorf("%s = %q, want nothing", m, got)
		}
	}
}

func TestClipboardWriteStalledReader(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	h.b.transferTimeout = 150 * time.Millisecond
	clipSetup(t, h)
	text := strings.Repeat("x", 1<<20)
	writeText(t, h, text)
	h.fc.until("wl_data_device.set_selection")
	fd := requestSend(t, h, h.fc.id("wl_data_source"), "text/plain")
	// Nobody reads for longer than the timeout: the client gives up and
	// closes its end, so the reader then gets a prefix and EOF.
	time.Sleep(400 * time.Millisecond)
	got := waitBytes(t, readAllFd(fd))
	if len(got) == 0 || len(got) >= len(text) {
		t.Errorf("a stalled reader got %d bytes, want a prefix of %d", len(got), len(text))
	}
	h.ui(func() {
		if n := len(h.b.transfers); n != 0 {
			t.Errorf("%d transfers left after the timeout", n)
		}
	})

	// A reader that goes away: the write fails with EPIPE (no SIGPIPE
	// kills the process) and the transfer ends at once.
	unix.Close(requestSend(t, h, h.fc.id("wl_data_source"), "text/plain"))
	time.Sleep(50 * time.Millisecond)
	h.ui(func() {
		if n := len(h.b.transfers); n != 0 {
			t.Errorf("%d transfers left after the reader closed", n)
		}
	})
}

func TestClipboardReadOtherClient(t *testing.T) {
	h := newHarness(t, clipboardGlobals(true))
	_, dev := clipSetup(t, h)

	cases := []struct {
		mimes []string
		want  string // MIME type received
		data  string
		text  string
	}{
		// Declared UTF-8 beats everything, wherever it is listed.
		{[]string{"text/html", "STRING", "TEXT", "text/plain", "UTF8_STRING", "text/plain;charset=utf-8"}, "text/plain;charset=utf-8", "pasted ✓", "pasted ✓"},
		{[]string{"STRING", "UTF8_STRING", "text/plain"}, "UTF8_STRING", "dos ✓\x00", "dos ✓"},
		{[]string{"STRING", "text/plain"}, "text/plain", "tres", "tres"},
		{[]string{"text/plain; charset=UTF-8"}, "text/plain; charset=UTF-8", "a\xffb", "a�b"},
		// No declared encoding and invalid UTF-8: ISO 8859-1.
		{[]string{"STRING", "TEXT"}, "TEXT", "caf\xe9", "café"},
		{[]string{"STRING"}, "STRING", "na\xefve", "naïve"},
	}
	prev := uint32(0)
	for _, c := range cases {
		offer := h.fc.setSelection(dev, c.mimes...)
		if prev != 0 {
			// The previous selection offer is destroyed (protocol rule).
			if r := h.fc.until("wl_data_offer.destroy"); r[len(r)-1].id != prev {
				t.Errorf("destroyed %v, want the previous offer %d", r[len(r)-1], prev)
			}
		}
		ch := readText(t, h)
		r := h.fc.until("wl_data_offer.receive")
		rec := r[len(r)-1]
		if rec.id != offer || rec.args[0].(string) != c.want {
			t.Errorf("offers %q: receive %v, want %q from offer %d", c.mimes, rec, c.want, offer)
		}
		fd := int(rec.args[1].(fdArg))
		unix.Write(fd, []byte(c.data))
		unix.Close(fd)
		if got := wait(t, ch); got.err != nil || got.text != c.text {
			t.Errorf("offers %q: ReadText = %q, %v; want %q", c.mimes, got.text, got.err, c.text)
		}
		prev = offer
	}

	// A selection without text, then an empty selection: "" and no
	// transfer.
	h.fc.setSelection(dev, "image/png", "text/html")
	if got := wait(t, readText(t, h)); got.err != nil || got.text != "" {
		t.Errorf("selection without text: ReadText = %q, %v", got.text, got.err)
	}
	h.fc.send(dev, ddSelection, uint32(0))
	if got := wait(t, readText(t, h)); got.err != nil || got.text != "" {
		t.Errorf("empty selection: ReadText = %q, %v", got.text, got.err)
	}
	h.fc.quiet(50*time.Millisecond, "wl_data_offer.receive")
}

func TestClipboardNoSelection(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	clipSetup(t, h)
	if got := wait(t, readText(t, h)); got.err != nil || got.text != "" {
		t.Errorf("ReadText = %q, %v; want \"\", nil", got.text, got.err)
	}
	h.fc.quiet(50*time.Millisecond, "wl_data_offer.receive")
}

func TestClipboardOwnSelection(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	_, dev := clipSetup(t, h)

	// Reading our own selection does not go through the compositor.
	writeText(t, h, "mine")
	reqs := h.fc.until("wl_data_device.set_selection")
	src := h.fc.id("wl_data_source")
	if got := wait(t, readText(t, h)); got.err != nil || got.text != "mine" {
		t.Errorf("own selection: ReadText = %q, %v", got.text, got.err)
	}

	// The compositor accepts it: the focused client gets an offer of its
	// own source, recognized by the marker and destroyed at once.
	var mimes []string
	for _, r := range reqs {
		if r.is("wl_data_source.offer") {
			mimes = append(mimes, r.args[0].(string))
		}
	}
	own := h.fc.setSelection(dev, mimes...)
	if r := h.fc.until("wl_data_offer.destroy"); r[len(r)-1].id != own {
		t.Errorf("destroyed %v, want our own offer %d", r[len(r)-1], own)
	}
	if got := wait(t, readText(t, h)); got.err != nil || got.text != "mine" {
		t.Errorf("own selection echoed: ReadText = %q, %v", got.text, got.err)
	}
	h.fc.quiet(50*time.Millisecond, "wl_data_offer.receive")

	// Another client takes the selection: our source is cancelled and
	// destroyed, and reads go to the new owner.
	b, theirs := h.fc.offerMsg(dev, "text/plain;charset=utf-8")
	h.fc.sendRaw(cat(event(src, dsCancelled), b, event(dev, ddSelection, theirs)))
	if r := h.fc.until("wl_data_source.destroy"); r[len(r)-1].id != src {
		t.Errorf("destroyed %v, want the cancelled source %d", r[len(r)-1], src)
	}
	ch := readText(t, h)
	r := h.fc.until("wl_data_offer.receive")
	if rec := r[len(r)-1]; rec.id != theirs {
		t.Errorf("receive %v, want from offer %d", rec, theirs)
	} else {
		fd := int(rec.args[1].(fdArg))
		unix.Write(fd, []byte("theirs"))
		unix.Close(fd)
	}
	if got := wait(t, ch); got.err != nil || got.text != "theirs" {
		t.Errorf("after cancelled: ReadText = %q, %v", got.text, got.err)
	}

	// A refused write: the compositor says nothing, and a later selection
	// event without our marker (at the next focus) shows that the
	// selection is still someone else's.
	writeText(t, h, "refused")
	h.fc.until("wl_data_device.set_selection")
	stale := h.fc.setSelection(dev, "text/plain;charset=utf-8", "application/x-tela-source-1-1")
	ch = readText(t, h)
	r = h.fc.until("wl_data_offer.receive")
	if rec := r[len(r)-1]; rec.id != stale {
		t.Errorf("receive %v, want from offer %d", rec, stale)
	} else {
		fd := int(rec.args[1].(fdArg))
		unix.Write(fd, []byte("still theirs"))
		unix.Close(fd)
	}
	if got := wait(t, ch); got.err != nil || got.text != "still theirs" {
		t.Errorf("after a refused write: ReadText = %q, %v", got.text, got.err)
	}
}

func TestClipboardCancelledWithoutNewSelection(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	clipSetup(t, h)
	writeText(t, h, "gone")
	h.fc.until("wl_data_device.set_selection")
	src := h.fc.id("wl_data_source")
	h.fc.send(src, dsCancelled)
	if r := h.fc.until("wl_data_source.destroy"); r[len(r)-1].id != src {
		t.Errorf("destroyed %v, want %d", r[len(r)-1], src)
	}
	if got := wait(t, readText(t, h)); got.err != nil || got.text != "" {
		t.Errorf("ReadText after losing ownership = %q, %v; want \"\"", got.text, got.err)
	}
}

func TestClipboardReadTimeout(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	h.b.transferTimeout = 150 * time.Millisecond
	_, dev := clipSetup(t, h)
	h.fc.setSelection(dev, "text/plain;charset=utf-8")
	start := time.Now()
	ch := readText(t, h)
	r := h.fc.until("wl_data_offer.receive")
	fd := int(r[len(r)-1].args[1].(fdArg))
	defer unix.Close(fd) // held open and silent: an unresponsive source
	unix.Write(fd, []byte("partial"))
	got := wait(t, ch)
	if !errors.Is(got.err, errTransferTimeout) || got.text != "" {
		t.Errorf("ReadText from a silent source = %q, %v; want errTransferTimeout", got.text, got.err)
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("timed out after %v, before the timeout", d)
	}
	h.ui(func() {
		if n := len(h.b.transfers); n != 0 {
			t.Errorf("%d transfers left after the timeout", n)
		}
	})
}

func TestClipboardIgnoresDragAndDrop(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	_, dev := clipSetup(t, h)
	sel := h.fc.setSelection(dev, "text/plain")
	surface := h.fc.id("wl_surface")

	for _, end := range []uint16{ddLeave, ddDrop} {
		b, dnd := h.fc.offerMsg(dev, "text/uri-list", "text/plain")
		h.fc.sendRaw(cat(b,
			event(dev, ddEnter, uint32(77), surface, 10.0, 20.0, dnd),
			event(dev, ddMotion, uint32(5), 11.0, 21.0),
			event(dev, end),
		))
		if r := h.fc.until("wl_data_offer.destroy"); r[len(r)-1].id != dnd {
			t.Errorf("destroyed %v, want the drag offer %d", r[len(r)-1], dnd)
		}
	}
	// The selection is untouched.
	ch := readText(t, h)
	r := h.fc.until("wl_data_offer.receive")
	if rec := r[len(r)-1]; rec.id != sel {
		t.Errorf("receive %v, want from the selection offer %d", rec, sel)
	} else {
		fd := int(rec.args[1].(fdArg))
		unix.Write(fd, []byte("kept"))
		unix.Close(fd)
	}
	if got := wait(t, ch); got.err != nil || got.text != "kept" {
		t.Errorf("ReadText after a drag = %q, %v", got.text, got.err)
	}
	// Nothing reached the app.
	h.noEvent(50 * time.Millisecond)
}

// TestClipboardPointerSerial checks the serial of a selection set after a
// click, on a seat without a keyboard.
func TestClipboardPointerSerial(t *testing.T) {
	g := append(standardGlobals(1), fakeGlobal{iface: "wl_data_device_manager", version: 3})
	h := newHarness(t, g)
	openWindow(t, h, platform.WindowOptions{}, 100, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	ptr, surface := h.fc.id("wl_pointer"), h.fc.id("wl_surface")
	h.fc.sendRaw(cat(
		event(ptr, 0, uint32(30), surface, 5.0, 5.0), event(ptr, 5), // enter, frame
		event(ptr, 3, uint32(31), uint32(0), uint32(btnLeft), uint32(1)), event(ptr, 5), // press
	))
	expect[platform.PointerEvent](h)
	expect[platform.PointerEvent](h)
	writeText(t, h, "clicked")
	r := h.fc.until("wl_data_device.set_selection")
	if sel := r[len(r)-1]; sel.u(1) != 31 {
		t.Errorf("set_selection serial %d, want the button serial 31", sel.u(1))
	}
}

func TestChooseMime(t *testing.T) {
	cases := []struct {
		mimes []string
		want  string
	}{
		{nil, ""},
		{[]string{"image/png", "text/html"}, ""},
		{[]string{"STRING", "TEXT"}, "TEXT"},
		{[]string{"TEXT", "text/plain"}, "text/plain"},
		{[]string{"text/plain", "UTF8_STRING"}, "UTF8_STRING"},
		{[]string{"UTF8_STRING", "TEXT/PLAIN;CHARSET=UTF-8"}, "TEXT/PLAIN;CHARSET=UTF-8"},
		{[]string{"utf8_string", "string"}, ""}, // atoms are case-sensitive
	}
	for _, c := range cases {
		if got := chooseMime(c.mimes); got != c.want {
			t.Errorf("chooseMime(%q) = %q, want %q", c.mimes, got, c.want)
		}
	}
}

func TestDecodeText(t *testing.T) {
	cases := []struct{ data, mime, want string }{
		{"ok ✓", "text/plain;charset=utf-8", "ok ✓"},
		{"a\xc3b", "UTF8_STRING", "a�b"},
		{"a\xc3b", "text/plain", "aÃb"},
		{"ok ✓", "STRING", "ok ✓"}, // valid UTF-8 is taken as such
		{"x\x00\x00", "TEXT", "x"},
		{"", "text/plain", ""},
	}
	for _, c := range cases {
		if got := decodeText([]byte(c.data), c.mime); got != c.want {
			t.Errorf("decodeText(%q, %s) = %q, want %q", c.data, c.mime, got, c.want)
		}
	}
	if got := string(latin1("ñ€")); got != "\xf1?" {
		t.Errorf("latin1 = %q", got)
	}
}
