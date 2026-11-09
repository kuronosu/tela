package platform

import (
	"fmt"
	"strings"
	"unicode"
)

// MenuRole is a standard item or menu that the OS may provide itself
// (MenuItem.Role), with Electron's names: the edit actions, the window
// actions, and the Edit, Window and Help menus of a menu bar.
//
// A role item (RoleUndo to RoleClose) is an ordinary activatable item
// with an ID: a backend that has a native action for the role may carry
// it out itself and not report the choice (macOS's app menu bar: AppKit's
// own items, acting on the key window and its focused field), and every
// other backend reports it like any item (MenuEvent, AppMenuEvent, a
// popup's answer, TrayEvent), so the layers above carry out the role. That
// is why a role item still needs a non-zero ID.
//
// A menu role (RoleEditMenu, RoleWindowMenu, RoleHelpMenu) marks a
// submenu: macOS lists the open windows at the end of the Window menu
// (NSApp.windowsMenu) and adds its search field to the Help menu
// (NSApp.helpMenu); elsewhere it is a plain submenu.
type MenuRole uint8

const (
	// RoleNone is an item of the app's own.
	RoleNone MenuRole = iota
	// RoleUndo and RoleRedo undo and redo the last edit of the focused
	// field.
	RoleUndo
	RoleRedo
	// RoleCut, RoleCopy, RolePaste and RoleSelectAll act on the focused
	// field's selection.
	RoleCut
	RoleCopy
	RolePaste
	RoleSelectAll
	// RoleMinimize minimizes, RoleZoom maximizes or restores (macOS's
	// zoom), and RoleClose asks to close the focused window.
	RoleMinimize
	RoleZoom
	RoleClose
	// RoleEditMenu is the submenu of the edit actions (Electron's
	// editMenu).
	RoleEditMenu
	// RoleWindowMenu is the Window menu: macOS lists the open windows at
	// its end (Electron's windowMenu).
	RoleWindowMenu
	// RoleHelpMenu is the Help menu: macOS adds its search field (Electron's
	// help).
	RoleHelpMenu
)

var menuRoleNames = [...]string{"", "undo", "redo", "cut", "copy", "paste", "selectAll", "minimize", "zoom", "close", "editMenu", "windowMenu", "help"}

// String returns Electron's name of the role ("" for RoleNone).
func (r MenuRole) String() string {
	if int(r) < len(menuRoleNames) {
		return menuRoleNames[r]
	}
	return fmt.Sprintf("MenuRole(%d)", uint8(r))
}

// Valid reports whether r is one of the MenuRole constants.
func (r MenuRole) Valid() bool { return r <= RoleHelpMenu }

// IsMenu reports whether r is the role of a submenu (RoleEditMenu,
// RoleWindowMenu, RoleHelpMenu) rather than of an item.
func (r MenuRole) IsMenu() bool { return r >= RoleEditMenu && r <= RoleHelpMenu }

// CheckAppMenu panics with an actionable message if a SetAppMenu call has
// an invalid menu or app items: [CheckMenu]'s rules over both together
// (an ID is unique across them, since one AppMenuEvent reports either),
// and no menu role among the app items (they go in macOS's app menu).
func CheckAppMenu(menu, appItems []MenuItem) {
	c := menuCheck{seen: map[MenuItemID]bool{}}
	c.walk(menu, "SetAppMenu menu")
	c.appItems = true
	c.walk(appItems, "SetAppMenu appItems")
}

// CheckAccessKey reports whether k can be a [MenuItem].AccessKey: an
// ASCII digit, or a letter of a script that keyboard layouts type
// directly (not Han, Hiragana, Katakana, Hangul, Bopomofo or Yi, which
// come from an IME, nor the fullwidth forms, the kana blocks' common
// letters such as "ー", or the CJK symbols).
func CheckAccessKey(k rune) bool {
	switch {
	case k >= '0' && k <= '9':
		return true
	case !unicode.IsLetter(k), k >= 0xff00 && k <= 0xffef:
		return false
	case k >= 0x3000 && k <= 0x30ff:
		// The CJK symbols and the kana blocks, with the letters of the
		// Common script there: the prolonged sound mark of "コピー".
		return false
	}
	return !unicode.In(k, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo, unicode.Yi)
}

// menuCheck walks the items of a menu for CheckMenu and CheckAppMenu.
type menuCheck struct {
	seen     map[MenuItemID]bool
	menus    [RoleHelpMenu - RoleEditMenu + 1]string // the path of each menu role used
	appItems bool                                    // menu roles are not allowed
}

func (c *menuCheck) walk(items []MenuItem, path string) {
	for i, it := range items {
		at := fmt.Sprintf("%s[%d]", path, i)
		if strings.IndexFunc(it.Accelerator, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 {
			panicf("platform: %s (%q) has Accelerator %q with a control character; it is text such as \"Ctrl+S\"", at, it.Label, it.Accelerator)
		}
		if it.Separator {
			continue
		}
		if k := it.AccessKey; k != 0 && !CheckAccessKey(k) {
			panicf("platform: %s (%q) has AccessKey %q; an access key is a letter or a digit the keyboard types, such as 'F' (an IME language's label gets it appended: \"ファイル(&F)\")", at, it.Label, k)
		}
		if !it.Role.Valid() {
			panicf("platform: %s (%q) has invalid Role %d; use one of the platform.Role… constants", at, it.Label, it.Role)
		}
		if it.Role.IsMenu() {
			switch {
			case c.appItems:
				panicf("platform: %s (%q) has Role %s among the app items; they go in macOS's app menu, which holds no Edit, Window or Help menu: put that submenu in the menu bar", at, it.Label, it.Role)
			case len(it.Submenu) == 0:
				panicf("platform: %s (%q) has Role %s, the role of a submenu; give the item a Submenu with its items", at, it.Label, it.Role)
			case c.menus[it.Role-RoleEditMenu] != "":
				panicf("platform: %s (%q) has Role %s, which %s has already; a menu has one menu of each role", at, it.Label, it.Role, c.menus[it.Role-RoleEditMenu])
			}
			c.menus[it.Role-RoleEditMenu] = at
		}
		switch {
		case len(it.Submenu) > 0:
			if it.Role != RoleNone && !it.Role.IsMenu() {
				panicf("platform: %s (%q) has a Submenu and Role %s, the role of an item; a submenu can only have RoleEditMenu, RoleWindowMenu or RoleHelpMenu", at, it.Label, it.Role)
			}
			c.walk(it.Submenu, at+".Submenu")
		case it.ID == 0:
			panicf("platform: %s (%q) has ID 0; items that can be activated need a non-zero ID (role items too: a backend without the role's native action reports the choice)", at, it.Label)
		case c.seen[it.ID]:
			panicf("platform: %s (%q) reuses ID %d; item IDs must be unique in the whole menu", at, it.Label, it.ID)
		default:
			c.seen[it.ID] = true
		}
	}
}
