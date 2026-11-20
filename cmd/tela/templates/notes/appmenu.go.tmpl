package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
)

// The app's menu bar (ADR-224, ADR-226). On macOS the menu bar belongs to
// the app, and Notes declares it once with desktop.AppMenuBar: File, Edit,
// View and Window, and Settings… in the app's own menu. Cmd+Q, About, Hide
// and the rest of the app menu are the OS's.
//
// The items do what the buttons and keys of the window do: they call the
// same functions as the title bar's Open and Save, the rail and the
// keyboard handler of the screen. On macOS an accelerator is the item's key
// equivalent, so the key chooses the item and never reaches the window's
// OnKeyDown; on Linux and Windows there is no menu bar, and that same
// handler (screen.go) takes the keys. Notes has no drawn menu bar either.
//
// The bar is NativeOnly: it is the app's menu bar where the platform has
// one (macOS), and nothing elsewhere. Without it, Windows would put it in
// the framed Preferences window, which has no menu bar, and Linux has
// nothing to give it.

// appMenu is the app's menu bar, with the screen's functions for its items.
// It renders nothing.
type appMenu struct {
	// New, Open, Save, Delete and Settings are the functions of the title
	// bar's buttons, the rail and the keys; Show opens the overview of a
	// view of the rail.
	New, Open, Save, Delete, Settings func()
	Show                              func(filter)
	// HasNote is true while a note is open: Save and Delete need one.
	HasNote bool
}

func (m appMenu) Build() tela.Node {
	view := func(label, key string, f filter) desktop.MenuItem {
		return desktop.MenuItem{Label: label, Accelerator: "CmdOrCtrl+" + key, OnSelect: func() { m.Show(f) }}
	}
	return desktop.AppMenuBar{
		AppItems: []desktop.MenuItem{
			{Label: "Settings…", Accelerator: "CmdOrCtrl+,", OnSelect: m.Settings},
		},
		Menus: []desktop.MenuItem{
			{Label: "File", Submenu: []desktop.MenuItem{
				{Label: "New Note", Accelerator: "CmdOrCtrl+N", OnSelect: m.New},
				{Label: "Open…", Accelerator: "CmdOrCtrl+O", OnSelect: m.Open},
				{Label: "Save", Accelerator: "CmdOrCtrl+S", OnSelect: m.Save, Disabled: !m.HasNote},
				{Label: "Delete Note", Accelerator: "CmdOrCtrl+Shift+D", OnSelect: m.Delete, Disabled: !m.HasNote},
				{Separator: true},
				{Role: desktop.RoleClose, Label: "Close Window", Accelerator: "CmdOrCtrl+W"},
			}},
			{Role: desktop.RoleEditMenu},
			{Label: "View", Submenu: []desktop.MenuItem{
				view("All Notes", "1", viewAll),
				view("Pinned", "2", viewPinned),
				view("Unsaved", "3", viewUnsaved),
			}},
			// macOS lists the open windows at the end of the Window menu.
			{Role: desktop.RoleWindowMenu, Submenu: []desktop.MenuItem{
				{Role: desktop.RoleMinimize}, {Role: desktop.RoleZoom},
			}},
		},
		NativeOnly: true,
		OnError:    menuBarError,
	}
}

// menuBarError ignores ErrUnsupported (no menu bar to give: tela snapshot,
// or a desktop without one) and tells the user of the terminal about
// anything else.
func menuBarError(err error) {
	if !errors.Is(err, desktop.ErrUnsupported) {
		fmt.Fprintln(os.Stderr, "notes: menu bar:", err)
	}
}
