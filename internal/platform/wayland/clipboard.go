//go:build linux

package wayland

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// The clipboard is the wl_data_device selection of a seat (no primary
// selection, no drag and drop). See the package documentation for the
// model; the quirks handled here:
//
//   - Ownership. The compositor confirms a set_selection only indirectly:
//     the old source gets wl_data_source.cancelled, and a focused client
//     gets a selection event with a new wl_data_offer, its own included. A
//     compositor that refuses the request (the client has no keyboard
//     focus, or the serial is older than the current selection's) says
//     nothing. Ignoring selection events while our source is alive (GTK's
//     approach) would then keep answering pastes with our stale text.
//     Here each source also offers a private MIME type naming it
//     (sourceMarker), so a selection event tells whether the compositor's
//     selection is our latest source or someone else's.
//   - Reading our own selection does not go through the compositor: the
//     text is returned from memory, like GTK and Qt do. Through the
//     compositor it would still work (both ends of the transfer are
//     non-blocking and served by the same loop), but slower.
//   - Pipes. pipe2(O_NONBLOCK) would make both ends non-blocking, and the
//     write end is handed to the source client: a client that writes with
//     plain blocking write(2) would get EAGAIN and truncate the data. The
//     pipe is created blocking and only our read end is switched.
//   - Drag and drop. Offers announced by wl_data_device.enter are kept
//     until leave or drop and destroyed unused; they never touch the
//     selection.

// dataDeviceManagerVersion is the highest wl_data_device_manager version
// this code knows: 3 adds drag-and-drop actions (unused) and changes
// nothing for selections.
const dataDeviceManagerVersion = 3

// defaultTransferTimeout bounds a transfer that makes no progress: a
// source that neither writes nor closes the pipe (a frozen application),
// or a reader that stops reading. Qt waits 1 s for clipboard data; a few
// seconds tolerate a busy source without leaving a paste pending for long.
// It is measured from the last byte moved, so large transfers are not cut.
const defaultTransferTimeout = 5 * time.Second

// maxClipboardText bounds the text read from another client.
const maxClipboardText = 64 << 20

// Text MIME types, offered by WriteText in this order (the preferred first,
// as GTK and Firefox list them). The X11 names (UTF8_STRING, TEXT, STRING)
// are what Xwayland and older toolkits ask for.
const (
	mimeUTF8       = "text/plain;charset=utf-8"
	mimeText       = "text/plain"
	mimeUTF8String = "UTF8_STRING"
	mimeX11Text    = "TEXT"
	mimeX11String  = "STRING"
)

var offeredMimes = [...]string{mimeUTF8, mimeText, mimeUTF8String, mimeX11Text, mimeX11String}

// errNoDataDevice is reported when the compositor has no
// wl_data_device_manager or no seat: there is no clipboard to use.
var errNoDataDevice = errors.New("wayland: the clipboard is unavailable: the compositor has no wl_data_device_manager or no seat")

// errTransferTimeout is reported by ReadText when the source client sends
// nothing for defaultTransferTimeout.
var errTransferTimeout = errors.New("wayland: the clipboard owner did not send its data in time")

// sourceCount numbers the data sources of the process (sourceMarker).
var sourceCount atomic.Uint64

// sourceMarker returns a private MIME type that names one data source of
// this process. Other clients ignore unknown types; a clipboard manager
// that copies every type gets an empty value.
func sourceMarker() string {
	return fmt.Sprintf("application/x-tela-source-%d-%d", os.Getpid(), sourceCount.Add(1))
}

// dataDevice is the wl_data_device of a seat with its selection state.
type dataDevice struct {
	s  *seat
	wl *protocols.WlDataDevice

	// offers are the wl_data_offers announced by data_offer and not yet
	// used by a selection or enter event, with their MIME types.
	offers map[*protocols.WlDataOffer]*dataOffer
	// selection is the current selection when it belongs to another
	// client (nil: empty, unknown, or ours).
	selection *dataOffer
	// dnd is the offer of a drag over one of our surfaces, ignored.
	dnd *dataOffer

	// source is our latest data source, alive until the compositor
	// cancels it or WriteText replaces it. own reports that the
	// compositor's selection is believed to be source: set by WriteText,
	// confirmed or refuted by selection events.
	source *dataSource
	own    bool
}

type dataOffer struct {
	wl    *protocols.WlDataOffer
	mimes []string
}

type dataSource struct {
	wl     *protocols.WlDataSource
	text   string
	marker string
}

func (o *dataOffer) has(mime string) bool {
	for _, m := range o.mimes {
		if m == mime {
			return true
		}
	}
	return false
}

func (o *dataOffer) destroy() {
	if !o.wl.Destroyed() {
		o.wl.Destroy()
	}
}

func (b *Backend) bindDataDeviceManager(name, version uint32) {
	if b.dataManager != nil {
		return
	}
	b.dataManager = protocols.WlRegistryBind[protocols.WlDataDeviceManager](b.registry, name, min(version, dataDeviceManagerVersion))
	for _, s := range b.seats {
		s.attachDataDevice()
	}
}

// attachDataDevice creates the seat's data device, once the seat and the
// manager both exist (the registry may announce them in either order).
func (s *seat) attachDataDevice() {
	if s.data != nil || s.b.dataManager == nil {
		return
	}
	d := &dataDevice{s: s, offers: map[*protocols.WlDataOffer]*dataOffer{}}
	d.wl = s.b.dataManager.GetDataDevice(s.wl)
	d.wl.SetDataOfferHandler(d.onDataOffer)
	d.wl.SetSelectionHandler(d.onSelection)
	d.wl.SetEnterHandler(d.onEnter)
	d.wl.SetLeaveHandler(func(protocols.WlDataDeviceLeaveEvent) { d.endDrag() })
	d.wl.SetDropHandler(func(protocols.WlDataDeviceDropEvent) { d.endDrag() })
	s.data = d
}

// release destroys the device and every object made from it. Transfers
// already running go on: they own their descriptors and bytes.
func (d *dataDevice) release() {
	for _, o := range d.offers {
		o.destroy()
	}
	clear(d.offers)
	for _, o := range []*dataOffer{d.selection, d.dnd} {
		if o != nil {
			o.destroy()
		}
	}
	d.selection, d.dnd = nil, nil
	if d.source != nil {
		d.source.wl.Destroy()
		d.source = nil
	}
	d.own = false
	if d.wl.Version() >= 2 && !d.wl.Destroyed() {
		d.wl.Release()
	}
}

func (d *dataDevice) onDataOffer(e protocols.WlDataDeviceDataOfferEvent) {
	o := &dataOffer{wl: e.Id}
	e.Id.SetOfferHandler(func(m protocols.WlDataOfferOfferEvent) { o.mimes = append(o.mimes, m.MimeType) })
	d.offers[e.Id] = o
}

// take returns the announced offer wl and forgets it. Other announced
// offers were never used and will not be (an offer is used by the event
// that follows its announcement), so they are destroyed.
func (d *dataDevice) take(wl *protocols.WlDataOffer) *dataOffer {
	var got *dataOffer
	for k, o := range d.offers {
		if k == wl {
			got = o
		} else {
			o.destroy()
		}
	}
	clear(d.offers)
	if got == nil && wl != nil {
		got = &dataOffer{wl: wl} // not announced: a compositor bug; no MIME types
	}
	return got
}

// onSelection records a new selection. The previous offer must be
// destroyed (the protocol's rule). An offer naming our latest source means
// the compositor accepted our set_selection: it is destroyed at once and
// reads come from memory.
func (d *dataDevice) onSelection(e protocols.WlDataDeviceSelectionEvent) {
	o := d.take(e.Id)
	if d.selection != nil {
		d.selection.destroy()
		d.selection = nil
	}
	d.own = false
	if o == nil {
		return // empty selection
	}
	if d.source != nil && o.has(d.source.marker) {
		d.own = true
		o.destroy()
		return
	}
	d.selection = o
}

func (d *dataDevice) onEnter(e protocols.WlDataDeviceEnterEvent) {
	d.endDrag()
	d.dnd = d.take(e.Id)
}

// endDrag destroys the offer of a drag, which Tela does not accept (no
// wl_data_offer.accept: the source sees no target and cancels).
func (d *dataDevice) endDrag() {
	if d.dnd != nil {
		d.dnd.destroy()
		d.dnd = nil
	}
}

// --- writing ---

// setSelection makes text the seat's selection. The serial is the seat's
// latest input serial (see seat.serial).
func (d *dataDevice) setSelection(text string) {
	b := d.s.b
	src := &dataSource{text: text, marker: sourceMarker()}
	src.wl = b.dataManager.CreateDataSource()
	src.wl.SetSendHandler(func(e protocols.WlDataSourceSendEvent) { b.serve(src, e.MimeType, e.Fd) })
	src.wl.SetCancelledHandler(func(protocols.WlDataSourceCancelledEvent) { d.cancelled(src) })
	for _, m := range offeredMimes {
		src.wl.Offer(m)
	}
	src.wl.Offer(src.marker)
	d.wl.SetSelection(src.wl, d.s.serial)
	// The previous source is replaced (GTK does the same): if the
	// compositor accepted the request it cancels it anyway, and if it
	// refused, the selection is cleared rather than left pointing at text
	// the app no longer means.
	if d.source != nil {
		d.source.wl.Destroy()
	}
	d.source, d.own = src, true
}

// cancelled handles wl_data_source.cancelled: another client took the
// selection (or the compositor dropped ours).
func (d *dataDevice) cancelled(src *dataSource) {
	src.wl.Destroy()
	if d.source == src {
		d.source, d.own = nil, false
	}
}

// serve answers a wl_data_source.send: the bytes are written to fd from
// the poll loop without blocking it.
func (b *Backend) serve(src *dataSource, mime string, fd int) {
	var data []byte
	switch {
	case mime == mimeX11String:
		data = latin1(src.text)
	case isTextMime(mime):
		data = []byte(src.text)
	}
	if len(data) == 0 {
		wire.CloseFd(fd) // empty text, our marker, or a type we never offered
		return
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		wire.CloseFd(fd)
		return
	}
	b.transfers = append(b.transfers, &transfer{fd: fd, write: true, data: data, deadline: b.Now() + b.transferTimeout})
}

func isTextMime(mime string) bool {
	for _, m := range offeredMimes {
		if m == mime {
			return true
		}
	}
	return false
}

// latin1 encodes s as ISO 8859-1, the encoding ICCCM gives STRING; runes
// outside it become '?', as GTK 3 does.
func latin1(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xff {
			r = '?'
		}
		out = append(out, byte(r))
	}
	return out
}

// --- reading ---

// readSelection starts reading the seat's selection. done is always
// posted to the loop, never called from here.
func (d *dataDevice) readSelection(done func(string, error)) {
	b := d.s.b
	if d.own && d.source != nil {
		text := d.source.text
		b.loop.Post(func() { done(text, nil) })
		return
	}
	mime := ""
	if d.selection != nil {
		mime = chooseMime(d.selection.mimes)
	}
	if mime == "" {
		b.loop.Post(func() { done("", nil) }) // empty, or no text in it
		return
	}
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC); err != nil {
		b.loop.Post(func() { done("", fmt.Errorf("wayland: cannot create a pipe for the clipboard: %w", err)) })
		return
	}
	if err := unix.SetNonblock(p[0], true); err != nil {
		unix.Close(p[0])
		unix.Close(p[1])
		b.loop.Post(func() { done("", fmt.Errorf("wayland: cannot read the clipboard: %w", err)) })
		return
	}
	// The request carries a duplicate of the write end; ours is closed at
	// once, so the read end sees EOF when the source closes its copy.
	d.selection.wl.Receive(mime, p[1])
	unix.Close(p[1])
	b.transfers = append(b.transfers, &transfer{fd: p[0], mime: mime, done: done, deadline: b.Now() + b.transferTimeout})
}

// chooseMime picks the text type to receive: declared UTF-8 first, then
// plain text, then the X11 legacy types (GTK's order); "" if none is
// offered.
func chooseMime(mimes []string) string {
	best, rank := "", -1
	for _, m := range mimes {
		if r := mimeRank(m); r > rank {
			best, rank = m, r
		}
	}
	return best
}

// mimeRank ranks a text type (higher is better, -1 for other types). MIME
// types compare without case and spaces ("text/plain; charset=UTF-8");
// the X11 names are atoms, which are case-sensitive.
func mimeRank(m string) int {
	norm := strings.ReplaceAll(m, " ", "")
	switch {
	case strings.EqualFold(norm, mimeUTF8):
		return 4
	case norm == mimeUTF8String:
		return 3
	case strings.EqualFold(norm, mimeText):
		return 2
	case norm == mimeX11Text:
		return 1
	case norm == mimeX11String:
		return 0
	}
	return -1
}

// decodeText turns received bytes into a Go string. Trailing NULs (some
// X11 sources terminate strings) are dropped. Types that declare UTF-8
// replace invalid sequences with U+FFFD, as browsers do. Types without a
// declared encoding (text/plain, TEXT, STRING) are UTF-8 when they are
// valid UTF-8, and ISO 8859-1 otherwise (STRING's ICCCM encoding, and what
// legacy sources send for the others), which never loses a byte.
func decodeText(data []byte, mime string) string {
	for len(data) > 0 && data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}
	if mimeRank(mime) >= 3 { // declared UTF-8
		return strings.ToValidUTF8(string(data), "�")
	}
	if utf8.Valid(data) {
		return string(data)
	}
	r := make([]rune, len(data))
	for i, c := range data {
		r[i] = rune(c)
	}
	return string(r)
}

// --- transfers ---

// transfer is one pipe being written (a send we serve) or read (a
// ReadText). Transfers are owned by the UI thread: the poll loop waits on
// their descriptors together with the display socket and does the
// non-blocking I/O itself, so no goroutine shares state with the backend
// and a peer that never reads or writes costs nothing but its deadline.
type transfer struct {
	fd       int
	write    bool
	data     []byte // write: bytes left; read: bytes so far
	mime     string // read: the type received
	done     func(string, error)
	deadline time.Duration // backend time; moved forward by progress
}

func (t *transfer) events() int16 {
	if t.write {
		return unix.POLLOUT
	}
	return unix.POLLIN
}

// service does the I/O a poll result allows. It returns false when the
// transfer is over (its descriptor is closed and done posted).
func (b *Backend) service(t *transfer, revents int16) bool {
	if revents == 0 {
		return true
	}
	if t.write {
		for len(t.data) > 0 {
			n, err := unix.Write(t.fd, t.data)
			switch {
			case err == unix.EINTR:
				continue
			case err == unix.EAGAIN:
				return true
			case err != nil:
				// The reader went away (EPIPE; Go ignores SIGPIPE for
				// descriptors other than stdout and stderr). Nobody waits
				// for the result.
				wire.CloseFd(t.fd)
				return false
			}
			t.data = t.data[n:]
			t.deadline = b.Now() + b.transferTimeout
		}
		wire.CloseFd(t.fd)
		return false
	}
	var buf [64 << 10]byte
	for {
		n, err := unix.Read(t.fd, buf[:])
		switch {
		case err == unix.EINTR:
			continue
		case err == unix.EAGAIN:
			return true
		case err != nil:
			b.finishRead(t, "", fmt.Errorf("wayland: cannot read the clipboard: %w", err))
			return false
		case n == 0:
			b.finishRead(t, decodeText(t.data, t.mime), nil)
			return false
		}
		if len(t.data)+n > maxClipboardText {
			b.finishRead(t, "", fmt.Errorf("wayland: the clipboard text exceeds %d MiB", maxClipboardText>>20))
			return false
		}
		t.data = append(t.data, buf[:n]...)
		t.deadline = b.Now() + b.transferTimeout
	}
}

func (b *Backend) finishRead(t *transfer, text string, err error) {
	wire.CloseFd(t.fd)
	done := t.done
	b.loop.Post(func() { done(text, err) })
}

// serviceTransfers runs the I/O of the transfers polled with fds (one
// PollFd per transfer, in order).
func (b *Backend) serviceTransfers(fds []unix.PollFd) {
	if len(fds) == 0 {
		return
	}
	live := b.transfers[:0]
	for i, t := range b.transfers {
		if i >= len(fds) || b.service(t, fds[i].Revents) {
			live = append(live, t)
		}
	}
	clear(b.transfers[len(live):])
	b.transfers = live
}

// expireTransfers ends the transfers whose deadline passed: a read reports
// errTransferTimeout, a write is abandoned (the reader gets what was
// written so far).
func (b *Backend) expireTransfers(now time.Duration) {
	live := b.transfers[:0]
	for _, t := range b.transfers {
		if now < t.deadline {
			live = append(live, t)
			continue
		}
		if t.write {
			wire.CloseFd(t.fd)
		} else {
			b.finishRead(t, "", errTransferTimeout)
		}
	}
	clear(b.transfers[len(live):])
	b.transfers = live
}

// transferDeadline returns the earliest transfer deadline.
func (b *Backend) transferDeadline() (time.Duration, bool) {
	var d time.Duration
	ok := false
	for _, t := range b.transfers {
		if !ok || t.deadline < d {
			d, ok = t.deadline, true
		}
	}
	return d, ok
}

// closeTransfers drops every transfer (the backend is shutting down; the
// loop is gone, so pending reads are never answered).
func (b *Backend) closeTransfers() {
	for _, t := range b.transfers {
		wire.CloseFd(t.fd)
	}
	b.transfers = nil
}

// --- platform.Clipboard ---

// clipboard is the platform.Clipboard of the backend: the selection of the
// seat whose keyboard focuses one of our windows, else of the first seat.
type clipboard struct{ b *Backend }

var _ platform.Clipboard = clipboard{}

func (b *Backend) clipboardDevice() *dataDevice {
	var first *dataDevice
	for _, s := range b.seats {
		if s.data == nil {
			continue
		}
		if s.kbd != nil && s.kbd.focus != nil {
			return s.data
		}
		if first == nil {
			first = s.data
		}
	}
	return first
}

// ReadText reads the selection's text. done runs from the event loop: at
// once for our own selection, an empty one or one without text, and when
// the transfer ends for another client's.
func (c clipboard) ReadText(done func(string, error)) {
	if done == nil {
		panic("wayland: Clipboard.ReadText called with a nil callback")
	}
	d := c.b.clipboardDevice()
	if d == nil {
		c.b.loop.Post(func() { done("", errNoDataDevice) })
		return
	}
	d.readSelection(done)
}

// WriteText makes text the selection. The compositor may refuse it
// silently (no keyboard focus); see the package documentation.
func (c clipboard) WriteText(text string) error {
	d := c.b.clipboardDevice()
	if d == nil {
		return errNoDataDevice
	}
	d.setSelection(text)
	return nil
}
