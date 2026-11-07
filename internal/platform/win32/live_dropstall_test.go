//go:build windows

package win32

import (
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// A drag source that hangs, in process: an IDataObject served from a
// multithreaded apartment whose EnumFormatEtc and GetData never return, and
// that the UI thread (a single-threaded apartment) reaches through a real
// COM proxy, as it reaches another process's data object. The reads are
// cut after dropStallTimeout (stallguard.go), and the thread still makes
// COM calls afterwards.

var (
	procCoMarshalInterThreadInterfaceInStream = ole32.NewProc("CoMarshalInterThreadInterfaceInStream")
	procCoGetInterfaceAndReleaseStream        = ole32.NewProc("CoGetInterfaceAndReleaseStream")
)

// hungRelease unblocks the hung object's methods; hungVtbl is the
// IDataObject vtable whose EnumFormatEtc and GetData wait for it.
var (
	hungRelease chan struct{}
	hungVtbl    [12]uintptr
)

func hungMethod(this, a, b uintptr) uintptr {
	<-hungRelease
	return eNotImpl
}

// hungDataObject returns a proxy to a data object that hangs, in the
// calling (single-threaded) apartment.
func hungDataObject(t *testing.T) comObject {
	t.Helper()
	initComVtables()
	hungVtbl = dataObjectVtbl
	hungVtbl[vtDataObjectGetData] = syscall.NewCallback(hungMethod)
	hungVtbl[vtDataObjectEnumFormatEtc] = syscall.NewCallback(hungMethod)

	streams := make(chan uintptr)
	go func() {
		runtime.LockOSThread()        // the apartment belongs to this thread; it is never released
		procCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
		srv := newComServer(&hungVtbl[0], []guid{iidDataObject}, struct{}{}, nil)
		var stream uintptr
		hr, _, _ := procCoMarshalInterThreadInterfaceInStream.Call(uintptr(unsafe.Pointer(&iidDataObject)), srv.ptr(), uintptr(unsafe.Pointer(&stream)))
		if int32(uint32(hr)) < 0 {
			stream = 0
		}
		streams <- stream
		<-hungRelease
	}()
	stream := <-streams
	if stream == 0 {
		t.Fatal("CoMarshalInterThreadInterfaceInStream failed")
	}
	var obj comObject
	hr, _, _ := procCoGetInterfaceAndReleaseStream.Call(stream, uintptr(unsafe.Pointer(&iidDataObject)), uintptr(unsafe.Pointer(&obj)))
	if int32(uint32(hr)) < 0 || obj == 0 {
		t.Fatalf("CoGetInterfaceAndReleaseStream = %#x", uint32(hr))
	}
	return obj
}

func TestLiveDropSourceStalls(t *testing.T) {
	requireLiveIntegration(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b := liveBackend(t)
	if err := b.desk.initCOM(); err != nil {
		t.Fatal(err)
	}
	old := dropStallTimeout
	dropStallTimeout = 600 * time.Millisecond
	defer func() { dropStallTimeout = old }()
	hungRelease = make(chan struct{})
	obj := hungDataObject(t)
	defer func() {
		close(hungRelease)
		obj.release()
	}()

	// EnumFormatEtc hangs: the formats are cut short and the guard says so.
	start := time.Now()
	var formats []clipFormat
	if !guarded(func(g *stallGuard) { formats = enumFormats(obj, g) }) {
		t.Errorf("EnumFormatEtc of a hung source was not cut short")
	}
	if d := time.Since(start); d < dropStallTimeout/2 || d > 5*time.Second {
		t.Errorf("EnumFormatEtc was cut after %v, want about %v", d, dropStallTimeout)
	}
	if len(formats) != 0 {
		t.Errorf("formats = %v, want none", formats)
	}

	// GetData hangs: the read returns the error and no data.
	start = time.Now()
	text := []clipFormat{clipFormatOf(cfUnicodeText)}
	var data platform.Data
	var err error
	stalled := guarded(func(g *stallGuard) { data, err = readDataObjectGuarded(obj, text, []string{platform.MIMEText}, g) })
	if !stalled || len(data) != 0 || err == nil {
		t.Errorf("a hung GetData: stalled %v, data %v, err %v; want a stall, no data and an error", stalled, data, err)
	}
	if err != nil && err.Error() != errDropStalled().Error() {
		t.Errorf("error = %v, want %v", err, errDropStalled())
	}
	if d := time.Since(start); d < dropStallTimeout/2 || d > 5*time.Second {
		t.Errorf("GetData was cut after %v, want about %v", d, dropStallTimeout)
	}

	// The thread's COM still works: a responsive object is read in full.
	want := platform.TextData("after the stall")
	s := newDataObject(want)
	defer s.release()
	obj2 := comObject(s.ptr())
	var got platform.Data
	if guarded(func(g *stallGuard) {
		fs := enumFormats(obj2, g)
		got, err = readDataObjectGuarded(obj2, fs, want.Types(), g)
	}) || err != nil || !dataEqual(got, want) {
		t.Errorf("a read after the stall = %v, %v; want %v", got, err, want)
	}
}
