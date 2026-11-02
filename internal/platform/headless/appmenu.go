package headless

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
)

// Platform contract v4 (ADR-225): the app's menu bar and quit requests.

// appMenuState is the app's menu bar, guarded by b.mu.
type appMenuState struct {
	menu, items []platform.MenuItem
	changes     int
}

// SetAppMenu records the app's menu bar and app items (AppMenu,
// AppMenuChanges); ServiceAppMenu unsupported returns
// platform.ErrUnsupported (every backend but macOS's).
func (d desktop) SetAppMenu(menu, appItems []platform.MenuItem) error {
	platform.CheckAppMenu(menu, appItems)
	if !d.supported(ServiceAppMenu) {
		return platform.ErrUnsupported
	}
	menu, appItems = cloneMenu(menu), cloneMenu(appItems)
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.appMenu = appMenuState{menu: menu, items: appItems, changes: b.desktop.appMenu.changes + 1}
	return nil
}

// AppMenu returns a copy of the app's menu bar and app items as the
// latest SetAppMenu gave them (nil, nil before any).
func (b *Backend) AppMenu() (menu, appItems []platform.MenuItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneMenu(b.desktop.appMenu.menu), cloneMenu(b.desktop.appMenu.items)
}

// AppMenuChanges returns how many SetAppMenu calls the backend accepted.
func (b *Backend) AppMenuChanges() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.desktop.appMenu.changes
}

// ChooseAppMenu queues the AppMenuEvent of the user choosing item id of
// the app's menu bar or app items; see InjectApp. Safe from any
// goroutine.
func (b *Backend) ChooseAppMenu(id platform.MenuItemID) {
	b.InjectApp(platform.AppMenuEvent{Item: id})
}

// RequestQuit queues a QuitRequestedEvent: the user or the OS asks the
// app to quit (macOS's Cmd+Q, the Dock's Quit, a logout). The backend
// keeps running: quitting is the app's answer (Quit). Safe from any
// goroutine.
func (b *Backend) RequestQuit() { b.InjectApp(platform.QuitRequestedEvent{}) }

// checkAppMenuEvent returns why e could not come from a real backend, or
// "": the item must be an enabled item without a submenu of the app's
// menu bar or app items. b.mu must be held.
func (b *Backend) checkAppMenuEvent(e platform.AppMenuEvent) string {
	st := &b.desktop.appMenu
	it, ok := platform.FindMenuItem(st.menu, e.Item)
	if !ok {
		it, ok = platform.FindMenuItem(st.items, e.Item)
	}
	switch {
	case !ok:
		return fmt.Sprintf("the app's menu bar has no item %d (SetAppMenu first)", e.Item)
	case it.Disabled:
		return fmt.Sprintf("menu item %d (%q) is disabled", e.Item, it.Label)
	}
	return ""
}
