package win32

import (
	"math"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of window menus (menu_windows.go): the menu bar's
// WM_COMMAND decoding and the popup menu's position. The entries come from
// buildMenu (tray.go), shared with tray menus.

// menuCommand decodes WM_COMMAND for a menu bar whose command table is
// cmds (buildMenu): a menu sends 0 in the high word of wParam and no
// control handle in lParam (accelerator tables, which Tela does not use,
// send 1; controls send their handle). ok is false for any other command.
func menuCommand(cmds []platform.MenuItemID, wparam, lparam uintptr) (platform.MenuItemID, bool) {
	if hiword(wparam) != 0 || lparam != 0 {
		return 0, false
	}
	cmd := int(loword(wparam))
	if cmd < 1 || cmd > len(cmds) {
		return 0, false
	}
	return cmds[cmd-1], true
}

// popupCommand maps TrackPopupMenuEx's TPM_RETURNCMD result to the chosen
// item: 0 when the menu was dismissed (or the command is unknown).
func popupCommand(cmds []platform.MenuItemID, cmd uintptr) platform.MenuItemID {
	if cmd < 1 || cmd > uintptr(len(cmds)) {
		return 0
	}
	return cmds[cmd-1]
}

// physicalPoint converts a point in logical pixels to physical ones at
// scale, rounded to the nearest pixel (half away from zero, as
// PhysicalSize does).
func physicalPoint(p platform.Point, scale float64) (x, y int32) {
	return int32(math.Round(float64(p.X) * scale)), int32(math.Round(float64(p.Y) * scale))
}
