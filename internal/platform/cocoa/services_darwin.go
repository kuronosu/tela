//go:build darwin

package cocoa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The desktop services (macOS desktop integration, ADR-224): NSWorkspace
// for OpenURI and OpenPath (this file), NSOpenPanel, NSSavePanel and
// NSAlert (dialog_darwin.go), NSStatusItem (tray_darwin.go), the keychain
// (secret_darwin.go), NSFileManager's trash (trash_darwin.go) and
// UNUserNotificationCenter (notify_darwin.go), Carbon hot keys
// (shortcut_darwin.go), single instance, the app ID and Launch Services'
// launches (launch_darwin.go), the associations (package assoc, on the
// app's bundle) and the app's menu bar (SetAppMenu, menu_darwin.go). The
// clipboard is in clipboard_darwin.go and the system settings in
// settings_darwin.go.
//
// Everything runs on the main thread except the secrets and the trash,
// which are safe from any goroutine. Answers reach the app through the
// shared loop (loop.Post, then a signal of the run loop source). The
// system calls some completion handlers on threads of its own (the
// notification center's): those hop to the main thread with
// CFRunLoopPerformBlock in the common modes (onMain), and so does what
// must start outside a delivery pass (a modal alert).

// Desktop returns the desktop services.
func (b *Backend) Desktop() platform.Desktop { return desktop{b: b} }

// desktop implements platform.Desktop for a Backend. The embedded
// UnsupportedDesktop checks the arguments of, and refuses, what is not
// implemented yet.
type desktop struct {
	platform.UnsupportedDesktop
	b *Backend
}

var _ platform.Desktop = desktop{}

// desktopState is the backend's state of the desktop services, on the
// main thread.
type desktopState struct {
	dialogs      []*dialogRequest // open panels and alerts, in request order
	modal        []func()         // detached alerts waiting for runModal, in request order
	modalPosted  bool             // a block will start the next one
	modalRunning bool             // runModal is running

	trays    map[platform.TrayID]*trayIcon
	lastTray platform.TrayID

	notes notifyState // notify_darwin.go

	keys   shortcutState // shortcut_darwin.go
	launch launchState   // launch_darwin.go: the app ID, single instance, launches
}

// dsel holds the selectors of the desktop services.
var dsel = struct {
	sharedWorkspace, openURL, urlWithString, scheme darwin.SEL
}{
	sharedWorkspace: darwin.Sel("sharedWorkspace"),
	openURL:         darwin.Sel("openURL:"),
	urlWithString:   darwin.Sel("URLWithString:"),
	scheme:          darwin.Sel("scheme"),
}

// OpenURI opens uri with -[NSWorkspace openURL:], which hands it to the
// default application of its scheme (Launch Services). parent is not
// needed: the opened application activates itself.
func (d desktop) OpenURI(_ platform.Window, uri string) error {
	if err := platform.CheckURI(uri); err != nil {
		return err
	}
	if d.b.closed {
		return errClosed
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	u := darwin.SendClass(darwin.GetClass("NSURL"), dsel.urlWithString, darwin.AutoString(uri))
	if u == 0 || darwin.GoString(darwin.Send(u, dsel.scheme)) == "" {
		return fmt.Errorf("cocoa: OpenURI(%q): NSURL does not accept it (percent-encode spaces and other characters outside RFC 3986)", uri)
	}
	if !darwin.SendBool(workspace(), dsel.openURL, u) {
		return fmt.Errorf("cocoa: OpenURI(%q): no application opens %s: URIs", uri, darwin.GoString(darwin.Send(u, dsel.scheme)))
	}
	return nil
}

// OpenPath opens a file in its default application, or a folder in the
// Finder, with -[NSWorkspace openURL:] of its file URL. A missing path is
// an error wrapping fs.ErrNotExist.
func (d desktop) OpenPath(_ platform.Window, path string) error {
	if path == "" || strings.ContainsRune(path, 0) {
		return fmt.Errorf("cocoa: OpenPath(%q): not a path", path)
	}
	if d.b.closed {
		return errClosed
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cocoa: OpenPath(%q): %w", path, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("cocoa: OpenPath: %w", err)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	if !darwin.SendBool(workspace(), dsel.openURL, darwin.FileURL(abs, fi.IsDir())) {
		return fmt.Errorf("cocoa: OpenPath(%q): no application opens it", abs)
	}
	return nil
}

// Associations returns the registration of URL schemes and file types:
// package assoc, which edits the app bundle's Info.plist and registers it
// with Launch Services (ADR-228). Safe from any goroutine, like every
// Associations.
func (desktop) Associations() platform.Associations { return assoc.Native() }

func workspace() darwin.ID {
	return darwin.SendClass(darwin.GetClass("NSWorkspace"), dsel.sharedWorkspace)
}

// onMain runs fn on the main thread at the next iteration of its run loop,
// in any of the common modes (so also inside a modal panel's or a
// tracking loop), outside the run loop source's callback and so outside
// any delivery pass, with an autorelease pool and the handler's panic
// guard. Safe from any goroutine. After Run returned it does nothing.
func (b *Backend) onMain(fn func()) {
	darwin.RunLoopPerformBlock(b.runLoop, darwin.CFRunLoopCommonModes, func() {
		if b.closed {
			return
		}
		b.guard(fn)
	})
}

// answer runs fn, the delivery of an asynchronous answer, on the UI
// thread from a later delivery pass (never from inside the request).
// After Run returned it is dropped: the contract calls done only while
// Run runs.
func (b *Backend) answer(fn func()) {
	if b.closed {
		return
	}
	b.loop.Post(fn)
	b.notify()
}

// appEvent queues an app-scoped event and runs a pass for it (one is
// started after the running pass when called from inside one).
func (b *Backend) appEvent(e platform.Event) {
	if b.closed {
		return
	}
	b.enqueue(nil, e)
	b.pump()
	b.notify()
}

// stopDesktop ends the services when Run returns, before the windows
// close: dialogs are ended without an answer (done is not called after
// Run), trays are removed, and the notification delegate is detached.
func (b *Backend) stopDesktop() {
	b.desk.modal = nil
	for _, r := range append([]*dialogRequest(nil), b.desk.dialogs...) {
		r.abandon()
	}
	b.desk.dialogs = nil
	for _, t := range b.desk.trays {
		t.remove()
	}
	b.desk.trays = nil
	b.stopNotifications()
	b.stopShortcuts()
	b.stopInstances()
}

// --- the classes of the services ---

// deskObjects maps the Objective-C helpers of the services (panel
// delegates, tray targets, the notification delegate) to their Go state.
// Go pointers are never handed to C.
var deskObjects = struct {
	sync.Mutex
	panels map[darwin.ID]*dialogRequest
	trays  map[darwin.ID]*trayIcon
	notes  map[darwin.ID]*Backend
}{panels: map[darwin.ID]*dialogRequest{}, trays: map[darwin.ID]*trayIcon{}, notes: map[darwin.ID]*Backend{}}

// deskClasses holds the helper classes, registered once per process.
var deskClasses struct {
	once                      sync.Once
	err                       error
	panelHelper, trayTarget   darwin.Class
	notificationDelegate      darwin.Class
	selFilterChanged, selTray darwin.SEL
	selTrayItem               darwin.SEL
	selParentWillClose        darwin.SEL
}

// setupDesktop registers the helper classes, once.
func setupDesktop() error {
	c := &deskClasses
	c.once.Do(func() {
		nsObject := darwin.GetClass("NSObject")
		c.selFilterChanged = darwin.Sel("filterChanged:")
		c.selTray = darwin.Sel("trayClicked:")
		c.selTrayItem = darwin.Sel("trayItem:")
		c.selParentWillClose = darwin.Sel("parentWillClose:")
		var err error
		// The panel helper is the target of the format popup and the
		// panel's delegate (NSOpenSavePanelDelegate, not declared: AppKit
		// asks respondsToSelector:).
		c.panelHelper, err = darwin.RegisterClass("TelaPanelHelper", nsObject, nil, []darwin.Method{
			{Sel: c.selFilterChanged, Fn: panelFilterChanged},
			{Sel: darwin.Sel("panel:shouldEnableURL:"), Fn: panelShouldEnableURL},
			{Sel: c.selParentWillClose, Fn: panelParentWillClose},
		})
		if err != nil {
			c.err = fmt.Errorf("cocoa: registering TelaPanelHelper: %w", err)
			return
		}
		c.trayTarget, err = darwin.RegisterClass("TelaTrayTarget", nsObject, nil, []darwin.Method{
			{Sel: c.selTray, Fn: trayClicked},
			{Sel: c.selTrayItem, Fn: trayItemChosen},
		})
		if err != nil {
			c.err = fmt.Errorf("cocoa: registering TelaTrayTarget: %w", err)
			return
		}
		c.notificationDelegate, err = darwin.RegisterClass("TelaNotificationDelegate", nsObject, nil, notificationDelegateMethods())
		if err != nil {
			c.err = fmt.Errorf("cocoa: registering TelaNotificationDelegate: %w", err)
		}
	})
	return c.err
}

// errNoHelpers wraps a failed class registration.
func errNoHelpers(what string, err error) error {
	return fmt.Errorf("cocoa: %s: %w (%w)", what, platform.ErrUnsupported, err)
}
