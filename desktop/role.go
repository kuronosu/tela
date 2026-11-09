package desktop

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Role marks a [MenuItem] as a standard item or menu (ADR-226), with
// Electron's names: the edit actions, the window actions, and the Edit,
// Window and Help menus. The zero value, RoleNone, is an item of the
// app's own.
//
//	desktop.MenuItem{Role: desktop.RoleCopy}       // "Copy", acting on the focused field
//	desktop.MenuItem{Role: desktop.RoleEditMenu}   // "Edit": Undo, Redo, Cut, Copy, Paste, Select All
//	desktop.MenuItem{Role: desktop.RoleMinimize, Label: "Minimizar"}
//
// An item with a Role and no Label gets the role's label ([Role.Label],
// English unless the app's catalogs translate it); a menu role with no Submenu gets the standard items,
// as Electron's role: 'editMenu' and 'windowMenu' do (RoleHelpMenu has no
// standard items: give it a Submenu).
//
// What choosing a role item does depends on the platform. Where the OS has
// the action, it carries it out itself and the choice never reaches the
// app (macOS's app menu bar: AppKit's own items, acting on the key window
// and its focused field). Everywhere else package desktop carries it out:
// minimize, zoom (maximize or restore) and close act on the window the
// menu belongs to (the key window for the app's menu bar and the tray's
// menu), and the edit roles send that window's focused element the
// shortcut of its platform (Ctrl+C, or Command+C under the Mac key
// flavor), so text fields cut, copy, paste, undo and select all as they do
// for the keys. An item's own OnSelect, if set, runs instead, and the
// platform does not perform the role.
type Role uint8

const (
	// RoleNone is an item of the app's own.
	RoleNone Role = iota
	// RoleUndo and RoleRedo undo and redo the last edit of the focused
	// field.
	RoleUndo
	RoleRedo
	// RoleCut, RoleCopy, RolePaste and RoleSelectAll act on the focused
	// field.
	RoleCut
	RoleCopy
	RolePaste
	RoleSelectAll
	// RoleMinimize minimizes the window, RoleZoom maximizes it or restores
	// it, and RoleClose asks to close it as its close button does.
	RoleMinimize
	RoleZoom
	RoleClose
	// RoleEditMenu is the Edit menu (Undo, Redo, Cut, Copy, Paste, Select
	// All unless the item has a Submenu of its own).
	RoleEditMenu
	// RoleWindowMenu is the Window menu (Minimize, Zoom, Close unless the
	// item has a Submenu of its own): macOS lists the open windows at its
	// end.
	RoleWindowMenu
	// RoleHelpMenu is the Help menu: macOS adds its search field to it.
	// It needs a Submenu.
	RoleHelpMenu
)

// String returns Electron's name of the role: "undo", "selectAll",
// "editMenu"... ("" for RoleNone).
func (r Role) String() string { return platform.MenuRole(r).String() }

// Label returns the role's default label: "Undo", "Select All", "Edit"...
// in the app's language ("" for RoleNone). It is the built-in label of
// the role (ADR-251: "tela.menu.undo", "tela.menu.selectAll"... of
// [tela.Labels]), English unless the app's catalogs translate it.
func (r Role) Label() string {
	if int(r) < len(roleLabelIDs) && roleLabelIDs[r] != "" {
		return tela.Label(roleLabelIDs[r])
	}
	return ""
}

// roleLabelIDs are the IDs of the roles' built-in labels.
var roleLabelIDs = [...]string{
	RoleUndo: "tela.menu.undo", RoleRedo: "tela.menu.redo", RoleCut: "tela.menu.cut",
	RoleCopy: "tela.menu.copy", RolePaste: "tela.menu.paste", RoleSelectAll: "tela.menu.selectAll",
	RoleMinimize: "tela.menu.minimize", RoleZoom: "tela.menu.zoom", RoleClose: "tela.menu.close",
	RoleEditMenu: "tela.menu.edit", RoleWindowMenu: "tela.menu.window", RoleHelpMenu: "tela.menu.help",
}

func (r Role) valid() bool  { return platform.MenuRole(r).Valid() }
func (r Role) isMenu() bool { return platform.MenuRole(r).IsMenu() }

// accelerator returns the text the role's item shows at its right: the
// edit roles' shortcut of the platform, "" for the others.
func (r Role) accelerator() string {
	var keys string
	switch r {
	case RoleUndo:
		keys = "CmdOrCtrl+Z"
	case RoleRedo:
		keys = "CmdOrCtrl+Shift+Z"
	case RoleCut:
		keys = "CmdOrCtrl+X"
	case RoleCopy:
		keys = "CmdOrCtrl+C"
	case RolePaste:
		keys = "CmdOrCtrl+V"
	case RoleSelectAll:
		keys = "CmdOrCtrl+A"
	default:
		return ""
	}
	s, err := ParseKeys(keys)
	if err != nil {
		return ""
	}
	return s
}

// standardMenu returns the items of a menu role with no Submenu.
func standardMenu(what string, it MenuItem) []MenuItem {
	switch it.Role {
	case RoleEditMenu:
		return []MenuItem{{Role: RoleUndo}, {Role: RoleRedo}, {Separator: true},
			{Role: RoleCut}, {Role: RoleCopy}, {Role: RolePaste}, {Role: RoleSelectAll}}
	case RoleWindowMenu:
		return []MenuItem{{Role: RoleMinimize}, {Role: RoleZoom}, {Role: RoleClose}}
	}
	panic(fmt.Sprintf("%s: the item %q has Role %v and no Submenu; the Help menu has no standard items: give it its own, such as desktop.MenuItem{Role: desktop.RoleHelpMenu, Submenu: []desktop.MenuItem{{Label: \"Documentation\", OnSelect: openDocs}}}", what, it.Label, it.Role))
}

// menuActions are what choosing the items of a converted menu does: the
// item's OnSelect, or, for a role item without one, its role.
type menuActions struct {
	fn   map[platform.MenuItemID]func()
	role map[platform.MenuItemID]Role
}

// run does what choosing item id does: perform carries out a role.
func (a menuActions) run(id platform.MenuItemID, perform func(Role)) {
	if f := a.fn[id]; f != nil {
		f()
		return
	}
	if r := a.role[id]; r != RoleNone && perform != nil {
		perform(r)
	}
}

// platformMenu returns the platform menu of items, numbering the items
// that can be chosen from 1 in menu order, and what choosing them does.
// An Accelerator with control characters panics (what names the menu).
func platformMenu(what string, items []MenuItem) ([]platform.MenuItem, menuActions) {
	ms, actions := platformMenus(what, items)
	return ms[0], actions
}

// platformMenus converts several lists of items that share one numbering
// (the app's menu bar and its app items: an AppMenuEvent reports an item
// of either).
func platformMenus(what string, lists ...[]MenuItem) ([][]platform.MenuItem, menuActions) {
	actions := menuActions{fn: map[platform.MenuItemID]func(){}, role: map[platform.MenuItemID]Role{}}
	var next platform.MenuItemID
	var conv func(items []MenuItem) []platform.MenuItem
	conv = func(items []MenuItem) []platform.MenuItem {
		if len(items) == 0 {
			return nil
		}
		out := make([]platform.MenuItem, len(items))
		for i, it := range items {
			if strings.IndexFunc(it.Accelerator, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 {
				panic(fmt.Sprintf("%s: the item %q has the Accelerator %q with a control character; it is the text shown, such as \"Ctrl+S\"", what, it.Label, it.Accelerator))
			}
			if !it.Role.valid() {
				panic(fmt.Sprintf("%s: the item %q has the invalid Role %d; use one of the desktop.Role… constants", what, it.Label, it.Role))
			}
			label := it.Label
			if label == "" {
				label = it.Role.Label()
			}
			switch {
			case it.Separator:
				out[i] = platform.MenuItem{Separator: true}
			case it.Role.isMenu():
				sub := it.Submenu
				if len(sub) == 0 {
					sub = standardMenu(what, it)
				}
				out[i] = platform.MenuItem{Label: label, Disabled: it.Disabled, Role: platform.MenuRole(it.Role), Submenu: conv(sub)}
			case len(it.Submenu) > 0:
				if it.Role != RoleNone {
					panic(fmt.Sprintf("%s: the item %q has Role %v, the role of an item, and a Submenu; a submenu can only have RoleEditMenu, RoleWindowMenu or RoleHelpMenu", what, it.Label, it.Role))
				}
				out[i] = platform.MenuItem{Label: label, Disabled: it.Disabled, Submenu: conv(it.Submenu)}
			default:
				next++
				accel := it.Accelerator
				if accel == "" {
					accel = it.Role.accelerator()
				}
				pi := platform.MenuItem{ID: next, Label: label, Accelerator: accel, Checkable: it.Checkbox, Checked: it.Checked, Disabled: it.Disabled}
				switch {
				case it.OnSelect != nil:
					// The item's own action replaces the role: the platform
					// must not perform it either.
					actions.fn[next] = it.OnSelect
				case it.Role != RoleNone:
					pi.Role = platform.MenuRole(it.Role)
					actions.role[next] = it.Role
				}
				out[i] = pi
			}
		}
		return out
	}
	out := make([][]platform.MenuItem, len(lists))
	for i, l := range lists {
		out[i] = conv(l)
	}
	return out, actions
}

// PerformRole carries out the item role r in window w, as package desktop
// does for a native menu whose platform only reports the choice:
// RoleMinimize, RoleZoom and RoleClose act on w, and the edit roles send
// the element of w that has the focus the role's shortcut (Ctrl+C, or
// Command+C under the Mac key flavor), so a text field cuts, copies,
// pastes, undoes and selects all as it does for the keys. It is for menus
// that Tela draws, such as kit's menus with a Role item: call it from a
// handler or an effect once the menu has closed and the focus is back on
// the element the command is for. RoleNone and the menu roles
// (RoleEditMenu, RoleWindowMenu, RoleHelpMenu) do nothing; so do the edit
// roles in a tree without a running app (tela snapshot).
func PerformRole(w tela.WindowHandle, r Role) {
	if r == RoleNone || !r.valid() || r.isMenu() {
		return
	}
	if d, ok := core.DesktopOf(w); ok {
		performRole(d, w, r)
		return
	}
	switch r {
	case RoleMinimize:
		w.Minimize()
	case RoleZoom:
		w.ToggleMaximize()
	case RoleClose:
		w.Close()
	}
}

// performRole carries out the role r that the platform reported instead of
// performing it, in window w of d's app.
func performRole(d core.Desktop, w tela.WindowHandle, r Role) {
	switch r {
	case RoleMinimize:
		w.Minimize()
	case RoleZoom:
		w.ToggleMaximize()
	case RoleClose:
		w.Close()
	case RoleUndo:
		pressEdit(d, w, "KeyZ", "z", 0)
	case RoleRedo:
		pressEdit(d, w, "KeyZ", "Z", platform.ModShift)
	case RoleCut:
		pressEdit(d, w, "KeyX", "x", 0)
	case RoleCopy:
		pressEdit(d, w, "KeyC", "c", 0)
	case RolePaste:
		pressEdit(d, w, "KeyV", "v", 0)
	case RoleSelectAll:
		pressEdit(d, w, "KeyA", "a", 0)
	}
}

// pressEdit sends w's focused element the shortcut of an edit role with
// the primary modifier of the window's flavor.
func pressEdit(d core.Desktop, w tela.WindowHandle, code, key string, extra platform.Modifiers) {
	c, ok := platform.ParseKeyCode(code)
	if !ok {
		return
	}
	d.PressKey(w, c, platform.Key(key), d.PrimaryModifier(w)|extra)
}

// roleIn returns the performer of the roles of a menu that belongs to
// window w: the window itself.
func roleIn(w tela.WindowHandle) func(Role) {
	return func(r Role) {
		if d, ok := core.DesktopOf(w); ok {
			performRole(d, w, r)
		}
	}
}

// roleInKey returns the performer of the roles of an app-wide menu (the
// app's menu bar, a tray's menu): the key window of the app that window w
// belongs to.
func roleInKey(w tela.WindowHandle) func(Role) {
	return func(r Role) {
		if d, ok := core.DesktopOf(w); ok {
			performRole(d, d.KeyWindow(), r)
		}
	}
}
