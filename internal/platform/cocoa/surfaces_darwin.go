//go:build darwin

package cocoa

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The desktop surfaces of platform contract v16 (ADR-298) on macOS:
//
//   - Start at login: +[SMAppService mainAppService] (ServiceManagement,
//     macOS 13 and later), registerAndReturnError: and
//     unregisterAndReturnError:; status Enabled is on, and
//     RequiresApproval (the user has to allow it in System Settings ›
//     Login Items, which macOS announces with a notification) counts as
//     off until the user does. The service is the main bundle's, so a
//     program outside a bundle is ErrUnsupported (tela run makes one).
//     SMAppService is thread-safe: the calls run on the caller's thread
//     (locked to it, in an autorelease pool of their own). A start as a
//     login item is told in applicationDidFinishLaunching: by the 'oapp'
//     Apple event that launched the app, whose keyAEPropData is
//     keyAELaunchedAsLogInItem (markLoginLaunch).
//   - Tasks: the app delegate's applicationDockMenu: returns an NSMenu of
//     the tasks, built on the main thread from the tasks SetTasks kept
//     (any goroutine, under a lock); each item sends dockMenuItem: to the
//     backend's TelaMenuTarget with its index as the tag, and the choice
//     is an app-scoped LaunchEvent{Launch{Args: task.Args, Dir: the
//     working directory}, Source: LaunchForwarded}, as a start that
//     another process forwards is on Windows and Linux: one behavior for
//     the layers above. The Dock asks for the menu each time it opens it.
//   - Recent documents: -[NSDocumentController noteNewRecentDocumentURL:]
//     and clearRecentDocuments:, AppKit calls made on the main thread (at
//     once from it, else from a run loop block: the call returns before
//     AppKit has it). The Dock menu and File › Open Recent show the files
//     of the document types the bundle declares (CFBundleDocumentTypes);
//     AppKit keeps the list itself, per app.

var surfSel = struct {
	mainAppService, registerApp, unregisterApp, status                darwin.SEL
	sharedDocumentController, noteNewRecent, clearRecent              darwin.SEL
	sharedAppleEventManager, currentAppleEvent, eventID               darwin.SEL
	paramDescriptorForKeyword, enumCodeValue, setToolTip, setImageSel darwin.SEL
}{
	mainAppService:            darwin.Sel("mainAppService"),
	registerApp:               darwin.Sel("registerAndReturnError:"),
	unregisterApp:             darwin.Sel("unregisterAndReturnError:"),
	status:                    darwin.Sel("status"),
	sharedDocumentController:  darwin.Sel("sharedDocumentController"),
	noteNewRecent:             darwin.Sel("noteNewRecentDocumentURL:"),
	clearRecent:               darwin.Sel("clearRecentDocuments:"),
	sharedAppleEventManager:   darwin.Sel("sharedAppleEventManager"),
	currentAppleEvent:         darwin.Sel("currentAppleEvent"),
	eventID:                   darwin.Sel("eventID"),
	paramDescriptorForKeyword: darwin.Sel("paramDescriptorForKeyword:"),
	enumCodeValue:             darwin.Sel("enumCodeValue"),
	setToolTip:                darwin.Sel("setToolTip:"),
	setImageSel:               darwin.Sel("setImage:"),
}

// dockItemAction is the action of the Dock menu's items.
const dockItemAction = "dockMenuItem:"

// dockState is the Dock menu: the tasks (any goroutine, under the lock)
// and the menu last built (main thread).
type dockState struct {
	mu    sync.Mutex
	tasks []platform.Task
	menu  darwin.ID // the NSMenu applicationDockMenu: returned last (owned)
}

// loginService returns +[SMAppService mainAppService], or an error when
// the system or the process has none.
func loginService(what string) (darwin.ID, error) {
	if !darwin.LoadServiceManagement() {
		return 0, fmt.Errorf("cocoa: %s: SMAppService needs macOS 13 or later: %w", what, platform.ErrUnsupported)
	}
	if !mainBundleUsable() {
		return 0, fmt.Errorf("cocoa: %s: the app's login item is its bundle's, and this program is not in an app bundle (start it with `tela run`, or build it with `tela build`): %w", what, platform.ErrUnsupported)
	}
	svc := darwin.SendClass(darwin.GetClass("SMAppService"), surfSel.mainAppService)
	if svc == 0 {
		return 0, fmt.Errorf("cocoa: %s: no SMAppService for the main app: %w", what, platform.ErrUnsupported)
	}
	return svc, nil
}

// SetStartAtLogin registers or unregisters the app's login item.
func (d desktop) SetStartAtLogin(app platform.HandlerApp, on bool) error {
	platform.CheckHandlerApp("SetStartAtLogin", app, on)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	svc, err := loginService("SetStartAtLogin")
	if err != nil {
		return err
	}
	status := darwin.SendInt(svc, surfSel.status)
	switch {
	case on && (status == darwin.SMAppServiceStatusEnabled || status == darwin.SMAppServiceStatusRequiresApproval):
		return nil // registered already (approved or waiting for the user)
	case !on && status == darwin.SMAppServiceStatusNotRegistered:
		return nil
	}
	sel, what := surfSel.registerApp, "register"
	if !on {
		sel, what = surfSel.unregisterApp, "unregister"
	}
	errOut := new(darwin.ID) // the autoreleased NSError, on the heap
	if !darwin.SendBool(svc, sel, unsafe.Pointer(errOut)) {
		domain, code, desc := darwin.ErrorInfo(*errOut)
		return fmt.Errorf("cocoa: SetStartAtLogin: SMAppService %s failed: %s (%s %d)", what, desc, domain, code)
	}
	return nil
}

// StartsAtLogin reports whether the app's login item is enabled.
func (d desktop) StartsAtLogin(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("StartsAtLogin", app, false)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	svc, err := loginService("StartsAtLogin")
	if err != nil {
		return false, err
	}
	return darwin.SendInt(svc, surfSel.status) == darwin.SMAppServiceStatusEnabled, nil
}

// markLoginLaunch records a start as a login item: the Apple event being
// handled (applicationDidFinishLaunching: runs inside the 'oapp' event's
// handling) says so. Main thread.
func markLoginLaunch() {
	mgr := darwin.SendClass(darwin.GetClass("NSAppleEventManager"), surfSel.sharedAppleEventManager)
	if mgr == 0 {
		return
	}
	ev := darwin.Send(mgr, surfSel.currentAppleEvent)
	if ev == 0 || uint32(darwin.SendUint(ev, surfSel.eventID)) != darwin.KAEOpenApplication {
		return
	}
	desc := darwin.Send(ev, surfSel.paramDescriptorForKeyword, uint32(darwin.KeyAEPropData))
	if desc != 0 && uint32(darwin.SendUint(desc, surfSel.enumCodeValue)) == darwin.KeyAELaunchedAsLogInItem {
		platform.MarkStartedAtLogin()
	}
}

// SetTasks keeps the tasks for the Dock menu, which AppKit asks for when
// it opens it.
func (d desktop) SetTasks(app platform.HandlerApp, tasks []platform.Task) error {
	platform.CheckHandlerApp("SetTasks", app, true)
	platform.CheckTasks("SetTasks", tasks)
	dock := &d.b.desk.dock
	dock.mu.Lock()
	dock.tasks = platform.CloneTasks(tasks)
	dock.mu.Unlock()
	return nil
}

// applicationDockMenu is -[NSApplicationDelegate applicationDockMenu:]:
// the tasks' menu, or nil without tasks.
func applicationDockMenu(_ darwin.ID, _ darwin.SEL, _ darwin.ID) darwin.ID {
	b := runningBackend()
	if b == nil || b.closed || b.menus.target == 0 {
		return 0
	}
	var menu darwin.ID
	b.guard(func() { menu = b.dockMenu() })
	return menu
}

// dockMenu builds the Dock menu of the tasks; the backend keeps it until
// the next one. Main thread.
func (b *Backend) dockMenu() darwin.ID {
	dock := &b.desk.dock
	dock.mu.Lock()
	tasks := platform.CloneTasks(dock.tasks)
	dock.mu.Unlock()
	if dock.menu != 0 {
		b.releaseLater(dock.menu)
		dock.menu = 0
	}
	if len(tasks) == 0 {
		return 0
	}
	menuClass, itemClass := darwin.GetClass("NSMenu"), darwin.GetClass("NSMenuItem")
	menu := darwin.Send(darwin.SendClass(menuClass, sel.alloc), trsel.initWithTitle, darwin.AutoString(""))
	darwin.Send(menu, trsel.setAutoenablesItems, false)
	for i, t := range tasks {
		mi := darwin.Send(darwin.SendClass(itemClass, sel.alloc), trsel.initWithTitleAction,
			darwin.AutoString(t.Title), darwin.Sel(dockItemAction), darwin.AutoString(""))
		darwin.Send(mi, trsel.setTarget, b.menus.target)
		darwin.Send(mi, trsel.setTag, i)
		darwin.Send(mi, trsel.setEnabled, true)
		if t.Description != "" {
			darwin.Send(mi, surfSel.setToolTip, darwin.AutoString(t.Description))
		}
		if img := b.trayImage(t.Icon, 16); img != 0 {
			darwin.Send(mi, surfSel.setImageSel, img)
			darwin.Release(img)
		}
		darwin.Send(menu, trsel.addItem, mi)
		darwin.Release(mi)
	}
	dock.menu = menu
	return menu
}

// dockMenuItemChosen is the action of the Dock menu's items: the tag is
// the task's index in the menu that was shown.
func dockMenuItemChosen(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	b := menuBackend(self)
	if b == nil || b.closed {
		return
	}
	b.guard(func() {
		i := darwin.SendInt(sender, trsel.tag)
		dock := &b.desk.dock
		dock.mu.Lock()
		var args []string
		ok := i >= 0 && i < len(dock.tasks)
		if ok {
			args = append([]string(nil), dock.tasks[i].Args...)
		}
		dock.mu.Unlock()
		if !ok {
			return
		}
		l := platform.Launch{Args: args}
		l.Dir, _ = os.Getwd()
		b.appEvent(platform.LaunchEvent{Launch: l, Source: platform.LaunchForwarded})
	})
}

// stopDock drops the Dock menu when Run returns.
func (b *Backend) stopDock() {
	if m := b.desk.dock.menu; m != 0 {
		b.releaseLater(m)
		b.desk.dock.menu = 0
	}
}

// AddRecentDocument notes the file with the shared document controller.
func (d desktop) AddRecentDocument(app platform.HandlerApp, path string) error {
	platform.CheckHandlerApp("AddRecentDocument", app, true)
	clean, fi, err := platform.CheckRecent(path)
	if err != nil {
		return err
	}
	d.b.onMainNow(func() {
		ctl := darwin.SendClass(darwin.GetClass("NSDocumentController"), surfSel.sharedDocumentController)
		if ctl != 0 {
			darwin.Send(ctl, surfSel.noteNewRecent, darwin.FileURL(clean, fi.IsDir()))
		}
	})
	return nil
}

// ClearRecentDocuments clears the shared document controller's list.
func (d desktop) ClearRecentDocuments(app platform.HandlerApp) error {
	platform.CheckHandlerApp("ClearRecentDocuments", app, false)
	d.b.onMainNow(func() {
		ctl := darwin.SendClass(darwin.GetClass("NSDocumentController"), surfSel.sharedDocumentController)
		if ctl != 0 {
			darwin.Send(ctl, surfSel.clearRecent, darwin.ID(0))
		}
	})
	return nil
}

// onMainNow runs fn on the main thread: at once when the caller is on
// it, else from a run loop block (see onMain). It runs in an autorelease
// pool of its own.
func (b *Backend) onMainNow(fn func()) {
	run := func() {
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		fn()
	}
	if darwin.IsMainThread() {
		run()
		return
	}
	b.onMain(run)
}
