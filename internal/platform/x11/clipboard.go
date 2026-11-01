//go:build linux

package x11

import (
	"bytes"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// clipboard is the CLIPBOARD selection (selection.go). Writes own it with
// the backend's selection window and serve every entry from the event
// loop; reads convert it (TARGETS, then the best target of the type),
// answering through the loop. While the backend owns the selection,
// reads answer from its own data without a round trip through the
// server, still through the loop.
type clipboard struct{ b *Backend }

// post runs fn on the UI thread during a later delivery pass.
func (c clipboard) post(fn func()) {
	c.b.loop.Post(fn)
	c.b.signal()
}

// local returns the data the backend serves on CLIPBOARD, if it owns it.
func (c clipboard) local() (platform.Data, bool) {
	if o := c.b.integ.owned[c.b.atoms.clipboard]; o != nil {
		return o.data, true
	}
	return nil, false
}

func (c clipboard) ReadText(done func(text string, err error)) {
	if done == nil {
		panic("x11: Clipboard.ReadText called with a nil callback")
	}
	c.read(platform.MIMEText, func(data []byte, err error) { done(string(data), err) })
}

func (c clipboard) WriteText(text string) error { return c.Write(platform.TextData(text)) }

func (c clipboard) ReadTypes(done func(types []string, err error)) error {
	if done == nil {
		panic("x11: Clipboard.ReadTypes called with a nil callback")
	}
	if d, ok := c.local(); ok {
		types := d.Types()
		c.post(func() { done(types, nil) })
		return nil
	}
	b := c.b
	b.readTargets(b.atoms.clipboard, xproto.TimeCurrentTime, func(names []string, err error) {
		types := normalizeTargets(names)
		b.loop.Post(func() { done(types, err) })
	})
	return nil
}

func (c clipboard) Read(mime string, done func(data []byte, err error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	c.read(mime, done)
	return nil
}

// read reads one Data type: nil data and a nil error when the clipboard
// does not offer it.
func (c clipboard) read(mime string, done func([]byte, error)) {
	if d, ok := c.local(); ok {
		data, _ := d.Get(mime)
		data = bytes.Clone(data)
		c.post(func() { done(data, nil) })
		return
	}
	b := c.b
	sel := b.atoms.clipboard
	b.readTargets(sel, xproto.TimeCurrentTime, func(names []string, err error) {
		if err != nil || len(names) == 0 {
			b.loop.Post(func() { done(nil, err) })
			return
		}
		b.readType(sel, xproto.TimeCurrentTime, names, mime, func(data []byte, _ bool, err error) {
			b.loop.Post(func() { done(data, err) })
		})
	})
}

// Write owns CLIPBOARD with a copy of data. Empty data is owned too, so
// the clipboard reads as empty (TARGETS lists only the meta targets).
func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	b := c.b
	return b.own(b.atoms.clipboard, b.selectionWindow(), data.Clone())
}
