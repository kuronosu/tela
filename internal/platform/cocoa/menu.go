package cocoa

import (
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// The pure half of the native menus (menu_darwin.go): the plan of the
// app's menu bar (the app menu Tela always puts first, the app's menus,
// the default bar), of popup menus, the actions of the roles, and
// accelerators as key equivalents. It has no build constraint, so it is
// tested on every OS; menu_darwin.go turns a plan into NSMenus.
//
// Tela's own labels (About, Services, Hide, Hide Others, Show All, Quit,
// the default Edit and Window menus, and the labels of role items given
// without one) are English unless the app gave the backend its own texts
// (platform.Strings, Desktop.SetStrings; ADR-251), which every plan
// function takes as its last argument (nil: English).

// menuSpecial marks a submenu AppKit treats in a special way.
type menuSpecial uint8

const (
	specialNone menuSpecial = iota
	// specialApp is the app menu, the first of the bar (AppKit shows it
	// with the app's name, whatever its title).
	specialApp
	// specialServices is the Services submenu of the app menu
	// (NSApp.servicesMenu, filled by the system).
	specialServices
	// specialEdit is the Edit menu (RoleEditMenu). AppKit adds its own
	// items (Emoji & Symbols, Start Dictation) to the menu it finds by
	// its title, "Edit" or the translation of the user's language; there
	// is no API that names it.
	specialEdit
	// specialWindows is the Window menu (NSApp.windowsMenu), where AppKit
	// lists the open windows.
	specialWindows
	// specialHelp is the Help menu (NSApp.helpMenu), where AppKit adds its
	// search field.
	specialHelp
)

// menuEntry is one item of a planned menu.
type menuEntry struct {
	Title     string
	Separator bool
	// Action is the selector the item sends ("" for a submenu, and for a
	// disabled item, which AppKit's automatic enabling then grays out).
	Action string
	// App items send Action to the backend's menu target with Tag, their
	// ID; the others send it to the first responder (a nil target: the key
	// window's TelaView, the window, NSApp).
	App bool
	Tag platform.MenuItemID
	// Key and Mods are the key equivalent (NSMenuItem keyEquivalent and
	// keyEquivalentModifierMask, NSEventModifierFlags bits); "" none.
	Key  string
	Mods uint64
	// Checked shows a check mark (Checkable items only; the backend never
	// toggles it).
	Checkable, Checked bool
	// HasSubmenu makes the item open Submenu (empty for Services, which
	// the system fills).
	HasSubmenu bool
	Submenu    []menuEntry
	Special    menuSpecial
}

// Selectors of the app items' actions (TelaMenuTarget's methods).
const (
	appItemAction   = "appMenuItem:"
	popupItemAction = "popupMenuItem:"
)

// roleActions are the AppKit actions of the item roles: the standard edit
// actions, sent to the first responder (TelaView), and NSWindow's.
var roleActions = map[platform.MenuRole]string{
	platform.RoleUndo:      "undo:",
	platform.RoleRedo:      "redo:",
	platform.RoleCut:       "cut:",
	platform.RoleCopy:      "copy:",
	platform.RolePaste:     "paste:",
	platform.RoleSelectAll: "selectAll:",
	platform.RoleMinimize:  "performMiniaturize:",
	platform.RoleZoom:      "performZoom:",
	platform.RoleClose:     "performClose:",
}

// roleLabels are the labels of role items and menus given without one
// (Electron's defaults).
var roleLabels = map[platform.MenuRole]string{
	platform.RoleUndo:       "Undo",
	platform.RoleRedo:       "Redo",
	platform.RoleCut:        "Cut",
	platform.RoleCopy:       "Copy",
	platform.RolePaste:      "Paste",
	platform.RoleSelectAll:  "Select All",
	platform.RoleMinimize:   "Minimize",
	platform.RoleZoom:       "Zoom",
	platform.RoleClose:      "Close Window",
	platform.RoleEditMenu:   "Edit",
	platform.RoleWindowMenu: "Window",
	platform.RoleHelpMenu:   "Help",
}

// roleLabelIDs are the IDs of the texts the app can give for the labels
// above (platform.Strings).
var roleLabelIDs = map[platform.MenuRole]string{
	platform.RoleUndo:       platform.StringMenuUndo,
	platform.RoleRedo:       platform.StringMenuRedo,
	platform.RoleCut:        platform.StringMenuCut,
	platform.RoleCopy:       platform.StringMenuCopy,
	platform.RolePaste:      platform.StringMenuPaste,
	platform.RoleSelectAll:  platform.StringMenuSelectAll,
	platform.RoleMinimize:   platform.StringMenuMinimize,
	platform.RoleZoom:       platform.StringMenuZoom,
	platform.RoleClose:      platform.StringMenuClose,
	platform.RoleEditMenu:   platform.StringMenuEdit,
	platform.RoleWindowMenu: platform.StringMenuWindow,
	platform.RoleHelpMenu:   platform.StringMenuHelp,
}

// roleLabel returns the label of a role item given without one: the app's
// text, else Electron's default.
func roleLabel(r platform.MenuRole, s platform.Strings) string {
	if id := roleLabelIDs[r]; id != "" {
		return s.Get(id, roleLabels[r])
	}
	return roleLabels[r]
}

// roleAccelerators are the key equivalents of role items given without an
// Accelerator (Electron's defaults; zoom has none).
var roleAccelerators = map[platform.MenuRole]string{
	platform.RoleUndo:      "Cmd+Z",
	platform.RoleRedo:      "Shift+Cmd+Z",
	platform.RoleCut:       "Cmd+X",
	platform.RoleCopy:      "Cmd+C",
	platform.RolePaste:     "Cmd+V",
	platform.RoleSelectAll: "Cmd+A",
	platform.RoleMinimize:  "Cmd+M",
	platform.RoleClose:     "Cmd+W",
}

// editKeys are the standard key presses of the edit roles under the Mac
// flavor, which TelaView delivers when it performs the role's action (a
// menu click, or the key equivalent of an item whose accelerator is
// another key): what AppKit reports for the key on the US layout.
var editKeys = map[platform.MenuRole]keyboard.MacKey{
	platform.RoleUndo:      {KeyCode: 0x06, Flags: keyboard.MacFlagCommand, Characters: "z", CharactersIgnoringModifiers: "z"},
	platform.RoleRedo:      {KeyCode: 0x06, Flags: keyboard.MacFlagCommand | keyboard.MacFlagShift, Characters: "Z", CharactersIgnoringModifiers: "Z"},
	platform.RoleCut:       {KeyCode: 0x07, Flags: keyboard.MacFlagCommand, Characters: "x", CharactersIgnoringModifiers: "x"},
	platform.RoleCopy:      {KeyCode: 0x08, Flags: keyboard.MacFlagCommand, Characters: "c", CharactersIgnoringModifiers: "c"},
	platform.RolePaste:     {KeyCode: 0x09, Flags: keyboard.MacFlagCommand, Characters: "v", CharactersIgnoringModifiers: "v"},
	platform.RoleSelectAll: {KeyCode: 0x00, Flags: keyboard.MacFlagCommand, Characters: "a", CharactersIgnoringModifiers: "a"},
}

// editRoleOf returns the edit role whose action is sel ("copy:").
func editRoleOf(sel string) (platform.MenuRole, bool) {
	for r := platform.RoleUndo; r <= platform.RoleSelectAll; r++ {
		if roleActions[r] == sel {
			return r, true
		}
	}
	return platform.RoleNone, false
}

// isStandardKey reports whether the key press e is the standard key of
// the edit role r (the same logical key and modifiers): a key equivalent
// that chose the role's item is then delivered as itself.
func isStandardKey(r platform.MenuRole, e platform.KeyEvent) bool {
	std, ok := keyboard.MacKeyDown(editKeys[r])
	return ok && e.Action == platform.KeyPress && e.Key == std.Key && e.Mods == std.Mods
}

// macAccelerator parses an accelerator as ParseAccelerator does under the
// Mac flavor, whatever the host: CmdOrCtrl is Command.
func macAccelerator(s string) (platform.Accelerator, error) {
	// The names are ASCII, so they are found by an ASCII-only fold over s's
	// own bytes: strings.ToLower can change the byte length of other runes
	// (U+0130), which would put an index into the lowered copy elsewhere.
	for _, name := range []string{"commandorcontrol", "cmdorctrl"} {
		for i := 0; i+len(name) <= len(s); {
			if asciiFoldEqual(s[i:i+len(name)], name) {
				s = s[:i] + "Cmd" + s[i+len(name):]
				i += len("Cmd")
				continue
			}
			i++
		}
	}
	return platform.ParseAccelerator(s)
}

// AppKit's function-key characters (NSEvent.h), the key equivalents of
// the named keys.
const (
	nsUpArrowFunctionKey     = 0xF700
	nsDownArrowFunctionKey   = 0xF701
	nsLeftArrowFunctionKey   = 0xF702
	nsRightArrowFunctionKey  = 0xF703
	nsF1FunctionKey          = 0xF704 // F2 to F35 follow
	nsInsertFunctionKey      = 0xF727
	nsDeleteFunctionKey      = 0xF728
	nsHomeFunctionKey        = 0xF729
	nsEndFunctionKey         = 0xF72B
	nsPageUpFunctionKey      = 0xF72C
	nsPageDownFunctionKey    = 0xF72D
	nsPrintScreenFunctionKey = 0xF72E
	nsScrollLockFunctionKey  = 0xF72F
	nsPauseFunctionKey       = 0xF730
	nsHelpFunctionKey        = 0xF746
)

// namedKeyEquivalents are the key equivalents of the named keys that have
// one; the others (media keys, modifiers) have none.
var namedKeyEquivalents = map[platform.Key]rune{
	platform.KeyEnter:       '\r',
	platform.KeyTab:         '\t',
	platform.KeyEscape:      0x1B,
	platform.KeyBackspace:   0x08, // NSBackspaceCharacter
	platform.KeyDelete:      nsDeleteFunctionKey,
	platform.KeyInsert:      nsInsertFunctionKey,
	platform.KeyArrowUp:     nsUpArrowFunctionKey,
	platform.KeyArrowDown:   nsDownArrowFunctionKey,
	platform.KeyArrowLeft:   nsLeftArrowFunctionKey,
	platform.KeyArrowRight:  nsRightArrowFunctionKey,
	platform.KeyHome:        nsHomeFunctionKey,
	platform.KeyEnd:         nsEndFunctionKey,
	platform.KeyPageUp:      nsPageUpFunctionKey,
	platform.KeyPageDown:    nsPageDownFunctionKey,
	platform.KeyPrintScreen: nsPrintScreenFunctionKey,
	platform.KeyScrollLock:  nsScrollLockFunctionKey,
	platform.KeyPause:       nsPauseFunctionKey,
	platform.KeyHelp:        nsHelpFunctionKey,
}

// functionKeys are F1 to F24, whose key equivalents follow
// NSF1FunctionKey.
var functionKeys = [...]platform.Key{
	platform.KeyF1, platform.KeyF2, platform.KeyF3, platform.KeyF4, platform.KeyF5, platform.KeyF6,
	platform.KeyF7, platform.KeyF8, platform.KeyF9, platform.KeyF10, platform.KeyF11, platform.KeyF12,
	platform.KeyF13, platform.KeyF14, platform.KeyF15, platform.KeyF16, platform.KeyF17, platform.KeyF18,
	platform.KeyF19, platform.KeyF20, platform.KeyF21, platform.KeyF22, platform.KeyF23, platform.KeyF24,
}

// keyEquivalent returns the NSMenuItem key equivalent of a: the key's
// character (letters lower case, Shift in the mask, as AppKit matches
// them) or AppKit's function-key character, and the NSEventModifierFlags
// of its modifiers. ok is false for a key without one.
func keyEquivalent(a platform.Accelerator) (key string, mods uint64, ok bool) {
	k := a.Key
	switch {
	case k == "":
		return "", 0, false
	case k.IsNamed():
		r, found := namedKeyEquivalents[k]
		for i, f := range functionKeys {
			if !found && k == f {
				r, found = rune(nsF1FunctionKey+i), true
			}
		}
		if !found {
			return "", 0, false
		}
		key = string(r)
	case utf8.RuneCountInString(string(k)) == 1:
		key = strings.ToLower(string(k))
	default:
		return "", 0, false
	}
	if a.Mods&platform.ModShift != 0 {
		mods |= keyboard.MacFlagShift
	}
	if a.Mods&platform.ModCtrl != 0 {
		mods |= keyboard.MacFlagControl
	}
	if a.Mods&platform.ModAlt != 0 {
		mods |= keyboard.MacFlagOption
	}
	if a.Mods&platform.ModSuper != 0 {
		mods |= keyboard.MacFlagCommand
	}
	return key, mods, true
}

// keyEquivalentMatches reports whether a key down whose
// charactersIgnoringModifiers is chars and whose modifierFlags is flags
// is the key equivalent key with modifier mask mask: the same Shift,
// Control, Option and Command (an upper-case key implies Shift, as in
// AppKit), and the same character, letters without regard to case
// (AppKit reports Shift+Cmd+Z as "Z"). Other flags (Caps Lock, Fn, the
// numeric pad, the device bits) do not count.
func keyEquivalentMatches(key string, mask uint64, chars string, flags uint64) bool {
	if key == "" || chars == "" {
		return false
	}
	const mods = keyboard.MacFlagShift | keyboard.MacFlagControl | keyboard.MacFlagOption | keyboard.MacFlagCommand
	if strings.ToLower(key) != key {
		mask |= keyboard.MacFlagShift
	}
	if mask&mods != flags&mods {
		return false
	}
	return strings.EqualFold(key, chars)
}

// accelKeyEquivalent returns the key equivalent of an item's Accelerator
// text (macAccelerator, then keyEquivalent). Text that does not parse, or
// a key without a key equivalent, gives none: the item shows no shortcut
// (NSMenuItem shows only its key equivalent) and the menu is still made.
func accelKeyEquivalent(s string) (key string, mods uint64) {
	if strings.TrimSpace(s) == "" {
		return "", 0
	}
	a, err := macAccelerator(s)
	if err != nil {
		return "", 0
	}
	key, mods, _ = keyEquivalent(a)
	return key, mods
}

// appMenuBarPlan returns the plan of the app's menu bar: the app menu
// (About, appItems, Services, Hide, Hide Others, Show All, Quit) titled
// name, then menu's items. Role items perform their AppKit action and
// menu roles mark their submenus (see roleEntry); every other activatable
// item is an app item, reported as an AppMenuEvent.
func appMenuBarPlan(name string, menu, appItems []platform.MenuItem, s platform.Strings) []menuEntry {
	bar := []menuEntry{appMenuEntry(name, plan(appItems, false, false, s), s)}
	for _, it := range plan(menu, false, false, s) {
		if !it.HasSubmenu && !it.Separator {
			// A top-level item without a submenu has nothing to show in
			// the menu bar: AppKit shows a top-level item only as the
			// title of its submenu. It gets a submenu of its own.
			it = menuEntry{Title: it.Title, HasSubmenu: true, Submenu: []menuEntry{it}}
		}
		if it.Separator {
			continue
		}
		bar = append(bar, it)
	}
	return bar
}

// defaultMenuBarPlan returns the bar of an app without one of its own
// (SetAppMenu(nil, nil), and from the start): the app menu, an Edit menu
// with the edit roles and a Window menu (Minimize, Zoom, Bring All to
// Front, then AppKit's list of windows), so Cmd+Q, Cmd+H, Cmd+M and the
// edit keys work in every Tela app.
func defaultMenuBarPlan(name string, s platform.Strings) []menuEntry {
	edit := menuEntry{Title: s.Get(platform.StringMenuEdit, "Edit"), HasSubmenu: true, Special: specialEdit, Submenu: []menuEntry{
		roleEntry(platform.RoleUndo, "", "", s),
		roleEntry(platform.RoleRedo, "", "", s),
		{Separator: true},
		roleEntry(platform.RoleCut, "", "", s),
		roleEntry(platform.RoleCopy, "", "", s),
		roleEntry(platform.RolePaste, "", "", s),
		roleEntry(platform.RoleSelectAll, "", "", s),
	}}
	window := menuEntry{Title: s.Get(platform.StringMenuWindow, "Window"), HasSubmenu: true, Special: specialWindows, Submenu: []menuEntry{
		roleEntry(platform.RoleMinimize, "", "", s),
		roleEntry(platform.RoleZoom, "", "", s),
		{Separator: true},
		{Title: s.Get(platform.StringMenuBringAllToFront, "Bring All to Front"), Action: "arrangeInFront:"},
	}}
	return []menuEntry{appMenuEntry(name, nil, s), edit, window}
}

// appMenuEntry returns the app menu with items after About.
func appMenuEntry(name string, items []menuEntry, s platform.Strings) menuEntry {
	sub := []menuEntry{
		{Title: s.Named(platform.StringMenuAbout, "About {name}", name), Action: "orderFrontStandardAboutPanel:"},
		{Separator: true},
	}
	if len(items) > 0 {
		sub = append(append(sub, items...), menuEntry{Separator: true})
	}
	cmd := keyboard.MacFlagCommand
	sub = append(sub,
		menuEntry{Title: s.Get(platform.StringMenuServices, "Services"), HasSubmenu: true, Special: specialServices},
		menuEntry{Separator: true},
		menuEntry{Title: s.Named(platform.StringMenuHide, "Hide {name}", name), Action: "hide:", Key: "h", Mods: cmd},
		menuEntry{Title: s.Get(platform.StringMenuHideOthers, "Hide Others"), Action: "hideOtherApplications:", Key: "h", Mods: cmd | keyboard.MacFlagOption},
		menuEntry{Title: s.Get(platform.StringMenuShowAll, "Show All"), Action: "unhideAllApplications:"},
		menuEntry{Separator: true},
		// terminate: goes through TelaApplication's override: a quit
		// request (quit_darwin.go).
		menuEntry{Title: s.Named(platform.StringMenuQuit, "Quit {name}", name), Action: "terminate:", Key: "q", Mods: cmd},
	)
	return menuEntry{Title: name, HasSubmenu: true, Special: specialApp, Submenu: sub}
}

// popupPlan returns the plan of a popup menu: every activatable item, role
// items included, is an app item answered with its ID (only the app menu
// bar performs roles), and accelerators are shown as key equivalents.
func popupPlan(items []platform.MenuItem, s platform.Strings) []menuEntry {
	return plan(items, true, false, s)
}

// plan converts items. popup makes every item an app item of the popup;
// disabled grays out every item (the items of a disabled submenu: AppKit
// enables a submenu's own item whenever it has a submenu).
func plan(items []platform.MenuItem, popup, disabled bool, s platform.Strings) []menuEntry {
	var out []menuEntry
	for _, it := range items {
		if it.Separator {
			out = append(out, menuEntry{Separator: true})
			continue
		}
		off := disabled || it.Disabled
		label := it.Label
		if label == "" {
			label = roleLabel(it.Role, s)
		}
		if len(it.Submenu) > 0 {
			e := menuEntry{Title: label, HasSubmenu: true, Submenu: plan(it.Submenu, popup, off, s)}
			if !popup {
				switch it.Role {
				case platform.RoleEditMenu:
					e.Special = specialEdit
				case platform.RoleWindowMenu:
					e.Special = specialWindows
				case platform.RoleHelpMenu:
					e.Special = specialHelp
				}
			}
			out = append(out, e)
			continue
		}
		var e menuEntry
		if it.Role != platform.RoleNone && !popup {
			e = roleEntry(it.Role, label, it.Accelerator, s)
		} else {
			e = menuEntry{Title: label, App: true, Tag: it.ID, Action: appItemAction}
			if popup {
				e.Action = popupItemAction
			}
			e.Key, e.Mods = accelKeyEquivalent(it.Accelerator)
		}
		e.Checkable, e.Checked = it.Checkable, it.Checkable && it.Checked
		if off {
			e.Action = ""
		}
		out = append(out, e)
	}
	return out
}

// roleEntry returns the item of an item role: its AppKit action, sent to
// the first responder, with the role's label and key equivalent when the
// item gives none.
func roleEntry(r platform.MenuRole, label, accel string, s platform.Strings) menuEntry {
	if label == "" {
		label = roleLabel(r, s)
	}
	if accel == "" {
		accel = roleAccelerators[r]
	}
	e := menuEntry{Title: label, Action: roleActions[r]}
	e.Key, e.Mods = accelKeyEquivalent(accel)
	return e
}

// asciiFoldEqual reports whether a equals the lower-case ASCII word b,
// ignoring the case of ASCII letters only.
func asciiFoldEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		c := a[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != b[i] {
			return false
		}
	}
	return true
}
