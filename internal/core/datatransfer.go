package core

import (
	"bytes"
	"image"
	"image/png"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The data of drag and drop and of the clipboard (ADR-209, ADR-211):
// HTML's DataTransfer over platform.Data.

// DropEffect is what a drop does with the data (HTML's dropEffect and
// effectAllowed): copy it, move it (the source deletes its copy) or link
// to it. As a set (DropCopy|DropMove) it is the effects a source allows.
type DropEffect = platform.DropEffect

// The drop effects.
const (
	// DropNone refuses a drop, or reports that a drag was refused or
	// cancelled.
	DropNone = platform.DropNone
	// DropCopy copies the data.
	DropCopy = platform.DropCopy
	// DropMove moves the data: the source deletes its copy.
	DropMove = platform.DropMove
	// DropLink links to the data.
	DropLink = platform.DropLink
	// DropAll is every effect (HTML's effectAllowed "all").
	DropAll = platform.DropAll
)

// The MIME types every platform maps onto its own formats; any other
// MIME type is a custom type ("application/x-myapp-card").
const (
	// MIMEText is UTF-8 plain text. "text/plain" and HTML's "text" name it
	// too.
	MIMEText = platform.MIMEText
	// MIMEHTML is an HTML fragment in UTF-8.
	MIMEHTML = platform.MIMEHTML
	// MIMEPNG is a PNG image.
	MIMEPNG = platform.MIMEPNG
	// MIMEURIList is a list of URIs, one per line: dragged files are file:
	// URIs (DataTransfer.Files). HTML's "url" and "Files" name it too.
	MIMEURIList = platform.MIMEURIList
)

// dtMode is what a DataTransfer allows.
type dtMode uint8

const (
	// dtWritable: a new DataTransfer (the zero value), or that of a
	// draggable element's OnDragStart: the setters work.
	dtWritable dtMode = iota
	// dtProtected: the events before a drop (dragenter, dragover,
	// dragleave): only the types are known (HTML's protected mode).
	dtProtected
	// dtReadOnly: the drop and the source's dragend: the data is known
	// and cannot change.
	dtReadOnly
)

// DataTransfer is the data of a drag and drop, as in HTML: the event
// handlers of a drag read it from DragEvent.DataTransfer. It holds the
// same content in one or more representations, each under a MIME type,
// best first (a rich text editor offers HTML, then plain text). The zero
// value is an empty DataTransfer that the setters fill.
//
// In OnDragStart of an h.Draggable element it is empty and writable: the
// handler puts the data in it (SetText, SetData, SetFiles...) and sets
// EffectAllowed. In OnDragEnter, OnDragOver and OnDragLeave only the
// types are known (Types, Has; HTML's protected mode): the getters return
// nothing until the drop. In OnDrop the data is there, and in the
// source's OnDragEnd DropEffect is the effect the target performed.
type DataTransfer struct {
	// DropEffect is the effect of the drop (HTML's dropEffect). Before
	// OnDragEnter and OnDragOver it holds the effect the platform's
	// convention picks from EffectAllowed and the modifiers held (Ctrl
	// copies, Shift moves, Ctrl+Shift links, else copy); a handler that
	// accepts the drop (PreventDefault in OnDragOver) may change it. In
	// OnDrop it is the accepted effect: setting DropNone tells the source
	// that the data was not taken (a moving source keeps it). In the
	// source's OnDragEnd it is the effect performed, DropNone for a drag
	// that was cancelled or refused.
	DropEffect DropEffect
	// EffectAllowed is the set of effects the source allows (HTML's
	// effectAllowed): OnDragStart starts it at DropAll and may narrow it
	// (tela.DropCopy|tela.DropMove); the target's events report it.
	EffectAllowed DropEffect

	data  platform.Data
	types []string // the offered types (dtProtected)
	mode  dtMode
	// look records the types the handlers of a drag over the window look
	// up before the drop (dndState.look), so the drop reads them.
	look func(t string)
	// The drag image (SetDragImage).
	img                image.Image
	imgSet             bool
	hotspotX, hotspotY int
}

// NewDataTransfer returns a writable DataTransfer holding a copy of data
// (package desktop's clipboard reads).
func NewDataTransfer(data platform.Data) *DataTransfer {
	return &DataTransfer{data: data.Clone(), mode: dtWritable}
}

// TransferData returns a copy of the data of d, nil for a nil d (package
// desktop's clipboard writes).
func TransferData(d *DataTransfer) platform.Data {
	if d == nil {
		return nil
	}
	return d.data.Clone()
}

// normType returns the MIME type a format names: HTML's "text" and
// "text/plain" (any charset spelled utf-8, or none) are MIMEText, "url"
// and "Files" are MIMEURIList; spaces are removed
// ("text/html; charset=utf-8"). Anything else is returned as is.
func normType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return MIMEText
	case "url", "files":
		return MIMEURIList
	}
	if t, ok := platform.SelectionType(format); ok {
		return t
	}
	return format
}

// wellKnown reports whether t is one of the types every drop reads.
func wellKnown(t string) bool {
	return t == MIMEText || t == MIMEHTML || t == MIMEPNG || t == MIMEURIList
}

// lookup normalizes format and, in protected mode, records that a handler
// looked it up.
func (d *DataTransfer) lookup(format string) string {
	t := normType(format)
	if d != nil && d.mode == dtProtected && d.look != nil {
		d.look(t)
	}
	return t
}

// Types returns the MIME types of the data, best first. Text is
// "text/plain;charset=utf-8" (MIMEText); compare with Has, which also
// takes "text/plain".
func (d *DataTransfer) Types() []string {
	if d == nil {
		return nil
	}
	if d.mode == dtProtected {
		return slices.Clone(d.types)
	}
	return d.data.Types()
}

// Has reports whether the data has a representation of the MIME type:
// "text/plain" (or "text"), "text/html", "image/png", "text/uri-list" (or
// "Files", "url"), or a custom type. It works before the drop too, which
// is how OnDragOver decides whether to accept a drag:
//
//	OnDragOver(func(e tela.DragEvent) {
//	    if e.DataTransfer.Has("Files") {
//	        e.PreventDefault() // accept the drop
//	    }
//	})
//
// A custom type a handler asks about before the drop is read at the drop,
// besides the well-known types.
func (d *DataTransfer) Has(mime string) bool {
	if d == nil {
		return false
	}
	t := d.lookup(mime)
	if d.mode == dtProtected {
		return slices.Contains(d.types, t)
	}
	return d.data.Has(t)
}

// Get returns the bytes of the representation of a MIME type (see Has
// for the names), and whether there is one. Before the drop it returns
// nothing. The caller must not modify the bytes.
func (d *DataTransfer) Get(mime string) ([]byte, bool) {
	if d == nil {
		return nil, false
	}
	t := d.lookup(mime)
	if d.mode == dtProtected {
		return nil, false
	}
	return d.data.Get(t)
}

// GetData returns the representation of a format as a string, like HTML's
// getData: "text" or "text/plain" is the text, "url" the first URI of the
// URI list, any other MIME type its bytes. It is "" when there is none,
// and before the drop.
func (d *DataTransfer) GetData(format string) string {
	b, ok := d.Get(format)
	if !ok {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(format), "url") {
		if uris := platform.ParseURIList(b); len(uris) > 0 {
			return uris[0]
		}
		return ""
	}
	return string(b)
}

// Text returns the plain text, "" when there is none.
func (d *DataTransfer) Text() string { return d.GetData(MIMEText) }

// HTML returns the HTML fragment, "" when there is none.
func (d *DataTransfer) HTML() string { return d.GetData(MIMEHTML) }

// Files returns the absolute paths of the files, in order: the local
// file: URIs of the URI list (other URIs, such as a link dragged from a
// browser, are left out; GetData("url") returns the first URI).
func (d *DataTransfer) Files() []string {
	b, ok := d.Get(MIMEURIList)
	if !ok {
		return nil
	}
	paths, _ := platform.Data{{Type: MIMEURIList, Bytes: b}}.Paths()
	return paths
}

// Image returns the PNG image, decoded; ok is false when there is none or
// it does not decode.
func (d *DataTransfer) Image() (img image.Image, ok bool) {
	b, has := d.Get(MIMEPNG)
	if !has {
		return nil, false
	}
	img, err := png.Decode(bytes.NewReader(b))
	return img, err == nil
}

// writable panics with an actionable message unless the setters of d may
// be called.
func (d *DataTransfer) writable(what string) {
	switch {
	case d == nil:
		panic("tela: DataTransfer." + what + " on a nil DataTransfer: DragEvent.DataTransfer is nil in the drag gesture of an element without h.Draggable(true); add h.Draggable(true) to drag the element's data")
	case d.mode == dtProtected:
		panic("tela: DataTransfer." + what + " in OnDragEnter, OnDragOver or OnDragLeave: the data of a drag is set by its source, in OnDragStart of an h.Draggable element; a target only reads it, in OnDrop")
	case d.mode == dtReadOnly:
		panic("tela: DataTransfer." + what + " after the drag started: set the data in OnDragStart of the h.Draggable element; OnDrop and OnDragEnd only read it (set DropEffect instead)")
	}
}

// Set puts the bytes of a MIME type in the data, replacing the bytes of
// that type, else after the other types (the order is best first). The
// names of Has work ("text/plain" is MIMEText). b is used as is: do not
// modify it afterwards. A type that is not a MIME type panics.
func (d *DataTransfer) Set(mime string, b []byte) {
	d.writable("Set")
	t := normType(mime)
	platform.CheckMIME("DataTransfer.Set", t)
	for i := range d.data {
		if d.data[i].Type == t {
			d.data[i].Bytes = b
			return
		}
	}
	d.data = append(d.data, platform.DataEntry{Type: t, Bytes: b})
}

// SetData puts a string under a format, like HTML's setData: "text" or
// "text/plain" is the text, "url" a URI list of one URI, any other MIME
// type takes the string's bytes.
//
//	OnDragStart(func(e tela.DragEvent) {
//	    e.DataTransfer.SetData("application/x-kanban-card", card.ID)
//	    e.DataTransfer.SetText(card.Title) // for other applications
//	    e.DataTransfer.EffectAllowed = tela.DropMove
//	})
func (d *DataTransfer) SetData(format, data string) {
	d.writable("SetData")
	if strings.EqualFold(strings.TrimSpace(format), "url") {
		d.Set(MIMEURIList, platform.URIList([]string{data}))
		return
	}
	d.Set(format, []byte(data))
}

// SetText puts plain text in the data.
func (d *DataTransfer) SetText(s string) {
	d.writable("SetText")
	d.Set(MIMEText, []byte(s))
}

// SetHTML puts an HTML fragment in the data; add SetText for the
// applications that read only text.
func (d *DataTransfer) SetHTML(s string) {
	d.writable("SetHTML")
	d.Set(MIMEHTML, []byte(s))
}

// SetFiles puts files in the data, as a URI list of their absolute
// paths: a file manager, or any application that takes files, receives
// them. It returns an error for an empty list or a path that is not
// absolute.
func (d *DataTransfer) SetFiles(paths ...string) error {
	d.writable("SetFiles")
	fd, err := platform.FileData(paths...)
	if err != nil {
		return err
	}
	d.Set(MIMEURIList, fd[0].Bytes)
	return nil
}

// SetImage puts an image in the data, encoded as PNG. It returns the
// encoder's error.
func (d *DataTransfer) SetImage(img image.Image) error {
	d.writable("SetImage")
	if img == nil {
		panic("tela: DataTransfer.SetImage(nil): pass the image to put in the data")
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	d.Set(MIMEPNG, b.Bytes())
	return nil
}

// ClearData removes the representations of the formats given (see Has
// for the names), or all of them when none is given.
func (d *DataTransfer) ClearData(formats ...string) {
	d.writable("ClearData")
	if len(formats) == 0 {
		d.data = nil
		return
	}
	for _, f := range formats {
		t := normType(f)
		d.data = slices.DeleteFunc(d.data, func(e platform.DataEntry) bool { return e.Type == t })
	}
}

// SetDragImage sets the picture shown under the pointer during the drag
// (HTML's setDragImage), in place of the element's own: img in device
// pixels, and (x, y) the point of img under the pointer. A nil img shows
// no picture, only the platform's cursor. Without SetDragImage the drag
// shows the element as it is on screen. Call it in OnDragStart; an image
// larger than 1024 pixels on a side is cut to the 1024 around the point.
func (d *DataTransfer) SetDragImage(img image.Image, x, y int) {
	d.writable("SetDragImage")
	d.img, d.imgSet, d.hotspotX, d.hotspotY = img, true, x, y
}
