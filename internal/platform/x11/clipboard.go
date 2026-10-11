//go:build linux

package x11

import "errors"

// ErrClipboardUnsupported is reported by the X11 clipboard until X11
// selections (CLIPBOARD, served from the event loop) are implemented with
// text editing in phase 5.
var ErrClipboardUnsupported = errors.New("x11: the clipboard is not supported yet; X11 selections arrive with text editing (phase 5)")

// clipboard is a placeholder that honors the asynchronous contract: the
// ReadText callback runs later from the event loop, never from inside
// ReadText.
type clipboard struct{ b *Backend }

func (c clipboard) ReadText(done func(text string, err error)) {
	if done == nil {
		panic("x11: Clipboard.ReadText called with a nil callback")
	}
	c.b.queue = append(c.b.queue, item{fn: func() { done("", ErrClipboardUnsupported) }})
}

func (c clipboard) WriteText(string) error { return ErrClipboardUnsupported }
