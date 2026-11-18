//go:build windows

package win32

import (
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The COM toast activator (ADR-297): a toast clicked after the app exited
// starts it, and the click arrives as the NotificationEvent it would have
// been, marked Earlier.
//
//   - The class. Its CLSID is assoc.ToastActivatorCLSID of the app ID, a
//     name-based UUID, so it is the same in every version of the app and
//     on every machine, and packaging computes the same one for an MSIX
//     manifest (ToastActivatorCLSID, com:ExeServer). Its class object
//     (CoRegisterClassObject, CLSCTX_LOCAL_SERVER, REGCLS_MULTIPLEUSE) is
//     registered when Run starts if the app's AUMID names it as its
//     CustomActivator (an earlier run showed a toast) or the process is
//     packaged, else by the run's first toast, so an app that never
//     notifies never touches COM's class table; SetAppID registers it again
//     under a new ID while Run runs (tela dev's host learns the app's ID
//     from the child), and shutdown revokes it. It must be registered
//     whenever the registry names it: COM would start another process for
//     a click on this run's toast otherwise.
//   - The registration (unpackaged apps). With the AUMID's (toast_windows.go,
//     the first toast of a run), for the current user:
//     HKCU\Software\Classes\CLSID\{clsid}\LocalServer32 = "<exe>"
//     -ToastActivated, and CustomActivator = {clsid} on the AUMID key,
//     under ADR-235's rules (registerToastActivator): Tela's marker, only
//     what changed (a moved executable rewrites the command), keys of
//     others left alone, nothing ever removed. Inside an MSIX package
//     (GetCurrentPackageFullName succeeds) nothing is written: the manifest
//     declares the server. Neither is anything written by tela dev's host
//     (SetDevHost): its executable is tela's, not the app's.
//   - Starting. COM runs the command with -ToastActivated (and may append
//     -Embedding), which this package's init removes from os.Args before
//     the app's main runs (toastargs_windows.go); the app starts as it
//     always does, its main window
//     opens, and Run registers the class object, which COM then calls. The
//     NotificationEvent is app-scoped, so core holds it until the main
//     window's first commit (ADR-253). While a process of the app runs,
//     COM calls its class object instead and starts nothing: with
//     desktop.SingleInstance no second process is started at all. The one
//     exception is the moment between a process's start and its Run, when
//     nothing is registered yet: COM then starts another process, which
//     (as a second instance) forwards its launch and exits before it registers, so the click raises the
//     running app's window and its NotificationEvent is lost.
//   - Threads. The class object lives in the UI thread's single-threaded
//     apartment (initCOM), so COM calls Activate on the UI thread through
//     its message loop, possibly inside an outgoing COM call of a handler
//     (the STA's reentrancy). Activate only queues the arguments
//     (activator.inbox) and posts wmTelaToast, as the agile delegates do,
//     and the event is made outside any handler (toastActivations).
//   - Both paths. A click on a toast of this process reaches its Activated
//     delegate and the activator. The activation arguments carry the run
//     (toastArguments), so the activator's report of a toast of this run
//     is the delegate's: whichever arrives first finishes the toast and
//     reports it, and the other finds it gone. A toast of another run is
//     reported with Earlier set, and its ID (that run's) touches nothing
//     of this run's.

var (
	procCoRegisterClassObject     = ole32.NewProc("CoRegisterClassObject")
	procCoRevokeClassObject       = ole32.NewProc("CoRevokeClassObject")
	procGetCurrentPackageFullName = kernel32.NewProc("GetCurrentPackageFullName")
)

// IIDs of the activator's interfaces (unknwn.h; notificationactivationcallback.h).
var (
	iidClassFactory                   = guid{0x00000001, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidNotificationActivationCallback = guid{0x53E31837, 0x6600, 0x4A81, [8]byte{0x93, 0x95, 0x75, 0xCF, 0xFE, 0x74, 0x6F, 0x94}}
)

const (
	clsctxLocalServer   = 0x4
	regclsMultipleUse   = 1
	classENoAggregation = 0x80040110
	// GetCurrentPackageFullName with no buffer: a package's full name does
	// not fit (winerror.h); APPMODEL_ERROR_NO_PACKAGE (15700) outside one.
	errInsufficientBuffer = 122
)

// toastActivator is the state of the class object, on the UI thread.
type toastActivator struct {
	devHost bool // SetDevHost: write no registration

	clsid    string     // registered under; "" when not registered
	cookie   uint32     // CoRegisterClassObject's
	factory  *comServer // the IClassFactory
	callback *comServer // the INotificationActivationCallback it hands out
	inbox    *toastInbox
}

// activatorImpl is the Go side of the activator's objects. Immutable.
type activatorImpl struct {
	inbox    *toastInbox
	callback *comServer // for CreateInstance
}

var (
	activatorVtblOnce sync.Once
	classFactoryVtbl  [5]uintptr // IUnknown, CreateInstance, LockServer
	activationVtbl    [4]uintptr // IUnknown, Activate
)

func initActivatorVtables() {
	activatorVtblOnce.Do(func() {
		qi := syscall.NewCallback(comQueryInterface)
		addRef := syscall.NewCallback(comAddRef)
		release := syscall.NewCallback(comRelease)
		classFactoryVtbl = [5]uintptr{qi, addRef, release,
			syscall.NewCallback(cfCreateInstance),
			syscall.NewCallback(cfLockServer),
		}
		activationVtbl = [4]uintptr{qi, addRef, release, syscall.NewCallback(nacActivate)}
	})
}

// SetDevHost tells the backend that it hosts tela dev children
// (remote.NewHost): the running executable is tela's, so the toast
// activator's registration is not written; the class object is still
// registered, so a click on the app's toast reaches this process rather
// than starting the app's own executable. UI thread.
func (b *Backend) SetDevHost() { b.desk.activator.devHost = true }

// packaged reports whether the process runs inside an MSIX package.
var packaged = sync.OnceValue(func() bool {
	if !available(procGetCurrentPackageFullName) {
		return false // before Windows 8
	}
	var n uint32
	r, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), 0)
	return r == 0 || r == errInsufficientBuffer
})

// toastActivatorCommand returns the LocalServer32 command of this
// process's executable; the live tests replace it.
var toastActivatorCommand = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return toastServerCommand(exe), nil
}

// writesToastActivator reports whether the first toast registers the
// activator: not inside a package, not in tela dev's host.
func (b *Backend) writesToastActivator() bool { return !b.desk.activator.devHost && !packaged() }

// registerToastActivatorFor writes the activator's registration for aumid
// (toast_windows.go calls it with a key registerAUMID owns). A failure is
// not reported: the toasts work while the app runs.
func (b *Backend) registerToastActivatorFor(reg aumidRegistry, aumid string) {
	if !b.writesToastActivator() {
		return
	}
	cmd, err := toastActivatorCommand()
	if err != nil {
		return
	}
	_, _ = registerToastActivator(reg, aumid, assoc.ToastActivatorCLSID(b.toastAppID()), cmd)
}

// serveToastActivator registers the class object when Run starts or the
// app ID changes during Run, if the app needs it then: it is registered
// already (under the old ID), the process is packaged, or the AUMID's
// registry key names the CLSID as its CustomActivator (a toast of an
// earlier run registered it, or an installer did).
func (b *Backend) serveToastActivator() {
	if b.desk.activator.clsid == "" && !packaged() {
		clsid := assoc.ToastActivatorCLSID(b.toastAppID())
		v, ok, _ := hkcuRegistry{}.String(aumidKeyPrefix+b.toastAppAUMID(), customActivatorValue)
		if !ok || !strings.EqualFold(v, "{"+clsid+"}") {
			return
		}
	}
	b.startToastActivator()
}

// startToastActivator registers the class object of the app's CLSID, or
// registers it again when the app ID changed; it does nothing where toasts
// are unavailable. UI thread, while Run runs (the first toast calls it).
func (b *Backend) startToastActivator() {
	a := &b.desk.activator
	clsid := assoc.ToastActivatorCLSID(b.toastAppID())
	if a.clsid == clsid || b.closed || b.desk.toast.off {
		return
	}
	b.stopToastActivator()
	if windowsBuild() < toastMinBuild || !available(procCoRegisterClassObject) || !available(procCoRevokeClassObject) {
		return
	}
	if b.desk.initCOM() != nil {
		return
	}
	inbox := &toastInbox{hwnd: b.msgHwnd}
	factory, callback := newToastActivatorObjects(inbox)
	id := guidOf(clsid)
	var cookie uint32
	r, _, _ := procCoRegisterClassObject.Call(uintptr(unsafe.Pointer(&id)), factory.ptr(), clsctxLocalServer, regclsMultipleUse, uintptr(unsafe.Pointer(&cookie)))
	if hr := int32(uint32(r)); hr < 0 {
		factory.release()
		callback.release()
		return
	}
	*a = toastActivator{devHost: a.devHost, clsid: clsid, cookie: cookie, factory: factory, callback: callback, inbox: inbox}
}

// newToastActivatorObjects creates the class factory and the activator it
// hands out, each with one reference, the caller's; the activator queues
// its activations in inbox.
func newToastActivatorObjects(inbox *toastInbox) (factory, callback *comServer) {
	initActivatorVtables()
	impl := &activatorImpl{inbox: inbox}
	callback = newComServerOf([]comIface{{&activationVtbl[0], []guid{iidNotificationActivationCallback}}}, impl, nil)
	impl.callback = callback
	factory = newComServerOf([]comIface{{&classFactoryVtbl[0], []guid{iidClassFactory}}}, impl, nil)
	return factory, callback
}

// stopToastActivator revokes the class object; COM releases its reference,
// and the activations not delivered yet are dropped.
func (b *Backend) stopToastActivator() {
	a := &b.desk.activator
	if a.clsid == "" {
		return
	}
	procCoRevokeClassObject.Call(uintptr(a.cookie))
	a.inbox.close()
	a.factory.release()
	a.callback.release()
	*a = toastActivator{devHost: a.devHost}
}

// toastActivations turns the activations Activate queued into
// NotificationEvents, reporting whether it queued any.
func (b *Backend) toastActivations() (queued bool) {
	a := &b.desk.activator
	if a.inbox == nil {
		return false
	}
	run := toastRun(b.start)
	for _, q := range a.inbox.take() {
		ev, thisRun, ok := activationEvent(q.args, run)
		if !ok {
			continue // not a Tela toast's arguments
		}
		if thisRun {
			e := b.desk.toast.entries[ev.ID]
			if e == nil {
				continue // its delegate reported it, or it was closed
			}
			b.finishToast(e, false)
		}
		b.enqueue(nil, ev)
		queued = true
	}
	return queued
}

// IClassFactory::CreateInstance(outer, riid, ppv): the activator, which is
// one object for every client.
func cfCreateInstance(this, outer, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	*(*uintptr)(ptrFromUintptr(ppv)) = 0
	if outer != 0 {
		return classENoAggregation
	}
	s := comServerOf(this)
	if s == nil {
		return eNoInterface
	}
	impl, ok := s.impl.(*activatorImpl)
	if !ok || impl.callback == nil {
		return eNoInterface
	}
	return comQueryInterface(impl.callback.ptr(), riid, ppv)
}

// IClassFactory::LockServer: the process lives as long as the app, so
// there is nothing to count.
func cfLockServer(this, lock uintptr) uintptr { return sOK }

// INotificationActivationCallback::Activate(appUserModelId, invokedArgs,
// data, count): invokedArgs is the toast's launch or the clicked button's
// arguments. The text inputs (data) are not used: Tela's toasts have none.
// The arguments are copied and queued for the UI thread's next message.
func nacActivate(this, aumid, args, data, count uintptr) uintptr {
	s := comServerOf(this)
	if s == nil {
		return sOK
	}
	impl, ok := s.impl.(*activatorImpl)
	if !ok {
		return sOK
	}
	text := ""
	if args != 0 {
		text = windows.UTF16PtrToString((*uint16)(ptrFromUintptr(args)))
	}
	impl.inbox.push(queuedToastEvent{toastEvent: toastEvent{kind: toastActivated, args: text}})
	return sOK
}
