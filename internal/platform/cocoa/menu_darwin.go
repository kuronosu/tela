//go:build darwin

package cocoa

import (
	"errors"
	"fmt"
	"sync"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// Native menus: the app's menu bar (NSApp.mainMenu, Desktop.SetAppMenu),
// popup menus (Window.PopupMenu) and the standard edit actions of
// TelaView. menu.go plans the menus; this file makes the NSMenus. The quit
// request is in quit_darwin.go.
//
//   - The bar is installed when the backend is created: the default one
//     (the app menu, Edit, Window; defaultMenuBarPlan) until SetAppMenu
//     gives the app's, and again after SetAppMenu(nil, nil) and when Run
//     returns (no item then targets a backend that is gone).
//   - App items send appMenuItem: to the backend's TelaMenuTarget with
//     their ID as the tag; the choice is an app-scoped AppMenuEvent. Role
//     items send AppKit's own action to the first responder (a nil
//     target): TelaView for the edit roles, the key window for the window
//     roles, NSApp for About, Hide and the others. They are not reported.
//   - Every menu enables its items itself (autoenablesItems, AppKit's
//     default): an item is enabled when the first object of the responder
//     chain that implements its action allows it (validateMenuItem:), so
//     the edit items are enabled while a TelaView is key and grayed out
//     otherwise, and the window items follow the key window. A disabled
//     item has no action, which grays it out; a disabled submenu grays out
//     its items (AppKit enables an item that has a submenu).
//   - An accelerator is the item's key equivalent (menu.go): the main menu
//     sees key downs first (TelaView's performKeyEquivalent: asks it before
//     delivering a Command or Control key, and AppKit asks it before
//     keyDown: for the others), so a key that chooses an item is not
//     delivered as a KeyEvent; its release is dropped by the held-key set,
//     which saw no press. When -[NSMenu performKeyEquivalent:] takes no
//     item, the backend matches the bar's key equivalents itself
//     (performKeyEquivalent) before delivering the key, since AppKit's
//     matching missed an item of the app's bar on macos-latest. A key no
//     item has is delivered as before. An
//     accelerator that does not parse, or whose key has no key equivalent
//     (media keys), is not shown and the item is still made: NSMenuItem
//     can show only a key equivalent.
//   - The edit roles: TelaView implements undo:, redo:, cut:, copy:,
//     paste: and selectAll: by delivering the role's standard key press
//     of the Mac flavor (Cmd+Z, Shift+Cmd+Z, Cmd+X, Cmd+C, Cmd+V, Cmd+A)
//     to its window as KeyEvents, the events the real key gives, so text
//     fields act on a menu click as on the key and need nothing else. When
//     the key equivalent itself chose the item, the key press is delivered
//     as it is (the real event, once; its release follows as usual);
//     otherwise (a click, or an item given another accelerator) a press
//     and a release of the standard key are.
//   - Popup menus: an NSMenu of the items (every item answered with its
//     ID, roles included) shown with
//     popUpMenuPositioningItem:atLocation:inView: on TelaView, whose
//     tracking loop runs until the user chooses or dismisses. It is
//     started from a run loop block outside any delivery pass (onMain),
//     one at a time in request order; the
//     answer goes through the loop. Quit cancels the tracking.

// msel holds the selectors of the menu code.
var msel = struct {
	setMainMenu, mainMenu, setServicesMenu, setWindowsMenu, setHelpMenu darwin.SEL
	setKeyEquivalentModifierMask, popUp, cancelTracking                 darwin.SEL
	mainBundle, objectForInfoDictionaryKey, processName, processInfo    darwin.SEL
	isKindOfClass, servicesMenu, supermenu, indexOfItemWithSubmenu      darwin.SEL
	numberOfItems, itemAtIndex, submenu, keyEquivalent, modifierMask    darwin.SEL
	isEnabled, isHidden, update, performActionForItemAtIndex            darwin.SEL
}{
	setMainMenu:                  darwin.Sel("setMainMenu:"),
	mainMenu:                     darwin.Sel("mainMenu"),
	setServicesMenu:              darwin.Sel("setServicesMenu:"),
	setWindowsMenu:               darwin.Sel("setWindowsMenu:"),
	setHelpMenu:                  darwin.Sel("setHelpMenu:"),
	setKeyEquivalentModifierMask: darwin.Sel("setKeyEquivalentModifierMask:"),
	popUp:                        darwin.Sel("popUpMenuPositioningItem:atLocation:inView:"),
	cancelTracking:               darwin.Sel("cancelTracking"),
	mainBundle:                   darwin.Sel("mainBundle"),
	objectForInfoDictionaryKey:   darwin.Sel("objectForInfoDictionaryKey:"),
	processName:                  darwin.Sel("processName"),
	processInfo:                  darwin.Sel("processInfo"),
	isKindOfClass:                darwin.Sel("isKindOfClass:"),
	servicesMenu:                 darwin.Sel("servicesMenu"),
	supermenu:                    darwin.Sel("supermenu"),
	indexOfItemWithSubmenu:       darwin.Sel("indexOfItemWithSubmenu:"),
	numberOfItems:                darwin.Sel("numberOfItems"),
	itemAtIndex:                  darwin.Sel("itemAtIndex:"),
	submenu:                      darwin.Sel("submenu"),
	keyEquivalent:                darwin.Sel("keyEquivalent"),
	modifierMask:                 darwin.Sel("keyEquivalentModifierMask"),
	isEnabled:                    darwin.Sel("isEnabled"),
	isHidden:                     darwin.Sel("isHidden"),
	update:                       darwin.Sel("update"),
	performActionForItemAtIndex:  darwin.Sel("performActionForItemAtIndex:"),
}

// menuState is the backend's state of the native menus and the quit
// request, on the main thread.
type menuState struct {
	target darwin.ID // TelaMenuTarget (owned): the target of the app items
	// menu and appItems are the app's bar as SetAppMenu gave it (copies);
	// both nil while the default bar is shown.
	menu, appItems []platform.MenuItem

	popups      []*popupRequest // waiting to be shown, in request order
	popup       *popupRequest   // shown (its tracking loop runs) or answering
	popupPosted bool            // a block will show the next one

	quit quitState // quit_darwin.go
}

// menuClasses holds the class of the menu targets, registered once per
// process.
var menuClasses struct {
	once   sync.Once
	err    error
	target darwin.Class
}

// menuTargets maps the TelaMenuTargets to their backends.
var menuTargets = struct {
	sync.Mutex
	m map[darwin.ID]*Backend
}{m: map[darwin.ID]*Backend{}}

// setupMenus registers TelaMenuTarget, once.
func setupMenus() error {
	c := &menuClasses
	c.once.Do(func() {
		c.target, c.err = darwin.RegisterClass("TelaMenuTarget", darwin.GetClass("NSObject"), nil, []darwin.Method{
			{Sel: darwin.Sel(appItemAction), Fn: appMenuItemChosen},
			{Sel: darwin.Sel(popupItemAction), Fn: popupMenuItemChosen},
		})
		if c.err != nil {
			c.err = fmt.Errorf("cocoa: registering TelaMenuTarget: %w", c.err)
		}
	})
	return c.err
}

// startMenus gives the backend its menu target and installs the default
// bar, when the backend is created.
func (b *Backend) startMenus() {
	if setupMenus() != nil {
		return // no app items then: SetAppMenu reports it
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	b.menus.target = darwin.Send(darwin.SendClass(menuClasses.target, sel.alloc), sel.init)
	menuTargets.Lock()
	menuTargets.m[b.menus.target] = b
	menuTargets.Unlock()
	b.installBar(defaultMenuBarPlan(appName()))
}

// stopMenus puts the default bar back and drops the popups that wait, when
// Run returns (before the windows close). An open popup was cancelled by
// Quit already.
func (b *Backend) stopMenus() {
	m := &b.menus
	m.popups = nil
	if m.popup != nil && m.popup.menu != 0 {
		darwin.Send(m.popup.menu, msel.cancelTracking)
	}
	if m.target == 0 {
		return
	}
	m.menu, m.appItems = nil, nil
	b.installBar(defaultMenuBarPlan(appName()))
	menuTargets.Lock()
	delete(menuTargets.m, m.target)
	menuTargets.Unlock()
	b.releaseLater(m.target)
	m.target = 0
}

// menusAfterPass runs after the deliveries of each pass: Quit cancels an
// open popup (its tracking loop would hold Run), and a quit request that
// waits for the app's decision is woken to look again (quit_darwin.go).
func (b *Backend) menusAfterPass() {
	m := &b.menus
	if b.loop.Quitting() && m.popup != nil && m.popup.menu != 0 {
		darwin.Send(m.popup.menu, msel.cancelTracking)
	}
	b.wakeQuitWait()
}

// SetAppMenu sets NSApp.mainMenu to the app's bar (appMenuBarPlan): the
// app menu with appItems, then menu's items. nil (or empty) menu and
// appItems put the default bar back.
func (d desktop) SetAppMenu(menu, appItems []platform.MenuItem) error {
	platform.CheckAppMenu(menu, appItems)
	b := d.b
	if b.closed {
		return errClosed
	}
	if err := setupMenus(); err != nil {
		return errNoHelpers("SetAppMenu", err)
	}
	if b.menus.target == 0 {
		return fmt.Errorf("cocoa: SetAppMenu: the backend has no menu target: %w", platform.ErrUnsupported)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	m := &b.menus
	if len(menu) == 0 && len(appItems) == 0 {
		m.menu, m.appItems = nil, nil
		b.installBar(defaultMenuBarPlan(appName()))
		return nil
	}
	m.menu, m.appItems = cloneMenuItems(menu), cloneMenuItems(appItems)
	b.installBar(appMenuBarPlan(appName(), m.menu, m.appItems))
	return nil
}

// barMenus are the submenus of a bar that NSApp is told about.
type barMenus struct{ services, windows, help darwin.ID }

// installBar makes the NSMenus of plan and installs them as the main
// menu. The bar it replaces is released at the next pass: SetAppMenu may
// run inside the action of one of its items.
func (b *Backend) installBar(plan []menuEntry) {
	var specials barMenus
	main := b.buildMenu(plan, "", &specials)
	old := darwin.Retain(darwin.Send(app.ns, msel.mainMenu))
	darwin.Send(app.ns, msel.setMainMenu, main)
	darwin.Send(app.ns, msel.setServicesMenu, specials.services)
	darwin.Send(app.ns, msel.setWindowsMenu, specials.windows)
	darwin.Send(app.ns, msel.setHelpMenu, specials.help)
	darwin.Release(main) // NSApp keeps it
	if old != 0 {
		b.releaseLater(old)
	}
}

// buildMenu returns an NSMenu (+1) titled title holding entries; the
// special submenus are recorded in specials (nil for a popup).
func (b *Backend) buildMenu(entries []menuEntry, title string, specials *barMenus) darwin.ID {
	menuClass, itemClass := darwin.GetClass("NSMenu"), darwin.GetClass("NSMenuItem")
	menu := darwin.Send(darwin.SendClass(menuClass, sel.alloc), trsel.initWithTitle, darwin.AutoString(title))
	for _, e := range entries {
		if e.Separator {
			darwin.Send(menu, trsel.addItem, darwin.SendClass(itemClass, trsel.separatorItem))
			continue
		}
		var act darwin.SEL
		if e.Action != "" {
			act = darwin.Sel(e.Action)
		}
		mi := darwin.Send(darwin.SendClass(itemClass, sel.alloc), trsel.initWithTitleAction,
			darwin.AutoString(e.Title), act, darwin.AutoString(e.Key))
		if e.Key != "" {
			// The default mask is Command: a key equivalent without
			// modifiers (F5) needs 0.
			darwin.Send(mi, msel.setKeyEquivalentModifierMask, uint(e.Mods))
		}
		if e.App {
			darwin.Send(mi, trsel.setTarget, b.menus.target)
			darwin.Send(mi, trsel.setTag, int(e.Tag))
		}
		if e.Checkable {
			state := darwin.NSControlStateValueOff
			if e.Checked {
				state = darwin.NSControlStateValueOn
			}
			darwin.Send(mi, trsel.setState, state)
		}
		if e.HasSubmenu {
			var sub darwin.ID
			if e.Special == specialServices && specials != nil {
				sub = adoptServicesMenu()
			}
			if sub == 0 {
				sub = b.buildMenu(e.Submenu, e.Title, specials)
			}
			darwin.Send(mi, trsel.setSubmenu, sub)
			if specials != nil {
				switch e.Special {
				case specialServices:
					specials.services = sub
				case specialWindows:
					specials.windows = sub
				case specialHelp:
					specials.help = sub
				}
			}
			darwin.Release(sub) // the item keeps it
		}
		darwin.Send(menu, trsel.addItem, mi)
		darwin.Release(mi)
	}
	return menu
}

// adoptServicesMenu returns NSApp's Services menu (+1), taken off the
// item of the bar it is in, or 0 when NSApp has none. AppKit keeps one
// Services menu object, the one it fills: on macos-latest
// -[NSApplication servicesMenu] was the same object before and after
// setServicesMenu: was given a new one, so a new menu would stay empty.
// A menu can be the submenu of one item only, so it moves to the new bar.
func adoptServicesMenu() darwin.ID {
	s := darwin.Retain(darwin.Send(app.ns, msel.servicesMenu))
	if s == 0 {
		return 0
	}
	if sup := darwin.Send(s, msel.supermenu); sup != 0 {
		if i := darwin.SendInt(sup, msel.indexOfItemWithSubmenu, s); i >= 0 {
			darwin.Send(darwin.Send(sup, msel.itemAtIndex, i), trsel.setSubmenu, darwin.ID(0))
		}
	}
	return s
}

// appName returns the name of the app menu's items: the bundle's
// CFBundleName, else the process name (an unbundled program: its file
// name).
func appName() string {
	if bundle := darwin.SendClass(darwin.GetClass("NSBundle"), msel.mainBundle); bundle != 0 {
		v := darwin.Send(bundle, msel.objectForInfoDictionaryKey, darwin.AutoString("CFBundleName"))
		if v != 0 && darwin.SendBool(v, msel.isKindOfClass, darwin.ID(darwin.GetClass("NSString"))) {
			if s := darwin.GoString(v); s != "" {
				return s
			}
		}
	}
	info := darwin.SendClass(darwin.GetClass("NSProcessInfo"), msel.processInfo)
	if s := darwin.GoString(darwin.Send(info, msel.processName)); s != "" {
		return s
	}
	return "Tela"
}

// menuBackend returns the backend of a TelaMenuTarget.
func menuBackend(target darwin.ID) *Backend {
	menuTargets.Lock()
	defer menuTargets.Unlock()
	return menuTargets.m[target]
}

// appMenuItemChosen is the action of the app items: the tag is the item's
// ID. Only enabled items of the current bar are reported.
func appMenuItemChosen(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	b := menuBackend(self)
	if b == nil || b.closed {
		return
	}
	b.guard(func() {
		id := platform.MenuItemID(darwin.SendInt(sender, trsel.tag))
		it, ok := platform.FindMenuItem(b.menus.menu, id)
		if !ok {
			it, ok = platform.FindMenuItem(b.menus.appItems, id)
		}
		if ok && !it.Disabled && it.Role == platform.RoleNone {
			b.appEvent(platform.AppMenuEvent{Item: id})
		}
	})
}

// --- TelaView's edit actions ---

// menuViewMethods are the methods of TelaView for the edit roles: the
// standard actions and their validation.
func menuViewMethods() []darwin.Method {
	var ms []darwin.Method
	for r := platform.RoleUndo; r <= platform.RoleSelectAll; r++ {
		role := r
		ms = append(ms, darwin.Method{Sel: darwin.Sel(roleActions[role]), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(func() {
					w.editAction(role)
					w.b.pump()
				})
			}
		}})
	}
	// TelaView implements only the edit actions: AppKit asks it about
	// their items while it is in the responder chain (the first
	// responder of the key window).
	ms = append(ms, darwin.Method{Sel: darwin.Sel("validateMenuItem:"), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) bool {
		w := windowFor(self)
		return w != nil && !w.Closed()
	}})
	return ms
}

// menuKeyEvent is the key down the main menu is asked about (mainMenuKey),
// or 0: an edit action it sends delivers that key press itself. Main
// thread only.
var menuKeyEvent darwin.ID

// mainMenuKey offers the key down ev to the main menu
// (performKeyEquivalent:) and reports whether an item took it.
func mainMenuKey(ev darwin.ID) bool {
	menu := darwin.Send(app.ns, isel.mainMenu)
	if menu == 0 {
		return false
	}
	prev := menuKeyEvent // a quit request's wait may nest key events
	menuKeyEvent = ev
	defer func() { menuKeyEvent = prev }()
	if darwin.SendBool(menu, isel.performKeyEquivalent, ev) {
		menuKeyStats.appKit++
		return true
	}
	if performKeyEquivalent(menu, darwin.GoString(darwin.Send(ev, isel.charactersIgnoring)), darwin.SendUint(ev, isel.modifierFlags)) {
		menuKeyStats.own++
		return true
	}
	return false
}

// menuKeyStats counts the key equivalents AppKit's
// -[NSMenu performKeyEquivalent:] matched and the ones only
// performKeyEquivalent did (tests).
var menuKeyStats struct{ appKit, own int }

// performKeyEquivalent chooses the enabled, visible item of menu
// (submenus included) whose key equivalent is chars with the modifiers of
// flags (keyEquivalentMatches), as -[NSMenu performKeyEquivalent:] does,
// and reports whether there was one. It is asked after AppKit's own
// matching, which on macos-latest did not choose the item of a Command
// key of the app's bar for a key event made from a CGEvent (an event
// without a window, as the tests make them; the cause is not known):
// menuKeyStats tells the tests which of the two chose.
func performKeyEquivalent(menu darwin.ID, chars string, flags uint64) bool {
	darwin.Send(menu, msel.update) // automatic enabling
	n := darwin.SendInt(menu, msel.numberOfItems)
	for i := 0; i < n; i++ {
		it := darwin.Send(menu, msel.itemAtIndex, i)
		if darwin.SendBool(it, msel.isHidden) || !darwin.SendBool(it, msel.isEnabled) {
			continue
		}
		if sub := darwin.Send(it, msel.submenu); sub != 0 {
			if performKeyEquivalent(sub, chars, flags) {
				return true
			}
			continue
		}
		key := darwin.GoString(darwin.Send(it, msel.keyEquivalent))
		if keyEquivalentMatches(key, darwin.SendUint(it, msel.modifierMask), chars, flags) {
			darwin.Send(menu, msel.performActionForItemAtIndex, i)
			return true
		}
	}
	return false
}

// editAction performs the edit role r in the window: the key press that
// chose the item when it is the role's standard key, else a press and a
// release of that key (a click, another accelerator, a sendAction:).
func (w *Window) editAction(r platform.MenuRole) {
	if ev := menuKeyEvent; ev != 0 {
		if e, ok := keyboard.MacKeyDown(w.macKey(ev)); ok && isStandardKey(r, e) {
			w.key(ev, true)
			return
		}
	}
	std := editKeys[r]
	std.Time = w.b.Now()
	if down, ok := keyboard.MacKeyDown(std); ok {
		w.deliverKey(down)
	}
	if up, ok := keyboard.MacKeyUp(std); ok {
		w.deliverKey(up)
	}
}

// macKey reads a key event as the keyboard package takes it.
func (w *Window) macKey(ev darwin.ID) keyboard.MacKey {
	return keyboard.MacKey{
		KeyCode:                     uint16(darwin.SendUint(ev, isel.keyCode)),
		Flags:                       darwin.SendUint(ev, isel.modifierFlags),
		Characters:                  darwin.GoString(darwin.Send(ev, isel.characters)),
		CharactersIgnoringModifiers: darwin.GoString(darwin.Send(ev, isel.charactersIgnoring)),
		ISO:                         darwin.KeyboardISO(),
		Time:                        w.b.eventTime(ev),
	}
}

// --- window menus ---

// SetMenu validates the menu and returns ErrUnsupported: a macOS window
// has no menu bar of its own (the app's is NSApp.mainMenu,
// Desktop.SetAppMenu), so the layers above draw it (ADR-224).
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// popupRequest is a popup menu waiting, shown or answering.
type popupRequest struct {
	w      *Window
	at     platform.Point
	items  []platform.MenuItem
	done   func(platform.MenuItemID, error)
	menu   darwin.ID // the NSMenu while its tracking loop runs
	chosen platform.MenuItemID
}

// errPopupClosed is returned by PopupMenu on a closed window.
var errPopupClosed = errors.New("cocoa: PopupMenu on a closed window")

// PopupMenu shows a context menu with its top-left corner at at (points,
// the view's coordinates), from a run loop block outside the pass.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	b := w.b
	if b.closed {
		return errClosed
	}
	if w.Closed() {
		return errPopupClosed
	}
	if err := setupMenus(); err != nil || b.menus.target == 0 {
		return errNoHelpers("PopupMenu", errors.Join(err, errors.New("no menu target")))
	}
	b.menus.popups = append(b.menus.popups, &popupRequest{w: w, at: at, items: cloneMenuItems(menu), done: done})
	b.nextPopup()
	return nil
}

// nextPopup shows the next waiting popup from a run loop block, once the
// one shown has answered.
func (b *Backend) nextPopup() {
	m := &b.menus
	if m.popup != nil || m.popupPosted || len(m.popups) == 0 {
		return
	}
	m.popupPosted = true
	b.onMain(func() {
		m.popupPosted = false
		if m.popup != nil || len(m.popups) == 0 {
			return
		}
		if b.pumping || !b.running {
			b.nextPopup() // not from inside a pass, nor before [NSApp run]
			return
		}
		r := m.popups[0]
		m.popups = m.popups[1:]
		m.popup = r
		b.showPopup(r)
	})
}

// showPopup runs the popup's tracking loop and answers it.
func (b *Backend) showPopup(r *popupRequest) {
	m := &b.menus
	finish := func() {
		m.popup = nil
		id := r.chosen
		b.answer(func() { r.done(id, nil) })
		b.nextPopup()
	}
	if r.w.Closed() || b.loop.Quitting() {
		finish()
		return
	}
	r.menu = b.buildMenu(popupPlan(r.items), "", nil)
	picked := darwin.SendBool(r.menu, msel.popUp, darwin.ID(0),
		darwin.CGPoint{X: float64(r.at.X), Y: float64(r.at.Y)}, r.w.view)
	b.releaseLater(r.menu)
	r.menu = 0
	if picked && r.chosen == 0 {
		// The item's action may come after the tracking loop returned:
		// answer one run loop turn later.
		b.onMain(finish)
		return
	}
	finish()
}

// popupMenuItemChosen is the action of a popup's items: the tag is the
// item's ID.
func popupMenuItemChosen(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	b := menuBackend(self)
	if b == nil || b.closed {
		return
	}
	b.guard(func() {
		r := b.menus.popup
		if r == nil {
			return
		}
		id := platform.MenuItemID(darwin.SendInt(sender, trsel.tag))
		if it, ok := platform.FindMenuItem(r.items, id); ok && !it.Disabled {
			r.chosen = id
		}
	})
}
