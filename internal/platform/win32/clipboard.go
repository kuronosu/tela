//go:build windows

package win32

import (
	"errors"

	"github.com/kuronosu/tela/internal/platform"
)

// ErrClipboardUnsupported is reported by the Win32 clipboard until it is
// implemented with text editing in phase 5.
var ErrClipboardUnsupported = errors.New("win32: the clipboard is not supported yet; it arrives with text editing (phase 5)")

// Clipboard returns a clipboard whose methods report that it is not
// supported yet.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

// clipboard is a placeholder that honors the asynchronous contract: the
// ReadText callback runs later from the event loop, never from inside
// ReadText.
type clipboard struct{ b *Backend }

func (c clipboard) ReadText(done func(text string, err error)) {
	if done == nil {
		panic("win32: Clipboard.ReadText called with a nil callback")
	}
	c.b.loop.Post(func() { done("", ErrClipboardUnsupported) })
	c.b.notify()
}

func (c clipboard) WriteText(string) error { return ErrClipboardUnsupported }
