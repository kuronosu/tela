//go:build windows

package win32

import (
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The COM side of the toasts without WinRT's toast classes: the delegate
// objects (reference counts, the interfaces they answer, what Invoke reads
// from its arguments) and HSTRINGs. They run on every Windows, Wine
// included; live_toast_test.go shows real toasts.

func requireHString(t *testing.T) {
	t.Helper()
	if !available(procWindowsCreateString) || !available(procWindowsGetStringRawBuffer) {
		t.Skip("no combase.dll HSTRING functions")
	}
}

func TestHString(t *testing.T) {
	requireHString(t)
	for _, s := range []string{"", "tela-notification=3&action=save", "naïve 😀", "bad \xff utf-8"} {
		h, err := newHString(s)
		if err != nil {
			t.Fatalf("newHString(%q): %v", s, err)
		}
		want := string([]rune(s)) // invalid UTF-8 becomes U+FFFD
		if got := h.String(); got != want {
			t.Errorf("HSTRING of %q reads %q", s, got)
		}
		h.free()
	}
}

func TestTokenArgs(t *testing.T) {
	tok := int64(-0x0123456789ABCDEF)
	args := tokenArgs(tok)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if len(args) != 1 || int64(args[0]) != tok {
			t.Errorf("tokenArgs(%#x) = %#x", tok, args)
		}
		return
	}
	if len(args) != 2 || uint64(args[0])|uint64(args[1])<<32 != uint64(tok) {
		t.Errorf("tokenArgs(%#x) = %#x", tok, args)
	}
}

// refs returns the reference count of a server, or -1 when it is gone from
// the table (its last reference was released).
func refs(s *comServer) int32 {
	if comServerOf(s.ptr()) == nil {
		return -1
	}
	return s.refs.Load()
}

// TestToastHandlerReferences calls a delegate object through its vtable as
// WinRT does: QueryInterface answers its delegate interface, IAgileObject
// and IUnknown only, every reference is counted, and the last Release
// removes it.
func TestToastHandlerReferences(t *testing.T) {
	for kind := range 3 {
		h := newToastHandler(&toastHandler{inbox: &toastInbox{}, id: 1, serial: 1, kind: toastEventKind(kind)})
		o := comObject(h.ptr())
		for _, iid := range []guid{iidUnknown, iidToastHandlers[kind], iidAgileObject} {
			q, hr := o.query(&iid)
			if hr < 0 || q != o {
				t.Errorf("kind %d: QueryInterface(%v) = %#x, %#x", kind, iid, q, uint32(hr))
				continue
			}
			q.release()
		}
		for _, iid := range []guid{iidToastHandlers[(kind+1)%3], iidXMLDocument, guidOf(iidStrInspectable)} {
			if q, hr := o.query(&iid); hr >= 0 || q != 0 {
				t.Errorf("kind %d: QueryInterface(%v) = %#x, %#x; want E_NOINTERFACE", kind, iid, q, uint32(hr))
			}
		}
		if n := refs(h); n != 1 {
			t.Errorf("kind %d: %d references after balanced calls, want 1", kind, n)
		}
		o.call(vtAddRef) // WinRT's add_ handler
		o.release()      // its remove_
		h.release()      // ours (finishToast)
		if n := refs(h); n != -1 {
			t.Errorf("kind %d: the delegate is still alive after its last Release (%d)", kind, n)
		}
	}
}

// fakeArgs is a COM object standing in for the arguments of a toast event:
// it answers the interfaces in iids, its method 6 is get_Arguments (an
// HSTRING) or get_Reason (an int32).
type fakeArgs struct {
	arguments string
	reason    int32
}

var (
	fakeArgsOnce sync.Once
	fakeArgsVtbl [7]uintptr
)

func newFakeArgs(t *testing.T, iid guid, a *fakeArgs) *comServer {
	t.Helper()
	fakeArgsOnce.Do(func() {
		fakeArgsVtbl = [7]uintptr{
			syscall.NewCallback(comQueryInterface), syscall.NewCallback(comAddRef), syscall.NewCallback(comRelease),
			0, 0, 0,
			syscall.NewCallback(func(this, out uintptr) uintptr {
				s := comServerOf(this)
				a := s.impl.(*fakeArgs)
				if s.iids[0][0] == iidToastActivatedEventArgs {
					h, err := newHString(a.arguments)
					if err != nil {
						return eOutOfMemory
					}
					*(*hstring)(ptrFromUintptr(out)) = h // the caller frees it
				} else {
					*(*int32)(ptrFromUintptr(out)) = a.reason
				}
				return sOK
			}),
		}
	})
	return newComServer(&fakeArgsVtbl[0], []guid{iid}, a, nil)
}

// TestToastInvoke invokes each delegate with fake arguments and checks the
// event it hands to the UI thread, and that it keeps no reference to them.
func TestToastInvoke(t *testing.T) {
	requireHString(t)
	inbox := &toastInbox{}
	invoke := func(kind toastEventKind, args *comServer) queuedToastEvent {
		t.Helper()
		h := newToastHandler(&toastHandler{inbox: inbox, id: 9, serial: 4, kind: kind})
		defer h.release()
		var p uintptr
		if args != nil {
			p = args.ptr()
		}
		if hr := comObject(h.ptr()).call(vtDelegateInvoke, 0, p); hr != sOK {
			t.Errorf("Invoke = %#x", uint32(hr))
		}
		if args != nil {
			if n := refs(args); n != 1 {
				t.Errorf("kind %d: the arguments have %d references after Invoke, want 1", kind, n)
			}
		}
		ev := inbox.take()
		if len(ev) != 1 {
			t.Fatalf("kind %d: %d events queued, want 1", kind, len(ev))
		}
		return ev[0]
	}

	act := newFakeArgs(t, iidToastActivatedEventArgs, &fakeArgs{arguments: toastArguments(9, "second")})
	defer act.release()
	if got, want := invoke(toastActivated, act), (queuedToastEvent{toastEvent{id: 9, kind: toastActivated, args: "tela-notification=9&action=second"}, 4}); got != want {
		t.Errorf("Activated: %+v, want %+v", got, want)
	}
	if got, want := invoke(toastActivated, nil), (queuedToastEvent{toastEvent{id: 9, kind: toastActivated}, 4}); got != want {
		t.Errorf("Activated without arguments: %+v, want %+v", got, want)
	}
	dis := newFakeArgs(t, guidOf(iidStrToastDismissedEventArgs), &fakeArgs{reason: toastTimedOut})
	defer dis.release()
	if got, want := invoke(toastDismissed, dis), (queuedToastEvent{toastEvent{id: 9, kind: toastDismissed, reason: toastTimedOut}, 4}); got != want {
		t.Errorf("Dismissed: %+v, want %+v", got, want)
	}
	if got, want := invoke(toastFailed, nil), (queuedToastEvent{toastEvent{id: 9, kind: toastFailed}, 4}); got != want {
		t.Errorf("Failed: %+v, want %+v", got, want)
	}

	inbox.close()
	inbox.push(queuedToastEvent{toastEvent: toastEvent{id: 1}})
	if ev := inbox.take(); len(ev) != 0 {
		t.Errorf("a closed inbox queued %v", ev)
	}
}

// TestToastMessagesDropStale feeds the UI thread's side events of a toast
// that was replaced or closed meanwhile: they report nothing.
func TestToastMessagesDropStale(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		b.Quit()
		_ = b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {}))
	}()
	tst := &b.desk.toast
	tst.inbox = &toastInbox{}
	tst.entries = map[platform.NotificationID]*toastEntry{3: {id: 3, serial: 2}}
	tst.inbox.push(queuedToastEvent{toastEvent{id: 3, kind: toastFailed}, 1}) // the replaced toast
	tst.inbox.push(queuedToastEvent{toastEvent{id: 4, kind: toastFailed}, 3}) // a closed one
	tst.inbox.push(queuedToastEvent{toastEvent{id: 3, kind: toastDismissed, reason: toastTimedOut}, 2})
	b.toastMessages()
	if b.dirty || len(tst.entries) != 1 {
		t.Errorf("stale and timed-out events: dirty %v, %d entries; want nothing queued and the toast kept", b.dirty, len(tst.entries))
	}
	tst.entries = nil
	tst.inbox = nil
}
