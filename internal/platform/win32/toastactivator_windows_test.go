//go:build windows

package win32

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	winreg "golang.org/x/sys/windows/registry"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The COM toast activator (toastactivator_windows.go) without the shell:
// its objects called through their vtables, the single report of a click
// both paths deliver, its class object registered with COM and reached by
// CoCreateInstance, and its registry keys written below a temporary HKCU
// key. live_toast_test.go clicks a real toast after the app exited.

// Vtable indices of the activator's interfaces.
const (
	vtFactoryCreateInstance = 3 // IClassFactory: CreateInstance, LockServer
	vtFactoryLockServer     = 4
	vtActivate              = 3 // INotificationActivationCallback: Activate
)

// activateToast calls Activate on o with args (nil for "") as the notification
// platform does.
func activateToast(o comObject, args string) int32 {
	var p uintptr
	if args != "" {
		p = uintptr(unsafe.Pointer(utf16(args)))
	}
	return o.call(vtActivate, uintptr(unsafe.Pointer(utf16("dev.telaui.notes"))), p, 0, 0)
}

// TestToastActivatorObjects checks the class factory and the activator:
// their interfaces, CreateInstance (one activator, no aggregation), what
// Activate queues, and balanced references.
func TestToastActivatorObjects(t *testing.T) {
	inbox := &toastInbox{}
	factory, callback := newToastActivatorObjects(inbox)
	f, c := comObject(factory.ptr()), comObject(callback.ptr())
	for _, tc := range []struct {
		o    comObject
		iid  guid
		want bool
	}{
		{f, iidUnknown, true}, {f, iidClassFactory, true}, {f, iidNotificationActivationCallback, false},
		{c, iidUnknown, true}, {c, iidNotificationActivationCallback, true}, {c, iidClassFactory, false}, {c, iidAgileObject, false},
	} {
		q, hr := tc.o.query(&tc.iid)
		if got := hr >= 0 && q == tc.o; got != tc.want {
			t.Errorf("QueryInterface(%v) on %#x = %#x, %#x; want answered %v", tc.iid, tc.o, q, uint32(hr), tc.want)
		}
		q.release()
	}
	var p uintptr
	if hr := f.call(vtFactoryCreateInstance, 0, uintptr(unsafe.Pointer(&iidNotificationActivationCallback)), uintptr(unsafe.Pointer(&p))); hr != sOK || p != callback.ptr() {
		t.Errorf("CreateInstance = %#x, %#x; want the activator %#x", uint32(hr), p, callback.ptr())
	}
	comObject(p).release()
	p = 1
	if hr := f.call(vtFactoryCreateInstance, uintptr(f), uintptr(unsafe.Pointer(&iidNotificationActivationCallback)), uintptr(unsafe.Pointer(&p))); uint32(hr) != classENoAggregation || p != 0 {
		t.Errorf("CreateInstance with an outer object = %#x, %#x; want CLASS_E_NOAGGREGATION and nil", uint32(hr), p)
	}
	if hr := f.call(vtFactoryCreateInstance, 0, uintptr(unsafe.Pointer(&iidClassFactory)), uintptr(unsafe.Pointer(&p))); hr >= 0 || p != 0 {
		t.Errorf("CreateInstance of IClassFactory = %#x, %#x; want E_NOINTERFACE", uint32(hr), p)
	}
	if hr := f.call(vtFactoryLockServer, 1); hr != sOK {
		t.Errorf("LockServer = %#x", uint32(hr))
	}

	args := toastArguments(4, "kx3j9a", "1")
	if hr := activateToast(c, args); hr != sOK {
		t.Errorf("Activate = %#x", uint32(hr))
	}
	if hr := activateToast(c, ""); hr != sOK {
		t.Errorf("Activate without arguments = %#x", uint32(hr))
	}
	want := []queuedToastEvent{{toastEvent: toastEvent{kind: toastActivated, args: args}}, {toastEvent: toastEvent{kind: toastActivated}}}
	if got := inbox.take(); !slices.Equal(got, want) {
		t.Errorf("queued %+v, want %+v", got, want)
	}
	if rf, rc := refs(factory), refs(callback); rf != 1 || rc != 1 {
		t.Errorf("references after balanced calls: factory %d, activator %d; want 1 and 1", rf, rc)
	}
	factory.release()
	callback.release()
	if rf, rc := refs(factory), refs(callback); rf != -1 || rc != -1 {
		t.Errorf("alive after their last Release: factory %d, activator %d", rf, rc)
	}
}

// deliveredNotifications runs a delivery pass of b's loop and returns the
// NotificationEvents it delivered.
func deliveredNotifications(b *Backend) []platform.NotificationEvent {
	var got []platform.NotificationEvent
	b.loop.Deliver(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		if ne, ok := e.(platform.NotificationEvent); ok {
			got = append(got, ne)
		}
	}))
	return got
}

// TestToastActivationsReportOnce feeds the UI thread a click on a toast of
// this run from both paths, in both orders, and a click on an earlier
// run's toast with the same ID: each click is reported once, and the
// earlier one is marked and leaves this run's toasts alone.
func TestToastActivationsReportOnce(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		b.Quit()
		_ = b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {}))
	}()
	run := toastRun(b.start)
	earlier := toastRun(b.start.Add(-time.Hour))
	a, tst := &b.desk.activator, &b.desk.toast
	a.inbox, tst.inbox = &toastInbox{}, &toastInbox{}
	defer func() { a.inbox, tst.inbox, tst.entries = nil, nil, nil }()

	// The activator first, then the delegate, then an earlier run's toast 3.
	tst.entries = map[platform.NotificationID]*toastEntry{3: {id: 3, serial: 1}, 4: {id: 4, serial: 1}}
	a.inbox.push(queuedToastEvent{toastEvent: toastEvent{kind: toastActivated, args: toastArguments(3, run, "1")}})
	a.inbox.push(queuedToastEvent{toastEvent: toastEvent{kind: toastActivated, args: toastArguments(3, earlier, "")}})
	a.inbox.push(queuedToastEvent{toastEvent: toastEvent{kind: toastActivated, args: "not Tela's"}})
	tst.inbox.push(queuedToastEvent{toastEvent{id: 3, kind: toastActivated, args: toastArguments(3, run, "1")}, 1})
	b.toastMessages()
	want := []platform.NotificationEvent{
		{ID: 3, Kind: platform.NotificationActionInvoked, Action: "1"},
		{ID: 3, Kind: platform.NotificationActivated, Earlier: true},
	}
	if got := deliveredNotifications(b); !slices.Equal(got, want) {
		t.Errorf("activator first: %+v, want %+v", got, want)
	}
	if _, ok := tst.entries[4]; !ok || len(tst.entries) != 1 {
		t.Errorf("entries %v; want toast 4 only", tst.entries)
	}

	// The delegate first, in an earlier message: the activator's report
	// finds the toast gone.
	tst.inbox.push(queuedToastEvent{toastEvent{id: 4, kind: toastActivated, args: toastArguments(4, run, "")}, 1})
	b.toastMessages()
	a.inbox.push(queuedToastEvent{toastEvent: toastEvent{kind: toastActivated, args: toastArguments(4, run, "")}})
	b.toastMessages()
	want = []platform.NotificationEvent{{ID: 4, Kind: platform.NotificationActivated}}
	if got := deliveredNotifications(b); !slices.Equal(got, want) {
		t.Errorf("delegate first: %+v, want %+v", got, want)
	}
}

// TestToastActivatorClassObject registers the class object with COM (as
// Run does), creates the activator through CoCreateInstance and the app's
// CLSID, and activates it: the click arrives once, as an earlier run's.
// SetAppID during Run registers the new ID's CLSID; Run's end revokes it
// and leaves no object alive.
func TestToastActivatorClassObject(t *testing.T) {
	if inWine() {
		t.Skip("Wine has no WinRT toasts, so no app of it registers an activator")
	}
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const appID, otherID = "dev.telaui.tela-activator-test", "dev.telaui.tela-activator-test2"
	b.Desktop().SetAppID(appID)
	comServers.Lock()
	baseline := len(comServers.m)
	comServers.Unlock()
	timer := time.AfterFunc(10*time.Second, b.Quit)
	defer timer.Stop()
	var events []platform.NotificationEvent
	var registered, again string
	b.Wake()
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.WakeEvent:
			// Run registered nothing: the registry names no activator of
			// this ID. The first toast would register it.
			if b.desk.activator.clsid != "" {
				t.Errorf("Run registered %s without a CustomActivator in the registry", b.desk.activator.clsid)
			}
			b.startToastActivator()
			registered = b.desk.activator.clsid
			if windowsBuild() < toastMinBuild {
				b.Quit() // no toasts, so no activator: registered is ""
				return
			}
			clsid := guidOf(assoc.ToastActivatorCLSID(appID))
			var p uintptr
			r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, clsctxInprocServer|clsctxLocalServer,
				uintptr(unsafe.Pointer(&iidNotificationActivationCallback)), uintptr(unsafe.Pointer(&p)))
			if hr := int32(uint32(r)); hr < 0 || p == 0 {
				t.Errorf("CoCreateInstance(%s) = %#x", assoc.ToastActivatorCLSID(appID), uint32(hr))
				b.Quit()
				return
			}
			if hr := activateToast(comObject(p), toastArguments(2, "earlier", "save")); hr != sOK {
				t.Errorf("Activate = %#x", uint32(hr))
			}
			comObject(p).release()
		case platform.NotificationEvent:
			events = append(events, e)
			b.Desktop().SetAppID(otherID)
			again = b.desk.activator.clsid
			b.Quit()
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if windowsBuild() < toastMinBuild {
		if registered != "" {
			t.Errorf("registered %s on build %d, which has no toasts of unpackaged apps", registered, windowsBuild())
		}
		return
	}
	if registered != assoc.ToastActivatorCLSID(appID) {
		t.Errorf("registered under %q, want %s", registered, assoc.ToastActivatorCLSID(appID))
	}
	if want := []platform.NotificationEvent{{ID: 2, Kind: platform.NotificationActionInvoked, Action: "save", Earlier: true}}; !slices.Equal(events, want) {
		t.Errorf("events %+v, want %+v", events, want)
	}
	if again != assoc.ToastActivatorCLSID(otherID) {
		t.Errorf("after SetAppID(%q) registered under %q, want %s", otherID, again, assoc.ToastActivatorCLSID(otherID))
	}
	if b.desk.activator.clsid != "" {
		t.Errorf("still registered after Run")
	}
	comServers.Lock()
	left := len(comServers.m) - baseline
	comServers.Unlock()
	if left != 0 {
		t.Errorf("%d COM objects of the activator alive after Run", left)
	}
}

// prefixRegistry is a registry whose keys are below prefix, so a test
// writes the real HKCU without touching the keys of installed apps.
type prefixRegistry struct {
	reg    aumidRegistry
	prefix string
}

func (r prefixRegistry) String(key, name string) (string, bool, error) {
	return r.reg.String(r.prefix+key, name)
}
func (r prefixRegistry) SetString(key, name, value string) error {
	return r.reg.SetString(r.prefix+key, name, value)
}
func (r prefixRegistry) ValueNames(key string) ([]string, error) {
	return r.reg.ValueNames(r.prefix + key)
}
func (r prefixRegistry) SubKeys(key string) ([]string, error) { return r.reg.SubKeys(r.prefix + key) }

// deleteRegistryTree removes key below HKCU and everything under it.
func deleteRegistryTree(t *testing.T, key string) {
	t.Helper()
	subs, err := hkcuRegistry{}.SubKeys(key)
	if err != nil {
		t.Errorf("listing HKCU\\%s: %v", key, err)
		return
	}
	for _, s := range subs {
		deleteRegistryTree(t, key+`\`+s)
	}
	if err := winreg.DeleteKey(winreg.CURRENT_USER, key); err != nil && !regMissing(err) {
		t.Errorf("deleting HKCU\\%s: %v", key, err)
	}
}

// TestToastActivatorRegistry writes the AUMID and the activator into the
// real HKCU below a temporary key and reads them back through advapi32:
// the default value of LocalServer32, the subkeys of the CLSID key, and no
// write when nothing changed.
func TestToastActivatorRegistry(t *testing.T) {
	root := fmt.Sprintf(`Software\Tela-test-activator-%d-%d`, os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { deleteRegistryTree(t, root) })
	reg := prefixRegistry{hkcuRegistry{}, root + `\`}
	const aumid = "dev.telaui.tela-activator-test"
	clsid := assoc.ToastActivatorCLSID(aumid)
	owned, _, err := registerAUMID(reg, aumid, "tela-activator-test", "")
	if err != nil || !owned {
		t.Fatalf("registerAUMID: owned %v, %v", owned, err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := toastServerCommand(exe)
	if changed, err := registerToastActivator(reg, aumid, clsid, cmd); !changed || err != nil {
		t.Fatalf("registerToastActivator: changed %v, %v", changed, err)
	}
	server := clsidKeyPrefix + "{" + clsid + "}"
	if v, ok, err := reg.String(server+`\LocalServer32`, ""); !ok || err != nil || v != cmd {
		t.Errorf("LocalServer32 = %q, %v, %v; want %q", v, ok, err, cmd)
	}
	if !strings.HasPrefix(cmd, `"`+exe+`" `) || !strings.HasSuffix(cmd, assoc.ToastActivatorFlag) {
		t.Errorf("command %q", cmd)
	}
	if subs, err := reg.SubKeys(server); err != nil || !slices.Equal(subs, []string{"LocalServer32"}) {
		t.Errorf("subkeys of the CLSID key = %v, %v", subs, err)
	}
	if v, ok, _ := reg.String(aumidKeyPrefix+aumid, customActivatorValue); !ok || v != "{"+clsid+"}" {
		t.Errorf("CustomActivator = %q, %v", v, ok)
	}
	if changed, err := registerToastActivator(reg, aumid, clsid, cmd); changed || err != nil {
		t.Errorf("the same registration again: changed %v, %v", changed, err)
	}
	if subs, err := reg.SubKeys(`Software\Classes\CLSID\{00000000-0000-0000-0000-000000000000}`); subs != nil || err != nil {
		t.Errorf("SubKeys of a missing key = %v, %v; want nil, nil", subs, err)
	}
}
