package headless

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
)

// SetMenu records the window's menu bar (Menu, MenuChanges). Like Win32,
// frameless windows (and TitleBarOverlay ones, Frameless there) have
// none: they return platform.ErrUnsupported, as every window does with
// ServiceMenus unsupported (a Linux backend).
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	if !w.b.supports(ServiceMenus) || w.opts.Frameless || w.opts.TitleBarOverlay {
		return platform.ErrUnsupported
	}
	menu = cloneMenu(menu)
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.WindowState.Closed() {
		w.menu = menu
		w.menuChanges++
	}
	return nil
}

// Menu returns a copy of the window's menu bar, or nil.
func (w *Window) Menu() []platform.MenuItem {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return cloneMenu(w.menu)
}

// MenuChanges returns how many SetMenu calls the window accepted.
func (w *Window) MenuChanges() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.menuChanges
}

// injectMenu validates and queues a MenuEvent: the user chose an enabled
// item without a submenu of the window's menu bar.
func (w *Window) injectMenu(e platform.MenuEvent) {
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	it, ok := findChoosable(w.menu, e.Item)
	problem := ""
	switch {
	case !ok:
		problem = fmt.Sprintf("the window's menu bar has no item %d (SetMenu first)", e.Item)
	case it.Disabled:
		problem = fmt.Sprintf("menu item %d (%q) is disabled", e.Item, it.Label)
	}
	if problem != "" {
		b.mu.Unlock()
		panic("headless: Inject(MenuEvent): " + problem)
	}
	b.loop.Enqueue(w, e)
	b.mu.Unlock()
	b.signal()
}

// PopupMenu records the menu as a dialog of kind DialogPopupMenu, answered
// like the other dialogs (SetDialogResponder, AnswerDialog) with
// DialogResponse.Item; ServiceMenus unsupported returns
// platform.ErrUnsupported (a Linux backend).
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	if w.Closed() {
		return errors.New("headless: PopupMenu on a closed window")
	}
	return desktop{w.b}.request(Dialog{Kind: DialogPopupMenu, Popup: Popup{Window: w, At: at, Menu: cloneMenu(menu)}}, ServiceMenus,
		func(r DialogResponse) {
			if r.Err != nil {
				done(0, r.Err)
				return
			}
			done(r.Item, nil)
		})
}

// findChoosable finds item id in menu as platform.FindMenuItem does, and
// reports it Disabled when it or any submenu above it is disabled: a real
// backend grays out a disabled submenu's items, so they cannot be chosen.
func findChoosable(menu []platform.MenuItem, id platform.MenuItemID) (platform.MenuItem, bool) {
	for _, it := range menu {
		switch {
		case it.Separator:
		case len(it.Submenu) > 0:
			if found, ok := findChoosable(it.Submenu, id); ok {
				found.Disabled = found.Disabled || it.Disabled
				return found, true
			}
		case it.ID == id:
			return it, true
		}
	}
	return platform.MenuItem{}, false
}
