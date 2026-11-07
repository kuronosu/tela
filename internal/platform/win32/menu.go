package win32

import (
	"math"
	"slices"
	"unicode"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of window menus (menu_windows.go): the menu bar's
// WM_COMMAND decoding, its access keys and the popup menu's position. The
// entries come from buildMenu (tray.go), shared with tray menus.

// Mnemonics of the menu bar. The contract's labels are plain text (no
// '&'), so a bar gets its access keys from the backend, as every level of
// a native menu has them: each item takes the first letter or digit of its
// label that no earlier item of the same menu took. Windows underlines the
// key when it shows access keys (the "Underline keyboard shortcuts and
// access keys" setting, or after Alt), Alt and the key opens the bar's
// menu, and inside an open menu the key chooses the item.

// isMnemonicRune reports whether r can be an access key: a Latin letter
// (with the accented ones of the Latin blocks) or a digit, which the user
// types with Alt on the layouts that have them. Labels in other scripts get
// none: their layouts do not type them with Alt (native apps add a Latin
// key in parentheses, which a plain label does not have).
func isMnemonicRune(r rune) bool {
	return r < 0x250 && (unicode.IsLetter(r) || (r >= '0' && r <= '9'))
}

// autoMnemonics returns, for each item of a menu level, the byte offset in
// its Label of the character that becomes its access key, or -1 for none
// (separators, disabled items, labels without a usable character, and keys
// already taken by an earlier item). The result is nil for an empty menu.
func autoMnemonics(items []platform.MenuItem) []int {
	if len(items) == 0 {
		return nil
	}
	out := make([]int, len(items))
	taken := map[rune]bool{}
	for i, it := range items {
		out[i] = -1
		if it.Separator || it.Disabled {
			continue
		}
		for off, r := range it.Label {
			if k := unicode.ToLower(r); isMnemonicRune(r) && !taken[k] {
				taken[k], out[i] = true, off
				break
			}
		}
	}
	return out
}

// menuLabelAt is menuLabel with the character at byte offset at marked as
// the mnemonic (an offset below zero marks none).
func menuLabelAt(s string, at int) string {
	if at < 0 || at >= len(s) {
		return menuLabel(s)
	}
	return menuLabel(s[:at]) + "&" + menuLabel(s[at:])
}

// buildBarMenu is buildMenu for a menu bar: every level has access keys,
// and mnemonics lists the keys (lower-cased) of the bar's own items.
func buildBarMenu(items []platform.MenuItem) (entries []menuEntry, cmds []platform.MenuItemID, mnemonics []rune) {
	return buildMenuWith(items, true)
}

// hasMnemonic reports whether Alt and the character c (the WM_SYSCHAR of a
// key, in either case) opens one of the bar's menus.
func hasMnemonic(mnemonics []rune, c uintptr) bool {
	return slices.Contains(mnemonics, unicode.ToLower(rune(c)))
}

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
