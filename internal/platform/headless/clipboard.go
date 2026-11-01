package headless

import (
	"bytes"

	"github.com/kuronosu/tela/internal/platform"
)

// Clipboard returns the in-memory clipboard. It holds platform.Data: text
// is its MIMEText entry. Reads answer through the loop, as on every
// backend. ServiceClipboard in SetUnsupported makes ReadTypes, Read and
// Write return platform.ErrUnsupported (a backend with text only);
// ReadText and WriteText always work.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

// ClipboardData returns a copy of the clipboard's content.
func (b *Backend) ClipboardData() platform.Data {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.clip.Clone()
}

// SetClipboardData puts d on the clipboard as another application would
// (a copy in a file manager or a browser). Invalid data panics. Safe from
// any goroutine.
func (b *Backend) SetClipboardData(d platform.Data) {
	platform.CheckData("SetClipboardData", d)
	d = d.Clone()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clip = d
}

// clipboard is the in-memory clipboard.
type clipboard struct{ b *Backend }

// post runs fn on the UI thread from the next loop iteration.
func (c clipboard) post(fn func()) {
	c.b.mu.Lock()
	c.b.loop.Post(fn)
	c.b.mu.Unlock()
	c.b.signal()
}

func (c clipboard) ReadText(done func(string, error)) {
	if done == nil {
		panic("headless: Clipboard.ReadText called with a nil callback")
	}
	c.post(func() {
		c.b.mu.Lock()
		s, _ := c.b.clip.Text()
		c.b.mu.Unlock()
		done(s, nil)
	})
}

func (c clipboard) WriteText(s string) error {
	c.b.mu.Lock()
	c.b.clip = platform.TextData(s)
	c.b.mu.Unlock()
	return nil
}

func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("headless: Clipboard.ReadTypes called with a nil callback")
	}
	if !c.b.supports(ServiceClipboard) {
		return platform.ErrUnsupported
	}
	c.post(func() {
		c.b.mu.Lock()
		types := c.b.clip.Types()
		c.b.mu.Unlock()
		done(types, nil)
	})
	return nil
}

func (c clipboard) Read(mime string, done func([]byte, error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	if !c.b.supports(ServiceClipboard) {
		return platform.ErrUnsupported
	}
	c.post(func() {
		c.b.mu.Lock()
		data, _ := c.b.clip.Get(mime)
		data = bytes.Clone(data)
		c.b.mu.Unlock()
		done(data, nil)
	})
	return nil
}

func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	if !c.b.supports(ServiceClipboard) {
		return platform.ErrUnsupported
	}
	data = data.Clone()
	c.b.mu.Lock()
	c.b.clip = data
	c.b.mu.Unlock()
	return nil
}
