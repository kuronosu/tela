//go:build darwin

package cocoa

import "github.com/kuronosu/tela/internal/platform"

// Clipboard returns the clipboard. The base backend has none yet (the text
// clipboard, NSPasteboard, is a task of phase 14; its other formats are
// macOS desktop integration): writes and requests fail with ErrUnsupported
// after the same argument checks as every backend.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

// Settings returns the default system settings; reading them (dark mode,
// accent color, reduced motion, contrast) is a task of phase 14.
func (b *Backend) Settings() platform.SystemSettings { return platform.SystemSettings{} }

// Desktop returns the desktop services: none yet (macOS desktop
// integration).
func (b *Backend) Desktop() platform.Desktop { return platform.UnsupportedDesktop{} }

type clipboard struct{ b *Backend }

// ReadText answers ErrUnsupported from the loop, never from inside the
// call.
func (c clipboard) ReadText(done func(text string, err error)) {
	if done == nil {
		panic("cocoa: Clipboard.ReadText called with a nil callback")
	}
	c.b.loop.Post(func() { done("", platform.ErrUnsupported) })
	c.b.notify()
}

func (c clipboard) WriteText(string) error { return platform.ErrUnsupported }

func (c clipboard) ReadTypes(done func(types []string, err error)) error {
	if done == nil {
		panic("cocoa: Clipboard.ReadTypes called with a nil callback")
	}
	return platform.ErrUnsupported
}

func (c clipboard) Read(mime string, done func(data []byte, err error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	return platform.ErrUnsupported
}

func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	return platform.ErrUnsupported
}
