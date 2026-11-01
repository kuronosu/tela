//go:build windows

package win32

import (
	"bytes"
	"slices"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// These tests need no window and no OLE apartment: they call the Go COM
// objects through their vtables, as COM would, and read them back with
// the client code that reads other programs' data objects.

// TestIntegrationStructLayout pins the structures OLE and the shell read
// by offset.
func TestIntegrationStructLayout(t *testing.T) {
	ptr := unsafe.Sizeof(uintptr(0))
	for _, tc := range []struct {
		name      string
		got, want uintptr
	}{
		{"FORMATETC", unsafe.Sizeof(formatEtc{}), 2*ptr + 12 + (ptr-4)*1}, // 32 on 64-bit, 20 on 32-bit
		{"FORMATETC.ptd", unsafe.Offsetof(formatEtc{}.ptd), ptr},
		{"FORMATETC.dwAspect", unsafe.Offsetof(formatEtc{}.aspect), 2 * ptr},
		{"STGMEDIUM", unsafe.Sizeof(stgMedium{}), 3 * ptr},
		{"STGMEDIUM.hGlobal", unsafe.Offsetof(stgMedium{}.data), ptr},
		{"SHDRAGIMAGE.hbmpDragImage", unsafe.Offsetof(shDragImage{}.bitmap), 16},
		{"SHDRAGIMAGE", unsafe.Sizeof(shDragImage{}), 16 + 2*ptr},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// newForeignDataObject is a data object that does not say it is this
// app's (no iidTelaDataObject): a drop target reads it like another
// program's, through its formats.
func newForeignDataObject(d platform.Data) *comServer {
	impl := &dataObjectServer{data: d}
	for _, f := range encodeData(d, true) {
		if id, err := formatID(f); err == nil {
			impl.formats = append(impl.formats, dataObjectFormat{id: id, data: f.data})
		}
	}
	return newComServer(&dataObjectVtbl[0], []guid{iidDataObject}, impl, impl.free)
}

func TestDataObjectServer(t *testing.T) {
	d := platform.Data{
		{Type: platform.MIMEHTML, Bytes: []byte("<i>x ✓</i>")},
		{Type: platform.MIMEText, Bytes: []byte("x ✓\r\ny")},
		{Type: "application/x-tela-test", Bytes: []byte{0, 1, 2, 3, 4}},
	}
	s := newDataObject(d)
	ptr := s.ptr()
	obj := comObject(ptr)

	// IUnknown.
	var p uintptr
	if hr := obj.call(vtQueryInterface, uintptr(unsafe.Pointer(&iidDataObject)), uintptr(unsafe.Pointer(&p))); hr != sOK || p != ptr {
		t.Fatalf("QueryInterface(IDataObject) = %#x, %#x", uint32(hr), p)
	}
	if n := obj.call(vtRelease); n != 1 {
		t.Errorf("Release after QueryInterface = %d, want 1", n)
	}
	if hr := obj.call(vtQueryInterface, uintptr(unsafe.Pointer(&iidDropTarget)), uintptr(unsafe.Pointer(&p))); uint32(hr) != eNoInterface || p != 0 {
		t.Errorf("QueryInterface(IDropTarget) = %#x, %#x; want E_NOINTERFACE", uint32(hr), p)
	}
	if own := ownDataObject(obj); own == nil || !slices.Equal(own.data.Types(), d.Types()) {
		t.Errorf("ownDataObject did not recognize this app's data object")
	}
	if n := s.refs.Load(); n != 1 {
		t.Errorf("references after the queries = %d, want 1", n)
	}

	// QueryGetData.
	text := formatEtc{cf: cfUnicodeText, aspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal}
	if hr := obj.call(5, uintptr(unsafe.Pointer(&text))); hr != sOK {
		t.Errorf("QueryGetData(CF_UNICODETEXT) = %#x", uint32(hr))
	}
	stream := text
	stream.tymed = tymedIStream
	if hr := obj.call(5, uintptr(unsafe.Pointer(&stream))); uint32(hr) != dvETymed {
		t.Errorf("QueryGetData(CF_UNICODETEXT in a stream) = %#x, want DV_E_TYMED", uint32(hr))
	}
	dib := text
	dib.cf = cfDIB
	if hr := obj.call(5, uintptr(unsafe.Pointer(&dib))); uint32(hr) != dvEFormatEtc {
		t.Errorf("QueryGetData(CF_DIB) = %#x, want DV_E_FORMATETC", uint32(hr))
	}

	// EnumFormatEtc and GetData, through the reader of other programs'
	// data objects.
	formats := dataObjectFormats(obj)
	if got := offeredTypes(formats); !slices.Equal(got, d.Types()) {
		t.Errorf("types read through EnumFormatEtc = %q, want %q", got, d.Types())
	}
	if got := readDataObject(obj, formats, d.Types()); !dataEqual(got, d) {
		t.Errorf("data read through GetData = %q, want %q", got, d)
	}

	// SetData keeps what the shell stores, and GetData gives copies.
	cf := registerFormat("DragImageBits")
	h, err := globalFromBytes([]byte("image bits"))
	if err != nil {
		t.Fatal(err)
	}
	fe := formatEtc{cf: uint16(cf), aspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal}
	med := stgMedium{tymed: tymedHGlobal, data: h}
	if hr := obj.call(7, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&med)), 1); hr != sOK {
		t.Fatalf("SetData(fRelease) = %#x", uint32(hr))
	}
	h2, _ := globalFromBytes([]byte("other bits"))
	med2 := stgMedium{tymed: tymedHGlobal, data: h2}
	if hr := obj.call(7, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&med2)), 0); hr != sOK {
		t.Fatalf("SetData(copy) = %#x", uint32(hr))
	}
	procGlobalFree.Call(h2) // a copy was kept
	for range 2 {
		got, err := getData(obj, clipFormat{id: cf, name: "DragImageBits"})
		if err != nil || !bytes.HasPrefix(got, []byte("other bits")) {
			t.Errorf("GetData of a stored medium = %q, %v", got, err)
		}
	}
	if got := len(dataObjectFormats(obj)); got != len(formats)+1 {
		t.Errorf("EnumFormatEtc after SetData lists %d formats, want %d", got, len(formats)+1)
	}

	// The last Release destroys the object.
	if n := s.release(); n != 0 {
		t.Errorf("last release = %d", n)
	}
	if comServerOf(ptr) != nil {
		t.Errorf("the object is still registered after its last release")
	}
	if n := comAddRef(ptr); n != 0 {
		t.Errorf("AddRef on a destroyed object = %d, want 0 (ignored)", n)
	}
}

func TestForeignDataObjectIsNotOwn(t *testing.T) {
	s := newForeignDataObject(platform.TextData("x"))
	defer s.release()
	if ownDataObject(comObject(s.ptr())) != nil {
		t.Errorf("a data object without iidTelaDataObject was taken for this app's")
	}
}

func dataEqual(a, b platform.Data) bool {
	return slices.EqualFunc(a, b, func(x, y platform.DataEntry) bool { return x.Type == y.Type && bytes.Equal(x.Bytes, y.Bytes) })
}
