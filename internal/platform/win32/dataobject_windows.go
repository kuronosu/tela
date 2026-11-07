//go:build windows

package win32

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// platform.Data on Win32, in the formats dataformat.go maps it to: the
// clipboard (OpenClipboard, EnumClipboardFormats, GetClipboardData,
// SetClipboardData with GlobalAlloc memory) and the IDataObject of OLE
// drag and drop, read from a drag's source and served for this app's
// drags.

// FORMATETC and STGMEDIUM constants (objidl.h).
const (
	tymedHGlobal    = 1
	tymedIStream    = 4
	dvaspectContent = 1
	datadirGet      = 1
)

// IDataObject, IEnumFORMATETC and ISequentialStream vtable indices.
const (
	vtDataObjectGetData       = 3
	vtDataObjectEnumFormatEtc = 8
	vtEnumFormatEtcNext       = 3
	vtStreamRead              = 3
)

// formatEtc is FORMATETC. Go lays it out as C does on 32 and 64 bits.
type formatEtc struct {
	cf     uint16
	ptd    uintptr // DVTARGETDEVICE*
	aspect uint32
	lindex int32
	tymed  uint32
}

// stgMedium is STGMEDIUM: the union is one pointer-sized field.
type stgMedium struct {
	tymed         uint32
	data          uintptr // HGLOBAL, IStream*…
	unkForRelease uintptr
}

// --- registered formats ---

// clipFormats caches the registered formats' IDs and names (they are the
// same for the whole session).
var clipFormats = struct {
	sync.Mutex
	ids   map[string]uint32
	names map[uint32]string
}{ids: map[string]uint32{}, names: map[uint32]string{}}

// registerFormat returns the ID of the format registered under name (0
// when RegisterClipboardFormatW fails).
func registerFormat(name string) uint32 {
	clipFormats.Lock()
	defer clipFormats.Unlock()
	if id, ok := clipFormats.ids[name]; ok {
		return id
	}
	r, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(utf16(name))))
	id := uint32(r)
	if id != 0 {
		clipFormats.ids[name], clipFormats.names[id] = id, name
	}
	return id
}

// clipFormatOf returns the format with its name, for a registered one.
func clipFormatOf(id uint32) clipFormat {
	if id < cfFirstRegistered || id > 0xFFFF {
		return clipFormat{id: id}
	}
	clipFormats.Lock()
	defer clipFormats.Unlock()
	name, ok := clipFormats.names[id]
	if !ok {
		var buf [256]uint16
		n, _, _ := procGetClipboardFormatNameW.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		name = fromUTF16Clipboard(buf[:n])
		if name != "" {
			clipFormats.names[id] = name
			clipFormats.ids[name] = id
		}
	}
	return clipFormat{id: id, name: name}
}

// formatID returns the ID of an encoded format, registering its name.
func formatID(f encodedFormat) (uint32, error) {
	if f.id != 0 {
		return f.id, nil
	}
	if id := registerFormat(f.name); id != 0 {
		return id, nil
	}
	return 0, fmt.Errorf("win32: RegisterClipboardFormatW(%q) failed", f.name)
}

// --- global memory ---

// globalFromBytes returns movable global memory holding b (at least one
// byte: GlobalAlloc of zero bytes returns a discarded block).
func globalFromBytes(b []byte) (uintptr, error) {
	h, _, e := procGlobalAlloc.Call(gmemMoveable, uintptr(max(len(b), 1)))
	if h == 0 {
		return 0, winError("GlobalAlloc", e)
	}
	if len(b) > 0 {
		p, _, e := procGlobalLock.Call(h)
		if p == 0 {
			procGlobalFree.Call(h)
			return 0, winError("GlobalLock", e)
		}
		copy(unsafe.Slice((*byte)(ptrFromUintptr(p)), len(b)), b)
		procGlobalUnlock.Call(h)
	}
	return h, nil
}

// bytesFromGlobal returns a copy of the memory of h.
func bytesFromGlobal(h uintptr) ([]byte, error) {
	size, _, _ := procGlobalSize.Call(h)
	p, _, e := procGlobalLock.Call(h)
	if p == 0 {
		return nil, winError("GlobalLock", e)
	}
	defer procGlobalUnlock.Call(h)
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(ptrFromUintptr(p)), size))
	return out, nil
}

// duplicateGlobal returns a copy of global memory.
func duplicateGlobal(h uintptr) (uintptr, error) {
	b, err := bytesFromGlobal(h)
	if err != nil {
		return 0, err
	}
	return globalFromBytes(b)
}

// --- reading another program's IDataObject ---

// dataObjectFormats returns the formats an IDataObject offers in global
// memory or a stream, in its order.
func dataObjectFormats(obj comObject) []clipFormat { return enumFormats(obj, nil) }

// enumFormats is dataObjectFormats under a guard (nil: none), which cuts it
// short when the source stops answering (stallguard.go).
func enumFormats(obj comObject, g *stallGuard) []clipFormat {
	var enum comObject
	hr := obj.call(vtDataObjectEnumFormatEtc, datadirGet, uintptr(unsafe.Pointer(&enum)))
	g.progress()
	if hr < 0 || enum == 0 {
		return nil
	}
	defer enum.release()
	var out []clipFormat
	for range 1024 { // a bound against a broken enumerator
		var fe formatEtc
		var fetched uint32
		hr := enum.call(vtEnumFormatEtcNext, 1, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&fetched)))
		g.progress()
		if hr != sOK || fetched == 0 || g.Stalled() {
			break
		}
		if fe.ptd != 0 {
			procCoTaskMemFree.Call(fe.ptd)
		}
		if fe.aspect != dvaspectContent || fe.tymed&(tymedHGlobal|tymedIStream) == 0 {
			continue
		}
		out = append(out, clipFormatOf(uint32(fe.cf)))
	}
	return out
}

// getData reads one format of an IDataObject, from global memory or a
// stream.
func getData(obj comObject, f clipFormat) ([]byte, error) { return getDataGuarded(obj, f, nil) }

// getDataGuarded is getData under a guard (nil: none).
func getDataGuarded(obj comObject, f clipFormat, g *stallGuard) ([]byte, error) {
	fe := formatEtc{cf: uint16(f.id), aspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal | tymedIStream}
	var med stgMedium
	hr := obj.call(vtDataObjectGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&med)))
	g.progress()
	if hr < 0 {
		return nil, hresultError("IDataObject.GetData", hr)
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&med)))
	switch med.tymed {
	case tymedHGlobal:
		return bytesFromGlobal(med.data)
	case tymedIStream:
		return readStream(comObject(med.data), g)
	}
	return nil, fmt.Errorf("win32: IDataObject.GetData returned storage medium %d", med.tymed)
}

// maxStreamRead bounds what is read from a stream (a source that never
// ends one must not exhaust memory).
const maxStreamRead = 1 << 30

// readStream reads an IStream from its current position to its end. Each
// chunk that arrives restarts the guard (nil: none).
func readStream(s comObject, g *stallGuard) ([]byte, error) {
	var out []byte
	buf := make([]byte, 64<<10)
	for len(out) < maxStreamRead {
		var n uint32
		hr := s.call(vtStreamRead, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)))
		g.progress()
		if hr < 0 {
			return nil, hresultError("IStream.Read", hr)
		}
		out = append(out, buf[:n]...)
		if n == 0 || hr != sOK { // S_FALSE: the end
			break
		}
	}
	return out, nil
}

// readDataObject returns the content of the given types from an
// IDataObject offering formats: for each type, the first of its formats
// that reads and converts (a type that fails is left out).
func readDataObject(obj comObject, formats []clipFormat, types []string) platform.Data {
	d, _ := readDataObjectGuarded(obj, formats, types, nil)
	return d
}

// readDataObjectGuarded is readDataObject under a guard (nil: none). When
// the guard cancels a call because the source stopped answering, the read
// stops and returns errDropStalled with no data: a drop has all the data it
// asked for or none.
func readDataObjectGuarded(obj comObject, formats []clipFormat, types []string, g *stallGuard) (platform.Data, error) {
	var d platform.Data
	for _, t := range types {
		for _, f := range formatsFor(t, formats) {
			raw, err := getDataGuarded(obj, f, g)
			if g.Stalled() {
				return nil, errDropStalled()
			}
			if err != nil {
				continue
			}
			if b, err := decodeFormat(f, raw); err == nil {
				d = append(d, platform.DataEntry{Type: t, Bytes: b})
				break
			}
		}
	}
	return d, nil
}

// --- this app's IDataObject ---

// iidTelaDataObject is answered only by this backend's data objects, so a
// drop target recognizes a drag of this app (DragEvent.FromApp) and reads
// its Data as it is.
var (
	iidDataObject     = guid{0x0000010E, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidTelaDataObject = guid{0x6A0C1F3E, 0x5B7D, 0x4C2A, [8]byte{0x9E, 0x41, 0x7D, 0x3B, 0x22, 0x8F, 0x51, 0xC6}}
)

// dataObjectServer is the IDataObject of a drag of this app: its Data in
// the formats of encodeData (rendered when the drag starts), plus what
// other code stores in it with SetData (the shell's drag image helper
// keeps the image there, and shell targets their drop descriptions and
// "Performed DropEffect").
type dataObjectServer struct {
	data    platform.Data
	formats []dataObjectFormat
	stored  []storedMedium
}

type dataObjectFormat struct {
	id   uint32
	data []byte
}

type storedMedium struct {
	fe  formatEtc
	med stgMedium
}

// newDataObject returns an IDataObject serving d; the caller holds its one
// reference.
func newDataObject(d platform.Data) *comServer {
	impl := &dataObjectServer{data: d}
	for _, f := range encodeData(d, true) {
		if id, err := formatID(f); err == nil {
			impl.formats = append(impl.formats, dataObjectFormat{id: id, data: f.data})
		}
	}
	return newComServer(&dataObjectVtbl[0], []guid{iidDataObject, iidTelaDataObject}, impl, impl.free)
}

// free releases the stored media.
func (d *dataObjectServer) free() {
	for i := range d.stored {
		procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&d.stored[i].med)))
	}
	d.stored = nil
}

// ownDataObject returns this app's data object behind an IDataObject, or
// nil for another program's.
func ownDataObject(obj comObject) *dataObjectServer {
	var p uintptr
	if hr := obj.call(vtQueryInterface, uintptr(unsafe.Pointer(&iidTelaDataObject)), uintptr(unsafe.Pointer(&p))); hr < 0 || p == 0 {
		return nil
	}
	defer comObject(p).release()
	if s := comServerOf(p); s != nil {
		d, _ := s.impl.(*dataObjectServer)
		return d
	}
	return nil
}

func dataObjectOf(this uintptr) *dataObjectServer {
	if s := comServerOf(this); s != nil {
		d, _ := s.impl.(*dataObjectServer)
		return d
	}
	return nil
}

func (d *dataObjectServer) storedIndex(cf uint16) int {
	for i, s := range d.stored {
		if s.fe.cf == cf {
			return i
		}
	}
	return -1
}

// lookup checks a request: the HRESULT GetData and QueryGetData answer,
// and the index of the rendered format (-1) or of the stored medium.
func (d *dataObjectServer) lookup(fe *formatEtc) (hr uintptr, rendered, stored int) {
	if fe.aspect != dvaspectContent {
		return dvEDVAspect, -1, -1
	}
	if i := d.storedIndex(fe.cf); i >= 0 {
		if fe.tymed&d.stored[i].med.tymed == 0 {
			return dvETymed, -1, -1
		}
		return sOK, -1, i
	}
	for i, f := range d.formats {
		if uint32(fe.cf) == f.id {
			if fe.tymed&tymedHGlobal == 0 {
				return dvETymed, -1, -1
			}
			return sOK, i, -1
		}
	}
	return dvEFormatEtc, -1, -1
}

// IDataObject::GetData(pformatetcIn, pmedium).
func doGetData(this, pfe, pmed uintptr) uintptr {
	d := dataObjectOf(this)
	if d == nil || pfe == 0 || pmed == 0 {
		return eInvalidArg
	}
	fe := *(*formatEtc)(ptrFromUintptr(pfe))
	out := (*stgMedium)(ptrFromUintptr(pmed))
	*out = stgMedium{}
	hr, rendered, stored := d.lookup(&fe)
	switch {
	case hr != sOK:
		return hr
	case rendered >= 0:
		h, err := globalFromBytes(d.formats[rendered].data)
		if err != nil {
			return eOutOfMemory
		}
		*out = stgMedium{tymed: tymedHGlobal, data: h}
	default:
		// A copy the caller releases: duplicated memory, or the stream
		// with a reference of its own (as Chromium's data object does).
		src := d.stored[stored].med
		switch src.tymed {
		case tymedHGlobal:
			h, err := duplicateGlobal(src.data)
			if err != nil {
				return eOutOfMemory
			}
			*out = stgMedium{tymed: tymedHGlobal, data: h}
		case tymedIStream:
			comObject(src.data).call(vtAddRef)
			*out = stgMedium{tymed: tymedIStream, data: src.data}
		default:
			return dvETymed
		}
	}
	return sOK
}

// IDataObject::GetDataHere: not supported (callers fall back to GetData).
func doGetDataHere(this, pfe, pmed uintptr) uintptr { return eNotImpl }

// IDataObject::QueryGetData(pformatetc).
func doQueryGetData(this, pfe uintptr) uintptr {
	d := dataObjectOf(this)
	if d == nil || pfe == 0 {
		return eInvalidArg
	}
	fe := *(*formatEtc)(ptrFromUintptr(pfe))
	hr, _, _ := d.lookup(&fe)
	return hr
}

// IDataObject::GetCanonicalFormatEtc: every format is its own canonical
// one (the answer of Chromium's and the shell's data objects).
func doGetCanonicalFormatEtc(this, pin, pout uintptr) uintptr {
	if pout != 0 {
		(*formatEtc)(ptrFromUintptr(pout)).ptd = 0
	}
	return eNotImpl
}

// IDataObject::SetData(pformatetc, pmedium, fRelease): stores global
// memory and streams under their format, replacing an earlier one. With
// fRelease the object takes the medium; otherwise it keeps a copy.
func doSetData(this, pfe, pmed, release uintptr) uintptr {
	d := dataObjectOf(this)
	if d == nil || pfe == 0 || pmed == 0 {
		return eInvalidArg
	}
	fe := *(*formatEtc)(ptrFromUintptr(pfe))
	fe.ptd = 0 // the target device belongs to the caller
	src := *(*stgMedium)(ptrFromUintptr(pmed))
	keep := src
	if release == 0 {
		switch src.tymed {
		case tymedHGlobal:
			h, err := duplicateGlobal(src.data)
			if err != nil {
				return eOutOfMemory
			}
			keep = stgMedium{tymed: tymedHGlobal, data: h}
		case tymedIStream:
			comObject(src.data).call(vtAddRef)
			keep = stgMedium{tymed: tymedIStream, data: src.data}
		default:
			return dvETymed
		}
	}
	fe.tymed = keep.tymed
	if i := d.storedIndex(fe.cf); i >= 0 {
		procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&d.stored[i].med)))
		d.stored[i] = storedMedium{fe: fe, med: keep}
	} else {
		d.stored = append(d.stored, storedMedium{fe: fe, med: keep})
	}
	return sOK
}

// IDataObject::EnumFormatEtc(dwDirection, ppenumFormatEtc): the shell's
// standard enumerator (SHCreateStdEnumFmtEtc copies the list).
func doEnumFormatEtc(this, dir, ppenum uintptr) uintptr {
	d := dataObjectOf(this)
	if d == nil || ppenum == 0 {
		return eInvalidArg
	}
	*(*uintptr)(ptrFromUintptr(ppenum)) = 0
	if dir != datadirGet {
		return eNotImpl
	}
	fes := make([]formatEtc, 0, len(d.formats)+len(d.stored)+1)
	for _, f := range d.formats {
		fes = append(fes, formatEtc{cf: uint16(f.id), aspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal})
	}
	for _, s := range d.stored {
		fes = append(fes, s.fe)
	}
	n := len(fes)
	fes = append(fes, formatEtc{}) // a valid address for an empty list
	r, _, _ := procSHCreateStdEnumFmtEtc.Call(uintptr(n), uintptr(unsafe.Pointer(&fes[0])), ppenum)
	return uintptr(uint32(r))
}

// IDataObject::DAdvise, DUnadvise and EnumDAdvise: no advise sinks (the
// data never changes).
func doDAdvise(this, pfe, advf, sink, pconn uintptr) uintptr { return oleEAdviseNotSupported }
func doDUnadvise(this, conn uintptr) uintptr                 { return oleEAdviseNotSupported }
func doEnumDAdvise(this, ppenum uintptr) uintptr             { return oleEAdviseNotSupported }

// --- clipboard ---

// ReadTypes reads the clipboard's formats from the event loop and reports
// their Data types (dataformat.go).
func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("win32: Clipboard.ReadTypes called with a nil callback; pass a func(types []string, err error)")
	}
	b := c.b
	b.loop.Post(func() {
		types, err := b.clipboardTypes()
		done(types, err)
	})
	b.notify()
	return nil
}

// Read reads the clipboard's content of one type from the event loop: nil
// and no error when no format carries it.
func (c clipboard) Read(mime string, done func([]byte, error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	b := c.b
	b.loop.Post(func() {
		data, err := b.readClipboardType(mime)
		done(data, err)
	})
	b.notify()
	return nil
}

// Write replaces the clipboard's content with data, in the formats of
// encodeData; empty data clears it. Windows keeps the memory, so nothing
// is served later.
func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	return c.b.writeClipboardData(data)
}

// clipboardFormats returns the clipboard's formats in its order; the
// clipboard must be open.
func clipboardFormats() []clipFormat {
	var out []clipFormat
	var f uintptr
	for range 4096 { // a bound against a broken clipboard chain
		f, _, _ = procEnumClipboardFormats.Call(f)
		if f == 0 {
			break
		}
		out = append(out, clipFormatOf(uint32(f)))
	}
	return out
}

func (b *Backend) clipboardTypes() ([]string, error) {
	if err := b.openClipboard(); err != nil {
		return nil, err
	}
	defer procCloseClipboard.Call()
	return offeredTypes(clipboardFormats()), nil
}

func (b *Backend) readClipboardType(mime string) ([]byte, error) {
	if err := b.openClipboard(); err != nil {
		return nil, err
	}
	defer procCloseClipboard.Call()
	var firstErr error
	for _, f := range formatsFor(mime, clipboardFormats()) {
		h, _, e := procGetClipboardData.Call(uintptr(f.id))
		if h == 0 {
			if firstErr == nil {
				firstErr = winError("GetClipboardData", e)
			}
			continue
		}
		raw, err := bytesFromGlobal(h)
		if err == nil {
			var data []byte
			if data, err = decodeFormat(f, raw); err == nil {
				return data, nil
			}
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

func (b *Backend) writeClipboardData(data platform.Data) error {
	type held struct {
		id uint32
		h  uintptr
	}
	var mem []held
	free := func(from int) {
		for _, m := range mem[from:] {
			procGlobalFree.Call(m.h)
		}
	}
	// Allocate and fill the data before opening the clipboard, which
	// holds it away from every other process.
	for _, f := range encodeData(data, false) {
		id, err := formatID(f)
		if err == nil {
			var h uintptr
			if h, err = globalFromBytes(f.data); err == nil {
				mem = append(mem, held{id, h})
				continue
			}
		}
		free(0)
		return err
	}
	if err := b.openClipboard(); err != nil {
		free(0)
		return err
	}
	defer procCloseClipboard.Call()
	if r, _, e := procEmptyClipboard.Call(); r == 0 {
		free(0)
		return winError("EmptyClipboard", e)
	}
	for i, m := range mem {
		// On success the system owns the memory.
		if r, _, e := procSetClipboardData.Call(uintptr(m.id), m.h); r == 0 {
			free(i)
			return winError(fmt.Sprintf("SetClipboardData(%s)", clipFormatOf(m.id).describe()), e)
		}
	}
	return nil
}

// describe names a format for errors.
func (f clipFormat) describe() string {
	if f.name != "" {
		return fmt.Sprintf("%q", f.name)
	}
	return fmt.Sprintf("format %d", f.id)
}
