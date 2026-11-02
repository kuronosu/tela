//go:build darwin

package cocoa

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Clipboard returns the clipboard: NSPasteboard.generalPasteboard, with
// the type mapping of pasteboard.go. The pasteboard server keeps a copy of
// what is written (nothing is promised or rendered later), so written data
// is served to other apps without the event loop, and outlives the app.
// Reads always go to the pasteboard, our own data included, and answer
// from the loop, never from inside the call; another app's write replaces
// ours at once (the pasteboard's changeCount counts the writes).
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

type clipboard struct{ b *Backend }

var _ platform.Clipboard = clipboard{}

// pbSel holds the selectors of the pasteboard code.
var pbSel = struct {
	generalPasteboard, changeCount, clearContents, types, pasteboardItems  darwin.SEL
	writeObjects, dataForType, stringForType, setDataForType, setStringFor darwin.SEL
	count, objectAtIndex, array, addObject                                 darwin.SEL
	typeWithMIMEType, typeWithIdentifier, identifier, preferredMIMEType    darwin.SEL
	urlWithString, filePathURL, absoluteString                             darwin.SEL
}{
	generalPasteboard:  darwin.Sel("generalPasteboard"),
	changeCount:        darwin.Sel("changeCount"),
	clearContents:      darwin.Sel("clearContents"),
	types:              darwin.Sel("types"),
	pasteboardItems:    darwin.Sel("pasteboardItems"),
	writeObjects:       darwin.Sel("writeObjects:"),
	dataForType:        darwin.Sel("dataForType:"),
	stringForType:      darwin.Sel("stringForType:"),
	setDataForType:     darwin.Sel("setData:forType:"),
	setStringFor:       darwin.Sel("setString:forType:"),
	count:              darwin.Sel("count"),
	objectAtIndex:      darwin.Sel("objectAtIndex:"),
	array:              darwin.Sel("array"),
	addObject:          darwin.Sel("addObject:"),
	typeWithMIMEType:   darwin.Sel("typeWithMIMEType:"),
	typeWithIdentifier: darwin.Sel("typeWithIdentifier:"),
	identifier:         darwin.Sel("identifier"),
	preferredMIMEType:  darwin.Sel("preferredMIMEType"),
	urlWithString:      darwin.Sel("URLWithString:"),
	filePathURL:        darwin.Sel("filePathURL"),
	absoluteString:     darwin.Sel("absoluteString"),
}

var errNoPasteboard = errors.New("cocoa: NSPasteboard.generalPasteboard is nil (no pasteboard server?)")

// generalPasteboard returns the general pasteboard (borrowed).
func generalPasteboard() darwin.ID {
	return darwin.SendClass(darwin.GetClass("NSPasteboard"), pbSel.generalPasteboard)
}

// ReadText reads the pasteboard's text from the event loop ("" when it
// holds none).
func (c clipboard) ReadText(done func(text string, err error)) {
	if done == nil {
		panic("cocoa: Clipboard.ReadText called with a nil callback; pass a func(text string, err error)")
	}
	c.post(func() {
		b, err := readPasteboard(platform.MIMEText)
		done(string(b), err)
	})
}

// WriteText makes text the pasteboard's only content:
// Write(TextData(text)).
func (c clipboard) WriteText(text string) error {
	return writePasteboard(platform.TextData(text))
}

// ReadTypes reports the pasteboard's types, normalized, from the loop.
func (c clipboard) ReadTypes(done func(types []string, err error)) error {
	if done == nil {
		panic("cocoa: Clipboard.ReadTypes called with a nil callback; pass a func(types []string, err error)")
	}
	c.post(func() {
		types, err := pasteboardTypes()
		done(types, err)
	})
	return nil
}

// Read reads the pasteboard's content of one type from the loop: nil and
// no error when it is not offered.
func (c clipboard) Read(mime string, done func(data []byte, err error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	c.post(func() {
		b, err := readPasteboard(mime)
		done(b, err)
	})
	return nil
}

// Write replaces the pasteboard's content with data (clearContents, then
// writeObjects: of the items of pasteboardItems); empty data clears it.
func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	return writePasteboard(data)
}

// post runs fn on the UI thread from a later delivery pass.
func (c clipboard) post(fn func()) {
	c.b.loop.Post(func() {
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		fn()
	})
	c.b.notify()
}

// --- writing ---

func writePasteboard(d platform.Data) error {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	pb := generalPasteboard()
	if pb == 0 {
		return errNoPasteboard
	}
	items := pasteboardItems(d, utiForMIME)
	darwin.Send(pb, pbSel.clearContents)
	if len(items) == 0 {
		return nil
	}
	array := darwin.SendClass(darwin.GetClass("NSMutableArray"), pbSel.array)
	for _, it := range items {
		item, err := newPasteboardItem(it)
		if err != nil {
			return err
		}
		darwin.Send(array, pbSel.addObject, item)
		darwin.Release(item) // the array keeps it
	}
	if !darwin.SendBool(pb, pbSel.writeObjects, array) {
		return errors.New("cocoa: NSPasteboard writeObjects: failed (another app took the pasteboard meanwhile?)")
	}
	return nil
}

// newPasteboardItem returns a new NSPasteboardItem (+1) holding the
// entries of it: an item of the clipboard, or of a drag (dnd_darwin.go).
func newPasteboardItem(it pbItem) (darwin.ID, error) {
	item := darwin.Send(darwin.SendClass(darwin.GetClass("NSPasteboardItem"), sel.alloc), sel.init)
	if item == 0 {
		return 0, errors.New("cocoa: creating an NSPasteboardItem failed")
	}
	for _, e := range it {
		typ := darwin.NSString(e.uti)
		var ok bool
		if e.text {
			s := darwin.NSStringBytes(e.data)
			ok = darwin.SendBool(item, pbSel.setStringFor, s, typ)
			darwin.Release(s)
		} else {
			data := darwin.NSData(e.data)
			ok = darwin.SendBool(item, pbSel.setDataForType, data, typ)
			darwin.Release(data)
		}
		darwin.Release(typ)
		if !ok {
			darwin.Release(item)
			return 0, fmt.Errorf("cocoa: NSPasteboardItem refused %d bytes of type %s", len(e.data), e.uti)
		}
	}
	return item, nil
}

// utiForMIME returns the pasteboard type of a custom MIME type: its UTType
// (declared or dynamic), else customType.
func utiForMIME(mime string) string {
	if needsCustomType(mime) || !darwin.LoadUniformTypeIdentifiers() {
		return customType(mime)
	}
	m := darwin.NSString(mime)
	defer darwin.Release(m)
	t := darwin.SendClass(darwin.GetClass("UTType"), pbSel.typeWithMIMEType, m)
	if t == 0 {
		return customType(mime)
	}
	if id := darwin.GoString(darwin.Send(t, pbSel.identifier)); id != "" {
		return id
	}
	return customType(mime)
}

// mimeForUTI returns the MIME type of a pasteboard type that is not well
// known: a customType identifier's, else the UTType's preferred MIME type
// (normalized like a Linux target: "text/plain" is MIMEText). Types
// without one (private Apple types, legacy names) are not data.
func mimeForUTI(uti string) (string, bool) {
	if m, ok := customMIME(uti); ok {
		return m, true
	}
	if !darwin.LoadUniformTypeIdentifiers() {
		return "", false
	}
	s := darwin.NSString(uti)
	defer darwin.Release(s)
	t := darwin.SendClass(darwin.GetClass("UTType"), pbSel.typeWithIdentifier, s)
	if t == 0 {
		return "", false
	}
	m := darwin.GoString(darwin.Send(t, pbSel.preferredMIMEType))
	if m == "" {
		return "", false
	}
	return platform.SelectionType(m)
}

// --- reading ---

// stringsOf returns the NSStrings of an NSArray.
func stringsOf(array darwin.ID) []string {
	if array == 0 {
		return nil
	}
	n := darwin.SendInt(array, pbSel.count)
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, darwin.GoString(darwin.Send(array, pbSel.objectAtIndex, uint(i))))
	}
	return out
}

// items returns the pasteboard's NSPasteboardItems (borrowed).
func items(pb darwin.ID) []darwin.ID {
	array := darwin.Send(pb, pbSel.pasteboardItems)
	if array == 0 {
		return nil
	}
	n := darwin.SendInt(array, pbSel.count)
	out := make([]darwin.ID, 0, n)
	for i := range n {
		out = append(out, darwin.Send(array, pbSel.objectAtIndex, uint(i)))
	}
	return out
}

// pasteboardTypes returns the general pasteboard's Data types, item by
// item.
func pasteboardTypes() ([]string, error) {
	pb := generalPasteboard()
	if pb == 0 {
		return nil, errNoPasteboard
	}
	return typesOf(pb), nil
}

// typesOf returns the Data types of the pasteboard pb (the general one, or
// a drag's), item by item.
func typesOf(pb darwin.ID) []string {
	var lists [][]string
	for _, it := range items(pb) {
		lists = append(lists, stringsOf(darwin.Send(it, pbSel.types)))
	}
	return offeredTypes(lists, mimeForUTI)
}

// readPasteboard returns the general pasteboard's content of mime (nil,
// nil when it is not offered).
func readPasteboard(mime string) ([]byte, error) {
	pb := generalPasteboard()
	if pb == 0 {
		return nil, errNoPasteboard
	}
	return readFrom(pb, mime)
}

// readFrom returns the content of mime on the pasteboard pb (the general
// one, or a drag's): nil, nil when it is not offered.
func readFrom(pb darwin.ID, mime string) ([]byte, error) {
	if mime == platform.MIMEURIList {
		return readURIList(pb), nil
	}
	var preferred []string
	switch mime {
	case platform.MIMEText, platform.MIMEHTML, platform.MIMEPNG:
	default:
		preferred = []string{utiForMIME(mime), customType(mime)}
	}
	offered := stringsOf(darwin.Send(pb, pbSel.types))
	var firstErr error
	for _, uti := range typesFor(mime, offered, preferred, mimeForUTI) {
		typ := darwin.NSString(uti)
		var b []byte
		if isStringType(uti) {
			if s := darwin.Send(pb, pbSel.stringForType, typ); s != 0 {
				b = darwin.StringBytes(s)
			}
		} else if data := darwin.Send(pb, pbSel.dataForType, typ); data != 0 {
			b = darwin.Bytes(data)
			if b == nil {
				b = []byte{}
			}
		}
		darwin.Release(typ)
		if b == nil {
			continue // vanished, or not convertible: try the next type
		}
		if isTIFF(uti) && mime == platform.MIMEPNG {
			png, err := pngFromTIFF(b)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			b = png
		}
		return b, nil
	}
	return nil, firstErr
}

// readURIList returns the URIs of the pasteboard's items as a
// text/uri-list: each item's public.file-url (file reference URLs
// resolved to path URLs), else its public.url. nil when there are none.
func readURIList(pb darwin.ID) []byte {
	fileURL, url := darwin.NSString(utiFileURL), darwin.NSString(utiURL)
	defer darwin.Release(fileURL)
	defer darwin.Release(url)
	var uris []string
	for _, it := range items(pb) {
		u := darwin.GoString(darwin.Send(it, pbSel.stringForType, fileURL))
		if u != "" && isFileReferenceURL(u) {
			u = resolveFileReference(u)
		}
		if u == "" {
			u = darwin.GoString(darwin.Send(it, pbSel.stringForType, url))
		}
		if u != "" {
			uris = append(uris, u)
		}
	}
	if len(uris) == 0 {
		return nil
	}
	return platform.URIList(uris)
}

// resolveFileReference turns a file reference URL into the path URL of
// the file it names (NSURL filePathURL), or returns it unchanged when the
// file is gone.
func resolveFileReference(u string) string {
	s := darwin.NSString(u)
	defer darwin.Release(s)
	nsURL := darwin.SendClass(darwin.GetClass("NSURL"), pbSel.urlWithString, s)
	if nsURL == 0 {
		return u
	}
	path := darwin.Send(nsURL, pbSel.filePathURL)
	if path == 0 {
		return u
	}
	if r := darwin.GoString(darwin.Send(path, pbSel.absoluteString)); r != "" {
		return r
	}
	return u
}
