//go:build windows

package win32

import (
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	// Another process (a clipboard manager, a remote desktop client) may
	// hold the clipboard open for a few milliseconds when it changes.
	clipboardTries = 20
	clipboardWait  = 5 * time.Millisecond
)

// Clipboard returns the system clipboard. Text goes through CF_UNICODETEXT
// with the message-only window as its owner. Windows keeps the data itself
// (nothing is rendered on demand), so a written text outlives the window and
// needs no serving from the event loop.
//
// Windows text uses CR LF line endings. ReadText returns LF text (CR LF and
// a lone CR become LF) and WriteText stores CR LF, as browsers on Windows do.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

type clipboard struct{ b *Backend }

var _ platform.Clipboard = clipboard{}

// ReadText reads the clipboard from the event loop, never from inside the
// call (the contract is the same for every backend). The text is "" when the
// clipboard holds no text.
func (c clipboard) ReadText(done func(text string, err error)) {
	if done == nil {
		panic("win32: Clipboard.ReadText called with a nil callback; pass a func(text string, err error)")
	}
	c.b.loop.Post(func() {
		text, err := c.b.readClipboard()
		done(text, err)
	})
	c.b.notify()
}

// WriteText replaces the clipboard's contents with text (LF line endings are
// stored as CR LF; NUL characters are dropped, as they would end the string).
func (c clipboard) WriteText(text string) error {
	return c.b.writeClipboard(clipboardFromLF(text))
}

// writeClipboard stores text, as is, as the clipboard's only content.
func (b *Backend) writeClipboard(text string) error {
	u := utf16Clipboard(text)
	size := uintptr(len(u)) * unsafe.Sizeof(u[0])
	// Allocate and fill the data before opening the clipboard, which
	// holds it away from every other process.
	h, _, e := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return winError("GlobalAlloc for the clipboard text", e)
	}
	p, _, e := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return winError("GlobalLock for the clipboard text", e)
	}
	copy(unsafe.Slice((*uint16)(ptrFromUintptr(p)), len(u)), u)
	procGlobalUnlock.Call(h)

	if err := b.openClipboard(); err != nil {
		procGlobalFree.Call(h)
		return err
	}
	defer procCloseClipboard.Call()
	if r, _, e := procEmptyClipboard.Call(); r == 0 {
		procGlobalFree.Call(h)
		return winError("EmptyClipboard", e)
	}
	// On success the system owns the memory.
	if r, _, e := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return winError("SetClipboardData", e)
	}
	return nil
}

// openClipboard opens the clipboard for the message-only window, retrying
// briefly while another process has it open.
func (b *Backend) openClipboard() error {
	var e error
	for range clipboardTries {
		r, _, err := procOpenClipboard.Call(b.msgHwnd)
		if r != 0 {
			return nil
		}
		e = err
		time.Sleep(clipboardWait)
	}
	return winError("OpenClipboard (another application keeps the clipboard open)", e)
}

func (b *Backend) readClipboard() (string, error) {
	text, err := b.readClipboardRaw()
	return lfFromClipboard(text), err
}

// readClipboardRaw returns the clipboard's text as stored.
func (b *Backend) readClipboardRaw() (string, error) {
	if r, _, _ := procIsClipboardFormatAvailable.Call(cfUnicodeText); r == 0 {
		return "", nil
	}
	if err := b.openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()
	h, _, e := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		// The format vanished between the check and the open.
		if r, _, _ := procIsClipboardFormatAvailable.Call(cfUnicodeText); r == 0 {
			return "", nil
		}
		return "", winError("GetClipboardData", e)
	}
	size, _, _ := procGlobalSize.Call(h)
	p, _, e := procGlobalLock.Call(h)
	if p == 0 {
		return "", winError("GlobalLock for the clipboard text", e)
	}
	defer procGlobalUnlock.Call(h)
	return fromUTF16Clipboard(unsafe.Slice((*uint16)(ptrFromUintptr(p)), size/unsafe.Sizeof(uint16(0)))), nil
}
