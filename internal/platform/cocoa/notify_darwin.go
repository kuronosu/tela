//go:build darwin

package cocoa

import (
	"fmt"
	"image/png"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Notifications are UNUserNotificationCenter requests (ADR-224), which
// macOS shows only for an app bundle with a bundle identifier: Notify
// first checks +[NSBundle mainBundle] (a path ending in .app and a
// bundleIdentifier), and outside one returns an error wrapping
// ErrUnsupported that names `tela run`, without touching the center (it
// raises an Objective-C exception there, which cannot unwind through Go).
// In a bundle:
//
//   - Authorization (alert and sound) is requested at the first Notify.
//     Notifications asked for before the answer wait for it. When it is
//     denied (now, or earlier in System Settings), each waiting
//     notification is reported NotificationClosed, so the app's handlers
//     run, and later Notify calls return an error wrapping ErrUnsupported
//     that names System Settings; each such call asks again in the
//     background, so turning notifications back on takes effect at the
//     next Notify.
//   - Each notification is a UNNotificationRequest with no trigger (shown
//     now), identified by a per-run prefix and the NotificationID; the
//     title and body are the content's, with the default sound, and the
//     icon is an attachment (a PNG of the image that fits 256 pixels,
//     written to a temporary file that the center moves into its store),
//     shown as the notification's thumbnail. The app's icon is the
//     notification's own.
//   - Actions are UNNotificationActions of a category, one category per
//     distinct list of actions (services.go names them). Every category has
//     the custom dismiss option, so a dismissal is reported.
//   - The center's delegate (TelaNotificationDelegate) reports a response:
//     the default action is NotificationActivated, the dismiss action
//     NotificationClosed, another action NotificationActionInvoked; its
//     completion handler is called at once. It also shows notifications
//     while the app is in front (willPresentNotification: with banner,
//     list and sound), which macOS otherwise hides.
//   - CloseNotification removes the request and the delivered
//     notification. Expired notifications are not reported (the center
//     does not say).
//
// The center calls its completion handlers and, possibly, its delegate
// on a thread of its own: they hop to the main thread (onMain).

// notifyState is the backend's notification state, on the main thread.
type notifyState struct {
	prefix   string    // request identifiers of this run: prefix + ID
	center   darwin.ID // UNUserNotificationCenter (shared)
	delegate darwin.ID // TelaNotificationDelegate (owned)
	granted  bool      // authorization answered yes
	denied   bool      // authorization answered no
	asking   bool      // a request for authorization is out
	waiting  []*pendingNote
	last     platform.NotificationID
	live     map[platform.NotificationID]bool // shown or waiting, not closed
	// categories are the registered UNNotificationCategories (owned),
	// by identifier.
	categories map[string]darwin.ID
}

// pendingNote is a request made but not added to the center yet.
type pendingNote struct {
	id      platform.NotificationID
	request darwin.ID // UNNotificationRequest (owned)
	icon    string    // the attachment's temporary file, or ""
}

// nsel holds the selectors of the notification code.
var nsel = struct {
	mainBundle, bundlePath, bundleIdentifier, currentCenter, setDelegate    darwin.SEL
	requestAuthorization, addRequest, removePending, removeDelivered        darwin.SEL
	setTitle, setBody, setSound, defaultSound, setCategoryIdentifier        darwin.SEL
	setAttachments, attachment, requestWithIdentifier, actionWithIdentifier darwin.SEL
	categoryWithIdentifier, setCategories, setWithArray, actionIdentifier   darwin.SEL
	notification, request, identifier                                       darwin.SEL
}{
	mainBundle:             darwin.Sel("mainBundle"),
	bundlePath:             darwin.Sel("bundlePath"),
	bundleIdentifier:       darwin.Sel("bundleIdentifier"),
	currentCenter:          darwin.Sel("currentNotificationCenter"),
	setDelegate:            darwin.Sel("setDelegate:"),
	requestAuthorization:   darwin.Sel("requestAuthorizationWithOptions:completionHandler:"),
	addRequest:             darwin.Sel("addNotificationRequest:withCompletionHandler:"),
	removePending:          darwin.Sel("removePendingNotificationRequestsWithIdentifiers:"),
	removeDelivered:        darwin.Sel("removeDeliveredNotificationsWithIdentifiers:"),
	setTitle:               darwin.Sel("setTitle:"),
	setBody:                darwin.Sel("setBody:"),
	setSound:               darwin.Sel("setSound:"),
	defaultSound:           darwin.Sel("defaultSound"),
	setCategoryIdentifier:  darwin.Sel("setCategoryIdentifier:"),
	setAttachments:         darwin.Sel("setAttachments:"),
	attachment:             darwin.Sel("attachmentWithIdentifier:URL:options:error:"),
	requestWithIdentifier:  darwin.Sel("requestWithIdentifier:content:trigger:"),
	actionWithIdentifier:   darwin.Sel("actionWithIdentifier:title:options:"),
	categoryWithIdentifier: darwin.Sel("categoryWithIdentifier:actions:intentIdentifiers:options:"),
	setCategories:          darwin.Sel("setNotificationCategories:"),
	setWithArray:           darwin.Sel("setWithArray:"),
	actionIdentifier:       darwin.Sel("actionIdentifier"),
	notification:           darwin.Sel("notification"),
	request:                darwin.Sel("request"),
	identifier:             darwin.Sel("identifier"),
}

// mainBundleUsable reports whether the process runs from an app bundle
// that the notification center accepts (bundled in services.go), once.
var mainBundleUsable = sync.OnceValue(func() bool {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	bundle := darwin.SendClass(darwin.GetClass("NSBundle"), nsel.mainBundle)
	if bundle == 0 {
		return false
	}
	return bundled(darwin.GoString(darwin.Send(bundle, nsel.bundlePath)),
		darwin.GoString(darwin.Send(bundle, nsel.bundleIdentifier)))
})

// Notify shows a notification; see the top of this file.
func (d desktop) Notify(n platform.Notification) (platform.NotificationID, error) {
	platform.CheckNotification(n)
	b := d.b
	if b.closed {
		return 0, errClosed
	}
	if !mainBundleUsable() {
		return 0, errNoBundle
	}
	if !darwin.LoadUserNotifications() {
		return 0, fmt.Errorf("cocoa: Notify: the UserNotifications framework is missing: %w", platform.ErrUnsupported)
	}
	if err := setupDesktop(); err != nil {
		return 0, errNoHelpers("Notify", err)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	s := &b.desk.notes
	if s.center == 0 && !b.startNotifications() {
		return 0, fmt.Errorf("cocoa: Notify: no UNUserNotificationCenter: %w", platform.ErrUnsupported)
	}
	if s.denied {
		b.askAuthorization() // turned back on in System Settings: the next call works
		return 0, errNotificationsDenied
	}
	s.last++
	p := &pendingNote{id: s.last}
	p.request, p.icon = b.noteRequest(p.id, n)
	if p.request == 0 {
		return 0, fmt.Errorf("cocoa: Notify: creating the notification request failed")
	}
	s.live[p.id] = true
	if s.granted {
		b.addNote(p)
	} else {
		s.waiting = append(s.waiting, p)
		b.askAuthorization()
	}
	return p.id, nil
}

// CloseNotification withdraws a notification (pending or delivered).
// Unknown and closed IDs, and every ID outside a bundle, are ignored.
func (d desktop) CloseNotification(id platform.NotificationID) error {
	b := d.b
	s := &b.desk.notes
	if id == 0 || !s.live[id] {
		return nil
	}
	delete(s.live, id) // a waiting request is dropped when authorization answers
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	ids := darwin.NSArray(darwin.AutoString(noteIdentifier(s.prefix, id)))
	darwin.Send(s.center, nsel.removePending, ids)
	darwin.Send(s.center, nsel.removeDelivered, ids)
	return nil
}

// startNotifications gets the center and sets its delegate, once per run.
func (b *Backend) startNotifications() bool {
	s := &b.desk.notes
	s.center = darwin.SendClass(darwin.GetClass("UNUserNotificationCenter"), nsel.currentCenter)
	if s.center == 0 {
		return false
	}
	s.prefix = fmt.Sprintf("dev.telaui.%d.%d.", os.Getpid(), time.Now().UnixNano())
	s.live = map[platform.NotificationID]bool{}
	s.categories = map[string]darwin.ID{}
	s.delegate = darwin.Send(darwin.SendClass(deskClasses.notificationDelegate, sel.alloc), sel.init)
	deskObjects.Lock()
	deskObjects.notes[s.delegate] = b
	deskObjects.Unlock()
	darwin.Send(s.center, nsel.setDelegate, s.delegate)
	return true
}

// stopNotifications detaches the delegate when Run returns. Delivered
// notifications stay in Notification Center, as other apps' do.
func (b *Backend) stopNotifications() {
	s := &b.desk.notes
	if s.delegate == 0 {
		return
	}
	deskObjects.Lock()
	delete(deskObjects.notes, s.delegate)
	deskObjects.Unlock()
	darwin.Send(s.center, nsel.setDelegate, darwin.ID(0))
	darwin.Release(s.delegate)
	s.delegate = 0
	for _, p := range s.waiting {
		p.drop()
	}
	s.waiting = nil
	for _, c := range s.categories {
		darwin.Release(c)
	}
	s.categories = nil
}

// askAuthorization asks for alert and sound authorization; the answer
// arrives on another thread and is handled on the main thread.
func (b *Backend) askAuthorization() {
	s := &b.desk.notes
	if s.asking {
		return
	}
	s.asking = true
	handler := darwin.NewBlock(func(_ darwin.Block, granted bool, _ darwin.ID) {
		b.onMain(func() { b.authorized(granted) })
	})
	defer handler.Release()
	darwin.Send(s.center, nsel.requestAuthorization,
		uint(darwin.UNAuthorizationOptionAlert|darwin.UNAuthorizationOptionSound), handler)
}

// authorized handles the authorization answer: the waiting notifications
// are added, or reported closed.
func (b *Backend) authorized(granted bool) {
	s := &b.desk.notes
	s.asking = false
	s.granted, s.denied = granted, !granted
	waiting := s.waiting
	s.waiting = nil
	for _, p := range waiting {
		switch {
		case !s.live[p.id]: // closed meanwhile
			p.drop()
		case granted:
			b.addNote(p)
		default:
			delete(s.live, p.id)
			p.drop()
			b.appEvent(platform.NotificationEvent{ID: p.id, Kind: platform.NotificationClosed})
		}
	}
}

// addNote adds a request to the center. A refused request is reported
// closed.
func (b *Backend) addNote(p *pendingNote) {
	s := &b.desk.notes
	id, icon := p.id, p.icon
	handler := darwin.NewBlock(func(_ darwin.Block, err darwin.ID) {
		if icon != "" {
			os.Remove(icon) // moved into the center's store when it was accepted
		}
		if err != 0 {
			b.onMain(func() {
				if b.desk.notes.live[id] {
					delete(b.desk.notes.live, id)
					b.appEvent(platform.NotificationEvent{ID: id, Kind: platform.NotificationClosed})
				}
			})
		}
	})
	defer handler.Release()
	darwin.Send(s.center, nsel.addRequest, p.request, handler)
	darwin.Release(p.request)
	p.request = 0
}

// drop releases a request that is not added.
func (p *pendingNote) drop() {
	darwin.Release(p.request)
	p.request = 0
	if p.icon != "" {
		os.Remove(p.icon)
	}
}

// noteRequest builds the request of notification id (+1), and returns the
// icon's temporary file.
func (b *Backend) noteRequest(id platform.NotificationID, n platform.Notification) (darwin.ID, string) {
	s := &b.desk.notes
	content := darwin.Send(darwin.SendClass(darwin.GetClass("UNMutableNotificationContent"), sel.alloc), sel.init)
	if content == 0 {
		return 0, ""
	}
	defer darwin.Release(content)
	darwin.Send(content, nsel.setTitle, darwin.AutoString(n.Title))
	darwin.Send(content, nsel.setBody, darwin.AutoString(n.Body))
	if sound := darwin.SendClass(darwin.GetClass("UNNotificationSound"), nsel.defaultSound); sound != 0 {
		darwin.Send(content, nsel.setSound, sound)
	}
	darwin.Send(content, nsel.setCategoryIdentifier, darwin.AutoString(b.noteCategory(n.Actions)))
	icon := ""
	if len(n.Icon) > 0 {
		if path, err := writeNoteIcon(n); err == nil {
			out := new(darwin.ID) // the NSError out-parameter, on the heap
			att := darwin.SendClass(darwin.GetClass("UNNotificationAttachment"), nsel.attachment,
				darwin.AutoString("icon"), darwin.FileURL(path, false), darwin.ID(0), unsafe.Pointer(out))
			if att != 0 {
				darwin.Send(content, nsel.setAttachments, darwin.NSArray(att))
				icon = path
			} else {
				os.Remove(path)
			}
		}
	}
	req := darwin.SendClass(darwin.GetClass("UNNotificationRequest"), nsel.requestWithIdentifier,
		darwin.AutoString(noteIdentifier(s.prefix, id)), content, darwin.ID(0))
	return darwin.Retain(req), icon
}

// writeNoteIcon writes the notification's icon, at the size that fits
// 256 pixels, to a temporary PNG file.
func writeNoteIcon(n platform.Notification) (string, error) {
	img := platform.IconNRGBA(platform.PickIcon(n.Icon, 256))
	f, err := os.CreateTemp("", "tela-notification-*.png")
	if err != nil {
		return "", err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// noteCategory returns the category identifier of a notification with
// actions, registering the category with the center the first time.
func (b *Backend) noteCategory(actions []platform.NotificationAction) string {
	s := &b.desk.notes
	id := noteCategory(actions)
	if _, ok := s.categories[id]; ok {
		return id
	}
	list := darwin.NewMutableArray()
	actionClass := darwin.GetClass("UNNotificationAction")
	for _, a := range actions {
		if act := darwin.SendClass(actionClass, nsel.actionWithIdentifier, darwin.AutoString(a.ID), darwin.AutoString(a.Label), uint(0)); act != 0 {
			darwin.ArrayAdd(list, act)
		}
	}
	cat := darwin.SendClass(darwin.GetClass("UNNotificationCategory"), nsel.categoryWithIdentifier,
		darwin.AutoString(id), list, darwin.NSArray(), uint(darwin.UNNotificationCategoryOptionCustomDismissAction))
	if cat == 0 {
		return id
	}
	s.categories[id] = darwin.Retain(cat)
	all := darwin.NewMutableArray()
	for _, c := range s.categories {
		darwin.ArrayAdd(all, c)
	}
	darwin.Send(s.center, nsel.setCategories, darwin.SendClass(darwin.GetClass("NSSet"), nsel.setWithArray, all))
	return id
}

// --- the center's delegate ---

func notificationDelegateMethods() []darwin.Method {
	return []darwin.Method{
		{Sel: darwin.Sel("userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:"), Fn: noteResponse},
		{Sel: darwin.Sel("userNotificationCenter:willPresentNotification:withCompletionHandler:"), Fn: notePresent},
	}
}

func noteBackend(delegate darwin.ID) *Backend {
	deskObjects.Lock()
	defer deskObjects.Unlock()
	return deskObjects.notes[delegate]
}

// noteResponse reports the user's response to a notification of this run.
// It may run on any thread: it reads the response, calls the completion
// handler and hands the rest to the main thread.
func noteResponse(self darwin.ID, _ darwin.SEL, _, response, completion darwin.ID) {
	pool := darwin.PoolPush()
	action := darwin.GoString(darwin.Send(response, nsel.actionIdentifier))
	ident := darwin.GoString(darwin.Send(darwin.Send(darwin.Send(response, nsel.notification), nsel.request), nsel.identifier))
	darwin.PoolPop(pool)
	darwin.CallBlock(completion)
	b := noteBackend(self)
	if b == nil {
		return
	}
	b.onMain(func() {
		s := &b.desk.notes
		id, ok := noteID(s.prefix, ident)
		if !ok || !s.live[id] {
			return
		}
		delete(s.live, id)
		b.appEvent(noteEvent(id, action, darwin.UNNotificationDefaultActionIdentifier, darwin.UNNotificationDismissActionIdentifier))
	})
}

// notePresent shows a notification while the app is in front.
func notePresent(_ darwin.ID, _ darwin.SEL, _, _, completion darwin.ID) {
	darwin.CallBlock(completion, uintptr(darwin.UNNotificationPresentationOptionBanner|
		darwin.UNNotificationPresentationOptionList|darwin.UNNotificationPresentationOptionSound))
}
