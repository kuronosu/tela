package desktop

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// The clipboard (ADR-209, ADR-213): the same tela.DataTransfer as drag
// and drop, so text, HTML, PNG images, files and custom types copy and
// paste between Tela and other applications. Text fields copy and paste
// by themselves; these functions are for the rest of an app (a "Copy"
// button, pasting an image or files).

// WriteClipboard replaces the clipboard's content with dt's, every
// representation of it, so another application pastes the one it
// understands:
//
//	var dt tela.DataTransfer
//	dt.SetHTML("<b>Groceries</b>: milk, eggs")
//	dt.SetText("Groceries: milk, eggs")
//	err := desktop.WriteClipboard(&dt)
//
// A nil or empty dt empties the clipboard. Call it on the UI thread. It
// returns an error when no app runs, or wrapping [ErrUnsupported] on a
// desktop whose clipboard takes only text (use [WriteClipboardText]).
func WriteClipboard(dt *tela.DataTransfer) error {
	d, err := running("WriteClipboard")
	if err != nil {
		return err
	}
	data := core.TransferData(dt)
	platform.CheckData("desktop.WriteClipboard", data)
	return wrap("WriteClipboard", d.Clipboard().Write(data))
}

// WriteClipboardText puts text on the clipboard, replacing its content.
// Call it on the UI thread.
func WriteClipboardText(text string) error {
	d, err := running("WriteClipboardText")
	if err != nil {
		return err
	}
	return wrap("WriteClipboardText", d.Clipboard().WriteText(text))
}

// ReadClipboard reads the clipboard and calls done, later, on the UI
// thread, with a DataTransfer holding what it read: the given types that
// the clipboard offers, or, with no types, the well-known ones it offers
// (text, HTML, a PNG image, files). Types are MIME types or HTML's
// names ("text", "Files", "url"). An empty clipboard gives an empty
// DataTransfer.
//
//	desktop.ReadClipboard(func(dt *tela.DataTransfer, err error) {
//	    switch {
//	    case err != nil:
//	    case len(dt.Files()) > 0:
//	        attach(dt.Files())
//	    case dt.Has("image/png"):
//	        img, _ := dt.Image()
//	        insertImage(img)
//	    default:
//	        insertText(dt.Text())
//	    }
//	})
//
// The DataTransfer is the caller's: it may keep and change it. Call it on
// the UI thread; done runs exactly once unless ReadClipboard returns an
// error (no running app, or [ErrUnsupported] on a desktop whose clipboard
// has only text: use [ReadClipboardText]). Tests preset the clipboard
// with telatest's App.SetClipboard.
func ReadClipboard(done func(dt *tela.DataTransfer, err error), types ...string) error {
	if done == nil {
		panic("desktop.ReadClipboard(nil): pass the function that receives the content, such as desktop.ReadClipboard(func(dt *tela.DataTransfer, err error) { ... })")
	}
	want := make([]string, len(types))
	for i, t := range types {
		want[i] = core.NormDataType(t)
		platform.CheckMIME("desktop.ReadClipboard", want[i])
	}
	d, err := running("ReadClipboard")
	if err != nil {
		return err
	}
	clip := d.Clipboard()
	err = clip.ReadTypes(func(offered []string, err error) {
		if err != nil {
			done(nil, wrap("ReadClipboard", err))
			return
		}
		readTypes(clip, platform.DropTypes(offered, want), nil, done)
	})
	return wrap("ReadClipboard", err)
}

// readTypes reads the types one after the other into data, then calls
// done.
func readTypes(clip platform.Clipboard, types []string, data platform.Data, done func(*tela.DataTransfer, error)) {
	if len(types) == 0 {
		done(core.NewDataTransfer(data), nil)
		return
	}
	t := types[0]
	err := clip.Read(t, func(b []byte, err error) {
		if err != nil {
			done(nil, wrap("ReadClipboard", err))
			return
		}
		if b != nil { // nil: the clipboard changed and no longer has t
			data = append(data, platform.DataEntry{Type: t, Bytes: b})
		}
		readTypes(clip, types[1:], data, done)
	})
	if err != nil {
		done(nil, wrap("ReadClipboard", err))
	}
}

// ClipboardTypes calls done, later, on the UI thread, with the types the
// clipboard offers (MIME types, best first), without reading the data:
// enough to enable a Paste button. Same rules as [ReadClipboard].
func ClipboardTypes(done func(types []string, err error)) error {
	if done == nil {
		panic("desktop.ClipboardTypes(nil): pass the function that receives the types")
	}
	d, err := running("ClipboardTypes")
	if err != nil {
		return err
	}
	return wrap("ClipboardTypes", d.Clipboard().ReadTypes(func(types []string, err error) {
		done(types, wrap("ClipboardTypes", err))
	}))
}

// ReadClipboardText reads the clipboard's text and calls done, later, on
// the UI thread ("" when it holds no text). It works on every desktop.
func ReadClipboardText(done func(text string, err error)) error {
	if done == nil {
		panic("desktop.ReadClipboardText(nil): pass the function that receives the text")
	}
	d, err := running("ReadClipboardText")
	if err != nil {
		return err
	}
	d.Clipboard().ReadText(func(s string, err error) { done(s, wrap("ReadClipboardText", err)) })
	return nil
}
