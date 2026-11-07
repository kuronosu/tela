//go:build windows

package win32

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"
	utf16pkg "unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	winreg "golang.org/x/sys/windows/registry"

	"github.com/kuronosu/tela/internal/platform"
)

// Notifications as WinRT toasts (ADR-235; toast.go has the pure half).
//
//   - WinRT through combase.dll (ADR-230): RoGetActivationFactory for the
//     statics of Windows.UI.Notifications.ToastNotificationManager and the
//     factory of ToastNotification, RoActivateInstance for
//     Windows.Data.Xml.Dom.XmlDocument, HSTRINGs with WindowsCreateString.
//     Interfaces are called by vtable index (com_windows.go); the indices
//     below follow the order of the methods in the SDK's IDL, which the
//     windows crate's vtables (windows-rs 0.58) confirm.
//   - The app: an unpackaged app needs an AppUserModelID the shell knows.
//     The first toast registers it for the current user under
//     HKCU\Software\Classes\AppUserModelId\<AUMID> (DisplayName: the
//     executable's name; IconUri: a PNG of the app's window or tray icon in
//     the user cache), which Windows 10 1809 and later accept without a
//     Start menu shortcut; older builds keep the balloons. A key that Tela
//     did not create is used as it is, and nothing is ever removed.
//     SetAppID also sets the process's explicit AUMID
//     (SetCurrentProcessExplicitAppUserModelID), which ties the taskbar
//     buttons and the toasts to the same app.
//   - One toast per notification: Tag its ID, Group the run
//     (toastGroup), so CloseNotification removes it from the screen
//     (ToastNotifier.Hide) and the Action Center
//     (ToastNotificationHistory.RemoveGroupedTagWithId), and a toast shown
//     with the same tag and group replaces it (replaceToast).
//   - Events: a COM delegate object of this package per event (Activated,
//     Dismissed, Failed). Their interface IDs are those of parameterized
//     interfaces (TypedEventHandler<ToastNotification, …>), taken from the
//     SDK headers and derived again from their signatures by
//     TestPinterfaceIIDs. The delegates are agile (IAgileObject), so WinRT
//     calls them on its own threads; they only hand the event to the UI
//     thread (toastInbox, a private message), where it becomes a
//     NotificationEvent. A click while the app does not run reaches
//     nothing: Tela registers no COM activator (CustomActivator), so
//     Windows only removes the toast.
//   - Apartment: the UI thread is in OLE's single-threaded apartment
//     (initCOM); RoInitialize(RO_INIT_SINGLETHREADED) joins WinRT to it
//     (S_FALSE) and shutdown balances it before OleUninitialize.
//   - Fallback: without combase, WinRT or the toast classes (Windows 7 and
//     8, Windows 10 before 1809, Wine), or when a toast fails to show,
//     the notification is a balloon (tray_windows.go).

// combase and the shell32 procedure of the toasts, apart from procs.go.
var (
	combase = windows.NewLazySystemDLL("combase.dll") // WinRT toasts (ADR-230)

	procRoInitialize               = combase.NewProc("RoInitialize")
	procRoUninitialize             = combase.NewProc("RoUninitialize")
	procRoActivateInstance         = combase.NewProc("RoActivateInstance")
	procRoGetActivationFactory     = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString        = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString        = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer  = combase.NewProc("WindowsGetStringRawBuffer")
	procSetCurrentProcessAppUserID = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

// toastMinBuild is the first Windows build where a toast of an unpackaged
// app shows with only the per-user AppUserModelId registry key (no Start
// menu shortcut carrying the AUMID): Windows 10 1809, as reported by the
// projects that rely on it (the Windows App SDK registers unpackaged apps
// the same way). Older builds need the shortcut, which Tela does not
// create, so they keep the balloons.
const toastMinBuild = 17763

// toastIconSide is the side in pixels of the image picked for a toast's logo
// and the app's registered icon (shown at 48 logical pixels at most, 96 at
// 200%; the next common icon size up).
const toastIconSide = 128

// wmTelaToast is posted to the message-only window when a delegate queued
// toast events.
const wmTelaToast = 0x8000 + 0x60

// guidOf converts a literal of toast.go.
func guidOf(s string) guid {
	d1, d2, d3, d4 := parseGUIDFields(s)
	return guid{d1, d2, d3, d4}
}

var (
	iidAgileObject               = guidOf(iidStrAgileObject)
	iidToastManagerStatics       = guidOf(iidStrToastManagerStatics)
	iidToastManagerStatics2      = guidOf(iidStrToastManagerStatics2)
	iidToastFactory              = guidOf(iidStrToastFactory)
	iidToastNotification2        = guidOf(iidStrToastNotification2)
	iidToastActivatedEventArgs   = guidOf(iidStrToastActivatedEventArgs)
	iidXMLDocument               = guidOf(iidStrXMLDocument)
	iidXMLDocumentIO             = guidOf(iidStrXMLDocumentIO)
	iidToastHandlers             = [3]guid{guidOf(iidStrToastActivatedHandler), guidOf(iidStrToastDismissedHandler), guidOf(iidStrToastFailedHandler)}
	errToastsUnsupportedVersion  = fmt.Errorf("win32: toasts of an unpackaged app need Windows 10 1809 (build %d) or later", toastMinBuild)
	errToastsNoWinRT             = errors.New("win32: WinRT is missing (combase.dll)")
	errToastsMultithreadedThread = errors.New("win32: the UI thread is in COM's multithreaded apartment")
)

// Vtable indices. IInspectable adds GetIids, GetRuntimeClassName and
// GetTrustLevel (3 to 5) to IUnknown, so the methods of a WinRT interface
// start at 6; a delegate is an IUnknown with Invoke at 3.
const (
	vtStaticsCreateToastNotifierWithID = 6 + 1 // IToastNotificationManagerStatics: CreateToastNotifier, CreateToastNotifierWithId, GetTemplateContent
	vtStatics2History                  = 6 + 0 // IToastNotificationManagerStatics2: History
	vtHistoryRemoveGroupedTagWithID    = 6 + 2 // IToastNotificationHistory: RemoveGroup, RemoveGroupWithId, RemoveGroupedTagWithId, RemoveGroupedTag, Remove, Clear, ClearWithId
	vtNotifierShow                     = 6 + 0 // IToastNotifier: Show, Hide, Setting, AddToSchedule, RemoveFromSchedule, GetScheduledToastNotifications
	vtNotifierHide                     = 6 + 1
	vtNotifierSetting                  = 6 + 2
	vtFactoryCreateToastNotification   = 6 + 0 // IToastNotificationFactory: CreateToastNotification
	vtToast2PutTag                     = 6 + 0 // IToastNotification2: put_Tag, get_Tag, put_Group, get_Group, put_SuppressPopup, get_SuppressPopup
	vtToast2PutGroup                   = 6 + 2
	vtXMLIOLoadXML                     = 6 + 0 // IXmlDocumentIO: LoadXml, LoadXmlWithSettings, SaveToFileAsync
	vtActivatedArgsArguments           = 6 + 0 // IToastActivatedEventArgs: get_Arguments
	vtDismissedArgsReason              = 6 + 0 // IToastDismissedEventArgs: get_Reason
	vtDelegateInvoke                   = 3
)

// The add and remove methods of IToastNotification's events (get_Content,
// put_ExpirationTime, get_ExpirationTime, then add_/remove_ Dismissed,
// Activated, Failed), by toastEventKind.
var (
	vtToastAddEvent    = [3]int{6 + 5, 6 + 3, 6 + 7}
	vtToastRemoveEvent = [3]int{6 + 6, 6 + 4, 6 + 8}
)

// NotificationSetting values that mean the user or a policy turned the
// app's notifications off (windows.ui.notifications.h).
const (
	notificationDisabledForApplication = 1
	notificationDisabledForUser        = 2
	notificationDisabledByGroupPolicy  = 3
)

// toastState is the state of the toasts, on the UI thread.
type toastState struct {
	off   bool   // balloons only (tests)
	appID string // SetAppID's ID, "" until it is called

	tried  bool  // startToasts ran
	err    error // why toasts are unavailable
	roInit bool  // RoInitialize needs RoUninitialize

	statics  comObject // IToastNotificationManagerStatics
	factory  comObject // IToastNotificationFactory
	history  comObject // IToastNotificationHistory, or 0
	notifier comObject // IToastNotifier for aumid
	aumid    string
	group    string

	registered string   // the AUMID registered in this run
	iconDir    string   // the icon files of registered
	icons      []string // the icon files used in this run
	pruned     bool

	inbox   *toastInbox
	entries map[platform.NotificationID]*toastEntry
	serial  uint64
}

// toastEntry is a toast on screen or in the Action Center.
type toastEntry struct {
	id       platform.NotificationID
	serial   uint64
	tag      string
	aumid    string
	toast    comObject // IToastNotification
	notifier comObject // the IToastNotifier that showed it (a reference)
	handlers [3]*comServer
	tokens   [3]int64
	added    [3]bool
}

// toastInbox carries the events from the delegates, on WinRT's threads, to
// the UI thread.
type toastInbox struct {
	hwnd   uintptr // the message-only window
	mu     sync.Mutex
	events []queuedToastEvent
	posted bool // a wmTelaToast is in the queue
	closed bool // Run returned
}

// queuedToastEvent is a toast event with the serial of the toast object
// that raised it: the events of a replaced toast are dropped.
type queuedToastEvent struct {
	toastEvent
	serial uint64
}

func (q *toastInbox) push(e queuedToastEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.events = append(q.events, e)
	if !q.posted {
		q.posted = postMessage(q.hwnd, wmTelaToast, 0, 0)
	}
}

func (q *toastInbox) take() []queuedToastEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	ev := q.events
	q.events, q.posted = nil, false
	return ev
}

func (q *toastInbox) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed, q.events = true, nil
}

// --- HSTRING and activation ---

// hstring is an HSTRING; 0 is the empty string.
type hstring uintptr

// newHString creates an HSTRING holding s (invalid UTF-8 becomes U+FFFD);
// the caller frees it.
func newHString(s string) (hstring, error) {
	if s == "" {
		return 0, nil
	}
	u := utf16pkg.Encode([]rune(s))
	var h hstring
	r, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&h)))
	if hr := int32(uint32(r)); hr < 0 {
		return 0, hresultError("WindowsCreateString", hr)
	}
	return h, nil
}

func (h hstring) free() {
	if h != 0 {
		procWindowsDeleteString.Call(uintptr(h))
	}
}

// String returns the text of h.
func (h hstring) String() string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := procWindowsGetStringRawBuffer.Call(uintptr(h), uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	return string(utf16pkg.Decode(unsafe.Slice((*uint16)(ptrFromUintptr(p)), n)))
}

// query returns the interface iid of o (QueryInterface), which the caller
// releases.
func (o comObject) query(iid *guid) (comObject, int32) {
	var p uintptr
	hr := o.call(vtQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&p)))
	if hr < 0 {
		return 0, hr
	}
	return comObject(p), hr
}

// activationFactory returns the activation factory (or statics) iid of a
// runtime class.
func activationFactory(class string, iid *guid) (comObject, error) {
	h, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer h.free()
	var f uintptr
	r, _, _ := procRoGetActivationFactory.Call(uintptr(h), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f)))
	if hr := int32(uint32(r)); hr < 0 || f == 0 {
		return 0, hresultError("RoGetActivationFactory("+class+")", hr)
	}
	return comObject(f), nil
}

// activateInstance creates an instance of a runtime class with its default
// constructor; the result is its IInspectable.
func activateInstance(class string) (comObject, error) {
	h, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer h.free()
	var p uintptr
	r, _, _ := procRoActivateInstance.Call(uintptr(h), uintptr(unsafe.Pointer(&p)))
	if hr := int32(uint32(r)); hr < 0 || p == 0 {
		return 0, hresultError("RoActivateInstance("+class+")", hr)
	}
	return comObject(p), nil
}

// tokenArgs passes an EventRegistrationToken (a 64-bit struct) by value:
// one register on 64-bit Windows, two stack words on 386.
func tokenArgs(tok int64) []uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return []uintptr{uintptr(tok)}
	}
	return []uintptr{uintptr(uint32(tok)), uintptr(uint32(uint64(tok) >> 32))}
}

// --- the delegates ---

// toastHandler is the Go side of a delegate object: which toast and event
// it reports. Immutable, so WinRT's threads read it freely.
type toastHandler struct {
	inbox  *toastInbox
	id     platform.NotificationID
	serial uint64
	kind   toastEventKind
}

var (
	toastVtblOnce    sync.Once
	toastHandlerVtbl [4]uintptr // IUnknown + Invoke, shared by the three delegate types
)

// newToastHandler creates a delegate object with one reference, the
// caller's. It answers IUnknown, its delegate interface and IAgileObject.
func newToastHandler(h *toastHandler) *comServer {
	toastVtblOnce.Do(func() {
		toastHandlerVtbl = [4]uintptr{
			syscall.NewCallback(comQueryInterface),
			syscall.NewCallback(comAddRef),
			syscall.NewCallback(comRelease),
			syscall.NewCallback(toastInvoke),
		}
	})
	return newComServer(&toastHandlerVtbl[0], []guid{iidToastHandlers[h.kind], iidAgileObject}, h, nil)
}

// toastInvoke is ITypedEventHandler::Invoke(sender, args), called on any
// thread: it reads what the arguments say and hands the event to the UI
// thread. sender and args are borrowed.
func toastInvoke(this, sender, args uintptr) uintptr {
	s := comServerOf(this)
	if s == nil {
		return sOK
	}
	h, ok := s.impl.(*toastHandler)
	if !ok {
		return sOK
	}
	ev := queuedToastEvent{toastEvent{id: h.id, kind: h.kind}, h.serial}
	switch h.kind {
	case toastActivated:
		// args is an IInspectable; a toast's is IToastActivatedEventArgs.
		if args != 0 {
			if a, hr := comObject(args).query(&iidToastActivatedEventArgs); hr >= 0 && a != 0 {
				var hs hstring
				if a.call(vtActivatedArgsArguments, uintptr(unsafe.Pointer(&hs))) >= 0 {
					ev.args = hs.String()
					hs.free()
				}
				a.release()
			}
		}
	case toastDismissed:
		ev.reason = toastUserCanceled
		if args != 0 {
			var reason int32
			if comObject(args).call(vtDismissedArgsReason, uintptr(unsafe.Pointer(&reason))) >= 0 {
				ev.reason = reason
			}
		}
	}
	h.inbox.push(ev)
	return sOK
}

// --- the toasts ---

// startToasts makes WinRT usable on the UI thread and gets the toast
// classes, once; a failure is kept and every notification is a balloon.
func (b *Backend) startToasts() error {
	t := &b.desk.toast
	if t.tried {
		return t.err
	}
	t.tried = true
	if err := b.openToasts(); err != nil {
		b.releaseToasts()
		t.tried, t.err = true, err
	}
	return t.err
}

func (b *Backend) openToasts() error {
	t := &b.desk.toast
	switch {
	case windowsBuild() < toastMinBuild:
		return errToastsUnsupportedVersion
	case !available(procRoInitialize), !available(procRoGetActivationFactory), !available(procRoActivateInstance),
		!available(procWindowsCreateString), !available(procWindowsDeleteString), !available(procWindowsGetStringRawBuffer):
		return errToastsNoWinRT
	}
	if err := b.desk.initCOM(); err != nil {
		return err
	}
	const roInitSingleThreaded = 0
	r, _, _ := procRoInitialize.Call(roInitSingleThreaded)
	switch hr := int32(uint32(r)); hr {
	case sOK, sFalse:
		t.roInit = true
	case hrChangedMode:
		return errToastsMultithreadedThread
	default:
		return hresultError("RoInitialize", hr)
	}
	var err error
	if t.statics, err = activationFactory(toastManagerRuntimeClass, &iidToastManagerStatics); err != nil {
		return err
	}
	if t.factory, err = activationFactory(toastNotificationRuntimeClass, &iidToastFactory); err != nil {
		return err
	}
	if s2, hr := t.statics.query(&iidToastManagerStatics2); hr >= 0 {
		var h uintptr
		if s2.call(vtStatics2History, uintptr(unsafe.Pointer(&h))) >= 0 {
			t.history = comObject(h)
		}
		s2.release()
	}
	t.inbox = &toastInbox{hwnd: b.msgHwnd}
	t.entries = map[platform.NotificationID]*toastEntry{}
	t.group = toastGroup(b.start)
	return nil
}

// releaseToasts releases the WinRT objects and leaves WinRT; the app ID and
// the test switch stay.
func (b *Backend) releaseToasts() {
	t := &b.desk.toast
	if t.inbox != nil {
		t.inbox.close()
	}
	for _, e := range t.entries {
		b.finishToast(e, false)
	}
	t.notifier.release()
	t.history.release()
	t.factory.release()
	t.statics.release()
	if t.roInit {
		procRoUninitialize.Call()
	}
	*t = toastState{off: t.off, appID: t.appID}
}

// toastShutdown runs when Run returns, before COM is left. The toasts stay
// in the Action Center (as other apps' do), with nobody to answer them.
func (b *Backend) toastShutdown() { b.releaseToasts() }

// setToastAppID records the app ID (desktop.SetAppID) and sets the
// process's explicit AppUserModelID, which Windows wants before the first
// window shows. It also groups the app's taskbar buttons under that ID
// rather than the executable's path.
func (b *Backend) setToastAppID(id string) {
	b.desk.toast.appID = id
	if available(procSetCurrentProcessAppUserID) {
		procSetCurrentProcessAppUserID.Call(uintptr(unsafe.Pointer(utf16(toastAUMID(id)))))
	}
}

// toastAppAUMID returns the AppUserModelID of the app.
func (b *Backend) toastAppAUMID() string {
	id := b.desk.toast.appID
	if id == "" {
		id = platform.ServiceAppID(platform.DefaultAppID())
	}
	return toastAUMID(id)
}

// toastNotifier returns the ToastNotifier of the app's AUMID, registering
// the AUMID first.
func (b *Backend) toastNotifier() (comObject, string, error) {
	t := &b.desk.toast
	aumid := b.toastAppAUMID()
	if t.notifier != 0 && t.aumid == aumid {
		return t.notifier, aumid, nil
	}
	b.registerToastApp(aumid)
	h, err := newHString(aumid)
	if err != nil {
		return 0, "", err
	}
	defer h.free()
	var n uintptr
	if hr := t.statics.call(vtStaticsCreateToastNotifierWithID, uintptr(h), uintptr(unsafe.Pointer(&n))); hr < 0 || n == 0 {
		return 0, "", hresultError("CreateToastNotifierWithId", hr)
	}
	t.notifier.release()
	t.notifier, t.aumid = comObject(n), aumid
	return t.notifier, aumid, nil
}

// registerToastApp registers aumid for the current user once per run
// (registerAUMID writes only what changed). A failure is not reported: the
// toast may show anyway, under the AUMID itself.
func (b *Backend) registerToastApp(aumid string) {
	t := &b.desk.toast
	if t.registered == aumid {
		return
	}
	t.registered, t.icons, t.pruned = aumid, nil, false
	t.iconDir = ""
	if cache, err := os.UserCacheDir(); err == nil {
		t.iconDir = filepath.Join(cache, "Tela", "notifications", sanitizedAppID(aumid))
	}
	reg := hkcuRegistry{}
	if old, ok, _ := reg.String(aumidKeyPrefix+aumid, "IconUri"); ok {
		t.icons = append(t.icons, old) // kept by pruning while registered
	}
	icon := ""
	if img := b.appImage(); img != nil {
		icon = b.toastIconPath(img)
	}
	_, _, _ = registerAUMID(reg, aumid, platform.DefaultAppID(), icon)
}

// appImage returns the app's icon at the toast size: the first window's
// icon (Window.SetIcon), else the first tray icon's, else nil.
func (b *Backend) appImage() image.Image {
	for _, w := range b.loop.Windows() {
		if len(w.icon) > 0 {
			return platform.PickIcon(w.icon, toastIconSide)
		}
	}
	if ic := b.findIcon(func(ic *shellIcon) bool { return ic.tray != 0 && len(ic.images) > 0 }); ic != nil {
		return platform.PickIcon(ic.images, toastIconSide)
	}
	return nil
}

// toastIconPath writes img to the icon cache and returns its path ("" on
// failure). The first write of a run prunes the files no longer used.
func (b *Backend) toastIconPath(img image.Image) string {
	t := &b.desk.toast
	if t.iconDir == "" || img == nil {
		return ""
	}
	now := time.Now()
	p, err := writeToastIcon(t.iconDir, img, now)
	if err != nil {
		return ""
	}
	if !slices.Contains(t.icons, p) {
		t.icons = append(t.icons, p)
	}
	if !t.pruned {
		t.pruned = true
		pruneToastIcons(t.iconDir, t.icons, now)
	}
	return p
}

// showToast shows n as a toast. ok is false when the caller should show a
// balloon instead: toasts are unavailable, or this one could not be shown.
func (b *Backend) showToast(n platform.Notification) (id platform.NotificationID, ok bool, err error) {
	t := &b.desk.toast
	if t.off || b.startToasts() != nil {
		return 0, false, nil
	}
	notifier, aumid, err := b.toastNotifier()
	if err != nil {
		return 0, false, nil
	}
	var setting int32
	if notifier.call(vtNotifierSetting, uintptr(unsafe.Pointer(&setting))) >= 0 {
		switch setting {
		case notificationDisabledForApplication, notificationDisabledForUser, notificationDisabledByGroupPolicy:
			return 0, true, fmt.Errorf("win32: Notify: the notifications of %s are turned off in Windows (NotificationSetting %d): %w", aumid, setting, platform.ErrUnsupported)
		}
	}
	id = b.desk.lastNote + 1
	if err := b.raiseToast(id, n, notifier, aumid); err != nil {
		return 0, false, nil
	}
	b.desk.lastNote = id
	return id, true, nil
}

// replaceToast shows n in place of the toast of notification id, under the
// same ID: a toast with the same tag and group replaces the old one on
// screen and in the Action Center. The contract has no request for it yet
// (the live tests use it).
func (b *Backend) replaceToast(id platform.NotificationID, n platform.Notification) error {
	t := &b.desk.toast
	old := t.entries[id]
	if old == nil {
		return fmt.Errorf("win32: replaceToast: notification %d is not a toast on screen", id)
	}
	if err := b.raiseToast(id, n, old.notifier, old.aumid); err != nil {
		t.entries[id] = old
		return err
	}
	b.finishToast(old, false)
	return nil
}

// raiseToast builds the toast of notification id, registers it in entries
// (before Show: an outgoing COM call may dispatch the messages that deliver
// its events) and shows it.
func (b *Backend) raiseToast(id platform.NotificationID, n platform.Notification, notifier comObject, aumid string) error {
	t := &b.desk.toast
	content := toastContent{ID: id, Title: n.Title, Body: n.Body, Actions: n.Actions}
	if len(n.Icon) > 0 {
		if p := b.toastIconPath(platform.PickIcon(n.Icon, toastIconSide)); p != "" {
			content.Image = fileURI(p)
		}
	}
	doc, err := loadXMLDocument(content.xml())
	if err != nil {
		return err
	}
	var toast uintptr
	hr := t.factory.call(vtFactoryCreateToastNotification, uintptr(doc), uintptr(unsafe.Pointer(&toast)))
	doc.release()
	if hr < 0 || toast == 0 {
		return hresultError("CreateToastNotification", hr)
	}
	t.serial++
	e := &toastEntry{id: id, serial: t.serial, tag: fmt.Sprint(id), aumid: aumid, toast: comObject(toast), notifier: notifier}
	notifier.call(vtAddRef)
	if t2, hr := e.toast.query(&iidToastNotification2); hr >= 0 {
		setHStringProperty(t2, vtToast2PutTag, e.tag)
		setHStringProperty(t2, vtToast2PutGroup, t.group)
		t2.release()
	}
	for kind := range e.handlers {
		e.handlers[kind] = newToastHandler(&toastHandler{inbox: t.inbox, id: id, serial: e.serial, kind: toastEventKind(kind)})
		if e.toast.call(vtToastAddEvent[kind], e.handlers[kind].ptr(), uintptr(unsafe.Pointer(&e.tokens[kind]))) >= 0 {
			e.added[kind] = true
		}
	}
	t.entries[id] = e
	if hr := notifier.call(vtNotifierShow, uintptr(e.toast)); hr < 0 {
		b.finishToast(e, false)
		return hresultError("ToastNotifier.Show", hr)
	}
	return nil
}

// loadXMLDocument returns an IXmlDocument loaded with s.
func loadXMLDocument(s string) (comObject, error) {
	inst, err := activateInstance(xmlDocumentRuntimeClass)
	if err != nil {
		return 0, err
	}
	defer inst.release()
	io, hr := inst.query(&iidXMLDocumentIO)
	if hr < 0 {
		return 0, hresultError("XmlDocument as IXmlDocumentIO", hr)
	}
	h, err := newHString(s)
	if err != nil {
		io.release()
		return 0, err
	}
	hr = io.call(vtXMLIOLoadXML, uintptr(h))
	h.free()
	io.release()
	if hr < 0 {
		return 0, hresultError("XmlDocument.LoadXml", hr)
	}
	doc, hr := inst.query(&iidXMLDocument)
	if hr < 0 {
		return 0, hresultError("XmlDocument as IXmlDocument", hr)
	}
	return doc, nil
}

// setHStringProperty calls a put_ method that takes an HSTRING.
func setHStringProperty(o comObject, method int, value string) {
	h, err := newHString(value)
	if err != nil {
		return
	}
	o.call(method, uintptr(h))
	h.free()
}

// closeToast withdraws the toast of notification id, reporting whether
// there was one. Its handlers go first, so its Dismissed (ApplicationHidden)
// reports nothing, as CloseNotification wants.
func (b *Backend) closeToast(id platform.NotificationID) bool {
	e := b.desk.toast.entries[id]
	if e == nil || id == 0 {
		return false
	}
	b.finishToast(e, true)
	return true
}

// finishToast forgets a toast and releases its objects; hide also takes it
// off the screen and out of the Action Center.
func (b *Backend) finishToast(e *toastEntry, hide bool) {
	t := &b.desk.toast
	if t.entries[e.id] == e {
		delete(t.entries, e.id)
	}
	for kind, h := range e.handlers {
		if e.added[kind] {
			e.toast.call(vtToastRemoveEvent[kind], tokenArgs(e.tokens[kind])...)
			e.added[kind] = false
		}
		if h != nil {
			h.release()
			e.handlers[kind] = nil
		}
	}
	if hide {
		e.notifier.call(vtNotifierHide, uintptr(e.toast))
		if t.history != 0 {
			tag, err1 := newHString(e.tag)
			group, err2 := newHString(t.group)
			aumid, err3 := newHString(e.aumid)
			if err1 == nil && err2 == nil && err3 == nil {
				t.history.call(vtHistoryRemoveGroupedTagWithID, uintptr(tag), uintptr(group), uintptr(aumid))
			}
			tag.free()
			group.free()
			aumid.free()
		}
	}
	e.toast.release()
	e.notifier.release()
	e.toast, e.notifier = 0, 0
}

// toastMessages delivers the events the delegates queued.
func (b *Backend) toastMessages() {
	t := &b.desk.toast
	if t.inbox == nil {
		return
	}
	queued := false
	for _, ev := range t.inbox.take() {
		e := t.entries[ev.id]
		if e == nil || e.serial != ev.serial {
			continue // closed, finished or replaced meanwhile
		}
		ne, report, finished := ev.notificationEvent()
		if finished {
			b.finishToast(e, false)
		}
		if report {
			b.enqueue(nil, ne)
			queued = true
		}
	}
	if queued {
		b.notify()
	}
}

// --- the registry ---

// hkcuRegistry is HKEY_CURRENT_USER for registerAUMID (advapi32 through
// golang.org/x/sys/windows/registry).
type hkcuRegistry struct{}

var _ aumidRegistry = hkcuRegistry{}

func regMissing(err error) bool {
	return errors.Is(err, winreg.ErrNotExist) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

func (hkcuRegistry) String(key, name string) (string, bool, error) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE)
	if regMissing(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	switch {
	case regMissing(err), errors.Is(err, winreg.ErrUnexpectedType):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return v, true, nil
}

func (hkcuRegistry) SetString(key, name, value string) error {
	k, _, err := winreg.CreateKey(winreg.CURRENT_USER, key, winreg.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (hkcuRegistry) ValueNames(key string) ([]string, error) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE)
	if regMissing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if names == nil && err == nil {
		names = []string{}
	}
	return names, err
}
