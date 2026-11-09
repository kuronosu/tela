package platform

import "strings"

// Strings are the texts a backend shows itself, in the app's language
// (platform contract v8, ADR-251): the layers above translate them and
// give the backend plain strings by ID with [Desktop.SetStrings]. Absent
// IDs are not translated: the backend keeps its own text. A text that
// takes the app's name has "{name}" in it, which the backend replaces.
type Strings map[string]string

// Get returns the text of id, or def when the app gave none (or an empty
// one).
func (s Strings) Get(id, def string) string {
	if t := s[id]; t != "" {
		return t
	}
	return def
}

// Named is Get with "{name}" replaced by name.
func (s Strings) Named(id, def, name string) string {
	return strings.ReplaceAll(s.Get(id, def), "{name}", name)
}

// The IDs of the texts backends read; core's label table (ARCHITECTURE
// section 5, tela.Labels) holds them all with their English defaults, and a
// test keeps the two in step.
const (
	// The buttons of a native message box (Win32, Cocoa).
	StringMessageOK     = "tela.messagebox.ok"
	StringMessageCancel = "tela.messagebox.cancel"
	StringMessageYes    = "tela.messagebox.yes"
	StringMessageNo     = "tela.messagebox.no"

	// The app menu and the standard items of the macOS menu bar. About,
	// Hide and Quit take the app's name.
	StringMenuAbout           = "tela.menu.about"
	StringMenuServices        = "tela.menu.services"
	StringMenuHide            = "tela.menu.hide"
	StringMenuHideOthers      = "tela.menu.hideOthers"
	StringMenuShowAll         = "tela.menu.showAll"
	StringMenuQuit            = "tela.menu.quit"
	StringMenuEdit            = "tela.menu.edit"
	StringMenuWindow          = "tela.menu.window"
	StringMenuHelp            = "tela.menu.help"
	StringMenuUndo            = "tela.menu.undo"
	StringMenuRedo            = "tela.menu.redo"
	StringMenuCut             = "tela.menu.cut"
	StringMenuCopy            = "tela.menu.copy"
	StringMenuPaste           = "tela.menu.paste"
	StringMenuSelectAll       = "tela.menu.selectAll"
	StringMenuMinimize        = "tela.menu.minimize"
	StringMenuZoom            = "tela.menu.zoom"
	StringMenuClose           = "tela.menu.close"
	StringMenuBringAllToFront = "tela.menu.bringAllToFront"

	// The label of the save panel's file type popup (Cocoa).
	StringPanelFormat = "tela.panel.format"
)

// StringIDs lists the IDs above.
var StringIDs = []string{
	StringMessageOK, StringMessageCancel, StringMessageYes, StringMessageNo,
	StringMenuAbout, StringMenuServices, StringMenuHide, StringMenuHideOthers, StringMenuShowAll,
	StringMenuQuit, StringMenuEdit, StringMenuWindow, StringMenuHelp, StringMenuUndo, StringMenuRedo,
	StringMenuCut, StringMenuCopy, StringMenuPaste, StringMenuSelectAll, StringMenuMinimize,
	StringMenuZoom, StringMenuClose, StringMenuBringAllToFront, StringPanelFormat,
}
